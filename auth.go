package kraube

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	oauthAuthorizeURL = "https://platform.claude.com/oauth/authorize"
	oauthTokenURL     = "https://platform.claude.com/v1/oauth/token"
	oauthClientID     = "https://claude.ai/oauth/claude-code-client-metadata"
	oauthRedirectBase = "http://localhost"
	oauthScopes       = "user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
	oauthBeta         = "oauth-2025-04-20"
)

// Credentials holds OAuth tokens.
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
	// Generate PKCE
	verifier, challenge, err := generatePKCE()
	if err != nil {
		return nil, fmt.Errorf("generate PKCE: %w", err)
	}

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

	// Build authorization URL
	params := url.Values{
		"client_id":             {oauthClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"scope":                 {oauthScopes},
		"state":                 {state},
	}
	authURL := oauthAuthorizeURL + "?" + params.Encode()

	// Channel to receive the auth code
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			errCh <- fmt.Errorf("state mismatch")
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			errCh <- fmt.Errorf("oauth error: %s: %s", errMsg, r.URL.Query().Get("error_description"))
			fmt.Fprintf(w, "<html><body><h1>Authentication failed</h1><p>%s</p></body></html>", errMsg)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code in callback")
			http.Error(w, "no code", http.StatusBadRequest)
			return
		}
		codeCh <- code
		fmt.Fprint(w, "<html><body><h1>Authentication successful</h1><p>You can close this window.</p></body></html>")
	})

	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	defer server.Shutdown(context.Background())

	// Open browser
	if err := openBrowser(authURL); err != nil {
		return nil, fmt.Errorf("open browser: %w", err)
	}

	// Wait for code or error
	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Exchange code for tokens
	return exchangeCode(ctx, code, verifier, redirectURI)
}

// RefreshAccessToken refreshes an expired access token.
func RefreshAccessToken(ctx context.Context, creds *Credentials) (*Credentials, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {creds.RefreshToken},
		"client_id":     {oauthClientID},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", oauthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh failed: HTTP %d", resp.StatusCode)
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
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// LoadCredentials reads credentials from disk.
func LoadCredentials(path string) (*Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}
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

	// Search all profile directories for .credentials.json
	entries, err := os.ReadDir(claudeDir)
	if err != nil {
		return nil, fmt.Errorf("read ~/.claude: %w", err)
	}

	var best *Credentials
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		creds, err := loadClaudeCredFile(filepath.Join(claudeDir, entry.Name(), ".credentials.json"))
		if err != nil {
			continue
		}
		if best == nil || creds.ExpiresAt > best.ExpiresAt {
			best = creds
		}
	}

	// Also check root ~/.claude/.credentials.json
	if creds, err := loadClaudeCredFile(filepath.Join(claudeDir, ".credentials.json")); err == nil {
		if best == nil || creds.ExpiresAt > best.ExpiresAt {
			best = creds
		}
	}

	if best == nil {
		return nil, fmt.Errorf("no OAuth credentials found in ~/.claude/")
	}
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

func exchangeCode(ctx context.Context, code, verifier, redirectURI string) (*Credentials, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"client_id":     {oauthClientID},
		"redirect_uri":  {redirectURI},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", oauthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed: HTTP %d", resp.StatusCode)
	}

	var result tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &Credentials{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    time.Now().UnixMilli() + int64(result.ExpiresIn)*1000,
	}, nil
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
