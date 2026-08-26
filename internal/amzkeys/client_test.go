package amzkeys

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"
	"testing"
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
	if err := Ready(DefaultHost, "", "k", "p", DefaultCardType, DefaultAmount); err == nil {
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
	if err := Ready(DefaultHost, "id", "key", pemKey, DefaultCardType, DefaultAmount); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(Ready(DefaultHost, "id", "key", "not-a-key", DefaultCardType, DefaultAmount).Error(), "私钥") {
		t.Fatal("bad key")
	}
}

func TestOkCode(t *testing.T) {
	if !okCode(float64(10000)) || !okCode("10000") {
		t.Fatal("ok")
	}
	if okCode(float64(40001)) {
		t.Fatal("fail")
	}
}
