package usage

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"opencode-go-manager/internal/model"
	"opencode-go-manager/internal/store"
)

const (
	DefaultInterval      = time.Minute
	DefaultModelInterval = 10 * time.Minute
	DefaultConcurrency   = 10
	minInterval          = 15 * time.Second
	maxInterval          = 24 * time.Hour
	minConcurrency       = 1
	maxConcurrency       = 64
)

type Syncer struct {
	store   *store.Store
	fetch   func(model.Account, string, bool) (model.AccountUsage, bool, error)
	mu      sync.Mutex
	st      model.UsageSyncStatus
	kicking map[string]*sync.WaitGroup
}

func NewSyncer(st *store.Store) *Syncer {
	return &Syncer{store: st, kicking: map[string]*sync.WaitGroup{}}
}

func (s *Syncer) doFetch(a model.Account, proxy string, includeModels bool) (model.AccountUsage, bool, error) {
	if s.fetch != nil {
		return s.fetch(a, proxy, includeModels)
	}
	return fetchAccount(a, proxy, "", "", includeModels)
}

func (s *Syncer) Status() model.UsageSyncStatus {
	s.mu.Lock()
	out := s.st
	s.mu.Unlock()
	out.IntervalSec = s.intervalSec()
	out.Concurrency = s.concurrency()
	return out
}

func (s *Syncer) StartAll() error {
	_, _ = s.store.DeleteExpiredAccounts(time.Now().Unix())
	list, err := s.store.ListPoolAccountsRaw()
	if err != nil {
		return err
	}
	return s.start(list, "全部已付款账号", false)
}

func (s *Syncer) StartAllForced() error {
	_, _ = s.store.DeleteExpiredAccounts(time.Now().Unix())
	list, err := s.store.ListPoolAccountsRaw()
	if err != nil {
		return err
	}
	return s.start(list, "全部已付款账号", true)
}

func (s *Syncer) StartBatch(batchID string) error {
	list, err := s.store.ListByBatch(batchID)
	if err != nil {
		return err
	}
	scan := make([]model.Account, 0, len(list))
	for _, a := range list {
		if strings.TrimSpace(a.CookieHeader) != "" {
			scan = append(scan, a)
		}
	}
	return s.start(scan, "本批次", true)
}

func (s *Syncer) One(id string) (model.AccountUsage, error) {
	a, err := s.store.Get(id)
	if err != nil {
		return model.AccountUsage{}, fmt.Errorf("账号不存在")
	}
	u, _, err := s.syncOne(a, true)
	return u, err
}

func (s *Syncer) StartLoop() {
	go func() {
		for {
			_ = s.StartAll()
			s.waitIdle()
			time.Sleep(s.interval())
		}
	}()
}

func (s *Syncer) waitIdle() {
	for {
		s.mu.Lock()
		running := s.st.Running
		s.mu.Unlock()
		if !running {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (s *Syncer) interval() time.Duration {
	d := time.Duration(s.intervalSec()) * time.Second
	if d < minInterval {
		return DefaultInterval
	}
	if d > maxInterval {
		return maxInterval
	}
	return d
}

func (s *Syncer) intervalSec() int {
	st, err := s.store.GetSettings()
	if err != nil || st.UsageRefreshSec < 15 {
		return int(DefaultInterval / time.Second)
	}
	if st.UsageRefreshSec > int(maxInterval/time.Second) {
		return int(maxInterval / time.Second)
	}
	return st.UsageRefreshSec
}

func (s *Syncer) modelIntervalSec() int {
	st, err := s.store.GetSettings()
	if err != nil || st.ModelUsageRefreshSec < 15 {
		return int(DefaultModelInterval / time.Second)
	}
	if st.ModelUsageRefreshSec > int(maxInterval/time.Second) {
		return int(maxInterval / time.Second)
	}
	return st.ModelUsageRefreshSec
}

func modelsStale(prev model.AccountUsage, now int64, intervalSec int) bool {
	if prev.ModelSyncedAt <= 0 {
		return true
	}
	return now-prev.ModelSyncedAt >= int64(intervalSec)
}

func (s *Syncer) concurrency() int {
	st, err := s.store.GetSettings()
	if err != nil || st.UsageRefreshConcurrency < minConcurrency {
		return DefaultConcurrency
	}
	if st.UsageRefreshConcurrency > maxConcurrency {
		return maxConcurrency
	}
	return st.UsageRefreshConcurrency
}

func (s *Syncer) Refresh(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	s.mu.Lock()
	if s.kicking == nil {
		s.kicking = map[string]*sync.WaitGroup{}
	}
	if wg, ok := s.kicking[id]; ok {
		s.mu.Unlock()
		wg.Wait()
		return
	}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	s.kicking[id] = wg
	s.mu.Unlock()

	a, err := s.store.Get(id)
	if err != nil {
		s.mu.Lock()
		delete(s.kicking, id)
		s.mu.Unlock()
		wg.Done()
		return
	}
	_, _, _ = s.syncOne(a, false)

	s.mu.Lock()
	delete(s.kicking, id)
	s.mu.Unlock()
	wg.Done()
}

func (s *Syncer) start(list []model.Account, label string, forceModels bool) error {
	s.mu.Lock()
	if s.st.Running {
		s.mu.Unlock()
		return fmt.Errorf("正在刷新用量，请稍后再试")
	}
	s.st = model.UsageSyncStatus{
		Running:    true,
		Total:      len(list),
		Message:    "正在扫描 " + label,
		StartedAt:  time.Now().Unix(),
		FinishedAt: s.st.FinishedAt,
	}
	s.mu.Unlock()
	go s.run(list, forceModels)
	return nil
}

func (s *Syncer) run(list []model.Account, forceModels bool) {
	defer func() {
		s.mu.Lock()
		s.st.Running = false
		s.st.FinishedAt = time.Now().Unix()
		s.st.Message = fmt.Sprintf("完成，已支付 %d，未支付 %d，失败 %d", s.st.Paid, s.st.Unpaid, s.st.Fail)
		s.mu.Unlock()
	}()
	n := s.concurrency()
	if n < minConcurrency {
		n = DefaultConcurrency
	}
	sem := make(chan struct{}, n)
	var wg sync.WaitGroup
	for _, a := range list {
		a := a
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			_, subscribed, err := s.syncOne(a, forceModels)
			s.mu.Lock()
			s.st.Done++
			if err != nil {
				s.st.Fail++
			} else if subscribed {
				s.st.Paid++
			} else {
				s.st.Unpaid++
			}
			s.mu.Unlock()
		}()
	}
	wg.Wait()
}

func (s *Syncer) syncOne(a model.Account, forceModels bool) (model.AccountUsage, bool, error) {
	prev, prevErr := s.store.GetAccountUsage(a.ID)
	if prevErr == nil && prev.MonthlyDone(time.Now().Unix()) {
		_ = s.store.Delete(a.ID)
		return model.AccountUsage{}, false, fmt.Errorf("月额度已用完，账号已删除")
	}
	if prevErr == nil && prev.CookieStale() && !forceModels {
		return prev, true, nil
	}
	st, _ := s.store.GetSettings()
	proxy := strings.TrimSpace(a.Proxy)
	if proxy == "" {
		proxy = st.Proxy
	}
	if strings.TrimSpace(a.CookieHeader) != "" && (strings.TrimSpace(a.UserID) == "" || strings.TrimSpace(a.APIKey) == "") {
		if herr := Hydrate(&a, proxy); herr != nil && strings.TrimSpace(a.UserID) == "" && strings.TrimSpace(a.WorkspaceID) == "" {
			u := model.AccountUsage{Error: herr.Error(), SyncedAt: time.Now().Unix(), CookieExpired: model.CookieExpiredMessage(herr.Error())}
			_ = s.store.SaveAccountUsage(a.ID, u)
			return u, false, herr
		}
		if strings.TrimSpace(a.UserID) != "" || strings.TrimSpace(a.APIKey) != "" || strings.TrimSpace(a.WorkspaceID) != "" {
			_ = s.store.SaveLoginResult(a)
		}
	}
	includeModels := forceModels || modelsStale(prev, time.Now().Unix(), s.modelIntervalSec())
	u, subscribed, err := s.doFetch(a, proxy, includeModels)
	if err != nil {
		expired := model.CookieExpiredMessage(err.Error())
		if prevErr == nil {
			prev.Error = err.Error()
			prev.SyncedAt = time.Now().Unix()
			if expired {
				prev.CookieExpired = true
			}
			_ = s.store.SaveAccountUsage(a.ID, prev)
			return prev, false, err
		}
		return model.AccountUsage{Error: err.Error(), SyncedAt: time.Now().Unix(), CookieExpired: expired}, false, err
	}
	u.CookieExpired = false
	if prevErr == nil && prev.HoldUntil > time.Now().Unix() {
		u.HoldUntil = prev.HoldUntil
		u.HoldKind = prev.HoldKind
	}
	if u.Days == nil {
		u.Days = prev.Days
		u.Models = prev.Models
		u.ModelSyncedAt = prev.ModelSyncedAt
	}
	if u.Days == nil {
		u.Days = []model.ModelDay{}
	}
	if u.Models == nil {
		u.Models = []model.ModelSpend{}
	}
	_ = s.store.SetAccountPaid(a.ID, subscribed)
	if err := s.store.SaveAccountUsage(a.ID, u); err != nil {
		return u, subscribed, err
	}
	return u, subscribed, nil
}
