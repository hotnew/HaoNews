package haonews

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	redis "github.com/redis/go-redis/v9"
)

type RedisClient struct {
	client *redis.Client
	cfg    RedisConfig
}

func ProbeRedis(cfg RedisConfig, timeout time.Duration) error {
	cfg = normalizeRedisConfig(cfg)
	if !cfg.Enabled {
		return nil
	}
	if timeout <= 0 {
		timeout = 1500 * time.Millisecond
	}
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		MaxRetries:   cfg.MaxRetries,
		DialTimeout:  time.Duration(cfg.DialTimeoutMs) * time.Millisecond,
		ReadTimeout:  time.Duration(cfg.ReadTimeoutMs) * time.Millisecond,
		WriteTimeout: time.Duration(cfg.WriteTimeoutMs) * time.Millisecond,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
	})
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return client.Ping(ctx).Err()
}

func NewRedisClient(cfg RedisConfig) (*RedisClient, error) {
	cfg = normalizeRedisConfig(cfg)
	if !cfg.Enabled {
		return nil, nil
	}
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		MaxRetries:   cfg.MaxRetries,
		DialTimeout:  time.Duration(cfg.DialTimeoutMs) * time.Millisecond,
		ReadTimeout:  time.Duration(cfg.ReadTimeoutMs) * time.Millisecond,
		WriteTimeout: time.Duration(cfg.WriteTimeoutMs) * time.Millisecond,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
	})
	if err := ProbeRedis(cfg, 5*time.Second); err != nil {
		_ = client.Close()
		return nil, err
	}
	return &RedisClient{client: client, cfg: cfg}, nil
}

func (r *RedisClient) Close() error {
	if r == nil || r.client == nil {
		return nil
	}
	return r.client.Close()
}

func (r *RedisClient) Enabled() bool {
	return r != nil && r.client != nil
}

func (r *RedisClient) Config() RedisConfig {
	if r == nil {
		return DefaultRedisConfig()
	}
	return r.cfg
}

func (r *RedisClient) Key(parts ...string) string {
	cfg := DefaultRedisConfig()
	if r != nil {
		cfg = r.cfg
	}
	prefix := cfg.KeyPrefix
	if prefix == "" {
		prefix = "haonews-"
	}
	return prefix + strings.Join(parts, ":")
}

func (r *RedisClient) DefaultTTL() time.Duration {
	if r == nil {
		return DefaultRedisConfig().HotWindow()
	}
	return r.cfg.HotWindow()
}

func (r *RedisClient) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	if !r.Enabled() {
		return nil
	}
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, body, ttl).Err()
}

func (r *RedisClient) GetJSON(ctx context.Context, key string, dest any) (bool, error) {
	if !r.Enabled() {
		return false, nil
	}
	body, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return false, nil
		}
		return false, err
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return false, err
	}
	return true, nil
}

func (r *RedisClient) Delete(ctx context.Context, keys ...string) error {
	if !r.Enabled() || len(keys) == 0 {
		return nil
	}
	return r.client.Del(ctx, keys...).Err()
}
