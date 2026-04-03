package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	konnektor "github.com/scott-walker/kraube-go-konnektor"
)

func main() {
	ctx := context.Background()

	session, err := konnektor.New(ctx, &konnektor.Options{
		Model: "claude-sonnet-4-5",
		// Custom permission handler — control which tools are allowed
		PermissionHandler: func(req konnektor.ToolPermissionRequest) konnektor.PermissionResponse {
			input, _ := json.MarshalIndent(req.Input, "", "  ")
			fmt.Printf("[Permission] Tool: %s\n  Input: %s\n", req.ToolName, input)

			// Allow reads, deny writes
			switch req.ToolName {
			case "Read", "Glob", "Grep":
				fmt.Println("  → ALLOW")
				return konnektor.PermissionResponse{Behavior: konnektor.PermissionAllow}
			default:
				fmt.Println("  → DENY")
				return konnektor.PermissionResponse{
					Behavior: konnektor.PermissionDeny,
					Message:  "Only read operations are allowed",
				}
			}
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer session.Close()

	messages, err := session.Query("List the files in the current directory and show me the contents of go.mod")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	for msg := range messages {
		if msg.Type == konnektor.MessageTypeAssistant {
			fmt.Print(msg.Text())
		}
	}
	fmt.Println()
}
