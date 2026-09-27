package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"opencode-go-manager/internal/model"
	"opencode-go-manager/internal/store"
)

func TestDashboardRangeAndResources(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "dashboard.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	s := &Server{store: st}
	bad := httptest.NewRecorder()
	s.dashboard(bad, httptest.NewRequest(http.MethodGet, "/api/dashboard?range=all", nil))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid range status %d", bad.Code)
	}
	for _, input := range []model.CreatePaidAccountInput{
		{Email: "one@example.com", APIKey: "private-key-one", CookieHeader: "private-cookie"},
		{Email: "two@example.com", APIKey: "private-key-one", CookieHeader: "private-cookie"},
		{Email: "three@example.com", APIKey: "private-key-two", CookieHeader: "private-cookie"},
		{Email: "four@example.com", CookieHeader: "private-cookie"},
	} {
		a, err := st.CreatePaidAccount(input)
		if err != nil {
			t.Fatal(err)
		}
		if input.Email == "three@example.com" {
			if err := st.SaveAccountUsage(a.ID, model.AccountUsage{SyncedAt: time.Now().Unix(), Monthly: model.UsageWindow{Status: "exhausted"}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	w := httptest.NewRecorder()
	s.dashboard(w, httptest.NewRequest(http.MethodGet, "/api/dashboard?range=30d", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "private-") || strings.Contains(w.Body.String(), "@example.com") {
		t.Fatal("dashboard exposed credentials or account emails")
	}
	var got model.Dashboard
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	a := got.Availability
	if got.Summary.Accounts != 4 || a.AccountsTotal != 4 || a.AccountsAvailable != 2 || a.AccountsUnavailable != 2 || a.KeysTotal != 2 || a.KeysAvailable != 1 || a.ModelsTotal == 0 || a.ModelsEnabled != a.ModelsTotal {
		t.Fatalf("availability: %+v", a)
	}
	if got.Summary.Requests != 0 || got.Summary.CostUSD != nil || got.Summary.AvgOutputTPS != nil {
		t.Fatalf("empty metrics: %+v", got.Summary)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("dashboard caching enabled")
	}
}
