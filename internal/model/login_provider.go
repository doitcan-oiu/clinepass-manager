package model

import "strings"

const (
	LoginGoogle    = "google"
	LoginMicrosoft = "microsoft"
)

func NormalizeLoginProvider(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case LoginMicrosoft, "ms", "outlook", "hotmail", "live":
		return LoginMicrosoft
	default:
		return LoginGoogle
	}
}

func (a Account) IsMicrosoft() bool {
	return NormalizeLoginProvider(a.LoginProvider) == LoginMicrosoft
}
