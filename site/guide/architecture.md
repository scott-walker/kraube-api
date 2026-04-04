# Architecture

## Layers

```
┌─────────────────────────────────┐
│  Your code / CLI                │
├─────────────────────────────────┤
│  kraube.Client                  │  ← entry point
│  ├── Messages.Create()          │  ← sync request
│  ├── Messages.Stream()          │  ← SSE streaming
│  └── Messages.CountTokens()     │  ← token counting
├─────────────────────────────────┤
│  TokenProvider                  │  ← where token comes from
│  ├── StaticTokenProvider        │
│  ├── CredentialsProvider        │
│  ├── FileTokenProvider          │
│  ├── EnvTokenProvider           │
│  └── CallbackTokenProvider      │
├─────────────────────────────────┤
│  Auth (auth.go)                 │  ← OAuth PKCE + refresh
├─────────────────────────────────┤
│  HTTP Transport                 │  ← billing header, metadata
│  └── Chrome TLS (uTLS)         │  ← Cloudflare bypass
├─────────────────────────────────┤
│  Types (types.go, request.go)   │  ← full API type coverage
├─────────────────────────────────┤
│  net/http + encoding/json       │  ← stdlib
└─────────────────────────────────┘
```

## File Organization

| File | Responsibility |
|------|----------------|
| `client.go` | Client struct, NewClient, HTTP transport, StreamReader |
| `provider.go` | TokenProvider interface and built-in implementations |
| `options.go` | Functional options for NewClient |
| `auth.go` | OAuth PKCE flow, token refresh, credentials persistence |
| `types.go` | API data types: Message, ContentBlock, Tool, Schema |
| `request.go` | Request/Response structs, APIError, streaming events |
| `models.go` | Model constants |
| `transport.go` | Chrome TLS fingerprint (uTLS) |
| `ratelimit.go` | Rate limit parsing and caching |
| `log.go` | Optional slog-based logging |
