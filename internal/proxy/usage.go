package proxy

import (
	"bytes"
	"encoding/json"
	"strings"

	"opencode-go-manager/internal/gomodel"
)

type tokenUsage struct {
	Input      int
	Output     int
	Reasoning  int
	CacheRead  int
	CacheWrite int
	Total      int
}

func (u tokenUsage) withTotal() tokenUsage {
	if u.Total <= 0 {
		u.Total = u.Input + u.Output
	}
	return u
}

func mergeUsage(dst *tokenUsage, src tokenUsage) {
	if src.Input > 0 {
		dst.Input = src.Input
	}
	if src.Output > 0 {
		dst.Output = src.Output
	}
	if src.Reasoning > 0 {
		dst.Reasoning = src.Reasoning
	}
	if src.CacheRead > 0 {
		dst.CacheRead = src.CacheRead
	}
	if src.CacheWrite > 0 {
		dst.CacheWrite = src.CacheWrite
	}
	if src.Total > 0 {
		dst.Total = src.Total
	}
}

type usageParser struct {
	stream bool
	buf    bytes.Buffer
	usage  tokenUsage
}

func newUsageParser(stream bool) *usageParser {
	return &usageParser{stream: stream}
}

func (p *usageParser) Write(b []byte) {
	if p.buf.Len() > 4<<20 {
		return
	}
	p.buf.Write(b)
	if p.stream {
		p.drainSSE()
	}
}

func (p *usageParser) Result() tokenUsage {
	if p.stream {
		p.consumeSSELine(strings.TrimSpace(p.buf.String()))
		return p.usage.withTotal()
	}
	if u, ok := parseUsageJSON(p.buf.Bytes()); ok {
		return u.withTotal()
	}
	return p.usage.withTotal()
}

func (p *usageParser) drainSSE() {
	raw := p.buf.Bytes()
	for {
		i := bytes.IndexByte(raw, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimSpace(string(raw[:i]))
		raw = raw[i+1:]
		p.consumeSSELine(line)
	}
	p.buf.Reset()
	p.buf.Write(raw)
}

func (p *usageParser) consumeSSELine(line string) {
	line = strings.TrimSpace(line)
	if line == "" || line == "[DONE]" || line == "data: [DONE]" {
		return
	}
	if strings.HasPrefix(line, "data:") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	}
	if line == "" || line == "[DONE]" || !strings.HasPrefix(line, "{") {
		return
	}
	if u, ok := parseUsageJSON([]byte(line)); ok {
		mergeUsage(&p.usage, u)
	}
}

func parseUsageJSON(raw []byte) (tokenUsage, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return tokenUsage{}, false
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return tokenUsage{}, false
	}
	u, ok := usageFromMap(obj)
	if !ok {
		return tokenUsage{}, false
	}
	return u, true
}

func usageFromMap(obj map[string]any) (tokenUsage, bool) {
	if obj == nil {
		return tokenUsage{}, false
	}
	if u, ok := asMap(obj["usage"]); ok {
		out := tokensFromUsageMap(u)
		if !out.empty() {
			return out, true
		}
	}
	if data, ok := asMap(obj["data"]); ok {
		if u, ok := asMap(data["usage"]); ok {
			out := tokensFromUsageMap(u)
			if !out.empty() {
				return out, true
			}
		}
	}
	if msg, ok := asMap(obj["message"]); ok {
		if u, ok := asMap(msg["usage"]); ok {
			out := tokensFromUsageMap(u)
			if !out.empty() {
				return out, true
			}
		}
	}
	if response, ok := asMap(obj["response"]); ok {
		if u, ok := asMap(response["usage"]); ok {
			out := tokensFromUsageMap(u)
			if !out.empty() {
				return out, true
			}
		}
	}
	out := tokensFromUsageMap(obj)
	if out.empty() {
		return tokenUsage{}, false
	}
	return out, true
}

func (u tokenUsage) empty() bool {
	return u.Input == 0 && u.Output == 0 && u.Reasoning == 0 && u.CacheRead == 0 && u.CacheWrite == 0 && u.Total == 0
}

func tokensFromUsageMap(u map[string]any) tokenUsage {
	out := tokenUsage{
		Input:      intField(u, "input_tokens", "prompt_tokens"),
		Output:     intField(u, "output_tokens", "completion_tokens"),
		Reasoning:  intField(u, "reasoning_tokens"),
		CacheRead:  intField(u, "cache_read_input_tokens", "cached_tokens"),
		CacheWrite: intField(u, "cache_creation_input_tokens", "cache_write_tokens", "cache_creation_tokens"),
		Total:      intField(u, "total_tokens"),
	}
	if details, ok := asMap(u["prompt_tokens_details"]); ok {
		if n := intField(details, "cached_tokens"); n > 0 {
			out.CacheRead = n
		}
	}
	if details, ok := asMap(u["input_tokens_details"]); ok {
		if n := intField(details, "cached_tokens"); n > 0 {
			out.CacheRead = n
		}
	}
	if details, ok := asMap(u["completion_tokens_details"]); ok {
		if n := intField(details, "reasoning_tokens"); n > 0 {
			out.Reasoning = n
		}
	}
	if details, ok := asMap(u["output_tokens_details"]); ok {
		if n := intField(details, "reasoning_tokens"); n > 0 {
			out.Reasoning = n
		}
	}
	return out
}

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func intField(m map[string]any, keys ...string) int {
	for _, k := range keys {
		if n := asInt(m[k]); n != 0 {
			return n
		}
	}
	return 0
}

func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	case string:
		n = strings.TrimSpace(n)
		if n == "" {
			return 0
		}
		var x int
		for _, c := range n {
			if c < '0' || c > '9' {
				return 0
			}
			x = x*10 + int(c-'0')
		}
		return x
	default:
		return 0
	}
}

func requestStream(body []byte) bool {
	var payload struct {
		Stream bool `json:"stream"`
	}
	_ = json.Unmarshal(body, &payload)
	return payload.Stream
}

func apiFormat(reqPath, modelID string) string {
	p := gomodel.UpstreamPath(reqPath, modelID)
	switch {
	case strings.HasSuffix(p, "/responses"):
		return "openai/responses"
	case strings.HasSuffix(p, "/messages"):
		return "anthropic/messages"
	default:
		return "openai/chat_completions"
	}
}
