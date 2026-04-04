# kraube

Легковесный Go-шлюз для Anthropic Messages API через OAuth подписку.

## Что это

Kraube предоставляет доступ к Claude (Opus, Sonnet, Haiku) через подписку Claude Pro/Max/Team — без API key, без Node.js, без лишних зависимостей.

Библиотека реплицирует HTTP-протокол Claude Code CLI: billing header, metadata, beta headers, Chrome TLS fingerprint. Всё что нужно — OAuth токен.

## Концепция

- **OAuth-only.** Единственный способ аутентификации — OAuth Bearer token через подписку claude.ai. Никаких API keys.
- **Stateless.** Клиент не привязан к файловой системе. Токен приходит через `TokenProvider` — интерфейс с одним методом. Откуда именно (файл, env, Vault, база, callback) — решает пользователь.
- **Lightweight.** Минимальная обёртка над HTTP. Не фреймворк, не SDK с абстракциями — просто типизированный HTTP-клиент.
- **Reverse-engineered.** Протокол взят из реверса Claude Code CLI. Если API не задокументирован — источник истины это бинарник CLI.

## Установка

```bash
go get github.com/kraube-go/kraube
```

## Quick start

```go
// Из сохранённых credentials
client, err := kraube.NewClient(ctx, kraube.WithCredentialsFile(""))

// Из токена напрямую
client, err := kraube.NewClient(ctx, kraube.WithAccessToken(token))

// Из env variable
client, err := kraube.NewClient(ctx, kraube.WithEnvToken("KRAUBE_TOKEN"))

// Свой провайдер
client, err := kraube.NewClient(ctx, kraube.WithTokenProvider(myProvider))

// Запрос
resp, err := client.Messages.Create(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelSonnet4_6,
    MaxTokens: 1024,
    Messages:  []kraube.Message{kraube.UserMessage("Hello!")},
})
fmt.Println(resp.Text())
```

## TokenProvider

Единственный интерфейс для аутентификации:

```go
type TokenProvider interface {
    Token(ctx context.Context) (*Credentials, error)
}
```

Встроенные реализации:

| Provider | Описание |
|----------|----------|
| `WithAccessToken(token)` | Статичный токен, без рефреша |
| `WithCredentials(creds)` | Credentials с авто-рефрешем |
| `WithCredentialsFile(path)` | Из JSON файла, рефреш сохраняется обратно |
| `WithEnvToken(envVar)` | Из переменной окружения |
| `WithTokenProvider(p)` | Любая своя реализация |

## Возможности

- Streaming и non-streaming запросы
- Tool use (custom tools, web_search, code_execution, text_editor, bash)
- Extended thinking
- Vision (images), documents
- System prompts с кешированием
- Structured output (JSON Schema)
- Подсчёт токенов
- Rate limit tracking

## CLI

```bash
go build -o kraube ./cmd/kraube/

kraube login                  # OAuth через браузер
kraube "Что такое Go?"        # запрос
kraube stream "Расскажи..."   # стриминг
kraube usage                  # лимиты подписки
```

## Документация

- [Использование](docs/usage.md) — примеры кода
- [Архитектура](docs/architecture.md) — структура проекта
- [Принципы](docs/principles.md) — почему так, а не иначе
- [Протокол](docs/protocol.md) — HTTP-протокол Claude Code CLI
- [Покрытие API](docs/api-coverage.md) — что реализовано
