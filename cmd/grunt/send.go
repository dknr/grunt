package main

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"grunt"
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

		client := grunt.NewClient(serverAddr, user)
		defer client.Close()

		if err := client.Register(); err != nil {
			log.Fatalf("Failed to register user: %v", err)
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
