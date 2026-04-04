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
	Credentials *Credentials  // current token snapshot from Provider
	Provider    TokenProvider // OAuth token source

	// OAuth profile (fetched on init for metadata.user_id).
	Profile   *Profile
	SessionID string // unique per client instance
	DeviceID  string // stable device identifier

	// Rate limits — updated automatically from response headers after each API call.
	RateLimit     *RateLimitInfo
	RateLimitPath string // path to cache file (for persistence between CLI runs)

	Messages *MessagesService
}

// NewClient creates a client with the given options.
//
//	client, err := kraube.NewClient(ctx, kraube.WithAccessToken("..."))
//	client, err := kraube.NewClient(ctx, kraube.WithCredentialsFile(""))
//	client, err := kraube.NewClient(ctx, kraube.WithTokenProvider(myProvider))
func NewClient(ctx context.Context, opts ...Option) (*Client, error) {
	cfg := &clientConfig{
		rateLimitPath: DefaultRateLimitPath(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.provider == nil {
		return nil, fmt.Errorf("no token source specified: use WithAccessToken, WithCredentialsFile, WithTokenProvider, etc")
	}

	c := &Client{
		BaseURL:       DefaultBaseURL,
		APIVersion:    DefaultAPIVersion,
		RateLimitPath: cfg.rateLimitPath,
	}

	if cfg.baseURL != "" {
		c.BaseURL = cfg.baseURL
	}
	if cfg.httpClient != nil {
		c.HTTPClient = cfg.httpClient
	} else {
		c.HTTPClient = &http.Client{Transport: newChromeTransport()}
	}

	// Resolve deferred providers.
	provider := cfg.provider
	if p, ok := provider.(*deferredFileProvider); ok {
		fp, err := NewFileTokenProvider(p.path)
		if err != nil {
			return nil, fmt.Errorf("load credentials: %w (run Login first)", err)
		}
		provider = fp
	}

	c.Provider = provider

	// Get initial credentials snapshot.
	creds, err := provider.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("get initial token: %w", err)
	}
	c.Credentials = creds

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
func (c *Client) fetchProfile(ctx context.Context) error {
	profile, err := FetchProfile(ctx, c.Credentials.AccessToken)
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

// ensureAuth refreshes OAuth token via the provider before each request.
func (c *Client) ensureAuth(ctx context.Context) error {
	if c.Provider == nil {
		return nil
	}
	creds, err := c.Provider.Token(ctx)
	if err != nil {
		return fmt.Errorf("token provider: %w", err)
	}
	c.Credentials = creds
	return nil
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

// injectBillingHeader prepends the billing header block to the system prompt.
// This is required for OAuth subscription access — without it, Sonnet and Opus
// models return 429 rate limit errors.
func (c *Client) injectBillingHeader(req *MessageRequest) {
	billingBlock := SystemBlock{
		Type: "text",
		Text: billingHeader,
	}

	if req.System == nil {
		// No system prompt set — create one with just the billing header.
		req.System = &SystemPrompt{
			Blocks: []SystemBlock{billingBlock},
		}
		return
	}

	// System prompt exists — convert to blocks format if needed and prepend.
	if req.System.Blocks != nil {
		// Already blocks format — prepend billing block.
		req.System.Blocks = append([]SystemBlock{billingBlock}, req.System.Blocks...)
	} else {
		// Plain text format — convert to blocks with billing + original text.
		req.System = &SystemPrompt{
			Blocks: []SystemBlock{
				billingBlock,
				{Type: "text", Text: req.System.Text},
			},
		}
	}
}

// prepareRequest injects metadata, billing header, and sets beta headers
// on the request. Called by Create and Stream before sending.
func (c *Client) prepareRequest(req *MessageRequest) {
	c.injectMetadata(req)
	c.injectBillingHeader(req)
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
	defer resp.Body.Close()

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
		defer resp.Body.Close()
		return nil, parseAPIError(resp, s.client)
	}

	logDebug("stream: connected")
	return newStreamReader(resp.Body), nil
}

// CountTokens counts tokens for a message request without sending it.
func (s *MessagesService) CountTokens(ctx context.Context, req *CountTokensRequest) (*CountTokensResponse, error) {
	logDebug("messages: count_tokens", "model", req.Model, "messages", len(req.Messages))

	resp, err := s.client.doWithModel(ctx, http.MethodPost, "/v1/messages/count_tokens", req.Model, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

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
	var bodySize int
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		bodySize = len(data)
		bodyReader = bytes.NewReader(data)
	}

	url := c.BaseURL + path + "?beta=true"
	logDebug("api: request", "method", method, "path", path, "body_bytes", bodySize)

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Anthropic-Version", c.APIVersion)
	req.Header.Set("Authorization", "Bearer "+c.Credentials.AccessToken)
	req.Header.Set("anthropic-dangerous-direct-browser-access", "true")
	req.Header.Set("User-Agent", "claude-cli/2.1.92 (external, cli)")
	req.Header.Set("x-app", "cli")
	req.Header.Set("Anthropic-Beta", betaHeadersForModel(model))

	start := time.Now()
	resp, err := c.HTTPClient.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		logError("api: request failed", "method", method, "path", path, "elapsed", elapsed, "error", err)
		return nil, err
	}

	logDebug("api: response", "method", method, "path", path, "status", resp.StatusCode, "elapsed", elapsed)
	if elapsed > 5*time.Second {
		logWarn("api: slow request", "method", method, "path", path, "elapsed", elapsed, "status", resp.StatusCode)
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
type StreamReader struct {
	body    io.ReadCloser
	scanner *bufio.Scanner
	err     error

	// Accumulated message built from stream events.
	message    *MessageResponse
	eventCount int
}

func newStreamReader(body io.ReadCloser) *StreamReader {
	return &StreamReader{
		body:    body,
		scanner: bufio.NewScanner(body),
		message: &MessageResponse{},
	}
}

// Next advances to the next SSE event. Returns false when done or on error.
func (r *StreamReader) Next() bool {
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
			r.err = r.processEvent(eventType, []byte(data))
			if r.err != nil {
				logError("stream: processing error", "event_type", eventType, "event_num", r.eventCount, "error", r.err)
				return false
			}

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
func (r *StreamReader) processEvent(eventType string, data []byte) error {
	switch eventType {
	case "message_start":
		var evt MessageStartEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return err
		}
		r.message = &evt.Message

	case "content_block_start":
		var evt ContentBlockStartEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return err
		}
		for len(r.message.Content) <= evt.Index {
			r.message.Content = append(r.message.Content, ContentBlock{})
		}
		r.message.Content[evt.Index] = evt.ContentBlock

	case "content_block_delta":
		var evt ContentBlockDeltaEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return err
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

	case "message_delta":
		var evt MessageDeltaEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return err
		}
		r.message.StopReason = evt.Delta.StopReason
		r.message.StopSequence = evt.Delta.StopSequence
		r.message.Usage.OutputTokens = evt.Usage.OutputTokens

	case "error":
		var evt ErrorEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			return err
		}
		logError("stream: server error", "error_type", evt.Error.Type, "message", evt.Error.Message)
		return &APIError{
			Type:   "error",
			Detail: evt.Error,
		}

	case "ping", "content_block_stop", "message_stop":
		// no-op
	}
	return nil
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
