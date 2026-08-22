package usage

import (
	"fmt"
	"strings"

	"opencode-go-manager/internal/cline"
	"opencode-go-manager/internal/model"
)

func Hydrate(a *model.Account, proxy string) error {
	if a == nil {
		return fmt.Errorf("账号为空")
	}
	cookie := strings.TrimSpace(a.CookieHeader)
	if cookie == "" {
		return fmt.Errorf("缺少 Cookie")
	}
	c := cline.New(cookie, proxy)
	if !cline.ValidUserID(a.UserID) {
		id, err := c.DiscoverUserID()
		if err != nil {
			return err
		}
		a.UserID = id
	}
	if strings.TrimSpace(a.WorkspaceID) == "" {
		a.WorkspaceID = a.UserID
	}
	if strings.TrimSpace(a.APIKey) != "" {
		return nil
	}
	key, err := c.CreateAPIKey("manager")
	if err != nil {
		return err
	}
	a.APIKey = key
	return nil
}
