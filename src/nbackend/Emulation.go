package nbackend

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/recassity/src/utils"
)

/* =========================
   Configuration
   ========================= */

const (
	CurrentNRelayVersion = "1.0.0"
)

/* =========================
   Neuro protocol structures
   ========================= */

type ClientMessage struct {
	Command string                 `json:"command"`
	Game    string                 `json:"game,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

type ServerMessage struct {
	Command string                 `json:"command"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

type ActionDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Schema      map[string]interface{} `json:"schema,omitempty"`
}

/* =========================
   Backend state per client
   ========================= */

type GameSession struct {
	GameName         string
	GameID           string // Normalized game identifier (e.g., "game-a")
	LatestActionNum  int
	Actions          map[string]ActionDefinition // Key: original action name
	NRelayCompatible bool
	NRelayVersion    string
	Client           *utilities.Client
}

/* =========================
   Emulation Backend
   ========================= */

type EmulationBackend struct {
	server     *utilities.Server
	sessions   map[*utilities.Client]*GameSession
	sessionsMu sync.RWMutex

	// Lock state - when a non-nrelay compatible integration connects
	locked         bool
	lockedToClient *utilities.Client
	lockMu         sync.RWMutex

	// Callbacks for integration client
	OnStartup            func(gameID string, gameName string)
	OnActionRegistered   func(gameID string, actionName string, action ActionDefinition)
	OnActionUnregistered func(gameID string, actionName string)
	OnContext            func(gameID string, message string, silent bool)
	OnActionResult       func(gameID string, actionID string, success bool, message string)
	OnActionForce        func(gameID string, state string, query string, ephemeralContext bool, priority string, actionNames []string)
}

/* =========================
   Constructor
   ========================= */

func NewEmulationBackend() *EmulationBackend {
	eb := &EmulationBackend{
		sessions: make(map[*utilities.Client]*GameSession),
		locked:   false,
	}

	// Create websocket server with message handler
	eb.server = utilities.New(eb.messageHandler)

	return eb
}

/* =========================
   Server Management
   ========================= */

func (eb *EmulationBackend) Attach(mux *http.ServeMux, path string) {
	eb.server.Attach(mux, path)
}

func (eb *EmulationBackend) Start(addr string) error {
	mux := http.NewServeMux()
	eb.Attach(mux, "/ws")
	log.Printf("Neuro backend emulation listening on ws://%s/ws\n", addr)
	return http.ListenAndServe(addr, mux)
}

/* =========================
   Message handler
   ========================= */

func (eb *EmulationBackend) messageHandler(c *utilities.Client, _ int, raw []byte) {
	var msg ClientMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		log.Println("invalid JSON:", err)
		return
	}

	switch msg.Command {
	case "startup":
		eb.handleStartup(c, msg)

	case "context":
		eb.handleContext(c, msg)

	case "actions/register":
		eb.handleRegisterActions(c, msg)

	case "actions/unregister":
		eb.handleUnregisterActions(c, msg)

	case "actions/force":
		eb.handleForceActions(c, msg)

	case "action/result":
		eb.handleActionResult(c, msg)

	default:
		log.Printf("unknown command: %s", msg.Command)
	}
}

/* =========================
   Command handlers
   ========================= */

func (eb *EmulationBackend) handleStartup(c *utilities.Client, msg ClientMessage) {
	// Check for nrelay-compatible field
	nrelayCompatible := false
	nrelayVersion := ""

	if msg.Data != nil {
		if version, ok := msg.Data["nrelay-compatible"].(string); ok {
			nrelayCompatible = true
			nrelayVersion = version
		}
	}

	eb.lockMu.Lock()
	defer eb.lockMu.Unlock()

	// If backend is locked and this is a new nrelay-compatible integration trying to connect
	if eb.locked && eb.lockedToClient != c {
		if nrelayCompatible {
			// Send nrelay/locked message
			eb.sendError(c, "nrelay/locked", "A non-NeuroRelay compatible integration is currently connected")
			log.Printf("Rejected nrelay-compatible integration (backend locked to non-compatible integration)")
			return
		}
		// If it's also non-compatible, reject it too
		eb.sendError(c, "nrelay/locked", "Another integration is currently connected")
		log.Printf("Rejected non-compatible integration (backend already locked)")
		return
	}

	// If not nrelay-compatible, lock the backend to this client
	if !nrelayCompatible {
		eb.locked = true
		eb.lockedToClient = c
		log.Printf("Backend locked to non-NeuroRelay compatible integration: %s", msg.Game)
	} else {
		log.Printf("NeuroRelay compatible integration connected: %s (version %s)", msg.Game, nrelayVersion)
	}

	// Generate game ID from game name
	gameID := eb.normalizeGameName(msg.Game)

	eb.sessionsMu.Lock()
	eb.sessions[c] = &GameSession{
		GameName:         msg.Game,
		GameID:           gameID,
		LatestActionNum:  0,
		Actions:          make(map[string]ActionDefinition),
		NRelayCompatible: nrelayCompatible,
		NRelayVersion:    nrelayVersion,
		Client:           c,
	}
	eb.sessionsMu.Unlock()

	log.Printf("Startup from game: %s (ID: %s)", msg.Game, gameID)

	// Notify integration client
	if eb.OnStartup != nil {
		eb.OnStartup(gameID, msg.Game)
	}
}

func (eb *EmulationBackend) handleContext(c *utilities.Client, msg ClientMessage) {
	eb.sessionsMu.RLock()
	session := eb.sessions[c]
	eb.sessionsMu.RUnlock()

	if session == nil {
		log.Println("Context received from unknown session")
		return
	}

	message, _ := msg.Data["message"].(string)
	silent, _ := msg.Data["silent"].(bool)

	log.Printf("Context from %s (silent: %v): %s", session.GameID, silent, message)

	// Notify integration client
	if eb.OnContext != nil {
		eb.OnContext(session.GameID, message, silent)
	}
}

func (eb *EmulationBackend) handleRegisterActions(c *utilities.Client, msg ClientMessage) {
	eb.sessionsMu.RLock()
	session := eb.sessions[c]
	eb.sessionsMu.RUnlock()

	if session == nil {
		log.Println("Register actions received from unknown session")
		return
	}

	rawActions, ok := msg.Data["actions"].([]interface{})
	if !ok {
		log.Println("Invalid actions data format")
		return
	}

	for _, a := range rawActions {
		b, _ := json.Marshal(a)
		var action ActionDefinition
		if err := json.Unmarshal(b, &action); err != nil {
			log.Printf("Failed to parse action: %v", err)
			continue
		}

		// Store original action
		session.Actions[action.Name] = action

		// Generate prefixed action name for neuro: gameID/actionName
		prefixedActionName := session.GameID + "/" + action.Name

		log.Printf("Registered action: %s -> %s", action.Name, prefixedActionName)

		// Notify integration client with prefixed name
		if eb.OnActionRegistered != nil {
			// Create a copy with the prefixed name for forwarding to Neuro
			prefixedAction := action
			prefixedAction.Name = prefixedActionName
			eb.OnActionRegistered(session.GameID, prefixedActionName, prefixedAction)
		}
	}
}

func (eb *EmulationBackend) handleUnregisterActions(c *utilities.Client, msg ClientMessage) {
	eb.sessionsMu.RLock()
	session := eb.sessions[c]
	eb.sessionsMu.RUnlock()

	if session == nil {
		log.Println("Unregister actions received from unknown session")
		return
	}

	names, ok := msg.Data["action_names"].([]interface{})
	if !ok {
		log.Println("Invalid action_names data format")
		return
	}

	for _, n := range names {
		if name, ok := n.(string); ok {
			delete(session.Actions, name)

			// Generate prefixed action name
			prefixedActionName := session.GameID + "/" + name

			log.Printf("Unregistered action: %s -> %s", name, prefixedActionName)

			// Notify integration client
			if eb.OnActionUnregistered != nil {
				eb.OnActionUnregistered(session.GameID, prefixedActionName)
			}
		}
	}
}

func (eb *EmulationBackend) handleForceActions(c *utilities.Client, msg ClientMessage) {
	eb.sessionsMu.RLock()
	session := eb.sessions[c]
	eb.sessionsMu.RUnlock()

	if session == nil {
		log.Println("Force actions received from unknown session")
		return
	}

	state, _ := msg.Data["state"].(string)
	query, _ := msg.Data["query"].(string)
	ephemeralContext, _ := msg.Data["ephemeral_context"].(bool)
	priority, _ := msg.Data["priority"].(string)

	if priority == "" {
		priority = "low"
	}

	rawActionNames, ok := msg.Data["action_names"].([]interface{})
	if !ok {
		log.Println("Invalid action_names in force")
		return
	}

	// Convert and prefix action names
	prefixedActionNames := make([]string, 0, len(rawActionNames))
	for _, name := range rawActionNames {
		if actionName, ok := name.(string); ok {
			prefixedActionNames = append(prefixedActionNames, session.GameID+"/"+actionName)
		}
	}

	log.Printf("Force actions from %s: %v", session.GameID, prefixedActionNames)

	// Notify integration client
	if eb.OnActionForce != nil {
		eb.OnActionForce(session.GameID, state, query, ephemeralContext, priority, prefixedActionNames)
	}
}

func (eb *EmulationBackend) handleActionResult(c *utilities.Client, msg ClientMessage) {
	eb.sessionsMu.RLock()
	session := eb.sessions[c]
	eb.sessionsMu.RUnlock()

	if session == nil {
		log.Println("Action result received from unknown session")
		return
	}

	actionID, _ := msg.Data["id"].(string)
	success, _ := msg.Data["success"].(bool)
	message, _ := msg.Data["message"].(string)

	log.Printf("Action result from %s: id=%s, success=%v", session.GameID, actionID, success)

	// Notify integration client
	if eb.OnActionResult != nil {
		eb.OnActionResult(session.GameID, actionID, success, message)
	}
}

/* =========================
   Public methods for integration client
   ========================= */

// SendAction sends an action command to a specific game client
func (eb *EmulationBackend) SendAction(gameID string, actionID string, actionName string, data interface{}) error {
	// Find the client for this game
	eb.sessionsMu.RLock()
	var targetClient *utilities.Client
	var targetSession *GameSession

	for client, session := range eb.sessions {
		if session.GameID == gameID {
			targetClient = client
			targetSession = session
			break
		}
	}
	eb.sessionsMu.RUnlock()

	if targetClient == nil {
		return fmt.Errorf("game session not found: %s", gameID)
	}

	// Remove the gameID prefix from the action name before sending to the game
	// "game-a/buy_books" -> "buy_books"
	originalActionName := strings.TrimPrefix(actionName, gameID+"/")

	payload := ServerMessage{
		Command: "action",
		Data: map[string]interface{}{
			"id":   actionID,
			"name": originalActionName,
			"data": data,
		},
	}

	return eb.sendJSON(targetClient, payload)
}

// GetAllSessions returns information about all connected sessions
func (eb *EmulationBackend) GetAllSessions() map[string]string {
	eb.sessionsMu.RLock()
	defer eb.sessionsMu.RUnlock()

	result := make(map[string]string)
	for _, session := range eb.sessions {
		result[session.GameID] = session.GameName
	}
	return result
}

// IsLocked returns whether the backend is locked to a non-compatible integration
func (eb *EmulationBackend) IsLocked() bool {
	eb.lockMu.RLock()
	defer eb.lockMu.RUnlock()
	return eb.locked
}

/* =========================
   Helper functions
   ========================= */

// normalizeGameName converts a game name into a safe game ID
// "Game A" -> "game-a", "Buckshot Roulette" -> "buckshot-roulette"
func (eb *EmulationBackend) normalizeGameName(gameName string) string {
	// Convert to lowercase
	gameID := strings.ToLower(gameName)

	// Replace spaces with hyphens
	gameID = strings.ReplaceAll(gameID, " ", "-")

	// Remove all non-alphanumeric characters except hyphens
	reg := regexp.MustCompile("[^a-z0-9-]+")
	gameID = reg.ReplaceAllString(gameID, "")

	// Remove multiple consecutive hyphens
	reg = regexp.MustCompile("-+")
	gameID = reg.ReplaceAllString(gameID, "-")

	// Trim hyphens from start and end
	gameID = strings.Trim(gameID, "-")

	return gameID
}

func (eb *EmulationBackend) sendError(c *utilities.Client, command string, message string) {
	resp := ServerMessage{
		Command: command,
		Data: map[string]interface{}{
			"error": message,
		},
	}
	eb.sendJSON(c, resp)
}

func (eb *EmulationBackend) sendJSON(c *utilities.Client, v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.Send(b)
	return nil
}

// HandleClientDisconnect should be called when a client disconnects
func (eb *EmulationBackend) HandleClientDisconnect(c *utilities.Client) {
	eb.sessionsMu.Lock()
	session := eb.sessions[c]
	delete(eb.sessions, c)
	eb.sessionsMu.Unlock()

	if session != nil {
		log.Printf("Client disconnected: %s (ID: %s)", session.GameName, session.GameID)

		// If this was the locked client, unlock the backend
		eb.lockMu.Lock()
		if eb.lockedToClient == c {
			eb.locked = false
			eb.lockedToClient = nil
			log.Println("Backend unlocked")
		}
		eb.lockMu.Unlock()
	}
}
