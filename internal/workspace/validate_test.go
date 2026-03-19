package workspace

import (
	"context"
	"html/template"
	"io/fs"
	"net/http"
	"testing"

	"hao.news/internal/apphost"
)

func TestValidatePluginManifest(t *testing.T) {
	manifest := apphost.PluginManifest{
		ID:           "sample-content",
		Name:         "Sample Content",
		BasePlugin:   "hao-news-content",
		DefaultTheme: "hao-news-theme",
	}
	report, err := ValidatePluginManifest(manifest, stubResolver{})
	if err != nil {
		t.Fatalf("validate plugin manifest: %v", err)
	}
	if report.Base == nil || report.Base.ID != "hao-news-content" {
		t.Fatalf("base manifest = %#v", report.Base)
	}
}

func TestValidateAppBundle(t *testing.T) {
	bundle := AppBundle{
		Root: t.TempDir(),
		App: apphost.AppManifest{
			ID:      "sample-app",
			Name:    "Sample App",
			Plugins: []string{"sample-content"},
			Theme:   "hao-news-theme",
		},
		Config: AppConfig{
			Project: "sample.project",
		},
		PluginConfigs: map[string]map[string]any{
			"sample-content": {
				"channel": "sample-world",
			},
		},
		PluginRoots: map[string]string{
			"sample-content": "/tmp/sample-content",
		},
	}
	registry := apphost.NewRegistry()
	registry.MustRegisterPlugin(pluginWithManifest(apphost.PluginManifest{
		ID:           "hao-news-content",
		Name:         "News Content",
		DefaultTheme: "hao-news-theme",
	}))
	registry.MustRegisterPlugin(pluginWithManifest(apphost.PluginManifest{
		ID:           "sample-content",
		Name:         "Sample Content",
		BasePlugin:   "hao-news-content",
		DefaultTheme: "hao-news-theme",
	}))
	registry.MustRegisterTheme(themeWithManifest(apphost.ThemeManifest{
		ID:               "hao-news-theme",
		Name:             "AiP2P Public Theme",
		SupportedPlugins: []string{"hao-news-content"},
		RequiredPlugins:  []string{"hao-news-content"},
	}))

	report, err := ValidateAppBundle(bundle, registry, registry)
	if err != nil {
		t.Fatalf("validate app bundle: %v", err)
	}
	if !report.Valid {
		t.Fatalf("valid = false")
	}
	if len(report.Plugins) != 1 || report.Plugins[0].Base == nil || report.Plugins[0].Base.ID != "hao-news-content" {
		t.Fatalf("plugins = %#v", report.Plugins)
	}
	if report.Config.Project != "sample.project" {
		t.Fatalf("project = %q", report.Config.Project)
	}
	if report.Plugins[0].Root != "/tmp/sample-content" {
		t.Fatalf("root = %q", report.Plugins[0].Root)
	}
	if got := report.Plugins[0].Config["channel"]; got != "sample-world" {
		t.Fatalf("config channel = %#v", got)
	}
}

type pluginWithManifest apphost.PluginManifest

func (p pluginWithManifest) Manifest() apphost.PluginManifest {
	return apphost.PluginManifest(p)
}

func (p pluginWithManifest) Build(context.Context, apphost.Config, apphost.WebTheme) (*apphost.Site, error) {
	return &apphost.Site{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})}, nil
}

type themeWithManifest apphost.ThemeManifest

func (t themeWithManifest) Manifest() apphost.ThemeManifest {
	return apphost.ThemeManifest(t)
}

func (themeWithManifest) ParseTemplates(template.FuncMap) (*template.Template, error) {
	return template.New("test"), nil
}

func (themeWithManifest) StaticFS() (fs.FS, error) {
	return nil, nil
}
