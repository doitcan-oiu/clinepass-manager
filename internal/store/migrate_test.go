package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	"opencode-go-manager/internal/model"

	_ "modernc.org/sqlite"
)

func TestMigrateAddsUsageJSURLToOldSettings(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE settings (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	proxy TEXT NOT NULL DEFAULT '',
	headless INTEGER NOT NULL DEFAULT 1,
	invite_url TEXT NOT NULL DEFAULT '',
	updated_at INTEGER NOT NULL DEFAULT 0
);
INSERT INTO settings (id, proxy, headless, invite_url, updated_at) VALUES (1, 'socks5://127.0.0.1:1080', 1, '', 1);
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	st, err := s.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if st.Proxy != "socks5://127.0.0.1:1080" {
		t.Fatalf("proxy=%q", st.Proxy)
	}
	if st.UsageJSURL != "" {
		t.Fatalf("usage_js_url=%q", st.UsageJSURL)
	}
	if err := s.SaveSettings(st); err != nil {
		t.Fatal(err)
	}
}

func TestPurgeOpenCodeAccountsOnClinePassMigrate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE settings (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	proxy TEXT NOT NULL DEFAULT '',
	headless INTEGER NOT NULL DEFAULT 1,
	invite_url TEXT NOT NULL DEFAULT '',
	updated_at INTEGER NOT NULL DEFAULT 0
);
INSERT INTO settings (id, proxy, headless, invite_url, updated_at) VALUES (1, 'socks5://127.0.0.1:1080', 1, 'https://opencode.ai/go', 1);
CREATE TABLE batches (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
INSERT INTO batches (id, name, created_at, updated_at) VALUES ('b1', '旧批次', 1, 1);
CREATE TABLE accounts (
	id TEXT PRIMARY KEY,
	email TEXT NOT NULL UNIQUE,
	password TEXT NOT NULL,
	recovery_email TEXT DEFAULT '',
	proxy TEXT DEFAULT '',
	fingerprint_seed INTEGER NOT NULL,
	status TEXT NOT NULL DEFAULT 'pending',
	workspace_id TEXT DEFAULT '',
	api_key TEXT DEFAULT '',
	user_id TEXT DEFAULT '',
	cookies_json TEXT DEFAULT '',
	cookie_header TEXT DEFAULT '',
	payment_url TEXT DEFAULT '',
	last_error TEXT DEFAULT '',
	last_login_at INTEGER DEFAULT 0,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	batch_id TEXT DEFAULT ''
);
INSERT INTO accounts (id, email, password, fingerprint_seed, status, workspace_id, api_key, cookie_header, created_at, updated_at, batch_id)
VALUES ('a1', 'old@x.com', 'p', 1, 'ready', 'wrk_OLD', 'sk-old', 'auth=x', 1, 1, 'b1');
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	st, err := s.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if st.Proxy != "socks5://127.0.0.1:1080" {
		t.Fatalf("proxy should be kept, got %q", st.Proxy)
	}
	if st.InviteURL != "https://authkit.cline.bot" {
		t.Fatalf("invite_url=%q", st.InviteURL)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("accounts=%d err=%v", n, err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM batches`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("batches=%d err=%v", n, err)
	}
	if _, err := s.CreatePaidAccount(model.CreatePaidAccountInput{Email: "n@x.com", CookieHeader: "cline_session_id=1", APIKey: "sk_1"}); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s2.Close() })
	if err := s2.db.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("second open should keep new accounts, n=%d err=%v", n, err)
	}
}
