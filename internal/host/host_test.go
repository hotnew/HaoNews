package host

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"hao.news/internal/extensions"
)

func TestNewLoadsAppDirTheme(t *testing.T) {
	root := t.TempDir()
	writeHostFile(t, root, "haonews.app.json", "{\n  \"id\": \"sample-news\",\n  \"name\": \"Sample News\",\n  \"plugins\": [\"sample-content\"],\n  \"theme\": \"sample-theme\"\n}\n")
	writeHostFile(t, root, "haonews.app.config.json", "{\n  \"project\": \"sample.project\",\n  \"runtime_root\": \"runtime-data\",\n  \"store_root\": \"workspace-store\",\n  \"sync_stale_after\": \"33s\"\n}\n")
	writeHostFile(t, root, filepath.Join("plugins", "sample-content", "haonews.plugin.json"), "{\n  \"id\": \"sample-content\",\n  \"name\": \"Sample Content\",\n  \"base_plugin\": \"hao-news-content\",\n  \"default_theme\": \"sample-theme\"\n}\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "haonews.theme.json"), "{\n  \"id\": \"sample-theme\",\n  \"name\": \"Sample Theme\",\n  \"supported_plugins\": [\"sample-content\"],\n  \"required_plugins\": [\"sample-content\"]\n}\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "templates", "home.html"), "home\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "templates", "post.html"), "post\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "templates", "directory.html"), "directory\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "templates", "collection.html"), "collection\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "templates", "network.html"), "network\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "templates", "archive_index.html"), "archive-index\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "templates", "archive_day.html"), "archive-day\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "templates", "archive_message.html"), "archive-message\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "templates", "writer_policy.html"), "writer-policy\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "templates", "partials.html"), "{{/* */}}\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "static", "styles.css"), "body{}\n")

	instance, err := New(context.Background(), Config{
		AppDir: root,
	})
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	if instance.Site().Theme.ID != "sample-theme" {
		t.Fatalf("theme id = %q", instance.Site().Theme.ID)
	}
	if instance.Site().Manifest.ID != "sample-content" {
		t.Fatalf("plugin id = %q", instance.Site().Manifest.ID)
	}
	if instance.config.Project != "sample.project" {
		t.Fatalf("project = %q", instance.config.Project)
	}
	if instance.config.RuntimeRoot != filepath.Join(root, "runtime-data") {
		t.Fatalf("runtime root = %q", instance.config.RuntimeRoot)
	}
	if instance.config.StoreRoot != filepath.Join(root, "workspace-store") {
		t.Fatalf("store root = %q", instance.config.StoreRoot)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	instance.Site().Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestNewDefaultsAppDirRuntimeRoot(t *testing.T) {
	root := t.TempDir()
	writeHostFile(t, root, "haonews.app.json", "{\n  \"id\": \"sample-news\",\n  \"name\": \"Sample News\",\n  \"plugins\": [\"sample-content\"],\n  \"theme\": \"sample-theme\"\n}\n")
	writeHostFile(t, root, filepath.Join("plugins", "sample-content", "haonews.plugin.json"), "{\n  \"id\": \"sample-content\",\n  \"name\": \"Sample Content\",\n  \"base_plugin\": \"hao-news-content\",\n  \"default_theme\": \"sample-theme\"\n}\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "haonews.theme.json"), "{\n  \"id\": \"sample-theme\",\n  \"name\": \"Sample Theme\",\n  \"supported_plugins\": [\"sample-content\"],\n  \"required_plugins\": [\"sample-content\"]\n}\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "templates", "home.html"), "home\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "templates", "post.html"), "post\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "templates", "directory.html"), "directory\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "templates", "collection.html"), "collection\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "templates", "network.html"), "network\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "templates", "archive_index.html"), "archive-index\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "templates", "archive_day.html"), "archive-day\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "templates", "archive_message.html"), "archive-message\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "templates", "writer_policy.html"), "writer-policy\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "templates", "partials.html"), "{{/* */}}\n")
	writeHostFile(t, root, filepath.Join("themes", "sample-theme", "static", "styles.css"), "body{}\n")

	instance, err := New(context.Background(), Config{AppDir: root})
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	if instance.config.RuntimeRoot != filepath.Join(root, "runtime") {
		t.Fatalf("runtime root = %q", instance.config.RuntimeRoot)
	}
}

func TestNewLoadsInstalledAppByID(t *testing.T) {
	store, err := extensions.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open extensions store: %v", err)
	}
	appRoot := t.TempDir()
	writeHostFile(t, appRoot, "haonews.app.json", "{\n  \"id\": \"installed-app\",\n  \"name\": \"Installed App\",\n  \"plugins\": [\"sample-content\"],\n  \"theme\": \"sample-theme\"\n}\n")
	writeHostFile(t, appRoot, filepath.Join("plugins", "sample-content", "haonews.plugin.json"), "{\n  \"id\": \"sample-content\",\n  \"name\": \"Sample Content\",\n  \"base_plugin\": \"hao-news-content\",\n  \"default_theme\": \"sample-theme\"\n}\n")
	writeHostFile(t, appRoot, filepath.Join("themes", "sample-theme", "haonews.theme.json"), "{\n  \"id\": \"sample-theme\",\n  \"name\": \"Sample Theme\",\n  \"supported_plugins\": [\"sample-content\"],\n  \"required_plugins\": [\"sample-content\"]\n}\n")
	writeHostFile(t, appRoot, filepath.Join("themes", "sample-theme", "templates", "home.html"), "home\n")
	writeHostFile(t, appRoot, filepath.Join("themes", "sample-theme", "templates", "post.html"), "post\n")
	writeHostFile(t, appRoot, filepath.Join("themes", "sample-theme", "templates", "directory.html"), "directory\n")
	writeHostFile(t, appRoot, filepath.Join("themes", "sample-theme", "templates", "collection.html"), "collection\n")
	writeHostFile(t, appRoot, filepath.Join("themes", "sample-theme", "templates", "network.html"), "network\n")
	writeHostFile(t, appRoot, filepath.Join("themes", "sample-theme", "templates", "archive_index.html"), "archive-index\n")
	writeHostFile(t, appRoot, filepath.Join("themes", "sample-theme", "templates", "archive_day.html"), "archive-day\n")
	writeHostFile(t, appRoot, filepath.Join("themes", "sample-theme", "templates", "archive_message.html"), "archive-message\n")
	writeHostFile(t, appRoot, filepath.Join("themes", "sample-theme", "templates", "writer_policy.html"), "writer-policy\n")
	writeHostFile(t, appRoot, filepath.Join("themes", "sample-theme", "templates", "partials.html"), "{{/* */}}\n")
	writeHostFile(t, appRoot, filepath.Join("themes", "sample-theme", "static", "styles.css"), "body{}\n")
	if _, err := store.InstallApp(appRoot, false); err != nil {
		t.Fatalf("install app: %v", err)
	}

	instance, err := New(context.Background(), Config{
		App:            "installed-app",
		ExtensionsRoot: store.Paths.Root,
	})
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	if instance.Site().Manifest.ID != "sample-content" {
		t.Fatalf("plugin id = %q", instance.Site().Manifest.ID)
	}
}

func writeHostFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}
