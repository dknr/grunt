package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"grunt"
)

var (
	verbose bool
)

var recvCmd = &cobra.Command{
	Use:   "recv",
	Short: "Receive messages from the grunt server",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		login := os.Getenv("GRUNT_LOGIN")
		if login == "" {
			log.Fatal("GRUNT_LOGIN environment variable not set (expected user:password)")
		}
		parts := strings.SplitN(login, ":", 2)
		if len(parts) != 2 {
			log.Fatal("GRUNT_LOGIN invalid (expected user:password)")
		}
		user, password := parts[0], parts[1]

		serverAddr, _ := cmd.Flags().GetString("server")
		if serverAddr == "" {
			serverAddr = "http://localhost:54765"
		}

		client := grunt.NewClient(serverAddr, user)
		defer client.Close()

		if err := client.Register(password); err != nil {
			log.Fatalf("Failed to register: %v", err)
		}

		if err := client.Login(password); err != nil {
			log.Fatalf("Failed to login: %v", err)
		}

		if err := client.Connect(); err != nil {
			log.Fatalf("Failed to connect: %v", err)
		}

		// Start listening for messages BEFORE fetching sync to avoid race condition
		messages := client.StartListening()
		if messages == nil {
			log.Fatalf("Failed to start listening for messages")
		}

		// Fetch sync history
		syncMsgs, err := client.SyncHistory(0) // 0 means get recent messages
		if err != nil {
			log.Printf("Error fetching sync: %v", err)
			// Continue anyway - we can still listen for live messages
		}

		// Print sync messages
		for _, m := range syncMsgs {
			fmt.Printf("[%s] %s: %s\n", m.UserID, m.Timestamp.Format("15:04:05"), m.Content)
		}

		// Listen for new messages
		for msgBytes := range messages {
			var envelope struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(msgBytes, &envelope); err != nil {
				fmt.Printf("Unknown message: %s\n", string(msgBytes))
				continue
			}

			switch envelope.Type {
			case "message":
				var broadcast grunt.Broadcast
				if err := json.Unmarshal(msgBytes, &broadcast); err == nil && broadcast.ID != 0 {
					fmt.Printf("[%s] %s: %s\n", broadcast.UserID, broadcast.Timestamp.Format("15:04:05"), broadcast.Content)
				}
			case "event":
				var evt grunt.Event
				if err := json.Unmarshal(msgBytes, &evt); err == nil && evt.Event != "" {
					if verbose {
						fmt.Printf("[System] %s: %s\n", evt.Event, evt.ClientID)
					}
				}
			case "error":
				var serr grunt.Error
				if err := json.Unmarshal(msgBytes, &serr); err == nil && serr.Message != "" {
					fmt.Printf("[Error] %s\n", serr.Message)
				}
			default:
				fmt.Printf("Unknown message: %s\n", string(msgBytes))
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(recvCmd)
	recvCmd.Flags().String("server", "http://localhost:54765", "Server address")
	recvCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show system messages")
}
