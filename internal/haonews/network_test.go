package haonews

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadNetworkBootstrapConfigParsesRedisConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "hao_news_net.inf")
	content := `network_mode=shared
redis_enabled=true
redis_addr=192.168.102.223:6379
redis_password=
redis_db=4
redis_key_prefix=haonews-test:
redis_max_retries=5
redis_dial_timeout_ms=4500
redis_read_timeout_ms=2500
redis_write_timeout_ms=2600
redis_pool_size=20
redis_min_idle_conns=3
redis_hot_window_days=9
redis_max_announcements=321
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write net config: %v", err)
	}

	cfg, err := LoadNetworkBootstrapConfig(path)
	if err != nil {
		t.Fatalf("LoadNetworkBootstrapConfig error = %v", err)
	}
	if !cfg.Redis.Enabled {
		t.Fatalf("redis should be enabled: %+v", cfg.Redis)
	}
	if cfg.Redis.Addr != "192.168.102.223:6379" {
		t.Fatalf("redis addr = %q", cfg.Redis.Addr)
	}
	if cfg.Redis.DB != 4 {
		t.Fatalf("redis db = %d", cfg.Redis.DB)
	}
	if cfg.Redis.KeyPrefix != "haonews-test:" {
		t.Fatalf("redis key prefix = %q", cfg.Redis.KeyPrefix)
	}
	if cfg.Redis.MaxRetries != 5 {
		t.Fatalf("redis max retries = %d", cfg.Redis.MaxRetries)
	}
	if cfg.Redis.HotWindowDays != 9 {
		t.Fatalf("redis hot window = %d", cfg.Redis.HotWindowDays)
	}
	if cfg.Redis.MaxAnnouncements != 321 {
		t.Fatalf("redis max announcements = %d", cfg.Redis.MaxAnnouncements)
	}
}
