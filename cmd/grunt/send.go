package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"grunt"
)

var sendCmd = &cobra.Command{
	Use:   "send <message>",
	Short: "Send a single message to the grunt server",
	Args:  cobra.ExactArgs(1),
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

		msgText := args[0]
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

		if err := client.SendMessage(msgText); err != nil {
			log.Fatalf("Failed to send message: %v", err)
		}

		fmt.Println("Message sent successfully.")
	},
}

func init() {
	rootCmd.AddCommand(sendCmd)
	sendCmd.Flags().String("server", "http://localhost:54765", "Server address")
}
