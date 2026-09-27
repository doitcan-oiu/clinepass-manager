package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"opencode-go-manager/internal/config"
)

func checkHealth(cfg config.Config, url string) error {
	token := strings.TrimSpace(cfg.AutoToken)
	if token == "" {
		dir := strings.TrimSpace(cfg.AutoDataDir)
		if dir == "" {
			dir = "./auto/data"
		}
		raw, err := os.ReadFile(filepath.Join(dir, "auto-token"))
		if err != nil {
			return fmt.Errorf("读取 Auto 连接密钥失败: %w", err)
		}
		token = strings.TrimSpace(string(raw))
	}
	if token == "" {
		return fmt.Errorf("Auto 连接密钥为空")
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Auto 健康检查返回 HTTP %d", res.StatusCode)
	}
	var health struct {
		OK              bool   `json:"ok"`
		Service         string `json:"service"`
		ProtocolVersion int    `json:"protocol_version"`
	}
	if err := json.NewDecoder(res.Body).Decode(&health); err != nil {
		return fmt.Errorf("Auto 健康检查响应无效: %w", err)
	}
	if !health.OK || health.Service != "auto" || health.ProtocolVersion != 1 {
		return fmt.Errorf("Auto 健康检查协议不匹配")
	}
	return nil
}
