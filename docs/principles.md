# Принципы Kraube

## OAuth-first

Kraube работает через OAuth (подписка Claude Pro/Max/Team).
Это **принципиальное решение**, а не fallback.

API key поддерживается как альтернатива, но основной сценарий —
пользователь подписки, который хочет работать с Claude без Node.js.

## Приоритет аутентификации

1. **kraube login** — собственный OAuth flow, credentials в `~/.config/kraube/credentials.json`
2. **kraube login --claude** — импорт из Claude Code (`~/.claude/.credentials.json`)
3. **NewClientAPIKey()** — API key для программного использования

## Формат credentials

```json
{
  "access_token": "...",
  "refresh_token": "...",
  "expires_at": 1712345678000
}
```

Хранится в `~/.config/kraube/credentials.json` (XDG-совместимо).
Права файла: `0600`.

## Auto-refresh

Клиент автоматически обновляет token перед каждым запросом, если до
истечения осталось менее 60 секунд. Обновлённые credentials
автоматически сохраняются на диск.

## Zero dependencies

Только Go stdlib. Никаких сторонних библиотек. Это не ограничение —
это выбор. Чем меньше зависимостей, тем выше надёжность и
предсказуемость.

## Библиотека, а не фреймворк

Kraube — это набор типов и функций. Не навязывает архитектуру.
Хочешь agent loop — пиши свой. Хочешь TUI — используй любую
библиотеку. Kraube даёт фундамент: типизированный HTTP-клиент
с полным покрытием Anthropic API.
