package netproxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	xproxy "golang.org/x/net/proxy"
)

func Parse(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(u.Host) == "" {
		return nil, fmt.Errorf("代理地址无效")
	}
	return u, nil
}

func IsSOCKS(u *url.URL) bool {
	if u == nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "socks", "socks5", "socks5h":
		return true
	default:
		return false
	}
}

func ResolveHTTP(raw string, req *http.Request) (*url.URL, error) {
	u, err := Parse(raw)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return http.ProxyFromEnvironment(req)
	}
	if IsSOCKS(u) {
		return nil, nil
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return u, nil
	default:
		return nil, fmt.Errorf("不支持的代理协议: %s", u.Scheme)
	}
}

func Apply(tr *http.Transport, proxy string) {
	ApplyFunc(tr, func() string { return proxy })
}

func ApplyFunc(tr *http.Transport, get func() string) {
	if tr == nil {
		return
	}
	if get == nil {
		get = func() string { return "" }
	}
	fallback := tr.DialContext
	if fallback == nil {
		d := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
		fallback = d.DialContext
	}
	tr.Proxy = func(req *http.Request) (*url.URL, error) {
		return ResolveHTTP(get(), req)
	}
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return DialContext(ctx, network, addr, get(), fallback)
	}
}

func DialContext(ctx context.Context, network, addr, proxy string, fallback func(context.Context, string, string) (net.Conn, error)) (net.Conn, error) {
	u, err := Parse(proxy)
	if err != nil {
		return nil, err
	}
	if !IsSOCKS(u) {
		if fallback == nil {
			d := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
			fallback = d.DialContext
		}
		return fallback(ctx, network, addr)
	}
	d, err := socksDialer(u)
	if err != nil {
		return nil, err
	}
	if cd, ok := d.(xproxy.ContextDialer); ok {
		return cd.DialContext(ctx, network, addr)
	}
	return d.Dial(network, addr)
}

func socksDialer(u *url.URL) (xproxy.Dialer, error) {
	var auth *xproxy.Auth
	if u.User != nil {
		pwd, _ := u.User.Password()
		auth = &xproxy.Auth{User: u.User.Username(), Password: pwd}
	}
	return xproxy.SOCKS5("tcp", u.Host, auth, &net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second})
}
