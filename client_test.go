package kraube

import (
	"strings"
	"testing"
)

func TestBetaHeadersForModel(t *testing.T) {
	tests := []struct {
		model string
		want  string
	}{
		{"claude-opus-4-6", betaOpus},
		{"claude-OPUS-4-6", betaOpus},
		{"claude-sonnet-4-6", betaBasic},
		{"claude-haiku-4-5-20251001", betaBasic},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := betaHeadersForModel(tt.model); got != tt.want {
				t.Errorf("betaHeadersForModel(%q) got different result", tt.model)
			}
		})
	}
}

func TestBillingHeader(t *testing.T) {
	if billingHeader == "" {
		t.Fatal("billingHeader is empty")
	}
	if !strings.Contains(billingHeader, "cc_version=") {
		t.Error("missing cc_version")
	}
	if !strings.Contains(billingHeader, "cch=") {
		t.Error("missing cch")
	}
	// cch should be 5 hex chars.
	parts := strings.Split(billingHeader, "cch=")
	if len(parts) < 2 {
		t.Fatal("cannot find cch=")
	}
	cch := strings.TrimSuffix(parts[1], ";")
	if len(cch) != 5 {
		t.Errorf("cch length = %d, want 5", len(cch))
	}
}

func TestInjectBillingHeader_NoSystem(t *testing.T) {
	c := &Client{}
	req := &MessageRequest{}
	c.injectBillingHeader(req)

	if req.System == nil || len(req.System.Blocks) != 1 {
		t.Fatal("expected 1 system block")
	}
	if req.System.Blocks[0].Text != billingHeader {
		t.Error("billing header not injected")
	}
}

func TestInjectBillingHeader_WithTextSystem(t *testing.T) {
	c := &Client{}
	req := &MessageRequest{
		System: SystemText("Be helpful."),
	}
	c.injectBillingHeader(req)

	if len(req.System.Blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(req.System.Blocks))
	}
	if req.System.Blocks[0].Text != billingHeader {
		t.Error("billing header should be first")
	}
	if req.System.Blocks[1].Text != "Be helpful." {
		t.Error("original text should be preserved")
	}
}

func TestInjectBillingHeader_WithBlocksSystem(t *testing.T) {
	c := &Client{}
	req := &MessageRequest{
		System: SystemBlocks(SystemBlock{Type: "text", Text: "existing"}),
	}
	c.injectBillingHeader(req)

	if len(req.System.Blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(req.System.Blocks))
	}
	if req.System.Blocks[0].Text != billingHeader {
		t.Error("billing header should be prepended")
	}
}

func TestInjectMetadata(t *testing.T) {
	c := &Client{
		Profile:   &Profile{AccountUUID: "acc-123"},
		SessionID: "sess-456",
		DeviceID:  "dev-789",
	}
	req := &MessageRequest{}
	c.injectMetadata(req)

	if req.Metadata == nil || req.Metadata.UserID == "" {
		t.Fatal("metadata not injected")
	}
	if !strings.Contains(req.Metadata.UserID, "acc-123") {
		t.Error("metadata should contain account UUID")
	}
}

func TestInjectMetadata_NoProfile(t *testing.T) {
	c := &Client{}
	req := &MessageRequest{}
	c.injectMetadata(req)

	if req.Metadata != nil {
		t.Error("metadata should not be injected without profile")
	}
}

func TestInjectMetadata_DoesNotOverwrite(t *testing.T) {
	c := &Client{
		Profile:   &Profile{AccountUUID: "acc-123"},
		SessionID: "sess-456",
		DeviceID:  "dev-789",
	}
	req := &MessageRequest{
		Metadata: &Metadata{UserID: "custom-id"},
	}
	c.injectMetadata(req)

	if req.Metadata.UserID != "custom-id" {
		t.Error("should not overwrite existing UserID")
	}
}

func TestGenerateUUID(t *testing.T) {
	uuid := generateUUID()
	// UUID v4 format: 8-4-4-4-12
	parts := strings.Split(uuid, "-")
	if len(parts) != 5 {
		t.Errorf("UUID format invalid: %q", uuid)
	}
	if len(uuid) != 36 {
		t.Errorf("UUID length = %d, want 36", len(uuid))
	}

	// Should be unique.
	uuid2 := generateUUID()
	if uuid == uuid2 {
		t.Error("two UUIDs should differ")
	}
}

func TestGenerateDeviceID(t *testing.T) {
	id := generateDeviceID()
	if len(id) != 64 { // sha256 hex
		t.Errorf("DeviceID length = %d, want 64", len(id))
	}
}
