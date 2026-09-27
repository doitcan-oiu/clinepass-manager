package browser

import (
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

func TestDiagnoseWithoutDisplayPreservesBrowserFailure(t *testing.T) {
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	// An empty executable search path and a missing binary keep this diagnostic
	// test independent of installed browsers, Xvfb and ldd.
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	got := Diagnose(filepath.Join(dir, "missing-chrome.exe"))
	if !strings.Contains(got, "直接启动 chrome 失败：") || !strings.Contains(got, "missing-chrome.exe") {
		t.Fatalf("browser launch failure was lost: %q", got)
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
