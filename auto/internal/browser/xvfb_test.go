package browser

import (
	"os"
	"runtime"
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

func TestNativeDesktopDoesNotRequireXDisplay(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("Linux uses DISPLAY and the Xvfb fallback")
	}
	// Windows/RDP and macOS use their native desktop, without X11 variables.
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("PATH", "")
	logf := func(format string, args ...any) {
		t.Errorf("native desktop should not try Xvfb: "+format, args...)
	}
	if err := EnsureVirtualDisplay(logf); err != nil {
		t.Errorf("native desktop required a virtual display: %v", err)
	}
	if PrepareDisplay(false, logf) {
		t.Error("headed mode was changed to headless without DISPLAY")
	}
	if !PrepareDisplay(true, logf) {
		t.Error("explicit headless mode was changed")
	}
	if VirtualDisplay() != "" || os.Getenv("DISPLAY") != "" {
		t.Error("native desktop must not create an X11 display")
	}
}
