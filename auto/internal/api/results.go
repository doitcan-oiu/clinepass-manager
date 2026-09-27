package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"opencode-go-manager/internal/model"
)

func eligibleResult(a model.Account) bool {
	return a.PaidAt > 0 && strings.TrimSpace(a.APIKey) != "" && strings.TrimSpace(a.CookieHeader) != ""
}

// Credentials are returned only over the authenticated server-to-server API.
func (s *Server) listResults(w http.ResponseWriter, r *http.Request) {
	batchID := strings.TrimSpace(r.URL.Query().Get("batch_id"))
	var list []model.Account
	var err error
	if batchID != "" {
		if _, err := s.store.GetBatch(batchID); err != nil {
			writeErr(w, http.StatusNotFound, "批次不存在")
			return
		}
		list, err = s.store.ListByBatch(batchID)
	} else {
		list, err = s.store.ListPoolAccountsRaw()
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	eligible := make([]model.Account, 0, len(list))
	for _, a := range list {
		if eligibleResult(a) {
			eligible = append(eligible, a)
		}
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 500 {
		size = 100
	}
	start := len(eligible)
	if page <= len(eligible)/size+1 {
		start = min((page-1)*size, len(eligible))
	}
	end := min(start+size, len(eligible))
	writeJSON(w, http.StatusOK, map[string]any{"items": eligible[start:end], "total": len(eligible), "page": page, "page_size": size})
}

func (s *Server) getAccount(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "账号不存在")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Delete(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusNotFound, "账号不存在")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) markBatchPaid(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.store.GetBatch(id); err != nil {
		writeErr(w, http.StatusNotFound, "批次不存在")
		return
	}
	// A clicked button is not proof of payment. The usage syncer sets PaidAt
	// only after the upstream confirms that each account has a subscription.
	if err := s.usage.StartBatch(id); err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	sum, _ := s.store.GetBatchSummary(id)
	writeJSON(w, http.StatusOK, map[string]any{"batch": sum, "sync": s.usage.Status(), "warning": ""})
}

func (s *Server) importAccounts(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Accounts []model.Account `json:"accounts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON 无效")
		return
	}
	if len(in.Accounts) == 0 || len(in.Accounts) > 500 {
		writeErr(w, http.StatusBadRequest, "每次可同步 1–500 个已付款账号")
		return
	}
	for _, a := range in.Accounts {
		if !eligibleResult(a) || strings.TrimSpace(a.Email) == "" {
			writeErr(w, http.StatusBadRequest, "同步账号须已付款并具有邮箱、API Key 和 Cookie")
			return
		}
	}
	items := make([]model.AccountPublic, 0, len(in.Accounts))
	created, updated := 0, 0
	for _, a := range in.Accounts {
		item, existed, err := s.store.ReceiveAutoCookieAccount(a)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if existed {
			updated++
		} else {
			created++
		}
		items = append(items, item.ListPublic())
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "created": created, "updated": updated})
}

func (s *Server) keepAccountCookie(w http.ResponseWriter, r *http.Request) {
	j, err := s.jobs.EnqueueCookie(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, j)
}
