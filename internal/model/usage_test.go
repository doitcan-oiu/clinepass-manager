package model

import "testing"

func TestSplitShelved(t *testing.T) {
	ok := PoolAccount{
		AccountPublic: AccountPublic{Email: "ok@x.com"},
		Usage:         AccountUsage{Weekly: UsageWindow{Status: "ok", UsagePercent: 100}},
	}
	weeklyHold := PoolAccount{
		AccountPublic: AccountPublic{Email: "week@x.com"},
		Usage: AccountUsage{
			HoldKind:  HoldWeekly,
			HoldUntil: 1 << 40,
			Weekly:    UsageWindow{Status: "ok", UsagePercent: 10},
		},
	}
	rollingHold := PoolAccount{
		AccountPublic: AccountPublic{Email: "roll@x.com"},
		Usage: AccountUsage{
			HoldKind:  HoldRolling,
			HoldUntil: 1 << 40,
			Rolling:   UsageWindow{Status: "ok", UsagePercent: 10},
		},
	}
	stale := PoolAccount{
		AccountPublic: AccountPublic{Email: "stale@x.com"},
		Usage: AccountUsage{
			CookieExpired: true,
			Rolling:       UsageWindow{Status: "ok", UsagePercent: 100},
			Error:         "Cookie 失效，被重定向到登录",
		},
	}
	active, weekly, rolling, expired := SplitShelved([]PoolAccount{ok, weeklyHold, rollingHold, stale})
	if len(active) != 1 || active[0].Email != "ok@x.com" {
		t.Fatalf("active %+v", emails(active))
	}
	if len(weekly) != 1 || weekly[0].Email != "week@x.com" {
		t.Fatalf("weekly %+v", emails(weekly))
	}
	if len(rolling) != 1 || rolling[0].Email != "roll@x.com" {
		t.Fatalf("rolling %+v", emails(rolling))
	}
	if len(expired) != 1 || expired[0].Email != "stale@x.com" {
		t.Fatalf("expired %+v", emails(expired))
	}
}

func TestCookieExpiredMessage(t *testing.T) {
	if !CookieExpiredMessage("Cookie 失效，被重定向到登录") {
		t.Fatal("redirect")
	}
	if !CookieExpiredMessage("Cookie 已过期（手动标记）") {
		t.Fatal("manual")
	}
	if !CookieExpiredMessage("缺少 Cookie") {
		t.Fatal("missing")
	}
	if CookieExpiredMessage("滚动额度已满") {
		t.Fatal("quota")
	}
	u := AccountUsage{CookieExpired: true, Error: ""}
	if !u.CookieStale() {
		t.Fatal("flag")
	}
}

func emails(list []PoolAccount) []string {
	out := make([]string, len(list))
	for i, a := range list {
		out[i] = a.Email
	}
	return out
}
