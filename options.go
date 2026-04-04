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
// to supply credentials from any source.
func WithTokenProvider(p TokenProvider) Option {
	return func(c *clientConfig) {
		c.provider = p
	}
}

// WithAccessToken creates a client with a static OAuth access token.
// The token will not be refreshed — use this when you manage token lifecycle externally.
func WithAccessToken(token string) Option {
	return func(c *clientConfig) {
		c.provider = NewStaticTokenProvider(token)
	}
}

// WithCredentials creates a client from existing OAuth credentials.
// If the credentials include a refresh token, they will be auto-refreshed.
func WithCredentials(creds *Credentials) Option {
	return func(c *clientConfig) {
		c.provider = NewCredentialsProvider(creds)
	}
}

// WithCredentialsFile loads OAuth credentials from a JSON file.
// Refreshed tokens are saved back to the same file.
// Pass "" to use the default path (~/.config/kraube/credentials.json).
func WithCredentialsFile(path string) Option {
	return func(c *clientConfig) {
		if path == "" {
			path = DefaultCredentialsPath()
		}
		c.provider = &deferredFileProvider{path: path}
	}
}

// WithEnvToken reads the OAuth access token from the given environment variable.
func WithEnvToken(envVar string) Option {
	return func(c *clientConfig) {
		c.provider = NewEnvTokenProvider(envVar)
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

func (p *deferredFileProvider) Token(_ context.Context) (*Credentials, error) {
	panic("deferredFileProvider.Token should not be called directly")
}
