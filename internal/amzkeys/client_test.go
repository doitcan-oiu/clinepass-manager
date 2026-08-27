package amzkeys

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

func TestSignPlainSortsKeys(t *testing.T) {
	plain := SignPlain(map[string]string{
		"app_id":      "2021840001691636",
		"app_key":     "29fe24756cebc88bf5be05df104cbe449e8f4819",
		"request_no":  "YM123456781634113044",
		"timestamp":   "2021-10-13 16:17:24",
		"sign_type":   "RSA2",
		"requestBody": `{"card_no":"1234560239768816"}`,
		"sign":        "should-skip",
	})
	want := `app_id=2021840001691636&app_key=29fe24756cebc88bf5be05df104cbe449e8f4819&requestBody={"card_no":"1234560239768816"}&request_no=YM123456781634113044&sign_type=RSA2&timestamp=2021-10-13 16:17:24`
	if plain != want {
		t.Fatalf("plain=%q", plain)
	}
}

func TestSignAndParseKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	b64 := base64.StdEncoding.EncodeToString(der)
	if _, err := ParsePrivateKey(pemKey); err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePrivateKey(b64); err != nil {
		t.Fatal(err)
	}
	sig, err := Sign(map[string]string{
		"app_id":      "1",
		"app_key":     "k",
		"request_no":  "YM123456781634113044",
		"timestamp":   "2021-10-13 16:17:24",
		"sign_type":   "RSA2",
		"requestBody": "{}",
	}, pemKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := base64.StdEncoding.DecodeString(sig); err != nil || sig == "" {
		t.Fatalf("sign=%q %v", sig, err)
	}
}

func TestDecryptItem(t *testing.T) {
	ram := "abcdefghijklmnop"
	plain := []byte(`[{"card_no":"4111111111111111","cvv":"123","valid_date":"2028-12"}]`)
	item, err := EncryptItem(plain, ram)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptItem(item, ram)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(plain) {
		t.Fatalf("got=%s", got)
	}
	cards, err := parseCards(got)
	if err != nil || len(cards) != 1 || cards[0].CardNo != "4111111111111111" {
		t.Fatalf("%+v %v", cards, err)
	}
}

func TestExpiryAndLast4(t *testing.T) {
	if got := ExpiryMMYY("2028-12"); got != "12 / 28" {
		t.Fatalf("year-month=%q", got)
	}
	if got := ExpiryMMYY("28-06"); got != "06 / 28" {
		t.Fatalf("yy-mm=%q", got)
	}
	if got := Last4("4111 1111 1111 1111"); got != "1111" {
		t.Fatalf("last4=%q", got)
	}
}

func TestReady(t *testing.T) {
	if err := Ready("", "", "", "", DefaultCardType, DefaultAmount); err == nil {
		t.Fatal("not saved")
	}
	if err := Ready(DefaultHost, "", "", "", DefaultCardType, DefaultAmount); err != nil {
		t.Fatal(err)
	}
	if err := Ready(ProductionHost, "", "k", "p", DefaultCardType, DefaultAmount); err == nil {
		t.Fatal("empty app id")
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	if err := Ready(ProductionHost, "id", "key", pemKey, DefaultCardType, DefaultAmount); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(Ready(ProductionHost, "id", "key", "not-a-key", DefaultCardType, DefaultAmount).Error(), "私钥") {
		t.Fatal("bad key")
	}
}

func TestResolveUsesOfficialSandbox(t *testing.T) {
	host, appID, appKey, priv := Resolve(DefaultHost, "prod-id", "prod-key", "prod-priv")
	if host != DefaultHost || appID != TestAppID || appKey != TestAppKey || priv != TestPrivateKey {
		t.Fatalf("%s %s %s", host, appID, appKey)
	}
	if _, err := ParsePrivateKey(TestPrivateKey); err != nil {
		t.Fatal(err)
	}
	if IsTestHost(ProductionHost) {
		t.Fatal("production")
	}
}

func TestParseCardsNumericRequestID(t *testing.T) {
	plain := []byte(`[{"card_type":467845,"request_id":46754741,"card_no":"4678454663786647","cvv":"525","valid_date":"2026-08","open_card_amount":"20.00","create_time":"2026-08-27 15:27:12","currency":"USD"}]`)
	cards, err := parseCards(plain)
	if err != nil || len(cards) != 1 {
		t.Fatalf("%+v %v", cards, err)
	}
	if cards[0].CardNo != "4678454663786647" || cards[0].CVV != "525" || string(cards[0].RequestID) != "46754741" {
		t.Fatalf("%+v", cards[0])
	}
	if cards[0].OpenCardAmount != 20 {
		t.Fatalf("amount=%v", cards[0].OpenCardAmount)
	}
}

func TestMaxPays(t *testing.T) {
	if got := MaxPays(20); got != 3 {
		t.Fatalf("20 -> %d", got)
	}
	if got := MaxPays(15.9); got != 3 {
		t.Fatalf("15.9 -> %d", got)
	}
	if got := MaxPays(10); got != 1 {
		t.Fatalf("10 -> %d", got)
	}
	if got := MaxPays(0); got != 3 {
		t.Fatalf("default -> %d", got)
	}
	if got := RemainingPays(20, 2); got != 1 {
		t.Fatalf("remaining=%d", got)
	}
	if got := RemainingPays(20, 3); got != 0 {
		t.Fatalf("exhausted remaining=%d", got)
	}
}

func TestTaskStale(t *testing.T) {
	if !TaskStale(0) {
		t.Fatal("zero")
	}
	if TaskStale(time.Now().Unix()) {
		t.Fatal("fresh")
	}
}

func TestOkCode(t *testing.T) {
	if !okCode(float64(10000)) || !okCode("10000") || !okCode(200) || !okCode("200") {
		t.Fatal("ok")
	}
	if okCode(float64(40001)) {
		t.Fatal("fail")
	}
}
