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

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins
	},
}

// Client represents a connected WebSocket client
type Client struct {
	ID       string
	Username string
	Conn     *websocket.Conn
	Hub      *Hub
	Send     chan []byte
	mu       sync.Mutex
}

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

// NewHub creates a new Hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run starts the hub
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("Client registered: %s", client.ID)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)
			}
			h.mu.Unlock()
			log.Printf("Client unregistered: %s", client.ID)

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// ReadPump pumps messages from the WebSocket connection to the hub
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, messageBytes, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Parse incoming message
		var msg map[string]interface{}
		if err := json.Unmarshal(messageBytes, &msg); err != nil {
			log.Printf("Error parsing message: %v", err)
			continue
		}

		eventType, ok := msg["type"].(string)
		if !ok {
			continue
		}

		switch eventType {
		case "join":
			if username, ok := msg["username"].(string); ok {
				c.Username = username
				c.mu.Lock()
				oldID := c.ID
				monitoringData.UpdateUsername(oldID, username)
				c.mu.Unlock()

				// Broadcast user joined
				response := map[string]interface{}{
					"type":     "user-joined",
					"username": username,
					"id":       c.ID,
				}
				c.broadcastToOthers(response)
			}

		case "message":
			username := c.Username
			if u, ok := msg["username"].(string); ok {
				username = u
			}
			text, _ := msg["text"].(string)

			monitoringData.AddMessage(c.ID, username, text)

			// Broadcast message to all clients including sender
			response := map[string]interface{}{
				"type":      "message",
				"id":        c.ID,
				"username":  username,
				"text":      text,
				"timestamp": time.Now().Format(time.RFC3339),
			}
			if price, ok := msg["price"]; ok {
				response["price"] = price
			}
			c.broadcastToAll(response)

		case "typing":
			username := c.Username
			if u, ok := msg["username"].(string); ok {
				username = u
			}
			monitoringData.AddTypingEvent(c.ID, username)

			// Broadcast to all except sender
			response := map[string]interface{}{
				"type":     "typing",
				"username": username,
			}
			if isTyping, ok := msg["isTyping"]; ok {
				response["isTyping"] = isTyping
			}
			c.broadcastToOthers(response)
		}
	}
}

// WritePump pumps messages from the hub to the WebSocket connection
func (c *Client) WritePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued messages
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// broadcastToAll broadcasts a message to all clients including sender
func (c *Client) broadcastToAll(data map[string]interface{}) {
	messageBytes, err := json.Marshal(data)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}
	c.Hub.broadcast <- messageBytes
}

// broadcastToOthers broadcasts a message to all clients except sender
func (c *Client) broadcastToOthers(data map[string]interface{}) {
	messageBytes, err := json.Marshal(data)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}

	c.Hub.mu.RLock()
	for client := range c.Hub.clients {
		if client != c {
			select {
			case client.Send <- messageBytes:
			default:
				close(client.Send)
				delete(c.Hub.clients, client)
			}
		}
	}
	c.Hub.mu.RUnlock()
}

// Connection represents a connected client
type Connection struct {
	ID           string    `json:"id"`
	ConnectedAt  time.Time `json:"connectedAt"`
	Username     *string   `json:"username"`
	MessagesSent int       `json:"messagesSent"`
	LastActivity time.Time `json:"lastActivity"`
}

// Message represents a chat message
type Message struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

// Event represents a monitoring event
type Event struct {
	Type      string    `json:"type"`
	SocketID  string    `json:"socketId"`
	Username  string    `json:"username,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
}

// MonitoringData stores all monitoring information
type MonitoringData struct {
	StartTime     time.Time
	Connections   map[string]*Connection
	TotalMessages int64
	TotalEvents   int64
	Messages      []Message
	Events        []Event
	mu            sync.RWMutex
}

// NewMonitoringData creates a new monitoring data instance
func NewMonitoringData() *MonitoringData {
	return &MonitoringData{
		StartTime:   time.Now(),
		Connections: make(map[string]*Connection),
		Messages:    make([]Message, 0),
		Events:      make([]Event, 0),
	}
}

// AddConnection adds a new connection
func (m *MonitoringData) AddConnection(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Connections[id] = &Connection{
		ID:           id,
		ConnectedAt:  time.Now(),
		Username:     nil,
		MessagesSent: 0,
		LastActivity: time.Now(),
	}

	m.Events = append(m.Events, Event{
		Type:      "connection",
		SocketID:  id,
		Timestamp: time.Now(),
		Message:   "Client connected",
	})
	m.TotalEvents++

	// Keep only last 500 events
	if len(m.Events) > 500 {
		m.Events = m.Events[len(m.Events)-500:]
	}
}

// RemoveConnection removes a connection
func (m *MonitoringData) RemoveConnection(id string, username string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.Connections, id)

	m.Events = append(m.Events, Event{
		Type:      "disconnection",
		SocketID:  id,
		Username:  username,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("%s disconnected", username),
	})
	m.TotalEvents++

	// Keep only last 500 events
	if len(m.Events) > 500 {
		m.Events = m.Events[len(m.Events)-500:]
	}
}

// AddMessage adds a message
func (m *MonitoringData) AddMessage(id, username, text string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.TotalMessages++
	m.TotalEvents++

	conn, exists := m.Connections[id]
	if exists {
		conn.MessagesSent++
		conn.LastActivity = time.Now()
	}

	message := Message{
		ID:        id,
		Username:  username,
		Text:      text,
		Timestamp: time.Now(),
	}
	m.Messages = append(m.Messages, message)

	// Keep only last 100 messages
	if len(m.Messages) > 100 {
		m.Messages = m.Messages[1:]
	}

	m.Events = append(m.Events, Event{
		Type:      "message",
		SocketID:  id,
		Username:  username,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Message from %s", username),
	})

	// Keep only last 500 events
	if len(m.Events) > 500 {
		m.Events = m.Events[len(m.Events)-500:]
	}
}

// UpdateUsername updates a connection's username
func (m *MonitoringData) UpdateUsername(id, username string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if conn, exists := m.Connections[id]; exists {
		conn.Username = &username
		conn.LastActivity = time.Now()
	}

	m.Events = append(m.Events, Event{
		Type:      "join",
		SocketID:  id,
		Username:  username,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("%s joined the chat", username),
	})
	m.TotalEvents++

	// Keep only last 500 events
	if len(m.Events) > 500 {
		m.Events = m.Events[len(m.Events)-500:]
	}
}

// AddTypingEvent adds a typing event
func (m *MonitoringData) AddTypingEvent(id, username string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Events = append(m.Events, Event{
		Type:      "typing",
		SocketID:  id,
		Username:  username,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("%s is typing", username),
	})
	m.TotalEvents++

	// Keep only last 500 events
	if len(m.Events) > 500 {
		m.Events = m.Events[len(m.Events)-500:]
	}
}

// GetStatus returns status information
func (m *MonitoringData) GetStatus() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"status":            "running",
		"uptime":            time.Since(m.StartTime).Milliseconds(),
		"startTime":         m.StartTime.Format(time.RFC3339),
		"currentTime":       time.Now().Format(time.RFC3339),
		"activeConnections": len(m.Connections),
		"totalMessages":     m.TotalMessages,
		"totalEvents":       m.TotalEvents,
	}
}

// GetMonitoring returns monitoring information
func (m *MonitoringData) GetMonitoring() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	connections := make([]Connection, 0, len(m.Connections))
	for _, conn := range m.Connections {
		connections = append(connections, *conn)
	}

	recentMessages := m.Messages
	if len(recentMessages) > 50 {
		recentMessages = recentMessages[len(recentMessages)-50:]
	}

	recentEvents := m.Events
	if len(recentEvents) > 100 {
		recentEvents = recentEvents[len(recentEvents)-100:]
	}

	return map[string]interface{}{
		"uptime":           time.Since(m.StartTime).Milliseconds(),
		"connections":      connections,
		"totalConnections": len(m.Connections),
		"totalMessages":    m.TotalMessages,
		"totalEvents":      m.TotalEvents,
		"recentMessages":   recentMessages,
		"recentEvents":     recentEvents,
	}
}

var monitoringData = NewMonitoringData()
var hub = NewHub()

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	// Start hub
	go hub.Run()

	// Setup Gin router
	router := gin.Default()

	// Configure CORS
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"*"},
		ExposeHeaders:    []string{"*"},
		AllowCredentials: true,
	}))

	// Register specific routes
	router.GET("/", func(c *gin.Context) {
		c.File("./status.html")
	})

	router.GET("/admin", func(c *gin.Context) {
		c.File("./admin.html")
	})

	router.GET("/api/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, monitoringData.GetStatus())
	})

	router.GET("/api/monitoring", func(c *gin.Context) {
		c.JSON(http.StatusOK, monitoringData.GetMonitoring())
	})

	// WebSocket endpoint
	router.GET("/ws", func(c *gin.Context) {
		handleWebSocket(c.Writer, c.Request)
	})

	// Serve static files for unmatched routes
	router.NoRoute(func(c *gin.Context) {
		filePath := "./" + c.Request.URL.Path
		if _, err := os.Stat(filePath); err == nil {
			c.File(filePath)
		} else {
			c.Status(http.StatusNotFound)
		}
	})

	// Create HTTP server
	httpServer := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	log.Printf("Server running on http://localhost:%s", port)
	log.Printf("WebSocket server ready for connections at ws://localhost:%s/ws", port)
	log.Printf("Chat app: http://localhost:%s", port)
	log.Printf("Monitoring dashboard: http://localhost:%s/admin", port)

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("Failed to start server:", err)
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	clientID := fmt.Sprintf("%d", time.Now().UnixNano())
	client := &Client{
		ID:       clientID,
		Username: "",
		Conn:     conn,
		Hub:      hub,
		Send:     make(chan []byte, 256),
	}

	monitoringData.AddConnection(clientID)

	hub.register <- client

	// Send connection confirmation
	conn.WriteJSON(map[string]interface{}{
		"type": "connect",
		"id":   clientID,
	})

	// Start goroutines for reading and writing
	go client.WritePump()
	go client.ReadPump()
}
