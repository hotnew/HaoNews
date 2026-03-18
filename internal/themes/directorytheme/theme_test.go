package directorytheme

import (
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadThemeFromDirectory(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "aip2p.theme.json", "{\n  \"id\": \"custom-theme\",\n  \"name\": \"Custom Theme\",\n  \"supported_plugins\": [\"aip2p-public-content\"],\n  \"required_plugins\": [\"aip2p-public-content\"]\n}\n")
	writeTestFile(t, root, "templates/home.html", "<!doctype html><html><body>home</body></html>\n")
	writeTestFile(t, root, "static/styles.css", "body { color: black; }\n")

	theme, err := Load(root)
	if err != nil {
		t.Fatalf("load theme: %v", err)
	}
	if theme.Manifest().ID != "custom-theme" {
		t.Fatalf("manifest id = %q, want custom-theme", theme.Manifest().ID)
	}
	tmpl, err := theme.ParseTemplates(template.FuncMap{})
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	if tmpl.Lookup("home.html") == nil {
		t.Fatalf("home.html template not loaded")
	}
	staticFS, err := theme.StaticFS()
	if err != nil {
		t.Fatalf("static fs: %v", err)
	}
	data, err := fs.ReadFile(staticFS, "styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	if string(data) != "body { color: black; }\n" {
		t.Fatalf("styles.css = %q", string(data))
	}
}

func writeTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}
