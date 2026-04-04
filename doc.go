// Package kraube is a pure Go client for the Anthropic API.
//
// It provides complete, typed access to the Messages API including
// streaming, tool use, extended thinking, caching, vision, and documents.
//
// No external dependencies — stdlib only.
//
//	client := kraube.NewClient("your-api-key")
//
//	resp, err := client.Messages.Create(ctx, &kraube.MessageRequest{
//	    Model:     kraube.ModelSonnet4_6,
//	    MaxTokens: 1024,
//	    Messages: []kraube.Message{
//	        kraube.UserMessage("Hello, Claude!"),
//	    },
//	})
package kraube
