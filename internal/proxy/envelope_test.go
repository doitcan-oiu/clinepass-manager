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
	out = append(out, uw.Finish(tokenUsage{})...)
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

func TestAssembleCompletionMergesKimiReasoning(t *testing.T) {
	raw := []byte(strings.Join([]string{
		`data: {"choices":[{"index":0,"delta":{"content":"","role":"assistant","reasoning":"The","reasoning_details":[{"type":"reasoning.text","text":"The","format":"unknown","index":0}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{"content":"","role":"assistant","reasoning":" user","reasoning_details":[{"type":"reasoning.text","text":" user","format":"unknown","index":0}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{"content":"2","role":"assistant"}}]}`,
		`data: {"choices":[{"index":0,"delta":{"content":""},"finish_reason":"stop","native_finish_reason":"stop"}],"id":"gen_1","model":"moonshotai/kimi-k3","usage":{"prompt_tokens":90,"completion_tokens":52,"total_tokens":142}}`,
		`data: [DONE]`,
	}, "\n"))
	got := assembleCompletion(raw, tokenUsage{}, providerPolicy{})
	var obj map[string]any
	if json.Unmarshal(got, &obj) != nil {
		t.Fatalf("json %s", got)
	}
	if obj["object"] != "chat.completion" || obj["id"] != "gen_1" {
		t.Fatalf("%s", got)
	}
	msg := obj["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	if msg["content"] != "2" {
		t.Fatalf("content=%v", msg["content"])
	}
	if msg["reasoning"] != "The user" {
		t.Fatalf("reasoning=%v", msg["reasoning"])
	}
	details := msg["reasoning_details"].([]any)
	if details[0].(map[string]any)["text"] != "The user" {
		t.Fatalf("details=%v", details)
	}
}

func TestForceUpstreamStream(t *testing.T) {
	got := forceUpstreamStream([]byte(`{"model":"kimi-k3","stream":false}`))
	var obj map[string]any
	if json.Unmarshal(got, &obj) != nil || obj["stream"] != true {
		t.Fatalf("%s", got)
	}
}

func TestCopyAndParseUnwrapsJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	src := bytes.NewReader([]byte(`{"data":{"object":"chat.completion","usage":{"prompt_tokens":20,"completion_tokens":5,"total_tokens":25}},"success":true}`))
	u, _, err := copyAndParse(rr, src, false, providerPolicy{})
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
	u, _, err := copyAndParse(rr, src, true, providerPolicy{})
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

func TestUnwrapIgnoresStubTopLevelUsage(t *testing.T) {
	raw := []byte(`{"success":true,"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0},"data":{"object":"chat.completion","choices":[],"usage":{"prompt_tokens":20,"completion_tokens":278,"total_tokens":298}}}`)
	got := normalizeCompletionJSON(raw, providerPolicy{})
	var obj map[string]any
	if json.Unmarshal(got, &obj) != nil {
		t.Fatal(string(got))
	}
	if _, ok := obj["success"]; ok {
		t.Fatal("wrapper left")
	}
	u, ok := obj["usage"].(map[string]any)
	if !ok || int(u["prompt_tokens"].(float64)) != 20 {
		t.Fatalf("usage %+v body=%s", u, got)
	}
}

func TestLiftUsageIfStillWrapped(t *testing.T) {
	raw := []byte(`{"object":"chat.completion","choices":[],"data":{"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}}`)
	got := normalizeCompletionJSON(raw, providerPolicy{})
	if !hasTopLevelUsage(got) {
		t.Fatalf("usage not lifted: %s", got)
	}
	var obj map[string]any
	if json.Unmarshal(got, &obj) != nil {
		t.Fatal(string(got))
	}
	u := obj["usage"].(map[string]any)
	if int(u["prompt_tokens"].(float64)) != 7 {
		t.Fatalf("%+v", u)
	}
}

func TestCopyAndParseNDJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	src := strings.NewReader("{\"data\":{\"object\":\"chat.completion.chunk\",\"choices\":[]},\"success\":true}\n{\"data\":{\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":6,\"total_tokens\":10}},\"success\":true}\n")
	u, _, err := copyAndParse(rr, src, true, providerPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if u.Input != 4 || u.Output != 6 {
		t.Fatalf("usage %+v", u)
	}
	body := rr.Body.String()
	if strings.Contains(body, `"success"`) {
		t.Fatalf("wrapper left: %s", body)
	}
	if !strings.Contains(body, `"usage":{"prompt_tokens":4`) {
		t.Fatalf("usage not top-level: %s", body)
	}
}

func TestCopyAndParseNonStreamSSE(t *testing.T) {
	rr := httptest.NewRecorder()
	src := strings.NewReader("data: {\"data\":{\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hi\"}}]},\"success\":true}\n\ndata: {\"data\":{\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":8,\"total_tokens\":10}},\"success\":true}\n\ndata: [DONE]\n")
	u, _, err := copyAndParse(rr, src, false, providerPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if u.Input != 2 || u.Output != 8 {
		t.Fatalf("usage %+v", u)
	}
	body := rr.Body.String()
	if strings.Contains(body, `"success":true`) || strings.Contains(body, "[DONE]") || strings.Contains(body, "data:") {
		t.Fatalf("should assemble JSON, got %s", body)
	}
	var obj map[string]any
	if json.Unmarshal(rr.Body.Bytes(), &obj) != nil {
		t.Fatalf("body %s", body)
	}
	if obj["object"] != "chat.completion" {
		t.Fatalf("object=%v", obj["object"])
	}
	choices, _ := obj["choices"].([]any)
	if len(choices) != 1 {
		t.Fatalf("choices %s", body)
	}
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	if msg["content"] != "hi" {
		t.Fatalf("content=%v body=%s", msg["content"], body)
	}
}

func TestCopyAndParseDoesNotInventUsage(t *testing.T) {
	rr := httptest.NewRecorder()
	src := strings.NewReader("data: {\"data\":{\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]},\"success\":true}\n\ndata: [DONE]\n")
	_, _, err := copyAndParse(rr, src, true, providerPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	body := rr.Body.String()
	if strings.Contains(body, `"prompt_tokens"`) {
		t.Fatal("should not invent usage when upstream had none")
	}
	if !strings.Contains(body, "[DONE]") {
		t.Fatalf("missing DONE: %s", body)
	}
}

func TestCopyAndParseSingleJSONStream(t *testing.T) {
	rr := httptest.NewRecorder()
	src := strings.NewReader(`{"data":{"object":"chat.completion","usage":{"prompt_tokens":20,"completion_tokens":278,"total_tokens":298}},"success":true}`)
	u, _, err := copyAndParse(rr, src, true, providerPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if u.Input != 20 || u.Output != 278 {
		t.Fatalf("usage %+v", u)
	}
	var obj map[string]any
	if json.Unmarshal(rr.Body.Bytes(), &obj) != nil {
		t.Fatalf("body %s", rr.Body.String())
	}
	if _, ok := obj["success"]; ok {
		t.Fatal("wrapper left")
	}
	usage, ok := obj["usage"].(map[string]any)
	if !ok || int(usage["prompt_tokens"].(float64)) != 20 {
		t.Fatalf("top-level usage missing: %s", rr.Body.String())
	}
}

func TestHideProvider(t *testing.T) {
	raw := []byte(`{"object":"chat.completion","provider":"DeepInfra","usage":{"prompt_tokens":5,"completion_tokens":17,"total_tokens":22},"choices":[]}`)
	got := normalizeCompletionJSON(raw, providerPolicy{Mode: "hide"})
	var obj map[string]any
	if json.Unmarshal(got, &obj) != nil {
		t.Fatal(string(got))
	}
	if _, ok := obj["provider"]; ok {
		t.Fatalf("provider still present: %s", got)
	}
	if !hasTopLevelUsage(got) {
		t.Fatal("usage removed")
	}
}

func TestReplaceProvider(t *testing.T) {
	raw := []byte(`{"object":"chat.completion","provider":"DeepInfra","usage":{"prompt_tokens":5,"completion_tokens":17,"total_tokens":22},"choices":[]}`)
	got := normalizeCompletionJSON(raw, providerPolicy{Mode: "replace", Value: "OpenAI"})
	var obj map[string]any
	if json.Unmarshal(got, &obj) != nil {
		t.Fatal(string(got))
	}
	if obj["provider"] != "OpenAI" {
		t.Fatalf("provider=%v body=%s", obj["provider"], got)
	}
}

func TestCopyAndParseHidesProviderInSSE(t *testing.T) {
	rr := httptest.NewRecorder()
	src := strings.NewReader("data: {\"object\":\"chat.completion.chunk\",\"provider\":\"DeepInfra\",\"choices\":[]}\n\ndata: [DONE]\n")
	_, _, err := copyAndParse(rr, src, true, providerPolicy{Mode: "hide"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rr.Body.String(), `"provider"`) {
		t.Fatalf("provider leaked: %s", rr.Body.String())
	}
}

func TestParseWrappedUsage(t *testing.T) {
	raw := []byte(`{"success":true,"data":{"usage":{"prompt_tokens":20,"completion_tokens":278,"total_tokens":298}}}`)
	u, ok := parseUsageJSON(raw)
	if !ok || u.Input != 20 || u.Output != 278 || u.Total != 298 {
		t.Fatalf("%+v %v", u, ok)
	}
}
