package cline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"opencode-go-manager/internal/netproxy"
)

const (
	APIBase               = "https://api.cline.bot"
	AppBase               = "https://app.cline.bot"
	AuthURL               = "https://authkit.cline.bot"
	ua                    = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"
	CostUnitsPerUSD       = 100_000_000.0
	MaxDailyInclusiveDays = 31
)

var (
	reUserID     = regexp.MustCompile(`usr-[A-Za-z0-9]+`)
	reDistinctID = regexp.MustCompile(`"distinct_id"\s*:\s*"(usr-[A-Za-z0-9]+)"`)
)

type Caps struct {
	RollingUSD float64
	WeeklyUSD  float64
	MonthlyUSD float64
}

type UsageItem struct {
	Date      string
	Model     string
	CostUnits float64
}

type Client struct {
	Cookie string
	Proxy  string
	base   string
	app    string
	http   *http.Client
}

func (c *Client) SetBase(api, app string) {
	if api != "" {
		c.base = strings.TrimRight(api, "/")
	}
	if app != "" {
		c.app = strings.TrimRight(app, "/")
	}
}

func New(cookie, proxy string) *Client {
	return &Client{
		Cookie: strings.TrimSpace(cookie),
		Proxy:  strings.TrimSpace(proxy),
		base:   APIBase,
		app:    AppBase,
		http:   httpClient(proxy),
	}
}

func DefaultCaps() Caps {
	return Caps{RollingUSD: 10, WeeklyUSD: 25, MonthlyUSD: 50}
}

func UnitsToUSD(units float64) float64 {
	return units / CostUnitsPerUSD
}

func UserIDFromCookie(cookie string) string {
	if m := reDistinctID.FindStringSubmatch(cookie); len(m) == 2 {
		return m[1]
	}
	decoded, _ := url.QueryUnescape(cookie)
	if m := reDistinctID.FindStringSubmatch(decoded); len(m) == 2 {
		return m[1]
	}
	if id := reUserID.FindString(decoded); strings.HasPrefix(id, "usr-") {
		return id
	}
	return ""
}

func UserIDFromHTML(html string) string {
	return reUserID.FindString(html)
}

func ValidUserID(id string) bool {
	return strings.HasPrefix(strings.TrimSpace(id), "usr-")
}

func UserIDFromProfile(raw []byte) string {
	var wrap struct {
		ID   string          `json:"id"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &wrap) == nil {
		if ValidUserID(wrap.ID) {
			return strings.TrimSpace(wrap.ID)
		}
		if len(wrap.Data) > 0 {
			var data struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(wrap.Data, &data) == nil && ValidUserID(data.ID) {
				return strings.TrimSpace(data.ID)
			}
			if id := UserIDFromHTML(string(wrap.Data)); ValidUserID(id) {
				return id
			}
		}
	}
	return UserIDFromHTML(string(raw))
}

func DailyWindow(now time.Time) (start, end time.Time) {
	loc := now.Location()
	if loc == nil {
		loc = time.UTC
	}
	end = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	start = end.AddDate(0, 0, -(MaxDailyInclusiveDays - 1))
	return start, end
}

func (c *Client) CreateAPIKey(name string) (string, error) {
	if name == "" {
		name = "manager"
	}
	raw, err := c.doJSON(http.MethodPost, c.base+"/api/v1/api-keys", map[string]string{"name": name})
	if err != nil {
		return "", err
	}
	var wrap struct {
		Success bool `json:"success"`
		Data    struct {
			SecretKey string `json:"secret_key"`
			APIKey    struct {
				ID string `json:"id"`
			} `json:"api_key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return "", fmt.Errorf("创建 API Key 返回无效")
	}
	key := strings.TrimSpace(wrap.Data.SecretKey)
	if key == "" {
		return "", fmt.Errorf("创建 API Key 没有返回 secret_key")
	}
	return key, nil
}

func (c *Client) DiscoverUserID() (string, error) {
	raw, meErr := c.doJSON(http.MethodGet, c.base+"/api/v1/users/me", nil)
	if meErr == nil {
		if id := UserIDFromProfile(raw); ValidUserID(id) {
			return id, nil
		}
	}
	if id := UserIDFromCookie(c.Cookie); ValidUserID(id) {
		return id, nil
	}
	html, err := c.getHTML(c.app + "/dashboard")
	if err != nil {
		if meErr != nil {
			return "", meErr
		}
		return "", err
	}
	if id := UserIDFromHTML(html); ValidUserID(id) {
		return id, nil
	}
	if meErr != nil {
		return "", meErr
	}
	return "", fmt.Errorf("没有解析到用户 ID")
}

func (c *Client) PlansCaps() (Caps, error) {
	raw, err := c.doJSON(http.MethodGet, c.base+"/api/v1/plans?type=individual", nil)
	if err != nil {
		return DefaultCaps(), err
	}
	caps := ParsePlansCaps(raw)
	if caps.MonthlyUSD <= 0 {
		return DefaultCaps(), nil
	}
	return caps, nil
}

type PlanUsageLimit struct {
	Type        string
	PercentUsed float64
	ResetsAt    time.Time
}

func (c *Client) PlanUsageLimits() ([]PlanUsageLimit, error) {
	raw, err := c.doJSON(http.MethodGet, c.base+"/api/v1/users/me/plan/usage-limits", nil)
	if err != nil {
		return nil, err
	}
	return ParsePlanUsageLimits(raw)
}

func ParsePlanUsageLimits(raw []byte) ([]PlanUsageLimit, error) {
	var wrap struct {
		Success bool `json:"success"`
		Data    struct {
			Limits []struct {
				Type        string  `json:"type"`
				PercentUsed float64 `json:"percentUsed"`
				ResetsAt    string  `json:"resetsAt"`
			} `json:"limits"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("套餐用量返回无效")
	}
	out := make([]PlanUsageLimit, 0, len(wrap.Data.Limits))
	for _, it := range wrap.Data.Limits {
		item := PlanUsageLimit{Type: strings.TrimSpace(it.Type), PercentUsed: it.PercentUsed}
		if t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(it.ResetsAt)); err == nil {
			item.ResetsAt = t
		}
		out = append(out, item)
	}
	return out, nil
}

func (c *Client) DailyUsages(userID string, start, end time.Time) ([]UsageItem, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("缺少用户 ID")
	}
	u := fmt.Sprintf("%s/api/v1/users/%s/usages/daily?startDate=%s&endDate=%s",
		c.base, url.PathEscape(userID), start.Format("2006-01-02"), end.Format("2006-01-02"))
	raw, err := c.doJSON(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	return ParseDailyUsages(raw)
}

func ParsePlansCaps(raw []byte) Caps {
	out := DefaultCaps()
	var wrap struct {
		Data []struct {
			Interval     string `json:"interval"`
			IsActive     bool   `json:"isActive"`
			Entitlements struct {
				ClinePass struct {
					InferenceCapThreshold struct {
						Last5HoursUsageCostUSDPerUser float64 `json:"last5HoursUsageCostUSDPerUser"`
						Last7DaysUsageCostUSDPerUser  float64 `json:"last7daysUsageCostUSDPerUser"`
						Last30DaysUsageCostUSDPerUser float64 `json:"last30daysUsageCostUSDPerUser"`
					} `json:"inferenceCapThreshold"`
				} `json:"cline_pass"`
			} `json:"entitlements"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &wrap) != nil {
		return out
	}
	pick := -1
	for i, p := range wrap.Data {
		if strings.EqualFold(p.Interval, "Monthly") {
			pick = i
			break
		}
	}
	if pick < 0 && len(wrap.Data) > 0 {
		pick = 0
	}
	if pick < 0 {
		return out
	}
	th := wrap.Data[pick].Entitlements.ClinePass.InferenceCapThreshold
	if th.Last5HoursUsageCostUSDPerUser > 0 {
		out.RollingUSD = UnitsToUSD(th.Last5HoursUsageCostUSDPerUser)
	}
	if th.Last7DaysUsageCostUSDPerUser > 0 {
		out.WeeklyUSD = UnitsToUSD(th.Last7DaysUsageCostUSDPerUser)
	}
	if th.Last30DaysUsageCostUSDPerUser > 0 {
		out.MonthlyUSD = UnitsToUSD(th.Last30DaysUsageCostUSDPerUser)
	}
	return out
}

func ParseDailyUsages(raw []byte) ([]UsageItem, error) {
	var wrap struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				Date        string  `json:"date"`
				AIModelName string  `json:"aiModelName"`
				CostUSD     float64 `json:"costUsd"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("用量返回无效")
	}
	out := make([]UsageItem, 0, len(wrap.Data.Items))
	for _, it := range wrap.Data.Items {
		if strings.TrimSpace(it.Date) == "" {
			continue
		}
		out = append(out, UsageItem{
			Date:      it.Date,
			Model:     strings.TrimSpace(it.AIModelName),
			CostUnits: it.CostUSD,
		})
	}
	return out, nil
}

func (c *Client) doJSON(method, rawURL string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", c.app)
	req.Header.Set("Referer", c.app+"/")
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Cookie", c.Cookie)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusSeeOther {
		return nil, fmt.Errorf("Cookie 失效，被重定向到登录")
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("未支付或没有权限")
	}
	if resp.StatusCode >= 400 {
		return nil, formatAPIError(resp.StatusCode, b)
	}
	var env struct {
		Success *bool `json:"success"`
	}
	if json.Unmarshal(b, &env) == nil && env.Success != nil && !*env.Success {
		return nil, formatAPIError(resp.StatusCode, b)
	}
	return b, nil
}

func formatAPIError(status int, body []byte) error {
	msg := strings.TrimSpace(string(body))
	var wrap struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &wrap) == nil {
		if e := strings.TrimSpace(wrap.Error); e != "" {
			msg = e
		} else if m := strings.TrimSpace(wrap.Message); m != "" {
			msg = m
		}
	}
	msg = strings.Join(strings.Fields(msg), " ")
	if len(msg) > 240 {
		msg = msg[:240]
	}
	if msg == "" {
		return fmt.Errorf("Cline API HTTP %d", status)
	}
	return fmt.Errorf("Cline API HTTP %d: %s", status, msg)
}

func (c *Client) getHTML(rawURL string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Cookie", c.Cookie)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusSeeOther || resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("Cookie 失效，被重定向到登录")
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("打开页面失败 HTTP %d", resp.StatusCode)
	}
	return string(b), nil
}

func httpClient(proxy string) *http.Client {
	tr := &http.Transport{}
	if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		tr = dt.Clone()
	}
	netproxy.Apply(tr, proxy)
	return &http.Client{
		Timeout:   45 * time.Second,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if strings.Contains(req.URL.Host, "authkit.") || strings.Contains(req.URL.Path, "/login") {
				return http.ErrUseLastResponse
			}
			if len(via) >= 8 {
				return fmt.Errorf("重定向过多")
			}
			return nil
		},
	}
}
