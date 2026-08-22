package proxy

import (
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
	if len(second) != 3 || second[2] != "Bearer sk-b" {
		t.Fatalf("cooled key a should be skipped, seen %v", second)
	}

	logs, err := st.ListRequestLogs(model.RequestLogFilter{}, 10, 0)
	if err != nil || len(logs) != 2 {
		t.Fatalf("logs %d %v", len(logs), err)
	}
	if logs[1].Retries != 1 || logs[1].AccountEmail != "b@x.com" || logs[1].Status != "completed" {
		t.Fatalf("first log %+v", logs[1])
	}
	if logs[0].Retries != 0 || logs[0].AccountEmail != "b@x.com" {
		t.Fatalf("second log %+v", logs[0])
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
