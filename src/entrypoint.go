package entrypoint
// this is the entrypoint for all the other files for neuro-relay

package main

import (
    "log"
    "github.com/cassitly/neuro-integration-sdk"
)

func main() {
    // Create client
    client, err := neuro.NewClient(neuro.ClientConfig{
        Game:         "Game Hub",
        WebsocketURL: "ws://localhost:8000",
    })
    if err != nil {
        log.Fatal(err)
    }

    // Connect
    if err := client.Connect(); err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Send context
    client.SendContext("Game started!", false)

    // Register actions (see examples below)
    // ...
}
