package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTokenProvisioningIsPersistentAndExplicitConfigWins(t *testing.T) {
	dir := t.TempDir()
	token, created, err := ensureToken(dir, "")
	if err != nil || !created || len(token) != 64 {
		t.Fatalf("create: %q %v %v", token, created, err)
	}
	stored, created, err := ensureToken(dir, "")
	if err != nil || created || stored != token {
		t.Fatalf("reuse: %q %v %v", stored, created, err)
	}
	configured, created, err := ensureToken(dir, "configured-value")
	if err != nil || created || configured != "configured-value" {
		t.Fatalf("configured: %q %v %v", configured, created, err)
	}
	stored, _, err = ensureToken(dir, "")
	if err != nil || stored != token {
		t.Fatal("explicit config replaced persisted token")
	}
}

func TestEmptyPersistedTokenFailsClosed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "auto-token"), []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ensureToken(dir, ""); err == nil {
		t.Fatal("empty token must not start unauthenticated")
	}
}
