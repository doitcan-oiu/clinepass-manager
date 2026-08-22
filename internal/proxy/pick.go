package proxy

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"opencode-go-manager/internal/gomodel"
	"opencode-go-manager/internal/model"
)

const defaultMaxRetries = 3
const maxRetryCap = 32

type balancer struct {
	mu       sync.Mutex
	cool     map[string]time.Time
	inflight map[string]int
	lastPick map[string]time.Time
}

func newBalancer() *balancer {
	return &balancer{
		cool:     map[string]time.Time{},
		inflight: map[string]int{},
		lastPick: map[string]time.Time{},
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
	b.mu.Lock()
	defer b.mu.Unlock()
	ranked := b.rankLocked(accounts, modelID, skip, time.Now())
	if len(ranked) == 0 {
		return model.PoolAccount{}, false
	}
	a := ranked[0]
	id := accountID(a)
	b.inflight[id]++
	b.lastPick[id] = time.Now()
	return a, true
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

func modelSpend(u model.AccountUsage, modelID string) float64 {
	id := gomodel.Normalize(modelID)
	for _, m := range u.Models {
		if gomodel.Normalize(m.Model) == id {
			return m.USD
		}
	}
	return 0
}

func canServeQuota(a model.PoolAccount, modelID string, now time.Time) bool {
	if strings.TrimSpace(a.APIKey) == "" {
		return false
	}
	if a.Usage.MonthlyExpired(now.Unix()) {
		return false
	}
	return !a.Usage.QuotaExhausted()
}

func unavailableReason(accounts []model.PoolAccount, now time.Time) string {
	if len(accounts) == 0 {
		return "没有可用账号"
	}
	var noKey, expired, exhausted, cooling int
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
		if lb.cooling(accountID(a), now) {
			cooling++
			continue
		}
	}
	switch {
	case exhausted == len(accounts)-noKey && exhausted > 0:
		return "没有可用账号：滚动/周/月配额已用尽"
	case cooling > 0 && exhausted+cooling+noKey+expired == len(accounts):
		return "没有可用账号：密钥暂时不可用（401）"
	case noKey == len(accounts):
		return "没有可用账号：缺少 API Key"
	default:
		return "没有可用账号"
	}
}

func (b *balancer) rankLocked(accounts []model.PoolAccount, modelID string, skip map[string]bool, now time.Time) []model.PoolAccount {
	out := make([]model.PoolAccount, 0, len(accounts))
	for _, a := range accounts {
		id := accountID(a)
		if skip[id] || b.cooling(id, now) || !canServeQuota(a, modelID, now) {
			continue
		}
		out = append(out, a)
	}
	mid := gomodel.Normalize(modelID)
	sort.SliceStable(out, func(i, j int) bool {
		a, c := out[i], out[j]
		as, cs := windowScore(a.Usage.Rolling, a.Usage.SyncedAt), windowScore(c.Usage.Rolling, c.Usage.SyncedAt)
		if as != cs {
			return as < cs
		}
		as, cs = windowScore(a.Usage.Weekly, a.Usage.SyncedAt), windowScore(c.Usage.Weekly, c.Usage.SyncedAt)
		if as != cs {
			return as < cs
		}
		as, cs = windowScore(a.Usage.Monthly, a.Usage.SyncedAt), windowScore(c.Usage.Monthly, c.Usage.SyncedAt)
		if as != cs {
			return as < cs
		}
		if mid != "" {
			sa, sc := modelSpend(a.Usage, mid), modelSpend(c.Usage, mid)
			if sa != sc {
				return sa < sc
			}
		}
		aid, cid := accountID(a), accountID(c)
		ia, ic := b.inflight[aid], b.inflight[cid]
		if ia != ic {
			return ia < ic
		}
		ta, tc := b.lastPick[aid], b.lastPick[cid]
		if !ta.Equal(tc) {
			return ta.Before(tc)
		}
		return a.Email < c.Email
	})
	return out
}

func Rank(accounts []model.PoolAccount, modelID string) []model.PoolAccount {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return lb.rankLocked(accounts, modelID, nil, time.Now())
}

func windowScore(w model.UsageWindow, synced int64) float64 {
	if w.Status == "" && synced == 0 {
		return 1000
	}
	return w.UsagePercent
}

func retryableStatus(code int) bool {
	switch code {
	case 401, 402, 408, 409, 429, 500, 502, 503, 504, 529:
		return true
	default:
		return false
	}
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

