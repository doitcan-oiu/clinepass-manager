package backup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"opencode-go-manager/internal/model"
)

type File struct {
	Version  int       `json:"version"`
	Accounts []Account `json:"accounts"`
}

type Account struct {
	Account       string `json:"account"`
	Password      string `json:"password,omitempty"`
	AuxEmail      string `json:"auxEmail,omitempty"`
	WorkspaceID   string `json:"workspaceID,omitempty"`
	Auth          string `json:"auth"`
	APIKey        string `json:"apiKey,omitempty"`
	UserID        string `json:"userID,omitempty"`
	LoginType     string `json:"loginType,omitempty"`
	LoginProvider string `json:"login_provider,omitempty"`
}

type ParseResult struct {
	Items   []model.CreatePaidAccountInput
	Skipped []string
}

func Parse(raw []byte) (ParseResult, error) {
	raw = bytes.TrimPrefix(bytes.TrimSpace(raw), []byte{0xEF, 0xBB, 0xBF})
	if len(raw) == 0 {
		return ParseResult{}, fmt.Errorf("文件是空的")
	}
	var list []Account
	switch raw[0] {
	case '[':
		if err := json.Unmarshal(raw, &list); err != nil {
			return ParseResult{}, fmt.Errorf("JSON 无效：%w", err)
		}
	case '{':
		var file File
		if err := json.Unmarshal(raw, &file); err != nil {
			return ParseResult{}, fmt.Errorf("JSON 无效：%w", err)
		}
		list = file.Accounts
		if len(list) == 0 {
			var wrap struct {
				Items []Account `json:"items"`
			}
			if err := json.Unmarshal(raw, &wrap); err == nil {
				list = wrap.Items
			}
		}
	default:
		return ParseResult{}, fmt.Errorf("不是 JSON 备份")
	}
	out := ParseResult{Items: []model.CreatePaidAccountInput{}, Skipped: []string{}}
	seen := map[string]bool{}
	for i, row := range list {
		in, err := row.toInput()
		if err != nil {
			out.Skipped = append(out.Skipped, fmt.Sprintf("第 %d 条：%s", i+1, err.Error()))
			continue
		}
		if seen[in.Email] {
			out.Skipped = append(out.Skipped, fmt.Sprintf("第 %d 条：重复账号 %s", i+1, in.Email))
			continue
		}
		seen[in.Email] = true
		out.Items = append(out.Items, in)
	}
	if len(out.Items) == 0 {
		return out, fmt.Errorf("没有可导入的账号（至少要有邮箱和 Cookie）")
	}
	return out, nil
}

func (a Account) toInput() (model.CreatePaidAccountInput, error) {
	email := strings.ToLower(strings.TrimSpace(a.Account))
	cookie := NormalizeCookie(a.Auth)
	if email == "" || !strings.Contains(email, "@") {
		return model.CreatePaidAccountInput{}, fmt.Errorf("缺少账号")
	}
	if cookie == "" {
		return model.CreatePaidAccountInput{}, fmt.Errorf("%s 缺少 Cookie", email)
	}
	return model.CreatePaidAccountInput{
		Email:         email,
		Password:      strings.TrimSpace(a.Password),
		RecoveryEmail: strings.TrimSpace(a.AuxEmail),
		APIKey:        strings.TrimSpace(a.APIKey),
		WorkspaceID:   strings.TrimSpace(a.WorkspaceID),
		UserID:        strings.TrimSpace(a.UserID),
		CookieHeader:  cookie,
		LoginProvider: model.NormalizeLoginProvider(firstNonEmpty(a.LoginType, a.LoginProvider)),
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func NormalizeCookie(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "[") {
		return cookieJSONToHeader(raw)
	}
	if strings.Contains(raw, "cline_session_id=") || strings.Contains(raw, "auth=") || strings.Contains(raw, "oc-") {
		return raw
	}
	if strings.HasPrefix(raw, "Fe26.") {
		return "auth=" + raw
	}
	return raw
}

func cookieJSONToHeader(raw string) string {
	var cookies []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(raw), &cookies); err != nil {
		return ""
	}
	parts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		if strings.TrimSpace(c.Name) == "" {
			continue
		}
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}

func Export(accounts []model.Account) File {
	out := File{Version: 1, Accounts: []Account{}}
	for _, a := range accounts {
		cookie := strings.TrimSpace(a.CookieHeader)
		if cookie == "" {
			cookie = cookieJSONToHeader(a.CookiesJSON)
		}
		out.Accounts = append(out.Accounts, Account{
			Account:     a.Email,
			Password:    a.Password,
			AuxEmail:    a.RecoveryEmail,
			WorkspaceID: a.WorkspaceID,
			Auth:        cookie,
			APIKey:      a.APIKey,
			UserID:      a.UserID,
			LoginType:   model.NormalizeLoginProvider(a.LoginProvider),
		})
	}
	return out
}
