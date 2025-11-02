// Package main implements a real-time WebSocket chat server using the Hub pattern.
// It provides:
//   - WebSocket connections for real-time bidirectional communication
//   - Message broadcasting to all connected clients
//   - User join/leave notifications
//   - Support for chat messages with optional price data
//   - CORS-enabled HTTP server with static file serving
//
// Architecture:
//   - Hub pattern: Central message broker managing all client connections
//   - Client struct: Represents each WebSocket connection with read/write pumps
//   - Goroutines: Separate read/write goroutines for each client connection
//
// Message Protocol:
//
//	Client -> Server:
//	  {"type": "join", "username": "string"}     - Join chat with username
//	  {"type": "message", "text": "string", "username": "string", "price": optional}
//
//	Server -> Client:
//	  {"type": "connect", "id": "string"}         - Connection confirmation
//	  {"type": "user-joined", "username": "string", "id": "string"}
//	  {"type": "message", "id": "string", "username": "string", "text": "string", "timestamp": "RFC3339", "price": optional}
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// upgrader upgrades HTTP connections to WebSocket connections.
// CheckOrigin is set to allow all origins (useful for development and cross-origin scenarios).
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins - in production, validate specific origins
	},
}

// Client represents a connected WebSocket client in the chat system.
// Each client has:
//   - ID: Unique identifier generated from UnixNano timestamp
//   - Username: Display name set when user sends a "join" message
//   - Conn: The actual WebSocket connection handle
//   - Hub: Reference to the central message broker
//   - Send: Buffered channel (256 capacity) for outgoing messages
type Client struct {
	ID       string          // Unique client identifier
	Username string          // Display name (can be empty initially)
	Conn     *websocket.Conn // WebSocket connection
	Hub      *Hub            // Reference to the message hub
	Send     chan []byte     // Channel for sending messages to this client
}

// Hub is the central message broker that manages all client connections.
// It implements the Hub pattern for WebSocket message broadcasting:
//   - clients: Map of all active client connections (pointer to Client -> bool)
//   - broadcast: Channel for messages that should be sent to all clients
//   - register: Channel for new client connections to be added
//   - unregister: Channel for client disconnections to be removed
//   - mu: Read-write mutex for thread-safe access to the clients map
//
// The Hub runs in its own goroutine and handles three main operations:
//  1. Register new clients (adds them to the clients map)
//  2. Unregister clients (removes them and closes their Send channel)
//  3. Broadcast messages (sends message to all connected clients)
type Hub struct {
	clients    map[*Client]bool // Active client connections
	broadcast  chan []byte      // Channel for broadcasting messages to all clients
	register   chan *Client     // Channel for registering new clients
	unregister chan *Client     // Channel for unregistering clients
	mu         sync.RWMutex     // Mutex for thread-safe map operations
}

// NewHub creates and initializes a new Hub instance with empty maps and channels.
// Returns a pointer to the Hub that should be started with Run() in a goroutine.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool), // Initialize empty clients map
		broadcast:  make(chan []byte),      // Unbuffered channel for broadcasts
		register:   make(chan *Client),     // Unbuffered channel for registrations
		unregister: make(chan *Client),     // Unbuffered channel for unregistrations
	}
}

// Run starts the hub's main event loop. This should be called in a goroutine.
// The function blocks indefinitely, processing three types of events:
//
// 1. Register: Adds a new client to the clients map (thread-safe with mutex)
// 2. Unregister: Removes a client from the map and closes its Send channel to signal cleanup
// 3. Broadcast: Sends a message to all connected clients via their Send channels
//
// If a client's Send channel is full (blocked), the client is automatically
// removed to prevent blocking the hub. This is a safety mechanism for slow clients.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			// New client connection - add to active clients
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("Client registered: %s", client.ID)

		case client := <-h.unregister:
			// Client disconnected - remove and cleanup
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send) // Signal WritePump to exit
			}
			h.mu.Unlock()
			log.Printf("Client unregistered: %s", client.ID)

		case message := <-h.broadcast:
			// Broadcast message to all active clients
			h.mu.RLock() // Use read lock since we're only reading the map
			for client := range h.clients {
				select {
				case client.Send <- message:
					// Message sent successfully
				default:
					// Send channel is blocked (client too slow) - remove them
					close(client.Send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// ReadPump handles reading messages from the WebSocket connection.
// Runs in a goroutine and processes incoming messages from the client.
//
// Features:
//   - 60-second read deadline with automatic pong handling for keepalive
//   - Graceful cleanup on disconnect (unregisters client and closes connection)
//   - JSON message parsing with error handling
//   - Supports two message types: "join" and "message"
//
// Message Handling:
//   - "join": Sets the client's username and broadcasts join notification to others
//   - "message": Broadcasts chat message to all clients with timestamp and optional price
//
// The function exits when the WebSocket connection is closed or an error occurs.
func (c *Client) ReadPump() {
	// Ensure cleanup happens when this goroutine exits
	defer func() {
		c.Hub.unregister <- c // Signal hub to remove this client
		c.Conn.Close()        // Close WebSocket connection
	}()

	// Set initial read deadline (60 seconds timeout)
	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	// Handle pong messages (keepalive mechanism)
	// Each pong resets the read deadline, preventing timeout for active connections
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Main message reading loop
	for {
		_, messageBytes, err := c.Conn.ReadMessage()
		if err != nil {
			// Log unexpected errors (normal closes are handled silently)
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break // Exit loop and trigger defer cleanup
		}

		// Parse incoming JSON message
		var msg map[string]interface{}
		if err := json.Unmarshal(messageBytes, &msg); err != nil {
			log.Printf("Error parsing message: %v", err)
			continue // Skip malformed messages
		}

		// Extract message type (required field)
		eventType, ok := msg["type"].(string)
		if !ok {
			continue // Skip messages without type field
		}

		// Route message based on type
		switch eventType {
		case "join":
			// Client is joining the chat - set username
			if username, ok := msg["username"].(string); ok {
				c.Username = username

				// Notify all other clients that a user joined
				response := map[string]interface{}{
					"type":     "user-joined",
					"username": username,
					"id":       c.ID,
				}
				c.broadcastToOthers(response) // Don't notify the sender
			}

		case "message":
			// Client is sending a chat message
			username := c.Username // Default to client's stored username
			if u, ok := msg["username"].(string); ok {
				username = u // Override with message username if provided
			}
			text, _ := msg["text"].(string)

			// Build response message with timestamp
			response := map[string]interface{}{
				"type":      "message",
				"id":        c.ID,
				"username":  username,
				"text":      text,
				"timestamp": time.Now().Format(time.RFC3339), // ISO 8601 format
			}
			// Include price field if present (optional field for special message types)
			if price, ok := msg["price"]; ok {
				response["price"] = price
			}
			c.broadcastToAll(response) // Broadcast to all including sender
		}
	}
}

// WritePump handles writing messages to the WebSocket connection.
// Runs in a goroutine and sends messages from the client's Send channel.
//
// Features:
//   - Ping keepalive: Sends ping every 54 seconds to keep connection alive
//   - Message batching: Collects multiple queued messages and sends them in one frame
//   - Write deadlines: 10-second timeout to prevent stuck writes
//   - Graceful shutdown: Sends close message when Send channel is closed
//
// The function exits when:
//   - The Send channel is closed (signaled by hub during unregister)
//   - A write error occurs (connection lost)
//   - Ping fails (connection dead)
func (c *Client) WritePump() {
	// Create ticker for ping keepalive (54 seconds, slightly less than 60s read timeout)
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()  // Stop ticker to prevent leaks
		c.Conn.Close() // Close connection
	}()

	for {
		select {
		case message, ok := <-c.Send:
			// Set write deadline for this operation
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

			// Channel closed means hub wants us to disconnect
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return // Exit goroutine
			}

			// Get a writer for text messages
			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return // Connection lost, exit
			}

			// Write the first message
			w.Write(message)

			// Batch optimization: Send any additional queued messages in the same frame
			// This reduces overhead by sending multiple messages with newline separators
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'}) // Newline separator
				w.Write(<-c.Send)     // Write next queued message
			}

			// Close writer to flush the frame to the network
			if err := w.Close(); err != nil {
				return // Write failed, exit
			}

		case <-ticker.C:
			// Send ping message to keep connection alive
			// The server expects pong responses (handled in ReadPump)
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return // Ping failed, connection dead
			}
		}
	}
}

// broadcastToAll broadcasts a message to all connected clients, including the sender.
// The message is JSON-marshaled and sent to the hub's broadcast channel.
// Used for chat messages so the sender can see their own message echoed back.
func (c *Client) broadcastToAll(data map[string]interface{}) {
	messageBytes, err := json.Marshal(data)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}
	// Send to hub's broadcast channel - hub will distribute to all clients
	c.Hub.broadcast <- messageBytes
}

// broadcastToOthers broadcasts a message to all connected clients except the sender.
// This method directly iterates through clients (with read lock) instead of using
// the hub's broadcast channel, allowing it to exclude the sender.
//
// Used for notifications like "user joined" where the sender doesn't need to
// see their own join notification.
//
// If a client's Send channel is full, that client is automatically removed
// to prevent blocking (same safety mechanism as in Hub.Run).
func (c *Client) broadcastToOthers(data map[string]interface{}) {
	messageBytes, err := json.Marshal(data)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}

	// Lock clients map for reading
	c.Hub.mu.RLock()
	for client := range c.Hub.clients {
		if client != c { // Skip the sender
			select {
			case client.Send <- messageBytes:
				// Message queued successfully
			default:
				// Send channel blocked - remove slow client
				close(client.Send)
				delete(c.Hub.clients, client)
			}
		}
	}
	c.Hub.mu.RUnlock()
}

// hub is the global message broker instance.
// All WebSocket clients register with this hub for message broadcasting.
var hub = NewHub()

// main is the entry point of the application.
// It initializes and starts the HTTP server with WebSocket support.
//
// Server Configuration:
//   - Port: From PORT environment variable, defaults to 3000
//   - CORS: Enabled for all origins (configurable for production)
//   - Routes:
//   - GET /: Serves status.html (main page)
//   - GET /ws: WebSocket endpoint for chat connections
//   - NoRoute: Serves static files if they exist, 404 otherwise
//
// The server runs until interrupted and handles:
//   - HTTP requests for static files and status page
//   - WebSocket connections for real-time chat
//   - CORS preflight requests
func main() {
	// Get port from environment or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	// Start the hub's event loop in a separate goroutine
	// This handles client registration, unregistration, and message broadcasting
	go hub.Run()

	// Initialize Gin router (HTTP framework)
	router := gin.Default()

	// Configure CORS middleware to allow cross-origin requests
	// In production, replace "*" with specific allowed origins
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"}, // All origins (dev mode)
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"*"}, // All headers
		ExposeHeaders:    []string{"*"}, // Expose all headers
		AllowCredentials: true,          // Allow cookies/auth headers
	}))

	// Root route: serve status page
	router.GET("/", func(c *gin.Context) {
		c.File("./status.html")
	})

	// WebSocket endpoint: handles upgrade to WebSocket protocol
	router.GET("/ws", func(c *gin.Context) {
		handleWebSocket(c.Writer, c.Request)
	})

	// Fallback route: serve static files for any other requests
	// Checks if file exists before serving (404 if not found)
	router.NoRoute(func(c *gin.Context) {
		filePath := "./" + c.Request.URL.Path
		if _, err := os.Stat(filePath); err == nil {
			c.File(filePath) // File exists, serve it
		} else {
			c.Status(http.StatusNotFound) // File not found
		}
	})

	// Create and configure HTTP server
	httpServer := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	// Log server startup information
	log.Printf("Server running on http://localhost:%s", port)
	log.Printf("WebSocket server ready for connections at ws://localhost:%s/ws", port)
	log.Printf("Chat app: http://localhost:%s", port)

	// Start server (blocks until error or shutdown)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("Failed to start server:", err)
	}
}

// handleWebSocket upgrades an HTTP connection to WebSocket and sets up a new client.
// This function is called for each WebSocket connection request at /ws endpoint.
//
// Process:
//  1. Upgrade HTTP connection to WebSocket protocol
//  2. Generate unique client ID from current nanosecond timestamp
//  3. Create Client instance with buffered Send channel (256 message capacity)
//  4. Register client with the hub
//  5. Send connection confirmation message to client with their ID
//  6. Start ReadPump and WritePump goroutines for bidirectional communication
//
// The client remains connected until:
//   - Client closes the connection
//   - Network error occurs
//   - Server closes the connection
//
// Once connected, the client can send:
//   - "join" messages to set username
//   - "message" messages to chat
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return // Upgrade failed, connection already handled
	}

	// Generate unique client identifier from current time (nanoseconds)
	// This ensures uniqueness even with rapid connections
	clientID := fmt.Sprintf("%d", time.Now().UnixNano())

	// Create new client instance
	client := &Client{
		ID:       clientID,               // Unique identifier
		Username: "",                     // Will be set when user sends "join"
		Conn:     conn,                   // WebSocket connection
		Hub:      hub,                    // Reference to global hub
		Send:     make(chan []byte, 256), // Buffered channel for outgoing messages
	}

	// Register client with hub (will be added to clients map)
	hub.register <- client

	// Send connection confirmation to client
	// This lets the client know the connection is established and provides their ID
	conn.WriteJSON(map[string]interface{}{
		"type": "connect",
		"id":   clientID,
	})

	log.Printf("######################### handleWebSocket")
	log.Printf("######################### Client connected: %s", clientID)
	log.Printf("######################### handleWebSocket")
	log.Printf("######################### handleWebSocket")

	// Start separate goroutines for reading and writing
	// These run concurrently and handle bidirectional communication
	go client.WritePump() // Handles outgoing messages from hub
	go client.ReadPump()  // Handles incoming messages from client
}
