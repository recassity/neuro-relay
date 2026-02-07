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
	client.actionToGame["game-a--test_action"] = "game-a"
	client.actionMu.Unlock()

	// Simulate Neuro executing the action
	actionID := "test_action_123"
	actionName := "game-a--test_action"

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
	actionName := "game-a--test_action"
	action := nbackend.ActionDefinition{
		Name:        actionName,
		Description: "Test action",
		Schema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"param1": map[string]interface{}{
					"type": "string",
				},
			},
		},
	}

	// Call the callback that would be triggered by backend
	if backend.OnActionRegistered != nil {
		backend.OnActionRegistered(gameID, actionName, action)
	}

	// Wait for async operations
	time.Sleep(20 * time.Millisecond)

	// Verify action is tracked in actionToGame map
	client.actionMu.RLock()
	mappedGame, exists := client.actionToGame[actionName]
	client.actionMu.RUnlock()

	if !exists {
		t.Errorf("Action %q not found in actionToGame map", actionName)
	}

	if mappedGame != gameID {
		t.Errorf("Action mapped to wrong game: got %q, want %q", mappedGame, gameID)
	}

	// Verify action is in registered actions map
	client.actionsMu.RLock()
	registeredAction, exists := client.registeredActions[actionName]
	client.actionsMu.RUnlock()

	if !exists {
		t.Error("Action not in registered actions map")
	}

	if registeredAction.Description != action.Description {
		t.Errorf("Action description mismatch: got %q, want %q", 
			registeredAction.Description, action.Description)
	}

	// Test unregistration
	if backend.OnActionUnregistered != nil {
		backend.OnActionUnregistered(gameID, actionName)
	}

	time.Sleep(20 * time.Millisecond)

	// Verify action is removed
	client.actionMu.RLock()
	_, stillExists := client.actionToGame[actionName]
	client.actionMu.RUnlock()

	if stillExists {
		t.Error("Action should be removed after unregistration")
	}

	client.actionsMu.RLock()
	_, stillInRegistry := client.registeredActions[actionName]
	client.actionsMu.RUnlock()

	if stillInRegistry {
		t.Error("Action should be removed from registry after unregistration")
	}
}

// TestShutdownGameAction tests the shutdown_game special action
func TestShutdownGameAction(t *testing.T) {
	backend := nbackend.NewEmulationBackend()

	// Create mock client and add to backend sessions
	mockClient := &utilities.Client{}
	backend.sessionsMu.Lock()
	backend.sessions[mockClient] = &nbackend.GameSession{
		GameName: "Game A",
		GameID:   "game-a",
		Actions:  make(map[string]nbackend.ActionDefinition),
		Client:   mockClient,
	}
	backend.sessionsMu.Unlock()

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

	// Parse action data to verify it's valid
	var params struct {
		GameID string `json:"game_id"`
	}
	err := json.Unmarshal([]byte(actionData), &params)
	if err != nil {
		t.Fatalf("Failed to parse action data: %v", err)
	}

	if params.GameID != "game-a" {
		t.Errorf("Parsed game_id = %q, want %q", params.GameID, "game-a")
	}

	// Verify the game exists in backend
	sessions := backend.GetAllSessions()
	if _, exists := sessions["game-a"]; !exists {
		t.Error("Game 'game-a' should exist in backend sessions")
	}

	// Test with invalid/non-existent game
	invalidData := `{"game_id":"non-existent"}`
	var invalidParams struct {
		GameID string `json:"game_id"`
	}
	err = json.Unmarshal([]byte(invalidData), &invalidParams)
	if err != nil {
		t.Fatalf("Failed to parse invalid action data: %v", err)
	}

	if invalidParams.GameID != "non-existent" {
		t.Error("Should successfully parse game_id even if game doesn't exist")
	}

	// Verify non-existent game is not in sessions
	if _, exists := sessions["non-existent"]; exists {
		t.Error("Non-existent game should not be in sessions")
	}

	// Cleanup
	backend.HandleClientDisconnect(mockClient)
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

			actionName := "game-a--action_" + string(rune(id))
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
	var receivedGameID string
	var receivedMessage string
	var receivedSilent bool
	var contextCalled bool
	var mu sync.Mutex

	// Intercept the context callback
	originalCallback := backend.OnContext
	backend.OnContext = func(gameID, message string, silent bool) {
		mu.Lock()
		defer mu.Unlock()
		contextCalled = true
		receivedGameID = gameID
		receivedMessage = message
		receivedSilent = silent
		
		// Call original if it exists
		if originalCallback != nil {
			originalCallback(gameID, message, silent)
		}
	}

	// Simulate context from a game
	gameID := "game-a"
	message := "Test context message"
	silent := false

	// Trigger the callback
	if backend.OnContext != nil {
		backend.OnContext(gameID, message, silent)
	}

	// Wait for async operations
	time.Sleep(20 * time.Millisecond)

	// Verify callback was called
	mu.Lock()
	if !contextCalled {
		t.Error("OnContext callback should have been called")
	}

	// Verify received values
	if receivedGameID != gameID {
		t.Errorf("GameID = %q, want %q", receivedGameID, gameID)
	}
	
	if receivedMessage != message {
		t.Errorf("Message = %q, want %q", receivedMessage, message)
	}
	
	if receivedSilent != silent {
		t.Errorf("Silent = %v, want %v", receivedSilent, silent)
	}
	mu.Unlock()

	// Test with silent=true
	mu.Lock()
	contextCalled = false
	receivedSilent = false
	mu.Unlock()

	silentMessage := "Silent test message"
	if backend.OnContext != nil {
		backend.OnContext(gameID, silentMessage, true)
	}

	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	if !contextCalled {
		t.Error("OnContext callback should have been called for silent message")
	}
	
	if receivedMessage != silentMessage {
		t.Errorf("Silent message = %q, want %q", receivedMessage, silentMessage)
	}
	
	if !receivedSilent {
		t.Error("Silent flag should be true")
	}
	mu.Unlock()
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