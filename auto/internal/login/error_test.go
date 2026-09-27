package login

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCompactMessageRedactsURLCredentialsAndOAuth(t *testing.T) {
	raw := "Microsoft 登录未完成，当前 URL=https://alice:private-password@login.live.com/oauth20_authorize.srf?state=secret-state&redirect_uri=https%3A%2F%2Fauthkit.cline.bot%2Fcallback#access_token=secret-token"
	got := CompactMessage(raw)
	if got != "Microsoft 登录未完成，当前 URL=https://login.live.com/oauth20_authorize.srf" {
		t.Fatalf("unexpected compact URL: %s", got)
	}
	if IsAuthkitFailure(errors.New(raw)) || IsAuthkitFailure(CompactError(errors.New(raw))) {
		t.Fatal("an OAuth redirect must not cause an AuthKit account retry")
	}
}

func TestCompactMessageKeepsUsefulBrowserFailure(t *testing.T) {
	raw := "BrowserType.launch: Target page, context or browser has been closed\nBrowser logs:\n<launching> chrome --password=private\n[pid=100][err] FATAL license_key=supersecret session=secret-session License expired\nCall log:\n- waiting for a browser"
	got := CompactMessage(raw)
	if !strings.Contains(got, "License expired") || strings.Contains(got, "supersecret") || strings.Contains(got, "secret-session") || strings.Contains(got, "<launching>") {
		t.Fatalf("bad compact browser failure: %s", got)
	}
	if utf8.RuneCountInString(got) > 200 {
		t.Fatalf("message too long: %d", utf8.RuneCountInString(got))
	}
}

func TestCompactErrorPreservesSentinel(t *testing.T) {
	cause := fmt.Errorf("upstream: %w; https://example.com/verify?token=sensitive", ErrPhoneTimeout)
	err := CompactError(cause)
	if !errors.Is(err, ErrPhoneTimeout) || errors.Unwrap(err) != cause || strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("lost cause or leaked query: %v", err)
	}
}

func TestAuthkitRetryUsesActualURLHost(t *testing.T) {
	tests := []struct {
		message string
		want    bool
	}{
		{"登录未完成 URL=https://authkit.cline.bot/?authorization_session_id=secret", true},
		{"登录未完成 URL=https://AUTHKIT.CLINE.BOT/callback?error=failed", true},
		{"登录未完成 URL=https://login.live.com/?redirect_uri=https://authkit.cline.bot/callback", false},
		{"登录未完成 URL=https://login.live.com/authkit.cline.bot?next=chrome-error://chromewebdata/", false},
		{"登录未完成 URL=https://authkit.cline.bot.attacker.example/", false},
		{"登录未完成 URL=https://authkit.cline.bot/radar-challenge", false},
		{"登录未完成 URL=chrome-error://chromewebdata/", true},
		{"AuthKit 回调错误 error=access_denied", true},
	}
	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			if got := IsAuthkitFailure(errors.New(tt.message)); got != tt.want {
				t.Fatalf("IsAuthkitFailure=%v, want %v", got, tt.want)
			}
		})
	}
}
