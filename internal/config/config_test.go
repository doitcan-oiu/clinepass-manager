package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirWritable(t *testing.T) {
	dir := t.TempDir()
	if !DirWritable(dir) {
		t.Fatal("temp dir should be writable")
	}
	parent := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(parent, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if DirWritable(filepath.Join(parent, "child")) {
		t.Fatal("child of a file must not be writable")
	}
	if DirWritable("") {
		t.Fatal("empty")
	}
}

func TestPrepareRuntimeKeepsWritableHome(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_RUNTIME_DIR", home)
	t.Setenv("CLOAKBROWSER_CACHE_DIR", "")
	cfg := Config{DataDir: filepath.Join(root, "data")}
	got, note, err := cfg.PrepareRuntime()
	if err != nil {
		t.Fatal(err)
	}
	if note != "" {
		t.Fatalf("note=%q", note)
	}
	if os.Getenv("HOME") != home {
		t.Fatalf("HOME=%q", os.Getenv("HOME"))
	}
	want := filepath.Join(home, ".cloakbrowser")
	if got.CloakCacheDir != want {
		t.Fatalf("cache=%q", got.CloakCacheDir)
	}
	if os.Getenv("CLOAKBROWSER_CACHE_DIR") != want {
		t.Fatalf("env cache=%q", os.Getenv("CLOAKBROWSER_CACHE_DIR"))
	}
}

func TestPrepareRuntimeRedirectsUnwritableHome(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(blocker, "home"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(root, "runtime"))
	cfg := Config{DataDir: filepath.Join(root, "data")}
	got, note, err := cfg.PrepareRuntime()
	if err != nil {
		t.Fatal(err)
	}
	wantHome := filepath.Join(cfg.DataDir, "home")
	if os.Getenv("HOME") != wantHome {
		t.Fatalf("HOME=%q", os.Getenv("HOME"))
	}
	if !strings.Contains(note, "不可写") {
		t.Fatalf("note=%q", note)
	}
	if got.CloakCacheDir != filepath.Join(wantHome, ".cloakbrowser") {
		t.Fatalf("cache=%q", got.CloakCacheDir)
	}
}

func TestPrepareRuntimeHonorsCacheOverride(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	cache := filepath.Join(root, "custom-cache")
	t.Setenv("HOME", home)
	t.Setenv("XDG_RUNTIME_DIR", home)
	cfg := Config{DataDir: filepath.Join(root, "data"), CloakCacheDir: cache}
	got, _, err := cfg.PrepareRuntime()
	if err != nil {
		t.Fatal(err)
	}
	if got.CloakCacheDir != cache {
		t.Fatalf("cache=%q", got.CloakCacheDir)
	}
}

func TestPrepareRuntimeRedirectsRuntimeDir(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(blocker, "run"))
	cfg := Config{DataDir: filepath.Join(root, "data")}
	_, note, err := cfg.PrepareRuntime()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(cfg.DataDir, "run")
	if os.Getenv("XDG_RUNTIME_DIR") != want {
		t.Fatalf("XDG_RUNTIME_DIR=%q", os.Getenv("XDG_RUNTIME_DIR"))
	}
	if !strings.Contains(note, "/run/user") && !strings.Contains(note, "XDG_RUNTIME_DIR") {
		t.Fatalf("note=%q", note)
	}
}

func TestLoadYAMLThenEnvOverride(t *testing.T) {
	clearConfigEnv(t)
	p := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(p, []byte("addr: \":1234\"\ndata_dir: /tmp/from-yaml\nmax_concurrent: 4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", p)
	t.Setenv("ADDR", ":5555")
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Addr != ":5555" {
		t.Fatalf("addr=%q", got.Addr)
	}
	if got.DataDir != "/tmp/from-yaml" {
		t.Fatalf("data_dir=%q", got.DataDir)
	}
	if got.MaxConcurrent != 4 {
		t.Fatalf("concurrent=%d", got.MaxConcurrent)
	}
	if got.ConfigFile != p {
		t.Fatalf("file=%q", got.ConfigFile)
	}
}

func TestLoadYAMLBarePort(t *testing.T) {
	clearConfigEnv(t)
	p := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(p, []byte("addr: 8888\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", p)
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Addr != ":8888" {
		t.Fatalf("addr=%q", got.Addr)
	}
}

func TestLoadMissingConfigFile(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("CONFIG_FILE", filepath.Join(t.TempDir(), "nope.yaml"))
	if _, err := Load(); err == nil {
		t.Fatal("missing CONFIG_FILE should fail")
	}
}

func TestLoadDefaultsWithoutFile(t *testing.T) {
	clearConfigEnv(t)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Addr != ":9999" || got.DataDir != "./data" || got.ConfigFile != "" {
		t.Fatalf("%+v", got)
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	clearConfigEnv(t)
	p := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(p, []byte("addr: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", p)
	if _, err := Load(); err == nil {
		t.Fatal("invalid yaml should fail")
	}
}

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"ADDR", "DATA_DIR", "INVITE_URL", "HEADLESS", "SLOW_MO", "PROXY",
		"MAX_CONCURRENT", "MAX_RETRIES", "LOGIN_ENGINE", "LOGIN_PYTHON",
		"CLOAKBROWSER_VERSION", "CLOAKBROWSER_CACHE_DIR", "CLOAKBROWSER_BINARY_PATH",
		"CLOAKBROWSER_LICENSE_KEY", "CONFIG_FILE",
	} {
		t.Setenv(k, "")
	}
}
