package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSanitizeDropsSecretsAndKeepsCacheAccounting(t *testing.T) {
	raw := []byte(`{"RequestID":"r1","AuthID":"a","Provider":"claude","Model":"sonnet","APIKey":"client-secret","SessionID":"private-session","RequestedAt":"2026-11-10T00:00:00Z","Failure":{"StatusCode":429,"Body":"provider-secret"},"ResponseHeaders":{"Set-Cookie":["session=secret"]},"Detail":{"InputTokens":10,"OutputTokens":20,"CacheReadTokens":30,"CacheCreationTokens":40,"CachedTokens":30,"TotalTokens":70}}`)
	var in usageInput
	if e := json.Unmarshal(raw, &in); e != nil {
		t.Fatal(e)
	}
	u := sanitize(in)
	out, e := json.Marshal(u)
	if e != nil {
		t.Fatal(e)
	}
	for _, secret := range []string{"client-secret", "private-session", "provider-secret", "Set-Cookie"} {
		if strings.Contains(string(out), secret) {
			t.Fatal("raw secret in sanitized usage")
		}
	}
	if u.Tokens.CacheRead != 30 || u.Tokens.CacheWrite != 40 || u.Tokens.Total != 70 {
		t.Fatal("cache read/write must remain separate, without double counting")
	}
	if u.Session == "" || u.At != time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC) {
		t.Fatal("usage identity missing")
	}
}

func TestClaudeColdCacheIsNotCountedAsReading(t *testing.T) {
	var in usageInput
	in.Provider = "claude"
	in.Detail.InputTokens = 10
	in.Detail.CacheCreationTokens = 100
	in.Detail.CachedTokens = 100
	got := sanitize(in).Tokens
	if got.CacheRead != 0 || got.CacheWrite != 100 || got.Input != 10 {
		t.Fatalf("cold cache normalized incorrectly: %#v", got)
	}
}
func TestCodexInputExcludesCachedSubset(t *testing.T) {
	var in usageInput
	in.Provider = "codex"
	in.Detail.InputTokens = 100
	in.Detail.CachedTokens = 70
	got := sanitize(in).Tokens
	if got.CacheRead != 70 || got.Input != 30 {
		t.Fatalf("cached input counted twice: %#v", got)
	}
}
