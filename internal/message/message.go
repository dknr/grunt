package message

import "time"

// Broadcast is the format sent to all connected clients.
type Broadcast struct {
	ID        int       `json:"id"`
	Content   string    `json:"content,omitempty"`
	ClientID  string    `json:"client_id,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// System is a server-to-client notification (join/leave).
type System struct {
	System   string `json:"system"`
	ClientID string `json:"client_id"`
}

// ClientMsg is sent from client to server.
type ClientMsg struct {
	Content string `json:"content"`
}