package main

import (
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// Example game demonstrating NeuroRelay Custom (NRC) Endpoints

type NRCompatibleGame struct {
	conn     *websocket.Conn
	gameID   string
	items    map[string]int
	nrActive bool
}

func NewNRCompatibleGame() *NRCompatibleGame {
	return &NRCompatibleGame{
		gameID: "nr-example-game",
		items: map[string]int{
			"coins":   100,
			"gems":    10,
			"potions": 5,
		},
		nrActive: false,
	}
}

// Send a message to NeuroRelay
func (g *NRCompatibleGame) sendMessage(command string, data map[string]interface{}) error {
	msg := map[string]interface{}{
		"command": command,
		"game":    "NR Example Game",
	}
	if data != nil {
		msg["data"] = data
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return g.conn.WriteMessage(websocket.TextMessage, msgBytes)
}

// Read a message from NeuroRelay
func (g *NRCompatibleGame) readMessage() (map[string]interface{}, error) {
	_, msgBytes, err := g.conn.ReadMessage()
	if err != nil {
		return nil, err
	}

	var msg map[string]interface{}
	if err := json.Unmarshal(msgBytes, &msg); err != nil {
		return nil, err
	}

	return msg, nil
}

// Initialize NeuroRelay compatibility
func (g *NRCompatibleGame) initializeNRCompatibility() error {
	log.Println("🔧 Initializing NeuroRelay compatibility...")

	// Send NRC startup
	err := g.sendMessage("nrc-endpoints/startup", map[string]interface{}{
		"nr-version": "1.0.0",
	})
	if err != nil {
		return err
	}

	// Wait for acknowledgment
	msg, err := g.readMessage()
	if err != nil {
		return err
	}

	if cmd, ok := msg["command"].(string); ok {
		switch cmd {
		case "nrc-endpoints/startup-ack":
			log.Println("✅ NeuroRelay compatibility enabled!")
			if data, ok := msg["data"].(map[string]interface{}); ok {
				if features, ok := data["features"].(map[string]interface{}); ok {
					log.Printf("📋 Available features: %+v", features)
				}
			}
			g.nrActive = true
			return nil

		case "nrc-endpoints/version-mismatch":
			log.Println("⚠️ Version mismatch!")
			if data, ok := msg["data"].(map[string]interface{}); ok {
				log.Printf("Requested: %v", data["requested"])
				log.Printf("Available: %v", data["available"])
				log.Printf("Suggestion: %v", data["suggestion"])
			}
			return nil

		case "nrc-endpoints/error":
			log.Println("❌ NRC startup error!")
			if data, ok := msg["data"].(map[string]interface{}); ok {
				log.Printf("Error: %v", data["error"])
			}
			return nil
		}
	}

	return nil
}

// Query health status
func (g *NRCompatibleGame) queryHealth() {
	if !g.nrActive {
		log.Println("⚠️ NR compatibility not active, skipping health check")
		return
	}

	log.Println("🏥 Querying NeuroRelay health...")

	err := g.sendMessage("nrc-endpoints/health", map[string]interface{}{
		"include": []string{
			"status",
			"version",
			"connected-games",
			"features",
			"lock-status",
		},
	})
	if err != nil {
		log.Printf("Error sending health check: %v", err)
		return
	}

	// Read health response
	msg, err := g.readMessage()
	if err != nil {
		log.Printf("Error reading health response: %v", err)
		return
	}

	if cmd, ok := msg["command"].(string); ok && cmd == "nrc-endpoints/health-response" {
		if data, ok := msg["data"].(map[string]interface{}); ok {
			log.Println("📊 Health Report:")
			log.Printf("  Status: %v", data["status"])
			log.Printf("  NR Version: %v", data["nr-version"])
			log.Printf("  Total Games: %v", data["total-games"])
			
			if games, ok := data["connected-games"].([]interface{}); ok {
				log.Printf("  Connected Games:")
				for _, game := range games {
					if g, ok := game.(map[string]interface{}); ok {
						log.Printf("    - %s (%s)", g["name"], g["id"])
					}
				}
			}
			
			if features, ok := data["features"].(map[string]interface{}); ok {
				log.Printf("  Features: %+v", features)
			}
			
			log.Printf("  Backend Locked: %v", data["backend-locked"])
		}
	}
}

// Register actions
func (g *NRCompatibleGame) registerActions() error {
	actions := []map[string]interface{}{
		{
			"name":        "buy_item",
			"description": "Buy an item from the shop",
			"schema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"item": map[string]interface{}{
						"type":        "string",
						"description": "Item to buy",
						"enum":        []string{"sword", "shield", "potion"},
					},
				},
				"required": []string{"item"},
			},
		},
		{
			"name":        "check_balance",
			"description": "Check current coin balance",
		},
	}

	return g.sendMessage("actions/register", map[string]interface{}{
		"actions": actions,
	})
}

// Handle incoming messages
func (g *NRCompatibleGame) handleMessages() {
	for {
		msg, err := g.readMessage()
		if err != nil {
			log.Printf("Error reading message: %v", err)
			return
		}

		if cmd, ok := msg["command"].(string); ok {
			switch cmd {
			case "action":
				g.handleAction(msg)
			default:
				log.Printf("Received: %s", cmd)
			}
		}
	}
}

// Handle action from Neuro
func (g *NRCompatibleGame) handleAction(msg map[string]interface{}) {
	data, ok := msg["data"].(map[string]interface{})
	if !ok {
		return
	}

	actionID, _ := data["id"].(string)
	actionName, _ := data["name"].(string)
	// This is apparently unused.
	// actionData, _ := data["data"].(string)

	log.Printf("⚡ Action received: %s (id: %s)", actionName, actionID)

	// Simple action handling
	success := true
	resultMsg := "Action completed"

	switch actionName {
	case "buy_item":
		log.Println("🛒 Buying item...")
		resultMsg = "Item purchased!"
	case "check_balance":
		log.Printf("💰 Current balance: %d coins", g.items["coins"])
		resultMsg = "Balance checked"
	default:
		success = false
		resultMsg = "Unknown action"
	}

	// Send result
	g.sendMessage("action/result", map[string]interface{}{
		"id":      actionID,
		"success": success,
		"message": resultMsg,
	})
}

func main() {
	relayURL := os.Getenv("NEURORELAY_URL")
	if relayURL == "" {
		relayURL = "ws://127.0.0.1:8001"
	}

	log.Println("===========================================")
	log.Println("  NR-Compatible Example Game")
	log.Println("  Demonstrating NRC Endpoints")
	log.Println("===========================================")
	log.Println()

	// Create game
	game := NewNRCompatibleGame()

	// Connect to NeuroRelay
	conn, _, err := websocket.DefaultDialer.Dial(relayURL, nil)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()
	game.conn = conn

	log.Printf("✅ Connected to NeuroRelay at %s", relayURL)

	// Step 1: Send standard startup
	log.Println("📤 Sending standard startup...")
	if err := game.sendMessage("startup", nil); err != nil {
		log.Fatalf("Failed to send startup: %v", err)
	}

	// Step 2: Initialize NR compatibility
	if err := game.initializeNRCompatibility(); err != nil {
		log.Printf("⚠️ NR compatibility failed: %v", err)
		log.Println("Continuing in standard mode...")
	}

	// Step 3: Register actions
	log.Println("📝 Registering actions...")
	if err := game.registerActions(); err != nil {
		log.Fatalf("Failed to register actions: %v", err)
	}
	log.Println("✅ Actions registered")

	// Step 4: Query initial health
	game.queryHealth()

	// Periodic health checks
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			game.queryHealth()
		}
	}()

	// Handle messages in background
	go game.handleMessages()

	log.Println()
	log.Println("🎮 Game is running!")
	log.Println("   - NR compatibility demonstrated")
	log.Println("   - Health checks every 30 seconds")
	log.Println("   - Ready for Neuro's actions")
	log.Println()
	log.Println("Press Ctrl+C to stop")
	log.Println()

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println()
	log.Println("👋 Shutting down...")
}
