package client

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Client provides a high-level interface for interacting with a grunt server.
type Client struct {
	ServerAddr string
	HTTPClient *http.Client
	UserID     string
	Token      string

	// Channels for communication
	messageChan chan []byte // raw SSE messages
	doneChan    chan struct{}
	listenDone  chan struct{}

	// Internal state
	mutex    sync.Mutex
	connected bool
	listening bool
	resp     *http.Response // SSE response body
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
func (c *Client) Register(password, inviteCode string) error {
	payload, err := json.Marshal(map[string]string{
		"user":       c.UserID,
		"password":   password,
		"invite_code": inviteCode,
	})
	if err != nil {
		return fmt.Errorf("marshal register request: %w", err)
	}
	resp, err := c.HTTPClient.Post(c.ServerAddr+"/api/user", "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to register user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("failed to register user: %s", resp.Status)
	}

	return nil
}

// Invite generates a new invite code using the current token.
func (c *Client) Invite() (string, time.Time, error) {
	req, err := http.NewRequest(http.MethodGet, c.ServerAddr+"/api/user/invite", nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create invite request: %w", err)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to get invite: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("failed to get invite: %s", resp.Status)
	}

	var result struct {
		Code      string `json:"code"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", time.Time{}, fmt.Errorf("failed to decode invite response: %w", err)
	}

	expiresAt, err := time.Parse(time.RFC3339, result.ExpiresAt)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to parse expires_at: %w", err)
	}

	return result.Code, expiresAt, nil
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
	resp, err := c.HTTPClient.Post(c.ServerAddr+"/api/user/login", "application/json", bytes.NewReader(payload))
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

// Connect establishes an SSE connection to the grunt server.
// Message listening is not started automatically; use StartListening() to begin.
// Uses the stored token for authentication via Authorization: Bearer header.
func (c *Client) Connect() error {
	c.mutex.Lock()
	if c.connected {
		c.mutex.Unlock()
		return nil // already connected
	}
	c.mutex.Unlock()

	if c.Token == "" {
		return fmt.Errorf("no token available; call Login() first")
	}

	req, err := http.NewRequest("GET", c.ServerAddr+"/api/chat/stream", nil)
	if err != nil {
		return fmt.Errorf("failed to create SSE request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to SSE stream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("SSE connection failed: %s", resp.Status)
	}

	c.mutex.Lock()
	c.resp = resp
	c.connected = true
	c.mutex.Unlock()

	return nil
}

// SyncHistory fetches message history from the server since the given ID.
// If since is 0, it fetches the most recent messages (up to a server-defined limit).
// Requires authentication via the stored token.
func (c *Client) SyncHistory(since int) ([]Broadcast, error) {
	endpoint := fmt.Sprintf("/api/chat/sync?since=%d", since)
	if since == 0 {
		endpoint = "/api/chat/sync?last=10" // fallback to get last 10 if since=0
	}

	req, err := http.NewRequest(http.MethodGet, c.ServerAddr+endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create sync request: %w", err)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(req)
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

// StartListening begins listening for incoming SSE messages and returns
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

// SendMessage sends a message to the grunt server via HTTP POST.
func (c *Client) SendMessage(content string) error {
	if c.Token == "" {
		return fmt.Errorf("no token available; call Login() first")
	}

	payload, err := json.Marshal(map[string]string{
		"content": content,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.ServerAddr+"/api/chat/message", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create send message request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to send message: %s - %s", resp.Status, string(body))
	}

	return nil
}

// ReceiveMessages returns a read-only channel for receiving raw messages
// from the server. The channel is closed when the connection is closed.
// Callers can unmarshal the bytes as message.Broadcast or message.Event as needed.
func (c *Client) ReceiveMessages() <-chan []byte {
	return c.messageChan
}

// Close closes the SSE connection and cleans up resources.
func (c *Client) Close() error {
	c.mutex.Lock()
	if !c.connected {
		c.mutex.Unlock()
		return nil // already closed
	}
	c.mutex.Unlock()

	close(c.doneChan)
	if c.resp != nil && c.resp.Body != nil {
		c.resp.Body.Close()
	}

	c.mutex.Lock()
	c.connected = false
	c.resp = nil
	c.mutex.Unlock()

	return nil
}

// readPump handles incoming SSE messages and broadcasts them on the message channel.
func (c *Client) readPump() {
	defer func() {
		close(c.messageChan)
	}()

	reader := bufio.NewReader(c.resp.Body)
	var dataBuf strings.Builder

	for {
		select {
		case <-c.doneChan:
			return
		case <-c.listenDone:
			return
		default:
		}

		line, err := reader.ReadBytes('\n')
		if err != nil {
			// EOF or read error — connection closed
			return
		}

		// Strip \r if present (handle \r\n line endings)
		lineStr := strings.TrimRight(string(line), "\r\n")

		if lineStr == "" {
			// Empty line signals end of SSE event
			if dataBuf.Len() > 0 {
				data := make([]byte, dataBuf.Len())
				copy(data, []byte(dataBuf.String()))
				select {
				case c.messageChan <- data:
				case <-c.doneChan:
					return
				case <-c.listenDone:
					return
				}
			}
			dataBuf.Reset()
			continue
		}

		if strings.HasPrefix(lineStr, "data:") {
			dataBuf.WriteString(strings.TrimPrefix(lineStr, "data:"))
			dataBuf.WriteString("\n")
		}
	}
}