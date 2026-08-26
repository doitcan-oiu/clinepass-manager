package login

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"opencode-go-manager/internal/browser"
	"opencode-go-manager/internal/cline"
	"opencode-go-manager/internal/config"
	"opencode-go-manager/internal/model"
)

type workerJob struct {
	Action   string         `json:"action"`
	Account  workerAccount  `json:"account"`
	Settings workerSettings `json:"settings"`
}

type workerAccount struct {
	ID              string `json:"id"`
	Email           string `json:"email"`
	Password        string `json:"password"`
	RecoveryEmail   string `json:"recovery_email"`
	FingerprintSeed int    `json:"fingerprint_seed"`
	LoginProvider   string `json:"login_provider"`
	CookiesJSON     string `json:"cookies_json"`
	CookieHeader    string `json:"cookie_header"`
}

type workerSettings struct {
	InviteURL       string  `json:"invite_url"`
	Headless        bool    `json:"headless"`
	Proxy           string  `json:"proxy"`
	HeroSMSAPIKey   string  `json:"hero_sms_api_key"`
	HeroSMSService  string  `json:"hero_sms_service"`
	HeroSMSCountry  int     `json:"hero_sms_country"`
	HeroSMSMaxPrice float64 `json:"hero_sms_max_price"`
	ProfileDir      string  `json:"profile_dir"`
	ScreenshotPath  string  `json:"screenshot_path"`
	CloakVersion    string  `json:"cloak_version"`
	CloakCacheDir   string  `json:"cloak_cache_dir"`
	CloakBinaryPath string  `json:"cloak_binary_path"`
	LicenseKey      string  `json:"license_key"`
	VirtualDisplay  bool    `json:"virtual_display"`
	AutoPay         bool    `json:"auto_pay"`
	ManagerAPI      string  `json:"manager_api"`
}

type workerMsg struct {
	Type         string `json:"type"`
	Msg          string `json:"msg"`
	OK           bool   `json:"ok"`
	Error        string `json:"error"`
	Code         string `json:"code"`
	CookiesJSON  string `json:"cookies_json"`
	CookieHeader string `json:"cookie_header"`
	PaymentURL   string `json:"payment_url"`
	Paid         bool   `json:"paid"`
	PayError     string `json:"pay_error"`
}

func runPythonOnce(cfg config.Config, acc model.Account, action string, log Logger) (Result, error) {
	if log == nil {
		log = func(string, ...any) {}
	}
	python, script, err := findWorker()
	if err != nil {
		return Result{}, err
	}
	headless := browser.PrepareDisplay(cfg.Headless, log)
	invite := strings.TrimSpace(cfg.InviteURL)
	if invite == "" {
		invite = cline.AuthURL
	}
	if err := os.MkdirAll(cfg.ProfilesDir(), 0o755); err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(cfg.ScreenshotsDir(), 0o755); err != nil {
		return Result{}, err
	}
	shot := filepath.Join(cfg.ScreenshotsDir(), acc.ID+".png")
	if action == "refresh" {
		shot = filepath.Join(cfg.ScreenshotsDir(), acc.ID+"-pay.png")
	}
	job := workerJob{
		Action: action,
		Account: workerAccount{
			ID:              acc.ID,
			Email:           acc.Email,
			Password:        acc.Password,
			RecoveryEmail:   acc.RecoveryEmail,
			FingerprintSeed: acc.FingerprintSeed,
			LoginProvider:   model.NormalizeLoginProvider(acc.LoginProvider),
			CookiesJSON:     acc.CookiesJSON,
			CookieHeader:    acc.CookieHeader,
		},
		Settings: workerSettings{
			InviteURL:       invite,
			Headless:        headless,
			Proxy:           cfg.Proxy,
			HeroSMSAPIKey:   cfg.HeroSMSAPIKey,
			HeroSMSService:  cfg.HeroSMSService,
			HeroSMSCountry:  cfg.HeroSMSCountry,
			HeroSMSMaxPrice: cfg.HeroSMSMaxPrice,
			ProfileDir:      filepath.Join(cfg.ProfilesDir(), acc.ID),
			ScreenshotPath:  shot,
			CloakVersion:    cfg.CloakVersion,
			CloakCacheDir:   cfg.CloakCacheDir,
			CloakBinaryPath: cfg.CloakBinaryPath,
			LicenseKey:      browser.ResolveLicense(cfg.LicenseKey),
			VirtualDisplay:  browser.VirtualDisplay() != "",
			AutoPay:         cfg.AutoPay,
			ManagerAPI:      cfg.ManagerAPI(),
		},
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return Result{}, err
	}
	log("登录引擎=python cloak humanize/geoip")
	cmd := exec.Command(python, script)
	browser.IsolateProcess(cmd)
	cmd.Dir = filepath.Dir(filepath.Dir(script))
	cmd.Stdin = strings.NewReader(string(payload))
	cmd.Env = workerEnv(cfg)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("启动登录工人失败: %w", err)
	}
	var last workerMsg
	gotResult := false
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var msg workerMsg
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			log("%s", line)
			continue
		}
		switch msg.Type {
		case "log":
			if msg.Msg != "" {
				log("%s", msg.Msg)
			}
		case "result":
			last = msg
			gotResult = true
		}
	}
	waitErr := cmd.Wait()
	if err := sc.Err(); err != nil {
		return Result{}, fmt.Errorf("读取登录工人输出失败: %w", err)
	}
	if !gotResult {
		if waitErr != nil {
			return Result{}, fmt.Errorf("登录工人异常退出: %w", waitErr)
		}
		return Result{}, fmt.Errorf("登录工人没有返回结果")
	}
	if !last.OK {
		return Result{}, mapWorkerCode(last.Code, last.Error)
	}
	res := Result{
		Email:        acc.Email,
		CookiesJSON:  last.CookiesJSON,
		CookieHeader: last.CookieHeader,
		PaymentURL:   last.PaymentURL,
		WorkspaceID:  acc.WorkspaceID,
		APIKey:       acc.APIKey,
		UserID:       acc.UserID,
		Paid:         last.Paid,
		PayError:     last.PayError,
	}
	if action != "refresh" {
		key, userID, err := createClineKey(cfg, res.CookieHeader, "", "", log)
		if err != nil {
			return Result{}, err
		}
		res.APIKey = key
		res.UserID = userID
		res.WorkspaceID = userID
		log("用户 ID: %s", userID)
	}
	return res, nil
}

func mapWorkerCode(code, msg string) error {
	switch strings.TrimSpace(code) {
	case "radar_denied":
		return ErrRadarDenied
	case "banned":
		return ErrAccountBanned
	case "sms_relogin":
		return ErrSMSNeedRelogin
	case "sms_timeout":
		return ErrPhoneTimeout
	case "authkit_stuck":
		if msg == "" {
			return ErrAuthkitStuck
		}
		return CompactError(fmt.Errorf("%w: %s", ErrAuthkitStuck, msg))
	default:
		if strings.TrimSpace(msg) == "" {
			return fmt.Errorf("登录工人失败")
		}
		return CompactError(fmt.Errorf("%s", msg))
	}
}

func findWorker() (python, script string, err error) {
	root, err := FindRepoRoot()
	if err != nil {
		return "", "", err
	}
	script = filepath.Join(root, "worker", "login.py")
	if st, e := os.Stat(script); e != nil || st.IsDir() {
		return "", "", fmt.Errorf("未找到登录工人 %s", script)
	}
	if p := strings.TrimSpace(os.Getenv("LOGIN_PYTHON")); p != "" {
		return p, script, nil
	}
	venv := filepath.Join(root, "worker", ".venv", "bin", "python")
	if st, e := os.Stat(venv); e == nil && !st.IsDir() {
		return venv, script, nil
	}
	if p, e := exec.LookPath("python3"); e == nil {
		return p, script, nil
	}
	return "", "", fmt.Errorf("未找到 Python，请先执行 make worker-venv")
}

func FindRepoRoot() (string, error) {
	var starts []string
	if cwd, err := os.Getwd(); err == nil {
		starts = append(starts, cwd)
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		starts = append(starts, dir, filepath.Dir(dir))
	}
	for _, start := range starts {
		dir := start
		for i := 0; i < 6; i++ {
			if looksLikeRoot(dir) {
				return dir, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", fmt.Errorf("未找到 worker/login.py，请在项目根目录启动")
}

func looksLikeRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "worker", "login.py"))
	return err == nil
}

func workerEnv(cfg config.Config) []string {
	env := make([]string, 0, 32)
	for _, kv := range os.Environ() {
		k, _, ok := strings.Cut(kv, "=")
		if !ok || browser.IsProxyEnv(k) {
			continue
		}
		if strings.EqualFold(k, "CLOAKBROWSER_CACHE_DIR") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "PYTHONUNBUFFERED=1")
	if key := browser.ResolveLicense(cfg.LicenseKey); key != "" {
		env = append(env, "CLOAKBROWSER_LICENSE_KEY="+key)
		if cfg.CloakVersion != "" {
			env = append(env, "CLOAKBROWSER_VERSION="+cfg.CloakVersion)
		}
	}
	cacheDir := strings.TrimSpace(cfg.CloakCacheDir)
	if cacheDir == "" {
		cacheDir = strings.TrimSpace(os.Getenv("CLOAKBROWSER_CACHE_DIR"))
	}
	if cacheDir != "" {
		env = append(env, "CLOAKBROWSER_CACHE_DIR="+cacheDir)
	}
	if cfg.CloakBinaryPath != "" {
		env = append(env, "CLOAKBROWSER_BINARY_PATH="+cfg.CloakBinaryPath)
	}
	return env
}
