package gomodel

import "strings"

const (
	RollingUSD = 10.0
	WeeklyUSD  = 25.0
	MonthlyUSD = 50.0
)

type Endpoint string

const (
	EndpointChat Endpoint = "chat"
)

type Info struct {
	ID       string
	Name     string
	Endpoint Endpoint
	LimitUSD float64 // ClinePass 没有单模型限额，恒为 0
}

var all = []Info{
	{ID: "cline-pass/glm-5.3", Name: "GLM-5.3", Endpoint: EndpointChat},
	{ID: "cline-pass/glm-5.2", Name: "GLM-5.2", Endpoint: EndpointChat},
	{ID: "cline-pass/kimi-k3", Name: "Kimi K3", Endpoint: EndpointChat},
	{ID: "cline-pass/kimi-k2.7-code", Name: "Kimi K2.7 Code", Endpoint: EndpointChat},
	{ID: "cline-pass/kimi-k2.6", Name: "Kimi K2.6", Endpoint: EndpointChat},
	{ID: "cline-pass/deepseek-v4-pro", Name: "DeepSeek V4 Pro", Endpoint: EndpointChat},
	{ID: "cline-pass/deepseek-v4-flash", Name: "DeepSeek V4 Flash", Endpoint: EndpointChat},
	{ID: "cline-pass/mimo-v2.5", Name: "MiMo-V2.5", Endpoint: EndpointChat},
	{ID: "cline-pass/mimo-v2.5-pro", Name: "MiMo-V2.5-Pro", Endpoint: EndpointChat},
	{ID: "cline-pass/minimax-m3", Name: "MiniMax M3", Endpoint: EndpointChat},
	{ID: "cline-pass/qwen3.8-max", Name: "Qwen3.8 Max", Endpoint: EndpointChat},
	{ID: "cline-pass/qwen3.7-max", Name: "Qwen3.7 Max", Endpoint: EndpointChat},
	{ID: "cline-pass/qwen3.7-plus", Name: "Qwen3.7 Plus", Endpoint: EndpointChat},
}

var byID = map[string]Info{}

func init() {
	for _, m := range all {
		byID[Normalize(m.ID)] = m
	}
}

func Normalize(id string) string {
	id = strings.TrimSpace(strings.ToLower(id))
	id = strings.TrimPrefix(id, "cline-pass/")
	id = strings.TrimPrefix(id, "cline/")
	id = strings.TrimPrefix(id, "opencode-go/")
	id = strings.TrimPrefix(id, "opencode/")
	return id
}

func Lookup(id string) (Info, bool) {
	m, ok := byID[Normalize(id)]
	return m, ok
}

func Canonical(id string) string {
	if m, ok := Lookup(id); ok {
		return m.ID
	}
	n := Normalize(id)
	if n == "" {
		return id
	}
	return "cline-pass/" + n
}

func LimitUSD(id string) float64 {
	return 0
}

func UpstreamPath(reqPath, model string) string {
	return "/api/v1/chat/completions"
}

func All() []Info {
	out := make([]Info, len(all))
	copy(out, all)
	return out
}
