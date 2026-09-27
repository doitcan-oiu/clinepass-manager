package api

import (
	"encoding/json"
	"net/http"
	"runtime"
	"strings"

	"opencode-go-manager/internal/model"
	"opencode-go-manager/internal/store"
)

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.publicConfig())
}

// The manager only persists forwarding and usage settings. Automation settings
// are owned by Auto's database and edited through /api/auto/config.
func (s *Server) patchConfig(w http.ResponseWriter, r *http.Request) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&raw); err != nil || raw == nil {
		writeErr(w, http.StatusBadRequest, "JSON 无效")
		return
	}
	allowed := map[string]bool{
		"proxy": true, "max_retries": true, "account_rpm": true, "api_proxy": true,
		"usage_refresh_sec": true, "model_usage_refresh_sec": true, "usage_refresh_concurrency": true,
		"provider_mode": true, "provider_value": true,
	}
	for key := range raw {
		if !allowed[key] {
			writeErr(w, http.StatusBadRequest, "主程序仅接受转发和用量设置；自动化设置请使用 /api/auto/config，连接设置请使用 /api/auto/connection")
			return
		}
	}
	payload, _ := json.Marshal(raw)
	var in struct {
		Proxy                   *string `json:"proxy"`
		MaxRetries              *int    `json:"max_retries"`
		AccountRPM              *int    `json:"account_rpm"`
		APIProxy                *bool   `json:"api_proxy"`
		UsageRefreshSec         *int    `json:"usage_refresh_sec"`
		ModelUsageRefreshSec    *int    `json:"model_usage_refresh_sec"`
		UsageRefreshConcurrency *int    `json:"usage_refresh_concurrency"`
		ProviderMode            *string `json:"provider_mode"`
		ProviderValue           *string `json:"provider_value"`
	}
	if json.Unmarshal(payload, &in) != nil {
		writeErr(w, http.StatusBadRequest, "设置值类型无效")
		return
	}
	cur, err := s.store.GetSettings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取主程序设置失败")
		return
	}
	if in.Proxy != nil {
		cur.Proxy = *in.Proxy
	}
	if in.MaxRetries != nil {
		if *in.MaxRetries < 0 || *in.MaxRetries > 32 {
			writeErr(w, http.StatusBadRequest, "失败换号次数须在 0–32")
			return
		}
		cur.MaxRetries = *in.MaxRetries
	}
	if in.AccountRPM != nil {
		if *in.AccountRPM < 1 || *in.AccountRPM > 1000 {
			writeErr(w, http.StatusBadRequest, "单账号 RPM 须在 1–1000")
			return
		}
		cur.AccountRPM = *in.AccountRPM
	}
	if in.APIProxy != nil {
		cur.APIProxy = *in.APIProxy
	}
	if in.ProviderMode != nil {
		cur.ProviderMode = strings.TrimSpace(*in.ProviderMode)
	}
	if in.ProviderValue != nil {
		cur.ProviderValue = *in.ProviderValue
	}
	if in.UsageRefreshSec != nil {
		if *in.UsageRefreshSec < 15 || *in.UsageRefreshSec > 86400 {
			writeErr(w, http.StatusBadRequest, "套餐配额刷新间隔须在 15–86400 秒")
			return
		}
		cur.UsageRefreshSec = *in.UsageRefreshSec
	}
	if in.ModelUsageRefreshSec != nil {
		if *in.ModelUsageRefreshSec < 15 || *in.ModelUsageRefreshSec > 86400 {
			writeErr(w, http.StatusBadRequest, "模型用量刷新间隔须在 15–86400 秒")
			return
		}
		cur.ModelUsageRefreshSec = *in.ModelUsageRefreshSec
	}
	if in.UsageRefreshConcurrency != nil {
		if *in.UsageRefreshConcurrency < 1 || *in.UsageRefreshConcurrency > 64 {
			writeErr(w, http.StatusBadRequest, "用量刷新并发须在 1–64")
			return
		}
		cur.UsageRefreshConcurrency = *in.UsageRefreshConcurrency
	}
	if err := s.store.SaveSettings(cur); err != nil {
		writeErr(w, http.StatusInternalServerError, "保存主程序设置失败")
		return
	}
	writeJSON(w, http.StatusOK, s.publicConfig())
}

func (s *Server) publicConfig() map[string]any {
	cfg := s.cfg
	st, err := s.store.GetSettings()
	if err == nil {
		cfg = store.ApplySettings(cfg, st)
	} else {
		st = model.Settings{AccountRPM: 5, APIProxy: true, UsageRefreshSec: 60, ModelUsageRefreshSec: 600, UsageRefreshConcurrency: 10}
	}
	return map[string]any{
		"proxy":                     cfg.Proxy,
		"max_retries":               cfg.MaxRetries,
		"account_rpm":               st.AccountRPM,
		"api_proxy":                 st.APIProxy,
		"usage_refresh_sec":         st.UsageRefreshSec,
		"model_usage_refresh_sec":   st.ModelUsageRefreshSec,
		"usage_refresh_concurrency": st.UsageRefreshConcurrency,
		"provider_mode":             st.ProviderMode,
		"provider_value":            st.ProviderValue,
		"platform":                  runtime.GOOS + "-" + runtime.GOARCH,
	}
}
