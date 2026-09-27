package gomodel

import (
	"fmt"
	"strings"
	"sync"
	"unicode"
)

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
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Endpoint Endpoint `json:"endpoint"`
	LimitUSD float64  `json:"limit_usd"` // ClinePass 没有单模型限额，恒为 0
}

var builtinModels = []Info{
	{ID: "cline-pass/glm-5.3", Name: "GLM-5.3", Endpoint: EndpointChat},
	{ID: "cline-pass/glm-5.3-flash", Name: "GLM-5.3 Flash", Endpoint: EndpointChat},
	{ID: "cline-pass/kimi-k3", Name: "Kimi K3", Endpoint: EndpointChat},
	{ID: "cline-pass/deepseek-v4-pro", Name: "DeepSeek V4 Pro", Endpoint: EndpointChat},
	{ID: "cline-pass/deepseek-v4.1-flash", Name: "DeepSeek V4.1 Flash", Endpoint: EndpointChat},
	{ID: "cline-pass/mimo-v2.5", Name: "MiMo-V2.5", Endpoint: EndpointChat},
	{ID: "cline-pass/mimo-v2.5-pro", Name: "MiMo-V2.5-Pro", Endpoint: EndpointChat},
	{ID: "cline-pass/minimax-m3", Name: "MiniMax M3", Endpoint: EndpointChat},
	{ID: "cline-pass/muse-spark-1.3-contributor", Name: "Muse Spark 1.3 Contributor", Endpoint: EndpointChat},
	{ID: "cline-pass/qwen3.8-max", Name: "Qwen3.8 Max", Endpoint: EndpointChat},
	{ID: "cline-pass/qwen3.7-max", Name: "Qwen3.7 Max", Endpoint: EndpointChat},
	{ID: "cline-pass/qwen3.7-plus", Name: "Qwen3.7 Plus", Endpoint: EndpointChat},
}

// Catalog publishes a complete model list at once; readers never see a partial refresh.
type Catalog struct {
	mu   sync.RWMutex
	all  []Info
	byID map[string]Info
}

func NewCatalog() *Catalog {
	c := &Catalog{}
	_ = c.Replace(builtinModels)
	return c
}

var defaultCatalog = NewCatalog()

func DefaultCatalog() *Catalog { return defaultCatalog }

func validateModels(models []Info) ([]Info, map[string]Info, error) {
	if len(models) == 0 || len(models) > 1000 {
		return nil, nil, fmt.Errorf("invalid model count: %d", len(models))
	}
	out := make([]Info, len(models))
	index := make(map[string]Info, len(models))
	for i, m := range models {
		m.ID = strings.TrimSpace(m.ID)
		m.Name = strings.TrimSpace(m.Name)
		id := strings.TrimPrefix(m.ID, "cline-pass/")
		if id == m.ID || id == "" || len(id) > 200 || m.Name == "" || len(m.Name) > 500 || strings.ContainsFunc(m.Name, unicode.IsControl) {
			return nil, nil, fmt.Errorf("invalid model at row %d", i+1)
		}
		if !(id[0] >= 'a' && id[0] <= 'z' || id[0] >= '0' && id[0] <= '9') {
			return nil, nil, fmt.Errorf("invalid model ID at row %d", i+1)
		}
		for _, r := range id {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
				return nil, nil, fmt.Errorf("invalid model ID at row %d", i+1)
			}
		}
		if _, exists := index[id]; exists {
			return nil, nil, fmt.Errorf("duplicate model ID: %s", m.ID)
		}
		if m.Endpoint != "" && m.Endpoint != EndpointChat {
			return nil, nil, fmt.Errorf("invalid model endpoint at row %d", i+1)
		}
		m.Endpoint = EndpointChat
		m.LimitUSD = 0
		out[i], index[id] = m, m
	}
	return out, index, nil
}

func (c *Catalog) Replace(models []Info) error {
	all, byID, err := validateModels(models)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.all, c.byID = all, byID
	c.mu.Unlock()
	return nil
}

func (c *Catalog) All() []Info {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]Info(nil), c.all...)
}

func (c *Catalog) Lookup(id string) (Info, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m, ok := c.byID[Normalize(id)]
	return m, ok
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
	return defaultCatalog.Lookup(id)
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
	return defaultCatalog.All()
}
