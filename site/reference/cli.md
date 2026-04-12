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
| `--debug` | all commands | Verbose debug logging to stderr |
| `--out PATH` | `login` only | Write credentials to a custom path |

## Environment

| Variable | Description |
|----------|-------------|
| `KRAUBE_DEBUG=1` | Enable debug logging (same as `--debug`) |
| `KRAUBE_CREDENTIALS_PATH` | Override the default credentials path globally (honored by both the CLI and `WithTokenFile("")`) |
