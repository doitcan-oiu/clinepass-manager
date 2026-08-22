package browser

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"opencode-go-manager/internal/config"
)

const linuxBrowserDeps = "libnss3 libnspr4 libatk1.0-0 libatk-bridge2.0-0 libcups2 libdrm2 libgbm1 libxkbcommon0 libxcomposite1 libxdamage1 libxfixes3 libxrandr2 libpango-1.0-0 libcairo2 libasound2t64 fonts-liberation xvfb"

func hasDisplay() bool {
	return strings.TrimSpace(os.Getenv("DISPLAY")) != "" || strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) != ""
}

func cachedChromePath(cfg config.Config) string {
	if p := strings.TrimSpace(cfg.CloakBinaryPath); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	dir, err := CacheDir(cfg.CloakCacheDir)
	if err != nil {
		return ""
	}
	ver := cfg.CloakVersion
	if ver == "" {
		ver = defaultVersion
	}
	p := filepath.Join(dir, "chromium-"+ver, chromeName())
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
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

func probeChrome(bin string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin,
		"--headless=new", "--no-sandbox", "--disable-setuid-sandbox",
		"--disable-gpu", "--disable-dev-shm-usage", "--dump-dom", "about:blank")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &bytes.Buffer{}
	env := browserLaunchEnv("")
	list := make([]string, 0, len(env))
	for k, v := range env {
		list = append(list, k+"="+v)
	}
	cmd.Env = list
	err := cmd.Run()
	msg := strings.TrimSpace(stderr.String())
	if err == nil {
		return ""
	}
	if msg == "" {
		msg = err.Error()
	}
	if len(msg) > 400 {
		msg = msg[:400]
	}
	return msg
}

func Diagnose(bin string) string {
	var parts []string
	if !hasDisplay() {
		if _, err := exec.LookPath("Xvfb"); err != nil {
			parts = append(parts, "服务器没有 DISPLAY，也没有 Xvfb。本机无头能过是因为本机有显示器。请执行：sudo apt-get install -y xvfb")
		} else {
			parts = append(parts, "服务器没有 DISPLAY，启动登录时会自动拉起 Xvfb 虚拟显示")
		}
	}
	if miss := missingLibs(bin); len(miss) > 0 {
		parts = append(parts, "缺少动态库 "+strings.Join(miss, ", ")+"。Debian/Ubuntu 执行：sudo apt-get install -y "+linuxBrowserDeps)
	}
	if msg := probeChrome(bin); msg != "" {
		parts = append(parts, "直接启动 chrome 失败："+msg)
	}
	if len(parts) == 0 {
		return ""
	}
	return "；" + strings.Join(parts, "；")
}

func StartupHint(cfg config.Config) string {
	bin := cachedChromePath(cfg)
	if bin == "" {
		return "首次提取支付链接会下载 CloakBrowser。Linux 服务器需先安装浏览器依赖：sudo apt-get install -y " + linuxBrowserDeps
	}
	if hint := Diagnose(bin); hint != "" {
		return strings.TrimPrefix(hint, "；")
	}
	return "CloakBrowser 运行环境检查通过：" + bin
}

func launchError(bin string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("启动 CloakBrowser 失败: %w%s", err, Diagnose(bin))
}
