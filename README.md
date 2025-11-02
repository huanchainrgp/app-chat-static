# Chat Server with WebSocket

A Go server with native WebSocket support for real-time chat functionality.

## Features

### Server Features
- **High-performance Go server** using Gin framework (v1.9.1)
- **Native WebSocket support** using Gorilla WebSocket (v1.5.0) with full duplex communication
- **Hub-based architecture** for efficient message broadcasting to multiple clients
- **Concurrent connection handling** using goroutines (2 per client: ReadPump + WritePump)
- **Automatic keepalive** with ping/pong mechanism (54s ping interval, 60s timeout)
- **Thread-safe operations** using mutex locks for client management
- **CORS support** for cross-origin requests
- **Static file serving** for HTML, CSS, and other assets

### Client Features
- **Beautiful chat client** (`status.html`) with modern gradient UI
- **Real-time messaging** with instant message delivery
- **User join/leave notifications** with automatic system messages
- **Connection status indicator** (visual green/red dot)
- **Online user count** display
- **Message history** with timestamps
- **Responsive design** for desktop and mobile devices
- **HTML escaping** for XSS protection
- **Auto-scroll** to latest messages
- **Enter key support** for quick message sending

### Admin Features
- **Admin monitoring dashboard** (`admin.html`) for real-time server monitoring
- **Active connection tracking** with detailed client information
- **Message statistics** (total messages, events, connections)
- **Server uptime** display
- **Event log** with connection, message, and user activity events
- **Recent messages** display
- **Auto-refresh** every 2 seconds for live updates

## Installation

### Prerequisites

- **Go 1.21 or higher** - Required for building and running the server
- **Make** (optional) - For using Makefile commands
- **Web browser** - For accessing the chat client and admin dashboard
- **Port 3000** (or custom port) - Must be available for the server

### System Requirements

- **CPU**: Any modern processor (supports thousands of concurrent connections)
- **Memory**: Minimal footprint (~10-50MB base, scales with connections)
- **Network**: Internet connection for WebSocket communication
- **Operating System**: Linux, macOS, Windows (cross-platform Go binary)

### Installing Dependencies

**Using Makefile (recommended):**
```bash
make install
```

**Or manually:**
```bash
go mod download
go mod tidy
```

This will install the following dependencies:
- `github.com/gin-gonic/gin` (v1.9.1) - HTTP web framework
- `github.com/gin-contrib/cors` (v1.5.0) - CORS middleware
- `github.com/gorilla/websocket` (v1.5.0) - WebSocket implementation

### Verifying Installation

```bash
# Check Go version
go version

# Verify dependencies
go mod verify

# List dependencies
go list -m all
```

## Usage

### Start the server

**Using Makefile (recommended):**

```bash
make start          # Build and start the server
make run            # Same as make start
make dev            # Run in development mode (auto-reload if air is installed)
```

**Or manually:**

```bash
go run main.go
```

**Or build and run:**

```bash
make build          # Build the binary
./server            # Run the binary
```

The server will start on `http://localhost:3000` (or the port specified by the `PORT` environment variable)

### Makefile Commands

- `make help` - Show all available commands
- `make install` - Install Go dependencies
- `make build` - Build the server binary
- `make start` or `make run` - Build and start the server
- `make dev` - Run in development mode (with auto-reload)
- `make clean` - Clean build artifacts
- `make test` - Run tests

### Access the Status Page

Open your browser and go to:
```
http://localhost:3000
```

## Architecture

### Hub Pattern

The server uses a **Hub pattern** for managing WebSocket connections. This architecture:

- **Centralized message broker**: All clients register with a single Hub
- **Efficient broadcasting**: Messages are sent to all connected clients via channels
- **Thread-safe operations**: Mutex locks ensure safe concurrent access
- **Scalable design**: Can handle thousands of concurrent connections

### Connection Lifecycle

1. **Client Connection**: Client opens WebSocket connection to `/ws`
2. **Upgrade**: Server upgrades HTTP connection to WebSocket
3. **Client Registration**: New client is added to Hub's client map
4. **Goroutine Spawn**: Two goroutines start per client:
   - `ReadPump`: Reads messages from client (handles `join`, `message` types)
   - `WritePump`: Writes messages to client (handles broadcasts, ping messages)
5. **Communication**: Messages flow through channels between Hub and clients
6. **Disconnection**: Client removed from Hub, channels closed, connection closed

### Keepalive Mechanism

The server implements automatic connection keepalive:

- **Ping Messages**: Server sends ping every 54 seconds
- **Pong Response**: Client automatically responds with pong
- **Read Timeout**: 60 seconds (resets on each pong)
- **Write Timeout**: 10 seconds per write operation
- **Dead Connection Detection**: Failed pings result in automatic cleanup

### Thread Safety

All shared data structures use mutex locks:

- **Hub clients map**: Protected by `sync.RWMutex` for concurrent reads/writes
- **Client registration**: Synchronized via channels and mutexes
- **Message broadcasting**: Thread-safe channel operations

### Performance Characteristics

- **Concurrent Connections**: Supports thousands of simultaneous connections
- **Memory Efficiency**: 
  - Buffered channels (256 bytes) prevent blocking
  - Message history limited to 100 messages
  - Event history limited to 500 events
  - Automatic pruning of old data
- **Low Latency**: Direct channel communication, no database overhead
- **CPU Usage**: Minimal per connection (~2 goroutines per client)

## WebSocket API

The server uses native WebSocket with JSON message format. All messages are sent as JSON strings over the WebSocket connection at the `/ws` endpoint.

### Connection Endpoint

- **URL**: `ws://localhost:3000/ws` (or `wss://` for HTTPS)
- **Protocol**: WebSocket (RFC 6455)
- **Message Format**: JSON strings
- **Connection Upgrade**: HTTP GET request upgraded to WebSocket

### Client to Server Messages

- **`join`** - Join the chat with a username
  ```json
  {
    "type": "join",
    "username": "John"
  }
  ```

- **`message`** - Send a chat message
  ```json
  {
    "type": "message",
    "username": "John",
    "text": "Hello everyone!",
    "price": 129999
  }
  ```
  
  **Note**: The `username` field is optional. If omitted, the server uses the username from the client's `join` message. The `price` field is optional and can be any numeric value.

### Server to Client Messages

- **`connect`** - Connection confirmation (sent immediately after WebSocket connection)
  ```json
  {
    "type": "connect",
    "id": "unique-client-id"
  }
  ```

- **`message`** - Receive a chat message
  ```json
  {
    "type": "message",
    "id": "client-id",
    "username": "John",
    "text": "Hello everyone!",
    "timestamp": "2024-01-01T12:00:00Z"
  }
  ```

- **`user-joined`** - Notify when a user joins
  ```json
  {
    "type": "user-joined",
    "username": "John",
    "id": "client-id"
  }
  ```

- **`user-left`** - Notify when a user leaves
  ```json
  {
    "type": "user-left",
    "username": "John"
  }
  ```

### Message Flow Diagram

```
Client                    Server Hub                    Other Clients
  |                          |                               |
  |-- WebSocket Connect --> |                               |
  |                          |-- Register Client -->         |
  |<-- connect (with ID) -- |                               |
  |                          |                               |
  |-- join (username) -----> |                               |
  |                          |-- Broadcast user-joined -->   |
  |                          |                          <-- user-joined
  |                          |                               |
  |-- message (text) ------->|                               |
  |                          |-- Broadcast message --------> |
  |<-- message (echo) ------ |<-- message (broadcast) -------|
  |                          |                               |
```

### JavaScript Example

**Basic Connection:**
```javascript
// Connect to WebSocket
const socket = new WebSocket('ws://localhost:3000/ws');

socket.onopen = () => {
  // Send join message after connection
  socket.send(JSON.stringify({
    type: 'join',
    username: 'John'
  }));
};

socket.onmessage = (event) => {
  const data = JSON.parse(event.data);
  
  switch(data.type) {
    case 'connect':
      console.log('Connected with ID:', data.id);
      break;
    case 'message':
      console.log(`${data.username}: ${data.text}`);
      break;
    case 'user-joined':
      console.log(`${data.username} joined`);
      break;
    case 'user-left':
      console.log(`${data.username} left`);
      break;
  }
};

// Send a message
socket.send(JSON.stringify({
  type: 'message',
  username: 'John',
  text: 'Hello!'
}));
```

**Complete Example with Error Handling:**
```javascript
const socket = new WebSocket('ws://localhost:3000/ws');
let clientId = '';
let reconnectAttempts = 0;
const maxReconnectAttempts = 5;

socket.onopen = () => {
  console.log('WebSocket connected');
  reconnectAttempts = 0;
};

socket.onmessage = (event) => {
  try {
    const data = JSON.parse(event.data);
    
    if (data.type === 'connect') {
      clientId = data.id;
      // Send join message
      socket.send(JSON.stringify({
        type: 'join',
        username: 'John'
      }));
    } else if (data.type === 'message') {
      displayMessage(data);
    }
  } catch (error) {
    console.error('Error parsing message:', error);
  }
};

socket.onerror = (error) => {
  console.error('WebSocket error:', error);
};

socket.onclose = () => {
  console.log('WebSocket disconnected');
  // Attempt reconnection
  if (reconnectAttempts < maxReconnectAttempts) {
    reconnectAttempts++;
    setTimeout(() => {
      // Reconnect logic here
    }, 1000 * reconnectAttempts);
  }
};
```

## Port Configuration

The default port is `3000`. You can change it by setting the `PORT` environment variable:

**Using Makefile:**
```bash
PORT=8080 make start
PORT=8080 make dev
```

**Direct execution:**
```bash
PORT=8080 go run main.go
```

**After building:**
```bash
PORT=8080 ./server
```

**Windows:**
```cmd
set PORT=8080
go run main.go
```

**Note**: Ensure the port is not in use by another application. You can check port availability:
- Linux/macOS: `lsof -i :3000` or `netstat -an | grep 3000`
- Windows: `netstat -an | findstr 3000`

## status.html - Chat Application Client

The `status.html` file is the main chat application client interface. It's a simple, easy-to-understand single-page HTML application that provides a real-time chat experience using native WebSocket connections.

### Features

- **Simple & Easy to Read Code**: Clean, well-commented code in Vietnamese for easy understanding
- **Username Entry Modal**: Users must enter a username before joining the chat (max 20 characters)
- **Real-time Messaging**: Instant message delivery via WebSocket
- **User Count Display**: Displays the number of online users
- **Connection Status**: Visual indicator (green/red dot) showing connection state
- **Message History**: Scrollable chat history with timestamps
- **System Notifications**: Automatic notifications for user joins/leaves
- **Modern UI**: Beautiful gradient design with smooth animations
- **Responsive Design**: Works on desktop and mobile devices

### Code Structure

The code is organized simply and clearly:

1. **Variables**: Basic variables for socket, username, user count, and client ID
2. **DOM Elements**: References to HTML elements
3. **Helper Functions**: 
   - `addSystemMessage()` - Shows system notifications
   - `showMessage()` - Displays chat messages with HTML escaping for security
4. **Main Functions**:
   - `joinChat()` - Handles joining chat, creates WebSocket connection, and sets up all event handlers
   - `sendMessage()` - Sends chat messages to server
5. **Event Listeners**: Simple Enter key handlers for username and message inputs

All code is commented in Vietnamese for easy understanding.

### User Interface Components

1. **Chat Header**
   - Application title with connection status indicator
   - Username display
   - Online user count

2. **Message Area**
   - Displays all chat messages
   - Different styling for own messages (right-aligned, purple gradient) vs others (left-aligned, white)
   - System messages in center (italics, gray)
   - Auto-scrolls to latest messages

3. **Input Area**
   - Text input field (max 500 characters)
   - Send button
   - Enter key support for sending messages

### WebSocket Protocol

The client communicates with the server using JSON messages over WebSocket:

#### Client to Server Messages

- **Join Chat**:
  ```json
  {
    "type": "join",
    "username": "John"
  }
  ```

- **Send Message**:
  ```json
  {
    "type": "message",
    "username": "John",
    "text": "Hello everyone!"
  }
  ```

#### Server to Client Messages

- **Connection Confirmation**:
  ```json
  {
    "type": "connect",
    "id": "client-unique-id"
  }
  ```

- **Message Broadcast**:
  ```json
  {
    "type": "message",
    "id": "client-id",
    "username": "John",
    "text": "Hello everyone!"
  }
  ```

- **User Joined**:
  ```json
  {
    "type": "user-joined",
    "username": "John"
  }
  ```

- **User Left**:
  ```json
  {
    "type": "user-left",
    "username": "John"
  }
  ```

### Connection Flow

1. User enters username and clicks "Vào Chat" (Join Chat)
2. WebSocket connection is established to `/ws` endpoint
3. Server sends `connect` message with unique client ID
4. Client sends `join` message with username
5. Server broadcasts `user-joined` to all clients
6. User can now send and receive messages

### WebSocket URL Detection

The client automatically detects the correct WebSocket protocol:
- Uses `wss://` (secure) if the page is loaded over HTTPS
- Uses `ws://` (insecure) if the page is loaded over HTTP
- Connects to: `{protocol}//{host}/ws`

### Localization

The interface uses Vietnamese language by default:
- "Nhập tên của bạn" = "Enter your name"
- "Vào Chat" = "Join Chat"
- "Nhập tin nhắn..." = "Enter message..."
- "Gửi" = "Send"
- "người online" = "people online"

### Browser Compatibility

- Modern browsers with WebSocket support
- Chrome, Firefox, Safari, Edge (latest versions)
- Mobile browsers (iOS Safari, Chrome Mobile)

### Security Considerations

**Client-Side Protection:**
- Username input is limited to 20 characters
- Message length is limited to 500 characters
- HTML escaping is applied to prevent XSS attacks
- WebSocket connections are subject to CORS policies

**Server-Side Considerations:**
- **Current Settings (Development Mode)**:
  - CORS allows all origins (`*`)
  - WebSocket origin check allows all origins
  - No authentication required
  - No rate limiting
  - No input sanitization on server side

**Production Recommendations:**
1. **Restrict CORS** to specific domains:
   ```go
   AllowOrigins: []string{"https://yourdomain.com"}
   ```
2. **Validate WebSocket origins** against whitelist
3. **Implement authentication** (JWT, session tokens)
4. **Add rate limiting** per client/IP
5. **Sanitize all inputs** on server side
6. **Use HTTPS/WSS** in production
7. **Add request size limits**
8. **Implement connection limits** per IP
9. **Add logging** for security monitoring
10. **Enable TLS/SSL** certificates

### Error Handling

The client includes error handling for:

- **Connection failures**: Automatic reconnection attempts (if implemented)
- **Invalid messages**: JSON parsing errors are caught and logged
- **WebSocket errors**: Error events are logged to console
- **Network issues**: Connection status indicator shows connection state
- **Server unavailability**: User-friendly error messages

The server handles:

- **Connection errors**: Logged and connections cleaned up gracefully
- **Invalid JSON**: Messages are skipped, error logged
- **Channel blocking**: Clients are removed if send channel is blocked
- **Dead connections**: Automatically detected via ping/pong timeout

## Admin Dashboard

The admin dashboard (`admin.html`) provides real-time monitoring of the chat server. Access it at:

```
http://localhost:3000/admin.html
```

### Features

1. **Statistics Dashboard**
   - Active connections count
   - Total messages processed
   - Total events tracked
   - Server uptime (HH:MM:SS format)

2. **Active Connections Panel**
   - List of all connected clients
   - Client ID, username, connection time
   - Messages sent per client
   - Last activity timestamp

3. **Recent Events Log**
   - Connection/disconnection events
   - User join/leave events
   - Message events
   - Typing indicators
   - Color-coded by event type

4. **Recent Messages Panel**
   - Last messages sent in chat
   - Username, timestamp, and message text
   - Scrollable history

5. **Auto-Refresh**
   - Updates every 2 seconds
   - Shows last update time
   - Connection status indicator

**Note**: The admin dashboard requires a monitoring API endpoint (`/api/monitoring`). If this endpoint is not implemented in your server, the dashboard will show connection errors. See `SERVER.md` for implementing the monitoring API.

## Project Structure

```
app-chat-static/
├── main.go            # Go server with WebSocket support
│                      #   - Hub pattern implementation
│                      #   - Client management
│                      #   - ReadPump/WritePump goroutines
│                      #   - Message routing and broadcasting
├── go.mod             # Go module definition and dependencies
├── go.sum             # Dependency checksums
├── Makefile           # Build and run commands
│                      #   - install, build, start, dev, clean, test
├── status.html        # Chat application client (main UI)
│                      #   - WebSocket client implementation
│                      #   - Real-time messaging interface
├── admin.html         # Admin monitoring dashboard
│                      #   - Real-time server statistics
│                      #   - Connection monitoring
├── chat.css           # Stylesheet for status.html
│                      #   - Modern gradient design
│                      #   - Responsive layout
├── package.json       # Node.js dependencies (legacy, if applicable)
├── yarn.lock          # Yarn lockfile (if applicable)
├── node_modules/      # Node.js dependencies (if applicable)
├── README.md          # This documentation file
└── SERVER.md          # Detailed server implementation documentation
```

### File Descriptions

- **`main.go`**: Main server file containing Hub, Client structs, WebSocket handlers, and HTTP routes
- **`status.html`**: Single-page chat application with embedded JavaScript for WebSocket communication
- **`admin.html`**: Admin dashboard for monitoring server activity (requires monitoring API)
- **`chat.css`**: CSS styles for the chat interface, including variables, animations, and responsive design
- **`Makefile`**: Convenient commands for building, running, and managing the server
- **`go.mod`**: Go module file specifying dependencies and Go version requirement
- **`SERVER.md`**: Comprehensive technical documentation of server architecture and implementation details

## Troubleshooting

### Common Issues

1. **Port Already in Use**
   ```
   Error: bind: address already in use
   ```
   **Solution**: Change the PORT environment variable or stop the process using port 3000
   ```bash
   # Find process using port 3000
   lsof -i :3000
   # Kill the process or use a different port
   PORT=8080 make start
   ```

2. **WebSocket Connection Fails**
   - Check that server is running
   - Verify CORS settings allow your origin
   - Check firewall settings
   - Ensure using correct protocol (ws:// for HTTP, wss:// for HTTPS)

3. **Messages Not Broadcasting**
   - Verify client sent `join` message first
   - Check browser console for WebSocket errors
   - Verify message format matches API specification
   - Check server logs for parsing errors

4. **Admin Dashboard Shows No Data**
   - Ensure `/api/monitoring` endpoint is implemented
   - Check browser console for fetch errors
   - Verify server is responding to API requests

5. **Connection Drops Frequently**
   - Check network stability
   - Verify ping/pong mechanism is working
   - Check server logs for timeout errors
   - Ensure firewall isn't blocking WebSocket connections

### Debug Mode

Enable verbose logging:

```go
// In main.go, before router setup
gin.SetMode(gin.DebugMode)
```

Check WebSocket connection:
- Open browser DevTools → Network → WS tab
- Monitor WebSocket frames
- Check for ping/pong messages

### Performance Issues

If experiencing high CPU or memory usage:

1. **Check active connections**: Use admin dashboard
2. **Monitor goroutines**: Add goroutine count logging
3. **Review message history limits**: Adjust in code if needed
4. **Check for memory leaks**: Monitor memory growth over time

## Deployment

### Building for Production

```bash
# Build optimized binary
GOOS=linux GOARCH=amd64 go build -o server main.go

# Or with flags for smaller binary
go build -ldflags="-s -w" -o server main.go
```

### Running in Production

**Systemd Service (Linux):**
```ini
[Unit]
Description=Chat Server
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=/opt/chat-server
ExecStart=/opt/chat-server/server
Environment="PORT=3000"
Restart=always

[Install]
WantedBy=multi-user.target
```

**Docker Deployment:**

Create `Dockerfile`:
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
COPY --from=builder /app/*.css .
EXPOSE 3000
CMD ["./server"]
```

Build and run:
```bash
docker build -t chat-server .
docker run -p 3000:3000 chat-server
```

### Environment Variables

- `PORT`: Server port (default: 3000)
- `GIN_MODE`: Gin framework mode (`debug`, `release`, `test`)

### Reverse Proxy Setup (nginx)

```nginx
server {
    listen 80;
    server_name chat.example.com;

    location / {
        proxy_pass http://localhost:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

## Development

### Running Tests

```bash
make test
# or
go test -v ./...
```

### Development Mode

```bash
make dev
```

This will:
- Use `air` for auto-reload if installed
- Fall back to `go run` if air is not available

**Installing air for auto-reload:**
```bash
go install github.com/cosmtrek/air@latest
```

### Code Structure

- **Hub Pattern**: Central message broker in `main.go`
- **Client Management**: Each client runs in 2 goroutines
- **Message Routing**: Type-based message routing in `ReadPump()`
- **Broadcasting**: Efficient channel-based message distribution

### Adding Features

To add new message types:

1. Add case in `ReadPump()` switch statement
2. Update client-side JavaScript in `status.html`
3. Document in README WebSocket API section
4. Add corresponding server-to-client message type if needed

## Additional Resources

- **Server Documentation**: See `SERVER.md` for detailed technical documentation
- **Gin Framework**: https://github.com/gin-gonic/gin
- **Gorilla WebSocket**: https://github.com/gorilla/websocket
- **WebSocket RFC**: https://tools.ietf.org/html/rfc6455

## License

This project is provided as-is for educational and development purposes.

---

**Need Help?** Check the troubleshooting section or review the detailed documentation in `SERVER.md`.

