# Changelog

## [0.5.0] - 2026-05-16

### Added
- **CLI generation flags for `query` / `stream` / default `kraube "prompt"`.** The CLI previously dispatched only a single-turn user message with hardcoded `ModelSonnet4_6` and `MaxTokens=4096`; this made it unusable for downstream callers (Python voice agents, scripts) that need multi-turn context and per-call control over generation. The new flags map directly onto `MessageRequest` and leave existing behaviour unchanged when omitted:
  - `--system TEXT` — inline system prompt (populates `MessageRequest.System` via `SystemText`).
  - `--system-file PATH` — read the system prompt from a file (same target field as `--system`; the two are mutually exclusive in practice, last-wins by parse order).
  - `--history PATH|-` — prior conversation as a JSON array `[{"role":"user|assistant","content":"..."}, ...]`. The literal `-` reads from stdin so callers can pipe history without temp files. The supplied prompt is appended as the final user message.
  - `--model NAME` — override the model id (default remains `claude-sonnet-4-6`).
  - `--max-tokens N` — response cap (default remains `4096`).
  - `--temperature F` — sampling temperature; when omitted the library default is preserved.
- **Token-level flush on `stream`.** `cmdStream` now calls `os.Stdout.Sync()` after every `text_delta`, so callers reading stdout chunk-by-chunk (typical Python subprocess pattern for voice pipelines) see each token as it lands instead of waiting for a full stdio buffer flush.

### Changed
- `cmdQuery` and `cmdStream` now accept a `genFlags` value and route through a new `buildRequest` helper. The default invocation (`kraube "prompt"`, no new flags) produces the exact same `MessageRequest` as before — `Model=ModelSonnet4_6`, `MaxTokens=4096`, single `UserMessage` — so existing scripts keep working without changes.

## [0.4.3] - 2026-04-14

### Fixed
- **`WithProxy` now covers OAuth token refresh**, not just `/v1/messages` and the initial profile fetch. Previously the refresh call inside the built-in token managers (`WithToken`, `WithTokenFile`, `WithEnvToken`) went through the package-level `authHTTPClient` (defaulting to `http.DefaultClient`), which ignored the Client's proxy configuration and produced `HTTP 403` from regions where a direct connection to `platform.claude.com/v1/oauth/token` is blocked. After `NewClient` resolves the per-Client `HTTPClient`, it is now propagated into the token manager so every subsequent refresh reuses the same transport — a single `WithProxy(...)` call is enough to route all outbound traffic of a Client instance. Callers no longer need to pair `WithProxy` with `NewProxiedHTTPClient` + `SetAuthHTTPClient`. The package-level `SetAuthHTTPClient` / `authHTTPClient` remain in place for **standalone** auth helpers that run without a Client (`Login`, `LoginManual`, top-level `FetchProfile`).

### Changed
- `refreshAccessToken` is now parameterised by `*http.Client`. When `nil`, it falls back to the package-level `authHTTPClient` — preserving the previous behaviour for any direct caller — but all internal call sites pass the per-Client HTTPClient.
- `tokenManager` and `envTokenManager` carry an `httpClient` field, populated by `NewClient`. `WithTokenProvider`-supplied providers remain the caller's responsibility: the wiring only applies to the library's built-in managers.

## [0.4.2] - 2026-04-14

### Fixed
- `ContentBlock.Content` is now a pointer (`*Content`) so `omitempty` actually drops the field when it is not set. Previously `text` and `tool_use` blocks serialised an empty `content: ""` property, which Anthropic rejected with `HTTP 400 "Extra inputs are not permitted"`. `ToolResultBlock` still emits a non-empty `*Content`; regression coverage in `TestContentBlock_NoEmptyContent` and `TestToolResultBlock_MarshalsContent`.

## [0.4.1] - 2026-04-13

### Fixed
- OAuth requests to `/v1/messages` now prepend the Claude Code identity preamble (`You are Claude Code, Anthropic's official CLI for Claude.`) as the first system block. The server-side gate on `api.anthropic.com` checks for this exact opening line and, when missing, rejects OAuth requests with `HTTP 403 forbidden: Request not allowed` even when billing and beta headers are otherwise valid. The internal `injectBillingHeader` helper was renamed to `injectSystemPrefix` to reflect that it now emits `identity → billing → user` blocks. User-supplied system prompts (string or blocks) are preserved verbatim after the two prefix blocks.

## [0.4.0] - 2026-04-13

### Added
- **Proxy support**, end-to-end for both API and OAuth traffic. New option `WithProxy(url)` and public helper `NewProxiedHTTPClient(url)`. Supported schemes: `http`, `https`, `socks5`, `socks5h`. Credentials in the URL are used as Basic proxy auth. With no option, the client automatically honors `HTTPS_PROXY` / `ALL_PROXY` from the environment; `WithProxy("")` is an explicit opt-out that forces a direct connection. HTTP/HTTPS proxies are handled via an in-house `CONNECT` tunnel built on top of uTLS, so the Chrome TLS fingerprint is preserved end-to-end regardless of proxy type.
- **Global CLI flag `--proxy URL`** — applies to every subcommand (`login`, `usage`, `query`, `stream`). Also installed for OAuth endpoints (via `SetAuthHTTPClient`) so `kraube login --proxy ...` routes through the proxy too.
- **`SetAuthHTTPClient(c *http.Client)`** — swap the package-level HTTP client used by standalone OAuth flows (`LoginManual`, token refresh/exchange, `FetchProfile`). Pass `nil` to restore `http.DefaultClient`.
- **Detailed error diagnostics.** On any non-2xx response the client now emits a single `api: error response` log entry with `method`, full `url`, `status`, `elapsed`, `local_addr`, `remote_addr`, `proxy` (redacted), request headers (with `Authorization` / `Cookie` / `Proxy-Authorization` / `Set-Cookie` redacted), request body, response headers, and response body — each bounded to 8 KB so large payloads don't flood stderr. Transport-level failures emit an analogous `api: request failed` entry. Response bodies on error paths are buffered in memory and re-exposed through `resp.Body`, so existing decoders continue to work transparently.
- **Egress visibility.** Successful responses and transport handshakes now log `local_addr` / `remote_addr` / `proxy` at debug level — useful when debugging multi-homed hosts, split proxies, or provider-side IP gating.

### Changed
- `APIError.Error()` now renders as `HTTP <status> <type>: <message>` when the HTTP status is known, so the numeric code is always visible without a type assertion. When `Status == 0`, the legacy `<type>: <message>` form is preserved.
- `Client.fetchProfile` (called by `NewClient`) now routes through `c.HTTPClient`, so `WithProxy` automatically applies to the profile request alongside `/v1/messages`.
- `chromeTransport` switches from a bare `net.Dial` to a context-aware `net.Dialer.DialContext` with a 30s timeout — respects `ctx` cancellation and avoids hangs on silent networks.

## [0.3.0] - 2026-04-12

### Breaking
- Token file format changed from plain-text back to JSON at `~/.config/kraube/credentials.json`. Stores `refreshToken`, `accessToken`, and `expiresAt` together so live access tokens are reused across restarts without extra refresh calls.
- Removed `SaveToken`, `LoadToken`, `DefaultTokenPath`. Use `SaveCredentials`, `LoadCredentials`, `DefaultCredentialsPath` + the new `Credentials` struct instead.
- `Login` / `LoginManual` now return `*Credentials` instead of `string`.
- CLI: `kraube login` writes the new JSON file (old `~/.config/kraube/token` is ignored — re-run `kraube login` after upgrading).

### Added
- Safe multi-process operation on the same machine: `WithTokenFile` now acquires an OS-level file lock (`flock(2)` / `LockFileEx`) around refresh and re-reads the file under the lock, so parallel processes share a single rotated refresh token without racing.
- `KRAUBE_CREDENTIALS_PATH` environment variable: overrides the default credentials path for both the CLI and `WithTokenFile("")`.
- CLI flag `--out PATH` on `kraube login` to write credentials to a custom path.
- `Credentials.IsAccessLive()` helper.

### Changed
- `WithToken(refreshToken)` is now explicitly in-memory only; rotated refresh tokens are not persisted. Use `WithTokenFile` for persistence.

## [0.2.0] - 2026-04-04

### Breaking
- `TokenProvider` interface now returns `(string, error)` instead of `(*Credentials, error)`
- Removed `Credentials` struct from public API
- Replaced `WithAccessToken`, `WithCredentials`, `WithCredentialsFile` with `WithToken`, `WithTokenFile`
- Replaced `SaveCredentials`/`LoadCredentials` with `SaveToken`/`LoadToken`
- Token file format changed from JSON to plain text (`~/.config/kraube/token`)
- `Login`/`LoginManual` now return `string` (token) instead of `*Credentials`

### Added
- Real-time streaming events: `Event()`, `EventType()`, `CurrentBlock()` on `StreamReader`
- `StreamEvent` interface with typed events for type-switch handling
- Unit tests covering auth, providers, streaming, types, request helpers, rate limits
- CLI `kraube stream` now outputs text in real time (character by character)

### Changed
- Simplified token model: users store one "token" — access tokens managed in memory
- All documentation updated to reflect new token API and streaming events

## [0.1.0] - 2026-04-04

### Added
- Stateless `TokenProvider` interface for flexible authentication
- Functional options pattern: `NewClient(ctx, ...Option)` as single entry point
- Built-in providers: Token, TokenFile, EnvToken, Callback
- `README.md` with full usage guide and brand assets
- GitHub Actions CI/CD (lint, test, release)
- GoReleaser configuration for cross-platform CLI builds
- `LICENSE` (MIT)

### Changed
- Rebranded from `kraube-go/kraube` to `scott-walker/kraube-api` (Kraube API)
- Removed API key support — OAuth-only by design
- Removed `NewClientOAuth`, `NewClientFromClaude`, `NewClientAPIKey` constructors
- Removed `AuthMode`, `AuthAPIKey` types
- Removed Claude Code credential import (`LoadClaudeCredentials`)
- Updated all documentation to reflect new architecture

## [0.0.0] - 2026-04-04

- Initial commit: OAuth PKCE flow, Messages API, streaming, tool use
- Chrome TLS fingerprint via uTLS
- Billing header and metadata injection
- Rate limit tracking and persistence
- CLI: login, query, stream, usage
