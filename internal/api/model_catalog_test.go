package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"opencode-go-manager/internal/gomodel"
	"opencode-go-manager/internal/proxy"
	"opencode-go-manager/internal/store"
)

type catalogRoundTrip func(*http.Request) (*http.Response, error)

func (f catalogRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestModelCatalogRefreshUpdatesPublicAndManagementAPIs(t *testing.T) {
	original := gomodel.All()
	t.Cleanup(func() { _ = gomodel.DefaultCatalog().Replace(original) })
	st, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	phase := 0
	client := &http.Client{Transport: catalogRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "docs.cline.bot" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Fatalf("unexpected documentation request: %s", r.URL)
		}
		if phase == 2 {
			return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
		}
		rows := `<tr><td>New model</td><td><code>cline-pass/new-model</code></td></tr>`
		if phase == 0 {
			rows += `<tr><td>GLM-5.3</td><td><code>cline-pass/glm-5.3</code></td></tr>`
		}
		body := `<html><body><h2 id="models">Models</h2><table><thead><tr><th>Model</th><th>Model ID</th></tr></thead><tbody>` + rows + `</tbody></table><h2>Usage</h2></body></html>`
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	s := &Server{store: st, proxy: proxy.New(st), catalog: gomodel.NewSyncer(st, client, nil)}
	request := func(method, path string, status int) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(method, path, nil))
		if w.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w
	}
	for _, count := range []int{2, 1} {
		request("POST", "/api/models/sync", http.StatusOK)
		public := request("GET", "/v1/models", http.StatusOK)
		var models struct {
			Object string `json:"object"`
			Data   []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(public.Body.Bytes(), &models); err != nil || models.Object != "list" || len(models.Data) != count || models.Data[0].ID != "cline-pass/new-model" {
			t.Fatalf("public catalog: %s, %v", public.Body, err)
		}
		if public.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("dynamic models response is cacheable")
		}
		management := request("GET", "/api/models", http.StatusOK)
		if !strings.Contains(management.Body.String(), `"name":"New model"`) {
			t.Fatalf("management models stale: %s", management.Body)
		}
		dashboard := request("GET", "/api/dashboard", http.StatusOK)
		if !strings.Contains(dashboard.Body.String(), fmt.Sprintf(`"models_total":%d`, count)) {
			t.Fatalf("dashboard model count stale: %s", dashboard.Body)
		}
		phase++
	}
	request("POST", "/api/models/sync", http.StatusBadGateway)
	public := request("GET", "/v1/models", http.StatusOK)
	if strings.Contains(public.Body.String(), "glm-5.3") || !strings.Contains(public.Body.String(), "new-model") {
		t.Fatalf("failed refresh lost last successful catalog: %s", public.Body)
	}
	status := request("GET", "/api/models/sync", http.StatusOK)
	if !strings.Contains(status.Body.String(), `"model_count":1`) || strings.Contains(status.Body.String(), `"last_error":""`) {
		t.Fatalf("sync failure not observable: %s", status.Body)
	}
	cache, err := st.LoadModelCatalog()
	if err != nil || len(cache.Models) != 1 || cache.Models[0].ID != "cline-pass/new-model" {
		t.Fatalf("durable catalog: %+v, %v", cache, err)
	}
}
