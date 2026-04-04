package kraube

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func TestParseRateLimitHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("anthropic-ratelimit-unified-5h-utilization", "0.75")
	h.Set("anthropic-ratelimit-unified-5h-reset", "1712345678")
	h.Set("anthropic-ratelimit-unified-7d-utilization", "0.30")
	h.Set("anthropic-ratelimit-unified-7d-reset", "1712999999")
	h.Set("anthropic-ratelimit-unified-status", "allowed")
	h.Set("anthropic-ratelimit-unified-representative-claim", "five_hour")
	h.Set("anthropic-ratelimit-unified-fallback", "available")

	rl := parseRateLimitHeaders(h)
	if rl == nil {
		t.Fatal("expected non-nil RateLimitInfo")
	}
	if rl.FiveHour == nil {
		t.Fatal("FiveHour is nil")
	}
	if rl.FiveHour.Utilization != 0.75 {
		t.Errorf("5h utilization = %f, want 0.75", rl.FiveHour.Utilization)
	}
	if rl.SevenDay == nil {
		t.Fatal("SevenDay is nil")
	}
	if rl.SevenDay.Utilization != 0.30 {
		t.Errorf("7d utilization = %f, want 0.30", rl.SevenDay.Utilization)
	}
	if rl.Status != "allowed" {
		t.Errorf("Status = %q", rl.Status)
	}
	if rl.Claim != "five_hour" {
		t.Errorf("Claim = %q", rl.Claim)
	}
	if !rl.Fallback {
		t.Error("Fallback should be true")
	}
}

func TestParseRateLimitHeaders_Empty(t *testing.T) {
	rl := parseRateLimitHeaders(http.Header{})
	if rl != nil {
		t.Error("expected nil for empty headers")
	}
}

func TestRateLimitWindow_UsedPercent(t *testing.T) {
	w := &RateLimitWindow{Utilization: 0.42}
	if got := w.UsedPercent(); got != 42.0 {
		t.Errorf("UsedPercent = %f, want 42", got)
	}
}

func TestRateLimitWindow_TimeUntilReset(t *testing.T) {
	w := &RateLimitWindow{ResetsAt: time.Now().Add(5 * time.Minute)}
	d := w.TimeUntilReset()
	if d < 4*time.Minute || d > 6*time.Minute {
		t.Errorf("TimeUntilReset = %v, want ~5m", d)
	}

	// Past reset.
	w2 := &RateLimitWindow{ResetsAt: time.Now().Add(-time.Minute)}
	if w2.TimeUntilReset() != 0 {
		t.Errorf("TimeUntilReset for past = %v, want 0", w2.TimeUntilReset())
	}
}

func TestRateLimitInfo_HasData(t *testing.T) {
	rl := &RateLimitInfo{}
	if rl.HasData() {
		t.Error("HasData should be false for zero UpdatedAt")
	}
	rl.UpdatedAt = time.Now()
	if !rl.HasData() {
		t.Error("HasData should be true")
	}
}

func TestSaveAndLoadRateLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ratelimit.json")

	now := time.Now().Truncate(time.Second)
	rl := &RateLimitInfo{
		FiveHour: &RateLimitWindow{Utilization: 0.5, ResetsAt: now.Add(time.Hour)},
		SevenDay: &RateLimitWindow{Utilization: 0.2, ResetsAt: now.Add(24 * time.Hour)},
		Status:   "allowed",
		Claim:    "five_hour",
		UpdatedAt: now,
	}

	if err := SaveRateLimit(path, rl); err != nil {
		t.Fatalf("SaveRateLimit: %v", err)
	}

	loaded, err := LoadRateLimit(path)
	if err != nil {
		t.Fatalf("LoadRateLimit: %v", err)
	}
	if loaded.Status != "allowed" {
		t.Errorf("Status = %q", loaded.Status)
	}
	if loaded.FiveHour == nil || loaded.FiveHour.Utilization != 0.5 {
		t.Errorf("FiveHour = %+v", loaded.FiveHour)
	}
	if loaded.SevenDay == nil || loaded.SevenDay.Utilization != 0.2 {
		t.Errorf("SevenDay = %+v", loaded.SevenDay)
	}
}
