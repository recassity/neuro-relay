# NeuroRelay - Multi-Game Integration Hub for Neuro-sama

NeuroRelay is a multiplexing system that allows multiple game integrations to connect to Neuro-sama simultaneously. It acts as an intelligent relay between games and Neuro, handling action routing, game identification, and compatibility management.

## Features

- **🎮 Multi-Game Support**: Connect multiple games to Neuro at the same time
- **🔀 Intelligent Action Routing**: Automatically prefixes and routes actions based on game ID
- **🔒 Compatibility Lock**: Protects against conflicts with non-NeuroRelay compatible integrations
- **📡 Transparent Relay**: Games use standard Neuro API without modifications
- **🏷️ Game Identification**: Automatic game ID generation from game names

## Architecture

```
┌─────────────────┐
│   Game A           │ ──┐
└─────────────────┘   │
                         │      ┌ ─────────────────┐       ┌──────────────┐
┌─────────────────┐   ├─────│ NeuroRelay          │──────│   Neuro-sama    │
│   Game B           │ ──┤      │   Emulated Backend  │       │   (Real)        │
└─────────────────┘   ├─────│                     |       └──────────────┘
                         │      └ ─────────────────┘
┌─────────────────┐   │
│   Game C           │ ──┘
└─────────────────┘
```

## How It Works

### 1. Emulated Backend
Games connect to NeuroRelay's emulated Neuro backend instead of the real one. The emulated backend:
- Accepts standard Neuro API messages
- Tracks connected games and their sessions
- Manages compatibility locking
- Processes and forwards messages to the integration client

### 2. Integration Client
The integration client connects to the real Neuro backend and:
- Registers as a single game (e.g., "Game Hub")
- Receives processed actions from the emulated backend
- Prefixes action names with game IDs (e.g., `game-a/buy_books`)
- Routes Neuro's responses back to the appropriate games

### 3. Action Prefixing
When a game registers an action:
```json
{
  "command": "actions/register",
  "game": "Game A",
  "data": {
    "actions": [{"name": "buy_books", "description": "..."}]
  }
}
```

NeuroRelay transforms it to:
```json
{
  "command": "actions/register",
  "game": "Game Hub",
  "data": {
    "actions": [{"name": "game-a/buy_books", "description": "..."}]
  }
}
```

## Compatibility System

### NeuroRelay-Compatible Integrations
Integrations can declare compatibility by including `nrelay-compatible` in their startup message:

```json
{
  "command": "startup",
  "game": "My Game",
  "data": {
    "nrelay-compatible": "1.0.0"
  }
}
```

Compatible integrations can coexist and share Neuro's attention.

### Non-Compatible Integrations
If an integration doesn't include the `nrelay-compatible` field:
1. The backend **locks** to that integration
2. All other integration attempts are rejected with `nrelay/locked` error
3. The lock persists until the non-compatible integration disconnects

This ensures backward compatibility with existing integrations.

## Game ID Generation

Game names are normalized to create game IDs:
- `"Game A"` → `"game-a"`
- `"Buckshot Roulette"` → `"buckshot-roulette"`
- `"My Amazing Game!"` → `"my-amazing-game"`

Rules:
- Lowercase conversion
- Spaces become hyphens
- Only alphanumeric characters and hyphens allowed
- Multiple consecutive hyphens collapsed
- Leading/trailing hyphens removed

## Installation

### Prerequisites
- Go 1.21 or higher
- Access to a Neuro backend instance

### Dependencies
```bash
go get github.com/gorilla/websocket
go get github.com/cassitly/neuro-integration-sdk
```

### Build
```bash
cd src
go build -o neurorelay entrypoint.go
```

## Usage

### Starting NeuroRelay
```bash
./neurorelay \
  -name "Game Hub" \
  -neuro-url "ws://localhost:8000" \
  -emulated-addr "127.0.0.1:8001"
```

### Command Line Options
- `-name`: Name shown to Neuro (default: "Game Hub")
- `-neuro-url`: Neuro backend WebSocket URL (default: "ws://localhost:8000")
- `-emulated-addr`: Address for emulated backend (default: "127.0.0.1:8001")

### Connecting Games
Games should connect to the emulated backend:
```go
client, err := neuro.NewClient(neuro.ClientConfig{
    Game:         "My Game",
    WebsocketURL: "ws://127.0.0.1:8001", // NeuroRelay instead of Neuro
})
```

For NeuroRelay-compatible integrations, include version in startup:
```go
// In your custom startup implementation
msg := map[string]interface{}{
    "command": "startup",
    "game":    "My Game",
    "data": map[string]interface{}{
        "nrelay-compatible": "1.0.0",
    },
}
```

## Configuration File

NeuroRelay uses `src/resources/authentication.yaml` for configuration:

```yaml
nakurity-backend:
  host: "127.0.0.1"
  port: 8001

nakurity-client:
  host: "127.0.0.1"
  port: 8000
```

## Project Structure

```
src/
├── entrypoint.go              # Main entry point
├── nIntegrationClient.go      # Integration client (relay to Neuro)
├── nbackend/
│   └── Emulation.go          # Emulated backend for games
├── utils/
│   └── wsServer.go           # WebSocket server utilities
└── resources/
    └── authentication.yaml   # Configuration
```

## API Reference

### EmulationBackend

```go
// Create a new emulated backend
backend := nbackend.NewEmulationBackend()

// Set up callbacks
backend.OnActionRegistered = func(gameID, actionName string, action ActionDefinition) {
    // Handle action registration
}

backend.OnContext = func(gameID, message string, silent bool) {
    // Handle context messages
}

// Start the server
backend.Start("127.0.0.1:8001")
```

### IntegrationClient

```go
// Create integration client
client, err := nintegration.NewIntegrationClient(nintegration.IntegrationClientConfig{
    RelayName:    "Game Hub",
    NeuroURL:     "ws://localhost:8000",
    EmulatedAddr: "127.0.0.1:8001",
})

// Start the relay
client.Start()

// Get connected games
games := client.GetConnectedGames() // map[gameID]gameName

// Check if locked
locked := client.IsBackendLocked()
```

## Message Flow Examples

### Action Registration
```
Game A → NeuroRelay: register "buy_books"
NeuroRelay → Neuro: register "game-a/buy_books"
```

### Action Execution
```
Neuro → NeuroRelay: execute "game-a/buy_books"
NeuroRelay → Game A: execute "buy_books"
Game A → NeuroRelay: result success
NeuroRelay → Neuro: result success
```

### Context Messages
```
Game A → NeuroRelay: context "Player bought a book"
NeuroRelay → Neuro: context "[game-a] Player bought a book"
```

## Error Handling

### Backend Locked Error
When a non-compatible integration is connected and another tries to connect:
```json
{
  "command": "nrelay/locked",
  "data": {
    "error": "A non-NeuroRelay compatible integration is currently connected"
  }
}
```

### Game Session Not Found
If an action targets a disconnected game:
```
Error: game session not found: game-xyz
```

## Logging

NeuroRelay provides comprehensive logging:
- Client connections/disconnections
- Action registrations/unregistrations
- Message forwarding
- Lock state changes
- Errors and warnings

Example output:
```
Backend locked to non-NeuroRelay compatible integration: Game A
Registered action: buy_books -> game-a/buy_books
Forwarding context to Neuro: [game-a] Player started game (silent: false)
Executing relayed action: game-a/buy_books (id: abc123, game: game-a)
```

## Best Practices

1. **Use Compatibility Field**: Always include `nrelay-compatible` for new integrations
2. **Unique Game Names**: Use descriptive, unique names for each game
3. **Error Handling**: Handle `nrelay/locked` errors gracefully
4. **Clean Disconnection**: Properly close connections to unlock the backend
5. **Action Naming**: Follow Neuro API conventions (lowercase, underscores/hyphens)

## Troubleshooting

### Games Can't Connect
- Check if backend is locked (look for "Backend locked" in logs)
- Verify the emulated backend address is correct
- Ensure no firewall blocks the port

### Actions Not Working
- Verify action names are properly prefixed in logs
- Check that the game session still exists
- Look for action routing errors in logs

### Backend Won't Unlock
- Wait for the non-compatible integration to disconnect
- Restart NeuroRelay if necessary
- Check for connection timeout issues

## Contributing

Contributions are welcome! Please:
1. Follow Go conventions and style guidelines
2. Add tests for new features
3. Update documentation
4. Handle errors appropriately

## License

MIT License - see [LICENSE](./LICENSE) file for details

## Credits

- Built with [neuro-integration-sdk](https://github.com/cassitly/neuro-integration-sdk)
- WebSocket library: [gorilla/websocket](https://github.com/gorilla/websocket)
- Designed for integration with [Neuro-sama](https://www.twitch.tv/vedal987)

## Version History

### 1.0.0 (Current)
- Initial release
- Multi-game multiplexing
- Compatibility lock system
- Action prefixing and routing
- Comprehensive logging
