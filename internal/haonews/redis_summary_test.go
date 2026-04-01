package haonews

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func TestReadRedisSyncSummary(t *testing.T) {
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

	first := SyncAnnouncement{
		InfoHash:  "1111111111111111111111111111111111111111",
		Ref:       "magnet:?xt=urn:btih:1111111111111111111111111111111111111111",
		Channel:   "news",
		Topics:    []string{"world"},
		CreatedAt: "2026-04-01T10:00:00Z",
	}
	second := SyncAnnouncement{
		InfoHash:  "2222222222222222222222222222222222222222",
		Ref:       "magnet:?xt=urn:btih:2222222222222222222222222222222222222222",
		Channel:   "news",
		Topics:    []string{"markets"},
		CreatedAt: "2026-04-01T10:01:00Z",
	}
	if err := cacheSyncAnnouncement(context.Background(), rc, first); err != nil {
		t.Fatalf("cacheSyncAnnouncement(first) error = %v", err)
	}
	if err := cacheSyncAnnouncement(context.Background(), rc, second); err != nil {
		t.Fatalf("cacheSyncAnnouncement(second) error = %v", err)
	}
	_ = cacheSyncQueueRef(context.Background(), rc, "/tmp/realtime.txt", SyncRef{Magnet: "haonews-sync://bundle/aaa?dn=one"})
	_ = cacheSyncQueueRef(context.Background(), rc, "/tmp/history.txt", SyncRef{Magnet: "haonews-sync://bundle/bbb?dn=two"})

	summary, err := readRedisSyncSummary(context.Background(), rc)
	if err != nil {
		t.Fatalf("readRedisSyncSummary error = %v", err)
	}
	if summary.AnnouncementCount != 2 {
		t.Fatalf("AnnouncementCount = %d", summary.AnnouncementCount)
	}
	if summary.ChannelIndexCount != 1 {
		t.Fatalf("ChannelIndexCount = %d", summary.ChannelIndexCount)
	}
	if summary.TopicIndexCount != 2 {
		t.Fatalf("TopicIndexCount = %d", summary.TopicIndexCount)
	}
	if summary.RealtimeQueueRefs != 1 || summary.HistoryQueueRefs != 1 {
		t.Fatalf("queue refs summary = %#v", summary)
	}
}
