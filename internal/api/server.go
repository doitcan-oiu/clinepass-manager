package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"opencode-go-manager/internal/config"
	"opencode-go-manager/internal/proxy"
	"opencode-go-manager/internal/store"
	"opencode-go-manager/internal/usage"
)

type Server struct {
	cfg     config.Config
	store   *store.Store
	usage   *usage.Syncer
	proxy   *proxy.Handler
	webRoot string
}

func New(cfg config.Config, st *store.Store, webRoot string) *Server {
	s := &Server{
		cfg:     cfg,
		store:   st,
		usage:   usage.NewSyncer(st),
		proxy:   proxy.New(st),
		webRoot: webRoot,
	}

	s.proxy.SetUsageRefresher(s.usage)
	s.usage.StartLoop()
	go s.expireLoop()
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/dashboard", s.dashboard)
	mux.HandleFunc("GET /api/auto/status", s.autoStatus)
	mux.HandleFunc("GET /api/auto/connection", s.getAutoConnection)
	mux.HandleFunc("PATCH /api/auto/connection", s.patchAutoConnection)
	mux.HandleFunc("POST /api/auto/test", s.testAutoConnection)
	mux.HandleFunc("GET /api/auto/config", s.forwardAutoConfig)
	mux.HandleFunc("PATCH /api/auto/config", s.forwardAutoConfig)
	mux.HandleFunc("GET /api/auto/accounts/{id}", s.forwardAutoAccount)
	mux.HandleFunc("DELETE /api/auto/accounts/{id}", s.forwardAutoAccount)
	mux.HandleFunc("POST /api/auto/accounts/{id}/login", s.forwardAutoAccount)
	mux.HandleFunc("POST /api/auto/accounts/{id}/refresh", s.forwardAutoAccount)
	mux.HandleFunc("GET /api/auto/usage/sync", s.forwardAutoAccount)
	mux.HandleFunc("POST /api/auto/import", s.importAutoResults)
	mux.HandleFunc("GET /api/config", s.getConfig)
	mux.HandleFunc("PATCH /api/config", s.patchConfig)
	mux.HandleFunc("GET /api/herosms/catalog", s.forwardAuto)
	mux.HandleFunc("GET /api/amzkeys/status", s.forwardAuto)
	mux.HandleFunc("POST /api/amzkeys/cards", s.forwardAuto)
	mux.HandleFunc("POST /api/amzkeys/cards/warm", s.forwardAuto)
	mux.HandleFunc("POST /api/amzkeys/cards/release", s.forwardAuto)
	mux.HandleFunc("DELETE /api/amzkeys/cards", s.forwardAuto)
	mux.HandleFunc("GET /api/amzkeys/auth-codes", s.forwardAuto)
	mux.HandleFunc("POST /api/accounts", s.createPaidAccount)
	mux.HandleFunc("GET /api/accounts/{id}", s.getAccount)
	mux.HandleFunc("DELETE /api/accounts/{id}", s.deleteAccount)
	mux.HandleFunc("POST /api/accounts/{id}/login", s.forwardAuto)
	mux.HandleFunc("POST /api/accounts/{id}/usage", s.refreshAccountUsage)
	mux.HandleFunc("POST /api/accounts/{id}/cookie-expired", s.markCookieExpired)
	mux.HandleFunc("POST /api/accounts/{id}/test", s.testAccount)
	mux.HandleFunc("GET /api/models", s.listModels)
	mux.HandleFunc("GET /api/batches", s.forwardAuto)
	mux.HandleFunc("POST /api/batches", s.forwardAuto)
	mux.HandleFunc("GET /api/batches/{id}", s.forwardAuto)
	mux.HandleFunc("DELETE /api/batches/{id}", s.forwardAuto)
	mux.HandleFunc("POST /api/batches/{id}/login", s.forwardAuto)
	mux.HandleFunc("POST /api/batches/{id}/refresh", s.forwardAuto)
	mux.HandleFunc("POST /api/batches/{id}/dispatch", s.forwardAuto)
	mux.HandleFunc("POST /api/batches/{id}/paid", s.forwardAuto)
	mux.HandleFunc("DELETE /api/batches/{id}/radar-denied", s.forwardAuto)
	mux.HandleFunc("POST /api/accounts/{id}/refresh", s.forwardAuto)
	mux.HandleFunc("GET /api/pool/accounts", s.listPoolAccounts)
	mux.HandleFunc("GET /api/pool/batches", s.listPaidBatches)
	mux.HandleFunc("GET /api/pool/pending", s.listPendingBatches)
	mux.HandleFunc("GET /api/pool/export", s.exportAccounts)
	mux.HandleFunc("POST /api/pool/import", s.importAccounts)
	mux.HandleFunc("GET /api/usage/sync", s.getUsageSync)
	mux.HandleFunc("POST /api/usage/sync", s.startUsageSync)
	mux.HandleFunc("POST /api/pool/cookies", s.keepPoolCookiesRemote)
	mux.HandleFunc("GET /api/jobs", s.forwardAuto)
	mux.HandleFunc("GET /api/jobs/{id}", s.forwardAuto)
	mux.HandleFunc("GET /api/jobs/{id}/events", s.forwardAuto)
	mux.HandleFunc("GET /api/logs", s.listLogs)
	mux.HandleFunc("DELETE /api/logs", s.clearLogs)
	mux.HandleFunc("GET /api/logs/stats", s.logStats)
	mux.HandleFunc("POST /api/cloak/update", s.forwardAuto)
	mux.Handle("/v1/", s.proxy)
	mux.HandleFunc("/", s.frontend)
	return cors(mux)
}

func (s *Server) expireLoop() {
	s.syncAutoCookies()
	_, _ = s.store.DeleteExpiredAccounts(time.Now().Unix())
	_ = s.store.PruneRequestLogs(0)
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		s.syncAutoCookies()
		now := time.Now()
		_, _ = s.store.DeleteExpiredAccounts(now.Unix())
		_ = s.store.PruneRequestLogs(now.UnixMilli())
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"time":     time.Now().Unix(),
		"platform": runtime.GOOS + "-" + runtime.GOARCH,
	})
}

func (s *Server) getAccount(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "账号不存在")
		return
	}
	writeJSON(w, http.StatusOK, a.Public())
}

func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Delete(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusNotFound, "账号不存在")
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
