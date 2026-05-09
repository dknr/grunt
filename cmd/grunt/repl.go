package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/chzyer/readline"
	"github.com/spf13/cobra"
	"grunt/client"
)

var inviteCode string

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

		if inviteCode == "" {
			log.Fatal("--invite-code flag is required")
		}

		serverAddr, _ := cmd.Flags().GetString("server")
		if serverAddr == "" {
			serverAddr = "http://localhost:54765"
		}

		client := client.NewClient(serverAddr, user)
		defer client.Close()

		if err := client.Register(password, inviteCode); err != nil {
			log.Fatalf("Failed to register: %v", err)
		}

		if err := client.Login(password); err != nil {
			log.Fatalf("Failed to login: %v", err)
		}

		rl, err := readline.NewEx(&readline.Config{
			Prompt:      "> ",
			HistoryLimit: 500,
		})
		if err != nil {
			log.Fatalf("Failed to initialize readline: %v", err)
		}
		defer rl.Close()

		w := rl.Stdout()
		fmt.Fprintf(w, "Connected. Type a message and press Enter. Type 'quit' or 'exit' to leave.\n")

		for {
			line, err := rl.Readline()
			if err != nil {
				if err == readline.ErrInterrupt {
					fmt.Fprintln(w)
					break
				}
				// EOF
				fmt.Fprintln(w)
				break
			}
			line = strings.TrimSpace(line)
			if line == "" || strings.EqualFold(line, "quit") || strings.EqualFold(line, "exit") {
				break
			}

			if err := client.SendMessage(line); err != nil {
				log.Fatalf("Failed to send message: %v", err)
			}

			fmt.Fprintln(w, "Message sent.")
		}
	},
}

func init() {
	rootCmd.AddCommand(replCmd)
	replCmd.Flags().StringVar(&inviteCode, "invite-code", "", "Invite code (required)")
	replCmd.Flags().String("server", "http://localhost:54765", "Server address")
}