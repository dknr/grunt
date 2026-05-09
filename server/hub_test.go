package server

import (
	"testing"
	"time"

	"grunt/server/storage"
)

func newTestHub(t *testing.T) *Hub {
	t.Helper()
	store, err := storage.New("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("Failed to create test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	hub := NewHub(store)
	go hub.Run()
	t.Cleanup(func() {
		// Stop the hub by closing the register channel if needed, 
		// but for now we just let it run or rely on test timeout.
		// A better way is to add a Stop method, but for now we skip graceful shutdown in tests.
	})
	return hub
}

func TestHubStartStop(t *testing.T) {
	hub := newTestHub(t)
	if hub == nil {
		t.Fatal("Hub should not be nil")
	}
	// If we get here without panic, the hub started successfully
}

func TestBroadcast(t *testing.T) {
	hub := newTestHub(t)

	// Create a mock subscriber
	sub := &Subscriber{
		hub:      hub,
		send:     make(chan []byte, 256),
		clientID: "test-client",
		done:     make(chan struct{}),
	}

	// Register the subscriber
	hub.register <- sub

	// Give it time to register
	time.Sleep(10 * time.Millisecond)

	// Broadcast a message
	msgData := []byte(`{"content":"test"}`)
	hub.BroadcastMessage(msgData)

	// Check if subscriber received the message
	// Discard the "join" system message that was sent when the subscriber registered
	select {
	case <-sub.send:
		// Expected "join" message, discard it
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for join message")
	}

	// Now check for the test message
	select {
	case received := <-sub.send:
		if string(received) != string(msgData) {
			t.Errorf("Expected message %s, got %s", msgData, received)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for message")
	}

	// Cleanup
	close(sub.done)
}

func TestClientRegister(t *testing.T) {
	hub := newTestHub(t)

	sub := &Subscriber{
		hub:      hub,
		send:     make(chan []byte, 256),
		clientID: "test-client-reg",
		done:     make(chan struct{}),
	}

	// Register the subscriber
	hub.register <- sub

	// Give it time to register
	time.Sleep(10 * time.Millisecond)

	// Check if subscriber is in the hub's subscriber map
	hub.mu.RLock()
	_, ok := hub.subscribers[sub.clientID]
	hub.mu.RUnlock()

	if !ok {
		t.Error("Subscriber should be registered in hub")
	}

	// Cleanup
	close(sub.done)
}
