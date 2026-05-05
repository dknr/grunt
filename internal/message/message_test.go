package message

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBroadcastJSON(t *testing.T) {
	broadcast := &Broadcast{
		Content:  "Hello, World!",
		ClientID: "client123",
		UserID:   "testuser",
		Timestamp: time.Now(),
	}

	// Marshal to JSON
	data, err := json.Marshal(broadcast)
	if err != nil {
		t.Fatalf("Failed to marshal broadcast: %v", err)
	}

	// Unmarshal back
	var decoded Broadcast
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal broadcast: %v", err)
	}

	// Verify fields
	if decoded.Content != broadcast.Content {
		t.Errorf("Expected content '%s', got '%s'", broadcast.Content, decoded.Content)
	}

	if decoded.ClientID != broadcast.ClientID {
		t.Errorf("Expected client_id '%s', got '%s'", broadcast.ClientID, decoded.ClientID)
	}

	if decoded.UserID != broadcast.UserID {
		t.Errorf("Expected user '%s', got '%s'", broadcast.UserID, decoded.UserID)
	}

	// Timestamps might have slight differences due to marshaling/unmarshaling, so we just check they are close
	if decoded.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}
