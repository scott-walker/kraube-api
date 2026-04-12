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
| `WithHTTPClient(hc)` | Custom `*http.Client`. Default: Chrome TLS transport. |
| `WithBaseURL(url)` | Override API base URL. Default: `https://api.anthropic.com` |

## Behavior

| Option | Description |
|--------|-------------|
| `WithoutProfile()` | Skip fetching OAuth profile on init. Faster startup. |
