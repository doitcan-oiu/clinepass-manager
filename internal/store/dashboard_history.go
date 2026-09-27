package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"opencode-go-manager/internal/gomodel"
	"opencode-go-manager/internal/model"
)

// These additive aggregates have no account identifiers or request bodies. They
// survive request-log retention and manual log clearing. Updates apply a delta
// in the same SQLite transaction as the log, so retries cannot double count.
var dashboardMetrics = []struct{ name, expression string }{
	{"requests", "1"},
	{"success", "CASE WHEN %sstatus = 'completed' THEN 1 ELSE 0 END"},
	{"errors", "CASE WHEN %sstatus = 'error' THEN 1 ELSE 0 END"},
	{"processing", "CASE WHEN %sstatus = 'processing' THEN 1 ELSE 0 END"},
	{"input_tokens", "%sinput_tokens"},
	{"output_tokens", "%soutput_tokens"},
	{"reasoning_tokens", "%sreasoning_tokens"},
	{"cache_read", "%scache_read"},
	{"cache_write", "%scache_write"},
	{"total_tokens", "%stotal_tokens"},
	{"ttft_sum", "CASE WHEN %sstatus = 'completed' AND %sstream = 1 AND %sttft_ms > 0 THEN %sttft_ms ELSE 0 END"},
	{"ttft_samples", "CASE WHEN %sstatus = 'completed' AND %sstream = 1 AND %sttft_ms > 0 THEN 1 ELSE 0 END"},
	{"output_tps_sum", "CASE WHEN %sstatus = 'completed' AND %sstream = 1 AND %sttft_ms > 0 AND %sduration_ms > %sttft_ms AND %soutput_tokens > 0 THEN 1000.0 * %soutput_tokens / (%sduration_ms - %sttft_ms) ELSE 0 END"},
	{"output_samples", "CASE WHEN %sstatus = 'completed' AND %sstream = 1 AND %sttft_ms > 0 AND %sduration_ms > %sttft_ms AND %soutput_tokens > 0 THEN 1 ELSE 0 END"},
}

func (s *Store) ensureDashboard() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	cols := []string{}
	for _, m := range dashboardMetrics {
		kind := "INTEGER"
		if m.name == "output_tps_sum" {
			kind = "REAL"
		}
		cols = append(cols, m.name+" "+kind+" NOT NULL DEFAULT 0")
	}
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS dashboard_hours (
		hour INTEGER NOT NULL, model TEXT NOT NULL, channel TEXT NOT NULL, ` + strings.Join(cols, ",") + `,
		PRIMARY KEY(hour, model, channel));
		CREATE TABLE IF NOT EXISTS dashboard_meta (id INTEGER PRIMARY KEY CHECK(id = 1), history_since INTEGER NOT NULL);
		CREATE TABLE IF NOT EXISTS dashboard_billing (
			account_id TEXT NOT NULL, date TEXT NOT NULL, model TEXT NOT NULL,
			usd REAL NOT NULL, synced_at INTEGER NOT NULL,
			PRIMARY KEY(account_id, date, model));
		CREATE INDEX IF NOT EXISTS idx_dashboard_billing_date ON dashboard_billing(date);`); err != nil {
		return err
	}
	var initialized int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM dashboard_meta`).Scan(&initialized); err != nil {
		return err
	}
	if initialized == 0 {
		names, expressions := dashboardMetricSQL("")
		for i := range expressions {
			expressions[i] = "SUM(" + expressions[i] + ")"
		}
		if _, err := tx.Exec(`INSERT INTO dashboard_hours (hour, model, channel, ` + strings.Join(names, ",") + `)
			SELECT created_at / 3600000 * 3600000, model, api_format, ` + strings.Join(expressions, ",") + `
			FROM request_logs GROUP BY 1, 2, 3`); err != nil {
			return err
		}
		// Conservatively report the earliest retained log as the history boundary.
		// Older pruned records cannot be reconstructed during upgrade.
		if _, err := tx.Exec(`INSERT INTO dashboard_meta(id, history_since) SELECT 1, COALESCE(MIN(created_at), ?) FROM request_logs`, time.Now().UnixMilli()); err != nil {
			return err
		}
		rows, err := tx.Query(`SELECT account_id, usage_json FROM account_usage`)
		if err != nil {
			return err
		}
		type existingUsage struct {
			id string
			u  model.AccountUsage
		}
		var usages []existingUsage
		for rows.Next() {
			var id, raw string
			if err := rows.Scan(&id, &raw); err != nil {
				rows.Close()
				return err
			}
			var u model.AccountUsage
			if json.Unmarshal([]byte(raw), &u) == nil {
				usages = append(usages, existingUsage{id, u})
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, u := range usages {
			if err := saveDashboardBilling(tx, u.id, u.u); err != nil {
				return err
			}
		}
	}
	_, err = tx.Exec(`CREATE TRIGGER IF NOT EXISTS dashboard_log_insert AFTER INSERT ON request_logs BEGIN ` + dashboardDelta("NEW.", 1) + ` END;
		CREATE TRIGGER IF NOT EXISTS dashboard_log_update AFTER UPDATE ON request_logs BEGIN ` + dashboardDelta("OLD.", -1) + dashboardDelta("NEW.", 1) + ` END;`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func dashboardMetricSQL(prefix string) (names, expressions []string) {
	for _, m := range dashboardMetrics {
		names = append(names, m.name)
		expressions = append(expressions, strings.ReplaceAll(m.expression, "%s", prefix))
	}
	return
}

func dashboardDelta(prefix string, sign int) string {
	names, values := dashboardMetricSQL(prefix)
	updates := make([]string, len(names))
	for i, name := range names {
		values[i] = fmt.Sprintf("%d * (%s)", sign, values[i])
		updates[i] = name + " = " + name + " + excluded." + name
	}
	return `INSERT INTO dashboard_hours (hour, model, channel, ` + strings.Join(names, ",") + `)
		VALUES (` + prefix + `created_at / 3600000 * 3600000, ` + prefix + `model, ` + prefix + `api_format, ` + strings.Join(values, ",") + `)
		ON CONFLICT(hour, model, channel) DO UPDATE SET ` + strings.Join(updates, ",") + `;`
}

// Daily bills are observations, not deltas: syncing the same day replaces its
// value. No foreign key is used, so expiry of an account retains known billing.
func saveDashboardBilling(tx *sql.Tx, accountID string, u model.AccountUsage) error {
	synced := u.ModelSyncedAt
	if synced == 0 {
		synced = u.SyncedAt
	}
	for _, day := range u.Days {
		if _, err := time.Parse("2006-01-02", day.Date); err != nil || day.USD < 0 || math.IsNaN(day.USD) || math.IsInf(day.USD, 0) {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO dashboard_billing(account_id, date, model, usd, synced_at) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(account_id, date, model) DO UPDATE SET usd = excluded.usd, synced_at = excluded.synced_at
			WHERE excluded.synced_at >= dashboard_billing.synced_at`, accountID, day.Date, gomodel.Canonical(day.Model), day.USD, synced); err != nil {
			return err
		}
	}
	return nil
}
