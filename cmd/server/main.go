package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"opencode-go-manager/internal/api"
	"opencode-go-manager/internal/browser"
	"opencode-go-manager/internal/config"
	"opencode-go-manager/internal/job"
	"opencode-go-manager/internal/login"
	"opencode-go-manager/internal/store"
)

func main() {
	root, err := login.FindRepoRoot()
	if err != nil {
		log.Fatalf("找不到项目根目录: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		log.Fatalf("进入项目根目录失败: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("读取配置失败: %v", err)
	}
	if cfg.ConfigFile != "" {
		log.Printf("配置文件 %s", cfg.ConfigFile)
	}
	prepared, note, err := cfg.PrepareRuntime()
	if err != nil {
		log.Fatalf("准备运行目录失败: %v", err)
	}
	cfg = prepared
	if note != "" {
		log.Print(note)
	}

	st, err := store.Open(cfg.DBPath())
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer st.Close()

	if err := st.SeedDefaults(cfg); err != nil {
		log.Fatalf("初始化设置失败: %v", err)
	}
	jobs := job.New(cfg, st)
	if stg, err := st.GetSettings(); err == nil {
		cfg = store.ApplySettings(cfg, stg)
	}
	webRoot := filepath.Join(root, "web", "dist")
	srv := api.New(cfg, st, jobs, webRoot)

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		log.Fatalf("监听 %s 失败: %v", cfg.Addr, err)
	}
	log.Printf("ClinePass Manager 监听 %s  根目录=%s", cfg.Addr, root)
	log.Printf("邀请链接: %s  headless=%v  concurrent=%d  登录引擎=%s", cfg.InviteURL, cfg.Headless, cfg.MaxConcurrent, login.Engine())
	if hint := browser.StartupHint(cfg); hint != "" {
		log.Printf("浏览器环境: %s", hint)
	}

	go prepareCloak(cfg, jobs)

	if err := http.Serve(ln, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}

func prepareCloak(cfg config.Config, jobs *job.Manager) {
	log.Printf("后台准备 CloakBrowser %s → %s", cfg.CloakVersion, cfg.CloakCacheDir)
	info, err := browser.EnsureBinary(cfg.CloakVersion, cfg.CloakCacheDir, cfg.CloakBinaryPath, browser.ResolveLicense(cfg.LicenseKey), log.Printf)
	if err != nil {
		log.Printf("启动时未能备好 CloakBrowser（登录时会再试）: %v", err)
		return
	}
	if err := os.Setenv("CLOAKBROWSER_BINARY_PATH", info.Path); err != nil {
		log.Printf("写入 CLOAKBROWSER_BINARY_PATH 失败: %v", err)
	}
	jobs.SetCloakBinaryPath(info.Path)
	log.Printf("CloakBrowser 就绪: %s", info.Path)
}
