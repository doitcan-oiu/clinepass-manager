package api

import (
	"net/http"
	"strings"
	"time"

	"opencode-go-manager/internal/gomodel"
	"opencode-go-manager/internal/proxy"
	"opencode-go-manager/internal/store"
)

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("range")
	if period == "" {
		period = "30d"
	}
	now := time.Now()
	if _, _, _, err := store.DashboardBounds(period, now); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	out, err := s.store.Dashboard(period, now)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	total, err := s.store.CountPoolAccounts("")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	accounts, err := s.store.ListPoolAccounts("", total+1, 0)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	settings, err := s.store.GetSettings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	available := proxy.RankWithRPM(accounts, "", settings.AccountRPM)
	out.Summary.Accounts = len(accounts)
	a := &out.Availability
	a.AccountsTotal = len(accounts)
	a.AccountsAvailable = len(available)
	a.AccountsUnavailable = a.AccountsTotal - a.AccountsAvailable
	a.ModelsTotal = len(gomodel.All())
	a.ModelsEnabled = a.ModelsTotal // The current catalog has no per-model disable switch.
	keys, availableKeys := map[string]struct{}{}, map[string]struct{}{}
	for _, account := range accounts {
		if key := strings.TrimSpace(account.APIKey); key != "" {
			keys[key] = struct{}{}
		}
	}
	for _, account := range available {
		if key := strings.TrimSpace(account.APIKey); key != "" {
			availableKeys[key] = struct{}{}
		}
	}
	a.KeysTotal = len(keys)
	a.KeysAvailable = len(availableKeys)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}
