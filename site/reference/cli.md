# CLI Reference

## Build

```bash
go build -o kraube ./cmd/kraube/
```

## Commands

### login

Authenticate via OAuth browser flow. Credentials are written to `~/.config/kraube/credentials.json` (JSON: `refreshToken`, `accessToken`, `expiresAt`) with `0600` permissions. Override the path with `--out PATH` or the `KRAUBE_CREDENTIALS_PATH` environment variable.

```bash
kraube login
kraube login --out /etc/kraube/app-a.json
KRAUBE_CREDENTIALS_PATH=/etc/kraube/app-b.json kraube login
```

A single `kraube login` on a machine is enough for any number of parallel processes using `WithTokenFile("")` — refresh is coordinated via an OS-level file lock.

### query (default)

Send a message and print the response.

```bash
kraube "What is Go?"
```

### stream

Stream the response via SSE.

```bash
kraube stream "Tell me a story"
```

### usage

Show subscription rate limits.

```bash
kraube usage
```

## Flags

| Flag | Scope | Description |
|------|-------|-------------|
| `--debug` | all commands | Verbose debug logging to stderr. Includes full `api: error response` dumps (status, URL, local/remote addresses, proxy, redacted headers, request & response bodies). |
| `--proxy URL` | all commands | Route all outbound traffic (API + OAuth) through a proxy. Schemes: `http`, `https`, `socks5`, `socks5h`. Credentials in the URL are used for Basic proxy auth. When omitted, `HTTPS_PROXY` / `ALL_PROXY` from the environment are honored automatically. |
| `--out PATH` | `login` only | Write credentials to a custom path |

```bash
kraube --proxy http://user:pass@proxy.example.com:8080 "hi"
kraube --proxy socks5://127.0.0.1:1080 stream "tell me a story"
HTTPS_PROXY=http://proxy:8080 kraube login    # env is enough — no flag needed
```

## Environment

| Variable | Description |
|----------|-------------|
| `KRAUBE_DEBUG=1` | Enable debug logging (same as `--debug`) |
| `KRAUBE_CREDENTIALS_PATH` | Override the default credentials path globally (honored by both the CLI and `WithTokenFile("")`) |
| `HTTPS_PROXY` / `https_proxy` | Proxy URL used when `--proxy` is not set (checked first) |
| `ALL_PROXY` / `all_proxy` | Fallback proxy URL when `HTTPS_PROXY` is absent |
