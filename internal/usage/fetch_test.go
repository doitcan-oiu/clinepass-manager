package usage

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"opencode-go-manager/internal/cline"
	"opencode-go-manager/internal/model"
)

func TestFetchAccountUsesPlanUsageLimits(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users/me/plan/usage-limits", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"limits":[
			{"type":"five_hour","percentUsed":90,"resetsAt":"2099-01-01T00:00:00Z"},
			{"type":"weekly","percentUsed":91,"resetsAt":"2099-01-08T00:00:00Z"},
			{"type":"monthly","percentUsed":45,"resetsAt":"2099-02-01T00:00:00Z"}
		]}}`))
	})
	mux.HandleFunc("/api/v1/users/{id}/usages/daily", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"items":[{"date":"2099-01-01","aiModelName":"glm-5.3","costUsd":9999999999}]}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	a := model.Account{CookieHeader: "cline_session_id=abc", UserID: "usr-01TESTUSERID000000000000"}
	u, subscribed, err := fetchAccount(a, "", srv.URL, srv.URL)
	if err != nil || !subscribed {
		t.Fatalf("%+v %v", u, err)
	}
	if u.Rolling.UsagePercent != 90 || u.Weekly.UsagePercent != 91 || u.Monthly.UsagePercent != 45 {
		t.Fatalf("windows %+v %+v %+v", u.Rolling, u.Weekly, u.Monthly)
	}
	if u.Rolling.ResetInSec <= 0 || u.Weekly.ResetInSec <= 0 || u.Monthly.ResetInSec <= 0 {
		t.Fatalf("reset %+v", u)
	}
}

func TestFetchAccountDailyQueryWithin31Days(t *testing.T) {
	var got url.Values
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users/me/plan/usage-limits", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"limits":[{"type":"five_hour","percentUsed":1,"resetsAt":"2099-01-01T00:00:00Z"}]}}`))
	})
	mux.HandleFunc("/api/v1/users/{id}/usages/daily", func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		_, _ = w.Write([]byte(`{"success":true,"data":{"items":[]}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	a := model.Account{
		CookieHeader: "cline_session_id=abc",
		UserID:       "usr-01TESTUSERID000000000000",
	}
	u, subscribed, err := fetchAccount(a, "", srv.URL, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !subscribed {
		t.Fatal(u)
	}
	start, _ := time.Parse("2006-01-02", got.Get("startDate"))
	end, _ := time.Parse("2006-01-02", got.Get("endDate"))
	if got.Get("startDate") == "" || got.Get("endDate") == "" {
		t.Fatalf("query=%v", got)
	}
	days := int(end.Sub(start).Hours()/24) + 1
	if days > cline.MaxDailyInclusiveDays {
		t.Fatalf("range %s..%s = %d days", got.Get("startDate"), got.Get("endDate"), days)
	}
}
