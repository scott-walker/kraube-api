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
	oauthAuthorizeURL = "https://claude.com/cai/oauth/authorize"
	oauthTokenURL     = "https://platform.claude.com/v1/oauth/token"
	oauthClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	oauthRedirectBase = "http://localhost"
	oauthScopes       = "user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
	oauthBeta         = "oauth-2025-04-20"
)

// Credentials holds OAuth tokens for Anthropic API access.
type Credentials struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"` // unix ms
}

// IsExpired returns true if the token has expired or expires within 60 seconds.
func (c *Credentials) IsExpired() bool {
	return time.Now().UnixMilli() >= c.ExpiresAt-60_000
}

// Login performs the full OAuth authorization_code flow with PKCE.
// It opens the browser and waits for the callback.
// Returns credentials on success.
func Login(ctx context.Context, openBrowser func(url string) error) (*Credentials, error) {
	logInfo("oauth: starting login flow")

	// Generate PKCE
	verifier, challenge, err := generatePKCE()
	if err != nil {
		return nil, fmt.Errorf("generate PKCE: %w", err)
	}
	logDebug("oauth: PKCE generated", "challenge_len", len(challenge), "verifier_len", len(verifier))

	state, err := randomString(32)
	if err != nil {
		return nil, fmt.Errorf("generate state: %w", err)
	}

	// Start local server to receive callback
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	defer listener.Close()

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
			fmt.Fprintf(w, "<html><body><h1>Authentication failed</h1><p>%s</p></body></html>", errMsg)
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
		fmt.Fprint(w, "<html><body><h1>Authentication successful</h1><p>You can close this window.</p></body></html>")
	})

	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	defer server.Shutdown(context.Background())

	// Open browser
	logDebug("oauth: opening browser")
	if err := openBrowser(authURL); err != nil {
		return nil, fmt.Errorf("open browser: %w", err)
	}
	logDebug("oauth: browser opened, waiting for callback...")

	// Wait for code or error
	var code string
	select {
	case code = <-codeCh:
		logDebug("oauth: code received, exchanging for tokens")
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		logWarn("oauth: login cancelled by context")
		return nil, ctx.Err()
	}

	// Exchange code for tokens
	creds, err := exchangeCode(ctx, code, verifier, redirectURI, state)
	if err != nil {
		return nil, err
	}
	logInfo("oauth: login successful", "expires_at", creds.ExpiresAt)
	return creds, nil
}

// RefreshAccessToken refreshes an expired access token.
func RefreshAccessToken(ctx context.Context, creds *Credentials) (*Credentials, error) {
	logDebug("oauth: refreshing access token", "token_url", oauthTokenURL)

	body := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": creds.RefreshToken,
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
	defer resp.Body.Close()

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

	newCreds := &Credentials{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    time.Now().UnixMilli() + int64(result.ExpiresIn)*1000,
	}
	if newCreds.RefreshToken == "" {
		newCreds.RefreshToken = creds.RefreshToken
	}
	logInfo("oauth: token refreshed", "expires_in", result.ExpiresIn, "new_expires_at", newCreds.ExpiresAt)
	return newCreds, nil
}

// --- Credentials persistence ---

// DefaultCredentialsPath returns ~/.config/kraube/credentials.json.
func DefaultCredentialsPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "kraube", "credentials.json")
}

// SaveCredentials writes credentials to disk.
func SaveCredentials(path string, creds *Credentials) error {
	logDebug("credentials: saving", "path", path)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	logDebug("credentials: saved", "path", path)
	return nil
}

// LoadCredentials reads credentials from disk.
func LoadCredentials(path string) (*Credentials, error) {
	logDebug("credentials: loading", "path", path)
	data, err := os.ReadFile(path)
	if err != nil {
		logDebug("credentials: file not found", "path", path, "error", err)
		return nil, err
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		logError("credentials: invalid JSON", "path", path, "error", err)
		return nil, err
	}
	logDebug("credentials: loaded", "path", path, "expired", creds.IsExpired(), "expires_at", creds.ExpiresAt)
	return &creds, nil
}

// LoadClaudeCredentials reads credentials from Claude Code's config.
// It searches ~/.claude/{profile}/.credentials.json for the newest valid credentials.
func LoadClaudeCredentials() (*Credentials, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	claudeDir := filepath.Join(home, ".claude")
	logDebug("claude: scanning profiles", "dir", claudeDir)

	// Search all profile directories for .credentials.json
	entries, err := os.ReadDir(claudeDir)
	if err != nil {
		return nil, fmt.Errorf("read ~/.claude: %w", err)
	}

	var best *Credentials
	var bestSource string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(claudeDir, entry.Name(), ".credentials.json")
		creds, err := loadClaudeCredFile(path)
		if err != nil {
			logDebug("claude: profile skipped", "profile", entry.Name(), "error", err)
			continue
		}
		logDebug("claude: profile found", "profile", entry.Name(), "expires_at", creds.ExpiresAt, "expired", creds.IsExpired())
		if best == nil || creds.ExpiresAt > best.ExpiresAt {
			best = creds
			bestSource = entry.Name()
		}
	}

	// Also check root ~/.claude/.credentials.json
	rootPath := filepath.Join(claudeDir, ".credentials.json")
	if creds, err := loadClaudeCredFile(rootPath); err == nil {
		logDebug("claude: root credentials found", "expires_at", creds.ExpiresAt)
		if best == nil || creds.ExpiresAt > best.ExpiresAt {
			best = creds
			bestSource = "root"
		}
	}

	if best == nil {
		logError("claude: no credentials found", "dir", claudeDir)
		return nil, fmt.Errorf("no OAuth credentials found in ~/.claude/")
	}
	logInfo("claude: credentials selected", "source", bestSource, "expires_at", best.ExpiresAt, "expired", best.IsExpired())
	return best, nil
}

func loadClaudeCredFile(path string) (*Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Try claudeAiOauth wrapper first
	var wrapped struct {
		ClaudeAiOauth *claudeOAuthCreds `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return nil, err
	}

	var raw *claudeOAuthCreds
	if wrapped.ClaudeAiOauth != nil {
		raw = wrapped.ClaudeAiOauth
	} else {
		// Try direct format
		var direct claudeOAuthCreds
		if err := json.Unmarshal(data, &direct); err != nil {
			return nil, err
		}
		if direct.AccessToken == "" {
			return nil, fmt.Errorf("no credentials")
		}
		raw = &direct
	}

	return &Credentials{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		ExpiresAt:    raw.ExpiresAt,
	}, nil
}

type claudeOAuthCreds struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    int64  `json:"expiresAt"`
}

// --- Token exchange ---

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

func exchangeCode(ctx context.Context, code, verifier, redirectURI, state string) (*Credentials, error) {
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
	defer resp.Body.Close()

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

	creds := &Credentials{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    time.Now().UnixMilli() + int64(result.ExpiresIn)*1000,
	}
	logInfo("oauth: tokens received", "expires_in", result.ExpiresIn, "elapsed", elapsed)
	return creds, nil
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
	defer resp.Body.Close()

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
