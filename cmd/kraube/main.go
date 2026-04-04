package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kraube-go/kraube"
)

func main() {
	// Dev logging: kraube --debug ... or KRAUBE_DEBUG=1
	if hasFlag("--debug") || os.Getenv("KRAUBE_DEBUG") == "1" {
		kraube.EnableDevLog()
	}

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  kraube login              — authenticate via browser")
		fmt.Fprintln(os.Stderr, "  kraube usage               — show plan usage limits")
		fmt.Fprintln(os.Stderr, "  kraube \"your prompt\"       — send a message")
		fmt.Fprintln(os.Stderr, "  kraube stream \"prompt\"     — stream response")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fmt.Fprintln(os.Stderr, "  --debug                    — verbose logging (or KRAUBE_DEBUG=1)")
		os.Exit(1)
	}

	ctx := context.Background()

	switch os.Args[1] {
	case "login":
		cmdLogin(ctx)
	case "usage":
		cmdUsage(ctx)
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
	// Manual OAuth flow: print URL, user pastes code
	creds, err := kraube.LoginManual(ctx, func(authURL string) (string, error) {
		fmt.Println("Open this URL in your browser:")
		fmt.Println()
		fmt.Println("  " + authURL)
		fmt.Println()
		fmt.Print("Paste the code here: ")
		reader := bufio.NewReader(os.Stdin)
		code, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return code, nil
	})
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

func cmdUsage(ctx context.Context) {
	client := mustClient(ctx)

	// Try cached data first.
	rl, _ := kraube.LoadRateLimit(kraube.DefaultRateLimitPath())

	// If no cache or stale (>5 min), make a cheap API call to refresh.
	// Rate limit headers only come from /v1/messages, not /count_tokens.
	if rl == nil || time.Since(rl.UpdatedAt) > 5*time.Minute {
		_, _ = client.Messages.Create(ctx, &kraube.MessageRequest{
			Model:     kraube.ModelSonnet4_6,
			MaxTokens: 1,
			Messages:  []kraube.Message{kraube.UserMessage("hi")},
		})
		// Reload from client (set by do()) or from disk (set by parseAPIError).
		if client.RateLimit != nil && client.RateLimit.HasData() {
			rl = client.RateLimit
		}
	}

	if rl == nil || !rl.HasData() {
		fmt.Println("No usage data available.")
		fmt.Println("Usage data appears after a successful API call (subscription plans only).")
		return
	}

	// Show staleness
	ago := time.Since(rl.UpdatedAt).Truncate(time.Second)
	fmt.Printf("Plan Usage (updated %s ago)\n", ago)

	fmt.Println()

	if rl.FiveHour != nil {
		printUsageBar("Session (5h)", rl.FiveHour)
	}
	if rl.SevenDay != nil {
		printUsageBar("Weekly  (7d)", rl.SevenDay)
	}

	if rl.FiveHour == nil && rl.SevenDay == nil {
		fmt.Println("No usage data available. Usage is only available for subscription plans.")
		return
	}

	if rl.Status == "rejected" {
		fmt.Println()
		claim := rl.Claim
		if claim == "" {
			claim = "usage limit"
		}
		fmt.Printf("You've hit your %s.\n", formatClaim(claim))
	}
}

func printUsageBar(label string, w *kraube.RateLimitWindow) {
	pct := w.UsedPercent()
	barWidth := 30
	filled := int(pct / 100 * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	reset := formatDuration(w.TimeUntilReset())
	fmt.Printf("  %s  [%s] %5.1f%%  resets in %s\n", label, bar, pct, reset)
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "now"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func formatClaim(claim string) string {
	switch claim {
	case "five_hour":
		return "session limit"
	case "seven_day":
		return "weekly limit"
	case "seven_day_opus":
		return "Opus limit"
	case "seven_day_sonnet":
		return "Sonnet limit"
	case "overage":
		return "extra usage limit"
	default:
		return claim
	}
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
	client, err := kraube.NewClient(ctx, kraube.WithCredentialsFile(""))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Not authenticated. Run: kraube login\n")
		os.Exit(1)
	}
	return client
}

func hasFlag(flag string) bool {
	for i, arg := range os.Args {
		if arg == flag {
			// Remove flag from args so it doesn't interfere with commands
			os.Args = append(os.Args[:i], os.Args[i+1:]...)
			return true
		}
	}
	return false
}

