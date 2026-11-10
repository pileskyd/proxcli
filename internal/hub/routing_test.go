package hub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"proxcli/internal/telemetry"
)

func TestSoonestResetRankingAndStaleness(t *testing.T) {
	now := time.Now().UTC()
	soon := now.Add(time.Hour)
	late := now.Add(5 * time.Hour)
	old := now.Add(-time.Hour)
	remaining, zero := 80.0, 0.0
	files := []credential{{ID: "late", Provider: "claude"}, {ID: "soon", Provider: "claude"}, {ID: "unknown", Provider: "claude"}, {ID: "blocked", Provider: "claude"}, {ID: "stale", Provider: "claude"}}
	snapshot := telemetry.Snapshot{Accounts: []telemetry.Account{{AuthID: "late", ObservedAt: now, Windows: []telemetry.Window{{RemainingPercent: &remaining, ResetAt: &late}}}, {AuthID: "soon", ObservedAt: now, Windows: []telemetry.Window{{RemainingPercent: &remaining, ResetAt: &soon}}}, {AuthID: "blocked", ObservedAt: now, Windows: []telemetry.Window{{RemainingPercent: &zero, ResetAt: &soon}}}, {AuthID: "stale", ObservedAt: old, Windows: []telemetry.Window{{RemainingPercent: &remaining, ResetAt: &soon}}}}}
	ranked := rankAccounts(files, snapshot, now)
	if ranked[0].AuthID != "soon" || ranked[1].AuthID != "late" || ranked[len(ranked)-1].AuthID != "blocked" {
		t.Fatalf("wrong ordering %#v", ranked)
	}
	for _, a := range ranked {
		if (a.AuthID == "unknown" || a.AuthID == "stale") && (a.Priority != 0 || a.ResetAt != nil) {
			t.Fatal("unknown/stale quota must not get fabricated reset")
		}
	}
}
func TestPriorityControllerRequiresVerifiedStrictCoreAndPatchesOnlyPriority(t *testing.T) {
	now := time.Now().UTC()
	reset := now.Add(time.Hour)
	remaining := 50.0
	patched := 0
	strict := false
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v8/management/proxcli/policy":
			if !strict {
				w.WriteHeader(404)
				return
			}
			jsonOut(w, 200, map[string]string{"policy": "quota-strict-v1"})
		case "/v8/management/config/routing":
			jsonOut(w, 200, map[string]any{"strategy": "fill-first", "session-affinity": true, "session-affinity-rebind-on": "quota-strict"})
		case "/v8/management/credentials/fields":
			patched++
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			if len(b) != 2 || b["name"] != "a.json" || b["priority"] != float64(10000) {
				t.Error("priority controller must not change proxy/prefix/tokens")
			}
			jsonOut(w, 200, map[string]bool{"ok": true})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer up.Close()
	s, e := New(Config{Upstream: up.URL, ManagementKey: "test", DataDir: t.TempDir(), ManageRouting: true})
	if e != nil {
		t.Fatal(e)
	}
	files := []credential{{ID: "a", Name: "a.json", Provider: "claude"}}
	snapshot := telemetry.Snapshot{Accounts: []telemetry.Account{{AuthID: "a", ObservedAt: now, Windows: []telemetry.Window{{RemainingPercent: &remaining, ResetAt: &reset}}}}}
	if e = s.reconcileRouting(context.Background(), files, snapshot, now); e != nil {
		t.Fatal(e)
	}
	if patched != 0 {
		t.Fatal("stock core must not be managed under a false strict guarantee")
	}
	strict = true
	if e = s.reconcileRouting(context.Background(), files, snapshot, now); e != nil {
		t.Fatal(e)
	}
	if patched != 1 {
		t.Fatal("priority not applied")
	}
	files[0].Priority = 10000
	if e = s.reconcileRouting(context.Background(), files, snapshot, now); e != nil {
		t.Fatal(e)
	}
	if patched != 1 {
		t.Fatal("unchanged priorities must not rewrite credentials")
	}
	w := call(t, s, "GET", "/api/routing", "")
	requireCode(t, w, 200)
	var status struct{ Active bool }
	_ = json.Unmarshal(w.Body.Bytes(), &status)
	if !status.Active {
		t.Fatal("verified routing not active")
	}
}
