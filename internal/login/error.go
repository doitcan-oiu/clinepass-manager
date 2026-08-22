package login

import (
	"strings"
	"unicode/utf8"
)

func CompactMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	if i := strings.Index(msg, "Call log:"); i >= 0 {
		msg = strings.TrimSpace(msg[:i])
	}
	msg = strings.Join(strings.Fields(msg), " ")
	if utf8.RuneCountInString(msg) > 120 {
		r := []rune(msg)
		msg = string(r[:120]) + "…"
	}
	return msg
}

func CompactError(err error) error {
	if err == nil {
		return nil
	}
	return &shortError{msg: CompactMessage(err.Error())}
}

type shortError struct {
	msg string
}

func (e *shortError) Error() string {
	return e.msg
}
