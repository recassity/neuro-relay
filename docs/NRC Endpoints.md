# NeuroRelay Custom (NRC) Endpoints

NeuroRelay provides custom endpoints for enhanced integration capabilities. These endpoints use the `nrc-endpoints/` command prefix and are only available to NeuroRelay-compatible integrations.

## Overview

NRC endpoints allow integrations to:
- Declare NeuroRelay compatibility without interfering with Neuro backend
- Query health and status information
- Enable/disable features based on version compatibility
- Receive real-time system information

## Endpoint Prefix

All NRC endpoints use the command format: `nrc-endpoints/{endpoint-name}`

## Available Endpoints

### 1. `nrc-endpoints/startup`

Declares NeuroRelay compatibility and version. This should be sent **after** the standard `startup` command.

#### Request Format

```json
{
  "command": "nrc-endpoints/startup",
  "game": "My Game",
  "data": {
    "nr-version": "1.0.0"
  }
}
```

#### Parameters
- `nr-version` (required): The NeuroRelay version your integration supports

#### Response: `nrc-endpoints/startup-ack`

```json
{
  "command": "nrc-endpoints/startup-ack",
  "data": {
    "nr-version": "1.0.0",
    "features": {
      "health-endpoint": true,
      "multiplexing": true,
      "custom-routing": true
    }
  }
}
```

#### Error Response: `nrc-endpoints/version-mismatch`

```json
{
  "command": "nrc-endpoints/version-mismatch",
  "data": {
    "requested": "2.0.0",
    "available": ["1.0.0"],
    "suggestion": "1.0.0"
  }
}
```

### 2. `nrc-endpoints/health`

Queries the health and status of NeuroRelay.

#### Request Format

```json
{
  "command": "nrc-endpoints/health",
  "game": "My Game",
  "data": {
    "include": [
      "status",
      "version",
      "connected-games",
      "neuro-backend",
      "uptime",
      "features",
      "lock-status"
    ]
  }
}
```

#### Parameters
- `include` (optional): Array of fields to include in response. If omitted, all fields are included.

Available fields:
- `status`: Overall health status
- `version`: NeuroRelay and game NR versions
- `connected-games`: List of all connected games
- `neuro-backend`: Neuro backend connection status
- `uptime`: System uptime information
- `features`: Enabled features for this integration
- `lock-status`: Backend lock status

#### Response: `nrc-endpoints/health-response`

```json
{
  "command": "nrc-endpoints/health-response",
  "data": {
    "status": "healthy",
    "nr-version": "1.0.0",
    "game-nr-version": "1.0.0",
    "connected-games": [
      {"id": "game-a", "name": "Game A"},
      {"id": "game-b", "name": "Game B"}
    ],
    "total-games": 2,
    "neuro-backend-connected": true,
    "uptime-seconds": 3600,
    "features": {
      "health-endpoint": true,
      "multiplexing": true,
      "custom-routing": true
    },
    "backend-locked": false
  }
}
```

### 3. Error Response: `nrc-endpoints/error`

Generic error response for NRC endpoints.

```json
{
  "command": "nrc-endpoints/error",
  "data": {
    "error": "Error message here"
  }
}
```

## Version Compatibility System

NeuroRelay uses semantic versioning and feature flags to ensure backward compatibility.

### Supported Versions

| Version | Features |
|---------|----------|
| 1.0.0   | Health endpoint, Multiplexing, Custom routing |

### Feature Flags

Each version declares which features it supports:

```go
type VersionFeatures struct {
    SupportsHealthEndpoint bool  // Can use health endpoint
    SupportsMultiplexing   bool  // Actions are prefixed with game ID
    SupportsCustomRouting  bool  // Supports custom routing features
}
```

## Integration Flow

### For NeuroRelay-Compatible Integrations

```
1. Connect to NeuroRelay (ws://127.0.0.1:8001)
   ↓
2. Send standard startup command
   {"command": "startup", "game": "My Game"}
   ↓
3. Send NRC startup to declare compatibility
   {"command": "nrc-endpoints/startup", "game": "My Game", "data": {"nr-version": "1.0.0"}}
   ↓
4. Receive startup acknowledgment with enabled features
   {"command": "nrc-endpoints/startup-ack", ...}
   ↓
5. Register actions (they will be prefixed automatically if multiplexing is enabled)
   {"command": "actions/register", ...}
   ↓
6. Optionally query health status
   {"command": "nrc-endpoints/health", ...}
```

### For Non-Compatible Integrations

```
1. Connect to NeuroRelay
   ↓
2. Send standard startup command only
   {"command": "startup", "game": "My Game"}
   ↓
3. Register actions (no prefixing, single-game mode)
   {"command": "actions/register", ...}
```

## Benefits of NRC Compatibility

### 1. **Multiplexing**
- Multiple games can connect simultaneously
- Actions are automatically prefixed with game ID
- No conflicts between games with same action names

### 2. **Health Monitoring**
- Query real-time system status
- See all connected games
- Monitor Neuro backend connection

### 3. **Version Safety**
- Features are enabled based on version support
- Graceful degradation for older versions
- Clear version mismatch messages

### 4. **Future-Proof**
- New features can be added without breaking old integrations
- Modular feature flags
- Backward compatible

## Example: Complete Integration

```go
package main

import (
    "encoding/json"
    "log"
    "github.com/cassitly/neuro-integration-sdk"
    "github.com/gorilla/websocket"
)

func main() {
    // Connect to NeuroRelay
    conn, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:8001", nil)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    // 1. Send standard startup
    startup := map[string]interface{}{
        "command": "startup",
        "game":    "My Awesome Game",
    }
    sendJSON(conn, startup)

    // 2. Declare NR compatibility
    nrcStartup := map[string]interface{}{
        "command": "nrc-endpoints/startup",
        "game":    "My Awesome Game",
        "data": map[string]interface{}{
            "nr-version": "1.0.0",
        },
    }
    sendJSON(conn, nrcStartup)

    // 3. Wait for acknowledgment
    var response map[string]interface{}
    readJSON(conn, &response)
    
    if response["command"] == "nrc-endpoints/startup-ack" {
        log.Println("✅ NeuroRelay compatible mode enabled!")
        
        features := response["data"].(map[string]interface{})["features"]
        log.Printf("Features: %+v", features)
    }

    // 4. Query health (optional)
    healthCheck := map[string]interface{}{
        "command": "nrc-endpoints/health",
        "game":    "My Awesome Game",
        "data": map[string]interface{}{
            "include": []string{"connected-games", "version"},
        },
    }
    sendJSON(conn, healthCheck)

    // Read health response
    readJSON(conn, &response)
    log.Printf("Health: %+v", response)

    // 5. Continue with normal Neuro API usage...
}

func sendJSON(conn *websocket.Conn, v interface{}) {
    data, _ := json.Marshal(v)
    conn.WriteMessage(websocket.TextMessage, data)
}

func readJSON(conn *websocket.Conn, v interface{}) {
    _, data, _ := conn.ReadMessage()
    json.Unmarshal(data, v)
}
```

## Health Check Monitoring

You can use the health endpoint for monitoring:

```javascript
// Periodic health check
setInterval(async () => {
    sendMessage({
        command: "nrc-endpoints/health",
        game: "My Game",
        data: {
            include: ["status", "connected-games", "neuro-backend"]
        }
    });
}, 30000); // Every 30 seconds
```

## Error Handling

Always handle potential errors:

```go
response := readMessage()

switch response.Command {
case "nrc-endpoints/startup-ack":
    // Success
    log.Println("NR compatibility enabled")

case "nrc-endpoints/version-mismatch":
    // Version not supported
    suggested := response.Data["suggestion"]
    log.Printf("Please update to version %s", suggested)

case "nrc-endpoints/error":
    // Generic error
    errorMsg := response.Data["error"]
    log.Printf("Error: %s", errorMsg)
}
```

## Best Practices

1. **Always send `nrc-endpoints/startup` after standard `startup`**
   - This ensures proper session initialization

2. **Check the startup acknowledgment**
   - Verify which features are enabled
   - Adapt your integration accordingly

3. **Use health checks sparingly**
   - Don't poll too frequently (recommended: 30-60 seconds minimum)
   - Only request fields you actually need

4. **Handle version mismatches gracefully**
   - Log the suggested version
   - Consider supporting multiple versions

5. **Test without NRC compatibility**
   - Ensure your integration works in both modes
   - Graceful degradation is important

## Troubleshooting

### "Session not found" error
**Cause**: Sent NRC startup before standard startup  
**Solution**: Always send standard `startup` command first

### "Health endpoint not supported" error
**Cause**: Integration version doesn't support health endpoint  
**Solution**: Update `nr-version` in startup or check version compatibility

### No response to NRC commands
**Cause**: NeuroRelay might not support NRC endpoints  
**Solution**: Check NeuroRelay version, ensure it's 1.0.0 or higher

## Future Endpoints

Planned for future versions:

- `nrc-endpoints/metrics` - Detailed performance metrics
- `nrc-endpoints/config` - Runtime configuration
- `nrc-endpoints/broadcast` - Send messages to other games
- `nrc-endpoints/priority` - Adjust action priority/routing

## Version History

### 1.0.0 (Current)
- Initial NRC endpoint system
- Health endpoint
- Startup compatibility declaration
- Version-based feature flags
