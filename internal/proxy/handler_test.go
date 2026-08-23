package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"opencode-go-manager/internal/model"
	"opencode-go-manager/internal/store"
)

func TestFailoverOn429ThenSucceeds(t *testing.T) {
	resetBalancer()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	a, err := st.CreatePaidAccount(model.CreatePaidAccountInput{Email: "a@x.com", APIKey: "sk-a", WorkspaceID: "ws", CookieHeader: "auth=1"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.CreatePaidAccount(model.CreatePaidAccountInput{Email: "b@x.com", APIKey: "sk-b", WorkspaceID: "ws", CookieHeader: "auth=1"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	mustUsage(t, st, a.ID, 1, now)
	mustUsage(t, st, b.ID, 40, now)

	var mu sync.Mutex
	var seen []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		mu.Lock()
		seen = append(seen, auth)
		mu.Unlock()
		if strings.HasSuffix(auth, "sk-a") {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(up.Close)

	h := New(st)
	h.upstream = up.URL

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"glm-5.3"}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if body, _ := io.ReadAll(rr.Body); string(body) != `{"ok":true}` {
		t.Fatalf("body %s", body)
	}
	mu.Lock()
	first := append([]string{}, seen...)
	mu.Unlock()
	if len(first) != 2 || first[0] != "Bearer sk-a" || first[1] != "Bearer sk-b" {
		t.Fatalf("first pass keys %v", first)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"glm-5.3"}`))
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	if rr2.Code != 200 {
		t.Fatalf("second status=%d", rr2.Code)
	}
	mu.Lock()
	second := append([]string{}, seen...)
	mu.Unlock()
	if len(second) != 4 || second[2] != "Bearer sk-a" || second[3] != "Bearer sk-b" {
		t.Fatalf("429 must not cool the same key, seen %v", second)
	}

	logs, err := st.ListRequestLogs(model.RequestLogFilter{}, 10, 0)
	if err != nil || len(logs) != 2 {
		t.Fatalf("logs %d %v", len(logs), err)
	}
	if logs[1].Retries != 1 || logs[1].AccountEmail != "b@x.com" || logs[1].Status != "completed" {
		t.Fatalf("first log %+v", logs[1])
	}
	if logs[0].Retries != 1 || logs[0].AccountEmail != "b@x.com" {
		t.Fatalf("second log %+v", logs[0])
	}
}

func TestForwardsUnwrappedClineUsage(t *testing.T) {
	resetBalancer()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	a, err := st.CreatePaidAccount(model.CreatePaidAccountInput{Email: "a@x.com", APIKey: "sk-a", WorkspaceID: "ws", CookieHeader: "auth=1"})
	if err != nil {
		t.Fatal(err)
	}
	mustUsage(t, st, a.ID, 1, time.Now().Unix())
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "9999")
		_, _ = w.Write([]byte(`{"data":{"object":"chat.completion","usage":{"prompt_tokens":20,"completion_tokens":278,"total_tokens":298}},"success":true}`))
	}))
	t.Cleanup(up.Close)
	h := New(st)
	h.upstream = up.URL
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"glm-5.3"}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d", rr.Code)
	}
	if rr.Header().Get("Content-Length") == "9999" {
		t.Fatal("stale Content-Length")
	}
	var obj map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &obj); err != nil {
		t.Fatal(err)
	}
	u, ok := obj["usage"].(map[string]any)
	if !ok {
		t.Fatalf("top-level usage missing: %s", rr.Body.String())
	}
	if int(u["prompt_tokens"].(float64)) != 20 || int(u["completion_tokens"].(float64)) != 278 {
		t.Fatalf("usage %+v", u)
	}
	logs, err := st.ListRequestLogs(model.RequestLogFilter{}, 1, 0)
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs %v", err)
	}
	if logs[0].InputTokens != 20 || logs[0].OutputTokens != 278 {
		t.Fatalf("log %+v", logs[0])
	}
}

func TestRecordsTokenUsage(t *testing.T) {
	resetBalancer()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	a, err := st.CreatePaidAccount(model.CreatePaidAccountInput{Email: "a@x.com", APIKey: "sk-a", WorkspaceID: "ws", CookieHeader: "auth=1"})
	if err != nil {
		t.Fatal(err)
	}
	mustUsage(t, st, a.ID, 1, time.Now().Unix())
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"prompt_tokens_details":{"cached_tokens":40}}}`))
	}))
	t.Cleanup(up.Close)
	h := New(st)
	h.upstream = up.URL
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"glm-5.3"}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d", rr.Code)
	}
	logs, err := st.ListRequestLogs(model.RequestLogFilter{}, 1, 0)
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs %v", err)
	}
	l := logs[0]
	if l.InputTokens != 100 || l.OutputTokens != 20 || l.TotalTokens != 120 || l.CacheRead != 40 {
		t.Fatalf("%+v", l)
	}
	if l.APIFormat != "openai/chat_completions" || l.Status != "completed" {
		t.Fatalf("%+v", l)
	}
}

func TestFailoverRespectsMaxRetries(t *testing.T) {
	resetBalancer()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cfg, err := st.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxRetries = 1
	if err := st.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	for _, email := range []string{"a@x.com", "b@x.com", "c@x.com"} {
		a, err := st.CreatePaidAccount(model.CreatePaidAccountInput{Email: email, APIKey: "sk-" + email, WorkspaceID: "ws", CookieHeader: "auth=1"})
		if err != nil {
			t.Fatal(err)
		}
		mustUsage(t, st, a.ID, 1, now)
	}
	var hits int
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate"}`))
	}))
	t.Cleanup(up.Close)
	h := New(st)
	h.upstream = up.URL
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"glm-5.3"}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if hits != 2 {
		t.Fatalf("retries=1 should try 2 keys, hits=%d status=%d", hits, rr.Code)
	}
	logs, err := st.ListRequestLogs(model.RequestLogFilter{}, 1, 0)
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs %v", err)
	}
	if logs[0].Retries != 1 {
		t.Fatalf("log retries=%d", logs[0].Retries)
	}
}

func Test429DoesNotExcludeSameAccount(t *testing.T) {
	resetBalancer()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	a, err := st.CreatePaidAccount(model.CreatePaidAccountInput{Email: "a@x.com", APIKey: "sk-a", WorkspaceID: "ws", CookieHeader: "auth=1"})
	if err != nil {
		t.Fatal(err)
	}
	mustUsage(t, st, a.ID, 40, time.Now().Unix())
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate"}`))
	}))
	t.Cleanup(up.Close)
	h := New(st)
	h.upstream = up.URL
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"glm-5.3"}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d", rr.Code)
	}
	u, err := st.GetAccountUsage(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if u.Rolling.Exhausted() {
		t.Fatalf("429 must not mark rolling full: %+v", u.Rolling)
	}
	p, err := st.GetPoolAccount(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(Rank([]model.PoolAccount{p}, "glm-5.3")) != 1 {
		t.Fatal("same account should still be pickable after 429")
	}
}

func TestRecordsUpstream400ErrorBody(t *testing.T) {
	resetBalancer()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	a, err := st.CreatePaidAccount(model.CreatePaidAccountInput{Email: "a@x.com", APIKey: "sk-a", WorkspaceID: "ws", CookieHeader: "auth=1"})
	if err != nil {
		t.Fatal(err)
	}
	mustUsage(t, st, a.ID, 1, time.Now().Unix())
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"model claude-sonnet-4 is not available","type":"invalid_request_error"}}`))
	}))
	t.Cleanup(up.Close)
	h := New(st)
	h.upstream = up.URL
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"glm-5.3"}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	logs, err := st.ListRequestLogs(model.RequestLogFilter{}, 1, 0)
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs %v", err)
	}
	l := logs[0]
	if l.Status != "error" || l.HTTPStatus != http.StatusBadRequest {
		t.Fatalf("%+v", l)
	}
	if !strings.Contains(l.Error, "model claude-sonnet-4 is not available") {
		t.Fatalf("error should keep upstream body, got %q", l.Error)
	}
	if !strings.Contains(l.Error, "400") {
		t.Fatalf("error should include HTTP status, got %q", l.Error)
	}
}

func TestFormatLogError(t *testing.T) {
	got := formatLogError(400, []byte(`{"error":{"message":"bad model"}}`))
	if got != "HTTP 400: bad model" {
		t.Fatalf("got %q", got)
	}
	got = formatLogError(422, []byte(`{"message":"validation failed"}`))
	if got != "HTTP 422: validation failed" {
		t.Fatalf("got %q", got)
	}
	got = formatLogError(502, []byte(`{"error":"upstream timeout"}`))
	if got != "HTTP 502: upstream timeout" {
		t.Fatalf("got %q", got)
	}
}

func mustUsage(t *testing.T, st *store.Store, id string, rolling float64, now int64) {
	t.Helper()
	if err := st.SaveAccountUsage(id, model.AccountUsage{
		SyncedAt: now,
		Rolling:  model.UsageWindow{Status: "ok", UsagePercent: rolling, ResetInSec: 300},
		Weekly:   model.UsageWindow{Status: "ok", UsagePercent: 10},
		Monthly:  model.UsageWindow{Status: "ok", UsagePercent: 10},
		Models:   []model.ModelSpend{{Model: "glm-5.3", USD: 1, LimitUSD: 15}},
	}); err != nil {
		t.Fatal(err)
	}
}
