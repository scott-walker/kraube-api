package kraube

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Server is a long-lived local HTTP daemon in front of a *Client. It exposes
// the Messages API over plain HTTP on localhost so that any process on the
// machine can talk to Anthropic without linking the library, and — more
// importantly — it keeps the OAuth access token alive with a background
// refresh loop. Run one `kraube serve` per machine and make it the sole
// owner of credentials.json: single-use refresh tokens rotate in exactly one
// place instead of racing across short-lived CLI invocations.
//
// Endpoints:
//
//	POST /v1/messages              — proxy (streaming SSE passed through raw)
//	POST /v1/messages/count_tokens — proxy
//	GET  /healthz                  — token liveness + keepalive state (no auth)
//	GET  /usage                    — cached subscription rate-limit windows
//
// Construct with NewServer, then Start + Shutdown (tests) or Start + Wait
// (the blocking signal-driven lifecycle used by the CLI).
type Server struct {
	client *Client
	cfg    ServerConfig

	httpServer *http.Server
	listener   net.Listener
	startedAt  time.Time
	stopCh     chan struct{} // closed by Shutdown; stops the keepalive loop
	serveErr   chan error    // terminal error from the HTTP serve loop
	wg         sync.WaitGroup

	// Keepalive state, reported by /healthz. Only refreshes that were
	// actually performed (or attempted and failed) are recorded — a tick
	// that finds the token still fresh is a no-op, not a refresh, and a
	// zero lastRefreshAt means no real refresh has happened yet.
	mu             sync.Mutex
	lastRefreshAt  time.Time // last actual refresh attempt; zero = none yet
	lastRefreshErr error     // nil = last actual attempt succeeded
}

// ServerConfig configures a Server. Zero values fall back to defaults.
type ServerConfig struct {
	// Addr is the listen address. Default "127.0.0.1:8787". A non-loopback
	// address requires AuthKey — NewServer refuses to build an open proxy.
	Addr string

	// AuthKey, when non-empty, is required on every endpoint except /healthz
	// as `Authorization: Bearer <key>` or `x-api-key: <key>`.
	AuthKey string

	// RefreshMargin is how long before expiresAt the keepalive refreshes the
	// access token. Default 10 minutes.
	RefreshMargin time.Duration

	// CheckInterval is the keepalive tick. Default 1 minute. Exposed mainly
	// so tests can tighten it; production has no reason to change it.
	CheckInterval time.Duration
}

// refreshBackoff is the retry schedule after a failed background refresh:
// each consecutive failure moves one step right, capping at the last entry.
// A success resets to the plain CheckInterval cadence. Kept coarse on
// purpose — hammering a failing OAuth endpoint burns nothing but goodwill.
var refreshBackoff = []time.Duration{30 * time.Second, time.Minute, 5 * time.Minute}

// NewServer validates the configuration and builds a Server around client.
// It does not bind the listener yet — call Start (or Run).
func NewServer(client *Client, cfg ServerConfig) (*Server, error) {
	if client == nil {
		return nil, fmt.Errorf("serve: nil client")
	}
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:8787"
	}
	if cfg.RefreshMargin <= 0 {
		cfg.RefreshMargin = 10 * time.Minute
	}
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = time.Minute
	}
	// Refuse to run an unauthenticated proxy on a non-loopback interface.
	// Anyone who can reach the port can spend the subscription — that must be
	// an explicit decision (set a key), not a default.
	if cfg.AuthKey == "" && !isLoopbackAddr(cfg.Addr) {
		return nil, fmt.Errorf("serve: refusing to listen on non-loopback %s without an auth key (set --auth-key or KRAUBE_SERVE_KEY, or bind to 127.0.0.1)", cfg.Addr)
	}
	return &Server{client: client, cfg: cfg}, nil
}

// isLoopbackAddr reports whether the host part of addr is a loopback
// interface. Unresolvable hostnames count as non-loopback — when in doubt,
// demand a key.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Start binds the listener and launches the HTTP serve loop and the token
// keepalive goroutine. It returns once the port is bound; use Addr for the
// resolved address (useful with ":0") and Shutdown to stop.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return fmt.Errorf("serve: listen %s: %w", s.cfg.Addr, err)
	}
	s.listener = ln
	s.startedAt = time.Now()
	s.stopCh = make(chan struct{})
	s.serveErr = make(chan error, 1)
	s.httpServer = &http.Server{Handler: s.routes()}

	s.wg.Add(2)
	go func() {
		defer s.wg.Done()
		s.keepalive()
	}()
	go func() {
		defer s.wg.Done()
		err := s.httpServer.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logError("serve: http server failed", "error", err)
			s.serveErr <- err
			return
		}
		s.serveErr <- nil
	}()

	logInfo("serve: listening", "addr", ln.Addr().String(), "refresh_margin", s.cfg.RefreshMargin, "auth", s.cfg.AuthKey != "")
	return nil
}

// Addr returns the bound listen address. Empty before Start.
func (s *Server) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// Shutdown stops the keepalive loop and gracefully drains the HTTP server
// (in-flight requests, including open SSE streams, run until ctx expires).
func (s *Server) Shutdown(ctx context.Context) error {
	close(s.stopCh)
	err := s.httpServer.Shutdown(ctx)
	s.wg.Wait()
	return err
}

// Wait blocks until ctx is cancelled (typically by SIGINT/SIGTERM via
// signal.NotifyContext) or the HTTP serve loop dies on its own, then shuts
// the server down gracefully with a 10-second drain window. Call after a
// successful Start; this is the blocking lifecycle used by the CLI.
func (s *Server) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.Shutdown(shutdownCtx)
	case err := <-s.serveErr:
		// The HTTP loop died on its own — still stop the keepalive.
		close(s.stopCh)
		s.wg.Wait()
		return err
	}
}

// --- Keepalive ---

// keepalive proactively refreshes the access token so it never gets close
// to expiring. Runs one check immediately at startup (a daemon restarted
// after a long pause may hold a stale token), then re-checks every
// CheckInterval. Failed refreshes back off per refreshBackoff and never
// crash the process: a failing OAuth endpoint does not burn the single-use
// refresh token, so retrying later is always safe. Successful rotations go
// through the same refreshPersistent path as lazy refresh — including the
// writability check that refuses to rotate when credentials.json cannot be
// written back.
func (s *Server) keepalive() {
	ticker := time.NewTicker(s.cfg.CheckInterval)
	defer ticker.Stop()

	failures := 0
	var notBefore time.Time // backoff gate after failures

	for {
		if time.Now().After(notBefore) {
			if err := s.refreshOnce(); err != nil {
				step := failures
				if step >= len(refreshBackoff) {
					step = len(refreshBackoff) - 1
				}
				delay := refreshBackoff[step]
				failures++
				notBefore = time.Now().Add(delay)
				logError("serve: background refresh failed", "error", err, "retry_in", delay, "consecutive_failures", failures)
			} else {
				failures = 0
				notBefore = time.Time{}
			}
		}
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
		}
	}
}

// refreshOnce performs a single keepalive check and records its outcome for
// /healthz. EnsureFresh is a cheap no-op while the token outlives the margin,
// and a no-op is deliberately not recorded: last_refresh_at must mean "a
// refresh actually ran", not "the loop ticked". An actual refresh is
// detected as either a failure (EnsureFresh only errors when it attempted a
// refresh) or a moved expiry (the token was rotated, or a rotation by
// another process was adopted from disk). Custom TokenProviders expose no
// expiry, so for them only failed attempts are ever recorded.
func (s *Server) refreshOnce() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	expBefore, knownBefore := s.client.AccessExpiry()
	err := s.client.EnsureFresh(ctx, s.cfg.RefreshMargin)
	expAfter, knownAfter := s.client.AccessExpiry()

	refreshed := err != nil || knownBefore != knownAfter || (knownAfter && !expAfter.Equal(expBefore))
	if refreshed {
		s.mu.Lock()
		s.lastRefreshAt = time.Now()
		s.lastRefreshErr = err
		s.mu.Unlock()
	}

	if err == nil {
		if exp, ok := s.client.AccessExpiry(); ok {
			logDebug("serve: token checked", "expires_at", exp.Format(time.RFC3339), "expires_in", time.Until(exp).Truncate(time.Second))
		}
	}
	return err
}

// --- HTTP handlers ---

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	// /healthz stays unauthenticated by design: liveness probes (systemd,
	// monitoring) must not need the key, and the endpoint leaks nothing
	// beyond token expiry timing.
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/v1/messages", s.requireKey(s.handleMessages))
	mux.HandleFunc("/v1/messages/count_tokens", s.requireKey(s.handleCountTokens))
	mux.HandleFunc("/usage", s.requireKey(s.handleUsage))
	return mux
}

// requireKey enforces the auth key (when configured) as either
// `Authorization: Bearer <key>` or `x-api-key: <key>`.
func (s *Server) requireKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.AuthKey == "" {
			next(w, r)
			return
		}
		supplied := r.Header.Get("x-api-key")
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			supplied = strings.TrimPrefix(h, "Bearer ")
		}
		if subtle.ConstantTimeCompare([]byte(supplied), []byte(s.cfg.AuthKey)) != 1 {
			writeJSONError(w, http.StatusUnauthorized, "authentication_error", "missing or invalid auth key (use Authorization: Bearer <key> or x-api-key)")
			return
		}
		next(w, r)
	}
}

// handleMessages proxies POST /v1/messages. The body is decoded into a
// MessageRequest so the full injection pipeline (identity preamble, billing
// header, metadata, beta headers) applies — the same request sent to
// Anthropic directly would be rejected with 403/429 without it. The upstream
// response is then copied through verbatim: for "stream": true bodies that
// means raw SSE bytes flushed chunk-by-chunk, with no event re-parsing in
// between.
func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed, use POST")
		return
	}
	var req MessageRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("parse request body: %v", err))
		return
	}

	resp, err := s.client.Messages.Raw(r.Context(), &req)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "api_error", fmt.Sprintf("upstream request failed: %v", err))
		return
	}
	defer func() { _ = resp.Body.Close() }()
	proxyResponse(w, resp)
}

// handleCountTokens proxies POST /v1/messages/count_tokens.
func (s *Server) handleCountTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed, use POST")
		return
	}
	var req CountTokensRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("parse request body: %v", err))
		return
	}

	resp, err := s.client.Messages.CountTokensRaw(r.Context(), &req)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "api_error", fmt.Sprintf("upstream request failed: %v", err))
		return
	}
	defer func() { _ = resp.Body.Close() }()
	proxyResponse(w, resp)
}

// healthResponse is the GET /healthz payload. The last_refresh_* trio
// describes background refreshes that were actually performed (or attempted
// and failed); all three are absent until the first real refresh — ticks
// that find the token still fresh do not count. started_at is when the
// daemon started, which is deliberately a separate fact.
type healthResponse struct {
	Status           string `json:"status"`     // "ok" | "degraded"
	StartedAt        string `json:"started_at"` // RFC3339, daemon start time
	Uptime           string `json:"uptime"`
	AccessTokenLive  bool   `json:"access_token_live"`
	ExpiresAt        string `json:"expires_at,omitempty"`         // RFC3339
	ExpiresIn        string `json:"expires_in,omitempty"`         // negative when already expired
	LastRefreshAt    string `json:"last_refresh_at,omitempty"`    // absent until a refresh actually ran
	LastRefreshOK    *bool  `json:"last_refresh_ok,omitempty"`    // absent until a refresh actually ran
	LastRefreshError string `json:"last_refresh_error,omitempty"` // set when the last actual refresh failed
}

// handleHealthz reports token liveness and keepalive state. HTTP 503 only
// when the token is dead AND the last background refresh failed — that is
// the "requests will not work and the daemon cannot fix it" state. A dead
// token with no failed refresh yet (e.g. right after startup) is "degraded"
// but 200: the keepalive is about to repair it.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed, use GET")
		return
	}

	exp, known := s.client.AccessExpiry()
	live := known && time.Until(exp) > accessTokenMargin

	s.mu.Lock()
	lastAt, lastErr := s.lastRefreshAt, s.lastRefreshErr
	s.mu.Unlock()

	h := healthResponse{
		Status:          "ok",
		StartedAt:       s.startedAt.Format(time.RFC3339),
		Uptime:          time.Since(s.startedAt).Truncate(time.Second).String(),
		AccessTokenLive: live,
	}
	if known {
		h.ExpiresAt = exp.Format(time.RFC3339)
		h.ExpiresIn = time.Until(exp).Truncate(time.Second).String()
	}
	if !lastAt.IsZero() {
		h.LastRefreshAt = lastAt.Format(time.RFC3339)
		ok := lastErr == nil
		h.LastRefreshOK = &ok
		if lastErr != nil {
			h.LastRefreshError = lastErr.Error()
		}
	}

	status := http.StatusOK
	if !live {
		h.Status = "degraded"
		if lastErr != nil {
			status = http.StatusServiceUnavailable
		}
	}
	writeJSON(w, status, h)
}

// usageWindow is one rate-limit window in the GET /usage payload.
type usageWindow struct {
	UsedPercent float64 `json:"used_percent"`
	ResetsAt    string  `json:"resets_at"`
	ResetsIn    string  `json:"resets_in"`
}

// usageResponse is the GET /usage payload.
type usageResponse struct {
	Status    string       `json:"status,omitempty"` // "allowed", "rejected", "allowed_warning"
	Claim     string       `json:"claim,omitempty"`
	FiveHour  *usageWindow `json:"five_hour,omitempty"`
	SevenDay  *usageWindow `json:"seven_day,omitempty"`
	UpdatedAt string       `json:"updated_at"`
	Age       string       `json:"age"`
}

// handleUsage serves the cached subscription rate-limit windows. The data
// comes for free from response headers of proxied calls (kept in
// client.RateLimit and mirrored to the rate-limit cache file), so no
// upstream request is ever made here — an empty cache yields 404 rather
// than a paid probe.
func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed, use GET")
		return
	}

	rl := s.client.RateLimit
	if (rl == nil || !rl.HasData()) && s.client.RateLimitPath != "" {
		if disk, err := LoadRateLimit(s.client.RateLimitPath); err == nil {
			rl = disk
		}
	}
	if rl == nil || !rl.HasData() {
		writeJSONError(w, http.StatusNotFound, "not_found_error", "no usage data cached yet — it appears after the first proxied /v1/messages call")
		return
	}

	u := usageResponse{
		Status:    rl.Status,
		Claim:     rl.Claim,
		UpdatedAt: rl.UpdatedAt.Format(time.RFC3339),
		Age:       time.Since(rl.UpdatedAt).Truncate(time.Second).String(),
	}
	if rl.FiveHour != nil {
		u.FiveHour = toUsageWindow(rl.FiveHour)
	}
	if rl.SevenDay != nil {
		u.SevenDay = toUsageWindow(rl.SevenDay)
	}
	writeJSON(w, http.StatusOK, u)
}

func toUsageWindow(w *RateLimitWindow) *usageWindow {
	return &usageWindow{
		UsedPercent: w.UsedPercent(),
		ResetsAt:    w.ResetsAt.Format(time.RFC3339),
		ResetsIn:    w.TimeUntilReset().Truncate(time.Second).String(),
	}
}

// --- Plumbing ---

// decodeJSONBody reads and decodes a JSON request body, bounded to 128 MB —
// generous enough for base64 vision payloads, small enough to keep a broken
// client from exhausting memory.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 128<<20))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if len(body) == 0 {
		return fmt.Errorf("empty body")
	}
	return json.Unmarshal(body, dst)
}

// hopHeaders are connection-scoped headers that must not be forwarded
// (RFC 7230 §6.1).
var hopHeaders = map[string]bool{
	"Connection":          true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailer":             true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

// proxyResponse copies the upstream status, headers, and body to the client.
// The body is copied chunk-by-chunk with a Flush after every read, so SSE
// events reach the client as they arrive instead of sitting in the
// ResponseWriter buffer until the stream ends.
func proxyResponse(w http.ResponseWriter, resp *http.Response) {
	dst := w.Header()
	for k, vv := range resp.Header {
		if hopHeaders[http.CanonicalHeaderKey(k)] {
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				// Client went away mid-stream — nothing sane to send anymore.
				logDebug("serve: client write failed mid-proxy", "error", werr)
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err == io.EOF {
			return
		}
		if err != nil {
			logWarn("serve: upstream body read failed mid-proxy", "error", err)
			return
		}
	}
}

// writeJSON writes v as a JSON response with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeJSONError writes an Anthropic-shaped error body so proxied clients
// can reuse their existing API error parsing.
func writeJSONError(w http.ResponseWriter, status int, errType, msg string) {
	writeJSON(w, status, APIError{
		Type:   "error",
		Detail: APIErrorDetail{Type: errType, Message: msg},
	})
}
