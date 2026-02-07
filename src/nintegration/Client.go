package nintegration

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	//"github.com/cassitly/neuro-integration-sdk"
	"github.com/recassity/neuro-relay/src/nbackend"
	"github.com/gorilla/websocket"
	"net/url"
	//"time"
)

/* =========================
   Integration Client - Fixed Version
   Handles Neuro connection manually to preserve action IDs
   ========================= */

type IntegrationClient struct {
	neuroConn *websocket.Conn // Direct WebSocket connection to Neuro
	backend   *nbackend.EmulationBackend

	// Track which actions belong to which game  
	actionToGame map[string]string // Maps "game-a/buy_books" -> "game-a"
	actionMu     sync.RWMutex

	// Track action IDs: Neuro ID -> Game ID
	actionIDToGame map[string]string
	actionIDMu     sync.RWMutex

	config        IntegrationClientConfig
	closeChan     chan struct{}
	registeredActions map[string]nbackend.ActionDefinition
	actionsMu     sync.RWMutex
	
	// Mutex to protect WebSocket writes (gorilla/websocket is not thread-safe)
	sendMu        sync.Mutex
}

type IntegrationClientConfig struct {
	RelayName    string
	NeuroURL     string
	EmulatedAddr string
}

func NewIntegrationClient(config IntegrationClientConfig) (*IntegrationClient, error) {
	backend := nbackend.NewEmulationBackend()

	ic := &IntegrationClient{
		backend:           backend,
		actionToGame:      make(map[string]string),
		actionIDToGame:    make(map[string]string),
		registeredActions: make(map[string]nbackend.ActionDefinition),
		closeChan:         make(chan struct{}),
		config:            config,
	}

	ic.setupBackendCallbacks()
	return ic, nil
}

func (ic *IntegrationClient) setupBackendCallbacks() {
	ic.backend.OnStartup = func(gameID string, gameName string) {
		log.Printf("Game started: %s (%s)", gameName, gameID)
		ic.sendContextToNeuro("Game '"+gameName+"' connected to relay", true)
	}

	ic.backend.OnActionRegistered = func(gameID string, actionName string, action nbackend.ActionDefinition) {
		ic.actionMu.Lock()
		ic.actionToGame[actionName] = gameID
		ic.actionsMu.Lock()
		ic.registeredActions[actionName] = action
		ic.actionsMu.Unlock()
		ic.actionMu.Unlock()

		log.Printf("Registering action with Neuro: %s", actionName)
		
		// Send register message to Neuro
		ic.sendToNeuro(map[string]interface{}{
			"command": "actions/register",
			"game":    ic.config.RelayName,
			"data": map[string]interface{}{
				"actions": []map[string]interface{}{
					{
						"name":        actionName,
						"description": action.Description,
						"schema":      action.Schema,
					},
				},
			},
		})
	}

	ic.backend.OnActionUnregistered = func(gameID string, actionName string) {
		ic.actionMu.Lock()
		delete(ic.actionToGame, actionName)
		ic.actionsMu.Lock()
		delete(ic.registeredActions, actionName)
		ic.actionsMu.Unlock()
		ic.actionMu.Unlock()

		log.Printf("Unregistering action from Neuro: %s", actionName)
		
		ic.sendToNeuro(map[string]interface{}{
			"command": "actions/unregister",
			"game":    ic.config.RelayName,
			"data": map[string]interface{}{
				"action_names": []string{actionName},
			},
		})
	}

	ic.backend.OnContext = func(gameID string, message string, silent bool) {
		prefixedMessage := "[" + gameID + "] " + message
		log.Printf("Forwarding context to Neuro: %s (silent: %v)", prefixedMessage, silent)
		ic.sendContextToNeuro(prefixedMessage, silent)
	}

	ic.backend.OnActionResult = func(gameID string, actionID string, success bool, message string) {
		log.Printf("Received action result from %s: id=%s, success=%v", gameID, actionID, success)
		log.Printf("Forwarding action result to Neuro: id=%s, success=%v, message=%s", actionID, success, message)
		
		// Send result to Neuro with the SAME action ID
		ic.sendToNeuro(map[string]interface{}{
			"command": "action/result",
			"game":    ic.config.RelayName,
			"data": map[string]interface{}{
				"id":      actionID,
				"success": success,
				"message": message,
			},
		})

		// Clean up tracking
		ic.actionIDMu.Lock()
		delete(ic.actionIDToGame, actionID)
		ic.actionIDMu.Unlock()
	}

	ic.backend.OnActionForce = func(gameID string, state string, query string, ephemeralContext bool, priority string, actionNames []string) {
		log.Printf("Forwarding action force to Neuro from %s: %v", gameID, actionNames)
		
		prefixedQuery := "[" + gameID + "] " + query

		data := map[string]interface{}{
			"query":             prefixedQuery,
			"action_names":      actionNames,
			"ephemeral_context": ephemeralContext,
			"priority":          priority,
		}
		
		if state != "" {
			data["state"] = state
		}

		ic.sendToNeuro(map[string]interface{}{
			"command": "actions/force",
			"game":    ic.config.RelayName,
			"data":    data,
		})
	}
}

func (ic *IntegrationClient) Start() error {
	// Start emulated backend
	go func() {
		if err := ic.backend.Start(ic.config.EmulatedAddr); err != nil {
			log.Fatalf("Emulated backend failed: %v", err)
		}
	}()

	// Connect to Neuro manually
	u, err := url.Parse(ic.config.NeuroURL)
	if err != nil {
		return fmt.Errorf("invalid neuro URL: %w", err)
	}

	log.Printf("Connecting to %s...", u.String())

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to connect to Neuro: %w", err)
	}

	ic.neuroConn = conn
	log.Println("WebSocket connection established")

	// Send startup
	log.Println("Sending startup message...")
	if err := ic.sendToNeuro(map[string]interface{}{
		"command": "startup",
		"game":    ic.config.RelayName,
	}); err != nil {
		return fmt.Errorf("failed to send startup: %w", err)
	}
	log.Println("Startup message sent successfully")

	// Start message handler
	go ic.handleNeuroMessages()

	log.Printf("NeuroRelay started:")
	log.Printf("  - Emulated backend: ws://%s/", ic.config.EmulatedAddr)
	log.Printf("  - Connected to Neuro as: %s", ic.config.RelayName)

	return nil
}

func (ic *IntegrationClient) handleNeuroMessages() {
	log.Println("Read loop started")
	for {
		select {
		case <-ic.closeChan:
			log.Println("Read loop stopping")
			return
		default:
			_, msgBytes, err := ic.neuroConn.ReadMessage()
			if err != nil {
				log.Printf("Read error: %v", err)
				return
			}

			log.Printf("Received message: %s", string(msgBytes))

			var msg map[string]interface{}
			if err := json.Unmarshal(msgBytes, &msg); err != nil {
				log.Printf("Failed to parse message: %v", err)
				continue
			}

			cmd, _ := msg["command"].(string)
			log.Printf("Received command: %s", cmd)

			switch cmd {
			case "action":
				ic.handleActionFromNeuro(msg)
			case "actions/reregister_all":
				log.Println("Received reregister_all request")
				ic.reregisterAllActions()
			default:
				log.Printf("Unhandled command: %s", cmd)
			}
		}
	}
}

func (ic *IntegrationClient) handleActionFromNeuro(msg map[string]interface{}) {
	data, ok := msg["data"].(map[string]interface{})
	if !ok {
		log.Println("Invalid action message: missing data")
		return
	}

	actionID, _ := data["id"].(string)
	actionName, _ := data["name"].(string)
	actionData, _ := data["data"].(string)

	log.Printf("Handling action: %s (ID: %s)", actionName, actionID)

	// Find which game this action belongs to
	ic.actionMu.RLock()
	gameID, exists := ic.actionToGame[actionName]
	ic.actionMu.RUnlock()

	if !exists {
		log.Printf("Unknown action: %s", actionName)
		ic.sendActionResult(actionID, false, "Unknown action: "+actionName)
		return
	}

	// Track this action ID
	ic.actionIDMu.Lock()
	ic.actionIDToGame[actionID] = gameID
	ic.actionIDMu.Unlock()

	log.Printf("Executing relayed action: %s (id: %s, game: %s)", actionName, actionID, gameID)

	// Forward to game with THE SAME action ID
	// The backend will handle sending the result if the game is disconnected
	if err := ic.backend.SendAction(gameID, actionID, actionName, actionData); err != nil {
		log.Printf("Failed to send action to game: %v", err)
		
		// IMPORTANT: Don't send duplicate results here!
		// The backend's SendAction already calls OnActionResult callback
		// if the game is disconnected, so we just need to clean up our tracking
		
		ic.actionIDMu.Lock()
		delete(ic.actionIDToGame, actionID)
		ic.actionIDMu.Unlock()
	}
}

func (ic *IntegrationClient) reregisterAllActions() {
	ic.actionsMu.RLock()
	actions := make([]map[string]interface{}, 0, len(ic.registeredActions))
	for name, action := range ic.registeredActions {
		actions = append(actions, map[string]interface{}{
			"name":        name,
			"description": action.Description,
			"schema":      action.Schema,
		})
	}
	ic.actionsMu.RUnlock()

	if len(actions) > 0 {
		log.Printf("Re-registering %d action(s)", len(actions))
		ic.sendToNeuro(map[string]interface{}{
			"command": "actions/register",
			"game":    ic.config.RelayName,
			"data": map[string]interface{}{
				"actions": actions,
			},
		})
	}
}

func (ic *IntegrationClient) sendToNeuro(msg map[string]interface{}) error {
	// CRITICAL FIX: Protect WebSocket writes with mutex
	// gorilla/websocket is NOT thread-safe for concurrent writes
	ic.sendMu.Lock()
	defer ic.sendMu.Unlock()
	
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	cmd, _ := msg["command"].(string)
	log.Printf("Sending: %s - %s", cmd, string(msgBytes))

	return ic.neuroConn.WriteMessage(websocket.TextMessage, msgBytes)
}

func (ic *IntegrationClient) sendActionResult(id string, success bool, message string) {
	ic.sendToNeuro(map[string]interface{}{
		"command": "action/result",
		"game":    ic.config.RelayName,
		"data": map[string]interface{}{
			"id":      id,
			"success": success,
			"message": message,
		},
	})
}

func (ic *IntegrationClient) sendContextToNeuro(message string, silent bool) {
	ic.sendToNeuro(map[string]interface{}{
		"command": "context",
		"game":    ic.config.RelayName,
		"data": map[string]interface{}{
			"message": message,
			"silent":  silent,
		},
	})
}

func (ic *IntegrationClient) Stop() error {
	log.Println("Shutting down NeuroRelay...")
	close(ic.closeChan)
	if ic.neuroConn != nil {
		return ic.neuroConn.Close()
	}
	return nil
}

func (ic *IntegrationClient) GetConnectedGames() map[string]string {
	return ic.backend.GetAllSessions()
}

func (ic *IntegrationClient) IsBackendLocked() bool {
	return ic.backend.IsLocked()
}