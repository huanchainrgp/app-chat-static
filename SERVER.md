# Go WebSocket Chat Server Documentation

This document explains the Go server implementation in `main.go` - a high-performance WebSocket chat server built with Gin and Gorilla WebSocket.

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [Key Components](#key-components)
- [WebSocket Protocol](#websocket-protocol)
- [API Endpoints](#api-endpoints)
- [Message Handling](#message-handling)
- [Connection Management](#connection-management)
- [Monitoring System](#monitoring-system)
- [Configuration](#configuration)
- [Dependencies](#dependencies)
- [Code Structure](#code-structure)

## Architecture Overview

The server uses a **Hub pattern** to manage multiple WebSocket connections concurrently. This pattern is ideal for real-time chat applications as it:

- Manages all connected clients in a centralized hub
- Broadcasts messages efficiently to multiple clients
- Handles client registration and unregistration
- Ensures thread-safe operations with mutex locks

### High-Level Flow

1. Client connects via WebSocket at `/ws` endpoint
2. Server upgrades HTTP connection to WebSocket
3. Client is registered in the Hub
4. Two goroutines are spawned per client:
   - `ReadPump`: Reads incoming messages from client
   - `WritePump`: Writes outgoing messages to client
5. Messages are broadcast to all or selected clients via the Hub
6. Connection monitoring data is tracked in real-time

## Key Components

### Hub

The `Hub` struct is the central message broker that manages all client connections:

```go
type Hub struct {
    clients    map[*Client]bool  // Active client connections
    broadcast  chan []byte        // Channel for broadcasting messages
    register   chan *Client       // Channel for registering new clients
    unregister chan *Client       // Channel for unregistering clients
    mu         sync.RWMutex      // Mutex for thread-safe operations
}
```

**Responsibilities:**
- Register/unregister clients
- Broadcast messages to all connected clients
- Thread-safe client management
- Runs in its own goroutine via `Run()` method

### Client

Represents a single WebSocket connection:

```go
type Client struct {
    ID       string              // Unique client identifier
    Username string              // Client's username (set after join)
    Conn     *websocket.Conn      // WebSocket connection
    Hub      *Hub                // Reference to the hub
    Send     chan []byte         // Buffered channel for outgoing messages
    mu       sync.Mutex          // Mutex for thread-safe operations
}
```

**Key Methods:**

- **`ReadPump()`**: Continuously reads messages from the WebSocket connection
  - Parses JSON messages
  - Routes messages by type (`join`, `message`, `typing`)
  - Handles connection errors and cleanup
  - Implements ping/pong keepalive (60 second timeout)

- **`WritePump()`**: Continuously writes messages to the WebSocket connection
  - Sends messages from the `Send` channel
  - Batches multiple queued messages
  - Sends ping messages every 54 seconds
  - Handles write deadlines (10 seconds)

- **`broadcastToAll()`**: Broadcasts message to all clients including sender
- **`broadcastToOthers()`**: Broadcasts message to all clients except sender

### MonitoringData

Tracks server statistics and connection information:

```go
type MonitoringData struct {
    StartTime     time.Time              // Server start time
    Connections   map[string]*Connection // Active connections
    TotalMessages int64                  // Total messages processed
    TotalEvents   int64                  // Total events tracked
    Messages      []Message              // Recent message history (max 100)
    Events        []Event                // Recent event history (max 500)
    mu            sync.RWMutex           // Mutex for thread-safe operations
}
```

**Features:**
- Connection tracking with timestamps
- Message history (last 100 messages)
- Event log (last 500 events)
- Thread-safe operations
- Real-time statistics via REST API

## WebSocket Protocol

### Connection Establishment

1. Client opens WebSocket connection to `/ws`
2. Server upgrades HTTP connection
3. Server generates unique client ID (nanosecond timestamp)
4. Server sends connection confirmation:
   ```json
   {
     "type": "connect",
     "id": "unique-client-id"
   }
   ```
5. Client sends `join` message with username
6. Server broadcasts `user-joined` to all other clients

### Message Types

#### Client → Server

**Join Chat:**
```json
{
  "type": "join",
  "username": "John"
}
```

**Send Message:**
```json
{
  "type": "message",
  "username": "John",
  "text": "Hello everyone!",
  "price": 129999  // Optional field
}
```

**Typing Indicator:**
```json
{
  "type": "typing",
  "username": "John",
  "isTyping": true
}
```

#### Server → Client

**Connection Confirmation:**
```json
{
  "type": "connect",
  "id": "client-id"
}
```

**Message Broadcast:**
```json
{
  "type": "message",
  "id": "client-id",
  "username": "John",
  "text": "Hello everyone!",
  "timestamp": "2024-01-01T12:00:00Z",
  "price": 129999  // If included in original message
}
```

**User Joined:**
```json
{
  "type": "user-joined",
  "username": "John",
  "id": "client-id"
}
```

**User Left:**
```json
{
  "type": "user-left",
  "username": "John"
}
```

**Typing Indicator:**
```json
{
  "type": "typing",
  "username": "John",
  "isTyping": true
}
```

## API Endpoints

### HTTP Routes

| Method | Route | Description |
|--------|-------|-------------|
| `GET` | `/` | Serves `status.html` (main chat client) |
| `GET` | `/admin` | Serves `admin.html` (monitoring dashboard) |
| `GET` | `/api/status` | Returns server status and statistics |
| `GET` | `/api/monitoring` | Returns detailed monitoring information |
| `GET` | `/ws` | WebSocket endpoint for real-time chat |
| `GET` | `/*` | Serves static files if they exist |

### Status API Response

```json
{
  "status": "running",
  "uptime": 1234567,
  "startTime": "2024-01-01T00:00:00Z",
  "currentTime": "2024-01-01T00:20:00Z",
  "activeConnections": 5,
  "totalMessages": 150,
  "totalEvents": 200
}
```

### Monitoring API Response

```json
{
  "uptime": 1234567,
  "connections": [
    {
      "id": "client-id",
      "connectedAt": "2024-01-01T00:00:00Z",
      "username": "John",
      "messagesSent": 10,
      "lastActivity": "2024-01-01T00:19:00Z"
    }
  ],
  "totalConnections": 5,
  "totalMessages": 150,
  "totalEvents": 200,
  "recentMessages": [...],
  "recentEvents": [...]
}
```

## Message Handling

### Join Message Flow

1. Client sends `join` message with username
2. Server updates client's username
3. Server updates monitoring data
4. Server broadcasts `user-joined` to **all other clients** (not sender)

### Message Flow

1. Client sends `message` with text content
2. Server validates and extracts message data
3. Server updates monitoring data (increments message count)
4. Server broadcasts message to **all clients** (including sender)
5. Message includes timestamp and optional fields (e.g., `price`)

### Typing Indicator Flow

1. Client sends `typing` message with `isTyping: true/false`
2. Server updates monitoring data
3. Server broadcasts to **all other clients** (not sender)
4. Indicator automatically clears after timeout on client side

## Connection Management

### Lifecycle

1. **Connection**: Client connects → WebSocket upgrade → Client registered in Hub
2. **Registration**: Client added to Hub's client map
3. **Communication**: ReadPump and WritePump goroutines handle bidirectional communication
4. **Disconnection**: Client removed from Hub → Channel closed → Connection closed

### Keepalive Mechanism

- **Ping Messages**: Server sends ping every 54 seconds
- **Pong Handler**: Client responds with pong
- **Read Timeout**: 60 seconds (resets on pong)
- **Write Timeout**: 10 seconds for each write operation

### Error Handling

- Unexpected close errors are logged
- Failed message parsing is logged and skipped
- Clients with failed sends are automatically removed
- Dead connections are cleaned up automatically

## Monitoring System

### Tracked Events

1. **Connection Events**
   - New connection established
   - Connection disconnected
   - Timestamp and client ID tracked

2. **User Events**
   - User joined (after sending `join` message)
   - Username updates
   - Last activity timestamps

3. **Message Events**
   - All chat messages
   - Sender information
   - Message timestamps
   - Message counts per user

4. **Typing Events**
   - Typing indicator activations
   - User who is typing

### Data Retention

- **Messages**: Last 100 messages kept in memory
- **Events**: Last 500 events kept in memory
- **Connections**: All active connections tracked
- Older data is automatically pruned when limits are exceeded

### Statistics

- Total messages processed
- Total events tracked
- Active connection count
- Server uptime (milliseconds)
- Per-user message counts

## Configuration

### Port Configuration

Default port: `3000`

Set custom port via environment variable:
```bash
PORT=8080 go run main.go
```

### CORS Configuration

The server is configured with permissive CORS settings:

```go
AllowOrigins:     []string{"*"}          // All origins allowed
AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
AllowHeaders:     []string{"*"}          // All headers allowed
ExposeHeaders:    []string{"*"}
AllowCredentials:  true
```

**Note**: For production, restrict `AllowOrigins` to specific domains.

### WebSocket Configuration

```go
var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        return true // Allow all origins
    },
}
```

**Note**: For production, implement proper origin validation.

## Dependencies

### Core Dependencies

- **github.com/gin-gonic/gin** (v1.9.1): HTTP web framework
- **github.com/gin-contrib/cors** (v1.5.0): CORS middleware
- **github.com/gorilla/websocket** (v1.5.0): WebSocket implementation

### Why These Libraries?

- **Gin**: High-performance HTTP framework with excellent middleware support
- **Gorilla WebSocket**: Robust WebSocket implementation with ping/pong support
- **CORS Middleware**: Enables cross-origin requests for web clients

## Code Structure

### Main Function Flow

```go
func main() {
    1. Get port from environment (default: 3000)
    2. Start Hub goroutine
    3. Initialize Gin router
    4. Configure CORS middleware
    5. Register routes:
       - GET / → status.html
       - GET /admin → admin.html
       - GET /api/status → status API
       - GET /api/monitoring → monitoring API
       - GET /ws → WebSocket handler
       - Fallback: serve static files
    6. Start HTTP server
}
```

### WebSocket Handler Flow

```go
func handleWebSocket(w, r) {
    1. Upgrade HTTP to WebSocket
    2. Generate unique client ID
    3. Create new Client instance
    4. Register client in monitoring
    5. Register client in Hub
    6. Send connection confirmation
    7. Start ReadPump goroutine
    8. Start WritePump goroutine
}
```

### ReadPump Flow

```go
func (c *Client) ReadPump() {
    Loop:
        1. Set read deadline (60 seconds)
        2. Read message from WebSocket
        3. Parse JSON message
        4. Extract message type
        5. Route by type:
           - "join" → Update username, broadcast user-joined
           - "message" → Broadcast message
           - "typing" → Broadcast typing indicator
        6. Handle errors (close connection on failure)
}
```

### WritePump Flow

```go
func (c *Client) WritePump() {
    Loop:
        1. Wait for message from Send channel OR ticker (54s)
        2. If message: write to WebSocket, batch queued messages
        3. If ticker: send ping message
        4. Set write deadline (10 seconds)
        5. Handle errors (close connection on failure)
}
```

## Performance Considerations

### Concurrency

- **Goroutines**: Each client connection uses 2 goroutines (read/write)
- **Hub**: Runs in single goroutine, uses channels for communication
- **Thread Safety**: All shared data structures use mutex locks

### Memory Management

- Message history limited to 100 messages
- Event history limited to 500 events
- Old data automatically pruned
- Buffered channels (256 bytes) prevent blocking

### Scalability

- Hub pattern allows horizontal scaling
- Can handle thousands of concurrent connections
- For very high load, consider:
  - Redis pub/sub for distributed messaging
  - Load balancer for multiple server instances
  - Message queue for persistent storage

## Error Handling

### Connection Errors

- WebSocket upgrade failures are logged
- Unexpected close errors are logged (not treated as fatal)
- Normal closures are handled gracefully

### Message Errors

- Invalid JSON is logged and skipped
- Missing message type is ignored
- Missing fields use default values (empty strings)

### Channel Errors

- Blocked channels close client connections
- Prevents memory leaks from unresponsive clients

## Security Considerations

### Current Implementation

⚠️ **Development Mode Settings** (should be hardened for production):

1. **CORS**: Allows all origins (`*`)
2. **WebSocket Origin**: Allows all origins
3. **No Authentication**: No user authentication required
4. **No Rate Limiting**: No protection against message flooding
5. **No Input Sanitization**: Messages are sent as-is (client-side escaping)

### Production Recommendations

1. **Restrict CORS** to specific domains
2. **Validate WebSocket origins** against whitelist
3. **Implement authentication** (JWT, session tokens)
4. **Add rate limiting** per client/IP
5. **Sanitize all inputs** on server side
6. **Use HTTPS/WSS** in production
7. **Add request size limits**
8. **Implement connection limits** per IP

## Testing

### Manual Testing

1. Start server: `go run main.go`
2. Open multiple browser tabs to `http://localhost:3000`
3. Test message sending/receiving
4. Test typing indicators
5. Test user join/leave notifications
6. Check monitoring dashboard at `/admin`
7. Test WebSocket connection/disconnection

### Integration Testing

- Use WebSocket testing tools (e.g., `wscat`)
- Test with multiple concurrent connections
- Verify message broadcasting
- Test error scenarios (invalid JSON, connection drops)

## Deployment

### Building

```bash
# Build binary
go build -o server main.go

# Or use Makefile
make build
```

### Running

```bash
# Run binary
./server

# Or use Makefile
make start
```

### Environment Variables

```bash
PORT=3000 ./server
```

### Docker Deployment (Example)

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o server main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/server .
COPY --from=builder /app/*.html .
EXPOSE 3000
CMD ["./server"]
```

## Troubleshooting

### Common Issues

1. **Port already in use**: Change PORT environment variable
2. **WebSocket connection fails**: Check firewall, verify CORS settings
3. **Messages not broadcasting**: Check Hub registration, verify message format
4. **Memory growth**: Normal for active connections, check message/event limits
5. **Connection drops**: Check network stability, verify ping/pong mechanism

### Debugging

- Enable Gin debug mode: `gin.SetMode(gin.DebugMode)`
- Add logging in ReadPump/WritePump
- Monitor `/api/monitoring` endpoint
- Check server logs for connection errors

## Future Enhancements

Potential improvements:

- [ ] Redis pub/sub for distributed messaging
- [ ] Message persistence (database)
- [ ] User authentication and authorization
- [ ] Private/direct messaging
- [ ] Message encryption
- [ ] File/image sharing
- [ ] Message reactions/emojis
- [ ] User presence status
- [ ] Message search
- [ ] Rate limiting middleware
- [ ] Metrics export (Prometheus)

