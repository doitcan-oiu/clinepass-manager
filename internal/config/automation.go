package config

// Pure settings validation shared by the manager and auto. This file neither
// contacts the card provider nor imports the automation runtime.
import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	AmzKeysDefaultHost     = "https://testapi.amzkeys.com"
	AmzKeysDefaultCardType = 467845
	AmzKeysDefaultAmount   = 20
)

func IsAmzKeysTestHost(host string) bool {
	h := strings.ToLower(strings.TrimRight(strings.TrimSpace(host), "/"))
	return h == "" || strings.Contains(h, "testapi.amzkeys.com")
}

func AmzKeysReady(host, appID, appKey, privateKey string, cardType int, amount float64) error {
	host, appID = strings.TrimSpace(host), strings.TrimSpace(appID)
	appKey, privateKey = strings.TrimSpace(appKey), strings.TrimSpace(privateKey)
	if IsAmzKeysTestHost(host) {
		if host == "" && appID == "" {
			return fmt.Errorf("请先在设置里保存 amzkeys卡台")
		}
		// Auto substitutes its shipped, validated official sandbox credentials.
		// Explicit sandbox selection needs no user-supplied secrets.
	} else {
		if appID == "" {
			return fmt.Errorf("缺少 AppID")
		}
		if appKey == "" {
			return fmt.Errorf("缺少 AppKey")
		}
		if privateKey == "" {
			return fmt.Errorf("生产环境请填写商务给的 RSA2 私钥，测试环境不用自己生成")
		}
		if _, err := ParseRSAPrivateKey(privateKey); err != nil {
			return fmt.Errorf("RSA2 私钥无效: %w", err)
		}
	}
	if cardType <= 0 {
		return fmt.Errorf("缺少卡段")
	}
	if amount <= 0 {
		return fmt.Errorf("开卡金额须大于 0")
	}
	return nil
}

// ResolveCloakLicense preserves the browser's explicit-key then local-file
// lookup. CLOAKBROWSER_LICENSE_KEY is already loaded into Config by Load.
func ResolveCloakLicense(explicit string) string {
	if key := strings.TrimSpace(explicit); key != "" {
		return key
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(home, ".cloakbrowser", "license.key"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func ParseRSAPrivateKey(raw string) (*rsa.PrivateKey, error) {
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
