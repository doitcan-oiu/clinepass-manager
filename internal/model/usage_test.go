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
	active, weekly, rolling := SplitShelved([]PoolAccount{ok, weeklyFull, weeklyLimited, rollingOnly, both})
	if len(active) != 1 || active[0].Email != "ok@x.com" {
		t.Fatalf("active %+v", emails(active))
	}
	if len(weekly) != 3 || weekly[0].Email != "full@x.com" || weekly[1].Email != "lim@x.com" || weekly[2].Email != "both@x.com" {
		t.Fatalf("weekly %+v", emails(weekly))
	}
	if len(rolling) != 1 || rolling[0].Email != "roll@x.com" {
		t.Fatalf("rolling %+v", emails(rolling))
	}
}

func emails(list []PoolAccount) []string {
	out := make([]string, len(list))
	for i, a := range list {
		out[i] = a.Email
	}
	return out
}
