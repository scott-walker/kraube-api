package main

import (
	"context"
	"fmt"
	"os"

	konnektor "github.com/scott-walker/kraube-go-konnektor"
)

func main() {
	ctx := context.Background()

	// Create a persistent session
	session, err := konnektor.New(ctx, &konnektor.Options{
		Model:          "claude-sonnet-4-5",
		PermissionMode: konnektor.PermissionModeAuto,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating session: %v\n", err)
		os.Exit(1)
	}
	defer session.Close()

	fmt.Printf("Session ID: %s\n", session.SessionID())

	// First query — streaming messages
	messages, err := session.Query("Write a haiku about Go programming.")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n--- First query ---")
	for msg := range messages {
		switch msg.Type {
		case konnektor.MessageTypeAssistant:
			fmt.Print(msg.Text())
		case konnektor.MessageTypeResult:
			fmt.Printf("\n[Done: %d turns, $%.4f]\n",
				msg.Result.NumTurns, msg.Result.TotalCostUSD)
		}
	}

	// Second query — same session, retains context
	messages, err = session.Query("Now write another one about Rust.")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n--- Second query (same session) ---")
	for msg := range messages {
		switch msg.Type {
		case konnektor.MessageTypeAssistant:
			fmt.Print(msg.Text())
		case konnektor.MessageTypeResult:
			fmt.Printf("\n[Done: %d turns, $%.4f]\n",
				msg.Result.NumTurns, msg.Result.TotalCostUSD)
		}
	}
}
