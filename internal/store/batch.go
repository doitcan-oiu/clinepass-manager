package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"opencode-go-manager/internal/model"
)

func isRadarDeniedError(msg string) bool {
	msg = strings.ToLower(msg)
	if strings.Contains(msg, "policy_denied") {
		return true
	}
	return strings.Contains(msg, "authkit radar") && strings.Contains(msg, "拦截")
}

func DefaultBatchName() string {
	return "批次-" + time.Now().Format("0102-150405")
}

const batchSummarySelect = `
SELECT b.id, b.name, b.created_at, b.updated_at, b.exported_at, b.exported_count, b.paid_at,
	COUNT(a.id),
	SUM(CASE WHEN a.status IS NOT NULL AND a.status != 'ready' THEN 1 ELSE 0 END),
	SUM(CASE WHEN a.status = 'ready' THEN 1 ELSE 0 END),
	SUM(CASE WHEN a.status = 'failed' THEN 1 ELSE 0 END),
	SUM(CASE WHEN a.payment_url IS NOT NULL AND a.payment_url != '' THEN 1 ELSE 0 END),
	SUM(CASE WHEN a.cookies_json IS NOT NULL AND a.cookies_json != '' AND a.workspace_id IS NOT NULL AND a.workspace_id != '' THEN 1 ELSE 0 END),
	SUM(CASE WHEN a.paid_at IS NOT NULL AND a.paid_at > 0 THEN 1 ELSE 0 END),
	SUM(CASE WHEN a.payment_url IS NOT NULL AND a.payment_url != '' AND IFNULL(a.paid_at, 0) = 0 THEN 1 ELSE 0 END),
	SUM(CASE WHEN a.cookies_json IS NOT NULL AND a.cookies_json != '' AND a.workspace_id IS NOT NULL AND a.workspace_id != '' AND IFNULL(a.paid_at, 0) = 0 THEN 1 ELSE 0 END)
FROM batches b
LEFT JOIN accounts a ON a.batch_id = b.id`

func scanBatchSummary(sc interface {
	Scan(dest ...any) error
}) (model.BatchSummary, error) {
	var b model.BatchSummary
	var pending, ready, failed, pay, cookies, paid, unpaidPay, unpaidCookie sql.NullInt64
	err := sc.Scan(&b.ID, &b.Name, &b.CreatedAt, &b.UpdatedAt, &b.ExportedAt, &b.ExportedCount, &b.PaidAt, &b.Total, &pending, &ready, &failed, &pay, &cookies, &paid, &unpaidPay, &unpaidCookie)
	if err != nil {
		return model.BatchSummary{}, err
	}
	b.Pending = int(pending.Int64)
	b.Ready = int(ready.Int64)
	b.Failed = int(failed.Int64)
	b.PayCount = int(pay.Int64)
	b.CookieCount = int(cookies.Int64)
	b.PaidCount = int(paid.Int64)
	b.UnpaidPayCount = int(unpaidPay.Int64)
	b.UnpaidCookieCount = int(unpaidCookie.Int64)
	return b, nil
}

func (s *Store) CountBatches() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM batches WHERE name != ?`, ManualBatchName).Scan(&n)
	return n, err
}

func (s *Store) ListBatchesPage(limit, offset int) ([]model.BatchSummary, error) {
	if limit < 1 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.Query(batchSummarySelect+`
WHERE b.name != ?
GROUP BY b.id
ORDER BY b.created_at DESC
LIMIT ? OFFSET ?`, ManualBatchName, limit, offset)
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

func (s *Store) GetBatch(id string) (model.Batch, error) {
	row := s.db.QueryRow(`SELECT id, name, created_at, updated_at, exported_at, exported_count, paid_at FROM batches WHERE id = ?`, id)
	var b model.Batch
	err := row.Scan(&b.ID, &b.Name, &b.CreatedAt, &b.UpdatedAt, &b.ExportedAt, &b.ExportedCount, &b.PaidAt)
	return b, err
}

func (s *Store) ListByBatchMeta(batchID string) ([]model.Account, error) {
	rows, err := s.db.Query(accountMetaSelect+`
FROM accounts a
LEFT JOIN batches b ON b.id = a.batch_id
WHERE a.batch_id = ?
ORDER BY a.created_at DESC`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Account{}
	for rows.Next() {
		a, err := scanAccountMeta(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) ListByBatch(batchID string) ([]model.Account, error) {
	rows, err := s.db.Query(accountSelect+`
FROM accounts a
LEFT JOIN batches b ON b.id = a.batch_id
WHERE a.batch_id = ?
ORDER BY a.created_at DESC`, batchID)
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

func (s *Store) CreateBatch(in model.CreateBatchInput) (model.BatchSummary, []string, error) {
	items := ParseBulk(in.Text)
	if len(items) == 0 {
		return model.BatchSummary{}, nil, fmt.Errorf("没有有效账号，格式：邮箱----密码 或 邮箱----密码----辅助邮箱，一行一个")
	}
	provider := model.NormalizeLoginProvider(in.LoginProvider)
	for i := range items {
		if strings.TrimSpace(items[i].LoginProvider) == "" {
			items[i].LoginProvider = provider
		} else {
			items[i].LoginProvider = model.NormalizeLoginProvider(items[i].LoginProvider)
		}
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = DefaultBatchName()
	}
	now := time.Now().Unix()
	b := model.Batch{ID: newID(), Name: name, CreatedAt: now, UpdatedAt: now}
	tx, err := s.db.Begin()
	if err != nil {
		return model.BatchSummary{}, nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`INSERT INTO batches (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		b.ID, b.Name, b.CreatedAt, b.UpdatedAt); err != nil {
		return model.BatchSummary{}, nil, err
	}
	errors := []string{}
	created := 0
	for _, item := range items {
		item.BatchID = b.ID
		if _, err := s.createInTx(tx, item); err != nil {
			errors = append(errors, item.Email+": "+err.Error())
			continue
		}
		created++
	}
	if created == 0 {
		return model.BatchSummary{}, errors, fmt.Errorf("没有写入任何账号")
	}
	if err := tx.Commit(); err != nil {
		return model.BatchSummary{}, nil, err
	}
	sum, err := s.GetBatchSummary(b.ID)
	return sum, errors, err
}

func (s *Store) GetBatchSummary(id string) (model.BatchSummary, error) {
	row := s.db.QueryRow(batchSummarySelect+` WHERE b.id = ? GROUP BY b.id`, id)
	b, err := scanBatchSummary(row)
	if err != nil {
		return model.BatchSummary{}, err
	}
	return b, nil
}

func (s *Store) DeleteRadarDenied(batchID string) (int, error) {
	if _, err := s.GetBatch(batchID); err != nil {
		return 0, err
	}
	list, err := s.ListByBatch(batchID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, a := range list {
		if a.Status != "failed" || !isRadarDeniedError(a.LastError) {
			continue
		}
		if err := s.Delete(a.ID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (s *Store) DeleteBatch(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM accounts WHERE batch_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM batches WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

func (s *Store) UniquePaymentLinks(batchID string) ([]model.PayLink, error) {
	if _, err := s.GetBatch(batchID); err != nil {
		return nil, err
	}
	list, err := s.ListByBatch(batchID)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	out := []model.PayLink{}
	for _, a := range list {
		if a.PaidAt > 0 {
			continue
		}
		u := strings.TrimSpace(a.PaymentURL)
		if u == "" {
			continue
		}
		email := strings.TrimSpace(a.Email)
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		out = append(out, model.PayLink{
			Email:    email,
			Password: a.Password,
			Cookie:   strings.TrimSpace(a.CookieHeader),
			URL:      u,
		})
	}
	return out, nil
}

func (s *Store) MarkExported(id string, count int) error {
	now := time.Now().Unix()
	res, err := s.db.Exec(`UPDATE batches SET exported_at = ?, exported_count = ?, updated_at = ? WHERE id = ?`,
		now, count, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ClearExported(id string) error {
	now := time.Now().Unix()
	_, err := s.db.Exec(`UPDATE batches SET exported_at = 0, exported_count = 0, updated_at = ? WHERE id = ?`, now, id)
	return err
}

func (s *Store) SetAccountPaid(id string, paid bool) error {
	now := time.Now().Unix()
	paidAt := int64(0)
	if paid {
		paidAt = now
	}
	_, err := s.db.Exec(`UPDATE accounts SET paid_at = ?, updated_at = ? WHERE id = ?`, paidAt, now, id)
	return err
}

func (s *Store) MarkPaid(id string) error {
	now := time.Now().Unix()
	res, err := s.db.Exec(`UPDATE batches SET paid_at = ?, updated_at = ? WHERE id = ?`, now, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) createInTx(tx *sql.Tx, in model.CreateAccountInput) (model.Account, error) {
	email, password, recovery := ParseRaw(in.Raw)
	if in.Email != "" {
		email = strings.TrimSpace(in.Email)
	}
	if in.Password != "" {
		password = in.Password
	}
	if in.RecoveryEmail != "" {
		recovery = strings.TrimSpace(in.RecoveryEmail)
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return model.Account{}, fmt.Errorf("邮箱和密码不能为空")
	}
	now := time.Now().Unix()
	a := model.Account{
		ID:              newID(),
		Email:           email,
		Password:        password,
		RecoveryEmail:   recovery,
		Proxy:           strings.TrimSpace(in.Proxy),
		FingerprintSeed: newSeed(),
		Status:          "pending",
		BatchID:         strings.TrimSpace(in.BatchID),
		LoginProvider:   model.NormalizeLoginProvider(in.LoginProvider),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err := tx.Exec(`
INSERT INTO accounts (
	id, email, password, recovery_email, proxy, fingerprint_seed, status,
	workspace_id, api_key, user_id, cookies_json, cookie_header, payment_url,
	last_error, last_login_at, created_at, updated_at, batch_id, login_provider
) VALUES (?, ?, ?, ?, ?, ?, ?, '', '', '', '', '', '', '', 0, ?, ?, ?, ?)`,
		a.ID, a.Email, a.Password, a.RecoveryEmail, a.Proxy, a.FingerprintSeed, a.Status, now, now, a.BatchID, a.LoginProvider)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return model.Account{}, fmt.Errorf("账号已存在")
		}
		return model.Account{}, err
	}
	return a, nil
}
