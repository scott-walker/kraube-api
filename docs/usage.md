# Использование Kraube API

## CLI

```bash
# Сборка
go build -o kraube ./cmd/kraube/

# Логин через подписку (Claude Pro/Max/Team)
kraube login

# Запрос
kraube "Что такое Go?"

# Стриминг
kraube stream "Напиши стихотворение"

# Лимиты подписки
kraube usage

# Локальный демон: прокси к Messages API + фоновый рефреш токена
kraube serve
```

### Демон `kraube serve`

Постоянно живой локальный HTTP-шлюз: проксирует `POST /v1/messages` и
`POST /v1/messages/count_tokens` (со всей inject-логикой OAuth — identity
preamble, billing header, metadata, beta headers; стриминг отдаётся сырыми
SSE-байтами с flush на каждый чанк), плюс `GET /healthz` и `GET /usage`.
Фоновая горутина рефрешит access-токен за `--refresh-margin` (по умолчанию
10 минут) до истечения, так что демон — единственный владелец
`credentials.json`, и одноразовый refresh-токен ротируется ровно в одном
месте.

```bash
kraube serve --listen 127.0.0.1:8787 --refresh-margin 10m
# не-loopback адрес требует ключ (--auth-key или KRAUBE_SERVE_KEY):
kraube serve --listen 0.0.0.0:8787 --auth-key s3cret
```

systemd-юнит с инструкцией — `deploy/kraube-serve.service`.
Из библиотеки то же самое доступно через `kraube.NewServer(client, cfg)`,
а примитив проактивного рефреша — через `client.EnsureFresh(ctx, margin)`
и `client.AccessExpiry()`.

## Создание клиента

```go
ctx := context.Background()

// Из сохранённых credentials (по умолчанию ~/.config/kraube/credentials.json;
// переопределяется KRAUBE_CREDENTIALS_PATH). Безопасно для параллельных процессов.
client, err := kraube.NewClient(ctx, kraube.WithTokenFile(""))

// Из refresh-токена напрямую (в памяти, ротация не сохраняется)
client, err := kraube.NewClient(ctx, kraube.WithToken(refreshToken))

// Из env variable
client, err := kraube.NewClient(ctx, kraube.WithEnvToken("KRAUBE_TOKEN"))

// Свой провайдер
client, err := kraube.NewClient(ctx, kraube.WithTokenProvider(myProvider))
```

### Дополнительные опции

```go
client, err := kraube.NewClient(ctx,
    kraube.WithToken(token),
    kraube.WithHTTPClient(&http.Client{Timeout: 30 * time.Second}),
    kraube.WithBaseURL("https://custom.endpoint"),
    kraube.WithoutProfile(), // пропустить загрузку профиля
)
```

### Прокси

Весь трафик инстанса Client'а — `/v1/messages`, начальный profile-fetch и OAuth token refresh — идёт через один и тот же транспорт. Достаточно одного `WithProxy(...)` при создании клиента. Chrome TLS fingerprint сохраняется: uTLS handshake идёт поверх туннеля напрямую до `api.anthropic.com` / `platform.claude.com`.

```go
// Явный прокси. Логин/пароль в URL — это Basic proxy auth.
client, err := kraube.NewClient(ctx,
    kraube.WithTokenFile(""),
    kraube.WithProxy("http://user:pass@proxy.example.com:8080"),
)

// SOCKS5
client, err := kraube.NewClient(ctx,
    kraube.WithTokenFile(""),
    kraube.WithProxy("socks5://127.0.0.1:1080"),
)

// Без опции — клиент сам подхватит HTTPS_PROXY / ALL_PROXY из окружения.
client, err := kraube.NewClient(ctx, kraube.WithTokenFile(""))

// Принудительно без прокси, даже если в env что-то задано.
client, err := kraube.NewClient(ctx,
    kraube.WithTokenFile(""),
    kraube.WithProxy(""),
)
```

Поддерживаемые схемы: `http`, `https`, `socks5`, `socks5h`. Любая другая схема — явная ошибка (чтобы не ходить «мимо прокси» по тихому). `host:port` без схемы интерпретируется как `http://`.

Для **standalone** auth-вызовов без инстанса Client'а (`Login`, `LoginManual`, top-level `FetchProfile`) используется отдельный пакетный HTTP-клиент. Чтобы направить и их через прокси:

```go
hc, _ := kraube.NewProxiedHTTPClient("http://proxy:8080")
kraube.SetAuthHTTPClient(hc)

creds, _ := kraube.LoginManual(ctx, readCode)
```

`NewProxiedHTTPClient("")` вернёт клиент, который читает `HTTPS_PROXY` / `ALL_PROXY` из env. `SetAuthHTTPClient(nil)` возвращает дефолт. Для обычного использования `kraube.NewClient(..., WithProxy(...))` этот вызов не нужен — refresh покрывается `WithProxy` автоматически.

В CLI всё то же самое доступно через флаг `--proxy URL`, который применяется к любой подкоманде, включая `kraube login`:

```bash
kraube --proxy http://user:pass@proxy.example.com:8080 "hi"
kraube --proxy socks5://127.0.0.1:1080 stream "расскажи историю"
HTTPS_PROXY=http://proxy:8080 kraube "hi"
```

### Диагностика через --debug

Флаг `--debug` (или `KRAUBE_DEBUG=1`, или `kraube.EnableDevLog()` программно) включает подробные stderr-логи. При ошибке клиент эмитит одну строку `api: error response`, содержащую всё необходимое для разбора:

- `method`, полный `url`, `status`, `elapsed`
- `local_addr`, `remote_addr`, `proxy` — какой именно egress использовался
- `request_headers` (с редактом `Authorization` / `Cookie`)
- `request_body`, `response_headers`, `response_body` (каждое поле до 8 КБ)

Для сетевых ошибок (когда соединение вообще не установилось) эмитится аналогичный `api: request failed` без блока ответа. Успешные запросы тоже логируют `local_addr` / `remote_addr` / `proxy` — удобно ловить ситуации, когда прокси «работает не с того IP».

`APIError.Error()` всегда печатает HTTP-статус: `HTTP 403 forbidden: Request not allowed` — без флагов и без type assertion.

### Свой TokenProvider

```go
type VaultProvider struct { /* ... */ }

func (v *VaultProvider) Token(ctx context.Context) (string, error) {
    secret, err := v.vault.Read(ctx, "secret/kraube")
    if err != nil {
        return "", err
    }
    return secret["token"], nil
}

client, err := kraube.NewClient(ctx, kraube.WithTokenProvider(&VaultProvider{...}))
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

// Real-time текст посимвольно
for stream.Next() {
    if evt, ok := stream.Event().(*kraube.ContentBlockDeltaEvent); ok {
        if evt.Delta.Type == "text_delta" {
            fmt.Print(evt.Delta.Text)
        }
    }
}
if err := stream.Err(); err != nil {
    log.Fatal(err)
}
fmt.Println()

// Или просто финальное сообщение
msg := stream.Message()
fmt.Println(msg.Text())
```

### Real-time events

Каждый `Next()` делает текущее SSE-событие доступным через `Event()`:

```go
for stream.Next() {
    switch evt := stream.Event().(type) {
    case *kraube.ContentBlockStartEvent:
        if evt.ContentBlock.Type == "tool_use" {
            fmt.Printf("Вызов: %s\n", evt.ContentBlock.Name)
        }
    case *kraube.ContentBlockDeltaEvent:
        if evt.Delta.Type == "text_delta" {
            fmt.Print(evt.Delta.Text)
        }
    case *kraube.ContentBlockStopEvent:
        if b := stream.CurrentBlock(); b != nil && b.Type == "tool_use" {
            fmt.Printf("Input: %s\n", string(b.Input))
        }
    case *kraube.MessageDeltaEvent:
        fmt.Printf("\n[%s, %d tokens]\n", evt.Delta.StopReason, evt.Usage.OutputTokens)
    }
}
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
            CacheControl: &kraube.CacheControl{Type: "ephemeral"},
        },
    ),
    Messages: []kraube.Message{kraube.UserMessage("Привет")},
})
```

## Tool Use

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

if resp.HasToolUse() {
    for _, tu := range resp.ToolUses() {
        fmt.Printf("Tool: %s, Input: %s\n", tu.Name, string(tu.Input))

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

## Built-in Tools

```go
// Web search
tools := []kraube.Tool{kraube.WebSearchTool()}

// Code execution
tools := []kraube.Tool{kraube.CodeExecutionTool()}

// Text editor + Bash (как в Claude Code)
tools := []kraube.Tool{kraube.TextEditorTool(), kraube.BashTool()}
```

## Extended Thinking

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

// Адаптивный режим
kraube.ThinkingAdaptive()

// Выключить
kraube.ThinkingDisabled()
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

// Или base64
kraube.ImageBase64Block("image/png", base64Data)
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
            // Невалидный токен
        default:
            log.Fatal(apiErr.Detail.Message)
        }
    }
    log.Fatal(err)
}
```
