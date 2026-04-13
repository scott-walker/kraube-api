package kraube

import (
	"testing"
)

func TestResolveProxy_ExplicitWinsOverEnv(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://env-proxy:1")
	t.Setenv("ALL_PROXY", "socks5://env-all:2")

	u, err := resolveProxy("http://explicit:9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u == nil || u.Host != "explicit:9" {
		t.Fatalf("want explicit:9, got %v", u)
	}
}

func TestResolveProxy_EnvPriority(t *testing.T) {
	// HTTPS_PROXY must win over ALL_PROXY.
	t.Setenv("HTTPS_PROXY", "http://https-env:1")
	t.Setenv("ALL_PROXY", "http://all-env:2")

	u, err := resolveProxy("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u == nil || u.Host != "https-env:1" {
		t.Fatalf("want https-env:1, got %v", u)
	}
}

func TestResolveProxy_FallsBackToAllProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("https_proxy", "")
	t.Setenv("ALL_PROXY", "socks5://all-env:1080")

	u, err := resolveProxy("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u == nil || u.Scheme != "socks5" || u.Host != "all-env:1080" {
		t.Fatalf("want socks5://all-env:1080, got %v", u)
	}
}

func TestResolveProxy_NoEnvNoExplicit(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("https_proxy", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("all_proxy", "")

	u, err := resolveProxy("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u != nil {
		t.Fatalf("want nil, got %v", u)
	}
}

func TestResolveProxy_BareHostPort(t *testing.T) {
	u, err := resolveProxy("proxy.example.com:8080")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u == nil || u.Scheme != "http" || u.Host != "proxy.example.com:8080" {
		t.Fatalf("want http://proxy.example.com:8080, got %v", u)
	}
}

func TestResolveProxy_UnsupportedScheme(t *testing.T) {
	if _, err := resolveProxy("ftp://bad:21"); err == nil {
		t.Fatal("want error for ftp scheme")
	}
}

func TestResolveProxy_SupportedSchemes(t *testing.T) {
	for _, scheme := range []string{"http", "https", "socks5", "socks5h"} {
		if _, err := resolveProxy(scheme + "://host:1"); err != nil {
			t.Errorf("scheme %q: unexpected error: %v", scheme, err)
		}
	}
}

func TestRedactProxyURL(t *testing.T) {
	u, _ := resolveProxy("http://alice:secret@proxy:8080")
	got := redactProxyURL(u)
	want := "http://alice:xxxxx@proxy:8080"
	if got != want {
		t.Errorf("redact = %q, want %q", got, want)
	}

	u2, _ := resolveProxy("http://proxy:8080")
	if got := redactProxyURL(u2); got != "http://proxy:8080" {
		t.Errorf("no-creds redact = %q", got)
	}

	if got := redactProxyURL(nil); got != "" {
		t.Errorf("nil redact = %q", got)
	}
}
