package main

import (
	"context"
	"fmt"
	"os"

	konnektor "github.com/scott-walker/kraube-go-konnektor"
)

func main() {
	ctx := context.Background()

	// Simple one-shot query
	text, err := konnektor.QueryText(ctx, "What is 2+2? Answer with just the number.", &konnektor.Options{
		Model:          "claude-sonnet-4-5",
		PermissionMode: konnektor.PermissionModeAuto,
		MaxTurns:       1,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Answer:", text)
}
