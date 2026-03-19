package directorytheme

import (
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"hao.news/internal/apphost"
)

type Theme struct {
	root         string
	templatesDir string
	staticDir    string
	manifest     apphost.ThemeManifest
}

func Load(root string) (Theme, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return Theme{}, fmt.Errorf("theme directory is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return Theme{}, err
	}
	data, err := os.ReadFile(filepath.Join(root, "haonews.theme.json"))
	if err != nil {
		return Theme{}, err
	}
	manifest, err := apphost.LoadThemeManifestJSON(data)
	if err != nil {
		return Theme{}, err
	}
	templatesDir, err := firstExistingDir(root, "templates", filepath.Join("web", "templates"))
	if err != nil {
		return Theme{}, fmt.Errorf("theme %q templates: %w", manifest.ID, err)
	}
	staticDir, err := firstExistingDir(root, "static", filepath.Join("web", "static"))
	if err != nil {
		return Theme{}, fmt.Errorf("theme %q static assets: %w", manifest.ID, err)
	}
	return Theme{
		root:         root,
		templatesDir: templatesDir,
		staticDir:    staticDir,
		manifest:     manifest,
	}, nil
}

func (t Theme) Manifest() apphost.ThemeManifest {
	return t.manifest
}

func (t Theme) ParseTemplates(funcMap template.FuncMap) (*template.Template, error) {
	if funcMap == nil {
		funcMap = template.FuncMap{}
	}
	pattern := filepath.ToSlash(filepath.Join(t.templatesDir, "*.html"))
	return template.New("").Funcs(funcMap).ParseFS(os.DirFS(t.root), pattern)
}

func (t Theme) StaticFS() (fs.FS, error) {
	return fs.Sub(os.DirFS(t.root), filepath.ToSlash(t.staticDir))
}

func firstExistingDir(root string, candidates ...string) (string, error) {
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if candidate == "." || candidate == "" {
			continue
		}
		info, err := os.Stat(filepath.Join(root, candidate))
		if err == nil && info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("expected one of %s", strings.Join(candidates, ", "))
}
