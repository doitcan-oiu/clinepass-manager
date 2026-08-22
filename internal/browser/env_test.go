package browser

import (
	"testing"
)

func TestIsProxyEnv(t *testing.T) {
	if !isProxyEnv("ALL_PROXY") || !isProxyEnv("http_proxy") || !isProxyEnv("HTTPS_PROXY") {
		t.Fatal("proxy keys")
	}
	if isProxyEnv("HOME") || isProxyEnv("DISPLAY") || isProxyEnv("WAYLAND_DISPLAY") {
		t.Fatal("must keep display env")
	}
}

func TestBrowserLaunchEnvStripsProxy(t *testing.T) {
	t.Setenv("ALL_PROXY", "socks5://user:pass@example.com:3000")
	t.Setenv("http_proxy", "http://127.0.0.1:7897")
	t.Setenv("DISPLAY", ":0")
	t.Setenv("HOME", "/tmp/cloak-home")
	env := browserLaunchEnv("")
	if _, ok := env["ALL_PROXY"]; ok {
		t.Fatal("ALL_PROXY should be stripped")
	}
	if _, ok := env["http_proxy"]; ok {
		t.Fatal("http_proxy should be stripped")
	}
	if env["DISPLAY"] != ":0" {
		t.Fatalf("DISPLAY=%q", env["DISPLAY"])
	}
	if env["HOME"] != "/tmp/cloak-home" {
		t.Fatalf("HOME=%q", env["HOME"])
	}
}
