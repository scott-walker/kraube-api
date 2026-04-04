# Архитектура Kraube API

## Принцип

Легковесный Go-шлюз для Anthropic Messages API через OAuth подписку.
Минимум зависимостей, stateless дизайн.

## Слои

```
┌─────────────────────────────────┐
│  Пользовательский код / CLI     │
├─────────────────────────────────┤
│  kraube.Client                  │  ← точка входа
│  ├── Messages.Create()          │  ← синхронный запрос
│  ├── Messages.Stream()          │  ← SSE streaming
│  └── Messages.CountTokens()     │  ← подсчёт токенов
├─────────────────────────────────┤
│  TokenProvider                  │  ← интерфейс: откуда токен
│  ├── StaticTokenProvider        │  ← фиксированный access token
│  ├── CredentialsProvider        │  ← credentials + авто-рефреш
│  ├── FileTokenProvider          │  ← JSON файл + рефреш на диск
│  ├── EnvTokenProvider           │  ← из env variable
│  └── CallbackTokenProvider      │  ← произвольная функция
├─────────────────────────────────┤
│  Auth (auth.go)                 │  ← OAuth PKCE + token refresh
│  ├── Login()                    │  ← browser → code → tokens
│  ├── LoginManual()              │  ← headless login
│  └── RefreshAccessToken()       │  ← рефреш токена
├─────────────────────────────────┤
│  HTTP transport                 │  ← JSON → HTTP → JSON
│  ├── billing header injection   │  ← обязателен для подписки
│  ├── metadata.user_id           │  ← device_id, account_uuid
│  └── Chrome TLS (uTLS)         │  ← обход Cloudflare
├─────────────────────────────────┤
│  Типы (types.go, request.go)    │  ← полная типизация API
├─────────────────────────────────┤
│  net/http + encoding/json       │  ← stdlib
└─────────────────────────────────┘
```

## Создание клиента

Единый конструктор `NewClient` с functional options:

```go
// Из файла
client, err := kraube.NewClient(ctx, kraube.WithCredentialsFile(""))

// Статичный токен
client, err := kraube.NewClient(ctx, kraube.WithAccessToken(token))

// Env variable
client, err := kraube.NewClient(ctx, kraube.WithEnvToken("KRAUBE_TOKEN"))

// Кастомный провайдер
client, err := kraube.NewClient(ctx, kraube.WithTokenProvider(myProvider))

// С опциями
client, err := kraube.NewClient(ctx,
    kraube.WithAccessToken(token),
    kraube.WithBaseURL("https://custom.endpoint"),
    kraube.WithHTTPClient(customHTTP),
    kraube.WithoutProfile(),
)
```

## Файлы

| Файл | Ответственность |
|------|----------------|
| `doc.go` | Package-level документация |
| `models.go` | Константы моделей |
| `types.go` | Типы данных API: Message, ContentBlock, Tool, Schema и т.д. |
| `request.go` | Request/Response структуры, APIError, streaming events |
| `client.go` | HTTP-клиент, MessagesService, StreamReader |
| `auth.go` | OAuth PKCE flow, token refresh, credentials persistence |
| `provider.go` | TokenProvider интерфейс и встроенные реализации |
| `options.go` | Functional options для NewClient |
| `transport.go` | Chrome TLS fingerprint (uTLS) |
| `ratelimit.go` | Rate limit парсинг и кеширование |
| `log.go` | Опциональное slog-логирование |
| `cmd/kraube/` | CLI: login, query, stream, usage |

## Принципы проектирования

1. **Типы = документация.** Каждая структура точно отражает JSON-схему API.

2. **Конструкторы для удобства, но не обязательны.** `UserMessage("text")` — сахар. Можно собрать `Message{}` руками.

3. **Streaming = аккумуляция.** `StreamReader` автоматически собирает финальный `MessageResponse` из дельт.

4. **Ошибки типизированы.** `APIError` имеет методы `IsRateLimit()`, `IsOverloaded()` и т.д.

5. **Stateless auth.** `TokenProvider` — единый интерфейс. Клиент не знает откуда токен.

6. **Реверс Claude Code как источник истины.** Если API не задокументирован — смотрим бинарник Claude Code CLI.

## Что НЕ входит в библиотеку (пока)

- Agent loop (tool execution cycle)
- MCP client
- Retry/backoff
- Batches API
- Local tool implementations (Bash, File I/O, etc.)
