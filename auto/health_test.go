package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"opencode-go-manager/internal/config"
)

func TestInstallerHealthCheckAuthenticatesAndRejectsUnauthenticatedService(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "auto-token"), []byte("health-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer health-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"service":"auto","protocol_version":1}`))
	}))
	defer srv.Close()
	if err := checkHealth(config.Config{AutoDataDir: dir}, srv.URL+"/api/health"); err != nil {
		t.Fatal(err)
	}
	if err := checkHealth(config.Config{AutoDataDir: dir, AutoToken: "wrong"}, srv.URL+"/api/health"); err == nil {
		t.Fatal("401 must not count as a healthy authenticated connection")
	}
	if err := checkHealth(config.Config{AutoDataDir: t.TempDir()}, srv.URL+"/api/health"); err == nil {
		t.Fatal("missing token should fail")
	}
}
