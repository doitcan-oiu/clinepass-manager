package amzkeys

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultHost      = "https://testapi.amzkeys.com"
	ProductionHost   = "https://ymapi.amzkeys.com:15970"
	DefaultCardType  = 467845
	DefaultAmount    = 20
	AESIV            = "SADEUT78WE23HGKW"
	createPath       = "/api/v1/card/create"
	taskDetailPath   = "/api/v1/card/taskDetail"
	cardTypesPath    = "/api/v1/card/getCardTypes"
	balancePath      = "/api/v1/account/balance"
	authCodePath     = "/api/v1/authorization/authCode"
	taskPollInterval = 5 * time.Second
	taskPollTimeout  = 6 * time.Minute
)

type Client struct {
	Host       string
	AppID      string
	AppKey     string
	PrivateKey string
	CardType   int
	Amount     float64
	http       *http.Client
	now        func() time.Time
}

type Balance struct {
	Currency        string `json:"currency"`
	AvailableAmount string `json:"available_amount"`
	FrozenAmount    string `json:"frozen_amount"`
}

type CardType struct {
	CardType          int    `json:"card_type"`
	NewCardFee        string `json:"new_card_fee"`
	ServiceFee        string `json:"service_fee"`
	MinOpencardAmount string `json:"min_opencard_amount"`
	MinRechargeAmount string `json:"min_recharge_amount"`
}

type Card struct {
	CardType       int         `json:"card_type"`
	RequestID      looseString `json:"request_id"`
	CardNo         string      `json:"card_no"`
	CVV            string      `json:"cvv"`
	ValidDate      string      `json:"valid_date"`
	OpenCardAmount float64     `json:"open_card_amount,string"`
	CreateTime     string      `json:"create_time"`
}

type looseString string

func (s *looseString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*s = ""
		return nil
	}
	if b[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*s = looseString(v)
		return nil
	}
	*s = looseString(string(b))
	return nil
}

type AuthCode struct {
	CardNoLast4               string `json:"card_no_last4"`
	AuthCode                  string `json:"auth_code"`
	MerchantName              string `json:"merchant_name"`
	TraderAmount              string `json:"trader_amount"`
	TraderBillingCurrencyCode string `json:"trader_billing_currency_code"`
	Content                   string `json:"content"`
	CreateTime                any    `json:"create_time"`
}

type Status struct {
	Host      string     `json:"host"`
	Balances  []Balance  `json:"balances"`
	CardTypes []CardType `json:"card_types"`
}

func New(host, appID, appKey, privateKey string, cardType int, amount float64) *Client {
	host, appID, appKey, privateKey = Resolve(host, appID, appKey, privateKey)
	if cardType <= 0 {
		cardType = DefaultCardType
	}
	if amount <= 0 {
		amount = DefaultAmount
	}
	return &Client{
		Host:       host,
		AppID:      appID,
		AppKey:     appKey,
		PrivateKey: privateKey,
		CardType:   cardType,
		Amount:     amount,
		http:       &http.Client{Timeout: 30 * time.Second},
		now:        time.Now,
	}
}

func Ready(host, appID, appKey, privateKey string, cardType int, amount float64) error {
	savedHost := strings.TrimSpace(host)
	savedAppID := strings.TrimSpace(appID)
	host, appID, appKey, privateKey = Resolve(host, appID, appKey, privateKey)
	if IsTestHost(host) && savedHost == "" && savedAppID == "" {
		return fmt.Errorf("请先在设置里保存 amzkeys卡台")
	}
	if appID == "" {
		return fmt.Errorf("缺少 AppID")
	}
	if appKey == "" {
		return fmt.Errorf("缺少 AppKey")
	}
	if privateKey == "" {
		return fmt.Errorf("生产环境请填写商务给的 RSA2 私钥，测试环境不用自己生成")
	}
	if _, err := ParsePrivateKey(privateKey); err != nil {
		return fmt.Errorf("RSA2 私钥无效: %w", err)
	}
	if cardType <= 0 {
		return fmt.Errorf("缺少卡段")
	}
	if amount <= 0 {
		return fmt.Errorf("开卡金额须大于 0")
	}
	_ = host
	return nil
}

func (c *Client) Status() (Status, error) {
	balances, err := c.AccountBalance()
	if err != nil {
		return Status{}, err
	}
	types, err := c.CardTypes()
	if err != nil {
		return Status{}, err
	}
	return Status{Host: c.Host, Balances: balances, CardTypes: types}, nil
}

func (c *Client) AccountBalance() ([]Balance, error) {
	raw, err := c.call(balancePath, map[string]any{})
	if err != nil {
		return nil, err
	}
	var out []Balance
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("解析账户余额失败: %w", err)
	}
	return out, nil
}

func (c *Client) CardTypes() ([]CardType, error) {
	raw, err := c.call(cardTypesPath, map[string]any{})
	if err != nil {
		return nil, err
	}
	var out []CardType
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("解析卡段失败: %w", err)
	}
	return out, nil
}

func (c *Client) SubmitCreate() (taskID, ram string, err error) {
	raw, err := c.call(createPath, map[string]any{
		"card_type":    c.CardType,
		"number":       1,
		"amount":       c.Amount,
		"account_type": "USD",
	})
	if err != nil {
		return "", "", err
	}
	var created struct {
		TaskID  any    `json:"task_id"`
		OrderNo string `json:"order_no"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		return "", "", fmt.Errorf("解析开卡任务失败: %w", err)
	}
	taskID = anyString(created.TaskID)
	if taskID == "" {
		return "", "", fmt.Errorf("开卡任务没有返回 task_id")
	}
	ram, err = randomRAM()
	if err != nil {
		return "", "", err
	}
	return taskID, ram, nil
}

func (c *Client) WaitTask(taskID, ram string) (Card, error) {
	taskID = strings.TrimSpace(taskID)
	ram = strings.TrimSpace(ram)
	if taskID == "" || ram == "" {
		return Card{}, fmt.Errorf("缺少开卡任务")
	}
	deadline := time.Now().Add(taskPollTimeout)
	var last string
	for time.Now().Before(deadline) {
		cards, status, err := c.taskCards(1, taskID, ram)
		if err != nil {
			if status == "1" || status == "3" || status == "4" || status == "5" {
				return Card{}, err
			}
			last = err.Error()
			time.Sleep(taskPollInterval)
			continue
		}
		if status == "3" || status == "5" {
			return Card{}, fmt.Errorf("开卡任务失败，状态 %s", status)
		}
		if len(cards) > 0 && strings.TrimSpace(cards[0].CardNo) != "" {
			return cards[0], nil
		}
		if status == "1" || status == "4" {
			if len(cards) == 0 {
				return Card{}, fmt.Errorf("开卡任务已完成但没有卡数据")
			}
		}
		time.Sleep(taskPollInterval)
	}
	if last != "" {
		return Card{}, fmt.Errorf("等待开卡超时: %s", last)
	}
	return Card{}, fmt.Errorf("等待开卡超时")
}

func (c *Client) CreateCard() (Card, error) {
	taskID, ram, err := c.SubmitCreate()
	if err != nil {
		return Card{}, err
	}
	return c.WaitTask(taskID, ram)
}

func (c *Client) AuthCodes() ([]AuthCode, error) {
	raw, err := c.call(authCodePath, map[string]any{})
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Item []AuthCode `json:"item"`
	}
	if err := json.Unmarshal(raw, &wrap); err == nil && wrap.Item != nil {
		return wrap.Item, nil
	}
	var items []AuthCode
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("解析 3DS 验证码失败: %w", err)
	}
	return items, nil
}

func jsonTaskID(taskID string) any {
	taskID = strings.TrimSpace(taskID)
	if n, err := strconv.ParseInt(taskID, 10, 64); err == nil {
		return n
	}
	return taskID
}

func (c *Client) taskCards(taskType int, taskID, ram string) ([]Card, string, error) {
	raw, err := c.call(taskDetailPath, map[string]any{
		"task_type": taskType,
		"task_id":   jsonTaskID(taskID),
		"ram":       ram,
	})
	if err != nil {
		return nil, "", err
	}
	var detail struct {
		TaskID     any    `json:"task_id"`
		TaskStatus any    `json:"task_status"`
		Item       string `json:"item"`
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		return nil, "", fmt.Errorf("解析任务详情失败: %w", err)
	}
	status := anyString(detail.TaskStatus)
	if strings.TrimSpace(detail.Item) == "" {
		return nil, status, nil
	}
	plain, err := DecryptItem(detail.Item, ram)
	if err != nil {
		return nil, status, err
	}
	cards, err := parseCards(plain)
	return cards, status, err
}

func parseCards(plain []byte) ([]Card, error) {
	plain = bytes.TrimSpace(plain)
	var cards []Card
	if err := json.Unmarshal(plain, &cards); err == nil && len(cards) > 0 {
		return cards, nil
	}
	var one Card
	if err := json.Unmarshal(plain, &one); err == nil && strings.TrimSpace(one.CardNo) != "" {
		return []Card{one}, nil
	}
	var wrap struct {
		Item []Card `json:"item"`
	}
	if err := json.Unmarshal(plain, &wrap); err == nil && len(wrap.Item) > 0 {
		return wrap.Item, nil
	}
	return nil, fmt.Errorf("解密后的卡数据无法解析")
}

func (c *Client) call(path string, body map[string]any) (json.RawMessage, error) {
	if body == nil {
		body = map[string]any{}
	}
	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	now := c.now()
	if now.IsZero() {
		now = time.Now()
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	params := map[string]string{
		"app_id":      c.AppID,
		"app_key":     c.AppKey,
		"request_no":  requestNo(now),
		"timestamp":   now.In(loc).Format("2006-01-02 15:04:05"),
		"sign_type":   "RSA2",
		"requestBody": string(reqBody),
	}
	sign, err := Sign(params, c.PrivateKey)
	if err != nil {
		return nil, err
	}
	params["sign"] = sign
	payload, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	url := c.Host + path
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := c.http
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("AmzKeys 请求失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("AmzKeys HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var env struct {
		Code         any             `json:"code"`
		Msg          string          `json:"msg"`
		ResponseBody json.RawMessage `json:"responseBody"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("AmzKeys 响应不是 JSON: %w", err)
	}
	if !okCode(env.Code) {
		msg := strings.TrimSpace(env.Msg)
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		return nil, fmt.Errorf("AmzKeys 失败: %s", msg)
	}
	if len(env.ResponseBody) == 0 || string(env.ResponseBody) == "null" {
		return json.RawMessage("{}"), nil
	}
	return env.ResponseBody, nil
}

func SignPlain(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "sign" || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	return strings.Join(parts, "&")
}

func Sign(params map[string]string, privateKey string) (string, error) {
	key, err := ParsePrivateKey(privateKey)
	if err != nil {
		return "", err
	}
	plain := SignPlain(params)
	sum := sha256.Sum256([]byte(plain))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", fmt.Errorf("RSA2 签名失败: %w", err)
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

func ParsePrivateKey(raw string) (*rsa.PrivateKey, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("私钥为空")
	}
	if block, _ := pem.Decode([]byte(raw)); block != nil {
		return parseRSAKey(block.Bytes)
	}
	compact := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, raw)
	der, err := base64.StdEncoding.DecodeString(compact)
	if err != nil {
		wrapped := "-----BEGIN PRIVATE KEY-----\n" + wrapPEM(compact) + "\n-----END PRIVATE KEY-----"
		if block, _ := pem.Decode([]byte(wrapped)); block != nil {
			return parseRSAKey(block.Bytes)
		}
		return nil, fmt.Errorf("私钥不是 PEM 或 Base64")
	}
	return parseRSAKey(der)
}

func parseRSAKey(der []byte) (*rsa.PrivateKey, error) {
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("私钥不是 RSA")
		}
		return rsaKey, nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	return nil, fmt.Errorf("无法解析 RSA 私钥")
}

func wrapPEM(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && i%64 == 0 {
			b.WriteByte('\n')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func DecryptItem(item, ram string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(item))
	if err != nil {
		return nil, fmt.Errorf("卡数据不是 base64: %w", err)
	}
	key := []byte(ram)
	if len(key) != 16 {
		return nil, fmt.Errorf("AES 密钥须为 16 位")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	iv := []byte(AESIV)
	if len(iv) != aes.BlockSize {
		return nil, fmt.Errorf("AES IV 长度无效")
	}
	out := make([]byte, len(raw))
	cipher.NewCFBDecrypter(block, iv).XORKeyStream(out, raw)
	return out, nil
}

func EncryptItem(plain []byte, ram string) (string, error) {
	key := []byte(ram)
	if len(key) != 16 {
		return "", fmt.Errorf("AES 密钥须为 16 位")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	out := make([]byte, len(plain))
	cipher.NewCFBEncrypter(block, []byte(AESIV)).XORKeyStream(out, plain)
	return base64.StdEncoding.EncodeToString(out), nil
}

func Last4(cardNo string) string {
	d := digitsOnly(cardNo)
	if len(d) < 4 {
		return d
	}
	return d[len(d)-4:]
}

func ExpiryMMYY(validDate string) string {
	s := strings.TrimSpace(validDate)
	s = strings.ReplaceAll(s, "/", "-")
	parts := strings.Split(s, "-")
	if len(parts) >= 2 {
		year := digitsOnly(parts[0])
		month := digitsOnly(parts[1])
		if len(year) == 4 {
			year = year[2:]
		}
		if len(month) == 1 {
			month = "0" + month
		}
		if len(year) >= 2 && len(month) >= 2 {
			return month + " / " + year[len(year)-2:]
		}
	}
	d := digitsOnly(s)
	if len(d) >= 4 {
		if len(d) == 6 { // YYYYMM
			return d[4:6] + " / " + d[2:4]
		}
		return d[:2] + " / " + d[2:4]
	}
	return s
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func requestNo(now time.Time) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return now.Format("060102150405") + hex.EncodeToString(b)
}

func randomRAM() (string, error) {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	out := make([]byte, 16)
	for i, n := range raw {
		out[i] = letters[int(n)%len(letters)]
	}
	return string(out), nil
}

func okCode(code any) bool {
	switch v := code.(type) {
	case float64:
		return successCode(int64(v))
	case json.Number:
		n, _ := v.Int64()
		return successCode(n)
	case string:
		s := strings.TrimSpace(v)
		return s == "10000" || s == "200" || strings.EqualFold(s, "SUCCESS")
	case int:
		return successCode(int64(v))
	case int64:
		return successCode(v)
	default:
		return false
	}
}

func successCode(n int64) bool {
	return n == 10000 || n == 200
}

func TaskStale(startedAt int64) bool {
	if startedAt <= 0 {
		return true
	}
	return time.Since(time.Unix(startedAt, 0)) > taskPollTimeout+time.Minute
}

func anyString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case json.Number:
		return strings.TrimSpace(t.String())
	case int:
		return strconv.Itoa(t)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
