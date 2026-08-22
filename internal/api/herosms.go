package api

import (
	"net/http"
	"strings"

	"opencode-go-manager/internal/herosms"
)

func (s *Server) heroSMSCatalog(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetSettings()
	if err != nil {
		st.Headless = true
	}
	key := strings.TrimSpace(r.URL.Query().Get("api_key"))
	if key == "" {
		key = strings.TrimSpace(st.HeroSMSAPIKey)
	}
	svc := strings.TrimSpace(r.URL.Query().Get("service"))
	if svc == "" {
		svc = strings.TrimSpace(st.HeroSMSService)
	}
	c := herosms.New(key, svc)
	cat, err := c.Catalog()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cat)
}
