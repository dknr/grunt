package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

var sendCmd = &cobra.Command{
	Use:   "send <user> <message>",
	Short: "Send a single message to the grunt server",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		user := args[0]
		msgText := args[1]
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

		msg := map[string]string{
			"content": msgText,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			log.Fatalf("Failed to marshal message: %v", err)
		}

		conn, _, err := websocket.DefaultDialer.Dial(strings.Replace(serverAddr, "http", "ws", 1)+"/ws?user="+user, nil)
		if err != nil {
			log.Fatalf("Failed to connect: %v", err)
		}
		defer conn.Close()

		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Fatalf("Failed to send message: %v", err)
		}

		fmt.Println("Message sent successfully.")
	},
}

func init() {
	rootCmd.AddCommand(sendCmd)
	sendCmd.Flags().String("server", "http://localhost:54765", "Server address")
}