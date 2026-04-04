# Options Reference

All options for `kraube.NewClient(ctx, ...Option)`.

## Token Source

| Option | Description |
|--------|-------------|
| `WithCredentialsFile(path)` | Load from JSON file. `""` = default path. Refreshes saved back. |
| `WithAccessToken(token)` | Static access token. No refresh. |
| `WithCredentials(creds)` | Credentials struct. Auto-refresh if refresh_token present. |
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
