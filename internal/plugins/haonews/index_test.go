package newsplugin

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFilterPostsSupportsQueryAndSort(t *testing.T) {
	t.Parallel()

	truthA := 0.8
	truthB := 0.5
	const pubKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	index := Index{
		Posts: []Post{
			{
				Bundle: Bundle{
					InfoHash:  "a",
					CreatedAt: time.Date(2026, 3, 12, 10, 0, 0, 0, time.UTC),
					Message: Message{
						Title:  "Oil rises in Europe",
						Author: "agent://collector/a",
					},
					Body: "Energy markets moved higher.",
				},
				ChannelGroup:      "world",
				SourceName:        pubKey,
				SourceSiteName:    "BBC News",
				OriginPublicKey:   pubKey,
				HasSourcePage:     true,
				Topics:            []string{"energy", "world"},
				TruthScoreAverage: &truthA,
			},
			{
				Bundle: Bundle{
					InfoHash:  "b",
					CreatedAt: time.Date(2026, 3, 12, 9, 0, 0, 0, time.UTC),
					Message: Message{
						Title:  "Chip shares retreat",
						Author: "agent://collector/b",
					},
					Body: "Technology stocks traded lower.",
				},
				ChannelGroup:      "markets",
				SourceName:        "CNBC",
				SourceSiteName:    "CNBC",
				Topics:            []string{"technology"},
				TruthScoreAverage: &truthB,
			},
		},
	}

	got := index.FilterPosts(FeedOptions{
		Query: "oil",
		Sort:  "truth",
		Now:   time.Date(2026, 3, 12, 12, 0, 0, 0, time.UTC),
	})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].InfoHash != "a" {
		t.Fatalf("infohash = %s, want a", got[0].InfoHash)
	}
}

func TestBuildIndexPrefersOriginPublicKeyForSourceGrouping(t *testing.T) {
	t.Parallel()

	const pubKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	bundles := []Bundle{
		{
			InfoHash:  "post-1",
			CreatedAt: time.Date(2026, 3, 12, 10, 0, 0, 0, time.UTC),
			Body:      "Energy markets moved higher.",
			Message: Message{
				Kind:      "post",
				Title:     "Oil rises in Europe",
				Author:    "agent://collector/a",
				Channel:   "hao.news/world",
				CreatedAt: "2026-03-12T10:00:00Z",
				Origin: &MessageOrigin{
					Author:    "writer://world/a",
					AgentID:   "agent://world/a",
					PublicKey: pubKey,
				},
				Extensions: map[string]any{
					"project": "hao.news",
					"source": map[string]any{
						"name": "BBC News",
						"url":  "https://example.com/oil",
					},
				},
			},
		},
	}

	index := buildIndex(bundles, "hao.news")
	if len(index.Posts) != 1 {
		t.Fatalf("posts len = %d, want 1", len(index.Posts))
	}
	post := index.Posts[0]
	if post.SourceName != pubKey {
		t.Fatalf("source group = %q, want %q", post.SourceName, pubKey)
	}
	if post.SourceSiteName != "BBC News" {
		t.Fatalf("source site = %q, want BBC News", post.SourceSiteName)
	}
	if post.OriginPublicKey != pubKey {
		t.Fatalf("origin public key = %q, want %q", post.OriginPublicKey, pubKey)
	}
	if !post.HasSourcePage {
		t.Fatal("expected signed post to have a source page")
	}
	if len(index.SourceStats) != 1 || index.SourceStats[0].Name != pubKey {
		t.Fatalf("source stats = %+v, want one public-key group", index.SourceStats)
	}
}

func TestBuildIndexDoesNotAddUnsignedPostsToSourceDirectory(t *testing.T) {
	t.Parallel()

	bundles := []Bundle{
		{
			InfoHash:  "post-unsigned",
			CreatedAt: time.Date(2026, 3, 12, 10, 0, 0, 0, time.UTC),
			Body:      "Unsigned body.",
			Message: Message{
				Kind:      "post",
				Title:     "Unsigned story",
				Author:    "agent://collector/unsigned",
				Channel:   "hao.news/world",
				CreatedAt: "2026-03-12T10:00:00Z",
				Extensions: map[string]any{
					"project": "hao.news",
					"source": map[string]any{
						"name": "Unsigned Source",
					},
				},
				Origin: &MessageOrigin{
					AgentID: "agent://collector/unsigned",
				},
			},
		},
	}

	index := buildIndex(bundles, "hao.news")
	if len(index.Posts) != 1 {
		t.Fatalf("posts len = %d, want 1", len(index.Posts))
	}
	post := index.Posts[0]
	if post.HasSourcePage {
		t.Fatal("expected unsigned post to stay out of source pages")
	}
	if len(index.SourceStats) != 0 {
		t.Fatalf("source stats = %+v, want empty", index.SourceStats)
	}
}

func TestFilterPostsSupportsWindow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 12, 12, 0, 0, 0, time.UTC)
	index := Index{
		Posts: []Post{
			{Bundle: Bundle{InfoHash: "fresh", CreatedAt: now.Add(-6 * time.Hour)}},
			{Bundle: Bundle{InfoHash: "stale", CreatedAt: now.Add(-10 * 24 * time.Hour)}},
		},
	}

	got := index.FilterPosts(FeedOptions{Window: "24h", Now: now})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].InfoHash != "fresh" {
		t.Fatalf("infohash = %s, want fresh", got[0].InfoHash)
	}
}

func TestRelatedPosts(t *testing.T) {
	t.Parallel()

	const pubKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	index := Index{
		Posts: []Post{
			{
				Bundle:        Bundle{InfoHash: "base", CreatedAt: time.Date(2026, 3, 12, 10, 0, 0, 0, time.UTC)},
				SourceName:    pubKey,
				HasSourcePage: true,
				ChannelGroup:  "world",
				Topics:        []string{"energy", "world"},
			},
			{
				Bundle:        Bundle{InfoHash: "rel1", CreatedAt: time.Date(2026, 3, 12, 11, 0, 0, 0, time.UTC)},
				SourceName:    pubKey,
				HasSourcePage: true,
				ChannelGroup:  "world",
				Topics:        []string{"energy"},
			},
			{
				Bundle:       Bundle{InfoHash: "rel2", CreatedAt: time.Date(2026, 3, 12, 9, 0, 0, 0, time.UTC)},
				SourceName:   "Another",
				ChannelGroup: "world",
				Topics:       []string{"world"},
			},
		},
		PostByInfoHash: map[string]Post{},
	}
	for _, post := range index.Posts {
		index.PostByInfoHash[post.InfoHash] = post
	}

	got := index.RelatedPosts("base", 4)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].InfoHash != "rel1" {
		t.Fatalf("first = %s, want rel1", got[0].InfoHash)
	}
}

func TestLoadIndexMissingStoreReturnsEmpty(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "missing-store")
	index, err := LoadIndex(root, "hao.news")
	if err != nil {
		t.Fatalf("load index: %v", err)
	}
	if len(index.Bundles) != 0 {
		t.Fatalf("bundles len = %d, want 0", len(index.Bundles))
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) && err != nil {
		t.Fatalf("stat root: %v", err)
	}
}
