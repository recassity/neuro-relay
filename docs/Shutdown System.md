# NeuroRelay Shutdown System

NeuroRelay implements a comprehensive shutdown system that handles both individual game shutdowns and NeuroRelay itself shutting down.

## Overview

The shutdown system works on two levels:

1. **Individual Game Shutdown**: Neuro can shut down specific games via the `shutdown_game` action
2. **NeuroRelay Shutdown**: Neuro can shut down NeuroRelay itself via the `shutdown/graceful` command

## How It Works

### 1. Shutting Down Individual Games

NeuroRelay automatically registers a special `shutdown_game` action with Neuro that allows her to gracefully shut down specific connected games.

#### Action Schema

```json
{
  "name": "shutdown_game",
  "description": "Request a game to shut down gracefully. The game will save progress and quit to main menu.",
  "schema": {
    "type": "object",
    "properties": {
      "game_id": {
        "type": "string",
        "description": "ID of the game to shutdown",
        "enum": ["game-a", "game-b", "game-c"]  // Dynamically updated as games connect/disconnect
      }
    },
    "required": ["game_id"]
  }
}
```

#### Flow Diagram

```
Neuro executes "shutdown_game" with game_id="game-a"
        ↓
IntegrationClient receives action
        ↓
Calls backend.SendShutdown("game-a", true)
        ↓
EmulationBackend sends shutdown/graceful to Game A
        ↓
Game A receives: {"command": "shutdown/graceful", "data": {"wants_shutdown": true}}
        ↓
Game A saves progress and quits to main menu
        ↓
Game A sends: {"command": "shutdown/ready", "game": "Game A"}
        ↓
NeuroRelay logs shutdown completion
```

### 2. Shutting Down NeuroRelay

When Neuro wants to shut down NeuroRelay itself (e.g., when switching to a different integration), she sends the standard `shutdown/graceful` command directly to NeuroRelay.

#### Flow Diagram

```
Neuro sends shutdown/graceful to NeuroRelay
        ↓
IntegrationClient receives command
        ↓
Sends shutdown/ready acknowledgment
        ↓
Neuro terminates NeuroRelay process
```

## Implementation Details

### EmulationBackend

**New Methods:**

```go
// SendShutdown sends a graceful shutdown command to a specific game
func (eb *EmulationBackend) SendShutdown(gameID string, wantsShutdown bool) error
```

**New Callback:**

```go
OnShutdownReady func(gameID string)  // Called when a game sends shutdown/ready
```

**New Message Handler:**

```go
func (eb *EmulationBackend) handleShutdownReady(c *utilities.Client, msg ClientMessage)
```

### IntegrationClient

**New Methods:**

```go
// registerShutdownAction registers/updates the shutdown_game action
func (ic *IntegrationClient) registerShutdownAction()

// handleShutdownGameAction handles the shutdown_game action from Neuro
func (ic *IntegrationClient) handleShutdownGameAction(actionID string, actionData string)

// handleGracefulShutdown handles shutdown/graceful from Neuro (to shutdown NeuroRelay)
func (ic *IntegrationClient) handleGracefulShutdown(msg map[string]interface{})
```

**Modified Callbacks:**

```go
ic.backend.OnStartup = func(gameID string, gameName string) {
    // ... existing code ...
    ic.registerShutdownAction()  // Update shutdown action with new game list
}

ic.backend.OnShutdownReady = func(gameID string) {
    log.Printf("Game %s is ready to shutdown", gameID)
    ic.sendContextToNeuro("Game '"+gameID+"' has shut down gracefully", true)
}
```

## Dynamic Game List

The `shutdown_game` action's enum is **dynamically updated** whenever:
- A new game connects (registered in `OnStartup`)
- A game disconnects (enum updated to remove that game)

This ensures Neuro only sees currently connected games when choosing which one to shut down.

```go
// When Game A connects:
enum: ["game-a"]

// When Game B connects:
enum: ["game-a", "game-b"]

// When Game A disconnects:
enum: ["game-b"]

// When all games disconnect:
// shutdown_game action is unregistered entirely
```

## Example Usage

### Game Shutdown Example

```
User: "Neuro, shut down Game A"
Neuro executes: shutdown_game(game_id="game-a")
NeuroRelay forwards shutdown command to Game A
Game A saves and quits to main menu
Game A sends shutdown/ready
NeuroRelay logs: "Game game-a is ready to shutdown"
```

### NeuroRelay Shutdown Example

```
User: "Neuro, shut down the game hub"
Neuro sends: shutdown/graceful(wants_shutdown=true)
NeuroRelay sends: shutdown/ready
Neuro terminates NeuroRelay process
```

## Backward Compatibility

Games do NOT need to implement shutdown handling to work with NeuroRelay. The shutdown feature is **optional**:

- **If a game implements shutdown/graceful**: It will save progress and quit gracefully
- **If a game doesn't implement shutdown/graceful**: The message is safely ignored

This ensures full backward compatibility with existing integrations.

## Implementation in Games

To support graceful shutdown in your game, handle the `shutdown/graceful` command:

```go
switch msg.Command {
case "shutdown/graceful":
    data := msg.Data["wants_shutdown"].(bool)
    
    if data {
        // Save game progress
        saveProgress()
        
        // Quit to main menu (don't exit the process)
        quitToMainMenu()
        
        // Send acknowledgment
        client.SendShutdownReady()
    }
}
```

**Important**: Games should **NOT** terminate their own process. They should:
1. Save any progress
2. Quit to main menu (or equivalent safe state)
3. Send `shutdown/ready`
4. Let Neuro terminate the process if needed

## Cancelling Shutdowns

The `wants_shutdown` parameter supports cancellation:

```go
// Request shutdown
{"command": "shutdown/graceful", "data": {"wants_shutdown": true}}

// Cancel shutdown (if not yet executed)
{"command": "shutdown/graceful", "data": {"wants_shutdown": false}}
```

Games should track the latest `wants_shutdown` value and only shutdown when it's `true` at a graceful shutdown point.

## Logging

NeuroRelay provides comprehensive logging for shutdown operations:

```
Registering shutdown_game action with games: [game-a game-b]
Handling action: shutdown_game (ID: abc123)
Requesting graceful shutdown for game: game-a
Sending shutdown command to game-a (wants_shutdown: true)
Game game-a is ready to shutdown

⚠️ NeuroRelay graceful shutdown requested by Neuro
Sending shutdown ready acknowledgment...
✅ Shutdown ready sent. NeuroRelay will be terminated by Neuro.
```

## Benefits

### For Multiplexing

- **Selective shutdown**: Shut down individual games without affecting others
- **Clean state**: Games can save progress before shutting down
- **No interference**: Other games continue running normally

### For NeuroRelay

- **Graceful termination**: NeuroRelay can acknowledge shutdown before being terminated
- **Proper cleanup**: Opportunity to clean up resources if needed
- **Standard protocol**: Uses official Neuro API shutdown spec

## API Compatibility

This implementation follows the **official Neuro API v2 shutdown specification** (from PROPOSALS.md):

- `shutdown/graceful` with `wants_shutdown` parameter
- `shutdown/ready` response
- Games quit to main menu (don't self-terminate)
- Process termination handled by Neuro

The only extension is the `shutdown_game` action, which is NeuroRelay-specific and allows multiplexed shutdown of individual games.

## Future Enhancements

Potential additions:

1. **Immediate shutdown**: Support `shutdown/immediate` for emergency shutdowns
2. **Timeout handling**: Auto-shutdown if game doesn't respond to graceful shutdown
3. **Shutdown cascades**: Optionally shutdown all games when NeuroRelay shuts down
4. **Shutdown hooks**: Allow games to register cleanup callbacks
5. **Shutdown status**: Track which games are in the process of shutting down

## Testing

To test shutdown handling:

```bash
# Terminal 1: Start NeuroRelay
./neurorelay

# Terminal 2: Start a game
cd game-a && go run main.go

# Terminal 3: Use Randy to send shutdown commands
# Shutdown individual game:
curl -X POST http://localhost:1337/ \
  -H 'Content-Type: application/json' \
  -d '{"command": "action", "data": {"id": "test1", "name": "shutdown_game", "data": "{\"game_id\":\"game-a\"}"}}'

# Shutdown NeuroRelay:
curl -X POST http://localhost:1337/ \
  -H 'Content-Type: application/json' \
  -d '{"command": "shutdown/graceful", "data": {"wants_shutdown": true}}'
```

## Summary

The shutdown system provides:
- ✅ Graceful shutdown of individual games via `shutdown_game` action
- ✅ Graceful shutdown of NeuroRelay via `shutdown/graceful` command
- ✅ Dynamic game list in action enum
- ✅ Full backward compatibility
- ✅ Follows official Neuro API v2 spec
- ✅ Comprehensive logging
- ✅ Safe multiplexing (shutting down one game doesn't affect others)