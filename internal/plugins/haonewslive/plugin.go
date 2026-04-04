package haonewslive

import (
	"context"
	_ "embed"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"hao.news/internal/apphost"
	corehaonews "hao.news/internal/haonews"
	"hao.news/internal/haonews/live"
	newsplugin "hao.news/internal/plugins/haonews"
)

type Plugin struct{}

var liveAnnouncementWatcherDisabledForTests bool
var liveArchiveLoopDisabledForTests bool

//go:embed haonews.plugin.json
var pluginManifestJSON []byte

func (Plugin) Manifest() apphost.PluginManifest {
	return apphost.MustLoadPluginManifestJSON(pluginManifestJSON)
}

func (Plugin) Build(ctx context.Context, cfg apphost.Config, theme apphost.WebTheme) (*apphost.Site, error) {
	cfg = newsplugin.ApplyDefaultConfig(cfg)
	options := newsplugin.OptionsForPlugins(newsplugin.AppOptions{}, cfg)
	app, err := newsplugin.NewWithThemeAndOptions(
		cfg.StoreRoot,
		cfg.Project,
		cfg.Version,
		cfg.ArchiveRoot,
		cfg.RulesPath,
		cfg.WriterPolicyPath,
		cfg.NetPath,
		theme,
		options,
	)
	if err != nil {
		return nil, err
	}
	var redisCfg corehaonews.RedisConfig
	if strings.TrimSpace(cfg.NetPath) != "" {
		netCfg, loadErr := corehaonews.LoadNetworkBootstrapConfig(cfg.NetPath)
		if loadErr != nil {
			return nil, loadErr
		}
		redisCfg = netCfg.Redis
	}
	store, err := live.OpenLocalStoreWithRedis(cfg.StoreRoot, redisCfg)
	if err != nil {
		return nil, err
	}
	logf := cfg.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	watchCtx, cancelWatch := context.WithCancel(ctx)
	var watcherMu sync.Mutex
	var watcher *live.AnnouncementWatcher
	if watcherNetPath, ok := liveAnnouncementWatcherNetPath(cfg); ok {
		go func() {
			startedWatcher, startErr := live.StartAnnouncementWatcher(watchCtx, cfg.StoreRoot, watcherNetPath)
			if startErr != nil {
				logf("haonews live: announcement watcher disabled: %v", startErr)
				return
			}
			watcherMu.Lock()
			watcher = startedWatcher
			watcherMu.Unlock()
		}()
	}
	if !disableLiveArchiveLoop() {
		go startLiveArchiveLoop(ctx, store, logf)
	}
	if !strings.HasSuffix(filepath.Base(strings.TrimSpace(os.Args[0])), ".test") {
		go startLiveArchiveWarmup(ctx, app, store)
	}
	staticFS, err := theme.StaticFS()
	if err != nil {
		cancelWatch()
		return nil, err
	}
	return &apphost.Site{
		Manifest: Plugin{}.Manifest(),
		Theme:    theme.Manifest(),
		Handler:  newHandler(app, store, staticFS),
		Close: func(context.Context) error {
			cancelWatch()
			watcherMu.Lock()
			startedWatcher := watcher
			watcher = nil
			watcherMu.Unlock()
			storeErr := store.Close()
			if startedWatcher != nil {
				if err := startedWatcher.Close(); err != nil {
					return err
				}
			}
			return storeErr
		},
	}, nil
}

func startLiveArchiveWarmup(ctx context.Context, app *newsplugin.App, store *live.LocalStore) {
	const warmupInterval = 45 * time.Second
	ticker := time.NewTicker(warmupInterval)
	defer ticker.Stop()
	for {
		warmLiveArchive(app, store)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func warmLiveArchive(app *newsplugin.App, store *live.LocalStore) {
	if app == nil || store == nil {
		return
	}
	if index, err := app.Index(); err == nil {
		_ = app.NodeStatus(index)
	}
	_, _, _ = loadLiveArchiveRoomSummaries(store)
}

func disableLiveAnnouncementWatcher() bool {
	if liveAnnouncementWatcherDisabledForTests {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(os.Getenv("HAONEWS_DISABLE_LIVE_ANNOUNCEMENT_WATCHER")), "1") ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("HAONEWS_DISABLE_LIVE_ANNOUNCEMENT_WATCHER")), "true")
}

func liveAnnouncementWatcherNetPath(cfg apphost.Config) (string, bool) {
	if disableLiveAnnouncementWatcher() {
		return "", false
	}
	netPath := strings.TrimSpace(cfg.NetPath)
	switch strings.ToLower(strings.TrimSpace(cfg.SyncMode)) {
	case "", "managed", "external":
		liveNetPath := filepath.Join(filepath.Dir(netPath), "hao_news_live_net.inf")
		if strings.TrimSpace(netPath) != "" && filepath.Clean(liveNetPath) == filepath.Clean(netPath) {
			return "", false
		}
		if _, err := os.Stat(liveNetPath); err != nil {
			return "", false
		}
		return liveNetPath, true
	default:
		if netPath == "" {
			return "", false
		}
		return netPath, true
	}
}

func disableLiveArchiveLoop() bool {
	if liveArchiveLoopDisabledForTests {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(os.Getenv("HAONEWS_DISABLE_LIVE_ARCHIVE_LOOP")), "1") ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("HAONEWS_DISABLE_LIVE_ARCHIVE_LOOP")), "true")
}

func newHandler(app *newsplugin.App, store *live.LocalStore, staticFS fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/live" {
			http.NotFound(w, r)
			return
		}
		handleLiveIndex(app, store, w, r)
	})
	mux.HandleFunc("/live/public", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/live/public" {
			http.NotFound(w, r)
			return
		}
		handleLiveRoom(app, store, publicLiveRootRoomID, w, r)
	})
	mux.HandleFunc("/live/public/moderation", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/live/public/moderation" {
			http.NotFound(w, r)
			return
		}
		handleLivePublicModeration(app, w, r)
	})
	mux.HandleFunc("/live/public/", func(w http.ResponseWriter, r *http.Request) {
		slug := strings.TrimSpace(newsplugin.PathValue("/live/public/", r.URL.Path))
		roomID := publicLivePathToRoomID(slug)
		if roomID == "" {
			http.NotFound(w, r)
			return
		}
		handleLiveRoom(app, store, roomID, w, r)
	})
	mux.HandleFunc("/live/pending", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/live/pending" {
			http.NotFound(w, r)
			return
		}
		handleLivePendingIndex(app, store, w, r)
	})
	mux.HandleFunc("/live/pending/", func(w http.ResponseWriter, r *http.Request) {
		roomID := strings.TrimSpace(newsplugin.PathValue("/live/pending/", r.URL.Path))
		if roomID == "" {
			http.NotFound(w, r)
			return
		}
		handleLivePendingRoom(app, store, roomID, w, r)
	})
	mux.HandleFunc("/live/history/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/live/history/") {
			http.NotFound(w, r)
			return
		}
		rest := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/live/history/"))
		if rest == "" {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(rest, "/")
		roomID := strings.TrimSpace(parts[0])
		if roomID == "" {
			http.NotFound(w, r)
			return
		}
		archiveID := ""
		if len(parts) > 1 {
			archiveID = strings.TrimSpace(parts[1])
		}
		handleLiveRoomHistory(app, store, roomID, archiveID, w, r)
	})
	mux.HandleFunc("/live/archive/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		roomID := strings.TrimSpace(newsplugin.PathValue("/live/archive/", r.URL.Path))
		if roomID == "" {
			http.NotFound(w, r)
			return
		}
		handleLiveArchiveNow(app, store, roomID, w, r)
	})
	mux.HandleFunc("/live/", func(w http.ResponseWriter, r *http.Request) {
		roomID := strings.TrimSpace(newsplugin.PathValue("/live/", r.URL.Path))
		if roomID == "" {
			http.NotFound(w, r)
			return
		}
		handleLiveRoom(app, store, roomID, w, r)
	})
	mux.HandleFunc("/api/live/rooms", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/live/rooms" {
			http.NotFound(w, r)
			return
		}
		handleAPILiveRooms(app, store, w, r)
	})
	mux.HandleFunc("/api/live/public", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/live/public" {
			http.NotFound(w, r)
			return
		}
		handleAPILiveRoom(app, store, publicLiveRootRoomID, w, r)
	})
	mux.HandleFunc("/api/live/public/moderation", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/live/public/moderation" {
			http.NotFound(w, r)
			return
		}
		handleAPILivePublicModeration(app, w, r)
	})
	mux.HandleFunc("/api/live/public/", func(w http.ResponseWriter, r *http.Request) {
		slug := strings.TrimSpace(newsplugin.PathValue("/api/live/public/", r.URL.Path))
		roomID := publicLivePathToRoomID(slug)
		if roomID == "" {
			http.NotFound(w, r)
			return
		}
		handleAPILiveRoom(app, store, roomID, w, r)
	})
	mux.HandleFunc("/api/live/pending", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/live/pending" {
			http.NotFound(w, r)
			return
		}
		handleAPILivePendingRooms(app, store, w, r)
	})
	mux.HandleFunc("/api/live/pending/", func(w http.ResponseWriter, r *http.Request) {
		roomID := strings.TrimSpace(newsplugin.PathValue("/api/live/pending/", r.URL.Path))
		if roomID == "" {
			http.NotFound(w, r)
			return
		}
		handleAPILivePendingRoom(app, store, roomID, w, r)
	})
	mux.HandleFunc("/api/live/history/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/live/history/") {
			http.NotFound(w, r)
			return
		}
		rest := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/live/history/"))
		if rest == "" {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(rest, "/")
		roomID := strings.TrimSpace(parts[0])
		if roomID == "" {
			http.NotFound(w, r)
			return
		}
		archiveID := ""
		if len(parts) > 1 {
			archiveID = strings.TrimSpace(parts[1])
		}
		handleAPILiveRoomHistory(store, roomID, archiveID, w, r)
	})
	mux.HandleFunc("/api/live/archive/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		roomID := strings.TrimSpace(newsplugin.PathValue("/api/live/archive/", r.URL.Path))
		if roomID == "" {
			http.NotFound(w, r)
			return
		}
		handleAPILiveArchiveNow(store, roomID, w, r)
	})
	mux.HandleFunc("/archive/live", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/archive/live" {
			http.NotFound(w, r)
			return
		}
		handleLiveArchiveIndex(app, store, w, r)
	})
	mux.HandleFunc("/api/archive/live", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/archive/live" {
			http.NotFound(w, r)
			return
		}
		handleAPILiveArchiveIndex(store, w, r)
	})
	mux.HandleFunc("/archive/live/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/archive/live/") {
			http.NotFound(w, r)
			return
		}
		rest := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/archive/live/"))
		if rest == "" {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(rest, "/")
		roomID := strings.TrimSpace(parts[0])
		if roomID == "" {
			http.NotFound(w, r)
			return
		}
		archiveID := ""
		if len(parts) > 1 {
			archiveID = strings.TrimSpace(parts[1])
		}
		handleLiveRoomHistory(app, store, roomID, archiveID, w, r)
	})
	mux.HandleFunc("/api/archive/live/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/archive/live/") {
			http.NotFound(w, r)
			return
		}
		rest := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/archive/live/"))
		if rest == "" {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(rest, "/")
		roomID := strings.TrimSpace(parts[0])
		if roomID == "" {
			http.NotFound(w, r)
			return
		}
		archiveID := ""
		if len(parts) > 1 {
			archiveID = strings.TrimSpace(parts[1])
		}
		handleAPILiveRoomHistory(store, roomID, archiveID, w, r)
	})
	mux.HandleFunc("/api/live/rooms/", func(w http.ResponseWriter, r *http.Request) {
		roomID := strings.TrimSpace(newsplugin.PathValue("/api/live/rooms/", r.URL.Path))
		if roomID == "" {
			http.NotFound(w, r)
			return
		}
		handleAPILiveRoom(app, store, roomID, w, r)
	})
	mux.Handle("/static/", newsplugin.NoStoreStaticHandler(staticFS))
	return mux
}

func startLiveArchiveLoop(ctx context.Context, store *live.LocalStore, logf func(string, ...any)) {
	run := func(now time.Time) {
		rooms, err := store.ListRooms()
		if err != nil {
			logf("haonews live: list rooms for archive loop: %v", err)
			return
		}
		for _, room := range rooms {
			if _, err := store.EnsureDailyHistoryArchives(room.RoomID, now); err != nil {
				logf("haonews live: ensure daily archive for %s: %v", room.RoomID, err)
			}
		}
	}
	run(time.Now())
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			run(now)
		}
	}
}
