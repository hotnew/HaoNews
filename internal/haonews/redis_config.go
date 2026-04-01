package haonews

import "time"

type RedisConfig struct {
	Enabled        bool   `json:"enabled"`
	Addr           string `json:"addr"`
	Password       string `json:"password,omitempty"`
	DB             int    `json:"db"`
	KeyPrefix      string `json:"key_prefix"`
	MaxRetries     int    `json:"max_retries"`
	DialTimeoutMs  int    `json:"dial_timeout_ms"`
	ReadTimeoutMs  int    `json:"read_timeout_ms"`
	WriteTimeoutMs int    `json:"write_timeout_ms"`
	PoolSize       int    `json:"pool_size"`
	MinIdleConns   int    `json:"min_idle_conns"`
	HotWindowDays  int    `json:"hot_window_days"`
}

func DefaultRedisConfig() RedisConfig {
	return RedisConfig{
		Enabled:        false,
		Addr:           "127.0.0.1:6379",
		DB:             0,
		KeyPrefix:      "haonews-",
		MaxRetries:     3,
		DialTimeoutMs:  3000,
		ReadTimeoutMs:  2000,
		WriteTimeoutMs: 2000,
		PoolSize:       10,
		MinIdleConns:   2,
		HotWindowDays:  7,
	}
}

func normalizeRedisConfig(cfg RedisConfig) RedisConfig {
	normalized := cfg
	defaults := DefaultRedisConfig()
	if normalized.Addr == "" {
		normalized.Addr = defaults.Addr
	}
	if normalized.KeyPrefix == "" {
		normalized.KeyPrefix = defaults.KeyPrefix
	}
	if normalized.MaxRetries <= 0 {
		normalized.MaxRetries = defaults.MaxRetries
	}
	if normalized.DialTimeoutMs <= 0 {
		normalized.DialTimeoutMs = defaults.DialTimeoutMs
	}
	if normalized.ReadTimeoutMs <= 0 {
		normalized.ReadTimeoutMs = defaults.ReadTimeoutMs
	}
	if normalized.WriteTimeoutMs <= 0 {
		normalized.WriteTimeoutMs = defaults.WriteTimeoutMs
	}
	if normalized.PoolSize <= 0 {
		normalized.PoolSize = defaults.PoolSize
	}
	if normalized.MinIdleConns < 0 {
		normalized.MinIdleConns = defaults.MinIdleConns
	}
	if normalized.HotWindowDays <= 0 {
		normalized.HotWindowDays = defaults.HotWindowDays
	}
	return normalized
}

func (c RedisConfig) Normalized() RedisConfig {
	return normalizeRedisConfig(c)
}

func (c RedisConfig) HotWindow() time.Duration {
	cfg := normalizeRedisConfig(c)
	return time.Duration(cfg.HotWindowDays) * 24 * time.Hour
}

func (c RedisConfig) ShortTTL() time.Duration {
	return 30 * time.Second
}
