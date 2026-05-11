package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/matoous/go-nanoid/v2"
	"grunt/client"
	"grunt/server/storage"
)

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
	subscribers  map[string]*Subscriber
	mu           sync.RWMutex
	store        *storage.Store
	broadcast    chan []byte
	register     chan *Subscriber
	unregister   chan *Subscriber
}

type Subscriber struct {
	hub      *Hub
	send     chan []byte
	clientID string
	userID   string
	done     chan struct{}
	writer   http.ResponseWriter
	flusher  http.Flusher
	ctx      context.Context
}

func NewHub(store *storage.Store) *Hub {
	return &Hub{
		subscribers:  make(map[string]*Subscriber),
		store:        store,
		broadcast:    make(chan []byte, 256),
		register:     make(chan *Subscriber),
		unregister:   make(chan *Subscriber),
	}
}

func (h *Hub) BroadcastMessage(data []byte) {
	h.broadcast <- data
}

func (h *Hub) Run() {
	for {
		select {
		case sub := <-h.register:
			h.mu.Lock()
			h.subscribers[sub.clientID] = sub
			h.mu.Unlock()
			slog.Info("Subscriber connected", "client_id", sub.clientID, "total_subscribers", len(h.subscribers))
			h.broadcastEvent("join", sub.clientID, sub.userID)

		case sub := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.subscribers[sub.clientID]; ok {
				delete(h.subscribers, sub.clientID)
				slog.Info("Subscriber disconnected", "client_id", sub.clientID, "total_subscribers", len(h.subscribers))
				h.broadcastEvent("leave", sub.clientID, sub.userID)
			}
			h.mu.Unlock()

		case msg := <-h.broadcast:
			h.mu.RLock()
			for _, sub := range h.subscribers {
				func() {
					defer func() {
						if r := recover(); r != nil {
							slog.Warn("Recovered from panic during broadcast", "client_id", sub.clientID, "error", r)
						}
					}()
					select {
					case sub.send <- msg:
					default:
						slog.Warn("Subscriber send buffer full, dropping message", "client_id", sub.clientID)
					}
				}()
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) broadcastEvent(event, clientID, userID string) {
	data, err := json.Marshal(&client.Event{
		Type:     "event",
		Event:    event,
		ClientID: clientID,
		UserID:   userID,
	})
	if err != nil {
		slog.Error("Error marshaling event message", "error", err)
		return
	}
	h.broadcast <- data
}

func (s *Subscriber) writePump() {
	defer func() {
		s.hub.unregister <- s
		close(s.send)
		slog.Info("Subscriber write pump exited", "client_id", s.clientID)
	}()

	for {
		select {
		case <-s.ctx.Done():
			return
		case msg, ok := <-s.send:
			if !ok {
				return
			}

			// Determine event type from message content
			var envelope struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(msg, &envelope); err != nil {
				slog.Error("Error unmarshaling message for SSE", "error", err)
				continue
			}

			eventType := "message"
			if envelope.Type == "event" {
				eventType = "event"
			}

			// Write SSE format directly to response writer
			if _, err := s.writer.Write([]byte("event: " + eventType + "\n")); err != nil {
				slog.Error("Error writing event line", "error", err, "client_id", s.clientID)
				return
			}
			if _, err := s.writer.Write([]byte("data: " + string(msg) + "\n\n")); err != nil {
				slog.Error("Error writing data line", "error", err, "client_id", s.clientID)
				return
			}
			s.flusher.Flush()
		}
	}
}

func (h *Hub) HandleSSEStream(w http.ResponseWriter, r *http.Request) {
	// Check for SSE support
	if accept := r.Header.Get("Accept"); accept != "" && !strings.Contains(accept, "text/event-stream") {
		http.Error(w, `{"error":"SSE not supported"}`, http.StatusBadRequest)
		return
	}

	// Authenticate: try Authorization header first, fall back to query param
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	userID, ok := ValidateToken(token)
	if !ok {
		http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
		return
	}

	// Ensure response supports flushing
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Generate client ID
	clientID, err := gonanoid.Generate("_-0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", 16)
	if err != nil {
		slog.Error("Error generating client ID", "error", err)
		http.Error(w, `{"error":"failed to generate client ID"}`, http.StatusInternalServerError)
		return
	}

	slog.Info("SSE connection", "user", userID, "client_id", clientID)

	subscriber := &Subscriber{
		hub:      h,
		send:     make(chan []byte, 256),
		done:     make(chan struct{}),
		clientID: clientID,
		userID:   userID,
		writer:   w,
		flusher:  flusher,
		ctx:      r.Context(),
	}

	h.register <- subscriber
	subscriber.writePump()
}