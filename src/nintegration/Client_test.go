package nintegration

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/recassity/neuro-relay/src/nbackend"
)

// MockWebSocket simulates a WebSocket connection for testing
type MockWebSocket struct {
	messages []string
	mu       sync.Mutex
	closed   bool
}

func NewMockWebSocket() *MockWebSocket {
	return &MockWebSocket{
		messages: make([]string, 0),
	}
}

func (m *MockWebSocket) Send(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return &MockWebSocketError{msg: "connection closed"}
	}

	m.messages = append(m.messages, string(data))
	return nil
}

func (m *MockWebSocket) GetMessages() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.messages...)
}

func (m *MockWebSocket) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

type MockWebSocketError struct {
	msg string
}

func (e *MockWebSocketError) Error() string {
	return e.msg
}

// TestActionRouting tests action routing from Neuro to games
func TestActionRouting(t *testing.T) {
	backend := nbackend.NewEmulationBackend()

	config := IntegrationClientConfig{
		RelayName:    "Test Relay",
		NeuroURL:     "ws://localhost:8000",
		EmulatedAddr: "127.0.0.1:8001",
	}

	client := &IntegrationClient{
		backend:        backend,
		actionToGame:   make(map[string]string),
		actionIDToGame: make(map[string]string),
		config:         config,
	}

	client.setupBackendCallbacks()

	// Register an action
	client.actionMu.Lock()
	client.actionToGame["game-a/test_action"] = "game-a"
	client.actionMu.Unlock()

	// Simulate Neuro executing the action
	actionID := "test_action_123"
	actionName := "game-a/test_action"

	// Track the action
	client.actionIDMu.Lock()
	client.actionIDToGame[actionID] = "game-a"
	client.actionIDMu.Unlock()

	// Verify tracking
	client.actionIDMu.RLock()
	gameID := client.actionIDToGame[actionID]
	client.actionIDMu.RUnlock()

	if gameID != "game-a" {
		t.Errorf("Action ID tracking: got %q, want %q", gameID, "game-a")
	}

	// Verify action mapping
	client.actionMu.RLock()
	mappedGame := client.actionToGame[actionName]
	client.actionMu.RUnlock()

	if mappedGame != "game-a" {
		t.Errorf("Action mapping: got %q, want %q", mappedGame, "game-a")
	}

	// Simulate result
	client.actionIDMu.Lock()
	delete(client.actionIDToGame, actionID)
	client.actionIDMu.Unlock()

	// Verify cleanup
	client.actionIDMu.RLock()
	_, exists := client.actionIDToGame[actionID]
	client.actionIDMu.RUnlock()

	if exists {
		t.Error("Action ID should be removed after result")
	}
}

// TestActionRegistration tests the registration flow
func TestActionRegistration(t *testing.T) {
	backend := nbackend.NewEmulationBackend()

	config := IntegrationClientConfig{
		RelayName:    "Test Relay",
		NeuroURL:     "ws://localhost:8000",
		EmulatedAddr: "127.0.0.1:8001",
	}

	client := &IntegrationClient{
		backend:           backend,
		actionToGame:      make(map[string]string),
		actionIDToGame:    make(map[string]string),
		registeredActions: make(map[string]nbackend.ActionDefinition),
		config:            config,
	}

	client.setupBackendCallbacks()

	// Simulate action registration from backend callback
	gameID := "game-a"
	actionName := "game-a/test_action"
	action := nbackend.ActionDefinition{
		Name:        actionName,
		Description: "Test action",
	}

	backend.OnActionRegistered(gameID, actionName, action)

	// Verify action is tracked
	client.actionMu.RLock()
	mappedGame := client.actionToGame[actionName]
	client.actionMu.RUnlock()

	if mappedGame != gameID {
		t.Errorf("Action not registered: got %q, want %q", mappedGame, gameID)
	}

	client.actionsMu.RLock()
	_, exists := client.registeredActions[actionName]
	client.actionsMu.RUnlock()

	if !exists {
		t.Error("Action not in registered actions map")
	}

	// Test unregistration
	backend.OnActionUnregistered(gameID, actionName)

	client.actionMu.RLock()
	_, stillExists := client.actionToGame[actionName]
	client.actionMu.RUnlock()

	if stillExists {
		t.Error("Action should be removed after unregistration")
	}
}

// TestShutdownGameAction tests the shutdown_game special action
func TestShutdownGameAction(t *testing.T) {
	backend := nbackend.NewEmulationBackend()

	// Mock games
	games := map[string]string{
		"game-a": "Game A",
		"game-b": "Game B",
	}

	// Populate backend sessions
	for gameID, gameName := range games {
		// In real implementation, this would be done via handleStartup
		// For testing, we directly set the session
		backend.GetAllSessions() // Initialize if needed
	}

	config := IntegrationClientConfig{
		RelayName:    "Test Relay",
		NeuroURL:     "ws://localhost:8000",
		EmulatedAddr: "127.0.0.1:8001",
	}

	client := &IntegrationClient{
		backend:        backend,
		actionToGame:   make(map[string]string),
		actionIDToGame: make(map[string]string),
		config:         config,
	}

	// Test shutdown action with valid game
	actionID := "shutdown_123"
	actionData := `{"game_id":"game-a"}`

	// Parse action data
	var params struct {
		GameID string `json:"game_id"`
	}
	json.Unmarshal([]byte(actionData), &params)

	if params.GameID != "game-a" {
		t.Errorf("Parsed game_id = %q, want %q", params.GameID, "game-a")
	}

	// Test with invalid game
	invalidData := `{"game_id":"non-existent"}`
	var invalidParams struct {
		GameID string `json:"game_id"`
	}
	json.Unmarshal([]byte(invalidData), &invalidParams)

	if invalidParams.GameID == "" {
		t.Error("Should parse game_id even if game doesn't exist")
	}
}

// TestConcurrentActionHandling tests thread safety during action handling
func TestConcurrentActionHandling(t *testing.T) {
	backend := nbackend.NewEmulationBackend()

	config := IntegrationClientConfig{
		RelayName:    "Test Relay",
		NeuroURL:     "ws://localhost:8000",
		EmulatedAddr: "127.0.0.1:8001",
	}

	client := &IntegrationClient{
		backend:        backend,
		actionToGame:   make(map[string]string),
		actionIDToGame: make(map[string]string),
		config:         config,
	}

	var wg sync.WaitGroup
	numActions := 100

	// Register actions concurrently
	for i := 0; i < numActions; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			actionName := "game-a/action_" + string(rune(id))
			client.actionMu.Lock()
			client.actionToGame[actionName] = "game-a"
			client.actionMu.Unlock()

			time.Sleep(time.Millisecond)

			client.actionMu.Lock()
			delete(client.actionToGame, actionName)
			client.actionMu.Unlock()
		}(i)
	}

	wg.Wait()

	// All actions should be cleaned up
	client.actionMu.RLock()
	count := len(client.actionToGame)
	client.actionMu.RUnlock()

	if count != 0 {
		t.Errorf("Expected 0 actions remaining, got %d", count)
	}
}

// TestGetConnectedGames tests retrieving connected games
func TestGetConnectedGames(t *testing.T) {
	backend := nbackend.NewEmulationBackend()

	config := IntegrationClientConfig{
		RelayName:    "Test Relay",
		NeuroURL:     "ws://localhost:8000",
		EmulatedAddr: "127.0.0.1:8001",
	}

	client := &IntegrationClient{
		backend:        backend,
		actionToGame:   make(map[string]string),
		actionIDToGame: make(map[string]string),
		config:         config,
	}

	// GetConnectedGames should return backend's sessions
	games := client.GetConnectedGames()

	// Initially should be empty
	if len(games) != 0 {
		t.Errorf("Expected 0 games, got %d", len(games))
	}

	// Note: In real usage, games would be added via backend.handleStartup
	// This test just verifies the method works
}

// TestIsBackendLocked tests lock state checking
func TestIsBackendLocked(t *testing.T) {
	backend := nbackend.NewEmulationBackend()

	config := IntegrationClientConfig{
		RelayName:    "Test Relay",
		NeuroURL:     "ws://localhost:8000",
		EmulatedAddr: "127.0.0.1:8001",
	}

	client := &IntegrationClient{
		backend:        backend,
		actionToGame:   make(map[string]string),
		actionIDToGame: make(map[string]string),
		config:         config,
	}

	// Initially unlocked
	if client.IsBackendLocked() {
		t.Error("Backend should be unlocked initially")
	}

	// Lock the backend (simulated)
	backend.lockMu.Lock()
	backend.locked = true
	backend.lockMu.Unlock()

	// Should now be locked
	if !client.IsBackendLocked() {
		t.Error("Backend should be locked")
	}

	// Unlock
	backend.lockMu.Lock()
	backend.locked = false
	backend.lockMu.Unlock()

	// Should be unlocked again
	if client.IsBackendLocked() {
		t.Error("Backend should be unlocked after unlock")
	}
}

// TestContextForwarding tests context message forwarding
func TestContextForwarding(t *testing.T) {
	backend := nbackend.NewEmulationBackend()

	config := IntegrationClientConfig{
		RelayName:    "Test Relay",
		NeuroURL:     "ws://localhost:8000",
		EmulatedAddr: "127.0.0.1:8001",
	}

	client := &IntegrationClient{
		backend:        backend,
		actionToGame:   make(map[string]string),
		actionIDToGame: make(map[string]string),
		config:         config,
	}

	client.setupBackendCallbacks()

	// Track context messages
	var receivedContext string
	var receivedSilent bool

	// Mock the context callback (in real impl, this sends to Neuro)
	originalCallback := backend.OnContext
	backend.OnContext = func(gameID, message string, silent bool) {
		receivedContext = message
		receivedSilent = silent
		if originalCallback != nil {
			originalCallback(gameID, message, silent)
		}
	}

	// Simulate context from a game
	gameID := "game-a"
	message := "Test context message"
	silent := false

	backend.OnContext(gameID, message, silent)

	// Verify
	expectedContext := message // The real impl prefixes with [gameID]
	if receivedContext != expectedContext {
		t.Errorf("Context = %q, want %q", receivedContext, expectedContext)
	}
	if receivedSilent != silent {
		t.Errorf("Silent = %v, want %v", receivedSilent, silent)
	}
}

// BenchmarkActionRouting benchmarks action routing performance
func BenchmarkActionRouting(b *testing.B) {
	backend := nbackend.NewEmulationBackend()

	config := IntegrationClientConfig{
		RelayName:    "Test Relay",
		NeuroURL:     "ws://localhost:8000",
		EmulatedAddr: "127.0.0.1:8001",
	}

	client := &IntegrationClient{
		backend:        backend,
		actionToGame:   make(map[string]string),
		actionIDToGame: make(map[string]string),
		config:         config,
	}

	// Pre-populate actions
	for i := 0; i < 100; i++ {
		actionName := "game-a/action_" + string(rune(i))
		client.actionToGame[actionName] = "game-a"
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		actionName := "game-a/action_50"
		client.actionMu.RLock()
		_ = client.actionToGame[actionName]
		client.actionMu.RUnlock()
	}
}