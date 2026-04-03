package haonewscontent

import (
	"context"
	_ "embed"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hao.news/internal/apphost"
	newsplugin "hao.news/internal/plugins/haonews"
)

type Plugin struct{}

//go:embed haonews.plugin.json
var pluginManifestJSON []byte

func (Plugin) Manifest() apphost.PluginManifest {
	return apphost.MustLoadPluginManifestJSON(pluginManifestJSON)
}

func (Plugin) Build(ctx context.Context, cfg apphost.Config, theme apphost.WebTheme) (*apphost.Site, error) {
	cfg = newsplugin.ApplyDefaultConfig(cfg)
	options := newsplugin.OptionsForPlugins(newsplugin.ContentOnlyAppOptions(), cfg)
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
	if !strings.HasSuffix(filepath.Base(os.Args[0]), ".test") {
		startBackgroundIndexWarmup(ctx, app)
	}
	stopSync, err := newsplugin.StartManagedSyncIfNeeded(ctx, cfg, options)
	if err != nil {
		return nil, err
	}
	staticFS, err := theme.StaticFS()
	if err != nil {
		return nil, err
	}
	return &apphost.Site{
		Manifest: Plugin{}.Manifest(),
		Theme:    theme.Manifest(),
		Handler:  newHandler(app, staticFS),
		Close: func(context.Context) error {
			stopSync()
			return nil
		},
	}, nil
}

func startBackgroundIndexWarmup(ctx context.Context, app *newsplugin.App) {
	const warmupInterval = 2 * time.Second
	app.SetWarmupPending()
	go func() {
		lastDerivedSignature := ""
		ticker := time.NewTicker(warmupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if index, err := app.Index(); err == nil {
				_ = app.NodeStatus(index)
				if signature, ok := app.CachedIndexSignature(); ok && signature != lastDerivedSignature {
					warmDerivedContentCaches(app, index)
					app.SetWarmupReady()
					lastDerivedSignature = signature
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func warmDerivedContentCaches(app *newsplugin.App, index newsplugin.Index) {
	now := time.Now()
	_, _ = app.FilteredPosts(index, newsplugin.FeedOptions{Now: now})
	_, _ = app.TopicDirectory(index, newsplugin.FeedOptions{Now: now})
	const warmTopicLimit = 6
	topics := make([]string, 0, warmTopicLimit)
	for i, stat := range index.TopicStats {
		if i >= warmTopicLimit {
			break
		}
		topic := strings.TrimSpace(stat.Name)
		if topic == "" {
			continue
		}
		_, _ = app.FilteredPosts(index, newsplugin.FeedOptions{Topic: topic, Now: now})
		topics = append(topics, topic)
	}
	warmTopicRSSCaches(app, index, topics)
	warmHTTPResponseCaches(app, topics)
}

func warmTopicRSSCaches(app *newsplugin.App, index newsplugin.Index, topics []string) {
	indexSig, ok := app.CachedIndexSignature()
	if !ok || strings.TrimSpace(indexSig) == "" {
		return
	}
	baseURL := warmupBaseURL(app.HTTPListenAddr())
	if baseURL == "" {
		return
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return
	}
	for _, topic := range topics {
		opts := newsplugin.FeedOptions{Topic: topic}
		rssPath := newsplugin.TopicRSSPath(topic)
		if rssPath == "" {
			continue
		}
		u := *base
		u.Path = rssPath
		req := &http.Request{URL: &u, Host: u.Host}
		_, _ = fetchTopicRSSResponseVariant(app, req, topic, opts, indexSig)
	}
}

func warmHTTPResponseCaches(app *newsplugin.App, topics []string) {
	baseURL := warmupBaseURL(app.HTTPListenAddr())
	if baseURL == "" {
		return
	}
	client := &http.Client{Timeout: 3 * time.Second}
	paths := []string{
		"/",
		"/topics",
		"/api/feed",
	}
	for _, topic := range topics {
		path := newsplugin.TopicPath(topic)
		if path == "" {
			continue
		}
		paths = append(paths, path, path+"/rss")
	}
	for _, path := range paths {
		req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (HaoNews warmup)")
		req.Header.Set("Cookie", "hao_news_network_warning_seen=1")
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		_ = resp.Body.Close()
	}
}

func warmupBaseURL(listenAddr string) string {
	listenAddr = strings.TrimSpace(listenAddr)
	if listenAddr == "" {
		return ""
	}
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		if strings.HasPrefix(listenAddr, ":") {
			return "http://127.0.0.1" + listenAddr
		}
		return ""
	}
	host = strings.TrimSpace(host)
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}
