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
	return fetchAccount(a, proxy, "", "")
}

func fetchAccount(a model.Account, proxy, apiBase, appBase string) (model.AccountUsage, bool, error) {
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
	if !cline.ValidUserID(userID) {
		id, err := c.DiscoverUserID()
		if err != nil {
			return model.AccountUsage{}, false, err
		}
		userID = id
	}

	out := model.AccountUsage{Days: []model.ModelDay{}, Models: []model.ModelSpend{}}
	caps := cline.DefaultCaps()
	if got, err := c.PlansCaps(); err == nil {
		caps = got
	}

	now := time.Now().In(cst)
	start, end := cline.DailyWindow(now)
	items, err := c.DailyUsages(userID, start, end)
	if err != nil {
		if strings.Contains(err.Error(), "未支付") {
			out.SyncedAt = time.Now().Unix()
			out.Error = "未支付"
			return out, false, nil
		}
		return out, false, err
	}

	days := make([]model.ModelDay, 0, len(items))
	for _, it := range items {
		days = append(days, model.ModelDay{
			Date:  it.Date,
			Model: it.Model,
			USD:   cline.UnitsToUSD(it.CostUnits),
		})
	}
	out.Days = days
	out.Models = AggregateMonth(days, now.Format("2006-01"))
	out.Rolling, out.Weekly, out.Monthly = WindowsFromDays(days, caps, now)
	out.SyncedAt = time.Now().Unix()
	return out, true, nil
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
