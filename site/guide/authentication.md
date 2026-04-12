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
6. Saves credentials to `~/.config/kraube/credentials.json`

Override the path with `--out PATH` or the `KRAUBE_CREDENTIALS_PATH` environment variable. Both `kraube login` and `WithTokenFile("")` honor the same env var, so a single override covers the whole stack.

## Credentials file

The file is JSON with `0600` permissions:

```json
{
  "refreshToken": "...",
  "accessToken": "...",
  "expiresAt": 1760000000000
}
```

Storing the live `accessToken` (and its `expiresAt`) means any process on the machine can start using Claude immediately without a refresh round-trip, as long as the cached access token is still valid.

## Auto-refresh and parallel processes

Access tokens are refreshed 60 seconds before expiry.

When you use `WithTokenFile`, refresh is coordinated across processes via an OS-level file lock (`flock(2)` on Linux/macOS, `LockFileEx` on Windows). The flow:

1. Fast path — if the in-memory access token is still live, return it.
2. Acquire the file lock.
3. Re-read the credentials file under the lock — another process may have just refreshed.
4. If the on-disk access token is live, use it.
5. Otherwise, exchange the refresh token, write the new credentials atomically, release the lock.

This means you can run any number of kraube-api processes on one machine against a single `kraube login`, and refresh happens exactly once per rotation.

## Headless Login

For SSH or headless environments, `LoginManual` prints the URL and waits for the user to paste the code. It returns a `*Credentials` that you pass to `SaveCredentials`:

```go
creds, err := kraube.LoginManual(ctx, func(authURL string) (string, error) {
    fmt.Println("Open:", authURL)
    fmt.Print("Paste code: ")
    // read from stdin...
})
if err != nil {
    return err
}
return kraube.SaveCredentials(kraube.DefaultCredentialsPath(), creds)
```
