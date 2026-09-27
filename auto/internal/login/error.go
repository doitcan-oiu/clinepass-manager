package login

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	ErrSMSNeedRelogin         = errors.New("两次未收到验证码，需要重新登录")
	ErrPhoneTimeout           = errors.New("手机号超时")
	ErrAuthkitStuck           = errors.New("AuthKit 页面异常")
	ErrAuthkitCallbackPending = errors.New("AuthKit 授权回调等待 30 秒后仍未完成，无法确认登录结果；请检查页面或联系服务方，已停止自动重试")
	ErrAccountBanned          = errors.New("账号已被封禁，已跳过")
	ErrRadarDenied            = errors.New("AuthKit Radar 服务端拒绝（policy_denied），已停止自动重试；请联系 Cline 核查")
)

func IsRadarDeniedMessage(msg string) bool {
	msg = strings.ToLower(msg)
	if strings.Contains(msg, "policy_denied") {
		return true
	}
	return strings.Contains(msg, "authkit radar") && strings.Contains(msg, "拦截")
}

func IsAuthkitFailure(err error) bool {
	if err == nil || errors.Is(err, ErrAccountBanned) || errors.Is(err, ErrRadarDenied) || errors.Is(err, ErrAuthkitCallbackPending) {
		return false
	}
	if errors.Is(err, ErrAuthkitStuck) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, raw := range messageURL.FindAllString(msg, -1) {
		u, parseErr := url.Parse(raw)
		if parseErr != nil {
			continue
		}
		if u.Scheme == "chrome-error" || u.Hostname() == "chromewebdata" {
			return true
		}
		if strings.EqualFold(u.Hostname(), "authkit.cline.bot") && !strings.Contains(u.Path, "radar-challenge") {
			return true
		}
	}
	// Keep textual AuthKit failures, but never interpret an OAuth redirect in a
	// query string (or an unrelated host's path) as the page we actually reached.
	text := messageURL.ReplaceAllString(msg, "")
	return strings.Contains(text, "authkit 页面异常") || strings.Contains(text, "authkit 回调错误") ||
		strings.Contains(text, "authkit.cline.bot") || strings.Contains(text, "chromewebdata")
}

var messageURL = regexp.MustCompile(`(?i)(?:https?|socks5h?|chrome-error)://[^\s<>"'，；）\)\]]+`)
var messageSecret = regexp.MustCompile(`(?i)\b(authorization_session_id|access_token|refresh_token|id_token|api_key|apikey|license_key|password|session|token|secret|cvv)\s*[:=]\s*(?:"[^"]*"|'[^']*'|[^\s,;，；]+)`)

func sanitizeMessage(msg string) string {
	msg = messageURL.ReplaceAllStringFunc(msg, func(raw string) string {
		u, err := url.Parse(raw)
		if err != nil {
			return "[链接已隐藏]"
		}
		u.User = nil
		u.RawQuery = ""
		u.ForceQuery = false
		u.Fragment = ""
		u.RawFragment = ""
		return u.String()
	})
	return messageSecret.ReplaceAllString(msg, "$1=[已隐藏]")
}

func CompactMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	extra := ""
	if i := strings.Index(msg, "Browser logs:"); i >= 0 {
		extra = lastUsefulLine(msg[i:])
		msg = strings.TrimSpace(msg[:i])
	}
	if i := strings.Index(msg, "Call log:"); i >= 0 {
		msg = strings.TrimSpace(msg[:i])
	}
	msg = strings.Join(strings.Fields(msg), " ")
	if extra != "" && !strings.Contains(msg, extra) {
		msg = strings.TrimSpace(msg + " " + extra)
	}
	msg = sanitizeMessage(msg)
	limit := 200
	if utf8.RuneCountInString(msg) > limit {
		r := []rune(msg)
		msg = string(r[:limit-1]) + "…"
	}
	return msg
}

func lastUsefulLine(s string) string {
	best := ""
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Browser logs:") || strings.HasPrefix(line, "<launching>") {
			continue
		}
		low := strings.ToLower(line)
		if strings.Contains(low, "error") || strings.Contains(low, "fatal") || strings.Contains(low, "session") || strings.Contains(low, "license") {
			best = line
		} else if best == "" {
			best = line
		}
	}
	return best
}

func CompactError(err error) error {
	if err == nil {
		return nil
	}
	return &shortError{msg: CompactMessage(err.Error()), cause: err}
}

type shortError struct {
	msg   string
	cause error
}

func (e *shortError) Error() string {
	return e.msg
}

func (e *shortError) Unwrap() error {
	return e.cause
}
