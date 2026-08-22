package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"opencode-go-manager/internal/config"
	"opencode-go-manager/internal/model"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS accounts (
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
	updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_accounts_status ON accounts(status);
CREATE TABLE IF NOT EXISTS settings (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	proxy TEXT NOT NULL DEFAULT '',
	headless INTEGER NOT NULL DEFAULT 1,
	invite_url TEXT NOT NULL DEFAULT '',
	usage_js_url TEXT NOT NULL DEFAULT '',
	updated_at INTEGER NOT NULL DEFAULT 0
);
INSERT OR IGNORE INTO settings (id, proxy, headless, invite_url, updated_at) VALUES (1, '', 1, '', 0);
CREATE TABLE IF NOT EXISTS batches (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
`)
	if err != nil {
		return err
	}
	if err := s.ensureAccountBatchColumn(); err != nil {
		return err
	}
	if err := s.ensureBatchExportColumns(); err != nil {
		return err
	}
	if err := s.ensureColumn("batches", "paid_at", `ALTER TABLE batches ADD COLUMN paid_at INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := s.ensureColumn("accounts", "paid_at", `ALTER TABLE accounts ADD COLUMN paid_at INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := s.ensureColumn("settings", "usage_js_url", `ALTER TABLE settings ADD COLUMN usage_js_url TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := s.ensureColumn("settings", "hero_sms_api_key", `ALTER TABLE settings ADD COLUMN hero_sms_api_key TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := s.ensureColumn("settings", "hero_sms_service", `ALTER TABLE settings ADD COLUMN hero_sms_service TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := s.ensureColumn("settings", "hero_sms_country", `ALTER TABLE settings ADD COLUMN hero_sms_country INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := s.ensureColumn("settings", "hero_sms_max_price", `ALTER TABLE settings ADD COLUMN hero_sms_max_price REAL NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := s.ensureColumn("settings", "max_concurrent", `ALTER TABLE settings ADD COLUMN max_concurrent INTEGER NOT NULL DEFAULT 1`); err != nil {
		return err
	}
	if err := s.ensureColumn("settings", "max_retries", `ALTER TABLE settings ADD COLUMN max_retries INTEGER NOT NULL DEFAULT 3`); err != nil {
		return err
	}
	if err := s.ensureColumn("settings", "email_suffix_blacklist", `ALTER TABLE settings ADD COLUMN email_suffix_blacklist TEXT NOT NULL DEFAULT '[]'`); err != nil {
		return err
	}
	if err := s.ensureColumn("accounts", "login_provider", `ALTER TABLE accounts ADD COLUMN login_provider TEXT NOT NULL DEFAULT 'google'`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE settings SET invite_url = 'https://authkit.cline.bot' WHERE invite_url = '' OR invite_url LIKE '%opencode.ai%'`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`
UPDATE accounts SET paid_at = (
	SELECT b.paid_at FROM batches b WHERE b.id = accounts.batch_id
)
WHERE IFNULL(paid_at, 0) = 0
	AND batch_id IN (SELECT id FROM batches WHERE paid_at > 0)`); err != nil {
		return err
	}
	_, err = s.db.Exec(`
CREATE TABLE IF NOT EXISTS account_usage (
	account_id TEXT PRIMARY KEY,
	usage_json TEXT NOT NULL DEFAULT '',
	synced_at INTEGER NOT NULL DEFAULT 0,
	error TEXT NOT NULL DEFAULT '',
	FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE
);`)
	if err != nil {
		return err
	}
	if err := s.ensureRequestLogs(); err != nil {
		return err
	}
	return s.purgeIncompatibleAccountData()
}

func (s *Store) purgeIncompatibleAccountData() error {
	if err := s.ensureColumn("settings", "provider", `ALTER TABLE settings ADD COLUMN provider TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	var provider string
	if err := s.db.QueryRow(`SELECT IFNULL(provider, '') FROM settings WHERE id = 1`).Scan(&provider); err != nil && err != sql.ErrNoRows {
		return err
	}
	if provider == "clinepass" {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, q := range []string{
		`DELETE FROM request_logs`,
		`DELETE FROM account_usage`,
		`DELETE FROM accounts`,
		`DELETE FROM batches`,
		`UPDATE settings SET provider = 'clinepass' WHERE id = 1`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_, _ = s.db.Exec(`DELETE FROM sqlite_sequence WHERE name IN ('request_logs')`)
	return nil
}

func (s *Store) ensureColumn(table, name, ddl string) error {
	var n int
	q := fmt.Sprintf(`SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name = ?`, table)
	if err := s.db.QueryRow(q, name).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		_, err := s.db.Exec(ddl)
		return err
	}
	return nil
}

func (s *Store) ensureBatchExportColumns() error {
	cols := []struct{ name, ddl string }{
		{"exported_at", `ALTER TABLE batches ADD COLUMN exported_at INTEGER NOT NULL DEFAULT 0`},
		{"exported_count", `ALTER TABLE batches ADD COLUMN exported_count INTEGER NOT NULL DEFAULT 0`},
	}
	for _, c := range cols {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('batches') WHERE name = ?`, c.name).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			if _, err := s.db.Exec(c.ddl); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) ensureAccountBatchColumn() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('accounts') WHERE name = 'batch_id'`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		if _, err := s.db.Exec(`ALTER TABLE accounts ADD COLUMN batch_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_accounts_batch ON accounts(batch_id)`); err != nil {
		return err
	}
	var orphans int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM accounts WHERE batch_id = ''`).Scan(&orphans); err != nil {
		return err
	}
	if orphans == 0 {
		return nil
	}
	now := time.Now().Unix()
	id := newID()
	if _, err := s.db.Exec(`INSERT INTO batches (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		id, "历史账号", now, now); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE accounts SET batch_id = ? WHERE batch_id = ''`, id)
	return err
}

func (s *Store) GetByEmail(email string) (model.Account, error) {
	row := s.db.QueryRow(accountSelect+`
FROM accounts a
LEFT JOIN batches b ON b.id = a.batch_id
WHERE a.email = ?`, strings.ToLower(strings.TrimSpace(email)))
	return scanAccount(row)
}

func (s *Store) Get(id string) (model.Account, error) {
	row := s.db.QueryRow(accountSelect+`
FROM accounts a
LEFT JOIN batches b ON b.id = a.batch_id
WHERE a.id = ?`, id)
	return scanAccount(row)
}

func (s *Store) Delete(id string) error {
	res, err := s.db.Exec(`DELETE FROM accounts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) UpdateStatus(id, status, lastError string) error {
	_, err := s.db.Exec(`UPDATE accounts SET status = ?, last_error = ?, updated_at = ? WHERE id = ?`,
		status, lastError, time.Now().Unix(), id)
	return err
}

func (s *Store) SaveLoginResult(a model.Account) error {
	now := time.Now().Unix()
	a.UpdatedAt = now
	a.LastLoginAt = now
	_, err := s.db.Exec(`
UPDATE accounts SET
	status = ?, workspace_id = ?, api_key = ?, user_id = ?,
	cookies_json = ?, cookie_header = ?, payment_url = ?,
	last_error = ?, last_login_at = ?, updated_at = ?
WHERE id = ?`,
		a.Status, a.WorkspaceID, a.APIKey, a.UserID,
		a.CookiesJSON, a.CookieHeader, a.PaymentURL,
		a.LastError, a.LastLoginAt, a.UpdatedAt, a.ID)
	return err
}

func ParseRaw(raw string) (email, password, recovery string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", ""
	}
	parts := strings.Split(raw, "----")
	if len(parts) >= 1 {
		email = strings.TrimSpace(parts[0])
	}
	if len(parts) >= 2 {
		password = parts[1]
	}
	if len(parts) >= 3 {
		recovery = strings.TrimSpace(parts[2])
	}
	return email, password, recovery
}

func ParseBulk(text string) []model.CreateAccountInput {
	var out []model.CreateAccountInput
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		email, password, recovery := ParseRaw(line)
		if email == "" || password == "" {
			continue
		}
		out = append(out, model.CreateAccountInput{
			Email:         email,
			Password:      password,
			RecoveryEmail: recovery,
			Raw:           line,
		})
	}
	return out
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func newSeed() int {
	n, err := rand.Int(rand.Reader, big.NewInt(90000))
	if err != nil {
		return 12345
	}
	return int(n.Int64()) + 10000
}

type rowScanner interface {
	Scan(dest ...any) error
}

const accountSelect = `SELECT a.id, a.email, a.password, a.recovery_email, a.proxy, a.fingerprint_seed, a.status,
	a.workspace_id, a.api_key, a.user_id, a.cookies_json, a.cookie_header, a.payment_url,
	a.last_error, a.last_login_at, a.created_at, a.updated_at, a.batch_id, IFNULL(b.name, ''), IFNULL(a.paid_at, 0),
	IFNULL(NULLIF(a.login_provider, ''), 'google')`

func scanAccount(sc rowScanner) (model.Account, error) {
	var a model.Account
	err := sc.Scan(
		&a.ID, &a.Email, &a.Password, &a.RecoveryEmail, &a.Proxy, &a.FingerprintSeed, &a.Status,
		&a.WorkspaceID, &a.APIKey, &a.UserID, &a.CookiesJSON, &a.CookieHeader, &a.PaymentURL,
		&a.LastError, &a.LastLoginAt, &a.CreatedAt, &a.UpdatedAt, &a.BatchID, &a.BatchName, &a.PaidAt, &a.LoginProvider,
	)
	a.LoginProvider = model.NormalizeLoginProvider(a.LoginProvider)
	return a, err
}

func (s *Store) GetSettings() (model.Settings, error) {
	row := s.db.QueryRow(`SELECT proxy, headless, invite_url, usage_js_url,
		IFNULL(hero_sms_api_key, ''), IFNULL(hero_sms_service, ''), IFNULL(hero_sms_country, 0), IFNULL(hero_sms_max_price, 0),
		IFNULL(max_concurrent, 1), IFNULL(max_retries, 3), IFNULL(email_suffix_blacklist, '')
		FROM settings WHERE id = 1`)
	var out model.Settings
	var headless int
	var blacklist string
	if err := row.Scan(&out.Proxy, &headless, &out.InviteURL, &out.UsageJSURL,
		&out.HeroSMSAPIKey, &out.HeroSMSService, &out.HeroSMSCountry, &out.HeroSMSMaxPrice, &out.MaxConcurrent, &out.MaxRetries, &blacklist); err != nil {
		return model.Settings{Headless: true, MaxRetries: 3}, err
	}
	out.Headless = headless != 0
	if out.MaxConcurrent < 1 {
		out.MaxConcurrent = 1
	}
	out.MaxRetries = clampMaxRetries(out.MaxRetries)
	out.EmailSuffixBlacklist = DecodeSuffixList(blacklist)
	return out, nil
}

func (s *Store) SaveSettings(in model.Settings) error {
	headless := 0
	if in.Headless {
		headless = 1
	}
	svc := strings.TrimSpace(in.HeroSMSService)
	if svc == "" {
		svc = "ot"
	}
	conc := in.MaxConcurrent
	if conc < 1 {
		conc = 1
	}
	retries := clampMaxRetries(in.MaxRetries)
	blacklist := EncodeSuffixList(in.EmailSuffixBlacklist)
	_, err := s.db.Exec(`
INSERT INTO settings (id, proxy, headless, invite_url, usage_js_url, hero_sms_api_key, hero_sms_service, hero_sms_country, hero_sms_max_price, max_concurrent, max_retries, email_suffix_blacklist, updated_at)
VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	proxy = excluded.proxy,
	headless = excluded.headless,
	invite_url = excluded.invite_url,
	usage_js_url = excluded.usage_js_url,
	hero_sms_api_key = excluded.hero_sms_api_key,
	hero_sms_service = excluded.hero_sms_service,
	hero_sms_country = excluded.hero_sms_country,
	hero_sms_max_price = excluded.hero_sms_max_price,
	max_concurrent = excluded.max_concurrent,
	max_retries = excluded.max_retries,
	email_suffix_blacklist = excluded.email_suffix_blacklist,
	updated_at = excluded.updated_at`,
		strings.TrimSpace(in.Proxy), headless, strings.TrimSpace(in.InviteURL), strings.TrimSpace(in.UsageJSURL),
		strings.TrimSpace(in.HeroSMSAPIKey), svc, in.HeroSMSCountry, in.HeroSMSMaxPrice, conc, retries, blacklist, time.Now().Unix())
	return err
}

func ApplySettings(cfg config.Config, s model.Settings) config.Config {
	cfg.Headless = s.Headless
	cfg.Proxy = strings.TrimSpace(s.Proxy)
	if strings.TrimSpace(s.InviteURL) != "" {
		cfg.InviteURL = strings.TrimSpace(s.InviteURL)
	}
	cfg.HeroSMSAPIKey = strings.TrimSpace(s.HeroSMSAPIKey)
	cfg.HeroSMSService = strings.TrimSpace(s.HeroSMSService)
	cfg.HeroSMSCountry = s.HeroSMSCountry
	cfg.HeroSMSMaxPrice = s.HeroSMSMaxPrice
	if s.MaxConcurrent >= 1 {
		cfg.MaxConcurrent = s.MaxConcurrent
	}
	cfg.MaxRetries = clampMaxRetries(s.MaxRetries)
	return cfg
}

func clampMaxRetries(n int) int {
	if n < 0 {
		return 3
	}
	if n > 32 {
		return 32
	}
	return n
}

func (s *Store) SeedDefaults(cfg config.Config) error {
	var updated int64
	err := s.db.QueryRow(`SELECT updated_at FROM settings WHERE id = 1`).Scan(&updated)
	if err != nil || updated != 0 {
		return nil
	}
	return s.SaveSettings(model.Settings{
		Proxy:      cfg.Proxy,
		Headless:   cfg.Headless,
		InviteURL:  cfg.InviteURL,
		MaxRetries: 3,
	})
}
