package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"opencode-go-manager/auto/internal/api"
	"opencode-go-manager/auto/internal/browser"
	"opencode-go-manager/auto/internal/job"
	"opencode-go-manager/auto/internal/login"
	"opencode-go-manager/internal/config"
	"opencode-go-manager/internal/store"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
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
	// The installer uses the same YAML/env parsing and token file as the
	// service. The credential never appears in process arguments.
	if len(os.Args) == 3 && os.Args[1] == "--health-check" {
		if err := checkHealth(cfg, os.Args[2]); err != nil {
			log.Fatal(err)
		}
		return
	}
	if strings.TrimSpace(cfg.AutoAddr) == "" {
		cfg.AutoAddr = ":9998"
	}
	// Auto owns a separate database, browser profiles, cards and secrets.
	cfg.DataDir = strings.TrimSpace(cfg.AutoDataDir)
	if cfg.DataDir == "" {
		cfg.DataDir = "./auto/data"
	}
	// Claim listener before provisioning credentials or scheduling work.
	ln, err := net.Listen("tcp", cfg.AutoAddr)
	if err != nil {
		log.Fatalf("监听 auto %s 失败: %v", cfg.AutoAddr, err)
	}
	defer ln.Close()
	token, created, err := ensureToken(cfg.DataDir, cfg.AutoToken)
	if err != nil {
		log.Fatalf("准备 Auto 连接密钥失败: %v", err)
	}
	cfg.AutoToken = token
	if created {
		log.Printf("首次启动 Auto，连接密钥：%s（请在主程序连接设置中填写；仅首次显示，保存在 %s/auto-token）", token, cfg.DataDir)
	}
	prepared, note, err := cfg.PrepareRuntime()
	if err != nil {
		log.Fatalf("准备自动化运行目录失败: %v", err)
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
		log.Fatal(err)
	}
	if settings, err := st.GetSettings(); err == nil {
		cfg = store.ApplySettings(cfg, settings)
	}
	// Payment worker calls auto directly, independent of manager's lifetime.
	cfg.Addr = cfg.AutoAddr
	jobs := job.New(cfg, st)
	srv := api.New(cfg, st, jobs)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go srv.Run(ctx)
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("ClinePass Auto 监听 %s；数据库 %s；登录引擎 %s", cfg.AutoAddr, cfg.DBPath(), login.Engine())
	if hint := browser.StartupHint(cfg); hint != "" {
		log.Print(hint)
	}
	if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
