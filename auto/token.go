package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ensureToken never silently replaces an existing credential. A generated token
// remains stable across restarts and is printed only by the creating process.
func ensureToken(dir, configured string) (string, bool, error) {
	if token := strings.TrimSpace(configured); token != "" {
		return token, false, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", false, err
	}
	path := filepath.Join(dir, "auto-token")
	read := func() (string, bool, error) {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", false, err
		}
		token := strings.TrimSpace(string(raw))
		if token == "" {
			return "", false, fmt.Errorf("%s 为空，请设置 AUTO_TOKEN 或移除空文件后重新启动", path)
		}
		return token, false, nil
	}
	if _, err := os.Stat(path); err == nil {
		return read()
	} else if !os.IsNotExist(err) {
		return "", false, err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", false, err
	}
	token := hex.EncodeToString(raw)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		return read()
	}
	if err != nil {
		return "", false, err
	}
	_, writeErr := f.WriteString(token + "\n")
	closeErr := f.Close()
	if writeErr != nil {
		return "", false, writeErr
	}
	if closeErr != nil {
		return "", false, closeErr
	}
	return token, true, nil
}
