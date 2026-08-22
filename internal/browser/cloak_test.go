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
		"--fingerprint-storage-quota=5000",
		"--ignore-gpu-blocklist",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s in %v", want, args)
		}
	}
	for _, banned := range []string{
		"--fingerprint-timezone=",
		"--fingerprint-locale=",
		"--lang=",
	} {
		if strings.Contains(joined, banned) {
			t.Fatalf("must not force locale/timezone: %v", args)
		}
	}
}

func TestIgnoreDefaultArgsHidesAutomation(t *testing.T) {
	got := strings.Join(IgnoreDefaultArgs(), " ")
	if !strings.Contains(got, "--enable-automation") || !strings.Contains(got, "--enable-unsafe-swiftshader") {
		t.Fatalf("%v", IgnoreDefaultArgs())
	}
}
