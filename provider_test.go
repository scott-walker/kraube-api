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

// countingRoundTripper wraps http.DefaultTransport and atomically counts
// the number of requests passing through it. Used to verify that the
// per-Client HTTPClient — not the package-level authHTTPClient — handles
// OAuth refresh.
type countingRoundTripper struct {
	calls atomic.Int32
	inner http.RoundTripper
}

func (c *countingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return c.inner.RoundTrip(r)
}

// TestNewClient_RefreshRoutesThroughHTTPClient verifies that after NewClient
// resolves the per-Client HTTPClient, token refresh goes through it instead
// of the package-level authHTTPClient. This is the regression test for the
// v0.4.3 fix: prior versions routed refresh through http.DefaultClient,
// causing 403s from regions where direct access to the OAuth endpoint is
// blocked even though WithProxy was set on the Client.
func TestNewClient_RefreshRoutesThroughHTTPClient(t *testing.T) {
	server := newMockTokenServer(t, "access-via-client", "refresh-new")
	defer server.Close()
	withMockTokenURL(t, server)

	// Sentinel client: every request passing through this transport is
	// counted. If refresh ignored WithHTTPClient and fell back to the
	// package global, the counter would stay at 0 and Token() would fail
	// (the mock server is only routable via httptest's Transport in practice,
	// but DefaultTransport also handles it over localhost — that's fine;
	// the signal we care about is *which* RoundTripper handled the call).
	rt := &countingRoundTripper{inner: http.DefaultTransport}
	custom := &http.Client{Transport: rt}

	// Make sure the package global is something that would obviously fail
	// if the code accidentally used it — a nil Transport default client can't
	// talk to the test server's scheme mismatch, but the simpler assertion
	// is just "rt.calls > 0 after NewClient".
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewClient(ctx,
		WithToken("seed-refresh-token"),
		WithHTTPClient(custom),
		WithoutProfile(),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client.HTTPClient != custom {
		t.Errorf("Client.HTTPClient was replaced; wanted the injected one")
	}
	if got := rt.calls.Load(); got < 1 {
		t.Errorf("custom transport saw %d requests, want >= 1 (refresh should route through c.HTTPClient)", got)
	}

	// Force a second refresh by invalidating the in-memory access token and
	// calling the provider directly. The counter must advance — proving the
	// manager kept the per-Client HTTPClient, not just used it once.
	tm, ok := client.Provider.(*tokenManager)
	if !ok {
		t.Fatalf("Provider is %T, want *tokenManager", client.Provider)
	}
	tm.mu.Lock()
	tm.creds.ExpiresAt = 0
	tm.mu.Unlock()

	before := rt.calls.Load()
	if _, err := tm.Token(ctx); err != nil {
		t.Fatalf("second Token: %v", err)
	}
	if after := rt.calls.Load(); after <= before {
		t.Errorf("second refresh did not route through custom transport: before=%d, after=%d", before, after)
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
