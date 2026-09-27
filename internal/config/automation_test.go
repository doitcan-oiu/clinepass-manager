package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestAmzKeysReadinessSandboxAndProduction(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	validKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	for _, tc := range []struct {
		name, host, id, key, private string
		cardType                     int
		amount                       float64
		want                         bool
	}{
		{"not saved", "", "", "", "", AmzKeysDefaultCardType, 20, false},
		{"sandbox blank secrets", AmzKeysDefaultHost, "", "", "", AmzKeysDefaultCardType, 20, true},
		{"sandbox overrides supplied secrets", AmzKeysDefaultHost, "id", "key", "invalid", AmzKeysDefaultCardType, 20, true},
		{"default sandbox explicitly saved id", "", "id", "", "", AmzKeysDefaultCardType, 20, true},
		{"production invalid RSA", "https://ymapi.amzkeys.com:15970", "id", "key", "invalid", AmzKeysDefaultCardType, 20, false},
		{"production missing id", "https://ymapi.amzkeys.com:15970", "", "key", validKey, AmzKeysDefaultCardType, 20, false},
		{"production valid PEM", "https://ymapi.amzkeys.com:15970", "id", "key", validKey, AmzKeysDefaultCardType, 20, true},
		{"production valid base64", "https://ymapi.amzkeys.com:15970", "id", "key", base64.StdEncoding.EncodeToString(der), AmzKeysDefaultCardType, 20, true},
		{"sandbox missing card", AmzKeysDefaultHost, "", "", "", 0, 20, false},
		{"sandbox invalid amount", AmzKeysDefaultHost, "", "", "", AmzKeysDefaultCardType, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := AmzKeysReady(tc.host, tc.id, tc.key, tc.private, tc.cardType, tc.amount)
			if (err == nil) != tc.want {
				t.Fatalf("ready=%v want=%v error=%v", err == nil, tc.want, err)
			}
		})
	}
}

func TestResolveCloakLicenseFileFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.MkdirAll(filepath.Join(home, ".cloakbrowser"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".cloakbrowser", "license.key"), []byte("  local-license\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ResolveCloakLicense(""); got != "local-license" {
		t.Fatalf("file fallback=%q", got)
	}
	if got := ResolveCloakLicense(" explicit-license "); got != "explicit-license" {
		t.Fatalf("explicit precedence=%q", got)
	}
}
