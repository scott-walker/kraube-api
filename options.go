package kraube

import (
	"context"
	"net/http"
)

// Option configures a Client during construction.
type Option func(*clientConfig)

type clientConfig struct {
	provider TokenProvider

	// HTTP
	httpClient *http.Client
	baseURL    string

	// Paths
	rateLimitPath string

	// Behavior
	skipProfile bool
}

// --- Token source options ---

// WithTokenProvider sets a custom TokenProvider.
// This is the most flexible option — implement the TokenProvider interface
// to supply access tokens from any source.
func WithTokenProvider(p TokenProvider) Option {
	return func(c *clientConfig) {
		c.provider = p
	}
}

// WithToken creates a client with the given token.
// The token is obtained from `kraube login` and handles authentication automatically.
func WithToken(token string) Option {
	return func(c *clientConfig) {
		c.provider = newTokenManager(token)
	}
}

// WithTokenFile loads the token from a file.
// Pass "" to use the default path (~/.config/kraube/token).
func WithTokenFile(path string) Option {
	return func(c *clientConfig) {
		if path == "" {
			path = DefaultTokenPath()
		}
		c.provider = &deferredFileProvider{path: path}
	}
}

// WithEnvToken reads the token from the given environment variable.
func WithEnvToken(envVar string) Option {
	return func(c *clientConfig) {
		c.provider = newEnvTokenManager(envVar)
	}
}

// --- HTTP / transport options ---

// WithHTTPClient sets a custom HTTP client. By default, the client uses
// a Chrome TLS fingerprinted transport.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *clientConfig) {
		c.httpClient = hc
	}
}

// WithBaseURL overrides the API base URL (default: https://api.anthropic.com).
func WithBaseURL(url string) Option {
	return func(c *clientConfig) {
		c.baseURL = url
	}
}

// --- Behavior options ---

// WithoutProfile skips fetching the OAuth profile on init.
// Useful for performance-sensitive scenarios where metadata.user_id is not needed.
func WithoutProfile() Option {
	return func(c *clientConfig) {
		c.skipProfile = true
	}
}

// --- Deferred providers (resolve at NewClient time) ---

type deferredFileProvider struct {
	path string
}

func (p *deferredFileProvider) Token(_ context.Context) (string, error) {
	panic("deferredFileProvider.Token should not be called directly")
}
