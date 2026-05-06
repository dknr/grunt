package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/matoous/go-nanoid/v2"
	"grunt"
	"grunt/cmd/internal/storage"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// TokenStore manages authentication tokens.
type TokenStore struct {
	tokens map[string]*TokenEntry
	mu     sync.RWMutex
}

type TokenEntry struct {
	UserID string
	Expiry time.Time
}

var tokenStore = &TokenStore{
	tokens: make(map[string]*TokenEntry),
}

// GenerateToken creates a new auth token for the given user.
func (h *Hub) GenerateToken(userID string) (string, error) {
	token, err := gonanoid.Generate("_-0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", 32)
	if err != nil {
		return "", err
	}
	expiresAt := time.Now().Add(24 * time.Hour)
	tokenStore.mu.Lock()
	tokenStore.tokens[token] = &TokenEntry{
		UserID: userID,
		Expiry: expiresAt,
	}
	tokenStore.mu.Unlock()
	return token, nil
}

// ValidateToken checks if the given token is valid and returns the associated user ID.
func ValidateToken(token string) (string, bool) {
	tokenStore.mu.RLock()
	defer tokenStore.mu.RUnlock()
	entry, ok := tokenStore.tokens[token]
	if !ok {
		return "", false
	}
	if time.Now().After(entry.Expiry) {
		// Token expired, clean it up
		delete(tokenStore.tokens, token)
		return "", false
	}
	return entry.UserID, true
}

type Hub struct {
	clients    map[string]*Client
	mu         sync.RWMutex
	store      *storage.Store
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
}

type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	clientID string
	userID   string
	done     chan struct{}
}

func NewHub(store *storage.Store) *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		store:      store,
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

func (h *Hub) BroadcastMessage(data []byte) {
	h.broadcast <- data
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.clientID] = client
			h.mu.Unlock()
			slog.Info("Client connected", "client_id", client.clientID, "total_clients", len(h.clients))
			h.broadcastSystem(&grunt.System{
				System:   "join",
				ClientID: client.clientID,
			})

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.clientID]; ok {
				delete(h.clients, client.clientID)
				// Do NOT close client.send here; writePump handles it
				slog.Info("Client disconnected", "client_id", client.clientID, "total_clients", len(h.clients))
				h.broadcastSystem(&grunt.System{
					System:   "leave",
					ClientID: client.clientID,
				})
			}
			h.mu.Unlock()

		case msg := <-h.broadcast:
			h.mu.RLock()
			for _, client := range h.clients {
				// Use recover to handle sending to a closed channel safely
				func() {
					defer func() {
						if r := recover(); r != nil {
							slog.Warn("Recovered from panic during broadcast", "client_id", client.clientID, "error", r)
						}
					}()
					client.send <- msg
				}()
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) broadcastSystem(sys *grunt.System) {
	data, err := json.Marshal(sys)
	if err != nil {
		slog.Error("Error marshaling system message", "error", err)
		return
	}
	h.broadcast <- data
}

func (c *Client) readPump() {
	defer func() {
		close(c.done)
		c.hub.unregister <- c
		c.conn.Close()
	}()

	for {
		_, rawMsg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				slog.Warn("WebSocket error for client", "client_id", c.clientID, "user", c.userID, "error", err)
			} else {
				slog.Info("Client read close (normal)", "client_id", c.clientID, "user", c.userID)
			}
			break
		}

		slog.Info("Received message", "client_id", c.clientID, "user", c.userID, "raw", string(rawMsg))

		// Parse client message
		var clientMsg grunt.ClientMsg
		if err := json.Unmarshal(rawMsg, &clientMsg); err != nil {
			slog.Warn("Client sent invalid JSON", "client_id", c.clientID, "user", c.userID, "error", err)
			continue
		}

		slog.Info("Parsed message", "client_id", c.clientID, "user", c.userID, "content", clientMsg.Content)

		// Create broadcast message
		broadcast := &grunt.Broadcast{
			Content:   clientMsg.Content,
			ClientID:  c.clientID,
			UserID:    c.userID,
			Timestamp: time.Now(),
		}

		// Save to storage
		id, err := c.hub.store.Save(broadcast)
		if err != nil {
			slog.Error("Error saving message", "client_id", c.clientID, "user", c.userID, "error", err)
			continue
		}
		broadcast.ID = int(id)

		slog.Info("Saved message", "client_id", c.clientID, "user", c.userID, "id", id)

		// Broadcast to other clients
		data, err := json.Marshal(broadcast)
		if err != nil {
			slog.Error("Error marshaling broadcast", "client_id", c.clientID, "user", c.userID, "error", err)
			continue
		}

		slog.Info("Broadcasting message", "client_id", c.clientID, "user", c.userID, "data", string(data))

		// Send to all clients (including sender for consistency)
		c.hub.broadcast <- data
	}
}

func (c *Client) writePump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
		close(c.send) // writePump is now responsible for closing the channel
		slog.Info("Client write pump exited", "client_id", c.clientID)
	}()

	for {
		select {
		case <-c.done:
			// Connection is closing, stop writing
			return
		case msg, ok := <-c.send:
			if !ok {
				// Channel closed, exit
				return
			}
			err := c.conn.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				slog.Error("Error writing to client", "client_id", c.clientID, "error", err)
				return
			}
		}
	}
}

func (h *Hub) HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		slog.Error("WebSocket upgrade error", "error", err)
		return
	}

	token := c.Query("token")
	if token == "" {
		slog.Warn("WebSocket connection rejected", "reason", "missing token")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
		conn.Close()
		return
	}

	userID, ok := ValidateToken(token)
	if !ok {
		slog.Warn("WebSocket connection rejected", "reason", "invalid or expired token")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
		conn.Close()
		return
	}

	clientID, err := gonanoid.Generate("_-0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", 16)
	if err != nil {
		slog.Error("Error generating client ID", "error", err)
		conn.Close()
		return
	}

	slog.Info("WebSocket connection", "user", userID, "client_id", clientID)

	client := &Client{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, 256),
		done:     make(chan struct{}),
		clientID: clientID,
		userID:   userID,
	}

	h.register <- client
	go client.writePump()
	go client.readPump()
}