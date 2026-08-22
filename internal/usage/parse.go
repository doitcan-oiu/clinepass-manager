package usage

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"opencode-go-manager/internal/gomodel"
	"opencode-go-manager/internal/model"
)

const CostUnitsPerUSD = 100_000_000.0

var (
	reWindow = func(key string) *regexp.Regexp {
		return regexp.MustCompile(key + `:\s*(?:\$R\[\d+\]\s*=\s*)?\{([^}]*)\}`)
	}
	reRolling = reWindow("rollingUsage")
	reWeekly  = reWindow("weeklyUsage")
	reMonthly = reWindow("monthlyUsage")
	reStatus  = regexp.MustCompile(`status:\s*"([^"]+)"`)
	reReset   = regexp.MustCompile(`resetInSec:\s*(-?\d+)`)
	rePercent = regexp.MustCompile(`usagePercent:\s*(-?\d+(?:\.\d+)?)`)
	reDay     = regexp.MustCompile(`date:\s*"(\d{4}-\d{2}-\d{2})"\s*,\s*model:\s*"([^"]+)"\s*,\s*totalCost:\s*(\d+)`)
)

func ParseWindows(html string) (rolling, weekly, monthly model.UsageWindow, err error) {
	rolling = parseWindow(html, reRolling)
	weekly = parseWindow(html, reWeekly)
	monthly = parseWindow(html, reMonthly)
	if rolling.Status == "" && weekly.Status == "" && monthly.Status == "" {
		return rolling, weekly, monthly, fmt.Errorf("页面里没有配额数据")
	}
	return rolling, weekly, monthly, nil
}

func PageKind(html string) string {
	rolling, weekly, monthly, err := ParseWindows(html)
	if err == nil && (rolling.Status != "" || weekly.Status != "" || monthly.Status != "") {
		return "subscribed"
	}
	low := strings.ToLower(html)
	if strings.Contains(low, "continue with google") || strings.Contains(html, "auth.opencode.ai") || strings.Contains(low, "/google/authorize") {
		return "auth"
	}
	if strings.Contains(html, "wrk_") || strings.Contains(html, "lite.subscription") || strings.Contains(html, "/workspace/") {
		return "unpaid"
	}
	return "auth"
}

func parseWindow(html string, re *regexp.Regexp) model.UsageWindow {
	m := re.FindStringSubmatch(html)
	if len(m) < 2 {
		return model.UsageWindow{}
	}
	block := m[1]
	w := model.UsageWindow{}
	if sm := reStatus.FindStringSubmatch(block); len(sm) == 2 {
		w.Status = sm[1]
	}
	if sm := reReset.FindStringSubmatch(block); len(sm) == 2 {
		w.ResetInSec, _ = strconv.Atoi(sm[1])
	}
	if sm := rePercent.FindStringSubmatch(block); len(sm) == 2 {
		w.UsagePercent, _ = strconv.ParseFloat(sm[1], 64)
	}
	return w
}

func ParseModelDays(body string) []model.ModelDay {
	matches := reDay.FindAllStringSubmatch(body, -1)
	out := make([]model.ModelDay, 0, len(matches))
	seen := map[string]struct{}{}
	for _, m := range matches {
		if len(m) != 4 {
			continue
		}
		units, err := strconv.ParseFloat(m[3], 64)
		if err != nil {
			continue
		}
		day := model.ModelDay{
			Date:  m[1],
			Model: m[2],
			USD:   units / CostUnitsPerUSD,
		}
		key := day.Date + "\t" + day.Model
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, day)
	}
	return out
}

func MergeDays(lists ...[]model.ModelDay) []model.ModelDay {
	out := make([]model.ModelDay, 0)
	seen := map[string]struct{}{}
	for _, list := range lists {
		for _, d := range list {
			key := d.Date + "\t" + d.Model
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, d)
		}
	}
	return out
}

func FilterYearMonths(days []model.ModelDay, months ...string) []model.ModelDay {
	if len(months) == 0 {
		return days
	}
	ok := map[string]struct{}{}
	for _, m := range months {
		ok[m] = struct{}{}
	}
	out := make([]model.ModelDay, 0, len(days))
	for _, d := range days {
		if len(d.Date) < 7 {
			continue
		}
		if _, hit := ok[d.Date[:7]]; hit {
			out = append(out, d)
		}
	}
	return out
}

func AggregateMonth(days []model.ModelDay, yearMonth string) []model.ModelSpend {
	sum := map[string]float64{}
	order := []string{}
	for _, d := range days {
		if yearMonth != "" && !strings.HasPrefix(d.Date, yearMonth) {
			continue
		}
		if _, ok := sum[d.Model]; !ok {
			order = append(order, d.Model)
		}
		sum[d.Model] += d.USD
	}
	out := make([]model.ModelSpend, 0, len(order))
	for _, id := range order {
		out = append(out, model.ModelSpend{
			Model:    id,
			USD:      sum[id],
			LimitUSD: 0,
		})
	}
	return out
}

func MonthSpend(days []model.ModelDay, yearMonth, modelID string) float64 {
	modelID = gomodel.Normalize(modelID)
	var n float64
	for _, d := range days {
		if yearMonth != "" && !strings.HasPrefix(d.Date, yearMonth) {
			continue
		}
		if gomodel.Normalize(d.Model) == modelID {
			n += d.USD
		}
	}
	return n
}
