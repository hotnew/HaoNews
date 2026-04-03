package apphost

import (
	"context"
	"errors"
	"html/template"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testTheme struct{}

func (testTheme) Manifest() ThemeManifest {
	return ThemeManifest{ID: "test-theme", Name: "Test Theme", SupportedPlugins: []string{"test-plugin"}}
}

func (testTheme) ParseTemplates(template.FuncMap) (*template.Template, error) {
	return template.New("test"), nil
}

func (testTheme) StaticFS() (fs.FS, error) {
	return nil, nil
}

type testPlugin struct {
	build func(context.Context, Config, WebTheme) (*Site, error)
}

func (p testPlugin) Manifest() PluginManifest {
	return PluginManifest{ID: "test-plugin", Name: "Test Plugin", DefaultTheme: "test-theme"}
}

func (p testPlugin) Build(ctx context.Context, cfg Config, theme WebTheme) (*Site, error) {
	return p.build(ctx, cfg, theme)
}

func TestRegistryWrapsHandlerPanics(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegisterTheme(testTheme{})
	registry.MustRegisterPlugin(testPlugin{
		build: func(context.Context, Config, WebTheme) (*Site, error) {
			return &Site{
				Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					panic("boom")
				}),
			}, nil
		},
	})

	site, err := registry.Build(context.Background(), Config{Plugin: "test-plugin"})
	if err != nil {
		t.Fatalf("build site: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestRegistrySurfacesStartupErrors(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegisterTheme(testTheme{})
	registry.MustRegisterPlugin(testPlugin{
		build: func(context.Context, Config, WebTheme) (*Site, error) {
			return nil, errors.New("init failed")
		},
	})

	_, err := registry.Build(context.Background(), Config{Plugin: "test-plugin"})
	if err == nil || err.Error() != "init failed" {
		t.Fatalf("err = %v, want init failed", err)
	}
}

func TestRegistryBuildsCompositeSite(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegisterTheme(themeWithManifest{
		manifest: ThemeManifest{
			ID:               "test-theme",
			Name:             "Test Theme",
			SupportedPlugins: []string{"first-plugin", "second-plugin"},
		},
	})
	registry.MustRegisterPlugin(namedTestPlugin("first-plugin", func(context.Context, Config, WebTheme) (*Site, error) {
		return &Site{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/" {
					http.NotFound(w, r)
					return
				}
				_, _ = w.Write([]byte("home"))
			}),
		}, nil
	}))
	registry.MustRegisterPlugin(namedTestPlugin("second-plugin", func(context.Context, Config, WebTheme) (*Site, error) {
		return &Site{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/network" {
					http.NotFound(w, r)
					return
				}
				_, _ = w.Write([]byte("network"))
			}),
		}, nil
	}))

	site, err := registry.Build(context.Background(), Config{
		Plugins: []string{"first-plugin", "second-plugin"},
		Theme:   "test-theme",
	})
	if err != nil {
		t.Fatalf("build composite site: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/network", nil)
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "network" {
		t.Fatalf("status/body = %d %q, want 200 %q", rec.Code, rec.Body.String(), "network")
	}
}

func TestRegistryCompositeSiteSupportsStreamingHandlers(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegisterTheme(themeWithManifest{
		manifest: ThemeManifest{
			ID:               "test-theme",
			Name:             "Test Theme",
			SupportedPlugins: []string{"first-plugin", "second-plugin"},
		},
	})
	registry.MustRegisterPlugin(namedTestPlugin("first-plugin", func(context.Context, Config, WebTheme) (*Site, error) {
		return &Site{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.NotFound(w, r)
			}),
		}, nil
	}))
	registry.MustRegisterPlugin(namedTestPlugin("second-plugin", func(context.Context, Config, WebTheme) (*Site, error) {
		return &Site{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/stream" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				flusher, ok := w.(http.Flusher)
				if !ok {
					t.Fatalf("composite recorder did not expose http.Flusher")
				}
				_, _ = w.Write([]byte(": hello\n\n"))
				flusher.Flush()
				_, _ = w.Write([]byte("event: team\ndata: ok\n\n"))
			}),
		}, nil
	}))

	site, err := registry.Build(context.Background(), Config{
		Plugins: []string{"first-plugin", "second-plugin"},
		Theme:   "test-theme",
	})
	if err != nil {
		t.Fatalf("build composite site: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("content-type = %q", got)
	}
	if !strings.Contains(rec.Body.String(), "event: team") {
		t.Fatalf("body = %q, want SSE payload", rec.Body.String())
	}
}

func TestRegistryScopesRuntimePathsPerPlugin(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegisterTheme(themeWithManifest{
		manifest: ThemeManifest{
			ID:               "test-theme",
			Name:             "Test Theme",
			SupportedPlugins: []string{"first-plugin", "second-plugin"},
		},
	})
	var firstCfg, secondCfg Config
	registry.MustRegisterPlugin(namedTestPlugin("first-plugin", func(_ context.Context, cfg Config, _ WebTheme) (*Site, error) {
		firstCfg = cfg
		return &Site{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})}, nil
	}))
	registry.MustRegisterPlugin(namedTestPlugin("second-plugin", func(_ context.Context, cfg Config, _ WebTheme) (*Site, error) {
		secondCfg = cfg
		return &Site{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})}, nil
	}))

	_, err := registry.Build(context.Background(), Config{
		Plugins:          []string{"first-plugin", "second-plugin"},
		Theme:            "test-theme",
		RuntimeRoot:      "/tmp/runtime",
		StoreRoot:        "/tmp/store",
		ArchiveRoot:      "/tmp/archive",
		RulesPath:        "/tmp/config/subscriptions.json",
		WriterPolicyPath: "/tmp/config/writer_policy.json",
		NetPath:          "/tmp/config/haonews_net.inf",
		TrackerPath:      "/tmp/config/Trackerlist.inf",
	})
	if err != nil {
		t.Fatalf("build composite site: %v", err)
	}

	if firstCfg.RuntimeRoot != "/tmp/runtime/plugins/first-plugin" {
		t.Fatalf("first runtime root = %q", firstCfg.RuntimeRoot)
	}
	if secondCfg.RuntimeRoot != "/tmp/runtime/plugins/second-plugin" {
		t.Fatalf("second runtime root = %q", secondCfg.RuntimeRoot)
	}
	if firstCfg.StoreRoot != "/tmp/store" || secondCfg.StoreRoot != "/tmp/store" {
		t.Fatalf("store roots = %q / %q", firstCfg.StoreRoot, secondCfg.StoreRoot)
	}
	if firstCfg.ArchiveRoot != "/tmp/archive" || secondCfg.ArchiveRoot != "/tmp/archive" {
		t.Fatalf("archive roots = %q / %q", firstCfg.ArchiveRoot, secondCfg.ArchiveRoot)
	}
	if firstCfg.RulesPath != "/tmp/config/subscriptions.json" || secondCfg.RulesPath != "/tmp/config/subscriptions.json" {
		t.Fatalf("rules paths = %q / %q", firstCfg.RulesPath, secondCfg.RulesPath)
	}
	if firstCfg.WriterPolicyPath != "/tmp/config/writer_policy.json" || secondCfg.WriterPolicyPath != "/tmp/config/writer_policy.json" {
		t.Fatalf("writer policy paths = %q / %q", firstCfg.WriterPolicyPath, secondCfg.WriterPolicyPath)
	}
	if firstCfg.NetPath != "/tmp/config/haonews_net.inf" || secondCfg.NetPath != "/tmp/config/haonews_net.inf" {
		t.Fatalf("net paths = %q / %q", firstCfg.NetPath, secondCfg.NetPath)
	}
	if firstCfg.TrackerPath != "/tmp/config/Trackerlist.inf" || secondCfg.TrackerPath != "/tmp/config/Trackerlist.inf" {
		t.Fatalf("tracker paths = %q / %q", firstCfg.TrackerPath, secondCfg.TrackerPath)
	}
}

func TestRegistryRejectsIncompatibleTheme(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegisterTheme(testTheme{})
	registry.MustRegisterPlugin(testPlugin{
		build: func(context.Context, Config, WebTheme) (*Site, error) {
			return &Site{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})}, nil
		},
	})
	registry.MustRegisterTheme(themeWithManifest{
		ThemeManifest{ID: "other-theme", Name: "Other", SupportedPlugins: []string{"another-plugin"}},
	})

	_, err := registry.Build(context.Background(), Config{
		Plugin: "test-plugin",
		Theme:  "other-theme",
	})
	if err == nil || err.Error() != `theme "other-theme" does not support plugin "test-plugin"` {
		t.Fatalf("err = %v", err)
	}
}

func TestRegistryRejectsThemeWithMissingRequiredPlugin(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegisterTheme(themeWithManifest{
		manifest: ThemeManifest{
			ID:               "stacked-theme",
			Name:             "Stacked Theme",
			SupportedPlugins: []string{"test-plugin"},
			RequiredPlugins:  []string{"test-plugin", "extra-plugin"},
		},
	})
	registry.MustRegisterPlugin(testPlugin{
		build: func(context.Context, Config, WebTheme) (*Site, error) {
			return &Site{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})}, nil
		},
	})

	_, err := registry.Build(context.Background(), Config{
		Plugin: "test-plugin",
		Theme:  "stacked-theme",
	})
	if err == nil || err.Error() != `theme "stacked-theme" requires plugins: extra-plugin` {
		t.Fatalf("err = %v", err)
	}
}

func TestRegistryAcceptsThemeCompatibilityViaBasePlugin(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegisterTheme(themeWithManifest{
		manifest: ThemeManifest{
			ID:               "hao-news-theme",
			Name:             "Hao.News Public Theme",
			SupportedPlugins: []string{"hao-news-content"},
			RequiredPlugins:  []string{"hao-news-content"},
		},
	})
	registry.MustRegisterPlugin(pluginWithManifest{
		manifest: PluginManifest{
			ID:           "sample-content",
			Name:         "Sample Content",
			BasePlugin:   "hao-news-content",
			DefaultTheme: "hao-news-theme",
		},
		build: func(context.Context, Config, WebTheme) (*Site, error) {
			return &Site{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})}, nil
		},
	})

	if _, err := registry.Build(context.Background(), Config{
		Plugin: "sample-content",
		Theme:  "hao-news-theme",
	}); err != nil {
		t.Fatalf("build with base-plugin compatibility: %v", err)
	}
}

type themeWithManifest struct {
	manifest ThemeManifest
}

func (t themeWithManifest) Manifest() ThemeManifest {
	return t.manifest
}

func (themeWithManifest) ParseTemplates(template.FuncMap) (*template.Template, error) {
	return template.New("test"), nil
}

func (themeWithManifest) StaticFS() (fs.FS, error) {
	return nil, nil
}

type namedTestPluginValue struct {
	id    string
	build func(context.Context, Config, WebTheme) (*Site, error)
}

func namedTestPlugin(id string, build func(context.Context, Config, WebTheme) (*Site, error)) namedTestPluginValue {
	return namedTestPluginValue{id: id, build: build}
}

func (p namedTestPluginValue) Manifest() PluginManifest {
	return PluginManifest{ID: p.id, Name: p.id, DefaultTheme: "test-theme"}
}

func (p namedTestPluginValue) Build(ctx context.Context, cfg Config, theme WebTheme) (*Site, error) {
	return p.build(ctx, cfg, theme)
}

type pluginWithManifest struct {
	manifest PluginManifest
	build    func(context.Context, Config, WebTheme) (*Site, error)
}

func (p pluginWithManifest) Manifest() PluginManifest {
	return p.manifest
}

func (p pluginWithManifest) Build(ctx context.Context, cfg Config, theme WebTheme) (*Site, error) {
	return p.build(ctx, cfg, theme)
}
