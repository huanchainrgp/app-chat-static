# Chat Server with Socket.IO

A Go server with Socket.IO for real-time chat functionality.

## Features

- Go web server (using Gin framework)
- Socket.IO WebSocket support
- Beautiful status page with real-time connection monitoring
- Support for multiple clients
- Typing indicators
- User join/leave notifications

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

## Socket.IO Events

### Client to Server Events

- `message` - Send a message
  ```javascript
  socket.emit('message', { username: 'John', text: 'Hello!' });
  ```

- `typing` - Send typing indicator
  ```javascript
  socket.emit('typing', { username: 'John', isTyping: true });
  ```

- `join` - Join the chat with a username
  ```javascript
  socket.emit('join', 'John');
  ```

### Server to Client Events

- `message` - Receive a message
  ```javascript
  socket.on('message', (data) => {
    console.log(data);
  });
  ```

- `typing` - Receive typing indicator
  ```javascript
  socket.on('typing', (data) => {
    console.log(data);
  });
  ```

- `user-joined` - Notify when a user joins
  ```javascript
  socket.on('user-joined', (data) => {
    console.log('User joined:', data);
  });
  ```

- `user-left` - Notify when a user leaves
  ```javascript
  socket.on('user-left', (data) => {
    console.log('User left:', data);
  });
  ```

## Port Configuration

The default port is `3000`. You can change it by setting the `PORT` environment variable:

```bash
PORT=8080 make start
# or
PORT=8080 go run main.go
```

## Project Structure

```
app-chat-static/
├── main.go            # Go server with Socket.IO
├── go.mod             # Go dependencies
├── Makefile           # Build and run commands
├── status.html        # Status page with connection monitoring
├── admin.html         # Admin monitoring dashboard
├── server.js          # Original Node.js server (legacy)
├── package.json       # Node.js dependencies (legacy)
└── README.md         # This file
```

