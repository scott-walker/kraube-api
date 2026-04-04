# Протокол HTTP API Anthropic для OAuth-подписки

Результаты реверс-инжиниринга Claude Code CLI (версия 2.1.92).
Документ описывает полный HTTP-протокол доступа к Anthropic Messages API
через OAuth-подписку (Pro/Max/Team), без платного API-ключа.

---

## 1. Endpoint

```
POST https://api.anthropic.com/v1/messages?beta=true
```

Параметр `?beta=true` **обязателен** для OAuth-токенов. Без него сервер
возвращает ошибку авторизации.

---

## 2. Обязательные HTTP-заголовки

| Заголовок | Значение | Назначение |
|-----------|----------|------------|
| `Authorization` | `Bearer <access_token>` | OAuth access token |
| `anthropic-dangerous-direct-browser-access` | `true` | **Обязателен** для OAuth-токенов. Без него — 403 |
| `Anthropic-Version` | `2023-06-01` | Версия API |
| `Content-Type` | `application/json` | Формат тела запроса |
| `User-Agent` | `claude-cli/2.1.92 (external, cli)` | Идентификация клиента |
| `x-app` | `cli` | Тип приложения |
| `Anthropic-Beta` | *(зависит от модели, см. ниже)* | Beta-флаги |

---

## 3. Beta-заголовки по моделям

Набор beta-фич зависит от запрашиваемой модели:

### Haiku / Sonnet (базовый набор)

```
Anthropic-Beta: oauth-2025-04-20,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05
```

### Opus (расширенный набор)

```
Anthropic-Beta: claude-code-20250219,oauth-2025-04-20,context-1m-2025-08-07,interleaved-thinking-2025-05-14,redact-thinking-2026-02-12,context-management-2025-06-27,prompt-caching-scope-2026-01-05,advanced-tool-use-2025-11-20,effort-2025-11-24
```

Отличия Opus от базового набора:
- `claude-code-20250219` — специальный режим Claude Code
- `context-1m-2025-08-07` — поддержка контекста 1M токенов
- `redact-thinking-2026-02-12` — редактирование thinking-блоков
- `context-management-2025-06-27` — управление контекстом
- `advanced-tool-use-2025-11-20` — расширенное использование инструментов
- `effort-2025-11-24` — управление уровнем усилий

---

## 4. Billing header (заголовок биллинга)

**Критически важно**: системный промпт (`system`) ДОЛЖЕН содержать billing header
как **первый элемент** массива. Без него Sonnet и Opus возвращают 429.

Формат:

```json
{
  "type": "text",
  "text": "x-anthropic-billing-header: cc_version=2.1.92.00a; cc_entrypoint=cli; cch=<hash>;"
}
```

Хеш `cch` вычисляется так:
1. Строка-источник: `cc_version=2.1.92.00a; cc_entrypoint=cli;`
2. SHA-256 от этой строки
3. Первые 5 символов hex-представления хеша

Пример на Go:

```go
seed := "cc_version=2.1.92.00a; cc_entrypoint=cli;"
h := sha256.Sum256([]byte(seed))
cch := hex.EncodeToString(h[:])[:5]
// Результат: "x-anthropic-billing-header: cc_version=2.1.92.00a; cc_entrypoint=cli; cch=<cch>;"
```

Billing header вставляется первым элементом в массив `system`, перед
пользовательским системным промптом.

---

## 5. Поле metadata.user_id

Для OAuth-запросов поле `metadata.user_id` обязательно. Значение — **JSON-строка**,
содержащая объект с тремя полями:

```json
{
  "metadata": {
    "user_id": "{\"device_id\":\"<sha256_hex>\",\"account_uuid\":\"<uuid>\",\"session_id\":\"<uuid>\"}"
  }
}
```

- `device_id` — случайный SHA-256 хеш (стабильный для устройства)
- `account_uuid` — UUID аккаунта из `/api/oauth/profile`
- `session_id` — UUID v4, генерируется при создании клиента

Профиль получается через:

```
GET https://api.anthropic.com/api/oauth/profile
Authorization: Bearer <access_token>
```

Ответ содержит `account.uuid` и `organization.uuid`.

---

## 6. TLS fingerprint (обязательно)

Cloudflare на `api.anthropic.com` **блокирует** стандартный TLS fingerprint Go
(net/http) для OAuth Bearer-запросов, возвращая 429.

Решение: использовать [uTLS](https://github.com/refraction-networking/utls) с профилем
`HelloChrome_Auto` для имитации TLS-отпечатка Chrome.

Kraube API использует uTLS для всех запросов.

---

## 7. OAuth flow

### 7.1 Authorization

```
GET https://claude.com/cai/oauth/authorize?code=true&client_id=<id>&redirect_uri=<uri>&response_type=code&code_challenge=<challenge>&code_challenge_method=S256&scope=<scopes>&state=<state>
```

Параметры:
- `code=true` — **обязательный** параметр (без него flow не работает)
- `client_id` = `9d1c250a-e61b-44d9-88ed-5944d1962f5e`
- `scope` = `user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload`
- PKCE: S256 challenge/verifier

Браузер перенаправляет на `http://localhost:<port>/callback?code=<code>&state=<state>`.

### 7.2 Token exchange

```
POST https://platform.claude.com/v1/oauth/token
Content-Type: application/json

{
  "grant_type": "authorization_code",
  "code": "<auth_code>",
  "code_verifier": "<pkce_verifier>",
  "client_id": "<client_id>",
  "redirect_uri": "<redirect_uri>",
  "state": "<state>"
}
```

**Важно**: тело запроса — JSON (не form-encoded). Параметр `state` обязателен.

Ответ:

```json
{
  "access_token": "...",
  "refresh_token": "...",
  "expires_in": 3600,
  "token_type": "Bearer"
}
```

### 7.3 Token refresh

```
POST https://platform.claude.com/v1/oauth/token
Content-Type: application/json

{
  "grant_type": "refresh_token",
  "refresh_token": "<refresh_token>",
  "client_id": "<client_id>",
  "scope": "user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
}
```

---

## 8. Rate limit headers

API возвращает rate limit информацию в заголовках ответа (включая ответы 429):

| Заголовок | Описание |
|-----------|----------|
| `anthropic-ratelimit-unified-status` | `allowed`, `rejected`, `allowed_warning` |
| `anthropic-ratelimit-unified-reset` | Unix timestamp сброса |
| `anthropic-ratelimit-unified-representative-claim` | `five_hour`, `seven_day`, `seven_day_opus`, `seven_day_sonnet`, `overage` |
| `anthropic-ratelimit-unified-fallback` | `available` если fallback доступен |
| `anthropic-ratelimit-unified-5h-utilization` | Использование 5-часового окна (0.0-1.0) |
| `anthropic-ratelimit-unified-5h-reset` | Unix timestamp сброса 5h окна |
| `anthropic-ratelimit-unified-7d-utilization` | Использование 7-дневного окна (0.0-1.0) |
| `anthropic-ratelimit-unified-7d-reset` | Unix timestamp сброса 7d окна |
| `anthropic-ratelimit-unified-overage-status` | Статус overage |
| `anthropic-ratelimit-unified-overage-reset` | Unix timestamp сброса overage |
| `anthropic-ratelimit-unified-overage-disabled-reason` | Причина отключения overage |

---

## 9. Формат полного запроса (пример)

```http
POST /v1/messages?beta=true HTTP/2
Host: api.anthropic.com
Authorization: Bearer eyJ...
Anthropic-Version: 2023-06-01
Anthropic-Beta: oauth-2025-04-20,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05
anthropic-dangerous-direct-browser-access: true
Content-Type: application/json
User-Agent: claude-cli/2.1.92 (external, cli)
x-app: cli

{
  "model": "claude-sonnet-4-6",
  "max_tokens": 8096,
  "stream": true,
  "system": [
    {
      "type": "text",
      "text": "x-anthropic-billing-header: cc_version=2.1.92.00a; cc_entrypoint=cli; cch=abcde;"
    },
    {
      "type": "text",
      "text": "You are a helpful assistant."
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "Hello!"
    }
  ],
  "metadata": {
    "user_id": "{\"device_id\":\"abc123...\",\"account_uuid\":\"uuid-here\",\"session_id\":\"uuid-here\"}"
  }
}
```

---

## 10. Хранение токенов

Claude Code хранит OAuth-токены в `~/.claude/{profile}/.credentials.json`:

```json
{
  "claudeAiOauth": {
    "accessToken": "...",
    "refreshToken": "...",
    "expiresAt": 1712345678000
  }
}
```

`expiresAt` — Unix timestamp в **миллисекундах**.

Kraube API хранит токен в `~/.config/kraube/token` (plain text, только refresh token).
