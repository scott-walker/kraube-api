package kraube

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
)

func newMockTokenServer(t *testing.T, accessToken, refreshToken string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
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

func TestTokenManager_FirstCallRefreshes(t *testing.T) {
	server := newMockTokenServer(t, "access-1", "refresh-new")
	defer server.Close()
	withMockTokenURL(t, server)

	m := newTokenManager("my-refresh")
	got, err := m.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got != "access-1" {
		t.Errorf("Token = %q, want access-1", got)
	}
}

func TestTokenManager_CachesAccessToken(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-cached",
			"expires_in":   3600,
		})
	}))
	defer server.Close()
	withMockTokenURL(t, server)

	m := newTokenManager("my-refresh")
	ctx := context.Background()

	m.Token(ctx)
	m.Token(ctx)
	m.Token(ctx)

	if c := calls.Load(); c != 1 {
		t.Errorf("refresh called %d times, want 1 (cached)", c)
	}
}

func TestTokenManager_SaveFnCalledOnRotation(t *testing.T) {
	server := newMockTokenServer(t, "access-1", "rotated-refresh")
	defer server.Close()
	withMockTokenURL(t, server)

	var savedToken string
	m := newTokenManager("original-refresh")
	m.saveFn = func(token string) error {
		savedToken = token
		return nil
	}

	_, err := m.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if savedToken != "rotated-refresh" {
		t.Errorf("savedToken = %q, want rotated-refresh", savedToken)
	}
}

func TestTokenManager_SaveFnNotCalledIfSameRefresh(t *testing.T) {
	server := newMockTokenServer(t, "access-1", "original-refresh")
	defer server.Close()
	withMockTokenURL(t, server)

	saveCalled := false
	m := newTokenManager("original-refresh")
	m.saveFn = func(token string) error {
		saveCalled = true
		return nil
	}

	m.Token(context.Background())
	if saveCalled {
		t.Error("saveFn should not be called when refresh token doesn't rotate")
	}
}

func TestEnvTokenManager(t *testing.T) {
	server := newMockTokenServer(t, "env-access", "")
	defer server.Close()
	withMockTokenURL(t, server)

	const envVar = "KRAUBE_TEST_TOKEN_12345"
	os.Setenv(envVar, "env-refresh-token")
	defer os.Unsetenv(envVar)

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
