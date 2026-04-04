# Streaming

Kraube API supports SSE streaming with automatic message assembly.

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
    // Events are processed automatically
}
if err := stream.Err(); err != nil {
    log.Fatal(err)
}

// Get the fully assembled message
msg := stream.Message()
fmt.Println(msg.Text())
```

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
