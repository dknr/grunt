package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"grunt"
)

var replCmd = &cobra.Command{
	Use:   "repl",
	Short: "Interactive REPL for sending messages to the grunt server",
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

		scanner := bufio.NewScanner(os.Stdin)
		fmt.Println("Connected. Type a message and press Enter. Type 'quit' or 'exit' to leave.")

		for {
			fmt.Print("> ")
			if !scanner.Scan() {
				fmt.Println()
				break
			}
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.EqualFold(line, "quit") || strings.EqualFold(line, "exit") {
				break
			}

			if err := client.SendMessage(line); err != nil {
				log.Fatalf("Failed to send message: %v", err)
			}

			fmt.Println("Message sent.")
		}

		if err := scanner.Err(); err != nil {
			log.Fatalf("Error reading input: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(replCmd)
	replCmd.Flags().String("server", "http://localhost:54765", "Server address")
}