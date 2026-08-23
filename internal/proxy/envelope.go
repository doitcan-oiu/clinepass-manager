package proxy

import (
	"bytes"
	"encoding/json"
)

func unwrapClineEnvelope(raw []byte) []byte {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return raw
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return raw
	}
	if usage, ok := obj["usage"]; ok {
		usage = bytes.TrimSpace(usage)
		if len(usage) > 0 && usage[0] == '{' {
			return raw
		}
	}
	data, ok := obj["data"]
	if !ok {
		return raw
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 || data[0] != '{' {
		return raw
	}
	if !completionLike(data) {
		return raw
	}
	return data
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

type sseUnwrapper struct {
	pending []byte
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
		out = append(out, rewriteSSELine(line)...)
		if crlf {
			out = append(out, '\r', '\n')
		} else {
			out = append(out, '\n')
		}
	}
	return out
}

func (s *sseUnwrapper) Flush() []byte {
	if len(s.pending) == 0 {
		return nil
	}
	rest := bytes.TrimSpace(s.pending)
	if len(rest) > 0 && rest[0] == '{' {
		return unwrapClineEnvelope(s.pending)
	}
	return rewriteSSELine(bytes.TrimRight(s.pending, "\r\n"))
}

func rewriteSSELine(line []byte) []byte {
	if !bytes.HasPrefix(line, []byte("data:")) {
		return line
	}
	payload := bytes.TrimSpace(line[len("data:"):])
	if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
		return line
	}
	unwrapped := unwrapClineEnvelope(payload)
	if bytes.Equal(unwrapped, payload) {
		return line
	}
	return append([]byte("data: "), unwrapped...)
}
