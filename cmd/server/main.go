package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"opencode-go-manager/internal/api"
	"opencode-go-manager/internal/config"
	"opencode-go-manager/internal/job"
	"opencode-go-manager/internal/store"
)

func main() {
	cfg := config.Load()
	for _, dir := range []string{cfg.DataDir, cfg.ProfilesDir(), cfg.ScreenshotsDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("创建目录失败: %v", err)
		}
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
	log.Printf("邀请链接: %s  headless=%v  concurrent=%d", cfg.InviteURL, cfg.Headless, cfg.MaxConcurrent)
	if err := http.ListenAndServe(cfg.Addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}
