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
	return &shortError{msg: CompactMessage(err.Error())}
}

type shortError struct {
	msg string
}

func (e *shortError) Error() string {
	return e.msg
}
