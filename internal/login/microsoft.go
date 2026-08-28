package login

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mxschmitt/playwright-go"

	"opencode-go-manager/internal/model"
)

const microsoftCardSettle = time.Second

func microsoftAuthSelectors() []string {
	return []string{
		`a[data-method="microsoft"]`,
		`a[href*="provider=MicrosoftOAuth"]`,
		`a[href*="MicrosoftOAuth"]`,
	}
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
	return strings.Contains(host, "microsoftonline.com") ||
		strings.Contains(host, "login.live.com") ||
		strings.Contains(host, "account.live.com") ||
		host == "login.microsoft.com"
}

func microsoftInvite(invite string) string {
	return strings.TrimSpace(invite)
}

func microsoftEmailSelectors() []string {
	return []string{
		`input[name="loginfmt"]`,
		`input#i0116`,
	}
}

func microsoftPasswordSelectors() []string {
	return []string{
		`input#passwordEntry`,
		`input[name="passwd"]`,
		`input#i0118`,
	}
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

func microsoftLogin(page playwright.Page, acc model.Account, log Logger) error {
	deadline := time.Now().Add(3 * time.Minute)
	emailDone := false
	passDone := false
	lastAuthkitClick := time.Time{}
	lastUnknown := time.Time{}

	log("等待微软登录卡片加载")
	if _, err := waitMicrosoftCard(page, microsoftEmailSelectors(), 30000, log, "账号"); err != nil && !errors.Is(err, errLoggedIn) {
		log("%v", err)
	}

	for time.Now().Before(deadline) {
		rawURL := page.URL()
		if loggedIn(page) {
			log("已离开微软登录，当前 URL=%s", rawURL)
			return nil
		}
		if urlHost(rawURL) == "authkit.cline.bot" {
			done, err := handleAuthkitWait(page, log, &lastAuthkitClick, model.LoginMicrosoft)
			if err != nil {
				return err
			}
			if done {
				return nil
			}
			continue
		}

		emailSel := firstReadyField(page, microsoftEmailSelectors())
		passSel := firstReadyField(page, microsoftPasswordSelectors())
		step := microsoftStep(emailSel != "", passSel != "", emailDone)

		if step == "email" {
			sel, err := waitMicrosoftCard(page, microsoftEmailSelectors(), 20000, log, "账号")
			if errors.Is(err, errLoggedIn) {
				return nil
			}
			if sel == "" {
				sleep(400)
				continue
			}
			log("填写 Microsoft 账号")
			if err := fillMicrosoftField(page, sel, acc.Email, log, "账号"); err != nil {
				return stepErr(err)
			}
			if err := clickOneOf(page, []string{`input#idSIButton9`, `input[type="submit"]`}, 15000, log, "账号下一步"); err != nil {
				if loggedIn(page) {
					return nil
				}
				return stepErr(err)
			}
			if err := waitMicrosoftEmailAccepted(page, sel, 25000, log); err != nil {
				log("邮箱卡片还没切走，再等异步加载：%v", err)
				emailDone = false
				continue
			}
			emailDone = true
			continue
		}
		if step == "password" {
			sel, err := waitMicrosoftCard(page, microsoftPasswordSelectors(), 20000, log, "密码")
			if errors.Is(err, errLoggedIn) {
				return nil
			}
			if sel == "" || firstReadyField(page, microsoftEmailSelectors()) != "" && !emailDone {
				sleep(400)
				continue
			}
			log("填写 Microsoft 密码")
			if err := fillMicrosoftField(page, sel, acc.Password, log, "密码"); err != nil {
				return stepErr(err)
			}
			if err := clickOneOf(page, microsoftNextSelectors(), 15000, log, "密码下一步"); err != nil {
				if loggedIn(page) {
					return nil
				}
				return stepErr(err)
			}
			passDone = true
			_ = waitLeaveMicrosoftField(page, sel, 20000)
			continue
		}
		if passDone && emailSel == "" && passSel == "" && onMicrosoftURL(rawURL) && visible(page, `input#idSIButton9, button[data-testid="primaryButton"]`) {
			log("点击微软页面下一步")
			_ = clickOneOf(page, microsoftNextSelectors(), 8000, log, "微软下一步")
			sleep(200)
			continue
		}
		if onMicrosoftURL(rawURL) && time.Since(lastUnknown) > 8*time.Second {
			log("微软页面未识别，当前 URL=%s", rawURL)
			lastUnknown = time.Now()
		}
		sleep(200)
	}
	if loggedIn(page) {
		return nil
	}
	return fmt.Errorf("Microsoft 登录未完成，当前 URL=%s", page.URL())
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
	if err != nil || !editable {
		return false
	}
	return true
}

func microsoftNextSelectors() []string {
	return []string{
		`button[data-testid="primaryButton"]`,
		`input#idSIButton9`,
		`input[type="submit"]#idSIButton9`,
		`input[type="submit"]`,
	}
}

func microsoftCardSettled(held time.Duration) bool {
	return held >= microsoftCardSettle
}

func waitMicrosoftCard(page playwright.Page, selectors []string, timeout float64, log Logger, name string) (string, error) {
	deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
	hold := ""
	var since time.Time
	last := time.Time{}
	for time.Now().Before(deadline) {
		if loggedIn(page) {
			return "", errLoggedIn
		}
		sel := firstReadyField(page, selectors)
		if sel != "" {
			if sel != hold {
				hold = sel
				since = time.Now()
			} else if microsoftCardSettled(time.Since(since)) {
				return sel, nil
			}
		} else {
			hold = ""
			since = time.Time{}
		}
		if log != nil && time.Since(last) > 6*time.Second {
			log("微软%s卡片尚未就绪，当前 URL=%s", name, page.URL())
			last = time.Now()
		}
		sleep(200)
	}
	return "", fmt.Errorf("等待微软%s输入卡片超时，当前 URL=%s", name, page.URL())
}

func fillMicrosoftField(page playwright.Page, selector, value string, log Logger, label string) error {
	if err := fillField(page, selector, value); err != nil {
		return err
	}
	loc := page.Locator(selector).First()
	got, err := loc.InputValue()
	if err != nil || strings.TrimSpace(got) != strings.TrimSpace(value) {
		log("%s未落到输入框，再按真人节奏填一次", label)
		if err := humanType(page, loc, value); err != nil {
			return CompactError(err)
		}
	}
	sleep(80)
	return nil
}

func waitLeaveMicrosoftField(page playwright.Page, selector string, timeout float64) error {
	deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
	for time.Now().Before(deadline) {
		if loggedIn(page) || !fieldReady(page, selector) {
			return nil
		}
		sleep(300)
	}
	return fmt.Errorf("仍停在微软输入页")
}

func waitMicrosoftEmailAccepted(page playwright.Page, emailSel string, timeout float64, log Logger) error {
	deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
	for time.Now().Before(deadline) {
		if loggedIn(page) {
			return errLoggedIn
		}
		if !fieldReady(page, emailSel) {
			remain := time.Until(deadline).Seconds() * 1000
			if remain < 8000 {
				remain = 8000
			}
			_, err := waitMicrosoftCard(page, microsoftPasswordSelectors(), remain, log, "密码")
			return err
		}
		sleep(200)
	}
	return fmt.Errorf("提交邮箱后仍停在邮箱页")
}
