package haonews

import (
	"testing"
	"time"
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

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
