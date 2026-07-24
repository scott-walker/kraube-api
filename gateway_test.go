package kraube

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// daemonRecorder captures what a fake `kraube serve` daemon receives from a
// gateway client, so tests can assert on the wire shape: auth header, path,
// and the un-injected request body.
type daemonRecorder struct {
	mu       sync.Mutex
	lastPath string
	lastAuth string
	lastBody []byte
}

func (d *daemonRecorder) record(r *http.Request, body []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastPath = r.URL.Path
	d.lastAuth = r.Header.Get("Authorization")
	d.lastBody = body
}

func (d *daemonRecorder) snapshot() (path, auth string, body []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lastPath, d.lastAuth, d.lastBody
}

// newFakeDaemon builds a fake daemon endpoint that records requests and
// answers /v1/messages with a fixed message.
func newFakeDaemon(t *testing.T, rec *daemonRecorder) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rec.record(r, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"msg_gw","type":"message","role":"assistant","content":[{"type":"text","text":"via daemon"}],"model":"claude-sonnet-4-6","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":2}}`)
	}))
}

// newGatewayTestClient builds a WithGateway client pointed at the fake
// daemon, with a temp rate-limit path so nothing under ~/.config is touched.
func newGatewayTestClient(t *testing.T, daemonURL, key string) *Client {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := NewClient(ctx, WithGateway(daemonURL, key))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.RateLimitPath = filepath.Join(t.TempDir(), "ratelimit.json")
	return client
}

func TestGateway_SendsKeyAndSkipsInjection(t *testing.T) {
	rec := &daemonRecorder{}
	daemon := newFakeDaemon(t, rec)
	defer daemon.Close()

	client := newGatewayTestClient(t, daemon.URL, "sekret")

	resp, err := client.Messages.Create(context.Background(), &MessageRequest{
		Model:     ModelSonnet4_6,
		MaxTokens: 16,
		System:    SystemText("custom system"),
		Messages:  []Message{UserMessage("hi")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if resp.ID != "msg_gw" {
		t.Errorf("response ID = %q, want msg_gw", resp.ID)
	}

	path, auth, body := rec.snapshot()
	if path != "/v1/messages" {
		t.Errorf("path = %q, want /v1/messages", path)
	}
	if auth != "Bearer sekret" {
		t.Errorf("Authorization = %q, want Bearer sekret", auth)
	}
	// No client-side injection: system must be exactly the caller's prompt.
	var sent struct {
		System json.RawMessage `json:"system"`
	}
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if string(sent.System) != `"custom system"` {
		t.Errorf("system = %s, want the raw caller prompt without identity/billing prefix", sent.System)
	}
	if strings.Contains(string(body), claudeCodeIdentity) {
		t.Error("request body contains the identity preamble — gateway client must not inject")
	}
}

func TestGateway_EmptyKeySendsNoAuthHeader(t *testing.T) {
	rec := &daemonRecorder{}
	daemon := newFakeDaemon(t, rec)
	defer daemon.Close()

	client := newGatewayTestClient(t, daemon.URL, "")

	if _, err := client.Messages.Create(context.Background(), &MessageRequest{
		Model:     ModelSonnet4_6,
		MaxTokens: 16,
		Messages:  []Message{UserMessage("hi")},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, auth, _ := rec.snapshot()
	if auth != "" {
		t.Errorf("Authorization = %q, want no header for an empty key", auth)
	}
}

func TestGateway_NoOAuthTraffic(t *testing.T) {
	// A gateway client must never touch the OAuth endpoint: point the
	// package-level token URL at a server that fails the test when hit.
	trap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("gateway client hit the OAuth token endpoint")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer trap.Close()
	withMockTokenURL(t, trap)

	rec := &daemonRecorder{}
	daemon := newFakeDaemon(t, rec)
	defer daemon.Close()

	client := newGatewayTestClient(t, daemon.URL, "k")
	if _, err := client.Messages.Create(context.Background(), &MessageRequest{
		Model:     ModelSonnet4_6,
		MaxTokens: 16,
		Messages:  []Message{UserMessage("hi")},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// EnsureFresh degrades to a plain Token() call for the static provider —
	// still no OAuth traffic.
	if err := client.EnsureFresh(context.Background(), time.Hour); err != nil {
		t.Fatalf("EnsureFresh: %v", err)
	}
}

func TestInjectSystemPrefix_Idempotent(t *testing.T) {
	c := &Client{}

	req := &MessageRequest{System: SystemText("user prompt")}
	c.injectSystemPrefix(req)
	c.injectSystemPrefix(req)

	if req.System == nil || req.System.Blocks == nil {
		t.Fatal("system prompt not converted to blocks")
	}
	var identityCount int
	for _, b := range req.System.Blocks {
		if b.Text == claudeCodeIdentity {
			identityCount++
		}
	}
	if identityCount != 1 {
		t.Errorf("identity preamble injected %d times, want exactly 1", identityCount)
	}
	if len(req.System.Blocks) != 3 {
		t.Errorf("system blocks = %d, want 3 (identity, billing, user)", len(req.System.Blocks))
	}
	if req.System.Blocks[2].Text != "user prompt" {
		t.Errorf("user block = %q, want the original prompt preserved", req.System.Blocks[2].Text)
	}
}

// TestServe_FullClientNoDoubleInjection covers the other half of the
// idempotency contract: a full (non-gateway) kraube client pointed at the
// daemon injects the prefix client-side, and the daemon's Raw path must not
// prepend a second copy before forwarding upstream.
func TestServe_FullClientNoDoubleInjection(t *testing.T) {
	oauth := newMockTokenServer(t, "access-dedup", "rotated")
	defer oauth.Close()
	withMockTokenURL(t, oauth)

	rec := &upstreamRecorder{}
	upstream := newMockUpstream(t, rec, nil)
	defer upstream.Close()

	serveClient := newServeTestClient(t, upstream.URL)
	_, baseURL := startTestServer(t, serveClient, ServerConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	full, err := NewClient(ctx,
		WithToken("seed-refresh"),
		WithBaseURL(baseURL),
		WithHTTPClient(&http.Client{}),
		WithoutProfile(),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	full.RateLimitPath = filepath.Join(t.TempDir(), "ratelimit.json")

	if _, err := full.Messages.Create(context.Background(), &MessageRequest{
		Model:     ModelSonnet4_6,
		MaxTokens: 16,
		System:    SystemText("user prompt"),
		Messages:  []Message{UserMessage("hi")},
	}); err != nil {
		t.Fatalf("Create via daemon: %v", err)
	}

	_, _, _, body := rec.snapshot()
	if got := strings.Count(string(body), claudeCodeIdentity); got != 1 {
		t.Errorf("upstream body contains the identity preamble %d times, want exactly 1", got)
	}
	if !strings.Contains(string(body), "user prompt") {
		t.Error("upstream body lost the caller's system prompt")
	}
}
