# Использование Kraube

## Создание клиента

```go
// Из env ANTHROPIC_API_KEY
client := kraube.NewClient("")

// Явно
client := kraube.NewClient("sk-ant-...")

// С кастомным HTTP-клиентом
client := kraube.NewClient("")
client.HTTPClient = &http.Client{Timeout: 30 * time.Second}

// С бета-фичами
client.Betas = []string{"some-beta-2025"}
```

## Простой запрос

```go
resp, err := client.Messages.Create(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelSonnet4_6,
    MaxTokens: 1024,
    Messages: []kraube.Message{
        kraube.UserMessage("Что такое Go?"),
    },
})
if err != nil {
    log.Fatal(err)
}
fmt.Println(resp.Text())
```

## Streaming

```go
stream, err := client.Messages.Stream(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelSonnet4_6,
    MaxTokens: 1024,
    Messages: []kraube.Message{
        kraube.UserMessage("Напиши стихотворение"),
    },
})
if err != nil {
    log.Fatal(err)
}
defer stream.Close()

for stream.Next() {
    // Поток событий — финальное сообщение собирается автоматически
}
if err := stream.Err(); err != nil {
    log.Fatal(err)
}

// Получить собранное сообщение
msg := stream.Message()
fmt.Println(msg.Text())
```

## System prompt

```go
// Простой текст
resp, _ := client.Messages.Create(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelSonnet4_6,
    MaxTokens: 1024,
    System:    kraube.SystemText("Ты полезный ассистент."),
    Messages:  []kraube.Message{kraube.UserMessage("Привет")},
})

// С кешированием
resp, _ := client.Messages.Create(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelSonnet4_6,
    MaxTokens: 1024,
    System: kraube.SystemBlocks(
        kraube.SystemBlock{
            Type: "text",
            Text: "Очень длинный системный промпт...",
            CacheControl: &kraube.CacheControl{Type: "ephemeral", TTL: "1h"},
        },
    ),
    Messages: []kraube.Message{kraube.UserMessage("Привет")},
})
```

## Tool Use

```go
// Определяем инструмент
weatherTool := kraube.Tool{
    Name:        "get_weather",
    Description: "Получить погоду в городе",
    InputSchema: &kraube.Schema{
        Type: "object",
        Properties: map[string]*kraube.Schema{
            "city": {Type: "string", Desc: "Название города"},
        },
        Required: []string{"city"},
    },
}

// Отправляем запрос
resp, _ := client.Messages.Create(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelSonnet4_6,
    MaxTokens: 1024,
    Tools:     []kraube.Tool{weatherTool},
    Messages:  []kraube.Message{kraube.UserMessage("Какая погода в Москве?")},
})

// Проверяем, хочет ли модель вызвать инструмент
if resp.HasToolUse() {
    for _, tu := range resp.ToolUses() {
        fmt.Printf("Tool: %s, Input: %s\n", tu.Name, string(tu.Input))

        // Выполняем инструмент и отправляем результат
        result, _ := client.Messages.Create(ctx, &kraube.MessageRequest{
            Model:     kraube.ModelSonnet4_6,
            MaxTokens: 1024,
            Tools:     []kraube.Tool{weatherTool},
            Messages: []kraube.Message{
                kraube.UserMessage("Какая погода в Москве?"),
                kraube.AssistantBlocks(resp.Content...),
                kraube.UserBlocks(kraube.ToolResultBlock(
                    tu.ID,
                    kraube.TextContent(`{"temp": 15, "condition": "cloudy"}`),
                    false,
                )),
            },
        })
        fmt.Println(result.Text())
    }
}
```

## Extended Thinking

```go
resp, _ := client.Messages.Create(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelOpus4_6,
    MaxTokens: 8192,
    Thinking:  kraube.ThinkingEnabled(4096),
    Messages:  []kraube.Message{kraube.UserMessage("Реши сложную задачу...")},
})

// Посмотреть процесс мышления
for _, b := range resp.ThinkingBlocks() {
    fmt.Println("Thinking:", b.Thinking)
}
fmt.Println("Answer:", resp.Text())
```

## Vision (изображения)

```go
resp, _ := client.Messages.Create(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelSonnet4_6,
    MaxTokens: 1024,
    Messages: []kraube.Message{
        kraube.UserBlocks(
            kraube.TextBlock("Что на этом изображении?"),
            kraube.ImageURLBlock("https://example.com/photo.jpg"),
        ),
    },
})
```

## Подсчёт токенов

```go
count, _ := client.Messages.CountTokens(ctx, &kraube.CountTokensRequest{
    Model: kraube.ModelSonnet4_6,
    Messages: []kraube.Message{
        kraube.UserMessage("Hello!"),
    },
})
fmt.Printf("Input tokens: %d\n", count.InputTokens)
```

## Обработка ошибок

```go
resp, err := client.Messages.Create(ctx, req)
if err != nil {
    var apiErr *kraube.APIError
    if errors.As(err, &apiErr) {
        switch {
        case apiErr.IsRateLimit():
            // Подождать и повторить
        case apiErr.IsOverloaded():
            // Сервер перегружен
        case apiErr.IsAuthentication():
            // Неверный API key
        default:
            log.Fatal(apiErr.Detail.Message)
        }
    }
    log.Fatal(err)
}
```

## Structured Output (JSON Schema)

```go
resp, _ := client.Messages.Create(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelSonnet4_6,
    MaxTokens: 1024,
    OutputConfig: &kraube.OutputConfig{
        Format: &kraube.OutputFormat{
            Type:   "json_schema",
            Schema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"age":{"type":"number"}},"required":["name","age"]}`),
        },
    },
    Messages: []kraube.Message{kraube.UserMessage("Сгенерируй профиль пользователя")},
})
```
