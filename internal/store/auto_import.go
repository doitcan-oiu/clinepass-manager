package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"opencode-go-manager/internal/backup"
	"opencode-go-manager/internal/model"
)

func (s *Store) ensureAutoTransfers() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS auto_imports (
		source TEXT NOT NULL, remote_id TEXT NOT NULL, local_id TEXT NOT NULL,
		fingerprint TEXT NOT NULL, remote_updated_at INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY(source, remote_id), FOREIGN KEY(local_id) REFERENCES accounts(id) ON DELETE CASCADE);
		CREATE TABLE IF NOT EXISTS auto_import_batches (
		source TEXT NOT NULL, remote_id TEXT NOT NULL, local_id TEXT NOT NULL,
		PRIMARY KEY(source, remote_id), FOREIGN KEY(local_id) REFERENCES batches(id) ON DELETE CASCADE);
		CREATE TABLE IF NOT EXISTS auto_cookie_jobs (
		source TEXT NOT NULL, job_id TEXT NOT NULL, remote_id TEXT NOT NULL, local_id TEXT NOT NULL,
		PRIMARY KEY(source, job_id), FOREIGN KEY(local_id) REFERENCES accounts(id) ON DELETE CASCADE);
		CREATE TABLE IF NOT EXISTS auto_account_links (
		source TEXT NOT NULL, remote_id TEXT NOT NULL, local_id TEXT NOT NULL,
		PRIMARY KEY(source, remote_id), FOREIGN KEY(local_id) REFERENCES accounts(id) ON DELETE CASCADE);
		INSERT OR IGNORE INTO auto_account_links(source,remote_id,local_id)
		SELECT source,remote_id,local_id FROM auto_imports;`)
	return err
}

// ImportAutoAccount copies a proven paid result without sharing IDs or databases.
// The fingerprint makes retries idempotent and avoids reapplying an unchanged
// remote result over a credential subsequently edited in the local account pool.
func (s *Store) ImportAutoAccount(source string, remote model.Account) (string, error) {
	remote.Email = strings.ToLower(strings.TrimSpace(remote.Email))
	remote.CookieHeader = strings.TrimSpace(remote.CookieHeader)
	if remote.CookieHeader == "" {
		remote.CookieHeader = backup.NormalizeCookie(remote.CookiesJSON)
	}
	if source == "" || remote.ID == "" || !strings.Contains(remote.Email, "@") || remote.PaidAt <= 0 || strings.TrimSpace(remote.APIKey) == "" || remote.CookieHeader == "" {
		return "skipped", nil
	}
	// Auto's egress proxy belongs to the remote server; never copy it to manager.
	remote.Proxy = ""
	remote.LoginProvider = model.NormalizeLoginProvider(remote.LoginProvider)
	fingerprintData, _ := json.Marshal(struct {
		Email, Password, Recovery, Key, Workspace, User, Cookies, Cookie, Provider string
		PaidAt, LastLoginAt                                                        int64
	}{remote.Email, remote.Password, remote.RecoveryEmail, remote.APIKey, remote.WorkspaceID, remote.UserID, remote.CookiesJSON, remote.CookieHeader, remote.LoginProvider, remote.PaidAt, remote.LastLoginAt})
	hash := sha256.Sum256(fingerprintData)
	fingerprint := hex.EncodeToString(hash[:])
	if err := s.ensureAutoTransfers(); err != nil {
		return "", err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var oldFingerprint string
	var remoteUpdatedAt int64
	err = tx.QueryRow(`SELECT fingerprint, remote_updated_at FROM auto_imports WHERE source = ? AND remote_id = ?`, source, remote.ID).Scan(&oldFingerprint, &remoteUpdatedAt)
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}
	if err == nil {
		if remote.UpdatedAt > 0 && remote.UpdatedAt < remoteUpdatedAt {
			return "skipped", nil
		}
		if oldFingerprint == fingerprint {
			// Even an unchanged result proves that older versions are stale.
			// Advance the watermark without rewriting local credentials.
			if remote.UpdatedAt > remoteUpdatedAt {
				if _, err := tx.Exec(`UPDATE auto_imports SET remote_updated_at=? WHERE source=? AND remote_id=?`, remote.UpdatedAt, source, remote.ID); err != nil {
					return "", err
				}
				if err := tx.Commit(); err != nil {
					return "", err
				}
			}
			return "skipped", nil
		}
	}
	local, err := scanAccount(tx.QueryRow(accountSelect+` FROM accounts a JOIN batches b ON b.id = a.batch_id WHERE a.email = ?`, remote.Email))
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}
	now := time.Now().Unix()
	result := "updated"
	if err == sql.ErrNoRows {
		result = "imported"
		var batchID string
		err = tx.QueryRow(`SELECT local_id FROM auto_import_batches WHERE source = ? AND remote_id = ?`, source, remote.BatchID).Scan(&batchID)
		if err != nil && err != sql.ErrNoRows {
			return "", err
		}
		if err == sql.ErrNoRows {
			batchID = newID()
			name := strings.TrimSpace(remote.BatchName)
			if name == "" {
				name = "成功账号"
			}
			if _, err := tx.Exec(`INSERT INTO batches(id,name,created_at,updated_at,paid_at) VALUES (?,?,?,?,?)`, batchID, "Auto · "+name, now, now, remote.PaidAt); err != nil {
				return "", err
			}
			if _, err := tx.Exec(`INSERT INTO auto_import_batches(source,remote_id,local_id) VALUES (?,?,?)`, source, remote.BatchID, batchID); err != nil {
				return "", err
			}
		}
		local = remote
		local.ID, local.BatchID, local.FingerprintSeed = newID(), batchID, newSeed()
		if _, err := tx.Exec(`INSERT INTO accounts(id,email,password,recovery_email,proxy,fingerprint_seed,status,workspace_id,api_key,user_id,cookies_json,cookie_header,last_login_at,created_at,updated_at,batch_id,paid_at,login_provider)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, local.ID, remote.Email, remote.Password, remote.RecoveryEmail, "", local.FingerprintSeed, "ready", remote.WorkspaceID, remote.APIKey, remote.UserID, remote.CookiesJSON, remote.CookieHeader, remote.LastLoginAt, now, now, batchID, remote.PaidAt, remote.LoginProvider); err != nil {
			return "", err
		}
	} else {
		if remote.Password == "" {
			remote.Password = local.Password
		}
		if remote.RecoveryEmail == "" {
			remote.RecoveryEmail = local.RecoveryEmail
		}
		if remote.WorkspaceID == "" {
			remote.WorkspaceID = local.WorkspaceID
		}
		if remote.UserID == "" {
			remote.UserID = local.UserID
		}
		if remote.CookiesJSON == "" {
			remote.CookiesJSON = local.CookiesJSON
		}
		if _, err := tx.Exec(`UPDATE accounts SET password=?, recovery_email=?, workspace_id=?, api_key=?, user_id=?, cookies_json=?, cookie_header=?, paid_at=?, status='ready', last_error='', last_login_at=?, updated_at=?, login_provider=? WHERE id=?`, remote.Password, remote.RecoveryEmail, remote.WorkspaceID, remote.APIKey, remote.UserID, remote.CookiesJSON, remote.CookieHeader, remote.PaidAt, remote.LastLoginAt, now, remote.LoginProvider, local.ID); err != nil {
			return "", err
		}
	}
	var usageRaw string
	err = tx.QueryRow(`SELECT usage_json FROM account_usage WHERE account_id=?`, local.ID).Scan(&usageRaw)
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}
	if err == nil {
		var u model.AccountUsage
		if json.Unmarshal([]byte(usageRaw), &u) == nil {
			u.CookieExpired = false
			if model.CookieExpiredMessage(u.Error) || strings.Contains(u.Error, "手动标记") {
				u.Error = ""
			}
			raw, _ := json.Marshal(u)
			if _, err := tx.Exec(`UPDATE account_usage SET usage_json=?,error=? WHERE account_id=?`, string(raw), u.Error, local.ID); err != nil {
				return "", err
			}
		}
	}
	if _, err := tx.Exec(`INSERT INTO auto_imports(source,remote_id,local_id,fingerprint,remote_updated_at) VALUES (?,?,?,?,?) ON CONFLICT(source,remote_id) DO UPDATE SET local_id=excluded.local_id,fingerprint=excluded.fingerprint,remote_updated_at=excluded.remote_updated_at`, source, remote.ID, local.ID, fingerprint, remote.UpdatedAt); err != nil {
		return "", err
	}
	if _, err := tx.Exec(`INSERT INTO auto_account_links(source,remote_id,local_id) VALUES (?,?,?) ON CONFLICT(source,remote_id) DO UPDATE SET local_id=excluded.local_id`, source, remote.ID, local.ID); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return result, nil
}

// LinkAutoAccount records the identity returned after uploading a local account
// to Auto. It does not import or create an account.
func (s *Store) LinkAutoAccount(source, remoteID, localID string) error {
	if source == "" || remoteID == "" || localID == "" {
		return fmt.Errorf("Auto 账号关联信息不完整")
	}
	if err := s.ensureAutoTransfers(); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO auto_account_links(source,remote_id,local_id) VALUES (?,?,?) ON CONFLICT(source,remote_id) DO UPDATE SET local_id=excluded.local_id`, source, remoteID, localID)
	return err
}

func (s *Store) HasAutoAccountLinks(source string) (bool, error) {
	if err := s.ensureAutoTransfers(); err != nil {
		return false, err
	}
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM auto_account_links WHERE source=?)`, source).Scan(&exists)
	return exists, err
}

// SyncAutoAccountCredentials only updates an explicitly linked local account.
// A local credential edit after the remote login wins; no account is created.
func (s *Store) SyncAutoAccountCredentials(source string, remote model.Account) (bool, error) {
	if remote.ID == "" || remote.LastLoginAt <= 0 || strings.TrimSpace(remote.CookieHeader) == "" || strings.TrimSpace(remote.APIKey) == "" {
		return false, nil
	}
	if err := s.ensureAutoTransfers(); err != nil {
		return false, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	local, err := scanAccount(tx.QueryRow(accountSelect+` FROM accounts a LEFT JOIN batches b ON b.id=a.batch_id JOIN auto_account_links l ON l.local_id=a.id WHERE l.source=? AND l.remote_id=?`, source, remote.ID))
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !strings.EqualFold(strings.TrimSpace(remote.Email), local.Email) || remote.LastLoginAt <= local.LastLoginAt {
		return false, nil
	}
	if remote.WorkspaceID == "" {
		remote.WorkspaceID = local.WorkspaceID
	}
	if remote.UserID == "" {
		remote.UserID = local.UserID
	}
	_, err = tx.Exec(`UPDATE accounts SET cookies_json=?,cookie_header=?,api_key=?,workspace_id=?,user_id=?,last_error='',last_login_at=?,updated_at=? WHERE id=?`, remote.CookiesJSON, strings.TrimSpace(remote.CookieHeader), strings.TrimSpace(remote.APIKey), remote.WorkspaceID, remote.UserID, remote.LastLoginAt, time.Now().Unix(), local.ID)
	if err != nil {
		return false, err
	}
	var raw string
	err = tx.QueryRow(`SELECT usage_json FROM account_usage WHERE account_id=?`, local.ID).Scan(&raw)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	if err == nil {
		var u model.AccountUsage
		if json.Unmarshal([]byte(raw), &u) == nil {
			u.CookieExpired = false
			if model.CookieExpiredMessage(u.Error) || strings.Contains(u.Error, "手动标记") {
				u.Error = ""
			}
			updated, _ := json.Marshal(u)
			if _, err := tx.Exec(`UPDATE account_usage SET usage_json=?,error=? WHERE account_id=?`, string(updated), u.Error, local.ID); err != nil {
				return false, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

type AutoCookieJob struct {
	Source, JobID, RemoteID, LocalID string
}

func (s *Store) SaveAutoCookieJob(j AutoCookieJob) error {
	if j.Source == "" || j.JobID == "" || j.RemoteID == "" || j.LocalID == "" {
		return fmt.Errorf("续期任务缺少账号或服务信息")
	}
	if err := s.ensureAutoTransfers(); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO auto_cookie_jobs(source,job_id,remote_id,local_id) VALUES (?,?,?,?) ON CONFLICT(source,job_id) DO UPDATE SET remote_id=excluded.remote_id,local_id=excluded.local_id`, j.Source, j.JobID, j.RemoteID, j.LocalID)
	return err
}

func (s *Store) ListAutoCookieJobs(source string) ([]AutoCookieJob, error) {
	if err := s.ensureAutoTransfers(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT source,job_id,remote_id,local_id FROM auto_cookie_jobs WHERE source=? ORDER BY job_id LIMIT 200`, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AutoCookieJob{}
	for rows.Next() {
		var j AutoCookieJob
		if err := rows.Scan(&j.Source, &j.JobID, &j.RemoteID, &j.LocalID); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *Store) DeleteAutoCookieJob(source, jobID string) error {
	_, err := s.db.Exec(`DELETE FROM auto_cookie_jobs WHERE source=? AND job_id=?`, source, jobID)
	return err
}
