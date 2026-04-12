package kraube

import (
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

	// Deferred token source resolution. When non-empty, NewClient reads
	// credentials from this path at construction time. A path of "_default_"
	// resolves to DefaultCredentialsPath() when NewClient runs, which picks
	// up $KRAUBE_CREDENTIALS_PATH at that moment.
	credentialsPath string
}

// --- Token source options ---

// WithTokenProvider sets a custom TokenProvider.
// This is the most flexible option — implement the TokenProvider interface
// to supply access tokens from any source.
func WithTokenProvider(p TokenProvider) Option {
	return func(c *clientConfig) {
		c.provider = p
		c.credentialsPath = ""
	}
}

// WithToken creates a client with the given refresh token held in memory.
//
// Rotated refresh tokens are kept in memory only — when the process exits,
// they are lost. For a persistent setup that survives restarts and works
// across parallel processes, use WithTokenFile instead.
func WithToken(refreshToken string) Option {
	return func(c *clientConfig) {
		c.provider = newMemoryTokenManager(refreshToken)
		c.credentialsPath = ""
	}
}

// WithTokenFile loads credentials from a JSON file on disk.
// Pass "" to use the default path (honors $KRAUBE_CREDENTIALS_PATH, otherwise
// ~/.config/kraube/credentials.json).
//
// This mode uses an OS-level file lock to serialize refresh operations across
// processes, so multiple instances on the same machine share a single rotated
// refresh token safely.
func WithTokenFile(path string) Option {
	return func(c *clientConfig) {
		c.provider = nil
		if path == "" {
			c.credentialsPath = "_default_"
		} else {
			c.credentialsPath = path
		}
	}
}

// WithEnvToken reads the refresh token from the given environment variable.
// Rotation is held in memory only.
func WithEnvToken(envVar string) Option {
	return func(c *clientConfig) {
		c.provider = newEnvTokenManager(envVar)
		c.credentialsPath = ""
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
