package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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

type refreshFunc func(id string)

func (f refreshFunc) Refresh(id string) { f(id) }

func Test429RefreshesUsageAndSkipsExhaustedKey(t *testing.T) {
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
	mustUsage(t, st, a.ID, 40, now)
	mustUsage(t, st, b.ID, 40, now)

	refreshed := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.Header.Get("Authorization"), "sk-a") {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"You have reached your 5-hour Clinepass limit"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(up.Close)
	h := New(st)
	h.upstream = up.URL
	h.SetUsageRefresher(refreshFunc(func(id string) {
		_ = st.SaveAccountUsage(id, model.AccountUsage{
			SyncedAt: time.Now().Unix(),
			Rolling:  model.UsageWindow{Status: "rate-limited", UsagePercent: 100, ResetInSec: 3600},
			Weekly:   model.UsageWindow{Status: "ok", UsagePercent: 10},
			Monthly:  model.UsageWindow{Status: "ok", UsagePercent: 10},
		})
		close(refreshed)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"glm-5.3"}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	select {
	case <-refreshed:
	case <-time.After(2 * time.Second):
		t.Fatal("429 did not refresh usage")
	}

	var seen []string
	up2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(up2.Close)
	h.upstream = up2.URL
	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"glm-5.3"}`))
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	if rr2.Code != 200 {
		t.Fatalf("second status=%d", rr2.Code)
	}
	if len(seen) != 1 || seen[0] != "Bearer sk-b" {
		t.Fatalf("exhausted key still used: %v", seen)
	}
}

func Test429HoldsKeyUntilRefreshFinishes(t *testing.T) {
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
	mustUsage(t, st, a.ID, 10, now)
	mustUsage(t, st, b.ID, 10, now)

	block := make(chan struct{})
	started := make(chan struct{})
	var mu sync.Mutex
	var seen []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		mu.Lock()
		seen = append(seen, auth)
		mu.Unlock()
		if strings.HasSuffix(auth, "sk-a") {
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
	h.SetUsageRefresher(refreshFunc(func(string) {
		close(started)
		<-block
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"glm-5.3"}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d", rr.Code)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh did not start")
	}

	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"glm-5.3"}`))
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	if rr2.Code != 200 {
		t.Fatalf("second status=%d", rr2.Code)
	}
	close(block)
	mu.Lock()
	got := append([]string{}, seen...)
	mu.Unlock()
	if len(got) < 3 {
		t.Fatalf("seen %v", got)
	}
	for i := 2; i < len(got); i++ {
		if got[i] == "Bearer sk-a" {
			t.Fatalf("held key reused before refresh finished: %v", got)
		}
	}
}

func TestHTML429DoesNotFailoverOrRefresh(t *testing.T) {
	resetBalancer()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	now := time.Now().Unix()
	for _, email := range []string{"a@x.com", "b@x.com"} {
		a, err := st.CreatePaidAccount(model.CreatePaidAccountInput{Email: email, APIKey: "sk-" + email, WorkspaceID: "ws", CookieHeader: "auth=1"})
		if err != nil {
			t.Fatal(err)
		}
		mustUsage(t, st, a.ID, 1, now)
	}
	var hits int
	var refreshed atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>429</title>429 Too Many Requests`))
	}))
	t.Cleanup(up.Close)
	h := New(st)
	h.upstream = up.URL
	h.SetUsageRefresher(refreshFunc(func(string) { refreshed.Add(1) }))
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-v4-flash"}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if hits != 1 {
		t.Fatalf("html 429 must not switch accounts, hits=%d", hits)
	}
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d", rr.Code)
	}
	if refreshed.Load() != 0 {
		t.Fatal("html 429 must not refresh usage")
	}
}

func TestImageInput500DoesNotFailover(t *testing.T) {
	resetBalancer()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	now := time.Now().Unix()
	for _, email := range []string{"a@x.com", "b@x.com"} {
		a, err := st.CreatePaidAccount(model.CreatePaidAccountInput{Email: email, APIKey: "sk-" + email, WorkspaceID: "ws", CookieHeader: "auth=1"})
		if err != nil {
			t.Fatal(err)
		}
		mustUsage(t, st, a.ID, 1, now)
	}
	var hits int
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"inference request failed: failed to invoke model 'deepseek/deepseek-v4-flash' from Openrouter: request failed with status 404: {\"error\":{\"message\":\"No endpoints found that support image input\",\"code\":404}}"}}`))
	}))
	t.Cleanup(up.Close)
	h := New(st)
	h.upstream = up.URL
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-v4-flash"}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if hits != 1 {
		t.Fatalf("image 500 must not switch accounts, hits=%d", hits)
	}
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", rr.Code)
	}
	logs, err := st.ListRequestLogs(model.RequestLogFilter{}, 1, 0)
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs %v", err)
	}
	if logs[0].Retries != 0 || !strings.Contains(logs[0].Error, "image input") {
		t.Fatalf("log %+v", logs[0])
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

func TestNonStreamForcesUpstreamSSEAndAssemblesJSON(t *testing.T) {
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

	var upstreamBody []byte
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"gen_1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hel\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"gen_1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"lo\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	t.Cleanup(up.Close)
	h := New(st)
	h.upstream = up.URL
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"kimi-k3","stream":false,"messages":[{"role":"user","content":"hi"}]}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var sent map[string]any
	if json.Unmarshal(upstreamBody, &sent) != nil || sent["stream"] != true {
		t.Fatalf("upstream should be stream=true, body=%s", upstreamBody)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("content-type=%s", ct)
	}
	var obj map[string]any
	if json.Unmarshal(rr.Body.Bytes(), &obj) != nil {
		t.Fatalf("client body %s", rr.Body.String())
	}
	if obj["object"] != "chat.completion" {
		t.Fatalf("object=%v", obj["object"])
	}
	choices := obj["choices"].([]any)
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	if msg["content"] != "hello" {
		t.Fatalf("content=%v body=%s", msg["content"], rr.Body.String())
	}
}

func TestIsTransportTimeout(t *testing.T) {
	if !isTransportTimeout(fmt.Errorf("Post https://api.cline.bot/api/v1/chat/completions: http2: timeout awaiting response headers")) {
		t.Fatal("header timeout")
	}
	if !isTransportTimeout(fmt.Errorf("net/http: timeout awaiting response headers")) {
		t.Fatal("http1 header timeout")
	}
	if isTransportTimeout(fmt.Errorf("context canceled")) {
		t.Fatal("canceled is not stale conn")
	}
}

func TestNewUpstreamTransportDisablesHTTP2(t *testing.T) {
	tr := newUpstreamTransport()
	if tr.ForceAttemptHTTP2 {
		t.Fatal("http2 should be off for api.cline.bot")
	}
	if tr.ResponseHeaderTimeout != 60*time.Second {
		t.Fatalf("header timeout=%s", tr.ResponseHeaderTimeout)
	}
}

func TestTransportTimeoutDoesNotFailoverAccounts(t *testing.T) {
	resetBalancer()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CreatePaidAccount(model.CreatePaidAccountInput{Email: "a@x.com", APIKey: "sk-a", WorkspaceID: "ws", CookieHeader: "auth=1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreatePaidAccount(model.CreatePaidAccountInput{Email: "b@x.com", APIKey: "sk-b", WorkspaceID: "ws", CookieHeader: "auth=1"}); err != nil {
		t.Fatal(err)
	}
	var seen atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Add(1)
		time.Sleep(400 * time.Millisecond)
	}))
	t.Cleanup(up.Close)
	h := New(st)
	h.upstream = up.URL
	tr := &http.Transport{ResponseHeaderTimeout: 80 * time.Millisecond}
	h.transport = tr
	h.client = &http.Client{Transport: tr}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"glm-5.3"}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if n := seen.Load(); n > 2 {
		t.Fatalf("transport timeout should not walk all accounts, hits=%d", n)
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

func TestForwardUsesGlobalHTTPProxy(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(up.Close)

	var viaProxy atomic.Bool
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		viaProxy.Store(true)
		req, err := http.NewRequest(r.Method, r.URL.String(), r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		req.Header = r.Header.Clone()
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	t.Cleanup(proxySrv.Close)

	cfg, err := st.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Proxy = proxySrv.URL
	if err := st.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}

	h := New(st)
	h.upstream = up.URL
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"glm-5.3"}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !viaProxy.Load() {
		t.Fatal("forwarding did not use global proxy")
	}

	cfg.APIProxy = false
	if err := st.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	viaProxy.Store(false)
	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"glm-5.3"}`))
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	if rr2.Code != 200 {
		t.Fatalf("direct status=%d body=%s", rr2.Code, rr2.Body.String())
	}
	if viaProxy.Load() {
		t.Fatal("api_proxy off must skip global proxy")
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
