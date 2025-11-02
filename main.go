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
