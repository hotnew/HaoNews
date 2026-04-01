package newsplugin

import (
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"

	corehaonews "hao.news/internal/haonews"
)

func TestRedisNodeStatusDisabled(t *testing.T) {
	entry, card := redisNodeStatus(NetworkBootstrapConfig{})
	if entry.Value != "disabled" || card.Value != "disabled" {
		t.Fatalf("expected disabled redis status, got entry=%q card=%q", entry.Value, card.Value)
	}
	if entry.Tone != "warn" || card.Tone != "warn" {
		t.Fatalf("expected warn tone, got entry=%q card=%q", entry.Tone, card.Tone)
	}
}

func TestRedisNodeStatusOnline(t *testing.T) {
	mini, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run error = %v", err)
	}
	defer mini.Close()

	mini.Set("haonews-test-sync:ann:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", `{"infohash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
	mini.ZAdd("haonews-test-sync:channel:news", 1711933200, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	mini.ZAdd("haonews-test-sync:topic:world", 1711933200, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	mini.RPush("haonews-test-sync:queue:refs:realtime", "haonews-sync://bundle/aaa?dn=one")
	mini.RPush("haonews-test-sync:queue:refs:history", "haonews-sync://bundle/bbb?dn=two")

	entry, card := redisNodeStatus(NetworkBootstrapConfig{
		Redis: corehaonews.RedisConfig{
			Enabled:   true,
			Addr:      mini.Addr(),
			KeyPrefix: "haonews-test-",
		},
	})
	if entry.Value != "online" || card.Value != "online" {
		t.Fatalf("expected online redis status, got entry=%q card=%q", entry.Value, card.Value)
	}
	if entry.Tone != "good" || card.Tone != "good" {
		t.Fatalf("expected good tone, got entry=%q card=%q", entry.Tone, card.Tone)
	}
	if !strings.Contains(entry.Detail, "haonews-test-") {
		t.Fatalf("expected prefix in detail, got %q", entry.Detail)
	}
	for _, want := range []string{"ann=1", "channel=1", "topic=1", "realtime=1/history=1"} {
		if !strings.Contains(entry.Detail, want) {
			t.Fatalf("expected redis summary detail to contain %q, got %q", want, entry.Detail)
		}
	}
}
