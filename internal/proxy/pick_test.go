package proxy

import (
	"net/http"
	"testing"
	"time"

	"opencode-go-manager/internal/model"
)

func TestRankSkipsExhaustedKeepsUsable(t *testing.T) {
	resetBalancer()
	limited := acc("a@x.com", "k1", win("ok", 10), win("rate-limited", 100), win("ok", 10), "glm-5.3", 1)
	fullPct := acc("f@x.com", "k6", win("ok", 100), win("ok", 10), win("ok", 10), "glm-5.3", 1)
	overModel := acc("b@x.com", "k2", win("ok", 5), win("ok", 10), win("ok", 20), "glm-5.3", 15)
	overPool := acc("g@x.com", "k7", win("ok", 5), win("ok", 10), win("ok", 20), "qwen3.7-plus", 60)
	best := acc("c@x.com", "k3", win("ok", 8), win("ok", 20), win("ok", 30), "glm-5.3", 2)
	busy := acc("d@x.com", "k4", win("ok", 40), win("ok", 20), win("ok", 30), "glm-5.3", 1)
	noKey := acc("e@x.com", "", win("ok", 0), win("ok", 0), win("ok", 0), "glm-5.3", 0)

	got := Rank([]model.PoolAccount{limited, fullPct, overModel, overPool, busy, best, noKey}, "glm-5.3")
	if len(got) != 4 {
		t.Fatalf("len=%d emails=%v", len(got), emails(got))
	}
	if got[0].Email != "b@x.com" || got[1].Email != "c@x.com" || got[2].Email != "d@x.com" || got[3].Email != "g@x.com" {
		t.Fatalf("order %v", emails(got))
	}
}

func TestRankSkipsRoundedHundredPercent(t *testing.T) {
	resetBalancer()
	rolling := acc("r@x.com", "k", win("ok", 99.5), win("ok", 10), win("ok", 10), "glm-5.3", 1)
	weekly := acc("w@x.com", "k", win("ok", 10), win("ok", 99.6), win("ok", 10), "glm-5.3", 1)
	monthly := acc("m@x.com", "k", win("ok", 10), win("ok", 10), win("ok", 100), "glm-5.3", 1)
	ok := acc("ok@x.com", "k", win("ok", 99.4), win("ok", 10), win("ok", 10), "glm-5.3", 1)
	got := Rank([]model.PoolAccount{rolling, weekly, monthly, ok}, "glm-5.3")
	if len(got) != 1 || got[0].Email != "ok@x.com" {
		t.Fatalf("got %v", emails(got))
	}
}

func TestRankUnknownModelUsesPackageOnly(t *testing.T) {
	resetBalancer()
	a := acc("a@x.com", "k", win("ok", 50), win("ok", 10), win("ok", 10), "glm-5.3", 2)
	b := acc("b@x.com", "k", win("ok", 10), win("ok", 10), win("ok", 10), "glm-5.3", 2)
	got := Rank([]model.PoolAccount{a, b}, "unknown-model")
	if len(got) != 2 {
		t.Fatalf("usable keys should stay eligible, got %v", emails(got))
	}
}

func TestRankKeepsKeyWhenWindowsHaveRoom(t *testing.T) {
	resetBalancer()
	a := model.PoolAccount{
		AccountPublic: model.AccountPublic{ID: "a", Email: "a@x.com", APIKey: "k"},
		Usage: model.AccountUsage{
			SyncedAt: time.Now().Unix(),
			Rolling:  win("ok", 10),
			Weekly:   win("ok", 10),
			Monthly:  win("ok", 40),
			Models: []model.ModelSpend{
				{Model: "glm-5.3", USD: 0.5, LimitUSD: 15},
				{Model: "grok-4.5", USD: 59.5, LimitUSD: 15},
			},
		},
	}
	got := Rank([]model.PoolAccount{a}, "glm-5.3")
	if len(got) != 1 {
		t.Fatalf("windows not full should keep key, got %v", emails(got))
	}
}

func TestRankKeepsKeyWhenPoolAndModelHaveRoom(t *testing.T) {
	resetBalancer()
	a := model.PoolAccount{
		AccountPublic: model.AccountPublic{ID: "a", Email: "a@x.com", APIKey: "k"},
		Usage: model.AccountUsage{
			SyncedAt: time.Now().Unix(),
			Rolling:  win("ok", 10),
			Weekly:   win("ok", 10),
			Monthly:  win("ok", 40),
			Models: []model.ModelSpend{
				{Model: "glm-5.3", USD: 0.5, LimitUSD: 15},
				{Model: "grok-4.5", USD: 10, LimitUSD: 15},
			},
		},
	}
	got := Rank([]model.PoolAccount{a}, "glm-5.3")
	if len(got) != 1 {
		t.Fatalf("should keep key with remaining pool, got %v", emails(got))
	}
}

func TestParseRetryAfter(t *testing.T) {
	if d := parseRetryAfter(http.Header{"Retry-After": {"45"}}); d != 45*time.Second {
		t.Fatalf("got %s", d)
	}
	if d := parseRetryAfter(http.Header{"Retry-After": {"9999"}}); d != 15*time.Minute {
		t.Fatalf("cap got %s", d)
	}
}

func TestCooldownSkipsKey(t *testing.T) {
	resetBalancer()
	a := acc("a@x.com", "k", win("ok", 1), win("ok", 1), win("ok", 1), "glm-5.3", 1)
	b := acc("b@x.com", "k", win("ok", 2), win("ok", 1), win("ok", 1), "glm-5.3", 1)
	lb.cooldown(a.ID, time.Hour)
	got := Rank([]model.PoolAccount{a, b}, "glm-5.3")
	if len(got) != 1 || got[0].Email != "b@x.com" {
		t.Fatalf("%+v", got)
	}
}

func TestReserveAllowsConcurrentSameAccount(t *testing.T) {
	resetBalancer()
	a := acc("a@x.com", "k", win("ok", 10), win("ok", 10), win("ok", 10), "glm-5.3", 1)
	first, ok := lb.reserve([]model.PoolAccount{a}, "glm-5.3", nil)
	if !ok {
		t.Fatal("first reserve")
	}
	second, ok := lb.reserve([]model.PoolAccount{a}, "glm-5.3", nil)
	if !ok {
		t.Fatal("same account must allow concurrent reserve")
	}
	lb.end(accountID(first))
	lb.end(accountID(second))
	if first.Email != "a@x.com" || second.Email != "a@x.com" {
		t.Fatalf("%s %s", first.Email, second.Email)
	}
}

func TestReserveSpreadsConcurrentPicks(t *testing.T) {
	resetBalancer()
	a := acc("a@x.com", "k", win("ok", 10), win("ok", 10), win("ok", 10), "glm-5.3", 1)
	b := acc("b@x.com", "k", win("ok", 10), win("ok", 10), win("ok", 10), "glm-5.3", 1)
	list := []model.PoolAccount{a, b}
	first, ok := lb.reserve(list, "glm-5.3", nil)
	if !ok {
		t.Fatal("first reserve")
	}
	second, ok := lb.reserve(list, "glm-5.3", nil)
	if !ok {
		t.Fatal("second reserve")
	}
	lb.end(accountID(first))
	lb.end(accountID(second))
	if first.Email == second.Email {
		t.Fatalf("same key picked twice: %s", first.Email)
	}
}

func TestInflightPrefersIdleKey(t *testing.T) {
	resetBalancer()
	a := acc("a@x.com", "k", win("ok", 10), win("ok", 10), win("ok", 10), "glm-5.3", 1)
	b := acc("b@x.com", "k", win("ok", 10), win("ok", 10), win("ok", 10), "glm-5.3", 1)
	lb.begin(a.ID)
	got := Rank([]model.PoolAccount{a, b}, "glm-5.3")
	lb.end(a.ID)
	if len(got) != 2 || got[0].Email != "b@x.com" {
		t.Fatalf("order %v", emails(got))
	}
}

func TestReserveLeastInflightNotLowestUsage(t *testing.T) {
	resetBalancer()
	low := acc("low@x.com", "k", win("ok", 5), win("ok", 5), win("ok", 5), "glm-5.3", 1)
	high := acc("high@x.com", "k", win("ok", 80), win("ok", 80), win("ok", 80), "glm-5.3", 1)
	dead := acc("dead@x.com", "k", win("ok", 100), win("ok", 10), win("ok", 10), "glm-5.3", 1)
	list := []model.PoolAccount{low, high, dead}

	var picks []string
	for i := 0; i < 4; i++ {
		a, ok := lb.reserve(list, "glm-5.3", nil)
		if !ok {
			t.Fatalf("reserve %d failed", i)
		}
		picks = append(picks, a.Email)
	}
	for _, email := range picks {
		lb.end(email)
	}
	if picks[0] != "high@x.com" && picks[0] != "low@x.com" {
		t.Fatalf("first pick %v", picks)
	}
	for i := 1; i < len(picks); i++ {
		if picks[i] == picks[i-1] {
			t.Fatalf("stuck on one key: %v", picks)
		}
		if picks[i] == "dead@x.com" {
			t.Fatalf("exhausted key picked: %v", picks)
		}
	}
}

func TestReserveSpreadsSequentialAfterEnd(t *testing.T) {
	resetBalancer()
	a := acc("a@x.com", "k", win("ok", 10), win("ok", 10), win("ok", 10), "glm-5.3", 1)
	b := acc("b@x.com", "k", win("ok", 80), win("ok", 10), win("ok", 10), "glm-5.3", 1)
	list := []model.PoolAccount{a, b}
	first, ok := lb.reserve(list, "glm-5.3", nil)
	if !ok {
		t.Fatal("first reserve")
	}
	lb.end(accountID(first))
	second, ok := lb.reserve(list, "glm-5.3", nil)
	if !ok {
		t.Fatal("second reserve")
	}
	lb.end(accountID(second))
	if first.Email == second.Email {
		t.Fatalf("sequential picks stuck on %s", first.Email)
	}
}

func TestRetryableStatus(t *testing.T) {
	if !retryableStatus(429) || !retryableStatus(502) || retryableStatus(400) {
		t.Fatal("retryable map")
	}
}

func win(status string, pct float64) model.UsageWindow {
	return model.UsageWindow{Status: status, UsagePercent: pct}
}

func acc(email, key string, rolling, weekly, monthly model.UsageWindow, modelID string, usd float64) model.PoolAccount {
	return model.PoolAccount{
		AccountPublic: model.AccountPublic{ID: email, Email: email, APIKey: key},
		Usage: model.AccountUsage{
			SyncedAt: time.Now().Unix(),
			Rolling:  rolling,
			Weekly:   weekly,
			Monthly:  monthly,
			Models:   []model.ModelSpend{{Model: modelID, USD: usd, LimitUSD: 15}},
		},
	}
}

func emails(list []model.PoolAccount) []string {
	out := make([]string, len(list))
	for i, a := range list {
		out[i] = a.Email
	}
	return out
}
