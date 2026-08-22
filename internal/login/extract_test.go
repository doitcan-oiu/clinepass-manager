package login

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"opencode-go-manager/internal/herosms"
)

func TestExtractFromHTML(t *testing.T) {
	html := `
<script>
$R[20]($R[14], $R[23] = [$R[24] = {
    id: "key_01M0K2ZZ3EN4F0WAZVRPQ6H3T0",
    name: "Default API Key",
    key: "sk-N742UchDf3sMOPEV26nyzGvCBRQccugYf63yge4jIWGB19WwMZz4CkT83Nscs1Dj",
    timeUsed: null,
    userID: "usr_01M0K2ZXN5TFXDY0KNRT06WZWV",
    email: "giv09p2i1mw4@lincolnrch.us",
    keyDisplay: "sk-N742...s1Dj"
}]);
id: "wrk_01M0K2ZXN5NWWYMY6D0ZES6HDN"
</script>`
	got := ExtractFromHTML(html)
	if got.WorkspaceID != "wrk_01M0K2ZXN5NWWYMY6D0ZES6HDN" {
		t.Fatalf("workspace=%s", got.WorkspaceID)
	}
	if got.UserID != "usr_01M0K2ZXN5TFXDY0KNRT06WZWV" {
		t.Fatalf("user=%s", got.UserID)
	}
	if got.Email != "giv09p2i1mw4@lincolnrch.us" {
		t.Fatalf("email=%s", got.Email)
	}
	if got.APIKey != "sk-N742UchDf3sMOPEV26nyzGvCBRQccugYf63yge4jIWGB19WwMZz4CkT83Nscs1Dj" {
		t.Fatalf("key=%s", got.APIKey)
	}
}

func TestClassifyGooglePasswordPageNotConsent(t *testing.T) {
	pwd := "https://accounts.google.com/v3/signin/challenge/pwd?continue=https%3A%2F%2Faccounts.google.com%2Fsignin%2Foauth%2Fconsent%3Fauthuser%3Dunknown"
	if got := classifyGoogle(pwd); got != "password" {
		t.Fatalf("pwd page classified as %q", got)
	}
	ident := "https://accounts.google.com/v3/signin/identifier?client_id=x"
	if got := classifyGoogle(ident); got != "email" {
		t.Fatalf("identifier classified as %q", got)
	}
	consent := "https://accounts.google.com/signin/oauth/id?authuser=1"
	if got := classifyGoogle(consent); got != "consent" {
		t.Fatalf("consent classified as %q", got)
	}
	errURL := "https://accounts.google.com/v3/signin/unknownerror?continue=https%3A%2F%2Faccounts.google.com%2Fsignin%2Foauth%2Fconsent%3Fx%3D1"
	if got := classifyGoogle(errURL); got != "unknownerror" {
		t.Fatalf("unknownerror classified as %q", got)
	}
	if got := googleContinueURL(errURL); got != "https://accounts.google.com/signin/oauth/consent?x=1" {
		t.Fatalf("continue=%q", got)
	}
}

func TestLeftGoogleURL(t *testing.T) {
	if leftGoogleURL("https://accounts.google.com/signin/oauth/id") {
		t.Fatal("oauth page should stay in google login")
	}
	if leftGoogleURL("about:blank") {
		t.Fatal("blank should not count as left")
	}
	if !leftGoogleURL("https://api.cline.bot/api/v1/auth/callback") {
		t.Fatal("oauth callback should leave google")
	}
	if !leftGoogleURL("https://authkit.cline.bot/radar-challenge/send") {
		t.Fatal("radar should leave google")
	}
	if !leftGoogleURL("https://app.cline.bot/dashboard") {
		t.Fatal("app should leave google")
	}
	login := "https://authkit.cline.bot/?redirect_uri=https%3A%2F%2Fapi.cline.bot%2Fapi%2Fv1%2Fauth%2Fcallback&authorization_session_id=01ABC"
	if leftGoogleURL(login) {
		t.Fatal("authkit login page is not finished login")
	}
	if !onAuthkitLogin(login) {
		t.Fatal("should detect authkit login bounce")
	}
	if onAuthkitLogin("https://authkit.cline.bot/radar-challenge/send") {
		t.Fatal("radar is not login page")
	}
	chooser := "https://accounts.google.com/v3/signin/accountchooser?state=eyJ&redirect_uri=https%3A%2F%2Fauthkit.cline.bot%2Fapi%2Fcallback"
	if onAuthkitLogin(chooser) {
		t.Fatal("google accountchooser must not look like authkit login")
	}
	if leftGoogleURL(chooser) {
		t.Fatal("accountchooser is still google")
	}
	if classifyGoogle(chooser) != "chooser" {
		t.Fatal("accountchooser step")
	}
	if onAuthkitLogin("https://accounts.google.com/signin/oauth/id?continue=https%3A%2F%2Fauthkit.cline.bot%2F") {
		t.Fatal("google oauth query must not look like authkit login")
	}
	if !onRadarURL("https://authkit.cline.bot/radar-challenge/send?user_id=1") {
		t.Fatal("phone verify is radar")
	}
	if onAuthkitLogin("https://authkit.cline.bot/radar-challenge/verify") {
		t.Fatal("otp page is not login")
	}
	if authkitSessionID(login) != "01ABC" {
		t.Fatalf("session id=%q", authkitSessionID(login))
	}
	if authkitSessionID("https://authkit.cline.bot/") != "" {
		t.Fatal("bare authkit login has no session")
	}
	if authkitSessionID(chooser) != "" {
		t.Fatal("google chooser must not take authkit session from query")
	}
}

func TestRadarSendURL(t *testing.T) {
	verify := "https://authkit.cline.bot/radar-challenge/verify?authorization_session_id=abc"
	got := radarSendURL(verify)
	if !strings.Contains(got, "radar-challenge/send") || strings.Contains(got, "radar-challenge/verify") {
		t.Fatalf("%s", got)
	}
}

func TestIsNoSMS(t *testing.T) {
	if !isNoSMS(herosms.ErrWaitCodeTimeout) {
		t.Fatal("timeout")
	}
	if !isNoSMS(herosms.ErrCancelled) {
		t.Fatal("cancelled")
	}
	if !isNoSMS(fmt.Errorf("%w: HTTP 503", herosms.ErrUnavailable)) {
		t.Fatal("unavailable")
	}
	if isNoSMS(errors.New("填写区号失败")) {
		t.Fatal("other error")
	}
}

func TestCompactErrorKeepsSMSSentinel(t *testing.T) {
	err := CompactError(ErrSMSNeedRelogin)
	if !errors.Is(err, ErrSMSNeedRelogin) {
		t.Fatal(err)
	}
}

func TestCompactMessageStripsPlaywrightCallLog(t *testing.T) {
	raw := `playwright: timeout: Timeout 20000ms exceeded.
Call log:
  - waiting for locator('input[name="Passwd"], #password input[type="password"]').first() to be visible
    - waiting for "https://auth.opencode.ai/google/callback" navigation to finish...
    - navigated to "https://opencode.ai/workspace/wrk_01M0K94G8V0C0SYSYFSJ9TVBV1"`
	got := CompactMessage(raw)
	if strings.Contains(got, "Call log") || strings.Contains(got, "navigated to") {
		t.Fatalf("still too long: %q", got)
	}
	if len([]rune(got)) > 200 {
		t.Fatalf("len=%d %q", len([]rune(got)), got)
	}
}

func TestToOptionalCookies(t *testing.T) {
	raw := `[{"name":"sid","value":"abc","domain":".opencode.ai","path":"/","expires":-1,"httpOnly":true,"secure":true}]`
	got, err := toOptionalCookies(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "sid" || got[0].Value != "abc" {
		t.Fatalf("%+v", got)
	}
	if got[0].Domain == nil || *got[0].Domain != ".opencode.ai" {
		t.Fatalf("domain %+v", got[0].Domain)
	}
}
