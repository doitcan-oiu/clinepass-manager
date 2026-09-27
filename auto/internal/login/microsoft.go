package login

import (
	"fmt"
	"strings"
	"time"

	"github.com/mxschmitt/playwright-go"

	"opencode-go-manager/internal/model"
)

const microsoftCardSettle = time.Second
const microsoftNoProgressTimeout = 35 * time.Second
const microsoftLoginTimeout = 120 * time.Second

func microsoftAuthSelectors() []string {
	return []string{`a[data-method="microsoft"]`, `a[href*="provider=MicrosoftOAuth"]`, `a[href*="MicrosoftOAuth"]`}
}

func visibleMicrosoftAuth(page playwright.Page) bool {
	for _, sel := range microsoftAuthSelectors() {
		if visible(page, sel) {
			return true
		}
	}
	return false
}

func onMicrosoftURL(u string) bool {
	host := urlHost(u)
	if host == "" {
		return false
	}
	return strings.Contains(host, "microsoftonline.com") || strings.Contains(host, "login.live.com") || strings.Contains(host, "account.live.com") || host == "login.microsoft.com"
}

func microsoftInvite(invite string) string { return strings.TrimSpace(invite) }

func microsoftEmailSelectors() []string { return []string{`input[name="loginfmt"]`, `input#i0116`} }
func microsoftPasswordSelectors() []string {
	return []string{`input#passwordEntry`, `input[name="passwd"]`, `input#i0118`}
}
func microsoftNextSelectors() []string {
	return []string{`button[data-testid="primaryButton"]`, `input#idSIButton9`, `input[type="submit"]`}
}

func microsoftStep(emailVisible, passVisible, emailDone bool) string {
	if emailVisible && !emailDone {
		return "email"
	}
	if passVisible && (emailDone || !emailVisible) {
		return "password"
	}
	return "other"
}

func microsoftContainsAny(text string, phrases ...string) bool {
	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

func microsoftProblem(text string, challengeVisible bool) string {
	text = strings.Join(strings.Fields(strings.ToLower(strings.ReplaceAll(text, "’", "'"))), " ")
	switch {
	case microsoftContainsAny(text, "account has been locked", "account is locked", "account has been suspended", "too many times", "账号已锁定", "帐户已锁定", "账户已锁定", "尝试次数过多", "尝试登录次数过多", "暂时阻止"):
		return "微软账号已锁定或尝试过于频繁，请先人工检查账号"
	case microsoftContainsAny(text, "account doesn't exist", "account does not exist", "couldn't find an account", "could not find an account", "该 microsoft 帐户不存在", "该 microsoft 账户不存在", "找不到该帐户", "账号不存在", "帐户不存在", "账户不存在"):
		return "微软账号不存在，请检查邮箱地址"
	case microsoftContainsAny(text, "account or password is incorrect", "password is incorrect", "password isn't correct", "incorrect password", "密码不正确", "密码错误", "密码有误"):
		return "微软账号或密码错误，请检查凭据后重试"
	case microsoftContainsAny(text, "verify your identity", "help us protect your account", "approve sign in request", "approve the sign in", "enter the code we sent", "enter your security code", "help us beat the robots", "prove you're not a robot", "update your password", "change your password", "more information required", "验证你的身份", "验证您的身份", "验证你的标识", "帮助我们保护", "批准登录请求", "输入我们发送", "输入安全代码", "添加安全信息", "提供更多信息", "证明你不是机器人", "证明您不是机器人", "更新你的密码", "更改密码"):
		return "微软要求验证码、身份验证或安全信息，请先人工完成验证后重试"
	case challengeVisible:
		return "微软要求验证码或身份验证，请先人工完成验证后重试"
	}
	return ""
}

func microsoftConfirmation(text string) string {
	text = strings.Join(strings.Fields(strings.ToLower(text)), " ")
	if microsoftContainsAny(text, "stay signed in?", "保持登录状态", "是否保持登录") {
		return "stay_signed_in"
	}
	if microsoftContainsAny(text, "let this app access your info", "permissions requested", "允许此应用访问", "让此应用访问", "请求的权限") {
		return "consent"
	}
	return ""
}

type microsoftProgress struct {
	started, lastProgress, heldSince time.Time
	held, waiting                    string
	seen, submitted                  map[string]bool
}

func newMicrosoftProgress(now time.Time) *microsoftProgress {
	return &microsoftProgress{started: now, lastProgress: now, heldSince: now, waiting: "登录页面加载", seen: map[string]bool{}, submitted: map[string]bool{}}
}

func (p *microsoftProgress) observe(state string, now time.Time) {
	if state != p.held {
		p.held, p.heldSince = state, now
	}
	// Changing query tokens, DOM flicker, and already visited cards are not progress.
	if state != "other" && !p.seen[state] {
		p.seen[state], p.lastProgress = true, now
	}
}

func (p *microsoftProgress) claim(state string) bool {
	if p.submitted[state] {
		return false
	}
	p.submitted[state] = true
	return true
}

func (p *microsoftProgress) timeout(now time.Time) error {
	if now.Sub(p.started) >= microsoftLoginTimeout {
		return fmt.Errorf("微软登录总耗时超过 120 秒，已停止本次登录，请检查页面后重试")
	}
	if now.Sub(p.lastProgress) >= microsoftNoProgressTimeout {
		return fmt.Errorf("微软%s超过 35 秒无进展，已停止本次登录，请检查账号或网络后重试", p.waiting)
	}
	return nil
}

func microsoftLogin(page playwright.Page, acc model.Account, log Logger) error {
	progress := newMicrosoftProgress(time.Now())
	emailAccepted := false
	log("等待微软登录页面")
	for {
		if loggedIn(page) {
			log("微软登录完成")
			return nil
		}
		now := time.Now()
		if err := progress.timeout(now); err != nil {
			return err
		}
		if urlHost(page.URL()) == "authkit.cline.bot" {
			if code := authkitCallbackError(page.URL()); code != "" {
				if strings.EqualFold(code, "policy_denied") {
					return ErrRadarDenied
				}
				return fmt.Errorf("微软授权回调失败，请检查登录页面后重试")
			}
			if !progress.seen["callback"] {
				log("微软授权已返回，等待登录完成")
				progress.waiting = "授权回调"
			}
			progress.observe("callback", now)
			sleep(200)
			continue
		}
		text, _ := page.Locator("body").InnerText(playwright.LocatorInnerTextOptions{Timeout: playwright.Float(1000)})
		challenge := false
		for _, sel := range []string{`input[name="otc"]`, `input[name="otcInput"]`, `input#iOttText`, `input[name="ProofConfirmation"]`, `#idDiv_SAOTCC_Description`, `#idRichContext_DisplaySign`, `iframe[src*="captcha"]`, `#wlspispHIPSolutionContainer`} {
			if visible(page, sel) {
				challenge = true
				break
			}
		}
		if message := microsoftProblem(text, challenge); message != "" {
			return fmt.Errorf("%s", message)
		}
		emailSel, passSel := firstReadyField(page, microsoftEmailSelectors()), firstReadyField(page, microsoftPasswordSelectors())
		if progress.submitted["email"] && emailSel == "" {
			emailAccepted = true
		}
		state := microsoftStep(emailSel != "", passSel != "", emailAccepted)
		if state == "other" && onMicrosoftURL(page.URL()) {
			if confirmation := microsoftConfirmation(text); confirmation != "" {
				state = confirmation
			}
		}
		progress.observe(state, now)
		if (state == "email" || state == "password") && microsoftCardSettled(now.Sub(progress.heldSince)) && progress.claim(state) {
			selector, value, label := emailSel, acc.Email, "账号"
			if state == "password" {
				selector, value, label = passSel, acc.Password, "密码"
			}
			if err := fillMicrosoftField(page, selector, value, log, label); err != nil {
				return stepErr(err)
			}
			if err := clickMicrosoftOnce(page, microsoftNextSelectors(), label); err != nil {
				return err
			}
			if state == "email" {
				log("已提交微软账号，等待密码页面")
				progress.waiting = "账号提交后等待"
			} else {
				log("已提交微软密码，等待验证结果")
				progress.waiting = "密码验证"
			}
			progress.lastProgress = time.Now()
		} else if (state == "stay_signed_in" || state == "consent") && progress.claim(state) {
			label := "保持登录"
			if state == "consent" {
				label = "授权确认"
			}
			if err := clickMicrosoftOnce(page, microsoftNextSelectors(), label); err != nil {
				return err
			}
			log("已确认微软%s，等待跳转", label)
			progress.waiting, progress.lastProgress = label, time.Now()
		}
		sleep(200)
	}
}

// A dispatched click is never retried: a navigation timeout may mean it succeeded.
func clickMicrosoftOnce(page playwright.Page, selectors []string, label string) error {
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if loggedIn(page) {
			return nil
		}
		for _, selector := range selectors {
			loc := page.Locator(selector).First()
			shown, err := loc.IsVisible()
			if err != nil || !shown {
				continue
			}
			enabled, err := loc.IsEnabled()
			if err != nil || !enabled {
				continue
			}
			if err := loc.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(8000), NoWaitAfter: playwright.Bool(true)}); err != nil {
				if loggedIn(page) {
					return nil
				}
				return fmt.Errorf("微软%s提交未完成，请检查页面后重试", label)
			}
			return nil
		}
		sleep(200)
	}
	return fmt.Errorf("微软%s按钮未就绪，请检查页面后重试", label)
}

func firstReadyField(page playwright.Page, selectors []string) string {
	for _, sel := range selectors {
		if fieldReady(page, sel) {
			return sel
		}
	}
	return ""
}

func fieldReady(page playwright.Page, selector string) bool {
	loc := page.Locator(selector).First()
	ok, err := loc.IsVisible()
	if err != nil || !ok {
		return false
	}
	hidden, err := loc.GetAttribute("aria-hidden")
	if err == nil && strings.EqualFold(strings.TrimSpace(hidden), "true") {
		return false
	}
	enabled, err := loc.IsEnabled()
	if err != nil || !enabled {
		return false
	}
	editable, err := loc.IsEditable()
	return err == nil && editable
}

func microsoftCardSettled(held time.Duration) bool { return held >= microsoftCardSettle }

func fillMicrosoftField(page playwright.Page, selector, value string, log Logger, label string) error {
	if err := fillField(page, selector, value); err != nil {
		return err
	}
	loc := page.Locator(selector).First()
	got, err := loc.InputValue()
	if err != nil || strings.TrimSpace(got) != strings.TrimSpace(value) {
		if err := humanType(page, loc, value); err != nil {
			return CompactError(err)
		}
		got, err = loc.InputValue()
		if err != nil || strings.TrimSpace(got) != strings.TrimSpace(value) {
			return fmt.Errorf("微软%s未能填入输入框，请检查页面后重试", label)
		}
	}
	return nil
}
