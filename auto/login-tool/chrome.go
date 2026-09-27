package main

import (
	"context"
	"fmt"
	"os"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

type browserSession struct {
	allocCancel context.CancelFunc
	cancel      context.CancelFunc
	ctx         context.Context
	profile     string
}

func openPayBrowser(chromePath string, row payRow) (*browserSession, error) {
	profile, err := os.MkdirTemp("", "cline-pay-*")
	if err != nil {
		return nil, err
	}
	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(chromePath),
		chromedp.UserDataDir(profile),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-features", "Translate,MediaRouter"),
		chromedp.Flag("no-default-browser-check", true),
	}
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx)
	sess := &browserSession{allocCancel: allocCancel, cancel: cancel, ctx: ctx, profile: profile}

	const dashURL = "https://app.cline.bot/dashboard"
	cookies := parseCookieHeader(row.Cookie)
	if len(cookies) == 0 {
		sess.Close()
		return nil, fmt.Errorf("这一行没有 Cookie")
	}
	// 必须用长期活着的 ctx 启动 Chrome。chromedp 内部是 CommandContext(ctx)，
	// 若这里再包一层 WithTimeout，Run 结束后 cancel 会把浏览器一起杀掉。
	if err := chromedp.Run(ctx,
		network.Enable(),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return injectCookies(ctx, cookies, dashURL)
		}),
		chromedp.Navigate(dashURL),
	); err != nil {
		sess.Close()
		return nil, fmt.Errorf("打开浏览器失败: %w", err)
	}
	return sess, nil
}

func injectCookies(ctx context.Context, cookies []cookieKV, _ string) error {
	ok := 0
	for _, c := range cookies {
		if skipCookie(c.Name) {
			continue
		}
		if err := setClineCookie(ctx, c); err != nil {
			fmt.Printf("跳过 Cookie %s：%s\n", c.Name, err)
			continue
		}
		ok++
	}
	if ok == 0 {
		return fmt.Errorf("没有写入任何可用于 app.cline.bot 的 Cookie")
	}
	fmt.Printf("已写入 %d 条 Cline Cookie\n", ok)
	return nil
}

func setClineCookie(ctx context.Context, c cookieKV) error {
	switch cookiePrefix(c.Name) {
	case "host", "secure":
		var last error
		for _, rawURL := range dashboardCookieURLs() {
			expr := network.SetCookie(c.Name, c.Value).
				WithURL(rawURL).
				WithPath("/").
				WithSecure(true)
			if err := expr.Do(ctx); err != nil {
				last = err
				continue
			}
			last = nil
			break
		}
		return last
	default:
		return network.SetCookie(c.Name, c.Value).
			WithDomain(".cline.bot").
			WithPath("/").
			WithSecure(true).
			Do(ctx)
	}
}

func (s *browserSession) Close() {
	if s == nil {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.allocCancel != nil {
		s.allocCancel()
	}
	if s.profile != "" {
		_ = os.RemoveAll(s.profile)
	}
}
