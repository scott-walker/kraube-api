package kraube

import "encoding/json"

// --- Roles ---

type Role = string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// --- Stop Reasons ---

type StopReason = string

const (
	StopReasonEndTurn      StopReason = "end_turn"
	StopReasonMaxTokens    StopReason = "max_tokens"
	StopReasonStopSequence StopReason = "stop_sequence"
	StopReasonToolUse      StopReason = "tool_use"
)

// --- Messages ---

// Message is a conversation turn (user or assistant).
type Message struct {
	Role    Role    `json:"role"`
	Content Content `json:"content"`
}

// UserMessage creates a user message from a string.
func UserMessage(text string) Message {
	return Message{Role: RoleUser, Content: TextContent(text)}
}

// AssistantMessage creates an assistant message from a string.
func AssistantMessage(text string) Message {
	return Message{Role: RoleAssistant, Content: TextContent(text)}
}

// UserBlocks creates a user message from content blocks.
func UserBlocks(blocks ...ContentBlock) Message {
	return Message{Role: RoleUser, Content: BlocksContent(blocks)}
}

// AssistantBlocks creates an assistant message from content blocks.
func AssistantBlocks(blocks ...ContentBlock) Message {
	return Message{Role: RoleAssistant, Content: BlocksContent(blocks)}
}

// --- Content (string | []ContentBlock) ---

// Content represents message content: either a plain string or an array of ContentBlock.
type Content struct {
	Text   string
	Blocks []ContentBlock
}

// TextContent creates a Content from a plain string.
func TextContent(s string) Content { return Content{Text: s} }

// BlocksContent creates a Content from content blocks.
func BlocksContent(blocks []ContentBlock) Content { return Content{Blocks: blocks} }

func (c Content) MarshalJSON() ([]byte, error) {
	if c.Blocks != nil {
		return json.Marshal(c.Blocks)
	}
	return json.Marshal(c.Text)
}

func (c *Content) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		return json.Unmarshal(data, &c.Text)
	}
	return json.Unmarshal(data, &c.Blocks)
}

// --- Content Blocks ---

// ContentBlock is a polymorphic content element.
// Check Type to determine which fields are populated.
type ContentBlock struct {
	Type string `json:"type"`

	// text
	Text      string          `json:"text,omitempty"`
	Citations []Citation      `json:"citations,omitempty"`

	// image
	Source *ImageSource `json:"source,omitempty"`

	// document
	Title   string          `json:"title,omitempty"`
	Context string          `json:"context,omitempty"`

	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result
	ToolUseID string   `json:"tool_use_id,omitempty"`
	IsError   bool     `json:"is_error,omitempty"`
	Content   *Content `json:"content,omitempty"`

	// thinking
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`

	// redacted_thinking
	Data string `json:"data,omitempty"`

	// search_result (server tool)

	// cache_control (can be on any block)
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

// --- Content Block Constructors ---

func TextBlock(text string) ContentBlock {
	return ContentBlock{Type: "text", Text: text}
}

func ImageBase64Block(mediaType, data string) ContentBlock {
	return ContentBlock{
		Type: "image",
		Source: &ImageSource{
			Type:      "base64",
			MediaType: mediaType,
			Data:      data,
		},
	}
}

func ImageURLBlock(url string) ContentBlock {
	return ContentBlock{
		Type: "image",
		Source: &ImageSource{
			Type: "url",
			URL:  url,
		},
	}
}

func ToolUseBlock(id, name string, input json.RawMessage) ContentBlock {
	return ContentBlock{Type: "tool_use", ID: id, Name: name, Input: input}
}

func ToolResultBlock(toolUseID string, content Content, isError bool) ContentBlock {
	return ContentBlock{
		Type:      "tool_result",
		ToolUseID: toolUseID,
		Content:   &content,
		IsError:   isError,
	}
}

func ThinkingBlock(thinking, signature string) ContentBlock {
	return ContentBlock{Type: "thinking", Thinking: thinking, Signature: signature}
}

// --- Image Source ---

type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

// --- Cache Control ---

type CacheControl struct {
	Type string `json:"type"` // "ephemeral"
	TTL  string `json:"ttl,omitempty"` // "5m" or "1h"
}

// --- Citations ---

type Citation struct {
	Type             string `json:"type"`
	CitedText        string `json:"cited_text,omitempty"`
	DocumentIndex    *int   `json:"document_index,omitempty"`
	DocumentTitle    string `json:"document_title,omitempty"`
	StartCharIndex   *int   `json:"start_char_index,omitempty"`
	EndCharIndex     *int   `json:"end_char_index,omitempty"`
	StartPageNumber  *int   `json:"start_page_number,omitempty"`
	EndPageNumber    *int   `json:"end_page_number,omitempty"`
	StartBlockIndex  *int   `json:"start_block_index,omitempty"`
	EndBlockIndex    *int   `json:"end_block_index,omitempty"`
	EncryptedIndex   string `json:"encrypted_index,omitempty"`
	Title            string `json:"title,omitempty"`
	URL              string `json:"url,omitempty"`
	Source           string `json:"source,omitempty"`
	SearchResultIndex *int  `json:"search_result_index,omitempty"`
}

// --- Usage ---

type Usage struct {
	InputTokens              int  `json:"input_tokens"`
	OutputTokens             int  `json:"output_tokens"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens,omitempty"`
}

// --- Tools ---

// Tool defines a tool the model can use.
type Tool struct {
	Type        string      `json:"type,omitempty"` // "custom" or built-in type
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	InputSchema *Schema     `json:"input_schema,omitempty"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

// Schema is a JSON Schema object for tool inputs.
type Schema struct {
	Type       string                 `json:"type"`
	Properties map[string]*Schema     `json:"properties,omitempty"`
	Required   []string               `json:"required,omitempty"`
	Items      *Schema                `json:"items,omitempty"`
	Enum       []string               `json:"enum,omitempty"`
	Desc       string                 `json:"description,omitempty"`

	// Additional JSON Schema fields
	MinItems    *int    `json:"minItems,omitempty"`
	MaxItems    *int    `json:"maxItems,omitempty"`
	Minimum     *float64 `json:"minimum,omitempty"`
	Maximum     *float64 `json:"maximum,omitempty"`
	Pattern     string  `json:"pattern,omitempty"`
	Default     any     `json:"default,omitempty"`
	AnyOf       []*Schema `json:"anyOf,omitempty"`
	OneOf       []*Schema `json:"oneOf,omitempty"`
}

// Built-in tool constructors.

func WebSearchTool(opts ...func(*Tool)) Tool {
	t := Tool{Type: "web_search_20260209", Name: "web_search"}
	for _, o := range opts {
		o(&t)
	}
	return t
}

func CodeExecutionTool() Tool {
	return Tool{Type: "code_execution_20260120", Name: "code_execution"}
}

func TextEditorTool() Tool {
	return Tool{Type: "text_editor_20250728", Name: "str_replace_editor"}
}

func BashTool() Tool {
	return Tool{Type: "bash_20250124", Name: "bash"}
}

// --- Tool Choice ---

type ToolChoice struct {
	Type                   string `json:"type"` // "auto", "any", "tool", "none"
	Name                   string `json:"name,omitempty"`
	DisableParallelToolUse bool   `json:"disable_parallel_tool_use,omitempty"`
}

var (
	ToolChoiceAuto = &ToolChoice{Type: "auto"}
	ToolChoiceAny  = &ToolChoice{Type: "any"}
	ToolChoiceNone = &ToolChoice{Type: "none"}
)

func ToolChoiceTool(name string) *ToolChoice {
	return &ToolChoice{Type: "tool", Name: name}
}

// --- Thinking ---

type ThinkingConfig struct {
	Type         string `json:"type"` // "enabled", "disabled", "adaptive"
	BudgetTokens int    `json:"budget_tokens,omitempty"`
	Display      string `json:"display,omitempty"` // "summarized", "omitted"
}

func ThinkingEnabled(budgetTokens int) *ThinkingConfig {
	return &ThinkingConfig{Type: "enabled", BudgetTokens: budgetTokens}
}

func ThinkingDisabled() *ThinkingConfig {
	return &ThinkingConfig{Type: "disabled"}
}

func ThinkingAdaptive() *ThinkingConfig {
	return &ThinkingConfig{Type: "adaptive"}
}

// --- Output Config ---

type OutputConfig struct {
	Format *OutputFormat `json:"format,omitempty"`
	Effort string       `json:"effort,omitempty"` // "low", "medium", "high", "max"
}

type OutputFormat struct {
	Type   string          `json:"type"` // "json_schema"
	Schema json.RawMessage `json:"schema"`
}

// --- Metadata ---

type Metadata struct {
	UserID string `json:"user_id,omitempty"`
}

// --- System Prompt ---

// SystemPrompt can be a plain string or an array of text blocks (for caching).
type SystemPrompt struct {
	Text   string
	Blocks []SystemBlock
}

type SystemBlock struct {
	Type         string        `json:"type"` // "text"
	Text         string        `json:"text"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

func SystemText(s string) *SystemPrompt { return &SystemPrompt{Text: s} }

func SystemBlocks(blocks ...SystemBlock) *SystemPrompt { return &SystemPrompt{Blocks: blocks} }

func (s SystemPrompt) MarshalJSON() ([]byte, error) {
	if s.Blocks != nil {
		return json.Marshal(s.Blocks)
	}
	return json.Marshal(s.Text)
}

func (s *SystemPrompt) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		return json.Unmarshal(data, &s.Text)
	}
	return json.Unmarshal(data, &s.Blocks)
}
