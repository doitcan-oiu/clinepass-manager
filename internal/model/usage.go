package model

import (
	"math"
	"strings"
)

type UsageWindow struct {
	Status       string  `json:"status"`
	ResetInSec   int     `json:"reset_in_sec"`
	UsagePercent float64 `json:"usage_percent"`
}

func (w UsageWindow) Exhausted() bool {
	s := strings.ToLower(strings.TrimSpace(w.Status))
	if s == "rate-limited" || s == "exhausted" || s == "limited" {
		return true
	}
	return math.Round(w.UsagePercent) >= 100
}

type ModelDay struct {
	Date  string  `json:"date"`
	Model string  `json:"model"`
	USD   float64 `json:"usd"`
}

type ModelSpend struct {
	Model    string  `json:"model"`
	USD      float64 `json:"usd"`
	LimitUSD float64 `json:"limit_usd"`
}

type AccountUsage struct {
	Rolling  UsageWindow  `json:"rolling"`
	Weekly   UsageWindow  `json:"weekly"`
	Monthly  UsageWindow  `json:"monthly"`
	Days     []ModelDay   `json:"days,omitempty"`
	Models   []ModelSpend `json:"models"`
	SyncedAt int64        `json:"synced_at"`
	Error    string       `json:"error"`
}

func (u AccountUsage) MonthlyExpiresAt() int64 {
	if u.SyncedAt <= 0 || u.Monthly.Status == "" || u.Monthly.ResetInSec <= 0 {
		return 0
	}
	return u.SyncedAt + int64(u.Monthly.ResetInSec)
}

func (u AccountUsage) MonthlyExpired(now int64) bool {
	at := u.MonthlyExpiresAt()
	return at > 0 && now >= at
}

func (u AccountUsage) QuotaExhausted() bool {
	return u.Rolling.Exhausted() || u.Weekly.Exhausted() || u.Monthly.Exhausted()
}

func SplitShelved(list []PoolAccount) (active, weekly, rolling []PoolAccount) {
	active = make([]PoolAccount, 0, len(list))
	weekly = make([]PoolAccount, 0)
	rolling = make([]PoolAccount, 0)
	for _, a := range list {
		switch {
		case a.Usage.Weekly.Exhausted():
			weekly = append(weekly, a)
		case a.Usage.Rolling.Exhausted():
			rolling = append(rolling, a)
		default:
			active = append(active, a)
		}
	}
	return active, weekly, rolling
}

type PoolAccount struct {
	AccountPublic
	Usage    AccountUsage `json:"usage"`
	Inflight int          `json:"inflight"`
}

type PoolPage struct {
	Items          []PoolAccount `json:"items"`
	WeeklyLimited  []PoolAccount `json:"weekly_limited"`
	RollingLimited []PoolAccount `json:"rolling_limited"`
	Total          int           `json:"total"`
	Page           int           `json:"page"`
	PageSize       int           `json:"page_size"`
	Stats          PoolStats     `json:"stats"`
}

type CreatePaidAccountInput struct {
	Email         string `json:"email"`
	Password      string `json:"password"`
	RecoveryEmail string `json:"recovery_email"`
	APIKey        string `json:"api_key"`
	WorkspaceID   string `json:"workspace_id"`
	UserID        string `json:"user_id"`
	CookieHeader  string `json:"cookie_header"`
	CookiesJSON   string `json:"cookies_json"`
	Proxy         string `json:"proxy"`
	LoginProvider string `json:"login_provider"`
}

type UsageSyncStatus struct {
	Running     bool   `json:"running"`
	Total       int    `json:"total"`
	Done        int    `json:"done"`
	Fail        int    `json:"fail"`
	Paid        int    `json:"paid"`
	Unpaid      int    `json:"unpaid"`
	Message     string `json:"message"`
	StartedAt   int64  `json:"started_at"`
	FinishedAt  int64  `json:"finished_at"`
	IntervalSec int    `json:"interval_sec"`
	Concurrency int    `json:"concurrency"`
}

type PoolStats struct {
	Total      int      `json:"total"`
	Ok         int      `json:"ok"`
	Tight      int      `json:"tight"`
	Exhausted  int      `json:"exhausted"`
	Inflight   int      `json:"inflight"`
	AvgRolling *float64 `json:"avg_rolling"`
	AvgWeekly  *float64 `json:"avg_weekly"`
	AvgMonthly *float64 `json:"avg_monthly"`
}
