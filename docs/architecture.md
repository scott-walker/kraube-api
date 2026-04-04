# Архитектура Kraube

## Принцип

Kraube — чистый Go-клиент для Anthropic API. Никаких внешних зависимостей, только stdlib.
Не обёртка над CLI, а прямая работа с HTTP API.

## Слои

```
┌─────────────────────────────────┐
│  Пользовательский код           │
├─────────────────────────────────┤
│  kraube.Client                  │  ← точка входа
│  ├── Messages.Create()          │  ← синхронный запрос
│  ├── Messages.Stream()          │  ← SSE streaming
│  └── Messages.CountTokens()     │  ← подсчёт токенов
├─────────────────────────────────┤
│  HTTP transport (client.do)     │  ← JSON → HTTP → JSON
├─────────────────────────────────┤
│  Типы (types.go, request.go)    │  ← полная типизация API
├─────────────────────────────────┤
│  net/http + encoding/json       │  ← stdlib
└─────────────────────────────────┘
```

## Файлы

| Файл | Ответственность |
|------|----------------|
| `doc.go` | Package-level документация |
| `models.go` | Константы моделей |
| `types.go` | Типы данных API: Message, ContentBlock, Tool, Schema, ThinkingConfig и т.д. |
| `request.go` | Request/Response структуры, APIError, streaming events |
| `client.go` | HTTP-клиент, MessagesService, StreamReader |

## Принципы проектирования

1. **Типы = документация.** Каждая структура точно отражает JSON-схему API. Не надо читать документацию — достаточно посмотреть тип.

2. **Конструкторы для удобства, но не обязательны.** `UserMessage("text")` — сахар. Можно собрать `Message{}` руками.

3. **Streaming = аккумуляция.** `StreamReader` автоматически собирает финальный `MessageResponse` из дельт. Можно читать по событиям, а можно дождаться `Message()`.

4. **Ошибки типизированы.** `APIError` имеет методы `IsRateLimit()`, `IsOverloaded()` и т.д. — для удобного branching.

5. **Zero dependencies.** Только stdlib. Никакого vendor lock-in.

## Что НЕ входит в библиотеку (пока)

- Agent loop (tool execution cycle)
- MCP client
- Retry/backoff
- Batches API
- Local tool implementations (Bash, File I/O, etc.)

Всё это — надстройки над фундаментом, который уже есть.
