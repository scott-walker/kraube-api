# Options Reference

All options for `kraube.NewClient(ctx, ...Option)`.

## Token Source

| Option | Description |
|--------|-------------|
| `WithTokenFile(path)` | Load credentials from JSON file. `""` = `$KRAUBE_CREDENTIALS_PATH` → `~/.config/kraube/credentials.json`. Uses OS-level file lock to coordinate refresh across parallel processes. |
| `WithToken(refreshToken)` | Refresh token held in memory; rotation is not persisted. |
| `WithEnvToken(envVar)` | Refresh token from environment variable (in-memory). |
| `WithTokenProvider(p)` | Custom TokenProvider implementation. |

## HTTP

| Option | Description |
|--------|-------------|
| `WithHTTPClient(hc)` | Custom `*http.Client`. Default: Chrome TLS transport. Takes precedence over `WithProxy`. |
| `WithBaseURL(url)` | Override API base URL. Default: `https://api.anthropic.com` |
| `WithProxy(url)` | Route all traffic of the Client instance through a proxy — `/v1/messages`, initial profile fetch, and OAuth token refresh share a single transport. Schemes: `http`, `https`, `socks5`, `socks5h`. Credentials in URL become Basic proxy auth. Omit the option to auto-pick up `HTTPS_PROXY` / `ALL_PROXY`; pass `""` to force a direct connection even when env is set. |

### Standalone HTTP helpers

| Function | Description |
|----------|-------------|
| `NewProxiedHTTPClient(url)` | Build an `*http.Client` with the Kraube API Chrome-fingerprinted transport + proxy. Pass `""` to honor `HTTPS_PROXY`/`ALL_PROXY` env. |
| `SetAuthHTTPClient(c)` | Install a package-level HTTP client used by **standalone** OAuth helpers that run without a Client instance (`Login`, `LoginManual`, top-level `FetchProfile`). Pass `nil` to restore `http.DefaultClient`. Not needed when `kraube.NewClient(..., WithProxy(...))` is used — per-instance refresh already uses the proxy. |

## Behavior

| Option | Description |
|--------|-------------|
| `WithoutProfile()` | Skip fetching OAuth profile on init. Faster startup. |
