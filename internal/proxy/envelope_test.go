package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUnwrapClineEnvelope(t *testing.T) {
	wrapped := []byte(`{"data":{"choices":[{"finish_reason":"stop","index":0,"message":{"content":"hi","role":"assistant"}}],"id":"gen_1","model":"zai/glm-5.3","object":"chat.completion","usage":{"prompt_tokens":20,"completion_tokens":278,"total_tokens":298}},"success":true}`)
	got := unwrapClineEnvelope(wrapped)
	var obj map[string]any
	if json.Unmarshal(got, &obj) != nil {
		t.Fatalf("json %s", got)
	}
	if _, ok := obj["success"]; ok {
		t.Fatal("wrapper left in place")
	}
	if _, ok := obj["data"]; ok {
		t.Fatal("data wrapper left in place")
	}
	if obj["object"] != "chat.completion" {
		t.Fatalf("object=%v", obj["object"])
	}
	u, ok := obj["usage"].(map[string]any)
	if !ok {
		t.Fatalf("usage missing: %s", got)
	}
	if int(u["prompt_tokens"].(float64)) != 20 || int(u["completion_tokens"].(float64)) != 278 {
		t.Fatalf("usage %+v", u)
	}
}

func TestUnwrapLeavesOpenAIBody(t *testing.T) {
	raw := []byte(`{"object":"chat.completion","usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3},"choices":[]}`)
	if !bytes.Equal(unwrapClineEnvelope(raw), raw) {
		t.Fatal("must keep already-compatible body")
	}
}

func TestUnwrapLeavesModelsList(t *testing.T) {
	raw := []byte(`{"object":"list","data":[{"id":"glm-5.3","object":"model"}]}`)
	if !bytes.Equal(unwrapClineEnvelope(raw), raw) {
		t.Fatal("must not unwrap models list")
	}
}

func TestRewriteWrappedSSE(t *testing.T) {
	uw := &sseUnwrapper{}
	out := uw.Transform([]byte("data: {\"data\":{\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]},\"success\":true}\n"))
	out = append(out, uw.Transform([]byte("data: {\"data\":{\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":2,\"total_tokens\":13}},\"success\":true}\n"))...)
	out = append(out, uw.Transform([]byte("data: [DONE]\n"))...)
	text := string(out)
	if strings.Contains(text, `"success"`) {
		t.Fatalf("wrapper left: %s", text)
	}
	if !strings.Contains(text, `"object":"chat.completion.chunk"`) {
		t.Fatalf("chunk missing: %s", text)
	}
	if !strings.Contains(text, `"usage":{"prompt_tokens":11`) {
		t.Fatalf("usage not top-level: %s", text)
	}
	if !strings.Contains(text, "data: [DONE]") {
		t.Fatalf("done missing: %s", text)
	}
}

func TestCopyAndParseUnwrapsJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	src := bytes.NewReader([]byte(`{"data":{"object":"chat.completion","usage":{"prompt_tokens":20,"completion_tokens":5,"total_tokens":25}},"success":true}`))
	u, _, err := copyAndParse(rr, src, false)
	if err != nil {
		t.Fatal(err)
	}
	if u.Input != 20 || u.Output != 5 || u.Total != 25 {
		t.Fatalf("usage %+v", u)
	}
	var obj map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &obj); err != nil {
		t.Fatal(err)
	}
	if _, ok := obj["usage"].(map[string]any); !ok {
		t.Fatalf("client body %s", rr.Body.String())
	}
	if _, ok := obj["success"]; ok {
		t.Fatal("success wrapper sent to client")
	}
}

func TestCopyAndParseUnwrapsSSE(t *testing.T) {
	rr := httptest.NewRecorder()
	src := io.NopCloser(strings.NewReader("data: {\"data\":{\"object\":\"chat.completion.chunk\",\"choices\":[]},\"success\":true}\n\ndata: {\"data\":{\"usage\":{\"prompt_tokens\":9,\"completion_tokens\":1,\"total_tokens\":10}},\"success\":true}\n\ndata: [DONE]\n"))
	u, _, err := copyAndParse(rr, src, true)
	if err != nil {
		t.Fatal(err)
	}
	if u.Input != 9 || u.Output != 1 {
		t.Fatalf("usage %+v", u)
	}
	body := rr.Body.String()
	if strings.Contains(body, `"success"`) {
		t.Fatalf("wrapper left: %s", body)
	}
	if !strings.Contains(body, `"usage":{"prompt_tokens":9`) {
		t.Fatalf("usage not top-level: %s", body)
	}
}

func TestParseWrappedUsage(t *testing.T) {
	raw := []byte(`{"success":true,"data":{"usage":{"prompt_tokens":20,"completion_tokens":278,"total_tokens":298}}}`)
	u, ok := parseUsageJSON(raw)
	if !ok || u.Input != 20 || u.Output != 278 || u.Total != 298 {
		t.Fatalf("%+v %v", u, ok)
	}
}
