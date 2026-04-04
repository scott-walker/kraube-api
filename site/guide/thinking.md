# Extended Thinking

Extended thinking lets Claude show its reasoning process before answering.

## Enabled Mode

```go
resp, _ := client.Messages.Create(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelOpus4_6,
    MaxTokens: 8192,
    Thinking:  kraube.ThinkingEnabled(4096), // budget_tokens
    Messages:  []kraube.Message{kraube.UserMessage("Solve this problem...")},
})

for _, b := range resp.ThinkingBlocks() {
    fmt.Println("Thinking:", b.Thinking)
}
fmt.Println("Answer:", resp.Text())
```

## Adaptive Mode

Let Claude decide whether to think:

```go
Thinking: kraube.ThinkingAdaptive()
```

## Disabled

```go
Thinking: kraube.ThinkingDisabled()
```
