package nbackend

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/recassity/neuro-relay/src/utils"
)

/* =========================
   Configuration
   ========================= */

const (
	CurrentNRelayVersion = "1.0.0"
)

// VersionFeatures defines which features are available in each NR version
type VersionFeatures struct {
	SupportsHealthEndpoint bool
	SupportsMultiplexing   bool
	SupportsCustomRouting  bool
}

var versionCompatibility = map[string]VersionFeatures{
	"1.0.0": {
		SupportsHealthEndpoint: true,
		SupportsMultiplexing:   true,
		SupportsCustomRouting:  true,
	},
	// Future versions can be added here
}

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
	VersionFeatures  VersionFeatures // Features available for this version
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
	OnShutdownReady      func(gameID string)
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
	eb.Attach(mux, "/")
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

	// Handle NeuroRelay Custom (NRC) endpoints
	if strings.HasPrefix(msg.Command, "nrc-endpoints/") {
		eb.handleNRCEndpoint(c, msg)
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

	case "shutdown/ready":
		eb.handleShutdownReady(c, msg)

	default:
		log.Printf("unknown command: %s", msg.Command)
	}
}

/* =========================
   NRC Endpoint Handlers
   ========================= */

func (eb *EmulationBackend) handleNRCEndpoint(c *utilities.Client, msg ClientMessage) {
	endpoint := strings.TrimPrefix(msg.Command, "nrc-endpoints/")

	switch endpoint {
	case "startup":
		eb.handleNRCStartup(c, msg)
	case "health":
		eb.handleNRCHealth(c, msg)
	default:
		log.Printf("unknown NRC endpoint: %s", endpoint)
		eb.sendError(c, "nrc-endpoints/error", "Unknown endpoint: "+endpoint)
	}
}

func (eb *EmulationBackend) handleNRCStartup(c *utilities.Client, msg ClientMessage) {
	eb.sessionsMu.RLock()
	session := eb.sessions[c]
	eb.sessionsMu.RUnlock()

	if session == nil {
		log.Println("NRC startup received from unknown session")
		eb.sendError(c, "nrc-endpoints/error", "Session not found. Send 'startup' command first.")
		return
	}

	// Extract NR version
	nrVersion, ok := msg.Data["nr-version"].(string)
	if !ok || nrVersion == "" {
		log.Printf("NRC startup from %s missing nr-version", session.GameID)
		eb.sendError(c, "nrc-endpoints/error", "Missing required field: nr-version")
		return
	}

	// Get version features
	features, supported := versionCompatibility[nrVersion]
	if !supported {
		log.Printf("Unsupported NR version: %s from %s", nrVersion, session.GameID)
		// Send available versions
		availableVersions := make([]string, 0, len(versionCompatibility))
		for v := range versionCompatibility {
			availableVersions = append(availableVersions, v)
		}
		eb.sendJSON(c, ServerMessage{
			Command: "nrc-endpoints/version-mismatch",
			Data: map[string]interface{}{
				"requested":  nrVersion,
				"available":  availableVersions,
				"suggestion": CurrentNRelayVersion,
			},
		})
		return
	}

	// Update session with NR compatibility
	session.NRelayCompatible = true
	session.NRelayVersion = nrVersion
	session.VersionFeatures = features

	log.Printf("NRC startup: %s is now NR-compatible (version %s)", session.GameID, nrVersion)

	// Send success response with enabled features
	eb.sendJSON(c, ServerMessage{
		Command: "nrc-endpoints/startup-ack",
		Data: map[string]interface{}{
			"nr-version": CurrentNRelayVersion,
			"features": map[string]interface{}{
				"health-endpoint": features.SupportsHealthEndpoint,
				"multiplexing":    features.SupportsMultiplexing,
				"custom-routing":  features.SupportsCustomRouting,
			},
		},
	})
}

func (eb *EmulationBackend) handleNRCHealth(c *utilities.Client, msg ClientMessage) {
	eb.sessionsMu.RLock()
	session := eb.sessions[c]
	eb.sessionsMu.RUnlock()

	if session == nil {
		log.Println("NRC health check from unknown session")
		return
	}

	if !session.VersionFeatures.SupportsHealthEndpoint {
		log.Printf("Health endpoint not supported for %s (version %s)", session.GameID, session.NRelayVersion)
		eb.sendError(c, "nrc-endpoints/error", "Health endpoint not supported in your NR version")
		return
	}

	// Parse what info to include
	includeFields := make(map[string]bool)
	if msg.Data != nil {
		if fields, ok := msg.Data["include"].([]interface{}); ok {
			for _, field := range fields {
				if fieldName, ok := field.(string); ok {
					includeFields[fieldName] = true
				}
			}
		} else {
			// Default: include all
			includeFields["status"] = true
			includeFields["version"] = true
			includeFields["connected-games"] = true
			includeFields["neuro-backend"] = true
			includeFields["uptime"] = true
		}
	}

	// Build health response
	healthData := make(map[string]interface{})

	if includeFields["status"] {
		healthData["status"] = "healthy"
	}

	if includeFields["version"] {
		healthData["nr-version"] = CurrentNRelayVersion
		healthData["game-nr-version"] = session.NRelayVersion
	}

	if includeFields["connected-games"] {
		games := eb.GetAllSessions()
		gameList := make([]map[string]interface{}, 0, len(games))
		for gameID, gameName := range games {
			gameList = append(gameList, map[string]interface{}{
				"id":   gameID,
				"name": gameName,
			})
		}
		healthData["connected-games"] = gameList
		healthData["total-games"] = len(games)
	}

	if includeFields["neuro-backend"] {
		// This will be filled by integration client if available
		healthData["neuro-backend-connected"] = true // Placeholder
	}

	if includeFields["uptime"] {
		// This would require tracking start time - placeholder for now
		healthData["uptime-seconds"] = 0
	}

	if includeFields["features"] {
		healthData["features"] = map[string]interface{}{
			"health-endpoint": session.VersionFeatures.SupportsHealthEndpoint,
			"multiplexing":    session.VersionFeatures.SupportsMultiplexing,
			"custom-routing":  session.VersionFeatures.SupportsCustomRouting,
		}
	}

	if includeFields["lock-status"] {
		healthData["backend-locked"] = eb.IsLocked()
	}

	log.Printf("Health check from %s: %v", session.GameID, includeFields)

	// Send health response
	eb.sendJSON(c, ServerMessage{
		Command: "nrc-endpoints/health-response",
		Data:    healthData,
	})
}

/* =========================
   Command handlers
   ========================= */

func (eb *EmulationBackend) handleStartup(c *utilities.Client, msg ClientMessage) {
	// Standard startup - treat all games as potentially compatible
	// Actual compatibility is determined via nrc-endpoints/startup

	eb.lockMu.Lock()
	defer eb.lockMu.Unlock()

	// Generate game ID from game name
	gameID := eb.normalizeGameName(msg.Game)

	// Create session with default compatibility (no NR features)
	eb.sessionsMu.Lock()
	eb.sessions[c] = &GameSession{
		GameName:         msg.Game,
		GameID:           gameID,
		LatestActionNum:  0,
		Actions:          make(map[string]ActionDefinition),
		NRelayCompatible: false, // Default to non-compatible
		NRelayVersion:    "",
		VersionFeatures: VersionFeatures{
			SupportsHealthEndpoint: false,
			SupportsMultiplexing:   false,
			SupportsCustomRouting:  false,
		},
		Client: c,
	}
	eb.sessionsMu.Unlock()

	log.Printf("Startup from game: %s (ID: %s) - awaiting NR compatibility check", msg.Game, gameID)

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

		// Only prefix actions if multiplexing is supported
		var actionNameToRegister string
		if session.VersionFeatures.SupportsMultiplexing {
			// Generate prefixed action name for neuro: gameID--actionName
			actionNameToRegister = session.GameID + "--" + action.Name
			log.Printf("Registered action with multiplexing: %s -> %s", action.Name, actionNameToRegister)
		} else {
			// No prefixing for non-multiplexing clients
			actionNameToRegister = action.Name
			log.Printf("Registered action without multiplexing: %s", action.Name)
		}

		// Notify integration client
		if eb.OnActionRegistered != nil {
			// Create a copy with the appropriate name for forwarding to Neuro
			forwardedAction := action
			forwardedAction.Name = actionNameToRegister
			eb.OnActionRegistered(session.GameID, actionNameToRegister, forwardedAction)
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

			// Generate action name based on multiplexing support
			var actionNameToUnregister string
			if session.VersionFeatures.SupportsMultiplexing {
				actionNameToUnregister = session.GameID + "/" + name
				log.Printf("Unregistered action with multiplexing: %s -> %s", name, actionNameToUnregister)
			} else {
				actionNameToUnregister = name
				log.Printf("Unregistered action without multiplexing: %s", name)
			}

			// Notify integration client
			if eb.OnActionUnregistered != nil {
				eb.OnActionUnregistered(session.GameID, actionNameToUnregister)
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

	// Convert action names, prefix only if multiplexing is supported
	processedActionNames := make([]string, 0, len(rawActionNames))
	for _, name := range rawActionNames {
		if actionName, ok := name.(string); ok {
			if session.VersionFeatures.SupportsMultiplexing {
				processedActionNames = append(processedActionNames, session.GameID+"/"+actionName)
			} else {
				processedActionNames = append(processedActionNames, actionName)
			}
		}
	}

	log.Printf("Force actions from %s: %v (multiplexing: %v)", session.GameID, processedActionNames, session.VersionFeatures.SupportsMultiplexing)

	// Notify integration client
	if eb.OnActionForce != nil {
		eb.OnActionForce(session.GameID, state, query, ephemeralContext, priority, processedActionNames)
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

func (eb *EmulationBackend) handleShutdownReady(c *utilities.Client, msg ClientMessage) {
	eb.sessionsMu.RLock()
	session := eb.sessions[c]
	eb.sessionsMu.RUnlock()

	if session == nil {
		log.Println("Shutdown ready received from unknown session")
		return
	}

	log.Printf("Game %s is ready to shutdown", session.GameID)

	// Notify integration client
	if eb.OnShutdownReady != nil {
		eb.OnShutdownReady(session.GameID)
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
		err := fmt.Errorf("game session not found: %s (client disconnected)", gameID)
		log.Printf("ERROR: %v", err)
		
		// CRITICAL: Send failure result back to integration client
		// so Neuro doesn't wait forever for a response
		if eb.OnActionResult != nil {
			log.Printf("Notifying Neuro that game '%s' disconnected for action %s", gameID, actionID)
			// Make the success bool true, so Neuro / Evil don't automatically retry
			// The message will indicate the disconnect
			eb.OnActionResult(gameID, actionID, true, "Game disconnected unexpectedly")
		}
		
		return err
	}

	// Remove the gameID prefix only if multiplexing is enabled for this session
	// Otherwise, send the action name as-is
	var originalActionName string
	if targetSession.VersionFeatures.SupportsMultiplexing {
		// "game-a/buy_books" -> "buy_books"
		originalActionName = strings.TrimPrefix(actionName, gameID+"/")
	} else {
		// Action name is already correct for non-multiplexed games
		originalActionName = actionName
	}

	payload := ServerMessage{
		Command: "action",
		Data: map[string]interface{}{
			"id":   actionID,
			"name": originalActionName,
			"data": data,
		},
	}

	// CRITICAL FIX: Use safe send that won't panic on closed channel
	// and notifies Neuro if send fails
	return eb.sendJSONSafe(targetClient, payload, gameID, actionID)
}

// SendShutdown sends a graceful shutdown command to a specific game  
// Returns the client connection for fallback forceful disconnect if needed
func (eb *EmulationBackend) SendShutdown(gameID string, wantsShutdown bool) (*utilities.Client, error) {
	// Find the client for this game
	eb.sessionsMu.RLock()
	var targetClient *utilities.Client

	for client, session := range eb.sessions {
		if session.GameID == gameID {
			targetClient = client
			break
		}
	}
	eb.sessionsMu.RUnlock()

	if targetClient == nil {
		return nil, fmt.Errorf("game session not found: %s", gameID)
	}

	log.Printf("Sending shutdown command to %s (wants_shutdown: %v)", gameID, wantsShutdown)

	payload := ServerMessage{
		Command: "shutdown/graceful",
		Data: map[string]interface{}{
			"wants_shutdown": wantsShutdown,
		},
	}

	err := eb.sendJSON(targetClient, payload)
	return targetClient, err
}

// ForceDisconnect forcefully closes a game's WebSocket connection
func (eb *EmulationBackend) ForceDisconnect(client *utilities.Client, gameID string) {
	log.Printf("⚠️ Forcefully disconnecting game: %s (shutdown timeout - game did not respond to graceful shutdown)", gameID)
	
	// The client's Close() method will trigger the websocket close,
	// which will automatically trigger the unregister mechanism in wsServer.go
	if err := client.Close(); err != nil {
		log.Printf("Error closing connection for %s: %v", gameID, err)
	}
	
	log.Printf("✅ Game %s forcefully disconnected via WebSocket close", gameID)
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

// sendJSONSafe sends JSON to a client with graceful handling of closed connections
// This prevents "send on closed channel" panics and notifies Neuro of disconnections
func (eb *EmulationBackend) sendJSONSafe(c *utilities.Client, v interface{}, gameID string, actionID string) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}

	// Use defer/recover to catch panics from sending to closed channel
	defer func() {
		if r := recover(); r != nil {
			log.Printf("WARNING: Failed to send to %s (client disconnected): %v", gameID, r)
			
			// Clean up the session if it still exists
			eb.sessionsMu.Lock()
			for client, session := range eb.sessions {
				if client == c {
					delete(eb.sessions, client)
					log.Printf("Cleaned up disconnected session: %s", session.GameID)
					break
				}
			}
			eb.sessionsMu.Unlock()
			
			// CRITICAL: Notify Neuro that the action failed due to disconnect
			if eb.OnActionResult != nil {
				log.Printf("Notifying Neuro of disconnect during send for action %s", actionID)
				eb.OnActionResult(gameID, actionID, false, "Game disconnected during action send")
			}
		}
	}()

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
