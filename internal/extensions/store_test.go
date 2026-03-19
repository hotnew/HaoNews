package extensions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreInstallListRemovePlugin(t *testing.T) {
	store := newTestStore(t)
	src := t.TempDir()
	writeExtFile(t, src, "haonews.plugin.json", "{\n  \"id\": \"sample-plugin\",\n  \"name\": \"Sample Plugin\",\n  \"base_plugin\": \"hao-news-content\",\n  \"default_theme\": \"hao-news-theme\"\n}\n")
	writeExtFile(t, src, "haonews.plugin.config.json", "{\n  \"channel\": \"sample-world\"\n}\n")

	entry, err := store.InstallPlugin(src, false)
	if err != nil {
		t.Fatalf("install plugin: %v", err)
	}
	if entry.Manifest.ID != "sample-plugin" {
		t.Fatalf("plugin id = %q", entry.Manifest.ID)
	}

	plugins, err := store.ListPlugins()
	if err != nil {
		t.Fatalf("list plugins: %v", err)
	}
	if len(plugins) != 1 || plugins[0].Manifest.ID != "sample-plugin" {
		t.Fatalf("plugins = %#v", plugins)
	}

	if err := store.RemovePlugin("sample-plugin"); err != nil {
		t.Fatalf("remove plugin: %v", err)
	}
	plugins, err = store.ListPlugins()
	if err != nil {
		t.Fatalf("list plugins after remove: %v", err)
	}
	if len(plugins) != 0 {
		t.Fatalf("plugins after remove = %#v", plugins)
	}
}

func TestStoreLinkTheme(t *testing.T) {
	store := newTestStore(t)
	src := t.TempDir()
	writeThemeScaffold(t, src, "sample-theme", "Sample Theme")

	entry, err := store.InstallTheme(src, true)
	if err != nil {
		t.Fatalf("link theme: %v", err)
	}
	if !entry.Metadata.Linked {
		t.Fatalf("linked = false")
	}
	if info, err := os.Lstat(filepath.Join(store.Paths.ThemesDir, "sample-theme")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("theme link missing: %v", err)
	}
}

func TestStoreInstallApp(t *testing.T) {
	store := newTestStore(t)
	src := t.TempDir()
	writeExtFile(t, src, "haonews.app.json", "{\n  \"id\": \"sample-app\",\n  \"name\": \"Sample App\",\n  \"plugins\": [\"sample-plugin\"],\n  \"theme\": \"sample-theme\"\n}\n")
	writeExtFile(t, src, "haonews.app.config.json", "{\n  \"project\": \"sample.project\"\n}\n")
	writeExtFile(t, src, filepath.Join("plugins", "sample-plugin", "haonews.plugin.json"), "{\n  \"id\": \"sample-plugin\",\n  \"name\": \"Sample Plugin\",\n  \"base_plugin\": \"hao-news-content\",\n  \"default_theme\": \"sample-theme\"\n}\n")
	writeThemeScaffold(t, filepath.Join(src, "themes", "sample-theme"), "sample-theme", "Sample Theme")

	entry, err := store.InstallApp(src, false)
	if err != nil {
		t.Fatalf("install app: %v", err)
	}
	if entry.Manifest.ID != "sample-app" {
		t.Fatalf("app id = %q", entry.Manifest.ID)
	}

	apps, err := store.ListApps()
	if err != nil {
		t.Fatalf("list apps: %v", err)
	}
	if len(apps) != 1 || apps[0].Manifest.ID != "sample-app" {
		t.Fatalf("apps = %#v", apps)
	}
}

func newTestStore(t *testing.T) Store {
	t.Helper()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := store.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	return store
}

func writeThemeScaffold(t *testing.T, root, id, name string) {
	t.Helper()
	writeExtFile(t, root, "haonews.theme.json", "{\n  \"id\": \""+id+"\",\n  \"name\": \""+name+"\",\n  \"supported_plugins\": [\"sample-plugin\"],\n  \"required_plugins\": [\"sample-plugin\"]\n}\n")
	writeExtFile(t, root, filepath.Join("templates", "home.html"), "home\n")
	writeExtFile(t, root, filepath.Join("templates", "post.html"), "post\n")
	writeExtFile(t, root, filepath.Join("templates", "directory.html"), "directory\n")
	writeExtFile(t, root, filepath.Join("templates", "collection.html"), "collection\n")
	writeExtFile(t, root, filepath.Join("templates", "network.html"), "network\n")
	writeExtFile(t, root, filepath.Join("templates", "archive_index.html"), "archive-index\n")
	writeExtFile(t, root, filepath.Join("templates", "archive_day.html"), "archive-day\n")
	writeExtFile(t, root, filepath.Join("templates", "archive_message.html"), "archive-message\n")
	writeExtFile(t, root, filepath.Join("templates", "writer_policy.html"), "writer-policy\n")
	writeExtFile(t, root, filepath.Join("templates", "partials.html"), "{{/* */}}\n")
	writeExtFile(t, root, filepath.Join("static", "styles.css"), "body{}\n")
}

func writeExtFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}
