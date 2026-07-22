# Serve Daemon

`kraube serve` runs a permanently-alive local HTTP gateway in front of the Anthropic Messages API. Any process on the machine — Python scripts, curl, another service — talks plain HTTP to localhost; the daemon handles OAuth, protocol injection, and token rotation.

```bash
kraube serve            # listens on 127.0.0.1:8787
```

## Why a daemon

Two problems that lazy, per-process token handling cannot solve:

1. **Stale tokens.** By default the access token is refreshed lazily, at request time, only within 60 seconds of expiry. A request that lands right at the boundary pays the refresh round-trip; a process that wakes up after a long sleep starts with a dead token. The daemon refreshes proactively in the background — when less than `--refresh-margin` (default 10 minutes) of lifetime remains — so the token on disk never approaches expiry.

2. **Refresh-token contention.** The refresh token is single-use: every refresh rotates it and invalidates the old one. `WithTokenFile` coordinates parallel processes with a file lock, but the safest topology is exactly one long-lived owner of `credentials.json`. With the daemon, that owner is `kraube serve`, and everything else is an HTTP client.

## Endpoints

| Endpoint | Auth | Description |
|----------|:---:|-------------|
| `POST /v1/messages` | key | Proxy to Anthropic |
| `POST /v1/messages/count_tokens` | key | Proxy to Anthropic |
| `GET /healthz` | — | Liveness + keepalive state |
| `GET /usage` | key | Cached rate-limit windows |

("key" applies only when `--auth-key` / `KRAUBE_SERVE_KEY` is set.)

### Proxying

The request body is decoded into a `MessageRequest` and sent through the same pipeline as the Go library, so all OAuth-gate requirements — the Claude Code identity preamble, the billing header, `metadata.user_id`, model-specific beta headers, the Chrome TLS fingerprint — are applied automatically. Requests forwarded without them would come back as `403` / `429`.

The upstream response is copied through verbatim. With `"stream": true`, that means raw SSE bytes flushed to the client chunk-by-chunk as they arrive — the daemon does not re-parse or re-serialize events:

```bash
curl -N http://127.0.0.1:8787/v1/messages -d '{
  "model": "claude-sonnet-4-6",
  "max_tokens": 256,
  "stream": true,
  "messages": [{"role": "user", "content": "Tell me a story"}]
}'
```

### Health

```bash
curl http://127.0.0.1:8787/healthz
```

```json
{
  "status": "ok",
  "uptime": "2h13m5s",
  "access_token_live": true,
  "expires_at": "2026-07-22T16:47:35+03:00",
  "expires_in": "54m10s",
  "last_refresh_at": "2026-07-22T15:48:01+03:00",
  "last_refresh_ok": true
}
```

`503` is returned only in the "requests will fail and the daemon cannot fix it" state: the token is dead **and** the last background refresh failed. A dead token right after startup is `"degraded"` but `200` — the keepalive is about to repair it. `/healthz` never requires the auth key, so systemd and monitoring probes stay simple.

### Usage

`GET /usage` serves the subscription rate-limit windows (5-hour session, 7-day weekly) from cache. The data arrives for free in response headers of proxied calls; the endpoint never makes a paid upstream probe. Until the first proxied call populates the cache it returns `404`.

## Keepalive semantics

The background loop checks once a minute and calls `Client.EnsureFresh(ctx, margin)`:

- Token outlives the margin → no-op.
- Token expires within the margin → refresh under the usual cross-process file lock, with a disk re-read (another process's rotation is reused) and the writability check that refuses to rotate when `credentials.json` cannot be written back — a rotation that isn't persisted would burn the refresh token for everyone.
- Refresh fails → logged, retried with backoff (30s → 1m → 5m). The process never exits over a refresh failure: a *failed* refresh does not consume the refresh token, so retrying is always safe.

`EnsureFresh` and `AccessExpiry` are exported on `Client`, so a library consumer running its own long-lived process can implement the same policy without the HTTP server:

```go
// somewhere in your service's background loop:
if err := client.EnsureFresh(ctx, 10*time.Minute); err != nil {
    log.Printf("token refresh failed (will retry): %v", err)
}
```

## Security

Fail-loud by design: binding to a non-loopback address without an auth key is refused at startup, because anyone who can reach the port can spend your subscription.

```bash
kraube serve --listen 0.0.0.0:8787                 # refused
kraube serve --listen 0.0.0.0:8787 --auth-key KEY  # ok
KRAUBE_SERVE_KEY=KEY kraube serve --listen 0.0.0.0:8787  # ok
```

With a key set, clients authenticate with either header:

```bash
curl -H "Authorization: Bearer KEY" http://host:8787/v1/messages -d '...'
curl -H "x-api-key: KEY"            http://host:8787/v1/messages -d '...'
```

Comparison is constant-time; failures return an Anthropic-shaped `authentication_error` JSON body.

## Running under systemd

A ready unit ships in [`deploy/kraube-serve.service`](https://github.com/scott-walker/kraube-api/blob/main/deploy/kraube-serve.service) with `Restart=always` and `After=network-online.target`. The usual per-user install (credentials in `~/.config/kraube`):

```bash
mkdir -p ~/.config/systemd/user
cp deploy/kraube-serve.service ~/.config/systemd/user/
# remove the User=/Group= lines from the unit, then:
systemctl --user daemon-reload
systemctl --user enable --now kraube-serve
loginctl enable-linger $USER   # keep it running after logout
```

For a system-wide install, copy the unit to `/etc/systemd/system/`, set `User=`/`Group=` to the account owning the credentials file (or point `KRAUBE_CREDENTIALS_PATH` elsewhere), and `systemctl enable --now kraube-serve`.

The daemon shuts down gracefully on SIGINT/SIGTERM: the keepalive stops and in-flight requests — including open SSE streams — get a 10-second drain window.
