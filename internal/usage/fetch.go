package usage

import (
	"fmt"
	"strings"
	"time"

	"opencode-go-manager/internal/cline"
	"opencode-go-manager/internal/gomodel"
	"opencode-go-manager/internal/model"
)

var cst = time.FixedZone("CST", 8*3600)

func FetchAccount(a model.Account, proxy string) (model.AccountUsage, bool, error) {
	return fetchAccount(a, proxy, "", "", true)
}

func fetchAccount(a model.Account, proxy, apiBase, appBase string, includeModels bool) (model.AccountUsage, bool, error) {
	cookie := strings.TrimSpace(a.CookieHeader)
	if cookie == "" {
		return model.AccountUsage{}, false, fmt.Errorf("缺少 Cookie")
	}
	userID := strings.TrimSpace(a.UserID)
	if !cline.ValidUserID(userID) {
		if ws := strings.TrimSpace(a.WorkspaceID); cline.ValidUserID(ws) {
			userID = ws
		} else {
			userID = ""
		}
	}
	c := cline.New(cookie, proxy)
	c.SetBase(apiBase, appBase)

	out := model.AccountUsage{Models: []model.ModelSpend{}}
	limits, err := c.PlanUsageLimits()
	if err != nil {
		if strings.Contains(err.Error(), "未支付") {
			out.SyncedAt = time.Now().Unix()
			out.Error = "未支付"
			return out, false, nil
		}
		return out, false, err
	}
	now := time.Now()
	out.Rolling, out.Weekly, out.Monthly = WindowsFromPlanLimits(limits, now)
	if out.Rolling.Status == "" && out.Weekly.Status == "" && out.Monthly.Status == "" {
		out.SyncedAt = now.Unix()
		out.Error = "没有套餐用量"
		return out, false, nil
	}

	if includeModels {
		if !cline.ValidUserID(userID) {
			if id, derr := c.DiscoverUserID(); derr == nil {
				userID = id
			}
		}
		if cline.ValidUserID(userID) {
			start, end := cline.DailyWindow(now.In(cst))
			if items, derr := c.DailyUsages(userID, start, end); derr == nil {
				days := make([]model.ModelDay, 0, len(items))
				for _, it := range items {
					days = append(days, model.ModelDay{
						Date:  it.Date,
						Model: it.Model,
						USD:   cline.UnitsToUSD(it.CostUnits),
					})
				}
				out.Days = days
				out.Models = AggregateMonth(days, now.In(cst).Format("2006-01"))
				out.ModelSyncedAt = now.Unix()
			}
		}
	}
	out.SyncedAt = now.Unix()
	return out, true, nil
}

func WindowsFromPlanLimits(limits []cline.PlanUsageLimit, now time.Time) (rolling, weekly, monthly model.UsageWindow) {
	for _, it := range limits {
		w := windowFromLimit(it, now)
		switch strings.ToLower(strings.TrimSpace(it.Type)) {
		case "five_hour", "five-hour", "5_hour", "rolling":
			rolling = w
		case "weekly":
			weekly = w
		case "monthly":
			monthly = w
		}
	}
	return rolling, weekly, monthly
}

func windowFromLimit(it cline.PlanUsageLimit, now time.Time) model.UsageWindow {
	pct := it.PercentUsed
	if pct < 0 {
		pct = 0
	}
	status := "ok"
	if pct >= 100 {
		status = "rate-limited"
		pct = 100
	}
	reset := 0
	if !it.ResetsAt.IsZero() {
		reset = int(it.ResetsAt.Sub(now).Seconds())
		if reset < 0 {
			reset = 0
		}
	}
	return model.UsageWindow{Status: status, UsagePercent: pct, ResetInSec: reset}
}

func WindowsFromDays(days []model.ModelDay, caps cline.Caps, now time.Time) (rolling, weekly, monthly model.UsageWindow) {
	if caps.RollingUSD <= 0 {
		caps.RollingUSD = gomodel.RollingUSD
	}
	if caps.WeeklyUSD <= 0 {
		caps.WeeklyUSD = gomodel.WeeklyUSD
	}
	if caps.MonthlyUSD <= 0 {
		caps.MonthlyUSD = gomodel.MonthlyUSD
	}
	today := now.Format("2006-01-02")
	weekFrom := now.AddDate(0, 0, -6).Format("2006-01-02")
	monthFrom := now.AddDate(0, 0, -29).Format("2006-01-02")
	var daySum, weekSum, monthSum float64
	for _, d := range days {
		if d.Date >= monthFrom {
			monthSum += d.USD
		}
		if d.Date >= weekFrom {
			weekSum += d.USD
		}
		if d.Date == today {
			daySum += d.USD
		}
	}
	rolling = windowFromSpend(daySum, caps.RollingUSD, int((5 * time.Hour).Seconds()))
	weekly = windowFromSpend(weekSum, caps.WeeklyUSD, int((7 * 24 * time.Hour).Seconds()))
	monthly = windowFromSpend(monthSum, caps.MonthlyUSD, int((30 * 24 * time.Hour).Seconds()))
	return rolling, weekly, monthly
}

func windowFromSpend(used, cap float64, reset int) model.UsageWindow {
	pct := 0.0
	if cap > 0 {
		pct = used / cap * 100
	}
	status := "ok"
	if pct >= 100 {
		status = "rate-limited"
		pct = 100
	}
	return model.UsageWindow{Status: status, UsagePercent: pct, ResetInSec: reset}
}
