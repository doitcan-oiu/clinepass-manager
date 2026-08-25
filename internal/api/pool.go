package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"opencode-go-manager/internal/backup"
	"opencode-go-manager/internal/model"
	"opencode-go-manager/internal/proxy"
)

func (s *Server) markBatchPaid(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.store.GetBatch(id); err != nil {
		writeErr(w, http.StatusNotFound, "批次不存在")
		return
	}
	if err := s.store.MarkPaid(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	sum, _ := s.store.GetBatchSummary(id)
	warning := ""
	if err := s.usage.StartBatch(id); err != nil {
		warning = err.Error()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"batch":   sum,
		"sync":    s.usage.Status(),
		"warning": warning,
	})
}

func (s *Server) listPoolAccounts(w http.ResponseWriter, r *http.Request) {
	_, _ = s.store.DeleteExpiredAccounts(0)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	batchID := r.URL.Query().Get("batch_id")
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 30
	}
	all, err := s.store.ListPoolAccounts(batchID, 100000, 0)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	proxy.AttachInflight(all)
	active, weekly, rolling := model.SplitShelved(all)
	stats, _ := s.store.PoolStats(batchID)
	for _, a := range all {
		stats.Inflight += a.Inflight
	}
	writeJSON(w, http.StatusOK, model.PoolPage{
		Items:          pageAccounts(active, page, pageSize),
		WeeklyLimited:  weekly,
		RollingLimited: rolling,
		Total:          len(active),
		Page:           page,
		PageSize:       pageSize,
		Stats:          stats,
	})
}

func pageAccounts(list []model.PoolAccount, page, pageSize int) []model.PoolAccount {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 30
	}
	start := (page - 1) * pageSize
	if start >= len(list) {
		return []model.PoolAccount{}
	}
	end := start + pageSize
	if end > len(list) {
		end = len(list)
	}
	return list[start:end]
}

func (s *Server) listPaidBatches(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListPaidBatches()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) listPendingBatches(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListUnpaidExported()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createPaidAccount(w http.ResponseWriter, r *http.Request) {
	var in model.CreatePaidAccountInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON 无效")
		return
	}
	a, err := s.store.CreatePaidAccount(in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	_, _ = s.usage.One(a.ID)
	p, err := s.store.GetPoolAccount(a.ID)
	if err != nil {
		writeJSON(w, http.StatusCreated, a.Public())
		return
	}
	p.Inflight = proxy.InflightOf(p.ID)
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) refreshAccountUsage(w http.ResponseWriter, r *http.Request) {
	u, err := s.usage.One(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := s.store.GetPoolAccount(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"usage": u})
		return
	}
	p.Inflight = proxy.InflightOf(p.ID)
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) getUsageSync(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.usage.Status())
}

func (s *Server) startUsageSync(w http.ResponseWriter, r *http.Request) {
	if err := s.usage.StartAll(); err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, s.usage.Status())
}

func (s *Server) exportAccounts(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListPoolAccountsRaw()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="clinepass-backup.json"`)
	writeJSON(w, http.StatusOK, backup.Export(list))
}

func (s *Server) importAccounts(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "读取文件失败")
		return
	}
	parsed, err := backup.Parse(raw)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	imported, updated := 0, 0
	skipped := append([]string{}, parsed.Skipped...)
	for _, in := range parsed.Items {
		_, wasUpdate, err := s.store.UpsertPaidAccount(in)
		if err != nil {
			skipped = append(skipped, in.Email+"："+err.Error())
			continue
		}
		if wasUpdate {
			updated++
		} else {
			imported++
		}
	}
	sync := s.usage.Status()
	warning := ""
	if imported+updated > 0 {
		if err := s.usage.StartAll(); err != nil {
			warning = err.Error()
		} else {
			sync = s.usage.Status()
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"imported": imported,
		"updated":  updated,
		"skipped":  skipped,
		"sync":     sync,
		"warning":  warning,
	})
}
