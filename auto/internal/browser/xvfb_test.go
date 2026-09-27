package browser

import (
	"os"
	"testing"
)

func TestEnsureVirtualDisplayNoopWhenDisplaySet(t *testing.T) {
	t.Setenv("DISPLAY", ":0")
	if err := EnsureVirtualDisplay(nil); err != nil {
		t.Fatal(err)
	}
	if virtualDisplay() != "" {
		t.Fatalf("should not start Xvfb when DISPLAY exists, got %q", virtualDisplay())
	}
	if os.Getenv("DISPLAY") != ":0" {
		t.Fatalf("DISPLAY=%q", os.Getenv("DISPLAY"))
	}
}

func TestPrepareDisplayKeepsWantedHeadlessWhenDisplayExists(t *testing.T) {
	t.Setenv("DISPLAY", ":0")
	if !PrepareDisplay(true, nil) {
		t.Fatal("local DISPLAY should keep requested headless")
	}
	if PrepareDisplay(false, nil) {
		t.Fatal("headed request should stay headed")
	}
}
