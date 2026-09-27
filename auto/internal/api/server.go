package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"opencode-go-manager/auto/internal/browser"
	"opencode-go-manager/auto/internal/export"
	"opencode-go-manager/auto/internal/job"
	"opencode-go-manager/internal/config"
	"opencode-go-manager/internal/model"
	"opencode-go-manager/internal/store"
	"opencode-go-manager/internal/usage"
)

type Server struct {
	cfg          config.Config
	store        *store.Store
	jobs         *job.Manager
	usage        *usage.Syncer
	amzCardMu    sync.Mutex
	settingsMu   sync.Mutex
	cloakVersion string
	cloakLicense string
}

func New(cfg config.Config, st *store.Store, jobs *job.Manager) *Server {
	s := &Server{cfg: cfg, store: st, jobs: jobs, usage: usage.NewSyncer(st), cloakVersion: cfg.CloakVersion, cloakLicense: cfg.LicenseKey}
	jobs.SetCardWarmer(s.WarmAmzKeysCard)
	return s
}

// Run owns all browser, payment and cookie scheduling. Only the auto process calls it.
func (s *Server) Run(ctx context.Context) {
	s.refreshCloak()
	s.WarmAmzKeysCard()
	s.maybeCookieKeep(time.Now())
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.reloadSettings()
			s.WarmAmzKeysCard()
			s.maybeCookieKeep(now)
		}
	}
}

func (s *Server) reloadSettings() {
	st, err := s.store.GetSettings()
	if err != nil {
		return
	}
	cfg := store.ApplySettings(s.cfg, st)
	s.settingsMu.Lock()
	changed := cfg.CloakVersion != s.cloakVersion || cfg.LicenseKey != s.cloakLicense
	s.cloakVersion, s.cloakLicense = cfg.CloakVersion, cfg.LicenseKey
	s.settingsMu.Unlock()
	s.jobs.Pump()
	if changed {
		s.refreshCloak()
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "auto", "protocol_version": 1, "platform": browser.PlatformTag(), "time": time.Now().Unix()})
	})
	mux.HandleFunc("POST /internal/reload", func(w http.ResponseWriter, r *http.Request) {
		s.reloadSettings()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /api/herosms/catalog", s.heroSMSCatalog)
	mux.HandleFunc("GET /api/config", s.getConfig)
	mux.HandleFunc("PATCH /api/config", s.patchConfig)
	mux.HandleFunc("GET /api/results", s.listResults)
	mux.HandleFunc("GET /api/usage/sync", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, s.usage.Status()) })
	mux.HandleFunc("GET /api/accounts/{id}", s.getAccount)
	mux.HandleFunc("DELETE /api/accounts/{id}", s.deleteAccount)
	mux.HandleFunc("POST /api/accounts/import", s.importAccounts)
	mux.HandleFunc("POST /api/accounts/{id}/cookie", s.keepAccountCookie)
	mux.HandleFunc("GET /api/amzkeys/status", s.amzKeysStatus)
	mux.HandleFunc("POST /api/amzkeys/cards", s.amzKeysCheckoutCard)
	mux.HandleFunc("POST /api/amzkeys/cards/warm", s.amzKeysWarmCard)
	mux.HandleFunc("POST /api/amzkeys/cards/release", s.amzKeysReleaseCard)
	mux.HandleFunc("DELETE /api/amzkeys/cards", s.amzKeysClearCard)
	mux.HandleFunc("GET /api/amzkeys/auth-codes", s.amzKeysAuthCodes)
	mux.HandleFunc("POST /api/accounts/{id}/login", s.loginAccount)
	mux.HandleFunc("POST /api/accounts/{id}/refresh", s.refreshAccount)
	mux.HandleFunc("GET /api/batches", s.listBatches)
	mux.HandleFunc("POST /api/batches", s.createBatch)
	mux.HandleFunc("GET /api/batches/{id}", s.getBatch)
	mux.HandleFunc("DELETE /api/batches/{id}", s.deleteBatch)
	mux.HandleFunc("POST /api/batches/{id}/login", s.loginBatch)
	mux.HandleFunc("POST /api/batches/{id}/refresh", s.refreshBatch)
	mux.HandleFunc("POST /api/batches/{id}/dispatch", s.dispatchBatch)
	mux.HandleFunc("POST /api/batches/{id}/paid", s.markBatchPaid)
	mux.HandleFunc("DELETE /api/batches/{id}/radar-denied", s.deleteRadarDenied)
	mux.HandleFunc("POST /api/pool/cookies", s.keepPoolCookies)
	mux.HandleFunc("GET /api/jobs", s.listJobs)
	mux.HandleFunc("GET /api/jobs/{id}", s.getJob)
	mux.HandleFunc("GET /api/jobs/{id}/events", s.jobEvents)
	mux.HandleFunc("POST /api/cloak/update", s.updateCloak)
	return s.authenticate(mux)
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	expected := sha256.Sum256([]byte(strings.TrimSpace(s.cfg.AutoToken)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Fields(r.Header.Get("Authorization"))
		valid := len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") && strings.TrimSpace(s.cfg.AutoToken) != ""
		if valid {
			actual := sha256.Sum256([]byte(parts[1]))
			valid = subtle.ConstantTimeCompare(expected[:], actual[:]) == 1
		}
		if !valid {
			w.Header().Set("WWW-Authenticate", `Bearer realm="auto"`)
			writeErr(w, http.StatusUnauthorized, "Auto 连接密钥无效")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
		next.ServeHTTP(w, r)
	})
}
func (s *Server) maybeCookieKeep(now time.Time) {
	st, err := s.store.GetSettings()
	if err != nil {
		return
	}
	if !job.ShouldRunCookieKeep(now, st.CookieKeepEnabled, st.CookieKeepHour, st.CookieKeepLastDate) {
		return
	}
	jobs, err := s.jobs.EnqueueCookieKeep()
	if err != nil {
		log.Printf("每日续 Cookie 入队失败: %v", err)
		return
	}
	if err := s.store.SetCookieKeepLastDate(job.CookieKeepDate(now)); err != nil {
		log.Printf("记下续 Cookie 日期失败: %v", err)
	}
	log.Printf("每日续 Cookie 已入队 %d 个有效账号，免费 Cloak 并发 %d", len(jobs), st.CookieKeepConcurrency)
}

func (s *Server) keepPoolCookies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "方法不允许")
		return
	}
	jobs, err := s.jobs.EnqueueCookieKeep()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.store.SetCookieKeepLastDate(job.CookieKeepDate(time.Now()))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"count": len(jobs),
		"jobs":  jobs,
	})
}

func (s *Server) refreshCloak() {
	go func() {
		cfg := s.cfg
		if st, err := s.store.GetSettings(); err == nil {
			cfg = store.ApplySettings(cfg, st)
		}
		s.jobs.SetCloakBinaryPath("")
		log.Printf("按设置准备 CloakBrowser %s", cfg.CloakVersion)
		info, err := browser.EnsureBinary(cfg.CloakVersion, cfg.CloakCacheDir, cfg.CloakBinaryPath, browser.ResolveLicense(cfg.LicenseKey), log.Printf)
		if err != nil {
			log.Printf("按设置准备 CloakBrowser 失败: %v", err)
			return
		}
		if key := browser.ResolveLicense(cfg.LicenseKey); key != "" {
			_ = os.Setenv("CLOAKBROWSER_LICENSE_KEY", key)
		}
		if cfg.CloakVersion != "" {
			_ = os.Setenv("CLOAKBROWSER_VERSION", cfg.CloakVersion)
		}
		s.jobs.SetCloakBinaryPath(info.Path)
		log.Printf("CloakBrowser 已按设置就绪: %s", info.Path)
	}()
}

func (s *Server) updateCloak(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg
	st, err := s.store.GetSettings()
	if err == nil {
		cfg = store.ApplySettings(cfg, st)
	} else {
		st = model.Settings{Headless: true, APIProxy: true}
	}
	current := strings.TrimSpace(cfg.CloakVersion)
	if current == "" {
		current = "151.0.7922.108.2"
	}
	latest, err := browser.LatestStableVersion(nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	if browser.CompareVersion(latest, current) <= 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"current":     current,
			"latest":      latest,
			"updated":     false,
			"downloading": false,
		})
		return
	}
	if browser.NeedsLicense(latest) && browser.ResolveLicense(cfg.LicenseKey) == "" {
		writeErr(w, http.StatusBadRequest, "最新版 "+latest+" 需要 License，请先保存 CloakBrowser License")
		return
	}
	st.CloakVersion = latest
	if err := s.store.SaveSettings(st); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.refreshCloak()
	writeJSON(w, http.StatusOK, map[string]any{
		"current":     current,
		"latest":      latest,
		"updated":     true,
		"downloading": true,
	})
}

func (s *Server) loginAccount(w http.ResponseWriter, r *http.Request) {
	autoPay := readAutoPay(r)
	if err := s.requireAutoPay(autoPay); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	j, err := s.jobs.Enqueue(r.PathValue("id"), autoPay)
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
	list, err := s.store.ListByBatchMeta(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	accounts := make([]model.AccountPublic, 0, len(list))
	for _, a := range list {
		accounts = append(accounts, a.ListPublic())
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"batch":    sum,
		"accounts": accounts,
	})
}

func (s *Server) deleteRadarDenied(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.store.GetBatch(id); err != nil {
		writeErr(w, http.StatusNotFound, "批次不存在")
		return
	}
	n, err := s.store.DeleteRadarDenied(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	sum, err := s.store.GetBatchSummary(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deleted": n,
		"batch":   sum,
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
	autoPay := readAutoPay(r)
	if err := s.requireAutoPay(autoPay); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	list, err := s.store.ListByBatchMeta(r.PathValue("id"))
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
		j, err := s.jobs.Enqueue(a.ID, autoPay)
		if err != nil {
			continue
		}
		jobs = append(jobs, j)
	}
	writeJSON(w, http.StatusAccepted, jobs)
}

func (s *Server) refreshBatch(w http.ResponseWriter, r *http.Request) {
	autoPay := readAutoPay(r)
	if err := s.requireAutoPay(autoPay); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	id := r.PathValue("id")
	if _, err := s.store.GetBatch(id); err != nil {
		writeErr(w, http.StatusNotFound, "批次不存在")
		return
	}
	list, err := s.store.ListByBatchMeta(id)
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
		j, err := s.jobs.EnqueueRefresh(a.ID, autoPay)
		if err != nil {
			continue
		}
		jobs = append(jobs, j)
	}
	writeJSON(w, http.StatusAccepted, jobs)
}

func (s *Server) refreshAccount(w http.ResponseWriter, r *http.Request) {
	autoPay := readAutoPay(r)
	if err := s.requireAutoPay(autoPay); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	j, err := s.jobs.EnqueueRefresh(r.PathValue("id"), autoPay)
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
	raw, err := export.PayLinksXLSX(links)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.store.MarkExported(id, len(links)); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	sum, _ := s.store.GetBatchSummary(id)
	name := strings.TrimSpace(sum.Name)
	if name == "" {
		name = "支付链接"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"batch":    sum,
		"items":    links,
		"count":    len(links),
		"filename": name + "-支付链接.xlsx",
		"xlsx":     base64.StdEncoding.EncodeToString(raw),
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
