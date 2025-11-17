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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
)

func init() {
	configureLogger()
}

func configureLogger() {
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	})
}

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
	Room     string          // Current room
	Joined   bool            // Whether the client finished join handshake
	Conn     *websocket.Conn // WebSocket connection
	Hub      *Hub            // Reference to the message hub
	Send     chan []byte     // Channel for sending messages to this client
}

// Broadcast represents a room-scoped message that should be delivered
// to all clients inside the same room, optionally excluding the sender.
type Broadcast struct {
	Room    string
	Data    []byte
	Exclude *Client
}

// UserStore manages registration credentials and active sessions.
type UserStore struct {
	mu       sync.RWMutex
	users    map[string]string // username -> hashed password
	sessions map[string]string // token -> username
}

// Room represents a chat room metadata record.
type Room struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// RoomStore stores available rooms.
type RoomStore struct {
	mu    sync.RWMutex
	rooms map[string]*Room
}

// NewUserStore creates a new user store.
func NewUserStore() *UserStore {
	return &UserStore{
		users:    make(map[string]string),
		sessions: make(map[string]string),
	}
}

// Register creates a new user and returns an auth token.
func (s *UserStore) Register(username, password string) (string, error) {
	username = strings.TrimSpace(strings.ToLower(username))
	if len(username) < 3 {
		return "", errors.New("tên người dùng phải có ít nhất 3 ký tự")
	}
	if len(password) < 6 {
		return "", errors.New("mật khẩu phải có ít nhất 6 ký tự")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[username]; exists {
		return "", errors.New("tên người dùng đã tồn tại")
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	s.users[username] = string(hashed)
	return s.createSession(username)
}

// Login validates credentials and returns a token.
func (s *UserStore) Login(username, password string) (string, error) {
	username = strings.TrimSpace(strings.ToLower(username))

	s.mu.RLock()
	hashed, exists := s.users[username]
	s.mu.RUnlock()

	if !exists {
		return "", errors.New("sai tên đăng nhập hoặc mật khẩu")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hashed), []byte(password)); err != nil {
		return "", errors.New("sai tên đăng nhập hoặc mật khẩu")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.createSession(username)
}

func (s *UserStore) createSession(username string) (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", err
	}
	s.sessions[token] = username
	return token, nil
}

// ValidateToken returns username for a given token.
func (s *UserStore) ValidateToken(token string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	username, ok := s.sessions[token]
	return username, ok
}

// NewRoomStore initializes rooms with a default "general" room.
func NewRoomStore() *RoomStore {
	store := &RoomStore{
		rooms: make(map[string]*Room),
	}
	store.rooms["general"] = &Room{
		Name:      "general",
		CreatedAt: time.Now(),
	}
	return store
}

// generateToken returns a random 32-byte hex token.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

var roomNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,30}$`)

// CreateRoom registers a new room.
func (r *RoomStore) CreateRoom(name string) error {
	name = strings.TrimSpace(strings.ToLower(name))
	if !roomNamePattern.MatchString(name) {
		return errors.New("tên phòng phải từ 3-30 ký tự, chỉ gồm chữ, số, -, _")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.rooms[name]; exists {
		return errors.New("phòng đã tồn tại")
	}

	r.rooms[name] = &Room{
		Name:      name,
		CreatedAt: time.Now(),
	}
	return nil
}

// Exists checks if a room is available.
func (r *RoomStore) Exists(name string) bool {
	name = strings.TrimSpace(strings.ToLower(name))
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.rooms[name]
	return ok
}

// List returns all rooms.
func (r *RoomStore) List() []*Room {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*Room, 0, len(r.rooms))
	for _, room := range r.rooms {
		result = append(result, room)
	}
	return result
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
	broadcast  chan Broadcast   // Channel for broadcasting messages to all clients
	register   chan *Client     // Channel for registering new clients
	unregister chan *Client     // Channel for unregistering clients
	mu         sync.RWMutex     // Mutex for thread-safe map operations
}

// NewHub creates and initializes a new Hub instance with empty maps and channels.
// Returns a pointer to the Hub that should be started with Run() in a goroutine.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool), // Initialize empty clients map
		broadcast:  make(chan Broadcast),   // Unbuffered channel for broadcasts
		register:   make(chan *Client),     // Unbuffered channel for registrations
		unregister: make(chan *Client),     // Unbuffered channel for unregistrations
	}
}

// RoomMemberCount returns the number of clients in a room.
func (h *Hub) RoomMemberCount(room string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	count := 0
	for client := range h.clients {
		if client.Room == room {
			count++
		}
	}
	return count
}

// HasClient checks if a client is still registered inside the hub.
func (h *Hub) HasClient(target *Client) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.clients[target]
	return ok
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
			log.Debug().Str("client_id", client.ID).Msg("Client registered")

		case client := <-h.unregister:
			// Client disconnected - remove and cleanup
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send) // Signal WritePump to exit
			}
			h.mu.Unlock()
			log.Debug().Str("client_id", client.ID).Msg("Client unregistered")

		case message := <-h.broadcast:
			// Broadcast message to all active clients in the same room
			h.mu.RLock()
			for client := range h.clients {
				if client.Room != message.Room {
					continue
				}
				if message.Exclude != nil && client == message.Exclude {
					continue
				}
				select {
				case client.Send <- message.Data:
				default:
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
		if c.Joined && c.Room != "" {
			c.broadcastPresence("user-left", false)
		}
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
				log.Warn().Err(err).Msg("WebSocket unexpected close")
			}
			break // Exit loop and trigger defer cleanup
		}

		// Parse incoming JSON message
		var msg map[string]interface{}
		if err := json.Unmarshal(messageBytes, &msg); err != nil {
			log.Error().Err(err).Msg("Failed to parse incoming message")
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
			// Finish handshake: only handle first join message
			if c.Joined || c.Room == "" {
				continue
			}
			c.Joined = true

			// Notify others
			c.broadcastPresence("user-joined", true)

			// Send room count back to the joining client
			c.sendDirect(map[string]interface{}{
				"type":  "room-count",
				"room":  c.Room,
				"count": c.roomCount(true),
			})

		case "message":
			if !c.Joined || c.Room == "" {
				continue
			}

			// Client is sending a chat message
			text, _ := msg["text"].(string)
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}

			// Build response message with timestamp
			response := map[string]interface{}{
				"type":      "message",
				"id":        c.ID,
				"username":  c.Username,
				"text":      text,
				"room":      c.Room,
				"timestamp": time.Now().Format(time.RFC3339), // ISO 8601 format
			}
			// Include price field if present (optional field for special message types)
			if price, ok := msg["price"]; ok {
				response["price"] = price
			}
			c.broadcastToRoom(true, response) // Broadcast to all including sender
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

// broadcastToRoom sends a message to all users in the client's room.
func (c *Client) broadcastToRoom(includeSelf bool, data map[string]interface{}) {
	if c.Room == "" {
		return
	}
	messageBytes, err := json.Marshal(data)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal broadcast payload")
		return
	}

	var exclude *Client
	if !includeSelf {
		exclude = c
	}

	c.Hub.broadcast <- Broadcast{
		Room:    c.Room,
		Data:    messageBytes,
		Exclude: exclude,
	}
}

// sendDirect queues a message directly to this client.
func (c *Client) sendDirect(data map[string]interface{}) {
	messageBytes, err := json.Marshal(data)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal direct payload")
		return
	}

	select {
	case c.Send <- messageBytes:
	default:
		log.Warn().Str("client_id", c.ID).Msg("Send buffer full")
	}
}

// roomCount calculates the current room size with optional inclusion of self.
func (c *Client) roomCount(includeSelf bool) int {
	count := c.Hub.RoomMemberCount(c.Room)
	if !includeSelf && count > 0 && c.Hub.HasClient(c) {
		count--
	}
	return count
}

// broadcastPresence notifies the room of joins/leaves.
func (c *Client) broadcastPresence(eventType string, includeSelf bool) {
	data := map[string]interface{}{
		"type":     eventType,
		"username": c.Username,
		"id":       c.ID,
		"room":     c.Room,
		"count":    c.roomCount(includeSelf),
	}
	c.broadcastToRoom(false, data)
}

// hub is the global message broker instance.
// All WebSocket clients register with this hub for message broadcasting.
var (
	hub       = NewHub()
	userStore = NewUserStore()
	roomStore = NewRoomStore()
)

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

	// Auth + room APIs
	router.POST("/api/register", handleRegister)
	router.POST("/api/login", handleLogin)
	router.GET("/api/rooms", handleListRooms)
	router.POST("/api/rooms", handleCreateRoom)

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
	log.Info().Msgf("Server running on http://localhost:%s", port)
	log.Info().Msgf("WebSocket endpoint ready at ws://localhost:%s/ws", port)
	log.Info().Msgf("Chat app UI: http://localhost:%s", port)

	// Start server (blocks until error or shutdown)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal().Err(err).Msg("Failed to start server")
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
	query := r.URL.Query()
	token := query.Get("token")
	roomName := strings.TrimSpace(strings.ToLower(query.Get("room")))

	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	username, ok := userStore.ValidateToken(token)
	if !ok {
		http.Error(w, "token không hợp lệ", http.StatusUnauthorized)
		return
	}

	if roomName == "" || !roomStore.Exists(roomName) {
		http.Error(w, "phòng không tồn tại", http.StatusBadRequest)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("WebSocket upgrade failed")
		return // Upgrade failed, connection already handled
	}

	// Generate unique client identifier from current time (nanoseconds)
	// This ensures uniqueness even with rapid connections
	clientID := fmt.Sprintf("%d", time.Now().UnixNano())

	// Create new client instance
	client := &Client{
		ID:       clientID,               // Unique identifier
		Username: username,               // Already authenticated
		Room:     roomName,               // Room from query
		Conn:     conn,                   // WebSocket connection
		Hub:      hub,                    // Reference to global hub
		Send:     make(chan []byte, 256), // Buffered channel for outgoing messages
	}

	// Register client with hub (will be added to clients map)
	hub.register <- client

	// Send connection confirmation to client
	conn.WriteJSON(map[string]interface{}{
		"type":  "connect",
		"id":    clientID,
		"room":  roomName,
		"name":  username,
		"count": hub.RoomMemberCount(roomName),
	})

	log.Debug().
		Str("client_id", clientID).
		Str("room", roomName).
		Msg("Client connected")

	// Start separate goroutines for reading and writing
	// These run concurrently and handle bidirectional communication
	go client.WritePump() // Handles outgoing messages from hub
	go client.ReadPump()  // Handles incoming messages from client
}

// ===== HTTP Handlers =====
type authRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
}

func handleRegister(c *gin.Context) {
	var req authRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dữ liệu không hợp lệ"})
		return
	}

	token, err := userStore.Register(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, authResponse{
		Token:    token,
		Username: strings.TrimSpace(strings.ToLower(req.Username)),
	})
}

func handleLogin(c *gin.Context) {
	var req authRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dữ liệu không hợp lệ"})
		return
	}

	token, err := userStore.Login(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, authResponse{
		Token:    token,
		Username: strings.TrimSpace(strings.ToLower(req.Username)),
	})
}

func handleListRooms(c *gin.Context) {
	rooms := roomStore.List()
	payload := make([]gin.H, 0, len(rooms))
	for _, room := range rooms {
		payload = append(payload, gin.H{
			"name":       room.Name,
			"created_at": room.CreatedAt,
			"members":    hub.RoomMemberCount(room.Name),
		})
	}
	c.JSON(http.StatusOK, gin.H{"rooms": payload})
}

func handleCreateRoom(c *gin.Context) {
	token := parseAuthToken(c.GetHeader("Authorization"))
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "thiếu token"})
		return
	}
	if _, ok := userStore.ValidateToken(token); !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token không hợp lệ"})
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dữ liệu không hợp lệ"})
		return
	}

	if err := roomStore.CreateRoom(req.Name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"name": strings.TrimSpace(strings.ToLower(req.Name))})
}

func parseAuthToken(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
