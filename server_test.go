package kraube

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// upstreamRecorder captures the last request the mock Anthropic upstream
// received, so tests can assert that the server applied the full injection
// pipeline (identity/billing system prefix, auth, beta headers) before
// forwarding.
type upstreamRecorder struct {
	mu       sync.Mutex
	lastPath string
	lastAuth string
	lastBeta string
	lastBody []byte
}

func (u *upstreamRecorder) record(r *http.Request, body []byte) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.lastPath = r.URL.Path
	u.lastAuth = r.Header.Get("Authorization")
	u.lastBeta = r.Header.Get("Anthropic-Beta")
	u.lastBody = body
}

func (u *upstreamRecorder) snapshot() (path, auth, beta string, body []byte) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.lastPath, u.lastAuth, u.lastBeta, u.lastBody
}

// newMockUpstream builds a fake Anthropic API server. Non-streaming
// /v1/messages returns a fixed JSON message; count_tokens returns a fixed
// count. Streaming behavior is provided by the test via streamFn (nil =
// no streaming expected).
func newMockUpstream(t *testing.T, rec *upstreamRecorder, streamFn http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rec.record(r, body)

		switch r.URL.Path {
		case "/v1/messages":
			if bytes.Contains(body, []byte(`"stream":true`)) {
				if streamFn == nil {
					t.Error("unexpected streaming request")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				streamFn(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Request-Id", "req_mock_1")
			_, _ = io.WriteString(w, `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"hello from upstream"}],"model":"claude-sonnet-4-6","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":2}}`)
		case "/v1/messages/count_tokens":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"input_tokens":42}`)
		default:
			t.Errorf("unexpected upstream path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// newServeTestClient builds a Client wired entirely to mocks: in-memory
// token manager seeded via the (already overridden) mock OAuth endpoint,
// mock upstream base URL, plain HTTP transport, no profile fetch, and a
// temp rate-limit path so nothing under ~/.config is ever touched.
func newServeTestClient(t *testing.T, upstreamURL string) *Client {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := NewClient(ctx,
		WithToken("seed-refresh"),
		WithBaseURL(upstreamURL),
		WithHTTPClient(&http.Client{}),
		WithoutProfile(),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.RateLimitPath = filepath.Join(t.TempDir(), "ratelimit.json")
	return client
}

// startTestServer starts a Server on an ephemeral loopback port and wires
// its shutdown into test cleanup. Returns the base URL.
func startTestServer(t *testing.T, client *Client, cfg ServerConfig) (*Server, string) {
	t.Helper()
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:0"
	}
	srv, err := NewServer(client, cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	return srv, "http://" + srv.Addr()
}

// waitFor polls cond until it returns true or the deadline passes.
func waitFor(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestServer_ProxyMessages(t *testing.T) {
	oauth := newMockTokenServer(t, "access-serve", "rotated")
	defer oauth.Close()
	withMockTokenURL(t, oauth)

	rec := &upstreamRecorder{}
	upstream := newMockUpstream(t, rec, nil)
	defer upstream.Close()

	client := newServeTestClient(t, upstream.URL)
	_, base := startTestServer(t, client, ServerConfig{})

	resp, err := http.Post(base+"/v1/messages", "application/json",
		strings.NewReader(`{"model":"claude-sonnet-4-6","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "hello from upstream") {
		t.Errorf("body not passed through: %s", body)
	}
	if got := resp.Header.Get("Request-Id"); got != "req_mock_1" {
		t.Errorf("upstream headers not passed through, Request-Id = %q", got)
	}

	// The upstream must see the fully prepared request — this is the whole
	// point of proxying through the Client instead of blind forwarding.
	path, auth, beta, upBody := rec.snapshot()
	if path != "/v1/messages" {
		t.Errorf("upstream path = %q", path)
	}
	if auth != "Bearer access-serve" {
		t.Errorf("upstream auth = %q, want Bearer access-serve", auth)
	}
	if beta != betaBasic {
		t.Errorf("upstream beta header = %q, want basic set", beta)
	}
	var sent MessageRequest
	if err := json.Unmarshal(upBody, &sent); err != nil {
		t.Fatalf("unmarshal upstream body: %v", err)
	}
	if sent.System == nil || len(sent.System.Blocks) < 2 {
		t.Fatalf("system prefix not injected: %s", upBody)
	}
	if sent.System.Blocks[0].Text != claudeCodeIdentity {
		t.Errorf("identity preamble missing, block[0] = %q", sent.System.Blocks[0].Text)
	}
	if sent.System.Blocks[1].Text != billingHeader {
		t.Errorf("billing header missing, block[1] = %q", sent.System.Blocks[1].Text)
	}
}

func TestServer_ProxyMessages_Streaming(t *testing.T) {
	oauth := newMockTokenServer(t, "access-stream", "rotated")
	defer oauth.Close()
	withMockTokenURL(t, oauth)

	const chunk1 = "event: message_start\ndata: {\"type\":\"message_start\"}\n\n"
	const chunk2 = "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	release := make(chan struct{})

	rec := &upstreamRecorder{}
	upstream := newMockUpstream(t, rec, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl := w.(http.Flusher)
		_, _ = io.WriteString(w, chunk1)
		fl.Flush()
		// Hold the stream open until the test confirms chunk1 arrived.
		// This proves the proxy forwards bytes as they come rather than
		// buffering the whole upstream body.
		<-release
		_, _ = io.WriteString(w, chunk2)
		fl.Flush()
	})
	defer upstream.Close()

	client := newServeTestClient(t, upstream.URL)
	_, base := startTestServer(t, client, ServerConfig{})

	resp, err := http.Post(base+"/v1/messages", "application/json",
		strings.NewReader(`{"model":"claude-sonnet-4-6","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream (passed through)", ct)
	}

	reader := bufio.NewReader(resp.Body)
	firstCh := make(chan []byte, 1)
	go func() {
		buf := make([]byte, len(chunk1))
		if _, err := io.ReadFull(reader, buf); err == nil {
			firstCh <- buf
		}
	}()
	select {
	case got := <-firstCh:
		if string(got) != chunk1 {
			t.Fatalf("first chunk = %q, want %q", got, chunk1)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first SSE chunk not delivered while stream is open — the proxy is buffering instead of flushing")
	}

	close(release)
	rest, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read rest: %v", err)
	}
	if string(rest) != chunk2 {
		t.Errorf("remaining bytes = %q, want %q (byte-for-byte SSE passthrough)", rest, chunk2)
	}
}

func TestServer_ProxyCountTokens(t *testing.T) {
	oauth := newMockTokenServer(t, "access-ct", "rotated")
	defer oauth.Close()
	withMockTokenURL(t, oauth)

	rec := &upstreamRecorder{}
	upstream := newMockUpstream(t, rec, nil)
	defer upstream.Close()

	client := newServeTestClient(t, upstream.URL)
	_, base := startTestServer(t, client, ServerConfig{})

	resp, err := http.Post(base+"/v1/messages/count_tokens", "application/json",
		strings.NewReader(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"input_tokens":42`) {
		t.Errorf("body = %s, want input_tokens 42", body)
	}
	path, _, _, _ := rec.snapshot()
	if path != "/v1/messages/count_tokens" {
		t.Errorf("upstream path = %q", path)
	}
}

func TestServer_AuthKey(t *testing.T) {
	oauth := newMockTokenServer(t, "access-auth", "rotated")
	defer oauth.Close()
	withMockTokenURL(t, oauth)

	rec := &upstreamRecorder{}
	upstream := newMockUpstream(t, rec, nil)
	defer upstream.Close()

	client := newServeTestClient(t, upstream.URL)
	_, base := startTestServer(t, client, ServerConfig{AuthKey: "sekret"})

	post := func(mutate func(*http.Request)) int {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, base+"/v1/messages",
			strings.NewReader(`{"model":"claude-sonnet-4-6","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`))
		req.Header.Set("Content-Type", "application/json")
		if mutate != nil {
			mutate(req)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}

	if got := post(nil); got != http.StatusUnauthorized {
		t.Errorf("no key: status = %d, want 401", got)
	}
	if got := post(func(r *http.Request) { r.Header.Set("Authorization", "Bearer wrong") }); got != http.StatusUnauthorized {
		t.Errorf("wrong key: status = %d, want 401", got)
	}
	if got := post(func(r *http.Request) { r.Header.Set("Authorization", "Bearer sekret") }); got != http.StatusOK {
		t.Errorf("bearer key: status = %d, want 200", got)
	}
	if got := post(func(r *http.Request) { r.Header.Set("x-api-key", "sekret") }); got != http.StatusOK {
		t.Errorf("x-api-key: status = %d, want 200", got)
	}

	// /healthz must stay reachable without the key (liveness probes).
	resp, err := http.Get(base + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("healthz without key: status = %d, want 200", resp.StatusCode)
	}
}

func TestServer_Healthz_OK(t *testing.T) {
	oauth := newMockTokenServer(t, "access-hz", "rotated")
	defer oauth.Close()
	withMockTokenURL(t, oauth)

	rec := &upstreamRecorder{}
	upstream := newMockUpstream(t, rec, nil)
	defer upstream.Close()

	client := newServeTestClient(t, upstream.URL)
	_, base := startTestServer(t, client, ServerConfig{CheckInterval: 20 * time.Millisecond})

	// Let several keepalive ticks pass. The token is live for the whole
	// margin, so every tick is a no-op — and no-ops must NOT be reported as
	// refreshes (the old behaviour stamped last_refresh_at at startup).
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get(base + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var h healthResponse
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if h.Status != "ok" {
		t.Errorf("status = %q, want ok", h.Status)
	}
	if !h.AccessTokenLive {
		t.Error("access_token_live = false, want true")
	}
	if h.ExpiresAt == "" || h.ExpiresIn == "" {
		t.Errorf("expiry fields missing: %+v", h)
	}
	if h.StartedAt == "" {
		t.Error("started_at missing")
	}
	if h.Uptime == "" {
		t.Error("uptime missing")
	}
	// No refresh has actually been performed — the trio must be absent.
	if h.LastRefreshAt != "" || h.LastRefreshOK != nil || h.LastRefreshError != "" {
		t.Errorf("last_refresh_* reported without an actual refresh: at=%q ok=%v err=%q",
			h.LastRefreshAt, h.LastRefreshOK, h.LastRefreshError)
	}
}

// TestServer_Healthz_RecordsActualRefresh is the positive half of the
// last_refresh_* semantics: once the keepalive performs a real rotation,
// /healthz reports its time and outcome.
func TestServer_Healthz_RecordsActualRefresh(t *testing.T) {
	oauth := newMockTokenServer(t, "access-rotated", "refresh-rotated")
	defer oauth.Close()
	withMockTokenURL(t, oauth)

	rec := &upstreamRecorder{}
	upstream := newMockUpstream(t, rec, nil)
	defer upstream.Close()

	client := newServeTestClient(t, upstream.URL)

	// Age the token into the refresh margin (2 minutes left vs the default
	// 10-minute margin): the first keepalive tick must perform a real
	// refresh and record it.
	tm, ok := client.Provider.(*tokenManager)
	if !ok {
		t.Fatalf("Provider is %T, want *tokenManager", client.Provider)
	}
	tm.mu.Lock()
	tm.creds.ExpiresAt = time.Now().Add(2 * time.Minute).UnixMilli()
	tm.mu.Unlock()

	_, base := startTestServer(t, client, ServerConfig{CheckInterval: 20 * time.Millisecond})

	var h healthResponse
	waitFor(t, 5*time.Second, "refresh to be recorded", func() bool {
		resp, err := http.Get(base + "/healthz")
		if err != nil {
			return false
		}
		defer func() { _ = resp.Body.Close() }()
		if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
			return false
		}
		return h.LastRefreshOK != nil
	})

	if !*h.LastRefreshOK {
		t.Errorf("last_refresh_ok = false, want true (error %q)", h.LastRefreshError)
	}
	if h.LastRefreshAt == "" {
		t.Error("last_refresh_at missing after an actual refresh")
	}
	if h.LastRefreshError != "" {
		t.Errorf("last_refresh_error = %q, want empty", h.LastRefreshError)
	}
	if !h.AccessTokenLive {
		t.Error("access_token_live = false after successful rotation, want true")
	}
}

func TestServer_Healthz_DeadTokenFailingRefresh(t *testing.T) {
	// OAuth endpoint that works exactly once (for NewClient's initial token)
	// and then starts failing — simulating a platform.claude.com outage after
	// the daemon has been running for a while.
	var fail atomic.Bool
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `oauth down`)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-initial",
			"refresh_token": "refresh-kept",
			"expires_in":    3600,
		})
	}))
	defer oauth.Close()
	withMockTokenURL(t, oauth)

	rec := &upstreamRecorder{}
	upstream := newMockUpstream(t, rec, nil)
	defer upstream.Close()

	client := newServeTestClient(t, upstream.URL)
	fail.Store(true)

	// Kill the access token so the keepalive is forced into a real refresh.
	tm, ok := client.Provider.(*tokenManager)
	if !ok {
		t.Fatalf("Provider is %T, want *tokenManager", client.Provider)
	}
	tm.mu.Lock()
	tm.creds.ExpiresAt = time.Now().UnixMilli()
	tm.mu.Unlock()

	_, base := startTestServer(t, client, ServerConfig{CheckInterval: 20 * time.Millisecond})

	var h healthResponse
	waitFor(t, 5*time.Second, "healthz to report 503", func() bool {
		resp, err := http.Get(base + "/healthz")
		if err != nil {
			return false
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusServiceUnavailable {
			return false
		}
		return json.NewDecoder(resp.Body).Decode(&h) == nil
	})

	if h.Status != "degraded" {
		t.Errorf("status = %q, want degraded", h.Status)
	}
	if h.AccessTokenLive {
		t.Error("access_token_live = true, want false")
	}
	if h.LastRefreshError == "" {
		t.Error("last_refresh_error missing")
	}
	if h.LastRefreshAt == "" {
		t.Error("last_refresh_at missing — a failed attempt is still an actual refresh attempt")
	}
	if h.LastRefreshOK == nil || *h.LastRefreshOK {
		t.Errorf("last_refresh_ok = %v, want false", h.LastRefreshOK)
	}
}

// TestServer_KeepaliveProactiveRefresh is the core serve guarantee: a token
// that is still live for the lazy 60-second policy but inside the serve
// refresh margin gets rotated in the background — and exactly once, since
// subsequent ticks see a fresh token.
func TestServer_KeepaliveProactiveRefresh(t *testing.T) {
	var calls atomic.Int32
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-rotated",
			"refresh_token": "refresh-rotated",
			"expires_in":    3600,
		})
	}))
	defer oauth.Close()
	withMockTokenURL(t, oauth)

	rec := &upstreamRecorder{}
	upstream := newMockUpstream(t, rec, nil)
	defer upstream.Close()

	// Credentials file with an access token that expires in 5 minutes:
	// live for Token() (margin 60s), stale for serve (margin 10m).
	credsPath := filepath.Join(t.TempDir(), "credentials.json")
	if err := SaveCredentials(credsPath, &Credentials{
		RefreshToken: "refresh-original",
		AccessToken:  "access-aging",
		ExpiresAt:    time.Now().Add(5 * time.Minute).UnixMilli(),
	}); err != nil {
		t.Fatalf("seed credentials: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := NewClient(ctx,
		WithTokenFile(credsPath),
		WithBaseURL(upstream.URL),
		WithHTTPClient(&http.Client{}),
		WithoutProfile(),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.RateLimitPath = filepath.Join(t.TempDir(), "ratelimit.json")
	if c := calls.Load(); c != 0 {
		t.Fatalf("NewClient triggered %d OAuth calls, want 0 (token still live)", c)
	}

	startTestServer(t, client, ServerConfig{
		RefreshMargin: 10 * time.Minute,
		CheckInterval: 20 * time.Millisecond,
	})

	waitFor(t, 5*time.Second, "proactive rotation to land on disk", func() bool {
		creds, err := LoadCredentials(credsPath)
		return err == nil && creds.RefreshToken == "refresh-rotated"
	})

	creds, err := LoadCredentials(credsPath)
	if err != nil {
		t.Fatalf("reload credentials: %v", err)
	}
	if creds.AccessToken != "access-rotated" {
		t.Errorf("on-disk accessToken = %q, want access-rotated", creds.AccessToken)
	}
	if remaining := time.Until(time.UnixMilli(creds.ExpiresAt)); remaining < 50*time.Minute {
		t.Errorf("on-disk expiresAt only %s away, want ~1h", remaining)
	}

	// Let several more ticks pass: the fresh token must not be re-rotated.
	time.Sleep(100 * time.Millisecond)
	if c := calls.Load(); c != 1 {
		t.Errorf("OAuth refresh calls = %d, want exactly 1", c)
	}
}

func TestServer_Usage(t *testing.T) {
	oauth := newMockTokenServer(t, "access-usage", "rotated")
	defer oauth.Close()
	withMockTokenURL(t, oauth)

	rec := &upstreamRecorder{}
	upstream := newMockUpstream(t, rec, nil)
	defer upstream.Close()

	client := newServeTestClient(t, upstream.URL)
	_, base := startTestServer(t, client, ServerConfig{})

	// Empty cache — /usage must say so instead of making a paid API call.
	resp, err := http.Get(base + "/usage")
	if err != nil {
		t.Fatalf("GET /usage: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("empty cache: status = %d, want 404", resp.StatusCode)
	}

	// Seed the on-disk cache (as a previous proxied call would have).
	if err := SaveRateLimit(client.RateLimitPath, &RateLimitInfo{
		Status:    "allowed",
		FiveHour:  &RateLimitWindow{Utilization: 0.42, ResetsAt: time.Now().Add(2 * time.Hour)},
		SevenDay:  &RateLimitWindow{Utilization: 0.1, ResetsAt: time.Now().Add(100 * time.Hour)},
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("SaveRateLimit: %v", err)
	}

	resp, err = http.Get(base + "/usage")
	if err != nil {
		t.Fatalf("GET /usage: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var u usageResponse
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if u.Status != "allowed" {
		t.Errorf("status = %q, want allowed", u.Status)
	}
	if u.FiveHour == nil || u.FiveHour.UsedPercent < 41.9 || u.FiveHour.UsedPercent > 42.1 {
		t.Errorf("five_hour = %+v, want ~42%%", u.FiveHour)
	}
	if u.SevenDay == nil {
		t.Error("seven_day missing")
	}
}

func TestServer_BadRequests(t *testing.T) {
	oauth := newMockTokenServer(t, "access-bad", "rotated")
	defer oauth.Close()
	withMockTokenURL(t, oauth)

	rec := &upstreamRecorder{}
	upstream := newMockUpstream(t, rec, nil)
	defer upstream.Close()

	client := newServeTestClient(t, upstream.URL)
	_, base := startTestServer(t, client, ServerConfig{})

	// Malformed JSON → 400 with an Anthropic-shaped error body.
	resp, err := http.Post(base+"/v1/messages", "application/json", strings.NewReader(`{not json`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad JSON: status = %d, want 400", resp.StatusCode)
	}
	if !strings.Contains(string(body), "invalid_request_error") {
		t.Errorf("bad JSON body = %s, want invalid_request_error", body)
	}

	// Wrong method → 405.
	resp, err = http.Get(base + "/v1/messages")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /v1/messages: status = %d, want 405", resp.StatusCode)
	}
}

func TestNewServer_RefusesNonLoopbackWithoutKey(t *testing.T) {
	oauth := newMockTokenServer(t, "access-lb", "rotated")
	defer oauth.Close()
	withMockTokenURL(t, oauth)

	rec := &upstreamRecorder{}
	upstream := newMockUpstream(t, rec, nil)
	defer upstream.Close()
	client := newServeTestClient(t, upstream.URL)

	for _, addr := range []string{"0.0.0.0:8787", ":8787", "192.168.1.10:8787", "example.com:8787"} {
		_, err := NewServer(client, ServerConfig{Addr: addr})
		if err == nil || !strings.Contains(err.Error(), "auth key") {
			t.Errorf("NewServer(%q) err = %v, want refusal mentioning auth key", addr, err)
		}
	}

	// Loopback without a key is fine.
	for _, addr := range []string{"127.0.0.1:0", "localhost:0", "[::1]:0"} {
		if _, err := NewServer(client, ServerConfig{Addr: addr}); err != nil {
			t.Errorf("NewServer(%q) err = %v, want nil", addr, err)
		}
	}

	// Non-loopback with a key passes validation (not started — binding
	// 0.0.0.0 in tests is pointless).
	if _, err := NewServer(client, ServerConfig{Addr: "0.0.0.0:8787", AuthKey: "k"}); err != nil {
		t.Errorf("NewServer with key err = %v, want nil", err)
	}
}
