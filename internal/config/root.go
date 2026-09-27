package config

import (
	"os"
	"path/filepath"
	"strings"
)

func (c Config) AutomationURL() string {
	return strings.TrimRight(strings.TrimSpace(c.AutoURL), "/")
}

// FindRoot does not require automation assets; deployed manager needs only its
// config/data and optional web/dist next to the binary or in its parent.
func FindRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	starts := []string{cwd}
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	for _, start := range starts {
		for dir, n := start, 0; n < 8; n++ {
			for _, marker := range []string{"go.mod", "config.yaml", "config.yml", filepath.Join("web", "dist")} {
				if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
					return dir, nil
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return cwd, nil
}

func (c Config) PrepareDataDir() (Config, error) {
	dir, err := filepath.Abs(c.DataDir)
	if err != nil {
		return c, err
	}
	c.DataDir = dir
	return c, os.MkdirAll(dir, 0o755)
}
