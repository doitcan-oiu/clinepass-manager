package browser

import (
	"os"
	"path/filepath"
	"strings"
)

func isProxyEnv(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "all_proxy", "http_proxy", "https_proxy", "ftp_proxy", "no_proxy",
		"socks_proxy", "socks5_proxy", "playwright_proxy":
		return true
	default:
		return false
	}
}

func browserLaunchEnv(license string) map[string]string {
	env := map[string]string{}
	for _, kv := range os.Environ() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" || isProxyEnv(k) {
			continue
		}
		env[k] = v
	}
	if key := firstNonEmpty(strings.TrimSpace(license), readLicenseFile()); key != "" {
		env["CLOAKBROWSER_LICENSE_KEY"] = key
	}
	return env
}

func readLicenseFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(home, ".cloakbrowser", "license.key"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func systemProxySet() bool {
	for _, kv := range os.Environ() {
		k, v, ok := strings.Cut(kv, "=")
		if ok && isProxyEnv(k) && strings.TrimSpace(v) != "" && !strings.EqualFold(k, "no_proxy") {
			return true
		}
	}
	return false
}
