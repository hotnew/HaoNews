package scaffold

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var unsafeName = regexp.MustCompile(`[^a-z0-9._-]+`)

type File struct {
	Path    string
	Content string
}

func Slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = unsafeName.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-.")
	if value == "" {
		return "sample"
	}
	return value
}

func PluginFiles(name string) ([]File, error) {
	id := Slug(name)
	channel := id + "/general"
	manifest, err := marshalIndented(map[string]any{
		"id":            id,
		"name":          strings.TrimSpace(name),
		"version":       "0.1.0",
		"plugin_kind":   "content",
		"description":   "Describe what this plugin does.",
		"base_plugin":   "hao-news-content",
		"default_theme": "hao-news-theme",
	})
	if err != nil {
		return nil, err
	}
	return []File{
		{Path: "aip2p.plugin.json", Content: manifest},
		{Path: "aip2p.plugin.config.json", Content: fmt.Sprintf("{\n  \"channel\": %q\n}\n", channel)},
		{Path: "README.md", Content: fmt.Sprintf("# %s\n\nThis is an AiP2P plugin scaffold.\n\nThe generated manifest is immediately runnable as a third-party directory plugin by delegating to the built-in `hao-news-content` runtime through `base_plugin`.\n\nTry it with:\n\n`aip2p serve --plugin-dir . --theme hao-news-theme`\n", strings.TrimSpace(name))},
		{Path: "config.schema.json", Content: "{\n  \"$schema\": \"https://json-schema.org/draft/2020-12/schema\",\n  \"type\": \"object\",\n  \"properties\": {}\n}\n"},
		{Path: "skills/README.md", Content: "# Skills\n\nPut plugin-specific skills here.\n"},
		{Path: "src/README.md", Content: "# Runtime\n\nThis scaffold uses `base_plugin` for runtime delegation today.\n\nKeep app-specific config, schemas, skills, and future runtime code here.\n"},
	}, nil
}

func ThemeFiles(name string) ([]File, error) {
	id := Slug(name)
	manifest, err := marshalIndented(map[string]any{
		"id":                id,
		"name":              strings.TrimSpace(name),
		"version":           "0.1.0",
		"description":       "Describe this theme and the page models it targets.",
		"supported_plugins": []string{},
		"required_plugins":  []string{},
	})
	if err != nil {
		return nil, err
	}
	return []File{
		{Path: "aip2p.theme.json", Content: manifest},
		{Path: "README.md", Content: fmt.Sprintf("# %s\n\nThis is an AiP2P theme scaffold.\n\nUpdate `aip2p.theme.json`, then edit the templates in `templates/` and assets in `static/`.\n\nThe generated files are intentionally minimal but runnable with `aip2p serve --theme-dir ./...`.\n", strings.TrimSpace(name))},
		{Path: "templates/home.html", Content: defaultThemeTemplate(name, "Home")},
		{Path: "templates/post.html", Content: defaultThemeTemplate(name, "Post")},
		{Path: "templates/directory.html", Content: defaultThemeTemplate(name, "Directory")},
		{Path: "templates/collection.html", Content: defaultThemeTemplate(name, "Collection")},
		{Path: "templates/network.html", Content: defaultThemeTemplate(name, "Network")},
		{Path: "templates/archive_index.html", Content: defaultThemeTemplate(name, "Archive Index")},
		{Path: "templates/archive_day.html", Content: defaultThemeTemplate(name, "Archive Day")},
		{Path: "templates/archive_message.html", Content: defaultThemeTemplate(name, "Archive Message")},
		{Path: "templates/writer_policy.html", Content: defaultThemeTemplate(name, "Writer Policy")},
		{Path: "templates/partials.html", Content: "{{/* add shared template blocks here */}}\n"},
		{Path: "static/styles.css", Content: defaultThemeStyles()},
	}, nil
}

func AppFiles(name string) ([]File, error) {
	id := Slug(name)
	basePluginID := "hao-news-content"
	pluginID := id + "-plugin"
	themeID := id + "-theme"
	projectID := id + ".sample"
	channel := id + "/general"
	manifest, err := marshalIndented(map[string]any{
		"id":          id,
		"name":        strings.TrimSpace(name),
		"version":     "0.1.0",
		"description": "Describe this AiP2P sample app. By default it composes built-in sample plugins with a local theme pack.",
		"plugins":     []string{pluginID},
		"theme":       themeID,
	})
	if err != nil {
		return nil, err
	}
	return []File{
		{Path: "aip2p.app.json", Content: manifest},
		{Path: "aip2p.app.config.json", Content: fmt.Sprintf("{\n  \"project\": %q,\n  \"runtime_root\": \"runtime\",\n  \"store_root\": \"runtime/store\",\n  \"archive_root\": \"runtime/archive\"\n}\n", projectID)},
		{Path: "README.md", Content: fmt.Sprintf("# %s\n\nThis is an AiP2P app scaffold.\n\nIt uses a local third-party plugin pack in `plugins/%s-plugin/` and a local theme pack in `themes/%s/`.\n\nThe generated plugin delegates to the built-in `hao-news-content` runtime, so the scaffold runs immediately.\n\nRun it with:\n\n`aip2p serve --app-dir .`\n", strings.TrimSpace(name), id, themeID)},
		{Path: filepath.Join("plugins", pluginID, "aip2p.plugin.json"), Content: fmt.Sprintf("{\n  \"id\": %q,\n  \"name\": %q,\n  \"version\": \"0.1.0\",\n  \"plugin_kind\": \"content\",\n  \"description\": \"Example third-party plugin pack that delegates to the built-in hao-news-content runtime.\",\n  \"base_plugin\": %q,\n  \"default_theme\": %q\n}\n", pluginID, strings.TrimSpace(name)+" Plugin", basePluginID, themeID)},
		{Path: filepath.Join("plugins", pluginID, "aip2p.plugin.config.json"), Content: fmt.Sprintf("{\n  \"channel\": %q\n}\n", channel)},
		{Path: filepath.Join("plugins", pluginID, "README.md"), Content: "# Plugin Sample\n\nThis plugin is immediately loadable through `aip2p serve --app-dir .` because it delegates to a built-in runtime using `base_plugin`.\n"},
		{Path: filepath.Join("themes", themeID, "aip2p.theme.json"), Content: fmt.Sprintf("{\n  \"id\": %q,\n  \"name\": %q,\n  \"version\": \"0.1.0\",\n  \"description\": \"Describe this app theme.\",\n  \"supported_plugins\": [%q],\n  \"required_plugins\": [%q]\n}\n", themeID, strings.TrimSpace(name)+" Theme", pluginID, pluginID)},
		{Path: filepath.Join("themes", themeID, "templates", "home.html"), Content: defaultThemeTemplate(name, "Home")},
		{Path: filepath.Join("themes", themeID, "templates", "post.html"), Content: defaultThemeTemplate(name, "Post")},
		{Path: filepath.Join("themes", themeID, "templates", "directory.html"), Content: defaultThemeTemplate(name, "Directory")},
		{Path: filepath.Join("themes", themeID, "templates", "collection.html"), Content: defaultThemeTemplate(name, "Collection")},
		{Path: filepath.Join("themes", themeID, "templates", "network.html"), Content: defaultThemeTemplate(name, "Network")},
		{Path: filepath.Join("themes", themeID, "templates", "archive_index.html"), Content: defaultThemeTemplate(name, "Archive Index")},
		{Path: filepath.Join("themes", themeID, "templates", "archive_day.html"), Content: defaultThemeTemplate(name, "Archive Day")},
		{Path: filepath.Join("themes", themeID, "templates", "archive_message.html"), Content: defaultThemeTemplate(name, "Archive Message")},
		{Path: filepath.Join("themes", themeID, "templates", "writer_policy.html"), Content: defaultThemeTemplate(name, "Writer Policy")},
		{Path: filepath.Join("themes", themeID, "templates", "partials.html"), Content: "{{/* add shared template blocks here */}}\n"},
		{Path: filepath.Join("themes", themeID, "static", "styles.css"), Content: defaultThemeStyles()},
	}, nil
}

func WriteFiles(root string, files []File) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return fmt.Errorf("output directory is required")
	}
	for _, file := range files {
		path := filepath.Join(root, file.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(file.Content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func marshalIndented(value any) (string, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(append(data, '\n')), nil
}

func defaultThemeTemplate(name, page string) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s - %s</title>
  <link rel="stylesheet" href="/static/styles.css">
</head>
<body>
  <main class="page">
    <p class="eyebrow">AiP2P Theme Scaffold</p>
    <h1>%s</h1>
    <p class="lede">Replace this placeholder template with a real layout for the %s page.</p>
  </main>
</body>
</html>
`, strings.TrimSpace(name), page, page, strings.TrimSpace(name))
}

func defaultThemeStyles() string {
	return `:root {
  color-scheme: light;
  --bg: #f6f1e8;
  --panel: #fffdf8;
  --ink: #241f1a;
  --muted: #6c5f52;
  --accent: #b54f2d;
  --border: #d8c9b7;
}

* {
  box-sizing: border-box;
}

body {
  margin: 0;
  background: linear-gradient(180deg, #efe3d0 0%, var(--bg) 45%);
  color: var(--ink);
  font-family: Georgia, "Times New Roman", serif;
}

.page {
  max-width: 760px;
  margin: 8vh auto;
  padding: 32px;
  background: var(--panel);
  border: 1px solid var(--border);
  border-radius: 24px;
  box-shadow: 0 20px 60px rgba(36, 31, 26, 0.08);
}

.eyebrow {
  margin: 0 0 12px;
  color: var(--accent);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

h1 {
  margin: 0 0 16px;
  font-size: clamp(2rem, 5vw, 3.5rem);
}

.lede {
  margin: 0;
  color: var(--muted);
  font-size: 1.05rem;
  line-height: 1.6;
}
`
}
