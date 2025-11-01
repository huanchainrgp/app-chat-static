import express from 'express';
import { Server } from 'socket.io';
import { createServer } from 'http';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

const app = express();
const httpServer = createServer(app);
const io = new Server(httpServer, {
  cors: {
    origin: "*",
    methods: ["GET", "POST"]
  }
});

const PORT = process.env.PORT || 3000;

// Store monitoring data
const monitoringData = {
  startTime: new Date(),
  connections: new Map(),
  totalMessages: 0,
  totalEvents: 0,
  messages: [],
  events: []
};

// Serve static files from the current directory
app.use(express.static(__dirname));

// Serve status.html as the main page
app.get('/', (req, res) => {
  res.sendFile(join(__dirname, 'status.html'));
});

// Serve admin.html as monitoring dashboard
app.get('/admin', (req, res) => {
  res.sendFile(join(__dirname, 'admin.html'));
});

// API endpoint to get server status
app.get('/api/status', (req, res) => {
  res.json({
    status: 'running',
    uptime: Date.now() - monitoringData.startTime.getTime(),
    startTime: monitoringData.startTime.toISOString(),
    currentTime: new Date().toISOString(),
    port: PORT,
    activeConnections: monitoringData.connections.size,
    totalMessages: monitoringData.totalMessages,
    totalEvents: monitoringData.totalEvents
  });
});

// API endpoint to get monitoring data
app.get('/api/monitoring', (req, res) => {
  res.json({
    uptime: Date.now() - monitoringData.startTime.getTime(),
    connections: Array.from(monitoringData.connections.values()),
    totalConnections: monitoringData.connections.size,
    totalMessages: monitoringData.totalMessages,
    totalEvents: monitoringData.totalEvents,
    recentMessages: monitoringData.messages.slice(-50),
    recentEvents: monitoringData.events.slice(-100)
  });
});

// Socket.IO connection handling
io.on('connection', (socket) => {
  console.log(`######### User connected: ${socket.id} #########`);
  
  // Track connection
  monitoringData.connections.set(socket.id, {
    id: socket.id,
    connectedAt: new Date(),
    username: null,
    messagesSent: 0,
    lastActivity: new Date()
  });
  
  monitoringData.events.push({
    type: 'connection',
    socketId: socket.id,
    timestamp: new Date().toISOString(),
    message: 'Client connected'
  });

  // Handle incoming messages
  socket.on('message', (data) => {
    console.log('Message received:', data);
    
    monitoringData.totalMessages++;
    monitoringData.totalEvents++;
    
    const connection = monitoringData.connections.get(socket.id);
    if (connection) {
      connection.messagesSent++;
      connection.lastActivity = new Date();
    }
    
    monitoringData.messages.push({
      id: socket.id,
      username: data.username,
      text: data.text,
      timestamp: new Date().toISOString()
    });
    
    // Keep only last 100 messages
    if (monitoringData.messages.length > 100) {
      monitoringData.messages.shift();
    }
    
    monitoringData.events.push({
      type: 'message',
      socketId: socket.id,
      username: data.username,
      timestamp: new Date().toISOString(),
      message: `Message from ${data.username}`
    });
    
    // Broadcast message to all clients including sender
    io.emit('message', {
      id: socket.id,
      ...data,
      timestamp: new Date().toISOString()
    });
  });

  // Handle typing indicators
  socket.on('typing', (data) => {
    socket.broadcast.emit('typing', data);
    
    monitoringData.events.push({
      type: 'typing',
      socketId: socket.id,
      username: data.username,
      timestamp: new Date().toISOString(),
      message: `${data.username} is typing`
    });
  });

  // Handle user joining
  socket.on('join', (username) => {
    socket.username = username;
    
    const connection = monitoringData.connections.get(socket.id);
    if (connection) {
      connection.username = username;
      connection.lastActivity = new Date();
    }
    
    monitoringData.events.push({
      type: 'join',
      socketId: socket.id,
      username: username,
      timestamp: new Date().toISOString(),
      message: `${username} joined the chat`
    });
    
    socket.broadcast.emit('user-joined', {
      username: username,
      id: socket.id
    });
  });

  // Handle disconnection
  socket.on('disconnect', () => {
    console.log(`User disconnected: ${socket.id}`);
    
    monitoringData.connections.delete(socket.id);
    
    monitoringData.events.push({
      type: 'disconnection',
      socketId: socket.id,
      username: socket.username || 'Anonymous',
      timestamp: new Date().toISOString(),
      message: `${socket.username || 'Anonymous'} disconnected`
    });
    
    socket.broadcast.emit('user-left', {
      id: socket.id,
      username: socket.username || 'Anonymous'
    });
    
    // Keep only last 500 events
    if (monitoringData.events.length > 500) {
      monitoringData.events = monitoringData.events.slice(-500);
    }
  });
});

httpServer.listen(PORT, () => {
  console.log(`Server running on http://localhost:${PORT}`);
  console.log(`Socket.IO server ready for connections`);
  console.log(`Chat app: http://localhost:${PORT}`);
  console.log(`Monitoring dashboard: http://localhost:${PORT}/admin`);
});

