package login

import (
	"os"
	"strings"
)

func Engine() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOGIN_ENGINE"))) {
	case "go":
		return "go"
	default:
		return "python"
	}
}
