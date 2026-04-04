# Authentication

Kraube API uses OAuth Bearer tokens from claude.ai subscriptions (Pro/Max/Team).

## OAuth Flow

The CLI provides a browser-based OAuth login:

```bash
kraube login
```

This performs an OAuth Authorization Code flow with PKCE:

1. Generates PKCE verifier and challenge
2. Opens browser to `https://claude.com/cai/oauth/authorize`
3. User authorizes in browser
4. Callback receives authorization code
5. Exchanges code for access + refresh tokens
6. Saves credentials to `~/.config/kraube/credentials.json`

## Credentials Format

```json
{
  "access_token": "eyJhbGci...",
  "refresh_token": "dGhpcyBp...",
  "expires_at": 1712345678000
}
```

`expires_at` is a Unix timestamp in **milliseconds**.

## Auto-refresh

Providers with a refresh token automatically refresh when the access token expires (60-second buffer). `FileTokenProvider` saves refreshed credentials back to disk.

## Headless Login

For SSH or headless environments, `LoginManual` prints the URL and waits for the user to paste the code:

```go
creds, err := kraube.LoginManual(ctx, func(authURL string) (string, error) {
    fmt.Println("Open:", authURL)
    fmt.Print("Paste code: ")
    // read from stdin...
})
```
