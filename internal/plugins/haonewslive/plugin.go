package haonewslive

import (
	"context"
	_ "embed"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"sync"

	"hao.news/internal/apphost"
	corehaonews "hao.news/internal/haonews"
	"hao.news/internal/haonews/live"
	newsplugin "hao.news/internal/plugins/haonews"
)

type Plugin struct{}

var liveAnnouncementWatcherDisabledForTests bool

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
	if !disableLiveAnnouncementWatcher() {
		go func() {
			startedWatcher, startErr := live.StartAnnouncementWatcher(watchCtx, cfg.StoreRoot, cfg.NetPath)
			if startErr != nil {
				logf("haonews live: announcement watcher disabled: %v", startErr)
				return
			}
			watcherMu.Lock()
			watcher = startedWatcher
			watcherMu.Unlock()
		}()
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

func disableLiveAnnouncementWatcher() bool {
	if liveAnnouncementWatcherDisabledForTests {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(os.Getenv("HAONEWS_DISABLE_LIVE_ANNOUNCEMENT_WATCHER")), "1") ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("HAONEWS_DISABLE_LIVE_ANNOUNCEMENT_WATCHER")), "true")
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
