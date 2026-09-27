package browser

import (
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestNeedsSOCKSAuthRelay(t *testing.T) {
	if !needsSOCKSAuthRelay("socks5://user:pass@us.example.com:3000") {
		t.Fatal("socks5 with userinfo should relay")
	}
	if !needsSOCKSAuthRelay("socks5h://jbi558732-region-US-sid-abc-t-5:secret@us.1024proxy.io:3000") {
		t.Fatal("socks5h with userinfo should relay")
	}
	if needsSOCKSAuthRelay("socks5://127.0.0.1:1080") {
		t.Fatal("socks5 without auth should use chrome directly")
	}
	if needsSOCKSAuthRelay("http://user:pass@host:8080") {
		t.Fatal("http proxy auth is supported by chrome")
	}
	if needsSOCKSAuthRelay("") {
		t.Fatal("empty")
	}
}

func TestParseProxyKeepsHTTPAuth(t *testing.T) {
	p := parseProxy("http://u:p@host:8080")
	if p == nil || p.Server != "http://host:8080" || p.Username == nil || *p.Username != "u" {
		t.Fatalf("%+v", p)
	}
}

func TestRelayCONNECT(t *testing.T) {
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = target.Close() })
	go func() {
		c, err := target.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 4)
		if _, err := io.ReadFull(c, buf); err != nil {
			return
		}
		_, _ = c.Write([]byte("PONG"))
	}()

	r, err := startRelay(&net.Dialer{Timeout: 3 * time.Second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })

	conn, err := net.DialTimeout("tcp", r.ln.Addr().String(), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	req := "CONNECT " + target.Addr().String() + " HTTP/1.1\r\nHost: " + target.Addr().String() + "\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 128)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); !containsStatus200(got) {
		t.Fatalf("handshake %q", got)
	}
	if _, err := conn.Write([]byte("PING")); err != nil {
		t.Fatal(err)
	}
	n, err = conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "PONG" {
		t.Fatalf("got %q", buf[:n])
	}
}

func TestRelayHTTPForward(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	srv := &http.Server{Handler: mux}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close(); _ = ln.Close() })
	go srv.Serve(ln)

	r, err := startRelay(&net.Dialer{Timeout: 3 * time.Second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })

	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(mustURL(r.BrowserURL()))},
		Timeout:   3 * time.Second,
	}
	resp, err := client.Get("http://" + ln.Addr().String() + "/ping")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "ok" {
		t.Fatalf("body %q", b)
	}
}

func containsStatus200(s string) bool {
	return len(s) >= 12 && (s[:12] == "HTTP/1.1 200" || s[:12] == "HTTP/1.0 200")
}

func mustURL(raw string) *url.URL {
	u, err := parseProxyURL(raw)
	if err != nil || u == nil {
		panic(raw)
	}
	return u
}
