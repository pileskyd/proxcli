package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestValidateProxy(t *testing.T) {
	for _, v := range []string{"socks5h://user:p%40ss@[::1]:1080", "socks5://localhost:1080", "https://example.com:443", "http://u:p@example.com:8080"} {
		if e := ValidateProxy(v); e != nil {
			t.Errorf("valid %s: %v", maskProxy(v), e)
		}
	}
	for _, v := range []string{"", "direct", "ftp://example.com:21", "http://:80", "socks5://example.com", "http://example.com:0", "http://example.com:65536", "http://example.com:abc", "http://example.com:80/path", "http://example.com:80?token=x", "http://example.com:80#x", "http://example.com:80?", "http://a:***@host:80", "http://bad host:80", "http://[wrong]:80"} {
		if ValidateProxy(v) == nil {
			t.Errorf("accepted invalid URL %q", v)
		}
	}
}

type fixture struct {
	mu       sync.Mutex
	patches  []map[string]any
	oauth    map[string]any
	requests int
}

func fixtureServer(t *testing.T) (*Server, *fixture, Config) {
	t.Helper()
	f := &fixture{}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests++
		if r.Header.Get("Authorization") != "Bearer management-test" {
			t.Errorf("wrong management authentication")
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/v8/management/credentials":
			jsonOut(w, 200, map[string]any{"files": []map[string]any{{"id": "auth-a", "name": "a.json", "provider": "claude", "status": "active"}, {"id": "auth-b", "name": "b.json", "provider": "codex", "status": "active"}}})
		case "/v0/management/proxy-url":
			jsonOut(w, 200, map[string]string{"proxy-url": "http://default:password@proxy:8080"})
		case "/v8/management/credentials/fields":
			if r.Method != "PATCH" {
				t.Error("wrong patch method")
			}
			var v map[string]any
			_ = json.NewDecoder(r.Body).Decode(&v)
			f.patches = append(f.patches, v)
			jsonOut(w, 200, map[string]string{"status": "ok"})
		case "/v8/management/oauth/auth-url":
			if r.Method != "POST" {
				t.Error("OAuth proxy must be in POST body")
			}
			_ = json.NewDecoder(r.Body).Decode(&f.oauth)
			jsonOut(w, 200, map[string]string{"url": "https://provider.example/auth?state=test-state", "state": "test-state"})
		case "/v8/management/oauth/status":
			jsonOut(w, 200, map[string]any{"status": "ok", "auth_id": "auth-b", "saved": true, "adopted": true})
		case "/v8/management/oauth/callback":
			jsonOut(w, 200, map[string]string{"status": "ok"})
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(up.Close)
	cfg := Config{Upstream: up.URL, ManagementKey: "management-test", DataDir: t.TempDir(), WebDir: t.TempDir()}
	s, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	return s, f, cfg
}
func call(t *testing.T, s *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "http://127.0.0.1:8787"+path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func requireCode(t *testing.T, w *httptest.ResponseRecorder, code int) {
	t.Helper()
	if w.Code != code {
		t.Fatalf("HTTP %d expected %d: %s", w.Code, code, w.Body.String())
	}
}
func TestPerSubscriptionIsolationPersistenceAndMask(t *testing.T) {
	s, f, cfg := fixtureServer(t)
	w := call(t, s, "PATCH", "/api/subscriptions", `{"name":"a.json","proxyMode":"custom","proxyUrl":"socks5h://alice:very-secret@proxy-a:1080","prefix":"work"}`)
	requireCode(t, w, 200)
	if len(f.patches) != 1 || f.patches[0]["name"] != "a.json" || f.patches[0]["proxy_url"] != "socks5h://alice:very-secret@proxy-a:1080" || f.patches[0]["prefix"] != "work" {
		t.Fatal("wrong upstream patch")
	}
	check := func(app *Server) {
		t.Helper()
		w := call(t, app, "GET", "/api/subscriptions", "")
		requireCode(t, w, 200)
		if strings.Contains(w.Body.String(), "very-secret") || strings.Contains(w.Body.String(), "password") {
			t.Fatal("password leaked to browser")
		}
		var body struct {
			Subscriptions []struct {
				ID                          string
				ProxyKnown                  bool
				ProxyMode, ProxyURL, Prefix string
			}
			DefaultProxy string
		}
		if e := json.Unmarshal(w.Body.Bytes(), &body); e != nil {
			t.Fatal(e)
		}
		if body.Subscriptions[0].Prefix != "work" {
			t.Fatal("prefix not preserved")
		}
		if len(body.Subscriptions) != 2 || !body.Subscriptions[0].ProxyKnown || body.Subscriptions[1].ProxyKnown {
			t.Fatal("route isolation / unknown imported account failed")
		}
	}
	check(s)
	restored, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	check(restored)
	encrypted, e := os.ReadFile(filepath.Join(cfg.DataDir, "state.enc"))
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(encrypted, []byte("very-secret")) {
		t.Fatal("plaintext password on disk")
	}
	info, _ := os.Stat(filepath.Join(cfg.DataDir, "state.enc"))
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("state file must have mode 0600")
	}
	// Omitted URL keeps real stored password, never replaces it with a browser mask.
	requireCode(t, call(t, restored, "PATCH", "/api/subscriptions", `{"name":"a.json","proxyMode":"custom","prefix":"next"}`), 200)
	if f.patches[1]["proxy_url"] != f.patches[0]["proxy_url"] {
		t.Fatal("stored password lost")
	}
	requireCode(t, call(t, restored, "PATCH", "/api/subscriptions", `{"name":"a.json","proxyMode":"inherit"}`), 200)
	if f.patches[2]["proxy_url"] != "" {
		t.Fatal("inherit must send empty string, never null")
	}
	requireCode(t, call(t, restored, "PATCH", "/api/subscriptions", `{"name":"a.json","proxyMode":"direct"}`), 200)
	if f.patches[3]["proxy_url"] != "direct" {
		t.Fatal("direct must bypass global and env proxy")
	}
}
func TestInvalidProxyNeverReachesPatch(t *testing.T) {
	s, f, _ := fixtureServer(t)
	requireCode(t, call(t, s, "PATCH", "/api/subscriptions", `{"name":"a.json","proxyMode":"custom","proxyUrl":"invalid"}`), 400)
	requireCode(t, call(t, s, "PATCH", "/api/subscriptions", `{"name":"a.json","proxyMode":"custom"}`), 400)
	if len(f.patches) != 0 {
		t.Fatal("invalid proxy reached upstream")
	}
}
func TestOAuthUsesSameProxyAndPersistsByAuthID(t *testing.T) {
	s, f, cfg := fixtureServer(t)
	requireCode(t, call(t, s, "POST", "/api/oauth/start", `{"provider":"claude","proxyMode":"custom","proxyUrl":"http://u:secret@exit:8080"}`), 200)
	if f.oauth["provider"] != "anthropic" || f.oauth["proxy_url"] != "http://u:secret@exit:8080" {
		t.Fatal("OAuth proxy not bound before token exchange")
	}
	restored, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	requireCode(t, call(t, restored, "POST", "/api/oauth/callback", `{"provider":"codex","url":"http://localhost:1455/callback?code=x&state=test-state"}`), 400)
	requireCode(t, call(t, restored, "POST", "/api/oauth/callback", `{"provider":"claude","url":"http://localhost:54545/callback?code=x&state=test-state"}`), 200)
	requireCode(t, call(t, restored, "GET", "/api/oauth/status?state=test-state", ""), 200)
	restored.store.mu.Lock()
	defer restored.store.mu.Unlock()
	if p := restored.store.state.Routes["auth-b"]; !p.Confirmed || p.URL != "http://u:secret@exit:8080" {
		t.Fatal("OAuth route not associated with completed auth_id")
	}
	if len(f.patches) != 0 {
		t.Fatal("OAuth must not patch another account heuristically")
	}
}
func TestGuardBlocksCrossOriginAndDNSRebinding(t *testing.T) {
	s, f, _ := fixtureServer(t)
	for _, tc := range []struct{ host, origin, site string }{{"evil.example:8787", "", ""}, {"127.0.0.1:8787", "https://evil.example", ""}, {"127.0.0.1:8787", "", "cross-site"}} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8787/api/subscriptions", nil)
		r.Host = tc.host
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Sec-Fetch-Site", tc.site)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		requireCode(t, w, 403)
	}
	if f.requests != 0 {
		t.Fatal("blocked requests reached upstream")
	}
}
func TestErrorDoesNotLeakUpstreamSecrets(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = io.WriteString(w, `{"error":"http://u:password@host:80 code=secret"}`)
	}))
	defer up.Close()
	s, e := New(Config{Upstream: up.URL, ManagementKey: "test", DataDir: t.TempDir()})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.credentials(context.Background())
	if e == nil || strings.Contains(e.Error(), "password") || strings.Contains(e.Error(), "code=") {
		t.Fatal("upstream body leaked")
	}
}
func TestStaticSPAAndMissingAssets(t *testing.T) {
	s, _, cfg := fixtureServer(t)
	if e := os.WriteFile(filepath.Join(cfg.WebDir, "index.html"), []byte("ProxCli"), 0600); e != nil {
		t.Fatal(e)
	}
	requireCode(t, call(t, s, "GET", "/subscriptions", ""), 200)
	requireCode(t, call(t, s, "GET", "/missing.js", ""), 404)
	requireCode(t, call(t, s, "GET", "/api/missing", ""), 404)
}

func TestOAuthDirectNeverFallsBackToAnotherRoute(t *testing.T) {
	s, f, _ := fixtureServer(t)
	requireCode(t, call(t, s, "POST", "/api/oauth/start", `{"provider":"codex","proxyMode":"direct"}`), 409)
	if f.requests != 0 {
		t.Fatal("direct OAuth must not silently use the global route")
	}
}
