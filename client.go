package kraube

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const (
	DefaultBaseURL   = "https://api.anthropic.com"
	DefaultAPIVersion = "2023-06-01"
)

// AuthMode determines how the client authenticates.
type AuthMode int

const (
	// AuthOAuth uses OAuth Bearer token (subscription-based access).
	AuthOAuth AuthMode = iota
	// AuthAPIKey uses X-API-Key header (API key access).
	AuthAPIKey
)

// Client is the Anthropic API client.
type Client struct {
	BaseURL    string
	APIVersion string
	HTTPClient *http.Client
	Betas      []string

	// Auth
	AuthMode    AuthMode
	APIKey      string       // for AuthAPIKey
	Credentials *Credentials // for AuthOAuth
	CredsPath   string       // path to credentials file (for auto-save after refresh)

	Messages *MessagesService
}

// NewClientOAuth creates a client using OAuth credentials.
// It loads credentials from the given path (or default), refreshes if expired, and saves back.
func NewClientOAuth(ctx context.Context, credsPath string) (*Client, error) {
	if credsPath == "" {
		credsPath = DefaultCredentialsPath()
	}

	creds, err := LoadCredentials(credsPath)
	if err != nil {
		return nil, fmt.Errorf("load credentials: %w (run Login first)", err)
	}

	c := &Client{
		BaseURL:     DefaultBaseURL,
		APIVersion:  DefaultAPIVersion,
		HTTPClient:  http.DefaultClient,
		AuthMode:    AuthOAuth,
		Credentials: creds,
		CredsPath:   credsPath,
	}
	c.Messages = &MessagesService{client: c}

	// Auto-refresh if expired
	if creds.IsExpired() {
		if err := c.refreshCredentials(ctx); err != nil {
			return nil, fmt.Errorf("refresh token: %w", err)
		}
	}

	return c, nil
}

// NewClientFromClaude creates a client reusing Claude Code's OAuth credentials (~/.claude/.credentials.json).
func NewClientFromClaude(ctx context.Context) (*Client, error) {
	creds, err := LoadClaudeCredentials()
	if err != nil {
		return nil, err
	}

	c := &Client{
		BaseURL:     DefaultBaseURL,
		APIVersion:  DefaultAPIVersion,
		HTTPClient:  http.DefaultClient,
		AuthMode:    AuthOAuth,
		Credentials: creds,
	}
	c.Messages = &MessagesService{client: c}

	if creds.IsExpired() {
		if err := c.refreshCredentials(ctx); err != nil {
			return nil, fmt.Errorf("refresh token: %w", err)
		}
	}

	return c, nil
}

// NewClientAPIKey creates a client using an API key.
// If apiKey is empty, reads from ANTHROPIC_API_KEY env var.
func NewClientAPIKey(apiKey string) *Client {
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	c := &Client{
		BaseURL:    DefaultBaseURL,
		APIVersion: DefaultAPIVersion,
		HTTPClient: http.DefaultClient,
		AuthMode:   AuthAPIKey,
		APIKey:     apiKey,
	}
	c.Messages = &MessagesService{client: c}
	return c
}

// refreshCredentials refreshes the OAuth token and saves to disk if CredsPath is set.
func (c *Client) refreshCredentials(ctx context.Context) error {
	newCreds, err := RefreshAccessToken(ctx, c.Credentials)
	if err != nil {
		return err
	}
	c.Credentials = newCreds

	if c.CredsPath != "" {
		_ = SaveCredentials(c.CredsPath, newCreds)
	}
	return nil
}

// ensureAuth refreshes OAuth token if needed before each request.
func (c *Client) ensureAuth(ctx context.Context) error {
	if c.AuthMode != AuthOAuth || c.Credentials == nil {
		return nil
	}
	if c.Credentials.IsExpired() {
		return c.refreshCredentials(ctx)
	}
	return nil
}

// do executes an HTTP request and returns the response.
func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	if err := c.ensureAuth(ctx); err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := c.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Anthropic-Version", c.APIVersion)

	switch c.AuthMode {
	case AuthOAuth:
		req.Header.Set("Authorization", "Bearer "+c.Credentials.AccessToken)
		req.Header.Set("Anthropic-Beta", oauthBeta)
	case AuthAPIKey:
		req.Header.Set("X-API-Key", c.APIKey)
	}

	if len(c.Betas) > 0 {
		existing := req.Header.Get("Anthropic-Beta")
		extra := strings.Join(c.Betas, ",")
		if existing != "" {
			req.Header.Set("Anthropic-Beta", existing+","+extra)
		} else {
			req.Header.Set("Anthropic-Beta", extra)
		}
	}

	return c.HTTPClient.Do(req)
}

// --- MessagesService ---

// MessagesService handles the /v1/messages endpoints.
type MessagesService struct {
	client *Client
}

// Create sends a non-streaming message request.
func (s *MessagesService) Create(ctx context.Context, req *MessageRequest) (*MessageResponse, error) {
	req.Stream = false

	resp, err := s.client.do(ctx, http.MethodPost, "/v1/messages", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseError(resp)
	}

	var msg MessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&msg); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &msg, nil
}

// Stream sends a streaming message request and returns a StreamReader.
// The caller must call StreamReader.Close() when done.
func (s *MessagesService) Stream(ctx context.Context, req *MessageRequest) (*StreamReader, error) {
	req.Stream = true

	resp, err := s.client.do(ctx, http.MethodPost, "/v1/messages", req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, parseError(resp)
	}

	return newStreamReader(resp.Body), nil
}

// CountTokens counts tokens for a message request without sending it.
func (s *MessagesService) CountTokens(ctx context.Context, req *CountTokensRequest) (*CountTokensResponse, error) {
	resp, err := s.client.do(ctx, http.MethodPost, "/v1/messages/count_tokens", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseError(resp)
	}

	var result CountTokensResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// --- StreamReader ---

// StreamReader reads SSE events from a streaming response.
type StreamReader struct {
	body    io.ReadCloser
	scanner *bufio.Scanner
	err     error

	// Accumulated message built from stream events.
	message *MessageResponse
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

			// Read the data line
			if !r.scanner.Scan() {
				return false
			}
			dataLine := r.scanner.Text()
			if !strings.HasPrefix(dataLine, "data: ") {
				continue
			}
			data := strings.TrimPrefix(dataLine, "data: ")

			r.err = r.processEvent(eventType, []byte(data))
			if r.err != nil {
				return false
			}

			// message_stop means we're done
			if eventType == "message_stop" {
				return false
			}
			return true
		}
	}
	r.err = r.scanner.Err()
	return false
}

// Event returns the current event type and raw data after Next() returns true.
// For structured access, use the Message() method after streaming completes.
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
		// Extend content slice if needed
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

func parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	var apiErr APIError
	if err := json.Unmarshal(body, &apiErr); err != nil {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	apiErr.Status = resp.StatusCode
	return &apiErr
}
