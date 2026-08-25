package proxy

import (
	"bytes"
	"encoding/json"
	"sort"
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

type completionChoice struct {
	index        int
	role         string
	content      strings.Builder
	reasoning    strings.Builder
	details      map[int]*strings.Builder
	detailMeta   map[int]map[string]any
	toolCalls    map[int]*toolCallAcc
	finish       string
	nativeFinish string
	refusal      any
}

type toolCallAcc struct {
	id   string
	typ  string
	name string
	args strings.Builder
}

func assembleCompletion(raw []byte, usage tokenUsage, pol providerPolicy) []byte {
	acc := map[int]*completionChoice{}
	meta := map[string]any{}
	var lastUsage any
	sawChunk := false

	for _, payload := range ssePayloads(raw) {
		norm := unwrapClineEnvelope(payload)
		var obj map[string]any
		if json.Unmarshal(norm, &obj) != nil {
			continue
		}
		if objType(obj) == "chat.completion" {
			if u, ok := obj["usage"]; ok {
				lastUsage = u
			}
			if lastUsage == nil && !usage.empty() {
				obj["usage"] = usageMap(usage)
			}
			return applyProviderPolicy(mustJSON(obj), pol)
		}
		if id, ok := obj["id"].(string); ok && id != "" {
			meta["id"] = id
		}
		if created, ok := obj["created"]; ok {
			meta["created"] = created
		}
		if model, ok := obj["model"].(string); ok && model != "" {
			meta["model"] = model
		}
		for _, key := range []string{"system_fingerprint", "service_tier"} {
			if v, ok := obj[key]; ok && v != nil {
				meta[key] = v
			}
		}
		if u, ok := obj["usage"]; ok && u != nil {
			lastUsage = u
		}
		choices, _ := obj["choices"].([]any)
		for _, rawChoice := range choices {
			ch, ok := rawChoice.(map[string]any)
			if !ok {
				continue
			}
			sawChunk = true
			idx := asInt(ch["index"])
			c := acc[idx]
			if c == nil {
				c = &completionChoice{index: idx, details: map[int]*strings.Builder{}, detailMeta: map[int]map[string]any{}, toolCalls: map[int]*toolCallAcc{}}
				acc[idx] = c
			}
			if fr, ok := ch["finish_reason"].(string); ok && fr != "" {
				c.finish = fr
			}
			if nfr, ok := ch["native_finish_reason"].(string); ok && nfr != "" {
				c.nativeFinish = nfr
			}
			delta, _ := ch["delta"].(map[string]any)
			if delta == nil {
				if msg, ok := ch["message"].(map[string]any); ok {
					delta = msg
				}
			}
			if delta == nil {
				continue
			}
			if role, ok := delta["role"].(string); ok && role != "" {
				c.role = role
			}
			appendAnyString(&c.content, delta["content"])
			appendAnyString(&c.reasoning, delta["reasoning"])
			appendAnyString(&c.reasoning, delta["reasoning_content"])
			if v, ok := delta["refusal"]; ok && v != nil {
				c.refusal = v
			}
			mergeReasoningDetails(c, delta["reasoning_details"])
			mergeToolCalls(c, delta["tool_calls"])
		}
	}

	out := map[string]any{"object": "chat.completion"}
	for k, v := range meta {
		out[k] = v
	}
	if lastUsage != nil {
		out["usage"] = lastUsage
	} else if !usage.empty() {
		out["usage"] = usageMap(usage)
	}

	idxs := make([]int, 0, len(acc))
	for i := range acc {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	choices := make([]any, 0, len(idxs))
	for _, i := range idxs {
		c := acc[i]
		msg := map[string]any{"role": c.role}
		if msg["role"] == "" {
			msg["role"] = "assistant"
		}
		msg["content"] = c.content.String()
		if c.reasoning.Len() > 0 {
			msg["reasoning"] = c.reasoning.String()
		}
		if details := reasoningDetails(c); len(details) > 0 {
			msg["reasoning_details"] = details
		}
		if tools := toolCalls(c); len(tools) > 0 {
			msg["tool_calls"] = tools
		}
		if c.refusal != nil {
			msg["refusal"] = c.refusal
		}
		ch := map[string]any{"index": c.index, "message": msg}
		if c.finish != "" {
			ch["finish_reason"] = c.finish
		}
		if c.nativeFinish != "" {
			ch["native_finish_reason"] = c.nativeFinish
		}
		choices = append(choices, ch)
	}
	if !sawChunk && len(choices) == 0 {
		out["choices"] = []any{}
	} else {
		out["choices"] = choices
	}
	return applyProviderPolicy(mustJSON(out), pol)
}

func ssePayloads(raw []byte) [][]byte {
	var out [][]byte
	for _, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimSpace(bytes.TrimSuffix(line, []byte("\r")))
		if len(line) == 0 || isSSEDone(line) {
			continue
		}
		if bytes.HasPrefix(line, []byte("data:")) {
			line = bytes.TrimSpace(line[len("data:"):])
		}
		if len(line) == 0 || isSSEDone(line) || line[0] != '{' {
			continue
		}
		out = append(out, append([]byte{}, line...))
	}
	return out
}

func objType(obj map[string]any) string {
	s, _ := obj["object"].(string)
	return s
}

func mustJSON(v map[string]any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

func usageMap(u tokenUsage) map[string]any {
	u = u.withTotal()
	return map[string]any{
		"prompt_tokens":     u.Input,
		"completion_tokens": u.Output,
		"total_tokens":      u.Total,
	}
}

func appendAnyString(b *strings.Builder, v any) {
	s, ok := v.(string)
	if !ok || s == "" {
		return
	}
	b.WriteString(s)
}

func mergeReasoningDetails(c *completionChoice, raw any) {
	items, ok := raw.([]any)
	if !ok {
		return
	}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		idx := asInt(m["index"])
		if c.details[idx] == nil {
			c.details[idx] = &strings.Builder{}
			c.detailMeta[idx] = map[string]any{}
			for _, k := range []string{"type", "format", "index"} {
				if v, ok := m[k]; ok {
					c.detailMeta[idx][k] = v
				}
			}
		}
		if t, ok := m["text"].(string); ok {
			c.details[idx].WriteString(t)
		}
	}
}

func reasoningDetails(c *completionChoice) []any {
	if len(c.details) == 0 {
		return nil
	}
	idxs := make([]int, 0, len(c.details))
	for i := range c.details {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	out := make([]any, 0, len(idxs))
	for _, i := range idxs {
		item := map[string]any{"text": c.details[i].String()}
		for k, v := range c.detailMeta[i] {
			item[k] = v
		}
		out = append(out, item)
	}
	return out
}

func mergeToolCalls(c *completionChoice, raw any) {
	items, ok := raw.([]any)
	if !ok {
		return
	}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		idx := asInt(m["index"])
		tc := c.toolCalls[idx]
		if tc == nil {
			tc = &toolCallAcc{}
			c.toolCalls[idx] = tc
		}
		if id, ok := m["id"].(string); ok && id != "" {
			tc.id = id
		}
		if typ, ok := m["type"].(string); ok && typ != "" {
			tc.typ = typ
		}
		fn, _ := m["function"].(map[string]any)
		if fn != nil {
			if name, ok := fn["name"].(string); ok && name != "" {
				tc.name = name
			}
			if args, ok := fn["arguments"].(string); ok {
				tc.args.WriteString(args)
			}
		}
	}
}

func toolCalls(c *completionChoice) []any {
	if len(c.toolCalls) == 0 {
		return nil
	}
	idxs := make([]int, 0, len(c.toolCalls))
	for i := range c.toolCalls {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	out := make([]any, 0, len(idxs))
	for _, i := range idxs {
		tc := c.toolCalls[i]
		fn := map[string]any{"name": tc.name, "arguments": tc.args.String()}
		item := map[string]any{"index": i, "function": fn}
		if tc.id != "" {
			item["id"] = tc.id
		}
		if tc.typ != "" {
			item["type"] = tc.typ
		}
		out = append(out, item)
	}
	return out
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
