# NeuroRelay Quick Start Guide

Get started with NeuroRelay in 5 minutes!

## Prerequisites

- Go 1.21+
- Access to a Neuro backend instance (or use the emulator)

## Step 1: Install Dependencies

```bash
go get github.com/gorilla/websocket
go get github.com/cassitly/neuro-integration-sdk
```

## Step 2: Build NeuroRelay

```bash
cd src
go build -o neurorelay entrypoint.go
```

## Step 3: Start NeuroRelay

```bash
./neurorelay \
  -name "Game Hub" \
  -neuro-url "ws://localhost:8000" \
  -emulated-addr "127.0.0.1:8001"
```

You should see:
```
=================================
  NeuroRelay - Integration Hub   
=================================
Version: 1.0.0

NeuroRelay is running!
- Games can connect to: ws://127.0.0.1:8001/ws
- Connected to Neuro as: Game Hub

Waiting for game integrations to connect...
```

## Step 4: Connect Your Game

In your game integration, connect to NeuroRelay instead of Neuro:

```go
client, err := neuro.NewClient(neuro.ClientConfig{
    Game:         "My Awesome Game",
    WebsocketURL: "ws://127.0.0.1:8001", // NeuroRelay, not Neuro!
})
```

## Step 5: Test with Example Game

Run the included example:

```bash
cd examples
go run example_game.go
```

You should see actions being registered and routed through NeuroRelay!

## What's Happening?

1. **NeuroRelay** connects to the real Neuro backend as "Game Hub"
2. **Your game** connects to NeuroRelay's emulated backend
3. **Actions** are automatically prefixed with your game ID:
   - Your game registers: `buy_book`
   - NeuroRelay forwards to Neuro: `my-awesome-game/buy_book`
4. **Neuro's responses** are routed back to the correct game

## Multiple Games

You can run multiple games simultaneously:

```bash
# Terminal 1: Start NeuroRelay
./neurorelay

# Terminal 2: Start Game A
cd game-a && go run main.go

# Terminal 3: Start Game B
cd game-b && go run main.go

# Terminal 4: Start Game C
cd game-c && go run main.go
```

All games share Neuro's attention!

## Making Your Game NeuroRelay-Compatible

To enable multiplexing, include the version field in startup:

```go
// Custom startup message
startupMsg := map[string]interface{}{
    "command": "startup",
    "game":    "My Game",
    "data": map[string]interface{}{
        "nrelay-compatible": "1.0.0",  // Add this!
    },
}
```

Without this field, NeuroRelay will lock to your game and reject other connections (backward compatibility mode).

## Troubleshooting

### "nrelay/locked" Error
Another non-compatible game is connected. Wait for it to disconnect or restart NeuroRelay.

### Actions Not Working
Check the logs - action names should show up as `game-id/action-name`

### Can't Connect
Verify:
- NeuroRelay is running
- Using correct address (ws://127.0.0.1:8001)
- No firewall blocking port 8001

## Next Steps

- Read the full [README.md](README.md) for detailed documentation
- Explore the [example game](examples/example_game.go)
- Check the [architecture section](README.md#architecture) to understand message flow
- Learn about [game ID generation](README.md#game-id-generation)

## Configuration

Edit `src/resources/authentication.yaml` to customize:

```yaml
nakurity-backend:
  host: "127.0.0.1"
  port: 8001  # Emulated backend port

nakurity-client:
  host: "127.0.0.1"
  port: 8000  # Real Neuro backend port
```

## Support

For issues, check:
1. NeuroRelay logs for errors
2. Game logs for connection issues
3. Neuro backend logs for action routing

Happy integrating! 🎮🤖
