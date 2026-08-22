package config

import (
	"fmt"
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

func (c Config) RuntimeHome() string {
	return filepath.Join(c.DataDir, "home")
}

func DirWritable(dir string) bool {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return false
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	f, err := os.CreateTemp(dir, ".write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

func currentHome() string {
	if v := strings.TrimSpace(os.Getenv("HOME")); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// PrepareRuntime 保证数据目录可写。systemd ProtectHome 会把 /root、/home 挂成只读，
// Cloak 默认写 $HOME/.cloakbrowser，这时改用 data/home。
func (c Config) PrepareRuntime() (Config, string, error) {
	for _, dir := range []string{c.DataDir, c.ProfilesDir(), c.ScreenshotsDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return c, "", fmt.Errorf("创建目录失败: %w", err)
		}
	}
	note := ""
	home := currentHome()
	if !DirWritable(home) {
		runtimeHome := c.RuntimeHome()
		if !DirWritable(runtimeHome) {
			return c, "", fmt.Errorf("运行 HOME 不可写: %s", runtimeHome)
		}
		if err := os.Setenv("HOME", runtimeHome); err != nil {
			return c, "", err
		}
		if home == "" {
			note = "HOME 为空，已改用 " + runtimeHome
		} else {
			note = "HOME=" + home + " 不可写（常见于 systemd ProtectHome），已改用 " + runtimeHome
		}
		home = runtimeHome
	}
	if strings.TrimSpace(c.CloakCacheDir) == "" {
		c.CloakCacheDir = filepath.Join(home, ".cloakbrowser")
	}
	if err := os.MkdirAll(c.CloakCacheDir, 0o755); err != nil {
		return c, note, fmt.Errorf("创建 Cloak 缓存目录失败: %w", err)
	}
	if err := os.Setenv("CLOAKBROWSER_CACHE_DIR", c.CloakCacheDir); err != nil {
		return c, note, err
	}
	return c, note, nil
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
