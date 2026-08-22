package login

import (
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

func handleRadar(page playwright.Page, cfg config.Config, log Logger) error {
	if !strings.Contains(page.URL(), "radar-challenge") {
		return nil
	}
	if strings.Contains(page.URL(), "radar-challenge/verify") {
		return fmt.Errorf("已经在验证码页，但没有进行中的接码")
	}
	if cfg.HeroSMSAPIKey == "" {
		return fmt.Errorf("遇到手机验证，请先在设置里填写 Hero SMS API Key")
	}
	if cfg.HeroSMSCountry <= 0 {
		return fmt.Errorf("遇到手机验证，请先在设置里选择接码区域和报价")
	}

	sms := herosms.New(cfg.HeroSMSAPIKey, cfg.HeroSMSService)
	log("Hero SMS 取号 country=%d price=%s", cfg.HeroSMSCountry, formatPrice(cfg.HeroSMSMaxPrice))
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

	if num.PhoneCode <= 0 || num.LocalNumber == "" {
		return fmt.Errorf("取到的号码无法拆分区号: %s", num.Phone)
	}
	cc := fmt.Sprintf("+%d", num.PhoneCode)
	if err := fillAuthkitField(page, `input[name="country_code"]`, cc); err != nil {
		return fmt.Errorf("填写区号失败: %w", err)
	}
	if err := fillAuthkitField(page, `input[name="local_number"]`, num.LocalNumber); err != nil {
		return fmt.Errorf("填写手机号失败: %w", err)
	}
	if err := clickOneOf(page, []string{
		`button[data-hak-cta][type="submit"]`,
		`button.ak-PrimaryButton[type="submit"]`,
		`button[type="submit"]`,
	}, 15000, log, "发送验证码"); err != nil {
		return err
	}
	if err := waitAnyURL(page, []string{"radar-challenge/verify"}, 30000); err != nil {
		return fmt.Errorf("没有进入验证码页: %w", err)
	}

	log("等待短信验证码")
	code, err := sms.WaitCode(num.ID, 2*time.Minute)
	if err != nil {
		return err
	}
	log("收到验证码")
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
		_ = loc.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(3000)})
		if err := loc.Fill(string(code[i]), playwright.LocatorFillOptions{Timeout: playwright.Float(3000)}); err != nil {
			_ = loc.Press(string(code[i]), playwright.LocatorPressOptions{Timeout: playwright.Float(2000)})
		}
		sleep(80)
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
		sleep(1000)
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
	_ = loc.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(5000)})
	_ = loc.Clear()
	setValue := `(el, v) => {
		const desc = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value");
		if (desc && desc.set) desc.set.call(el, v);
		else el.value = v;
		el.dispatchEvent(new Event("input", { bubbles: true }));
		el.dispatchEvent(new Event("change", { bubbles: true }));
	}`
	if _, err := loc.Evaluate(setValue, value); err != nil {
		if ferr := loc.Fill(value, playwright.LocatorFillOptions{Timeout: playwright.Float(8000)}); ferr != nil {
			return CompactError(ferr)
		}
	}
	if authkitInputMatch(loc, value) {
		return nil
	}
	_ = loc.Clear()
	for i := 0; i < 8; i++ {
		_ = loc.Press("Backspace")
	}
	current, _ := loc.InputValue()
	typed := value
	if strings.HasPrefix(strings.TrimSpace(current), "+") && strings.HasPrefix(value, "+") {
		typed = strings.TrimPrefix(value, "+")
	}
	if err := loc.Type(typed, playwright.LocatorTypeOptions{Timeout: playwright.Float(8000), Delay: playwright.Float(30)}); err != nil {
		_ = loc.Fill(value, playwright.LocatorFillOptions{Timeout: playwright.Float(5000)})
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
