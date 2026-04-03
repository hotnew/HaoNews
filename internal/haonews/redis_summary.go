package haonews

import (
	"context"
	"strings"
	"time"
)

type RedisSyncSummary struct {
	AnnouncementCount int `json:"announcement_count"`
	ChannelIndexCount int `json:"channel_index_count"`
	TopicIndexCount   int `json:"topic_index_count"`
	RealtimeQueueRefs int `json:"realtime_queue_refs"`
	HistoryQueueRefs  int `json:"history_queue_refs"`
}

func ReadRedisSyncSummary(cfg RedisConfig, timeout time.Duration) (RedisSyncSummary, error) {
	cfg = normalizeRedisConfig(cfg)
	if !cfg.Enabled {
		return RedisSyncSummary{}, nil
	}
	rc, err := NewRedisClient(cfg)
	if err != nil {
		return RedisSyncSummary{}, err
	}
	defer rc.Close()

	if timeout <= 0 {
		timeout = 1500 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return readRedisSyncSummary(ctx, rc)
}

func readRedisSyncSummary(ctx context.Context, rc *RedisClient) (RedisSyncSummary, error) {
	if rc == nil || !rc.Enabled() {
		return RedisSyncSummary{}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var summary RedisSyncSummary
	var err error
	if summary.AnnouncementCount, err = countRedisKeys(ctx, rc, rc.Key("sync", "ann", "*")); err != nil {
		return RedisSyncSummary{}, err
	}
	if summary.ChannelIndexCount, err = countRedisKeys(ctx, rc, rc.Key("sync", "channel", "*")); err != nil {
		return RedisSyncSummary{}, err
	}
	if summary.TopicIndexCount, err = countRedisKeys(ctx, rc, rc.Key("sync", "topic", "*")); err != nil {
		return RedisSyncSummary{}, err
	}
	if summary.RealtimeQueueRefs, err = redisListLength(ctx, rc, rc.Key("sync", "queue", "refs", "realtime")); err != nil {
		return RedisSyncSummary{}, err
	}
	if summary.HistoryQueueRefs, err = redisListLength(ctx, rc, rc.Key("sync", "queue", "refs", "history")); err != nil {
		return RedisSyncSummary{}, err
	}
	return summary, nil
}

func countRedisKeys(ctx context.Context, rc *RedisClient, pattern string) (int, error) {
	if rc == nil || !rc.Enabled() || strings.TrimSpace(pattern) == "" {
		return 0, nil
	}
	var (
		count  int
		cursor uint64
	)
	for {
		keys, next, err := rc.client.Scan(ctx, cursor, pattern, 256).Result()
		if err != nil {
			return 0, err
		}
		count += len(keys)
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return count, nil
}

func redisListLength(ctx context.Context, rc *RedisClient, key string) (int, error) {
	if rc == nil || !rc.Enabled() || strings.TrimSpace(key) == "" {
		return 0, nil
	}
	n, err := rc.client.LLen(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}
