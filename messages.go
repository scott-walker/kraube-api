package konnektor

import "encoding/json"

// MessageType identifies the type of SDK message.
type MessageType string

const (
	MessageTypeUser             MessageType = "user"
	MessageTypeAssistant        MessageType = "assistant"
	MessageTypeSystem           MessageType = "system"
	MessageTypeResult           MessageType = "result"
	MessageTypeStreamEvent      MessageType = "stream_event"
	MessageTypeControlRequest   MessageType = "control_request"
	MessageTypeControlResponse  MessageType = "control_response"
	MessageTypeControlCancel    MessageType = "control_cancel_request"
	MessageTypeRateLimitEvent   MessageType = "rate_limit_event"
	MessageTypeToolProgress     MessageType = "tool_progress"
	MessageTypeToolUseSummary   MessageType = "tool_use_summary"
	MessageTypeAuthStatus       MessageType = "auth_status"
	MessageTypePromptSuggestion MessageType = "prompt_suggestion"
)

// SystemSubtype for system messages.
type SystemSubtype string

const (
	SystemSubtypeInit                SystemSubtype = "init"
	SystemSubtypeStatus              SystemSubtype = "status"
	SystemSubtypeTaskStarted         SystemSubtype = "task_started"
	SystemSubtypeTaskProgress        SystemSubtype = "task_progress"
	SystemSubtypeTaskNotification    SystemSubtype = "task_notification"
	SystemSubtypeHookStarted         SystemSubtype = "hook_started"
	SystemSubtypeHookProgress        SystemSubtype = "hook_progress"
	SystemSubtypeHookResponse        SystemSubtype = "hook_response"
	SystemSubtypeAPIRetry            SystemSubtype = "api_retry"
	SystemSubtypeCompactBoundary     SystemSubtype = "compact_boundary"
	SystemSubtypeSessionStateChanged SystemSubtype = "session_state_changed"
)

// ResultSubtype for result messages.
type ResultSubtype string

const (
	ResultSubtypeSuccess              ResultSubtype = "success"
	ResultSubtypeErrorDuringExecution ResultSubtype = "error_during_execution"
	ResultSubtypeErrorMaxTurns        ResultSubtype = "error_max_turns"
	ResultSubtypeErrorMaxBudget       ResultSubtype = "error_max_budget_usd"
)

// ControlSubtype for control request/response messages.
type ControlSubtype string

const (
	ControlSubtypeInitialize     ControlSubtype = "initialize"
	ControlSubtypeInterrupt      ControlSubtype = "interrupt"
	ControlSubtypeCanUseTool     ControlSubtype = "can_use_tool"
	ControlSubtypeHookCallback   ControlSubtype = "hook_callback"
	ControlSubtypeMCPMessage     ControlSubtype = "mcp_message"
	ControlSubtypeMCPStatus      ControlSubtype = "mcp_status"
	ControlSubtypeMCPReconnect   ControlSubtype = "mcp_reconnect"
	ControlSubtypeMCPToggle      ControlSubtype = "mcp_toggle"
	ControlSubtypeSetModel       ControlSubtype = "set_model"
	ControlSubtypeSetPermission  ControlSubtype = "set_permission_mode"
	ControlSubtypeRewindFiles    ControlSubtype = "rewind_files"
	ControlSubtypeStopTask       ControlSubtype = "stop_task"
	ControlSubtypeGetContextUsage ControlSubtype = "get_context_usage"
	ControlSubtypeEndSession     ControlSubtype = "end_session"
	ControlSubtypeSuccess        ControlSubtype = "success"
	ControlSubtypeError          ControlSubtype = "error"
)

// PermissionMode controls how tool permissions are handled.
type PermissionMode string

const (
	PermissionModeDefault           PermissionMode = "default"
	PermissionModeAcceptEdits       PermissionMode = "acceptEdits"
	PermissionModeBypassPermissions PermissionMode = "bypassPermissions"
	PermissionModePlan              PermissionMode = "plan"
	PermissionModeDontAsk           PermissionMode = "dontAsk"
	PermissionModeAuto              PermissionMode = "auto"
)

// PermissionBehavior for tool permission responses.
type PermissionBehavior string

const (
	PermissionAllow PermissionBehavior = "allow"
	PermissionDeny  PermissionBehavior = "deny"
)

// --- Stdin Messages (SDK -> CLI) ---

// UserMessage is sent to the CLI to provide user input.
type UserMessage struct {
	Type              MessageType      `json:"type"`
	SessionID         string           `json:"session_id"`
	Message           MessageContent   `json:"message"`
	ParentToolUseID   *string          `json:"parent_tool_use_id"`
}

// MessageContent represents the role+content of a message.
type MessageContent struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []ContentBlock
}

// ControlRequest is a control protocol message sent to the CLI.
type ControlRequest struct {
	Type      MessageType           `json:"type"`
	RequestID string                `json:"request_id"`
	Request   ControlRequestInner   `json:"request"`
}

// ControlRequestInner holds the subtype-specific payload.
type ControlRequestInner struct {
	Subtype ControlSubtype         `json:"subtype"`
	Hooks   map[string]any         `json:"hooks,omitempty"`
	Agents  map[string]any         `json:"agents,omitempty"`
	// Additional fields for various subtypes
	Extra   map[string]any         `json:"-"`
}

// ControlResponse is sent back to the CLI in response to its control_request.
type ControlResponse struct {
	Type     MessageType                `json:"type"`
	Response ControlResponseInner       `json:"response"`
}

// ControlResponseInner holds the response payload.
type ControlResponseInner struct {
	Subtype   ControlSubtype `json:"subtype"`
	RequestID string         `json:"request_id"`
	Response  any            `json:"response,omitempty"`
	Error     string         `json:"error,omitempty"`
}

// ControlCancelRequest cancels a pending control request.
type ControlCancelRequest struct {
	Type      MessageType `json:"type"`
	RequestID string      `json:"request_id"`
}

// --- Stdout Messages (CLI -> SDK) ---

// RawMessage is used for initial JSON parsing to determine message type.
type RawMessage struct {
	Type    MessageType   `json:"type"`
	Subtype string        `json:"subtype,omitempty"`
	Raw     json.RawMessage
}

// AssistantMessage represents an assistant response from the CLI.
type AssistantMessage struct {
	Type            MessageType    `json:"type"`
	Message         AssistantBody  `json:"message"`
	ParentToolUseID *string        `json:"parent_tool_use_id"`
	Error           *string        `json:"error,omitempty"`
	UUID            string         `json:"uuid,omitempty"`
	SessionID       string         `json:"session_id,omitempty"`
}

// AssistantBody is the core message payload from the Anthropic API.
type AssistantBody struct {
	ID          string         `json:"id,omitempty"`
	Role        string         `json:"role,omitempty"`
	Content     []ContentBlock `json:"content"`
	Model       string         `json:"model,omitempty"`
	StopReason  *string        `json:"stop_reason,omitempty"`
	Usage       *Usage         `json:"usage,omitempty"`
}

// ContentBlock represents a single block in the message content array.
type ContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Content   any             `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
}

// Usage holds token usage information.
type Usage struct {
	InputTokens              int  `json:"input_tokens"`
	OutputTokens             int  `json:"output_tokens"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens,omitempty"`
}

// SystemInitMessage is the first message emitted by the CLI after initialization.
type SystemInitMessage struct {
	Type             MessageType    `json:"type"`
	Subtype          SystemSubtype  `json:"subtype"`
	SessionID        string         `json:"session_id"`
	UUID             string         `json:"uuid"`
	ClaudeCodeVersion string        `json:"claude_code_version,omitempty"`
	CWD              string         `json:"cwd,omitempty"`
	Model            string         `json:"model,omitempty"`
	Tools            []string       `json:"tools,omitempty"`
	PermissionMode   PermissionMode `json:"permissionMode,omitempty"`
	MCPServers       []MCPServerStatus `json:"mcp_servers,omitempty"`
}

// MCPServerStatus represents an MCP server's status.
type MCPServerStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// SystemMessage is a generic system event.
type SystemMessage struct {
	Type      MessageType   `json:"type"`
	Subtype   SystemSubtype `json:"subtype"`
	UUID      string        `json:"uuid,omitempty"`
	SessionID string        `json:"session_id,omitempty"`
	// Fields vary by subtype — use Raw for full access.
	Raw       json.RawMessage `json:"-"`
}

// ResultMessage is emitted when a conversation turn completes.
type ResultMessage struct {
	Type          MessageType   `json:"type"`
	Subtype       ResultSubtype `json:"subtype"`
	DurationMs    int           `json:"duration_ms"`
	DurationAPIMs int           `json:"duration_api_ms"`
	IsError       bool          `json:"is_error"`
	NumTurns      int           `json:"num_turns"`
	SessionID     string        `json:"session_id"`
	StopReason    *string       `json:"stop_reason,omitempty"`
	TotalCostUSD  float64       `json:"total_cost_usd"`
	Usage         *Usage        `json:"usage,omitempty"`
	Result        *string       `json:"result,omitempty"`
	Errors        []string      `json:"errors,omitempty"`
	UUID          string        `json:"uuid,omitempty"`
}

// StreamEvent wraps a partial streaming event from the Anthropic API.
type StreamEvent struct {
	Type            MessageType     `json:"type"`
	Event           json.RawMessage `json:"event"`
	ParentToolUseID *string         `json:"parent_tool_use_id,omitempty"`
	UUID            string          `json:"uuid,omitempty"`
	SessionID       string          `json:"session_id,omitempty"`
}

// ToolPermissionRequest is a control_request from the CLI asking for tool permission.
type ToolPermissionRequest struct {
	Subtype     ControlSubtype `json:"subtype"`
	ToolName    string         `json:"tool_name"`
	Input       json.RawMessage `json:"input"`
	ToolUseID   string         `json:"tool_use_id,omitempty"`
	AgentID     string         `json:"agent_id,omitempty"`
}

// PermissionResponse is sent back to the CLI to allow or deny a tool.
type PermissionResponse struct {
	Behavior PermissionBehavior `json:"behavior"`
	Message  string             `json:"message,omitempty"`
}

// RateLimitEvent represents rate limit status changes.
type RateLimitEvent struct {
	Type          MessageType   `json:"type"`
	RateLimitInfo RateLimitInfo `json:"rate_limit_info"`
	UUID          string        `json:"uuid,omitempty"`
	SessionID     string        `json:"session_id,omitempty"`
}

// RateLimitInfo holds rate limit details.
type RateLimitInfo struct {
	Status        string   `json:"status"`
	ResetsAt      *int64   `json:"resetsAt,omitempty"`
	RateLimitType string   `json:"rateLimitType,omitempty"`
	Utilization   *float64 `json:"utilization,omitempty"`
}

// ToolProgressMessage is a heartbeat for long-running tools.
type ToolProgressMessage struct {
	Type               MessageType `json:"type"`
	ToolUseID          string      `json:"tool_use_id"`
	ToolName           string      `json:"tool_name"`
	ParentToolUseID    *string     `json:"parent_tool_use_id,omitempty"`
	ElapsedTimeSeconds float64     `json:"elapsed_time_seconds"`
	UUID               string      `json:"uuid,omitempty"`
	SessionID          string      `json:"session_id,omitempty"`
}

// Message is a parsed SDK message. Check Type to determine which field is populated.
type Message struct {
	Type MessageType

	// Populated based on Type:
	Assistant       *AssistantMessage
	User            *UserMessage
	System          *SystemMessage
	SystemInit      *SystemInitMessage
	Result          *ResultMessage
	StreamEvent     *StreamEvent
	ControlRequest  *ControlRequest
	ControlResponse *ControlResponse
	RateLimitEvent  *RateLimitEvent
	ToolProgress    *ToolProgressMessage

	// Raw JSON for types not explicitly handled.
	Raw json.RawMessage
}

// Text returns the concatenated text content from an AssistantMessage.
func (m *Message) Text() string {
	if m.Assistant == nil {
		return ""
	}
	var text string
	for _, block := range m.Assistant.Message.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}
	return text
}

// parseMessage parses a raw JSON line into a Message.
func parseMessage(data []byte) (*Message, error) {
	var raw struct {
		Type    MessageType `json:"type"`
		Subtype string     `json:"subtype,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	msg := &Message{
		Type: raw.Type,
		Raw:  json.RawMessage(data),
	}

	switch raw.Type {
	case MessageTypeAssistant:
		var v AssistantMessage
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		msg.Assistant = &v

	case MessageTypeUser:
		var v UserMessage
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		msg.User = &v

	case MessageTypeSystem:
		if raw.Subtype == string(SystemSubtypeInit) {
			var v SystemInitMessage
			if err := json.Unmarshal(data, &v); err != nil {
				return nil, err
			}
			msg.SystemInit = &v
		} else {
			var v SystemMessage
			if err := json.Unmarshal(data, &v); err != nil {
				return nil, err
			}
			v.Raw = json.RawMessage(data)
			msg.System = &v
		}

	case MessageTypeResult:
		var v ResultMessage
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		msg.Result = &v

	case MessageTypeStreamEvent:
		var v StreamEvent
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		msg.StreamEvent = &v

	case MessageTypeControlRequest:
		var v ControlRequest
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		msg.ControlRequest = &v

	case MessageTypeControlResponse:
		var v ControlResponse
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		msg.ControlResponse = &v

	case MessageTypeRateLimitEvent:
		var v RateLimitEvent
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		msg.RateLimitEvent = &v

	case MessageTypeToolProgress:
		var v ToolProgressMessage
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		msg.ToolProgress = &v
	}

	return msg, nil
}
