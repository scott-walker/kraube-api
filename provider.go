package kraube

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

// TokenProvider abstracts how credentials are obtained.
// The client calls Token() before each request to get valid credentials.
type TokenProvider interface {
	// Token returns current valid credentials.
	// Implementations should handle refreshing internally if needed.
	Token(ctx context.Context) (*Credentials, error)
}

// --- Built-in providers ---

// StaticTokenProvider returns a fixed access token that never refreshes.
// Use this when you have a token from an external source and manage
// its lifecycle yourself.
type StaticTokenProvider struct {
	creds *Credentials
}

// NewStaticTokenProvider creates a provider from a raw access token.
// The token has no expiry and will not be refreshed.
func NewStaticTokenProvider(accessToken string) *StaticTokenProvider {
	return &StaticTokenProvider{
		creds: &Credentials{
			AccessToken: accessToken,
			ExpiresAt:   time.Now().Add(365 * 24 * time.Hour).UnixMilli(), // far future
		},
	}
}

func (p *StaticTokenProvider) Token(_ context.Context) (*Credentials, error) {
	return p.creds, nil
}

// CredentialsProvider returns fixed credentials and auto-refreshes when expired.
type CredentialsProvider struct {
	mu    sync.Mutex
	creds *Credentials
	path  string // optional: save refreshed creds to disk
}

// NewCredentialsProvider creates a provider from existing credentials.
// If the credentials contain a refresh token, they will be auto-refreshed
// when expired.
func NewCredentialsProvider(creds *Credentials) *CredentialsProvider {
	return &CredentialsProvider{creds: creds}
}

func (p *CredentialsProvider) Token(ctx context.Context) (*Credentials, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.creds.IsExpired() {
		return p.creds, nil
	}

	if p.creds.RefreshToken == "" {
		return nil, fmt.Errorf("token expired and no refresh token available")
	}

	logDebug("provider: token expired, refreshing")
	newCreds, err := RefreshAccessToken(ctx, p.creds)
	if err != nil {
		return nil, fmt.Errorf("refresh token: %w", err)
	}
	p.creds = newCreds

	if p.path != "" {
		if err := SaveCredentials(p.path, newCreds); err != nil {
			logWarn("provider: failed to save refreshed credentials", "path", p.path, "error", err)
		}
	}

	return p.creds, nil
}

// FileTokenProvider loads credentials from a JSON file and auto-refreshes.
type FileTokenProvider struct {
	inner *CredentialsProvider
}

// NewFileTokenProvider creates a provider that reads credentials from path.
// Refreshed tokens are saved back to the same file.
func NewFileTokenProvider(path string) (*FileTokenProvider, error) {
	creds, err := LoadCredentials(path)
	if err != nil {
		return nil, fmt.Errorf("load credentials from %s: %w", path, err)
	}
	p := &CredentialsProvider{creds: creds, path: path}
	return &FileTokenProvider{inner: p}, nil
}

func (p *FileTokenProvider) Token(ctx context.Context) (*Credentials, error) {
	return p.inner.Token(ctx)
}

// EnvTokenProvider reads the access token from an environment variable.
type EnvTokenProvider struct {
	envVar string
}

// NewEnvTokenProvider creates a provider that reads from the given env var.
// The env var is read on each call to Token(), so changes are picked up.
func NewEnvTokenProvider(envVar string) *EnvTokenProvider {
	return &EnvTokenProvider{envVar: envVar}
}

func (p *EnvTokenProvider) Token(_ context.Context) (*Credentials, error) {
	token := os.Getenv(p.envVar)
	if token == "" {
		return nil, fmt.Errorf("environment variable %s is not set", p.envVar)
	}
	return &Credentials{
		AccessToken: token,
		ExpiresAt:   time.Now().Add(365 * 24 * time.Hour).UnixMilli(),
	}, nil
}

// CallbackTokenProvider wraps a user-supplied function as a TokenProvider.
type CallbackTokenProvider struct {
	fn func(ctx context.Context) (*Credentials, error)
}

// NewCallbackTokenProvider creates a provider from a callback function.
// The function is called on each Token() invocation — the caller is responsible
// for caching/refreshing as needed.
func NewCallbackTokenProvider(fn func(ctx context.Context) (*Credentials, error)) *CallbackTokenProvider {
	return &CallbackTokenProvider{fn: fn}
}

func (p *CallbackTokenProvider) Token(ctx context.Context) (*Credentials, error) {
	return p.fn(ctx)
}
