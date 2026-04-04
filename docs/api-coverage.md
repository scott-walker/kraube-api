# Покрытие Anthropic API

Статус покрытия API библиотекой Kraube.

## Messages API (`/v1/messages`)

### Request параметры

| Параметр | Тип | Статус |
|----------|-----|--------|
| `model` | string | ✅ |
| `max_tokens` | int | ✅ |
| `messages` | []Message | ✅ |
| `system` | string \| []SystemBlock | ✅ |
| `temperature` | *float64 | ✅ |
| `top_p` | *float64 | ✅ |
| `top_k` | *int | ✅ |
| `stop_sequences` | []string | ✅ |
| `tools` | []Tool | ✅ |
| `tool_choice` | *ToolChoice | ✅ |
| `stream` | bool | ✅ (автоматически) |
| `thinking` | *ThinkingConfig | ✅ |
| `output_config` | *OutputConfig | ✅ |
| `metadata` | *Metadata | ✅ |
| `service_tier` | string | ✅ |
| `cache_control` | *CacheControl | ✅ |

### Content Block types

| Тип | Input (request) | Output (response) |
|-----|-----------------|-------------------|
| `text` | ✅ TextBlock() | ✅ |
| `image` (base64) | ✅ ImageBase64Block() | — |
| `image` (url) | ✅ ImageURLBlock() | — |
| `document` | ✅ (struct) | — |
| `tool_use` | ✅ ToolUseBlock() | ✅ |
| `tool_result` | ✅ ToolResultBlock() | — |
| `thinking` | ✅ ThinkingBlock() | ✅ |
| `redacted_thinking` | ✅ (struct) | ✅ |
| `search_result` | ✅ (struct) | ✅ |

### Tool types

| Тип | Статус |
|-----|--------|
| Custom tools | ✅ Tool{} + Schema{} |
| `web_search` | ✅ WebSearchTool() |
| `code_execution` | ✅ CodeExecutionTool() |
| `text_editor` | ✅ TextEditorTool() |
| `bash` | ✅ BashTool() |
| Tool choice (auto/any/tool/none) | ✅ |

### Response

| Поле | Статус |
|------|--------|
| `id` | ✅ |
| `type` | ✅ |
| `role` | ✅ |
| `content` | ✅ |
| `model` | ✅ |
| `stop_reason` | ✅ |
| `stop_sequence` | ✅ |
| `usage` | ✅ |

### Streaming events

| Event | Статус |
|-------|--------|
| `message_start` | ✅ |
| `content_block_start` | ✅ |
| `content_block_delta` | ✅ |
| `content_block_stop` | ✅ |
| `message_delta` | ✅ |
| `message_stop` | ✅ |
| `ping` | ✅ |
| `error` | ✅ |

### Delta types

| Delta | Статус |
|-------|--------|
| `text_delta` | ✅ |
| `input_json_delta` | ✅ |
| `thinking_delta` | ✅ |
| `signature_delta` | ✅ |

## Token Counting (`/v1/messages/count_tokens`)

| | Статус |
|--|--------|
| Request | ✅ |
| Response | ✅ |

## Errors

| Тип ошибки | Статус |
|------------|--------|
| `invalid_request_error` | ✅ IsInvalidRequest() |
| `authentication_error` | ✅ IsAuthentication() |
| `permission_error` | ✅ IsPermission() |
| `not_found_error` | ✅ IsNotFound() |
| `rate_limit_error` | ✅ IsRateLimit() |
| `overloaded_error` | ✅ IsOverloaded() |
| HTTP status code | ✅ APIError.Status |

## Ещё не реализовано

| API | Приоритет |
|-----|-----------|
| Message Batches (`/v1/messages/batches`) | 🔲 Следующий |
| Retry с exponential backoff | 🔲 Следующий |
| Admin API | 🔲 Низкий |
| Models API (`/v1/models`) | 🔲 Низкий |
