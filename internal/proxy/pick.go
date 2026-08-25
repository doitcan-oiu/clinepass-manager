package proxy

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"opencode-go-manager/internal/model"
)

const defaultMaxRetries = 3
const maxRetryCap = 32
const defaultAccountRPM = 5
const maxAccountRPM = 1000
const rpmWindow = time.Minute

type balancer struct {
	mu         sync.Mutex
	cool       map[string]time.Time
	inflight   map[string]int
	lastPick   map[string]time.Time
	refreshing map[string]int
	picks      map[string][]time.Time
}

func newBalancer() *balancer {
	return &balancer{
		cool:       map[string]time.Time{},
		inflight:   map[string]int{},
		lastPick:   map[string]time.Time{},
		refreshing: map[string]int{},
		picks:      map[string][]time.Time{},
	}
}

var lb = newBalancer()

func resetBalancer() {
	lb = newBalancer()
}

func accountID(a model.PoolAccount) string {
	if id := strings.TrimSpace(a.ID); id != "" {
		return id
	}
	return a.Email
}

func (b *balancer) begin(id string) {
	if id == "" {
		return
	}
	b.mu.Lock()
	b.inflight[id]++
	b.lastPick[id] = time.Now()
	b.mu.Unlock()
}

func (b *balancer) end(id string) {
	if id == "" {
		return
	}
	b.mu.Lock()
	if b.inflight[id] > 0 {
		b.inflight[id]--
	}
	b.mu.Unlock()
}

func (b *balancer) hold(id string) {
	if id == "" {
		return
	}
	b.mu.Lock()
	b.refreshing[id]++
	b.mu.Unlock()
}

func (b *balancer) unhold(id string) {
	if id == "" {
		return
	}
	b.mu.Lock()
	if b.refreshing[id] > 0 {
		b.refreshing[id]--
		if b.refreshing[id] == 0 {
			delete(b.refreshing, id)
		}
	}
	b.mu.Unlock()
}

func (b *balancer) holding(id string) bool {
	return b.refreshing[id] > 0
}

func (b *balancer) cooldown(id string, d time.Duration) {
	if id == "" || d <= 0 {
		return
	}
	b.mu.Lock()
	b.cool[id] = time.Now().Add(d)
	b.mu.Unlock()
}

func (b *balancer) cooling(id string, now time.Time) bool {
	until, ok := b.cool[id]
	if !ok {
		return false
	}
	if now.Before(until) {
		return true
	}
	delete(b.cool, id)
	return false
}

func (b *balancer) reserve(accounts []model.PoolAccount, modelID string, skip map[string]bool) (model.PoolAccount, bool) {
	return b.reserveWithRPM(accounts, modelID, skip, defaultAccountRPM)
}

func (b *balancer) reserveWithRPM(accounts []model.PoolAccount, modelID string, skip map[string]bool, rpm int) (model.PoolAccount, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	ranked := b.rankLocked(accounts, modelID, skip, now, rpm)
	if len(ranked) == 0 {
		return model.PoolAccount{}, false
	}
	a := ranked[0]
	id := accountID(a)
	b.inflight[id]++
	b.lastPick[id] = now
	b.recordPickLocked(id, now)
	return a, true
}

func clampAccountRPM(n int) int {
	if n < 1 {
		return defaultAccountRPM
	}
	if n > maxAccountRPM {
		return maxAccountRPM
	}
	return n
}

func (b *balancer) prunePicksLocked(id string, now time.Time) {
	xs := b.picks[id]
	if len(xs) == 0 {
		return
	}
	cutoff := now.Add(-rpmWindow)
	i := 0
	for i < len(xs) && !xs[i].After(cutoff) {
		i++
	}
	if i == 0 {
		return
	}
	if i >= len(xs) {
		delete(b.picks, id)
		return
	}
	b.picks[id] = append([]time.Time{}, xs[i:]...)
}

func (b *balancer) rpmCountLocked(id string, now time.Time) int {
	b.prunePicksLocked(id, now)
	return len(b.picks[id])
}

func (b *balancer) rpmFullLocked(id string, now time.Time, rpm int) bool {
	return b.rpmCountLocked(id, now) >= clampAccountRPM(rpm)
}

func (b *balancer) recordPickLocked(id string, now time.Time) {
	b.prunePicksLocked(id, now)
	b.picks[id] = append(b.picks[id], now)
}

func cooldownFor(_ model.PoolAccount, status int, _ http.Header) time.Duration {
	if status == http.StatusUnauthorized {
		return time.Hour
	}
	return 0
}

func parseRetryAfter(h http.Header) time.Duration {
	if h == nil {
		return 0
	}
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil {
		if n < 1 {
			return 0
		}
		d := time.Duration(n) * time.Second
		if d > 15*time.Minute {
			return 15 * time.Minute
		}
		return d
	}
	t, err := http.ParseTime(v)
	if err != nil {
		return 0
	}
	d := time.Until(t)
	if d < time.Second {
		return 0
	}
	if d > 15*time.Minute {
		return 15 * time.Minute
	}
	return d
}

func canServeQuota(a model.PoolAccount, now time.Time) bool {
	if strings.TrimSpace(a.APIKey) == "" {
		return false
	}
	if a.Usage.MonthlyExpired(now.Unix()) {
		return false
	}
	return !a.Usage.QuotaExhausted()
}

func unavailableReason(accounts []model.PoolAccount, now time.Time) string {
	return unavailableReasonRPM(accounts, now, defaultAccountRPM)
}

func unavailableReasonRPM(accounts []model.PoolAccount, now time.Time, rpm int) string {
	if len(accounts) == 0 {
		return "没有可用账号"
	}
	var noKey, expired, exhausted, cooling, rpmLimited int
	lb.mu.Lock()
	defer lb.mu.Unlock()
	for _, a := range accounts {
		if strings.TrimSpace(a.APIKey) == "" {
			noKey++
			continue
		}
		if a.Usage.MonthlyExpired(now.Unix()) {
			expired++
			continue
		}
		if a.Usage.QuotaExhausted() {
			exhausted++
			continue
		}
		id := accountID(a)
		if lb.cooling(id, now) {
			cooling++
			continue
		}
		if lb.rpmFullLocked(id, now, rpm) {
			rpmLimited++
			continue
		}
	}
	switch {
	case exhausted == len(accounts)-noKey && exhausted > 0:
		return "没有可用账号：滚动/周/月配额已用尽"
	case cooling > 0 && exhausted+cooling+noKey+expired == len(accounts):
		return "没有可用账号：密钥暂时不可用（401）"
	case rpmLimited > 0 && exhausted+cooling+noKey+expired+rpmLimited == len(accounts):
		return "没有可用账号：已达单账号 RPM 上限"
	case noKey == len(accounts):
		return "没有可用账号：缺少 API Key"
	default:
		return "没有可用账号"
	}
}

func (b *balancer) rankLocked(accounts []model.PoolAccount, modelID string, skip map[string]bool, now time.Time, rpm int) []model.PoolAccount {
	out := make([]model.PoolAccount, 0, len(accounts))
	for _, a := range accounts {
		id := accountID(a)
		if skip[id] || b.cooling(id, now) || b.holding(id) || !canServeQuota(a, now) || b.rpmFullLocked(id, now, rpm) {
			continue
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		aid, cid := accountID(out[i]), accountID(out[j])
		ia, ic := b.inflight[aid], b.inflight[cid]
		if ia != ic {
			return ia < ic
		}
		ta, tc := b.lastPick[aid], b.lastPick[cid]
		if !ta.Equal(tc) {
			return ta.Before(tc)
		}
		return out[i].Email < out[j].Email
	})
	return out
}

func Rank(accounts []model.PoolAccount, modelID string) []model.PoolAccount {
	return RankWithRPM(accounts, modelID, defaultAccountRPM)
}

func RankWithRPM(accounts []model.PoolAccount, modelID string, rpm int) []model.PoolAccount {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return lb.rankLocked(accounts, modelID, nil, time.Now(), rpm)
}

func InflightOf(id string) int {
	if id == "" {
		return 0
	}
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return lb.inflight[id]
}

func AttachInflight(list []model.PoolAccount) {
	if len(list) == 0 {
		return
	}
	lb.mu.Lock()
	defer lb.mu.Unlock()
	for i := range list {
		list[i].Inflight = lb.inflight[accountID(list[i])]
	}
}

func retryableStatus(code int) bool {
	switch code {
	case 401, 402, 408, 409, 429, 500, 502, 503, 504, 529:
		return true
	default:
		return false
	}
}

func isHTMLRateLimit(status int, body []byte, contentType string) bool {
	if status != http.StatusTooManyRequests {
		return false
	}
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "text/html") {
		return true
	}
	s := strings.TrimSpace(string(body))
	if s == "" {
		return false
	}
	low := strings.ToLower(s)
	if strings.Contains(low, "<!doctype") || strings.Contains(low, "<html") {
		return true
	}
	return extractJSONError(body) == "" && strings.Contains(low, "too many requests")
}

func isPermanentModelError(body []byte) bool {
	msg := strings.ToLower(errorMessage(body, ""))
	if msg == "" {
		return false
	}
	for _, p := range []string{
		"no endpoints found that support image",
		"does not support image",
		"image input is not supported",
		"vision is not supported",
	} {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

func maxAttemptsFromSettings(retries int) int {
	if retries < 0 {
		retries = defaultMaxRetries
	}
	if retries > maxRetryCap {
		retries = maxRetryCap
	}
	return retries + 1
}
