package kraube

import (
	"encoding/json"
	"testing"
)

func TestUserMessage(t *testing.T) {
	m := UserMessage("hello")
	if m.Role != RoleUser {
		t.Errorf("Role = %q, want user", m.Role)
	}
	if m.Content.Text != "hello" {
		t.Errorf("Content.Text = %q, want hello", m.Content.Text)
	}
}

func TestAssistantMessage(t *testing.T) {
	m := AssistantMessage("hi")
	if m.Role != RoleAssistant {
		t.Errorf("Role = %q, want assistant", m.Role)
	}
}

func TestUserBlocks(t *testing.T) {
	m := UserBlocks(TextBlock("a"), TextBlock("b"))
	if m.Role != RoleUser {
		t.Errorf("Role = %q, want user", m.Role)
	}
	if len(m.Content.Blocks) != 2 {
		t.Errorf("blocks count = %d, want 2", len(m.Content.Blocks))
	}
}

func TestContentMarshalJSON_Text(t *testing.T) {
	c := TextContent("hello")
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `"hello"` {
		t.Errorf("marshal = %s, want %q", data, `"hello"`)
	}
}

func TestContentMarshalJSON_Blocks(t *testing.T) {
	c := BlocksContent([]ContentBlock{TextBlock("a")})
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == `"a"` {
		t.Error("blocks content should not marshal as string")
	}
	// Should be an array.
	if data[0] != '[' {
		t.Errorf("expected array, got %c", data[0])
	}
}

func TestContentUnmarshalJSON(t *testing.T) {
	// String content.
	var c1 Content
	_ = json.Unmarshal([]byte(`"hello"`), &c1)
	if c1.Text != "hello" {
		t.Errorf("Text = %q, want hello", c1.Text)
	}

	// Blocks content.
	var c2 Content
	_ = json.Unmarshal([]byte(`[{"type":"text","text":"hi"}]`), &c2)
	if len(c2.Blocks) != 1 {
		t.Fatalf("blocks = %d, want 1", len(c2.Blocks))
	}
	if c2.Blocks[0].Text != "hi" {
		t.Errorf("block text = %q, want hi", c2.Blocks[0].Text)
	}
}

func TestSystemPromptMarshalJSON(t *testing.T) {
	// Text.
	sp := SystemText("You are helpful.")
	data, _ := json.Marshal(sp)
	if string(data) != `"You are helpful."` {
		t.Errorf("marshal = %s", data)
	}

	// Blocks.
	sp2 := SystemBlocks(SystemBlock{Type: "text", Text: "block"})
	data2, _ := json.Marshal(sp2)
	if data2[0] != '[' {
		t.Error("blocks should marshal as array")
	}
}

func TestTextBlock(t *testing.T) {
	b := TextBlock("hello")
	if b.Type != "text" || b.Text != "hello" {
		t.Errorf("TextBlock = %+v", b)
	}
}

func TestImageBase64Block(t *testing.T) {
	b := ImageBase64Block("image/png", "abc123")
	if b.Type != "image" || b.Source == nil {
		t.Fatal("unexpected block")
	}
	if b.Source.Type != "base64" || b.Source.MediaType != "image/png" || b.Source.Data != "abc123" {
		t.Errorf("Source = %+v", b.Source)
	}
}

func TestImageURLBlock(t *testing.T) {
	b := ImageURLBlock("https://example.com/img.jpg")
	if b.Source == nil || b.Source.Type != "url" || b.Source.URL != "https://example.com/img.jpg" {
		t.Errorf("Source = %+v", b.Source)
	}
}

func TestToolUseBlock(t *testing.T) {
	b := ToolUseBlock("id1", "search", json.RawMessage(`{"q":"test"}`))
	if b.Type != "tool_use" || b.ID != "id1" || b.Name != "search" {
		t.Errorf("ToolUseBlock = %+v", b)
	}
}

func TestToolResultBlock(t *testing.T) {
	b := ToolResultBlock("id1", TextContent("result"), false)
	if b.Type != "tool_result" || b.ToolUseID != "id1" || b.IsError {
		t.Errorf("ToolResultBlock = %+v", b)
	}
}

func TestThinkingBlock(t *testing.T) {
	b := ThinkingBlock("thought", "sig")
	if b.Type != "thinking" || b.Thinking != "thought" || b.Signature != "sig" {
		t.Errorf("ThinkingBlock = %+v", b)
	}
}

func TestThinkingConfig(t *testing.T) {
	e := ThinkingEnabled(4096)
	if e.Type != "enabled" || e.BudgetTokens != 4096 {
		t.Errorf("ThinkingEnabled = %+v", e)
	}
	d := ThinkingDisabled()
	if d.Type != "disabled" {
		t.Errorf("ThinkingDisabled = %+v", d)
	}
	a := ThinkingAdaptive()
	if a.Type != "adaptive" {
		t.Errorf("ThinkingAdaptive = %+v", a)
	}
}

func TestBuiltInTools(t *testing.T) {
	ws := WebSearchTool()
	if ws.Name != "web_search" {
		t.Errorf("WebSearchTool name = %q", ws.Name)
	}
	ce := CodeExecutionTool()
	if ce.Name != "code_execution" {
		t.Errorf("CodeExecutionTool name = %q", ce.Name)
	}
	te := TextEditorTool()
	if te.Name != "str_replace_editor" {
		t.Errorf("TextEditorTool name = %q", te.Name)
	}
	b := BashTool()
	if b.Name != "bash" {
		t.Errorf("BashTool name = %q", b.Name)
	}
}

func TestToolChoice(t *testing.T) {
	tc := ToolChoiceTool("my_tool")
	if tc.Type != "tool" || tc.Name != "my_tool" {
		t.Errorf("ToolChoiceTool = %+v", tc)
	}
	if ToolChoiceAuto.Type != "auto" {
		t.Error("ToolChoiceAuto")
	}
	if ToolChoiceAny.Type != "any" {
		t.Error("ToolChoiceAny")
	}
	if ToolChoiceNone.Type != "none" {
		t.Error("ToolChoiceNone")
	}
}

func TestPtrHelpers(t *testing.T) {
	f := Float64(3.14)
	if *f != 3.14 {
		t.Errorf("Float64 = %f", *f)
	}
	i := Int(42)
	if *i != 42 {
		t.Errorf("Int = %d", *i)
	}
}
