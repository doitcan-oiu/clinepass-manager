package browser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

var (
	xvfbOnce    sync.Once
	xvfbErr     error
	xvfbDisplay string
)

func virtualDisplay() string {
	return xvfbDisplay
}

func VirtualDisplay() string {
	return xvfbDisplay
}

func EnsureVirtualDisplay(logf func(string, ...any)) error {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	if hasDisplay() {
		return nil
	}
	xvfbOnce.Do(func() {
		xvfbErr = startXvfb(logf)
	})
	return xvfbErr
}

func PrepareDisplay(wantedHeadless bool, logf func(string, ...any)) bool {
	headless := wantedHeadless
	if err := EnsureVirtualDisplay(logf); err != nil && !hasDisplay() {
		if logf != nil {
			logf("%v", err)
		}
	}
	if virtualDisplay() != "" && wantedHeadless {
		headless = false
	}
	if !headless && !hasDisplay() {
		headless = true
	}
	return headless
}

func startXvfb(logf func(string, ...any)) error {
	bin, err := exec.LookPath("Xvfb")
	if err != nil {
		return fmt.Errorf("服务器没有 DISPLAY，且未安装 Xvfb。Debian/Ubuntu 执行: sudo apt-get install -y xvfb")
	}
	for n := 99; n < 130; n++ {
		display := fmt.Sprintf(":%d", n)
		lock := filepath.Join("/tmp", fmt.Sprintf(".X%d-lock", n))
		if _, err := os.Stat(lock); err == nil {
			continue
		}
		cmd := exec.Command(bin, display, "-screen", "0", "1920x1080x24", "-ac", "-nolisten", "tcp")
		IsolateProcess(cmd)
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Start(); err != nil {
			continue
		}
		time.Sleep(200 * time.Millisecond)
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			continue
		}
		go func() { _ = cmd.Wait() }()
		if err := os.Setenv("DISPLAY", display); err != nil {
			_ = cmd.Process.Kill()
			return err
		}
		xvfbDisplay = display
		logf("服务器没有显示器，已启动虚拟显示 Xvfb %s，按有界面方式跑 Chrome", display)
		return nil
	}
	return fmt.Errorf("没有可用的 Xvfb 显示号")
}
