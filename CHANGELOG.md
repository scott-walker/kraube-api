# Changelog

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
