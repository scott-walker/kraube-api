package kraube

import (
	"io"
	"strings"
	"testing"
)

func newTestStream(events string) *StreamReader {
	return newStreamReader(io.NopCloser(strings.NewReader(events)))
}

func TestStreamReader_TextDelta(t *testing.T) {
	sse := `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}

event: message_stop
data: {"type":"message_stop"}

`

	stream := newTestStream(sse)
	defer func() { _ = stream.Close() }()

	var texts []string
	for stream.Next() {
		if evt, ok := stream.Event().(*ContentBlockDeltaEvent); ok {
			if evt.Delta.Type == "text_delta" {
				texts = append(texts, evt.Delta.Text)
			}
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}

	if len(texts) != 2 || texts[0] != "Hello" || texts[1] != " world" {
		t.Errorf("texts = %v, want [Hello, world]", texts)
	}

	msg := stream.Message()
	if msg.Text() != "Hello world" {
		t.Errorf("Message().Text() = %q, want %q", msg.Text(), "Hello world")
	}
	if msg.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want end_turn", msg.StopReason)
	}
	if msg.Usage.OutputTokens != 5 {
		t.Errorf("OutputTokens = %d, want 5", msg.Usage.OutputTokens)
	}
}

func TestStreamReader_ToolUse(t *testing.T) {
	sse := `event: message_start
data: {"type":"message_start","message":{"id":"msg_2","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_123","name":"get_weather"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"Tokyo\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":20}}

event: message_stop
data: {"type":"message_stop"}

`

	stream := newTestStream(sse)
	defer func() { _ = stream.Close() }()

	var toolStartName string
	var toolStopInput string

	for stream.Next() {
		switch evt := stream.Event().(type) {
		case *ContentBlockStartEvent:
			if evt.ContentBlock.Type == "tool_use" {
				toolStartName = evt.ContentBlock.Name
			}
		case *ContentBlockStopEvent:
			if b := stream.CurrentBlock(); b != nil && b.Type == "tool_use" {
				toolStopInput = string(b.Input)
			}
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}

	if toolStartName != "get_weather" {
		t.Errorf("tool name = %q, want get_weather", toolStartName)
	}
	if toolStopInput != `{"city":"Tokyo"}` {
		t.Errorf("tool input = %q, want {\"city\":\"Tokyo\"}", toolStopInput)
	}

	msg := stream.Message()
	if !msg.HasToolUse() {
		t.Error("HasToolUse = false, want true")
	}
	uses := msg.ToolUses()
	if len(uses) != 1 {
		t.Fatalf("ToolUses count = %d, want 1", len(uses))
	}
	if uses[0].Name != "get_weather" {
		t.Errorf("tool name = %q, want get_weather", uses[0].Name)
	}
}

func TestStreamReader_Thinking(t *testing.T) {
	sse := `event: message_start
data: {"type":"message_start","message":{"id":"msg_3","type":"message","role":"assistant","content":[],"model":"claude-opus-4-6","stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me think..."}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Answer"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":30}}

event: message_stop
data: {"type":"message_stop"}

`

	stream := newTestStream(sse)
	defer func() { _ = stream.Close() }()

	var thinkingDelta string
	for stream.Next() {
		if evt, ok := stream.Event().(*ContentBlockDeltaEvent); ok {
			if evt.Delta.Type == "thinking_delta" {
				thinkingDelta += evt.Delta.Thinking
			}
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}

	if thinkingDelta != "Let me think..." {
		t.Errorf("thinking = %q, want 'Let me think...'", thinkingDelta)
	}

	msg := stream.Message()
	blocks := msg.ThinkingBlocks()
	if len(blocks) != 1 {
		t.Fatalf("ThinkingBlocks count = %d, want 1", len(blocks))
	}
	if blocks[0].Thinking != "Let me think..." {
		t.Errorf("thinking = %q, want 'Let me think...'", blocks[0].Thinking)
	}
	if msg.Text() != "Answer" {
		t.Errorf("Text = %q, want Answer", msg.Text())
	}
}

func TestStreamReader_EventType(t *testing.T) {
	sse := `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"m","stop_reason":null,"usage":{"input_tokens":0,"output_tokens":0}}}

event: ping
data: {"type":"ping"}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":0}}

event: message_stop
data: {"type":"message_stop"}

`

	stream := newTestStream(sse)
	defer func() { _ = stream.Close() }()

	var types []string
	for stream.Next() {
		types = append(types, stream.EventType())
	}

	want := []string{"message_start", "ping", "message_delta"}
	if len(types) != len(want) {
		t.Fatalf("event types = %v, want %v", types, want)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Errorf("types[%d] = %q, want %q", i, types[i], want[i])
		}
	}
}

func TestStreamReader_CurrentBlock_NilForNonBlockEvents(t *testing.T) {
	sse := `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"m","stop_reason":null,"usage":{"input_tokens":0,"output_tokens":0}}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":0}}

event: message_stop
data: {"type":"message_stop"}

`

	stream := newTestStream(sse)
	defer func() { _ = stream.Close() }()

	for stream.Next() {
		if stream.CurrentBlock() != nil {
			t.Errorf("CurrentBlock should be nil for %s", stream.EventType())
		}
	}
}

func TestStreamReader_EventNilBeforeAndAfter(t *testing.T) {
	sse := `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"m","stop_reason":null,"usage":{"input_tokens":0,"output_tokens":0}}}

event: message_stop
data: {"type":"message_stop"}

`

	stream := newTestStream(sse)
	defer func() { _ = stream.Close() }()

	// Before first Next().
	if stream.Event() != nil {
		t.Error("Event should be nil before first Next()")
	}

	for stream.Next() {
	}

	// After stream ends via message_stop, the stop event is still accessible.
	if stream.EventType() != "message_stop" {
		t.Errorf("EventType after end = %q, want message_stop", stream.EventType())
	}
}

func TestStreamReader_BackwardCompatible(t *testing.T) {
	sse := `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"m","stop_reason":null,"usage":{"input_tokens":0,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}

event: message_stop
data: {"type":"message_stop"}

`

	stream := newTestStream(sse)
	defer func() { _ = stream.Close() }()

	// Old-style usage: just iterate without checking events.
	for stream.Next() {
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if stream.Message().Text() != "Hi" {
		t.Errorf("Text = %q, want Hi", stream.Message().Text())
	}
}
