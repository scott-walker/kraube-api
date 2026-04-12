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

## Credentials

После `kraube login` credentials хранятся в `~/.config/kraube/credentials.json` (XDG-совместимо, права `0600`). Файл содержит `refreshToken`, `accessToken` и `expiresAt`, так что любой процесс подхватывает живой access-токен без повторного refresh. Путь переопределяется через `KRAUBE_CREDENTIALS_PATH` или флаг `--out` у `kraube login`.

## Auto-refresh и параллельные процессы

Access-токен рефрешится за 60 секунд до истечения. При использовании `WithTokenFile` операции чтения/ротации сериализуются OS-level file lock (`flock(2)` на Linux/macOS, `LockFileEx` на Windows): если два процесса одновременно видят, что access истёк, ровно один из них делает HTTP-refresh, остальные подхватывают результат из файла под тем же локом. Это позволяет запускать неограниченное количество процессов kraube-api на одной машине с одним логином.

## Минимум зависимостей

Только Go stdlib + uTLS (для Chrome TLS fingerprint). Чем меньше зависимостей, тем выше надёжность и предсказуемость.

## Библиотека, а не фреймворк

Kraube API — набор типов и функций. Не навязывает архитектуру. Хочешь agent loop — пиши свой. Хочешь TUI — используй любую библиотеку. Kraube API даёт фундамент: типизированный HTTP-клиент с полным покрытием Messages API.
