package herosms

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	HandlerURL     = "https://hero-sms.com/stubs/handler_api.php"
	RESTBase       = "https://hero-sms.com/api/v1"
	DefaultService = "ot"
)

type Client struct {
	APIKey  string
	Service string
	base    string
	rest    string
	http    *http.Client
}

type Country struct {
	ID        int     `json:"id"`
	Name      string  `json:"name"`
	PhoneCode int     `json:"phone_code,omitempty"`
	Quotes    []Quote `json:"quotes"`
}

type Quote struct {
	Price float64 `json:"price"`
	Count int     `json:"count"`
}

type Catalog struct {
	Balance   float64   `json:"balance"`
	Service   string    `json:"service"`
	Services  []Service `json:"services,omitempty"`
	Countries []Country `json:"countries"`
}

type Service struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type Number struct {
	ID          string
	Phone       string
	CountryCode int
	PhoneCode   int
	LocalNumber string
	Cost        float64
}

func New(apiKey, service string) *Client {
	svc := strings.TrimSpace(service)
	if svc == "" {
		svc = DefaultService
	}
	return &Client{
		APIKey:  strings.TrimSpace(apiKey),
		Service: svc,
		base:    HandlerURL,
		rest:    RESTBase,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) Catalog() (Catalog, error) {
	if c.APIKey == "" {
		return Catalog{}, fmt.Errorf("还没有配置 Hero SMS API Key")
	}
	bal, err := c.Balance()
	if err != nil {
		return Catalog{}, err
	}
	svcs, _ := c.Services()
	countries, err := c.countriesWithQuotes()
	if err != nil {
		return Catalog{}, err
	}
	return Catalog{
		Balance:   bal,
		Service:   c.Service,
		Services:  svcs,
		Countries: countries,
	}, nil
}

func (c *Client) Balance() (float64, error) {
	body, err := c.handler("getBalance", nil)
	if err != nil {
		return 0, err
	}
	text := strings.TrimSpace(string(body))
	if strings.HasPrefix(text, "ACCESS_BALANCE:") {
		n, err := strconv.ParseFloat(strings.TrimPrefix(text, "ACCESS_BALANCE:"), 64)
		if err != nil {
			return 0, fmt.Errorf("余额无效: %s", text)
		}
		return n, nil
	}
	var wrap struct {
		Balance any `json:"balance"`
	}
	if json.Unmarshal(body, &wrap) == nil {
		if n, ok := asFloat(wrap.Balance); ok {
			return n, nil
		}
	}
	return 0, fmt.Errorf("读取余额失败: %s", clip(text, 200))
}

func (c *Client) Services() ([]Service, error) {
	body, err := c.handler("getServicesList", url.Values{})
	if err != nil {
		return nil, err
	}
	return parseServices(body), nil
}

func (c *Client) GetNumber(country int, maxPrice float64) (Number, error) {
	if country <= 0 {
		return Number{}, fmt.Errorf("还没有选择 Hero SMS 区域")
	}
	q := url.Values{}
	q.Set("service", c.Service)
	q.Set("country", strconv.Itoa(country))
	if maxPrice > 0 {
		q.Set("maxPrice", formatPrice(maxPrice))
		q.Set("fixedPrice", "true")
	}
	body, err := c.handler("getNumberV2", q)
	if err != nil {
		return Number{}, err
	}
	text := strings.TrimSpace(string(body))
	if strings.HasPrefix(text, "ACCESS_NUMBER:") {
		return parseAccessNumber(text, country)
	}
	if err := asSMSError(text); err != nil {
		return Number{}, err
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return Number{}, fmt.Errorf("取号失败: %s", clip(text, 200))
	}
	n := Number{
		ID:          asString(first(raw, "activationId", "id")),
		Phone:       digits(asString(first(raw, "phoneNumber", "phone", "number"))),
		CountryCode: asInt(first(raw, "countryCode", "country")),
		PhoneCode:   asInt(first(raw, "countryPhoneCode", "phoneCode")),
		Cost:        asFloatDef(first(raw, "activationCost", "cost", "price")),
	}
	if n.CountryCode == 0 {
		n.CountryCode = country
	}
	applyPhoneSplit(&n)
	if n.ID == "" || n.Phone == "" {
		return Number{}, fmt.Errorf("取号失败: %s", clip(text, 200))
	}
	if n.PhoneCode <= 0 || n.LocalNumber == "" {
		return Number{}, fmt.Errorf("无法解析区号: %s", n.Phone)
	}
	return n, nil
}

func (c *Client) WaitCode(id string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		code, wait, err := c.Status(id)
		if err != nil {
			return "", err
		}
		if code != "" {
			return code, nil
		}
		if !wait {
			return "", fmt.Errorf("接码已取消或失败")
		}
		time.Sleep(5 * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时")
}

func (c *Client) Status(id string) (code string, waiting bool, err error) {
	q := url.Values{}
	q.Set("id", strings.TrimSpace(id))
	body, err := c.handler("getStatus", q)
	if err != nil {
		return "", false, err
	}
	text := strings.TrimSpace(string(body))
	switch {
	case text == "STATUS_WAIT_CODE", text == "STATUS_WAIT_RETRY", strings.HasPrefix(text, "STATUS_WAIT_RETRY:"):
		return "", true, nil
	case strings.HasPrefix(text, "STATUS_OK:"):
		return digits(strings.TrimPrefix(text, "STATUS_OK:")), false, nil
	case text == "STATUS_CANCEL", text == "STATUS_CANCELLED":
		return "", false, nil
	default:
		if err := asSMSError(text); err != nil {
			return "", false, err
		}
		return "", true, nil
	}
}

func (c *Client) Finish(id string) {
	c.setStatus(id, 6)
}

func (c *Client) Cancel(id string) {
	c.setStatus(id, 8)
}

func (c *Client) setStatus(id string, status int) {
	if strings.TrimSpace(id) == "" {
		return
	}
	q := url.Values{}
	q.Set("id", strings.TrimSpace(id))
	q.Set("status", strconv.Itoa(status))
	_, _ = c.handler("setStatus", q)
}

func (c *Client) countriesWithQuotes() ([]Country, error) {
	names, phoneCodes, _ := c.countryMeta()
	if offers, err := c.restOffers(); err == nil && len(offers) > 0 {
		return mergeNames(offers, names, phoneCodes), nil
	}
	body, err := c.handler("getPrices", url.Values{"service": {c.Service}})
	if err != nil {
		return nil, err
	}
	out, err := parsePrices(body, c.Service, names, phoneCodes)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("这个服务没有可用报价")
	}
	return out, nil
}

func (c *Client) countryMeta() (map[int]string, map[int]int, error) {
	names := map[int]string{}
	codes := map[int]int{}
	body, err := c.handler("getCountries", nil)
	if err != nil {
		return names, codes, err
	}
	var raw map[string]any
	if json.Unmarshal(body, &raw) != nil {
		return names, codes, nil
	}
	for _, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		id := asInt(first(m, "id", "country", "countryId"))
		if id == 0 {
			continue
		}
		name := asString(first(m, "chn", "cn", "eng", "rus", "name", "countryName"))
		if name != "" {
			names[id] = name
		}
		if pc := asInt(first(m, "phoneCode", "countryPhoneCode")); isCallingCode(pc) {
			codes[id] = pc
		} else if pc := asInt(m["code"]); isCallingCode(pc) {
			codes[id] = pc
		}
	}
	return names, codes, nil
}

func (c *Client) restOffers() ([]Country, error) {
	u, err := url.Parse(strings.TrimRight(c.rest, "/") + "/activations/offers/sms")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("api_key", c.APIKey)
	q.Set("service", c.Service)
	u.RawQuery = q.Encode()
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("报价接口 HTTP %d", resp.StatusCode)
	}
	return parseOffers(b, c.Service)
}

func (c *Client) handler(action string, extra url.Values) ([]byte, error) {
	u, err := url.Parse(c.base)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("api_key", c.APIKey)
	q.Set("action", action)
	for k, vs := range extra {
		for _, v := range vs {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(string(b))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Hero SMS HTTP %d: %s", resp.StatusCode, clip(text, 200))
	}
	if err := asSMSError(text); err != nil {
		return nil, err
	}
	return b, nil
}

func parseAccessNumber(text string, country int) (Number, error) {
	parts := strings.Split(text, ":")
	if len(parts) < 3 {
		return Number{}, fmt.Errorf("取号失败: %s", text)
	}
	phone := digits(parts[len(parts)-1])
	n := Number{ID: parts[1], Phone: phone, CountryCode: country}
	applyPhoneSplit(&n)
	if n.PhoneCode <= 0 || n.LocalNumber == "" {
		return Number{}, fmt.Errorf("无法解析区号: %s", phone)
	}
	return n, nil
}

func applyPhoneSplit(n *Number) {
	phone := digits(n.Phone)
	hint := n.PhoneCode
	if hint <= 0 {
		hint = CallingCodeForCountry(n.CountryCode)
	}
	inferred, inferredLocal := inferCallingCode(phone)
	if hint > 0 && strings.HasPrefix(phone, strconv.Itoa(hint)) {
		n.PhoneCode, n.LocalNumber = SplitPhone(n.Phone, hint)
		return
	}
	if inferred > 0 && (hint <= 0 || inferred == hint || (len(phone) >= 11 && !strings.HasPrefix(phone, strconv.Itoa(hint)))) {
		n.PhoneCode, n.LocalNumber = inferred, inferredLocal
		return
	}
	if hint > 0 {
		n.PhoneCode, n.LocalNumber = SplitPhone(n.Phone, hint)
		return
	}
	n.PhoneCode, n.LocalNumber = inferred, inferredLocal
}

func parsePrices(body []byte, service string, names map[int]string, phoneCodes map[int]int) ([]Country, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("价格列表无效")
	}
	out := []Country{}
	for ck, cv := range raw {
		cid, err := strconv.Atoi(ck)
		if err != nil {
			continue
		}
		quotes := quotesFromValue(cv, service)
		if len(quotes) == 0 {
			continue
		}
		out = append(out, Country{
			ID:        cid,
			Name:      countryName(cid, names),
			PhoneCode: phoneCodes[cid],
			Quotes:    quotes,
		})
	}
	sortCountries(out)
	return out, nil
}

func parseOffers(body []byte, service string) ([]Country, error) {
	var top any
	if err := json.Unmarshal(body, &top); err != nil {
		return nil, err
	}
	items := offerItems(top)
	byID := map[int]*Country{}
	order := []int{}
	for _, item := range items {
		svc := asString(first(item, "service", "serviceCode", "code"))
		if svc != "" && service != "" && !strings.EqualFold(svc, service) {
			continue
		}
		id := asInt(first(item, "country", "countryId", "countryCode", "id"))
		if id == 0 {
			continue
		}
		price, ok := asFloat(first(item, "price", "cost", "activationCost", "maxPrice"))
		if !ok {
			continue
		}
		count := asInt(first(item, "count", "quantity", "available", "stock"))
		c, hit := byID[id]
		if !hit {
			name := asString(first(item, "countryName", "chn", "eng", "name"))
			c = &Country{
				ID:        id,
				Name:      name,
				PhoneCode: asInt(first(item, "countryPhoneCode", "phoneCode")),
			}
			byID[id] = c
			order = append(order, id)
		}
		c.Quotes = appendQuote(c.Quotes, Quote{Price: price, Count: count})
	}
	out := make([]Country, 0, len(order))
	for _, id := range order {
		c := byID[id]
		if len(c.Quotes) == 0 {
			continue
		}
		if c.Name == "" {
			c.Name = fmt.Sprintf("国家 %d", c.ID)
		}
		out = append(out, *c)
	}
	sortCountries(out)
	return out, nil
}

func offerItems(v any) []map[string]any {
	switch t := v.(type) {
	case []any:
		out := []map[string]any{}
		for _, x := range t {
			if m, ok := x.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	case map[string]any:
		if inner, ok := t["data"]; ok {
			return offerItems(inner)
		}
		if inner, ok := t["offers"]; ok {
			return offerItems(inner)
		}
		if inner, ok := t["items"]; ok {
			return offerItems(inner)
		}
		out := []map[string]any{}
		for _, x := range t {
			switch n := x.(type) {
			case map[string]any:
				out = append(out, n)
			case []any:
				out = append(out, offerItems(n)...)
			}
		}
		return out
	default:
		return nil
	}
}

func quotesFromValue(v any, service string) []Quote {
	switch t := v.(type) {
	case map[string]any:
		if q, ok := quoteFromMap(t); ok {
			return []Quote{q}
		}
		if svc, ok := t[service]; ok {
			return quotesFromValue(svc, service)
		}
		out := []Quote{}
		for _, x := range t {
			out = append(out, quotesFromValue(x, service)...)
		}
		return out
	case []any:
		out := []Quote{}
		for _, x := range t {
			out = append(out, quotesFromValue(x, service)...)
		}
		return out
	default:
		return nil
	}
}

func quoteFromMap(m map[string]any) (Quote, bool) {
	price, ok := asFloat(first(m, "cost", "price"))
	if !ok {
		return Quote{}, false
	}
	return Quote{Price: price, Count: asInt(first(m, "count", "quantity"))}, true
}

func appendQuote(list []Quote, q Quote) []Quote {
	for i, old := range list {
		if old.Price == q.Price {
			if q.Count > old.Count {
				list[i].Count = q.Count
			}
			return list
		}
	}
	return append(list, q)
}

func parseServices(body []byte) []Service {
	var top any
	if json.Unmarshal(body, &top) != nil {
		return nil
	}
	out := []Service{}
	seen := map[string]bool{}
	add := func(code, name string) {
		code = strings.TrimSpace(code)
		if code == "" || seen[code] {
			return
		}
		seen[code] = true
		if name == "" {
			name = code
		}
		out = append(out, Service{Code: code, Name: name})
	}
	walkServices(top, add)
	return out
}

func walkServices(v any, add func(code, name string)) {
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			walkServices(x, add)
		}
	case map[string]any:
		code := asString(first(t, "code", "service", "id"))
		name := asString(first(t, "name", "eng", "chn"))
		if code != "" {
			add(code, name)
			return
		}
		for _, x := range t {
			walkServices(x, add)
		}
	}
}

func mergeNames(list []Country, names map[int]string, phoneCodes map[int]int) []Country {
	for i := range list {
		if list[i].Name == "" || strings.HasPrefix(list[i].Name, "国家 ") {
			if n := names[list[i].ID]; n != "" {
				list[i].Name = n
			}
		}
		if list[i].PhoneCode == 0 {
			list[i].PhoneCode = phoneCodes[list[i].ID]
		}
	}
	return list
}

func countryName(id int, names map[int]string) string {
	if n := names[id]; n != "" {
		return n
	}
	return fmt.Sprintf("国家 %d", id)
}

func sortCountries(list []Country) {
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if list[j].ID < list[i].ID {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
	for i := range list {
		qs := list[i].Quotes
		for a := 0; a < len(qs); a++ {
			for b := a + 1; b < len(qs); b++ {
				if qs[b].Price < qs[a].Price {
					qs[a], qs[b] = qs[b], qs[a]
				}
			}
		}
		list[i].Quotes = qs
	}
}

func localNumber(phone string, phoneCode int) string {
	_, local := SplitPhone(phone, phoneCode)
	return local
}

func asSMSError(text string) error {
	switch {
	case text == "BAD_KEY", text == "BAD_API_KEY":
		return fmt.Errorf("Hero SMS API Key 无效")
	case text == "NO_NUMBERS":
		return fmt.Errorf("这个区域/报价没有号码")
	case text == "NO_BALANCE":
		return fmt.Errorf("Hero SMS 余额不足")
	case text == "WRONG_SERVICE":
		return fmt.Errorf("Hero SMS 服务代码无效")
	case text == "WRONG_MAX_PRICE":
		return fmt.Errorf("报价无效，请重新选择")
	case strings.HasPrefix(text, "BANNED"):
		return fmt.Errorf("Hero SMS 账号被限制")
	default:
		return nil
	}
}

func first(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	default:
		return ""
	}
}

func asInt(v any) int {
	n, _ := asFloat(v)
	return int(n)
}

func asFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		n, err := t.Float64()
		return n, err == nil
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return n, err == nil
	default:
		return 0, false
	}
}

func asFloatDef(v any) float64 {
	n, _ := asFloat(v)
	return n
}

func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func formatPrice(n float64) string {
	s := strconv.FormatFloat(n, 'f', 4, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" {
		return "0"
	}
	return s
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
