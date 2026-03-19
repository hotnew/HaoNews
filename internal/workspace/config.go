package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	appManifestName    = "haonews.app.json"
	appConfigName      = "haonews.app.config.json"
	pluginManifestName = "haonews.plugin.json"
	pluginConfigName   = "haonews.plugin.config.json"
)

type AppConfig struct {
	Project          string `json:"project,omitempty"`
	Version          string `json:"version,omitempty"`
	Theme            string `json:"theme,omitempty"`
	RuntimeRoot      string `json:"runtime_root,omitempty"`
	StoreRoot        string `json:"store_root,omitempty"`
	ArchiveRoot      string `json:"archive_root,omitempty"`
	RulesPath        string `json:"rules_path,omitempty"`
	WriterPolicyPath string `json:"writer_policy_path,omitempty"`
	NetPath          string `json:"net_path,omitempty"`
	TrackerPath      string `json:"tracker_path,omitempty"`
	SyncMode         string `json:"sync_mode,omitempty"`
	SyncBinaryPath   string `json:"sync_binary_path,omitempty"`
	SyncStaleAfter   string `json:"sync_stale_after,omitempty"`
}

func (c AppConfig) Resolved(root string) (AppConfig, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return c, nil
	}
	if strings.TrimSpace(c.SyncStaleAfter) != "" {
		if _, err := time.ParseDuration(strings.TrimSpace(c.SyncStaleAfter)); err != nil {
			return AppConfig{}, fmt.Errorf("invalid sync_stale_after %q: %w", c.SyncStaleAfter, err)
		}
	}
	c.RuntimeRoot = resolvePath(root, c.RuntimeRoot)
	c.StoreRoot = resolvePath(root, c.StoreRoot)
	c.ArchiveRoot = resolvePath(root, c.ArchiveRoot)
	c.RulesPath = resolvePath(root, c.RulesPath)
	c.WriterPolicyPath = resolvePath(root, c.WriterPolicyPath)
	c.NetPath = resolvePath(root, c.NetPath)
	c.TrackerPath = resolvePath(root, c.TrackerPath)
	c.SyncBinaryPath = resolvePath(root, c.SyncBinaryPath)
	return c, nil
}

func (c AppConfig) SyncStaleAfterDuration() (time.Duration, error) {
	if strings.TrimSpace(c.SyncStaleAfter) == "" {
		return 0, nil
	}
	return time.ParseDuration(strings.TrimSpace(c.SyncStaleAfter))
}

func LoadAppConfig(root string) (AppConfig, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return AppConfig{}, fmt.Errorf("app directory is required")
	}
	data, err := os.ReadFile(filepath.Join(root, appConfigName))
	if err != nil {
		if os.IsNotExist(err) {
			return AppConfig{}, nil
		}
		return AppConfig{}, err
	}
	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return AppConfig{}, err
	}
	return cfg.Resolved(root)
}

func LoadPluginConfig(root string) (map[string]any, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("plugin directory is required")
	}
	data, err := os.ReadFile(filepath.Join(root, pluginConfigName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if len(cfg) == 0 {
		return nil, nil
	}
	return cfg, nil
}

func resolvePath(root, value string) string {
	value = strings.TrimSpace(value)
	if value == "" || filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(root, value)
}
