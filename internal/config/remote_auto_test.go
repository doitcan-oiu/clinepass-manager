package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAutoConnectionAndStorageEnvironment(t *testing.T) {
	clearConfigEnv(t)
	p := filepath.Join(t.TempDir(), "auto.yaml")
	if err := os.WriteFile(p, []byte("auto_addr: ':7777'\nauto_token: yaml-token\nauto_data_dir: ./remote-auto\ndata_dir: ./manager-only\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", p)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AutoAddr != ":7777" || cfg.AutoToken != "yaml-token" || cfg.AutoDataDir != "./remote-auto" || cfg.DataDir != "./manager-only" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	t.Setenv("AUTO_TOKEN", "env-token")
	t.Setenv("AUTO_DATA_DIR", "./env-auto")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AutoToken != "env-token" || cfg.AutoDataDir != "./env-auto" || cfg.DataDir != "./manager-only" {
		t.Fatalf("environment did not isolate Auto: %+v", cfg)
	}
	base := defaults()
	if base.AutoURL != "" || base.AutomationURL() != "" {
		t.Fatal("manager must wait for an explicitly configured Auto connection")
	}
	if base.AutoAddr != ":9998" || base.AutoDataDir == base.DataDir {
		t.Fatalf("unsafe remote defaults: %+v", base)
	}
}
