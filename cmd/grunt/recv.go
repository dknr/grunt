package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
	"grunt/internal/message"
)

var recvCmd = &cobra.Command{
	Use:   "recv <user>",
	Short: "Receive messages from the grunt server",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		user := args[0]
		serverAddr, _ := cmd.Flags().GetString("server")
		if serverAddr == "" {
			serverAddr = "http://localhost:54765"
		}

		// Register user if not exists
		resp, err := http.Post(serverAddr+"/user", "application/json", strings.NewReader(fmt.Sprintf(`{"user":"%s"}`, user)))
		if err != nil {
			log.Fatalf("Failed to register user: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
			log.Fatalf("Failed to register user: %s", resp.Status)
		}

		// Connect WebSocket first
		conn, _, err := websocket.DefaultDialer.Dial(strings.Replace(serverAddr, "http", "ws", 1)+"/ws?user="+user, nil)
		if err != nil {
			log.Fatalf("Failed to connect: %v", err)
		}
		defer conn.Close()

				var wg sync.WaitGroup

		// Start listener goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				_, msg, err := conn.ReadMessage()
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						log.Printf("Error reading message: %v", err)
					}
					return
				}

				var broadcast message.Broadcast
				if err := json.Unmarshal(msg, &broadcast); err == nil && broadcast.ID != 0 {
					fmt.Printf("[%s] %s: %s\n", broadcast.UserID, broadcast.Timestamp.Format("15:04:05"), broadcast.Content)
					continue
				}

				var sys message.System
				if err := json.Unmarshal(msg, &sys); err == nil && sys.System != "" {
					fmt.Printf("[System] %s: %s\n", sys.System, sys.ClientID)
					continue
				}

				fmt.Printf("Unknown message: %s\n", string(msg))
			}
		}()

		// Fetch sync history
		resp2, err := http.Get(serverAddr+"/sync?last=10")
		if err != nil {
			log.Printf("Error fetching sync: %v", err)
			return
		}
		defer resp2.Body.Close()

		var syncMsgs []message.Broadcast
		if err := json.NewDecoder(resp2.Body).Decode(&syncMsgs); err != nil {
			log.Printf("Error decoding sync response: %v", err)
			return
		}

		// Wait for a brief moment to let any in-flight messages be buffered
		time.Sleep(100 * time.Millisecond)

		// Print sync messages
		for _, m := range syncMsgs {
			fmt.Printf("[%s] %s: %s\n", m.UserID, m.Timestamp.Format("15:04:05"), m.Content)
		}

		// Keep listening for new messages
		wg.Wait()
	},
}

func init() {
	rootCmd.AddCommand(recvCmd)
	recvCmd.Flags().String("server", "http://localhost:54765", "Server address")
}