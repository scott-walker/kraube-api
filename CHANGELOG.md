# Changelog

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
