package kraube

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveAndLoadCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "credentials.json")
	creds := &Credentials{
		RefreshToken: "r-abc",
		AccessToken:  "a-xyz",
		ExpiresAt:    time.Now().Add(time.Hour).UnixMilli(),
	}

	if err := SaveCredentials(path, creds); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("permissions = %o, want 0600", perm)
	}

	got, err := LoadCredentials(path)
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if got.RefreshToken != creds.RefreshToken || got.AccessToken != creds.AccessToken || got.ExpiresAt != creds.ExpiresAt {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, creds)
	}
}

func TestLoadCredentials_NotFound(t *testing.T) {
	_, err := LoadCredentials("/nonexistent/path/credentials.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadCredentials_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	_ = os.WriteFile(path, []byte(""), 0600)

	_, err := LoadCredentials(path)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error = %v, want 'empty' in message", err)
	}
}

func TestLoadCredentials_Malformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	_ = os.WriteFile(path, []byte("not-json"), 0600)

	_, err := LoadCredentials(path)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestLoadCredentials_MissingRefresh(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	_ = os.WriteFile(path, []byte(`{"accessToken":"a"}`), 0600)

	_, err := LoadCredentials(path)
	if err == nil {
		t.Fatal("expected error when refreshToken missing")
	}
}

func TestDefaultCredentialsPath_Env(t *testing.T) {
	t.Setenv(CredentialsPathEnv, "/tmp/custom-creds.json")
	if got := DefaultCredentialsPath(); got != "/tmp/custom-creds.json" {
		t.Errorf("DefaultCredentialsPath = %q, want env override", got)
	}
}

func TestDefaultCredentialsPath_Default(t *testing.T) {
	t.Setenv(CredentialsPathEnv, "")
	path := DefaultCredentialsPath()
	if !strings.HasSuffix(path, filepath.Join("kraube", "credentials.json")) {
		t.Errorf("DefaultCredentialsPath = %q, want suffix kraube/credentials.json", path)
	}
}

func TestExtractAuthCode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"raw code", "J7kB3wLmpnzZ", "J7kB3wLmpnzZ"},
		{"with fragment", "J7kB3wLmpnzZ#state123", "J7kB3wLmpnzZ"},
		{"full URL", "https://platform.claude.com/oauth/code/callback?code=ABC123&state=xyz", "ABC123"},
		{"URL with fragment", "https://example.com/callback?code=ABC123#frag", "ABC123"},
		{"quoted", `"J7kB3wLmpnzZ"`, "J7kB3wLmpnzZ"},
		{"angle brackets", "<J7kB3wLmpnzZ>", "J7kB3wLmpnzZ"},
		{"whitespace", "  J7kB3wLmpnzZ  ", "J7kB3wLmpnzZ"},
		{"with query no scheme", "code123?extra=1", "code123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractAuthCode(tt.input)
			if got != tt.want {
				t.Errorf("extractAuthCode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGeneratePKCE(t *testing.T) {
	v, c, err := generatePKCE()
	if err != nil {
		t.Fatalf("generatePKCE: %v", err)
	}
	if len(v) != 43 {
		t.Errorf("verifier length = %d, want 43", len(v))
	}
	if len(c) == 0 {
		t.Error("challenge is empty")
	}
	// Challenge should be different from verifier (it's a hash).
	if v == c {
		t.Error("verifier and challenge should differ")
	}
}

func TestRandomString(t *testing.T) {
	s1, err := randomString(32)
	if err != nil {
		t.Fatalf("randomString: %v", err)
	}
	if len(s1) != 32 {
		t.Errorf("length = %d, want 32", len(s1))
	}

	s2, _ := randomString(32)
	if s1 == s2 {
		t.Error("two calls should produce different strings")
	}
}

func TestCredentialsIsAccessLive(t *testing.T) {
	tests := []struct {
		name  string
		creds Credentials
		live  bool
	}{
		{
			"empty access token",
			Credentials{AccessToken: "", ExpiresAt: time.Now().Add(time.Hour).UnixMilli()},
			false,
		},
		{
			"live",
			Credentials{AccessToken: "tok", ExpiresAt: time.Now().Add(5 * time.Minute).UnixMilli()},
			true,
		},
		{
			"expired",
			Credentials{AccessToken: "tok", ExpiresAt: time.Now().Add(-time.Minute).UnixMilli()},
			false,
		},
		{
			"within 60s buffer",
			Credentials{AccessToken: "tok", ExpiresAt: time.Now().Add(30 * time.Second).UnixMilli()},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.creds.IsAccessLive(); got != tt.live {
				t.Errorf("IsAccessLive = %v, want %v", got, tt.live)
			}
		})
	}
}

func TestRefreshAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["grant_type"] != "refresh_token" {
			t.Errorf("grant_type = %q, want refresh_token", body["grant_type"])
		}
		if body["refresh_token"] != "my-refresh" {
			t.Errorf("refresh_token = %q, want my-refresh", body["refresh_token"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	// Temporarily override the token URL.
	origURL := oauthTokenURL
	defer func() { setOAuthTokenURL(origURL) }()
	setOAuthTokenURL(server.URL)

	tokens, err := refreshAccessToken(context.Background(), nil, "my-refresh")
	if err != nil {
		t.Fatalf("refreshAccessToken: %v", err)
	}
	if tokens.accessToken != "new-access" {
		t.Errorf("accessToken = %q, want new-access", tokens.accessToken)
	}
	if tokens.refreshToken != "new-refresh" {
		t.Errorf("refreshToken = %q, want new-refresh", tokens.refreshToken)
	}
	if tokens.expiresAt <= time.Now().UnixMilli() {
		t.Error("expiresAt should be in the future")
	}
}

func TestRefreshAccessToken_KeepsOldRefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "new-access",
			"expires_in":   3600,
			// No refresh_token in response.
		})
	}))
	defer server.Close()

	origURL := oauthTokenURL
	defer func() { setOAuthTokenURL(origURL) }()
	setOAuthTokenURL(server.URL)

	tokens, err := refreshAccessToken(context.Background(), nil, "old-refresh")
	if err != nil {
		t.Fatalf("refreshAccessToken: %v", err)
	}
	if tokens.refreshToken != "old-refresh" {
		t.Errorf("refreshToken = %q, want old-refresh (preserved)", tokens.refreshToken)
	}
}
