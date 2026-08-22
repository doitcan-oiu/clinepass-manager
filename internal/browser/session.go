package browser

import (
	"fmt"
	"os"
	"strings"

	"github.com/mxschmitt/playwright-go"

	"opencode-go-manager/internal/config"
)

type Session struct {
	PW          *playwright.Playwright
	Context     playwright.BrowserContext
	Page        playwright.Page
	Binary      BinaryInfo
	userDataDir string
	ephemeral   bool
	relay       *Relay
}

type LaunchOptions struct {
	UserDataDir string
	Seed        int
	Headless    bool
	SlowMo      float64
	Proxy       string
	Locale      string
	Timezone    string
}

func InstallDriver() error {
	return playwright.Install(&playwright.RunOptions{
		SkipInstallBrowsers: true,
	})
}

func Launch(cfg config.Config, opt LaunchOptions, logf func(string, ...any)) (*Session, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	info, err := EnsureBinary(cfg.CloakVersion, cfg.CloakCacheDir, cfg.CloakBinaryPath, logf)
	if err != nil {
		return nil, err
	}
	if err := InstallDriver(); err != nil {
		logf("安装 Playwright driver 失败，将继续尝试启动: %v", err)
	}
	pw, err := playwright.Run(&playwright.RunOptions{SkipInstallBrowsers: true})
	if err != nil {
		return nil, fmt.Errorf("启动 Playwright 失败: %w", err)
	}

	if !opt.Headless && !hasDisplay() {
		opt.Headless = true
		logf("服务器没有图形界面，已改为无头模式")
	}
	args := DefaultStealthArgs(opt.Seed)
	args = append(args, "--disable-setuid-sandbox", "--disable-dev-shm-usage")
	if opt.Headless {
		args = append(args, "--ozone-platform=headless", "--disable-gpu")
	} else if strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) != "" {
		args = append(args, "--ozone-platform=x11")
		logf("有界面模式遇到 Wayland，改走 X11，避免 Chrome 启动即退出")
	}
	if systemProxySet() && strings.TrimSpace(opt.Proxy) == "" {
		logf("已忽略环境变量里的系统代理，浏览器只使用设置里的代理")
	}
	launch := playwright.BrowserTypeLaunchPersistentContextOptions{
		ExecutablePath:    playwright.String(info.Path),
		Headless:          playwright.Bool(opt.Headless),
		Args:              args,
		IgnoreDefaultArgs: IgnoreDefaultArgs(),
		Locale:            playwright.String(firstNonEmpty(opt.Locale, "en-US")),
		TimezoneId:        playwright.String(firstNonEmpty(opt.Timezone, "America/New_York")),
		ColorScheme:       playwright.ColorSchemeLight,
		AcceptDownloads:   playwright.Bool(true),
		Env:               browserLaunchEnv(cfg.LicenseKey),
	}
	if opt.SlowMo > 0 {
		launch.SlowMo = playwright.Float(opt.SlowMo)
	}
	if opt.Headless {
		launch.Viewport = &playwright.Size{Width: 1920, Height: 947}
	} else {
		launch.NoViewport = playwright.Bool(true)
	}
	browserProxy := strings.TrimSpace(opt.Proxy)
	var relay *Relay
	if needsSOCKSAuthRelay(browserProxy) {
		r, err := StartSOCKSRelay(browserProxy, logf)
		if err != nil {
			_ = pw.Stop()
			return nil, fmt.Errorf("启动本地代理中继失败: %w", err)
		}
		relay = r
		browserProxy = r.BrowserURL()
		logf("Chrome 不支持带账密的 SOCKS5，已走本地中继 %s", browserProxy)
	}
	if proxy := parseProxy(browserProxy); proxy != nil {
		launch.Proxy = proxy
	}
	cleanup := func() {
		if relay != nil {
			_ = relay.Close()
		}
		_ = pw.Stop()
	}
	ephemeral := false
	if strings.TrimSpace(opt.UserDataDir) == "" {
		dir, err := os.MkdirTemp("", "cloak-guest-*")
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("创建访客目录失败: %w", err)
		}
		opt.UserDataDir = dir
		ephemeral = true
		logf("访客模式，独立配置目录")
	} else if err := os.MkdirAll(opt.UserDataDir, 0o755); err != nil {
		cleanup()
		return nil, err
	}

	ctx, err := pw.Chromium.LaunchPersistentContext(opt.UserDataDir, launch)
	if err != nil {
		if ephemeral {
			_ = os.RemoveAll(opt.UserDataDir)
		}
		cleanup()
		return nil, launchError(info.Path, err)
	}
	pages := ctx.Pages()
	var page playwright.Page
	if len(pages) > 0 {
		page = pages[0]
	} else {
		page, err = ctx.NewPage()
		if err != nil {
			_ = ctx.Close()
			if ephemeral {
				_ = os.RemoveAll(opt.UserDataDir)
			}
			cleanup()
			return nil, err
		}
	}
	return &Session{PW: pw, Context: ctx, Page: page, Binary: info, userDataDir: opt.UserDataDir, ephemeral: ephemeral, relay: relay}, nil
}

func (s *Session) Close() {
	if s == nil {
		return
	}
	if s.Context != nil {
		_ = s.Context.Close()
	}
	if s.PW != nil {
		_ = s.PW.Stop()
	}
	if s.relay != nil {
		_ = s.relay.Close()
	}
	if s.ephemeral && s.userDataDir != "" {
		_ = os.RemoveAll(s.userDataDir)
	}
}

func parseProxy(raw string) *playwright.Proxy {
	u, err := parseProxyURL(raw)
	if err != nil {
		return &playwright.Proxy{Server: strings.TrimSpace(raw)}
	}
	if u == nil {
		return nil
	}
	p := &playwright.Proxy{Server: u.Scheme + "://" + u.Host}
	if u.User != nil {
		p.Username = playwright.String(u.User.Username())
		if pwd, ok := u.User.Password(); ok {
			p.Password = playwright.String(pwd)
		}
	}
	return p
}

func firstNonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}
