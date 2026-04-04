// Package kraube is a pure Go alternative to the Claude Code CLI for
// interacting with the Anthropic Messages API via OAuth subscription
// (Pro/Max/Team plans).
//
// It reverse-engineers the Claude Code CLI's HTTP protocol — including the
// billing header, metadata.user_id, model-specific beta headers, and Chrome
// TLS fingerprinting — to provide full API access without requiring a paid
// API key.
//
// Features:
//   - OAuth-first authentication (login, refresh, profile)
//   - Automatic billing header injection for subscription access
//   - Model-aware beta header selection (Opus vs Sonnet/Haiku)
//   - Chrome TLS fingerprint via uTLS (bypasses Cloudflare blocking)
//   - Streaming and non-streaming message requests
//   - Tool use, extended thinking, caching, vision, documents
//   - Rate limit tracking and persistence
//
// Quick start:
//
//	client, err := kraube.NewClientOAuth(ctx, "")
//	resp, err := client.Messages.Create(ctx, &kraube.MessageRequest{
//	    Model:     kraube.ModelSonnet4_6,
//	    MaxTokens: 1024,
//	    Messages:  []kraube.Message{kraube.UserMessage("Hello!")},
//	})
package kraube
