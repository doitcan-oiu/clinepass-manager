package proxy

import (
	"testing"
)

func TestParseOpenAIUsage(t *testing.T) {
	raw := []byte(`{"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"prompt_tokens_details":{"cached_tokens":40},"completion_tokens_details":{"reasoning_tokens":8}}}`)
	u, ok := parseUsageJSON(raw)
	if !ok {
		t.Fatal("parse")
	}
	if u.Input != 100 || u.Output != 20 || u.Total != 120 || u.CacheRead != 40 || u.Reasoning != 8 {
		t.Fatalf("%+v", u)
	}
}

func TestParseAnthropicUsage(t *testing.T) {
	raw := []byte(`{"usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":3,"cache_creation_input_tokens":2}}`)
	u, ok := parseUsageJSON(raw)
	if !ok || u.Input != 10 || u.CacheRead != 3 || u.CacheWrite != 2 {
		t.Fatalf("%+v %v", u, ok)
	}
}

func TestParseSSEUsage(t *testing.T) {
	p := newUsageParser(true)
	p.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n"))
	p.Write([]byte("data: {\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":2,\"total_tokens\":13}}\n"))
	p.Write([]byte("data: [DONE]\n"))
	u := p.Result()
	if u.Input != 11 || u.Output != 2 || u.Total != 13 {
		t.Fatalf("%+v", u)
	}
}

func TestAPIFormat(t *testing.T) {
	if got := apiFormat("/v1/chat/completions", "glm-5.3"); got != "openai/chat_completions" {
		t.Fatal(got)
	}
	if got := apiFormat("/v1/messages", "cline-pass/minimax-m3"); got != "openai/chat_completions" {
		t.Fatal(got)
	}
}
