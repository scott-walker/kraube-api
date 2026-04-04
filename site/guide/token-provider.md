# TokenProvider

The `TokenProvider` interface is the single abstraction for authentication in Kraube API. The client calls `Token()` before each request.

```go
type TokenProvider interface {
    Token(ctx context.Context) (*Credentials, error)
}
```

## Built-in Providers

### WithCredentialsFile

Loads credentials from a JSON file. Refreshed tokens are saved back.

```go
client, err := kraube.NewClient(ctx, kraube.WithCredentialsFile(""))
// "" = default path: ~/.config/kraube/credentials.json
```

### WithAccessToken

Static token, no refresh. Use when you manage lifecycle externally.

```go
client, err := kraube.NewClient(ctx, kraube.WithAccessToken("eyJhbGci..."))
```

### WithCredentials

Credentials struct with optional refresh token. Auto-refreshes when expired.

```go
client, err := kraube.NewClient(ctx, kraube.WithCredentials(&kraube.Credentials{
    AccessToken:  "eyJhbGci...",
    RefreshToken: "dGhpcyBp...",
    ExpiresAt:    1712345678000, // unix ms
}))
```

### WithEnvToken

Reads access token from an environment variable on each call.

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

func (v *VaultProvider) Token(ctx context.Context) (*kraube.Credentials, error) {
    secret, err := v.client.KVv2("secret").Get(ctx, v.path)
    if err != nil {
        return nil, err
    }
    return &kraube.Credentials{
        AccessToken:  secret.Data["access_token"].(string),
        RefreshToken: secret.Data["refresh_token"].(string),
        ExpiresAt:    secret.Data["expires_at"].(int64),
    }, nil
}
```

## Provider Comparison

| Option | Provider | Auto-refresh | Persistence |
|--------|----------|:---:|:---:|
| `WithCredentialsFile(path)` | `FileTokenProvider` | Yes | Saves to disk |
| `WithAccessToken(token)` | `StaticTokenProvider` | No | None |
| `WithCredentials(creds)` | `CredentialsProvider` | Yes | In-memory only |
| `WithEnvToken(envVar)` | `EnvTokenProvider` | No | Reads env each call |
| `WithTokenProvider(p)` | Custom | Up to you | Up to you |
