# Chat Server with Socket.IO

A simple Express server with Socket.IO for real-time chat functionality.

## Features

- Express web server
- Socket.IO WebSocket support
- Beautiful status page with real-time connection monitoring
- Support for multiple clients
- Typing indicators
- User join/leave notifications

## Installation

```bash
yarn install
```

## Usage

### Start the server

```bash
yarn start
```

Or for development with auto-reload:

```bash
yarn dev
```

The server will start on `http://localhost:3000`

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
PORT=8080 yarn start
```

## Project Structure

```
app-chat-static/
├── server.js          # Express + Socket.IO server
├── status.html        # Status page with connection monitoring
├── package.json       # Dependencies and scripts
├── .gitignore        # Git ignore rules
└── README.md         # This file
```

