package kraube

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	oauthAuthorizeURL   = "https://claude.com/cai/oauth/authorize"
	oauthClientID       = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	oauthRedirectBase   = "http://localhost"
	oauthManualRedirect = "https://platform.claude.com/oauth/code/callback"
	oauthScopes         = "user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
	oauthBeta           = "oauth-2025-04-20"
)

// oauthTokenURL is a var (not const) so tests can override it with a mock server.
var oauthTokenURL = "https://platform.claude.com/v1/oauth/token"

// setOAuthTokenURL overrides the token URL (for testing).
func setOAuthTokenURL(url string) { oauthTokenURL = url }

// oauthTokens is the internal representation of an OAuth token pair.
// Access tokens are short-lived (~1 hour) and managed in memory.
// Refresh tokens are long-lived and persisted to disk.
type oauthTokens struct {
	accessToken  string
	refreshToken string
	expiresAt    int64 // unix ms
}

func (t *oauthTokens) isExpired() bool {
	return t.accessToken == "" || time.Now().UnixMilli() >= t.expiresAt-60_000
}

// Login performs the full OAuth authorization_code flow with PKCE.
// It opens the browser and waits for the callback.
// Returns the token for storage on success.
func Login(ctx context.Context, openBrowser func(url string) error) (string, error) {
	logInfo("oauth: starting login flow")

	// Generate PKCE
	verifier, challenge, err := generatePKCE()
	if err != nil {
		return "", fmt.Errorf("generate PKCE: %w", err)
	}
	logDebug("oauth: PKCE generated", "challenge_len", len(challenge), "verifier_len", len(verifier))

	state, err := randomString(32)
	if err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}

	// Start local server to receive callback
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return "", fmt.Errorf("listen: %w", err)
	}
	defer func() { _ = listener.Close() }()

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("%s:%d/callback", oauthRedirectBase, port)
	logDebug("oauth: callback server started", "port", port, "redirect_uri", redirectURI)

	// Build authorization URL
	params := url.Values{
		"code":                  {"true"},
		"client_id":             {oauthClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"scope":                 {oauthScopes},
		"state":                 {state},
	}
	authURL := oauthAuthorizeURL + "?" + params.Encode()
	logDebug("oauth: authorization URL built", "url", authURL)

	// Channel to receive the auth code
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		logDebug("oauth: callback received", "query", r.URL.RawQuery)
		if r.URL.Query().Get("state") != state {
			logError("oauth: state mismatch in callback")
			errCh <- fmt.Errorf("state mismatch")
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			desc := r.URL.Query().Get("error_description")
			logError("oauth: authorization error", "error", errMsg, "description", desc)
			errCh <- fmt.Errorf("oauth error: %s: %s", errMsg, desc)
			_, _ = fmt.Fprintf(w, "<html><body><h1>Authentication failed</h1><p>%s</p></body></html>", errMsg)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			logError("oauth: no code in callback")
			errCh <- fmt.Errorf("no code in callback")
			http.Error(w, "no code", http.StatusBadRequest)
			return
		}
		logDebug("oauth: authorization code received", "code_len", len(code))
		codeCh <- code
		_, _ = fmt.Fprint(w, "<html><body><h1>Authentication successful</h1><p>You can close this window.</p></body></html>")
	})

	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Shutdown(context.Background()) }()

	// Open browser
	logDebug("oauth: opening browser")
	if err := openBrowser(authURL); err != nil {
		return "", fmt.Errorf("open browser: %w", err)
	}
	logDebug("oauth: browser opened, waiting for callback...")

	// Wait for code or error
	var code string
	select {
	case code = <-codeCh:
		logDebug("oauth: code received, exchanging for tokens")
	case err := <-errCh:
		return "", err
	case <-ctx.Done():
		logWarn("oauth: login cancelled by context")
		return "", ctx.Err()
	}

	// Exchange code for tokens
	tokens, err := exchangeCode(ctx, code, verifier, redirectURI, state)
	if err != nil {
		return "", err
	}
	logInfo("oauth: login successful")
	return tokens.refreshToken, nil
}

// LoginManual performs OAuth login without a local callback server.
// It prints the authorization URL and waits for the user to paste the code.
// Ideal for headless/SSH environments.
//
// The readCode function should prompt the user and return the pasted code.
func LoginManual(ctx context.Context, readCode func(authURL string) (string, error)) (string, error) {
	logInfo("oauth: starting manual login flow")

	verifier, challenge, err := generatePKCE()
	if err != nil {
		return "", fmt.Errorf("generate PKCE: %w", err)
	}

	state, err := randomString(32)
	if err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}

	params := url.Values{
		"code":                  {"true"},
		"client_id":             {oauthClientID},
		"redirect_uri":          {oauthManualRedirect},
		"response_type":         {"code"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"scope":                 {oauthScopes},
		"state":                 {state},
	}
	authURL := oauthAuthorizeURL + "?" + params.Encode()
	logDebug("oauth: manual authorization URL built", "url", authURL)

	raw, err := readCode(authURL)
	if err != nil {
		return "", fmt.Errorf("read code: %w", err)
	}
	code := extractAuthCode(strings.TrimSpace(raw))
	if code == "" {
		return "", fmt.Errorf("could not extract authorization code from input")
	}
	logDebug("oauth: manual code extracted", "code_len", len(code))

	tokens, err := exchangeCode(ctx, code, verifier, oauthManualRedirect, state)
	if err != nil {
		return "", err
	}
	logInfo("oauth: manual login successful")
	return tokens.refreshToken, nil
}

// refreshAccessToken exchanges a refresh token for a new access token.
func refreshAccessToken(ctx context.Context, refreshToken string) (*oauthTokens, error) {
	logDebug("oauth: refreshing access token", "token_url", oauthTokenURL)

	body := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     oauthClientID,
		"scope":         oauthScopes,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", oauthTokenURL, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		logError("oauth: refresh request failed", "elapsed", elapsed, "error", err)
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	logDebug("oauth: refresh response", "status", resp.StatusCode, "elapsed", elapsed)

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		logError("oauth: refresh failed", "status", resp.StatusCode, "body", string(respBody))
		return nil, fmt.Errorf("token refresh failed: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	tokens := &oauthTokens{
		accessToken:  result.AccessToken,
		refreshToken: result.RefreshToken,
		expiresAt:    time.Now().UnixMilli() + int64(result.ExpiresIn)*1000,
	}
	if tokens.refreshToken == "" {
		tokens.refreshToken = refreshToken
	}
	logInfo("oauth: token refreshed", "expires_in", result.ExpiresIn)
	return tokens, nil
}

// --- Token persistence ---

// DefaultTokenPath returns ~/.config/kraube/token.
func DefaultTokenPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "kraube", "token")
}

// SaveToken writes a token to disk with restricted permissions.
func SaveToken(path, token string) error {
	logDebug("token: saving", "path", path)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(token), 0600); err != nil {
		return err
	}
	logDebug("token: saved", "path", path)
	return nil
}

// LoadToken reads a token from disk.
func LoadToken(path string) (string, error) {
	logDebug("token: loading", "path", path)
	data, err := os.ReadFile(path)
	if err != nil {
		logDebug("token: file not found", "path", path, "error", err)
		return "", err
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("token file is empty: %s", path)
	}
	logDebug("token: loaded", "path", path)
	return token, nil
}

// --- Token exchange ---

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

func exchangeCode(ctx context.Context, code, verifier, redirectURI, state string) (*oauthTokens, error) {
	logDebug("oauth: exchanging code for tokens", "token_url", oauthTokenURL, "redirect_uri", redirectURI, "code_len", len(code))

	body := map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"code_verifier": verifier,
		"client_id":     oauthClientID,
		"redirect_uri":  redirectURI,
		"state":         state,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", oauthTokenURL, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		logError("oauth: token exchange request failed", "elapsed", elapsed, "error", err)
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	logDebug("oauth: token exchange response", "status", resp.StatusCode, "elapsed", elapsed)

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		logError("oauth: token exchange failed", "status", resp.StatusCode, "body", string(respBody), "elapsed", elapsed)
		return nil, fmt.Errorf("token exchange failed: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	tokens := &oauthTokens{
		accessToken:  result.AccessToken,
		refreshToken: result.RefreshToken,
		expiresAt:    time.Now().UnixMilli() + int64(result.ExpiresIn)*1000,
	}
	logInfo("oauth: tokens received", "expires_in", result.ExpiresIn, "elapsed", elapsed)
	return tokens, nil
}

// --- Profile ---

// Profile holds account info from the OAuth profile endpoint.
type Profile struct {
	AccountUUID    string `json:"account_uuid"`
	OrganizationUUID string `json:"organization_uuid"`
	DisplayName    string `json:"display_name"`
	Email          string `json:"email"`
}

// FetchProfile retrieves the user's profile using an OAuth access token.
func FetchProfile(ctx context.Context, accessToken string) (*Profile, error) {
	logDebug("oauth: fetching profile")

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.anthropic.com/api/oauth/profile", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch profile failed: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Account struct {
			UUID        string `json:"uuid"`
			DisplayName string `json:"display_name"`
			Email       string `json:"email"`
		} `json:"account"`
		Organization struct {
			UUID string `json:"uuid"`
		} `json:"organization"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	p := &Profile{
		AccountUUID:      result.Account.UUID,
		OrganizationUUID: result.Organization.UUID,
		DisplayName:      result.Account.DisplayName,
		Email:            result.Account.Email,
	}
	logInfo("oauth: profile fetched", "account", p.AccountUUID, "org", p.OrganizationUUID)
	return p, nil
}

// --- Code extraction ---

// extractAuthCode extracts the OAuth authorization code from user input.
// Handles all formats a user might paste:
//
//   - Raw code: "J7kB3wLmpnzZ..."
//   - Code with URL fragment: "J7kB3wLmpnzZ...#state_value"
//   - Full callback URL: "https://platform.claude.com/oauth/code/callback?code=J7kB3w...&state=..."
//   - Callback URL with fragment: "https://...?code=J7kB3w...#fragment"
//
// The function is intentionally permissive — it tries to extract a valid code
// from whatever the user provides rather than failing on unexpected input.
func extractAuthCode(input string) string {
	// Remove any surrounding whitespace, quotes, angle brackets.
	input = strings.TrimSpace(input)
	input = strings.Trim(input, "\"'<>")

	// If it looks like a URL, parse it properly.
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		if u, err := url.Parse(input); err == nil {
			if code := u.Query().Get("code"); code != "" {
				return code
			}
		}
	}

	// Strip URL fragment (code#state → code).
	if i := strings.IndexByte(input, '#'); i >= 0 {
		input = input[:i]
	}

	// Strip query string if somehow present without URL scheme.
	if i := strings.IndexByte(input, '?'); i >= 0 {
		input = input[:i]
	}

	return input
}

// --- PKCE ---

func generatePKCE() (verifier, challenge string, err error) {
	verifier, err = randomString(43)
	if err != nil {
		return
	}
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return
}

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b)[:n], nil
}
