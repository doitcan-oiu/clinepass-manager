package api

import (
	"net/http"
	"strconv"

	"opencode-go-manager/internal/model"
)

func (s *Server) listLogs(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}
	f := model.RequestLogFilter{
		Model:  r.URL.Query().Get("model"),
		Email:  r.URL.Query().Get("email"),
		Status: r.URL.Query().Get("status"),
		Stream: r.URL.Query().Get("stream"),
	}
	total, err := s.store.CountRequestLogs(f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	list, err := s.store.ListRequestLogs(f, pageSize, (page-1)*pageSize)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	stats, _ := s.store.RequestLogStats(0)
	writeJSON(w, http.StatusOK, model.RequestLogPage{
		Items:    list,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Stats:    stats,
	})
}

func (s *Server) clearLogs(w http.ResponseWriter, r *http.Request) {
	if err := s.store.ClearRequestLogs(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) logStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.RequestLogStats(0)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}
