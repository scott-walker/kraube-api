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

Accepts the generation flags listed below (`--system`, `--history`, `--model`, `--max-tokens`, `--temperature`).

### stream

Stream the response via SSE. Stdout is flushed after every `text_delta`, so callers reading the pipe see tokens as they land — convenient for voice pipelines and other streaming consumers.

```bash
kraube stream "Tell me a story"
```

Accepts the same generation flags as `query`.

### usage

Show subscription rate limits.

```bash
kraube usage
```

## Global flags

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

## Generation flags

Available on `query`, `stream`, and the default `kraube "prompt"` invocation. When omitted, the CLI sends the same request it always did — single-turn `UserMessage`, model `claude-sonnet-4-6`, `max_tokens` 4096 — so existing scripts keep working without changes.

| Flag | Description |
|------|-------------|
| `--system TEXT` | System prompt as inline text. Populates `MessageRequest.System` via `SystemText`. |
| `--system-file PATH` | Read the system prompt from a file. Same target field as `--system`. |
| `--history PATH\|-` | Prior conversation as a JSON array `[{"role":"user\|assistant","content":"..."}, ...]`. Pass `-` to read from stdin (handy for piping history without a temp file). The prompt argument is appended as the final user message. |
| `--model NAME` | Override the model id. Default: `claude-sonnet-4-6`. |
| `--max-tokens N` | Response cap in tokens. Default: `4096`. |
| `--temperature F` | Sampling temperature `0.0..1.0`. When omitted, the library default is used. |

Multi-turn example — pipe history through stdin and continue the conversation:

```bash
echo '[
  {"role":"user","content":"My name is Scott."},
  {"role":"assistant","content":"Got it, Scott."}
]' | kraube stream \
  --system "Reply in one short sentence." \
  --history - \
  --model claude-sonnet-4-6 \
  --max-tokens 64 \
  "What is my name?"
```

Single-turn with a custom system prompt and lower temperature:

```bash
kraube --system "Reply only with the literal answer, no explanation." \
       --temperature 0.0 \
       "2 + 2 ="
```

## Environment

| Variable | Description |
|----------|-------------|
| `KRAUBE_DEBUG=1` | Enable debug logging (same as `--debug`) |
| `KRAUBE_CREDENTIALS_PATH` | Override the default credentials path globally (honored by both the CLI and `WithTokenFile("")`) |
| `HTTPS_PROXY` / `https_proxy` | Proxy URL used when `--proxy` is not set (checked first) |
| `ALL_PROXY` / `all_proxy` | Fallback proxy URL when `HTTPS_PROXY` is absent |
