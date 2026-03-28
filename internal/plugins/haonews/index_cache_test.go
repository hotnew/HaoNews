package newsplugin

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppIndexCachesUntilStoreSignatureChanges(t *testing.T) {
	t.Parallel()

	storeRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(storeRoot, "data"), 0o755); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(storeRoot, "torrents"), 0o755); err != nil {
		t.Fatalf("mkdir torrents: %v", err)
	}

	oldInterval := indexCacheProbeInterval
	indexCacheProbeInterval = 10 * time.Millisecond
	defer func() { indexCacheProbeInterval = oldInterval }()

	loads := 0
	app := &App{
		storeRoot:  storeRoot,
		project:    "hao.news",
		rulesPath:  filepath.Join(storeRoot, "config", "subscriptions.json"),
		writerPath: filepath.Join(storeRoot, "config", "writer_policy.json"),
		archive:    "",
		loadIndex: func(_, _ string) (Index, error) {
			loads++
			return Index{
				PostByInfoHash:  map[string]Post{},
				RepliesByPost:   map[string][]Reply{},
				ReactionsByPost: map[string][]Reaction{},
			}, nil
		},
	}

	if _, err := app.Index(); err != nil {
		t.Fatalf("first index: %v", err)
	}
	if _, err := app.Index(); err != nil {
		t.Fatalf("second index: %v", err)
	}
	if loads != 1 {
		t.Fatalf("load count = %d, want 1", loads)
	}

	time.Sleep(indexCacheProbeInterval + 10*time.Millisecond)
	if err := os.WriteFile(filepath.Join(storeRoot, "data", "touch.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write touch file: %v", err)
	}

	if _, err := app.Index(); err != nil {
		t.Fatalf("third index: %v", err)
	}
	if loads != 2 {
		t.Fatalf("load count after store change = %d, want 2", loads)
	}
}
