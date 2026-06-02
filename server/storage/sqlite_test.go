package storage

import (
	"bytes"
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

	// Try to create the same user again — should fail with constraint violation
	err = store.CreateUser("testuser", "password456")
	if err == nil {
		t.Error("Expected error when creating duplicate user, got nil")
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

	// Verify ascending order (oldest first)
	for i := 1; i < len(messages); i++ {
		if messages[i].ID <= messages[i-1].ID {
			t.Error("Messages should be in ascending ID order")
		}
	}

	// Sync with limit (last 3 messages)
	messages, err = store.Sync(0, 3)
	if err != nil {
		t.Fatalf("Failed to sync messages with limit: %v", err)
	}

	if len(messages) != 3 {
		t.Fatalf("Expected 3 messages with limit, got %d", len(messages))
	}

	// Verify limited results are the last 3 in ascending order
	if messages[0].ID != 3 || messages[1].ID != 4 || messages[2].ID != 5 {
		t.Errorf("Expected last 3 messages (IDs 3,4,5) in ascending order, got IDs %d,%d,%d",
			messages[0].ID, messages[1].ID, messages[2].ID)
	}

	// Sync after ID 2
	messages, err = store.Sync(2, 0)
	if err != nil {
		t.Fatalf("Failed to sync messages after ID 2: %v", err)
	}

	if len(messages) != 3 {
		t.Fatalf("Expected 3 messages after ID 2, got %d", len(messages))
	}

	// Verify ascending order after ID 2
	if messages[0].ID != 3 || messages[1].ID != 4 || messages[2].ID != 5 {
		t.Errorf("Expected messages 3,4,5 in ascending order after ID 2, got IDs %d,%d,%d",
			messages[0].ID, messages[1].ID, messages[2].ID)
	}

	// Sync after last message (should return empty)
	messages, err = store.Sync(5, 0)
	if err != nil {
		t.Fatalf("Failed to sync messages after ID 5: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("Expected 0 messages after ID 5, got %d", len(messages))
	}

	// Sync with limit when no messages match (since beyond max)
	messages, err = store.Sync(10, 3)
	if err != nil {
		t.Fatalf("Failed to sync messages with limit after high ID: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("Expected 0 messages with limit after high ID, got %d", len(messages))
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

func TestSetAndGetAvatar(t *testing.T) {
	store := newTestStore(t)

	err := store.CreateUser("testuser", "password")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	avatarData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xDE, 0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41, 0x54, 0x08, 0xD7, 0x63, 0x60, 0x60, 0x60, 0x00, 0x00, 0x00, 0x05, 0x00, 0x01, 0x12, 0x2E, 0x36, 0x5A, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82}

	if err := store.SetAvatar("testuser", avatarData); err != nil {
		t.Fatalf("Failed to set avatar: %v", err)
	}

	got, err := store.GetAvatar("testuser")
	if err != nil {
		t.Fatalf("Failed to get avatar: %v", err)
	}
	if !bytes.Equal(got, avatarData) {
		t.Error("avatar data mismatch: got different bytes than set")
	}
}

func TestGetAvatar_NonexistentUser(t *testing.T) {
	store := newTestStore(t)

	_, err := store.GetAvatar("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent user, got nil")
	}
}

func TestGetAvatar_NoAvatar(t *testing.T) {
	store := newTestStore(t)

	err := store.CreateUser("nopicture", "password")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	avatar, err := store.GetAvatar("nopicture")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(avatar) != 0 {
		t.Error("expected empty avatar for user without avatar")
	}
}

func TestHasAvatar(t *testing.T) {
	store := newTestStore(t)

	err := store.CreateUser("withpic", "password")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	err = store.CreateUser("withoutpic", "password2")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if store.HasAvatar("withpic") {
		t.Error("expected HasAvatar false before setting")
	}

	avatarData := []byte("fake-png-data")
	if err := store.SetAvatar("withpic", avatarData); err != nil {
		t.Fatalf("Failed to set avatar: %v", err)
	}

	if !store.HasAvatar("withpic") {
		t.Error("expected HasAvatar true after setting")
	}
	if store.HasAvatar("withoutpic") {
		t.Error("expected HasAvatar false for user without avatar")
	}
	if store.HasAvatar("nonexistent") {
		t.Error("expected HasAvatar false for nonexistent user")
	}
}

func TestHasAvatar_NilStore(t *testing.T) {
	var s *Store
	if s.HasAvatar("any") {
		t.Error("nil store should return false for HasAvatar")
	}
}

func TestSetAvatar_NonexistentUser(t *testing.T) {
	store := newTestStore(t)

	// UPDATE on nonexistent user doesn't error in SQLite — it just affects 0 rows.
	// Verify the avatar was NOT stored by checking GetAvatar returns empty.
	err := store.SetAvatar("ghost", []byte("data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := store.GetAvatar("ghost")
	if err == nil && len(got) > 0 {
		t.Error("expected no avatar stored for nonexistent user")
	}
}

func TestUpdateAvatar(t *testing.T) {
	store := newTestStore(t)

	err := store.CreateUser("updatepic", "password")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	first := []byte("first-avatar")
	second := []byte("second-avatar")

	if err := store.SetAvatar("updatepic", first); err != nil {
		t.Fatalf("Failed to set first avatar: %v", err)
	}
	if err := store.SetAvatar("updatepic", second); err != nil {
		t.Fatalf("Failed to set second avatar: %v", err)
	}

	got, err := store.GetAvatar("updatepic")
	if err != nil {
		t.Fatalf("Failed to get avatar: %v", err)
	}
	if !bytes.Equal(got, second) {
		t.Error("avatar was not updated: got old data")
	}
}
