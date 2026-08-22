package main

import (
	"log"
	"net/http"
	"path/filepath"

	"opencode-go-manager/internal/api"
	"opencode-go-manager/internal/browser"
	"opencode-go-manager/internal/config"
	"opencode-go-manager/internal/job"
	"opencode-go-manager/internal/login"
	"opencode-go-manager/internal/store"
)

func main() {
	cfg := config.Load()
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
	webRoot := filepath.Join("web", "dist")
	srv := api.New(cfg, st, jobs, webRoot)

	log.Printf("ClinePass Manager 监听 %s", cfg.Addr)
	log.Printf("邀请链接: %s  headless=%v  concurrent=%d  登录引擎=%s", cfg.InviteURL, cfg.Headless, cfg.MaxConcurrent, login.Engine())
	if hint := browser.StartupHint(cfg); hint != "" {
		log.Printf("浏览器环境: %s", hint)
	}
	if err := http.ListenAndServe(cfg.Addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}
