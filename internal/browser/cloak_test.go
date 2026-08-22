package browser

import (
	"strings"
	"testing"
)

func TestDefaultStealthArgsUseBinaryFingerprint(t *testing.T) {
	args := DefaultStealthArgs(12345)
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--fingerprint=12345",
		"--fingerprint-timezone=America/New_York",
		"--fingerprint-locale=en-US",
		"--lang=en-US",
		"--fingerprint-storage-quota=5000",
		"--ignore-gpu-blocklist",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s in %v", want, args)
		}
	}
}

func TestIgnoreDefaultArgsHidesAutomation(t *testing.T) {
	got := strings.Join(IgnoreDefaultArgs(), " ")
	if !strings.Contains(got, "--enable-automation") || !strings.Contains(got, "--enable-unsafe-swiftshader") {
		t.Fatalf("%v", IgnoreDefaultArgs())
	}
}
