# API Coverage

Full coverage status of the Anthropic Messages API.

## Messages API (`/v1/messages`)

| Parameter | Status |
|-----------|:---:|
| model | Supported |
| max_tokens | Supported |
| messages | Supported |
| system | Supported |
| temperature | Supported |
| top_p | Supported |
| top_k | Supported |
| stop_sequences | Supported |
| tools | Supported |
| tool_choice | Supported |
| stream | Supported |
| thinking | Supported |
| output_config | Supported |
| metadata | Supported |
| service_tier | Supported |

## Content Block Types

| Type | Status |
|------|:---:|
| text | Supported |
| image (base64) | Supported |
| image (URL) | Supported |
| document | Supported |
| tool_use | Supported |
| tool_result | Supported |
| thinking | Supported |
| redacted_thinking | Supported |

## Tool Types

| Type | Status |
|------|:---:|
| Custom tools | Supported |
| web_search | Supported |
| code_execution | Supported |
| text_editor | Supported |
| bash | Supported |

## Streaming Events

All 8 SSE event types are supported:

| Event | Status |
|-------|:---:|
| message_start | Supported |
| content_block_start | Supported |
| content_block_delta | Supported |
| content_block_stop | Supported |
| message_delta | Supported |
| message_stop | Supported |
| ping | Supported |
| error | Supported |

## Error Handling

| Method | Description |
|--------|-------------|
| `IsInvalidRequest()` | 400 — bad request |
| `IsAuthentication()` | 401 — invalid token |
| `IsPermission()` | 403 — no permission |
| `IsNotFound()` | 404 — not found |
| `IsRateLimit()` | 429 — rate limited |
| `IsOverloaded()` | 529 — server overloaded |

## Not Yet Implemented

| Feature | Priority |
|---------|----------|
| Message Batches | Next |
| Retry with backoff | Next |
| Admin API | Low |
| Models API | Low |
