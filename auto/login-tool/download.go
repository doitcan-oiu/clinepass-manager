package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const chromeMetaURL = "https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions-with-downloads.json"

type chromeMeta struct {
	Channels map[string]struct {
		Version   string `json:"version"`
		Downloads struct {
			Chrome []struct {
				Platform string `json:"platform"`
				URL      string `json:"url"`
			} `json:"chrome"`
		} `json:"downloads"`
	} `json:"channels"`
}

func chromePlatform() string {
	switch {
	case runtime.GOOS == "windows" && runtime.GOARCH == "amd64":
		return "win64"
	case runtime.GOOS == "windows":
		return "win32"
	case runtime.GOOS == "linux" && runtime.GOARCH == "arm64":
		return "linux-arm64"
	case runtime.GOOS == "linux":
		return "linux64"
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
		return "mac-arm64"
	case runtime.GOOS == "darwin":
		return "mac-x64"
	default:
		return runtime.GOOS + "-" + runtime.GOARCH
	}
}

func cachedChrome(dir string) string {
	var names []string
	if runtime.GOOS == "windows" {
		names = []string{
			filepath.Join("chrome-win64", "chrome.exe"),
			filepath.Join("chrome-win32", "chrome.exe"),
			"chrome.exe",
		}
	} else {
		names = []string{
			filepath.Join("chrome-linux64", "chrome"),
			filepath.Join("chrome-linux-arm64", "chrome"),
			"chrome",
		}
	}
	for _, name := range names {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func ensureChrome(dir string) (string, error) {
	if p := cachedChrome(dir); p != "" {
		fmt.Printf("使用已下载的 Chrome：%s\n", p)
		return p, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	plat := chromePlatform()
	fmt.Printf("本机没有缓存 Chrome，正在下载 Chrome for Testing（%s）...\n", plat)
	url, ver, err := chromeDownloadURL(plat)
	if err != nil {
		return "", err
	}
	fmt.Printf("版本 %s\n%s\n", ver, url)
	zipPath := filepath.Join(dir, "chrome.zip")
	if err := downloadFile(url, zipPath); err != nil {
		return "", err
	}
	if err := unzip(zipPath, dir); err != nil {
		return "", err
	}
	_ = os.Remove(zipPath)
	p := cachedChrome(dir)
	if p == "" {
		return "", fmt.Errorf("解压后没有找到 chrome 可执行文件，目录：%s", dir)
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(p, 0o755)
	}
	fmt.Printf("Chrome 已就绪：%s\n", p)
	return p, nil
}

func chromeDownloadURL(platform string) (string, string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(chromeMetaURL)
	if err != nil {
		return "", "", fmt.Errorf("查询 Chrome 下载地址失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("查询 Chrome 下载地址失败: HTTP %d", resp.StatusCode)
	}
	var meta chromeMeta
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return "", "", fmt.Errorf("解析 Chrome 下载列表失败: %w", err)
	}
	ch, ok := meta.Channels["Stable"]
	if !ok {
		return "", "", fmt.Errorf("下载列表没有 Stable 通道")
	}
	for _, item := range ch.Downloads.Chrome {
		if item.Platform == platform {
			return item.URL, ch.Version, nil
		}
	}
	return "", "", fmt.Errorf("没有 %s 的 Chrome 安装包", platform)
}

func downloadFile(url, dest string) error {
	tmp := dest + ".part"
	_ = os.Remove(tmp)
	client := &http.Client{Timeout: 20 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	total := resp.ContentLength
	var written int64
	buf := make([]byte, 32*1024)
	last := time.Now()
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				_ = f.Close()
				return err
			}
			written += int64(n)
			if time.Since(last) > time.Second {
				printDownloadProgress(written, total)
				last = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = f.Close()
			return readErr
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	printDownloadProgress(written, total)
	fmt.Println()
	return os.Rename(tmp, dest)
}

func printDownloadProgress(written, total int64) {
	if total > 0 {
		fmt.Printf("\r已下载 %.1f / %.1f MB", float64(written)/1024/1024, float64(total)/1024/1024)
		return
	}
	fmt.Printf("\r已下载 %.1f MB", float64(written)/1024/1024)
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("打开压缩包失败: %w", err)
	}
	defer r.Close()
	for _, f := range r.File {
		name := filepath.Clean(f.Name)
		if strings.HasPrefix(name, "..") {
			continue
		}
		target := filepath.Join(dest, name)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) && target != filepath.Clean(dest) {
			continue
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		_ = out.Close()
		_ = rc.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}
