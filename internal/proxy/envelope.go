package proxy

import (
	"bytes"
	"encoding/json"
	"strings"
)

func unwrapClineEnvelope(raw []byte) []byte {
	cur := bytes.TrimSpace(raw)
	for i := 0; i < 3; i++ {
		next := unwrapOnce(cur)
		if bytes.Equal(next, cur) {
			return next
		}
		cur = next
	}
	return cur
}

func unwrapOnce(raw []byte) []byte {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return raw
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return raw
	}
	data, ok := obj["data"]
	if !ok {
		return raw
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 || data[0] != '{' || !completionLike(data) {
		return raw
	}
	_, hasSuccess := obj["success"]
	_, hasObject := obj["object"]
	_, hasChoices := obj["choices"]
	if hasSuccess || (!hasObject && !hasChoices) {
		return data
	}
	return raw
}

func completionLike(raw []byte) bool {
	var inner map[string]json.RawMessage
	if json.Unmarshal(raw, &inner) != nil {
		return false
	}
	for _, k := range []string{"object", "choices", "usage", "delta"} {
		if _, ok := inner[k]; ok {
			return true
		}
	}
	return false
}

type providerPolicy struct {
	Mode  string
	Value string
}

func providerPolicyFromSettings(mode, value string) providerPolicy {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "hide", "remove", "delete":
		return providerPolicy{Mode: "hide"}
	case "replace", "set", "fixed":
		return providerPolicy{Mode: "replace", Value: value}
	default:
		return providerPolicy{Mode: "keep"}
	}
}

func applyProviderPolicy(raw []byte, pol providerPolicy) []byte {
	if pol.Mode != "hide" && pol.Mode != "replace" {
		return raw
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return raw
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return raw
	}
	_, hasProvider := obj["provider"]
	if !hasProvider && !completionLike(raw) {
		return raw
	}
	if pol.Mode == "hide" {
		if !hasProvider {
			return raw
		}
		delete(obj, "provider")
	} else {
		b, err := json.Marshal(pol.Value)
		if err != nil {
			return raw
		}
		obj["provider"] = b
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return raw
	}
	return out
}

func normalizeCompletionJSON(raw []byte, pol providerPolicy) []byte {
	out := unwrapClineEnvelope(raw)
	out = liftUsageToTop(out)
	return applyProviderPolicy(out, pol)
}

func hasTopLevelUsage(raw []byte) bool {
	var obj map[string]any
	if json.Unmarshal(bytes.TrimSpace(raw), &obj) != nil {
		return false
	}
	u, ok := asMap(obj["usage"])
	return ok && !tokensFromUsageMap(u).empty()
}

func liftUsageToTop(raw []byte) []byte {
	raw = bytes.TrimSpace(raw)
	if hasTopLevelUsage(raw) {
		return raw
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return raw
	}
	data, ok := obj["data"]
	if !ok {
		return raw
	}
	var inner map[string]json.RawMessage
	if json.Unmarshal(data, &inner) != nil {
		return raw
	}
	usage, ok := inner["usage"]
	if !ok {
		return raw
	}
	usage = bytes.TrimSpace(usage)
	if len(usage) == 0 || usage[0] != '{' {
		return raw
	}
	obj["usage"] = usage
	out, err := json.Marshal(obj)
	if err != nil {
		return raw
	}
	return out
}

func looksLikeSSE(raw []byte) bool {
	t := bytes.TrimLeft(raw, "\r\n\t ")
	return bytes.HasPrefix(t, []byte("data:")) ||
		bytes.Contains(t, []byte("\ndata:")) ||
		bytes.Contains(t, []byte("\r\ndata:"))
}

func encodeSSEUsage(u tokenUsage, pol providerPolicy) []byte {
	u = u.withTotal()
	payload, err := json.Marshal(map[string]any{
		"usage": map[string]any{
			"prompt_tokens":     u.Input,
			"completion_tokens": u.Output,
			"total_tokens":      u.Total,
		},
	})
	if err != nil {
		return nil
	}
	payload = applyProviderPolicy(payload, pol)
	out := append([]byte("data: "), payload...)
	return append(out, '\n', '\n')
}

type sseUnwrapper struct {
	pending  []byte
	heldDONE []byte
	sawUsage bool
	pol      providerPolicy
}

func (s *sseUnwrapper) Transform(in []byte) []byte {
	s.pending = append(s.pending, in...)
	var out []byte
	for {
		i := bytes.IndexByte(s.pending, '\n')
		if i < 0 {
			break
		}
		line := s.pending[:i]
		s.pending = s.pending[i+1:]
		crlf := bytes.HasSuffix(line, []byte("\r"))
		if crlf {
			line = line[:len(line)-1]
		}
		rewritten, hold := s.rewriteLine(line)
		if hold {
			s.heldDONE = append(append([]byte{}, line...), newline(crlf)...)
			continue
		}
		if len(rewritten) == 0 && len(bytes.TrimSpace(line)) == 0 {
			out = append(out, newline(crlf)...)
			continue
		}
		out = append(out, rewritten...)
		out = append(out, newline(crlf)...)
	}
	return out
}

func newline(crlf bool) []byte {
	if crlf {
		return []byte("\r\n")
	}
	return []byte("\n")
}

func (s *sseUnwrapper) Finish(u tokenUsage) []byte {
	var out []byte
	if len(s.pending) > 0 {
		rest := bytes.TrimSpace(s.pending)
		if len(rest) > 0 && rest[0] == '{' {
			norm := normalizeCompletionJSON(s.pending, s.pol)
			if hasTopLevelUsage(norm) {
				s.sawUsage = true
			}
			out = append(out, norm...)
		} else if rewritten, hold := s.rewriteLine(bytes.TrimRight(s.pending, "\r\n")); hold {
			s.heldDONE = append(s.heldDONE, bytes.TrimRight(s.pending, "\r\n")...)
		} else if len(rewritten) > 0 {
			out = append(out, rewritten...)
		}
		s.pending = nil
	}
	if !s.sawUsage && !u.empty() {
		out = append(out, encodeSSEUsage(u, s.pol)...)
		s.sawUsage = true
	}
	if len(s.heldDONE) > 0 {
		out = append(out, s.heldDONE...)
		s.heldDONE = nil
	}
	return out
}

func (s *sseUnwrapper) rewriteLine(line []byte) ([]byte, bool) {
	trimmed := bytes.TrimSpace(line)
	if isSSEDone(trimmed) {
		return nil, true
	}
	if bytes.HasPrefix(trimmed, []byte("data:")) {
		payload := bytes.TrimSpace(trimmed[len("data:"):])
		if len(payload) == 0 {
			return line, false
		}
		if isSSEDone(payload) {
			return nil, true
		}
		norm := normalizeCompletionJSON(payload, s.pol)
		if hasTopLevelUsage(norm) {
			s.sawUsage = true
		}
		if bytes.Equal(norm, payload) {
			return line, false
		}
		return append([]byte("data: "), norm...), false
	}
	if len(trimmed) > 0 && trimmed[0] == '{' {
		norm := normalizeCompletionJSON(trimmed, s.pol)
		if hasTopLevelUsage(norm) {
			s.sawUsage = true
		}
		return norm, false
	}
	return line, false
}

func isSSEDone(b []byte) bool {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("[DONE]")) {
		return true
	}
	return bytes.Equal(bytes.TrimSpace(bytes.TrimPrefix(b, []byte("data:"))), []byte("[DONE]"))
}
