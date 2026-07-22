package kraube

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gofrs/flock"
)

// TokenProvider abstracts how access tokens are obtained.
// The client calls Token() before each request to get a valid access token string.
type TokenProvider interface {
	// Token returns a valid access token ready for use in API requests.
	// Implementations should handle refreshing internally if needed.
	Token(ctx context.Context) (string, error)
}

// refreshableProvider is the optional internal contract implemented by the
// built-in token managers. It powers Client.EnsureFresh / Client.AccessExpiry
// (and through them the `kraube serve` keepalive) without widening the public
// TokenProvider interface: custom providers keep their one-method contract
// and simply degrade to lazy refresh.
type refreshableProvider interface {
	// ensureFresh refreshes when the access token expires within margin.
	ensureFresh(ctx context.Context, margin time.Duration) error
	// expiry reports the current access token's expiresAt (ok=false when
	// no access token is held yet).
	expiry() (time.Time, bool)
}

// --- Built-in providers ---

// tokenManager manages the OAuth access token lifecycle.
//
// Two modes:
//
//   - In-memory only (path == ""): refresh token kept in memory, rotated
//     copies are not persisted. Suitable for WithToken(string).
//
//   - Persistent (path != ""): refresh + access + expiresAt are stored in a
//     JSON credentials file. Access to the file is serialized across
//     processes via flock(2) on Linux/macOS and LockFileEx on Windows.
//     Every refresh re-reads the file under lock first, so concurrent
//     processes share a single rotation instead of racing.
type tokenManager struct {
	mu sync.Mutex

	// Persistent-mode fields.
	path string // "" = in-memory only

	// Current state (mirrors file contents when path != "").
	creds *Credentials

	// HTTP client used for the OAuth refresh call. When nil, refreshAccessToken
	// falls back to the package-level authHTTPClient. NewClient sets this to
	// the per-instance c.HTTPClient so that WithProxy / WithHTTPClient apply
	// to refresh automatically — no SetAuthHTTPClient dance required.
	httpClient *http.Client
}

// setHTTPClient installs the HTTP client used for refresh calls. Called by
// NewClient after the per-Client HTTPClient is constructed. Safe to call
// before Token() is invoked; concurrent use is not expected at this stage.
func (m *tokenManager) setHTTPClient(hc *http.Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.httpClient = hc
}

// newMemoryTokenManager constructs an in-memory tokenManager seeded with a refresh token.
// Rotated refresh tokens are held in memory only — restarts lose them.
func newMemoryTokenManager(refreshToken string) *tokenManager {
	return &tokenManager{
		creds: &Credentials{RefreshToken: refreshToken},
	}
}

// newFileTokenManager constructs a persistent tokenManager bound to a credentials file.
// The file must already exist (e.g. created by `kraube login`); this function reads it.
func newFileTokenManager(path string) (*tokenManager, error) {
	creds, err := LoadCredentials(path)
	if err != nil {
		return nil, fmt.Errorf("load credentials: %w (run `kraube login` first)", err)
	}
	return &tokenManager{path: path, creds: creds}, nil
}

func (m *tokenManager) Token(ctx context.Context) (string, error) {
	// Fast path: in-memory access token still valid.
	m.mu.Lock()
	if m.creds.IsAccessLive() {
		t := m.creds.AccessToken
		m.mu.Unlock()
		return t, nil
	}
	m.mu.Unlock()

	// Slow path: need a refresh.
	if m.path == "" {
		return m.refreshInMemory(ctx, accessTokenMargin)
	}
	return m.refreshPersistent(ctx, accessTokenMargin)
}

// ensureFresh refreshes the access token when it expires within margin.
// Unlike Token — which only refreshes inside the fixed 60-second window —
// this lets a caller force rotation well ahead of expiry. `kraube serve`
// uses it from its background keepalive so the token on disk never gets
// close to actually expiring. No-op when the token already outlives margin.
func (m *tokenManager) ensureFresh(ctx context.Context, margin time.Duration) error {
	m.mu.Lock()
	live := m.creds.LiveFor(margin)
	m.mu.Unlock()
	if live {
		return nil
	}
	var err error
	if m.path == "" {
		_, err = m.refreshInMemory(ctx, margin)
	} else {
		_, err = m.refreshPersistent(ctx, margin)
	}
	return err
}

// expiry reports the expiresAt of the current access token. ok is false
// when no access token is held yet (fresh manager before the first refresh).
func (m *tokenManager) expiry() (time.Time, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.creds == nil || m.creds.AccessToken == "" {
		return time.Time{}, false
	}
	return time.UnixMilli(m.creds.ExpiresAt), true
}

// refreshInMemory performs a refresh without cross-process synchronization.
// Used when no credentials file is bound — the rotated refresh token lives
// only for the lifetime of this process. The margin controls the re-check
// under the lock: a token that outlives margin is reused instead of rotated.
func (m *tokenManager) refreshInMemory(ctx context.Context, margin time.Duration) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Re-check under the lock — another goroutine may have refreshed.
	if m.creds.LiveFor(margin) {
		return m.creds.AccessToken, nil
	}

	logDebug("provider: refreshing (in-memory)")
	tokens, err := refreshAccessToken(ctx, m.httpClient, m.creds.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("refresh token: %w", err)
	}
	m.creds = tokens.toCredentials()
	return m.creds.AccessToken, nil
}

// refreshPersistent performs a refresh under an exclusive file lock.
// After acquiring the lock, the file is re-read — another process may have
// already rotated the token, in which case we simply reuse its result
// (as long as it outlives the requested margin).
func (m *tokenManager) refreshPersistent(ctx context.Context, margin time.Duration) (string, error) {
	lock := flock.New(m.path + ".lock")
	if err := lockWithContext(ctx, lock); err != nil {
		return "", fmt.Errorf("acquire credentials lock: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Re-read the file under the lock — another process may have refreshed.
	onDisk, err := LoadCredentials(m.path)
	if err != nil {
		return "", fmt.Errorf("reload credentials: %w", err)
	}

	if onDisk.LiveFor(margin) {
		logDebug("provider: reused access token from disk (another process refreshed)")
		m.creds = onDisk
		return m.creds.AccessToken, nil
	}

	// Refuse to rotate when the file cannot be written back. The refresh
	// token is single-use on the server side: a successful refresh whose
	// result is not persisted silently invalidates the on-disk token for
	// every other process sharing the file (observed with a read-only
	// container bind mount). Fail loudly before touching the OAuth endpoint.
	if f, err := os.OpenFile(m.path, os.O_WRONLY, 0); err != nil {
		return "", fmt.Errorf("credentials file %s is not writable, refusing to refresh (rotated token would be lost): %w", m.path, err)
	} else {
		_ = f.Close()
	}

	logDebug("provider: refreshing (persistent)", "path", m.path)
	tokens, err := refreshAccessToken(ctx, m.httpClient, onDisk.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("refresh token: %w", err)
	}
	m.creds = tokens.toCredentials()

	if err := SaveCredentials(m.path, m.creds); err != nil {
		return "", fmt.Errorf("save rotated credentials: %w", err)
	}
	return m.creds.AccessToken, nil
}

// lockWithContext acquires the flock, honoring context cancellation.
func lockWithContext(ctx context.Context, lock *flock.Flock) error {
	const retryInterval = 50 * time.Millisecond
	for {
		ok, err := lock.TryLock()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryInterval):
		}
	}
}

// envTokenManager reads the refresh token from an environment variable and
// delegates to an in-memory tokenManager. Rotation is not persisted.
type envTokenManager struct {
	mu         sync.Mutex
	envVar     string
	token      string // last seen token value
	inner      *tokenManager
	httpClient *http.Client // propagated to inner on (re)creation
}

func newEnvTokenManager(envVar string) *envTokenManager {
	return &envTokenManager{envVar: envVar}
}

// setHTTPClient installs the HTTP client used for refresh and propagates it
// to the inner tokenManager (if already created). Mirrors tokenManager.setHTTPClient.
func (m *envTokenManager) setHTTPClient(hc *http.Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.httpClient = hc
	if m.inner != nil {
		m.inner.setHTTPClient(hc)
	}
}

// resolveInner reads the env var and returns the inner tokenManager,
// recreating it when the variable value changed since the last call.
func (m *envTokenManager) resolveInner() (*tokenManager, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	token := os.Getenv(m.envVar)
	if token == "" {
		return nil, fmt.Errorf("environment variable %s is not set", m.envVar)
	}

	// If token changed, create a new inner manager.
	if m.inner == nil || token != m.token {
		m.token = token
		m.inner = newMemoryTokenManager(token)
		m.inner.httpClient = m.httpClient
	}
	return m.inner, nil
}

func (m *envTokenManager) Token(ctx context.Context) (string, error) {
	inner, err := m.resolveInner()
	if err != nil {
		return "", err
	}
	return inner.Token(ctx)
}

// ensureFresh delegates the proactive refresh to the inner tokenManager.
func (m *envTokenManager) ensureFresh(ctx context.Context, margin time.Duration) error {
	inner, err := m.resolveInner()
	if err != nil {
		return err
	}
	return inner.ensureFresh(ctx, margin)
}

// expiry reports the current access token expiry from the inner manager.
func (m *envTokenManager) expiry() (time.Time, bool) {
	m.mu.Lock()
	inner := m.inner
	m.mu.Unlock()
	if inner == nil {
		return time.Time{}, false
	}
	return inner.expiry()
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
