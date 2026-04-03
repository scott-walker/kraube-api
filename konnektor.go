// Package konnektor is a Go SDK for Claude Code CLI.
//
// It spawns a claude CLI process and communicates via stdin/stdout JSON protocol,
// providing the same capabilities as the official Python/TypeScript SDKs.
//
// Usage:
//
//	session, err := konnektor.New(ctx, &konnektor.Options{
//	    Model: "claude-sonnet-4-5",
//	})
//
//	messages, err := session.Query("What is 2+2?")
//	for msg := range messages {
//	    if msg.Assistant != nil {
//	        fmt.Print(msg.Text())
//	    }
//	}
package konnektor

import "context"

// New creates a new Session with the given options.
// It spawns the claude CLI process and performs the initialize handshake.
func New(ctx context.Context, opts *Options) (*Session, error) {
	if opts == nil {
		opts = &Options{}
	}
	return newSession(ctx, opts)
}

// Query is a convenience function that creates a session, sends a single prompt,
// collects all messages, and closes the session.
func Query(ctx context.Context, prompt string, opts *Options) ([]*Message, error) {
	session, err := New(ctx, opts)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	ch, err := session.Query(prompt)
	if err != nil {
		return nil, err
	}

	var messages []*Message
	for msg := range ch {
		messages = append(messages, msg)
	}
	return messages, nil
}

// QueryText is a convenience function that sends a prompt and returns just the text result.
func QueryText(ctx context.Context, prompt string, opts *Options) (string, error) {
	messages, err := Query(ctx, prompt, opts)
	if err != nil {
		return "", err
	}

	// Look for result message first
	for _, msg := range messages {
		if msg.Result != nil && msg.Result.Result != nil {
			return *msg.Result.Result, nil
		}
	}

	// Fall back to concatenating assistant text
	var text string
	for _, msg := range messages {
		if t := msg.Text(); t != "" {
			text += t
		}
	}
	return text, nil
}
