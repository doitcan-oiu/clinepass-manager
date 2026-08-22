package browser

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	xproxy "golang.org/x/net/proxy"
)

type Relay struct {
	ln     net.Listener
	dialer xproxy.Dialer
	close  sync.Once
}

func needsSOCKSAuthRelay(raw string) bool {
	u, err := parseProxyURL(raw)
	if err != nil || u == nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "socks", "socks5", "socks5h":
	default:
		return false
	}
	if u.User == nil {
		return false
	}
	if u.User.Username() != "" {
		return true
	}
	_, hasPass := u.User.Password()
	return hasPass
}

func parseProxyURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	return url.Parse(raw)
}

func socksDialer(raw string) (xproxy.Dialer, error) {
	u, err := parseProxyURL(raw)
	if err != nil {
		return nil, err
	}
	if u == nil || strings.TrimSpace(u.Host) == "" {
		return nil, fmt.Errorf("代理地址无效")
	}
	var auth *xproxy.Auth
	if u.User != nil {
		pwd, _ := u.User.Password()
		auth = &xproxy.Auth{User: u.User.Username(), Password: pwd}
	}
	return xproxy.SOCKS5("tcp", u.Host, auth, &net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second})
}

func StartSOCKSRelay(upstream string, logf func(string, ...any)) (*Relay, error) {
	dialer, err := socksDialer(upstream)
	if err != nil {
		return nil, err
	}
	return startRelay(dialer, logf)
}

func startRelay(dialer xproxy.Dialer, logf func(string, ...any)) (*Relay, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("监听本地中继失败: %w", err)
	}
	r := &Relay{ln: ln, dialer: dialer}
	go r.serve(logf)
	return r, nil
}

func (r *Relay) BrowserURL() string {
	if r == nil || r.ln == nil {
		return ""
	}
	return "http://" + r.ln.Addr().String()
}

func (r *Relay) Close() error {
	var err error
	r.close.Do(func() {
		if r.ln != nil {
			err = r.ln.Close()
		}
	})
	return err
}

func (r *Relay) serve(logf func(string, ...any)) {
	for {
		conn, err := r.ln.Accept()
		if err != nil {
			return
		}
		go r.handle(conn, logf)
	}
}

func (r *Relay) handle(conn net.Conn, logf func(string, ...any)) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	br := bufio.NewReader(conn)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	target := req.Host
	if target == "" && req.URL != nil {
		target = req.URL.Host
	}
	if target == "" {
		_, _ = conn.Write([]byte("HTTP/1.1 400 Bad Request\r\nConnection: close\r\n\r\n"))
		return
	}
	if !strings.Contains(target, ":") {
		if req.Method == http.MethodConnect {
			target += ":443"
		} else {
			target += ":80"
		}
	}

	dest, err := r.dialer.Dial("tcp", target)
	if err != nil {
		_, _ = conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\n\r\n"))
		logf("本地中继连上游失败 %s: %v", target, err)
		return
	}
	defer dest.Close()
	_ = conn.SetDeadline(time.Time{})

	if req.Method == http.MethodConnect {
		if _, err := conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
			return
		}
		if n := br.Buffered(); n > 0 {
			peek, _ := br.Peek(n)
			if _, err := dest.Write(peek); err != nil {
				return
			}
		}
		tunnel(conn, dest)
		return
	}

	req.RequestURI = ""
	if req.URL != nil && req.URL.Scheme == "" {
		req.URL.Scheme = "http"
	}
	if err := req.Write(dest); err != nil {
		return
	}
	tunnel(conn, dest)
}

func tunnel(a, b net.Conn) {
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(a, b)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(b, a)
		done <- struct{}{}
	}()
	<-done
}
