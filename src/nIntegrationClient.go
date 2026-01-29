package nintegration

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/cassitly/neuro-integration-sdk"
	"github.com/recassity/src/nbackend"
)

/* =========================
   Integration Client
   ========================= */

// IntegrationClient manages the connection to the real Neuro backend
// and coordinates with the emulated backend
type IntegrationClient struct {
	neuroClient *neuro.Client
	backend     *nbackend.EmulationBackend

	// Track which actions belong to which game
	actionToGame map[string]string // Maps "game-a/buy_books" -> "game-a"
	actionMu     sync.RWMutex

	// Track action IDs and their corresponding games
	actionIDToGame map[string]string // Maps action ID -> game ID
	actionIDMu     sync.RWMutex

	config IntegrationClientConfig
}

type IntegrationClientConfig struct {
	RelayName    string // Name shown to Neuro (e.g., "Game Hub")
	NeuroURL     string // Neuro backend WebSocket URL
	EmulatedAddr string // Address for the emulated backend (e.g., "127.0.0.1:8001")
}

/* =========================
   Constructor
   ========================= */

func NewIntegrationClient(config IntegrationClientConfig) (*IntegrationClient, error) {
	// Create emulation backend
	backend := nbackend.NewEmulationBackend()

	// Create Neuro client
	neuroClient, err := neuro.NewClient(neuro.ClientConfig{
		Game:         config.RelayName,
		WebsocketURL: config.NeuroURL,
	})
	if err != nil {
		return nil, err
	}

	ic := &IntegrationClient{
		neuroClient:    neuroClient,
		backend:        backend,
		actionToGame:   make(map[string]string),
		actionIDToGame: make(map[string]string),
		config:         config,
	}

	// Set up backend callbacks
	ic.setupBackendCallbacks()

	return ic, nil
}

/* =========================
   Setup
   ========================= */

func (ic *IntegrationClient) setupBackendCallbacks() {
	// Called when a game sends startup
	ic.backend.OnStartup = func(gameID string, gameName string) {
		log.Printf("Game started: %s (%s)", gameName, gameID)
		// Send context to Neuro about the new game
		ic.neuroClient.SendContext("Game '"+gameName+"' connected to relay", true)
	}

	// Called when a game registers an action
	ic.backend.OnActionRegistered = func(gameID string, actionName string, action nbackend.ActionDefinition) {
		// actionName is already prefixed: "game-a/buy_books"
		ic.actionMu.Lock()
		ic.actionToGame[actionName] = gameID
		ic.actionMu.Unlock()

		log.Printf("Registering action with Neuro: %s", actionName)

		// Create action handler for Neuro
		handler := &RelayActionHandler{
			name:        actionName,
			description: action.Description,
			schema:      action.Schema,
			gameID:      gameID,
			client:      ic,
		}

		// Register with Neuro
		if err := ic.neuroClient.RegisterAction(handler); err != nil {
			log.Printf("Failed to register action %s: %v", actionName, err)
		}
	}

	// Called when a game unregisters an action
	ic.backend.OnActionUnregistered = func(gameID string, actionName string) {
		ic.actionMu.Lock()
		delete(ic.actionToGame, actionName)
		ic.actionMu.Unlock()

		log.Printf("Unregistering action from Neuro: %s", actionName)

		// Unregister from Neuro
		if err := ic.neuroClient.UnregisterAction(actionName); err != nil {
			log.Printf("Failed to unregister action %s: %v", actionName, err)
		}
	}

	// Called when a game sends context
	ic.backend.OnContext = func(gameID string, message string, silent bool) {
		// Prefix the context with the game name for clarity
		prefixedMessage := "[" + gameID + "] " + message
		log.Printf("Forwarding context to Neuro: %s (silent: %v)", prefixedMessage, silent)

		if err := ic.neuroClient.SendContext(prefixedMessage, silent); err != nil {
			log.Printf("Failed to send context: %v", err)
		}
	}

	// Called when a game sends action result
	ic.backend.OnActionResult = func(gameID string, actionID string, success bool, message string) {
		log.Printf("Forwarding action result to Neuro: id=%s, success=%v", actionID, success)

		if err := ic.neuroClient.SendActionResult(actionID, success, message); err != nil {
			log.Printf("Failed to send action result: %v", err)
		}

		// Clean up action ID tracking
		ic.actionIDMu.Lock()
		delete(ic.actionIDToGame, actionID)
		ic.actionIDMu.Unlock()
	}

	// Called when a game forces actions
	ic.backend.OnActionForce = func(gameID string, state string, query string, ephemeralContext bool, priority string, actionNames []string) {
		log.Printf("Forwarding action force to Neuro from %s: %v", gameID, actionNames)

		// Convert priority string to enum
		var prio neuro.Priority
		switch priority {
		case "critical":
			prio = neuro.PriorityCritical
		case "high":
			prio = neuro.PriorityHigh
		case "medium":
			prio = neuro.PriorityMedium
		default:
			prio = neuro.PriorityLow
		}

		// Prefix the query with game context
		prefixedQuery := "[" + gameID + "] " + query

		// Build force options
		opts := []neuro.ForceOption{
			neuro.WithPriority(prio),
			neuro.WithEphemeralContext(ephemeralContext),
		}

		if state != "" {
			opts = append(opts, neuro.WithState(state))
		}

		if err := ic.neuroClient.ForceActions(prefixedQuery, actionNames, opts...); err != nil {
			log.Printf("Failed to force actions: %v", err)
		}
	}
}

/* =========================
   Start/Stop
   ========================= */

func (ic *IntegrationClient) Start() error {
	// Start the emulated backend server
	go func() {
		if err := ic.backend.Start(ic.config.EmulatedAddr); err != nil {
			log.Fatalf("Emulated backend failed: %v", err)
		}
	}()

	// Connect to real Neuro backend
	if err := ic.neuroClient.Connect(); err != nil {
		return err
	}

	log.Printf("NeuroRelay started:")
	log.Printf("  - Emulated backend: ws://%s/ws", ic.config.EmulatedAddr)
	log.Printf("  - Connected to Neuro as: %s", ic.config.RelayName)

	// Handle errors from Neuro client
	go func() {
		for err := range ic.neuroClient.Errors() {
			log.Printf("Neuro client error: %v", err)
		}
	}()

	return nil
}

func (ic *IntegrationClient) Stop() error {
	log.Println("Shutting down NeuroRelay...")
	return ic.neuroClient.Close()
}

/* =========================
   Relay Action Handler
   ========================= */

// RelayActionHandler implements neuro.ActionHandler for relayed actions
type RelayActionHandler struct {
	name        string
	description string
	schema      map[string]interface{}
	gameID      string
	client      *IntegrationClient
}

func (h *RelayActionHandler) GetName() string {
	return h.name
}

func (h *RelayActionHandler) GetDescription() string {
	return h.description
}

func (h *RelayActionHandler) GetSchema() *neuro.ActionSchema {
	if h.schema == nil {
		return nil
	}

	// Convert map to ActionSchema
	schemaType, _ := h.schema["type"].(string)
	properties, _ := h.schema["properties"].(map[string]interface{})
	
	var required []string
	if req, ok := h.schema["required"].([]interface{}); ok {
		required = make([]string, len(req))
		for i, r := range req {
			required[i], _ = r.(string)
		}
	}

	return &neuro.ActionSchema{
		Type:       schemaType,
		Properties: properties,
		Required:   required,
	}
}

func (h *RelayActionHandler) Validate(data json.RawMessage) (interface{}, neuro.ExecutionResult) {
	// For relay actions, we don't validate here - we just pass through to the game
	// The game will validate and return the result

	// Store the action data to send to the game
	var actionData interface{}
	if len(data) > 0 {
		json.Unmarshal(data, &actionData)
	}

	return actionData, neuro.NewSuccessResult("Action forwarded to game")
}

func (h *RelayActionHandler) Execute(state interface{}) {
	// Generate a unique action ID
	actionID := h.generateActionID()

	// Track which game this action belongs to
	h.client.actionIDMu.Lock()
	h.client.actionIDToGame[actionID] = h.gameID
	h.client.actionIDMu.Unlock()

	log.Printf("Executing relayed action: %s (id: %s, game: %s)", h.name, actionID, h.gameID)

	// Convert state to JSON string (as per Neuro API spec)
	var dataStr string
	if state != nil {
		dataBytes, err := json.Marshal(state)
		if err != nil {
			log.Printf("Failed to marshal action data: %v", err)
			return
		}
		dataStr = string(dataBytes)
	}

	// Send action to the appropriate game via emulated backend
	if err := h.client.backend.SendAction(h.gameID, actionID, h.name, dataStr); err != nil {
		log.Printf("Failed to send action to game: %v", err)
	}
}

func (h *RelayActionHandler) generateActionID() string {
	// Simple action ID generation - in production, use UUID or similar
	return h.gameID + "_" + h.name + "_" + string(rune(time.Now().UnixNano()))
}

/* =========================
   Utility Methods
   ========================= */

func (ic *IntegrationClient) GetConnectedGames() map[string]string {
	return ic.backend.GetAllSessions()
}

func (ic *IntegrationClient) IsBackendLocked() bool {
	return ic.backend.IsLocked()
}
