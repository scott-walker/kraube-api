# Changelog

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
