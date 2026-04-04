package kraube

import (
	"encoding/json"
	"testing"
)

func TestMessageResponse_Text(t *testing.T) {
	r := &MessageResponse{
		Content: []ContentBlock{
			{Type: "thinking", Thinking: "hmm"},
			{Type: "text", Text: "Hello "},
			{Type: "text", Text: "world"},
		},
	}
	if got := r.Text(); got != "Hello world" {
		t.Errorf("Text = %q, want 'Hello world'", got)
	}
}

func TestMessageResponse_Text_Empty(t *testing.T) {
	r := &MessageResponse{}
	if got := r.Text(); got != "" {
		t.Errorf("Text = %q, want empty", got)
	}
}

func TestMessageResponse_ToolUses(t *testing.T) {
	r := &MessageResponse{
		Content: []ContentBlock{
			{Type: "text", Text: "I'll search"},
			{Type: "tool_use", ID: "t1", Name: "search", Input: json.RawMessage(`{"q":"go"}`)},
			{Type: "tool_use", ID: "t2", Name: "read", Input: json.RawMessage(`{"f":"a.go"}`)},
		},
	}
	uses := r.ToolUses()
	if len(uses) != 2 {
		t.Fatalf("ToolUses count = %d, want 2", len(uses))
	}
	if uses[0].Name != "search" || uses[1].Name != "read" {
		t.Errorf("names = %q, %q", uses[0].Name, uses[1].Name)
	}
}

func TestMessageResponse_ThinkingBlocks(t *testing.T) {
	r := &MessageResponse{
		Content: []ContentBlock{
			{Type: "thinking", Thinking: "deep thought"},
			{Type: "text", Text: "answer"},
		},
	}
	blocks := r.ThinkingBlocks()
	if len(blocks) != 1 || blocks[0].Thinking != "deep thought" {
		t.Errorf("ThinkingBlocks = %v", blocks)
	}
}

func TestMessageResponse_HasToolUse(t *testing.T) {
	r1 := &MessageResponse{StopReason: StopReasonToolUse}
	if !r1.HasToolUse() {
		t.Error("HasToolUse should be true")
	}
	r2 := &MessageResponse{StopReason: StopReasonEndTurn}
	if r2.HasToolUse() {
		t.Error("HasToolUse should be false")
	}
}

func TestAPIError_Error(t *testing.T) {
	e := &APIError{Detail: APIErrorDetail{Type: "rate_limit_error", Message: "slow down"}}
	if got := e.Error(); got != "rate_limit_error: slow down" {
		t.Errorf("Error = %q", got)
	}
}

func TestAPIError_TypeChecks(t *testing.T) {
	tests := []struct {
		errType string
		check   func(*APIError) bool
	}{
		{"invalid_request_error", (*APIError).IsInvalidRequest},
		{"authentication_error", (*APIError).IsAuthentication},
		{"permission_error", (*APIError).IsPermission},
		{"not_found_error", (*APIError).IsNotFound},
		{"rate_limit_error", (*APIError).IsRateLimit},
		{"overloaded_error", (*APIError).IsOverloaded},
	}
	for _, tt := range tests {
		t.Run(tt.errType, func(t *testing.T) {
			e := &APIError{Detail: APIErrorDetail{Type: tt.errType}}
			if !tt.check(e) {
				t.Errorf("Is* returned false for %s", tt.errType)
			}
		})
	}
}

func TestEventType_Methods(t *testing.T) {
	tests := []struct {
		event StreamEvent
		want  string
	}{
		{&MessageStartEvent{}, "message_start"},
		{&ContentBlockStartEvent{}, "content_block_start"},
		{&ContentBlockDeltaEvent{}, "content_block_delta"},
		{&ContentBlockStopEvent{}, "content_block_stop"},
		{&MessageDeltaEvent{}, "message_delta"},
		{&MessageStopEvent{}, "message_stop"},
		{&PingEvent{}, "ping"},
		{&ErrorEvent{}, "error"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.event.EventType(); got != tt.want {
				t.Errorf("EventType = %q, want %q", got, tt.want)
			}
		})
	}
}
