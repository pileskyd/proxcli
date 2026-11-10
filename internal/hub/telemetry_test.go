package hub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCollectorProbesThroughCredentialAndDoesNotDrainUsageQueue(t *testing.T) {
	probes := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v8/management/credentials":
			jsonOut(w, 200, map[string]any{"files": []map[string]any{{"id": "a", "auth_index": "index-a", "provider": "claude"}}})
		case "/v8/management/requests/api-call":
			probes++
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			if b["auth_index"] != "index-a" || b["url"] != "https://api.anthropic.com/api/oauth/usage" || b["method"] != "GET" {
				t.Error("wrong quota probe")
			}
			if _, exists := b["proxy_url"]; exists {
				t.Error("must preserve per-credential proxy")
			}
			headers := b["header"].(map[string]any)
			if headers["Authorization"] != "Bearer $TOKEN$" {
				t.Error("must resolve token inside CPA")
			}
			jsonOut(w, 200, map[string]any{"status_code": 200, "body": `{"five_hour":{"utilization":42,"resets_at":"2099-01-01T00:00:00Z"},"token":"sensitive-do-not-store"}`})
		default:
			t.Errorf("unexpected upstream path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer up.Close()
	s, e := New(Config{Upstream: up.URL, ManagementKey: "test", DataDir: t.TempDir()})
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	if e = s.collectTelemetry(context.Background(), now); e != nil {
		t.Fatal(e)
	}
	if e = s.collectTelemetry(context.Background(), now.Add(time.Minute)); e != nil {
		t.Fatal(e)
	}
	if probes != 1 {
		t.Fatal("probe throttling failed")
	}
	w := call(t, s, "GET", "/api/quotas", "")
	requireCode(t, w, 200)
	if !strings.Contains(w.Body.String(), `"remainingPercent":58`) || strings.Contains(w.Body.String(), "sensitive") {
		t.Fatal("quota parsing/privacy failed")
	}
	w = call(t, s, "GET", "/api/history?range=24h", "")
	requireCode(t, w, 200)
	if !strings.Contains(w.Body.String(), `"authId":"a"`) {
		t.Fatal("quota history missing")
	}
	requireCode(t, call(t, s, "GET", "/api/history?range=forever", ""), 400)
}
