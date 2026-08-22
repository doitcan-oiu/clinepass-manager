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

func TestFetchAccountDailyQueryWithin31Days(t *testing.T) {
	var got url.Values
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/plans", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
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
