package job

import (
	"strings"
	"time"
)

const (
	KindLogin   = "login"
	KindRefresh = "refresh"
	KindCookie  = "cookie"
)

func CookieKeepDate(t time.Time) string {
	return t.Format("2006-01-02")
}

func ClampCookieKeepHour(hour int) int {
	if hour < 0 || hour > 23 {
		return 4
	}
	return hour
}

func ShouldRunCookieKeep(now time.Time, enabled bool, hour int, lastDate string) bool {
	if !enabled {
		return false
	}
	hour = ClampCookieKeepHour(hour)
	if now.Hour() < hour {
		return false
	}
	return strings.TrimSpace(lastDate) != CookieKeepDate(now)
}
