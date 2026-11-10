package telemetry

import (
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Window is an observed subscription limit, never an estimate derived from tokens.
type Window struct {
	ID               string     `json:"id"`
	Model            string     `json:"model,omitempty"`
	RemainingPercent *float64   `json:"remainingPercent"`
	ResetAt          *time.Time `json:"resetAt"`
	Exhausted        bool       `json:"exhausted"`
}
type Account struct {
	AuthID     string    `json:"authId"`
	Provider   string    `json:"provider"`
	Disabled   bool      `json:"disabled"`
	ObservedAt time.Time `json:"observedAt"`
	Source     string    `json:"source"`
	Windows    []Window  `json:"windows"`
	Error      string    `json:"error,omitempty"`
}
type Snapshot struct {
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
	Accounts  []Account `json:"accounts"`
}

func remaining(used float64) *float64 {
	if math.IsNaN(used) || math.IsInf(used, 0) || used < 0 || used > 100 {
		return nil
	}
	n := 100 - used
	return &n
}
func epoch(raw string) *time.Time {
	if v, e := strconv.ParseFloat(raw, 64); e == nil && v > 0 && v < 4102444800 {
		t := time.Unix(int64(v), 0).UTC()
		return &t
	}
	if t, e := time.Parse(time.RFC3339, raw); e == nil {
		t = t.UTC()
		return &t
	}
	return nil
}
func CanonicalProvider(provider string) string {
	switch strings.ToLower(provider) {
	case "anthropic", "claude":
		return "claude"
	case "codex":
		return "codex"
	}
	return strings.ToLower(provider)
}

// FromHeaders accepts only known limit watermarks; unrelated headers never persist.
func FromHeaders(provider string, signals map[string]string) []Window {
	h := http.Header{}
	for k, v := range signals {
		h.Set(k, v)
	}
	var windows []Window
	if CanonicalProvider(provider) == "claude" {
		for _, id := range []string{"5h", "7d", "7d-sonnet", "7d-opus"} {
			p := "Anthropic-Ratelimit-Unified-" + id + "-"
			raw := h.Get(p + "Utilization")
			status := strings.ToLower(h.Get(p + "Status"))
			reset := epoch(h.Get(p + "Reset"))
			if raw == "" && status == "" && reset == nil {
				continue
			}
			w := Window{ID: id, ResetAt: reset, Exhausted: status == "rejected"}
			if n, e := strconv.ParseFloat(raw, 64); e == nil {
				w.RemainingPercent = remaining(n * 100)
			}
			if strings.HasSuffix(id, "sonnet") {
				w.Model = "sonnet"
			}
			if strings.HasSuffix(id, "opus") {
				w.Model = "opus"
			}
			if w.RemainingPercent != nil && *w.RemainingPercent <= 0 {
				w.Exhausted = true
			}
			windows = append(windows, w)
		}
	} else if CanonicalProvider(provider) == "codex" {
		for _, id := range []string{"primary", "secondary"} {
			p := "X-Codex-" + id + "-"
			raw := h.Get(p + "Used-Percent")
			reset := epoch(h.Get(p + "Reset-At"))
			if raw == "" && reset == nil {
				continue
			}
			w := Window{ID: id, ResetAt: reset}
			if n, e := strconv.ParseFloat(raw, 64); e == nil {
				w.RemainingPercent = remaining(n)
			}
			if w.RemainingPercent != nil && *w.RemainingPercent <= 0 {
				w.Exhausted = true
			}
			windows = append(windows, w)
		}
	}
	return windows
}
func FromProbe(provider string, raw []byte) ([]Window, error) {
	if CanonicalProvider(provider) == "claude" {
		var body map[string]json.RawMessage
		if e := json.Unmarshal(raw, &body); e != nil {
			return nil, e
		}
		var result []Window
		for _, id := range []string{"five_hour", "seven_day", "seven_day_sonnet", "seven_day_opus"} {
			rawWindow, ok := body[id]
			var b struct {
				Utilization *float64 `json:"utilization"`
				ResetsAt    string   `json:"resets_at"`
			}
			if !ok || json.Unmarshal(rawWindow, &b) != nil || b.Utilization == nil {
				continue
			}
			w := Window{ID: map[string]string{"five_hour": "5h", "seven_day": "7d", "seven_day_sonnet": "7d-sonnet", "seven_day_opus": "7d-opus"}[id], RemainingPercent: remaining(*b.Utilization), ResetAt: epoch(b.ResetsAt)}
			if strings.HasSuffix(id, "sonnet") {
				w.Model = "sonnet"
			}
			if strings.HasSuffix(id, "opus") {
				w.Model = "opus"
			}
			if w.RemainingPercent == nil {
				continue
			}
			w.Exhausted = *w.RemainingPercent <= 0
			result = append(result, w)
		}
		return result, nil
	}
	var body struct {
		RateLimit struct {
			Primary   *probeWindow `json:"primary_window"`
			Secondary *probeWindow `json:"secondary_window"`
		} `json:"rate_limit"`
	}
	if e := json.Unmarshal(raw, &body); e != nil {
		return nil, e
	}
	result := []Window{}
	for _, b := range []struct {
		id string
		w  *probeWindow
	}{{"primary", body.RateLimit.Primary}, {"secondary", body.RateLimit.Secondary}} {
		if b.w == nil || b.w.UsedPercent == nil {
			continue
		}
		w := Window{ID: b.id, RemainingPercent: remaining(*b.w.UsedPercent), ResetAt: epoch(strconv.FormatInt(b.w.ResetAt, 10))}
		if w.RemainingPercent == nil {
			continue
		}
		w.Exhausted = *w.RemainingPercent <= 0
		result = append(result, w)
	}
	return result, nil
}

type probeWindow struct {
	UsedPercent *float64 `json:"used_percent"`
	ResetAt     int64    `json:"reset_at"`
}

// Applicable ignores unrelated model-specific buckets and expired observations.
func Applicable(a Account, model string, now time.Time, maxAge time.Duration) []Window {
	if a.ObservedAt.IsZero() || now.Sub(a.ObservedAt) > maxAge || a.ObservedAt.After(now.Add(time.Minute)) {
		return nil
	}
	var out []Window
	for _, w := range a.Windows {
		if w.Model != "" && !strings.Contains(strings.ToLower(model), w.Model) {
			continue
		}
		if w.ResetAt != nil && !w.ResetAt.After(now) {
			continue
		}
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
