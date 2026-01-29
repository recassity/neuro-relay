package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/recassity/neuro-relay/src/nintegration"
)

func main() {
	// Parse command line flags
	relayName := flag.String("name", "Game Hub", "Name of the relay shown to Neuro")
	neuroURL := flag.String("neuro-url", "ws://localhost:8000", "Neuro backend WebSocket URL")
	emulatedAddr := flag.String("emulated-addr", "127.0.0.1:8001", "Address for emulated backend")
	flag.Parse()

	log.Println("=================================")
	log.Println("  NeuroRelay - Integration Hub   ")
	log.Println("=================================")
	log.Printf("Version: %s", "1.0.0")
	log.Println()

	// Create integration client
	client, err := nintegration.NewIntegrationClient(nintegration.IntegrationClientConfig{
		RelayName:    *relayName,
		NeuroURL:     *neuroURL,
		EmulatedAddr: *emulatedAddr,
	})
	if err != nil {
		log.Fatalf("Failed to create integration client: %v", err)
	}

	// Start the relay system
	if err := client.Start(); err != nil {
		log.Fatalf("Failed to start relay: %v", err)
	}

	log.Println()
	log.Println("NeuroRelay is running!")
	log.Println("- Games can connect to: ws://" + *emulatedAddr + "/ws")
	log.Println("- Connected to Neuro as: " + *relayName)
	log.Println()
	log.Println("Waiting for game integrations to connect...")
	log.Println("Press Ctrl+C to stop")
	log.Println()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println()
	log.Println("Shutting down NeuroRelay...")
	client.Stop()
	log.Println("Goodbye!")
}
