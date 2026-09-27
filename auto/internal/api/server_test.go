package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"opencode-go-manager/auto/internal/job"
	"opencode-go-manager/internal/config"
	"opencode-go-manager/internal/model"
	"opencode-go-manager/internal/store"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	cfg := config.Config{DataDir: t.TempDir(), MaxConcurrent: 1, AutoToken: "test-auto-token"}
	autoStore, err := store.Open(cfg.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = autoStore.Close() })
	if err := autoStore.SeedDefaults(cfg); err != nil {
		t.Fatal(err)
	}
	return New(cfg, autoStore, job.New(cfg, autoStore))
}

func request(s *Server, method, path, body, token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	s.Handler().ServeHTTP(w, r)
	return w
}

func TestStandaloneRoutesKeepIndependentDatabase(t *testing.T) {
	s := testServer(t)
	managerCfg := config.Config{DataDir: t.TempDir()}
	managerStore, err := store.Open(managerCfg.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer managerStore.Close()
	w := request(s, http.MethodPost, "/api/batches", `{"name":"split","text":"split@example.com----password----recovery@example.com"}`, s.cfg.AutoToken)
	if w.Code != http.StatusCreated {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	count, err := managerStore.CountBatches()
	if err != nil || count != 0 {
		t.Fatalf("manager must have independent batches count=%d err=%v", count, err)
	}
	count, err = s.store.CountBatches()
	if err != nil || count != 1 {
		t.Fatalf("auto batch count=%d err=%v", count, err)
	}
	w = request(s, http.MethodGet, "/api/jobs", "", s.cfg.AutoToken)
	var jobs []any
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &jobs) != nil || len(jobs) != 0 {
		t.Fatalf("constructor scheduled work: %d %s", w.Code, w.Body.String())
	}
	w = request(s, http.MethodGet, "/v1/models", "", s.cfg.AutoToken)
	if w.Code != http.StatusNotFound {
		t.Fatalf("auto must not serve proxy: %d", w.Code)
	}
}

func TestAllAutoRoutesRequireToken(t *testing.T) {
	s := testServer(t)
	for _, token := range []string{"", "incorrect", s.cfg.AutoToken + "extra"} {
		for _, path := range []string{"/api/health", "/api/results", "/api/config", "/api/accounts/unknown"} {
			w := request(s, http.MethodGet, path, "", token)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("%s token %q: %d", path, token, w.Code)
			}
		}
	}
	w := request(s, http.MethodPost, "/api/batches", `{"name":"unauthorized"}`, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized mutation: %d", w.Code)
	}
	count, _ := s.store.CountBatches()
	if count != 0 {
		t.Fatal("unauthenticated mutation persisted")
	}
	w = request(s, http.MethodGet, "/api/health", "", s.cfg.AutoToken)
	var health map[string]any
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &health) != nil || health["service"] != "auto" || health["protocol_version"] != float64(1) {
		t.Fatalf("health: %d %s", w.Code, w.Body.String())
	}
	s.cfg.AutoToken = ""
	w = request(s, http.MethodGet, "/api/health", "", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatal("missing server token must fail closed")
	}
}

func TestRemoteConfigMasksSecretsAndRejectsManagerSettings(t *testing.T) {
	s := testServer(t)
	w := request(s, http.MethodPatch, "/api/config", `{"hero_sms_api_key":"very-secret-sms-value","max_concurrent":3,"proxy":"socks5://auto-proxy:1080"}`, s.cfg.AutoToken)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "very-secret-sms-value") || !strings.Contains(w.Body.String(), "********") {
		t.Fatal("secret was not masked")
	}
	w = request(s, http.MethodPatch, "/api/config", `{"hero_sms_api_key":"very********alue","cookie_keep_hour":7}`, s.cfg.AutoToken)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	st, _ := s.store.GetSettings()
	if st.HeroSMSAPIKey != "very-secret-sms-value" || st.CookieKeepHour != 7 || st.MaxConcurrent != 3 {
		t.Fatalf("settings not persisted: %+v", st)
	}
	for _, body := range []string{`{"account_rpm":999}`, `{"auto_token":"replacement"}`, `{"max_concurrent":0}`, `{"cookie_keep_hour":24}`} {
		w = request(s, http.MethodPatch, "/api/config", body, s.cfg.AutoToken)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid config accepted: %s => %d", body, w.Code)
		}
	}
}

func TestResultsOnlyExportEligiblePaidAccounts(t *testing.T) {
	s := testServer(t)
	b, _, err := s.store.CreateBatch(model.CreateBatchInput{Name: "results", Text: "one@example.com----pw\ntwo@example.com----pw\nthree@example.com----pw\nfour@example.com----pw"})
	if err != nil {
		t.Fatal(err)
	}
	accounts, err := s.store.ListByBatch(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range accounts {
		a.APIKey, a.CookieHeader = "key", "session=value"
		if i == 2 {
			a.APIKey = ""
		}
		if i == 3 {
			a.CookieHeader = ""
		}
		if err := s.store.SaveLoginResult(a); err != nil {
			t.Fatal(err)
		}
		if i != 1 {
			if err := s.store.SetAccountPaid(a.ID, true); err != nil {
				t.Fatal(err)
			}
		}
	}
	w := request(s, http.MethodGet, "/api/results?batch_id="+b.ID+"&page_size=1", "", s.cfg.AutoToken)
	var result struct {
		Items []model.Account `json:"items"`
		Total int             `json:"total"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &result) != nil {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if result.Total != 1 || len(result.Items) != 1 || result.Items[0].Password != "pw" || !eligibleResult(result.Items[0]) {
		t.Fatalf("invalid result: %+v", result)
	}
	w = request(s, http.MethodGet, "/api/results?page=9223372036854775807&page_size=500", "", s.cfg.AutoToken)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Items) != 0 {
		t.Fatalf("overflow page: %s", w.Body.String())
	}
}

func TestImportPaidAccountsIsIdempotentAndReturnsRemoteIDs(t *testing.T) {
	s := testServer(t)
	a := model.Account{ID: "manager-local-id", Email: "paid@example.com", Password: "pw", APIKey: "key", CookieHeader: "session=value", PaidAt: 1}
	raw, _ := json.Marshal(map[string]any{"accounts": []model.Account{a}})
	var result struct {
		Items   []model.AccountPublic `json:"items"`
		Created int                   `json:"created"`
		Updated int                   `json:"updated"`
	}
	for i := 0; i < 2; i++ {
		w := request(s, http.MethodPost, "/api/accounts/import", string(raw), s.cfg.AutoToken)
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		if len(result.Items) != 1 || result.Items[0].ID == a.ID || result.Created != 1-i || result.Updated != i {
			t.Fatalf("import result: %+v", result)
		}
	}
	w := request(s, http.MethodGet, "/api/accounts/"+result.Items[0].ID, "", s.cfg.AutoToken)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"password":"pw"`) {
		t.Fatalf("credential export: %s", w.Body.String())
	}
	s.jobs.HoldPump()
	w = request(s, http.MethodPost, fmt.Sprintf("/api/accounts/%s/cookie", result.Items[0].ID), "", s.cfg.AutoToken)
	if w.Code != http.StatusAccepted {
		t.Fatalf("cookie enqueue: %d %s", w.Code, w.Body.String())
	}
}

func TestManualPaymentCheckDoesNotMarkUnverifiedAccountsPaid(t *testing.T) {
	s := testServer(t)
	b, _, err := s.store.CreateBatch(model.CreateBatchInput{Name: "unverified", Text: "notpaid@example.com----pw"})
	if err != nil {
		t.Fatal(err)
	}
	w := request(s, http.MethodPost, "/api/batches/"+b.ID+"/paid", "", s.cfg.AutoToken)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	list, err := s.store.ListByBatch(b.ID)
	if err != nil || len(list) != 1 || list[0].PaidAt != 0 {
		t.Fatalf("unverified payment incorrectly marked: %+v %v", list, err)
	}
	w = request(s, http.MethodGet, "/api/usage/sync", "", s.cfg.AutoToken)
	if w.Code != http.StatusOK {
		t.Fatalf("sync status: %d", w.Code)
	}
}

func TestCookieUploadPreservesNewerAutoCredentials(t *testing.T) {
	s := testServer(t)
	a := model.Account{ID: "manager-local", Email: "renew@example.com", Password: "pw", APIKey: "key", CookieHeader: "session=old-manager", PaidAt: 1, LastLoginAt: 10}
	upload := func(a model.Account) {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"accounts": []model.Account{a}})
		w := request(s, http.MethodPost, "/api/accounts/import", string(raw), s.cfg.AutoToken)
		if w.Code != http.StatusOK {
			t.Fatalf("upload: %d %s", w.Code, w.Body.String())
		}
	}
	upload(a)
	local, err := s.store.GetByEmail(a.Email)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.store.SaveCookies(local.ID, `[{"name":"session","value":"renewed-auto"}]`, "session=renewed-auto"); err != nil {
		t.Fatal(err)
	}
	renewed, _ := s.store.Get(local.ID)
	for _, timestamp := range []int64{0, 10, renewed.LastLoginAt} {
		a.LastLoginAt = timestamp
		upload(a)
		got, _ := s.store.Get(local.ID)
		if got.CookieHeader != "session=renewed-auto" || got.CookiesJSON != renewed.CookiesJSON || got.LastLoginAt != renewed.LastLoginAt {
			t.Fatalf("stale upload replaced Auto session: %+v", got)
		}
	}
	a.LastLoginAt = renewed.LastLoginAt + 1
	a.CookieHeader, a.CookiesJSON = "session=newer-manager", `[{"name":"session","value":"newer-manager"}]`
	upload(a)
	got, _ := s.store.Get(local.ID)
	if got.CookieHeader != a.CookieHeader || got.LastLoginAt != a.LastLoginAt {
		t.Fatalf("newer manager session was not accepted: %+v", got)
	}
}
