package utilities

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// MessageHandler is called when a message arrives from a client.
// Implementations may call c.Send(...) to reply to the client.
type MessageHandler func(c *Client, messageType int, data []byte)

// Server is a reusable websocket server.
type Server struct {
	Upgrader websocket.Upgrader

	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
	mu         sync.RWMutex

	handler MessageHandler
}

// Client represents a connected websocket client.
type Client struct {
	conn   *websocket.Conn
	send   chan []byte
	server *Server
}

// New creates a new Server with the provided MessageHandler.
// If handler is nil, messages are ignored (but connection still works).
func New(handler MessageHandler) *Server {
	s := &Server{
		Upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			// Allow all origins by default; override if you need stricter checks.
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan []byte),
		handler:    handler,
	}
	// run the internal manager
	go s.run()
	return s
}

// Attach registers the websocket handler on the provided mux under path.
func (s *Server) Attach(mux *http.ServeMux, path string) {
	mux.HandleFunc(path, s.handleWS)
}

// Broadcast sends a payload to all connected clients (fire-and-forget).
func (s *Server) Broadcast(payload []byte) {
	// copy to avoid race if caller reuses slice
	cpy := make([]byte, len(payload))
	copy(cpy, payload)
	s.broadcast <- cpy
}

// handleWS upgrades the connection and starts client pumps.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade failed:", err)
		return
	}
	client := &Client{
		conn:   conn,
		send:   make(chan []byte, 256),
		server: s,
	}
	s.register <- client

	// start pumps
	go client.writePump()
	go client.readPump()
}

// run manages registration, unregistration and broadcasts.
func (s *Server) run() {
	for {
		select {
		case c := <-s.register:
			s.mu.Lock()
			s.clients[c] = true
			s.mu.Unlock()
			log.Println("client registered; total:", len(s.clients))
		case c := <-s.unregister:
			s.mu.Lock()
			if _, ok := s.clients[c]; ok {
				delete(s.clients, c)
				close(c.send)
			}
			s.mu.Unlock()
			log.Println("client unregistered; total:", len(s.clients))
		case msg := <-s.broadcast:
			s.mu.RLock()
			for c := range s.clients {
				// non-blocking send; drop if client buffer full
				select {
				case c.send <- msg:
				default:
					// client is too slow; remove it
					go func(cl *Client) { s.unregister <- cl }(c)
				}
			}
			s.mu.RUnlock()
		}
	}
}

// Close closes the WebSocket connection from the server side
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}

	// Close the WebSocket connection
	// This will trigger the readPump to exit, which calls unregister
	return c.conn.Close()
}

// Send enqueues a message to be written to this client.
func (c *Client) Send(message []byte) {
	// copy to avoid race if caller reuses slice
	cpy := make([]byte, len(message))
	copy(cpy, message)
	select {
	case c.send <- cpy:
	default:
		// client send buffer full; drop and unregister to avoid blocking
		go func() { c.server.unregister <- c }()
	}
}

// readPump reads messages from the websocket and dispatches to the server handler.
func (c *Client) readPump() {
	defer func() {
		c.server.unregister <- c
		c.conn.Close()
	}()

	// Configure read limits and pong handler
	c.conn.SetReadLimit(512 * 1024) // 512KB limit (adjust as needed)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		msgType, msg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Println("unexpected close error:", err)
			}
			break
		}
		// Dispatch to handler (if set)
		if c.server.handler != nil {
			// call handler in its own goroutine to avoid blocking readPump
			go c.server.handler(c, msgType, msg)
		}
	}
}

const (
	// These values follow Gorilla websocket examples.
	pingPeriod = 30 * time.Second
	pongWait   = 60 * time.Second
	writeWait  = 10 * time.Second
)

// writePump writes messages from the send channel to the websocket.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// server closed the channel
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Write a single message (text/binary)
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			_, _ = w.Write(message)

			// Drain other queued messages and write them in the same websocket message if present (optimization)
			n := len(c.send)
			for i := 0; i < n; i++ {
				_, _ = w.Write([]byte{'\n'}) // simple separator — adapt for your protocol
				_, _ = w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
