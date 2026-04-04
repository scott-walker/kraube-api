# Принципы Kraube API

## Шлюз, а не SDK

Kraube API — легковесный шлюз для доступа к Anthropic Messages API через OAuth подписку. Не SDK с абстракциями, не фреймворк — минимальная обёртка, которая делает протокол доступным из Go.

## OAuth-only

Единственный способ аутентификации — OAuth Bearer token через подписку Claude Pro/Max/Team. Никаких API keys.

OAuth flow идёт на **claude.ai** (подписка), а не на platform.claude.com (Console). Эндпоинты и client ID взяты из реверса Claude Code CLI.

## Stateless

Клиент не знает откуда пришёл токен. Всё через `TokenProvider` — интерфейс с одним методом:

```go
type TokenProvider interface {
    Token(ctx context.Context) (string, error)
}
```

Файл, env variable, Vault, Redis, callback — любой источник. Клиент просто вызывает `Token()` перед каждым запросом.

## Reverse-engineering как источник истины

Если поведение не описано в официальной документации Anthropic API — **источник истины это бинарник Claude Code CLI** (`~/.local/share/claude/versions/`). Парсим, смотрим эндпоинты, параметры, протоколы — и реплицируем.

## Токен

После `kraube login` токен хранится в `~/.config/kraube/token` (XDG-совместимо, права `0600`). Это единственное, что нужно для аутентификации — access token'ы получаются и обновляются автоматически.

## Auto-refresh

Все встроенные провайдеры автоматически получают access token и рефрешат его за 60 секунд до истечения. Access token живёт только в памяти и никогда не записывается на диск.

## Минимум зависимостей

Только Go stdlib + uTLS (для Chrome TLS fingerprint). Чем меньше зависимостей, тем выше надёжность и предсказуемость.

## Библиотека, а не фреймворк

Kraube API — набор типов и функций. Не навязывает архитектуру. Хочешь agent loop — пиши свой. Хочешь TUI — используй любую библиотеку. Kraube API даёт фундамент: типизированный HTTP-клиент с полным покрытием Messages API.
