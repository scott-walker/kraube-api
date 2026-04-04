package kraube

// MessageRequest is the request body for POST /v1/messages.
type MessageRequest struct {
	Model         Model          `json:"model"`
	MaxTokens     int            `json:"max_tokens"`
	Messages      []Message      `json:"messages"`
	System        *SystemPrompt  `json:"system,omitempty"`
	Temperature   *float64       `json:"temperature,omitempty"`
	TopP          *float64       `json:"top_p,omitempty"`
	TopK          *int           `json:"top_k,omitempty"`
	StopSequences []string       `json:"stop_sequences,omitempty"`
	Tools         []Tool         `json:"tools,omitempty"`
	ToolChoice    *ToolChoice    `json:"tool_choice,omitempty"`
	Stream        bool           `json:"stream,omitempty"`
	Thinking      *ThinkingConfig `json:"thinking,omitempty"`
	OutputConfig  *OutputConfig  `json:"output_config,omitempty"`
	Metadata      *Metadata      `json:"metadata,omitempty"`
	ServiceTier   string         `json:"service_tier,omitempty"`
	CacheControl  *CacheControl  `json:"cache_control,omitempty"`
}

// Ptr helpers for optional fields.
func Float64(v float64) *float64 { return &v }
func Int(v int) *int             { return &v }

// CountTokensRequest is the request body for POST /v1/messages/count_tokens.
type CountTokensRequest struct {
	Model         Model          `json:"model"`
	Messages      []Message      `json:"messages"`
	System        *SystemPrompt  `json:"system,omitempty"`
	Tools         []Tool         `json:"tools,omitempty"`
	Thinking      *ThinkingConfig `json:"thinking,omitempty"`
}

// --- Responses ---

// MessageResponse is the response from POST /v1/messages (non-streaming).
type MessageResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"` // "message"
	Role         Role           `json:"role"`
	Content      []ContentBlock `json:"content"`
	Model        string         `json:"model"`
	StopReason   StopReason     `json:"stop_reason"`
	StopSequence *string        `json:"stop_sequence,omitempty"`
	Usage        Usage          `json:"usage"`
}

// Text returns concatenated text from all text content blocks.
func (r *MessageResponse) Text() string {
	var s string
	for _, b := range r.Content {
		if b.Type == "text" {
			s += b.Text
		}
	}
	return s
}

// ToolUses returns all tool_use blocks from the response.
func (r *MessageResponse) ToolUses() []ContentBlock {
	var uses []ContentBlock
	for _, b := range r.Content {
		if b.Type == "tool_use" {
			uses = append(uses, b)
		}
	}
	return uses
}

// ThinkingBlocks returns all thinking blocks from the response.
func (r *MessageResponse) ThinkingBlocks() []ContentBlock {
	var blocks []ContentBlock
	for _, b := range r.Content {
		if b.Type == "thinking" {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

// HasToolUse returns true if the model wants to call a tool.
func (r *MessageResponse) HasToolUse() bool {
	return r.StopReason == StopReasonToolUse
}

// CountTokensResponse is the response from POST /v1/messages/count_tokens.
type CountTokensResponse struct {
	InputTokens int `json:"input_tokens"`
}

// --- Error ---

// APIError represents an error response from the Anthropic API.
type APIError struct {
	Type   string         `json:"type"` // "error"
	Detail APIErrorDetail `json:"error"`
	Status int            `json:"-"`
}

type APIErrorDetail struct {
	Type    string `json:"type"` // "invalid_request_error", "authentication_error", etc.
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	return e.Detail.Type + ": " + e.Detail.Message
}

func (e *APIError) IsInvalidRequest() bool { return e.Detail.Type == "invalid_request_error" }
func (e *APIError) IsAuthentication() bool { return e.Detail.Type == "authentication_error" }
func (e *APIError) IsPermission() bool     { return e.Detail.Type == "permission_error" }
func (e *APIError) IsNotFound() bool       { return e.Detail.Type == "not_found_error" }
func (e *APIError) IsRateLimit() bool      { return e.Detail.Type == "rate_limit_error" }
func (e *APIError) IsOverloaded() bool     { return e.Detail.Type == "overloaded_error" }

// --- Streaming Events ---

// StreamEvent is the interface satisfied by all typed SSE events.
// Use a type switch to handle specific event types in the streaming loop.
type StreamEvent interface {
	EventType() string
}

// Typed event payloads:

type MessageStartEvent struct {
	Type    string          `json:"type"` // "message_start"
	Message MessageResponse `json:"message"`
}

func (*MessageStartEvent) EventType() string { return "message_start" }

type ContentBlockStartEvent struct {
	Type         string       `json:"type"` // "content_block_start"
	Index        int          `json:"index"`
	ContentBlock ContentBlock `json:"content_block"`
}

func (*ContentBlockStartEvent) EventType() string { return "content_block_start" }

type ContentBlockDeltaEvent struct {
	Type  string `json:"type"` // "content_block_delta"
	Index int    `json:"index"`
	Delta Delta  `json:"delta"`
}

func (*ContentBlockDeltaEvent) EventType() string { return "content_block_delta" }

type ContentBlockStopEvent struct {
	Type  string `json:"type"` // "content_block_stop"
	Index int    `json:"index"`
}

func (*ContentBlockStopEvent) EventType() string { return "content_block_stop" }

type MessageDeltaEvent struct {
	Type  string     `json:"type"` // "message_delta"
	Delta DeltaFinal `json:"delta"`
	Usage Usage      `json:"usage"`
}

func (*MessageDeltaEvent) EventType() string { return "message_delta" }

type MessageStopEvent struct {
	Type string `json:"type"` // "message_stop"
}

func (*MessageStopEvent) EventType() string { return "message_stop" }

// Delta is the incremental content in a content_block_delta.
type Delta struct {
	Type      string `json:"type"` // "text_delta", "input_json_delta", "thinking_delta", "signature_delta"
	Text      string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
}

// DeltaFinal is the delta in a message_delta event.
type DeltaFinal struct {
	StopReason   StopReason `json:"stop_reason"`
	StopSequence *string    `json:"stop_sequence,omitempty"`
}

// PingEvent is a keepalive ping.
type PingEvent struct {
	Type string `json:"type"` // "ping"
}

func (*PingEvent) EventType() string { return "ping" }

// ErrorEvent is an error during streaming.
type ErrorEvent struct {
	Type  string         `json:"type"` // "error"
	Error APIErrorDetail `json:"error"`
}

func (*ErrorEvent) EventType() string { return "error" }
