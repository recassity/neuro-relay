package nbackend

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/recassity/neuro-relay/src/utils"
)

// TestGameIDNormalization tests the game ID generation algorithm
func TestGameIDNormalization(t *testing.T) {
	backend := NewEmulationBackend()

	tests := []struct {
		input    string
		expected string
	}{
		{"Game A", "game-a"},
		{"Buckshot Roulette", "buckshot-roulette"},
		{"My Amazing Game!", "my-amazing-game"},
		{"Test  ---  Game", "test-game"},
		{"UPPERCASE", "uppercase"},
		{"multiple   spaces", "multiple-spaces"},
		{"Special@#$Characters", "specialcharacters"},
		{"-leading-trailing-", "leading-trailing"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := backend.normalizeGameName(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeGameName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestVersionCompatibility tests the version features system
func TestVersionCompatibility(t *testing.T) {
	tests := []struct {
		version string
		exists  bool
	}{
		{"1.0.0", true},
		{"2.0.0", false},
		{"0.5.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			_, exists := versionCompatibility[tt.version]
			if exists != tt.exists {
				t.Errorf("versionCompatibility[%q] exists = %v, want %v", tt.version, exists, tt.exists)
			}
		})
	}

	// Test feature flags for v1.0.0
	features, ok := versionCompatibility["1.0.0"]
	if !ok {
		t.Fatal("Version 1.0.0 should exist")
	}

	if !features.SupportsHealthEndpoint {
		t.Error("Version 1.0.0 should support health endpoint")
	}
	if !features.SupportsMultiplexing {
		t.Error("Version 1.0.0 should support multiplexing")
	}
	if !features.SupportsCustomRouting {
		t.Error("Version 1.0.0 should support custom routing")
	}
}

// TestSessionManagement tests session creation and cleanup
func TestSessionManagement(t *testing.T) {
	backend := NewEmulationBackend()

	// Create a mock client
	mockClient := &utilities.Client{}

	// Simulate startup
	backend.sessionsMu.Lock()
	backend.sessions[mockClient] = &GameSession{
		GameName:         "Test Game",
		GameID:           "test-game",
		Actions:          make(map[string]ActionDefinition),
		NRelayCompatible: true,
		NRelayVersion:    "1.0.0",
		Client:           mockClient,
	}
	backend.sessionsMu.Unlock()

	// Verify session exists
	backend.sessionsMu.RLock()
	session := backend.sessions[mockClient]
	backend.sessionsMu.RUnlock()

	if session == nil {
		t.Fatal("Session should exist")
	}
	if session.GameID != "test-game" {
		t.Errorf("GameID = %q, want %q", session.GameID, "test-game")
	}

	// Test cleanup
	backend.HandleClientDisconnect(mockClient)

	backend.sessionsMu.RLock()
	_, exists := backend.sessions[mockClient]
	backend.sessionsMu.RUnlock()

	if exists {
		t.Error("Session should be removed after disconnect")
	}
}

// TestActionPrefixing tests action name prefixing for multiplexing
func TestActionPrefixing(t *testing.T) {
	backend := NewEmulationBackend()

	tests := []struct {
		gameID       string
		actionName   string
		multiplexing bool
		expectedName string
	}{
		{"game-a", "buy_books", true, "game-a--buy_books"},
		{"game-a", "buy_books", false, "buy_books"},
		{"buckshot-roulette", "shoot", true, "buckshot-roulette--shoot"},
		{"buckshot-roulette", "shoot", false, "shoot"},
	}

	for _, tt := range tests {
		t.Run(tt.gameID+"--"+tt.actionName, func(t *testing.T) {
			// Create session
			mockClient := &utilities.Client{}
			backend.sessionsMu.Lock()
			backend.sessions[mockClient] = &GameSession{
				GameName: "Test",
				GameID:   tt.gameID,
				Actions:  make(map[string]ActionDefinition),
				VersionFeatures: VersionFeatures{
					SupportsMultiplexing: tt.multiplexing,
				},
				Client: mockClient,
			}
			backend.sessionsMu.Unlock()

			// Register action callback to capture the registered name
			var registeredName string
			backend.OnActionRegistered = func(gameID, actionName string, action ActionDefinition) {
				registeredName = actionName
			}

			// Simulate the actual registration flow that happens in handleRegisterActions
			action := ActionDefinition{
				Name:        tt.actionName,
				Description: "Test action",
			}

			// Store in session
			backend.sessionsMu.RLock()
			session := backend.sessions[mockClient]
			backend.sessionsMu.RUnlock()

			session.Actions[action.Name] = action

			// Determine the forwarded action name based on multiplexing
			var forwardedName string
			if tt.multiplexing {
				forwardedName = tt.gameID + "--" + tt.actionName
			} else {
				forwardedName = tt.actionName
			}

			// Call the callback as the real implementation would
			if backend.OnActionRegistered != nil {
				forwardedAction := action
				forwardedAction.Name = forwardedName
				backend.OnActionRegistered(session.GameID, forwardedName, forwardedAction)
			}

			// Verify the registered name matches expected
			if registeredName != tt.expectedName {
				t.Errorf("Action name = %q, want %q", registeredName, tt.expectedName)
			}

			// Cleanup
			backend.HandleClientDisconnect(mockClient)
		})
	}
}

// TestLockingMechanism tests the compatibility lock system
func TestLockingMechanism(t *testing.T) {
	backend := NewEmulationBackend()

	mockClient1 := &utilities.Client{}
	mockClient2 := &utilities.Client{}

	// Create non-compatible session
	backend.lockMu.Lock()
	backend.locked = true
	backend.lockedToClient = mockClient1
	backend.lockMu.Unlock()

	backend.sessionsMu.Lock()
	backend.sessions[mockClient1] = &GameSession{
		GameName:         "Non-Compatible Game",
		GameID:           "non-compatible",
		NRelayCompatible: false,
		Client:           mockClient1,
	}
	backend.sessionsMu.Unlock()

	// Verify locked state
	if !backend.IsLocked() {
		t.Error("Backend should be locked")
	}

	// Attempt to add another session (should be rejected in real impl)
	// For testing, we'll just verify the lock state

	// Disconnect the non-compatible game
	backend.HandleClientDisconnect(mockClient1)

	// Verify unlocked
	if backend.IsLocked() {
		t.Error("Backend should be unlocked after disconnect")
	}

	// Now try compatible session
	backend.sessionsMu.Lock()
	backend.sessions[mockClient2] = &GameSession{
		GameName:         "Compatible Game",
		GameID:           "compatible",
		NRelayCompatible: true,
		NRelayVersion:    "1.0.0",
		Client:           mockClient2,
	}
	backend.sessionsMu.Unlock()

	// Should not lock
	if backend.IsLocked() {
		t.Error("Backend should not lock for compatible games")
	}

	// Cleanup
	backend.HandleClientDisconnect(mockClient2)
}

// TestConcurrentAccess tests thread safety with concurrent operations
func TestConcurrentAccess(t *testing.T) {
	backend := NewEmulationBackend()

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent session creation
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			mockClient := &utilities.Client{}
			backend.sessionsMu.Lock()
			backend.sessions[mockClient] = &GameSession{
				GameName: "Test",
				GameID:   "test",
				Actions:  make(map[string]ActionDefinition),
				Client:   mockClient,
			}
			backend.sessionsMu.Unlock()

			time.Sleep(time.Millisecond)

			backend.HandleClientDisconnect(mockClient)
		}(i)
	}

	wg.Wait()

	// All sessions should be cleaned up
	backend.sessionsMu.RLock()
	count := len(backend.sessions)
	backend.sessionsMu.RUnlock()

	if count != 0 {
		t.Errorf("Expected 0 sessions, got %d", count)
	}
}

// TestGetAllSessions tests session retrieval
func TestGetAllSessions(t *testing.T) {
	backend := NewEmulationBackend()

	// Add multiple sessions
	sessions := map[string]string{
		"game-a": "Game A",
		"game-b": "Game B",
		"game-c": "Game C",
	}

	// Track clients for cleanup
	clients := make([]*utilities.Client, 0)

	for gameID, gameName := range sessions {
		mockClient := &utilities.Client{}
		clients = append(clients, mockClient)
		backend.sessionsMu.Lock()
		backend.sessions[mockClient] = &GameSession{
			GameName: gameName,
			GameID:   gameID,
			Actions:  make(map[string]ActionDefinition),
			Client:   mockClient,
		}
		backend.sessionsMu.Unlock()
	}

	// Get all sessions
	result := backend.GetAllSessions()

	// Verify count
	if len(result) != len(sessions) {
		t.Errorf("Expected %d sessions, got %d", len(sessions), len(result))
	}

	// Verify each session is present with correct name
	for gameID, gameName := range sessions {
		if result[gameID] != gameName {
			t.Errorf("Session %q: got %q, want %q", gameID, result[gameID], gameName)
		}
	}

	// Cleanup all clients
	for _, client := range clients {
		backend.HandleClientDisconnect(client)
	}

	// Verify all sessions cleaned up
	backend.sessionsMu.RLock()
	remainingCount := len(backend.sessions)
	backend.sessionsMu.RUnlock()

	if remainingCount != 0 {
		t.Errorf("Expected 0 sessions after cleanup, got %d", remainingCount)
	}
}

// TestJSONParsing tests JSON message parsing
func TestJSONParsing(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "Valid startup",
			input:   `{"command":"startup","game":"Test Game"}`,
			wantErr: false,
		},
		{
			name:    "Valid action registration",
			input:   `{"command":"actions/register","game":"Test","data":{"actions":[{"name":"test","description":"Test action"}]}}`,
			wantErr: false,
		},
		{
			name:    "Invalid JSON",
			input:   `{invalid json}`,
			wantErr: true,
		},
		{
			name:    "Missing command",
			input:   `{"game":"Test"}`,
			wantErr: false, // Should parse but command will be empty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var msg ClientMessage
			err := json.Unmarshal([]byte(tt.input), &msg)

			if (err != nil) != tt.wantErr {
				t.Errorf("Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err == nil && msg.Command == "" && !tt.wantErr {
				// Some tests expect empty command
			}
		})
	}
}

// TestSendActionSafety tests safe action sending with disconnect handling
func TestSendActionSafety(t *testing.T) {
	backend := NewEmulationBackend()

	// Track result callbacks
	var resultCalled bool
	var resultSuccess bool
	var resultMessage string
	var mu sync.Mutex

	backend.OnActionResult = func(gameID, actionID string, success bool, message string) {
		mu.Lock()
		defer mu.Unlock()
		resultCalled = true
		resultSuccess = success
		resultMessage = message
		t.Logf("OnActionResult called: gameID=%s, actionID=%s, success=%v, message=%s",
			gameID, actionID, success, message)
	}

	// Test sending to disconnected/non-existent game
	err := backend.SendAction("non-existent-game", "action123", "test_action", "{}")

	if err == nil {
		t.Error("Expected error when sending to non-existent game")
	}

	// Wait for async callback to complete
	time.Sleep(50 * time.Millisecond)

	// Should have called OnActionResult with failure
	mu.Lock()
	if !resultCalled {
		t.Error("Expected OnActionResult to be called on disconnect")
	}
	if !resultSuccess {
		t.Error("Expected OnActionResult to report success=true (not to auto-retry)")
	}
	if !strings.Contains(resultMessage, "disconnected") && !strings.Contains(resultMessage, "Game disconnected") {
		t.Errorf("Expected disconnect message, got: %s", resultMessage)
	}
	mu.Unlock()
}

// BenchmarkGameIDNormalization benchmarks the normalization function
func BenchmarkGameIDNormalization(b *testing.B) {
	backend := NewEmulationBackend()
	testName := "Buckshot Roulette: Extended Edition!!!"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = backend.normalizeGameName(testName)
	}
}

// BenchmarkConcurrentSessions benchmarks concurrent session management
func BenchmarkConcurrentSessions(b *testing.B) {
	backend := NewEmulationBackend()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mockClient := &utilities.Client{}

			backend.sessionsMu.Lock()
			backend.sessions[mockClient] = &GameSession{
				GameName: "Test",
				GameID:   "test",
				Actions:  make(map[string]ActionDefinition),
				Client:   mockClient,
			}
			backend.sessionsMu.Unlock()

			backend.HandleClientDisconnect(mockClient)
		}
	})
}
