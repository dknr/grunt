package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

// Client provides a high-level interface for interacting with a grunt server.
type Client struct {
	ServerAddr string
	HTTPClient *http.Client
	WSConn     *websocket.Conn
	UserID     string
	Token      string

	// Channels for communication
	messageChan chan []byte // raw WebSocket messages
	doneChan    chan struct{}
	listenDone  chan struct{}

	// Internal state
	mutex    sync.Mutex
	connected bool
	listening bool
}

// NewClient creates a new grunt client for the given server address and user ID.
func NewClient(serverAddr, userID string) *Client {
	return &Client{
		ServerAddr: serverAddr,
		HTTPClient: &http.Client{},
		UserID:     userID,
		messageChan: make(chan []byte, 100),
		doneChan:    make(chan struct{}),
		listenDone:  make(chan struct{}),
	}
}

// Register registers the user with the grunt server.
func (c *Client) Register(password string) error {
	payload, err := json.Marshal(map[string]string{
		"user":     c.UserID,
		"password": password,
	})
	if err != nil {
		return fmt.Errorf("marshal register request: %w", err)
	}
	resp, err := c.HTTPClient.Post(c.ServerAddr+"/user", "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to register user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("failed to register user: %s", resp.Status)
	}

	return nil
}

// Login authenticates the user with the grunt server and stores the token.
func (c *Client) Login(password string) error {
	payload, err := json.Marshal(map[string]string{
		"user":     c.UserID,
		"password": password,
	})
	if err != nil {
		return fmt.Errorf("marshal login request: %w", err)
	}
	resp, err := c.HTTPClient.Post(c.ServerAddr+"/auth/login", "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to login: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to login: %s", resp.Status)
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode login response: %w", err)
	}

	c.Token = result.Token
	return nil
}

// Connect establishes a WebSocket connection to the grunt server.
// Message listening is not started automatically; use StartListening() to begin.
// Uses the stored token for authentication.
func (c *Client) Connect() error {
	c.mutex.Lock()
	if c.connected {
		c.mutex.Unlock()
		return nil // already connected
	}
	c.mutex.Unlock()

	var wsURL string
	if c.Token != "" {
		wsURL = strings.Replace(c.ServerAddr, "http", "ws", 1) + "/ws?token=" + c.Token
	} else {
		wsURL = strings.Replace(c.ServerAddr, "http", "ws", 1) + "/ws?user=" + c.UserID
	}

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to websocket: %w", err)
	}
	c.WSConn = conn

	c.mutex.Lock()
	c.connected = true
	c.mutex.Unlock()

	return nil
}

// SyncHistory fetches message history from the server since the given ID.
// If since is 0, it fetches the most recent messages (up to a server-defined limit).
func (c *Client) SyncHistory(since int) ([]Broadcast, error) {
	endpoint := fmt.Sprintf("/sync?since=%d", since)
	if since == 0 {
		endpoint = "/sync?last=10" // fallback to get last 10 if since=0
	}

	resp, err := c.HTTPClient.Get(c.ServerAddr + endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sync history: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code fetching sync: %d", resp.StatusCode)
	}

	var broadcasts []Broadcast
	if err := json.NewDecoder(resp.Body).Decode(&broadcasts); err != nil {
		return nil, fmt.Errorf("failed to decode sync response: %w", err)
	}

	return broadcasts, nil
}

// StartListening begins listening for incoming WebSocket messages and returns
// a read-only channel for receiving them. Should be called after Connect().
func (c *Client) StartListening() <-chan []byte {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if !c.connected {
		// Not connected, return nil channel
		return nil
	}
	if c.listening {
		// Already listening
		return c.messageChan
	}
	c.listening = true
	go c.readPump()
	return c.messageChan
}

// StopListening stops listening for incoming WebSocket messages.
func (c *Client) StopListening() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if !c.listening {
		return
	}
	c.listening = false
	close(c.listenDone)
}

// SendMessage sends a message to the grunt server via the WebSocket connection.
func (c *Client) SendMessage(content string) error {
	c.mutex.Lock()
	if !c.connected || c.WSConn == nil {
		c.mutex.Unlock()
		return fmt.Errorf("websocket not connected")
	}
	c.mutex.Unlock()

	clientMsg := ClientMsg{Content: content}
	data, err := json.Marshal(clientMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	err = c.WSConn.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// ReceiveMessages returns a read-only channel for receiving raw WebSocket messages
// from the server. The channel is closed when the connection is closed.
// Callers can unmarshal the bytes as message.Broadcast or message.System as needed.
func (c *Client) ReceiveMessages() <-chan []byte {
	return c.messageChan
}

// Close closes the WebSocket connection and cleans up resources.
func (c *Client) Close() error {
	c.mutex.Lock()
	if !c.connected {
		c.mutex.Unlock()
		return nil // already closed
	}
	c.mutex.Unlock()

	close(c.doneChan)
	if c.WSConn != nil {
		err := c.WSConn.Close()
		if err != nil {
			return fmt.Errorf("error closing websocket: %w", err)
		}
	}

	c.mutex.Lock()
	c.connected = false
	c.mutex.Unlock()

	return nil
}

// readPump handles incoming WebSocket messages and broadcasts them on the message channel.
func (c *Client) readPump() {
	defer func() {
		close(c.messageChan)
	}()

	for {
		select {
		case <-c.doneChan:
			// Connection closed, stop pumping
			return
		case <-c.listenDone:
			// Listener explicitly stopped
			return
		default:
			_, msg, err := c.WSConn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					// Log error but don't return - connection might be closing normally
				}
				return
			}

			select {
			case c.messageChan <- msg:
			case <-c.doneChan:
				return
			case <-c.listenDone:
				return
			}
		}
	}
}