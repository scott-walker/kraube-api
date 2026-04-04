# Tool Use

Kraube API supports custom tools and built-in tools (web search, code execution, text editor, bash).

## Custom Tools

```go
tool := kraube.Tool{
    Name:        "get_weather",
    Description: "Get weather for a city",
    InputSchema: &kraube.Schema{
        Type: "object",
        Properties: map[string]*kraube.Schema{
            "city": {Type: "string", Desc: "City name"},
        },
        Required: []string{"city"},
    },
}

resp, _ := client.Messages.Create(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelSonnet4_6,
    MaxTokens: 1024,
    Tools:     []kraube.Tool{tool},
    Messages:  []kraube.Message{kraube.UserMessage("Weather in Tokyo?")},
})
```

## Handling Tool Use

```go
if resp.HasToolUse() {
    for _, tu := range resp.ToolUses() {
        // Execute tool, get result
        result := executeMyTool(tu.Name, tu.Input)

        // Send result back
        next, _ := client.Messages.Create(ctx, &kraube.MessageRequest{
            Model:     kraube.ModelSonnet4_6,
            MaxTokens: 1024,
            Tools:     []kraube.Tool{tool},
            Messages: []kraube.Message{
                kraube.UserMessage("Weather in Tokyo?"),
                kraube.AssistantBlocks(resp.Content...),
                kraube.UserBlocks(kraube.ToolResultBlock(tu.ID, kraube.TextContent(result), false)),
            },
        })
        fmt.Println(next.Text())
    }
}
```

## Built-in Tools

```go
kraube.WebSearchTool()      // web search
kraube.CodeExecutionTool()  // sandboxed code execution
kraube.TextEditorTool()     // file editing (Claude Code style)
kraube.BashTool()           // bash command execution
```
