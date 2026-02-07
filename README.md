# NeuroRelay - Multi-Game Integration Hub for Neuro-sama

[![Version](https://img.shields.io/badge/version-0.1.0--alpha-blue)](https://github.com/Nakashireyumi/neuro-relay/releases)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8)](go.mod)

**NeuroRelay** is a production-ready multiplexing system that enables multiple game integrations to connect to Neuro-sama simultaneously. It acts as an intelligent relay between games and Neuro, handling action routing, game identification, and compatibility management with zero code changes to existing integrations.

## 🎯 What Problem Does This Solve?

The official Neuro backend only supports **one integration at a time**. This means:
- You can't run multiple games concurrently
- Switching games requires disconnecting and reconnecting
- No way to test multiple integrations simultaneously
- Integration developers can't coordinate or share Neuro's attention

**NeuroRelay solves this** by presenting itself as a single integration to Neuro while managing multiple games internally.

## ✨ Key Features

- **🎮 Multi-Game Multiplexing**: Run unlimited games concurrently
- **🔀 Intelligent Action Routing**: Automatic game ID prefixing (`game-a/buy_books`)
- **🔌 Zero Integration Changes**: Works with existing Neuro SDK integrations
- **🏷️ Automatic Game ID Generation**: Converts "Buckshot Roulette" → `buckshot-roulette`
- **🔒 Backward Compatibility**: Non-compatible integrations lock the relay for solo use
- **📡 Transparent Protocol**: Games use standard Neuro API without modifications
- **🛑 Graceful Shutdown**: Per-game and relay-wide shutdown support
- **🧪 NRC Endpoints**: Health checks and version compatibility for advanced integrations

## 🏗️ Architecture

```
┌─────────────────┐
│   Game A        │ ──┐
└─────────────────┘   │
                      │      ┌──────────────────┐       ┌──────────────┐
┌─────────────────┐   ├─────▶│ NeuroRelay       │──────▶│  Neuro-sama  │
│   Game B        │ ──┤      │ Emulated Backend │       │  (Real)      │
└─────────────────┘   ├─────▶│ Integration      │       └──────────────┘
                      │      └──────────────────┘
┌─────────────────┐   │
│   Game C        │ ──┘
└─────────────────┘

Games connect to:        NeuroRelay connects to:
ws://127.0.0.1:8001     ws://localhost:8000
(Emulated Backend)      (Real Neuro Backend)
```

### Component Overview

| Component | Port | Purpose |
|-----------|------|---------|
| **Emulated Backend** | 8001 | Accepts game connections, manages sessions |
| **Integration Client** | - | Connects to Neuro as unified "Game Hub" |
| **Real Neuro Backend** | 8000 | Official Neuro backend (unchanged) |

## 🚀 Quick Start

### Prerequisites

- **Go 1.21+**
- Access to a Neuro backend instance (or Randy for testing)

### Installation

```bash
# Clone the repository
git clone https://github.com/recassity/neuro-relay
cd neuro-relay

# Install dependencies
go get github.com/gorilla/websocket
go get github.com/cassitly/neuro-integration-sdk

# Build
cd src
go build -o neurorelay entrypoint.go
```

### Running NeuroRelay

```bash
./neurorelay \
  -name "Game Hub" \
  -neuro-url "ws://localhost:8000" \
  -emulated-addr "127.0.0.1:8001"
```

**Expected Output:**
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

### Connecting Your Game

In your game integration, simply point to NeuroRelay instead of the real backend:

```go
client, err := neuro.NewClient(neuro.ClientConfig{
    Game:         "My Awesome Game",
    WebsocketURL: "ws://127.0.0.1:8001", // NeuroRelay, not real Neuro!
})
```

**That's it!** No other code changes needed.

## 📊 How It Works

### 1. Action Prefixing

When a game registers an action:

```json
// Game A sends:
{
  "command": "actions/register",
  "game": "Buckshot Roulette",
  "data": {
    "actions": [{"name": "shoot", "description": "..."}]
  }
}
```

NeuroRelay transforms it:

```json
// NeuroRelay forwards to Neuro:
{
  "command": "actions/register",
  "game": "Game Hub",
  "data": {
    "actions": [{"name": "buckshot-roulette--shoot", "description": "..."}]
  }
}
```

### 2. Action Execution Flow

```
Neuro executes: "buckshot-roulette/shoot"
        ↓
Integration Client receives action
        ↓
Strips prefix: "shoot"
        ↓
Routes to Buckshot Roulette game session
        ↓
Game executes and returns result
        ↓
NeuroRelay forwards result to Neuro
```

### 3. Game ID Generation

Game names are automatically normalized:

| Input | Output |
|-------|--------|
| `"Game A"` | `game-a` |
| `"Buckshot Roulette"` | `buckshot-roulette` |
| `"My Amazing Game!"` | `my-amazing-game` |

**Rules:**
- Lowercase conversion
- Spaces → hyphens
- Only alphanumeric + hyphens
- Collapse multiple hyphens
- Trim leading/trailing hyphens

## 🔧 Configuration

### Command Line Options

| Flag | Default | Description |
|------|---------|-------------|
| `-name` | `"Game Hub"` | Name shown to Neuro |
| `-neuro-url` | `ws://localhost:8000` | Real Neuro backend URL |
| `-emulated-addr` | `127.0.0.1:8001` | Emulated backend address |

### Configuration File

Edit `src/resources/authentication.yaml`:

```yaml
nakurity-backend:  # Emulated backend for games
  host: "127.0.0.1"
  port: 8001

nakurity-client:   # Real Neuro backend
  host: "127.0.0.1"
  port: 8000
```

## 🎮 Example: Running Multiple Games

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

All games now share Neuro's attention! Actions are automatically prefixed and routed.

## 🔒 Compatibility System

### NeuroRelay-Compatible Mode

Integrations can declare compatibility by including `nrelay-compatible` in their startup:

```go
startupMsg := map[string]interface{}{
    "command": "startup",
    "game":    "My Game",
    "data": map[string]interface{}{
        "nrelay-compatible": "1.0.0",  // Declare compatibility
    },
}
```

**Benefits:**
- Multiple games run concurrently
- Actions prefixed automatically
- No conflicts between games

### Legacy Mode (Non-Compatible)

If a game **doesn't** include `nrelay-compatible`:
1. NeuroRelay **locks** to that game exclusively
2. Other connections are rejected with `nrelay/locked` error
3. Lock persists until the game disconnects

This ensures **100% backward compatibility** with existing integrations.

## 🧪 Advanced Features

### NRC Endpoints

NeuroRelay-compatible integrations can use custom endpoints:

```json
// Declare compatibility
{
  "command": "nrc-endpoints/startup",
  "game": "My Game",
  "data": {"nr-version": "1.0.0"}
}

// Health check
{
  "command": "nrc-endpoints/health",
  "game": "My Game",
  "data": {
    "include": ["status", "connected-games", "version"]
  }
}
```

See [NRC Endpoints Documentation](docs/NRC%20Endpoints.md) for details.

### Graceful Shutdown

**Shutdown Individual Games:**
```json
// Neuro can execute:
{
  "action": "shutdown_game",
  "data": {"game_id": "buckshot-roulette"}
}
```

**Shutdown NeuroRelay:**
```json
// Neuro sends:
{
  "command": "shutdown/graceful",
  "data": {"wants_shutdown": true}
}
```

See [Shutdown System Documentation](docs/Shutdown%20System.md) for details.

## 📚 Documentation

- **[Architecture](docs/Architecture.md)**: Deep dive into system design
- **[NRC Endpoints](docs/NRC%20Endpoints.md)**: Custom endpoints for enhanced integrations
- **[Shutdown System](docs/Shutdown%20System.md)**: Graceful shutdown implementation
- **[Quickstart](docs/Quickstart.md)**: 5-minute getting started guide

## 🧪 Testing

### With Randy (Mock Neuro)

```bash
# Terminal 1: Start Randy
cd Randy
npm install
npm start

# Terminal 2: Start NeuroRelay
./neurorelay -neuro-url "ws://localhost:8000"

# Terminal 3: Run example game
cd examples
go run example_game.go
```

### Unit Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Verbose output
go test -v ./...
```

## 🐛 Troubleshooting

### `nrelay/locked` Error

**Problem:** Another non-compatible game is connected.

**Solution:** 
1. Wait for the other game to disconnect, or
2. Restart NeuroRelay, or
3. Update your game to be NeuroRelay-compatible

### Actions Not Working

**Check:**
1. Are action names prefixed in logs? (`game-id--action-name`)
2. Is the game session still connected?
3. Check NeuroRelay logs for routing errors

### Can't Connect to NeuroRelay

**Verify:**
- NeuroRelay is running (`ps aux | grep neurorelay`)
- Using correct address (`ws://127.0.0.1:8001`)
- No firewall blocking port 8001

## 🤝 Contributing

We welcome contributions! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Follow Go conventions and add tests
4. Update documentation
5. Submit a pull request

### Code Style

- Use `gofmt` for formatting
- Add godoc comments for public APIs
- Handle errors explicitly
- Write tests for new features

## 📝 Version History

### 0.1.0-alpha (Current)
- Complete Go rewrite from Python
- Multi-game multiplexing
- Backward compatibility system
- NRC endpoints for health/version checks
- Graceful shutdown support
- Comprehensive documentation

### 0.0.1-alpha (Legacy)
- Initial Python implementation
- Basic relay functionality

## 📄 License

MIT License - see [LICENSE](LICENSE) file for details.

## 🙏 Credits

- **Built with**: [neuro-integration-sdk](https://github.com/cassitly/neuro-integration-sdk)
- **WebSocket library**: [gorilla/websocket](https://github.com/gorilla/websocket)
- **Designed for**: [Neuro-sama](https://www.twitch.tv/vedal987)
- **Original Python version**: [Nakurity](https://github.com/Nakashireyumi/neuro-relay)

## 🔗 Links

- **Official Neuro SDK**: https://github.com/VedalAI/neuro-sdk
- **Issue Tracker**: https://github.com/recassity/neuro-relay/issues
- **Discussions**: https://github.com/recassity/neuro-relay/discussions

---

**Ready to get started?** Check out the [Quickstart Guide](docs/Quickstart.md)!