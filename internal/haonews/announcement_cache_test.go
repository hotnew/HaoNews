package haonews

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestCacheSyncAnnouncement(t *testing.T) {
	mini, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run error = %v", err)
	}
	defer mini.Close()

	rc, err := NewRedisClient(RedisConfig{
		Enabled:   true,
		Addr:      mini.Addr(),
		KeyPrefix: "haonews-test-",
	})
	if err != nil {
		t.Fatalf("NewRedisClient error = %v", err)
	}
	defer rc.Close()

	announcement := SyncAnnouncement{
		InfoHash:  "0123456789abcdef0123456789abcdef01234567",
		Ref:       "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567",
		Title:     "hello",
		Channel:   "news",
		Topics:    []string{"world", "news"},
		CreatedAt: time.Unix(1711929600, 0).UTC().Format(time.RFC3339),
	}
	if err := cacheSyncAnnouncement(context.Background(), rc, announcement); err != nil {
		t.Fatalf("cacheSyncAnnouncement error = %v", err)
	}

	var loaded SyncAnnouncement
	ok, err := rc.GetJSON(context.Background(), syncAnnouncementRedisKey(rc, announcement.InfoHash), &loaded)
	if err != nil {
		t.Fatalf("GetJSON error = %v", err)
	}
	if !ok {
		t.Fatalf("expected cached announcement")
	}
	if loaded.InfoHash != announcement.InfoHash || loaded.Title != announcement.Title {
		t.Fatalf("loaded announcement = %+v", loaded)
	}

	channelMembers, err := rc.client.ZRevRange(context.Background(), syncChannelRedisKey(rc, "news"), 0, -1).Result()
	if err != nil {
		t.Fatalf("ZRevRange(channel) error = %v", err)
	}
	if len(channelMembers) != 1 || channelMembers[0] != announcement.InfoHash {
		t.Fatalf("channel members = %#v", channelMembers)
	}

	topicMembers, err := rc.client.ZRevRange(context.Background(), syncTopicRedisKey(rc, "world"), 0, -1).Result()
	if err != nil {
		t.Fatalf("ZRevRange(topic) error = %v", err)
	}
	if len(topicMembers) != 1 || topicMembers[0] != announcement.InfoHash {
		t.Fatalf("topic members = %#v", topicMembers)
	}
}

func TestCacheSyncQueueRefAndRemoveCachedSyncQueueRef(t *testing.T) {
	mini, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run error = %v", err)
	}
	defer mini.Close()

	rc, err := NewRedisClient(RedisConfig{
		Enabled:   true,
		Addr:      mini.Addr(),
		KeyPrefix: "haonews-test-",
	})
	if err != nil {
		t.Fatalf("NewRedisClient error = %v", err)
	}
	defer rc.Close()

	ref := SyncRef{
		InfoHash: "93a71a010a59022c8670e06e2c92fa279f98d974",
		Magnet:   "haonews-sync://bundle/93a71a010a59022c8670e06e2c92fa279f98d974?dn=test",
	}
	queuePath := "/tmp/realtime.txt"
	if err := cacheSyncQueueRef(context.Background(), rc, queuePath, ref); err != nil {
		t.Fatalf("cacheSyncQueueRef error = %v", err)
	}
	items, err := rc.client.LRange(context.Background(), syncQueueRedisKey(rc, queuePath), 0, -1).Result()
	if err != nil {
		t.Fatalf("LRange error = %v", err)
	}
	if len(items) != 1 || items[0] != ref.Magnet {
		t.Fatalf("queue items = %#v", items)
	}
	if err := removeCachedSyncQueueRef(context.Background(), rc, queuePath, ref); err != nil {
		t.Fatalf("removeCachedSyncQueueRef error = %v", err)
	}
	items, err = rc.client.LRange(context.Background(), syncQueueRedisKey(rc, queuePath), 0, -1).Result()
	if err != nil {
		t.Fatalf("LRange(after remove) error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("queue items after remove = %#v", items)
	}
}

func TestLoadCachedSyncAnnouncementsByTopicAndChannel(t *testing.T) {
	mini, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run error = %v", err)
	}
	defer mini.Close()

	rc, err := NewRedisClient(RedisConfig{
		Enabled:   true,
		Addr:      mini.Addr(),
		KeyPrefix: "haonews-test-",
	})
	if err != nil {
		t.Fatalf("NewRedisClient error = %v", err)
	}
	defer rc.Close()

	older := SyncAnnouncement{
		InfoHash:  "1111111111111111111111111111111111111111",
		Ref:       "magnet:?xt=urn:btih:1111111111111111111111111111111111111111",
		Title:     "older",
		Channel:   "news",
		Topics:    []string{"world"},
		CreatedAt: time.Unix(1711929600, 0).UTC().Format(time.RFC3339),
	}
	newer := SyncAnnouncement{
		InfoHash:  "2222222222222222222222222222222222222222",
		Ref:       "magnet:?xt=urn:btih:2222222222222222222222222222222222222222",
		Title:     "newer",
		Channel:   "news",
		Topics:    []string{"world"},
		CreatedAt: time.Unix(1711933200, 0).UTC().Format(time.RFC3339),
	}
	if err := cacheSyncAnnouncement(context.Background(), rc, older); err != nil {
		t.Fatalf("cacheSyncAnnouncement(older) error = %v", err)
	}
	if err := cacheSyncAnnouncement(context.Background(), rc, newer); err != nil {
		t.Fatalf("cacheSyncAnnouncement(newer) error = %v", err)
	}

	channelAnnouncements, err := loadCachedSyncAnnouncementsByChannel(context.Background(), rc, "news", 10)
	if err != nil {
		t.Fatalf("loadCachedSyncAnnouncementsByChannel error = %v", err)
	}
	if len(channelAnnouncements) != 2 {
		t.Fatalf("channel announcements len = %d", len(channelAnnouncements))
	}
	if channelAnnouncements[0].InfoHash != newer.InfoHash || channelAnnouncements[1].InfoHash != older.InfoHash {
		t.Fatalf("channel announcements order = %#v", channelAnnouncements)
	}

	topicAnnouncements, err := loadCachedSyncAnnouncementsByTopic(context.Background(), rc, "world", 10)
	if err != nil {
		t.Fatalf("loadCachedSyncAnnouncementsByTopic error = %v", err)
	}
	if len(topicAnnouncements) != 2 {
		t.Fatalf("topic announcements len = %d", len(topicAnnouncements))
	}
	if topicAnnouncements[0].InfoHash != newer.InfoHash || topicAnnouncements[1].InfoHash != older.InfoHash {
		t.Fatalf("topic announcements order = %#v", topicAnnouncements)
	}
}

func TestLoadCachedSyncQueueRefs(t *testing.T) {
	mini, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run error = %v", err)
	}
	defer mini.Close()

	rc, err := NewRedisClient(RedisConfig{
		Enabled:   true,
		Addr:      mini.Addr(),
		KeyPrefix: "haonews-test-",
	})
	if err != nil {
		t.Fatalf("NewRedisClient error = %v", err)
	}
	defer rc.Close()

	queuePath := "/tmp/history.txt"
	first := SyncRef{Magnet: "haonews-sync://bundle/aaa?dn=one"}
	second := SyncRef{Magnet: "haonews-sync://bundle/bbb?dn=two"}
	if err := cacheSyncQueueRef(context.Background(), rc, queuePath, first); err != nil {
		t.Fatalf("cacheSyncQueueRef(first) error = %v", err)
	}
	if err := cacheSyncQueueRef(context.Background(), rc, queuePath, second); err != nil {
		t.Fatalf("cacheSyncQueueRef(second) error = %v", err)
	}
	refs, err := loadCachedSyncQueueRefs(context.Background(), rc, queuePath, 10)
	if err != nil {
		t.Fatalf("loadCachedSyncQueueRefs error = %v", err)
	}
	if len(refs) != 2 || refs[0] != first.Magnet || refs[1] != second.Magnet {
		t.Fatalf("queue refs = %#v", refs)
	}
}

func TestCacheSyncAnnouncementTrimsToMaxAnnouncements(t *testing.T) {
	mini, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run error = %v", err)
	}
	defer mini.Close()

	rc, err := NewRedisClient(RedisConfig{
		Enabled:          true,
		Addr:             mini.Addr(),
		KeyPrefix:        "haonews-test-",
		MaxAnnouncements: 2,
	})
	if err != nil {
		t.Fatalf("NewRedisClient error = %v", err)
	}
	defer rc.Close()

	first := SyncAnnouncement{
		InfoHash:  "1111111111111111111111111111111111111111",
		Ref:       "magnet:?xt=urn:btih:1111111111111111111111111111111111111111",
		Title:     "first",
		Channel:   "news",
		Topics:    []string{"world"},
		CreatedAt: "2026-04-01T10:00:00Z",
	}
	second := SyncAnnouncement{
		InfoHash:  "2222222222222222222222222222222222222222",
		Ref:       "magnet:?xt=urn:btih:2222222222222222222222222222222222222222",
		Title:     "second",
		Channel:   "news",
		Topics:    []string{"world"},
		CreatedAt: "2026-04-01T10:01:00Z",
	}
	third := SyncAnnouncement{
		InfoHash:  "3333333333333333333333333333333333333333",
		Ref:       "magnet:?xt=urn:btih:3333333333333333333333333333333333333333",
		Title:     "third",
		Channel:   "news",
		Topics:    []string{"world"},
		CreatedAt: "2026-04-01T10:02:00Z",
	}
	for _, item := range []SyncAnnouncement{first, second, third} {
		if err := cacheSyncAnnouncement(context.Background(), rc, item); err != nil {
			t.Fatalf("cacheSyncAnnouncement(%s) error = %v", item.Title, err)
		}
	}

	indexMembers, err := rc.client.ZRange(context.Background(), syncAnnouncementIndexRedisKey(rc), 0, -1).Result()
	if err != nil {
		t.Fatalf("ZRange(index) error = %v", err)
	}
	if len(indexMembers) != 2 {
		t.Fatalf("index members len = %d, members=%#v", len(indexMembers), indexMembers)
	}
	if indexMembers[0] != second.InfoHash || indexMembers[1] != third.InfoHash {
		t.Fatalf("index members = %#v", indexMembers)
	}

	if ok, err := rc.client.Exists(context.Background(), syncAnnouncementRedisKey(rc, first.InfoHash)).Result(); err != nil {
		t.Fatalf("Exists(first) error = %v", err)
	} else if ok != 0 {
		t.Fatalf("expected first announcement key to be trimmed")
	}

	channelMembers, err := rc.client.ZRevRange(context.Background(), syncChannelRedisKey(rc, "news"), 0, -1).Result()
	if err != nil {
		t.Fatalf("ZRevRange(channel) error = %v", err)
	}
	if len(channelMembers) != 2 || channelMembers[0] != third.InfoHash || channelMembers[1] != second.InfoHash {
		t.Fatalf("channel members after trim = %#v", channelMembers)
	}
}
