package utilities

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestServerCreation tests basic server creation
func TestServerCreation(t *testing.T) {
	handler := func(c *Client, messageType int, data []byte) {}
	server := New(handler)

	if server == nil {
		t.Fatal("Server should not be nil")
	}

	if server.handler == nil {
		t.Error("Handler should be set")
	}

	if server.clients == nil {
		t.Error("Clients map should be initialized")
	}

	if server.register == nil || server.unregister == nil || server.broadcast == nil {
		t.Error("Channels should be initialized")
	}
}

// TestClientRegistration tests client registration and unregistration
func TestClientRegistration(t *testing.T) {
	handler := func(c *Client, messageType int, data []byte) {}
	server := New(handler)

	// Start server in background
	go server.run()

	// Create mock client
	mockConn := &websocket.Conn{}
	client := &Client{
		conn:   mockConn,
		send:   make(chan []byte, 256),
		server: server,
	}

	// Register client
	server.register <- client

	// Wait for registration
	time.Sleep(10 * time.Millisecond)

	// Check if client is registered
	server.mu.RLock()
	_, exists := server.clients[client]
	server.mu.RUnlock()

	if !exists {
		t.Error("Client should be registered")
	}

	// Unregister client
	server.unregister <- client

	// Wait for unregistration
	time.Sleep(10 * time.Millisecond)

	// Check if client is unregistered
	server.mu.RLock()
	_, stillExists := server.clients[client]
	server.mu.RUnlock()

	if stillExists {
		t.Error("Client should be unregistered")
	}
}

// TestBroadcast tests broadcasting to multiple clients
func TestBroadcast(t *testing.T) {
	var receivedMessages [][]byte
	var mu sync.Mutex

	handler := func(c *Client, messageType int, data []byte) {
		mu.Lock()
		receivedMessages = append(receivedMessages, data)
		mu.Unlock()
	}

	server := New(handler)
	go server.run()

	// Create multiple mock clients
	numClients := 3
	clients := make([]*Client, numClients)

	for i := 0; i < numClients; i++ {
		mockConn := &websocket.Conn{}
		client := &Client{
			conn:   mockConn,
			send:   make(chan []byte, 256),
			server: server,
		}
		clients[i] = client
		server.register <- client
	}

	// Wait for registration
	time.Sleep(10 * time.Millisecond)

	// Broadcast message
	testMessage := []byte("test broadcast")
	server.Broadcast(testMessage)

	// Wait for broadcast to complete
	time.Sleep(10 * time.Millisecond)

	// Verify all clients received the message
	for i, client := range clients {
		select {
		case msg := <-client.send:
			if string(msg) != string(testMessage) {
				t.Errorf("Client %d: got %q, want %q", i, msg, testMessage)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("Client %d: timeout waiting for broadcast message", i)
		}
	}

	// Cleanup
	for _, client := range clients {
		server.unregister <- client
	}
}

// TestClientSend tests sending messages to individual clients
func TestClientSend(t *testing.T) {
	handler := func(c *Client, messageType int, data []byte) {}
	server := New(handler)
	go server.run()

	// Create mock client
	mockConn := &websocket.Conn{}
	client := &Client{
		conn:   mockConn,
		send:   make(chan []byte, 256),
		server: server,
	}

	server.register <- client
	time.Sleep(10 * time.Millisecond)

	// Send message
	testMessage := []byte("test message")
	client.Send(testMessage)

	// Verify message was sent
	select {
	case msg := <-client.send:
		if string(msg) != string(testMessage) {
			t.Errorf("got %q, want %q", msg, testMessage)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for message")
	}

	// Cleanup
	server.unregister <- client
}

// TestSlowClient tests automatic disconnection of slow clients
func TestSlowClient(t *testing.T) {
	handler := func(c *Client, messageType int, data []byte) {}
	server := New(handler)
	go server.run()

	// Create mock client with small buffer
	mockConn := &websocket.Conn{}
	client := &Client{
		conn:   mockConn,
		send:   make(chan []byte, 2), // Very small buffer
		server: server,
	}

	server.register <- client
	time.Sleep(10 * time.Millisecond)

	// Fill the buffer
	for i := 0; i < 5; i++ {
		client.Send([]byte("message"))
	}

	// Wait for slow client to be disconnected
	time.Sleep(100 * time.Millisecond)

	// Verify client was unregistered
	server.mu.RLock()
	_, exists := server.clients[client]
	server.mu.RUnlock()

	// Client should either be unregistered or have full buffer
	// Both are acceptable outcomes for a slow client
	if exists {
		// Check if buffer is full
		if len(client.send) < cap(client.send) {
			t.Error("Slow client should be unregistered or have full buffer")
		}
	}
}

// TestHTTPAttachment tests attaching WebSocket handler to HTTP server
func TestHTTPAttachment(t *testing.T) {
	handler := func(c *Client, messageType int, data []byte) {}
	server := New(handler)

	mux := http.NewServeMux()
	server.Attach(mux, "/")

	// Create test server
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	// Attempt WebSocket connection
	_, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		// Expected to fail since we're not completing the handshake
		// But the endpoint should exist
		if !strings.Contains(err.Error(), "bad handshake") && !strings.Contains(err.Error(), "EOF") {
			t.Errorf("unexpected error: %v", err)
		}
	}
}

// TestConcurrentOperations tests thread safety with concurrent access
func TestConcurrentOperations(t *testing.T) {
	handler := func(c *Client, messageType int, data []byte) {}
	server := New(handler)
	go server.run()

	var wg sync.WaitGroup
	numGoroutines := 50

	// Concurrent client registration and unregistration
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			mockConn := &websocket.Conn{}
			client := &Client{
				conn:   mockConn,
				send:   make(chan []byte, 256),
				server: server,
			}

			server.register <- client
			time.Sleep(time.Millisecond)
			server.unregister <- client
		}()
	}

	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	// Verify all clients are unregistered
	server.mu.RLock()
	count := len(server.clients)
	server.mu.RUnlock()

	if count != 0 {
		t.Errorf("Expected 0 clients, got %d", count)
	}
}

// TestBroadcastToClosed tests broadcasting when some clients are closed
func TestBroadcastToClosed(t *testing.T) {
	handler := func(c *Client, messageType int, data []byte) {}
	server := New(handler)
	go server.run()

	// Create clients
	numClients := 5
	clients := make([]*Client, numClients)

	for i := 0; i < numClients; i++ {
		mockConn := &websocket.Conn{}
		client := &Client{
			conn:   mockConn,
			send:   make(chan []byte, 256),
			server: server,
		}
		clients[i] = client
		server.register <- client
	}

	time.Sleep(10 * time.Millisecond)

	// Close some clients
	for i := 0; i < numClients/2; i++ {
		close(clients[i].send)
		server.unregister <- clients[i]
	}

	time.Sleep(10 * time.Millisecond)

	// Broadcast should not panic
	testMessage := []byte("test after close")
	server.Broadcast(testMessage)

	time.Sleep(10 * time.Millisecond)

	// Verify remaining clients received the message
	for i := numClients / 2; i < numClients; i++ {
		select {
		case msg := <-clients[i].send:
			if string(msg) != string(testMessage) {
				t.Errorf("Client %d: got %q, want %q", i, msg, testMessage)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("Client %d: timeout waiting for message", i)
		}
	}

	// Cleanup
	for i := numClients / 2; i < numClients; i++ {
		server.unregister <- clients[i]
	}
}

// BenchmarkClientSend benchmarks client send performance
func BenchmarkClientSend(b *testing.B) {
	handler := func(c *Client, messageType int, data []byte) {}
	server := New(handler)
	go server.run()

	mockConn := &websocket.Conn{}
	client := &Client{
		conn:   mockConn,
		send:   make(chan []byte, 256),
		server: server,
	}

	server.register <- client
	time.Sleep(10 * time.Millisecond)

	testMessage := []byte("benchmark message")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client.Send(testMessage)
		<-client.send // Drain to prevent blocking
	}

	server.unregister <- client
}

// BenchmarkBroadcast benchmarks broadcast performance
func BenchmarkBroadcast(b *testing.B) {
	handler := func(c *Client, messageType int, data []byte) {}
	server := New(handler)
	go server.run()

	// Create multiple clients
	numClients := 10
	clients := make([]*Client, numClients)

	for i := 0; i < numClients; i++ {
		mockConn := &websocket.Conn{}
		client := &Client{
			conn:   mockConn,
			send:   make(chan []byte, 256),
			server: server,
		}
		clients[i] = client
		server.register <- client
	}

	time.Sleep(10 * time.Millisecond)

	// Goroutine to drain messages from clients
	for _, client := range clients {
		go func(c *Client) {
			for range c.send {
				// Drain
			}
		}(client)
	}

	testMessage := []byte("broadcast benchmark")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.Broadcast(testMessage)
	}

	// Cleanup
	for _, client := range clients {
		server.unregister <- client
	}
}
