package browser

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	defaultVersion = "151.0.7922.108.2"
	downloadBase   = "https://cloakbrowser.dev"
	githubBase     = "https://github.com/CloakHQ/cloakbrowser/releases/download"
	signingKeyB64  = "MKFKwIhUcKWq5xTuNA0Ovg99njcDEcEJvmWYYhApvaU="
)

var ensureMu sync.Mutex

type BinaryInfo struct {
	Path     string `json:"path"`
	Version  string `json:"version"`
	Platform string `json:"platform"`
	Cached   bool   `json:"cached"`
}

func PlatformTag() string {
	switch runtime.GOOS {
	case "linux":
		if runtime.GOARCH == "arm64" {
			return "linux-arm64"
		}
		return "linux-x64"
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return "darwin-arm64"
		}
		return "darwin-x64"
	case "windows":
		return "windows-x64"
	default:
		return runtime.GOOS + "-" + runtime.GOARCH
	}
}

func CacheDir(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cloakbrowser"), nil
}

func chromeName() string {
	switch runtime.GOOS {
	case "windows":
		return "chrome.exe"
	case "darwin":
		return filepath.Join("Chromium.app", "Contents", "MacOS", "Chromium")
	default:
		return "chrome"
	}
}

func proDownloadURL(version string) string {
	return downloadBase + "/api/download/" + version
}

func freeDownloadURLs(version string) []string {
	name := archiveName()
	return []string{
		fmt.Sprintf("%s/chromium-v%s/%s", downloadBase, version, name),
		fmt.Sprintf("%s/chromium-v%s/%s", githubBase, version, name),
	}
}

func needsLicense(version string) bool {
	return !strings.HasPrefix(version, "145.") && !strings.HasPrefix(version, "146.")
}

func cachedBinaryPath(dir, version string) string {
	for _, name := range []string{"chromium-" + version + "-pro", "chromium-" + version} {
		p := filepath.Join(dir, name, chromeName())
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func archiveName() string {
	if runtime.GOOS == "windows" {
		return "cloakbrowser-" + PlatformTag() + ".zip"
	}
	return "cloakbrowser-" + PlatformTag() + ".tar.gz"
}

func DefaultStealthArgs(seed int) []string {
	if seed <= 0 {
		seed = 10000 + int(time.Now().UnixNano()%90000)
	}
	args := []string{
		"--no-sandbox",
		fmt.Sprintf("--fingerprint=%d", seed),
		"--fingerprint-storage-quota=5000",
		"--ignore-gpu-blocklist",
		"--fingerprint-windows-font-metrics",
		"--fingerprint-allow-3p-cookies",
	}
	if runtime.GOOS == "darwin" {
		args = append(args, "--fingerprint-platform=macos")
	} else {
		args = append(args, "--fingerprint-platform=windows")
	}
	return args
}

func IgnoreDefaultArgs() []string {
	return []string{"--enable-automation", "--enable-unsafe-swiftshader"}
}

func EnsureBinary(version, cacheDir, overridePath, license string, logf func(string, ...any)) (BinaryInfo, error) {
	ensureMu.Lock()
	defer ensureMu.Unlock()

	if overridePath != "" {
		if _, err := os.Stat(overridePath); err != nil {
			return BinaryInfo{}, fmt.Errorf("CLOAKBROWSER_BINARY_PATH 无效: %w", err)
		}
		return BinaryInfo{Path: overridePath, Version: version, Platform: PlatformTag(), Cached: true}, nil
	}
	if version == "" {
		version = defaultVersion
	}
	license = ResolveLicense(license)
	dir, err := CacheDir(cacheDir)
	if err != nil {
		return BinaryInfo{}, err
	}
	if p := cachedBinaryPath(dir, version); p != "" {
		return BinaryInfo{Path: p, Version: version, Platform: PlatformTag(), Cached: true}, nil
	}

	if logf == nil {
		logf = func(string, ...any) {}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return BinaryInfo{}, err
	}

	pro := license != ""
	if !pro && needsLicense(version) {
		return BinaryInfo{}, fmt.Errorf("CloakBrowser %s 是 Pro 包，GitHub 公开地址没有 tar.gz（只有校验文件，直链会 404）。请填写 cloakbrowser_license_key，走官方 %s", version, proDownloadURL(version))
	}

	tmp, err := os.CreateTemp("", "cloakbrowser-*"+filepath.Ext(archiveName()))
	if err != nil {
		return BinaryInfo{}, err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	var lastErr error
	if pro {
		u := proDownloadURL(version)
		logf("用 license 从官方接口下载 CloakBrowser %s：%s", version, u)
		lastErr = downloadFile(u, tmpPath, logf, map[string]string{
			"Authorization": "Bearer " + license,
			"X-Platform":    PlatformTag(),
		})
		if lastErr == nil {
			lastErr = verifyArchive(tmpPath, version, true, logf)
		}
	} else {
		for _, u := range freeDownloadURLs(version) {
			logf("下载免费 CloakBrowser %s：%s", version, u)
			if err := downloadFile(u, tmpPath, logf, nil); err != nil {
				lastErr = err
				continue
			}
			if err := verifyArchive(tmpPath, version, false, logf); err != nil {
				lastErr = err
				continue
			}
			lastErr = nil
			break
		}
	}
	if lastErr != nil {
		return BinaryInfo{}, fmt.Errorf("下载 CloakBrowser 失败: %w", lastErr)
	}

	binDir := filepath.Join(dir, "chromium-"+version)
	if pro {
		binDir = filepath.Join(dir, "chromium-"+version+"-pro")
	}
	binPath := filepath.Join(binDir, chromeName())
	logf("解压 CloakBrowser 到 %s", binDir)
	if err := extractArchive(tmpPath, binDir); err != nil {
		return BinaryInfo{}, err
	}
	flattenSingleDir(binDir)
	if err := os.Chmod(binPath, 0o755); err != nil && runtime.GOOS != "windows" {
		if _, statErr := os.Stat(binPath); statErr != nil {
			return BinaryInfo{}, fmt.Errorf("解压后未找到 chrome: %s", binPath)
		}
	}
	if _, err := os.Stat(binPath); err != nil {
		return BinaryInfo{}, fmt.Errorf("解压后未找到 chrome: %s", binPath)
	}
	return BinaryInfo{Path: binPath, Version: version, Platform: PlatformTag(), Cached: false}, nil
}

func downloadFile(url, dest string, logf func(string, ...any), headers map[string]string) error {
	client := &http.Client{Timeout: 15 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "opencode-go-manager")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%s -> HTTP %d（license 无效或无权下载这个版本）", url, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s -> HTTP %d", url, resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return err
	}
	logf("已下载 %.1f MB", float64(n)/1024/1024)
	return nil
}

func verifyArchive(path, version string, pro bool, logf func(string, ...any)) error {
	manifest, sig, err := fetchSignedManifest(version, pro)
	if err != nil {
		return fmt.Errorf("无法获取签名清单: %w", err)
	}
	if err := verifyEd25519(manifest, sig); err != nil {
		return err
	}
	text := string(manifest)
	if declared := parseManifestVersion(text); declared != "" && declared != version {
		return fmt.Errorf("清单版本不匹配: 期望 %s, 实际 %s", version, declared)
	}
	sums := parseChecksums(text)
	want, ok := sums[archiveName()]
	if !ok {
		return fmt.Errorf("SHA256SUMS 中没有 %s", archiveName())
	}
	got, err := fileSHA256(path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("SHA256 不匹配")
	}
	logf("CloakBrowser 二进制校验通过（Ed25519 + SHA256）")
	return nil
}

func fetchSignedManifest(version string, pro bool) ([]byte, []byte, error) {
	var bases []string
	if pro {
		bases = []string{
			downloadBase + "/releases/pro/chromium-v" + version,
			githubBase + "/chromium-v" + version + "-pro",
		}
	} else {
		bases = []string{
			fmt.Sprintf("%s/chromium-v%s", downloadBase, version),
			fmt.Sprintf("%s/chromium-v%s", githubBase, version),
		}
	}
	client := &http.Client{Timeout: 30 * time.Second}
	var last error
	for _, base := range bases {
		manifest, err := httpGet(client, base+"/SHA256SUMS")
		if err != nil {
			last = err
			continue
		}
		sig, err := httpGet(client, base+"/SHA256SUMS.sig")
		if err != nil {
			last = err
			continue
		}
		return manifest, sig, nil
	}
	if last == nil {
		last = fmt.Errorf("no source")
	}
	return nil, nil, last
}

func httpGet(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s -> HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func verifyEd25519(manifest, sigB64 []byte) error {
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigB64)))
	if err != nil {
		return fmt.Errorf("SHA256SUMS.sig 不是合法 base64")
	}
	pubRaw, err := base64.StdEncoding.DecodeString(signingKeyB64)
	if err != nil || len(pubRaw) != ed25519.PublicKeySize {
		return fmt.Errorf("内置公钥无效")
	}
	if !ed25519.Verify(ed25519.PublicKey(pubRaw), manifest, sig) {
		return fmt.Errorf("Ed25519 签名校验失败")
	}
	return nil
}

func parseManifestVersion(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "version=") {
			return strings.TrimSpace(strings.TrimPrefix(line, "version="))
		}
	}
	return ""
}

func parseChecksums(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "version=") || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && len(fields[0]) == 64 {
			out[fields[1]] = fields[0]
		}
	}
	return out
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
