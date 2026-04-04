package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/kraube-go/kraube"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  kraube login              — authenticate via browser")
		fmt.Fprintln(os.Stderr, "  kraube login --claude      — reuse Claude Code credentials")
		fmt.Fprintln(os.Stderr, "  kraube \"your prompt\"       — send a message")
		fmt.Fprintln(os.Stderr, "  kraube stream \"prompt\"     — stream response")
		os.Exit(1)
	}

	ctx := context.Background()

	switch os.Args[1] {
	case "login":
		cmdLogin(ctx)
	case "stream":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: kraube stream \"prompt\"")
			os.Exit(1)
		}
		cmdStream(ctx, strings.Join(os.Args[2:], " "))
	default:
		cmdQuery(ctx, strings.Join(os.Args[1:], " "))
	}
}

func cmdLogin(ctx context.Context) {
	// Check for --claude flag
	for _, arg := range os.Args[2:] {
		if arg == "--claude" {
			creds, err := kraube.LoadClaudeCredentials()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to load Claude credentials: %v\n", err)
				os.Exit(1)
			}
			path := kraube.DefaultCredentialsPath()
			if err := kraube.SaveCredentials(path, creds); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to save credentials: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Credentials imported from Claude Code → %s\n", path)
			return
		}
	}

	// Full OAuth flow
	fmt.Println("Opening browser for authentication...")
	creds, err := kraube.Login(ctx, openBrowser)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Login failed: %v\n", err)
		os.Exit(1)
	}

	path := kraube.DefaultCredentialsPath()
	if err := kraube.SaveCredentials(path, creds); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save credentials: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Authenticated! Credentials saved to %s\n", path)
}

func cmdQuery(ctx context.Context, prompt string) {
	client := mustClient(ctx)

	resp, err := client.Messages.Create(ctx, &kraube.MessageRequest{
		Model:     kraube.ModelSonnet4_6,
		MaxTokens: 4096,
		Messages:  []kraube.Message{kraube.UserMessage(prompt)},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(resp.Text())
}

func cmdStream(ctx context.Context, prompt string) {
	client := mustClient(ctx)

	stream, err := client.Messages.Stream(ctx, &kraube.MessageRequest{
		Model:     kraube.ModelSonnet4_6,
		MaxTokens: 4096,
		Messages:  []kraube.Message{kraube.UserMessage(prompt)},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer stream.Close()

	for stream.Next() {
	}
	if err := stream.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Stream error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(stream.Message().Text())
}

func mustClient(ctx context.Context) *kraube.Client {
	client, err := kraube.NewClientOAuth(ctx, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Not authenticated. Run: kraube login\n")
		os.Exit(1)
	}
	return client
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return fmt.Errorf("unsupported OS: %s — open manually: %s", runtime.GOOS, url)
	}
}
