package newsplugin

import (
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
