package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"opencode-go-manager/internal/model"
	"opencode-go-manager/internal/store"
)

type autoResultsPage struct {
	Items    []model.Account `json:"items"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}

// Requests in a transfer share a connection snapshot. Changing the connection
// in another browser must not redirect credentials or later pages mid-transfer.
func readAutoJSON(ctx context.Context, c model.AutoConnection, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("无法编码 Auto 请求")
		}
		reader = bytes.NewReader(raw)
	}
	res, err := autoRequestConnection(ctx, method, path, reader, c)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("%s（HTTP %d）", autoHTTPError(res.StatusCode), res.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 16<<20)).Decode(out); err != nil {
		return fmt.Errorf("Auto 返回的数据格式无效，请确认两端版本一致")
	}
	return nil
}

func readAutoResults(ctx context.Context, c model.AutoConnection, batchID string) ([]model.Account, error) {
	accounts := []model.Account{}
	seen := map[string]bool{}
	expectedTotal, pageSize := -1, 0
	for page := 1; ; page++ {
		query := url.Values{"page": {fmt.Sprint(page)}, "page_size": {"100"}}
		if batchID != "" {
			query.Set("batch_id", batchID)
		}
		var result autoResultsPage
		if err := readAutoJSON(ctx, c, http.MethodGet, "/api/results?"+query.Encode(), nil, &result); err != nil {
			return nil, err
		}
		if result.Page != page || result.PageSize <= 0 || result.PageSize > 100 || result.Total < 0 || len(result.Items) > result.PageSize {
			return nil, fmt.Errorf("Auto 返回的结果分页无效")
		}
		if expectedTotal < 0 {
			expectedTotal, pageSize = result.Total, result.PageSize
		}
		if result.Total != expectedTotal || result.PageSize != pageSize {
			return nil, fmt.Errorf("Auto 结果在导入期间发生变化，请稍后重试")
		}
		expectedCount := min(pageSize, expectedTotal-(page-1)*pageSize)
		if expectedCount < 0 || len(result.Items) != expectedCount {
			return nil, fmt.Errorf("Auto 返回的结果分页不完整，导入已中止")
		}
		for _, a := range result.Items {
			if batchID != "" && a.BatchID != batchID {
				return nil, fmt.Errorf("Auto 返回了其他批次的账号，导入已中止")
			}
			if a.ID == "" || seen[a.ID] {
				return nil, fmt.Errorf("Auto 返回了重复或无效的账号，导入已中止，请稍后重试")
			}
			seen[a.ID] = true
			accounts = append(accounts, a)
		}
		if page*result.PageSize >= result.Total {
			break
		}
		if len(result.Items) == 0 || page >= 1000 {
			return nil, fmt.Errorf("Auto 结果不完整或过多，请按批次导入")
		}
	}
	return accounts, nil
}

func (s *Server) importAutoResults(w http.ResponseWriter, r *http.Request) {
	var in struct {
		BatchID string `json:"batch_id"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	d.DisallowUnknownFields()
	if err := d.Decode(&in); err != nil && err != io.EOF {
		writeErr(w, http.StatusBadRequest, "导入参数无效")
		return
	}
	c, err := s.autoConnection()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	accounts, err := readAutoResults(ctx, c, in.BatchID)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	imported, updated, skipped := 0, 0, 0
	warning := ""
	for _, account := range accounts {
		state, err := s.store.ImportAutoAccount(c.URL, account)
		if err != nil {
			warning = "部分账号未能保存，请重试导入；已成功保存的账号不会重复创建"
			skipped++
			continue
		}
		switch state {
		case "imported":
			imported++
		case "updated":
			updated++
		default:
			skipped++
		}
	}
	var status *model.UsageSyncStatus
	if s.usage != nil && imported+updated > 0 {
		if err := s.usage.StartAllForced(); err != nil {
			warning = "账号已导入；当前用量刷新正在进行，可稍后手动刷新"
		}
		st := s.usage.Status()
		status = &st
	}
	writeJSON(w, http.StatusOK, map[string]any{"imported": imported, "updated": updated, "skipped": skipped, "sync": status, "warning": warning})
}

func (s *Server) keepPoolCookiesRemote(w http.ResponseWriter, r *http.Request) {
	c, err := s.autoConnection()
	if err == nil {
		err = autoConnectionReady(c)
	}
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	accounts, err := s.store.ListPoolAccountsRaw()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取账号池失败")
		return
	}
	eligible := []model.Account{}
	for _, a := range accounts {
		if a.CookieHeader == "" && a.CookiesJSON == "" {
			continue
		}
		if a.APIKey == "" {
			continue
		}
		a.Proxy = ""
		eligible = append(eligible, a)
	}
	jobs := []model.Job{}
	warnings := 0
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	for start := 0; start < len(eligible); start += 100 {
		end := min(start+100, len(eligible))
		localByEmail := map[string]model.Account{}
		for _, a := range eligible[start:end] {
			localByEmail[strings.ToLower(strings.TrimSpace(a.Email))] = a
		}
		var uploaded struct {
			Items []model.AccountPublic `json:"items"`
		}
		if err := readAutoJSON(ctx, c, http.MethodPost, "/api/accounts/import", map[string]any{"accounts": eligible[start:end]}, &uploaded); err != nil {
			if len(jobs) == 0 {
				writeErr(w, http.StatusBadGateway, err.Error())
				return
			}
			warnings += len(eligible) - start
			break
		}
		for _, remote := range uploaded.Items {
			email := strings.ToLower(strings.TrimSpace(remote.Email))
			local, ok := localByEmail[email]
			if !ok || remote.ID == "" {
				warnings++
				continue
			}
			delete(localByEmail, email)
			if err := s.store.LinkAutoAccount(c.URL, remote.ID, local.ID); err != nil {
				warnings++
				continue
			}
			var job model.Job
			if err := readAutoJSON(ctx, c, http.MethodPost, "/api/accounts/"+url.PathEscape(remote.ID)+"/cookie", nil, &job); err != nil {
				warnings++
				continue
			}
			if job.ID == "" || job.AccountID != remote.ID {
				warnings++
				continue
			}
			if err := s.store.SaveAutoCookieJob(store.AutoCookieJob{Source: c.URL, JobID: job.ID, RemoteID: remote.ID, LocalID: local.ID}); err != nil {
				warnings++
				continue
			}
			jobs = append(jobs, job)
		}
		warnings += len(localByEmail)
	}
	if len(jobs) > 0 {
		go s.syncAutoCookies()
	}
	warning := ""
	if warnings > 0 {
		warning = fmt.Sprintf("%d 个账号未完成排队或任务跟踪，可稍后重试", warnings)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "count": len(jobs), "jobs": jobs, "warning": warning})
}

var autoCookieSyncMu sync.Mutex

// Pending job mappings survive manager restarts. Only the currently configured
// service is polled; old IDs are never sent to a newly selected Auto server.
func (s *Server) syncAutoCookies() {
	if !autoCookieSyncMu.TryLock() {
		return
	}
	defer autoCookieSyncMu.Unlock()
	c, err := s.autoConnection()
	if err != nil || autoConnectionReady(c) != nil {
		return
	}
	pending, err := s.store.ListAutoCookieJobs(c.URL)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	for _, task := range pending {
		if ctx.Err() != nil {
			return
		}
		res, err := autoRequestConnection(ctx, http.MethodGet, "/api/jobs/"+url.PathEscape(task.JobID), nil, c)
		if err != nil {
			return
		}
		if res.StatusCode == http.StatusNotFound {
			res.Body.Close()
			_ = s.store.SetAccountLastError(task.LocalID, "Auto 续期任务已丢失（服务可能重启），请重新续期")
			_ = s.store.DeleteAutoCookieJob(task.Source, task.JobID)
			continue
		}
		if res.StatusCode != http.StatusOK {
			res.Body.Close()
			return
		}
		var job model.Job
		err = json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&job)
		res.Body.Close()
		if err != nil || job.ID != task.JobID || job.AccountID != task.RemoteID {
			continue
		}
		if job.Status == "failed" {
			_ = s.store.SetAccountLastError(task.LocalID, "Auto 续期失败，请在自动化任务日志查看详情")
			_ = s.store.DeleteAutoCookieJob(task.Source, task.JobID)
			continue
		}
		if job.Status != "success" {
			continue
		}
		var remote model.Account
		if err := readAutoJSON(ctx, c, http.MethodGet, "/api/accounts/"+url.PathEscape(task.RemoteID), nil, &remote); err != nil {
			continue
		}
		local, err := s.store.Get(task.LocalID)
		if err != nil {
			_ = s.store.DeleteAutoCookieJob(task.Source, task.JobID)
			continue
		}
		if !strings.EqualFold(local.Email, remote.Email) || remote.ID != task.RemoteID || remote.CookieHeader == "" {
			continue
		}
		if err := s.store.LinkAutoAccount(task.Source, task.RemoteID, task.LocalID); err != nil {
			continue
		}
		// Use the remote credential timestamp so a delayed completed task never
		// overwrites a cookie edited locally while that task was running.
		if _, err := s.store.SyncAutoAccountCredentials(task.Source, remote); err != nil {
			log.Print("保存 Auto 续期结果失败")
			continue
		}
		_ = s.store.DeleteAutoCookieJob(task.Source, task.JobID)
	}
	s.syncAutoLinkedCredentials(ctx, c)
}

// Daily jobs originate on Auto and have no manager-side job record. Pull their
// results only into previously linked local accounts after verifying every page.
func (s *Server) syncAutoLinkedCredentials(ctx context.Context, c model.AutoConnection) {
	linked, err := s.store.HasAutoAccountLinks(c.URL)
	if err != nil || !linked || ctx.Err() != nil {
		return
	}
	accounts, err := readAutoResults(ctx, c, "")
	if err != nil {
		return
	}
	for _, remote := range accounts {
		if ctx.Err() != nil {
			return
		}
		if _, err := s.store.SyncAutoAccountCredentials(c.URL, remote); err != nil {
			log.Print("同步 Auto 定时续期结果失败")
		}
	}
}
