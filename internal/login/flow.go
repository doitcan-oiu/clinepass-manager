package login

import (
	"encoding/json"
	"errors"
	"fmt"
	neturl "net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mxschmitt/playwright-go"

	"opencode-go-manager/internal/browser"
	"opencode-go-manager/internal/cline"
	"opencode-go-manager/internal/config"
	"opencode-go-manager/internal/model"
)

type Logger func(format string, args ...any)

type Result struct {
	WorkspaceID  string
	APIKey       string
	UserID       string
	Email        string
	CookiesJSON  string
	CookieHeader string
	PaymentURL   string
	Paid         bool
	PayError     string
}

func Run(cfg config.Config, acc model.Account, log Logger) (Result, error) {
	if log == nil {
		log = func(string, ...any) {}
	}
	var last error
	for round := 1; round <= 2; round++ {
		if round > 1 {
			log("两次都没收到验证码，重新走一遍登录（第 %d 轮）", round)
		}
		res, err := runOnceDispatch(cfg, acc, log)
		if err == nil {
			return res, nil
		}
		if !errors.Is(err, ErrSMSNeedRelogin) {
			return Result{}, err
		}
		last = err
	}
	if last != nil {
		log("两轮登录共 4 次接码都没收到验证码，跳过")
	}
	return Result{}, ErrPhoneTimeout
}

func runOnceDispatch(cfg config.Config, acc model.Account, log Logger) (Result, error) {
	if Engine() == "python" || cfg.AutoPay {
		return runPythonOnce(cfg, acc, "login", log)
	}
	return runOnce(cfg, acc, log)
}

func runOnce(cfg config.Config, acc model.Account, log Logger) (Result, error) {
	profile := ""
	shotDir := cfg.ScreenshotsDir()
	sess, err := browser.Launch(cfg, browser.LaunchOptions{
		UserDataDir: profile,
		Seed:        acc.FingerprintSeed,
		Headless:    cfg.Headless,
		SlowMo:      cfg.SlowMo,
		Proxy:       cfg.Proxy,
	}, log)
	if err != nil {
		return Result{}, err
	}
	defer sess.Close()

	page := sess.Page
	var result Result
	defer func() {
		if result.WorkspaceID == "" && result.APIKey == "" {
			_ = screenshot(page, filepath.Join(shotDir, acc.ID+".png"))
		}
	}()

	provider := model.NormalizeLoginProvider(acc.LoginProvider)
	invite := cfg.InviteURL
	if invite == "" {
		invite = cline.AuthURL
	}
	log("无头模式=%v", cfg.Headless)
	if cfg.Proxy != "" {
		log("使用全局代理")
	}
	log("登录方式=%s", provider)
	log("打开邀请链接 %s", invite)
	if _, err := page.Goto(invite, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(60000),
	}); err != nil {
		return Result{}, fmt.Errorf("打开邀请链接失败: %w", err)
	}
	sleep(2000)

	if onClineApp(page.URL()) && !strings.Contains(page.URL(), "radar-challenge") {
		log("当前已在 Cline，跳过身份登录")
	} else {
		if err := startIdentityLogin(page, acc, provider, log); err != nil {
			return Result{}, wrapIfAuthkit(err, page.URL())
		}
		log("等待进入 Cline")
		if err := waitCline(page, 180000, log, provider); err != nil {
			return Result{}, wrapIfAuthkit(err, page.URL())
		}
		if err := handleRadar(page, cfg, log); err != nil {
			return Result{}, CompactError(err)
		}
		if err := handleTerms(page, log); err != nil {
			return Result{}, CompactError(err)
		}
		if err := waitCline(page, 60000, log, provider); err != nil {
			return Result{}, wrapIfAuthkit(err, page.URL())
		}
	}

	if _, err := page.Goto(cline.AppBase+"/dashboard", playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(60000),
	}); err != nil {
		log("打开 dashboard 失败: %v", err)
	}
	sleep(800)
	if cookieExpired(page.URL()) {
		return Result{}, wrapIfAuthkit(fmt.Errorf("登录后没有进入 Cline，当前 URL=%s", page.URL()), page.URL())
	}

	cookies, err := sess.Context.Cookies()
	if err != nil {
		return Result{}, fmt.Errorf("读取 Cookie 失败: %w", err)
	}
	result.CookiesJSON, result.CookieHeader = serializeCookies(cookies)
	log("已保存 %d 条 Cookie", len(cookies))

	key, userID, err := createClineKey(cfg, result.CookieHeader, "", "", log)
	if err != nil {
		return Result{}, err
	}
	result.APIKey = key
	result.UserID = userID
	result.WorkspaceID = userID
	result.Email = acc.Email
	log("用户 ID: %s", userID)

	pay, err := captureClinePayment(page, log)
	if err != nil {
		log("获取支付链接失败: %v", err)
	} else {
		result.PaymentURL = pay
		log("支付链接: %s", pay)
	}
	return result, nil
}

func RefreshPayment(cfg config.Config, acc model.Account, log Logger) (Result, error) {
	if log == nil {
		log = func(string, ...any) {}
	}
	if Engine() == "python" || cfg.AutoPay {
		return runPythonOnce(cfg, acc, "refresh", log)
	}
	if strings.TrimSpace(acc.CookiesJSON) == "" && strings.TrimSpace(acc.CookieHeader) == "" {
		return Result{}, fmt.Errorf("没有可用 Cookie，需要先完整登录")
	}
	cookies, err := toOptionalCookies(acc.CookiesJSON)
	if err != nil {
		return Result{}, fmt.Errorf("Cookie 无效: %w", err)
	}

	shotDir := cfg.ScreenshotsDir()
	sess, err := browser.Launch(cfg, browser.LaunchOptions{
		Seed:     acc.FingerprintSeed,
		Headless: cfg.Headless,
		SlowMo:   cfg.SlowMo,
		Proxy:    cfg.Proxy,
	}, log)
	if err != nil {
		return Result{}, err
	}
	defer sess.Close()

	page := sess.Page
	var result Result
	result.WorkspaceID = acc.WorkspaceID
	result.APIKey = acc.APIKey
	result.UserID = acc.UserID
	result.Email = acc.Email
	defer func() {
		if result.PaymentURL == "" {
			_ = screenshot(page, filepath.Join(shotDir, acc.ID+"-pay.png"))
		}
	}()

	if err := sess.Context.AddCookies(cookies); err != nil {
		return Result{}, fmt.Errorf("写入 Cookie 失败: %w", err)
	}
	log("已注入 %d 条 Cookie，跳过谷歌登录", len(cookies))

	pay, err := captureClinePayment(page, log)
	if err != nil {
		return Result{}, CompactError(err)
	}
	result.PaymentURL = pay
	log("支付链接: %s", pay)

	fresh, err := sess.Context.Cookies()
	if err == nil && len(fresh) > 0 {
		result.CookiesJSON, result.CookieHeader = serializeCookies(fresh)
		log("已更新 %d 条 Cookie", len(fresh))
	} else {
		result.CookiesJSON = acc.CookiesJSON
		result.CookieHeader = acc.CookieHeader
	}
	return result, nil
}

func googleLogin(page playwright.Page, acc model.Account, log Logger) error {
	deadline := time.Now().Add(3 * time.Minute)
	accountChooser := fmt.Sprintf(`div[data-identifier="%s"]`, acc.Email)
	recoverySel := `input[name="knowledgePreregisteredEmailResponse"], input#knowledge-preregistered-email-response`
	emailSel := `input#identifierId:not([aria-hidden="true"])`
	passSel := `input[name="Passwd"], #password input[type="password"]`
	emailDone := false
	lastUnknown := time.Time{}
	lastAuthkitClick := time.Time{}

	for time.Now().Before(deadline) {
		rawURL := page.URL()
		if loggedIn(page) {
			log("已离开谷歌登录，当前 URL=%s", rawURL)
			return nil
		}
		step := classifyGoogle(rawURL)
		if step == "password" {
			emailDone = true
		}

		if urlHost(rawURL) == "authkit.cline.bot" {
			done, err := handleAuthkitWait(page, log, &lastAuthkitClick, model.LoginGoogle)
			if err != nil {
				return err
			}
			if done {
				return nil
			}
			continue
		}
		if step == "tos" {
			if err := acceptWorkspaceTos(page, log); err != nil {
				if loggedIn(page) {
					return nil
				}
				return stepErr(err)
			}
			continue
		}
		if step == "unknownerror" {
			if err := recoverGoogleUnknownError(page, log); err != nil {
				if loggedIn(page) {
					return nil
				}
				return stepErr(err)
			}
			continue
		}
		if step == "consent" {
			log("检测到授权页，点击同意授权")
			if err := clickOneOf(page, consentSelectors(), 25000, log, "同意授权"); err != nil {
				if loggedIn(page) {
					return nil
				}
				return stepErr(err)
			}
			log("等待离开授权页")
			if err := waitLeaveLogged(page, "/signin/oauth", 45000, log, "授权页"); err != nil {
				log("%v", err)
			}
			continue
		}
		if visible(page, recoverySel) && acc.RecoveryEmail != "" {
			log("填写辅助邮箱")
			if err := fillField(page, recoverySel, acc.RecoveryEmail); err != nil {
				return stepErr(err)
			}
			if err := clickFirst(page, []string{`#idvPreregisteredEmailNext button`, `div[id$="Next"] button`}, 15000, log, "辅助邮箱下一步"); err != nil {
				return stepErr(err)
			}
			sleep(1200)
			continue
		}
		if step == "chooser" || visible(page, accountChooser) {
			if visible(page, accountChooser) {
				log("选择已列出的账号")
				_ = clickFirst(page, []string{accountChooser}, 8000, log, "点击账号卡片")
			}
			_ = waitLeaveLogged(page, "accountchooser", 15000, log, "账号选择页")
			continue
		}

		if step == "password" || (emailDone && visible(page, passSel)) {
			if !visible(page, passSel) {
				log("已到密码页，等待密码框出现")
				if err := waitVisible(page, passSel, 20000); err != nil {
					if errors.Is(err, errLoggedIn) {
						return nil
					}
					sleep(500)
					continue
				}
			}
			log("填写 Google 密码")
			waitOverlayGone(page)
			if err := fillField(page, passSel, acc.Password); err != nil {
				return stepErr(err)
			}
			sleep(800)
			if err := clickFirst(page, []string{`#passwordNext button`}, 15000, log, "密码下一步"); err != nil {
				return stepErr(err)
			}
			log("等待离开密码页")
			_ = waitPathLeft(page, "/challenge/pwd", 30000)
			if leftGoogle(page) {
				return nil
			}
			continue
		}

		if !emailDone && (step == "email" || (step == "" && visible(page, emailSel) && !visible(page, passSel))) {
			log("填写 Google 账号")
			waitOverlayGone(page)
			if err := fillField(page, emailSel, acc.Email); err != nil {
				return stepErr(err)
			}
			sleep(800)
			if err := clickFirst(page, []string{`#identifierNext button`}, 15000, log, "账号下一步"); err != nil {
				return stepErr(err)
			}
			emailDone = true
			log("等待跳转到密码页")
			if err := waitPathContains(page, "/challenge/pwd", 30000); err != nil {
				log("尚未进入密码页，当前 %s", page.URL())
			}
			continue
		}
		if captchaVisible(page) {
			log("检测到验证码/安全检查，请在打开的浏览器中手动完成后等待")
			sleep(3000)
			continue
		}
		if strings.Contains(rawURL, "accounts.google.com") && time.Since(lastUnknown) > 8*time.Second {
			log("谷歌页面未识别，当前 URL=%s", rawURL)
			lastUnknown = time.Now()
		}
		sleep(500)
	}
	if leftGoogle(page) {
		return nil
	}
	return fmt.Errorf("Google 登录未完成，当前 URL=%s", page.URL())
}

func classifyGoogle(raw string) string {
	u, err := neturl.Parse(raw)
	if err != nil {
		return ""
	}
	p := u.Path
	switch {
	case strings.Contains(p, "workspacetermsofservice"):
		return "tos"
	case strings.Contains(p, "/challenge/pwd"):
		return "password"
	case strings.Contains(p, "/signin/identifier"), strings.Contains(p, "/v3/signin/identifier"):
		return "email"
	case strings.Contains(p, "/signin/oauth"):
		return "consent"
	case strings.Contains(p, "accountchooser"):
		return "chooser"
	case strings.Contains(p, "unknownerror"):
		return "unknownerror"
	default:
		return ""
	}
}

func isStripe(u string) bool {
	return strings.Contains(u, "stripe.com") || strings.Contains(u, "checkout.stripe.com")
}

func acceptWorkspaceTos(page playwright.Page, log Logger) error {
	if err := waitVisible(page, `#gaplustosNext button`, 15000); err != nil {
		return err
	}
	log("检测到服务条款页，准备同意")
	for i := 1; i <= 3; i++ {
		if classifyGoogle(page.URL()) != "tos" || loggedIn(page) {
			return nil
		}
		scrollTos(page)
		checkTosBoxes(page)
		log("点击同意服务条款（第 %d 次）", i)
		if err := clickTosButton(page); err != nil {
			if classifyGoogle(page.URL()) != "tos" || loggedIn(page) {
				return nil
			}
			log("第 %d 次点击未生效: %v", i, err)
		} else {
			log("同意服务条款 成功")
		}
		if err := waitLeaveLogged(page, "workspacetermsofservice", 12000, log, "服务条款页"); err == nil {
			return nil
		}
	}
	log("服务条款仍在，当前 URL=%s", page.URL())
	return nil
}

func recoverGoogleUnknownError(page playwright.Page, log Logger) error {
	log("谷歌返回 unknownerror，尝试恢复")
	_ = clickOneOf(page, []string{
		`#next button`,
		`div[id$="Next"] button`,
		`button[type="submit"]`,
	}, 2500, log, "错误页下一步")
	sleep(800)
	if classifyGoogle(page.URL()) != "unknownerror" {
		return nil
	}
	next := googleContinueURL(page.URL())
	if next == "" {
		return fmt.Errorf("谷歌 unknownerror，没有 continue 可跳转")
	}
	log("打开 continue 继续授权")
	if _, err := page.Goto(next, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(30000),
	}); err != nil {
		return fmt.Errorf("打开 continue 失败: %w", err)
	}
	sleep(800)
	if classifyGoogle(page.URL()) == "unknownerror" {
		return fmt.Errorf("谷歌 unknownerror 恢复失败，当前 URL=%s", page.URL())
	}
	return nil
}

func googleContinueURL(raw string) string {
	u, err := neturl.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(u.Query().Get("continue"))
}

func scrollTos(page playwright.Page) {
	humanScroll(page)
}

func checkTosBoxes(page playwright.Page) {
	loc := page.Locator(`#gaplustos input[type="checkbox"], form input[type="checkbox"]`)
	n, err := loc.Count()
	if err != nil || n == 0 {
		return
	}
	for i := 0; i < n && i < 4; i++ {
		box := loc.Nth(i)
		ok, err := box.IsVisible()
		if err != nil || !ok {
			continue
		}
		checked, _ := box.IsChecked()
		if !checked {
			_ = box.Check(playwright.LocatorCheckOptions{Timeout: playwright.Float(2000)})
		}
	}
}

func clickTosButton(page playwright.Page) error {
	waitOverlayGone(page)
	loc := page.Locator(`#gaplustosNext button`).First()
	ok, err := loc.IsVisible()
	if err != nil || !ok {
		return fmt.Errorf("未找到同意按钮")
	}
	_ = loc.ScrollIntoViewIfNeeded()
	if err := humanClick(page, loc); err == nil {
		return nil
	}
	_ = loc.Focus()
	if err := loc.Press("Enter", playwright.LocatorPressOptions{Timeout: playwright.Float(2000)}); err == nil {
		return nil
	}
	return fmt.Errorf("同意按钮点不下去")
}

func consentSelectors() []string {
	return []string{
		`div[jsname="uRHG6"] button`,
		`#submit_approve_access`,
		`button[data-idom-class*="P62QJc"]`,
	}
}

func startIdentityLogin(page playwright.Page, acc model.Account, provider string, log Logger) error {
	if urlHost(page.URL()) == "authkit.cline.bot" && !onRadarFlow(page) {
		humanIdleAuthkit(page, log)
	}
	if provider == model.LoginMicrosoft {
		if !onMicrosoftURL(page.URL()) {
			if err := clickOneOf(page, microsoftAuthSelectors(), 20000, log, "选择 Microsoft 登录"); err != nil {
				if !onMicrosoftURL(page.URL()) && !onCline(page.URL()) {
					return err
				}
				log("已在授权相关页面，继续")
			}
			sleep(800)
		}
		if err := waitAnyURL(page, []string{"microsoftonline.com", "login.live.com", "radar-challenge", "app.cline.bot"}, 45000); err != nil {
			log("等待微软授权页超时，当前 URL=%s", page.URL())
		}
		if onMicrosoftURL(page.URL()) || visible(page, `input[name="loginfmt"], input#i0116, input[name="passwd"], input#passwordEntry`) {
			return microsoftLogin(page, acc, log)
		}
		return nil
	}
	if !strings.Contains(page.URL(), "accounts.google.com") {
		if err := clickOneOf(page, googleAuthSelectors(), 20000, log, "选择 Google 登录"); err != nil {
			if !strings.Contains(page.URL(), "accounts.google.com") && !onCline(page.URL()) {
				return err
			}
			log("已在授权相关页面，继续")
		}
		sleep(800)
	}
	if err := waitAnyURL(page, []string{"accounts.google.com", "radar-challenge", "app.cline.bot"}, 45000); err != nil {
		log("等待授权页超时，当前 URL=%s", page.URL())
	}
	if strings.Contains(page.URL(), "accounts.google.com") || visible(page, `input#identifierId, input[name="identifier"], input[name="Passwd"]`) {
		return googleLogin(page, acc, log)
	}
	return nil
}

func waitCline(page playwright.Page, timeout float64, log Logger, provider string) error {
	deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
	last := time.Time{}
	lastAuthkitClick := time.Time{}
	for time.Now().Before(deadline) {
		if onCline(page.URL()) || onRadarFlow(page) {
			return nil
		}
		if classifyGoogle(page.URL()) == "tos" && visible(page, `#gaplustosNext button`) {
			_ = acceptWorkspaceTos(page, log)
		}
		if classifyGoogle(page.URL()) == "unknownerror" {
			_ = recoverGoogleUnknownError(page, log)
		}
		if classifyGoogle(page.URL()) == "consent" && (visible(page, `div[jsname="uRHG6"] button`) || visible(page, `#submit_approve_access`)) {
			_ = clickOneOf(page, consentSelectors(), 8000, log, "同意授权")
		}
		if urlHost(page.URL()) == "authkit.cline.bot" && !onRadarFlow(page) {
			done, err := handleAuthkitWait(page, log, &lastAuthkitClick, provider)
			if err != nil {
				return err
			}
			if done {
				return nil
			}
			continue
		}
		if time.Since(last) > 8*time.Second {
			log("仍在等待进入 Cline，当前 URL=%s", page.URL())
			last = time.Now()
		}
		sleep(500)
	}
	return fmt.Errorf("等待进入 Cline 超时，当前 URL=%s", page.URL())
}

func googleAuthSelectors() []string {
	return []string{
		`a[data-method="google"]`,
		`a[href*="provider=GoogleOAuth"]`,
		`a[href*="GoogleOAuth"]`,
	}
}

func visibleGoogleAuth(page playwright.Page) bool {
	for _, sel := range googleAuthSelectors() {
		if visible(page, sel) {
			return true
		}
	}
	return false
}

func clickFirst(page playwright.Page, selectors []string, timeout float64, log Logger, step string) error {
	return clickOneOf(page, selectors, timeout, log, step)
}

func clickOneOf(page playwright.Page, selectors []string, timeout float64, log Logger, step string) error {
	waitOverlayGone(page)
	deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
	var lastErr error
	for time.Now().Before(deadline) {
		for _, sel := range selectors {
			loc := page.Locator(sel).First()
			ok, err := loc.IsVisible()
			if err != nil || !ok {
				continue
			}
			if err := humanClick(page, loc); err != nil {
				lastErr = err
				continue
			}
			log("%s 成功", step)
			sleep(1200)
			return nil
		}
		sleep(250)
	}
	if lastErr != nil {
		return CompactError(fmt.Errorf("%s 失败: %w", step, lastErr))
	}
	return fmt.Errorf("%s 失败: 未找到可点击按钮", step)
}

func fillField(page playwright.Page, selector, value string) error {
	return fillFieldOpt(page, selector, value, true)
}

func fillAny(page playwright.Page, selector, value string) error {
	return fillFieldOpt(page, selector, value, false)
}

func fillFieldOpt(page playwright.Page, selector, value string, stopIfCline bool) error {
	if err := waitVisibleOpt(page, selector, 20000, stopIfCline); err != nil {
		return err
	}
	loc := page.Locator(selector).First()
	waitOverlayGone(page)
	if err := humanType(page, loc, value); err != nil {
		if stopIfCline && loggedIn(page) {
			return errLoggedIn
		}
		return CompactError(err)
	}
	return nil
}

func waitVisible(page playwright.Page, selector string, timeout float64) error {
	return waitVisibleOpt(page, selector, timeout, true)
}

func waitVisibleOpt(page playwright.Page, selector string, timeout float64, stopIfCline bool) error {
	loc := page.Locator(selector).First()
	deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
	for time.Now().Before(deadline) {
		if stopIfCline && leftGoogle(page) {
			return errLoggedIn
		}
		ok, err := loc.IsVisible()
		if err == nil && ok {
			return nil
		}
		sleep(200)
	}
	if stopIfCline && leftGoogle(page) {
		return errLoggedIn
	}
	return fmt.Errorf("等待元素超时")
}

func waitOverlayGone(page playwright.Page) {
	loc := page.Locator(`div[jsname="OQ2Y6"]`)
	_ = loc.First().WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateHidden,
		Timeout: playwright.Float(8000),
	})
}

func waitPathContains(page playwright.Page, part string, timeout float64) error {
	deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
	for time.Now().Before(deadline) {
		if strings.Contains(googlePath(page.URL()), part) {
			return nil
		}
		sleep(200)
	}
	return fmt.Errorf("等待 path 包含 %s 超时", part)
}

func waitPathLeft(page playwright.Page, part string, timeout float64) error {
	return waitLeaveLogged(page, part, timeout, nil, "")
}

func waitLeaveLogged(page playwright.Page, part string, timeout float64, log Logger, name string) error {
	deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
	last := time.Time{}
	for time.Now().Before(deadline) {
		u := page.URL()
		if leftGoogleURL(u) {
			if log != nil {
				log("已离开谷歌，当前 URL=%s", u)
			}
			return nil
		}
		if !strings.Contains(googlePath(u), part) {
			return nil
		}
		if log != nil && name != "" && time.Since(last) > 8*time.Second {
			log("仍在等待离开%s，当前 URL=%s", name, u)
			last = time.Now()
		}
		sleep(200)
	}
	if leftGoogle(page) {
		return nil
	}
	return fmt.Errorf("等待离开 %s 超时，当前 URL=%s", part, page.URL())
}

func googlePath(raw string) string {
	u, err := neturl.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Path
}

func loggedIn(page playwright.Page) bool {
	return leftGoogle(page) || onRadarFlow(page)
}

func onRadarFlow(page playwright.Page) bool {
	if onRadarURL(page.URL()) {
		return true
	}
	return visible(page, `input[name="local_number"]`) ||
		visible(page, `input[name="country_code"]`) ||
		visible(page, `input[data-test="otp-input"]`) ||
		visible(page, `.ak-Otp`)
}

func handleAuthkitWait(page playwright.Page, log Logger, lastClick *time.Time, provider string) (bool, error) {
	rawURL := page.URL()
	if onRadarFlow(page) || onCline(rawURL) {
		log("已进入手机验证或 Cline，当前 URL=%s", rawURL)
		return true, nil
	}
	if code := authkitCallbackError(rawURL); code != "" {
		log("AuthKit 回调错误 error=%s，当前 URL=%s", code, rawURL)
		if strings.EqualFold(code, "policy_denied") {
			return false, ErrRadarDenied
		}
		return false, fmt.Errorf("AuthKit 回调失败：%s", code)
	}
	sid := authkitSessionID(rawURL)
	authVisible := visibleAuthButton(page, provider)
	log("到达 AuthKit，当前 URL=%s title=%q session=%s 登录按钮=%v 方式=%s", rawURL, pageTitle(page), sid, authVisible, provider)
	waitMS := 8000.0
	if sid != "" {
		waitMS = 10000
		log("OAuth 已回到 AuthKit（有 authorization_session_id），先确认是否跳到接码页")
	}
	if waitAuthkitAdvance(page, waitMS) {
		return onRadarFlow(page) || onCline(page.URL()), nil
	}
	after := page.URL()
	if code := authkitCallbackError(after); code != "" {
		log("AuthKit 等待后出现回调错误 error=%s，当前 URL=%s", code, after)
		if strings.EqualFold(code, "policy_denied") {
			return false, ErrRadarDenied
		}
		return false, fmt.Errorf("AuthKit 回调失败：%s", code)
	}
	log("AuthKit 等待后仍未进入接码，当前 URL=%s title=%q 登录按钮=%v", after, pageTitle(page), visibleAuthButton(page, provider))
	if authkitBannedAfterWait(after) {
		log("仍停在 AuthKit，账号已被封禁，跳过")
		return false, ErrAccountBanned
	}
	if onAuthkitLogin(after) && visibleAuthButton(page, provider) && time.Since(*lastClick) > 12*time.Second {
		label := "再次选择 Google 登录"
		sels := googleAuthSelectors()
		if provider == model.LoginMicrosoft {
			label = "再次选择 Microsoft 登录"
			sels = microsoftAuthSelectors()
		}
		log("AuthKit 仍是登录页，%s", label)
		_ = clickOneOf(page, sels, 8000, log, label)
		*lastClick = time.Now()
	}
	sleep(500)
	return false, nil
}

func authkitBannedAfterWait(u string) bool {
	return authkitSessionID(u) != "" && urlHost(u) == "authkit.cline.bot" && !onRadarURL(u) && !onCline(u)
}

func visibleAuthButton(page playwright.Page, provider string) bool {
	if provider == model.LoginMicrosoft {
		return visibleMicrosoftAuth(page)
	}
	return visibleGoogleAuth(page)
}

func pageTitle(page playwright.Page) string {
	t, err := page.Title()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(t)
}

func authkitSessionID(u string) string {
	return authkitQuery(u, "authorization_session_id")
}

func authkitCallbackError(u string) string {
	return authkitQuery(u, "error")
}

func authkitQuery(u, key string) string {
	parsed, err := neturl.Parse(u)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Query().Get(key))
}

func waitAuthkitAdvance(page playwright.Page, timeout float64) bool {
	deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
	for time.Now().Before(deadline) {
		if onCline(page.URL()) || onRadarFlow(page) {
			return true
		}
		if urlHost(page.URL()) != "authkit.cline.bot" {
			return true
		}
		if authkitCallbackError(page.URL()) != "" {
			return false
		}
		sleep(400)
	}
	return onCline(page.URL()) || onRadarFlow(page) || urlHost(page.URL()) != "authkit.cline.bot"
}

func onRadarURL(u string) bool {
	p := googlePath(u)
	return strings.Contains(p, "radar-challenge") || strings.Contains(p, "/radar")
}

func leftGoogle(page playwright.Page) bool {
	return leftGoogleURL(page.URL())
}

func wrapIfAuthkit(err error, pageURL string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrAccountBanned) || errors.Is(err, ErrRadarDenied) {
		return err
	}
	if strings.EqualFold(authkitCallbackError(pageURL), "policy_denied") {
		return ErrRadarDenied
	}
	if errors.Is(err, ErrAuthkitStuck) || isAuthkitProblemURL(pageURL) || IsAuthkitFailure(err) {
		if errors.Is(err, ErrAuthkitStuck) {
			return CompactError(err)
		}
		return CompactError(fmt.Errorf("%w: %s", ErrAuthkitStuck, err.Error()))
	}
	return CompactError(err)
}

func isAuthkitProblemURL(u string) bool {
	if u == "" {
		return false
	}
	if strings.HasPrefix(u, "chrome-error") || strings.HasPrefix(u, "chrome://") {
		return true
	}
	return urlHost(u) == "authkit.cline.bot" && !onRadarURL(u)
}

func urlHost(raw string) string {
	u, err := neturl.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func leftGoogleURL(u string) bool {
	if u == "" || u == "about:blank" || strings.HasPrefix(u, "chrome-error") || strings.HasPrefix(u, "chrome://") {
		return false
	}
	host := urlHost(u)
	if host == "" || strings.HasSuffix(host, "google.com") || strings.HasSuffix(host, "google.com.hk") || onMicrosoftURL(u) {
		return false
	}
	if onAuthkitLogin(u) {
		return false
	}
	return onCline(u) || host == "api.cline.bot"
}

func onAuthkitLogin(u string) bool {
	if urlHost(u) != "authkit.cline.bot" {
		return false
	}
	if onRadarURL(u) {
		return false
	}
	if strings.Contains(googlePath(u), "/api/") {
		return false
	}
	return true
}

func onCline(u string) bool {
	if urlHost(u) == "app.cline.bot" {
		return true
	}
	return onRadarURL(u)
}

func onClineApp(u string) bool {
	return urlHost(u) == "app.cline.bot"
}

func cookieExpired(u string) bool {
	host := urlHost(u)
	if strings.HasSuffix(host, "google.com") || strings.HasSuffix(host, "google.com.hk") || onMicrosoftURL(u) {
		return true
	}
	return host == "authkit.cline.bot" && !onRadarURL(u)
}

func stepErr(err error) error {
	if err == nil || errors.Is(err, errLoggedIn) {
		return nil
	}
	return CompactError(err)
}

var errLoggedIn = errors.New("已离开谷歌登录")

func visible(page playwright.Page, selector string) bool {
	ok, err := page.Locator(selector).First().IsVisible()
	return err == nil && ok
}

func captchaVisible(page playwright.Page) bool {
	return visible(page, `iframe[src*="recaptcha"], iframe[src*="challenge"], #captcha, div[id*="captcha"]`)
}

func waitAnyURL(page playwright.Page, parts []string, timeout float64) error {
	deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
	for time.Now().Before(deadline) {
		u := page.URL()
		for _, p := range parts {
			if strings.Contains(u, p) {
				return nil
			}
		}
		sleep(300)
	}
	return fmt.Errorf("等待 URL 超时")
}

func waitForHTML(page playwright.Page, needles []string, timeout float64) error {
	deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
	for time.Now().Before(deadline) {
		html, err := page.Content()
		if err == nil {
			for _, n := range needles {
				if strings.Contains(html, n) {
					return nil
				}
			}
		}
		sleep(400)
	}
	return fmt.Errorf("页面内容未出现期望片段")
}

func screenshot(page playwright.Page, path string) error {
	if page == nil {
		return nil
	}
	_, err := page.Screenshot(playwright.PageScreenshotOptions{
		Path:     playwright.String(path),
		FullPage: playwright.Bool(true),
	})
	return err
}

func serializeCookies(cookies []playwright.Cookie) (jsonStr, header string) {
	b, _ := json.Marshal(cookies)
	parts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		if c.Name == "" {
			continue
		}
		parts = append(parts, c.Name+"="+c.Value)
	}
	return string(b), strings.Join(parts, "; ")
}

func toOptionalCookies(raw string) ([]playwright.OptionalCookie, error) {
	var cookies []playwright.Cookie
	if err := json.Unmarshal([]byte(raw), &cookies); err != nil {
		return nil, err
	}
	out := make([]playwright.OptionalCookie, 0, len(cookies))
	for _, c := range cookies {
		if c.Name == "" {
			continue
		}
		oc := playwright.OptionalCookie{
			Name:     c.Name,
			Value:    c.Value,
			HttpOnly: playwright.Bool(c.HttpOnly),
			Secure:   playwright.Bool(c.Secure),
			SameSite: c.SameSite,
		}
		if c.Expires > 0 {
			oc.Expires = playwright.Float(c.Expires)
		}
		if c.Domain != "" {
			oc.Domain = playwright.String(c.Domain)
		}
		if c.Path != "" {
			oc.Path = playwright.String(c.Path)
		} else {
			oc.Path = playwright.String("/")
		}
		if c.PartitionKey != nil && *c.PartitionKey != "" {
			oc.PartitionKey = c.PartitionKey
		}
		out = append(out, oc)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("Cookie 为空")
	}
	return out, nil
}

var wsRe = regexp.MustCompile(`opencode\.ai/workspace/(wrk_[A-Za-z0-9]+)`)

func workspaceIDFromURL(u string) string {
	m := wsRe.FindStringSubmatch(u)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

func sleep(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

func maskKey(k string) string {
	if len(k) <= 10 {
		return k
	}
	return k[:6] + "..." + k[len(k)-4:]
}
