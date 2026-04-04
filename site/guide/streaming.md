# Streaming

Kraube API supports SSE streaming with automatic message assembly and real-time event access.

## Basic Streaming

```go
stream, err := client.Messages.Stream(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelSonnet4_6,
    MaxTokens: 1024,
    Messages:  []kraube.Message{kraube.UserMessage("Tell me a story")},
})
if err != nil {
    log.Fatal(err)
}
defer stream.Close()

for stream.Next() {
    // Events are processed and accumulated automatically
}
if err := stream.Err(); err != nil {
    log.Fatal(err)
}

// Get the fully assembled message
msg := stream.Message()
fmt.Println(msg.Text())
```

## Real-Time Events

Each `Next()` call exposes the current SSE event via `Event()`. Use a type switch to react in real time — text appearing character by character, tool calls as they start, thinking indicators.

### Text streaming

```go
for stream.Next() {
    if evt, ok := stream.Event().(*kraube.ContentBlockDeltaEvent); ok {
        if evt.Delta.Type == "text_delta" {
            fmt.Print(evt.Delta.Text)
        }
    }
}
fmt.Println()
```

### Full event handling

```go
for stream.Next() {
    switch evt := stream.Event().(type) {
    case *kraube.ContentBlockStartEvent:
        switch evt.ContentBlock.Type {
        case "thinking":
            fmt.Print("Thinking...")
        case "tool_use":
            fmt.Printf("Using tool: %s\n", evt.ContentBlock.Name)
        }

    case *kraube.ContentBlockDeltaEvent:
        switch evt.Delta.Type {
        case "text_delta":
            fmt.Print(evt.Delta.Text)
        case "input_json_delta":
            // tool input arriving chunk by chunk
        }

    case *kraube.ContentBlockStopEvent:
        block := stream.CurrentBlock()
        if block != nil && block.Type == "tool_use" {
            fmt.Printf("Tool %s called with: %s\n", block.Name, string(block.Input))
        }

    case *kraube.MessageDeltaEvent:
        fmt.Printf("\n[%s, %d tokens]\n", evt.Delta.StopReason, evt.Usage.OutputTokens)
    }
}
```

## StreamReader Methods

| Method | Returns | Description |
|--------|---------|-------------|
| `Next()` | `bool` | Advance to next event. False when done. |
| `Event()` | `StreamEvent` | Current typed SSE event (type switch). |
| `EventType()` | `string` | Event type string (`"content_block_delta"`, etc.). |
| `CurrentBlock()` | `*ContentBlock` | Content block being built (accumulated state). |
| `Message()` | `*MessageResponse` | Final assembled message after stream ends. |
| `Err()` | `error` | Any error during streaming. |
| `Close()` | `error` | Release the response body. |

## Event Types

| Type | Struct | When |
|------|--------|------|
| `message_start` | `MessageStartEvent` | Stream begins, initial metadata |
| `content_block_start` | `ContentBlockStartEvent` | New block (text, tool_use, thinking) |
| `content_block_delta` | `ContentBlockDeltaEvent` | Incremental content (text, JSON, thinking) |
| `content_block_stop` | `ContentBlockStopEvent` | Block complete |
| `message_delta` | `MessageDeltaEvent` | Stop reason, final usage |
| `message_stop` | `MessageStopEvent` | Stream ends (Next returns false) |
| `ping` | `PingEvent` | Keepalive |
| `error` | `ErrorEvent` | Server error |

## How it Works

`StreamReader` reads SSE events and accumulates them into a final `MessageResponse`:

- `message_start` — initializes the message
- `content_block_start` — starts a new content block
- `content_block_delta` — appends text, tool input, or thinking deltas
- `message_delta` — sets stop reason and final usage
- `message_stop` — streaming complete

After `stream.Next()` returns `false`, `stream.Message()` contains the complete response — identical to what `Messages.Create()` would return.

## Error Handling

```go
for stream.Next() {}

if err := stream.Err(); err != nil {
    var apiErr *kraube.APIError
    if errors.As(err, &apiErr) {
        if apiErr.IsRateLimit() {
            // handle rate limit
        }
    }
}
```
