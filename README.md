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

Kraube can route all API traffic — both `/v1/messages` and OAuth endpoints — through an HTTP, HTTPS, or SOCKS5 proxy. The Chrome TLS fingerprint is preserved end-to-end: the proxy only sees a plain TCP hop, and the uTLS handshake runs over the tunnel directly against `api.anthropic.com`.

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

**Standalone auth calls** (`LoginManual`, `FetchProfile`) use a package-level HTTP client. Point it through a proxy with `SetAuthHTTPClient`:

```go
hc, _ := kraube.NewProxiedHTTPClient("http://proxy:8080")
kraube.SetAuthHTTPClient(hc)

creds, _ := kraube.LoginManual(ctx, readCode)
```

`NewProxiedHTTPClient("")` returns a client that follows `HTTPS_PROXY` / `ALL_PROXY` from the environment. Passing `nil` to `SetAuthHTTPClient` restores `http.DefaultClient`.

**CLI:**

```bash
kraube --proxy http://user:pass@proxy.example.com:8080 "hi"
kraube --proxy socks5://127.0.0.1:1080 stream "tell me a joke"

# Or via environment — no flag needed:
HTTPS_PROXY=http://proxy:8080 kraube "hi"
```

The `--proxy` flag applies to every subcommand (`login`, `usage`, `query`, `stream`) and is installed for OAuth calls as well, so `kraube login --proxy ...` also goes through the proxy.

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
kraube --debug "prompt"               # verbose logging
kraube --proxy http://user:pass@host:8080 "prompt"   # route via proxy
```

Available flags: `--debug`, `--proxy URL`, `--out PATH` (login only). The client also honors `HTTPS_PROXY` / `ALL_PROXY` and `KRAUBE_CREDENTIALS_PATH` from the environment.

## Documentation

- [Usage Examples](docs/usage.md) — full code examples
- [Architecture](docs/architecture.md) �� project structure
- [Principles](docs/principles.md) — design decisions
- [Protocol](docs/protocol.md) — Claude Code CLI HTTP protocol
- [API Coverage](docs/api-coverage.md) — what's implemented
- [Changelog](CHANGELOG.md) — version history

## License

MIT
