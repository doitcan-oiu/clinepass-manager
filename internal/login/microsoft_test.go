package login

import (
	"testing"
	"time"

	"opencode-go-manager/internal/model"
)

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
	if got := microsoftInvite(""); got != "https://authkit.cline.bot/sign-up" {
		t.Fatalf("empty=%q", got)
	}
	if got := microsoftInvite("https://authkit.cline.bot/?ref=1"); got != "https://authkit.cline.bot/sign-up" {
		t.Fatalf("authkit=%q", got)
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
