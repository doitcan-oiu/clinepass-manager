package login

import (
	"regexp"
	"strings"
)

type Extracted struct {
	WorkspaceID string
	APIKey      string
	UserID      string
	Email       string
}

var (
	reWorkspace = regexp.MustCompile(`id:\s*"?(wrk_[A-Za-z0-9]+)"?`)
	reAPIKey    = regexp.MustCompile(`key:\s*"?(sk-[A-Za-z0-9]+)"?`)
	reUserID    = regexp.MustCompile(`userID:\s*"?(usr_[A-Za-z0-9]+)"?`)
	reEmail     = regexp.MustCompile(`email:\s*"([^"]+@[^"]+)"`)
	reWSLoose   = regexp.MustCompile(`wrk_[A-Z0-9]+`)
	reKeyLoose  = regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`)
	reUserLoose = regexp.MustCompile(`usr_[A-Z0-9]+`)
)

func ExtractFromHTML(html string) Extracted {
	var out Extracted
	if m := reWorkspace.FindStringSubmatch(html); len(m) == 2 {
		out.WorkspaceID = m[1]
	}
	if m := reAPIKey.FindStringSubmatch(html); len(m) == 2 {
		out.APIKey = m[1]
	}
	if m := reUserID.FindStringSubmatch(html); len(m) == 2 {
		out.UserID = m[1]
	}
	if m := reEmail.FindStringSubmatch(html); len(m) == 2 {
		out.Email = m[1]
	}
	if out.WorkspaceID == "" {
		out.WorkspaceID = reWSLoose.FindString(html)
	}
	if out.APIKey == "" {
		out.APIKey = reKeyLoose.FindString(html)
	}
	if out.UserID == "" {
		out.UserID = reUserLoose.FindString(html)
	}
	out.WorkspaceID = strings.TrimSpace(out.WorkspaceID)
	out.APIKey = strings.TrimSpace(out.APIKey)
	out.UserID = strings.TrimSpace(out.UserID)
	out.Email = strings.TrimSpace(out.Email)
	return out
}
