package server

import (
	"sync"
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
		hub.Stop()
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

// TestBroadcastEventNonBlockingWhenBufferFull guards against the Run loop
// deadlocking on its own send: broadcastEvent is called from inside the Run
// loop, which is the sole consumer of h.broadcast.
func TestBroadcastEventNonBlockingWhenBufferFull(t *testing.T) {
	hub := NewHub(nil)

	// Run loop is intentionally not started: a full buffer with no
	// consumer is the worst case for a blocking send.
	for i := 0; i < cap(hub.broadcast); i++ {
		hub.broadcast <- []byte(`{"type":"message","content":"filler"}`)
	}

	done := make(chan struct{})
	go func() {
		hub.broadcastEvent("join", "test-client", "test-user")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("broadcastEvent blocked on a full broadcast buffer")
	}
}

// TestRegisterAndUnregisterDuringBroadcastFlood simulates a burst of
// SendMessage calls flooding the broadcast buffer while an SSE client
// connects and disconnects; register/unregister must still be processed.
func TestRegisterAndUnregisterDuringBroadcastFlood(t *testing.T) {
	hub := newTestHub(t)

	sub := &Subscriber{
		hub:      hub,
		send:     make(chan []byte, 256),
		clientID: "flood-client",
		done:     make(chan struct{}),
	}

	stopFlood := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stopFlood:
					return
				default:
					hub.BroadcastMessage([]byte(`{"type":"message","content":"burst"}`))
				}
			}
		}()
	}

	deadline := time.Now().Add(3 * time.Second)

	hub.register <- sub
	for {
		hub.mu.RLock()
		_, ok := hub.subscribers[sub.clientID]
		hub.mu.RUnlock()
		if ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("register not processed during broadcast flood")
		}
		time.Sleep(time.Millisecond)
	}

	hub.unregister <- sub
	for {
		hub.mu.RLock()
		_, ok := hub.subscribers[sub.clientID]
		hub.mu.RUnlock()
		if !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("unregister not processed during broadcast flood")
		}
		time.Sleep(time.Millisecond)
	}

	close(stopFlood)
	wg.Wait()
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
