# Kraube API

Легковесный Go-шлюз для Anthropic Messages API через OAuth подписку.

## Что это

Kraube API предоставляет доступ к Claude (Opus, Sonnet, Haiku) через подписку Claude Pro/Max/Team — без API key, без Node.js, без лишних зависимостей.

Библиотека реплицирует HTTP-протокол Claude Code CLI: billing header, metadata, beta headers, Chrome TLS fingerprint. Всё что нужно — OAuth токен.

## Концепция

- **OAuth-only.** Единственный способ аутентификации — OAuth Bearer token через подписку claude.ai. Никаких API keys.
- **Stateless.** Клиент не привязан к файловой системе. Токен приходит через `TokenProvider` — интерфейс с одним методом. Откуда именно (файл, env, Vault, база, callback) — решает пользователь.
- **Lightweight.** Минимальная обёртка над HTTP. Не фреймворк, не SDK с абстракциями — просто типизированный HTTP-клиент.
- **Reverse-engineered.** Протокол взят из реверса Claude Code CLI. Если API не задокументирован — источник истины это бинарник CLI.

## Установка

```bash
go get github.com/scott-walker/kraube-api
```

## Гайд по использованию

### 1. Аутентификация

Первый шаг — получить OAuth токен. Kraube API предоставляет CLI для этого:

```bash
go build -o kraube ./cmd/kraube/
kraube login
```

Откроется браузер, после авторизации credentials сохранятся в `~/.config/kraube/credentials.json`.

### 2. Создание клиента

Единый конструктор `NewClient` с functional options. Токен можно передать любым способом:

```go
ctx := context.Background()

// Вариант 1: из файла credentials (после kraube login)
client, err := kraube.NewClient(ctx, kraube.WithCredentialsFile(""))

// Вариант 2: из access token напрямую
client, err := kraube.NewClient(ctx, kraube.WithAccessToken("eyJhbGci..."))

// Вариант 3: из переменной окружения
client, err := kraube.NewClient(ctx, kraube.WithEnvToken("KRAUBE_TOKEN"))

// Вариант 4: из готовых credentials (с авто-рефрешем по refresh_token)
client, err := kraube.NewClient(ctx, kraube.WithCredentials(&kraube.Credentials{
    AccessToken:  "eyJhbGci...",
    RefreshToken: "dGhpcyBp...",
    ExpiresAt:    1712345678000, // unix ms
}))

// Вариант 5: свой провайдер (Vault, Redis, DB — что угодно)
client, err := kraube.NewClient(ctx, kraube.WithTokenProvider(myProvider))
```

Дополнительные опции:

```go
client, err := kraube.NewClient(ctx,
    kraube.WithCredentialsFile(""),
    kraube.WithHTTPClient(&http.Client{Timeout: 30 * time.Second}),
    kraube.WithBaseURL("https://custom.endpoint"),
    kraube.WithoutProfile(), // пропустить загрузку профиля
)
```

### 3. Свой TokenProvider

Реализуй один метод — и клиент будет брать токен откуда угодно:

```go
type TokenProvider interface {
    Token(ctx context.Context) (*Credentials, error)
}
```

Пример — провайдер из Vault:

```go
type VaultProvider struct {
    client *vault.Client
    path   string
}

func (v *VaultProvider) Token(ctx context.Context) (*kraube.Credentials, error) {
    secret, err := v.client.KVv2("secret").Get(ctx, v.path)
    if err != nil {
        return nil, err
    }
    return &kraube.Credentials{
        AccessToken:  secret.Data["access_token"].(string),
        RefreshToken: secret.Data["refresh_token"].(string),
        ExpiresAt:    secret.Data["expires_at"].(int64),
    }, nil
}

client, err := kraube.NewClient(ctx, kraube.WithTokenProvider(&VaultProvider{
    client: vaultClient,
    path:   "kraube/oauth",
}))
```

Встроенные провайдеры:

| Option | Provider | Рефреш |
|--------|----------|--------|
| `WithAccessToken(token)` | `StaticTokenProvider` | Нет |
| `WithCredentials(creds)` | `CredentialsProvider` | Да, если есть refresh_token |
| `WithCredentialsFile(path)` | `FileTokenProvider` | Да, сохраняет обратно в файл |
| `WithEnvToken(envVar)` | `EnvTokenProvider` | Нет, читает env при каждом вызове |
| `WithTokenProvider(p)` | Любой | Зависит от реализации |

### 4. Простой запрос

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

### 5. Streaming

```go
stream, err := client.Messages.Stream(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelSonnet4_6,
    MaxTokens: 1024,
    Messages:  []kraube.Message{kraube.UserMessage("Напиши стихотворение")},
})
if err != nil {
    log.Fatal(err)
}
defer stream.Close()

for stream.Next() {
    // события обрабатываются автоматически
}
if err := stream.Err(); err != nil {
    log.Fatal(err)
}

fmt.Println(stream.Message().Text())
```

### 6. System prompt

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
            CacheControl: &kraube.CacheControl{Type: "ephemeral"},
        },
    ),
    Messages: []kraube.Message{kraube.UserMessage("Привет")},
})
```

### 7. Tool Use

```go
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

resp, _ := client.Messages.Create(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelSonnet4_6,
    MaxTokens: 1024,
    Tools:     []kraube.Tool{weatherTool},
    Messages:  []kraube.Message{kraube.UserMessage("Какая погода в Москве?")},
})

// Обработка tool use
if resp.HasToolUse() {
    for _, tu := range resp.ToolUses() {
        fmt.Printf("Tool: %s, Input: %s\n", tu.Name, string(tu.Input))

        // Вернуть результат инструмента
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

Built-in tools:

```go
tools := []kraube.Tool{kraube.WebSearchTool()}       // веб-поиск
tools := []kraube.Tool{kraube.CodeExecutionTool()}    // выполнение кода
tools := []kraube.Tool{kraube.TextEditorTool(), kraube.BashTool()} // как в Claude Code
```

### 8. Extended Thinking

```go
resp, _ := client.Messages.Create(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelOpus4_6,
    MaxTokens: 8192,
    Thinking:  kraube.ThinkingEnabled(4096),
    Messages:  []kraube.Message{kraube.UserMessage("Реши сложную задачу...")},
})

for _, b := range resp.ThinkingBlocks() {
    fmt.Println("Thinking:", b.Thinking)
}
fmt.Println("Answer:", resp.Text())

// Или адаптивный режим
kraube.ThinkingAdaptive()
```

### 9. Vision

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

// Или base64
kraube.ImageBase64Block("image/png", base64Data)
```

### 10. Structured Output

```go
resp, _ := client.Messages.Create(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelSonnet4_6,
    MaxTokens: 1024,
    OutputConfig: &kraube.OutputConfig{
        Format: &kraube.OutputFormat{
            Type:   "json_schema",
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
    Messages: []kraube.Message{kraube.UserMessage("Сгенерируй профиль пользователя")},
})
```

### 11. Подсчёт токенов

```go
count, _ := client.Messages.CountTokens(ctx, &kraube.CountTokensRequest{
    Model:    kraube.ModelSonnet4_6,
    Messages: []kraube.Message{kraube.UserMessage("Hello!")},
})
fmt.Printf("Input tokens: %d\n", count.InputTokens)
```

### 12. Обработка ошибок

```go
resp, err := client.Messages.Create(ctx, req)
if err != nil {
    var apiErr *kraube.APIError
    if errors.As(err, &apiErr) {
        switch {
        case apiErr.IsRateLimit():
            // подождать и повторить
        case apiErr.IsOverloaded():
            // сервер перегружен
        case apiErr.IsAuthentication():
            // невалидный токен
        default:
            log.Fatal(apiErr.Detail.Message)
        }
    }
    log.Fatal(err)
}
```

## CLI

```bash
go build -o kraube ./cmd/kraube/

kraube login                  # OAuth через браузер
kraube "Что такое Go?"        # запрос
kraube stream "Расскажи..."   # стриминг
kraube usage                  # лимиты подписки
kraube --debug "prompt"       # с отладочным логированием
```

## Документация

- [Архитектура](docs/architecture.md) — структура проекта
- [Принципы](docs/principles.md) — почему так, а не иначе
- [Протокол](docs/protocol.md) — HTTP-протокол Claude Code CLI
- [Покрытие API](docs/api-coverage.md) — что реализовано
