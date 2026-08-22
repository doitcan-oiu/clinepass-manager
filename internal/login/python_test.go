package login

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEngineDefaultPython(t *testing.T) {
	t.Setenv("LOGIN_ENGINE", "")
	if Engine() != "python" {
		t.Fatalf("default=%q", Engine())
	}
	t.Setenv("LOGIN_ENGINE", "go")
	if Engine() != "go" {
		t.Fatalf("go=%q", Engine())
	}
}

func TestMapWorkerCode(t *testing.T) {
	if !errors.Is(mapWorkerCode("radar_denied", "x"), ErrRadarDenied) {
		t.Fatal("radar")
	}
	if !errors.Is(mapWorkerCode("banned", "x"), ErrAccountBanned) {
		t.Fatal("banned")
	}
	if !errors.Is(mapWorkerCode("sms_relogin", "x"), ErrSMSNeedRelogin) {
		t.Fatal("sms")
	}
	if !errors.Is(mapWorkerCode("sms_timeout", "x"), ErrPhoneTimeout) {
		t.Fatal("timeout")
	}
	if !errors.Is(mapWorkerCode("authkit_stuck", "boom"), ErrAuthkitStuck) {
		t.Fatal("authkit")
	}
	if IsAuthkitFailure(mapWorkerCode("radar_denied", "x")) {
		t.Fatal("radar should not retry")
	}
}

func TestFindWorker(t *testing.T) {
	python, script, err := findWorker()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(script); err != nil {
		t.Fatal(script, err)
	}
	if python == "" {
		t.Fatal("empty python")
	}
}

func TestFindRepoRoot(t *testing.T) {
	root, err := FindRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "worker", "login.py")); err != nil {
		t.Fatal(root, err)
	}
}
