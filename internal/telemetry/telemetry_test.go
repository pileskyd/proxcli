package telemetry

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestQuotaProviderScalesAndUnknowns(t *testing.T) {
	h := FromHeaders("claude", map[string]string{"anthropic-ratelimit-unified-5h-utilization": "0.53", "anthropic-ratelimit-unified-5h-reset": "1800000000", "Authorization": "Bearer secret"})
	if len(h) != 1 || h[0].RemainingPercent == nil || *h[0].RemainingPercent != 47 {
		t.Fatalf("header utilization is fractional: %#v", h)
	}
	p, e := FromProbe("claude", []byte(`{"five_hour":{"utilization":53,"resets_at":"2027-01-15T08:00:00Z"},"seven_day":null,"seven_day_sonnet":{"utilization":100}}`))
	if e != nil {
		t.Fatal(e)
	}
	if len(p) != 2 || *p[0].RemainingPercent != 47 || !p[1].Exhausted || p[1].Model != "sonnet" {
		t.Fatal("probe scale/model buckets incorrect")
	}
	p, e = FromProbe("codex", []byte(`{"rate_limit":{"primary_window":{"used_percent":0,"reset_at":1800000000},"secondary_window":{"used_percent":101}}}`))
	if e != nil {
		t.Fatal(e)
	}
	if len(p) != 1 || *p[0].RemainingPercent != 100 {
		t.Fatal("missing/invalid limits must not become zero remaining")
	}
	if len(FromHeaders("codex", map[string]string{})) != 0 {
		t.Fatal("missing observation must stay unknown")
	}
}
func TestApplicableDoesNotInventResetOrMixModels(t *testing.T) {
	now := time.Unix(1800000000, 0).UTC()
	past := now.Add(-time.Second)
	future := now.Add(time.Hour)
	zero := 0.0
	a := Account{ObservedAt: now, Windows: []Window{{ID: "expired", RemainingPercent: &zero, ResetAt: &past}, {ID: "opus", Model: "opus", ResetAt: &future}, {ID: "general", ResetAt: nil}}}
	w := Applicable(a, "claude-sonnet", now, 10*time.Minute)
	if len(w) != 1 || w[0].ID != "general" {
		t.Fatal("must ignore expired and unrelated model buckets")
	}
	if len(Applicable(a, "claude-sonnet", now.Add(11*time.Minute), 10*time.Minute)) != 0 {
		t.Fatal("stale data must not schedule")
	}
}
func TestJournalPersistsOnlyTypedRecordsAndDeduplicates(t *testing.T) {
	dir := t.TempDir()
	j, e := OpenJournal(dir)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC().Truncate(time.Second)
	u := Usage{ID: "execution-1", At: now, AuthID: "account-1", Provider: "codex", Session: SessionHash("client", "session"), Tokens: Tokens{Input: 40, Output: 10, CacheRead: 30, CacheWrite: 20, Total: 50}}
	if e = j.AddUsage(u); e != nil {
		t.Fatal(e)
	}
	if e = j.AddUsage(u); e != nil {
		t.Fatal(e)
	}
	u.ID = "execution-2"
	u.Failed = true
	u.Tokens.Total = -5
	if e = j.AddUsage(u); e != nil {
		t.Fatal(e)
	}
	a := Account{AuthID: "account-1", Provider: "codex", ObservedAt: now, Windows: []Window{}}
	if e = j.AddQuota(a); e != nil {
		t.Fatal(e)
	}
	if e = j.AddDecision(Decision{At: now, AuthID: "account-1", Provider: "codex", Session: u.Session, Reason: "new-session"}); e != nil {
		t.Fatal(e)
	}
	restored, _ := OpenJournal(dir)
	h, e := restored.History(now.Add(-time.Hour), now.Add(time.Second), "codex", "account-1")
	if e != nil {
		t.Fatal(e)
	}
	if len(h.Buckets) != 1 || h.Buckets[0].Requests != 2 || h.Buckets[0].Failed != 1 || h.Buckets[0].Tokens.Total != 50 || h.Buckets[0].Tokens.CacheWrite != 40 {
		t.Fatalf("wrong accounting: %#v", h.Buckets)
	}
	if len(h.Quotas) != 1 || len(h.Decisions) != 1 {
		t.Fatal("quota/routing history missing")
	}
	h, e = restored.History(now.Add(-time.Hour), now.Add(time.Second), "claude", "")
	if e != nil || len(h.Buckets) != 0 {
		t.Fatal("provider filter failed")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "usage-"+now.Format("2006-01-02")+".jsonl"))
	if strings.Contains(string(raw), `"session":"session"`) {
		t.Fatal("raw session ID persisted")
	}
	info, _ := os.Stat(filepath.Join(dir, "usage-"+now.Format("2006-01-02")+".jsonl"))
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("history permissions")
	}
}
func TestPartialJournalAndConservativePruning(t *testing.T) {
	dir := t.TempDir()
	j, _ := OpenJournal(dir)
	now := time.Now().UTC()
	path := filepath.Join(dir, "usage-"+now.Format("2006-01-02")+".jsonl")
	_ = os.WriteFile(path, []byte(`{"partial":`), 0600)
	h, e := j.History(now.Add(-time.Hour), now, "", "")
	if e != nil || len(h.Buckets) != 0 {
		t.Fatal("partial tail must not corrupt history")
	}
	old := filepath.Join(dir, "usage-2000-01-01.jsonl")
	keep := filepath.Join(dir, "user-notes.txt")
	_ = os.WriteFile(old, []byte(""), 0600)
	_ = os.WriteFile(keep, []byte(""), 0600)
	if e = j.Prune(now); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(old); !os.IsNotExist(e) {
		t.Fatal("old journal not pruned")
	}
	if _, e = os.Stat(keep); e != nil {
		t.Fatal("unrelated user file removed")
	}
}
