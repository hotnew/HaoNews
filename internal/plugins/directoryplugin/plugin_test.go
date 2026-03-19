package directoryplugin

import (
	"context"
	"html/template"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"hao.news/internal/apphost"
)

type testResolver struct {
	plugin apphost.HTTPPlugin
}

func (r testResolver) ResolvePlugin(id string) (apphost.HTTPPlugin, apphost.PluginManifest, error) {
	return r.plugin, r.plugin.Manifest(), nil
}

type testBasePlugin struct{}

func (testBasePlugin) Manifest() apphost.PluginManifest {
	return apphost.PluginManifest{ID: "hao-news-content", Name: "News Content", DefaultTheme: "hao-news-theme"}
}

func (testBasePlugin) Build(context.Context, apphost.Config, apphost.WebTheme) (*apphost.Site, error) {
	return &apphost.Site{
		Manifest: apphost.PluginManifest{ID: "hao-news-content", Name: "News Content", DefaultTheme: "hao-news-theme"},
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("ok"))
		}),
	}, nil
}

type testTheme struct{}

func (testTheme) Manifest() apphost.ThemeManifest {
	return apphost.ThemeManifest{ID: "hao-news-theme", Name: "AiP2P Public Theme"}
}

func (testTheme) ParseTemplates(template.FuncMap) (*template.Template, error) {
	return template.New("test"), nil
}

func (testTheme) StaticFS() (fs.FS, error) {
	return nil, nil
}

func TestLoadBuildsDelegatingPlugin(t *testing.T) {
	root := t.TempDir()
	writePluginFile(t, root, "aip2p.plugin.json", "{\n  \"id\": \"sample-content\",\n  \"name\": \"Sample Content\",\n  \"base_plugin\": \"hao-news-content\",\n  \"default_theme\": \"hao-news-theme\"\n}\n")

	plugin, err := Load(root, testResolver{plugin: testBasePlugin{}})
	if err != nil {
		t.Fatalf("load plugin: %v", err)
	}
	if plugin.Manifest().ID != "sample-content" {
		t.Fatalf("plugin manifest id = %q", plugin.Manifest().ID)
	}

	site, err := plugin.Build(context.Background(), apphost.Config{Plugin: "sample-content"}, testTheme{})
	if err != nil {
		t.Fatalf("build plugin: %v", err)
	}
	if site.Manifest.ID != "sample-content" {
		t.Fatalf("site manifest id = %q", site.Manifest.ID)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("status/body = %d %q", rec.Code, rec.Body.String())
	}
}

func writePluginFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}
