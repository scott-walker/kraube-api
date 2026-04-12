# TokenProvider

The `TokenProvider` interface is the single abstraction for authentication in Kraube API. The client calls `Token()` before each request to get a valid access token.

```go
type TokenProvider interface {
    Token(ctx context.Context) (string, error)
}
```

## Built-in Options

### WithTokenFile

Loads credentials (refresh + access + expiresAt) from a JSON file. Rotation is serialized across processes via an OS-level file lock, so this is the recommended mode for anything that runs in more than one process on the same machine.

```go
client, err := kraube.NewClient(ctx, kraube.WithTokenFile(""))
// "" = default path (honors $KRAUBE_CREDENTIALS_PATH, else ~/.config/kraube/credentials.json)
```

### WithToken

Takes a refresh token directly. Access tokens are obtained and refreshed automatically, but rotated refresh tokens are kept **in memory only** — they are lost when the process exits. Use `WithTokenFile` for persistent, multi-process setups.

```go
client, err := kraube.NewClient(ctx, kraube.WithToken("sk-ant-..."))
```

### WithEnvToken

Reads the token from an environment variable.

```go
client, err := kraube.NewClient(ctx, kraube.WithEnvToken("KRAUBE_TOKEN"))
```

### WithTokenProvider

Any custom implementation.

```go
client, err := kraube.NewClient(ctx, kraube.WithTokenProvider(myProvider))
```

## Custom Provider Example

```go
type VaultProvider struct {
    client *vault.Client
    path   string
}

func (v *VaultProvider) Token(ctx context.Context) (string, error) {
    secret, err := v.client.KVv2("secret").Get(ctx, v.path)
    if err != nil {
        return "", err
    }
    return secret.Data["token"].(string), nil
}
```

## Provider Comparison

| Option | Auto-refresh | Persistence | Multi-process safe |
|--------|:---:|:---:|:---:|
| `WithTokenFile(path)` | Yes | Writes rotated credentials to disk | Yes (file lock) |
| `WithToken(token)` | Yes | In-memory only | No |
| `WithEnvToken(envVar)` | Yes | In-memory only | No |
| `WithTokenProvider(p)` | Up to you | Up to you | Up to you |
