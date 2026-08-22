package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Addr            string
	DataDir         string
	InviteURL       string
	Headless        bool
	SlowMo          float64
	CloakVersion    string
	CloakCacheDir   string
	CloakBinaryPath string
	LicenseKey      string
	MaxConcurrent   int
	MaxRetries      int
	Proxy           string
	HeroSMSAPIKey   string
	HeroSMSService  string
	HeroSMSCountry  int
	HeroSMSMaxPrice float64
}

func Load() Config {
	return Config{
		Addr:            env("ADDR", ":9999"),
		DataDir:         env("DATA_DIR", "./data"),
		InviteURL:       env("INVITE_URL", "https://authkit.cline.bot"),
		Headless:        envBool("HEADLESS", true),
		SlowMo:          envFloat("SLOW_MO", 0),
		CloakVersion:    env("CLOAKBROWSER_VERSION", "146.0.7680.177.5"),
		CloakCacheDir:   env("CLOAKBROWSER_CACHE_DIR", ""),
		CloakBinaryPath: env("CLOAKBROWSER_BINARY_PATH", ""),
		LicenseKey:      env("CLOAKBROWSER_LICENSE_KEY", ""),
		MaxConcurrent:   envInt("MAX_CONCURRENT", 1),
		MaxRetries:      envInt("MAX_RETRIES", 3),
		Proxy:           env("PROXY", ""),
	}
}

func (c Config) DBPath() string {
	return filepath.Join(c.DataDir, "manager.db")
}

func (c Config) ProfilesDir() string {
	return filepath.Join(c.DataDir, "profiles")
}

func (c Config) ScreenshotsDir() string {
	return filepath.Join(c.DataDir, "screenshots")
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envFloat(key string, fallback float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return n
}
