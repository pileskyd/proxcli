package hub

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"proxcli/internal/telemetry"
)

type quotaObservation struct {
	ObservedAt time.Time         `json:"observed_at"`
	Signals    map[string]string `json:"signals"`
}
type collector struct {
	mu        sync.Mutex
	collectMu sync.Mutex
	snapshot  telemetry.Snapshot
	journal   *telemetry.Journal
	path      string
	lastProbe map[string]time.Time
	lastError string
	wake      chan struct{}
	running   bool
}

func newCollector(dir string) (*collector, error) {
	dir = filepath.Join(dir, "telemetry")
	j, e := telemetry.OpenJournal(dir)
	if e != nil {
		return nil, e
	}
	c := &collector{journal: j, path: filepath.Join(dir, "quotas.json"), lastProbe: map[string]time.Time{}, wake: make(chan struct{}, 1), snapshot: telemetry.Snapshot{Version: 1, Accounts: []telemetry.Account{}}}
	if previous, e := telemetry.ReadSnapshot(c.path); e == nil {
		c.snapshot = previous
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	return c, nil
}

// RunTelemetry owns collection; cancellation stops outbound requests on shutdown.
func (s *Server) RunTelemetry(ctx context.Context) {
	s.telemetry.mu.Lock()
	if s.telemetry.running {
		s.telemetry.mu.Unlock()
		return
	}
	s.telemetry.running = true
	s.telemetry.mu.Unlock()
	defer func() { s.telemetry.mu.Lock(); s.telemetry.running = false; s.telemetry.mu.Unlock() }()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		collectCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		e := s.collectTelemetry(collectCtx, time.Now().UTC())
		cancel()
		s.telemetry.mu.Lock()
		if e != nil {
			s.telemetry.lastError = e.Error()
		} else {
			s.telemetry.lastError = ""
		}
		s.telemetry.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.telemetry.wake:
		}
	}
}
func (s *Server) collectTelemetry(ctx context.Context, now time.Time) error {
	c := s.telemetry
	if !c.collectMu.TryLock() {
		return nil
	}
	defer c.collectMu.Unlock()
	files, e := s.credentials(ctx)
	if e != nil {
		return e
	}
	c.mu.Lock()
	old := c.snapshot
	c.mu.Unlock()
	previous := map[string]telemetry.Account{}
	for _, a := range old.Accounts {
		previous[a.AuthID] = a
	}
	if live, e := telemetry.ReadSnapshot(filepath.Join(s.cfg.DataDir, "telemetry", "usage-quotas.json")); e == nil {
		for _, a := range live.Accounts {
			if a.ObservedAt.After(previous[a.AuthID].ObservedAt) {
				previous[a.AuthID] = a
			}
		}
	}
	snapshot := telemetry.Snapshot{Version: 1, UpdatedAt: now, Accounts: []telemetry.Account{}}
	for _, f := range files {
		provider := f.Provider
		if provider == "" {
			provider = f.Type
		}
		provider = telemetry.CanonicalProvider(provider)
		a := previous[f.ID]
		a.AuthID = f.ID
		a.Provider = provider
		a.Disabled = f.Disabled
		if a.Windows == nil {
			a.Windows = []telemetry.Window{}
		}
		if f.Quota.ObservedAt.After(a.ObservedAt) {
			windows := telemetry.FromHeaders(provider, f.Quota.Signals)
			if len(windows) > 0 {
				a.Windows = windows
				a.ObservedAt = f.Quota.ObservedAt
				a.Source = "response-headers"
				a.Error = ""
			}
		}
		// Probe only fixed provider URLs through CPA: tokens stay inside CPA and per-auth proxies are preserved.
		if !f.Disabled && f.AuthIndex != "" && (provider == "claude" || provider == "codex") && now.Sub(c.lastProbe[f.ID]) >= 5*time.Minute {
			if ctx.Err() != nil {
				a.Error = "Сбор квот прерван; повторная проверка позже"
			} else {
				c.lastProbe[f.ID] = now
				windows, e := s.probeQuota(ctx, f, provider)
				if e != nil {
					a.Error = "Не удалось обновить квоту: проверьте доступ к usage API и прокси аккаунта"
				} else if len(windows) > 0 {
					a.Windows = windows
					a.ObservedAt = time.Now().UTC()
					a.Source = "provider-usage-api"
					a.Error = ""
				} else {
					a.Error = "Провайдер не вернул поддерживаемые окна квот"
				}
			}
		}
		before := previous[f.ID]
		if !a.ObservedAt.IsZero() && a.ObservedAt.After(before.ObservedAt) {
			if e = c.journal.AddQuota(a); e != nil {
				return e
			}
		}
		snapshot.Accounts = append(snapshot.Accounts, a)
	}
	if e = telemetry.AtomicJSON(c.path, snapshot); e != nil {
		return e
	}
	c.mu.Lock()
	c.snapshot = snapshot
	c.mu.Unlock()
	if e = c.journal.Prune(now); e != nil {
		return e
	}
	return s.reconcileRouting(ctx, files, snapshot, now)
}
func (s *Server) probeQuota(ctx context.Context, f credential, provider string) ([]telemetry.Window, error) {
	endpoint := "https://api.anthropic.com/api/oauth/usage"
	headers := map[string]string{"Authorization": "Bearer $TOKEN$", "Accept": "application/json"}
	if provider == "claude" {
		headers["anthropic-beta"] = "oauth-2025-04-20"
	} else {
		endpoint = "https://chatgpt.com/backend-api/wham/usage"
		if f.AccountID == "" {
			f.AccountID = f.IDToken.AccountID
		}
		if f.AccountID != "" {
			headers["ChatGPT-Account-Id"] = f.AccountID
		}
	}
	var response struct {
		StatusCode int    `json:"status_code"`
		Body       string `json:"body"`
	}
	body := map[string]any{"auth_index": f.AuthIndex, "method": "GET", "url": endpoint, "header": headers}
	if e := s.upstream(ctx, "POST", "/v8/management/requests/api-call", body, &response, false); e != nil {
		return nil, e
	}
	if response.StatusCode != 200 {
		return nil, fail(502, "Не удалось получить квоту аккаунта")
	}
	return telemetry.FromProbe(provider, []byte(response.Body))
}
func (s *Server) quotas(w http.ResponseWriter, r *http.Request) error {
	c := s.telemetry
	c.mu.Lock()
	snapshot := c.snapshot
	running := c.running
	lastError := c.lastError
	c.mu.Unlock()
	jsonOut(w, 200, map[string]any{"snapshot": snapshot, "collecting": running, "error": lastError, "staleAfterSeconds": 600, "source": "provider-usage-api / response-headers"})
	return nil
}
func (s *Server) refreshTelemetry(w http.ResponseWriter, r *http.Request) error {
	var body struct{}
	if e := decode(w, r, &body); e != nil {
		return e
	}
	select {
	case s.telemetry.wake <- struct{}{}:
	default:
	}
	jsonOut(w, 202, map[string]any{"ok": true, "message": "Обновление запрошено; активные проверки выполняются не чаще раза в 5 минут на аккаунт"})
	return nil
}
func (s *Server) history(w http.ResponseWriter, r *http.Request) error {
	now := time.Now().UTC()
	duration := 24 * time.Hour
	switch r.URL.Query().Get("range") {
	case "", "24h":
	case "6h":
		duration = 6 * time.Hour
	case "7d":
		duration = 7 * 24 * time.Hour
	case "30d":
		duration = 30 * 24 * time.Hour
	default:
		return fail(400, "Диапазон: 6h, 24h, 7d или 30d")
	}
	h, e := s.telemetry.journal.History(now.Add(-duration), now, r.URL.Query().Get("provider"), r.URL.Query().Get("authId"))
	if e != nil {
		return fail(503, "Не удалось прочитать историю: уменьшите диапазон или проверьте каталог данных")
	}
	jsonOut(w, 200, h)
	return nil
}

func readJSONFile(path string, value any) error {
	raw, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	return json.Unmarshal(raw, value)
}
