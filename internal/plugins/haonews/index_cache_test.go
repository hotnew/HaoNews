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

func TestCurrentIndexSignatureUsesQuickProbeCacheBetweenDeepChecks(t *testing.T) {
	t.Parallel()

	storeRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(storeRoot, "data"), 0o755); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(storeRoot, "torrents"), 0o755); err != nil {
		t.Fatalf("mkdir torrents: %v", err)
	}

	oldDeepInterval := indexCacheDeepProbeInterval
	indexCacheDeepProbeInterval = time.Hour
	defer func() { indexCacheDeepProbeInterval = oldDeepInterval }()

	oldFullFunc := currentIndexFullSignatureFunc
	fullCalls := 0
	currentIndexFullSignatureFunc = func(a *App, now time.Time, quickSignature string) (string, error) {
		fullCalls++
		return a.currentIndexFullSignature(now, quickSignature)
	}
	defer func() { currentIndexFullSignatureFunc = oldFullFunc }()

	app := &App{
		storeRoot:  storeRoot,
		rulesPath:  filepath.Join(storeRoot, "config", "subscriptions.json"),
		writerPath: filepath.Join(storeRoot, "config", "writer_policy.json"),
	}

	if _, err := app.currentIndexSignature(); err != nil {
		t.Fatalf("first currentIndexSignature: %v", err)
	}
	if fullCalls == 0 {
		t.Fatalf("full signature not called")
	}
	firstCalls := fullCalls

	if _, err := app.currentIndexSignature(); err != nil {
		t.Fatalf("second currentIndexSignature: %v", err)
	}
	if _, err := app.currentIndexQuickSignature(); err != nil {
		t.Fatalf("quick signature after second call: %v", err)
	}

	if err := os.WriteFile(filepath.Join(storeRoot, "data", "touch.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write touch file: %v", err)
	}
	if _, err := app.currentIndexSignature(); err != nil {
		t.Fatalf("third currentIndexSignature: %v", err)
	}
	if fullCalls <= firstCalls {
		t.Fatalf("full signature calls after quick change = %d, want > %d", fullCalls, firstCalls)
	}
}

func TestContentSignatureForIndexIgnoresProbeOnlyChanges(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)
	indexA := Index{
		Posts: []Post{
			{
				Bundle: Bundle{
					InfoHash:  "abc123",
					CreatedAt: now,
					Message: Message{
						Title:   "Same post",
						Channel: "hao.news/news",
					},
				},
				ChannelGroup: "news",
				SourceName:   "world",
				Topics:       []string{"world"},
			},
		},
		ChannelStats: []FacetStat{{Name: "news", Count: 1}},
		TopicStats:   []FacetStat{{Name: "world", Count: 1}},
		SourceStats:  []FacetStat{{Name: "world", Count: 1}},
	}
	indexB := indexA.Clone()
	indexB.Posts[0].Dir = "/tmp/different-probe-only-path"
	indexB.Posts[0].Magnet = "magnet:?xt=urn:btih:different"

	if gotA, gotB := contentSignatureForIndex(indexA), contentSignatureForIndex(indexB); gotA != gotB {
		t.Fatalf("content signature changed for probe-only differences: %q != %q", gotA, gotB)
	}
}
