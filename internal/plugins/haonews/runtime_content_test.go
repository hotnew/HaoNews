package newsplugin

import (
	"testing"
	"time"
)

func TestBuildTopicDirectoryUsesCurrentTabAndWindow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)
	index := Index{
		Posts: []Post{
			{
				Bundle:        Bundle{InfoHash: "hot-world", CreatedAt: now.Add(-2 * time.Hour)},
				Topics:        []string{"world"},
				ReplyCount:    2,
				ReactionCount: 3,
				Upvotes:       3,
				CommentCount:  2,
			},
			{
				Bundle:        Bundle{InfoHash: "new-world", CreatedAt: now.Add(-3 * time.Hour)},
				Topics:        []string{"world"},
				ReplyCount:    0,
				ReactionCount: 0,
				Upvotes:       0,
				CommentCount:  0,
			},
			{
				Bundle:        Bundle{InfoHash: "old-futures", CreatedAt: now.Add(-72 * time.Hour)},
				Topics:        []string{"futures"},
				ReplyCount:    5,
				ReactionCount: 6,
				Upvotes:       4,
				CommentCount:  5,
			},
		},
		TopicStats: []FacetStat{
			{Name: "world", Count: 2},
			{Name: "futures", Count: 1},
		},
	}

	items := BuildTopicDirectory(index, FeedOptions{
		Tab:    "hot",
		Window: "24h",
		Now:    now,
	})

	if len(items) != 1 {
		t.Fatalf("topic items = %d, want 1", len(items))
	}
	if items[0].Name != "world" {
		t.Fatalf("topic item = %q, want world", items[0].Name)
	}
	if items[0].StoryCount != 1 {
		t.Fatalf("story count = %d, want 1", items[0].StoryCount)
	}
	if items[0].ReplyCount != 2 || items[0].ReactionCount != 3 {
		t.Fatalf("unexpected counts: %+v", items[0])
	}
}
