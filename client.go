package kraube

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	DefaultBaseURL    = "https://api.anthropic.com"
	DefaultAPIVersion = "2023-06-01"

	// ccVersion is the Claude Code version we impersonate for the billing header.
	ccVersion = "2.1.92.00a"

	// ccEntrypoint is the entrypoint identifier for the billing header.
	ccEntrypoint = "cli"
)

// claudeCodeIdentity is the identity preamble that Claude Code CLI prepends
// to every /v1/messages system prompt when authenticating via OAuth
// subscription. The server-side gate on api.anthropic.com treats this
// string as a marker that the request is coming from an official client;
// without it, OAuth requests can be rejected with "forbidden: Request not
// allowed" even when billing / beta headers are otherwise valid.
const claudeCodeIdentity = "You are Claude Code, Anthropic's official CLI for Claude."

// billingHeader is the pre-computed billing header value injected into the
// system prompt for OAuth requests. Without this header, Sonnet and Opus
// models return 429. The hash (cch) is the first 5 characters of
// sha256("cc_version=<ver>; cc_entrypoint=<entry>;").
var billingHeader string

func init() {
	seed := fmt.Sprintf("cc_version=%s; cc_entrypoint=%s;", ccVersion, ccEntrypoint)
	h := sha256.Sum256([]byte(seed))
	cch := hex.EncodeToString(h[:])[:5]
	billingHeader = fmt.Sprintf("x-anthropic-billing-header: cc_version=%s; cc_entrypoint=%s; cch=%s;", ccVersion, ccEntrypoint, cch)
}

// Beta header sets per model class. Determined by traffic analysis of Claude Code CLI.
const (
	// betaBasic is used for Haiku and Sonnet models.
	betaBasic = "oauth-2025-04-20,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05"

	// betaOpus is used for Opus models — includes additional capabilities.
	betaOpus = "claude-code-20250219,oauth-2025-04-20,context-1m-2025-08-07,interleaved-thinking-2025-05-14,redact-thinking-2026-02-12,context-management-2025-06-27,prompt-caching-scope-2026-01-05,advanced-tool-use-2025-11-20,effort-2025-11-24"
)

// betaHeadersForModel returns the appropriate Anthropic-Beta header value
// based on the model name. Opus models require an extended set of beta
// features; all other models use the basic set.
func betaHeadersForModel(model string) string {
	if strings.Contains(strings.ToLower(model), "opus") {
		return betaOpus
	}
	return betaBasic
}

// Client is the Anthropic API client.
type Client struct {
	BaseURL    string
	APIVersion string
	HTTPClient *http.Client

	// Auth (OAuth only — subscription-based access via claude.ai).
	Provider    TokenProvider // token source
	accessToken string        // current access token, updated by ensureAuth

	// OAuth profile (fetched on init for metadata.user_id).
	Profile   *Profile
	SessionID string // unique per client instance
	DeviceID  string // stable device identifier

	// Rate limits — updated automatically from response headers after each API call.
	RateLimit     *RateLimitInfo
	RateLimitPath string // path to cache file (for persistence between CLI runs)

	// gateway marks a WithGateway client: requests go to a `kraube serve`
	// daemon that applies the OAuth injection pipeline itself, so this client
	// must not inject the system prefix or metadata (see prepareRequest).
	gateway bool

	Messages *MessagesService
}

// NewClient creates a client with the given options.
//
//	client, err := kraube.NewClient(ctx, kraube.WithToken("..."))
//	client, err := kraube.NewClient(ctx, kraube.WithTokenFile(""))
//	client, err := kraube.NewClient(ctx, kraube.WithTokenProvider(myProvider))
func NewClient(ctx context.Context, opts ...Option) (*Client, error) {
	cfg := &clientConfig{
		rateLimitPath: DefaultRateLimitPath(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.provider == nil && cfg.credentialsPath == "" {
		return nil, fmt.Errorf("no token source specified: use WithToken, WithTokenFile, WithTokenProvider, etc")
	}

	c := &Client{
		BaseURL:       DefaultBaseURL,
		APIVersion:    DefaultAPIVersion,
		RateLimitPath: cfg.rateLimitPath,
	}

	if cfg.baseURL != "" {
		c.BaseURL = cfg.baseURL
	}
	c.gateway = cfg.gateway
	if cfg.httpClient != nil {
		c.HTTPClient = cfg.httpClient
	} else if cfg.gateway {
		// Gateway mode talks plain HTTP to a local daemon: no Chrome TLS
		// fingerprint needed, and HTTPS_PROXY / ALL_PROXY (meant for the
		// Anthropic egress) must not capture this traffic — the daemon is the
		// one that goes through the proxy.
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.Proxy = nil
		c.HTTPClient = &http.Client{Transport: tr}
	} else {
		// Resolve proxy: explicit WithProxy wins; if absent (not just empty),
		// fall back to HTTPS_PROXY / ALL_PROXY. Passing WithProxy("") is an
		// explicit opt-out that forces a direct connection.
		var proxyURL *url.URL
		if cfg.proxySet {
			if cfg.proxy != "" {
				u, err := resolveProxy(cfg.proxy)
				if err != nil {
					return nil, fmt.Errorf("proxy: %w", err)
				}
				proxyURL = u
			}
		} else {
			u, err := resolveProxy("")
			if err != nil {
				return nil, fmt.Errorf("proxy: %w", err)
			}
			proxyURL = u
		}
		if proxyURL != nil {
			logDebug("client: proxy configured", "url", redactProxyURL(proxyURL))
		}
		c.HTTPClient = &http.Client{Transport: newChromeTransport(proxyURL)}
	}

	// Resolve deferred file-based provider. This is done here (not in the
	// option) so $KRAUBE_CREDENTIALS_PATH is read at NewClient time.
	provider := cfg.provider
	if provider == nil && cfg.credentialsPath != "" {
		path := cfg.credentialsPath
		if path == "_default_" {
			path = DefaultCredentialsPath()
		}
		tm, err := newFileTokenManager(path)
		if err != nil {
			return nil, err
		}
		provider = tm
	}

	c.Provider = provider

	// Route token refresh through the per-Client HTTPClient so WithProxy /
	// WithHTTPClient apply to OAuth refresh the same way they apply to
	// /v1/messages. Only our built-in token managers are wired — custom
	// TokenProvider implementations supplied via WithTokenProvider remain
	// the caller's responsibility.
	switch p := provider.(type) {
	case *tokenManager:
		p.setHTTPClient(c.HTTPClient)
	case *envTokenManager:
		p.setHTTPClient(c.HTTPClient)
	}

	// Get initial access token.
	accessToken, err := provider.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("get initial token: %w", err)
	}
	c.accessToken = accessToken

	// Fetch profile for metadata.user_id.
	if !cfg.skipProfile {
		if err := c.fetchProfile(ctx); err != nil {
			logWarn("client: failed to fetch profile", "error", err)
		}
	}

	c.Messages = &MessagesService{client: c}
	logDebug("client: ready", "base_url", c.BaseURL)
	return c, nil
}

// fetchProfile retrieves the OAuth profile and sets up session/device IDs.
// Routes through c.HTTPClient so WithProxy / WithHTTPClient apply to the
// profile request the same way they apply to /v1/messages.
func (c *Client) fetchProfile(ctx context.Context) error {
	profile, err := fetchProfileWithClient(ctx, c.HTTPClient, c.accessToken)
	if err != nil {
		return err
	}
	c.Profile = profile
	c.SessionID = generateUUID()
	c.DeviceID = generateDeviceID()
	return nil
}

// metadataUserID builds the metadata.user_id JSON string required by the API.
// The API expects a JSON-encoded object with device_id, account_uuid, and session_id.
func (c *Client) metadataUserID() string {
	if c.Profile == nil {
		return ""
	}
	m := map[string]string{
		"device_id":    c.DeviceID,
		"account_uuid": c.Profile.AccountUUID,
		"session_id":   c.SessionID,
	}
	data, _ := json.Marshal(m)
	return string(data)
}

func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func generateDeviceID() string {
	b := make([]byte, 32)
	rand.Read(b)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// ensureAuth refreshes the access token via the provider before each request.
func (c *Client) ensureAuth(ctx context.Context) error {
	if c.Provider == nil {
		return nil
	}
	accessToken, err := c.Provider.Token(ctx)
	if err != nil {
		return fmt.Errorf("token provider: %w", err)
	}
	c.accessToken = accessToken
	return nil
}

// EnsureFresh proactively refreshes the access token when it expires within
// margin. The built-in token managers (WithToken, WithTokenFile, WithEnvToken)
// normally refresh lazily, only inside a 60-second window before expiry;
// long-lived processes like `kraube serve` call EnsureFresh from a background
// loop with a larger margin so the token is rotated well ahead of time and
// requests never pay the refresh latency (or hit a dead token).
//
// For custom TokenProvider implementations there is no expiry contract, so
// EnsureFresh degrades to a plain Token() call — the provider applies its own
// refresh policy.
func (c *Client) EnsureFresh(ctx context.Context, margin time.Duration) error {
	if c.Provider == nil {
		return nil
	}
	if rp, ok := c.Provider.(refreshableProvider); ok {
		return rp.ensureFresh(ctx, margin)
	}
	_, err := c.Provider.Token(ctx)
	return err
}

// AccessExpiry reports when the current access token expires. ok is false
// when the provider does not expose expiry (custom TokenProvider) or no
// access token has been obtained yet.
func (c *Client) AccessExpiry() (expiresAt time.Time, ok bool) {
	if rp, isBuiltin := c.Provider.(refreshableProvider); isBuiltin {
		return rp.expiry()
	}
	return time.Time{}, false
}

// injectMetadata adds metadata.user_id for OAuth clients if not already set.
func (c *Client) injectMetadata(req *MessageRequest) {
	if c.Profile == nil {
		return
	}
	uid := c.metadataUserID()
	if uid == "" {
		return
	}
	if req.Metadata == nil {
		req.Metadata = &Metadata{}
	}
	if req.Metadata.UserID == "" {
		req.Metadata.UserID = uid
	}
}

// injectSystemPrefix prepends the two system-level markers that the API
// server expects from an official Claude Code client on OAuth-authenticated
// requests:
//
//  1. The Claude Code identity preamble ("You are Claude Code, ..."). The
//     server gate checks for this exact opening line when deciding whether
//     an OAuth request is allowed; missing it yields HTTP 403 "forbidden:
//     Request not allowed".
//  2. The billing header ("x-anthropic-billing-header: cc_version=..."),
//     without which Sonnet and Opus responses come back as 429.
//
// Both blocks land *before* any user-supplied system blocks, in the order
// identity → billing → user. Callers that already provide their own system
// prompt (string or blocks) keep it verbatim; the prefix is prepended.
func (c *Client) injectSystemPrefix(req *MessageRequest) {
	// Idempotent: a request that already starts with the identity preamble
	// has been through this injection once — e.g. a full (non-gateway) kraube
	// client pointed at `kraube serve` injects client-side, and the daemon's
	// Raw path must not prepend a second copy.
	if hasSystemPrefix(req) {
		return
	}

	identityBlock := SystemBlock{Type: "text", Text: claudeCodeIdentity}
	billingBlock := SystemBlock{Type: "text", Text: billingHeader}

	if req.System == nil {
		req.System = &SystemPrompt{
			Blocks: []SystemBlock{identityBlock, billingBlock},
		}
		return
	}

	if req.System.Blocks != nil {
		req.System.Blocks = append(
			[]SystemBlock{identityBlock, billingBlock},
			req.System.Blocks...,
		)
		return
	}

	// Plain-text system prompt — convert to blocks, keeping the original
	// text as the third (user-level) block.
	req.System = &SystemPrompt{
		Blocks: []SystemBlock{
			identityBlock,
			billingBlock,
			{Type: "text", Text: req.System.Text},
		},
	}
}

// hasSystemPrefix reports whether the request's system prompt already starts
// with the Claude Code identity preamble, i.e. injectSystemPrefix already ran
// on it (possibly in another process — see the daemon note above).
func hasSystemPrefix(req *MessageRequest) bool {
	if req.System == nil {
		return false
	}
	if req.System.Blocks != nil {
		return len(req.System.Blocks) > 0 && req.System.Blocks[0].Text == claudeCodeIdentity
	}
	return req.System.Text == claudeCodeIdentity
}

// prepareRequest injects metadata and the identity + billing system prefix.
// Called by Create, Stream, and Raw before sending. Gateway clients skip
// injection entirely — the daemon they talk to applies the full pipeline
// server-side, and injecting on both sides would duplicate the prefix.
func (c *Client) prepareRequest(req *MessageRequest) {
	if c.gateway {
		return
	}
	c.injectMetadata(req)
	c.injectSystemPrefix(req)
}

func fmtWindowPct(w *RateLimitWindow) string {
	if w == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.0f%%", w.UsedPercent())
}

// --- MessagesService ---

// MessagesService handles the /v1/messages endpoints.
type MessagesService struct {
	client *Client
}

// Create sends a non-streaming message request.
func (s *MessagesService) Create(ctx context.Context, req *MessageRequest) (*MessageResponse, error) {
	req.Stream = false
	s.client.prepareRequest(req)
	logDebug("messages: create", "model", req.Model, "max_tokens", req.MaxTokens, "messages", len(req.Messages), "tools", len(req.Tools))

	// Set model-specific beta headers before marshaling the request.
	resp, err := s.client.doWithModel(ctx, http.MethodPost, "/v1/messages", req.Model, req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, parseAPIError(resp, s.client)
	}

	var msg MessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&msg); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	logInfo("messages: created", "id", msg.ID, "model", msg.Model, "stop_reason", msg.StopReason,
		"input_tokens", msg.Usage.InputTokens, "output_tokens", msg.Usage.OutputTokens)
	return &msg, nil
}

// Stream sends a streaming message request and returns a StreamReader.
// The caller must call StreamReader.Close() when done.
func (s *MessagesService) Stream(ctx context.Context, req *MessageRequest) (*StreamReader, error) {
	req.Stream = true
	s.client.prepareRequest(req)
	logDebug("messages: stream", "model", req.Model, "max_tokens", req.MaxTokens, "messages", len(req.Messages), "tools", len(req.Tools))

	resp, err := s.client.doWithModel(ctx, http.MethodPost, "/v1/messages", req.Model, req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		return nil, parseAPIError(resp, s.client)
	}

	logDebug("stream: connected")
	return newStreamReader(resp.Body), nil
}

// Raw sends a message request and returns the raw upstream *http.Response
// without decoding it — status, headers, and body bytes are the server's,
// untouched. All OAuth injection required by the subscription gate (identity
// preamble, billing header, metadata.user_id, model-specific beta headers)
// is applied exactly as in Create/Stream; without it the API answers 403/429.
//
// req.Stream is preserved as-is, so a caller proxying a client-supplied body
// (like `kraube serve`) passes both streaming SSE and plain JSON responses
// through byte-for-byte without re-parsing events. The caller owns resp.Body
// and must Close it.
func (s *MessagesService) Raw(ctx context.Context, req *MessageRequest) (*http.Response, error) {
	s.client.prepareRequest(req)
	logDebug("messages: raw", "model", req.Model, "stream", req.Stream, "messages", len(req.Messages))
	return s.client.doWithModel(ctx, http.MethodPost, "/v1/messages", req.Model, req)
}

// CountTokensRaw sends a count_tokens request and returns the raw upstream
// *http.Response. Mirrors CountTokens (which performs no system-prefix or
// metadata injection — the endpoint does not require them) but leaves
// decoding to the caller, who owns resp.Body.
func (s *MessagesService) CountTokensRaw(ctx context.Context, req *CountTokensRequest) (*http.Response, error) {
	logDebug("messages: count_tokens raw", "model", req.Model, "messages", len(req.Messages))
	return s.client.doWithModel(ctx, http.MethodPost, "/v1/messages/count_tokens", req.Model, req)
}

// CountTokens counts tokens for a message request without sending it.
func (s *MessagesService) CountTokens(ctx context.Context, req *CountTokensRequest) (*CountTokensResponse, error) {
	logDebug("messages: count_tokens", "model", req.Model, "messages", len(req.Messages))

	resp, err := s.client.doWithModel(ctx, http.MethodPost, "/v1/messages/count_tokens", req.Model, req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, parseAPIError(resp, s.client)
	}

	var result CountTokensResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	logDebug("messages: token count", "input_tokens", result.InputTokens)
	return &result, nil
}

// doWithModel wraps do() and injects the model-specific Anthropic-Beta header
// into the HTTP request. It re-marshals the body to inject the header at the
// HTTP transport level.
func (c *Client) doWithModel(ctx context.Context, method, path string, model string, body any) (*http.Response, error) {
	if err := c.ensureAuth(ctx); err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}

	var bodyReader io.Reader
	var bodyBytes []byte
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		bodyBytes = data
		bodyReader = bytes.NewReader(data)
	}

	url := c.BaseURL + path + "?beta=true"
	logDebug("api: request", "method", method, "path", path, "body_bytes", len(bodyBytes))

	// Attach a connInfo to the request context so chromeTransport can record
	// which local/remote addresses were used for the TCP connection. This lets
	// us surface the egress IP in error logs (useful when routing via proxies).
	ci := &connInfo{}
	ctx = context.WithValue(ctx, connInfoKey{}, ci)

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Anthropic-Version", c.APIVersion)
	// An empty access token happens only in gateway mode against an
	// unauthenticated loopback daemon — send no Authorization header rather
	// than a dangling "Bearer ".
	if c.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}
	req.Header.Set("anthropic-dangerous-direct-browser-access", "true")
	req.Header.Set("User-Agent", "claude-cli/2.1.92 (external, cli)")
	req.Header.Set("x-app", "cli")
	req.Header.Set("Anthropic-Beta", betaHeadersForModel(model))

	start := time.Now()
	resp, err := c.HTTPClient.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		logError("api: request failed",
			"method", method,
			"url", url,
			"local_addr", ci.LocalAddr,
			"remote_addr", ci.RemoteAddr,
			"elapsed", elapsed,
			"error", err,
			"request_headers", dumpHeaders(req.Header),
			"request_body", truncateBody(bodyBytes, 8192),
		)
		return nil, err
	}

	logDebug("api: response",
		"method", method,
		"path", path,
		"status", resp.StatusCode,
		"elapsed", elapsed,
		"local_addr", ci.LocalAddr,
		"remote_addr", ci.RemoteAddr,
	)
	if elapsed > 5*time.Second {
		logWarn("api: slow request", "method", method, "path", path, "elapsed", elapsed, "status", resp.StatusCode)
	}

	// On error responses, drain the body into a buffer, replace resp.Body with
	// an in-memory reader (so downstream parseAPIError can still decode it),
	// and emit a full diagnostic log with request + response details.
	if resp.StatusCode >= 400 {
		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			logWarn("api: error response body read failed", "error", readErr)
		}
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		logError("api: error response",
			"method", method,
			"url", url,
			"status", resp.StatusCode,
			"elapsed", elapsed,
			"local_addr", ci.LocalAddr,
			"remote_addr", ci.RemoteAddr,
			"request_headers", dumpHeaders(req.Header),
			"request_body", truncateBody(bodyBytes, 8192),
			"response_headers", dumpHeaders(resp.Header),
			"response_body", truncateBody(respBody, 8192),
		)
	}

	// Update rate limit info from response headers.
	if rl := parseRateLimitHeaders(resp.Header); rl != nil {
		c.RateLimit = rl
		logDebug("api: rate limits updated",
			"status", rl.Status,
			"5h_used", fmtWindowPct(rl.FiveHour),
			"7d_used", fmtWindowPct(rl.SevenDay),
		)
		if c.RateLimitPath != "" {
			if err := SaveRateLimit(c.RateLimitPath, rl); err != nil {
				logWarn("api: failed to save rate limits", "error", err)
			}
		}
	}

	return resp, nil
}

// --- StreamReader ---

// StreamReader reads SSE events from a streaming response.
// After each Next() call, Event() returns the current typed event and
// CurrentBlock() returns the content block being built (if applicable).
type StreamReader struct {
	body    io.ReadCloser
	scanner *bufio.Scanner
	err     error

	// Accumulated message built from stream events.
	message    *MessageResponse
	eventCount int

	// Current event from the last Next() call.
	currentEvent StreamEvent
}

func newStreamReader(body io.ReadCloser) *StreamReader {
	return &StreamReader{
		body:    body,
		scanner: bufio.NewScanner(body),
		message: &MessageResponse{},
	}
}

// Next advances to the next SSE event. Returns false when done or on error.
// After Next returns true, call Event() to access the current event.
func (r *StreamReader) Next() bool {
	r.currentEvent = nil

	for r.scanner.Scan() {
		line := r.scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			eventType := strings.TrimPrefix(line, "event: ")

			// Read the data line.
			if !r.scanner.Scan() {
				return false
			}
			dataLine := r.scanner.Text()
			if !strings.HasPrefix(dataLine, "data: ") {
				continue
			}
			data := strings.TrimPrefix(dataLine, "data: ")

			r.eventCount++
			evt, err := r.processEvent(eventType, []byte(data))
			if err != nil {
				r.err = err
				logError("stream: processing error", "event_type", eventType, "event_num", r.eventCount, "error", r.err)
				return false
			}
			r.currentEvent = evt

			// message_stop means we're done.
			if eventType == "message_stop" {
				logInfo("stream: completed", "events", r.eventCount, "blocks", len(r.message.Content),
					"stop_reason", r.message.StopReason, "output_tokens", r.message.Usage.OutputTokens)
				return false
			}
			return true
		}
	}
	r.err = r.scanner.Err()
	return false
}

// processEvent handles a single SSE event and updates the accumulated message.
func (r *StreamReader) processEvent(eventType string, data []byte) (StreamEvent, error) {
	switch eventType {
	case "message_start":
		var evt MessageStartEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return nil, err
		}
		r.message = &evt.Message
		return &evt, nil

	case "content_block_start":
		var evt ContentBlockStartEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return nil, err
		}
		for len(r.message.Content) <= evt.Index {
			r.message.Content = append(r.message.Content, ContentBlock{})
		}
		r.message.Content[evt.Index] = evt.ContentBlock
		return &evt, nil

	case "content_block_delta":
		var evt ContentBlockDeltaEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return nil, err
		}
		if evt.Index < len(r.message.Content) {
			block := &r.message.Content[evt.Index]
			switch evt.Delta.Type {
			case "text_delta":
				block.Text += evt.Delta.Text
			case "input_json_delta":
				block.Input = append(block.Input, []byte(evt.Delta.PartialJSON)...)
			case "thinking_delta":
				block.Thinking += evt.Delta.Thinking
			case "signature_delta":
				block.Signature += evt.Delta.Signature
			}
		}
		return &evt, nil

	case "content_block_stop":
		var evt ContentBlockStopEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return nil, err
		}
		return &evt, nil

	case "message_delta":
		var evt MessageDeltaEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return nil, err
		}
		r.message.StopReason = evt.Delta.StopReason
		r.message.StopSequence = evt.Delta.StopSequence
		r.message.Usage.OutputTokens = evt.Usage.OutputTokens
		return &evt, nil

	case "message_stop":
		var evt MessageStopEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return nil, err
		}
		return &evt, nil

	case "ping":
		var evt PingEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return nil, err
		}
		return &evt, nil

	case "error":
		var evt ErrorEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return nil, err
		}
		logError("stream: server error", "error_type", evt.Error.Type, "message", evt.Error.Message)
		return nil, &APIError{
			Type:   "error",
			Detail: evt.Error,
		}
	}
	return nil, nil
}

// Message returns the accumulated message after streaming completes.
func (r *StreamReader) Message() *MessageResponse {
	return r.message
}

// Err returns any error encountered during streaming.
func (r *StreamReader) Err() error {
	return r.err
}

// Close releases the underlying response body.
func (r *StreamReader) Close() error {
	return r.body.Close()
}

// Event returns the current SSE event from the last Next() call.
// Use a type switch to handle specific event types:
//
//	for stream.Next() {
//	    switch evt := stream.Event().(type) {
//	    case *kraube.ContentBlockDeltaEvent:
//	        fmt.Print(evt.Delta.Text)
//	    case *kraube.ContentBlockStartEvent:
//	        fmt.Println("Block:", evt.ContentBlock.Type)
//	    }
//	}
//
// Returns nil before the first Next() call or after the stream ends.
func (r *StreamReader) Event() StreamEvent {
	return r.currentEvent
}

// EventType returns the type string of the current event
// (e.g. "content_block_delta", "message_start").
// Returns "" if no current event.
func (r *StreamReader) EventType() string {
	if r.currentEvent == nil {
		return ""
	}
	return r.currentEvent.EventType()
}

// CurrentBlock returns the content block associated with the current event,
// reflecting accumulated state so far (e.g. tool_use with partial input).
// Returns nil for events without a block (message_start, message_delta, ping).
func (r *StreamReader) CurrentBlock() *ContentBlock {
	var idx int
	switch evt := r.currentEvent.(type) {
	case *ContentBlockStartEvent:
		idx = evt.Index
	case *ContentBlockDeltaEvent:
		idx = evt.Index
	case *ContentBlockStopEvent:
		idx = evt.Index
	default:
		return nil
	}
	if idx < len(r.message.Content) {
		return &r.message.Content[idx]
	}
	return nil
}

// --- Error parsing ---

// parseAPIError reads the error body and also updates rate limit info on the client.
func parseAPIError(resp *http.Response, c *Client) error {
	// Even error responses (especially 429) carry rate limit headers.
	if c != nil {
		if rl := parseRateLimitHeaders(resp.Header); rl != nil {
			c.RateLimit = rl
			logDebug("api: rate limits from error response",
				"status", rl.Status,
				"5h_used", fmtWindowPct(rl.FiveHour),
				"7d_used", fmtWindowPct(rl.SevenDay),
			)
			if c.RateLimitPath != "" {
				if err := SaveRateLimit(c.RateLimitPath, rl); err != nil {
					logWarn("api: failed to save rate limits", "error", err)
				}
			}
		}
	}

	body, _ := io.ReadAll(resp.Body)
	var apiErr APIError
	if err := json.Unmarshal(body, &apiErr); err != nil {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	apiErr.Status = resp.StatusCode
	return &apiErr
}

// dumpHeaders serializes HTTP headers into a compact single-line form for
// logging. The Authorization header is redacted so bearer tokens never leak
// into stderr or log files.
func dumpHeaders(h http.Header) string {
	if len(h) == 0 {
		return ""
	}
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	// Stable order makes logs diffable across runs.
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(k)
		b.WriteString(": ")
		switch strings.ToLower(k) {
		case "authorization", "proxy-authorization", "cookie", "set-cookie":
			b.WriteString("***redacted***")
		default:
			b.WriteString(strings.Join(h.Values(k), ","))
		}
	}
	return b.String()
}

// truncateBody returns a string form of body, trimmed to at most max bytes.
// Used to keep error logs bounded even when the server returns a huge body.
func truncateBody(body []byte, max int) string {
	if len(body) == 0 {
		return ""
	}
	if len(body) <= max {
		return string(body)
	}
	return string(body[:max]) + fmt.Sprintf("... (+%d bytes truncated)", len(body)-max)
}
