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
	LoginEngine     string
	LoginPython     string
	ConfigFile      string
}

func Load() (Config, error) {
	c := defaults()
	path, required, err := discoverFile()
	if err != nil {
		return c, err
	}
	if path != "" {
		if err := loadFile(path, &c); err != nil {
			if required || !os.IsNotExist(err) {
				return c, err
			}
		} else {
			c.ConfigFile = path
		}
	}
	applyEnv(&c)
	c.Addr = normalizeAddr(c.Addr)
	if strings.TrimSpace(c.DataDir) == "" {
		c.DataDir = "./data"
	}
	c.applyProcessEnv()
	return c, nil
}

func defaults() Config {
	return Config{
		Addr:          ":9999",
		DataDir:       "./data",
		InviteURL:     "https://authkit.cline.bot",
		Headless:      true,
		MaxConcurrent: 1,
		MaxRetries:    3,
		CloakVersion:  "146.0.7680.177.5",
		LoginEngine:   "python",
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

func (c Config) RuntimeDir() string {
	return filepath.Join(c.DataDir, "run")
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
	if abs, err := filepath.Abs(c.DataDir); err == nil {
		c.DataDir = abs
	}
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
	if note2, err := c.prepareRuntimeDir(); err != nil {
		return c, note, err
	} else if note2 != "" {
		if note == "" {
			note = note2
		} else {
			note = note + "；" + note2
		}
	}
	return c, note, nil
}

func currentRuntimeDir() string {
	if v := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")); v != "" {
		return v
	}
	return filepath.Join("/run/user", strconv.Itoa(os.Getuid()))
}

func (c Config) prepareRuntimeDir() (string, error) {
	cur := currentRuntimeDir()
	if DirWritable(cur) {
		return "", nil
	}
	dir := c.RuntimeDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("创建 XDG_RUNTIME_DIR 失败: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", err
	}
	if !DirWritable(dir) {
		return "", fmt.Errorf("XDG_RUNTIME_DIR 不可写: %s", dir)
	}
	if err := os.Setenv("XDG_RUNTIME_DIR", dir); err != nil {
		return "", err
	}
	if cur == "" {
		return "XDG_RUNTIME_DIR 为空，已改用 " + dir, nil
	}
	return "XDG_RUNTIME_DIR=" + cur + " 不可写（常见于 systemd ProtectHome 屏蔽 /run/user），已改用 " + dir, nil
}

func applyEnv(c *Config) {
	if v := strings.TrimSpace(os.Getenv("ADDR")); v != "" {
		c.Addr = v
	}
	if v := strings.TrimSpace(os.Getenv("DATA_DIR")); v != "" {
		c.DataDir = v
	}
	if v := strings.TrimSpace(os.Getenv("INVITE_URL")); v != "" {
		c.InviteURL = v
	}
	if strings.TrimSpace(os.Getenv("HEADLESS")) != "" {
		c.Headless = envBool("HEADLESS", c.Headless)
	}
	if strings.TrimSpace(os.Getenv("SLOW_MO")) != "" {
		c.SlowMo = envFloat("SLOW_MO", c.SlowMo)
	}
	if v := strings.TrimSpace(os.Getenv("CLOAKBROWSER_VERSION")); v != "" {
		c.CloakVersion = v
	}
	if v := strings.TrimSpace(os.Getenv("CLOAKBROWSER_CACHE_DIR")); v != "" {
		c.CloakCacheDir = v
	}
	if v := strings.TrimSpace(os.Getenv("CLOAKBROWSER_BINARY_PATH")); v != "" {
		c.CloakBinaryPath = v
	}
	if v := strings.TrimSpace(os.Getenv("CLOAKBROWSER_LICENSE_KEY")); v != "" {
		c.LicenseKey = v
	}
	if strings.TrimSpace(os.Getenv("MAX_CONCURRENT")) != "" {
		c.MaxConcurrent = envInt("MAX_CONCURRENT", c.MaxConcurrent)
	}
	if strings.TrimSpace(os.Getenv("MAX_RETRIES")) != "" {
		c.MaxRetries = envInt("MAX_RETRIES", c.MaxRetries)
	}
	if v := strings.TrimSpace(os.Getenv("PROXY")); v != "" {
		c.Proxy = v
	}
	if v := strings.TrimSpace(os.Getenv("LOGIN_ENGINE")); v != "" {
		c.LoginEngine = v
	}
	if v := strings.TrimSpace(os.Getenv("LOGIN_PYTHON")); v != "" {
		c.LoginPython = v
	}
}

func (c Config) applyProcessEnv() {
	if strings.TrimSpace(c.LoginEngine) != "" {
		_ = os.Setenv("LOGIN_ENGINE", c.LoginEngine)
	}
	if strings.TrimSpace(c.LoginPython) != "" {
		_ = os.Setenv("LOGIN_PYTHON", c.LoginPython)
	}
}

func normalizeAddr(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ":9999"
	}
	if !strings.Contains(s, ":") {
		return ":" + s
	}
	return s
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
