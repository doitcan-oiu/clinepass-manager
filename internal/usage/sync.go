package usage

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"opencode-go-manager/internal/model"
	"opencode-go-manager/internal/store"
)

type Syncer struct {
	store *store.Store
	mu    sync.Mutex
	st    model.UsageSyncStatus
}

func NewSyncer(st *store.Store) *Syncer {
	return &Syncer{store: st}
}

func (s *Syncer) Status() model.UsageSyncStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.st
}

func (s *Syncer) StartAll() error {
	_, _ = s.store.DeleteExpiredAccounts(time.Now().Unix())
	list, err := s.store.ListPoolAccountsRaw()
	if err != nil {
		return err
	}
	return s.start(list, "全部已付款账号")
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
	return s.start(scan, "本批次")
}

func (s *Syncer) One(id string) (model.AccountUsage, error) {
	a, err := s.store.Get(id)
	if err != nil {
		return model.AccountUsage{}, fmt.Errorf("账号不存在")
	}
	u, _, err := s.syncOne(a)
	return u, err
}

func (s *Syncer) start(list []model.Account, label string) error {
	s.mu.Lock()
	if s.st.Running {
		s.mu.Unlock()
		return fmt.Errorf("正在刷新用量，请稍后再试")
	}
	s.st = model.UsageSyncStatus{Running: true, Total: len(list), Message: "正在扫描 " + label}
	s.mu.Unlock()
	go s.run(list)
	return nil
}

func (s *Syncer) run(list []model.Account) {
	defer func() {
		s.mu.Lock()
		s.st.Running = false
		s.st.Message = fmt.Sprintf("完成，已支付 %d，未支付 %d，失败 %d", s.st.Paid, s.st.Unpaid, s.st.Fail)
		s.mu.Unlock()
	}()
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, a := range list {
		a := a
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			_, subscribed, err := s.syncOne(a)
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

func (s *Syncer) syncOne(a model.Account) (model.AccountUsage, bool, error) {
	if prev, err := s.store.GetAccountUsage(a.ID); err == nil && prev.MonthlyExpired(time.Now().Unix()) {
		_ = s.store.Delete(a.ID)
		return model.AccountUsage{}, false, fmt.Errorf("月配额已到期，账号已删除")
	}
	st, _ := s.store.GetSettings()
	proxy := strings.TrimSpace(a.Proxy)
	if proxy == "" {
		proxy = st.Proxy
	}
	if strings.TrimSpace(a.CookieHeader) != "" && (strings.TrimSpace(a.UserID) == "" || strings.TrimSpace(a.APIKey) == "") {
		if herr := Hydrate(&a, proxy); herr != nil && strings.TrimSpace(a.UserID) == "" && strings.TrimSpace(a.WorkspaceID) == "" {
			_ = s.store.SaveAccountUsage(a.ID, model.AccountUsage{Error: herr.Error(), SyncedAt: time.Now().Unix()})
			return model.AccountUsage{}, false, herr
		}
		if strings.TrimSpace(a.UserID) != "" || strings.TrimSpace(a.APIKey) != "" || strings.TrimSpace(a.WorkspaceID) != "" {
			_ = s.store.SaveLoginResult(a)
		}
	}
	u, subscribed, err := FetchAccount(a, proxy)
	if err != nil {
		_ = s.store.SaveAccountUsage(a.ID, model.AccountUsage{
			Error:    err.Error(),
			SyncedAt: time.Now().Unix(),
		})
		return model.AccountUsage{}, false, err
	}
	_ = s.store.SetAccountPaid(a.ID, subscribed)
	if err := s.store.SaveAccountUsage(a.ID, u); err != nil {
		return u, subscribed, err
	}
	return u, subscribed, nil
}
