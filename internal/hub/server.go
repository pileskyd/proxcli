package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Upstream, ManagementKey, AccessKey, DataDir, WebDir string
	ManageRouting                                       bool
}
type Server struct {
	cfg       Config
	client    *http.Client
	store     *store
	telemetry *collector
}
type apiError struct {
	code    int
	message string
}

func (e *apiError) Error() string     { return e.message }
func fail(code int, msg string) error { return &apiError{code, msg} }
func New(cfg Config) (*Server, error) {
	u, e := url.Parse(cfg.Upstream)
	if e != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("PROXCLI_UPSTREAM должен быть корневым http(s) URL без credentials")
	}
	cfg.Upstream = strings.TrimRight(cfg.Upstream, "/")
	st, e := openStore(cfg.DataDir)
	if e != nil {
		return nil, e
	}
	// Management traffic must never inherit an outbound account proxy or redirect with credentials.
	telemetry, e := newCollector(cfg.DataDir)
	if e != nil {
		return nil, e
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Server{cfg: cfg, store: st, telemetry: telemetry, client: &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func jsonOut(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return fail(415, "Ожидается application/json")
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return fail(400, "Неверное тело JSON")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return fail(400, "Ожидается один JSON объект")
	}
	return nil
}
func (s *Server) upstream(ctx context.Context, method, path string, body any, result any, access bool) error {
	key := s.cfg.ManagementKey
	if access {
		key = s.cfg.AccessKey
	}
	if key == "" {
		if access {
			return fail(503, "Задайте PROXCLI_ACCESS_KEY для загрузки моделей")
		}
		return fail(503, "Задайте PROXCLI_MANAGEMENT_KEY в окружении сервера")
	}
	var b io.Reader
	if body != nil {
		data, e := json.Marshal(body)
		if e != nil {
			return e
		}
		b = bytes.NewReader(data)
	}
	req, e := http.NewRequestWithContext(ctx, method, s.cfg.Upstream+path, b)
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, e := s.client.Do(req)
	if e != nil {
		return fail(502, "CLIProxyAPI недоступен: проверьте адрес и запущен ли сервис")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		// Never relay upstream bodies: they can contain proxy credentials or OAuth codes.
		switch res.StatusCode {
		case 401, 403:
			return fail(502, "CLIProxyAPI отклонил ключ доступа к API")
		case 404:
			return fail(502, "Endpoint CLIProxyAPI не найден: требуется v8 и включённый Management API")
		case 400:
			return fail(400, "CLIProxyAPI отклонил параметры запроса")
		case 409:
			return fail(409, "CLIProxyAPI сообщил о конфликте состояния; обновите данные")
		}
		return fail(502, fmt.Sprintf("CLIProxyAPI вернул HTTP %d", res.StatusCode))
	}
	if result == nil {
		_, e = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		return e
	}
	if e = json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(result); e != nil {
		return fail(502, "CLIProxyAPI вернул некорректный JSON")
	}
	return nil
}

type credential struct {
	Priority  int    `json:"priority"`
	ID        string `json:"id"`
	AuthIndex string `json:"auth_index"`
	AccountID string `json:"account_id"`
	IDToken   struct {
		AccountID string `json:"chatgpt_account_id"`
	} `json:"id_token"`
	Quota    quotaObservation `json:"quota"`
	Name     string           `json:"name"`
	Provider string           `json:"provider"`
	Type     string           `json:"type"`
	Email    string           `json:"email"`
	Status   string           `json:"status"`
	Disabled bool             `json:"disabled"`
	Prefix   string           `json:"prefix"`
}

func (s *Server) credentials(ctx context.Context) ([]credential, error) {
	var resp struct {
		Files []credential `json:"files"`
	}
	e := s.upstream(ctx, "GET", "/v8/management/credentials", nil, &resp, false)
	return resp.Files, e
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	register := func(pattern string, handler func(http.ResponseWriter, *http.Request) error) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if e := handler(w, r); e != nil {
				var ae *apiError
				if errors.As(e, &ae) {
					jsonOut(w, ae.code, map[string]string{"error": ae.message})
				} else {
					jsonOut(w, 500, map[string]string{"error": "Не удалось сохранить настройки ProxCli; проверьте доступ к каталогу данных"})
				}
			}
		})
	}
	register("GET /api/status", s.status)
	register("GET /api/quotas", s.quotas)
	register("POST /api/telemetry/refresh", s.refreshTelemetry)
	register("GET /api/history", s.history)
	register("GET /api/routing", s.routingStatus)
	register("GET /api/subscriptions", s.subscriptions)
	register("PATCH /api/subscriptions", s.patch)
	register("PATCH /api/subscriptions/status", s.toggle)
	register("POST /api/oauth/start", s.oauthStart)
	register("GET /api/oauth/status", s.oauthStatus)
	register("POST /api/oauth/callback", s.oauthCallback)
	register("GET /api/models", func(w http.ResponseWriter, r *http.Request) error {
		var result any
		if e := s.upstream(r.Context(), "GET", "/v1/models", nil, &result, true); e != nil {
			return e
		}
		jsonOut(w, 200, result)
		return nil
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, 404, map[string]string{"error": "Endpoint не найден"})
	})
	mux.HandleFunc("/", s.static)
	return s.guard(mux)
}
func (s *Server) status(w http.ResponseWriter, r *http.Request) error {
	_, e := s.credentials(r.Context())
	body := map[string]any{"connected": e == nil, "upstream": s.cfg.Upstream}
	if e != nil {
		body["error"] = e.Error()
	}
	jsonOut(w, 200, body)
	return nil
}
func (s *Server) subscriptions(w http.ResponseWriter, r *http.Request) error {
	files, e := s.credentials(r.Context())
	if e != nil {
		return e
	}
	var global map[string]string
	if e = s.upstream(r.Context(), "GET", "/v0/management/proxy-url", nil, &global, false); e != nil {
		return e
	}
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	list := make([]map[string]any, 0, len(files))
	for _, f := range files {
		p, known := s.store.state.Routes[f.ID]
		known = known && p.Confirmed
		if p.Mode == "" {
			p.Mode = "inherit"
		}
		provider := f.Provider
		if provider == "" {
			provider = f.Type
		}
		list = append(list, map[string]any{"id": f.ID, "name": f.Name, "provider": provider, "email": f.Email, "status": f.Status, "disabled": f.Disabled, "prefix": p.Prefix, "proxyMode": p.Mode, "proxyUrl": maskProxy(p.URL), "proxyConfigured": p.Mode == "custom" && known, "proxyKnown": known})
	}
	jsonOut(w, 200, map[string]any{"subscriptions": list, "defaultProxy": maskProxy(global["proxy-url"])})
	return nil
}
func (s *Server) patch(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Name   string  `json:"name"`
		Mode   string  `json:"proxyMode"`
		URL    *string `json:"proxyUrl"`
		Prefix *string `json:"prefix"`
	}
	if e := decode(w, r, &req); e != nil {
		return e
	}
	if req.Name == "" {
		return fail(400, "Укажите подписку")
	}
	if req.Prefix != nil && (len(*req.Prefix) > 128 || strings.ContainsAny(*req.Prefix, "/\\ \t\r\n")) {
		return fail(400, "Префикс должен быть до 128 символов, без пробелов и слешей")
	}
	files, e := s.credentials(r.Context())
	if e != nil {
		return e
	}
	var id string
	for _, f := range files {
		if f.Name == req.Name {
			id = f.ID
			break
		}
	}
	if id == "" {
		return fail(404, "Подписка не найдена")
	}
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	old := s.store.state.Routes[id]
	value, e := proxyValue(req.Mode, req.URL, old)
	if e != nil {
		return fail(400, e.Error())
	}
	// Persist an uncertain marker before changing upstream. A crash can never display an uncommitted route as confirmed.
	prefix := old.Prefix
	if req.Prefix != nil {
		prefix = *req.Prefix
	}
	s.store.state.Routes[id] = route{Mode: req.Mode, URL: value, Prefix: prefix}
	if e = s.store.save(); e != nil {
		s.store.state.Routes[id] = old
		return e
	}
	body := map[string]any{"name": req.Name, "proxy_url": value}
	if req.Prefix != nil {
		body["prefix"] = *req.Prefix
	}
	if e = s.upstream(r.Context(), "PATCH", "/v8/management/credentials/fields", body, nil, false); e != nil {
		return e
	}
	s.store.state.Routes[id] = route{Mode: req.Mode, URL: value, Prefix: prefix, Confirmed: true}
	if e = s.store.save(); e != nil {
		s.store.state.Routes[id] = route{}
		return e
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
	return nil
}
func (s *Server) toggle(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Name     string `json:"name"`
		Disabled *bool  `json:"disabled"`
	}
	if e := decode(w, r, &req); e != nil {
		return e
	}
	if req.Name == "" || req.Disabled == nil {
		return fail(400, "Нужны name и disabled")
	}
	if e := s.upstream(r.Context(), "PATCH", "/v8/management/credentials/status", req, nil, false); e != nil {
		return e
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
	return nil
}
func providerID(v string) string {
	switch v {
	case "claude":
		return "anthropic"
	case "codex":
		return "codex"
	}
	return ""
}
func (s *Server) oauthStart(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Provider string  `json:"provider"`
		Mode     string  `json:"proxyMode"`
		URL      *string `json:"proxyUrl"`
	}
	if e := decode(w, r, &req); e != nil {
		return e
	}
	p := providerID(req.Provider)
	if p == "" {
		return fail(400, "Выберите Claude или Codex")
	}
	// OAuth v8 rejects the direct sentinel accepted by credential PATCH.
	if req.Mode == "direct" {
		return fail(409, "CLIProxyAPI v8 не поддерживает явный direct при OAuth-входе. Выберите собственный прокси или общий маршрут хаба. Режим direct можно задать подписке после входа.")
	}
	value, e := proxyValue(req.Mode, req.URL, route{})
	if e != nil {
		return fail(400, e.Error())
	}
	var result struct {
		URL   string `json:"url"`
		State string `json:"state"`
	}
	if e = s.upstream(r.Context(), "POST", "/v8/management/oauth/auth-url", map[string]any{"provider": p, "proxy_url": value, "is_webui": true}, &result, false); e != nil {
		return e
	}
	u, e := url.Parse(result.URL)
	if e != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || result.State == "" {
		return fail(502, "CLIProxyAPI не вернул ссылку OAuth")
	}
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	s.store.state.Logins[result.State] = login{Route: route{Mode: req.Mode, URL: value, Confirmed: true}, Provider: req.Provider, Created: time.Now()}
	if e = s.store.save(); e != nil {
		return e
	}
	jsonOut(w, 200, result)
	return nil
}
func (s *Server) oauthStatus(w http.ResponseWriter, r *http.Request) error {
	stateID := r.URL.Query().Get("state")
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	session, ok := s.store.state.Logins[stateID]
	if !ok || time.Since(session.Created) > 15*time.Minute {
		return fail(404, "OAuth сессия неизвестна или истекла; начните вход заново")
	}
	var result struct {
		Status  string `json:"status"`
		Error   string `json:"error"`
		AuthID  string `json:"auth_id"`
		Saved   bool   `json:"saved"`
		Adopted *bool  `json:"adopted"`
	}
	if e := s.upstream(r.Context(), "GET", "/v8/management/oauth/status?state="+url.QueryEscape(stateID), nil, &result, false); e != nil {
		return e
	}
	if result.Status == "ok" {
		if result.AuthID == "" || !result.Saved {
			return fail(502, "Хаб завершил OAuth, но не подтвердил сохранение подписки")
		}
		if result.Adopted != nil && !*result.Adopted {
			return fail(409, "Подписка сохранена, но не активирована ядром CLIProxyAPI; проверьте хаб")
		}
		savedRoute := session.Route
		savedRoute.Prefix = s.store.state.Routes[result.AuthID].Prefix
		s.store.state.Routes[result.AuthID] = savedRoute
		if e := s.store.save(); e != nil {
			return e
		}
	}
	body := map[string]string{"status": result.Status}
	if result.Error != "" {
		body["error"] = "Хаб не завершил OAuth. Проверьте прокси и повторите вход; подробности — в журнале CLIProxyAPI"
	}
	jsonOut(w, 200, body)
	return nil
}
func (s *Server) oauthCallback(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Provider string `json:"provider"`
		URL      string `json:"url"`
	}
	if e := decode(w, r, &req); e != nil {
		return e
	}
	u, e := url.Parse(req.URL)
	if e != nil || u.Scheme != "http" || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") || u.Query().Get("code") == "" || u.Query().Get("state") == "" {
		return fail(400, "Нужен полный localhost callback URL с code и state")
	}
	s.store.mu.Lock()
	session, ok := s.store.state.Logins[u.Query().Get("state")]
	s.store.mu.Unlock()
	if !ok || session.Provider != req.Provider || time.Since(session.Created) > 15*time.Minute {
		return fail(400, "Callback не соответствует начатому входу")
	}
	if e = s.upstream(r.Context(), "POST", "/v8/management/oauth/callback", map[string]string{"provider": providerID(req.Provider), "redirect_url": req.URL}, nil, false); e != nil {
		return e
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
	return nil
}
func localHost(host string) bool { return host == "localhost" || host == "127.0.0.1" || host == "::1" }
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		host, _, e := net.SplitHostPort(r.Host)
		if e != nil {
			host = r.Host
		}
		if !localHost(host) {
			jsonOut(w, 403, map[string]string{"error": "Панель доступна только через localhost"})
			return
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" {
			jsonOut(w, 403, map[string]string{"error": "Запрос с другого сайта запрещён"})
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, e := url.Parse(origin)
			if e != nil || u.Scheme != "http" || !localHost(u.Hostname()) || (u.Host != r.Host && u.Port() != "5173") {
				jsonOut(w, 403, map[string]string{"error": "Origin запрещён"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(405)
		return
	}
	clean := filepath.Clean("/" + r.URL.Path)
	path := filepath.Join(s.cfg.WebDir, clean)
	info, e := os.Stat(path)
	if e != nil || info.IsDir() {
		if filepath.Ext(clean) != "" {
			http.NotFound(w, r)
			return
		}
		path = filepath.Join(s.cfg.WebDir, "index.html")
	}
	if _, e = os.Stat(path); e != nil {
		http.Error(w, "Соберите UI: cd web && bun install && bun run build", 503)
		return
	}
	http.ServeFile(w, r, path)
}
