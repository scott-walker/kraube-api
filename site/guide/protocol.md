# Protocol Details

Kraube API replicates the Claude Code CLI's HTTP protocol for OAuth subscription access. See the full [protocol documentation](https://github.com/scott-walker/kraube-api/blob/main/docs/protocol.md) in the repository.

## Key Protocol Elements

### Endpoint

```
POST https://api.anthropic.com/v1/messages?beta=true
```

### Required Headers

| Header | Value |
|--------|-------|
| `Authorization` | `Bearer <access_token>` |
| `anthropic-dangerous-direct-browser-access` | `true` |
| `Anthropic-Version` | `2023-06-01` |
| `User-Agent` | `claude-cli/2.1.92 (external, cli)` |
| `x-app` | `cli` |
| `Anthropic-Beta` | Model-specific beta features |

### Billing Header

A system prompt block is injected automatically:

```
x-anthropic-billing-header: cc_version=2.1.92.00a; cc_entrypoint=cli; cch=<hash>;
```

Without this, Sonnet and Opus models return 429.

### Chrome TLS

OAuth requests require a Chrome TLS fingerprint (via uTLS). Cloudflare blocks Go's default TLS fingerprint.
