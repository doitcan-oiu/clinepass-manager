package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type fileConfig struct {
	Addr            *flexString `yaml:"addr"`
	DataDir         *string     `yaml:"data_dir"`
	InviteURL       *string     `yaml:"invite_url"`
	Headless        *bool       `yaml:"headless"`
	SlowMo          *float64    `yaml:"slow_mo"`
	Proxy           *string     `yaml:"proxy"`
	MaxConcurrent   *int        `yaml:"max_concurrent"`
	MaxRetries      *int        `yaml:"max_retries"`
	LoginEngine     *string     `yaml:"login_engine"`
	LoginPython     *string     `yaml:"login_python"`
	CloakVersion    *string     `yaml:"cloakbrowser_version"`
	CloakCacheDir   *string     `yaml:"cloakbrowser_cache_dir"`
	CloakBinaryPath *string     `yaml:"cloakbrowser_binary_path"`
	LicenseKey      *string     `yaml:"cloakbrowser_license_key"`
}

type flexString string

func (s *flexString) UnmarshalYAML(value *yaml.Node) error {
	if value == nil || value.Kind != yaml.ScalarNode {
		return fmt.Errorf("addr 需要写成字符串或端口号，例如 \":9999\"")
	}
	*s = flexString(value.Value)
	return nil
}

func discoverFile() (path string, required bool, err error) {
	if p := strings.TrimSpace(os.Getenv("CONFIG_FILE")); p != "" {
		return p, true, nil
	}
	for _, name := range []string{"config.yaml", "config.yml"} {
		st, e := os.Stat(name)
		if e == nil && !st.IsDir() {
			return name, false, nil
		}
		if e != nil && !os.IsNotExist(e) {
			return "", false, e
		}
	}
	return "", false, nil
}

func loadFile(path string, c *Config) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var f fileConfig
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	if f.Addr != nil {
		c.Addr = string(*f.Addr)
	}
	setStr := func(dst *string, src *string) {
		if src != nil {
			*dst = strings.TrimSpace(*src)
		}
	}
	setStr(&c.DataDir, f.DataDir)
	setStr(&c.InviteURL, f.InviteURL)
	setStr(&c.Proxy, f.Proxy)
	setStr(&c.LoginEngine, f.LoginEngine)
	setStr(&c.LoginPython, f.LoginPython)
	setStr(&c.CloakVersion, f.CloakVersion)
	setStr(&c.CloakCacheDir, f.CloakCacheDir)
	setStr(&c.CloakBinaryPath, f.CloakBinaryPath)
	setStr(&c.LicenseKey, f.LicenseKey)
	if f.Headless != nil {
		c.Headless = *f.Headless
	}
	if f.SlowMo != nil {
		c.SlowMo = *f.SlowMo
	}
	if f.MaxConcurrent != nil {
		c.MaxConcurrent = *f.MaxConcurrent
	}
	if f.MaxRetries != nil {
		c.MaxRetries = *f.MaxRetries
	}
	return nil
}
