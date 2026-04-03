package haonews

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	redis "github.com/redis/go-redis/v9"
)

func syncAnnouncementRedisKey(rc *RedisClient, infoHash string) string {
	if rc == nil {
		return ""
	}
	infoHash = normalizeInfoHash(infoHash)
	if infoHash == "" {
		return ""
	}
	return rc.Key("sync", "ann", infoHash)
}

func syncChannelRedisKey(rc *RedisClient, channel string) string {
	if rc == nil {
		return ""
	}
	channel = strings.TrimSpace(channel)
	if channel == "" {
		return ""
	}
	return rc.Key("sync", "channel", channel)
}

func syncTopicRedisKey(rc *RedisClient, topic string) string {
	if rc == nil {
		return ""
	}
	topic = canonicalTopic(topic)
	if topic == "" {
		return ""
	}
	return rc.Key("sync", "topic", topic)
}

func syncQueueRedisKey(rc *RedisClient, queuePath string) string {
	if rc == nil {
		return ""
	}
	queuePath = strings.TrimSpace(queuePath)
	if queuePath == "" {
		return ""
	}
	base := strings.ToLower(filepath.Base(queuePath))
	token := base
	switch {
	case strings.HasSuffix(base, "history.txt"), strings.HasSuffix(base, ".history"):
		token = "history"
	case base == "realtime.txt":
		token = "realtime"
	}
	token = strings.NewReplacer(".", "-", "/", "-", "\\", "-", " ", "-").Replace(token)
	token = strings.Trim(token, "-")
	if token == "" {
		token = "refs"
	}
	return rc.Key("sync", "queue", "refs", token)
}

func syncAnnouncementScore(announcement SyncAnnouncement) float64 {
	createdAt := strings.TrimSpace(announcement.CreatedAt)
	if createdAt != "" {
		if ts, err := time.Parse(time.RFC3339, createdAt); err == nil {
			return float64(ts.Unix())
		}
		if ts, err := time.Parse("2006-01-02 15:04:05 MST", createdAt); err == nil {
			return float64(ts.Unix())
		}
		if unix, err := strconv.ParseInt(createdAt, 10, 64); err == nil {
			return float64(unix)
		}
	}
	return float64(time.Now().UTC().Unix())
}

func cacheSyncAnnouncement(ctx context.Context, rc *RedisClient, announcement SyncAnnouncement) error {
	if rc == nil || !rc.Enabled() {
		return nil
	}
	announcement = normalizeAnnouncement(announcement)
	key := syncAnnouncementRedisKey(rc, announcement.InfoHash)
	if strings.TrimSpace(key) == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	body, err := json.Marshal(announcement)
	if err != nil {
		return err
	}
	ttl := rc.DefaultTTL()
	score := syncAnnouncementScore(announcement)
	pipe := rc.client.TxPipeline()
	pipe.Set(ctx, key, body, ttl)
	if channelKey := syncChannelRedisKey(rc, announcement.Channel); channelKey != "" {
		pipe.ZAdd(ctx, channelKey, redis.Z{Score: score, Member: announcement.InfoHash})
		pipe.Expire(ctx, channelKey, ttl)
	}
	for _, topic := range announcement.Topics {
		if topicKey := syncTopicRedisKey(rc, topic); topicKey != "" {
			pipe.ZAdd(ctx, topicKey, redis.Z{Score: score, Member: announcement.InfoHash})
			pipe.Expire(ctx, topicKey, ttl)
		}
	}
	_, err = pipe.Exec(ctx)
	return err
}

func cacheSyncQueueRef(ctx context.Context, rc *RedisClient, queuePath string, ref SyncRef) error {
	if rc == nil || !rc.Enabled() {
		return nil
	}
	key := syncQueueRedisKey(rc, queuePath)
	if strings.TrimSpace(key) == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return rc.client.RPush(ctx, key, strings.TrimSpace(ref.Magnet)).Err()
}

func removeCachedSyncQueueRef(ctx context.Context, rc *RedisClient, queuePath string, ref SyncRef) error {
	if rc == nil || !rc.Enabled() {
		return nil
	}
	key := syncQueueRedisKey(rc, queuePath)
	if strings.TrimSpace(key) == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return rc.client.LRem(ctx, key, 0, strings.TrimSpace(ref.Magnet)).Err()
}

func loadCachedSyncAnnouncementsByChannel(ctx context.Context, rc *RedisClient, channel string, limit int64) ([]SyncAnnouncement, error) {
	if rc == nil || !rc.Enabled() {
		return nil, nil
	}
	key := syncChannelRedisKey(rc, channel)
	if strings.TrimSpace(key) == "" {
		return nil, nil
	}
	return loadCachedSyncAnnouncementsByIndex(ctx, rc, key, limit)
}

func loadCachedSyncAnnouncementsByTopic(ctx context.Context, rc *RedisClient, topic string, limit int64) ([]SyncAnnouncement, error) {
	if rc == nil || !rc.Enabled() {
		return nil, nil
	}
	key := syncTopicRedisKey(rc, topic)
	if strings.TrimSpace(key) == "" {
		return nil, nil
	}
	return loadCachedSyncAnnouncementsByIndex(ctx, rc, key, limit)
}

func loadCachedSyncQueueRefs(ctx context.Context, rc *RedisClient, queuePath string, limit int64) ([]string, error) {
	if rc == nil || !rc.Enabled() {
		return nil, nil
	}
	key := syncQueueRedisKey(rc, queuePath)
	if strings.TrimSpace(key) == "" {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 {
		limit = 100
	}
	return rc.client.LRange(ctx, key, 0, limit-1).Result()
}

func loadCachedSyncAnnouncementsByIndex(ctx context.Context, rc *RedisClient, indexKey string, limit int64) ([]SyncAnnouncement, error) {
	if rc == nil || !rc.Enabled() || strings.TrimSpace(indexKey) == "" {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 {
		limit = 20
	}
	infoHashes, err := rc.client.ZRevRange(ctx, indexKey, 0, limit-1).Result()
	if err != nil {
		return nil, err
	}
	if len(infoHashes) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(infoHashes))
	for _, infoHash := range infoHashes {
		if key := syncAnnouncementRedisKey(rc, infoHash); key != "" {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil, nil
	}
	values, err := rc.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	out := make([]SyncAnnouncement, 0, len(infoHashes))
	for _, raw := range values {
		if raw == nil {
			continue
		}
		var announcement SyncAnnouncement
		switch value := raw.(type) {
		case string:
			if err := json.Unmarshal([]byte(value), &announcement); err != nil {
				return nil, err
			}
		case []byte:
			if err := json.Unmarshal(value, &announcement); err != nil {
				return nil, err
			}
		default:
			body, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(body, &announcement); err != nil {
				return nil, err
			}
		}
		if strings.TrimSpace(announcement.InfoHash) == "" {
			continue
		}
		out = append(out, normalizeAnnouncement(announcement))
	}
	return out, nil
}
