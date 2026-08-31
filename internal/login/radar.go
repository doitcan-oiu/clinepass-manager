package login

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mxschmitt/playwright-go"

	"opencode-go-manager/internal/cline"
	"opencode-go-manager/internal/config"
	"opencode-go-manager/internal/herosms"
	"opencode-go-manager/internal/model"
	"opencode-go-manager/internal/usage"
)

const radarPhoneAttempts = 3

func handleRadar(page playwright.Page, cfg config.Config, log Logger) error {
	if !strings.Contains(googlePath(page.URL()), "radar-challenge") || onIdentityProvider(page.URL()) {
		return nil
	}
	if err := waitRadarForm(page); err != nil {
		return err
	}
	if cfg.HeroSMSAPIKey == "" {
		return fmt.Errorf("遇到手机验证，请先在设置里填写 Hero SMS API Key")
	}
	if cfg.HeroSMSCountry <= 0 {
		return fmt.Errorf("遇到手机验证，请先在设置里选择接码区域和报价")
	}

	sendURL := page.URL()
	if strings.Contains(sendURL, "radar-challenge/verify") {
		sendURL = radarSendURL(sendURL)
	}
	if err := gotoRadarSend(page, sendURL, log); err != nil {
		return err
	}
	sendURL = page.URL()

	sms := herosms.New(cfg.HeroSMSAPIKey, cfg.HeroSMSService)
	for i := 1; i <= radarPhoneAttempts; i++ {
		err := requestRadarCode(page, cfg, sms, log, i)
		if err == nil {
			return nil
		}
		if !shouldRetryRadarPhone(err) {
			return err
		}
		log("第 %d/%d 次接码失败: %v", i, radarPhoneAttempts, err)
		if i < radarPhoneAttempts {
			log("返回手机号输入页，换一个 Hero SMS 号码重试")
			sleep(2000)
			if err := gotoRadarSend(page, sendURL, log); err != nil {
				return err
			}
		}
	}
	return ErrSMSNeedRelogin
}

func requestRadarCode(page playwright.Page, cfg config.Config, sms *herosms.Client, log Logger, attempt int) error {
	if err := waitRadarForm(page); err != nil {
		return err
	}
	log("Hero SMS 取号 country=%d price=%s（第 %d/%d 次）", cfg.HeroSMSCountry, formatPrice(cfg.HeroSMSMaxPrice), attempt, radarPhoneAttempts)
	num, err := sms.GetNumber(cfg.HeroSMSCountry, cfg.HeroSMSMaxPrice)
	if err != nil {
		return err
	}
	finished := false
	defer func() {
		if !finished {
			sms.Cancel(num.ID)
		}
	}()
	log("已取号 +%d %s id=%s", num.PhoneCode, num.LocalNumber, num.ID)
	sleep(200)

	if num.PhoneCode <= 0 || num.LocalNumber == "" {
		return fmt.Errorf("取到的号码无法拆分区号: %s", num.Phone)
	}
	cc := fmt.Sprintf("+%d", num.PhoneCode)
	if err := fillAuthkitField(page, `input[name="country_code"]`, cc); err != nil {
		return fmt.Errorf("填写区号失败: %w", err)
	}
	sleep(150)
	if err := fillAuthkitField(page, `input[name="local_number"]`, num.LocalNumber); err != nil {
		return fmt.Errorf("填写手机号失败: %w", err)
	}
	sleep(200)
	if err := clickOneOf(page, []string{
		`button[data-hak-cta][type="submit"]`,
		`button.ak-PrimaryButton[type="submit"]`,
		`button[type="submit"]`,
	}, 15000, log, "发送验证码"); err != nil {
		return err
	}
	if err := waitAnyURL(page, []string{"radar-challenge/verify"}, 30000); err != nil {
		if onRadarSend(page.URL()) || visible(page, `input[name="local_number"]`) {
			return fmt.Errorf("发送验证码失败，号码可能已被使用: %w", err)
		}
		return fmt.Errorf("没有进入验证码页: %w", err)
	}
	sleep(200)

	log("等待短信验证码")
	code, err := sms.WaitCode(num.ID, 2*time.Minute)
	if err != nil {
		return err
	}
	log("收到验证码")
	sleep(120)
	if err := fillOTP(page, code); err != nil {
		return err
	}
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if !strings.Contains(page.URL(), "radar-challenge") {
			sms.Finish(num.ID)
			finished = true
			return nil
		}
		sleep(400)
	}
	return fmt.Errorf("提交验证码后仍停在验证页，当前 URL=%s", page.URL())
}

func gotoRadarSend(page playwright.Page, sendURL string, log Logger) error {
	if onRadarSend(page.URL()) && visible(page, `input[name="local_number"]`) {
		return nil
	}
	target := strings.TrimSpace(sendURL)
	if target == "" || strings.Contains(target, "radar-challenge/verify") {
		target = radarSendURL(page.URL())
	}
	if log != nil && target != "" {
		log("打开手机号输入页")
	}
	if target != "" {
		if _, err := page.Goto(target, playwright.PageGotoOptions{
			WaitUntil: playwright.WaitUntilStateDomcontentloaded,
			Timeout:   playwright.Float(30000),
		}); err != nil {
			if log != nil {
				log("打开手机号页失败: %v，尝试后退", err)
			}
			_, _ = page.GoBack()
		}
	} else {
		_, _ = page.GoBack()
	}
	sleep(300)
	if onRadarSend(page.URL()) || visible(page, `input[name="local_number"]`) {
		return nil
	}
	return fmt.Errorf("无法回到手机号输入页，当前 URL=%s", page.URL())
}

func radarSendURL(raw string) string {
	return strings.Replace(raw, "radar-challenge/verify", "radar-challenge/send", 1)
}

func waitRadarForm(page playwright.Page) error {
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if visible(page, `input[name="country_code"]`) && visible(page, `input[name="local_number"]`) {
			return nil
		}
		sleep(200)
	}
	return fmt.Errorf("接码页没有可填的手机号输入框，当前 URL=%s", page.URL())
}

func onRadarSend(u string) bool {
	return strings.Contains(u, "radar-challenge/send")
}

func isNoSMS(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, herosms.ErrWaitCodeTimeout) ||
		errors.Is(err, herosms.ErrCancelled) ||
		errors.Is(err, herosms.ErrUnavailable)
}

func shouldRetryRadarPhone(err error) bool {
	if isNoSMS(err) {
		return true
	}
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "号码可能已被使用") {
		return true
	}
	return strings.Contains(msg, "等待 URL 超时") && strings.Contains(msg, "radar-challenge/send")
}

func fillOTP(page playwright.Page, code string) error {
	code = digitsOnly(code)
	if len(code) < 4 {
		return fmt.Errorf("验证码无效")
	}
	if len(code) > 6 {
		code = code[:6]
	}
	inputs := page.Locator(`input[data-test="otp-input"], input[data-index], .ak-Otp input`)
	n, err := inputs.Count()
	if err != nil || n == 0 {
		if err := fillAny(page, `input[name="code"], input[autocomplete="one-time-code"]`, code); err != nil {
			return fmt.Errorf("填写验证码失败")
		}
		return nil
	}
	for i := 0; i < n && i < len(code); i++ {
		loc := inputs.Nth(i)
		if err := humanType(page, loc, string(code[i])); err != nil {
			_ = loc.Press(string(code[i]), playwright.LocatorPressOptions{Timeout: playwright.Float(2000)})
		}
		sleep(20)
	}
	return nil
}

func handleTerms(page playwright.Page, log Logger) error {
	if !visible(page, `label#terms, #terms`) {
		return nil
	}
	log("勾选条款")
	if err := clickOneOf(page, []string{`label#terms`, `#terms`}, 8000, log, "勾选条款"); err != nil {
		return err
	}
	sleep(400)
	return clickOneOf(page, []string{
		`button[type="button"].w-full`,
		`button.bg-black[type="button"]`,
		`button[type="button"]`,
	}, 15000, log, "确认注册")
}

func captureClinePayment(page playwright.Page, log Logger) (string, error) {
	u := page.URL()
	if !strings.Contains(u, "/onboarding/") && !strings.Contains(u, "/checkout") {
		if _, err := page.Goto(cline.AppBase+"/onboarding/individual-plan", playwright.PageGotoOptions{
			WaitUntil: playwright.WaitUntilStateDomcontentloaded,
			Timeout:   playwright.Float(60000),
		}); err != nil {
			return "", fmt.Errorf("打开套餐页失败: %w", err)
		}
		sleep(250)
	}
	if cookieExpired(page.URL()) {
		return "", fmt.Errorf("Cookie 已失效，需要重新登录")
	}
	if !strings.Contains(page.URL(), "/checkout") {
		_ = clickOneOf(page, []string{
			`button:has(svg.lucide-chevron-right)`,
			`button.h-12`,
			`button[type="button"]:has(svg)`,
		}, 20000, log, "进入结账")
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if src := iframePaymentSrc(page); src != "" {
			return src, nil
		}
		if isStripe(page.URL()) {
			return page.URL(), nil
		}
		sleep(500)
	}
	return "", fmt.Errorf("等待支付 iframe 超时")
}

func iframePaymentSrc(page playwright.Page) string {
	loc := page.Locator(`iframe[src*="stripe"], iframe[src*="checkout"], iframe`).First()
	ok, err := loc.IsVisible()
	if err != nil || !ok {
		return ""
	}
	src, err := loc.GetAttribute("src")
	if err != nil {
		return ""
	}
	src = strings.TrimSpace(src)
	if src == "" {
		return ""
	}
	if strings.Contains(src, "stripe") || strings.Contains(src, "checkout") || strings.HasPrefix(src, "https://") {
		return src
	}
	return ""
}

func createClineKey(cfg config.Config, cookie, existingKey, existingUser string, log Logger) (key, userID string, err error) {
	a := model.Account{CookieHeader: cookie, APIKey: existingKey, UserID: existingUser, WorkspaceID: existingUser}
	if err := usage.Hydrate(&a, cfg.Proxy); err != nil {
		if existingKey != "" {
			return existingKey, existingUser, nil
		}
		return "", "", err
	}
	if a.APIKey != "" && a.APIKey != existingKey {
		log("已创建 API Key %s...", maskKey(a.APIKey))
	}
	return a.APIKey, a.UserID, nil
}

func fillAuthkitField(page playwright.Page, selector, value string) error {
	if err := waitVisibleOpt(page, selector, 20000, false); err != nil {
		return err
	}
	loc := page.Locator(selector).First()
	waitOverlayGone(page)
	if err := humanType(page, loc, value); err != nil {
		return CompactError(err)
	}
	if authkitInputMatch(loc, value) {
		return nil
	}
	current, _ := loc.InputValue()
	typed := value
	if strings.HasPrefix(strings.TrimSpace(current), "+") && strings.HasPrefix(value, "+") {
		typed = strings.TrimPrefix(value, "+")
	}
	if err := humanType(page, loc, typed); err != nil {
		return CompactError(err)
	}
	if !authkitInputMatch(loc, value) {
		got, _ := loc.InputValue()
		return fmt.Errorf("页面是 %q，期望 %q", got, value)
	}
	return nil
}

func authkitInputMatch(loc playwright.Locator, want string) bool {
	got, err := loc.InputValue()
	if err != nil {
		return false
	}
	return digitsOnly(got) == digitsOnly(want)
}

func formatPrice(n float64) string {
	if n <= 0 {
		return "auto"
	}
	return fmt.Sprintf("%g", n)
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
