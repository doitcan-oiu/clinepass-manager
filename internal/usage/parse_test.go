package usage

import (
	"math"
	"strings"
	"testing"

	"opencode-go-manager/internal/model"
)

func TestParseWindowsFromGoPage(t *testing.T) {
	// Representative serialized page data belongs with the test. Local captured
	// pages in 相关操作 are ignored and may contain private account information.
	const page = `<html><script>
		window.data = {
			rollingUsage: $R[12] = {status: "ok", resetInSec: 18000, usagePercent: 0},
			weeklyUsage: {status: "rate-limited", resetInSec: 604800, usagePercent: 100},
			monthlyUsage: $R[14] = {status: "ok", resetInSec: 2441872, usagePercent: 55}
		};
	</script></html>`
	rolling, weekly, monthly, err := ParseWindows(page)
	if err != nil {
		t.Fatal(err)
	}
	if rolling.Status != "ok" || rolling.UsagePercent != 0 || rolling.ResetInSec != 18000 {
		t.Fatalf("rolling %+v", rolling)
	}
	if weekly.Status != "rate-limited" || weekly.UsagePercent != 100 {
		t.Fatalf("weekly %+v", weekly)
	}
	if monthly.Status != "ok" || monthly.UsagePercent != 55 || monthly.ResetInSec != 2441872 {
		t.Fatalf("monthly %+v", monthly)
	}
	if PageKind(page) != "subscribed" {
		t.Fatal("go page should be subscribed")
	}
}

func TestPageKindUnpaidAndAuth(t *testing.T) {
	if PageKind(`<html>workspace wrk_01ABC lite.subscription.get</html>`) != "unpaid" {
		t.Fatal("expected unpaid")
	}
	if PageKind(`<a href="/google/authorize">Continue with Google</a>`) != "auth" {
		t.Fatal("expected auth")
	}
}

func TestParseModelDaysFromUsageDoc(t *testing.T) {
	const page = `<html><script>
		window.data = {days: [
			{date: "2026-08-20", model: "glm-5.3", totalCost: 40000000},
			{date: "2026-08-21", model: "glm-5.3", totalCost: 22642336},
			{date: "2026-08-21", model: "kimi-k3", totalCost: 123456789},
			{date: "2026-08-21", model: "qwen3.8-max", totalCost: 0}
		]};
		// Framework hydration can repeat the same daily record.
		window.cache = {date: "2026-08-21", model: "glm-5.3", totalCost: 22642336};
		window.invalid = {date: "2026-08-21", model: "unknown", totalCost: "invalid"};
	</script></html>`
	days := ParseModelDays(page)
	if len(days) != 4 {
		t.Fatalf("days=%d %+v", len(days), days)
	}
	var glm float64
	for _, d := range days {
		if d.Model == "glm-5.3" {
			glm += d.USD
		}
	}
	want := 62642336 / CostUnitsPerUSD
	if math.Abs(glm-want) > 1e-12 {
		t.Fatalf("glm usd=%v want=%v", glm, want)
	}
	if days[2].Model != "kimi-k3" || math.Abs(days[2].USD-1.23456789) > 1e-12 || days[3].USD != 0 {
		t.Fatalf("currency conversion or zero-cost row lost: %+v", days)
	}
	month := AggregateMonth(days, "2026-08")
	if len(month) != 3 {
		t.Fatalf("models=%d %+v", len(month), month)
	}
	if !strings.Contains(strings.Join(func() []string {
		s := make([]string, len(month))
		for i, m := range month {
			s[i] = m.Model
		}
		return s
	}(), ","), "qwen3.8-max") {
		t.Fatalf("missing qwen: %+v", month)
	}
	if MonthSpend(days, "2026-08", "glm-5.3") != glm {
		t.Fatal("month spend mismatch")
	}
}

func TestMergeDaysAndAggregateTwoMonths(t *testing.T) {
	july := []model.ModelDay{{Date: "2026-07-28", Model: "glm-5.3", USD: 1.2}}
	aug := []model.ModelDay{{Date: "2026-08-22", Model: "glm-5.3", USD: 0.5}, {Date: "2026-08-22", Model: "kimi-k3", USD: 2}}
	days := MergeDays(july, aug, july)
	if len(days) != 3 {
		t.Fatalf("days=%d %+v", len(days), days)
	}
	sum := AggregateMonth(days, "")
	var glm, kimi float64
	for _, m := range sum {
		switch m.Model {
		case "glm-5.3":
			glm = m.USD
		case "kimi-k3":
			kimi = m.USD
		}
	}
	if glm != 1.7 || kimi != 2 {
		t.Fatalf("glm=%v kimi=%v %+v", glm, kimi, sum)
	}
	kept := FilterYearMonths(days, "2026-07", "2026-08")
	if len(kept) != 3 {
		t.Fatalf("kept=%d", len(kept))
	}
}
