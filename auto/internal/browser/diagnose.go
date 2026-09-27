package browser

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"opencode-go-manager/internal/config"
)

const linuxBrowserDeps = "libnss3 libnspr4 libatk1.0-0 libatk-bridge2.0-0 libcups2 libdrm2 libgbm1 libxkbcommon0 libxcomposite1 libxdamage1 libxfixes3 libxrandr2 libpango-1.0-0 libcairo2 libasound2t64 fonts-liberation fonts-noto-color-emoji fonts-freefont-ttf fonts-unifont xvfb"

func hasDisplay() bool {
	return strings.TrimSpace(os.Getenv("DISPLAY")) != "" || strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) != ""
}

func cachedChromePath(cfg config.Config) string {
	if p := strings.TrimSpace(cfg.CloakBinaryPath); p != "" {
		// Report a broken explicit path rather than silently inspecting a
		// different cached browser than the one the worker will launch.
		return p
	}
	dir, err := CacheDir(cfg.CloakCacheDir)
	if err != nil {
		return ""
	}
	ver := cfg.CloakVersion
	if ver == "" {
		ver = defaultVersion
	}
	if p := cachedBinaryPath(dir, ver); p != "" {
		return p
	}
	return ""
}

func parseLddMissing(out string) []string {
	var missing []string
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "not found") {
			continue
		}
		name := strings.Fields(line)[0]
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		missing = append(missing, name)
	}
	return missing
}

func missingLibs(bin string) []string {
	if strings.TrimSpace(bin) == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ldd", bin)
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return nil
	}
	return parseLddMissing(string(out))
}

// Diagnose checks local prerequisites without launching another browser. A
// raw launch does not use the worker's configuration and can fail Pro license
// validation or consume an extra session while the real task is starting.
func Diagnose(bin string) string {
	var parts []string
	info, statErr := os.Stat(bin)
	if statErr != nil {
		parts = append(parts, "浏览器文件不可用："+statErr.Error())
	} else if info.IsDir() {
		parts = append(parts, "浏览器路径指向目录而非可执行文件："+bin)
	}
	if runtime.GOOS == "linux" {
		if !hasDisplay() {
			if _, err := exec.LookPath("Xvfb"); err != nil {
				parts = append(parts, "服务器没有 DISPLAY，也没有 Xvfb。本机无头能过是因为本机有显示器。请执行：sudo apt-get install -y xvfb")
			} else {
				parts = append(parts, "服务器没有 DISPLAY，启动登录时会自动拉起 Xvfb 虚拟显示")
			}
		}
		if statErr == nil && !info.IsDir() {
			if miss := missingLibs(bin); len(miss) > 0 {
				parts = append(parts, "缺少动态库 "+strings.Join(miss, ", ")+"。Debian/Ubuntu 执行：sudo apt-get install -y "+linuxBrowserDeps)
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "；" + strings.Join(parts, "；")
}

func StartupHint(cfg config.Config) string {
	bin := cachedChromePath(cfg)
	if bin == "" {
		hint := "首次提取支付链接会下载 CloakBrowser。"
		if runtime.GOOS == "linux" {
			hint += "Linux 服务器需先安装浏览器依赖：sudo apt-get install -y " + linuxBrowserDeps
		}
		return hint
	}
	if hint := Diagnose(bin); hint != "" {
		return strings.TrimPrefix(hint, "；")
	}
	return "CloakBrowser 文件已就绪：" + bin + "；浏览器将在批次中按设置启动"
}

func launchError(bin string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("启动 CloakBrowser 失败: %w%s", err, Diagnose(bin))
}
