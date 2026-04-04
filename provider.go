package kraube

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

// TokenProvider abstracts how access tokens are obtained.
// The client calls Token() before each request to get a valid access token string.
type TokenProvider interface {
	// Token returns a valid access token ready for use in API requests.
	// Implementations should handle refreshing internally if needed.
	Token(ctx context.Context) (string, error)
}

// --- Built-in providers ---

// tokenManager manages the OAuth access token lifecycle from a stored token.
// It holds the token (refresh token) and obtains short-lived access tokens
// automatically, refreshing them when they expire.
type tokenManager struct {
	mu           sync.Mutex
	refreshToken string
	accessToken  string
	expiresAt    int64                    // unix ms
	saveFn       func(token string) error // optional: persist rotated refresh token
}

func newTokenManager(token string) *tokenManager {
	return &tokenManager{refreshToken: token}
}

func (m *tokenManager) Token(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.accessToken != "" && !m.isExpired() {
		return m.accessToken, nil
	}

	logDebug("provider: access token expired or missing, refreshing")
	tokens, err := refreshAccessToken(ctx, m.refreshToken)
	if err != nil {
		return "", fmt.Errorf("refresh token: %w", err)
	}

	m.accessToken = tokens.accessToken
	m.expiresAt = tokens.expiresAt

	// Handle refresh token rotation.
	if tokens.refreshToken != "" && tokens.refreshToken != m.refreshToken {
		m.refreshToken = tokens.refreshToken
		if m.saveFn != nil {
			if err := m.saveFn(m.refreshToken); err != nil {
				logWarn("provider: failed to save rotated token", "error", err)
			}
		}
	}

	return m.accessToken, nil
}

func (m *tokenManager) isExpired() bool {
	return m.accessToken == "" || timeNowMs() >= m.expiresAt-60_000
}

// envTokenManager reads the token from an environment variable and delegates
// to a tokenManager for access token lifecycle.
type envTokenManager struct {
	mu     sync.Mutex
	envVar string
	token  string // last seen token value
	inner  *tokenManager
}

func newEnvTokenManager(envVar string) *envTokenManager {
	return &envTokenManager{envVar: envVar}
}

func (m *envTokenManager) Token(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	token := os.Getenv(m.envVar)
	if token == "" {
		return "", fmt.Errorf("environment variable %s is not set", m.envVar)
	}

	// If token changed, create new inner manager.
	if m.inner == nil || token != m.token {
		m.token = token
		m.inner = newTokenManager(token)
	}

	return m.inner.Token(ctx)
}

// CallbackTokenProvider wraps a user-supplied function as a TokenProvider.
// The function should return a valid access token string.
type CallbackTokenProvider struct {
	fn func(ctx context.Context) (string, error)
}

// NewCallbackTokenProvider creates a provider from a callback function.
// The function is called on each Token() invocation — the caller is responsible
// for caching/refreshing as needed.
func NewCallbackTokenProvider(fn func(ctx context.Context) (string, error)) *CallbackTokenProvider {
	return &CallbackTokenProvider{fn: fn}
}

func (p *CallbackTokenProvider) Token(ctx context.Context) (string, error) {
	return p.fn(ctx)
}

// timeNowMs returns the current time in unix milliseconds.
func timeNowMs() int64 {
	return time.Now().UnixMilli()
}
