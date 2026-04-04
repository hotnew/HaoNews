package haonews

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestSubscribedAnnouncementTopics(t *testing.T) {
	t.Parallel()

	networkID := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	topics := subscribedAnnouncementTopics(networkID, SyncSubscriptions{
		Topics:          []string{"world", "国际"},
		Tags:            []string{"breaking"},
		DiscoveryFeeds:  []string{"news"},
		DiscoveryTopics: []string{"期货"},
	})
	if len(topics) != 4 {
		t.Fatalf("topics len = %d, want 4", len(topics))
	}
	if !containsString(topics, "haonews/announce/"+networkID+"/topic/world") {
		t.Fatalf("missing topic subscription: %v", topics)
	}
	if !containsString(topics, "haonews/announce/"+networkID+"/tag/breaking") {
		t.Fatalf("missing tag subscription: %v", topics)
	}
	if !containsString(topics, "haonews/announce/"+networkID+"/channel/hao.news%2Fnews") {
		t.Fatalf("missing discovery feed subscription: %v", topics)
	}
	if !containsString(topics, "haonews/announce/"+networkID+"/topic/futures") {
		t.Fatalf("missing discovery topic subscription: %v", topics)
	}
}

func TestDiscoveryNamespacesIncludeConfiguredFeedsAndTopics(t *testing.T) {
	t.Parallel()

	networkID := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	namespaces := discoveryNamespaces(networkID, []string{"hao.news/global"}, SyncSubscriptions{
		DiscoveryFeeds:  []string{"global", "news"},
		DiscoveryTopics: []string{"国际"},
	})
	if !containsString(namespaces, "haonews/discovery/"+networkID+"/hao.news%2Fglobal") {
		t.Fatalf("missing configured namespace: %v", namespaces)
	}
	if !containsString(namespaces, "haonews/discovery/"+networkID+"/feed%2Fglobal") {
		t.Fatalf("missing global discovery feed: %v", namespaces)
	}
	if !containsString(namespaces, "haonews/discovery/"+networkID+"/feed%2Fnews") {
		t.Fatalf("missing news discovery feed: %v", namespaces)
	}
	if !containsString(namespaces, "haonews/discovery/"+networkID+"/topic%2Fworld") {
		t.Fatalf("missing world discovery topic: %v", namespaces)
	}
}

func TestTeamSyncDiscoveryNamespaceUsesNetworkScopedTeamPath(t *testing.T) {
	t.Parallel()

	networkID := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	got := teamSyncDiscoveryNamespace(networkID, "Archive-Demo")
	want := "haonews/discovery/" + networkID + "/team%2Farchive-demo%2Fsync"
	if got != want {
		t.Fatalf("teamSyncDiscoveryNamespace = %q, want %q", got, want)
	}
}

func TestSubscribedAnnouncementTopicsCanonicalizesDiscoveryFeeds(t *testing.T) {
	t.Parallel()

	networkID := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	topics := subscribedAnnouncementTopics(networkID, SyncSubscriptions{
		DiscoveryFeeds: []string{"hao.news/news", "NEWS", "all", "新手"},
	})
	if !containsString(topics, "haonews/announce/"+networkID+"/global") {
		t.Fatalf("missing global subscription: %v", topics)
	}
	if !containsString(topics, "haonews/announce/"+networkID+"/channel/hao.news%2Fnews") {
		t.Fatalf("missing news channel subscription: %v", topics)
	}
	if !containsString(topics, "haonews/announce/"+networkID+"/channel/hao.news%2Fnew-agents") {
		t.Fatalf("missing new-agents channel subscription: %v", topics)
	}
}

func TestMatchesAnnouncement(t *testing.T) {
	t.Parallel()

	announcement := SyncAnnouncement{
		Channel:   "latest.org/world",
		Author:    "agent://pc75/openclaw01",
		NetworkID: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Topics:    []string{"world", "pc75"},
		Tags:      []string{"breaking"},
	}
	if !matchesAnnouncement(announcement, SyncSubscriptions{Topics: []string{"pc75"}}) {
		t.Fatal("expected topic match")
	}
	if !matchesAnnouncement(announcement, SyncSubscriptions{Channels: []string{"latest.org/world"}}) {
		t.Fatal("expected channel match")
	}
	if !matchesAnnouncement(announcement, SyncSubscriptions{Authors: []string{"agent://pc75/openclaw01"}}) {
		t.Fatal("expected author match")
	}
	if matchesAnnouncement(announcement, SyncSubscriptions{Topics: []string{"markets"}}) {
		t.Fatal("unexpected topic match")
	}
}

func TestMatchesAnnouncementFiltersByPublicKeys(t *testing.T) {
	t.Parallel()

	announcement := SyncAnnouncement{
		OriginPublicKey: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ParentPublicKey: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Topics:          []string{"technology"},
	}
	if !matchesAnnouncement(announcement, SyncSubscriptions{AllowedOriginKeys: []string{"AAaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}) {
		t.Fatal("expected origin public key allow match")
	}
	if !matchesAnnouncement(announcement, SyncSubscriptions{AllowedParentKeys: []string{"BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"}}) {
		t.Fatal("expected parent public key allow match")
	}
	if matchesAnnouncement(announcement, SyncSubscriptions{BlockedOriginKeys: []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, Topics: []string{"technology"}}) {
		t.Fatal("expected blocked origin key to win")
	}
	if matchesAnnouncement(announcement, SyncSubscriptions{BlockedParentKeys: []string{"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}, Topics: []string{"technology"}}) {
		t.Fatal("expected blocked parent key to win")
	}
}

func TestRunPubSubHandlerWithTimeoutReturnsTimeout(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := runPubSubHandlerWithTimeout(ctx, 10*time.Millisecond, func() (bool, error) {
		time.Sleep(50 * time.Millisecond)
		return true, nil
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("runPubSubHandlerWithTimeout error = %v, want timeout", err)
	}
}

func TestRunPubSubHandlerWithTimeoutReturnsValue(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	got, err := runPubSubHandlerWithTimeout(ctx, time.Second, func() (bool, error) {
		return true, nil
	})
	if err != nil {
		t.Fatalf("runPubSubHandlerWithTimeout error = %v", err)
	}
	if !got {
		t.Fatal("runPubSubHandlerWithTimeout got false, want true")
	}
}

func TestMatchesHistoryAnnouncementUsesHistorySelectors(t *testing.T) {
	t.Parallel()

	announcement := SyncAnnouncement{
		Channel:   "hao.news/world",
		Author:    "agent://pc75/openclaw01",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Topics:    []string{"world", "energy"},
	}
	if !matchesHistoryAnnouncement(announcement, SyncSubscriptions{HistoryAuthors: []string{"agent://pc75/openclaw01"}}) {
		t.Fatal("expected history author match")
	}
	if matchesHistoryAnnouncement(announcement, SyncSubscriptions{HistoryAuthors: []string{"agent://pc76/main"}}) {
		t.Fatal("unexpected history author match")
	}
	if !matchesHistoryAnnouncement(announcement, SyncSubscriptions{HistoryTopics: []string{"energy"}}) {
		t.Fatal("expected history topic match")
	}
}

func TestMatchesAnnouncementCanonicalizesTopicAliases(t *testing.T) {
	t.Parallel()

	announcement := SyncAnnouncement{
		Topics: []string{"世界", "期货"},
	}
	if !matchesAnnouncement(announcement, SyncSubscriptions{Topics: []string{"world"}}) {
		t.Fatal("expected world alias match")
	}
	if !matchesAnnouncement(announcement, SyncSubscriptions{Topics: []string{"futures"}}) {
		t.Fatal("expected futures alias match")
	}
}

func TestSyncSubscriptionsNormalizeCanonicalizesTopicAliases(t *testing.T) {
	t.Parallel()

	rules := SyncSubscriptions{
		Topics:          []string{"world", "世界", "国际", "macro", "未收录"},
		DiscoveryTopics: []string{"新闻", "news"},
		HistoryTopics:   []string{"期货", "futures", "brief"},
		TopicWhitelist:  []string{"WORLD", "news", "futures"},
		TopicAliases: map[string]string{
			"macro": "world",
			"brief": "news",
		},
	}
	rules.Normalize()

	if len(rules.Topics) != 1 || rules.Topics[0] != "world" {
		t.Fatalf("topics = %v, want [world]", rules.Topics)
	}
	if len(rules.DiscoveryTopics) != 1 || rules.DiscoveryTopics[0] != "news" {
		t.Fatalf("discovery topics = %v, want [news]", rules.DiscoveryTopics)
	}
	if len(rules.HistoryTopics) != 2 || rules.HistoryTopics[0] != "futures" || rules.HistoryTopics[1] != "news" {
		t.Fatalf("history topics = %v, want [futures news]", rules.HistoryTopics)
	}
	if len(rules.TopicWhitelist) != 3 || rules.TopicWhitelist[0] != "futures" || rules.TopicWhitelist[1] != "news" || rules.TopicWhitelist[2] != "world" {
		t.Fatalf("topic whitelist = %v, want [futures news world]", rules.TopicWhitelist)
	}
	if rules.TopicAliases["macro"] != "world" || rules.TopicAliases["brief"] != "news" {
		t.Fatalf("topic aliases = %v, want macro->world and brief->news", rules.TopicAliases)
	}
}

func TestSyncSubscriptionsNormalizePublicKeys(t *testing.T) {
	t.Parallel()

	rules := SyncSubscriptions{
		AllowedOriginKeys: []string{
			"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"bad",
		},
		BlockedParentKeys: []string{
			"BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB",
			"",
		},
	}
	rules.Normalize()

	if len(rules.AllowedOriginKeys) != 1 || rules.AllowedOriginKeys[0] != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("allowed origin keys = %v", rules.AllowedOriginKeys)
	}
	if len(rules.BlockedParentKeys) != 1 || rules.BlockedParentKeys[0] != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("blocked parent keys = %v", rules.BlockedParentKeys)
	}
}

func TestSyncSubscriptionsNormalizeLivePublicKeys(t *testing.T) {
	t.Parallel()

	rules := SyncSubscriptions{
		LiveAllowedOriginKeys: []string{
			"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"bad",
		},
		LiveBlockedParentKeys: []string{
			"BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB",
			"",
		},
	}
	rules.Normalize()

	if len(rules.LiveAllowedOriginKeys) != 1 || rules.LiveAllowedOriginKeys[0] != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("live allowed origin keys = %v", rules.LiveAllowedOriginKeys)
	}
	if len(rules.LiveBlockedParentKeys) != 1 || rules.LiveBlockedParentKeys[0] != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("live blocked parent keys = %v", rules.LiveBlockedParentKeys)
	}
}

func TestSyncSubscriptionsNormalizeLivePublicModeration(t *testing.T) {
	t.Parallel()

	rules := SyncSubscriptions{
		LivePublicMutedOriginKeys: []string{
			"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			"bad",
		},
		LivePublicMutedParentKeys: []string{
			"BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB",
			"",
		},
		LivePublicRateLimitMessages:      -2,
		LivePublicRateLimitWindowSeconds: -8,
	}
	rules.Normalize()

	if len(rules.LivePublicMutedOriginKeys) != 1 || rules.LivePublicMutedOriginKeys[0] != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("live public muted origin keys = %v", rules.LivePublicMutedOriginKeys)
	}
	if len(rules.LivePublicMutedParentKeys) != 1 || rules.LivePublicMutedParentKeys[0] != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("live public muted parent keys = %v", rules.LivePublicMutedParentKeys)
	}
	if rules.LivePublicRateLimitMessages != 0 || rules.LivePublicRateLimitWindowSeconds != 0 {
		t.Fatalf("live public rate limits = %d/%d, want 0/0", rules.LivePublicRateLimitMessages, rules.LivePublicRateLimitWindowSeconds)
	}
}

func TestMatchesAnnouncementFiltersByMaxAgeDays(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	announcement := SyncAnnouncement{
		Channel:   "latest.org/world",
		CreatedAt: now.Add(-48 * time.Hour).Format(time.RFC3339),
		Topics:    []string{"world", "pc75"},
	}
	if matchesAnnouncement(announcement, SyncSubscriptions{Topics: []string{"all"}, MaxAgeDays: 1}) {
		t.Fatal("expected stale announcement to be filtered")
	}
	if !matchesAnnouncement(announcement, SyncSubscriptions{Topics: []string{"all"}, MaxAgeDays: 3}) {
		t.Fatal("expected announcement within max age")
	}
}

func TestMatchesAnnouncementFiltersByMaxBundleMB(t *testing.T) {
	t.Parallel()

	announcement := SyncAnnouncement{
		SizeBytes: 12 * 1024 * 1024,
		Topics:    []string{"world"},
	}
	if matchesAnnouncement(announcement, SyncSubscriptions{Topics: []string{"all"}, MaxBundleMB: 10}) {
		t.Fatal("expected oversized announcement to be filtered")
	}
	if !matchesAnnouncement(announcement, SyncSubscriptions{Topics: []string{"all"}, MaxBundleMB: 20}) {
		t.Fatal("expected announcement within size limit")
	}
}

func TestPubSubRuntimeReserveSubscriptionRespectsLimit(t *testing.T) {
	t.Parallel()

	runtime := &pubsubRuntime{maxSubs: 2}
	if !runtime.reserveSubscription() {
		t.Fatal("first reserve should succeed")
	}
	if !runtime.reserveSubscription() {
		t.Fatal("second reserve should succeed")
	}
	if runtime.reserveSubscription() {
		t.Fatal("third reserve should fail")
	}
	if got := runtime.subCount.Load(); got != 2 {
		t.Fatalf("subCount = %d, want 2", got)
	}
	runtime.releaseSubscription()
	if !runtime.reserveSubscription() {
		t.Fatal("reserve should succeed after release")
	}
}

func TestPubSubRuntimeConnectDiscoveredPeersCapsConcurrency(t *testing.T) {
	t.Parallel()

	runtime := &pubsubRuntime{
		connSema: make(chan struct{}, 2),
	}
	var current atomic.Int32
	var maxSeen atomic.Int32
	runtime.connectFn = func(ctx context.Context, info peer.AddrInfo) error {
		now := current.Add(1)
		for {
			seen := maxSeen.Load()
			if now <= seen || maxSeen.CompareAndSwap(seen, now) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		current.Add(-1)
		return nil
	}

	infos := []peer.AddrInfo{
		{ID: "peer-1"},
		{ID: "peer-2"},
		{ID: "peer-3"},
		{ID: "peer-4"},
	}
	runtime.connectDiscoveredPeers(context.Background(), infos, func(peer.ID) bool { return false }, "self")

	if got := maxSeen.Load(); got > 2 {
		t.Fatalf("max concurrent connects = %d, want <= 2", got)
	}
}

func TestPubSubRuntimeConnectDiscoveredPeersRecordsErrors(t *testing.T) {
	t.Parallel()

	runtime := &pubsubRuntime{
		connSema: make(chan struct{}, 1),
		connectFn: func(ctx context.Context, info peer.AddrInfo) error {
			return fmt.Errorf("boom")
		},
	}

	runtime.connectDiscoveredPeers(context.Background(), []peer.AddrInfo{{ID: "peer-1"}}, func(peer.ID) bool { return false }, "self")

	status := runtime.Status()
	if !strings.Contains(status.LastError, "boom") {
		t.Fatalf("last error = %q, want connect error", status.LastError)
	}
}

func TestPubSubRuntimeConnectDiscoveredPeersSkipsExistingAndSelf(t *testing.T) {
	t.Parallel()

	runtime := &pubsubRuntime{
		connSema: make(chan struct{}, 2),
	}
	var mu sync.Mutex
	var seen []peer.ID
	runtime.connectFn = func(ctx context.Context, info peer.AddrInfo) error {
		mu.Lock()
		seen = append(seen, info.ID)
		mu.Unlock()
		return nil
	}

	runtime.connectDiscoveredPeers(context.Background(),
		[]peer.AddrInfo{{ID: "self"}, {ID: "peer-2"}, {ID: "peer-3"}},
		func(id peer.ID) bool { return id == "peer-3" },
		"self",
	)

	if len(seen) != 1 || seen[0] != peer.ID("peer-2") {
		t.Fatalf("connected peers = %v, want [peer-2]", seen)
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
