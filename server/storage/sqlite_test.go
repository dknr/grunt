package storage

import (
	"testing"
	"time"

	"grunt/client"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := New("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("Failed to create test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestNewDB(t *testing.T) {
	store, err := New("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Tables should be created
	err = store.db.Ping()
	if err != nil {
		t.Fatalf("Database not accessible: %v", err)
	}
}

func TestCreateUser(t *testing.T) {
	store := newTestStore(t)

	// Create a new user
	err := store.CreateUser("testuser", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Try to create the same user again (should not error due to OR IGNORE)
	err = store.CreateUser("testuser", "password456")
	if err != nil {
		t.Fatalf("Failed to create duplicate user: %v", err)
	}
}

func TestVerifyUser(t *testing.T) {
	store := newTestStore(t)

	// Verify nonexistent user
	ok, err := store.VerifyUser("nonexistent", "password")
	if err != nil {
		t.Fatalf("Failed to verify nonexistent user: %v", err)
	}
	if ok {
		t.Error("Nonexistent user should not verify")
	}

	// Create user and verify correct password
	err = store.CreateUser("testuser", "correctpassword")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	ok, err = store.VerifyUser("testuser", "correctpassword")
	if err != nil {
		t.Fatalf("Failed to verify user: %v", err)
	}
	if !ok {
		t.Error("User should verify with correct password")
	}

	// Verify with wrong password
	ok, err = store.VerifyUser("testuser", "wrongpassword")
	if err != nil {
		t.Fatalf("Failed to verify user with wrong password: %v", err)
	}
	if ok {
		t.Error("User should not verify with wrong password")
	}
}

func TestSaveMessage(t *testing.T) {
	store := newTestStore(t)

	// Create a user first
	err := store.CreateUser("testuser", "password")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	msg := &client.Broadcast{
		Content:  "Hello, World!",
		ClientID: "client123",
		UserID:   "testuser",
		Timestamp: time.Now(),
	}

	id, err := store.Save(msg)
	if err != nil {
		t.Fatalf("Failed to save message: %v", err)
	}

	if id <= 0 {
		t.Errorf("Expected positive ID, got %d", id)
	}

	// Verify message was saved
	messages, err := store.Sync(0, 10)
	if err != nil {
		t.Fatalf("Failed to sync messages: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	if messages[0].Content != "Hello, World!" {
		t.Errorf("Expected content 'Hello, World!', got '%s'", messages[0].Content)
	}

	if messages[0].ClientID != "client123" {
		t.Errorf("Expected client_id 'client123', got '%s'", messages[0].ClientID)
	}

	if messages[0].UserID != "testuser" {
		t.Errorf("Expected user 'testuser', got '%s'", messages[0].UserID)
	}
}

func TestSyncMessages(t *testing.T) {
	store := newTestStore(t)

	// Create a user
	err := store.CreateUser("testuser", "password")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Save multiple messages
	for i := 1; i <= 5; i++ {
		msg := &client.Broadcast{
			Content:  "Message " + string(rune('0'+i)),
			ClientID: "client123",
			UserID:   "testuser",
			Timestamp: time.Now(),
		}
		_, err := store.Save(msg)
		if err != nil {
			t.Fatalf("Failed to save message: %v", err)
		}
	}

	// Sync all messages
	messages, err := store.Sync(0, 0)
	if err != nil {
		t.Fatalf("Failed to sync messages: %v", err)
	}

	if len(messages) != 5 {
		t.Fatalf("Expected 5 messages, got %d", len(messages))
	}

	// Sync with limit
	messages, err = store.Sync(0, 3)
	if err != nil {
		t.Fatalf("Failed to sync messages with limit: %v", err)
	}

	if len(messages) != 3 {
		t.Fatalf("Expected 3 messages with limit, got %d", len(messages))
	}

	// Sync after ID 2
	messages, err = store.Sync(2, 0)
	if err != nil {
		t.Fatalf("Failed to sync messages after ID 2: %v", err)
	}

	if len(messages) != 3 {
		t.Fatalf("Expected 3 messages after ID 2, got %d", len(messages))
	}
}

func TestClose(t *testing.T) {
	store, err := New("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	err = store.Close()
	if err != nil {
		t.Fatalf("Failed to close store: %v", err)
	}

	// Try to use after close
	err = store.db.Ping()
	if err == nil {
		t.Error("Expected error when pinging closed database")
	}
}
