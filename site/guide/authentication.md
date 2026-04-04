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
5. Exchanges code for tokens
6. Saves token to `~/.config/kraube/token`

## Token

After login, the token is stored as a plain string in `~/.config/kraube/token` with `0600` permissions.

The token is all you need — short-lived access tokens are obtained and refreshed automatically under the hood.

## Auto-refresh

All built-in providers automatically obtain and refresh access tokens when needed. Access tokens live only in memory and are never written to disk.

## Headless Login

For SSH or headless environments, `LoginManual` prints the URL and waits for the user to paste the code:

```go
token, err := kraube.LoginManual(ctx, func(authURL string) (string, error) {
    fmt.Println("Open:", authURL)
    fmt.Print("Paste code: ")
    // read from stdin...
})
```
