package browser

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"opencode-go-manager/internal/config"
)

func TestParseLddMissing(t *testing.T) {
	raw := `
	linux-vdso.so.1 (0x0000)
	libnss3.so => not found
	libgbm.so.1 => not found
	libc.so.6 => /lib/x86_64-linux-gnu/libc.so.6 (0x0000)
`
	got := parseLddMissing(raw)
	if len(got) != 2 || got[0] != "libnss3.so" || got[1] != "libgbm.so.1" {
		t.Fatalf("%v", got)
	}
}

func TestDiagnoseWithoutDisplayReportsMissingBinary(t *testing.T) {
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	// An empty executable search path and a missing binary keep this diagnostic
	// test independent of installed browsers, Xvfb and ldd.
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	got := Diagnose(filepath.Join(dir, "missing-chrome.exe"))
	if !strings.Contains(got, "浏览器文件不可用") || !strings.Contains(got, "missing-chrome.exe") {
		t.Fatalf("missing browser file was not reported: %q", got)
	}
	if runtime.GOOS == "linux" {
		if !strings.Contains(got, "服务器没有 DISPLAY，也没有 Xvfb") || !strings.Contains(got, "apt-get") {
			t.Fatalf("Linux display advice was lost: %q", got)
		}
		return
	}
	for _, linuxOnly := range []string{"DISPLAY", "Xvfb", "apt-get", "Debian", "缺少动态库"} {
		if strings.Contains(got, linuxOnly) {
			t.Errorf("%s diagnostic includes Linux advice %q: %q", runtime.GOOS, linuxOnly, got)
		}
	}
}

func TestStartupHintDoesNotLaunchCachedBrowser(t *testing.T) {
	t.Setenv("DISPLAY", ":0")
	t.Setenv("CLOAKBROWSER_LICENSE_KEY", "")
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	bin := filepath.Join(dir, "chrome.exe")
	// A cached file is enough for a startup inventory. It deliberately cannot
	// execute: normal startup must leave launching and license checks to jobs.
	if err := os.WriteFile(bin, []byte("not an executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, headless := range []bool{false, true} {
		got := StartupHint(config.Config{CloakBinaryPath: bin, LicenseKey: "test-config-only-key", Headless: headless})
		if !strings.Contains(got, "CloakBrowser 文件已就绪") || !strings.Contains(got, "批次") {
			t.Errorf("startup attempted a browser launch or claimed runtime validation: %q", got)
		}
		if strings.Contains(got, "test-config-only-key") {
			t.Fatal("startup hint exposed a license")
		}
	}
}

func TestStartupHintReportsInvalidExplicitBinary(t *testing.T) {
	dir := t.TempDir()
	got := StartupHint(config.Config{CloakBinaryPath: filepath.Join(dir, "missing.exe"), CloakCacheDir: dir})
	if !strings.Contains(got, "浏览器文件不可用") || !strings.Contains(got, "missing.exe") {
		t.Fatalf("explicit missing binary was ignored: %q", got)
	}
}

func TestLaunchErrorPreservesOriginalWithoutSecondBrowser(t *testing.T) {
	t.Setenv("DISPLAY", ":0")
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	bin := filepath.Join(dir, "chrome.exe")
	if err := os.WriteFile(bin, []byte("not an executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	original := errors.New("process did exit: exitCode=77, signal=null")
	got := launchError(bin, original)
	if !errors.Is(got, original) || !strings.Contains(got.Error(), "exitCode=77") {
		t.Fatalf("original launch failure lost: %v", got)
	}
	if strings.Contains(got.Error(), "直接启动 chrome") {
		t.Fatalf("error handler ran a second browser: %v", got)
	}
}

func TestStartupHintWithoutCachedBrowser(t *testing.T) {
	got := StartupHint(config.Config{CloakCacheDir: t.TempDir()})
	if !strings.Contains(got, "首次提取支付链接会下载 CloakBrowser") {
		t.Fatalf("first-run download advice was lost: %q", got)
	}
	if runtime.GOOS == "linux" {
		if !strings.Contains(got, "sudo apt-get install -y "+linuxBrowserDeps) {
			t.Fatalf("Linux dependency advice was lost: %q", got)
		}
		return
	}
	for _, linuxOnly := range []string{"Linux", "sudo", "apt-get", "xvfb"} {
		if strings.Contains(got, linuxOnly) {
			t.Errorf("%s startup hint includes Linux advice %q: %q", runtime.GOOS, linuxOnly, got)
		}
	}
}
