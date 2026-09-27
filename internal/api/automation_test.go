package api

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"opencode-go-manager/internal/config"
	"opencode-go-manager/internal/model"
	"opencode-go-manager/internal/store"
)

func newAutoTestServer(t *testing.T, c model.AutoConnection) *Server {
	t.Helper()
	cfg := config.Config{DataDir: t.TempDir(), AutoURL: c.URL, AutoToken: c.Token, Headless: true}
	st, err := store.Open(filepath.Join(cfg.DataDir, "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SeedDefaults(cfg); err != nil {
		t.Fatal(err)
	}
	return &Server{cfg: cfg, store: st}
}

func autoTestRequest(s *Server, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
	return w
}

func TestAutoConnectionPersistsSwitchesAndHidesToken(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer saved-token" {
			t.Error("saved token missing")
		}
		_, _ = io.WriteString(w, `{"source":"new-server"}`)
	}))
	defer upstream.Close()
	s := newAutoTestServer(t, model.AutoConnection{URL: "http://127.0.0.1:1", Token: "initial-secret"})
	payload, _ := json.Marshal(map[string]any{"url": upstream.URL + "/", "token": "saved-token"})
	w := autoTestRequest(s, http.MethodPatch, "/api/auto/connection", string(payload))
	if w.Code != 200 || strings.Contains(w.Body.String(), "saved-token") || !strings.Contains(w.Body.String(), `"token_configured":true`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	w = autoTestRequest(s, http.MethodGet, "/api/jobs", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "new-server") {
		t.Fatalf("runtime change not applied: %d %s", w.Code, w.Body.String())
	}
	w = autoTestRequest(s, http.MethodPatch, "/api/auto/connection", `{"token":""}`)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	c, err := s.store.GetAutoConnection()
	if err != nil || c.Token != "saved-token" || c.URL != upstream.URL {
		t.Fatalf("%+v %v", c, err)
	}
	if err := s.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(s.cfg.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	s.store = reopened
	c, err = s.autoConnection()
	if err != nil || c.Token != "saved-token" {
		t.Fatalf("restart lost connection: %+v %v", c, err)
	}
	w = autoTestRequest(s, http.MethodPatch, "/api/auto/connection", `{"clear_token":true}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"token_configured":false`) {
		t.Fatal(w.Body.String())
	}
	c, _ = s.autoConnection()
	if c.Token != "" {
		t.Fatal("cleared token fell back to environment")
	}
}

func TestAutoConnectionRejectsCredentialURLsWithoutEcho(t *testing.T) {
	s := newAutoTestServer(t, model.AutoConnection{})
	for _, u := range []string{"ftp://host", "https://user:private-password@host", "https://host?token=private-password", "https://host#private-password", "http://"} {
		payload, _ := json.Marshal(map[string]string{"url": u})
		w := autoTestRequest(s, http.MethodPatch, "/api/auto/connection", string(payload))
		if w.Code != 400 || strings.Contains(w.Body.String(), "private-password") {
			t.Fatalf("%q: %d %s", u, w.Code, w.Body.String())
		}
	}
}

func TestAutomationForwardingPreservesRequestAndIsolatesCredentials(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || r.URL.RequestURI() != "/base/api/accounts/example/refresh?retry=1" || string(body) != `{"auto_pay":true}` {
			t.Errorf("unexpected request %s %s %s", r.Method, r.URL, body)
		}
		if r.Header.Get("Authorization") != "Bearer service-token" {
			t.Error("wrong remote token")
		}
		for _, h := range []string{"Cookie", "X-Api-Key", "Proxy-Authorization", "X-Forwarded-For", "Forwarded", "Origin"} {
			if r.Header.Get(h) != "" {
				t.Errorf("leaked header %s", h)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "remote=private")
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"id":"job-1"}`)
	}))
	defer upstream.Close()
	s := newAutoTestServer(t, model.AutoConnection{URL: upstream.URL + "/base", Token: "service-token"})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/accounts/example/refresh?retry=1", strings.NewReader(`{"auto_pay":true}`))
	for _, h := range []string{"Authorization", "Cookie", "X-Api-Key", "Proxy-Authorization", "X-Forwarded-For", "Forwarded", "Origin"} {
		r.Header.Set(h, "browser-private")
	}
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusAccepted || w.Body.String() != `{"id":"job-1"}` || w.Header().Get("Set-Cookie") != "" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestAutomationEventsStreamWithoutWaitingForCompletion(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer stream-token" {
			t.Error("missing stream auth")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer upstream.Close()
	s := newAutoTestServer(t, model.AutoConnection{URL: upstream.URL, Token: "stream-token"})
	gateway := httptest.NewServer(s.Handler())
	defer gateway.Close()
	client := &http.Client{Timeout: 2 * time.Second}
	res, err := client.Get(gateway.URL + "/api/jobs/job-1/events")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	line, err := bufio.NewReader(res.Body).ReadString('\n')
	if err != nil || line != "data: first\n" {
		t.Fatalf("line=%q error=%v", line, err)
	}
}

func TestManagerWorksAndSavesSettingsWithAutoOffline(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	s := newAutoTestServer(t, model.AutoConnection{URL: closed.URL, Token: "private-token"})
	for _, path := range []string{"/api/health", "/api/config", "/api/auto/status"} {
		w := autoTestRequest(s, http.MethodGet, path, "")
		if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "private-token") {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	w := autoTestRequest(s, http.MethodGet, "/api/jobs", "")
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "自动化服务未连接") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	w = autoTestRequest(s, http.MethodPatch, "/api/config", `{"account_rpm":17}`)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	settings, err := s.store.GetSettings()
	if err != nil || settings.AccountRPM != 17 {
		t.Fatalf("settings=%+v err=%v", settings, err)
	}
	w = autoTestRequest(s, http.MethodPatch, "/api/config", `{"account_rpm":19,"headless":false}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "/api/auto/config") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	settings, _ = s.store.GetSettings()
	if settings.AccountRPM != 17 {
		t.Fatal("mixed automation request partially persisted")
	}
	out := s.publicConfig()
	for _, field := range []string{"headless", "hero_sms_api_key", "amzkeys_private_key", "cookie_keep_enabled", "max_concurrent"} {
		if _, exists := out[field]; exists {
			t.Errorf("main config exposed automation setting %s", field)
		}
	}
}

func TestAutoTestUsesCandidateAuthWithoutSaving(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer candidate-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "candidate-token")
			return
		}
		_, _ = io.WriteString(w, `{"ok":true,"service":"auto","protocol_version":1}`)
	}))
	defer upstream.Close()
	s := newAutoTestServer(t, model.AutoConnection{URL: upstream.URL, Token: "old-token"})
	w := autoTestRequest(s, http.MethodPost, "/api/auto/test", `{"token":"candidate-token"}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"connected":true`) || strings.Contains(w.Body.String(), "candidate-token") {
		t.Fatal(w.Body.String())
	}
	c, _ := s.autoConnection()
	if c.Token != "old-token" {
		t.Fatal("test unexpectedly saved candidate")
	}
	w = autoTestRequest(s, http.MethodGet, "/api/auto/status", "")
	if !strings.Contains(w.Body.String(), `"connected":false`) || !strings.Contains(w.Body.String(), "身份验证失败") || strings.Contains(w.Body.String(), "old-token") {
		t.Fatal(w.Body.String())
	}
	w = autoTestRequest(s, http.MethodGet, "/api/jobs", "")
	if w.Code != http.StatusBadGateway || strings.Contains(w.Body.String(), "candidate-token") || !strings.Contains(w.Body.String(), "身份验证失败") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestAutoStatusRequiresAutoIdentity(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true,"service":"manager"}`)
	}))
	defer upstream.Close()
	s := newAutoTestServer(t, model.AutoConnection{URL: upstream.URL, Token: "some-token"})
	w := autoTestRequest(s, http.MethodGet, "/api/auto/status", "")
	if !strings.Contains(w.Body.String(), `"connected":false`) || !strings.Contains(w.Body.String(), "不是兼容") {
		t.Fatal(w.Body.String())
	}
}

func TestAutoRequestsRejectRedirectWithoutLeakingToken(t *testing.T) {
	leaked := false
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true }))
	defer other.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer upstream.Close()
	s := newAutoTestServer(t, model.AutoConnection{URL: upstream.URL, Token: "private-token"})
	res, err := s.autoRequest(context.Background(), http.MethodGet, "/api/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusTemporaryRedirect || leaked {
		t.Fatalf("status %d leak %t", res.StatusCode, leaked)
	}
	w := autoTestRequest(s, http.MethodGet, "/api/jobs", "")
	if w.Code != http.StatusBadGateway || w.Header().Get("Location") != "" || leaked {
		t.Fatalf("%d %s leak=%t", w.Code, w.Body.String(), leaked)
	}
}

func TestAutoConfigAndAccountRoutesUseRemoteStore(t *testing.T) {
	var seen string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Method + " " + r.URL.Path
		_, _ = io.WriteString(w, `{"headless":false}`)
	}))
	defer upstream.Close()
	s := newAutoTestServer(t, model.AutoConnection{URL: upstream.URL, Token: "token"})
	for _, tc := range []struct{ method, path, want string }{
		{http.MethodGet, "/api/auto/config", "GET /api/config"},
		{http.MethodPatch, "/api/auto/config", "PATCH /api/config"},
		{http.MethodGet, "/api/auto/accounts/example", "GET /api/accounts/example"},
		{http.MethodDelete, "/api/auto/accounts/example", "DELETE /api/accounts/example"},
		{http.MethodPost, "/api/auto/accounts/example/login", "POST /api/accounts/example/login"},
		{http.MethodPost, "/api/auto/accounts/example/refresh", "POST /api/accounts/example/refresh"},
		{http.MethodPost, "/api/batches/example/paid", "POST /api/batches/example/paid"},
		{http.MethodGet, "/api/auto/usage/sync", "GET /api/usage/sync"},
	} {
		w := autoTestRequest(s, tc.method, tc.path, "{}")
		if w.Code != 200 || seen != tc.want {
			t.Fatalf("%s: %d %s seen=%s", tc.path, w.Code, w.Body.String(), seen)
		}
	}
	settings, _ := s.store.GetSettings()
	if !settings.Headless {
		t.Fatal("remote configuration modified manager settings")
	}
}

func TestAutoProxyRedactsTokenFromUpstreamErrors(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":"unexpected Bearer private-token"}`)
	}))
	defer upstream.Close()
	s := newAutoTestServer(t, model.AutoConnection{URL: upstream.URL, Token: "private-token"})
	w := autoTestRequest(s, http.MethodGet, "/api/jobs", "")
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "private-token") || !strings.Contains(w.Body.String(), "[redacted]") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestAutoRequestTimeoutIsActionableAndRedacted(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer upstream.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	_, err := autoRequestConnection(ctx, http.MethodGet, "/api/health", nil, model.AutoConnection{URL: upstream.URL, Token: "private-token"})
	if err == nil || !strings.Contains(err.Error(), "超时") || strings.Contains(err.Error(), "private-token") {
		t.Fatalf("error = %v", err)
	}
}

func TestAutoStatusRejectsLegacyUnauthenticatedService(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `{"ok":true,"service":"auto"}`) }))
	defer upstream.Close()
	s := newAutoTestServer(t, model.AutoConnection{URL: upstream.URL, Token: "private-token"})
	w := autoTestRequest(s, http.MethodGet, "/api/auto/status", "")
	if !strings.Contains(w.Body.String(), `"connected":false`) || !strings.Contains(w.Body.String(), "不是兼容") {
		t.Fatal(w.Body.String())
	}
}
