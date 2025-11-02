# Chat Server with WebSocket

A Go server with native WebSocket support for real-time chat functionality.

## Features

- Go web server (using Gin framework)
- Native WebSocket support (using Gorilla WebSocket)
- Beautiful chat client (`status.html`) with real-time messaging
- Admin monitoring dashboard (`admin.html`)
- Support for multiple concurrent clients
- User join/leave notifications
- Connection status monitoring

## Installation

### Using Go

```bash
make install
# or
go mod download
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

## WebSocket API

The server uses native WebSocket with JSON message format. All messages are sent as JSON strings over the WebSocket connection at the `/ws` endpoint.

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

### JavaScript Example

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
  }
};

// Send a message
socket.send(JSON.stringify({
  type: 'message',
  username: 'John',
  text: 'Hello!'
}));
```

## Port Configuration

The default port is `3000`. You can change it by setting the `PORT` environment variable:

```bash
PORT=8080 make start
# or
PORT=8080 go run main.go
```

## status.html - Chat Application Client

The `status.html` file is the main chat application client interface. It's a single-page HTML application that provides a modern, real-time chat experience using native WebSocket connections.

### Features

- **Username Entry Modal**: Users must enter a username before joining the chat (max 20 characters)
- **Real-time Messaging**: Instant message delivery via WebSocket
- **User Count Display**: Displays the number of online users
- **Connection Status**: Visual indicator (green/red dot) showing connection state
- **Message History**: Scrollable chat history with timestamps
- **System Notifications**: Automatic notifications for user joins/leaves
- **Modern UI**: Beautiful gradient design with smooth animations
- **Responsive Design**: Works on desktop and mobile devices

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

- Username input is limited to 20 characters
- Message length is limited to 500 characters
- HTML escaping is applied to prevent XSS attacks
- WebSocket connections are subject to CORS policies

## Project Structure

```
app-chat-static/
├── main.go            # Go server with WebSocket support
├── go.mod             # Go dependencies
├── Makefile           # Build and run commands
├── status.html        # Chat application client (main UI)
├── admin.html         # Admin monitoring dashboard
├── server.js          # Original Node.js server (legacy)
├── package.json       # Node.js dependencies (legacy)
└── README.md         # This file
```

