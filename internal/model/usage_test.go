package model

import "testing"

func TestSplitWeeklyLimited(t *testing.T) {
	ok := PoolAccount{
		AccountPublic: AccountPublic{Email: "ok@x.com"},
		Usage:         AccountUsage{Weekly: UsageWindow{Status: "ok", UsagePercent: 40}},
	}
	full := PoolAccount{
		AccountPublic: AccountPublic{Email: "full@x.com"},
		Usage:         AccountUsage{Weekly: UsageWindow{Status: "ok", UsagePercent: 100}},
	}
	limited := PoolAccount{
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
	active, weekly := SplitWeeklyLimited([]PoolAccount{ok, full, limited, rollingOnly})
	if len(active) != 2 || active[0].Email != "ok@x.com" || active[1].Email != "roll@x.com" {
		t.Fatalf("active %+v", emails(active))
	}
	if len(weekly) != 2 || weekly[0].Email != "full@x.com" || weekly[1].Email != "lim@x.com" {
		t.Fatalf("weekly %+v", emails(weekly))
	}
}

func emails(list []PoolAccount) []string {
	out := make([]string, len(list))
	for i, a := range list {
		out[i] = a.Email
	}
	return out
}
