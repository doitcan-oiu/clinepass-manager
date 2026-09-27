package login

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"

	"opencode-go-manager/internal/model"
)

func TestMicrosoftProgressStopsStallAndDoesNotCountRepeatedCards(t *testing.T) {
	start := time.Unix(100, 0)
	progress := newMicrosoftProgress(start)
	progress.observe("password", start)
	if !progress.claim("password") || progress.claim("password") {
		t.Fatal("each credential may be submitted only once")
	}
	for second := 1; second <= 35; second++ {
		state := "other"
		if second%2 == 0 {
			state = "password"
		}
		progress.observe(state, start.Add(time.Duration(second)*time.Second))
	}
	if err := progress.timeout(start.Add(35 * time.Second)); err == nil || !strings.Contains(err.Error(), "35 秒无进展") {
		t.Fatalf("repeated/flickering cards must not reset deadline: %v", err)
	}
}

func TestMicrosoftProgressAllowsNewStagesButBoundsTotalTime(t *testing.T) {
	start := time.Unix(100, 0)
	progress := newMicrosoftProgress(start)
	for i, stage := range []string{"email", "password", "stay_signed_in", "consent", "callback"} {
		now := start.Add(time.Duration(i) * 30 * time.Second)
		progress.observe(stage, now)
		if i < 4 && progress.timeout(now) != nil {
			t.Fatalf("genuine stage advance must receive time to finish: %s", stage)
		}
		if !progress.claim(stage) || progress.claim(stage) {
			t.Fatalf("stage may be submitted only once: %s", stage)
		}
	}
	if err := progress.timeout(start.Add(120 * time.Second)); err == nil || !strings.Contains(err.Error(), "总耗时超过 120 秒") || strings.Contains(err.Error(), "35 秒无进展") {
		t.Fatalf("total deadline must use an accurate reason: %v", err)
	}
}

func TestMicrosoftProblemAndConfirmation(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"Your account or password is incorrect", "密码错误"},
		{"That Microsoft account doesn’t exist", "账号不存在"},
		{"Your account has been locked", "账号已锁定"},
		{"Verify your identity", "人工完成验证"},
		{"Enter the code we sent", "人工完成验证"},
		{"添加安全信息", "人工完成验证"},
	} {
		if got := microsoftProblem(tc.text, false); !strings.Contains(got, tc.want) {
			t.Errorf("%q => %q; want %q", tc.text, got, tc.want)
		}
	}
	if got := microsoftProblem("输入密码 忘记密码？ 使用验证码登录", false); got != "" {
		t.Fatalf("normal sign-in page must not be treated as a challenge: %s", got)
	}
	if got := microsoftProblem("", true); !strings.Contains(got, "人工完成验证") {
		t.Fatalf("visible OTP control must stop automation: %s", got)
	}
	for _, tc := range []struct{ text, want string }{{"保持登录状态？", "stay_signed_in"}, {"Let this app access your info?", "consent"}, {"Next / Continue / 下一步", ""}} {
		if got := microsoftConfirmation(tc.text); got != tc.want {
			t.Errorf("confirmation %q => %q; want %q", tc.text, got, tc.want)
		}
	}
}

type microsoftClickPage struct {
	playwright.Page
	loc *microsoftClickLocator
}

func (p *microsoftClickPage) URL() string {
	return "https://login.live.com/oauth20_authorize.srf?state=secret"
}
func (p *microsoftClickPage) Locator(_ string, _ ...playwright.PageLocatorOptions) playwright.Locator {
	return p.loc
}

type embeddedMicrosoftLocator interface{ playwright.Locator }

type microsoftClickLocator struct {
	embeddedMicrosoftLocator
	clicks int
	err    error
}

func (l *microsoftClickLocator) First() playwright.Locator { return l }
func (l *microsoftClickLocator) IsVisible(_ ...playwright.LocatorIsVisibleOptions) (bool, error) {
	return true, nil
}
func (l *microsoftClickLocator) IsEnabled(_ ...playwright.LocatorIsEnabledOptions) (bool, error) {
	return true, nil
}
func (l *microsoftClickLocator) Click(_ ...playwright.LocatorClickOptions) error {
	l.clicks++
	return l.err
}

func TestMicrosoftClickTimeoutNeverResubmits(t *testing.T) {
	for _, clickErr := range []error{nil, errors.New("navigation timeout after click")} {
		loc := &microsoftClickLocator{err: clickErr}
		err := clickMicrosoftOnce(&microsoftClickPage{loc: loc}, microsoftNextSelectors(), "密码")
		if loc.clicks != 1 {
			t.Fatalf("ambiguous submission must not be repeated: %d", loc.clicks)
		}
		if (clickErr == nil) != (err == nil) {
			t.Fatalf("clickErr=%v result=%v", clickErr, err)
		}
		if err != nil && strings.Contains(err.Error(), "secret") {
			t.Fatalf("error leaked OAuth token: %s", err)
		}
	}
}

func TestOnMicrosoftURL(t *testing.T) {
	if !onMicrosoftURL("https://login.microsoftonline.com/common/oauth2/v2.0/authorize") {
		t.Fatal("microsoftonline")
	}
	if !onMicrosoftURL("https://login.live.com/oauth20_authorize.srf") {
		t.Fatal("login.live.com")
	}
	if !onMicrosoftURL("https://account.live.com/Consent/Update") {
		t.Fatal("account.live.com")
	}
	if onMicrosoftURL("https://authkit.cline.bot/sign-up") {
		t.Fatal("authkit is not microsoft host")
	}
	if onMicrosoftURL("https://app.cline.bot/dashboard") {
		t.Fatal("app is not microsoft host")
	}
}

func TestMicrosoftInvite(t *testing.T) {
	if got := microsoftInvite(""); got != "" {
		t.Fatalf("empty=%q", got)
	}
	if got := microsoftInvite("https://authkit.cline.bot/?ref=1"); got != "https://authkit.cline.bot/?ref=1" {
		t.Fatalf("keep invite=%q", got)
	}
	if got := microsoftInvite("https://authkit.cline.bot/sign-up"); got != "https://authkit.cline.bot/sign-up" {
		t.Fatalf("already=%q", got)
	}
}

func TestMicrosoftCardSettled(t *testing.T) {
	if microsoftCardSettled(200 * time.Millisecond) {
		t.Fatal("async card should not be ready on first paint")
	}
	if !microsoftCardSettled(microsoftCardSettle) {
		t.Fatal("card should be ready after it stays put")
	}
}

func TestMicrosoftStepPrefersEmailWhenBothVisible(t *testing.T) {
	if got := microsoftStep(true, true, false); got != "email" {
		t.Fatalf("both visible before email done => %q", got)
	}
	if got := microsoftStep(true, true, true); got != "password" {
		t.Fatalf("both visible after email done => %q", got)
	}
	if got := microsoftStep(false, true, false); got != "password" {
		t.Fatalf("only password => %q", got)
	}
	if got := microsoftStep(true, false, false); got != "email" {
		t.Fatalf("only email => %q", got)
	}
	if got := microsoftStep(false, false, false); got != "other" {
		t.Fatalf("none => %q", got)
	}
}

func TestNormalizeLoginProviderAliases(t *testing.T) {
	if model.NormalizeLoginProvider("outlook") != model.LoginMicrosoft {
		t.Fatal("outlook")
	}
	if model.NormalizeLoginProvider("") != model.LoginGoogle {
		t.Fatal("empty defaults google")
	}
}
