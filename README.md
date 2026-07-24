<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/brand.png">
  <img src="assets/brand.png" alt="Kraube API" width="480" style="background: white; padding: 12px 20px; border-radius: 8px;">
</picture>

**Lightweight Go gateway for Anthropic Messages API via OAuth subscription**

Access Claude (Opus, Sonnet, Haiku) through your Pro/Max/Team subscription. No API key needed.

[![CI](https://github.com/scott-walker/kraube-api/actions/workflows/ci.yml/badge.svg)](https://github.com/scott-walker/kraube-api/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/scott-walker/kraube-api)](https://github.com/scott-walker/kraube-api/releases/latest)
[![Go Report Card](https://goreportcard.com/badge/github.com/scott-walker/kraube-api)](https://goreportcard.com/report/github.com/scott-walker/kraube-api)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[Documentation](https://scott-walker.github.io/kraube-api/) · [Quick Start](#quick-start) · [TokenProvider](#tokenprovider) · [Releases](https://github.com/scott-walker/kraube-api/releases)

</div>

---

## What it does

- **OAuth gateway** to Anthropic Messages API through claude.ai subscription (Pro/Max/Team)
- **Stateless** — token comes from any source via `TokenProvider` interface
- **Replicates** Claude Code CLI protocol: billing header, metadata, beta headers, Chrome TLS
- **Streaming** and non-streaming requests, tool use, extended thinking, vision, documents
- **Daemon mode** — `kraube serve`: local HTTP gateway with proactive background token refresh
- **Lightweight** — minimal wrapper over HTTP, not a framework

## Quick Start

### Install

```bash
go get github.com/scott-walker/kraube-api
```

### Authenticate

```bash
go build -o kraube ./cmd/kraube/
kraube login
```

Opens browser for OAuth, saves credentials to `~/.config/kraube/credentials.json`. Override with `--out PATH` or `KRAUBE_CREDENTIALS_PATH`.

### Use

```go
ctx := context.Background()

client, err := kraube.NewClient(ctx, kraube.WithTokenFile(""))
if err != nil {
    log.Fatal(err)
}

resp, err := client.Messages.Create(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelSonnet4_6,
    MaxTokens: 1024,
    Messages:  []kraube.Message{kraube.UserMessage("Hello!")},
})
fmt.Println(resp.Text())
```

## TokenProvider

Single interface for authentication. Token can come from anywhere:

```go
type TokenProvider interface {
    Token(ctx context.Context) (string, error)
}
```

Built-in options:

| Option | Description | Auto-refresh | Multi-process safe |
|--------|-------------|:---:|:---:|
| `WithTokenFile(path)` | JSON credentials file + OS-level file lock | Yes | Yes |
| `WithToken(token)` | Refresh token held in memory | Yes | No (in-memory only) |
| `WithEnvToken(envVar)` | Refresh token from env var (in-memory) | Yes | No |
| `WithTokenProvider(p)` | Any custom implementation | Up to you | Up to you |

```go
// From file (after kraube login). Safe across parallel processes on the
// same machine — refresh is serialized via flock(2) / LockFileEx.
client, _ := kraube.NewClient(ctx, kraube.WithTokenFile(""))

// Refresh token directly. In-memory only — rotations are not persisted.
client, _ := kraube.NewClient(ctx, kraube.WithToken(os.Getenv("MY_TOKEN")))

// Custom provider (Vault, Redis, DB...)
client, _ := kraube.NewClient(ctx, kraube.WithTokenProvider(myVaultProvider))
```

## Usage Guide

### Streaming

```go
stream, err := client.Messages.Stream(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelSonnet4_6,
    MaxTokens: 1024,
    Messages:  []kraube.Message{kraube.UserMessage("Tell me a story")},
})
defer stream.Close()

for stream.Next() {
    switch evt := stream.Event().(type) {
    case *kraube.ContentBlockStartEvent:
        if evt.ContentBlock.Type == "tool_use" {
            fmt.Printf("Tool: %s\n", evt.ContentBlock.Name)
        }
    case *kraube.ContentBlockDeltaEvent:
        if evt.Delta.Type == "text_delta" {
            fmt.Print(evt.Delta.Text) // real-time text
        }
    }
}
fmt.Println()
```

### System Prompt

```go
resp, _ := client.Messages.Create(ctx, &kraube.MessageRequest{
    Model:    kraube.ModelSonnet4_6,
    MaxTokens: 1024,
    System:   kraube.SystemText("You are a helpful assistant."),
    Messages: []kraube.Message{kraube.UserMessage("Hi")},
})
```

### Tool Use

```go
tool := kraube.Tool{
    Name:        "get_weather",
    Description: "Get weather for a city",
    InputSchema: &kraube.Schema{
        Type: "object",
        Properties: map[string]*kraube.Schema{
            "city": {Type: "string", Desc: "City name"},
        },
        Required: []string{"city"},
    },
}

resp, _ := client.Messages.Create(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelSonnet4_6,
    MaxTokens: 1024,
    Tools:     []kraube.Tool{tool},
    Messages:  []kraube.Message{kraube.UserMessage("Weather in Tokyo?")},
})

if resp.HasToolUse() {
    for _, tu := range resp.ToolUses() {
        fmt.Printf("Tool: %s, Input: %s\n", tu.Name, string(tu.Input))
    }
}
```

Built-in tools:

```go
kraube.WebSearchTool()      // web search
kraube.CodeExecutionTool()  // code execution
kraube.TextEditorTool()     // text editor (Claude Code style)
kraube.BashTool()           // bash execution
```

### Extended Thinking

```go
resp, _ := client.Messages.Create(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelOpus4_6,
    MaxTokens: 8192,
    Thinking:  kraube.ThinkingEnabled(4096),
    Messages:  []kraube.Message{kraube.UserMessage("Solve this...")},
})

for _, b := range resp.ThinkingBlocks() {
    fmt.Println("Thinking:", b.Thinking)
}
fmt.Println("Answer:", resp.Text())
```

### Vision

```go
resp, _ := client.Messages.Create(ctx, &kraube.MessageRequest{
    Model:     kraube.ModelSonnet4_6,
    MaxTokens: 1024,
    Messages: []kraube.Message{
        kraube.UserBlocks(
            kraube.TextBlock("What's in this image?"),
            kraube.ImageURLBlock("https://example.com/photo.jpg"),
        ),
    },
})
```

### Error Handling

```go
resp, err := client.Messages.Create(ctx, req)
if err != nil {
    var apiErr *kraube.APIError
    if errors.As(err, &apiErr) {
        switch {
        case apiErr.IsRateLimit():
            // wait and retry
        case apiErr.IsOverloaded():
            // server overloaded
        case apiErr.IsAuthentication():
            // invalid token
        }
    }
}
```

`APIError.Error()` renders as `HTTP <status> <type>: <message>` so the numeric status is always visible, even without a type assertion.

With `--debug` (or `kraube.EnableDevLog()`), every failing request emits a full `api: error response` log line containing the method, URL, HTTP status, local and remote socket addresses, proxy URL (redacted), request headers (with `Authorization` / `Cookie` redacted), request body, response headers, and response body — bounded to 8 KB per field so huge payloads don't flood stderr. Successful requests also log `local_addr` / `remote_addr` / `proxy`, which makes egress-IP problems (multi-homed hosts, split proxies) easy to diagnose.

### Proxy

Kraube API routes all traffic of a single Client instance — `/v1/messages`, the initial profile fetch, and OAuth token refresh — through a single transport. A single `WithProxy(...)` is enough; the library propagates the per-Client `HTTPClient` into the token manager, so refresh reuses the same tunnel. The Chrome TLS fingerprint is preserved end-to-end: the proxy only sees a plain TCP hop, and the uTLS handshake runs over the tunnel directly against `api.anthropic.com` and `platform.claude.com`.

**Programmatic:**

```go
// Explicit proxy URL. Credentials in the URL become Basic proxy auth.
client, _ := kraube.NewClient(ctx,
    kraube.WithTokenFile(""),
    kraube.WithProxy("http://user:pass@proxy.example.com:8080"),
)

// SOCKS5 (username/password optional)
client, _ := kraube.NewClient(ctx,
    kraube.WithTokenFile(""),
    kraube.WithProxy("socks5://127.0.0.1:1080"),
)

// No explicit option — HTTPS_PROXY / ALL_PROXY are picked up from the
// environment automatically.
client, _ := kraube.NewClient(ctx, kraube.WithTokenFile(""))

// Force a direct connection, ignoring env variables.
client, _ := kraube.NewClient(ctx,
    kraube.WithTokenFile(""),
    kraube.WithProxy(""),
)
```

Supported schemes: `http`, `https`, `socks5`, `socks5h`. Any other scheme is a hard error rather than a silent fallback. A bare `host:port` is tolerated and assumed to be `http://`.

**Standalone auth calls without a Client instance** (`Login`, `LoginManual`, top-level `FetchProfile`) go through a package-level HTTP client. Point it through a proxy with `SetAuthHTTPClient`:

```go
hc, _ := kraube.NewProxiedHTTPClient("http://proxy:8080")
kraube.SetAuthHTTPClient(hc)

creds, _ := kraube.LoginManual(ctx, readCode)
```

`NewProxiedHTTPClient("")` returns a client that follows `HTTPS_PROXY` / `ALL_PROXY` from the environment. Passing `nil` to `SetAuthHTTPClient` restores `http.DefaultClient`. For the normal `kraube.NewClient(..., WithProxy(...))` path this helper is not required — refresh is already covered by `WithProxy`.

**CLI:**

```bash
kraube --proxy http://user:pass@proxy.example.com:8080 "hi"
kraube --proxy socks5://127.0.0.1:1080 stream "tell me a joke"

# Or via environment — no flag needed:
HTTPS_PROXY=http://proxy:8080 kraube "hi"
```

The `--proxy` flag applies to every subcommand (`login`, `usage`, `query`, `stream`) and is installed for OAuth calls as well, so `kraube login --proxy ...` also goes through the proxy.

## Serve — local daemon

`kraube serve` runs a permanently-alive local HTTP gateway in front of the Anthropic Messages API. Two problems it solves:

1. **Tokens never go stale.** The OAuth access token is refreshed lazily by default — at request time, only within 60 seconds of expiry. The daemon refreshes proactively in the background (`--refresh-margin`, default 10 minutes ahead), so requests never pay refresh latency and never race an expiring token. Failed refreshes retry with backoff (30s → 1m → 5m) and never crash the process.
2. **One owner for `credentials.json`.** The refresh token is single-use: every refresh rotates it. With many short-lived processes sharing the file, rotation relies on file locking; with the daemon, exactly one long-lived process owns the credentials and everyone else talks HTTP.

```bash
kraube serve                                   # 127.0.0.1:8787
kraube serve --listen 127.0.0.1:9000 --refresh-margin 15m
kraube serve --listen 0.0.0.0:8787 --auth-key s3cret   # non-loopback requires a key
```

Endpoints:

| Endpoint | Description |
|----------|-------------|
| `POST /v1/messages` | Proxy to Anthropic. All OAuth injection (identity preamble, billing header, metadata, beta headers) is applied server-side; with `"stream": true` the SSE bytes are passed through raw, flushed chunk-by-chunk. |
| `POST /v1/messages/count_tokens` | Proxy. |
| `GET /healthz` | Token liveness, expiry, start time/uptime, and the last *actually performed* background refresh (fields absent until one runs). `503` when the token is dead and refresh is failing. No auth required. |
| `GET /usage` | Cached subscription rate-limit windows (5h / 7d). Never makes a paid upstream call; `404` until the first proxied request populates the cache. |

```bash
curl http://127.0.0.1:8787/healthz
curl -N http://127.0.0.1:8787/v1/messages -d '{
  "model": "claude-sonnet-4-6", "max_tokens": 256, "stream": true,
  "messages": [{"role": "user", "content": "hi"}]
}'
```

Security is fail-loud: listening on a non-loopback address without `--auth-key` (or `KRAUBE_SERVE_KEY`) is refused at startup — anyone who can reach the port can spend your subscription. With a key set, all endpoints except `/healthz` require `Authorization: Bearer <key>` or `x-api-key: <key>`.

The daemon shuts down gracefully on SIGINT/SIGTERM (in-flight requests, including open streams, get a 10-second drain window). For production, a systemd unit with `Restart=always` and install instructions for both system-wide and `systemctl --user` setups ships in [`deploy/kraube-serve.service`](deploy/kraube-serve.service).

Go programs that link the library talk to the daemon via `WithGateway` — a constructor-only swap from a direct client. A gateway client carries no OAuth state at all (no credentials file, no refresh, no profile fetch), authenticates with the serve key, skips client-side injection (the daemon does it), and uses a plain direct transport so `HTTPS_PROXY` never captures daemon traffic:

```go
client, err := kraube.NewClient(ctx,
    kraube.WithGateway("http://127.0.0.1:8787", os.Getenv("KRAUBE_SERVE_KEY")),
)
// Messages.Create / Stream, errors, rate-limit tracking — unchanged.
```

Library consumers can also embed the daemon itself via `kraube.NewServer(client, kraube.ServerConfig{...})`, or just reuse the keepalive primitive in their own long-lived processes:

```go
// Refresh proactively when less than 10 minutes of token lifetime remain.
err := client.EnsureFresh(ctx, 10*time.Minute)
exp, ok := client.AccessExpiry() // current expiresAt, when known
```

## Architecture

```
Your code ──▶ kraube.Client
                  │
          ┌───────┴──��────┐
          ▼               ▼
    TokenProvider     HTTP Transport
    (any source)      (Chrome TLS)
          │               │
          ▼               ▼
    OAuth Token    api.anthropic.com
                   + billing header
                   + metadata.user_id
                   + beta headers
```

## CLI

```bash
go build -o kraube ./cmd/kraube/

kraube login                          # OAuth via browser
kraube "What is Go?"                  # send a message
kraube stream "Tell me..."            # stream response
kraube usage                          # subscription limits
kraube serve                          # local HTTP daemon (proxy + keepalive)
kraube version                        # print the binary version (also --version / -v)
kraube --debug "prompt"               # verbose logging
kraube --proxy http://user:pass@host:8080 "prompt"   # route via proxy
```

Available flags: `--debug`, `--proxy URL`, `--version`/`-v`, `--help`/`-h`, `--out PATH` (login only), and for `serve`: `--listen ADDR`, `--auth-key KEY`, `--refresh-margin DURATION`. The client also honors `HTTPS_PROXY` / `ALL_PROXY`, `KRAUBE_CREDENTIALS_PATH`, and `KRAUBE_SERVE_KEY` from the environment. Any unrecognized flag is rejected with an error and exit code 1 — it is never sent to the API as a prompt.

## Documentation

- [Usage Examples](docs/usage.md) — full code examples
- [Architecture](docs/architecture.md) �� project structure
- [Principles](docs/principles.md) — design decisions
- [Protocol](docs/protocol.md) — Claude Code CLI HTTP protocol
- [API Coverage](docs/api-coverage.md) — what's implemented
- [Changelog](CHANGELOG.md) — version history

## License

MIT
