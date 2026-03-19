package newsplugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSyncMarkdownArchiveWritesUTCDateFolders(t *testing.T) {
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

	expected := filepath.Join(root, "2026-03-11", "post-abc123.md")
	data, err := os.ReadFile(expected)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "immutable local Markdown mirror") {
		t.Fatalf("archive missing mirror header: %s", text)
	}
	if !strings.Contains(text, "<p>HTML is allowed.</p>") {
		t.Fatalf("archive missing raw body: %s", text)
	}
	if index.Bundles[0].ArchiveMD != expected {
		t.Fatalf("archive path = %q, want %q", index.Bundles[0].ArchiveMD, expected)
	}
}
