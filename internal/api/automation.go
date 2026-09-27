package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"opencode-go-manager/internal/model"
)

var autoTransport = &http.Transport{
	Proxy:               nil,
	DialContext:         (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 10 * time.Minute, IdleConnTimeout: 90 * time.Second,
}

var autoClient = &http.Client{
	Transport:     autoTransport,
	Timeout:       10 * time.Minute,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
}

func normalizeAutoURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", errors.New("Auto 地址须为 http:// 或 https:// 服务地址，不能包含用户名、密码、查询参数或片段")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func (s *Server) autoConnection() (model.AutoConnection, error) {
	if s.store != nil {
		c, err := s.store.GetAutoConnection()
		if err == nil {
			return c, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return model.AutoConnection{}, errors.New("读取 Auto 连接设置失败")
		}
	}
	u, err := normalizeAutoURL(s.cfg.AutoURL)
	if err != nil {
		return model.AutoConnection{}, err
	}
	return model.AutoConnection{URL: u, Token: strings.TrimSpace(s.cfg.AutoToken)}, nil
}

func publicAutoConnection(c model.AutoConnection) map[string]any {
	return map[string]any{"url": c.URL, "token_configured": c.Token != "", "configured": c.URL != "" && c.Token != ""}
}

func (s *Server) getAutoConnection(w http.ResponseWriter, r *http.Request) {
	c, err := s.autoConnection()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, publicAutoConnection(c))
}

type autoConnectionInput struct {
	URL        *string `json:"url"`
	Token      *string `json:"token"`
	ClearToken bool    `json:"clear_token"`
}

func (in autoConnectionInput) apply(c model.AutoConnection) (model.AutoConnection, error) {
	if in.URL != nil {
		var err error
		c.URL, err = normalizeAutoURL(*in.URL)
		if err != nil {
			return c, err
		}
	}
	if in.Token != nil && strings.TrimSpace(*in.Token) != "" {
		c.Token = strings.TrimSpace(*in.Token)
	}
	if in.ClearToken {
		c.Token = ""
	}
	if strings.ContainsAny(c.Token, "\r\n") || len(c.Token) > 4096 {
		return c, errors.New("Auto 访问令牌无效")
	}
	return c, nil
}

func (s *Server) patchAutoConnection(w http.ResponseWriter, r *http.Request) {
	var in autoConnectionInput
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	d.DisallowUnknownFields()
	if err := d.Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "连接设置 JSON 无效")
		return
	}
	c, err := s.autoConnection()
	if err == nil {
		c, err = in.apply(c)
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.SaveAutoConnection(c); err != nil {
		writeErr(w, http.StatusInternalServerError, "保存 Auto 连接设置失败")
		return
	}
	writeJSON(w, http.StatusOK, publicAutoConnection(c))
}

func autoConnectionReady(c model.AutoConnection) error {
	if c.URL == "" {
		return errors.New("尚未配置 Auto 服务地址，请在设置中的 Auto 连接填写连接信息")
	}
	if c.Token == "" {
		return errors.New("尚未配置 Auto 访问令牌，请填写 Auto 服务器的连接密钥")
	}
	return nil
}

func autoRequestConnection(ctx context.Context, method, path string, body io.Reader, c model.AutoConnection) (*http.Response, error) {
	if err := autoConnectionReady(c); err != nil {
		return nil, err
	}
	base, err := normalizeAutoURL(c.URL)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return nil, errors.New("Auto 请求路径无效")
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, body)
	if err != nil {
		return nil, errors.New("无法创建 Auto 请求，请检查连接设置")
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := autoClient.Do(req)
	if err != nil {
		return nil, autoConnectionError(err)
	}
	return res, nil
}

// autoRequest uses the saved connection at request time. Multi-request workflows
// should snapshot autoConnection and call autoRequestConnection with that value.
func (s *Server) autoRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	c, err := s.autoConnection()
	if err != nil {
		return nil, err
	}
	return autoRequestConnection(ctx, method, path, body, c)
}

func autoConnectionError(err error) error {
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &netErr) && netErr.Timeout() {
		return errors.New("Auto 服务连接超时，请检查服务地址、网络和防火墙")
	}
	return errors.New("自动化服务未连接，请检查 Auto 服务地址、服务器状态和网络")
}

func autoHTTPError(code int) string {
	if code == http.StatusUnauthorized || code == http.StatusForbidden {
		return "Auto 身份验证失败，请检查访问令牌是否与 Auto 服务器的连接密钥一致"
	}
	if code >= 300 && code < 400 {
		return "Auto 服务返回重定向，请直接配置最终的服务地址"
	}
	return "Auto 服务返回异常，请检查 Auto 服务器状态及版本"
}

func (s *Server) forwardAuto(w http.ResponseWriter, r *http.Request) {
	c, err := s.autoConnection()
	if err == nil {
		err = autoConnectionReady(c)
	}
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	targetURL, err := normalizeAutoURL(c.URL)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	target, _ := url.Parse(targetURL)
	p := &httputil.ReverseProxy{
		Transport:     autoTransport,
		FlushInterval: -1,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			// Only protocol headers are forwarded; browser cookies, credentials,
			// origin, and forwarding headers must stay with the manager.
			pr.Out.Header = make(http.Header)
			for _, key := range []string{"Accept", "Content-Type", "Last-Event-ID"} {
				if values := pr.In.Header.Values(key); len(values) != 0 {
					pr.Out.Header[key] = values
				}
			}
			pr.Out.Header.Set("Authorization", "Bearer "+c.Token)
		},
		ModifyResponse: func(res *http.Response) error {
			res.Header.Del("Set-Cookie")
			if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden || res.StatusCode >= 300 && res.StatusCode < 400 {
				_ = res.Body.Close()
				payload, _ := json.Marshal(map[string]string{"error": autoHTTPError(res.StatusCode)})
				res.StatusCode = http.StatusBadGateway
				res.Header.Del("Location")
				res.Header.Del("WWW-Authenticate")
				res.Header.Set("Content-Type", "application/json; charset=utf-8")
				res.Header.Del("Content-Length")
				res.ContentLength = int64(len(payload))
				res.Body = io.NopCloser(bytes.NewReader(payload))
			} else if res.StatusCode >= 400 {
				payload, e := io.ReadAll(io.LimitReader(res.Body, 1024*1024))
				_ = res.Body.Close()
				if e != nil {
					return errors.New("Auto 响应读取失败")
				}
				payload = bytes.ReplaceAll(payload, []byte(c.Token), []byte("[redacted]"))
				res.Body = io.NopCloser(bytes.NewReader(payload))
				res.ContentLength = int64(len(payload))
				res.Header.Del("Content-Length")
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			writeErr(w, http.StatusServiceUnavailable, autoConnectionError(err).Error())
		},
	}
	p.ServeHTTP(w, r)
}

func (s *Server) forwardAutoConfig(w http.ResponseWriter, r *http.Request) {
	r = r.Clone(r.Context())
	r.URL.Path = "/api/config"
	r.URL.RawPath = ""
	s.forwardAuto(w, r)
}

func (s *Server) forwardAutoAccount(w http.ResponseWriter, r *http.Request) {
	r = r.Clone(r.Context())
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api/auto")
	r.URL.Path = "/api" + r.URL.Path
	r.URL.RawPath = ""
	s.forwardAuto(w, r)
}

func checkAutoConnection(ctx context.Context, c model.AutoConnection) map[string]any {
	out := map[string]any{"connected": false, "url": c.URL}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	res, err := autoRequestConnection(ctx, http.MethodGet, "/api/health", nil, c)
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		out["error"] = autoHTTPError(res.StatusCode)
		return out
	}
	var health struct {
		OK              bool   `json:"ok"`
		Service         string `json:"service"`
		ProtocolVersion int    `json:"protocol_version"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&health) != nil || !health.OK || health.Service != "auto" || health.ProtocolVersion != 1 {
		out["error"] = "该地址不是兼容的 Auto 服务，请检查地址和 Auto 版本"
		return out
	}
	out["connected"] = true
	out["protocol_version"] = health.ProtocolVersion
	return out
}

func (s *Server) autoStatus(w http.ResponseWriter, r *http.Request) {
	c, err := s.autoConnection()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"connected": false, "url": "", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, checkAutoConnection(r.Context(), c))
}

func (s *Server) testAutoConnection(w http.ResponseWriter, r *http.Request) {
	var in autoConnectionInput
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	d.DisallowUnknownFields()
	if err := d.Decode(&in); err != nil && !errors.Is(err, io.EOF) {
		writeErr(w, http.StatusBadRequest, "连接设置 JSON 无效")
		return
	}
	c, err := s.autoConnection()
	if err == nil {
		c, err = in.apply(c)
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, checkAutoConnection(r.Context(), c))
}
