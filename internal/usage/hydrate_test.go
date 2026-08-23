package usage

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"opencode-go-manager/internal/cline"
	"opencode-go-manager/internal/model"
)

func TestHydrateWith(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users/me", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"success":true,"data":{"id":"usr-01TESTUSERID000000000000"}}`)
	})
	mux.HandleFunc("/api/v1/api-keys", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"success":true,"data":{"secret_key":"sk_newkey123"}}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	a := model.Account{CookieHeader: "cline_session_id=abc"}
	c := cline.New(a.CookieHeader, "")
	c.SetBase(srv.URL, srv.URL)
	id, err := c.DiscoverUserID()
	if err != nil {
		t.Fatal(err)
	}
	a.UserID = id
	a.WorkspaceID = id
	key, err := c.CreateAPIKey("manager")
	if err != nil {
		t.Fatal(err)
	}
	a.APIKey = key
	if a.UserID != "usr-01TESTUSERID000000000000" || a.APIKey != "sk_newkey123" {
		t.Fatalf("%+v", a)
	}
}

func TestWindowsFromPlanLimits(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	r, w, m := WindowsFromPlanLimits([]cline.PlanUsageLimit{
		{Type: "five_hour", PercentUsed: 90, ResetsAt: now.Add(time.Hour)},
		{Type: "weekly", PercentUsed: 91, ResetsAt: now.Add(2 * time.Hour)},
		{Type: "monthly", PercentUsed: 100, ResetsAt: now.Add(3 * time.Hour)},
	}, now)
	if r.UsagePercent != 90 || r.Status != "ok" || r.ResetInSec != 3600 {
		t.Fatalf("rolling %+v", r)
	}
	if w.UsagePercent != 91 || w.ResetInSec != 7200 {
		t.Fatalf("weekly %+v", w)
	}
	if m.UsagePercent != 100 || m.Status != "rate-limited" {
		t.Fatalf("monthly %+v", m)
	}
}

func TestWindowsFromDays(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	days := []model.ModelDay{
		{Date: "2026-08-21", Model: "cline-pass/glm-5.3", USD: 2},
		{Date: "2026-08-20", Model: "cline-pass/glm-5.3", USD: 8},
		{Date: "2026-07-01", Model: "cline-pass/glm-5.3", USD: 40},
	}
	r, w, m := WindowsFromDays(days, cline.Caps{RollingUSD: 10, WeeklyUSD: 25, MonthlyUSD: 50}, now)
	if r.UsagePercent != 20 || w.UsagePercent != 40 || m.UsagePercent != 20 {
		t.Fatalf("r=%v w=%v m=%v", r, w, m)
	}
}
