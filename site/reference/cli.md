# CLI Reference

## Build

```bash
go build -o kraube ./cmd/kraube/
```

## Commands

### login

Authenticate via OAuth browser flow.

```bash
kraube login
```

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

| Flag | Description |
|------|-------------|
| `--debug` | Enable verbose debug logging to stderr |

Environment: `KRAUBE_DEBUG=1` also enables debug logging.
