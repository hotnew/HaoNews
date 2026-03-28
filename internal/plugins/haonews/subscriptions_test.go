package newsplugin

import (
	"os"
	"testing"
	"time"
)

func TestApplySubscriptionRulesFiltersByTopicAndCarriesReplies(t *testing.T) {
	t.Parallel()

	postWorld := Bundle{
		InfoHash: "post-world",
		Message: Message{
			Kind:    "post",
			Channel: "hao.news/world",
			Extensions: map[string]any{
				"project": "hao.news",
				"topics":  []any{"world", "energy"},
			},
		},
	}
	replyWorld := Bundle{
		InfoHash: "reply-world",
		Message: Message{
			Kind:    "reply",
			ReplyTo: &MessageLink{InfoHash: "post-world"},
			Extensions: map[string]any{
				"project": "hao.news",
			},
		},
	}
	postTech := Bundle{
		InfoHash: "post-tech",
		Message: Message{
			Kind:    "post",
			Channel: "hao.news/tech",
			Extensions: map[string]any{
				"project": "hao.news",
				"topics":  []any{"technology"},
			},
		},
	}

	index := buildIndex([]Bundle{postWorld, replyWorld, postTech}, "hao.news")
	filtered := ApplySubscriptionRules(index, "hao.news", SubscriptionRules{Topics: []string{"energy"}})

	if len(filtered.Posts) != 1 {
		t.Fatalf("posts len = %d, want 1", len(filtered.Posts))
	}
	if filtered.Posts[0].InfoHash != "post-world" {
		t.Fatalf("post = %s, want post-world", filtered.Posts[0].InfoHash)
	}
	if got := len(filtered.RepliesByPost["post-world"]); got != 1 {
		t.Fatalf("replies len = %d, want 1", got)
	}
	if len(filtered.Bundles) != 2 {
		t.Fatalf("bundles len = %d, want 2", len(filtered.Bundles))
	}
}

func TestLoadSubscriptionRulesNormalizesDiscoverySelectors(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := root + "/subscriptions.json"
	data := `{
  "topics": ["all"],
  "discovery_feeds": ["news", "NEWS", "hao.news/live", "all", "新手"],
  "discovery_topics": ["world", "WORLD", "期货"]
}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	rules, err := LoadSubscriptionRules(path)
	if err != nil {
		t.Fatalf("LoadSubscriptionRules() error = %v", err)
	}
	if len(rules.DiscoveryFeeds) != 4 {
		t.Fatalf("discovery feeds len = %d, want 4", len(rules.DiscoveryFeeds))
	}
	if len(rules.DiscoveryTopics) != 2 {
		t.Fatalf("discovery topics len = %d, want 2", len(rules.DiscoveryTopics))
	}
	if rules.DiscoveryFeeds[0] != "news" || rules.DiscoveryFeeds[1] != "live" || rules.DiscoveryFeeds[2] != "global" || rules.DiscoveryFeeds[3] != "new-agents" {
		t.Fatalf("unexpected normalized discovery feeds: %v", rules.DiscoveryFeeds)
	}
	if rules.DiscoveryTopics[0] != "world" || rules.DiscoveryTopics[1] != "futures" {
		t.Fatalf("unexpected normalized discovery topics: %v", rules.DiscoveryTopics)
	}
}

func TestLoadSubscriptionRulesNormalizesTopicAliases(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := root + "/subscriptions.json"
	data := `{
  "topics": ["世界", "国际", "world"],
  "history_topics": ["新闻", "news"],
  "discovery_topics": ["期货", "futures"]
}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	rules, err := LoadSubscriptionRules(path)
	if err != nil {
		t.Fatalf("LoadSubscriptionRules() error = %v", err)
	}
	if len(rules.Topics) != 1 || rules.Topics[0] != "world" {
		t.Fatalf("topics = %v, want [world]", rules.Topics)
	}
	if len(rules.HistoryTopics) != 1 || rules.HistoryTopics[0] != "news" {
		t.Fatalf("history topics = %v, want [news]", rules.HistoryTopics)
	}
	if len(rules.DiscoveryTopics) != 1 || rules.DiscoveryTopics[0] != "futures" {
		t.Fatalf("discovery topics = %v, want [futures]", rules.DiscoveryTopics)
	}
}

func TestLoadSubscriptionRulesAppliesConfiguredTopicAliasesAndWhitelist(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := root + "/subscriptions.json"
	data := `{
  "topics": ["macro", "未收录"],
  "history_topics": ["brief"],
  "discovery_topics": ["期货", "unknown"],
  "topic_whitelist": ["world", "news", "futures"],
  "topic_aliases": {
    "macro": "world",
    "brief": "news"
  }
}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	rules, err := LoadSubscriptionRules(path)
	if err != nil {
		t.Fatalf("LoadSubscriptionRules() error = %v", err)
	}
	if len(rules.Topics) != 1 || rules.Topics[0] != "world" {
		t.Fatalf("topics = %v, want [world]", rules.Topics)
	}
	if len(rules.HistoryTopics) != 1 || rules.HistoryTopics[0] != "news" {
		t.Fatalf("history topics = %v, want [news]", rules.HistoryTopics)
	}
	if len(rules.DiscoveryTopics) != 1 || rules.DiscoveryTopics[0] != "futures" {
		t.Fatalf("discovery topics = %v, want [futures]", rules.DiscoveryTopics)
	}
	if len(rules.TopicWhitelist) != 3 || rules.TopicWhitelist[0] != "futures" || rules.TopicWhitelist[1] != "news" || rules.TopicWhitelist[2] != "world" {
		t.Fatalf("topic whitelist = %v, want [futures news world]", rules.TopicWhitelist)
	}
	if rules.TopicAliases["macro"] != "world" || rules.TopicAliases["brief"] != "news" {
		t.Fatalf("topic aliases = %v, want macro->world and brief->news", rules.TopicAliases)
	}
}

func TestLoadSubscriptionRulesNormalizesApprovalModeAndFeed(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := root + "/subscriptions.json"
	data := `{
  "whitelist_mode": "APPROVAL",
  "approval_feed": "pending approval",
  "topics": ["all"]
}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	rules, err := LoadSubscriptionRules(path)
	if err != nil {
		t.Fatalf("LoadSubscriptionRules() error = %v", err)
	}
	if rules.WhitelistMode != "approval" {
		t.Fatalf("whitelist mode = %q, want approval", rules.WhitelistMode)
	}
	if rules.ApprovalFeed != "pending-approval" {
		t.Fatalf("approval feed = %q, want pending-approval", rules.ApprovalFeed)
	}
}

func TestLoadSubscriptionRulesKeepsAutoRoutePending(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := root + "/subscriptions.json"
	data := `{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "auto_route_pending": true,
  "topics": ["all"]
}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	rules, err := LoadSubscriptionRules(path)
	if err != nil {
		t.Fatalf("LoadSubscriptionRules() error = %v", err)
	}
	if !rules.AutoRoutePending {
		t.Fatalf("auto route pending = %v, want true", rules.AutoRoutePending)
	}
}

func TestLoadSubscriptionRulesNormalizesApprovalRoutes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := root + "/subscriptions.json"
	data := `{
  "whitelist_mode": "approval",
  "topics": ["world", "news", "futures"],
  "topic_whitelist": ["world", "news", "futures"],
  "topic_aliases": {
    "国际": "world"
  },
  "approval_routes": {
    "国际": "reviewer-world",
    "feed/NEWS": "reviewer-news",
    "topic/unknown": "reviewer-drop"
  }
}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	rules, err := LoadSubscriptionRules(path)
	if err != nil {
		t.Fatalf("LoadSubscriptionRules() error = %v", err)
	}
	if len(rules.ApprovalRoutes) != 2 {
		t.Fatalf("approval routes len = %d, want 2", len(rules.ApprovalRoutes))
	}
	if got := rules.ApprovalRoutes["topic/world"]; got != "reviewer-world" {
		t.Fatalf("topic/world route = %q, want reviewer-world", got)
	}
	if got := rules.ApprovalRoutes["feed/news"]; got != "reviewer-news" {
		t.Fatalf("feed/news route = %q, want reviewer-news", got)
	}
}

func TestLoadSubscriptionRulesNormalizesApprovalAutoApprove(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := root + "/subscriptions.json"
	data := `{
  "whitelist_mode": "approval",
  "topic_whitelist": ["world", "news", "futures"],
  "topic_aliases": {
    "国际": "world"
  },
  "approval_auto_approve": ["国际", "feed/NEWS", "topic/unknown"]
}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	rules, err := LoadSubscriptionRules(path)
	if err != nil {
		t.Fatalf("LoadSubscriptionRules() error = %v", err)
	}
	if len(rules.ApprovalAutoApprove) != 2 {
		t.Fatalf("approval auto approve len = %d, want 2", len(rules.ApprovalAutoApprove))
	}
	if rules.ApprovalAutoApprove[0] != "topic/world" || rules.ApprovalAutoApprove[1] != "feed/news" {
		t.Fatalf("approval auto approve = %v, want [topic/world feed/news]", rules.ApprovalAutoApprove)
	}
}

func TestApplySubscriptionRulesReservedAllTopicShowsEverything(t *testing.T) {
	t.Parallel()

	postWorld := Bundle{
		InfoHash: "post-world",
		Message: Message{
			Kind:    "post",
			Channel: "hao.news/world",
			Extensions: map[string]any{
				"project": "hao.news",
				"topics":  []any{"world"},
			},
		},
	}
	postTech := Bundle{
		InfoHash: "post-tech",
		Message: Message{
			Kind:    "post",
			Channel: "hao.news/tech",
			Extensions: map[string]any{
				"project": "hao.news",
				"topics":  []any{"technology"},
			},
		},
	}

	index := buildIndex([]Bundle{postWorld, postTech}, "hao.news")
	filtered := ApplySubscriptionRules(index, "hao.news", SubscriptionRules{Topics: []string{"all"}})

	if len(filtered.Posts) != 2 {
		t.Fatalf("posts len = %d, want 2", len(filtered.Posts))
	}
}

func TestApplySubscriptionRulesApprovalModeKeepsPendingPostsOutOfDefaultFeed(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)
	postWorld := Bundle{
		InfoHash:  "post-world",
		CreatedAt: now,
		Message: Message{
			Kind:      "post",
			Channel:   "hao.news/world",
			CreatedAt: now.Format(time.RFC3339),
			Extensions: map[string]any{
				"project": "hao.news",
				"topics":  []any{"world"},
			},
		},
	}
	postTech := Bundle{
		InfoHash:  "post-tech",
		CreatedAt: now,
		Message: Message{
			Kind:      "post",
			Channel:   "hao.news/tech",
			CreatedAt: now.Format(time.RFC3339),
			Extensions: map[string]any{
				"project": "hao.news",
				"topics":  []any{"technology"},
			},
		},
	}

	index := buildIndex([]Bundle{postWorld, postTech}, "hao.news")
	filtered := ApplySubscriptionRules(index, "hao.news", SubscriptionRules{
		WhitelistMode: "approval",
		ApprovalFeed:  "pending-approval",
		Topics:        []string{"world"},
	})

	if len(filtered.Posts) != 2 {
		t.Fatalf("posts len = %d, want 2", len(filtered.Posts))
	}
	if !filtered.PostByInfoHash["post-tech"].PendingApproval {
		t.Fatalf("post-tech pending = false, want true")
	}
	if filtered.PostByInfoHash["post-tech"].ApprovalFeed != "pending-approval" {
		t.Fatalf("approval feed = %q, want pending-approval", filtered.PostByInfoHash["post-tech"].ApprovalFeed)
	}
	visible := filtered.FilterPosts(FeedOptions{Now: now})
	if len(visible) != 1 || visible[0].InfoHash != "post-world" {
		t.Fatalf("visible posts = %+v, want only post-world", visible)
	}
	pending := filtered.FilterPosts(FeedOptions{PendingApproval: true, Now: now})
	if len(pending) != 1 || pending[0].InfoHash != "post-tech" {
		t.Fatalf("pending posts = %+v, want only post-tech", pending)
	}
}

func TestApplySubscriptionRulesFiltersByMaxAgeDays(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	fresh := Bundle{
		InfoHash: "post-fresh",
		Message: Message{
			Kind:      "post",
			Channel:   "hao.news/world",
			CreatedAt: now.Add(-12 * time.Hour).Format(time.RFC3339),
			Extensions: map[string]any{
				"project": "hao.news",
				"topics":  []any{"world"},
			},
		},
	}
	stale := Bundle{
		InfoHash: "post-stale",
		Message: Message{
			Kind:      "post",
			Channel:   "hao.news/world",
			CreatedAt: now.Add(-72 * time.Hour).Format(time.RFC3339),
			Extensions: map[string]any{
				"project": "hao.news",
				"topics":  []any{"world"},
			},
		},
	}

	index := buildIndex([]Bundle{fresh, stale}, "hao.news")
	filtered := ApplySubscriptionRules(index, "hao.news", SubscriptionRules{Topics: []string{"all"}, MaxAgeDays: 1})

	if len(filtered.Posts) != 1 {
		t.Fatalf("posts len = %d, want 1", len(filtered.Posts))
	}
	if filtered.Posts[0].InfoHash != "post-fresh" {
		t.Fatalf("post = %s, want post-fresh", filtered.Posts[0].InfoHash)
	}
}

func TestApplySubscriptionRulesFiltersByMaxBundleMB(t *testing.T) {
	t.Parallel()

	small := Bundle{
		InfoHash:  "post-small",
		SizeBytes: 2 * 1024 * 1024,
		Message: Message{
			Kind:    "post",
			Channel: "hao.news/world",
			Extensions: map[string]any{
				"project": "hao.news",
				"topics":  []any{"world"},
			},
		},
	}
	large := Bundle{
		InfoHash:  "post-large",
		SizeBytes: 12 * 1024 * 1024,
		Message: Message{
			Kind:    "post",
			Channel: "hao.news/world",
			Extensions: map[string]any{
				"project": "hao.news",
				"topics":  []any{"world"},
			},
		},
	}

	index := buildIndex([]Bundle{small, large}, "hao.news")
	filtered := ApplySubscriptionRules(index, "hao.news", SubscriptionRules{Topics: []string{"all"}, MaxBundleMB: 10})

	if len(filtered.Posts) != 1 {
		t.Fatalf("posts len = %d, want 1", len(filtered.Posts))
	}
	if filtered.Posts[0].InfoHash != "post-small" {
		t.Fatalf("post = %s, want post-small", filtered.Posts[0].InfoHash)
	}
}

func TestApplySubscriptionRulesFiltersByMaxItemsPerDay(t *testing.T) {
	t.Parallel()

	day := time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC)
	first := Bundle{
		InfoHash: "post-first",
		Message: Message{
			Kind:      "post",
			Channel:   "hao.news/world",
			CreatedAt: day.Format(time.RFC3339),
			Extensions: map[string]any{
				"project": "hao.news",
				"topics":  []any{"world"},
			},
		},
	}
	second := Bundle{
		InfoHash: "post-second",
		Message: Message{
			Kind:      "post",
			Channel:   "hao.news/world",
			CreatedAt: day.Add(-1 * time.Hour).Format(time.RFC3339),
			Extensions: map[string]any{
				"project": "hao.news",
				"topics":  []any{"world"},
			},
		},
	}

	index := buildIndex([]Bundle{first, second}, "hao.news")
	filtered := ApplySubscriptionRules(index, "hao.news", SubscriptionRules{Topics: []string{"all"}, MaxItemsPerDay: 1})

	if len(filtered.Posts) != 1 {
		t.Fatalf("posts len = %d, want 1", len(filtered.Posts))
	}
	if filtered.Posts[0].InfoHash != "post-first" {
		t.Fatalf("post = %s, want post-first", filtered.Posts[0].InfoHash)
	}
}

func TestApplySubscriptionRulesFiltersByAuthor(t *testing.T) {
	t.Parallel()

	index := buildIndex([]Bundle{
		{
			InfoHash: "post-pc75",
			Message: Message{
				Kind:    "post",
				Author:  "agent://pc75/openclaw01",
				Channel: "hao.news/world",
				Extensions: map[string]any{
					"project": "hao.news",
					"topics":  []any{"world"},
				},
			},
		},
		{
			InfoHash: "post-pc76",
			Message: Message{
				Kind:    "post",
				Author:  "agent://pc76/main",
				Channel: "hao.news/world",
				Extensions: map[string]any{
					"project": "hao.news",
					"topics":  []any{"world"},
				},
			},
		},
	}, "hao.news")

	filtered := ApplySubscriptionRules(index, "hao.news", SubscriptionRules{Authors: []string{"agent://pc75/openclaw01"}})
	if len(filtered.Posts) != 1 {
		t.Fatalf("posts len = %d, want 1", len(filtered.Posts))
	}
	if filtered.Posts[0].InfoHash != "post-pc75" {
		t.Fatalf("post = %s, want post-pc75", filtered.Posts[0].InfoHash)
	}
}
