package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	autoapi "opencode-go-manager/auto/internal/api"
	"opencode-go-manager/auto/internal/job"
	managerapi "opencode-go-manager/internal/api"
	"opencode-go-manager/internal/config"
	"opencode-go-manager/internal/model"
	"opencode-go-manager/internal/store"
)

// Exercise both real HTTP handlers against distinct databases without starting
// browser workers or contacting billing, SMS or payment providers.
func TestIndependentManagerAndAuto(t *testing.T) {
	openStore := func(name string) *store.Store {
		t.Helper()
		st, err := store.Open(filepath.Join(t.TempDir(), name+".db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = st.Close() })
		if err := st.SeedDefaults(config.Config{Headless: true, MaxConcurrent: 1}); err != nil {
			t.Fatal(err)
		}
		return st
	}
	remoteStore, mainStore := openStore("auto"), openStore("manager")
	cfg := config.Config{AutoToken: "integration-only-token", Headless: true, MaxConcurrent: 1}
	remote := httptest.NewServer(autoapi.New(cfg, remoteStore, job.New(cfg, remoteStore)).Handler())
	defer remote.Close()
	manager := httptest.NewServer(managerapi.New(config.Config{}, mainStore, t.TempDir()).Handler())
	defer manager.Close()
	request := func(method, path string, input any, output any, want int) {
		t.Helper()
		var body io.Reader
		if input != nil {
			b, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			body = bytes.NewReader(b)
		}
		req, err := http.NewRequest(method, manager.URL+path, body)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != want {
			b, _ := io.ReadAll(res.Body)
			t.Fatalf("%s %s: got %d, want %d: %s", method, path, res.StatusCode, want, b)
		}
		if output != nil {
			if err := json.NewDecoder(res.Body).Decode(output); err != nil {
				t.Fatal(err)
			}
		}
	}
	request("PATCH", "/api/auto/connection", map[string]string{"url": remote.URL, "token": cfg.AutoToken}, nil, 200)
	var health struct {
		Connected bool `json:"connected"`
	}
	request("POST", "/api/auto/test", nil, &health, 200)
	if !health.Connected {
		t.Fatal("real authenticated Auto health failed")
	}
	request("PATCH", "/api/auto/config", map[string]any{"headless": false, "max_concurrent": 2}, nil, 200)
	remoteSettings, err := remoteStore.GetSettings()
	if err != nil || remoteSettings.Headless || remoteSettings.MaxConcurrent != 2 {
		t.Fatalf("remote settings not updated: %+v, %v", remoteSettings, err)
	}
	mainSettings, err := mainStore.GetSettings()
	if err != nil || !mainSettings.Headless || mainSettings.MaxConcurrent != 1 {
		t.Fatalf("manager settings affected: %+v, %v", mainSettings, err)
	}
	var created struct {
		Batch model.Batch `json:"batch"`
	}
	request("POST", "/api/batches", model.CreateBatchInput{Name: "remote batch", LoginProvider: "microsoft", Text: "test@example.test----test-password"}, &created, 201)
	accounts, err := remoteStore.ListByBatch(created.Batch.ID)
	if err != nil || len(accounts) != 1 {
		t.Fatalf("batch not stored remotely: %v", err)
	}
	if count, err := mainStore.CountBatches(); err != nil || count != 0 {
		t.Fatalf("remote batch leaked into local database: %d %v", count, err)
	}
	var account model.Account
	request("GET", "/api/auto/accounts/"+accounts[0].ID, nil, &account, 200)
	if account.Email != "test@example.test" {
		t.Fatalf("wrong remote account: %q", account.Email)
	}
	request("GET", "/api/accounts/"+accounts[0].ID, nil, nil, 404)
	var result struct {
		Imported int `json:"imported"`
	}
	request("POST", "/api/auto/import", map[string]string{"batch_id": created.Batch.ID}, &result, 200)
	if result.Imported != 0 {
		t.Fatal("unpaid remote account was imported")
	}
	request("DELETE", "/api/auto/accounts/"+accounts[0].ID, nil, nil, 204)
	if _, err := remoteStore.Get(accounts[0].ID); err == nil {
		t.Fatal("remote account not deleted")
	}
}
