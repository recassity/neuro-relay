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

// Example game that connects to NeuroRelay
type ExampleGame struct {
	client *neuro.Client
	items  map[string]int
}

func NewExampleGame() *ExampleGame {
	return &ExampleGame{
		items: map[string]int{
			"coins":   100,
			"gems":    10,
			"potions": 5,
		},
	}
}

// ======= ACTIONS =======

// BuyBookAction - Example action to buy a book
type BuyBookAction struct {
	game *ExampleGame
}

func (a *BuyBookAction) GetName() string {
	return "buy_book"
}

func (a *BuyBookAction) GetDescription() string {
	return "Buy a book from the shop. Costs 10 coins."
}

func (a *BuyBookAction) GetSchema() *neuro.ActionSchema {
	return neuro.WrapSchema(map[string]interface{}{
		"book_type": map[string]interface{}{
			"type":        "string",
			"description": "Type of book to buy",
			"enum":        []string{"fiction", "non-fiction", "textbook", "comic"},
		},
	}, []string{"book_type"})
}

func (a *BuyBookAction) Validate(data json.RawMessage) (interface{}, neuro.ExecutionResult) {
	var params struct {
		BookType string `json:"book_type"`
	}

	if err := neuro.ParseActionData(data, &params); err != nil {
		return nil, neuro.NewFailureResult("Invalid parameters")
	}

	validTypes := map[string]bool{
		"fiction": true, "non-fiction": true,
		"textbook": true, "comic": true,
	}

	if !validTypes[params.BookType] {
		return nil, neuro.NewFailureResult("Invalid book type: " + params.BookType)
	}

	// Check if player has enough coins
	if a.game.items["coins"] < 10 {
		return nil, neuro.NewFailureResult("Not enough coins! Need 10 coins.")
	}

	return params.BookType, neuro.NewSuccessResult("Buying " + params.BookType + " book")
}

func (a *BuyBookAction) Execute(state interface{}) {
	bookType := state.(string)
	a.game.items["coins"] -= 10
	log.Printf("📚 Bought a %s book! Coins remaining: %d", bookType, a.game.items["coins"])
}

// CheckInventoryAction - Example action to check inventory
type CheckInventoryAction struct {
	game *ExampleGame
}

func (a *CheckInventoryAction) GetName() string {
	return "check_inventory"
}

func (a *CheckInventoryAction) GetDescription() string {
	return "Check the current inventory and resources"
}

func (a *CheckInventoryAction) GetSchema() *neuro.ActionSchema {
	return nil // No parameters needed
}

func (a *CheckInventoryAction) Validate(data json.RawMessage) (interface{}, neuro.ExecutionResult) {
	return nil, neuro.NewSuccessResult("Checking inventory")
}

func (a *CheckInventoryAction) Execute(state interface{}) {
	log.Println("📦 Current Inventory:")
	for item, count := range a.game.items {
		log.Printf("  - %s: %d", item, count)
	}

	// Send inventory as context back to Neuro
	inventoryMsg := "Current inventory: "
	for item, count := range a.game.items {
		inventoryMsg += item + ": " + string(rune(count+'0')) + ", "
	}
	a.game.client.SendContext(inventoryMsg, false)
}

// ======= MAIN =======

func main() {
	// Configuration
	relayURL := os.Getenv("NEURORELAY_URL")
	if relayURL == "" {
		relayURL = "ws://127.0.0.1:8001" // Default NeuroRelay address
	}

	log.Println("=================================")
	log.Println("  Example Game - NeuroRelay Demo")
	log.Println("=================================")
	log.Println()

	// Create game instance
	game := NewExampleGame()

	// Create Neuro client pointing to NeuroRelay
	client, err := neuro.NewClient(neuro.ClientConfig{
		Game:         "Example Game",
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

	// Send NeuroRelay-compatible startup message
	// This requires a custom startup with the nrelay-compatible field
	// For this example, we'll use the standard SDK which doesn't support this yet
	// In production, you would need to modify the SDK or send a raw message

	// Send initial context
	client.SendContext("Example Game started! You can buy books or check inventory.", false)

	// Register actions
	if err := client.RegisterActions([]neuro.ActionHandler{
		&BuyBookAction{game: game},
		&CheckInventoryAction{game: game},
	}); err != nil {
		log.Fatalf("Failed to register actions: %v", err)
	}

	log.Println("✅ Registered actions:")
	log.Println("  - buy_book")
	log.Println("  - check_inventory")
	log.Println()

	// Send periodic updates
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			client.SendContext("Game is still running. Coins: "+string(rune(game.items["coins"]+'0')), true)
		}
	}()

	// Handle errors
	go func() {
		for err := range client.Errors() {
			log.Printf("❌ Error: %v", err)
		}
	}()

	log.Println("Game is running! Waiting for Neuro's actions...")
	log.Println("Press Ctrl+C to stop")
	log.Println()

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println()
	log.Println("Shutting down Example Game...")
	client.SendShutdownReady()
	log.Println("Goodbye!")
}
