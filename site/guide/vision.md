# Vision & Documents

## Images

### From URL

```go
resp, _ := client.Messages.Create(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelSonnet4_6,
    MaxTokens: 1024,
    Messages: []kraube.Message{
        kraube.UserBlocks(
            kraube.TextBlock("What's in this image?"),
            kraube.ImageURLBlock("https://example.com/photo.jpg"),
        ),
    },
})
```

### From Base64

```go
kraube.ImageBase64Block("image/png", base64Data)
```

## Documents

PDF and other document types are supported through document content blocks. See the [API Coverage](../reference/api) for supported formats.
