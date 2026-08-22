package browser

import (
	"fmt"
	"net/url"
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

	args := DefaultStealthArgs(opt.Seed)
	launch := playwright.BrowserTypeLaunchPersistentContextOptions{
		ExecutablePath:    playwright.String(info.Path),
		Headless:          playwright.Bool(opt.Headless),
		Args:              args,
		IgnoreDefaultArgs: IgnoreDefaultArgs(),
		Locale:            playwright.String(firstNonEmpty(opt.Locale, "en-US")),
		TimezoneId:        playwright.String(firstNonEmpty(opt.Timezone, "America/New_York")),
		ColorScheme:       playwright.ColorSchemeLight,
		AcceptDownloads:   playwright.Bool(true),
	}
	if opt.SlowMo > 0 {
		launch.SlowMo = playwright.Float(opt.SlowMo)
	}
	if opt.Headless {
		launch.Viewport = &playwright.Size{Width: 1920, Height: 947}
	} else {
		launch.NoViewport = playwright.Bool(true)
	}
	if proxy := parseProxy(opt.Proxy); proxy != nil {
		launch.Proxy = proxy
	}
	ephemeral := false
	if strings.TrimSpace(opt.UserDataDir) == "" {
		dir, err := os.MkdirTemp("", "cloak-guest-*")
		if err != nil {
			_ = pw.Stop()
			return nil, fmt.Errorf("创建访客目录失败: %w", err)
		}
		opt.UserDataDir = dir
		ephemeral = true
		logf("访客模式，独立配置目录")
	} else if err := os.MkdirAll(opt.UserDataDir, 0o755); err != nil {
		_ = pw.Stop()
		return nil, err
	}

	ctx, err := pw.Chromium.LaunchPersistentContext(opt.UserDataDir, launch)
	if err != nil {
		_ = pw.Stop()
		if ephemeral {
			_ = os.RemoveAll(opt.UserDataDir)
		}
		return nil, fmt.Errorf("启动 CloakBrowser 失败: %w", err)
	}
	pages := ctx.Pages()
	var page playwright.Page
	if len(pages) > 0 {
		page = pages[0]
	} else {
		page, err = ctx.NewPage()
		if err != nil {
			_ = ctx.Close()
			_ = pw.Stop()
			if ephemeral {
				_ = os.RemoveAll(opt.UserDataDir)
			}
			return nil, err
		}
	}
	return &Session{PW: pw, Context: ctx, Page: page, Binary: info, userDataDir: opt.UserDataDir, ephemeral: ephemeral}, nil
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
	if s.ephemeral && s.userDataDir != "" {
		_ = os.RemoveAll(s.userDataDir)
	}
}

func parseProxy(raw string) *playwright.Proxy {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return &playwright.Proxy{Server: raw}
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
