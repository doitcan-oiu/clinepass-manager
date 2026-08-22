package store

import (
	"strings"
	"time"

	"opencode-go-manager/internal/model"
)

func (s *Store) ensureRequestLogs() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS request_logs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	created_at INTEGER NOT NULL,
	model TEXT NOT NULL DEFAULT '',
	api_format TEXT NOT NULL DEFAULT '',
	stream INTEGER NOT NULL DEFAULT 0,
	account_id TEXT NOT NULL DEFAULT '',
	account_email TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT '',
	http_status INTEGER NOT NULL DEFAULT 0,
	input_tokens INTEGER NOT NULL DEFAULT 0,
	output_tokens INTEGER NOT NULL DEFAULT 0,
	reasoning_tokens INTEGER NOT NULL DEFAULT 0,
	cache_read INTEGER NOT NULL DEFAULT 0,
	cache_write INTEGER NOT NULL DEFAULT 0,
	total_tokens INTEGER NOT NULL DEFAULT 0,
	duration_ms INTEGER NOT NULL DEFAULT 0,
	ttft_ms INTEGER NOT NULL DEFAULT 0,
	retries INTEGER NOT NULL DEFAULT 0,
	error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_request_logs_created ON request_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_request_logs_model ON request_logs(model);
CREATE INDEX IF NOT EXISTS idx_request_logs_status ON request_logs(status);
`)
	return err
}

func (s *Store) InsertRequestLog(l model.RequestLog) (int64, error) {
	if l.CreatedAt <= 0 {
		l.CreatedAt = time.Now().UnixMilli()
	}
	if l.Status == "" {
		l.Status = "processing"
	}
	stream := 0
	if l.Stream {
		stream = 1
	}
	res, err := s.db.Exec(`
INSERT INTO request_logs (
	created_at, model, api_format, stream, account_id, account_email, status, http_status,
	input_tokens, output_tokens, reasoning_tokens, cache_read, cache_write, total_tokens,
	duration_ms, ttft_ms, retries, error
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.CreatedAt, l.Model, l.APIFormat, stream, l.AccountID, l.AccountEmail, l.Status, l.HTTPStatus,
		l.InputTokens, l.OutputTokens, l.ReasoningTokens, l.CacheRead, l.CacheWrite, l.TotalTokens,
		l.DurationMS, l.TTFTMS, l.Retries, l.Error)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateRequestLog(l model.RequestLog) error {
	if l.ID <= 0 {
		return nil
	}
	stream := 0
	if l.Stream {
		stream = 1
	}
	errMsg := l.Error
	if len(errMsg) > 1000 {
		errMsg = errMsg[:1000]
	}
	_, err := s.db.Exec(`
UPDATE request_logs SET
	model = ?, api_format = ?, stream = ?, account_id = ?, account_email = ?, status = ?, http_status = ?,
	input_tokens = ?, output_tokens = ?, reasoning_tokens = ?, cache_read = ?, cache_write = ?, total_tokens = ?,
	duration_ms = ?, ttft_ms = ?, retries = ?, error = ?
WHERE id = ?`,
		l.Model, l.APIFormat, stream, l.AccountID, l.AccountEmail, l.Status, l.HTTPStatus,
		l.InputTokens, l.OutputTokens, l.ReasoningTokens, l.CacheRead, l.CacheWrite, l.TotalTokens,
		l.DurationMS, l.TTFTMS, l.Retries, errMsg, l.ID)
	return err
}

func (s *Store) CountRequestLogs(f model.RequestLogFilter) (int, error) {
	where, args := requestLogWhere(f)
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM request_logs`+where, args...).Scan(&n)
	return n, err
}

func (s *Store) ListRequestLogs(f model.RequestLogFilter, limit, offset int) ([]model.RequestLog, error) {
	if limit < 1 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	where, args := requestLogWhere(f)
	args = append(args, limit, offset)
	rows, err := s.db.Query(`SELECT
	id, created_at, model, api_format, stream, account_id, account_email, status, http_status,
	input_tokens, output_tokens, reasoning_tokens, cache_read, cache_write, total_tokens,
	duration_ms, ttft_ms, retries, error
FROM request_logs`+where+` ORDER BY id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.RequestLog{}
	for rows.Next() {
		var l model.RequestLog
		var stream int
		if err := rows.Scan(
			&l.ID, &l.CreatedAt, &l.Model, &l.APIFormat, &stream, &l.AccountID, &l.AccountEmail, &l.Status, &l.HTTPStatus,
			&l.InputTokens, &l.OutputTokens, &l.ReasoningTokens, &l.CacheRead, &l.CacheWrite, &l.TotalTokens,
			&l.DurationMS, &l.TTFTMS, &l.Retries, &l.Error,
		); err != nil {
			return nil, err
		}
		l.Stream = stream != 0
		out = append(out, l)
	}
	return out, rows.Err()
}

func requestLogWhere(f model.RequestLogFilter) (string, []any) {
	var parts []string
	var args []any
	if m := strings.TrimSpace(f.Model); m != "" {
		parts = append(parts, `model = ?`)
		args = append(args, m)
	}
	if e := strings.TrimSpace(f.Email); e != "" {
		parts = append(parts, `account_email LIKE ?`)
		args = append(args, "%"+e+"%")
	}
	if st := strings.TrimSpace(f.Status); st != "" {
		parts = append(parts, `status = ?`)
		args = append(args, st)
	}
	switch strings.TrimSpace(f.Stream) {
	case "1", "true", "stream":
		parts = append(parts, `stream = 1`)
	case "0", "false", "sync":
		parts = append(parts, `stream = 0`)
	}
	if len(parts) == 0 {
		return "", args
	}
	return ` WHERE ` + strings.Join(parts, " AND "), args
}

func (s *Store) RequestLogStats(now int64) (model.RequestLogStats, error) {
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	st := model.RequestLogStats{Models: []string{}}
	if err := s.scanLogAgg(now-60_000, &st.RPM1m, &st.TPM1m); err != nil {
		return st, err
	}
	var req5, tok5 int
	if err := s.scanLogAgg(now-5*60_000, &req5, &tok5); err != nil {
		return st, err
	}
	st.RPM5m = float64(req5) / 5
	st.TPM5m = float64(tok5) / 5
	if err := s.scanLogAgg(now-60*60_000, &st.Requests1h, &st.Tokens1h); err != nil {
		return st, err
	}
	if err := s.scanLogAgg(now-24*60*60_000, &st.Requests24h, &st.Tokens24h); err != nil {
		return st, err
	}
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM request_logs WHERE created_at >= ? AND status = 'completed'`, now-60*60_000).Scan(&st.Success1h)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM request_logs WHERE created_at >= ? AND status = 'error'`, now-60*60_000).Scan(&st.Error1h)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM request_logs WHERE status = 'processing'`).Scan(&st.Processing)
	rows, err := s.db.Query(`SELECT DISTINCT model FROM request_logs WHERE model != '' ORDER BY model LIMIT 200`)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return st, err
		}
		st.Models = append(st.Models, m)
	}
	return st, rows.Err()
}

func (s *Store) scanLogAgg(since int64, count, tokens *int) error {
	return s.db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(total_tokens), 0) FROM request_logs WHERE created_at >= ?`,
		since,
	).Scan(count, tokens)
}

func (s *Store) ClearRequestLogs() error {
	for {
		res, err := s.db.Exec(`DELETE FROM request_logs WHERE id IN (SELECT id FROM request_logs LIMIT 2000)`)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			break
		}
	}
	_, _ = s.db.Exec(`DELETE FROM sqlite_sequence WHERE name = 'request_logs'`)
	return nil
}

func (s *Store) PruneRequestLogs(now int64) error {
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	cutoff := now - 14*24*60*60*1000
	if _, err := s.db.Exec(`DELETE FROM request_logs WHERE created_at < ?`, cutoff); err != nil {
		return err
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM request_logs`).Scan(&n); err != nil {
		return err
	}
	if n <= 50000 {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM request_logs WHERE id IN (SELECT id FROM request_logs ORDER BY id ASC LIMIT ?)`, n-50000)
	return err
}
