package model

import "testing"

func TestSplitShelved(t *testing.T) {
	ok := PoolAccount{
		AccountPublic: AccountPublic{Email: "ok@x.com"},
		Usage:         AccountUsage{Weekly: UsageWindow{Status: "ok", UsagePercent: 40}},
	}
	weeklyFull := PoolAccount{
		AccountPublic: AccountPublic{Email: "full@x.com"},
		Usage:         AccountUsage{Weekly: UsageWindow{Status: "ok", UsagePercent: 100}},
	}
	weeklyLimited := PoolAccount{
		AccountPublic: AccountPublic{Email: "lim@x.com"},
		Usage:         AccountUsage{Weekly: UsageWindow{Status: "rate-limited", UsagePercent: 90}},
	}
	rollingOnly := PoolAccount{
		AccountPublic: AccountPublic{Email: "roll@x.com"},
		Usage: AccountUsage{
			Rolling: UsageWindow{Status: "ok", UsagePercent: 100},
			Weekly:  UsageWindow{Status: "ok", UsagePercent: 10},
		},
	}
	both := PoolAccount{
		AccountPublic: AccountPublic{Email: "both@x.com"},
		Usage: AccountUsage{
			Rolling: UsageWindow{Status: "ok", UsagePercent: 100},
			Weekly:  UsageWindow{Status: "ok", UsagePercent: 100},
		},
	}
	stale := PoolAccount{
		AccountPublic: AccountPublic{Email: "stale@x.com"},
		Usage: AccountUsage{
			CookieExpired: true,
			Rolling:       UsageWindow{Status: "ok", UsagePercent: 40},
			Error:         "Cookie 失效，被重定向到登录",
		},
	}
	active, weekly, rolling, expired := SplitShelved([]PoolAccount{ok, weeklyFull, weeklyLimited, rollingOnly, both, stale})
	if len(active) != 1 || active[0].Email != "ok@x.com" {
		t.Fatalf("active %+v", emails(active))
	}
	if len(weekly) != 3 || weekly[0].Email != "full@x.com" || weekly[1].Email != "lim@x.com" || weekly[2].Email != "both@x.com" {
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
