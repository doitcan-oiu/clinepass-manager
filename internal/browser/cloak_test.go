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
		"--fingerprint-windows-font-metrics",
		"--fingerprint-allow-3p-cookies",
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

func TestProDownloadUsesOfficialAPI(t *testing.T) {
	got := proDownloadURL("151.0.7922.108.2")
	if got != "https://cloakbrowser.dev/api/download/151.0.7922.108.2" {
		t.Fatalf("%s", got)
	}
	if !needsLicense("151.0.7922.108.2") {
		t.Fatal("151 needs license")
	}
	if needsLicense("146.0.7680.177.5") {
		t.Fatal("146 is the public free build")
	}
}

func TestEnsureBinary151WithoutLicenseExplains404(t *testing.T) {
	_, err := EnsureBinary("151.0.7922.108.2", t.TempDir(), "", "", nil)
	if err == nil || !strings.Contains(err.Error(), "/api/download/") {
		t.Fatalf("%v", err)
	}
}

func TestIgnoreDefaultArgsHidesAutomation(t *testing.T) {
	got := strings.Join(IgnoreDefaultArgs(), " ")
	if !strings.Contains(got, "--enable-automation") || !strings.Contains(got, "--enable-unsafe-swiftshader") {
		t.Fatalf("%v", IgnoreDefaultArgs())
	}
}
