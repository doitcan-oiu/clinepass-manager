package cline

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParsePlansCaps(t *testing.T) {
	raw := []byte(`{
	  "data": [{
	    "interval": "Monthly",
	    "entitlements": {
	      "cline_pass": {
	        "inferenceCapThreshold": {
	          "last5HoursUsageCostUSDPerUser": 1000000000,
	          "last7daysUsageCostUSDPerUser": 2500000000,
	          "last30daysUsageCostUSDPerUser": 5000000000
	        }
	      }
	    }
	  }],
	  "success": true
	}`)
	got := ParsePlansCaps(raw)
	if got.RollingUSD != 10 || got.WeeklyUSD != 25 || got.MonthlyUSD != 50 {
		t.Fatalf("%+v", got)
	}
}

func TestParseDailyUsages(t *testing.T) {
	raw := []byte(`{"data":{"items":[
		{"date":"2026-08-21","aiModelName":"cline-pass/glm-5.3","costUsd":291419},
		{"date":"2026-08-21","aiModelName":"cline-pass/qwen3.8-max","costUsd":1178845131}
	]},"success":true}`)
	got, err := ParseDailyUsages(raw)
	if err != nil || len(got) != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	if UnitsToUSD(got[0].CostUnits) < 0.002 || UnitsToUSD(got[0].CostUnits) > 0.003 {
		t.Fatalf("usd=%v", UnitsToUSD(got[0].CostUnits))
	}
	if UnitsToUSD(got[1].CostUnits) < 11.7 || UnitsToUSD(got[1].CostUnits) > 11.8 {
		t.Fatalf("usd=%v", UnitsToUSD(got[1].CostUnits))
	}
}

func TestUserIDFromCookie(t *testing.T) {
	raw := `ph_phc_xxx_posthog=%7B%22distinct_id%22%3A%22usr-01M0GXFC8R7EM787K2E3KTEE2K%22%7D`
	if got := UserIDFromCookie(raw); got != "usr-01M0GXFC8R7EM787K2E3KTEE2K" {
		t.Fatalf("%s", got)
	}
}

func TestUserIDFromProfile(t *testing.T) {
	got := UserIDFromProfile([]byte(`{"success":true,"data":{"id":"usr-01PROFILEID00000000000000","email":"a@x.com"}}`))
	if got != "usr-01PROFILEID00000000000000" {
		t.Fatalf("%s", got)
	}
	got = UserIDFromProfile([]byte(`{"id":"usr-01FLATID00000000000000000"}`))
	if got != "usr-01FLATID00000000000000000" {
		t.Fatalf("%s", got)
	}
}

func TestDailyWindowInclusive31Days(t *testing.T) {
	now := time.Date(2026, 8, 22, 15, 4, 0, 0, time.FixedZone("CST", 8*3600))
	start, end := DailyWindow(now)
	if start.Format("2006-01-02") != "2026-07-23" {
		t.Fatalf("start=%s", start.Format("2006-01-02"))
	}
	if end.Format("2006-01-02") != "2026-08-22" {
		t.Fatalf("end=%s", end.Format("2006-01-02"))
	}
	days := int(end.Sub(start).Hours()/24) + 1
	if days != MaxDailyInclusiveDays {
		t.Fatalf("days=%d", days)
	}
}

func TestDiscoverUserIDFromUsersMe(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users/me", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"success":true,"data":{"id":"usr-01TESTUSERID000000000000"}}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New("cline_session_id=abc", "")
	c.SetBase(srv.URL, srv.URL)
	id, err := c.DiscoverUserID()
	if err != nil {
		t.Fatal(err)
	}
	if id != "usr-01TESTUSERID000000000000" {
		t.Fatalf("%s", id)
	}
}

func TestDailyUsagesIncludesAPIErrorBody(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users/{id}/usages/daily", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"success":false,"error":"date range must not exceed 31 days"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New("cline_session_id=abc", "")
	c.SetBase(srv.URL, srv.URL)
	_, err := c.DailyUsages("usr-01TESTUSERID000000000000", time.Now().AddDate(0, 0, -40), time.Now())
	if err == nil || !strings.Contains(err.Error(), "31 days") {
		t.Fatalf("err=%v", err)
	}
}
