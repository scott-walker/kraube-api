# Options Reference

All options for `kraube.NewClient(ctx, ...Option)`.

## Token Source

| Option | Description |
|--------|-------------|
| `WithTokenFile(path)` | Load from file. `""` = default path. |
| `WithToken(token)` | Direct token. |
| `WithEnvToken(envVar)` | Read token from environment variable. |
| `WithTokenProvider(p)` | Custom TokenProvider implementation. |

## HTTP

| Option | Description |
|--------|-------------|
| `WithHTTPClient(hc)` | Custom `*http.Client`. Default: Chrome TLS transport. |
| `WithBaseURL(url)` | Override API base URL. Default: `https://api.anthropic.com` |

## Behavior

| Option | Description |
|--------|-------------|
| `WithoutProfile()` | Skip fetching OAuth profile on init. Faster startup. |
