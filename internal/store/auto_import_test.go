package store

import (
	"opencode-go-manager/internal/model"
	"path/filepath"
	"testing"
)

func remotePaidFixture() model.Account {
	return model.Account{ID: "remote-id", Email: "User@example.com", Password: "remote-pass", APIKey: "remote-key", CookieHeader: "session=remote", CookiesJSON: `[{"name":"session","value":"remote"}]`, WorkspaceID: "remote-workspace", PaidAt: 100, UpdatedAt: 200, BatchID: "remote-batch", BatchName: "Batch A", LoginProvider: "google", Proxy: "socks5://remote-only:1080"}
}

func TestImportAutoAccountIsIdempotentAndIndependent(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "main.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	r := remotePaidFixture()
	state, err := st.ImportAutoAccount("https://auto.example", r)
	if err != nil || state != "imported" {
		t.Fatalf("%s %v", state, err)
	}
	local, err := st.GetByEmail(r.Email)
	if err != nil {
		t.Fatal(err)
	}
	if local.ID == r.ID || local.BatchID == r.BatchID || local.Proxy != "" || local.PaidAt != r.PaidAt || local.APIKey != r.APIKey || local.Password != r.Password || local.BatchName != "Auto · Batch A" {
		t.Fatalf("import mapping mismatch id=%s batch=%s", local.ID, local.BatchName)
	}
	if err := st.SaveCookies(local.ID, "", "session=local-new"); err != nil {
		t.Fatal(err)
	}
	state, err = st.ImportAutoAccount("https://auto.example", r)
	if err != nil || state != "skipped" {
		t.Fatalf("repeat %s %v", state, err)
	}
	local, _ = st.Get(local.ID)
	if local.CookieHeader != "session=local-new" {
		t.Fatal("unchanged remote overwrote local credentials")
	}
	if err := st.SetCookieExpired(local.ID, true); err != nil {
		t.Fatal(err)
	}
	r.UpdatedAt++
	r.CookieHeader = "session=remote-new"
	state, err = st.ImportAutoAccount("https://auto.example", r)
	if err != nil || state != "updated" {
		t.Fatalf("update %s %v", state, err)
	}
	local, _ = st.Get(local.ID)
	if local.CookieHeader != r.CookieHeader {
		t.Fatal("new result missing")
	}
	u, _ := st.GetAccountUsage(local.ID)
	if u.CookieStale() {
		t.Fatal("fresh imported cookie remains expired")
	}
	r.UpdatedAt--
	r.CookieHeader = "session=stale"
	state, err = st.ImportAutoAccount("https://auto.example", r)
	if err != nil || state != "skipped" {
		t.Fatalf("stale %s %v", state, err)
	}
	n, _ := st.CountPoolAccounts("")
	if n != 1 {
		t.Fatalf("duplicate accounts %d", n)
	}
}

func TestImportAutoAccountRejectsUnprovenResults(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "main.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for _, change := range []func(*model.Account){func(a *model.Account) { a.PaidAt = 0 }, func(a *model.Account) { a.APIKey = "" }, func(a *model.Account) { a.CookieHeader = ""; a.CookiesJSON = "" }, func(a *model.Account) { a.ID = "" }} {
		a := remotePaidFixture()
		change(&a)
		state, err := st.ImportAutoAccount("https://auto.example", a)
		if err != nil || state != "skipped" {
			t.Fatalf("%s %v", state, err)
		}
	}
	n, _ := st.CountPoolAccounts("")
	if n != 0 {
		t.Fatal("invalid result entered pool")
	}
}

func TestAutoCookieJobsPersistAndStayWithSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a := remotePaidFixture()
	if _, err := st.ImportAutoAccount("https://auto-a.example", a); err != nil {
		t.Fatal(err)
	}
	local, _ := st.GetByEmail(a.Email)
	if err := st.SaveAutoCookieJob(AutoCookieJob{Source: "https://auto-a.example", JobID: "job", RemoteID: a.ID, LocalID: local.ID}); err != nil {
		t.Fatal(err)
	}
	st.Close()
	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	items, err := st.ListAutoCookieJobs("https://auto-a.example")
	if err != nil || len(items) != 1 {
		t.Fatalf("pending %v %d", err, len(items))
	}
	other, err := st.ListAutoCookieJobs("https://auto-b.example")
	if err != nil || len(other) != 0 {
		t.Fatal("jobs crossed servers")
	}
	if err := st.Delete(local.ID); err != nil {
		t.Fatal(err)
	}
	items, err = st.ListAutoCookieJobs("https://auto-a.example")
	if err != nil || len(items) != 0 {
		t.Fatal("deleted account retained pending cookie job")
	}
}

func TestImportAutoAccountUnchangedResultAdvancesVersion(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "main.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	r := remotePaidFixture()
	if _, err := st.ImportAutoAccount("https://auto.example", r); err != nil {
		t.Fatal(err)
	}
	r.UpdatedAt = 400
	if result, err := st.ImportAutoAccount("https://auto.example", r); err != nil || result != "skipped" {
		t.Fatalf("%s %v", result, err)
	}
	r.UpdatedAt, r.CookieHeader = 300, "session=stale"
	if result, err := st.ImportAutoAccount("https://auto.example", r); err != nil || result != "skipped" {
		t.Fatalf("older version accepted: %s %v", result, err)
	}
	local, _ := st.GetByEmail(r.Email)
	if local.CookieHeader != "session=remote" {
		t.Fatal("unchanged newer version failed to protect credentials")
	}
}

func TestSyncAutoCredentialsPreservesNewerLocalEditsAndSource(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "main.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	r := remotePaidFixture()
	if _, err := st.ImportAutoAccount("https://auto.example", r); err != nil {
		t.Fatal(err)
	}
	local, _ := st.GetByEmail(r.Email)
	if _, err := st.db.Exec(`UPDATE accounts SET cookie_header='session=local-new',api_key='local-key',last_login_at=500,updated_at=500 WHERE id=?`, local.ID); err != nil {
		t.Fatal(err)
	}
	r.LastLoginAt, r.CookieHeader, r.APIKey = 400, "session=remote-new", "remote-new-key"
	if changed, err := st.SyncAutoAccountCredentials("https://auto.example", r); err != nil || changed {
		t.Fatalf("newer local credentials overwritten: %t %v", changed, err)
	}
	r.LastLoginAt = 600
	if changed, err := st.SyncAutoAccountCredentials("https://different-auto.example", r); err != nil || changed {
		t.Fatalf("unrelated server credentials applied: %t %v", changed, err)
	}
	local, _ = st.Get(local.ID)
	if local.CookieHeader != "session=local-new" || local.APIKey != "local-key" {
		t.Fatal("local credentials changed")
	}
	// Usage refreshes modify updated_at but do not create newer credentials.
	if _, err := st.db.Exec(`UPDATE accounts SET updated_at=1000 WHERE id=?`, local.ID); err != nil {
		t.Fatal(err)
	}
	if changed, err := st.SyncAutoAccountCredentials("https://auto.example", r); err != nil || !changed {
		t.Fatalf("newer remote result not applied: %t %v", changed, err)
	}
	local, _ = st.Get(local.ID)
	if local.CookieHeader != r.CookieHeader || local.APIKey != r.APIKey {
		t.Fatal("newer remote credentials missing")
	}
	local, _, err = st.UpsertPaidAccount(model.CreatePaidAccountInput{Email: local.Email, APIKey: "manually-edited-key", CookieHeader: "session=manual"})
	if err != nil || local.LastLoginAt <= r.LastLoginAt {
		t.Fatalf("manual credential timestamp not updated: %d %v", local.LastLoginAt, err)
	}
	r.LastLoginAt++
	if changed, err := st.SyncAutoAccountCredentials("https://auto.example", r); err != nil || changed {
		t.Fatalf("newer manually edited cookie overwritten: %t %v", changed, err)
	}
}
