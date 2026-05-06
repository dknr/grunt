package grunt

import "time"

// Broadcast is a user message sent to all connected clients.
type Broadcast struct {
	Type      string    `json:"type"`
	ID        int       `json:"id"`
	Content   string    `json:"content,omitempty"`
	ClientID  string    `json:"client_id,omitempty"`
	UserID    string    `json:"user,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// Event is a server-to-client notification (join/leave).
type Event struct {
	Type     string `json:"type"`
	Event    string `json:"event"`
	ClientID string `json:"client_id"`
	UserID   string `json:"user"`
}

// Error is a server-to-client error notification.
type Error struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// ClientMsg is sent from client to server.
type ClientMsg struct {
	Content string `json:"content"`
}