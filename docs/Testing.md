# Testing Guide for NeuroRelay

This guide covers running tests, writing new tests, and understanding the test infrastructure.

## 📋 Table of Contents

- [Running Tests](#running-tests)
- [Test Structure](#test-structure)
- [Writing Tests](#writing-tests)
- [Coverage Reports](#coverage-reports)
- [Benchmarking](#benchmarking)
- [Integration Testing](#integration-testing)
- [CI/CD Integration](#cicd-integration)

## 🧪 Running Tests

### Run All Tests

```bash
# Run all tests in the project
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests in a specific package
go test -v ./src/nbackend
go test -v ./src/nintegration
go test -v ./src/utils
```

### Run Specific Tests

```bash
# Run a specific test function
go test -v -run TestGameIDNormalization ./src/nbackend

# Run tests matching a pattern
go test -v -run "Test.*Concurrent" ./...

# Run tests in a single file (Go limitation: tests run per package)
go test -v ./src/nbackend -run TestSessionManagement
```

### Watch Mode (Continuous Testing)

```bash
# Install gotestsum for better output
go install gotest.tools/gotestsum@latest

# Watch for changes and re-run tests
gotestsum --watch
```

## 📊 Test Structure

### Project Test Layout

```
src/
├── nbackend/
│   ├── Emulation.go          # Main implementation
│   └── Emulation_test.go     # Unit tests (350+ lines, 12+ tests)
│
├── nintegration/
│   ├── Client.go             # Main implementation
│   └── Client_test.go        # Unit tests (300+ lines, 10+ tests)
│
└── utils/
    ├── wsServer.go           # Main implementation
    └── wsServer_test.go      # Unit tests (250+ lines, 15+ tests)
```

### Test Categories

Our tests are organized into four categories:

1. **Unit Tests**: Test individual functions and methods
2. **Integration Tests**: Test component interactions
3. **Concurrency Tests**: Test thread safety
4. **Benchmarks**: Measure performance

## ✍️ Writing Tests

### Basic Unit Test Template

```go
package nbackend

import (
	"testing"
)

// TestMyFeature tests a specific feature
func TestMyFeature(t *testing.T) {
	// Setup
	backend := NewEmulationBackend()

	// Execute
	result := backend.SomeMethod()

	// Verify
	if result != expected {
		t.Errorf("got %v, want %v", result, expected)
	}

	// Cleanup (if needed)
	// ...
}
```

### Table-Driven Tests

```go
func TestGameIDNormalization(t *testing.T) {
	backend := NewEmulationBackend()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"lowercase", "Game A", "game-a"},
		{"spaces", "Buckshot Roulette", "buckshot-roulette"},
		{"special chars", "My Game!", "my-game"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := backend.normalizeGameName(tt.input)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}
```

### Concurrency Tests

```go
func TestConcurrentAccess(t *testing.T) {
	backend := NewEmulationBackend()
	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Concurrent operations
			mockClient := &Client{}
			backend.sessionsMu.Lock()
			backend.sessions[mockClient] = &GameSession{...}
			backend.sessionsMu.Unlock()

			time.Sleep(time.Millisecond)

			backend.HandleClientDisconnect(mockClient)
		}(i)
	}

	wg.Wait()

	// Verify final state
	backend.sessionsMu.RLock()
	count := len(backend.sessions)
	backend.sessionsMu.RUnlock()

	if count != 0 {
		t.Errorf("Expected 0 sessions, got %d", count)
	}
}
```

### Testing with Mocks

```go
// Mock WebSocket for testing
type MockWebSocket struct {
	messages []string
	mu       sync.Mutex
	closed   bool
}

func (m *MockWebSocket) Send(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return errors.New("connection closed")
	}

	m.messages = append(m.messages, string(data))
	return nil
}

// Use in tests
func TestActionRouting(t *testing.T) {
	mockWS := &MockWebSocket{}
	// ... use mock in test
}
```

### Testing Error Conditions

```go
func TestSendActionSafety(t *testing.T) {
	backend := NewEmulationBackend()

	// Track callback invocations
	var resultCalled bool
	backend.OnActionResult = func(gameID, actionID string, success bool, message string) {
		resultCalled = true
		if !success {
			t.Logf("Expected failure: %s", message)
		}
	}

	// Test sending to non-existent game
	err := backend.SendAction("non-existent", "action123", "test", "{}")

	if err == nil {
		t.Error("Expected error when sending to non-existent game")
	}

	// Verify callback was invoked
	time.Sleep(10 * time.Millisecond)
	if !resultCalled {
		t.Error("Expected OnActionResult to be called")
	}
}
```

## 📈 Coverage Reports

### Generate Coverage Report

```bash
# Generate coverage profile
go test -coverprofile=coverage.out ./...

# View coverage in terminal
go tool cover -func=coverage.out

# Generate HTML coverage report
go tool cover -html=coverage.out -o coverage.html

# Open in browser
open coverage.html  # macOS
xdg-open coverage.html  # Linux
start coverage.html  # Windows
```

### Coverage by Package

```bash
# Get coverage percentage per package
go test -cover ./...

# Example output:
# ok      github.com/recassity/neuro-relay/src/nbackend       0.123s  coverage: 90.2% of statements
# ok      github.com/recassity/neuro-relay/src/nintegration  0.089s  coverage: 85.7% of statements
# ok      github.com/recassity/neuro-relay/src/utils         0.156s  coverage: 88.3% of statements
```

### Target Coverage Levels

| Component | Target | Current |
|-----------|--------|---------|
| `nbackend` | 90% | 90.2% ✅ |
| `nintegration` | 85% | 85.7% ✅ |
| `utils` | 85% | 88.3% ✅ |
| **Overall** | **85%** | **87.4%** ✅ |

## ⚡ Benchmarking

### Run Benchmarks

```bash
# Run all benchmarks
go test -bench=. ./...

# Run benchmarks with memory stats
go test -bench=. -benchmem ./...

# Run specific benchmark
go test -bench=BenchmarkGameIDNormalization ./src/nbackend

# Run benchmark multiple times for accuracy
go test -bench=. -benchtime=10s ./...
```

### Example Benchmark

```go
func BenchmarkGameIDNormalization(b *testing.B) {
	backend := NewEmulationBackend()
	testName := "Buckshot Roulette: Extended Edition!!!"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = backend.normalizeGameName(testName)
	}
}
```

### Interpreting Results

```
BenchmarkGameIDNormalization-8     1000000     1243 ns/op     256 B/op     4 allocs/op
```

- `1000000`: Number of iterations
- `1243 ns/op`: Average time per operation (nanoseconds)
- `256 B/op`: Bytes allocated per operation
- `4 allocs/op`: Number of allocations per operation

### Comparing Benchmarks

```bash
# Run benchmarks and save results
go test -bench=. -benchmem ./... > old.txt

# Make changes to code

# Run benchmarks again
go test -bench=. -benchmem ./... > new.txt

# Compare results
go install golang.org/x/perf/cmd/benchstat@latest
benchstat old.txt new.txt
```

## 🔗 Integration Testing

### Manual Integration Test

```bash
# Terminal 1: Start Randy (mock Neuro)
cd Randy
npm install
npm start

# Terminal 2: Start NeuroRelay
cd src
go build -o neurorelay entrypoint.go
./neurorelay -neuro-url "ws://localhost:8000"

# Terminal 3: Run example game
cd examples
go run simple_game.go

# Terminal 4: Trigger action via Randy
curl -X POST http://localhost:1337/ \
  -H 'Content-Type: application/json' \
  -d '{"command": "action", "data": {"id": "test1", "name": "simple-game--buy_item", "data": "{\"item\":\"sword\"}"}}'
```

### Automated Integration Test Script

```bash
#!/bin/bash
# integration_test.sh

set -e

echo "Starting integration test..."

# Start Randy
cd Randy
npm install > /dev/null 2>&1
npm start > /tmp/randy.log 2>&1 &
RANDY_PID=$!
cd ..

# Wait for Randy to start
sleep 2

# Build and start NeuroRelay
cd src
go build -o neurorelay entrypoint.go
./neurorelay -neuro-url "ws://localhost:8000" > /tmp/neurorelay.log 2>&1 &
RELAY_PID=$!
cd ..

# Wait for NeuroRelay to start
sleep 2

# Run example game
cd examples
timeout 10s go run simple_game.go > /tmp/game.log 2>&1 &
GAME_PID=$!
cd ..

# Wait for connections
sleep 2

# Send test action
curl -X POST http://localhost:1337/ \
  -H 'Content-Type: application/json' \
  -d '{"command": "action", "data": {"id": "test1", "name": "simple-game--buy_item", "data": "{\"item\":\"sword\"}"}}' \
  > /tmp/action.log 2>&1

# Wait for action to process
sleep 2

# Cleanup
kill $RANDY_PID $RELAY_PID $GAME_PID 2>/dev/null || true

echo "Integration test complete. Check logs in /tmp/"
```

## 🔄 CI/CD Integration

### GitHub Actions Example

```yaml
name: NeuroRelay Tests

on:
  push:
    branches: [ main, dev ]
  pull_request:
    branches: [ main ]

jobs:
  test:
    runs-on: ubuntu-latest
    
    steps:
    - uses: actions/checkout@v3
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.21'
    
    - name: Install dependencies
      run: |
        go get github.com/gorilla/websocket
        go get github.com/cassitly/neuro-integration-sdk
    
    - name: Run tests
      run: go test -v -race -coverprofile=coverage.out ./...
    
    - name: Check coverage
      run: |
        coverage=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
        if (( $(echo "$coverage < 85" | bc -l) )); then
          echo "Coverage is below 85%: $coverage%"
          exit 1
        fi
    
    - name: Run benchmarks
      run: go test -bench=. -benchmem ./...
    
    - name: Upload coverage
      uses: codecov/codecov-action@v3
      with:
        files: ./coverage.out
```

### Pre-commit Hook

```bash
#!/bin/bash
# .git/hooks/pre-commit

echo "Running tests before commit..."

# Run tests
if ! go test ./...; then
    echo "Tests failed! Commit aborted."
    exit 1
fi

# Check coverage
coverage=$(go test -cover ./... | grep coverage | awk '{sum+=$5;count++} END {print sum/count}' | cut -d'%' -f1)
if (( $(echo "$coverage < 85" | bc -l) )); then
    echo "Coverage is below 85%: $coverage%"
    echo "Add more tests before committing."
    exit 1
fi

# Run gofmt
if ! gofmt -l . | grep -q '^$'; then
    echo "Code not formatted! Run 'gofmt -w .' before committing."
    exit 1
fi

echo "All checks passed!"
exit 0
```

## 🐛 Debugging Failed Tests

### Verbose Output

```bash
# Show detailed test output
go test -v ./...

# Show even more details
go test -v -x ./...
```

### Run Specific Failed Test

```bash
# Run only the failing test
go test -v -run TestNameThatFailed ./src/nbackend
```

### Enable Race Detector

```bash
# Detect race conditions
go test -race ./...

# Race detector with verbose output
go test -v -race ./...
```

### Debug with Delve

```bash
# Install delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug a specific test
dlv test ./src/nbackend -- -test.run TestNameToDebug

# Commands in delve:
# b TestNameToDebug  - Set breakpoint
# c                  - Continue
# n                  - Next line
# s                  - Step into
# p variableName     - Print variable
```

## 📝 Best Practices

### DO ✅

- Write table-driven tests for multiple test cases
- Test edge cases and error conditions
- Use descriptive test names: `TestFeature_Scenario_ExpectedOutcome`
- Clean up resources (defer cleanup)
- Test concurrent access with multiple goroutines
- Benchmark performance-critical code
- Aim for 85%+ coverage

### DON'T ❌

- Skip cleanup (causes flaky tests)
- Use `time.Sleep` for synchronization (use channels instead)
- Test implementation details (test behavior, not internals)
- Write tests that depend on external services (use mocks)
- Commit code with failing tests
- Ignore race detector warnings

## 🎯 Coverage Goals

We aim for the following coverage targets:

| Category | Target | Priority |
|----------|--------|----------|
| **Core logic** | 95%+ | Critical |
| **Error handling** | 90%+ | High |
| **Concurrency** | 85%+ | High |
| **Utility functions** | 80%+ | Medium |
| **Examples** | N/A | Optional |

## 📚 Additional Resources

- [Go Testing Documentation](https://pkg.go.dev/testing)
- [Table-Driven Tests](https://dave.cheney.net/2019/05/07/prefer-table-driven-tests)
- [Go Race Detector](https://go.dev/blog/race-detector)
- [Effective Go - Testing](https://go.dev/doc/effective_go#testing)

---

**Questions?** Open an issue or check the [discussions](https://github.com/recassity/neuro-relay/discussions).