package store

import (
	"path/filepath"
	"testing"
	"time"

	"opencode-go-manager/internal/config"
	"opencode-go-manager/internal/model"
)

func TestRequestLogsAndStats(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	now := time.Now().UnixMilli()
	id, err := s.InsertRequestLog(model.RequestLog{
		CreatedAt: now - 10_000,
		Model:     "glm-5.3",
		APIFormat: "openai/chat_completions",
		Status:    "processing",
		Stream:    true,
	})
	if err != nil || id <= 0 {
		t.Fatalf("insert %d %v", id, err)
	}
	if err := s.UpdateRequestLog(model.RequestLog{
		ID:           id,
		Model:        "glm-5.3",
		APIFormat:    "openai/chat_completions",
		Stream:       true,
		AccountEmail: "a@x.com",
		Status:       "completed",
		HTTPStatus:   200,
		InputTokens:  100,
		OutputTokens: 20,
		TotalTokens:  120,
		DurationMS:   4500,
		TTFTMS:       800,
	}); err != nil {
		t.Fatal(err)
	}
	_, err = s.InsertRequestLog(model.RequestLog{
		CreatedAt:    now - 2*60_000,
		Model:        "kimi-k3",
		Status:       "error",
		HTTPStatus:   429,
		TotalTokens:  0,
		AccountEmail: "b@x.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	list, err := s.ListRequestLogs(model.RequestLogFilter{}, 10, 0)
	if err != nil || len(list) != 2 {
		t.Fatalf("list %d %v", len(list), err)
	}
	var glm model.RequestLog
	for _, l := range list {
		if l.ID == id {
			glm = l
		}
	}
	if glm.TotalTokens != 120 || !glm.Stream || glm.Status != "completed" {
		t.Fatalf("%+v", glm)
	}
	n, err := s.CountRequestLogs(model.RequestLogFilter{Model: "glm-5.3"})
	if err != nil || n != 1 {
		t.Fatalf("count %d %v", n, err)
	}
	st, err := s.RequestLogStats(now)
	if err != nil {
		t.Fatal(err)
	}
	if st.RPM1m != 1 || st.TPM1m != 120 {
		t.Fatalf("1m %+v", st)
	}
	if st.Requests1h != 2 || st.Success1h != 1 || st.Error1h != 1 {
		t.Fatalf("1h %+v", st)
	}
	if len(st.Models) != 2 {
		t.Fatalf("models %v", st.Models)
	}
	if err := s.ClearRequestLogs(); err != nil {
		t.Fatal(err)
	}
	n, err = s.CountRequestLogs(model.RequestLogFilter{})
	if err != nil || n != 0 {
		t.Fatalf("cleared %d %v", n, err)
	}
}

func TestMaxConcurrentSettings(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	st, err := s.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if st.MaxConcurrent < 1 {
		t.Fatalf("default concurrent=%d", st.MaxConcurrent)
	}
	st.MaxConcurrent = 8
	if err := s.SaveSettings(st); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSettings()
	if err != nil || got.MaxConcurrent != 8 {
		t.Fatalf("%+v %v", got, err)
	}
	if got.MaxRetries != 3 {
		t.Fatalf("default retries=%d", got.MaxRetries)
	}
	got.MaxRetries = 0
	if err := s.SaveSettings(got); err != nil {
		t.Fatal(err)
	}
	again, err := s.GetSettings()
	if err != nil || again.MaxRetries != 0 {
		t.Fatalf("zero retries %+v %v", again, err)
	}
	got.MaxRetries = 5
	if err := s.SaveSettings(got); err != nil {
		t.Fatal(err)
	}
	again, err = s.GetSettings()
	if err != nil || again.MaxRetries != 5 {
		t.Fatalf("retries %+v %v", again, err)
	}
	got.EmailSuffixBlacklist = []string{"foxcroftp.us", "@Mail.com"}
	if err := s.SaveSettings(got); err != nil {
		t.Fatal(err)
	}
	again, err = s.GetSettings()
	if err != nil || len(again.EmailSuffixBlacklist) != 2 || again.EmailSuffixBlacklist[0] != "foxcroftp.us" || again.EmailSuffixBlacklist[1] != "mail.com" {
		t.Fatalf("blacklist %+v %v", again.EmailSuffixBlacklist, err)
	}
	got.ProviderMode = "hide"
	got.ProviderValue = "OpenAI"
	if err := s.SaveSettings(got); err != nil {
		t.Fatal(err)
	}
	again, err = s.GetSettings()
	if err != nil || again.ProviderMode != "hide" || again.ProviderValue != "OpenAI" {
		t.Fatalf("provider %+v %v", again, err)
	}
	got.ProviderMode = "replace"
	if err := s.SaveSettings(got); err != nil {
		t.Fatal(err)
	}
	again, err = s.GetSettings()
	if err != nil || again.ProviderMode != "replace" {
		t.Fatalf("provider mode %+v %v", again, err)
	}
	if again.AccountRPM != 5 {
		t.Fatalf("default account rpm=%d", again.AccountRPM)
	}
	if !again.APIProxy {
		t.Fatal("default api_proxy should be on")
	}
	again.APIProxy = false
	if err := s.SaveSettings(again); err != nil {
		t.Fatal(err)
	}
	gotAPIProxy, err := s.GetSettings()
	if err != nil || gotAPIProxy.APIProxy {
		t.Fatalf("api_proxy %+v %v", gotAPIProxy, err)
	}
	again = gotAPIProxy
	again.APIProxy = true
	if err := s.SaveSettings(again); err != nil {
		t.Fatal(err)
	}
	again, err = s.GetSettings()
	if err != nil || !again.APIProxy {
		t.Fatalf("api_proxy on %+v %v", again, err)
	}
	if again.UsageRefreshSec != 60 || again.UsageRefreshConcurrency != 10 {
		t.Fatalf("usage refresh defaults %+v", again)
	}
	again.AccountRPM = 8
	if err := s.SaveSettings(again); err != nil {
		t.Fatal(err)
	}
	gotRPM, err := s.GetSettings()
	if err != nil || gotRPM.AccountRPM != 8 {
		t.Fatalf("account rpm %+v %v", gotRPM, err)
	}
	again = gotRPM
	again.UsageRefreshSec = 120
	again.UsageRefreshConcurrency = 16
	if err := s.SaveSettings(again); err != nil {
		t.Fatal(err)
	}
	gotRefresh, err := s.GetSettings()
	if err != nil || gotRefresh.UsageRefreshSec != 120 || gotRefresh.UsageRefreshConcurrency != 16 {
		t.Fatalf("usage refresh %+v %v", gotRefresh, err)
	}
	if !gotRefresh.CookieKeepEnabled || gotRefresh.CookieKeepHour != 4 {
		t.Fatalf("cookie keep default %+v", gotRefresh)
	}
	gotRefresh.CookieKeepEnabled = false
	gotRefresh.CookieKeepHour = 22
	if err := s.SaveSettings(gotRefresh); err != nil {
		t.Fatal(err)
	}
	gotKeep, err := s.GetSettings()
	if err != nil || gotKeep.CookieKeepEnabled || gotKeep.CookieKeepHour != 22 {
		t.Fatalf("cookie keep %+v %v", gotKeep, err)
	}
	if err := s.SetCookieKeepLastDate("2026-08-27"); err != nil {
		t.Fatal(err)
	}
	gotKeep, err = s.GetSettings()
	if err != nil || gotKeep.CookieKeepLastDate != "2026-08-27" {
		t.Fatalf("cookie keep date %+v %v", gotKeep, err)
	}
	again = gotRefresh
	again.CloakVersion = "151.0.7922.108.2"
	again.CloakLicenseKey = "ck_test_license"
	if err := s.SaveSettings(again); err != nil {
		t.Fatal(err)
	}
	gotCloak, err := s.GetSettings()
	if err != nil || gotCloak.CloakVersion != "151.0.7922.108.2" || gotCloak.CloakLicenseKey != "ck_test_license" {
		t.Fatalf("cloak %+v %v", gotCloak, err)
	}
	cfg := ApplySettings(config.Config{CloakVersion: "146.0", LicenseKey: "yaml-key"}, gotCloak)
	if cfg.CloakVersion != "151.0.7922.108.2" || cfg.LicenseKey != "ck_test_license" {
		t.Fatalf("apply cloak %+v", cfg)
	}
	again = gotCloak
	again.AmzKeysHost = "https://testapi.amzkeys.com"
	again.AmzKeysAppID = "2026240003167464"
	again.AmzKeysAppKey = "amz-app-key"
	again.AmzKeysPrivateKey = "amz-rsa-key"
	again.AmzKeysCardType = 467845
	again.AmzKeysCardAmount = 20
	if err := s.SaveSettings(again); err != nil {
		t.Fatal(err)
	}
	gotAmz, err := s.GetSettings()
	if err != nil || gotAmz.AmzKeysHost != "https://testapi.amzkeys.com" || gotAmz.AmzKeysAppID != "2026240003167464" || gotAmz.AmzKeysAppKey != "amz-app-key" || gotAmz.AmzKeysPrivateKey != "amz-rsa-key" || gotAmz.AmzKeysCardType != 467845 || gotAmz.AmzKeysCardAmount != 20 {
		t.Fatalf("amzkeys %+v %v", gotAmz, err)
	}
	cfg = ApplySettings(config.Config{}, gotAmz)
	if cfg.AmzKeysAppID != "2026240003167464" || cfg.AmzKeysCardType != 467845 {
		t.Fatalf("apply amzkeys %+v", cfg)
	}
	card := model.AmzKeysCard{CardNo: "4111111111111111", CVV: "123", ValidDate: "2028-12", RequestID: "r1", Amount: 20, PayCount: 1, Next: &model.AmzKeysCard{TaskID: "n1", RAM: "abcdefghijklmnop", TaskStartedAt: 2}}
	if err := s.SetAmzKeysCard(card); err != nil {
		t.Fatal(err)
	}
	gotCard, err := s.GetAmzKeysCard()
	if err != nil || !gotCard.Ready() || gotCard.CardNo != card.CardNo || gotCard.CVV != "123" || gotCard.PayCount != 1 || gotCard.Next == nil || !gotCard.Next.Pending() {
		t.Fatalf("active card %+v %v", gotCard, err)
	}
	if err := s.SaveSettings(gotAmz); err != nil {
		t.Fatal(err)
	}
	gotCard, err = s.GetAmzKeysCard()
	if err != nil || gotCard.CardNo != card.CardNo {
		t.Fatalf("card must survive settings save %+v %v", gotCard, err)
	}
	pending := model.AmzKeysCard{TaskID: "2434", RAM: "abcdefghijklmnop", TaskStartedAt: 1}
	if err := s.SetAmzKeysCard(pending); err != nil {
		t.Fatal(err)
	}
	gotPending, err := s.GetAmzKeysCard()
	if err != nil || !gotPending.Pending() || gotPending.TaskID != "2434" || gotPending.RAM != "abcdefghijklmnop" {
		t.Fatalf("pending card %+v %v", gotPending, err)
	}
	if err := s.SetAmzKeysCard(model.AmzKeysCard{}); err != nil {
		t.Fatal(err)
	}
	gotCard, err = s.GetAmzKeysCard()
	if err != nil || gotCard.Ready() {
		t.Fatalf("cleared card %+v %v", gotCard, err)
	}
}
