package kraube

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
	xproxy "golang.org/x/net/proxy"
)

// connInfoKey is the context key used to pass a *connInfo down into the
// transport so the caller can observe which local/remote addresses were used
// for the connection (useful when debugging proxies / multi-homed egress).
type connInfoKey struct{}

// connInfo captures dial-time addresses for a single request. Populated by
// chromeTransport.RoundTrip after the TCP dial succeeds.
type connInfo struct {
	LocalAddr  string
	RemoteAddr string
	ProxyURL   string // redacted proxy URL actually used, empty for direct
}

// newChromeTransport creates an http.RoundTripper that mimics Chrome's TLS fingerprint
// using uTLS with HelloChrome_Auto. This is required because Cloudflare on
// api.anthropic.com blocks Go's default TLS fingerprint with 429 for OAuth
// Bearer token requests.
//
// If proxyURL is non-nil, outbound connections are routed through it. Supported
// schemes: http, https (HTTP CONNECT tunnel), socks5, socks5h. The uTLS
// handshake always happens end-to-end with the real target host, so the
// Chrome fingerprint is preserved regardless of proxy type.
func newChromeTransport(proxyURL *url.URL) http.RoundTripper {
	return &chromeTransport{proxyURL: proxyURL}
}

type chromeTransport struct {
	h2Transport *http2.Transport
	proxyURL    *url.URL
}

func (t *chromeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	host, port, err := net.SplitHostPort(req.URL.Host)
	if err != nil {
		host = req.URL.Host
		port = "443"
	}
	addr := net.JoinHostPort(host, port)

	rawConn, err := t.dialTarget(req.Context(), addr)
	if err != nil {
		return nil, err
	}

	localAddr := rawConn.LocalAddr().String()
	remoteAddr := rawConn.RemoteAddr().String()
	redactedProxy := redactProxyURL(t.proxyURL)
	if ci, ok := req.Context().Value(connInfoKey{}).(*connInfo); ok && ci != nil {
		ci.LocalAddr = localAddr
		ci.RemoteAddr = remoteAddr
		ci.ProxyURL = redactedProxy
	}

	uconn := utls.UClient(rawConn, &utls.Config{
		ServerName: host,
	}, utls.HelloChrome_Auto)

	if err := uconn.Handshake(); err != nil {
		_ = rawConn.Close()
		return nil, fmt.Errorf("utls handshake %s: %w", addr, err)
	}

	alpn := uconn.ConnectionState().NegotiatedProtocol
	logDebug("transport: utls connected",
		"addr", addr,
		"local", localAddr,
		"remote", remoteAddr,
		"proxy", redactedProxy,
		"alpn", alpn,
	)

	if alpn == "h2" {
		return t.roundTripH2(uconn, req)
	}
	return t.roundTripH1(uconn, req)
}

// dialTarget returns a raw TCP connection to addr, routed through the
// configured proxy (if any). For HTTP/HTTPS proxies it opens a CONNECT
// tunnel; for SOCKS5 it delegates to golang.org/x/net/proxy. When no proxy
// is configured it falls back to a direct dial — the historical behavior.
func (t *chromeTransport) dialTarget(ctx context.Context, addr string) (net.Conn, error) {
	if t.proxyURL == nil {
		return dialContextTCP(ctx, addr)
	}

	scheme := strings.ToLower(t.proxyURL.Scheme)
	switch scheme {
	case "http", "https":
		return t.dialHTTPConnect(ctx, addr)
	case "socks5", "socks5h":
		return t.dialSOCKS5(ctx, addr)
	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q", scheme)
	}
}

// dialContextTCP wraps net.Dialer.DialContext with a sane default timeout so
// we respect context cancellation and don't hang indefinitely on silent
// networks.
func dialContextTCP(ctx context.Context, addr string) (net.Conn, error) {
	d := net.Dialer{Timeout: 30 * time.Second}
	return d.DialContext(ctx, "tcp", addr)
}

// dialHTTPConnect opens a TCP connection to an HTTP(S) proxy, sends a
// CONNECT request for the real target, and returns the resulting tunnel.
// For https:// proxies the hop-to-proxy is itself TLS-wrapped first. The
// uTLS handshake for the real target runs on top of this tunnel by the
// caller, so Cloudflare still sees a Chrome-fingerprinted ClientHello.
func (t *chromeTransport) dialHTTPConnect(ctx context.Context, targetAddr string) (net.Conn, error) {
	proxyHost := t.proxyURL.Host
	if _, _, err := net.SplitHostPort(proxyHost); err != nil {
		// No explicit port — pick a sane default per scheme.
		if strings.EqualFold(t.proxyURL.Scheme, "https") {
			proxyHost = net.JoinHostPort(proxyHost, "443")
		} else {
			proxyHost = net.JoinHostPort(proxyHost, "8080")
		}
	}

	conn, err := dialContextTCP(ctx, proxyHost)
	if err != nil {
		return nil, fmt.Errorf("dial proxy %s: %w", proxyHost, err)
	}

	// For an https:// proxy, wrap the hop in stdlib TLS (not uTLS — the
	// fingerprint we care about is the one the *real* target sees, and that
	// one is on the inner handshake).
	if strings.EqualFold(t.proxyURL.Scheme, "https") {
		host, _, _ := net.SplitHostPort(proxyHost)
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("proxy tls handshake %s: %w", proxyHost, err)
		}
		conn = tlsConn
	}

	var reqBuf strings.Builder
	reqBuf.WriteString("CONNECT ")
	reqBuf.WriteString(targetAddr)
	reqBuf.WriteString(" HTTP/1.1\r\nHost: ")
	reqBuf.WriteString(targetAddr)
	reqBuf.WriteString("\r\nProxy-Connection: keep-alive\r\nUser-Agent: kraube/connect\r\n")
	if t.proxyURL.User != nil {
		user := t.proxyURL.User.Username()
		pass, _ := t.proxyURL.User.Password()
		creds := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
		reqBuf.WriteString("Proxy-Authorization: Basic ")
		reqBuf.WriteString(creds)
		reqBuf.WriteString("\r\n")
	}
	reqBuf.WriteString("\r\n")

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	}

	if _, err := conn.Write([]byte(reqBuf.String())); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("proxy CONNECT write: %w", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: "CONNECT"})
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("proxy CONNECT read: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_ = conn.Close()
		return nil, fmt.Errorf("proxy CONNECT %s: HTTP %d %s", targetAddr, resp.StatusCode, resp.Status)
	}

	// Clear the deadline — the caller will run its own TLS handshake.
	_ = conn.SetDeadline(time.Time{})

	// If the reader buffered bytes beyond the response (shouldn't happen for
	// CONNECT, but belt-and-suspenders), wrap the conn so subsequent reads
	// see those bytes first.
	if br.Buffered() > 0 {
		peeked, _ := br.Peek(br.Buffered())
		conn = &prefixedConn{Conn: conn, prefix: append([]byte(nil), peeked...)}
	}

	logDebug("transport: proxy CONNECT ok", "proxy", redactProxyURL(t.proxyURL), "target", targetAddr)
	return conn, nil
}

// dialSOCKS5 routes via a SOCKS5 proxy using golang.org/x/net/proxy. The
// socks5h scheme semantically means "resolve target hostname on the proxy",
// which is also what x/net/proxy does with any hostname argument, so we
// treat both the same.
func (t *chromeTransport) dialSOCKS5(ctx context.Context, targetAddr string) (net.Conn, error) {
	var auth *xproxy.Auth
	if t.proxyURL.User != nil {
		pass, _ := t.proxyURL.User.Password()
		auth = &xproxy.Auth{User: t.proxyURL.User.Username(), Password: pass}
	}

	d, err := xproxy.SOCKS5("tcp", t.proxyURL.Host, auth, &net.Dialer{Timeout: 30 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("socks5 dialer: %w", err)
	}
	// Prefer context-aware dial when supported (it is in modern x/net/proxy).
	if cd, ok := d.(xproxy.ContextDialer); ok {
		conn, err := cd.DialContext(ctx, "tcp", targetAddr)
		if err != nil {
			return nil, fmt.Errorf("socks5 dial %s: %w", targetAddr, err)
		}
		logDebug("transport: socks5 dial ok", "proxy", redactProxyURL(t.proxyURL), "target", targetAddr)
		return conn, nil
	}
	conn, err := d.Dial("tcp", targetAddr)
	if err != nil {
		return nil, fmt.Errorf("socks5 dial %s: %w", targetAddr, err)
	}
	logDebug("transport: socks5 dial ok", "proxy", redactProxyURL(t.proxyURL), "target", targetAddr)
	return conn, nil
}

// prefixedConn prepends a byte slice to an existing net.Conn's Read stream.
// Used when bufio.Reader buffered extra bytes beyond the CONNECT response.
type prefixedConn struct {
	net.Conn
	prefix []byte
}

func (p *prefixedConn) Read(b []byte) (int, error) {
	if len(p.prefix) > 0 {
		n := copy(b, p.prefix)
		p.prefix = p.prefix[n:]
		return n, nil
	}
	return p.Conn.Read(b)
}

func (t *chromeTransport) roundTripH2(conn net.Conn, req *http.Request) (*http.Response, error) {
	if t.h2Transport == nil {
		t.h2Transport = &http2.Transport{
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				// This won't be called — we pass the conn directly.
				return nil, fmt.Errorf("unexpected DialTLSContext call")
			},
		}
	}

	cc, err := t.h2Transport.NewClientConn(conn)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("h2 client conn: %w", err)
	}
	return cc.RoundTrip(req)
}

func (t *chromeTransport) roundTripH1(conn net.Conn, req *http.Request) (*http.Response, error) {
	tr := &http.Transport{
		DialTLS: func(network, addr string) (net.Conn, error) {
			return conn, nil
		},
	}
	return tr.RoundTrip(req)
}
