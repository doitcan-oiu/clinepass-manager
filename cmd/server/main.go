package main

import (
	"context"
	"log"
	"net/http"
	"opencode-go-manager/internal/api"
	"opencode-go-manager/internal/config"
	"opencode-go-manager/internal/store"
	"os"
	"path/filepath"
)

func main() {
	root, err := config.FindRoot()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		log.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("读取配置失败: %v", err)
	}
	cfg, err = cfg.PrepareDataDir()
	if err != nil {
		log.Fatalf("准备数据目录失败: %v", err)
	}
	st, err := store.Open(cfg.DBPath())
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer st.Close()
	if err := st.SeedDefaults(cfg); err != nil {
		log.Fatalf("初始化设置失败: %v", err)
	}
	if settings, err := st.GetSettings(); err == nil {
		cfg = store.ApplySettings(cfg, settings)
	}
	srv := api.New(cfg, st, filepath.Join(root, "web", "dist"))
	go srv.RunModelCatalog(context.Background())
	log.Printf("ClinePass Manager 监听 %s；数据库 %s；Auto 连接由网页设置管理", cfg.Addr, cfg.DBPath())
	if err := http.ListenAndServe(cfg.Addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}
