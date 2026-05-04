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
	"grunt/internal/message"
	"grunt/internal/storage"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
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

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.clientID] = client
			h.mu.Unlock()
			slog.Info("Client connected", "client_id", client.clientID, "total_clients", len(h.clients))
			h.broadcastSystem(&message.System{
				System:   "join",
				ClientID: client.clientID,
			})

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.clientID]; ok {
				delete(h.clients, client.clientID)
				// Do NOT close client.send here; writePump handles it
				slog.Info("Client disconnected", "client_id", client.clientID, "total_clients", len(h.clients))
				h.broadcastSystem(&message.System{
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

func (h *Hub) broadcastSystem(sys *message.System) {
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
				slog.Warn("WebSocket error for client", "client_id", c.clientID, "error", err)
			} else {
				slog.Info("Client read close (normal)", "client_id", c.clientID)
			}
			break
		}

		// Parse client message
		var clientMsg message.ClientMsg
		if err := json.Unmarshal(rawMsg, &clientMsg); err != nil {
			slog.Warn("Client sent invalid JSON", "client_id", c.clientID, "error", err)
			continue
		}

		// Create broadcast message
		broadcast := &message.Broadcast{
			Content:  clientMsg.Content,
			ClientID: c.clientID,
			Timestamp: time.Now(),
		}

		// Save to storage
		id, err := c.hub.store.Save(broadcast)
		if err != nil {
			slog.Error("Error saving message", "error", err)
			continue
		}
		broadcast.ID = int(id)

		// Broadcast to other clients
		data, err := json.Marshal(broadcast)
		if err != nil {
			slog.Error("Error marshaling broadcast", "error", err)
			continue
		}

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

	clientID, err := gonanoid.Generate("_-0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", 16)
	if err != nil {
		slog.Error("Error generating client ID", "error", err)
		conn.Close()
		return
	}

	client := &Client{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, 256),
		done:     make(chan struct{}),
		clientID: clientID,
	}

	h.register <- client
	go client.writePump()
	go client.readPump()
}