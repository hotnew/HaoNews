package newsplugin

import (
	"time"

	"hao.news/internal/apphost"
)

func ApplyDefaultConfig(cfg apphost.Config) apphost.Config {
	if cfg.RuntimeRoot == "" {
		if paths, err := DefaultRuntimePaths(); err == nil {
			cfg.RuntimeRoot = paths.Root
		}
	}
	runtime := RuntimePathsFromRoot(cfg.RuntimeRoot)
	if cfg.Project == "" {
		cfg.Project = "hao.news"
	}
	if cfg.Version == "" {
		cfg.Version = "dev"
	}
	if cfg.StoreRoot == "" {
		cfg.StoreRoot = runtime.StoreRoot
	}
	if cfg.ArchiveRoot == "" {
		cfg.ArchiveRoot = runtime.ArchiveRoot
	}
	if cfg.RulesPath == "" {
		cfg.RulesPath = runtime.RulesPath
	}
	if cfg.WriterPolicyPath == "" {
		cfg.WriterPolicyPath = runtime.WriterPolicyPath
	}
	if cfg.NetPath == "" {
		cfg.NetPath = runtime.NetPath
	}
	if cfg.TrackerPath == "" {
		cfg.TrackerPath = runtime.TrackerPath
	}
	if cfg.SyncBinaryPath == "" {
		cfg.SyncBinaryPath = runtime.SyncBinPath
	}
	if cfg.SyncMode == "" {
		cfg.SyncMode = string(SyncModeManaged)
	}
	if cfg.SyncStaleAfter <= 0 {
		cfg.SyncStaleAfter = 2 * time.Minute
	}
	return cfg
}
