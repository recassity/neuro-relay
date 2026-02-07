# NeuroRelay Quick Start Guide

Get NeuroRelay running in 5 minutes!

## Prerequisites

✅ **Go 1.21 or higher**  
✅ **Access to a Neuro backend** (or Randy for testing)

Not sure if you have Go? Run:
```bash
go version
# Should output: go version go1.21.x or higher
```

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

You should now have a `neurorelay` executable in the `src/` directory.

## Step 3: Start NeuroRelay

```bash
./neurorelay \
  -name "Game Hub" \
  -neuro-url "ws://localhost:8000" \
  -emulated-addr "127.0.0.1:8001"
```

### Expected Output

```
=================================
  NeuroRelay - Integration Hub   
=================================
Version: 0.1.0-alpha

NeuroRelay is running!
- Games can connect to: ws://127.0.0.1:8001
- Connected to Neuro as: Game Hub

Waiting for game integrations to connect...
```

✅ **Success!** NeuroRelay is now running.

## Step 4: Connect Your Game

In your game integration code, change the WebSocket URL:

### Before (Connecting Directly to Neuro):
```go
client, err := neuro.NewClient(neuro.ClientConfig{
    Game:         "My Game",
    WebsocketURL: "ws://localhost:8000", // ❌ Direct to Neuro
})
```

### After (Connecting Through NeuroRelay):
```go
client, err := neuro.NewClient(neuro.ClientConfig{
    Game:         "My Game",
    WebsocketURL: "ws://127.0.0.1:8001", // ✅ Through NeuroRelay
})
```

That's the **only change** you need to make!

## Step 5: Test with Example Game

Run the included example to verify everything works:

```bash
cd examples
go run example_game.go
```

### Expected Output

```
=================================
  Example Game - NeuroRelay Demo
=================================

✅ Connected to NeuroRelay at ws://127.0.0.1:8001
✅ Registered actions:
  - buy_book
  - check_inventory

Game is running! Waiting for Neuro's actions...
Press Ctrl+C to stop
```

## What's Happening Under the Hood?

```
[Your Game] ──ws://127.0.0.1:8001──▶ [NeuroRelay] ──ws://localhost:8000──▶ [Neuro Backend]
                                           │
                                           ▼
                                    Automatic Action Prefixing:
                                    "buy_book" → "my-game--buy_book"
```

1. **Your game** connects to NeuroRelay's emulated backend (port 8001)
2. **NeuroRelay** connects to the real Neuro backend (port 8000) as "Game Hub"
3. **Actions** are automatically prefixed with your game ID
4. **Neuro's responses** are routed back to the correct game

## Running Multiple Games

Now try running multiple games at once:

```bash
# Terminal 1: NeuroRelay (already running)
./neurorelay

# Terminal 2: Start Example Game
cd examples && go run example_game.go

# Terminal 3: Start Another Instance (with different game name)
cd examples && GAME_NAME="Game Two" go run example_game.go
```

Both games now share Neuro's attention! 🎉

## Testing with Randy (Mock Neuro)

If you don't have access to a real Neuro backend, use Randy:

```bash
# Terminal 1: Start Randy
cd Randy
npm install
npm start
# Randy runs on ws://localhost:8000

# Terminal 2: Start NeuroRelay
cd src
./neurorelay -neuro-url "ws://localhost:8000"

# Terminal 3: Run your game
cd examples
go run example_game.go
```

Randy will randomly execute actions, simulating Neuro's behavior.

## Making Your Game NeuroRelay-Compatible

To enable full multiplexing and avoid locking the relay, add this to your startup:

```go
// After connecting, send custom startup with compatibility flag
startupMsg := map[string]interface{}{
    "command": "startup",
    "game":    "My Game",
    "data": map[string]interface{}{
        "nrelay-compatible": "1.0.0", // ✅ Add this line
    },
}

// Send via raw WebSocket (SDK doesn't support custom startup yet)
msgBytes, _ := json.Marshal(startupMsg)
ws.Send(msgBytes)
```

**Benefits:**
- ✅ Multiple games run concurrently
- ✅ Actions automatically prefixed
- ✅ No locking behavior

Without this flag, NeuroRelay will lock to your game exclusively (backward compatibility mode).

## Command Line Options

```bash
./neurorelay [options]
```

| Option | Default | Description |
|--------|---------|-------------|
| `-name` | `"Game Hub"` | Name shown to Neuro |
| `-neuro-url` | `ws://localhost:8000` | Real Neuro backend WebSocket URL |
| `-emulated-addr` | `127.0.0.1:8001` | Address for emulated backend |

### Examples

**Custom relay name:**
```bash
./neurorelay -name "Recass's Game Hub"
```

**Different ports:**
```bash
./neurorelay -neuro-url "ws://192.168.1.100:8000" -emulated-addr "127.0.0.1:9000"
```

**Connect to remote Neuro backend:**
```bash
./neurorelay -neuro-url "wss://neuro.example.com:8000"
```

## Configuration File

For persistent configuration, edit `src/resources/authentication.yaml`:

```yaml
nakurity-backend:  # Where games connect
  host: "127.0.0.1"
  port: 8001

nakurity-client:   # Real Neuro backend
  host: "127.0.0.1"
  port: 8000
```

Command line flags override these settings.

## Troubleshooting

### "Connection refused" Error

**Problem:** Can't connect to NeuroRelay.

**Solutions:**
1. Verify NeuroRelay is running: `ps aux | grep neurorelay`
2. Check the port: `netstat -an | grep 8001`
3. Verify firewall isn't blocking port 8001

### "nrelay/locked" Error

**Problem:** Another non-compatible game is connected.

**Solution:**
```bash
# Option 1: Stop the other game and reconnect
# Option 2: Restart NeuroRelay
pkill neurorelay && ./neurorelay

# Option 3: Make your game NeuroRelay-compatible (see above)
```

### Actions Not Executing

**Check:**
1. Look for action names in logs: `grep "game-id--action" neurorelay.log`
2. Verify game session is still connected
3. Check if action registration succeeded

**Enable verbose logging:**
```bash
./neurorelay -v  # Add this flag for detailed logs
```

### NeuroRelay Won't Start

**Common issues:**

**Port already in use:**
```bash
# Find what's using port 8001
lsof -i :8001

# Kill it or use a different port
./neurorelay -emulated-addr "127.0.0.1:8002"
```

**Can't connect to Neuro backend:**
```bash
# Verify Neuro backend is running
curl -i -N -H "Connection: Upgrade" \
  -H "Upgrade: websocket" \
  http://localhost:8000/

# If using Randy, make sure it's started:
cd Randy && npm start
```

## Next Steps

Now that you have NeuroRelay running:

1. **Read the [Architecture](Architecture.md)** to understand message flow
2. **Explore [NRC Endpoints](NRC%20Endpoints.md)** for health checks and versioning
3. **Review the [Shutdown System](Shutdown%20System.md)** for graceful shutdowns
4. **Check out [Example Game](../examples/example_game.go)** for integration patterns

## Getting Help

- **Issues**: https://github.com/recassity/neuro-relay/issues
- **Discussions**: https://github.com/recassity/neuro-relay/discussions
- **Official Neuro SDK**: https://github.com/VedalAI/neuro-sdk

## Summary Checklist

- ✅ Go 1.21+ installed
- ✅ Dependencies installed (`go get ...`)
- ✅ NeuroRelay built (`go build`)
- ✅ NeuroRelay running (port 8001)
- ✅ Game connects to `ws://127.0.0.1:8001`
- ✅ Actions prefixed automatically
- ✅ Multiple games can connect

**Happy integrating!** 🎮🤖