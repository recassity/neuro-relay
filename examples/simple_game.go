package main

import (
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cassitly/neuro-integration-sdk"
)

// SimpleGame demonstrates NeuroRelay integration with minimal complexity
type SimpleGame struct {
	client *neuro.Client
	coins  int
}

func NewSimpleGame() *SimpleGame {
	return &SimpleGame{
		coins: 100,
	}
}

// ======= ACTIONS =======

// BuyItemAction - Simple action with parameter validation
type BuyItemAction struct {
	game *SimpleGame
}

func (a *BuyItemAction) GetName() string {
	return "buy_item"
}

func (a *BuyItemAction) GetDescription() string {
	return "Buy an item from the shop. Costs 10 coins."
}

func (a *BuyItemAction) GetSchema() *neuro.ActionSchema {
	return neuro.WrapSchema(map[string]interface{}{
		"item": map[string]interface{}{
			"type":        "string",
			"description": "Item to purchase",
			"enum":        []string{"sword", "shield", "potion"},
		},
	}, []string{"item"})
}

func (a *BuyItemAction) Validate(data json.RawMessage) (interface{}, neuro.ExecutionResult) {
	var params struct {
		Item string `json:"item"`
	}

	// Parse action data
	if err := neuro.ParseActionData(data, &params); err != nil {
		return nil, neuro.NewFailureResult("Invalid parameters")
	}

	// Validate item
	validItems := map[string]bool{
		"sword":  true,
		"shield": true,
		"potion": true,
	}
	if !validItems[params.Item] {
		return nil, neuro.NewFailureResult("Invalid item: " + params.Item)
	}

	// Check if player has enough coins
	if a.game.coins < 10 {
		return nil, neuro.NewFailureResult("Not enough coins! Need 10 coins.")
	}

	return params.Item, neuro.NewSuccessResult("Buying " + params.Item)
}

func (a *BuyItemAction) Execute(state interface{}) {
	item := state.(string)
	a.game.coins -= 10
	log.Printf("✅ Bought %s! Coins remaining: %d", item, a.game.coins)
}

// CheckBalanceAction - Simple action without parameters
type CheckBalanceAction struct {
	game *SimpleGame
}

func (a *CheckBalanceAction) GetName() string {
	return "check_balance"
}

func (a *CheckBalanceAction) GetDescription() string {
	return "Check the current coin balance"
}

func (a *CheckBalanceAction) GetSchema() *neuro.ActionSchema {
	return nil // No parameters needed
}

func (a *CheckBalanceAction) Validate(data json.RawMessage) (interface{}, neuro.ExecutionResult) {
	return nil, neuro.NewSuccessResult("Checking balance")
}

func (a *CheckBalanceAction) Execute(state interface{}) {
	log.Printf("💰 Current balance: %d coins", a.game.coins)
	a.game.client.SendContext("You have "+string(rune(a.game.coins+'0'))+" coins remaining.", false)
}

// ======= MAIN =======

func main() {
	// Get NeuroRelay URL from environment or use default
	relayURL := os.Getenv("NEURORELAY_URL")
	if relayURL == "" {
		relayURL = "ws://127.0.0.1:8001" // Default NeuroRelay address
	}

	log.Println("=================================")
	log.Println("  Simple Game - NeuroRelay Demo")
	log.Println("=================================")
	log.Println()

	// Create game instance
	game := NewSimpleGame()

	// Create Neuro client pointing to NeuroRelay
	client, err := neuro.NewClient(neuro.ClientConfig{
		Game:         "Simple Game",
		WebsocketURL: relayURL,
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	game.client = client

	// Connect to NeuroRelay
	if err := client.Connect(); err != nil {
		log.Fatalf("Failed to connect to NeuroRelay: %v", err)
	}
	defer client.Close()

	log.Printf("✅ Connected to NeuroRelay at %s", relayURL)

	// Send initial context
	client.SendContext("Simple Game started! You can buy items or check your balance.", false)

	// Register actions
	if err := client.RegisterActions([]neuro.ActionHandler{
		&BuyItemAction{game: game},
		&CheckBalanceAction{game: game},
	}); err != nil {
		log.Fatalf("Failed to register actions: %v", err)
	}

	log.Println("✅ Registered actions:")
	log.Println("  - buy_item (sword, shield, potion)")
	log.Println("  - check_balance")
	log.Println()

	// Send periodic status updates
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			if game.coins > 0 {
				client.SendContext("Game is running. Coins: "+string(rune(game.coins+'0')), true)
			}
		}
	}()

	// Handle errors
	go func() {
		for err := range client.Errors() {
			log.Printf("❌ Error: %v", err)
		}
	}()

	log.Println("🎮 Game is running! Waiting for Neuro's actions...")
	log.Println("Press Ctrl+C to stop")
	log.Println()

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println()
	log.Println("Shutting down Simple Game...")
	client.SendShutdownReady()
	log.Println("Goodbye!")
}