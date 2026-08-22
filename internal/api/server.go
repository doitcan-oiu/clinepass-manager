package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"opencode-go-manager/internal/browser"
	"opencode-go-manager/internal/config"
	"opencode-go-manager/internal/job"
	"opencode-go-manager/internal/model"
	"opencode-go-manager/internal/proxy"
	"opencode-go-manager/internal/store"
	"opencode-go-manager/internal/usage"
)

type Server struct {
	cfg     config.Config
	store   *store.Store
	jobs    *job.Manager
	usage   *usage.Syncer
	proxy   *proxy.Handler
	webRoot string
}

func New(cfg config.Config, st *store.Store, jobs *job.Manager, webRoot string) *Server {
	s := &Server{
		cfg:     cfg,
		store:   st,
		jobs:    jobs,
		usage:   usage.NewSyncer(st),
		proxy:   proxy.New(st),
		webRoot: webRoot,
	}
	go s.expireLoop()
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/config", s.getConfig)
	mux.HandleFunc("PATCH /api/config", s.patchConfig)
	mux.HandleFunc("GET /api/herosms/catalog", s.heroSMSCatalog)
	mux.HandleFunc("POST /api/accounts", s.createPaidAccount)
	mux.HandleFunc("DELETE /api/accounts/{id}", s.deleteAccount)
	mux.HandleFunc("POST /api/accounts/{id}/login", s.loginAccount)
	mux.HandleFunc("POST /api/accounts/{id}/usage", s.refreshAccountUsage)
	mux.HandleFunc("GET /api/batches", s.listBatches)
	mux.HandleFunc("POST /api/batches", s.createBatch)
	mux.HandleFunc("GET /api/batches/{id}", s.getBatch)
	mux.HandleFunc("DELETE /api/batches/{id}", s.deleteBatch)
	mux.HandleFunc("POST /api/batches/{id}/login", s.loginBatch)
	mux.HandleFunc("POST /api/batches/{id}/refresh", s.refreshBatch)
	mux.HandleFunc("POST /api/batches/{id}/dispatch", s.dispatchBatch)
	mux.HandleFunc("POST /api/batches/{id}/paid", s.markBatchPaid)
	mux.HandleFunc("POST /api/accounts/{id}/refresh", s.refreshAccount)
	mux.HandleFunc("GET /api/pool/accounts", s.listPoolAccounts)
	mux.HandleFunc("GET /api/pool/batches", s.listPaidBatches)
	mux.HandleFunc("GET /api/pool/pending", s.listPendingBatches)
	mux.HandleFunc("GET /api/pool/export", s.exportAccounts)
	mux.HandleFunc("POST /api/pool/import", s.importAccounts)
	mux.HandleFunc("GET /api/usage/sync", s.getUsageSync)
	mux.HandleFunc("POST /api/usage/sync", s.startUsageSync)
	mux.HandleFunc("GET /api/jobs", s.listJobs)
	mux.HandleFunc("GET /api/jobs/{id}", s.getJob)
	mux.HandleFunc("GET /api/jobs/{id}/events", s.jobEvents)
	mux.HandleFunc("GET /api/logs", s.listLogs)
	mux.HandleFunc("DELETE /api/logs", s.clearLogs)
	mux.HandleFunc("GET /api/logs/stats", s.logStats)
	mux.Handle("/v1/", s.proxy)
	mux.HandleFunc("/", s.frontend)
	return cors(mux)
}

func (s *Server) expireLoop() {
	_, _ = s.store.DeleteExpiredAccounts(time.Now().Unix())
	_ = s.store.PruneRequestLogs(0)
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		_, _ = s.store.DeleteExpiredAccounts(now.Unix())
		_ = s.store.PruneRequestLogs(now.UnixMilli())
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"time":     time.Now().Unix(),
		"platform": browser.PlatformTag(),
	})
}

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.publicConfig())
}

func (s *Server) patchConfig(w http.ResponseWriter, r *http.Request) {
	cur, err := s.store.GetSettings()
	if err != nil {
		cur = model.Settings{Headless: true}
	}
	var in struct {
		Proxy           *string  `json:"proxy"`
		Headless        *bool    `json:"headless"`
		InviteURL       *string  `json:"invite_url"`
		HeroSMSAPIKey   *string  `json:"hero_sms_api_key"`
		HeroSMSService  *string  `json:"hero_sms_service"`
		HeroSMSCountry  *int     `json:"hero_sms_country"`
		HeroSMSMaxPrice *float64 `json:"hero_sms_max_price"`
		MaxConcurrent   *int     `json:"max_concurrent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON 无效")
		return
	}
	if in.Proxy != nil {
		cur.Proxy = *in.Proxy
	}
	if in.Headless != nil {
		cur.Headless = *in.Headless
	}
	if in.InviteURL != nil {
		cur.InviteURL = *in.InviteURL
	}
	if in.HeroSMSAPIKey != nil {
		key := strings.TrimSpace(*in.HeroSMSAPIKey)
		if !strings.Contains(key, "********") {
			cur.HeroSMSAPIKey = key
		}
	}
	if in.HeroSMSService != nil {
		cur.HeroSMSService = strings.TrimSpace(*in.HeroSMSService)
	}
	if in.HeroSMSCountry != nil {
		cur.HeroSMSCountry = *in.HeroSMSCountry
	}
	if in.HeroSMSMaxPrice != nil {
		cur.HeroSMSMaxPrice = *in.HeroSMSMaxPrice
	}
	if in.MaxConcurrent != nil {
		if *in.MaxConcurrent < 1 {
			writeErr(w, http.StatusBadRequest, "并发数至少为 1")
			return
		}
		cur.MaxConcurrent = *in.MaxConcurrent
	}
	if err := s.store.SaveSettings(cur); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.jobs.Pump()
	writeJSON(w, http.StatusOK, s.publicConfig())
}

func (s *Server) publicConfig() map[string]any {
	cfg := s.cfg
	st, err := s.store.GetSettings()
	if err == nil {
		cfg = store.ApplySettings(cfg, st)
	} else {
		st = model.Settings{}
	}
	return map[string]any{
		"invite_url":          cfg.InviteURL,
		"headless":            cfg.Headless,
		"proxy":               cfg.Proxy,
		"cloak_version":       cfg.CloakVersion,
		"max_concurrent":      cfg.MaxConcurrent,
		"platform":            browser.PlatformTag(),
		"hero_sms_api_key":    maskSecret(st.HeroSMSAPIKey),
		"hero_sms_configured": strings.TrimSpace(st.HeroSMSAPIKey) != "",
		"hero_sms_service":    st.HeroSMSService,
		"hero_sms_country":    st.HeroSMSCountry,
		"hero_sms_max_price":  st.HeroSMSMaxPrice,
	}
}

func maskSecret(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "********"
	}
	return s[:4] + "********" + s[len(s)-4:]
}

func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Delete(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusNotFound, "账号不存在")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) loginAccount(w http.ResponseWriter, r *http.Request) {
	j, err := s.jobs.Enqueue(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "账号不存在")
		return
	}
	writeJSON(w, http.StatusAccepted, j)
}

func (s *Server) listBatches(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 30
	}
	total, err := s.store.CountBatches()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	list, err := s.store.ListBatchesPage(pageSize, (page-1)*pageSize)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":     list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (s *Server) createBatch(w http.ResponseWriter, r *http.Request) {
	var in model.CreateBatchInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON 无效")
		return
	}
	b, errors, err := s.store.CreateBatch(in)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":  err.Error(),
			"errors": errors,
		})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"batch":  b,
		"errors": errors,
	})
}

func (s *Server) getBatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sum, err := s.store.GetBatchSummary(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "批次不存在")
		return
	}
	list, err := s.store.ListByBatch(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	accounts := make([]model.AccountPublic, 0, len(list))
	for _, a := range list {
		accounts = append(accounts, a.Public())
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"batch":    sum,
		"accounts": accounts,
	})
}

func (s *Server) deleteBatch(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteBatch(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusNotFound, "批次不存在")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) loginBatch(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListByBatch(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.store.GetBatch(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusNotFound, "批次不存在")
		return
	}
	jobs := []*model.Job{}
	for _, a := range list {
		if a.PaidAt > 0 {
			continue
		}
		if a.Status == "ready" || a.Status == "queued" || a.Status == "running" {
			continue
		}
		j, err := s.jobs.Enqueue(a.ID)
		if err != nil {
			continue
		}
		jobs = append(jobs, j)
	}
	writeJSON(w, http.StatusAccepted, jobs)
}

func (s *Server) refreshBatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.store.GetBatch(id); err != nil {
		writeErr(w, http.StatusNotFound, "批次不存在")
		return
	}
	list, err := s.store.ListByBatch(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.store.ClearExported(id)
	jobs := []*model.Job{}
	for _, a := range list {
		if a.PaidAt > 0 {
			continue
		}
		if a.Status == "queued" || a.Status == "running" {
			continue
		}
		j, err := s.jobs.EnqueueRefresh(a.ID)
		if err != nil {
			continue
		}
		jobs = append(jobs, j)
	}
	writeJSON(w, http.StatusAccepted, jobs)
}

func (s *Server) refreshAccount(w http.ResponseWriter, r *http.Request) {
	j, err := s.jobs.EnqueueRefresh(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, j)
}

func (s *Server) dispatchBatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	links, err := s.store.UniquePaymentLinks(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "批次不存在")
		return
	}
	if len(links) == 0 {
		writeErr(w, http.StatusBadRequest, "还没有支付链接，请先提取")
		return
	}
	lines := make([]string, 0, len(links))
	for _, l := range links {
		lines = append(lines, l.URL)
	}
	if err := s.store.MarkExported(id, len(links)); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	sum, _ := s.store.GetBatchSummary(id)
	writeJSON(w, http.StatusOK, map[string]any{
		"batch": sum,
		"items": links,
		"count": len(links),
		"text":  strings.Join(lines, "\n") + "\n",
	})
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.jobs.List())
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	j, ok := s.jobs.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "任务不存在")
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (s *Server) jobEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.jobs.Get(id); !ok {
		writeErr(w, http.StatusNotFound, "任务不存在")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "不支持 SSE")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch, cancel := s.jobs.Subscribe(id)
	defer cancel()
	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			b, _ := json.Marshal(ev)
			_, _ = io.WriteString(w, "data: "+string(b)+"\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) frontend(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	root := s.webRoot
	if root == "" {
		root = "web/dist"
	}
	path := r.URL.Path
	if path == "/" {
		path = "/index.html"
	}
	full := filepath.Join(root, filepath.Clean(path))
	if !strings.HasPrefix(full, filepath.Clean(root)) {
		http.NotFound(w, r)
		return
	}
	if st, err := os.Stat(full); err == nil && !st.IsDir() {
		http.ServeFile(w, r, full)
		return
	}
	index := filepath.Join(root, "index.html")
	if _, err := os.Stat(index); err == nil {
		http.ServeFile(w, r, index)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "前端尚未构建。开发模式请运行: cd web && npm run dev\n")
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, x-api-key, Anthropic-Version")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
