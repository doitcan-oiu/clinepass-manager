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

func transferServer(t *testing.T, remoteURL string) *Server {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "main.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.SaveAutoConnection(model.AutoConnection{URL: remoteURL, Token: "transfer-secret"}); err != nil {
		t.Fatal(err)
	}
	return &Server{store: st}
}

func TestRemoteResultsImportWithoutSharedDatabase(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer transfer-secret" {
			t.Error("missing authorization")
		}
		writeJSON(w, 200, autoResultsPage{Items: []model.Account{{ID: "remote", Email: "paid@example.com", PaidAt: 100, UpdatedAt: 200, APIKey: "upstream-secret-key", CookieHeader: "session=secret", BatchID: "batch-a", BatchName: "Remote batch"}, {ID: "unpaid", Email: "unpaid@example.com", APIKey: "no", CookieHeader: "no"}}, Total: 2, Page: 1, PageSize: 100})
	}))
	defer upstream.Close()
	s := transferServer(t, upstream.URL)
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		s.importAutoResults(w, httptest.NewRequest("POST", "/api/auto/import", strings.NewReader(`{}`)))
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "secret") {
			t.Fatal("response exposed credentials")
		}
		var result struct{ Imported, Updated, Skipped int }
		json.Unmarshal(w.Body.Bytes(), &result)
		if i == 0 && (result.Imported != 1 || result.Skipped != 1) {
			t.Fatalf("first import %+v", result)
		}
		if i == 1 && (result.Imported != 0 || result.Skipped != 2) {
			t.Fatalf("repeat %+v", result)
		}
	}
	local, err := s.store.GetByEmail("paid@example.com")
	if err != nil || local.ID == "remote" || local.APIKey != "upstream-secret-key" {
		t.Fatal("remote paid credentials not transferred to independent local ID")
	}
	if n, _ := s.store.CountPoolAccounts(""); n != 1 {
		t.Fatalf("pool %d", n)
	}
}

func TestRemoteImportFailsBeforeWritingIncompletePageSet(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			w.WriteHeader(401)
			return
		}
		writeJSON(w, 200, autoResultsPage{Items: []model.Account{{ID: "remote", Email: "paid@example.com", PaidAt: 100, APIKey: "key", CookieHeader: "session=x"}}, Total: 2, Page: 1, PageSize: 1})
	}))
	defer upstream.Close()
	s := transferServer(t, upstream.URL)
	w := httptest.NewRecorder()
	s.importAutoResults(w, httptest.NewRequest("POST", "/api/auto/import", nil))
	if w.Code != 502 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if n, _ := s.store.CountPoolAccounts(""); n != 0 {
		t.Fatal("part of an incomplete remote fetch was imported")
	}
}

func TestCookieResultSynchronizesMatchingLocalAccount(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer transfer-secret" {
			t.Error("missing authorization")
		}
		switch r.URL.Path {
		case "/api/jobs/job":
			writeJSON(w, 200, model.Job{ID: "job", AccountID: "remote", Status: "success"})
		case "/api/accounts/remote":
			writeJSON(w, 200, model.Account{ID: "remote", Email: "paid@example.com", APIKey: "key", CookieHeader: "session=renewed", LastLoginAt: time.Now().Unix() + 1})
		case "/api/results":
			writeJSON(w, 200, autoResultsPage{Items: []model.Account{}, Page: 1, PageSize: 100})
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	s := transferServer(t, upstream.URL)
	a, err := s.store.CreatePaidAccount(model.CreatePaidAccountInput{Email: "paid@example.com", APIKey: "key", CookieHeader: "session=old"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.store.SaveAutoCookieJob(store.AutoCookieJob{Source: upstream.URL, JobID: "job", RemoteID: "remote", LocalID: a.ID}); err != nil {
		t.Fatal(err)
	}
	s.syncAutoCookies()
	a, err = s.store.Get(a.ID)
	if err != nil || a.CookieHeader != "session=renewed" {
		t.Fatal("renewed cookie not synchronized")
	}
	remaining, _ := s.store.ListAutoCookieJobs(upstream.URL)
	if len(remaining) != 0 {
		t.Fatal("completed sync not removed")
	}
}

func TestDelayedCookieJobPreservesNewerLocalCookie(t *testing.T) {
	remote := model.Account{ID: "remote", Email: "paid@example.com", APIKey: "key", CookieHeader: "session=older", LastLoginAt: time.Now().Unix() - 10}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/jobs/job":
			writeJSON(w, 200, model.Job{ID: "job", AccountID: "remote", Status: "success"})
		case "/api/accounts/remote":
			writeJSON(w, 200, remote)
		case "/api/results":
			writeJSON(w, 200, autoResultsPage{Items: []model.Account{remote}, Total: 1, Page: 1, PageSize: 100})
		default:
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	s := transferServer(t, upstream.URL)
	a, _, err := s.store.UpsertPaidAccount(model.CreatePaidAccountInput{Email: remote.Email, APIKey: "local-key", CookieHeader: "session=newer-local"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.store.SaveAutoCookieJob(store.AutoCookieJob{Source: upstream.URL, JobID: "job", RemoteID: remote.ID, LocalID: a.ID}); err != nil {
		t.Fatal(err)
	}
	s.syncAutoCookies()
	a, err = s.store.Get(a.ID)
	if err != nil || a.CookieHeader != "session=newer-local" || a.APIKey != "local-key" {
		t.Fatalf("delayed result overwrote local credentials: %+v %v", a, err)
	}
	remaining, err := s.store.ListAutoCookieJobs(upstream.URL)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("completed stale task not cleared: %+v %v", remaining, err)
	}
}

func TestRemoteImportRejectsInconsistentPagesBeforeWriting(t *testing.T) {
	paid := func(id string) model.Account {
		return model.Account{ID: id, Email: id + "@example.com", PaidAt: 100, APIKey: "key", CookieHeader: "session=valid"}
	}
	for _, tc := range []struct {
		name  string
		pages []autoResultsPage
	}{
		{"short final page", []autoResultsPage{{Items: []model.Account{paid("one")}, Total: 2, Page: 1, PageSize: 100}}},
		{"duplicate account", []autoResultsPage{{Items: []model.Account{paid("one")}, Total: 2, Page: 1, PageSize: 1}, {Items: []model.Account{paid("one")}, Total: 2, Page: 2, PageSize: 1}}},
		{"changing total", []autoResultsPage{{Items: []model.Account{paid("one")}, Total: 2, Page: 1, PageSize: 1}, {Items: []model.Account{paid("two")}, Total: 1, Page: 2, PageSize: 1}}},
		{"changing page size", []autoResultsPage{{Items: []model.Account{paid("one")}, Total: 2, Page: 1, PageSize: 1}, {Items: []model.Account{paid("two")}, Total: 2, Page: 2, PageSize: 2}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if page >= len(tc.pages) {
					t.Error("requested unexpected page")
					w.WriteHeader(500)
					return
				}
				writeJSON(w, 200, tc.pages[page])
				page++
			}))
			defer upstream.Close()
			s := transferServer(t, upstream.URL)
			w := autoTestRequest(s, http.MethodPost, "/api/auto/import", "{}")
			if w.Code != http.StatusBadGateway {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if n, _ := s.store.CountPoolAccounts(""); n != 0 {
				t.Fatal("incomplete result wrote local accounts")
			}
		})
	}
}

func TestRemoteScheduledRenewalUpdatesOnlyLinkedAccounts(t *testing.T) {
	now := time.Now().Unix()
	remote := model.Account{ID: "known", Email: "known@example.com", PaidAt: now - 100, APIKey: "old-key", CookieHeader: "session=old", LastLoginAt: now - 100, UpdatedAt: now - 100}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/results" {
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		writeJSON(w, 200, autoResultsPage{Items: []model.Account{remote, {ID: "unimported", Email: "unimported@example.com", PaidAt: now, APIKey: "secret", CookieHeader: "session=other", LastLoginAt: now + 10}}, Total: 2, Page: 1, PageSize: 100})
	}))
	defer upstream.Close()
	s := transferServer(t, upstream.URL)
	if _, err := s.store.ImportAutoAccount(upstream.URL, remote); err != nil {
		t.Fatal(err)
	}
	local, _ := s.store.GetByEmail(remote.Email)
	if err := s.store.SetCookieExpired(local.ID, true); err != nil {
		t.Fatal(err)
	}
	remote.APIKey, remote.CookieHeader, remote.LastLoginAt = "renewed-key", "session=renewed", now+10
	s.syncAutoCookies()
	local, err := s.store.GetByEmail(remote.Email)
	if err != nil || local.APIKey != "renewed-key" || local.CookieHeader != "session=renewed" || local.LastLoginAt != now+10 {
		t.Fatalf("scheduled renewal not copied: %+v %v", local, err)
	}
	u, _ := s.store.GetAccountUsage(local.ID)
	if u.CookieStale() {
		t.Fatal("renewed cookie remains expired")
	}
	if n, _ := s.store.CountPoolAccounts(""); n != 1 {
		t.Fatalf("background sync auto-created accounts: %d", n)
	}
}
