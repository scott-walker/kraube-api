package kraube

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

// newChromeTransport creates an http.RoundTripper that mimics Chrome's TLS fingerprint
// using uTLS with HelloChrome_Auto. This is required because Cloudflare on
// api.anthropic.com blocks Go's default TLS fingerprint with 429 for OAuth
// Bearer token requests.
func newChromeTransport() http.RoundTripper {
	return &chromeTransport{}
}

type chromeTransport struct {
	h2Transport *http2.Transport
}

func (t *chromeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	host, port, err := net.SplitHostPort(req.URL.Host)
	if err != nil {
		host = req.URL.Host
		port = "443"
	}
	addr := net.JoinHostPort(host, port)

	rawConn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}

	uconn := utls.UClient(rawConn, &utls.Config{
		ServerName: host,
	}, utls.HelloChrome_Auto)

	if err := uconn.Handshake(); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("utls handshake %s: %w", addr, err)
	}

	alpn := uconn.ConnectionState().NegotiatedProtocol
	logDebug("transport: utls connected", "addr", addr, "alpn", alpn)

	if alpn == "h2" {
		return t.roundTripH2(uconn, req)
	}
	return t.roundTripH1(uconn, req)
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
		conn.Close()
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
