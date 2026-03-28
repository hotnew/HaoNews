package newsplugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSyncMarkdownArchiveWritesUTCPlus8DateFolders(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	index := Index{
		Bundles: []Bundle{
			{
				InfoHash: "abc123",
				Magnet:   "magnet:?xt=urn:btih:abc123",
				Message: Message{
					Protocol:  "haonews/0.1",
					Kind:      "post",
					Author:    "agent://collector/a",
					CreatedAt: "2026-03-12T01:00:00+08:00",
					Title:     "Test story",
					Channel:   "hao.news/world",
					Extensions: map[string]any{
						"project": "hao.news",
					},
				},
				Body:      "<p>HTML is allowed.</p>\n\n```go\nfmt.Println(\"hi\")\n```",
				CreatedAt: time.Date(2026, 3, 11, 17, 0, 0, 0, time.UTC),
			},
		},
	}

	if err := SyncMarkdownArchive(&index, root); err != nil {
		t.Fatalf("sync archive: %v", err)
	}

	expected := filepath.Join(root, "2026-03-12", "post-abc123.md")
	data, err := os.ReadFile(expected)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "immutable local Markdown mirror") {
		t.Fatalf("archive missing mirror header: %s", text)
	}
	if !strings.Contains(text, "UTC+8 date folder") {
		t.Fatalf("archive missing UTC+8 header: %s", text)
	}
	if !strings.Contains(text, "<p>HTML is allowed.</p>") {
		t.Fatalf("archive missing raw body: %s", text)
	}
	if index.Bundles[0].ArchiveMD != expected {
		t.Fatalf("archive path = %q, want %q", index.Bundles[0].ArchiveMD, expected)
	}
}

func TestPrepareMarkdownArchiveSetsPathsWithoutWritingFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	index := Index{
		Bundles: []Bundle{{
			InfoHash: "abc123",
			Message: Message{
				Kind:      "post",
				CreatedAt: "2026-03-12T01:00:00+08:00",
				Extensions: map[string]any{
					"project": "hao.news",
				},
			},
			CreatedAt: time.Date(2026, 3, 11, 17, 0, 0, 0, time.UTC),
		}},
		Posts: []Post{{Bundle: Bundle{InfoHash: "abc123"}}},
		PostByInfoHash: map[string]Post{
			"abc123": {Bundle: Bundle{InfoHash: "abc123"}},
		},
		RepliesByPost:   map[string][]Reply{},
		ReactionsByPost: map[string][]Reaction{},
	}

	PrepareMarkdownArchive(&index, root)

	expected := filepath.Join(root, "2026-03-12", "post-abc123.md")
	if index.Bundles[0].ArchiveMD != expected {
		t.Fatalf("bundle archive path = %q, want %q", index.Bundles[0].ArchiveMD, expected)
	}
	if got := index.PostByInfoHash["abc123"].ArchiveMD; got != expected {
		t.Fatalf("post archive path = %q, want %q", got, expected)
	}
	if _, err := os.Stat(expected); !os.IsNotExist(err) {
		t.Fatalf("prepare archive should not write file, stat err = %v", err)
	}
}
