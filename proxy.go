package kraube

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// proxyEnvVars lists the environment variables consulted by resolveProxy,
// in priority order. HTTPS_PROXY wins over ALL_PROXY because the API is
// always accessed over HTTPS. Lowercase variants are accepted as fallback
// for compatibility with curl / git conventions.
var proxyEnvVars = []string{
	"HTTPS_PROXY", "https_proxy",
	"ALL_PROXY", "all_proxy",
}

// resolveProxy decides which proxy URL (if any) should be used for outbound
// API traffic. Priority: explicit argument > HTTPS_PROXY > ALL_PROXY > nil.
// A nil result means "connect directly".
//
// Accepted schemes: http, https, socks5, socks5h. Any other scheme is an
// error — failing loud is better than silently falling back to direct.
func resolveProxy(explicit string) (*url.URL, error) {
	raw := strings.TrimSpace(explicit)
	source := "explicit"
	if raw == "" {
		for _, name := range proxyEnvVars {
			if v := strings.TrimSpace(os.Getenv(name)); v != "" {
				raw = v
				source = "env:" + name
				break
			}
		}
	}
	if raw == "" {
		return nil, nil
	}

	// Tolerate a bare "host:port" form — assume http://.
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse proxy URL from %s: %w", source, err)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("proxy URL from %s has no host: %q", source, raw)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "socks5", "socks5h":
		// supported
	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q from %s (want http/https/socks5/socks5h)", u.Scheme, source)
	}
	return u, nil
}

// redactProxyURL returns a log-safe rendering of a proxy URL. Any embedded
// password is replaced by "xxxxx" (stdlib url.URL.Redacted convention).
// Username is preserved so it is still possible to tell *which* credential
// is in use from a log line.
func redactProxyURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	return u.Redacted()
}
