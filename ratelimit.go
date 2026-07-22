package kraube

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// RateLimitWindow holds utilization data for a single rate limit window.
type RateLimitWindow struct {
	Utilization float64   // 0.0–1.0 (fraction of limit used)
	ResetsAt    time.Time // when this window resets
}

// UsedPercent returns utilization as a percentage (0–100).
func (w *RateLimitWindow) UsedPercent() float64 {
	return w.Utilization * 100
}

// TimeUntilReset returns duration until the window resets.
func (w *RateLimitWindow) TimeUntilReset() time.Duration {
	d := time.Until(w.ResetsAt)
	if d < 0 {
		return 0
	}
	return d
}

// RateLimitInfo holds the latest rate limit state from API response headers.
// Updated automatically after each API call.
type RateLimitInfo struct {
	// Per-window utilization (nil if not present in response).
	FiveHour *RateLimitWindow // 5-hour session window
	SevenDay *RateLimitWindow // 7-day weekly window

	// Unified status from headers.
	Status   string // "allowed", "rejected", "allowed_warning"
	ResetsAt time.Time
	Claim    string // "five_hour", "seven_day", "seven_day_opus", "seven_day_sonnet", "overage"
	Fallback bool   // fallback mode available

	// Extra usage / overage.
	OverageStatus         string // overage status
	OverageResetsAt       time.Time
	OverageDisabledReason string

	// Timestamp when this info was last updated.
	UpdatedAt time.Time
}

// HasData returns true if any rate limit data has been received.
func (r *RateLimitInfo) HasData() bool {
	return !r.UpdatedAt.IsZero()
}

// parseRateLimitHeaders extracts rate limit info from API response headers.
func parseRateLimitHeaders(h http.Header) *RateLimitInfo {
	info := &RateLimitInfo{}
	hasAny := false

	// 5-hour window
	if w := parseWindow(h, "5h"); w != nil {
		info.FiveHour = w
		hasAny = true
	}

	// 7-day window
	if w := parseWindow(h, "7d"); w != nil {
		info.SevenDay = w
		hasAny = true
	}

	// Unified status
	if v := h.Get("anthropic-ratelimit-unified-status"); v != "" {
		info.Status = v
		hasAny = true
	}
	if v := h.Get("anthropic-ratelimit-unified-reset"); v != "" {
		if ts, err := strconv.ParseFloat(v, 64); err == nil {
			info.ResetsAt = time.Unix(int64(ts), 0)
		}
	}
	if v := h.Get("anthropic-ratelimit-unified-representative-claim"); v != "" {
		info.Claim = v
	}
	if h.Get("anthropic-ratelimit-unified-fallback") == "available" {
		info.Fallback = true
	}

	// Overage
	if v := h.Get("anthropic-ratelimit-unified-overage-status"); v != "" {
		info.OverageStatus = v
		hasAny = true
	}
	if v := h.Get("anthropic-ratelimit-unified-overage-reset"); v != "" {
		if ts, err := strconv.ParseFloat(v, 64); err == nil {
			info.OverageResetsAt = time.Unix(int64(ts), 0)
		}
	}
	if v := h.Get("anthropic-ratelimit-unified-overage-disabled-reason"); v != "" {
		info.OverageDisabledReason = v
	}

	if !hasAny {
		return nil
	}
	info.UpdatedAt = time.Now()
	return info
}

func parseWindow(h http.Header, abbrev string) *RateLimitWindow {
	uStr := h.Get("anthropic-ratelimit-unified-" + abbrev + "-utilization")
	rStr := h.Get("anthropic-ratelimit-unified-" + abbrev + "-reset")
	if uStr == "" || rStr == "" {
		return nil
	}

	u, err := strconv.ParseFloat(uStr, 64)
	if err != nil {
		return nil
	}
	ts, err := strconv.ParseFloat(rStr, 64)
	if err != nil {
		return nil
	}

	return &RateLimitWindow{
		Utilization: u,
		ResetsAt:    time.Unix(int64(ts), 0),
	}
}

// --- Persistence ---

// rateLimitJSON is the on-disk format for rate limit cache.
type rateLimitJSON struct {
	FiveHourUtil  *float64 `json:"five_hour_utilization,omitempty"`
	FiveHourReset *int64   `json:"five_hour_reset,omitempty"`
	SevenDayUtil  *float64 `json:"seven_day_utilization,omitempty"`
	SevenDayReset *int64   `json:"seven_day_reset,omitempty"`
	Status        string   `json:"status,omitempty"`
	ResetsAt      int64    `json:"resets_at,omitempty"`
	Claim         string   `json:"claim,omitempty"`
	UpdatedAt     int64    `json:"updated_at"`
}

// DefaultRateLimitPath returns ~/.config/kraube/ratelimit.json.
func DefaultRateLimitPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "kraube", "ratelimit.json")
}

// SaveRateLimit writes rate limit info to disk.
func SaveRateLimit(path string, rl *RateLimitInfo) error {
	if rl == nil {
		return nil
	}
	j := rateLimitJSON{
		Status:    rl.Status,
		Claim:     rl.Claim,
		UpdatedAt: rl.UpdatedAt.Unix(),
	}
	if rl.FiveHour != nil {
		j.FiveHourUtil = &rl.FiveHour.Utilization
		ts := rl.FiveHour.ResetsAt.Unix()
		j.FiveHourReset = &ts
	}
	if rl.SevenDay != nil {
		j.SevenDayUtil = &rl.SevenDay.Utilization
		ts := rl.SevenDay.ResetsAt.Unix()
		j.SevenDayReset = &ts
	}
	if !rl.ResetsAt.IsZero() {
		j.ResetsAt = rl.ResetsAt.Unix()
	}
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// LoadRateLimit reads cached rate limit info from disk.
func LoadRateLimit(path string) (*RateLimitInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var j rateLimitJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, err
	}
	rl := &RateLimitInfo{
		Status:    j.Status,
		Claim:     j.Claim,
		UpdatedAt: time.Unix(j.UpdatedAt, 0),
	}
	if j.ResetsAt != 0 {
		rl.ResetsAt = time.Unix(j.ResetsAt, 0)
	}
	if j.FiveHourUtil != nil && j.FiveHourReset != nil {
		rl.FiveHour = &RateLimitWindow{
			Utilization: *j.FiveHourUtil,
			ResetsAt:    time.Unix(*j.FiveHourReset, 0),
		}
	}
	if j.SevenDayUtil != nil && j.SevenDayReset != nil {
		rl.SevenDay = &RateLimitWindow{
			Utilization: *j.SevenDayUtil,
			ResetsAt:    time.Unix(*j.SevenDayReset, 0),
		}
	}
	return rl, nil
}
