package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"proxcli/internal/telemetry"
	"strings"
	"testing"
	"time"
)

// Opt-in contract test against an actual upstream binary, isolated from user auth.
func TestCLIProxyAPIIntegration(t *testing.T) {
	binary := os.Getenv("PROXCLI_TEST_CORE")
	if binary == "" {
		t.Skip("set PROXCLI_TEST_CORE to a CLIProxyAPI v8 binary")
	}
	dir := t.TempDir()
	authDir := filepath.Join(dir, "auth")
	if e := os.Mkdir(authDir, 0700); e != nil {
		t.Fatal(e)
	}
	// Synthetic credential. No usable tokens or real provider traffic.
	auth := `{"type":"codex","email":"fixture@example.invalid","access_token":"not-a-real-token","refresh_token":"","expired":"2099-01-01T00:00:00Z"}`
	if e := os.WriteFile(filepath.Join(authDir, "fixture.json"), []byte(auth), 0600); e != nil {
		t.Fatal(e)
	}
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	key := "integration-management-not-a-real-secret"
	config := fmt.Sprintf("config-version: 8\nserver:\n  host: 127.0.0.1\n  port: %d\nmanagement:\n  secret-key: %q\n  allow-remote: false\n  disable-control-panel: true\naccess:\n  api-keys: [integration-access]\noauth:\n  auth-dir: %q\nrequests:\n  proxy-url: \"\"\n", port, key, authDir)
	if os.Getenv("PROXCLI_TEST_POLICY") == "true" {
		config += "routing:\n  strategy: fill-first\n  session-affinity: true\n  session-affinity-ttl: 168h\n  session-affinity-rebind-on: quota-strict\n"
	}
	if plugin := os.Getenv("PROXCLI_TEST_PLUGIN"); plugin != "" {
		config += fmt.Sprintf("plugins:\n  enabled: true\n  dir: %q\n  configs:\n    proxcli-usage:\n      enabled: true\n", filepath.Dir(plugin))
	}
	configPath := filepath.Join(dir, "config.yaml")
	if e = os.WriteFile(configPath, []byte(config), 0600); e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(binary, "-config", configPath)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "HTTP_PROXY=", "HTTPS_PROXY=", "ALL_PROXY=", "http_proxy=", "https_proxy=", "all_proxy=", "PROXCLI_DATA_DIR="+filepath.Join(dir, "panel"))
	// Do not surface upstream logs: OAuth URLs and credential details belong to the hub.
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	s, e := New(Config{Upstream: fmt.Sprintf("http://127.0.0.1:%d", port), ManagementKey: key, DataDir: filepath.Join(dir, "panel")})
	if e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		w := call(t, s, "GET", "/api/status", "")
		if strings.Contains(w.Body.String(), `"connected":true`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("core did not become ready: %s", w.Body.String())
		}
		time.Sleep(100 * time.Millisecond)
	}
	var w *httptest.ResponseRecorder
	for {
		w = call(t, s, "GET", "/api/subscriptions", "")
		if bytes.Contains(w.Body.Bytes(), []byte("fixture.json")) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("credential not discovered: %s", w.Body.String())
		}
		time.Sleep(100 * time.Millisecond)
	}
	requireCode(t, w, 200)
	requireCode(t, call(t, s, "PATCH", "/api/subscriptions", `{"name":"fixture.json","proxyMode":"custom","proxyUrl":"socks5h://u:secret@127.0.0.1:9","prefix":"fixture"}`), 200)
	raw, e := os.ReadFile(filepath.Join(authDir, "fixture.json"))
	if e != nil {
		t.Fatal(e)
	}
	var saved map[string]any
	if e = json.Unmarshal(raw, &saved); e != nil {
		t.Fatal(e)
	}
	if saved["proxy_url"] != "socks5h://u:secret@127.0.0.1:9" || saved["prefix"] != "fixture" {
		t.Fatal("upstream did not persist per-auth proxy/prefix")
	}
	requireCode(t, call(t, s, "PATCH", "/api/subscriptions/status", `{"name":"fixture.json","disabled":true}`), 200)
	requireCode(t, call(t, s, "PATCH", "/api/subscriptions", `{"name":"fixture.json","proxyMode":"direct"}`), 200)
	requireCode(t, call(t, s, "PATCH", "/api/subscriptions", `{"name":"fixture.json","proxyMode":"inherit"}`), 200)
	// Generate URL only. Never follow it or exchange a real authorization code.
	w = call(t, s, "POST", "/api/oauth/start", `{"provider":"codex","proxyMode":"custom","proxyUrl":"socks5h://u:secret@127.0.0.1:9"}`)
	requireCode(t, w, 200)
	var started struct {
		URL   string `json:"url"`
		State string `json:"state"`
	}
	if e = json.Unmarshal(w.Body.Bytes(), &started); e != nil || started.State == "" {
		t.Fatal("OAuth start did not return state")
	}
	w = call(t, s, "GET", "/api/oauth/status?state="+started.State, "")
	requireCode(t, w, 200)
	if !strings.Contains(w.Body.String(), `"status":"wait"`) {
		t.Fatalf("expected pending OAuth: %s", w.Body.String())
	}
	if os.Getenv("PROXCLI_TEST_POLICY") == "true" {
		requireCode(t, call(t, s, "PATCH", "/api/subscriptions/status", `{"name":"fixture.json","disabled":false}`), 200)
		files, err := s.credentials(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		var fixture credential
		for _, f := range files {
			if f.Name == "fixture.json" {
				fixture = f
			}
		}
		now := time.Now().UTC()
		reset := now.Add(time.Hour)
		remaining := 50.0
		s.cfg.ManageRouting = true
		if err = s.reconcileRouting(context.Background(), files, telemetry.Snapshot{Accounts: []telemetry.Account{{AuthID: fixture.ID, ObservedAt: now, Windows: []telemetry.Window{{RemainingPercent: &remaining, ResetAt: &reset}}}}}, now); err != nil {
			t.Fatal(err)
		}
		status := call(t, s, "GET", "/api/routing", "")
		requireCode(t, status, 200)
		if !strings.Contains(status.Body.String(), `"active":true`) {
			t.Fatalf("strict core not activated: %s", status.Body.String())
		}
		raw, err := os.ReadFile(filepath.Join(authDir, "fixture.json"))
		if err != nil {
			t.Fatal(err)
		}
		var record map[string]any
		_ = json.Unmarshal(raw, &record)
		if record["priority"] != float64(10000) || record["prefix"] != "fixture" || record["proxy_url"] != "" {
			t.Fatal("real priority update altered unrelated auth fields")
		}
	}
	if os.Getenv("PROXCLI_TEST_PLUGIN") != "" {
		if _, err := os.Stat(filepath.Join(dir, "panel", "telemetry", "usage-status.json")); err != nil {
			t.Fatalf("usage plugin not loaded: %v", err)
		}
	}
	t.Log("real CLIProxyAPI: list, per-auth proxy/prefix persistence, toggle, direct/inherit and OAuth with custom proxy passed")
}
