package scaffold

import (
	"strings"
	"testing"
)

func TestThemeFilesAreRunnableScaffold(t *testing.T) {
	files, err := ThemeFiles("Sample Theme")
	if err != nil {
		t.Fatalf("theme files: %v", err)
	}
	paths := make(map[string]string, len(files))
	for _, file := range files {
		paths[file.Path] = file.Content
	}
	required := []string{
		"aip2p.theme.json",
		"templates/home.html",
		"templates/post.html",
		"templates/directory.html",
		"templates/collection.html",
		"templates/network.html",
		"templates/archive_index.html",
		"templates/archive_day.html",
		"templates/archive_message.html",
		"templates/writer_policy.html",
		"templates/partials.html",
		"static/styles.css",
	}
	for _, path := range required {
		if _, ok := paths[path]; !ok {
			t.Fatalf("missing scaffold file %q", path)
		}
	}
	if got := paths["aip2p.theme.json"]; got == "" || !strings.Contains(got, `"required_plugins": []`) {
		t.Fatalf("theme manifest missing required_plugins: %q", got)
	}
}

func TestPluginFilesIncludeBasePlugin(t *testing.T) {
	files, err := PluginFiles("Sample Plugin")
	if err != nil {
		t.Fatalf("plugin files: %v", err)
	}
	paths := make(map[string]string, len(files))
	for _, file := range files {
		paths[file.Path] = file.Content
	}
	if got := paths["aip2p.plugin.json"]; got == "" || !strings.Contains(got, `"base_plugin": "hao-news-content"`) {
		t.Fatalf("plugin manifest missing base_plugin: %q", got)
	}
	if _, ok := paths["aip2p.plugin.config.json"]; !ok {
		t.Fatalf("missing plugin config scaffold")
	}
	if got := paths["aip2p.plugin.config.json"]; !strings.Contains(got, `"sample-plugin/general"`) {
		t.Fatalf("plugin config does not include generated channel: %q", got)
	}
}

func TestAppFilesUseLocalPluginPack(t *testing.T) {
	files, err := AppFiles("Sample App")
	if err != nil {
		t.Fatalf("app files: %v", err)
	}
	paths := make(map[string]string, len(files))
	for _, file := range files {
		paths[file.Path] = file.Content
	}
	if got := paths["aip2p.app.json"]; got == "" || !strings.Contains(got, `"plugins": [`) || !strings.Contains(got, `"sample-app-plugin"`) {
		t.Fatalf("app manifest does not target local plugin: %q", got)
	}
	if _, ok := paths["aip2p.app.config.json"]; !ok {
		t.Fatalf("missing app config scaffold")
	}
	if got := paths["aip2p.app.config.json"]; !strings.Contains(got, `"sample-app.sample"`) {
		t.Fatalf("app config does not include generated project id: %q", got)
	}
	if got := paths["plugins/sample-app-plugin/aip2p.plugin.json"]; got == "" || !strings.Contains(got, `"base_plugin": "hao-news-content"`) {
		t.Fatalf("app plugin manifest missing base_plugin: %q", got)
	}
	if _, ok := paths["plugins/sample-app-plugin/aip2p.plugin.config.json"]; !ok {
		t.Fatalf("missing app plugin config scaffold")
	}
	if got := paths["plugins/sample-app-plugin/aip2p.plugin.config.json"]; !strings.Contains(got, `"sample-app/general"`) {
		t.Fatalf("app plugin config does not include generated channel: %q", got)
	}
	if got := paths["themes/sample-app-theme/aip2p.theme.json"]; got == "" || !strings.Contains(got, `"sample-app-plugin"`) {
		t.Fatalf("theme manifest does not depend on local plugin: %q", got)
	}
}
