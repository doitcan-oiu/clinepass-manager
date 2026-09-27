package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"opencode-go-manager/internal/gomodel"
	"opencode-go-manager/internal/netproxy"
	"opencode-go-manager/internal/store"
)

func newModelCatalogSyncer(st *store.Store) *gomodel.Syncer {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	netproxy.ApplyFunc(transport, func() string {
		settings, err := st.GetSettings()
		if err != nil || !settings.APIProxy {
			return ""
		}
		return strings.TrimSpace(settings.Proxy)
	})
	return gomodel.NewSyncer(st, &http.Client{Transport: transport, Timeout: 20 * time.Second}, nil)
}

// The process entrypoint owns the background loop. Constructing an API handler
// loads its persisted catalog without starting network calls in tests or tools.
func (s *Server) RunModelCatalog(ctx context.Context) {
	if s.catalog != nil {
		s.catalog.Run(ctx)
	}
}

func (s *Server) getModelCatalogSync(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.catalog == nil {
		writeErr(w, http.StatusServiceUnavailable, "模型目录同步尚未初始化")
		return
	}
	writeJSON(w, http.StatusOK, s.catalog.Status())
}

func (s *Server) refreshModelCatalog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.catalog == nil {
		writeErr(w, http.StatusServiceUnavailable, "模型目录同步尚未初始化")
		return
	}
	if err := s.catalog.Refresh(r.Context()); err != nil {
		status, message := http.StatusBadGateway, err.Error()
		if errors.Is(err, gomodel.ErrRefreshInProgress) {
			status, message = http.StatusConflict, "模型目录正在同步，请稍后重试"
		}
		writeJSON(w, status, map[string]any{"error": message, "sync": s.catalog.Status()})
		return
	}
	writeJSON(w, http.StatusOK, s.catalog.Status())
}
