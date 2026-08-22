package job

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"opencode-go-manager/internal/config"
	"opencode-go-manager/internal/login"
	"opencode-go-manager/internal/model"
	"opencode-go-manager/internal/store"
)

type Manager struct {
	cfg   config.Config
	store *store.Store

	mu      sync.Mutex
	jobs    map[string]*model.Job
	subs    map[string][]chan model.JobEvent
	active  int
	queue   []string
	running map[string]string
}

func New(cfg config.Config, st *store.Store) *Manager {
	if cfg.MaxConcurrent < 1 {
		cfg.MaxConcurrent = 1
	}
	return &Manager{
		cfg:     cfg,
		store:   st,
		jobs:    map[string]*model.Job{},
		subs:    map[string][]chan model.JobEvent{},
		running: map[string]string{},
	}
}

func (m *Manager) List() []model.Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]model.Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		cp := *j
		out = append(out, cp)
	}
	return out
}

func (m *Manager) Get(id string) (*model.Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return nil, false
	}
	cp := *j
	return &cp, true
}

func (m *Manager) Subscribe(jobID string) (chan model.JobEvent, func()) {
	ch := make(chan model.JobEvent, 64)
	m.mu.Lock()
	m.subs[jobID] = append(m.subs[jobID], ch)
	if j, ok := m.jobs[jobID]; ok {
		for _, ev := range j.Logs {
			select {
			case ch <- ev:
			default:
			}
		}
	}
	m.mu.Unlock()
	return ch, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		list := m.subs[jobID]
		n := list[:0]
		for _, c := range list {
			if c != ch {
				n = append(n, c)
			}
		}
		m.subs[jobID] = n
		close(ch)
	}
}

func (m *Manager) Enqueue(accountID string) (*model.Job, error) {
	return m.enqueue(accountID, "login")
}

func (m *Manager) EnqueueRefresh(accountID string) (*model.Job, error) {
	acc, err := m.store.Get(accountID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(acc.CookiesJSON) == "" || strings.TrimSpace(acc.WorkspaceID) == "" {
		return nil, fmt.Errorf("没有可用 Cookie")
	}
	return m.enqueue(accountID, "refresh")
}

func (m *Manager) enqueue(accountID, kind string) (*model.Job, error) {
	acc, err := m.store.Get(accountID)
	if err != nil {
		return nil, err
	}
	if acc.PaidAt > 0 {
		return nil, fmt.Errorf("已经支付，跳过提取")
	}
	if kind == "" {
		kind = "login"
	}
	job := &model.Job{
		ID:        newID(),
		AccountID: acc.ID,
		Email:     acc.Email,
		Kind:      kind,
		Status:    "queued",
		StartedAt: time.Now().Unix(),
	}
	m.mu.Lock()
	m.jobs[job.ID] = job
	m.queue = append(m.queue, job.ID)
	m.mu.Unlock()
	_ = m.store.UpdateStatus(acc.ID, "queued", "")
	m.pump()
	return job, nil
}

func (m *Manager) Pump() {
	m.pump()
}

func (m *Manager) maxConcurrentLocked() int {
	if st, err := m.store.GetSettings(); err == nil && st.MaxConcurrent >= 1 {
		return st.MaxConcurrent
	}
	if m.cfg.MaxConcurrent < 1 {
		return 1
	}
	return m.cfg.MaxConcurrent
}

func (m *Manager) pump() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for m.active < m.maxConcurrentLocked() && len(m.queue) > 0 {
		id := m.queue[0]
		m.queue = m.queue[1:]
		job := m.jobs[id]
		if job == nil {
			continue
		}
		m.active++
		m.running[job.AccountID] = job.ID
		go m.run(job)
	}
}

func (m *Manager) run(job *model.Job) {
	defer func() {
		m.mu.Lock()
		m.active--
		delete(m.running, job.AccountID)
		m.mu.Unlock()
		m.pump()
	}()

	m.setStatus(job, "running", "")
	_ = m.store.UpdateStatus(job.AccountID, "running", "")
	acc, err := m.store.Get(job.AccountID)
	if err != nil {
		m.fail(job, err.Error())
		return
	}
	startMsg := "开始登录 %s"
	if job.Kind == "refresh" {
		startMsg = "开始刷新支付链接 %s"
	}
	m.logf(job, "info", startMsg, acc.Email)

	cfg := m.cfg
	if st, err := m.store.GetSettings(); err == nil {
		cfg = store.ApplySettings(cfg, st)
	}

	var res login.Result
	if job.Kind == "refresh" {
		res, err = login.RefreshPayment(cfg, acc, func(format string, args ...any) {
			m.logf(job, "info", format, args...)
		})
	} else {
		res, err = m.runLoginWithAuthkitRetry(cfg, acc, job)
	}
	if err != nil {
		if errors.Is(err, login.ErrPhoneTimeout) {
			m.logf(job, "info", "手机号超时，跳过")
		}
		if errors.Is(err, login.ErrAccountBanned) {
			m.logf(job, "info", "账号已被封禁，跳过")
		}
		msg := login.CompactMessage(err.Error())
		m.fail(job, msg)
		acc.Status = "failed"
		acc.LastError = msg
		_ = m.store.SaveLoginResult(acc)
		return
	}
	acc.Status = "ready"
	acc.LastError = ""
	acc.WorkspaceID = res.WorkspaceID
	acc.APIKey = res.APIKey
	acc.UserID = res.UserID
	acc.CookiesJSON = res.CookiesJSON
	acc.CookieHeader = res.CookieHeader
	acc.PaymentURL = res.PaymentURL
	if res.CookiesJSON != "" {
		acc.CookiesJSON = res.CookiesJSON
		acc.CookieHeader = res.CookieHeader
	}
	if err := m.store.SaveLoginResult(acc); err != nil {
		m.fail(job, err.Error())
		return
	}
	if job.Kind == "refresh" {
		m.logf(job, "info", "刷新完成")
	} else {
		m.logf(job, "info", "登录完成")
	}
	m.setStatus(job, "success", "")
}

const authkitAccountRetries = 2

func (m *Manager) runLoginWithAuthkitRetry(cfg config.Config, acc model.Account, job *model.Job) (login.Result, error) {
	var last error
	for attempt := 0; attempt <= authkitAccountRetries; attempt++ {
		if attempt > 0 {
			m.logf(job, "info", "AuthKit 页面异常，同一账号第 %d 次重试", attempt)
		}
		res, err := login.Run(cfg, acc, func(format string, args ...any) {
			m.logf(job, "info", format, args...)
		})
		if err == nil {
			return res, nil
		}
		last = err
		if errors.Is(err, login.ErrAccountBanned) || !login.IsAuthkitFailure(err) {
			return login.Result{}, err
		}
	}
	return login.Result{}, last
}

func (m *Manager) fail(job *model.Job, msg string) {
	m.logf(job, "error", "%s", msg)
	m.setStatus(job, "failed", msg)
	_ = m.store.UpdateStatus(job.AccountID, "failed", msg)
}

func (m *Manager) setStatus(job *model.Job, status, errMsg string) {
	m.mu.Lock()
	job.Status = status
	job.Error = errMsg
	if status == "success" || status == "failed" {
		job.EndedAt = time.Now().Unix()
	}
	m.mu.Unlock()
}

func (m *Manager) logf(job *model.Job, level, format string, args ...any) {
	ev := model.JobEvent{
		JobID:     job.ID,
		AccountID: job.AccountID,
		Level:     level,
		Message:   fmt.Sprintf(format, args...),
		Time:      time.Now().UnixMilli(),
	}
	m.mu.Lock()
	job.Logs = append(job.Logs, ev)
	subs := append([]chan model.JobEvent{}, m.subs[job.ID]...)
	m.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
