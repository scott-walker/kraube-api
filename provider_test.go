package kraube

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newMockTokenServer(t *testing.T, accessToken, refreshToken string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  accessToken,
			"refresh_token": refreshToken,
			"expires_in":    3600,
		})
	}))
}

func withMockTokenURL(t *testing.T, server *httptest.Server) {
	t.Helper()
	orig := oauthTokenURL
	setOAuthTokenURL(server.URL)
	t.Cleanup(func() { setOAuthTokenURL(orig) })
}

func TestMemoryTokenManager_FirstCallRefreshes(t *testing.T) {
	server := newMockTokenServer(t, "access-1", "refresh-new")
	defer server.Close()
	withMockTokenURL(t, server)

	m := newMemoryTokenManager("my-refresh")
	got, err := m.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got != "access-1" {
		t.Errorf("Token = %q, want access-1", got)
	}
}

func TestMemoryTokenManager_CachesAccessToken(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-cached",
			"expires_in":   3600,
		})
	}))
	defer server.Close()
	withMockTokenURL(t, server)

	m := newMemoryTokenManager("my-refresh")
	ctx := context.Background()

	_, _ = m.Token(ctx)
	_, _ = m.Token(ctx)
	_, _ = m.Token(ctx)

	if c := calls.Load(); c != 1 {
		t.Errorf("refresh called %d times, want 1 (cached)", c)
	}
}

func TestFileTokenManager_PersistsRotation(t *testing.T) {
	server := newMockTokenServer(t, "access-1", "rotated-refresh")
	defer server.Close()
	withMockTokenURL(t, server)

	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	if err := SaveCredentials(path, &Credentials{RefreshToken: "original-refresh"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	m, err := newFileTokenManager(path)
	if err != nil {
		t.Fatalf("newFileTokenManager: %v", err)
	}
	if _, err := m.Token(context.Background()); err != nil {
		t.Fatalf("Token: %v", err)
	}

	onDisk, err := LoadCredentials(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if onDisk.RefreshToken != "rotated-refresh" {
		t.Errorf("on-disk refreshToken = %q, want rotated-refresh", onDisk.RefreshToken)
	}
	if onDisk.AccessToken != "access-1" {
		t.Errorf("on-disk accessToken = %q, want access-1", onDisk.AccessToken)
	}
	if onDisk.ExpiresAt <= time.Now().UnixMilli() {
		t.Errorf("on-disk expiresAt should be in the future, got %d", onDisk.ExpiresAt)
	}
}

// TestFileTokenManager_ParallelProcesses simulates two processes racing to
// refresh the same credentials file: only one HTTP refresh should happen,
// the other reads the rotated token from disk under the lock.
func TestFileTokenManager_ParallelProcesses(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		// Slow down so both workers contend on the lock.
		time.Sleep(50 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "shared-access",
			"refresh_token": "rotated",
			"expires_in":    3600,
		})
	}))
	defer server.Close()
	withMockTokenURL(t, server)

	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	if err := SaveCredentials(path, &Credentials{RefreshToken: "original"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Two independent managers simulate two processes: separate in-memory
	// state, same file. They must coordinate via flock.
	m1, err := newFileTokenManager(path)
	if err != nil {
		t.Fatalf("m1: %v", err)
	}
	m2, err := newFileTokenManager(path)
	if err != nil {
		t.Fatalf("m2: %v", err)
	}

	var wg sync.WaitGroup
	var t1, t2 string
	var e1, e2 error
	wg.Add(2)
	go func() { defer wg.Done(); t1, e1 = m1.Token(context.Background()) }()
	go func() { defer wg.Done(); t2, e2 = m2.Token(context.Background()) }()
	wg.Wait()

	if e1 != nil || e2 != nil {
		t.Fatalf("Token errors: %v, %v", e1, e2)
	}
	if t1 != "shared-access" || t2 != "shared-access" {
		t.Errorf("tokens = %q / %q, want both shared-access", t1, t2)
	}
	if c := calls.Load(); c != 1 {
		t.Errorf("refresh HTTP calls = %d, want 1 (one winner, one reads-after-lock)", c)
	}
}

func TestEnvTokenManager(t *testing.T) {
	server := newMockTokenServer(t, "env-access", "")
	defer server.Close()
	withMockTokenURL(t, server)

	const envVar = "KRAUBE_TEST_TOKEN_12345"
	t.Setenv(envVar, "env-refresh-token")

	m := newEnvTokenManager(envVar)
	got, err := m.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got != "env-access" {
		t.Errorf("Token = %q, want env-access", got)
	}
}

func TestEnvTokenManager_EmptyVar(t *testing.T) {
	m := newEnvTokenManager("KRAUBE_NONEXISTENT_VAR_99999")
	_, err := m.Token(context.Background())
	if err == nil {
		t.Fatal("expected error for unset env var")
	}
}

func TestCallbackTokenProvider(t *testing.T) {
	p := NewCallbackTokenProvider(func(ctx context.Context) (string, error) {
		return "callback-token", nil
	})
	got, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got != "callback-token" {
		t.Errorf("Token = %q, want callback-token", got)
	}
}
