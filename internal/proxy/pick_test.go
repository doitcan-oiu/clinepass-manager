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
	if len(got) != 6 {
		t.Fatalf("len=%d emails=%v", len(got), emails(got))
	}
	if got[0].Email != "a@x.com" || got[1].Email != "b@x.com" || got[2].Email != "c@x.com" || got[3].Email != "d@x.com" || got[4].Email != "f@x.com" || got[5].Email != "g@x.com" {
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
	if len(got) != 3 {
		t.Fatalf("rolling/weekly percent must not skip, got %v", emails(got))
	}
	for _, a := range got {
		if a.Email == "m@x.com" {
			t.Fatalf("monthly done still picked: %v", emails(got))
		}
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

func TestRankKeepsCookieExpiredWhenWindowsHaveRoom(t *testing.T) {
	resetBalancer()
	stale := acc("stale@x.com", "k", win("ok", 40), win("ok", 40), win("ok", 10), "glm-5.3", 1)
	stale.Usage.CookieExpired = true
	stale.Usage.Error = "Cookie 失效，被重定向到登录"
	got := Rank([]model.PoolAccount{stale}, "glm-5.3")
	if len(got) != 1 || got[0].Email != "stale@x.com" {
		t.Fatalf("cookie-expired with room should still schedule, got %v", emails(got))
	}
}

func TestRankSkipsCookieExpiredWeeklyHold(t *testing.T) {
	resetBalancer()
	stale := acc("stale@x.com", "k", win("ok", 10), win("ok", 100), win("ok", 40), "glm-5.3", 1)
	stale.Usage.CookieExpired = true
	stale.Usage.HoldKind = model.HoldWeekly
	stale.Usage.HoldUntil = time.Now().Add(3*24*time.Hour + 7*time.Hour).Unix()
	got := Rank([]model.PoolAccount{stale}, "glm-5.3")
	if len(got) != 0 {
		t.Fatalf("weekly hold must wait even without cookie, got %v", emails(got))
	}
}

func TestRankKeepsWeeklyPercentWithout429Hold(t *testing.T) {
	resetBalancer()
	full := acc("w@x.com", "k", win("ok", 100), win("ok", 100), win("ok", 40), "glm-5.3", 1)
	got := Rank([]model.PoolAccount{full}, "glm-5.3")
	if len(got) != 1 {
		t.Fatalf("usage percent alone must not stop scheduling, got %v", emails(got))
	}
}

func TestRankSkipsMonthlyDone(t *testing.T) {
	resetBalancer()
	done := acc("done@x.com", "k", win("ok", 10), win("ok", 10), win("ok", 100), "glm-5.3", 1)
	got := Rank([]model.PoolAccount{done}, "glm-5.3")
	if len(got) != 0 {
		t.Fatalf("monthly $50 done must drop, got %v", emails(got))
	}
}

func TestClassifyQuotaLimit(t *testing.T) {
	if ClassifyQuotaLimit("You have reached your 5-hour Clinepass limit") != model.HoldRolling {
		t.Fatal("rolling")
	}
	if ClassifyQuotaLimit("Error 429: You have reached your weekly Clinepass limit. The limit resets in 3d 7h, please try again later.") != model.HoldWeekly {
		t.Fatal("weekly")
	}
	if ClassifyQuotaLimit("monthly plan limit") != model.HoldMonthly {
		t.Fatal("monthly")
	}
	if ClassifyQuotaLimit("rate") != "" {
		t.Fatal("unknown")
	}
}

func TestCooldownForCookieExpired429(t *testing.T) {
	a := acc("stale@x.com", "k", win("ok", 40), win("ok", 10), win("ok", 10), "glm-5.3", 1)
	a.Usage.CookieExpired = true
	if kind, d := cooldownFor(a, http.StatusTooManyRequests, nil, []byte(`{"error":"rate"}`)); kind != "" || d != 10*time.Minute {
		t.Fatalf("unknown stale 429 kind=%s d=%s", kind, d)
	}
	if kind, d := cooldownFor(a, http.StatusTooManyRequests, http.Header{"Retry-After": {"30"}}, []byte(`{"error":"rate"}`)); d != 30*time.Second || kind != "" {
		t.Fatalf("retry-after kind=%s d=%s", kind, d)
	}
	if kind, d := cooldownFor(a, http.StatusTooManyRequests, nil, []byte(`{"error":"You have reached your 5-hour Clinepass limit"}`)); kind != model.HoldRolling || d != 5*time.Hour {
		t.Fatalf("rolling kind=%s d=%s", kind, d)
	}
	if kind, d := cooldownFor(a, http.StatusTooManyRequests, nil, []byte(`{"error":"weekly limit reached"}`)); kind != model.HoldWeekly || d != 7*24*time.Hour {
		t.Fatalf("weekly kind=%s d=%s", kind, d)
	}
	weeklyBody := []byte(`{"error":"Error 429: You have reached your weekly Clinepass limit. The limit resets in 3d 7h, please try again later."}`)
	if kind, d := cooldownFor(a, http.StatusTooManyRequests, nil, weeklyBody); kind != model.HoldWeekly || d != 3*24*time.Hour+7*time.Hour {
		t.Fatalf("weekly reset-in kind=%s d=%s", kind, d)
	}
	fresh := acc("ok@x.com", "k", win("ok", 10), win("ok", 10), win("ok", 10), "glm-5.3", 1)
	if kind, d := cooldownFor(fresh, http.StatusTooManyRequests, nil, []byte(`{"error":"rate"}`)); kind != "" || d != 0 {
		t.Fatalf("fresh unknown 429 kind=%s d=%s", kind, d)
	}
}

func TestParseResetIn(t *testing.T) {
	if d := parseResetIn("The limit resets in 3d 7h, please try again later."); d != 3*24*time.Hour+7*time.Hour {
		t.Fatalf("got %s", d)
	}
	if d := parseResetIn("resets in 4h 12m"); d != 4*time.Hour+12*time.Minute {
		t.Fatalf("got %s", d)
	}
	if d := parseResetIn("no reset"); d != 0 {
		t.Fatalf("got %s", d)
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

func TestHoldSkipsKeyUntilUnhold(t *testing.T) {
	resetBalancer()
	a := acc("a@x.com", "k", win("ok", 10), win("ok", 10), win("ok", 10), "glm-5.3", 1)
	b := acc("b@x.com", "k", win("ok", 10), win("ok", 10), win("ok", 10), "glm-5.3", 1)
	lb.hold(a.ID)
	got := Rank([]model.PoolAccount{a, b}, "glm-5.3")
	if len(got) != 1 || got[0].Email != "b@x.com" {
		t.Fatalf("held key still ranked: %v", emails(got))
	}
	lb.unhold(a.ID)
	got = Rank([]model.PoolAccount{a, b}, "glm-5.3")
	if len(got) != 2 {
		t.Fatalf("after unhold got %v", emails(got))
	}
}

func TestReserveLeastInflightNotLowestUsage(t *testing.T) {
	resetBalancer()
	low := acc("low@x.com", "k", win("ok", 5), win("ok", 5), win("ok", 5), "glm-5.3", 1)
	high := acc("high@x.com", "k", win("ok", 80), win("ok", 80), win("ok", 80), "glm-5.3", 1)
	dead := acc("dead@x.com", "k", win("ok", 100), win("ok", 10), win("ok", 10), "glm-5.3", 1)
	dead.Usage.HoldKind = model.HoldRolling
	dead.Usage.HoldUntil = time.Now().Add(2 * time.Hour).Unix()
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

func TestHTMLRateLimitDetection(t *testing.T) {
	html := []byte(`<!doctype html><meta charset="utf-8"><meta name=viewport content="width=device-width, initial-scale=1"><title>429</title>429 Too Many Requests`)
	if !isHTMLRateLimit(429, html, "text/html") {
		t.Fatal("html 429")
	}
	if !isHTMLRateLimit(429, html, "") {
		t.Fatal("html body without content-type")
	}
	if isHTMLRateLimit(429, []byte(`{"error":"You have reached your 5-hour Clinepass limit"}`), "application/json") {
		t.Fatal("json quota 429 must stay failover")
	}
	if isHTMLRateLimit(500, html, "text/html") {
		t.Fatal("only 429 is edge limit")
	}
}

func TestPermanentModelError(t *testing.T) {
	body := []byte(`{"error":{"message":"inference request failed: failed to invoke model 'deepseek/deepseek-v4-flash' from Openrouter: request failed with status 404: {\"error\":{\"message\":\"No endpoints found that support image input\",\"code\":404}}"}}`)
	if !isPermanentModelError(body) {
		t.Fatal("image input 404 must be permanent")
	}
	if isPermanentModelError([]byte(`{"error":"upstream timeout"}`)) {
		t.Fatal("transient 500 should still retry")
	}
}

func TestReserveRespectsAccountRPM(t *testing.T) {
	resetBalancer()
	a := acc("a@x.com", "k", win("ok", 10), win("ok", 10), win("ok", 10), "glm-5.3", 1)
	list := []model.PoolAccount{a}
	for i := 0; i < 2; i++ {
		if _, ok := lb.reserveWithRPM(list, "glm-5.3", nil, 2); !ok {
			t.Fatalf("reserve %d should succeed", i)
		}
	}
	if _, ok := lb.reserveWithRPM(list, "glm-5.3", nil, 2); ok {
		t.Fatal("third reserve must hit rpm=2")
	}
	got := RankWithRPM(list, "glm-5.3", 2)
	if len(got) != 0 {
		t.Fatalf("ranked while at rpm cap: %v", emails(got))
	}
}

func TestAttachInflight(t *testing.T) {
	resetBalancer()
	a := acc("a@x.com", "k", win("ok", 10), win("ok", 10), win("ok", 10), "glm-5.3", 1)
	if _, ok := lb.reserve([]model.PoolAccount{a}, "glm-5.3", nil); !ok {
		t.Fatal("reserve")
	}
	list := []model.PoolAccount{a}
	AttachInflight(list)
	if list[0].Inflight != 1 {
		t.Fatalf("inflight=%d", list[0].Inflight)
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
