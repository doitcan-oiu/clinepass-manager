package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"opencode-go-manager/internal/gomodel"
	"opencode-go-manager/internal/model"
	"opencode-go-manager/internal/store"
)

type Handler struct {
	store    *store.Store
	client   *http.Client
	upstream string
}

func New(st *store.Store) *Handler {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          64,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 120 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &Handler{
		store:    st,
		client:   &http.Client{Transport: tr},
		upstream: "https://api.cline.bot",
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v1/models" && (r.Method == http.MethodGet || r.Method == http.MethodOptions) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.models(w)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"只支持 POST"}`, http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		writeProxyErr(w, http.StatusBadRequest, "读取请求失败")
		return
	}
	modelID := requestModel(body)
	stream := requestStream(body)
	rec := model.RequestLog{
		CreatedAt: time.Now().UnixMilli(),
		Model:     gomodel.Canonical(modelID),
		APIFormat: apiFormat(r.URL.Path, modelID),
		Stream:    stream,
		Status:    "processing",
	}
	if id, err := h.store.InsertRequestLog(rec); err == nil {
		rec.ID = id
	}
	started := time.Now()
	state := logResult{status: "error", httpStatus: http.StatusServiceUnavailable, err: "没有可用账号"}
	defer func() {
		h.finishLog(&rec, started, state)
	}()

	list, err := h.pool()
	if err != nil {
		state.err = err.Error()
		writeProxyErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	path := gomodel.UpstreamPath(r.URL.Path, modelID)
	body = rewriteModel(body, gomodel.Canonical(modelID))
	tried := map[string]bool{}
	var lastStatus int
	var lastBody []byte
	var lastAcc model.PoolAccount
	for i := 0; i < h.maxAttempts(); i++ {
		a, ok := lb.reserve(list, modelID, tried)
		if !ok {
			break
		}
		if i > 0 {
			state.retries++
		}
		lastAcc = a
		state.acc = a
		keyID := accountID(a)
		tried[keyID] = true
		resp, ferr := h.do(r, path, a.APIKey, body)
		if ferr != nil {
			lb.end(keyID)
			lastStatus = http.StatusBadGateway
			lastBody, _ = json.Marshal(map[string]any{"error": ferr.Error()})
			state.httpStatus = lastStatus
			state.err = formatLogError(lastStatus, lastBody)
			continue
		}
		if retryableStatus(resp.StatusCode) {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			lb.end(keyID)
			if d := cooldownFor(a, resp.StatusCode, resp.Header); d > 0 {
				lb.cooldown(keyID, d)
			}
			lastStatus, lastBody = resp.StatusCode, b
			state.httpStatus = lastStatus
			state.err = formatLogError(resp.StatusCode, b)
			continue
		}
		if resp.StatusCode >= 400 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			lb.end(keyID)
			copyHeader(w.Header(), resp.Header)
			if w.Header().Get("Content-Type") == "" {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
			}
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(b)
			if u, ok := parseUsageJSON(b); ok {
				state.usage = u
			}
			state.httpStatus = resp.StatusCode
			state.status = "error"
			state.err = formatLogError(resp.StatusCode, b)
			return
		}
		copyHeader(w.Header(), resp.Header)
		w.Header().Del("Content-Length")
		w.WriteHeader(resp.StatusCode)
		sse := stream || strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "event-stream")
		usage, firstAt, _ := copyAndParse(w, resp.Body, sse)
		resp.Body.Close()
		lb.end(keyID)
		if !firstAt.IsZero() {
			state.ttft = firstAt.Sub(started)
		}
		state.usage = usage
		state.httpStatus = resp.StatusCode
		state.status = "completed"
		state.err = ""
		return
	}
	if lastStatus == 0 {
		state.err = unavailableReason(list, time.Now())
		writeProxyErr(w, http.StatusServiceUnavailable, state.err)
		return
	}
	state.acc = lastAcc
	state.httpStatus = lastStatus
	if state.err == "" {
		state.err = formatLogError(lastStatus, lastBody)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(lastStatus)
	_, _ = w.Write(lastBody)
}

func (h *Handler) maxAttempts() int {
	retries := defaultMaxRetries
	if st, err := h.store.GetSettings(); err == nil {
		retries = st.MaxRetries
	}
	return maxAttemptsFromSettings(retries)
}

func (h *Handler) pool() ([]model.PoolAccount, error) {
	_, _ = h.store.DeleteExpiredAccounts(0)
	return h.store.ListPoolAccounts("", 10000, 0)
}

func (h *Handler) models(w http.ResponseWriter) {
	type item struct {
		ID      string  `json:"id"`
		Object  string  `json:"object"`
		OwnedBy string  `json:"owned_by"`
		Limit   float64 `json:"limit_usd"`
	}
	data := []item{}
	for _, m := range gomodel.All() {
		data = append(data, item{ID: m.ID, Object: "model", OwnedBy: "cline-pass", Limit: 0})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

func (h *Handler) do(orig *http.Request, path, apiKey string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(orig.Context(), http.MethodPost, strings.TrimRight(h.upstream, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if ct := orig.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	} else {
		req.Header.Set("Content-Type", "application/json")
	}
	if v := orig.Header.Get("Accept"); v != "" {
		req.Header.Set("Accept", v)
	}
	if v := orig.Header.Get("Anthropic-Version"); v != "" {
		req.Header.Set("Anthropic-Version", v)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	return h.client.Do(req)
}

func requestModel(body []byte) string {
	var payload struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &payload)
	return payload.Model
}

func rewriteModel(body []byte, canonical string) []byte {
	if strings.TrimSpace(canonical) == "" {
		return body
	}
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return body
	}
	payload["model"] = canonical
	out, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return out
}

func copyHeader(dst, src http.Header) {
	for k, vs := range src {
		switch strings.ToLower(k) {
		case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailers", "transfer-encoding", "upgrade":
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

func writeProxyErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": map[string]any{"message": msg, "type": "unavailable"}})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

type logResult struct {
	acc        model.PoolAccount
	status     string
	httpStatus int
	usage      tokenUsage
	retries    int
	err        string
	ttft       time.Duration
}

func (h *Handler) finishLog(rec *model.RequestLog, started time.Time, state logResult) {
	if rec == nil || rec.ID <= 0 {
		return
	}
	rec.AccountID = state.acc.ID
	rec.AccountEmail = state.acc.Email
	rec.Status = state.status
	rec.HTTPStatus = state.httpStatus
	rec.InputTokens = state.usage.Input
	rec.OutputTokens = state.usage.Output
	rec.ReasoningTokens = state.usage.Reasoning
	rec.CacheRead = state.usage.CacheRead
	rec.CacheWrite = state.usage.CacheWrite
	rec.TotalTokens = state.usage.withTotal().Total
	rec.DurationMS = int(time.Since(started).Milliseconds())
	rec.TTFTMS = int(state.ttft.Milliseconds())
	rec.Retries = state.retries
	rec.Error = state.err
	_ = h.store.UpdateRequestLog(*rec)
}

func copyAndParse(w http.ResponseWriter, src io.Reader, stream bool) (tokenUsage, time.Time, error) {
	var first time.Time
	parse := newUsageParser(stream)
	flusher, _ := w.(http.Flusher)
	if !stream {
		raw, err := io.ReadAll(src)
		if len(raw) > 0 {
			first = time.Now()
			parse.Write(raw)
			_, _ = w.Write(unwrapClineEnvelope(raw))
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil && err != io.EOF {
			return parse.Result(), first, err
		}
		return parse.Result(), first, nil
	}
	uw := &sseUnwrapper{}
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if first.IsZero() {
				first = time.Now()
			}
			parse.Write(buf[:n])
			if out := uw.Transform(buf[:n]); len(out) > 0 {
				_, _ = w.Write(out)
				if flusher != nil {
					flusher.Flush()
				}
			}
		}
		if err != nil {
			if rest := uw.Flush(); len(rest) > 0 {
				_, _ = w.Write(rest)
				if flusher != nil {
					flusher.Flush()
				}
			}
			if err == io.EOF {
				return parse.Result(), first, nil
			}
			return parse.Result(), first, err
		}
	}
}

func formatLogError(status int, body []byte) string {
	msg := strings.TrimSpace(errorMessage(body, http.StatusText(status)))
	if msg == "" {
		if status > 0 {
			return fmt.Sprintf("HTTP %d", status)
		}
		return "请求失败"
	}
	if status > 0 {
		return fmt.Sprintf("HTTP %d: %s", status, msg)
	}
	return msg
}

func errorMessage(body []byte, fallback string) string {
	if m := extractJSONError(body); m != "" {
		return m
	}
	s := strings.TrimSpace(string(body))
	if s != "" {
		if len(s) > 400 {
			return s[:400]
		}
		return s
	}
	return fallback
}

func extractJSONError(body []byte) string {
	var wrap map[string]any
	if json.Unmarshal(body, &wrap) != nil {
		return ""
	}
	switch e := wrap["error"].(type) {
	case string:
		if s := strings.TrimSpace(e); s != "" {
			return s
		}
	case map[string]any:
		if m := stringField(e, "message", "msg", "detail"); m != "" {
			return m
		}
	}
	return stringField(wrap, "message", "detail", "msg")
}

func stringField(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok {
			if t := strings.TrimSpace(s); t != "" {
				return t
			}
		}
	}
	return ""
}
