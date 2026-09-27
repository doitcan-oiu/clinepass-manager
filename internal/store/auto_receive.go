package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"opencode-go-manager/internal/model"
)

// ReceiveAutoCookieAccount uploads a manager account to Auto for renewal.
// The transaction shares SQLite's writer lock with SaveCookies, preventing an
// upload from replacing credentials renewed by a concurrently finishing job.
// Equal or unknown login times prefer Auto's existing cookie.
func (s *Store) ReceiveAutoCookieAccount(in model.Account) (model.Account, bool, error) {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.CookieHeader = strings.TrimSpace(in.CookieHeader)
	if in.Email == "" || in.CookieHeader == "" || strings.TrimSpace(in.APIKey) == "" || in.PaidAt <= 0 {
		return model.Account{}, false, fmt.Errorf("同步账号须已付款并具有邮箱、API Key 和 Cookie")
	}
	batch, err := s.ManualBatch()
	if err != nil {
		return model.Account{}, false, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return model.Account{}, false, err
	}
	defer tx.Rollback()
	old, err := scanAccount(tx.QueryRow(accountSelect+` FROM accounts a JOIN batches b ON b.id=a.batch_id WHERE a.email=?`, in.Email))
	if err != nil && err != sql.ErrNoRows {
		return model.Account{}, false, err
	}
	existed := err == nil
	if existed && strings.TrimSpace(old.CookieHeader) != "" && in.LastLoginAt <= old.LastLoginAt {
		return old, true, nil
	}
	now := time.Now().Unix()
	if existed {
		in.ID, in.BatchID, in.BatchName = old.ID, old.BatchID, old.BatchName
		in.CreatedAt, in.FingerprintSeed, in.Proxy = old.CreatedAt, old.FingerprintSeed, old.Proxy
		if old.PaidAt > 0 {
			in.PaidAt = old.PaidAt
		}
		if in.Password == "" {
			in.Password = old.Password
		}
		if in.RecoveryEmail == "" {
			in.RecoveryEmail = old.RecoveryEmail
		}
		if in.WorkspaceID == "" {
			in.WorkspaceID = old.WorkspaceID
		}
		if in.UserID == "" {
			in.UserID = old.UserID
		}
		if in.LoginProvider == "" {
			in.LoginProvider = old.LoginProvider
		}
	} else {
		in.ID, in.BatchID, in.BatchName = newID(), batch.ID, batch.Name
		in.CreatedAt, in.FingerprintSeed, in.Proxy = now, newSeed(), ""
	}
	in.LoginProvider = model.NormalizeLoginProvider(in.LoginProvider)
	in.Status, in.LastError, in.UpdatedAt = "ready", "", now
	if existed {
		_, err = tx.Exec(`UPDATE accounts SET password=?, recovery_email=?, status='ready', workspace_id=?, api_key=?, user_id=?, cookies_json=?, cookie_header=?, last_error='', last_login_at=?, updated_at=?, paid_at=?, login_provider=? WHERE id=?`,
			in.Password, in.RecoveryEmail, in.WorkspaceID, in.APIKey, in.UserID, in.CookiesJSON, in.CookieHeader, in.LastLoginAt, now, in.PaidAt, in.LoginProvider, in.ID)
	} else {
		_, err = tx.Exec(`INSERT INTO accounts(id,email,password,recovery_email,proxy,fingerprint_seed,status,workspace_id,api_key,user_id,cookies_json,cookie_header,last_login_at,created_at,updated_at,batch_id,paid_at,login_provider) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			in.ID, in.Email, in.Password, in.RecoveryEmail, "", in.FingerprintSeed, "ready", in.WorkspaceID, in.APIKey, in.UserID, in.CookiesJSON, in.CookieHeader, in.LastLoginAt, now, now, in.BatchID, in.PaidAt, in.LoginProvider)
	}
	if err != nil {
		return model.Account{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return model.Account{}, false, err
	}
	return in, existed, nil
}
