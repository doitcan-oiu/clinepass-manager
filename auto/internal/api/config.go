package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"opencode-go-manager/auto/internal/browser"
	"opencode-go-manager/internal/config"
	"opencode-go-manager/internal/store"
)

// This API deliberately excludes manager routing, provider and usage settings.
// It changes only the configuration persisted on the Auto server.
func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.publicConfig())
}

func (s *Server) patchConfig(w http.ResponseWriter, r *http.Request) {
	cur, err := s.store.GetSettings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var in struct {
		Proxy                 *string  `json:"proxy"`
		Headless              *bool    `json:"headless"`
		InviteURL             *string  `json:"invite_url"`
		HeroSMSAPIKey         *string  `json:"hero_sms_api_key"`
		HeroSMSService        *string  `json:"hero_sms_service"`
		HeroSMSCountry        *int     `json:"hero_sms_country"`
		HeroSMSMaxPrice       *float64 `json:"hero_sms_max_price"`
		MaxConcurrent         *int     `json:"max_concurrent"`
		MaxRetries            *int     `json:"max_retries"`
		CloakVersion          *string  `json:"cloak_version"`
		CloakLicenseKey       *string  `json:"cloak_license_key"`
		AmzKeysHost           *string  `json:"amzkeys_host"`
		AmzKeysAppID          *string  `json:"amzkeys_app_id"`
		AmzKeysAppKey         *string  `json:"amzkeys_app_key"`
		AmzKeysPrivateKey     *string  `json:"amzkeys_private_key"`
		AmzKeysCardType       *int     `json:"amzkeys_card_type"`
		AmzKeysCardAmount     *float64 `json:"amzkeys_card_amount"`
		CookieKeepEnabled     *bool    `json:"cookie_keep_enabled"`
		CookieKeepHour        *int     `json:"cookie_keep_hour"`
		CookieKeepConcurrency *int     `json:"cookie_keep_concurrency"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Auto 设置字段或 JSON 无效："+err.Error())
		return
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		writeErr(w, http.StatusBadRequest, "只能提交一个 JSON 对象")
		return
	}
	setString := func(dst *string, value *string, secret bool) {
		if value != nil && (!secret || !strings.Contains(*value, "********")) {
			*dst = strings.TrimSpace(*value)
		}
	}
	setString(&cur.Proxy, in.Proxy, false)
	setString(&cur.InviteURL, in.InviteURL, false)
	setString(&cur.HeroSMSAPIKey, in.HeroSMSAPIKey, true)
	setString(&cur.HeroSMSService, in.HeroSMSService, false)
	setString(&cur.CloakVersion, in.CloakVersion, false)
	setString(&cur.CloakLicenseKey, in.CloakLicenseKey, true)
	setString(&cur.AmzKeysHost, in.AmzKeysHost, false)
	setString(&cur.AmzKeysAppID, in.AmzKeysAppID, false)
	setString(&cur.AmzKeysAppKey, in.AmzKeysAppKey, true)
	setString(&cur.AmzKeysPrivateKey, in.AmzKeysPrivateKey, true)
	if in.Headless != nil {
		cur.Headless = *in.Headless
	}
	if in.CookieKeepEnabled != nil {
		cur.CookieKeepEnabled = *in.CookieKeepEnabled
	}
	checks := []struct {
		value    *int
		dst      *int
		min, max int
		name     string
	}{
		{in.MaxConcurrent, &cur.MaxConcurrent, 1, 128, "提取并发"},
		{in.MaxRetries, &cur.MaxRetries, 0, 32, "重试次数"},
		{in.HeroSMSCountry, &cur.HeroSMSCountry, 0, 10000, "短信国家"},
		{in.AmzKeysCardType, &cur.AmzKeysCardType, 0, 2147483647, "卡类型"},
		{in.CookieKeepHour, &cur.CookieKeepHour, 0, 23, "续 Cookie 小时"},
		{in.CookieKeepConcurrency, &cur.CookieKeepConcurrency, 1, 32, "续 Cookie 并发"},
	}
	for _, check := range checks {
		if check.value != nil {
			if *check.value < check.min || *check.value > check.max {
				writeErr(w, http.StatusBadRequest, check.name+"超出允许范围")
				return
			}
			*check.dst = *check.value
		}
	}
	if in.HeroSMSMaxPrice != nil {
		if *in.HeroSMSMaxPrice < 0 {
			writeErr(w, http.StatusBadRequest, "短信最高价格不能为负")
			return
		}
		cur.HeroSMSMaxPrice = *in.HeroSMSMaxPrice
	}
	if in.AmzKeysCardAmount != nil {
		if *in.AmzKeysCardAmount < 0 {
			writeErr(w, http.StatusBadRequest, "开卡金额不能为负")
			return
		}
		cur.AmzKeysCardAmount = *in.AmzKeysCardAmount
	}
	if err := s.store.SaveSettings(cur); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.reloadSettings()
	writeJSON(w, http.StatusOK, s.publicConfig())
}

func (s *Server) publicConfig() map[string]any {
	st, err := s.store.GetSettings()
	cfg := s.cfg
	if err == nil {
		cfg = store.ApplySettings(cfg, st)
	}
	last4, pending, payCount, maxPays, nextLast4, nextPending, lastErr := amzKeysCardView(s)
	return map[string]any{
		"invite_url": cfg.InviteURL, "headless": cfg.Headless, "proxy": cfg.Proxy,
		"cloak_version": cfg.CloakVersion, "cloak_license_key": maskSecret(cfg.LicenseKey),
		"cloak_license_configured": config.ResolveCloakLicense(cfg.LicenseKey) != "",
		"max_concurrent":           cfg.MaxConcurrent, "max_retries": cfg.MaxRetries, "platform": browser.PlatformTag(),
		"hero_sms_api_key": maskSecret(st.HeroSMSAPIKey), "hero_sms_configured": strings.TrimSpace(st.HeroSMSAPIKey) != "",
		"hero_sms_service": st.HeroSMSService, "hero_sms_country": st.HeroSMSCountry, "hero_sms_max_price": st.HeroSMSMaxPrice,
		"amzkeys_host": amzKeysHost(st.AmzKeysHost), "amzkeys_app_id": st.AmzKeysAppID,
		"amzkeys_app_key": maskSecret(st.AmzKeysAppKey), "amzkeys_private_key": maskSecret(st.AmzKeysPrivateKey),
		"amzkeys_card_type": amzKeysCardType(st.AmzKeysCardType), "amzkeys_card_amount": amzKeysCardAmount(st.AmzKeysCardAmount),
		"amzkeys_configured": amzKeysConfigured(st), "amzkeys_card_last4": last4,
		"amzkeys_card_pending": pending, "amzkeys_card_pay_count": payCount, "amzkeys_card_max_pays": maxPays,
		"amzkeys_card_next_last4": nextLast4, "amzkeys_card_next_pending": nextPending, "amzkeys_card_error": lastErr,
		"cookie_keep_enabled": st.CookieKeepEnabled, "cookie_keep_hour": max(0, min(23, st.CookieKeepHour)),
		"cookie_keep_concurrency": st.CookieKeepConcurrency, "cookie_keep_last_date": st.CookieKeepLastDate,
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
