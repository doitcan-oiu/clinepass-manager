package store

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"opencode-go-manager/internal/model"
)

func openDashboardStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "dashboard.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestDashboardBucketsAndRequestUpdates(t *testing.T) {
	s := openDashboardStore(t)
	now := time.Date(2026, 9, 27, 14, 25, 0, 0, time.UTC)
	since, _, _, _ := DashboardBounds("7d", now)
	for _, at := range []time.Time{since.Add(-time.Millisecond), since, now.Add(-time.Hour), now.Add(time.Hour)} {
		if _, err := s.InsertRequestLog(model.RequestLog{CreatedAt: at.UnixMilli(), Model: "glm-5.3", Status: "error"}); err != nil {
			t.Fatal(err)
		}
	}
	rec := model.RequestLog{CreatedAt: now.Add(-time.Minute).UnixMilli(), Model: "glm-5.3", APIFormat: "openai/chat_completions", Stream: true}
	var err error
	rec.ID, err = s.InsertRequestLog(rec)
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.Dashboard("7d", now)
	if err != nil {
		t.Fatal(err)
	}
	if before.Summary.Requests != 3 || before.Summary.Processing != 1 || before.Summary.Error != 2 {
		t.Fatalf("before: %+v", before.Summary)
	}
	rec.Status, rec.InputTokens, rec.OutputTokens, rec.TotalTokens = "completed", 100, 30, 130
	rec.CacheRead, rec.DurationMS, rec.TTFTMS = 25, 2000, 500
	for i := 0; i < 2; i++ {
		if err := s.UpdateRequestLog(rec); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Dashboard("7d", now)
	if err != nil {
		t.Fatal(err)
	}
	st := got.Summary
	if st.Requests != 3 || st.Success != 1 || st.Processing != 0 || st.TotalTokens != 130 || st.CacheHitRate != 25 {
		t.Fatalf("after: %+v", st)
	}
	if st.AvgTTFTMS == nil || *st.AvgTTFTMS != 500 || st.AvgOutputTPS == nil || *st.AvgOutputTPS != 20 || st.TTFTSamples != 1 {
		t.Fatalf("timing: %+v", st)
	}
	if math.Abs(st.SuccessRate-100.0/3) > 0.001 {
		t.Fatalf("success rate %f", st.SuccessRate)
	}
	if len(got.Timeline) != 7 || got.Timeline[0].Requests != 1 || got.Timeline[6].Requests != 2 || len(got.Activity) != 180 {
		t.Fatalf("buckets: %+v", got.Timeline)
	}
	if got.Activity[179].Requests != 2 {
		t.Fatalf("today: %+v", got.Activity[179])
	}
	if st.CostUSD != nil || got.BillingAvailable {
		t.Fatal("missing billing must remain unavailable")
	}
	if len(got.Models) != 10 || got.Models[0].ID != "cline-pass/glm-5.3" {
		t.Fatalf("models: %+v", got.Models)
	}
	// Moving the final record to another protocol/model must remove the old contribution.
	rec.Model, rec.APIFormat = "kimi-k3", "openai/responses"
	if err := s.UpdateRequestLog(rec); err != nil {
		t.Fatal(err)
	}
	got, err = s.Dashboard("7d", now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Summary.Requests != 3 || got.Models[0].ID != "cline-pass/kimi-k3" {
		t.Fatalf("update: %+v", got)
	}
	for _, c := range got.Channels {
		if c.ID == "openai/chat_completions" {
			t.Fatal("empty historical channel retained")
		}
	}
}

func TestDashboardRangeAndEmpty(t *testing.T) {
	s := openDashboardStore(t)
	now := time.Date(2026, 9, 27, 0, 1, 0, 0, time.FixedZone("CST", 8*3600))
	for period, count := range map[string]int{"24h": 24, "7d": 7, "30d": 30, "90d": 90} {
		got, err := s.Dashboard(period, now)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Timeline) != count || got.Summary.Requests != 0 || got.Summary.AvgTTFTMS != nil || got.Summary.AvgOutputTPS != nil {
			t.Fatalf("%s: %+v", period, got)
		}
		if got.Timezone != "UTC" || got.Activity[179].Date != "2026-09-26" {
			t.Fatalf("UTC date: %+v", got.Activity[179])
		}
		if !got.HistoryPartial || got.BillingAvailable || got.Summary.CostUSD != nil {
			t.Fatalf("unobserved data: %+v", got)
		}
	}
	if _, err := s.Dashboard("all", now); err == nil {
		t.Fatal("invalid range accepted")
	}
	got, err := s.Dashboard("", now)
	if err != nil || got.Range != "30d" {
		t.Fatalf("default: %s %v", got.Range, err)
	}
}

func TestDashboardSurvivesRetentionAndClear(t *testing.T) {
	s := openDashboardStore(t)
	now := time.Now().UTC()
	if _, err := s.InsertRequestLog(model.RequestLog{CreatedAt: now.AddDate(0, 0, -60).UnixMilli(), Model: "glm-5.3", Status: "completed", TotalTokens: 77}); err != nil {
		t.Fatal(err)
	}
	if err := s.PruneRequestLogs(now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	logs, err := s.CountRequestLogs(model.RequestLogFilter{})
	if err != nil || logs != 0 {
		t.Fatalf("prune logs %d %v", logs, err)
	}
	got, err := s.Dashboard("90d", now)
	if err != nil || got.Summary.Requests != 1 || got.Summary.TotalTokens != 77 {
		t.Fatalf("retained %+v %v", got.Summary, err)
	}
	if _, err := s.InsertRequestLog(model.RequestLog{CreatedAt: now.UnixMilli(), Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearRequestLogs(); err != nil {
		t.Fatal(err)
	}
	if err := s.ensureDashboard(); err != nil {
		t.Fatal(err)
	}
	got, err = s.Dashboard("90d", now)
	if err != nil || got.Summary.Requests != 2 {
		t.Fatalf("clear/reopen %+v %v", got.Summary, err)
	}
	// High-volume cap must not shrink the aggregate history either.
	if _, err := s.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x < 50001)
		INSERT INTO request_logs(created_at,status) SELECT ?, 'completed' FROM n`, now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err := s.PruneRequestLogs(now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	logs, err = s.CountRequestLogs(model.RequestLogFilter{})
	if err != nil || logs != 50000 {
		t.Fatalf("cap %d %v", logs, err)
	}
	got, err = s.Dashboard("90d", now)
	if err != nil || got.Summary.Requests != 50003 {
		t.Fatalf("cap aggregate %+v %v", got.Summary, err)
	}
}

func TestDashboardBillingObservations(t *testing.T) {
	s := openDashboardStore(t)
	now := time.Now().UTC()
	account, err := s.CreatePaidAccount(model.CreatePaidAccountInput{Email: "billing@example.com", CookieHeader: "session=private", APIKey: "sk-private"})
	if err != nil {
		t.Fatal(err)
	}
	u := model.AccountUsage{SyncedAt: now.Unix(), ModelSyncedAt: now.Unix(), Days: []model.ModelDay{
		{Date: now.Format("2006-01-02"), Model: "glm-5.3", USD: 1.25},
		{Date: now.AddDate(0, 0, -1).Format("2006-01-02"), Model: "glm-5.3", USD: 2},
		{Date: now.AddDate(0, 0, -50).Format("2006-01-02"), Model: "glm-5.3", USD: 10},
	}}
	for i := 0; i < 2; i++ {
		if err := s.SaveAccountUsage(account.ID, u); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Dashboard("7d", now)
	if err != nil {
		t.Fatal(err)
	}
	if !got.BillingAvailable || got.Summary.CostUSD == nil || *got.Summary.CostUSD != 3.25 {
		t.Fatalf("billing: %+v", got)
	}
	if got.Summary.AvgRequestCostUSD != nil || got.Timeline[0].CostUSD != nil || got.Models[0].CostUSD == nil || *got.Models[0].CostUSD != 3.25 {
		t.Fatalf("billing attribution %+v", got)
	}
	got, err = s.Dashboard("24h", now)
	if err != nil || got.BillingAvailable || got.Summary.CostUSD != nil {
		t.Fatalf("24h billing %+v %v", got, err)
	}
	u.ModelSyncedAt--
	u.Days[0].USD = 99
	if err := s.SaveAccountUsage(account.ID, u); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(account.ID); err != nil {
		t.Fatal(err)
	}
	got, err = s.Dashboard("90d", now)
	if err != nil || got.Summary.CostUSD == nil || *got.Summary.CostUSD != 13.25 {
		t.Fatalf("archived billing %+v %v", got.Summary, err)
	}
}

func TestDashboardBackfillsExistingLogsOnce(t *testing.T) {
	s := openDashboardStore(t)
	now := time.Now()
	// Simulate a database created before the dashboard migration.
	if _, err := s.db.Exec(`DROP TRIGGER dashboard_log_insert; DROP TRIGGER dashboard_log_update;
		DROP TABLE dashboard_meta; DROP TABLE dashboard_hours; DROP TABLE dashboard_billing;`); err != nil {
		t.Fatal(err)
	}
	old := now.AddDate(0, 0, -3)
	if _, err := s.InsertRequestLog(model.RequestLog{CreatedAt: old.UnixMilli(), Status: "completed", TotalTokens: 42}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.ensureDashboard(); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Dashboard("30d", now)
	if err != nil || got.Summary.Requests != 1 || got.Summary.TotalTokens != 42 || got.HistorySince != old.UnixMilli() {
		t.Fatalf("backfill %+v %v", got, err)
	}
}

func TestDashboardSharedDatabaseStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared.db")
	start := make(chan struct{})
	type result struct {
		s   *Store
		err error
	}
	results := make(chan result, 2)
	for i := 0; i < 2; i++ {
		go func() { <-start; s, err := Open(path); results <- result{s, err} }()
	}
	close(start)
	var stores []*Store
	var openErrors []error
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			openErrors = append(openErrors, r.err)
			continue
		}
		stores = append(stores, r.s)
		t.Cleanup(func() { _ = r.s.Close() })
	}
	if len(openErrors) > 0 {
		t.Fatal(openErrors)
	}
	now := time.Now()
	for _, s := range stores {
		if _, err := s.InsertRequestLog(model.RequestLog{CreatedAt: now.UnixMilli(), Status: "completed"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range stores {
		got, err := s.Dashboard("24h", now)
		if err != nil || got.Summary.Requests != 2 {
			t.Fatalf("shared aggregate %+v %v", got.Summary, err)
		}
	}
}
