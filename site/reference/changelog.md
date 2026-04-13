# Changelog

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
- Credentials are now stored as JSON at `~/.config/kraube/credentials.json` (`refreshToken` + `accessToken` + `expiresAt`). The old plain-text `~/.config/kraube/token` is no longer read — re-run `kraube login` after upgrading.
- Removed `SaveToken`, `LoadToken`, `DefaultTokenPath`. Use `SaveCredentials`, `LoadCredentials`, `DefaultCredentialsPath` and the new `Credentials` struct.
- `Login` / `LoginManual` now return `*Credentials` instead of `string`.

### Added
- Multi-process safety: `WithTokenFile` coordinates refresh via an OS-level file lock (`flock(2)` on Linux/macOS, `LockFileEx` on Windows), with read-after-lock semantics. Parallel processes on one machine share a single rotation.
- `KRAUBE_CREDENTIALS_PATH` environment variable: honored by both the CLI and `WithTokenFile("")`.
- CLI flag `--out PATH` on `kraube login`.
- `Credentials.IsAccessLive()` helper.

### Changed
- `WithToken(refreshToken)` is now explicitly in-memory only — rotation is not persisted. Use `WithTokenFile` for persistence and parallelism.

## [0.2.0] - 2026-04-04

### Breaking
- `TokenProvider` interface returns `(string, error)` instead of `(*Credentials, error)`
- Removed `Credentials` struct — replaced with single "token" concept
- New options: `WithToken`, `WithTokenFile` replace old credential options
- Token stored as plain text in `~/.config/kraube/token`

### Added
- Real-time streaming events: `Event()`, `EventType()`, `CurrentBlock()`
- `StreamEvent` interface for typed event handling via type switch
- Unit test suite covering auth, providers, streaming, types, rate limits
- CLI real-time text output in `kraube stream`

### Changed
- Simplified authentication: one token, access tokens managed in memory
- Updated all documentation

## [0.1.0] - 2026-04-04

### Added
- Stateless `TokenProvider` interface for flexible authentication
- Functional options pattern: `NewClient(ctx, ...Option)` as single entry point
- Built-in providers: Token, TokenFile, EnvToken, Callback
- `README.md` with full usage guide and brand assets
- GitHub Actions CI/CD (lint, test, release)
- GoReleaser configuration for cross-platform CLI builds
- Documentation site with VitePress
- `LICENSE` (MIT)

### Changed
- Rebranded to Kraube API (`scott-walker/kraube-api`)
- OAuth-only by design — removed API key support
- Single `NewClient` constructor replaces three old constructors

## [0.0.0] - 2026-04-04

- Initial commit: OAuth PKCE flow, Messages API, streaming, tool use
- Chrome TLS fingerprint via uTLS
- Billing header and metadata injection
- Rate limit tracking and persistence
- CLI: login, query, stream, usage
