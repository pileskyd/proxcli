package hub

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"proxcli/internal/telemetry"
)

type routingAccount struct {
	AuthID   string     `json:"authId"`
	Provider string     `json:"provider"`
	Priority int        `json:"priority"`
	ResetAt  *time.Time `json:"resetAt"`
	Reason   string     `json:"reason"`
}
type routingState struct {
	Active    bool             `json:"active"`
	Enabled   bool             `json:"enabled"`
	Strategy  string           `json:"strategy"`
	Policy    string           `json:"policy"`
	UpdatedAt time.Time        `json:"updatedAt"`
	Error     string           `json:"error,omitempty"`
	Accounts  []routingAccount `json:"accounts"`
}

func rankAccounts(files []credential, snapshot telemetry.Snapshot, now time.Time) []routingAccount {
	byID := map[string]telemetry.Account{}
	for _, a := range snapshot.Accounts {
		byID[a.AuthID] = a
	}
	ranked := []routingAccount{}
	for _, f := range files {
		provider := telemetry.CanonicalProvider(f.Provider)
		if provider == "" {
			provider = telemetry.CanonicalProvider(f.Type)
		}
		if provider != "claude" && provider != "codex" {
			continue
		}
		r := routingAccount{AuthID: f.ID, Provider: provider, Reason: "unknown-quota"}
		windows := telemetry.Applicable(byID[f.ID], "", now, 10*time.Minute)
		blocked := false
		for _, w := range windows {
			// Per-model limits cannot be expressed by an account-wide priority.
			if w.Model != "" {
				continue
			}
			if w.Exhausted || (w.RemainingPercent != nil && *w.RemainingPercent <= 0) {
				blocked = true
			}
			if w.RemainingPercent != nil && *w.RemainingPercent > 0 && w.ResetAt != nil && (r.ResetAt == nil || w.ResetAt.Before(*r.ResetAt)) {
				r.ResetAt = w.ResetAt
			}
		}
		if f.Disabled {
			r.Reason = "disabled"
			r.Priority = -20000
		} else if blocked {
			r.Reason = "quota-exhausted"
			r.Priority = -10000
			r.ResetAt = nil
		} else if r.ResetAt != nil {
			r.Reason = "soonest-reset"
			r.Priority = 1
		}
		ranked = append(ranked, r)
	}
	sort.Slice(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if a.Provider != b.Provider {
			return a.Provider < b.Provider
		}
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		if a.ResetAt != nil && b.ResetAt != nil && !a.ResetAt.Equal(*b.ResetAt) {
			return a.ResetAt.Before(*b.ResetAt)
		}
		return a.AuthID < b.AuthID
	})
	rank := 10000
	provider := ""
	for i := range ranked {
		if provider != ranked[i].Provider {
			provider = ranked[i].Provider
			rank = 10000
		}
		if ranked[i].Reason == "soonest-reset" {
			ranked[i].Priority = rank
			rank--
		}
	}
	return ranked
}
func (s *Server) reconcileRouting(ctx context.Context, files []credential, snapshot telemetry.Snapshot, now time.Time) error {
	state := routingState{Enabled: s.cfg.ManageRouting, Strategy: "soonest-reset-first", Policy: "quota-strict-v1", UpdatedAt: now, Accounts: []routingAccount{}}
	save := func() error {
		return telemetry.AtomicJSON(filepath.Join(s.cfg.DataDir, "telemetry", "routing-status.json"), state)
	}
	if !s.cfg.ManageRouting {
		state.Error = "Автоматическое управление приоритетами выключено (PROXCLI_MANAGE_ROUTING)"
		return save()
	}
	var marker struct {
		Policy          string `json:"policy"`
		SchedulerActive bool   `json:"schedulerActive"`
		HomeEnabled     bool   `json:"homeEnabled"`
	}
	if e := s.upstream(ctx, "GET", "/v8/management/proxcli/policy", nil, &marker, false); e != nil || marker.Policy != "quota-strict-v1" {
		state.Error = "Для защиты активных сессий запустите ядро ProxCli: bun run build:core, затем bun run hub"
		return save()
	}
	if marker.HomeEnabled {
		state.Error = "Home mode управляет выбором аккаунта отдельно; quota-strict не включён"
		return save()
	}
	if marker.SchedulerActive {
		state.Error = "Активен сторонний scheduler-плагин, который обходит session affinity; строгая политика не включена"
		return save()
	}
	var config struct {
		Strategy string `json:"strategy"`
		Affinity bool   `json:"session-affinity"`
		Rebind   string `json:"session-affinity-rebind-on"`
	}
	if e := s.upstream(ctx, "GET", "/v8/management/config/routing", nil, &config, false); e != nil {
		return e
	}
	if config.Strategy != "fill-first" || !config.Affinity || config.Rebind != "quota-strict" {
		state.Error = "Нужны routing.strategy=fill-first, session-affinity=true и session-affinity-rebind-on=quota-strict"
		return save()
	}
	state.Accounts = rankAccounts(files, snapshot, now)
	byID := map[string]credential{}
	for _, f := range files {
		byID[f.ID] = f
	}
	for _, account := range state.Accounts {
		f := byID[account.AuthID]
		if f.Priority == account.Priority {
			continue
		}
		if e := s.upstream(ctx, "PATCH", "/v8/management/credentials/fields", map[string]any{"name": f.Name, "priority": account.Priority}, nil, false); e != nil {
			state.Error = "Не все приоритеты обновлены; повторная попытка на следующем сборе квот"
			_ = save()
			return e
		}
		if e := s.telemetry.journal.AddDecision(telemetry.Decision{At: now, AuthID: account.AuthID, Provider: account.Provider, Reason: "priority-updated:" + account.Reason}); e != nil {
			return e
		}
	}
	state.Active = true
	return save()
}
func (s *Server) routingStatus(w http.ResponseWriter, r *http.Request) error {
	var value map[string]any
	path := filepath.Join(s.cfg.DataDir, "telemetry", "routing-status.json")
	if e := readJSONFile(path, &value); os.IsNotExist(e) {
		value = map[string]any{"active": false, "enabled": s.cfg.ManageRouting, "strategy": "soonest-reset-first", "error": "Ожидается первый сбор квот и проверка ядра"}
	} else if e != nil {
		return e
	}
	if raw, ok := value["updatedAt"].(string); ok {
		if at, e := time.Parse(time.RFC3339, raw); e != nil || time.Since(at) > 3*time.Minute {
			value["active"] = false
			value["error"] = "Статус маршрутизации устарел; проверьте соединение с хабом"
		}
	}
	observer := map[string]any{"active": false}
	if e := readJSONFile(filepath.Join(s.cfg.DataDir, "telemetry", "usage-status.json"), &observer); e == nil {
		raw, _ := observer["updatedAt"].(string)
		at, e := time.Parse(time.RFC3339, raw)
		observer["active"] = e == nil && time.Since(at) < 30*time.Second
	}
	value["usageObserver"] = observer
	jsonOut(w, 200, value)
	return nil
}
