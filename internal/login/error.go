package login

import (
	"errors"
	"strings"
	"unicode/utf8"
)

var (
	ErrSMSNeedRelogin = errors.New("两次未收到验证码，需要重新登录")
	ErrPhoneTimeout   = errors.New("手机号超时")
	ErrAuthkitStuck   = errors.New("AuthKit 页面异常")
	ErrAccountBanned  = errors.New("账号已被封禁，已跳过")
)

func IsAuthkitFailure(err error) bool {
	if err == nil || errors.Is(err, ErrAccountBanned) {
		return false
	}
	if errors.Is(err, ErrAuthkitStuck) {
		return true
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "chrome-error") || strings.Contains(msg, "chromewebdata") {
		return true
	}
	if strings.Contains(msg, "authkit.cline.bot") && !strings.Contains(msg, "radar-challenge") {
		return true
	}
	return false
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
	limit := 200
	if utf8.RuneCountInString(msg) > limit {
		r := []rune(msg)
		msg = string(r[:limit]) + "…"
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
