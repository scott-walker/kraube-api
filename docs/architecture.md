# Архитектура Kraube

## Принцип

Kraube — альтернатива Claude Code CLI на чистом Go.
Никаких внешних зависимостей, только stdlib.
Прямая работа с HTTP API через OAuth (подписка) или API key.

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
│  Auth (auth.go)                 │  ← OAuth PKCE + token refresh
│  ├── Login()                    │  ← browser → code → tokens
│  ├── RefreshAccessToken()       │  ← auto-refresh
│  ├── LoadCredentials()          │  ← ~/.config/kraube/credentials.json
│  └── LoadClaudeCredentials()    │  ← ~/.claude/.credentials.json
├─────────────────────────────────┤
│  HTTP transport (client.do)     │  ← JSON → HTTP → JSON
├─────────────────────────────────┤
│  Типы (types.go, request.go)    │  ← полная типизация API
├─────────────────────────────────┤
│  net/http + encoding/json       │  ← stdlib
└─────────────────────────────────┘
```

## Конструкторы клиента

| Функция | Режим | Описание |
|---------|-------|----------|
| `NewClientOAuth(ctx, "")` | OAuth | Основной. Загружает credentials, auto-refresh |
| `NewClientFromClaude(ctx)` | OAuth | Импорт из Claude Code |
| `NewClientAPIKey(apiKey)` | API Key | Альтернатива для программного доступа |

## Файлы

| Файл | Ответственность |
|------|----------------|
| `doc.go` | Package-level документация |
| `models.go` | Константы моделей |
| `types.go` | Типы данных API: Message, ContentBlock, Tool, Schema, ThinkingConfig и т.д. |
| `request.go` | Request/Response структуры, APIError, streaming events |
| `client.go` | HTTP-клиент, MessagesService, StreamReader, AuthMode |
| `auth.go` | OAuth PKCE flow, token refresh, credentials persistence |
| `cmd/kraube/` | CLI: login, query, stream |

## Принципы проектирования

1. **Типы = документация.** Каждая структура точно отражает JSON-схему API.

2. **Конструкторы для удобства, но не обязательны.** `UserMessage("text")` — сахар. Можно собрать `Message{}` руками.

3. **Streaming = аккумуляция.** `StreamReader` автоматически собирает финальный `MessageResponse` из дельт.

4. **Ошибки типизированы.** `APIError` имеет методы `IsRateLimit()`, `IsOverloaded()` и т.д.

5. **Zero dependencies.** Только stdlib.

6. **Реверс Claude Code как источник истины.** Если API не задокументирован — смотрим бинарник Claude Code CLI (`~/.local/share/claude/versions/`).

## Что НЕ входит в библиотеку (пока)

- Agent loop (tool execution cycle)
- MCP client
- Retry/backoff
- Batches API
- Local tool implementations (Bash, File I/O, etc.)
