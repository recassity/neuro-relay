# NeuroRelay Architecture

## Overview

NeuroRelay implements a multiplexing layer between multiple game integrations and Neuro-sama, allowing simultaneous connections while maintaining backward compatibility with non-compatible integrations.

## Component Diagram

```
┌──────────────────────────────────────────────────────────────────┐
│                         NeuroRelay System                        │
├──────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌─────────────────────┐         ┌──────────────────────────┐    │
│  │  Emulated Backend   │◄───────►│  Integration Client      │    │
│  │  (nbackend)         │         │  (nIntegrationClient)    │    │
│  │                     │         │                          │    │
│  │ • Message Handler   │         │ • Neuro SDK Client       │    │
│  │ • Session Manager   │         │ • Action Router          │    │
│  │ • Lock Manager      │         │ • Callback Handler       │    │
│  │ • Game ID Generator │         │ • ID Mapper              │    │
│  └─────────────────────┘         └──────────────────────────┘    │
│         ▲                                    │                   │
│         │                                    │                   │
└─────────┼────────────────────────────────────┼───────────────────┘
          │                                    │
          │ ws://127.0.0.1:8001/               │ ws://localhost:8000
          │                                    ▼
  ┌───────┴─────────┐                 ┌──────────────┐
  │  Game A         │                 │  Neuro-sama  │
  │  Game B         │                 │  (Backend)   │
  │  Game C         │                 └──────────────┘
  └─────────────────┘
```

## Core Components

### 1. Emulated Backend (`src/nbackend/Emulation.go`)

The emulated backend presents a standard Neuro API interface to connecting games.

#### Responsibilities:
- Accept WebSocket connections from games
- Parse and validate Neuro protocol messages
- Manage game sessions and state
- Generate unique game IDs from game names
- Enforce compatibility locking
- Route messages to the integration client via callbacks

#### Key Data Structures:

```go
type EmulationBackend struct {
    server     *utilities.Server              // WebSocket server
    sessions   map[*Client]*GameSession       // Active game sessions
    locked     bool                           // Lock state
    lockedToClient *utilities.Client          // Client holding the lock
    
    // Callbacks for integration client
    OnStartup            func(gameID, gameName string)
    OnActionRegistered   func(gameID, actionName string, action ActionDefinition)
    OnActionUnregistered func(gameID, actionName string)
    OnContext            func(gameID, message string, silent bool)
    OnActionResult       func(gameID, actionID string, success bool, message string)
    OnActionForce        func(gameID, state, query string, ...)
}

type GameSession struct {
    GameName         string
    GameID           string                    // Normalized: "game-a"
    LatestActionNum  int
    Actions          map[string]ActionDefinition
    NRelayCompatible bool
    NRelayVersion    string
    Client           *utilities.Client
}
```

#### Message Flow:

```
Game → Emulated Backend: {"command": "startup", "game": "Game A"}
                         ↓
              Generate GameID: "game-a"
              Check compatibility field
              Create session
                         ↓
              Callback: OnStartup("game-a", "Game A")
```

### 2. Integration Client (`src/nIntegrationClient.go`)

The integration client connects to the real Neuro backend and manages bidirectional message routing.

#### Responsibilities:
- Connect to Neuro backend as a single unified game
- Register actions with prefixed names (`game-id--action-name`)
- Route Neuro's action executions to appropriate games
- Track action IDs to game mappings
- Forward context and results between games and Neuro

#### Key Data Structures:

```go
type IntegrationClient struct {
    neuroClient    *neuro.Client                // SDK client to Neuro
    backend        *nbackend.EmulationBackend   // Emulated backend
    actionToGame   map[string]string            // "game-a/buy" → "game-a"
    actionIDToGame map[string]string            // "abc123" → "game-a"
}

type RelayActionHandler struct {
    name        string              // "game-a/buy_book"
    description string
    schema      map[string]interface{}
    gameID      string              // "game-a"
    client      *IntegrationClient
}
```

#### Action Registration Flow:

```
Game A registers: "buy_book"
        ↓
Emulated Backend:
  - Store in session.Actions["buy_book"]
  - Generate prefixed name: "game-a/buy_book"
  - Call OnActionRegistered("game-a", "game-a/buy_book", action)
        ↓
Integration Client:
  - Track: actionToGame["game-a/buy_book"] = "game-a"
  - Create RelayActionHandler
  - Register with Neuro SDK
        ↓
Neuro receives: Action "game-a/buy_book" from "Game Hub"
```

#### Action Execution Flow:

```
Neuro executes: "game-a/buy_book"
        ↓
Integration Client (RelayActionHandler.Execute):
  - Generate unique actionID: "game-a_buy_book_12345"
  - Track: actionIDToGame["game-a_buy_book_12345"] = "game-a"
  - Call backend.SendAction("game-a", actionID, "game-a--buy_book", data)
        ↓
Emulated Backend.SendAction:
  - Find session for "game-a"
  - Strip prefix: "game-a--buy_book" → "buy_book"
  - Send to game: {"command": "action", "data": {"id": actionID, "name": "buy_book", ...}}
        ↓
Game A receives: action "buy_book"
Game A validates and executes
        ↓
Game A sends result: {"command": "action/result", "data": {"id": actionID, "success": true}}
        ↓
Emulated Backend.handleActionResult:
  - Call OnActionResult("game-a", actionID, true, "Success")
        ↓
Integration Client:
  - Forward to Neuro: SendActionResult(actionID, true, "Success")
  - Clean up: delete actionIDToGame[actionID]
        ↓
Neuro receives result
```

### 3. WebSocket Server (`src/utils/wsServer.go`)

Reusable WebSocket server library with client management.

#### Features:
- Connection upgrading
- Client registration/unregistration
- Message broadcasting
- Automatic ping/pong keep-alive
- Graceful handling of slow clients
- Concurrent message handling

#### Usage:

```go
server := utilities.New(func(c *Client, msgType int, data []byte) {
    // Handle message from client
})
server.Attach(mux, "/")
```

## Game ID Generation

Game names are normalized to create safe, unique identifiers:

```
Input: "Game A"          → Output: "game-a"
Input: "Buckshot Roulette" → Output: "buckshot-roulette"
Input: "My Game!"        → Output: "my-game"
Input: "Test  ---  Game" → Output: "test-game"
```

Algorithm:
1. Convert to lowercase
2. Replace spaces with hyphens
3. Remove non-alphanumeric (except hyphens)
4. Collapse multiple hyphens
5. Trim leading/trailing hyphens

## Compatibility System

### Compatible Mode (Multiplexing Enabled)

When a game includes `"nrelay-compatible": "1.0.0"`:

```
┌─────────┐ ┌─────────┐ ┌─────────┐
│ Game A  │ │ Game B  │ │ Game C  │
│ (compat)│ │ (compat)│ │ (compat)│
└────┬────┘ └────┬────┘ └────┬────┘
     │           │           │
     └───────────┼───────────┘
                 │
         ┌───────▼────────┐
         │  NeuroRelay    │
         │  (unlocked)    │
         └───────┬────────┘
                 │
         ┌───────▼────────┐
         │  Neuro-sama    │
         └────────────────┘

All games can connect and share Neuro
Actions: game-a/action1, game-b/action2, game-c/action3
```

### Locked Mode (Backward Compatibility)

When a game doesn't include the compatibility field:

```
┌─────────┐ ┌─────────┐ ┌─────────┐
│ Game A  │ │ Game B  │ │ Game C  │
│(no comp)│ │ (compat)│ │ (compat)│
└────┬────┘ └────┬────┘ └────┬────┘
     │           │           │
     │           │           │
     │           X           X  ← Rejected: "nrelay/locked"
     │
     │
┌────▼─────────┐
│ NeuroRelay   │
│ (LOCKED)     │
└────┬─────────┘
     │
┌────▼─────────┐
│ Neuro-sama   │
└──────────────┘

Only non-compatible game connects
Backend locked until it disconnects
```

## Callback Architecture

The emulated backend uses callbacks to communicate with the integration client:

```go
// Emulated Backend → Integration Client
OnStartup(gameID, gameName)
OnActionRegistered(gameID, actionName, action)
OnActionUnregistered(gameID, actionName)
OnContext(gameID, message, silent)
OnActionResult(gameID, actionID, success, message)
OnActionForce(gameID, state, query, ephemeral, priority, actionNames)

// Integration Client → Emulated Backend
backend.SendAction(gameID, actionID, actionName, data)
backend.GetAllSessions()
backend.IsLocked()
```

## Concurrency & Thread Safety

### Mutex Protection:

```go
// Emulated Backend
sessionsMu sync.RWMutex     // Protects sessions map
lockMu     sync.RWMutex     // Protects lock state

// Integration Client
actionMu   sync.RWMutex     // Protects actionToGame map
actionIDMu sync.RWMutex     // Protects actionIDToGame map

// WebSocket Server
mu         sync.RWMutex     // Protects clients map
```

### Goroutine Management:

```go
// Message handlers run in separate goroutines
go messageHandler(client, msgType, data)

// Each WebSocket client has dedicated read/write pumps
go client.readPump()
go client.writePump()

// Integration client error handling
go func() {
    for err := range client.Errors() {
        log.Printf("Error: %v", err)
    }
}()
```

## Error Handling

### Connection Errors:
- WebSocket upgrade failures → Log and continue
- Unexpected disconnections → Clean up session, unlock if needed
- Read/write errors → Close connection, unregister client

### Protocol Errors:
- Invalid JSON → Log, ignore message
- Unknown command → Log, continue
- Missing required fields → Return error to client

### Application Errors:
- Action to non-existent game → Log error, fail action
- Duplicate action registration → Ignore (idempotent)
- Backend locked → Send `nrelay/locked` error

## State Management

### Session Lifecycle:

```
1. Connection Established
   ↓
2. Startup Message Received
   ↓
3. Session Created (or Lock Check)
   ↓
4. Actions Registered
   ↓
5. Message Exchange (runtime)
   ↓
6. Connection Closed
   ↓
7. Session Cleaned Up, Lock Released
```

### Action State Tracking:

```
Registration:
  game.Actions[name] = definition
  actionToGame[prefixedName] = gameID

Execution:
  actionIDToGame[uniqueID] = gameID
  
Result:
  delete(actionIDToGame[uniqueID])
  
Unregistration:
  delete(game.Actions[name])
  delete(actionToGame[prefixedName])
```

## Performance Considerations

### Message Buffering:
- Client send channel: 256 message buffer
- Broadcast channel: Unbounded (but processed quickly)
- Non-blocking sends to slow clients → auto-disconnect

### Connection Limits:
- No hard limit on concurrent games
- Memory usage: ~1KB per game session
- CPU usage: Minimal (event-driven architecture)

### Scaling:
- Single process handles dozens of games
- For 100+ games, consider:
  - Load balancing multiple NeuroRelay instances
  - Sharding games across relays
  - Using Redis for shared state

## Security Considerations

### Input Validation:
- JSON parsing with error handling
- Game ID normalization prevents injection
- Action name validation (alphanumeric + dash/underscore)

### Resource Protection:
- WebSocket read limit: 512KB
- Connection timeout: 60 seconds
- Slow client auto-disconnect

### Isolation:
- Each game session is isolated
- Actions can only target their own game
- No cross-game message leakage

## Future Enhancements

### Potential Features:
1. **Persistent Sessions**: Resume on reconnect
2. **Action Quotas**: Rate limiting per game
3. **Priority Queue**: VIP game actions
4. **Analytics**: Action usage statistics
5. **Web Dashboard**: Monitor connected games
6. **Authentication**: Token-based game auth
7. **Load Balancing**: Multiple Neuro backends

### API Extensions:
1. **Game-to-Game Messages**: Inter-game communication
2. **Shared State**: Cross-game data sharing
3. **Event Broadcasting**: Global game events
4. **Plugin System**: Custom message handlers

## Debugging

### Enable Verbose Logging:

```go
// In entrypoint.go
log.SetFlags(log.LstdFlags | log.Lshortfile)

// In messageHandler
log.Printf("[%s] Received: %s", session.GameID, msg.Command)
log.Printf("[%s] Forwarding: %s", session.GameID, prefixedName)
```

### Trace Action Flow:

```bash
# Follow an action through the system
grep "buy_book" logs.txt

# Output:
# Registered action: buy_book -> game-a/buy_book
# Registering action with Neuro: game-a/buy_book
# Executing relayed action: game-a/buy_book (id: abc123)
# Forwarding action result to Neuro: id=abc123, success=true
```

### Monitor Connections:

```bash
# Check active sessions
curl http://localhost:8001/api/sessions  # Future feature

# Current method: Check logs for registration count
grep "client registered" logs.txt
```

## Conclusion

NeuroRelay provides a robust, scalable multiplexing layer for Neuro integrations. Its architecture balances simplicity, performance, and backward compatibility, making it suitable for both small hobby projects and larger deployment scenarios.
