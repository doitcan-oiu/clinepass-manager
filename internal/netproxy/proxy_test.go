package netproxy

import (
	"net/http"
	"testing"
)

func TestParseAndSOCKS(t *testing.T) {
	u, err := Parse("socks5://user:pass@127.0.0.1:1080")
	if err != nil || !IsSOCKS(u) || u.Host != "127.0.0.1:1080" {
		t.Fatalf("%+v %v", u, err)
	}
	u, err = Parse("socks5h://127.0.0.1:1080")
	if err != nil || !IsSOCKS(u) {
		t.Fatalf("%+v %v", u, err)
	}
	u, err = Parse("http://127.0.0.1:8080")
	if err != nil || IsSOCKS(u) {
		t.Fatalf("http %+v %v", u, err)
	}
	u, err = Parse("")
	if err != nil || u != nil {
		t.Fatalf("empty %+v %v", u, err)
	}
}

func TestResolveHTTP(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://api.cline.bot/", nil)
	if err != nil {
		t.Fatal(err)
	}
	u, err := ResolveHTTP("socks5://127.0.0.1:1080", req)
	if err != nil || u != nil {
		t.Fatalf("socks should skip HTTP proxy, got %+v %v", u, err)
	}
	u, err = ResolveHTTP("http://user:pass@127.0.0.1:8888", req)
	if err != nil || u == nil || u.Host != "127.0.0.1:8888" {
		t.Fatalf("http %+v %v", u, err)
	}
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("http_proxy", "")
	t.Setenv("https_proxy", "")
	u, err = ResolveHTTP("", req)
	if err != nil || u != nil {
		t.Fatalf("empty %+v %v", u, err)
	}
}
