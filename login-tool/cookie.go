package main

import (
	"strings"
)

type cookieKV struct {
	Name  string
	Value string
}

func parseCookieHeader(raw string) []cookieKV {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ";")
	out := make([]cookieKV, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, ok := strings.Cut(part, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			continue
		}
		out = append(out, cookieKV{Name: name, Value: strings.TrimSpace(value)})
	}
	return out
}

func cookiePrefix(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(n, "__host-"):
		return "host"
	case strings.HasPrefix(n, "__secure-"):
		return "secure"
	default:
		return ""
	}
}

func skipCookie(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return true
	}
	exact := []string{
		"sid", "hsid", "ssid", "apisid", "sapisid", "nid",
		"g_state", "account_chooser", "signinoptions",
		"estsauth", "estsauthpersistent", "mspok", "mspauth",
	}
	for _, key := range exact {
		if n == key {
			return true
		}
	}
	if strings.Contains(n, "msaauth") || strings.Contains(n, "msauth") {
		return true
	}
	if strings.HasPrefix(n, "__host-ms") || strings.HasPrefix(n, "__secure-ms") {
		return true
	}
	return false
}

func dashboardCookieURLs() []string {
	return []string{
		"https://app.cline.bot/",
		"https://authkit.cline.bot/",
	}
}
