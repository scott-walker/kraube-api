package kraube

import (
	"fmt"
	"net/http"
)

// NewProxiedHTTPClient returns an *http.Client that uses kraube's Chrome-
// fingerprinted transport routed through the given proxy URL. Useful for
// installing a custom auth HTTP client via SetAuthHTTPClient, or as a
// starting point for WithHTTPClient.
//
// Passing an empty proxyURL yields a client that reads HTTPS_PROXY /
// ALL_PROXY from the environment (same rules as NewClient without
// WithProxy). Pass a literal URL to force a specific proxy.
func NewProxiedHTTPClient(proxyURL string) (*http.Client, error) {
	u, err := resolveProxy(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("proxy: %w", err)
	}
	return &http.Client{Transport: newChromeTransport(u)}, nil
}

// Option configures a Client during construction.
type Option func(*clientConfig)

type clientConfig struct {
	provider TokenProvider

	// HTTP
	httpClient *http.Client
	baseURL    string

	// Proxy. proxy holds an explicit URL passed via WithProxy. proxySet marks
	// whether WithProxy was called at all — so an empty string explicitly
	// disables env-based proxy resolution, while a truly absent option falls
	// back to HTTPS_PROXY / ALL_PROXY.
	proxy    string
	proxySet bool

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

// WithProxy routes all outbound API traffic through the given proxy URL.
// Supported schemes: http, https, socks5, socks5h. The URL may embed
// credentials (http://user:pass@host:port) for Basic proxy auth.
//
// When this option is omitted, the client automatically honors the
// HTTPS_PROXY / ALL_PROXY environment variables (in that order). Passing
// an empty string here explicitly disables both env lookup and proxying,
// forcing a direct connection even if the environment is set.
//
//	client, _ := kraube.NewClient(ctx,
//	    kraube.WithTokenFile(""),
//	    kraube.WithProxy("http://user:pass@proxy.example.com:8080"),
//	)
func WithProxy(proxyURL string) Option {
	return func(c *clientConfig) {
		c.proxy = proxyURL
		c.proxySet = true
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
