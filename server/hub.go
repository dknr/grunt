package server

import (
	"context"
	"encoding/json"
	"html"
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

type Hub struct {
	subscribers  map[string]*Subscriber
	mu           sync.RWMutex
	store        *storage.Store
	broadcast    chan []byte
	register     chan *Subscriber
	unregister   chan *Subscriber
}

type Subscriber struct {
	hub        *Hub
	send       chan []byte
	clientID   string
	userID     string
	done       chan struct{}
	writer     http.ResponseWriter
	flusher    http.Flusher
	ctx        context.Context
	renderHTML bool // true for web UI (HTMX), false for API clients
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
			slog.Info("Subscriber connected", "client_id", sub.clientID, "total_subscribers", len(h.subscribers), "format", map[bool]string{true: "html", false: "json"}[sub.renderHTML])
			if sub.renderHTML {
				slog.Info("HTML subscriber registered - SSE stream active", "client_id", sub.clientID)
			}
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
			
			// Pre-render HTML if any subscribers want it
			var htmlFragment string
			hasHTMLSubs := false
			htmlSubCount := 0
			jsonSubCount := 0
			for _, sub := range h.subscribers {
				if sub.renderHTML {
					hasHTMLSubs = true
					htmlSubCount++
				} else {
					jsonSubCount++
				}
			}
			
			slog.Info("Broadcasting message", "total_subs", len(h.subscribers), "html_subs", htmlSubCount, "json_subs", jsonSubCount)
			
			if hasHTMLSubs {
				var envelope struct {
					Type string `json:"type"`
				}
				if err := json.Unmarshal(msg, &envelope); err != nil {
					slog.Error("Error unmarshaling broadcast for type detection", "error", err)
				} else if envelope.Type == "event" {
					// Render join/leave events as subdued text: "join: alice"
					var evt client.Event
					if err := json.Unmarshal(msg, &evt); err != nil {
						htmlFragment = `<span class="sse-event">` + html.EscapeString(string(msg)) + `</span>`
					} else {
						htmlFragment = `<span class="sse-event">` + html.EscapeString(evt.Event) + `: ` + html.EscapeString(evt.UserID) + `</span>`
					}
					slog.Debug("Rendered event fragment", "len", len(htmlFragment))
				} else {
					var broadcastMsg client.Broadcast
					if err := json.Unmarshal(msg, &broadcastMsg); err != nil {
						slog.Error("Error unmarshaling message for HTML rendering", "error", err)
					} else {
						htmlFragment = renderMessageHTML(broadcastMsg)
						slog.Debug("Rendered HTML fragment", "len", len(htmlFragment), "user", broadcastMsg.UserID, "id", broadcastMsg.ID)
					}
				}
			}
			
			for _, sub := range h.subscribers {
				func() {
					defer func() {
						if r := recover(); r != nil {
							slog.Warn("Recovered from panic during broadcast", "client_id", sub.clientID, "error", r)
						}
					}()
					
					var dataToSend []byte
					if sub.renderHTML {
						dataToSend = []byte(htmlFragment)
					} else {
						dataToSend = msg
					}
					
					select {
					case sub.send <- dataToSend:
						slog.Debug("Queued message for subscriber", "client_id", sub.clientID, "format", map[bool]string{true: "html", false: "json"}[sub.renderHTML])
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

			if s.renderHTML {
				// HTML subscribers receive pre-rendered fragments.
				// Send event line so HTMX SSE extension matches sse-swap="message"
				if _, err := s.writer.Write([]byte("event: message\n")); err != nil {
					slog.Error("Error writing event line", "error", err, "client_id", s.clientID)
					return
				}
				if _, err := s.writer.Write([]byte("data: " + string(msg) + "\n\n")); err != nil {
					slog.Error("Error writing data line", "error", err, "client_id", s.clientID)
					return
				}
				s.flusher.Flush()
			} else {
				// JSON subscribers: wrap in SSE event format
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
}

func (h *Hub) HandleSSEStream(w http.ResponseWriter, r *http.Request) {
	// Check for SSE support
	if accept := r.Header.Get("Accept"); accept != "" && !strings.Contains(accept, "text/event-stream") {
		http.Error(w, `{"error":"SSE not supported"}`, http.StatusBadRequest)
		return
	}

	// Authenticate via middleware (already validated)
	if !AuthenticatedFromContext(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	userID := UserIDFromContext(r)

	// Ensure response supports flushing
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	// Determine output format based on query parameter
	renderHTML := r.URL.Query().Get("format") == "html"

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

	slog.Info("SSE connection", "user", userID, "client_id", clientID, "format", map[bool]string{true: "html", false: "json"}[renderHTML])

	subscriber := &Subscriber{
		hub:        h,
		send:       make(chan []byte, 256),
		done:       make(chan struct{}),
		clientID:   clientID,
		userID:     userID,
		writer:     w,
		flusher:    flusher,
		ctx:        r.Context(),
		renderHTML: renderHTML,
	}

	h.register <- subscriber
	subscriber.writePump()
}
