# Structured Output

Force Claude to respond with a specific JSON schema:

```go
resp, _ := client.Messages.Create(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelSonnet4_6,
    MaxTokens: 1024,
    OutputConfig: &kraube.OutputConfig{
        Format: &kraube.OutputFormat{
            Type: "json_schema",
            Schema: json.RawMessage(`{
                "type": "object",
                "properties": {
                    "name": {"type": "string"},
                    "age":  {"type": "number"}
                },
                "required": ["name", "age"]
            }`),
        },
    },
    Messages: []kraube.Message{kraube.UserMessage("Generate a user profile")},
})
```

The response `resp.Text()` will contain valid JSON matching the schema.
