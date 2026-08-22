package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"opencode-go-manager/internal/model"
)

const poolSelect = accountSelect + `,
	IFNULL(u.usage_json, ''), IFNULL(u.synced_at, 0), IFNULL(u.error, '')
FROM accounts a
JOIN batches b ON b.id = a.batch_id
LEFT JOIN account_usage u ON u.account_id = a.id
WHERE a.paid_at > 0`

func (s *Store) CountPoolAccounts(batchID string) (int, error) {
	q := `SELECT COUNT(*) FROM accounts a WHERE a.paid_at > 0`
	args := []any{}
	if batchID != "" {
		q += ` AND a.batch_id = ?`
		args = append(args, batchID)
	}
	var n int
	err := s.db.QueryRow(q, args...).Scan(&n)
	return n, err
}

func (s *Store) GetPoolAccount(id string) (model.PoolAccount, error) {
	row := s.db.QueryRow(poolSelect+` AND a.id = ?`, id)
	return scanPoolAccount(row)
}

func (s *Store) ListPoolAccounts(batchID string, limit, offset int) ([]model.PoolAccount, error) {
	if limit < 1 {
		limit = 30
	}
	q := poolSelect
	args := []any{}
	if batchID != "" {
		q += ` AND a.batch_id = ?`
		args = append(args, batchID)
	}
	q += ` ORDER BY a.email LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.PoolAccount{}
	for rows.Next() {
		p, err := scanPoolAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) ListPoolAccountsRaw() ([]model.Account, error) {
	rows, err := s.db.Query(accountSelect + `
FROM accounts a
JOIN batches b ON b.id = a.batch_id
WHERE a.paid_at > 0
ORDER BY a.email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Account{}
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) ListPaidBatches() ([]model.BatchSummary, error) {
	rows, err := s.db.Query(batchSummarySelect + `
GROUP BY b.id
HAVING SUM(CASE WHEN a.paid_at IS NOT NULL AND a.paid_at > 0 THEN 1 ELSE 0 END) > 0
ORDER BY MAX(a.paid_at) DESC, b.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.BatchSummary{}
	for rows.Next() {
		b, err := scanBatchSummary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) ListUnpaidExported() ([]model.BatchSummary, error) {
	rows, err := s.db.Query(batchSummarySelect + `
WHERE b.paid_at = 0 AND b.exported_count > 0
GROUP BY b.id
ORDER BY b.exported_at DESC, b.created_at DESC
LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.BatchSummary{}
	for rows.Next() {
		b, err := scanBatchSummary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) SaveAccountUsage(accountID string, u model.AccountUsage) error {
	raw, err := json.Marshal(u)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
INSERT INTO account_usage (account_id, usage_json, synced_at, error)
VALUES (?, ?, ?, ?)
ON CONFLICT(account_id) DO UPDATE SET
	usage_json = excluded.usage_json,
	synced_at = excluded.synced_at,
	error = excluded.error`,
		accountID, string(raw), u.SyncedAt, u.Error)
	return err
}

func (s *Store) GetAccountUsage(accountID string) (model.AccountUsage, error) {
	var raw string
	var synced int64
	var usageErr string
	err := s.db.QueryRow(`SELECT usage_json, synced_at, error FROM account_usage WHERE account_id = ?`, accountID).
		Scan(&raw, &synced, &usageErr)
	if err != nil {
		return model.AccountUsage{}, err
	}
	u := decodeUsage(raw, synced, usageErr)
	return u, nil
}

const ManualBatchName = "手动账号"

func (s *Store) ManualBatch() (model.Batch, error) {
	row := s.db.QueryRow(`SELECT id, name, created_at, updated_at, exported_at, exported_count, paid_at FROM batches WHERE name = ? ORDER BY created_at LIMIT 1`, ManualBatchName)
	var b model.Batch
	err := row.Scan(&b.ID, &b.Name, &b.CreatedAt, &b.UpdatedAt, &b.ExportedAt, &b.ExportedCount, &b.PaidAt)
	if err == nil {
		if b.PaidAt == 0 {
			_ = s.MarkPaid(b.ID)
			b.PaidAt = time.Now().Unix()
		}
		return b, nil
	}
	if err != sql.ErrNoRows {
		return model.Batch{}, err
	}
	now := time.Now().Unix()
	b = model.Batch{ID: newID(), Name: ManualBatchName, CreatedAt: now, UpdatedAt: now, PaidAt: now}
	_, err = s.db.Exec(`INSERT INTO batches (id, name, created_at, updated_at, exported_at, exported_count, paid_at) VALUES (?, ?, ?, ?, 0, 0, ?)`,
		b.ID, b.Name, b.CreatedAt, b.UpdatedAt, b.PaidAt)
	return b, err
}

func (s *Store) CreatePaidAccount(in model.CreatePaidAccountInput) (model.Account, error) {
	a, _, err := s.UpsertPaidAccount(in)
	return a, err
}

func (s *Store) UpsertPaidAccount(in model.CreatePaidAccountInput) (model.Account, bool, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	cookie := strings.TrimSpace(in.CookieHeader)
	if email == "" || cookie == "" {
		return model.Account{}, false, fmt.Errorf("至少要有账号和 Cookie")
	}
	b, err := s.ManualBatch()
	if err != nil {
		return model.Account{}, false, err
	}
	now := time.Now().Unix()
	if old, err := s.GetByEmail(email); err == nil {
		if pw := strings.TrimSpace(in.Password); pw != "" {
			old.Password = pw
		}
		if rec := strings.TrimSpace(in.RecoveryEmail); rec != "" {
			old.RecoveryEmail = rec
		}
		if ws := strings.TrimSpace(in.WorkspaceID); ws != "" {
			old.WorkspaceID = ws
		}
		if key := strings.TrimSpace(in.APIKey); key != "" {
			old.APIKey = key
		}
		if uid := strings.TrimSpace(in.UserID); uid != "" {
			old.UserID = uid
		}
		if js := strings.TrimSpace(in.CookiesJSON); js != "" {
			old.CookiesJSON = js
		}
		old.CookieHeader = cookie
		old.Status = "ready"
		old.PaidAt = now
		old.UpdatedAt = now
		if old.BatchID == "" {
			old.BatchID = b.ID
			old.BatchName = b.Name
		}
		_, err = s.db.Exec(`
UPDATE accounts SET
	password = ?, recovery_email = ?, status = ?, workspace_id = ?, api_key = ?, user_id = ?,
	cookies_json = ?, cookie_header = ?, paid_at = ?, updated_at = ?, batch_id = ?
WHERE id = ?`,
			old.Password, old.RecoveryEmail, old.Status, old.WorkspaceID, old.APIKey, old.UserID,
			old.CookiesJSON, old.CookieHeader, now, now, old.BatchID, old.ID)
		if err != nil {
			return model.Account{}, false, err
		}
		old.PaidAt = now
		return old, true, nil
	}
	a := model.Account{
		ID:              newID(),
		Email:           email,
		Password:        in.Password,
		RecoveryEmail:   strings.TrimSpace(in.RecoveryEmail),
		Proxy:           strings.TrimSpace(in.Proxy),
		FingerprintSeed: newSeed(),
		Status:          "ready",
		WorkspaceID:     strings.TrimSpace(in.WorkspaceID),
		APIKey:          strings.TrimSpace(in.APIKey),
		UserID:          strings.TrimSpace(in.UserID),
		CookiesJSON:     strings.TrimSpace(in.CookiesJSON),
		CookieHeader:    cookie,
		PaidAt:          now,
		BatchID:         b.ID,
		BatchName:       b.Name,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err = s.db.Exec(`
INSERT INTO accounts (
	id, email, password, recovery_email, proxy, fingerprint_seed, status,
	workspace_id, api_key, user_id, cookies_json, cookie_header, payment_url,
	last_error, last_login_at, created_at, updated_at, batch_id, paid_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', '', 0, ?, ?, ?, ?)`,
		a.ID, a.Email, a.Password, a.RecoveryEmail, a.Proxy, a.FingerprintSeed, a.Status,
		a.WorkspaceID, a.APIKey, a.UserID, a.CookiesJSON, a.CookieHeader,
		now, now, a.BatchID, now)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return model.Account{}, false, fmt.Errorf("账号已存在")
		}
		return model.Account{}, false, err
	}
	return a, false, nil
}

func scanPoolAccount(sc rowScanner) (model.PoolAccount, error) {
	a, raw, synced, usageErr, err := scanAccountUsage(sc)
	if err != nil {
		return model.PoolAccount{}, err
	}
	return model.PoolAccount{
		AccountPublic: a.Public(),
		Usage:         decodeUsage(raw, synced, usageErr),
	}, nil
}

func scanAccountUsage(sc rowScanner) (model.Account, string, int64, string, error) {
	var a model.Account
	var raw, usageErr string
	var synced int64
	err := sc.Scan(
		&a.ID, &a.Email, &a.Password, &a.RecoveryEmail, &a.Proxy, &a.FingerprintSeed, &a.Status,
		&a.WorkspaceID, &a.APIKey, &a.UserID, &a.CookiesJSON, &a.CookieHeader, &a.PaymentURL,
		&a.LastError, &a.LastLoginAt, &a.CreatedAt, &a.UpdatedAt, &a.BatchID, &a.BatchName, &a.PaidAt,
		&raw, &synced, &usageErr,
	)
	return a, raw, synced, usageErr, err
}

func decodeUsage(raw string, synced int64, usageErr string) model.AccountUsage {
	u := model.AccountUsage{Days: []model.ModelDay{}, Models: []model.ModelSpend{}, SyncedAt: synced, Error: usageErr}
	if strings.TrimSpace(raw) == "" {
		return u
	}
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		u.Error = usageErr
		u.SyncedAt = synced
		return u
	}
	if u.Days == nil {
		u.Days = []model.ModelDay{}
	}
	if u.Models == nil {
		u.Models = []model.ModelSpend{}
	}
	if u.SyncedAt == 0 {
		u.SyncedAt = synced
	}
	if u.Error == "" {
		u.Error = usageErr
	}
	return u
}

func (s *Store) DeleteExpiredAccounts(now int64) (int, error) {
	if now <= 0 {
		now = time.Now().Unix()
	}
	list, err := s.ListPoolAccounts("", 100000, 0)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, a := range list {
		if !a.Usage.MonthlyExpired(now) {
			continue
		}
		if err := s.Delete(a.ID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (s *Store) PoolStats(batchID string) (model.PoolStats, error) {
	list, err := s.ListPoolAccounts(batchID, 100000, 0)
	if err != nil {
		return model.PoolStats{}, err
	}
	st := model.PoolStats{Total: len(list)}
	var rSum, wSum, mSum float64
	var rN, wN, mN int
	for _, a := range list {
		u := a.Usage
		ex := u.QuotaExhausted()
		tight := !ex && (u.Rolling.UsagePercent >= 80 || u.Weekly.UsagePercent >= 80 || u.Monthly.UsagePercent >= 80)
		if u.Error != "" && u.Rolling.Status == "" {
			continue
		}
		if ex {
			st.Exhausted++
		} else if tight {
			st.Tight++
		} else if u.Rolling.Status != "" || u.Weekly.Status != "" || u.Monthly.Status != "" {
			st.Ok++
		}
		if u.Rolling.Status != "" {
			rSum += u.Rolling.UsagePercent
			rN++
		}
		if u.Weekly.Status != "" {
			wSum += u.Weekly.UsagePercent
			wN++
		}
		if u.Monthly.Status != "" {
			mSum += u.Monthly.UsagePercent
			mN++
		}
	}
	avg := func(sum float64, n int) *float64 {
		if n == 0 {
			return nil
		}
		v := sum / float64(n)
		return &v
	}
	st.AvgRolling = avg(rSum, rN)
	st.AvgWeekly = avg(wSum, wN)
	st.AvgMonthly = avg(mSum, mN)
	return st, nil
}
