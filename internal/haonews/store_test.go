package haonews

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTorrentPathUsesHashShards(t *testing.T) {
	t.Parallel()

	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	got := store.TorrentPath("3c96913354d68bacf282bcde14c523a1aa36a6ed")
	want := filepath.Join(store.TorrentDir, "3c", "96", "3c96913354d68bacf282bcde14c523a1aa36a6ed.torrent")
	if got != want {
		t.Fatalf("TorrentPath = %q, want %q", got, want)
	}
}

func TestExistingTorrentPathFallsBackToLegacyLayout(t *testing.T) {
	t.Parallel()

	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	legacy := filepath.Join(store.TorrentDir, "3c96913354d68bacf282bcde14c523a1aa36a6ed.torrent")
	if err := os.WriteFile(legacy, []byte("legacy"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	got, err := store.ExistingTorrentPath("3c96913354d68bacf282bcde14c523a1aa36a6ed")
	if err != nil {
		t.Fatalf("ExistingTorrentPath error = %v", err)
	}
	if got != legacy {
		t.Fatalf("ExistingTorrentPath = %q, want %q", got, legacy)
	}
}

func TestWalkTorrentFilesFindsShardedAndLegacyOncePerInfoHash(t *testing.T) {
	t.Parallel()

	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	sharded := store.TorrentPath("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err := os.MkdirAll(filepath.Dir(sharded), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(sharded, []byte("new"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	legacy := filepath.Join(store.TorrentDir, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.torrent")
	if err := os.WriteFile(legacy, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	second := filepath.Join(store.TorrentDir, "bb", "bb", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.torrent")
	if err := os.MkdirAll(filepath.Dir(second), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(second, []byte("second"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	var hashes []string
	if err := store.WalkTorrentFiles(func(infoHash, _ string) error {
		hashes = append(hashes, infoHash)
		return nil
	}); err != nil {
		t.Fatalf("WalkTorrentFiles error = %v", err)
	}
	if len(hashes) != 2 {
		t.Fatalf("hash count = %d, want 2", len(hashes))
	}
}

func TestWalkTorrentFilesSkipsDeepNestedDirectories(t *testing.T) {
	t.Parallel()

	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	valid := filepath.Join(store.TorrentDir, "aa", "bb", "aabbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.torrent")
	if err := os.MkdirAll(filepath.Dir(valid), 0o755); err != nil {
		t.Fatalf("MkdirAll valid error = %v", err)
	}
	if err := os.WriteFile(valid, []byte("valid"), 0o644); err != nil {
		t.Fatalf("WriteFile valid error = %v", err)
	}
	deep := filepath.Join(store.TorrentDir, "aa", "bb", "cc", "dd", "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee.torrent")
	if err := os.MkdirAll(filepath.Dir(deep), 0o755); err != nil {
		t.Fatalf("MkdirAll deep error = %v", err)
	}
	if err := os.WriteFile(deep, []byte("deep"), 0o644); err != nil {
		t.Fatalf("WriteFile deep error = %v", err)
	}

	var hashes []string
	if err := store.WalkTorrentFiles(func(infoHash, _ string) error {
		hashes = append(hashes, infoHash)
		return nil
	}); err != nil {
		t.Fatalf("WalkTorrentFiles error = %v", err)
	}
	if len(hashes) != 1 {
		t.Fatalf("hash count = %d, want 1", len(hashes))
	}
	if hashes[0] != "aabbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("hashes[0] = %q, want valid shallow torrent", hashes[0])
	}
}
