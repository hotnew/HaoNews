package directoryplugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hao.news/internal/apphost"
)

type baseResolver interface {
	ResolvePlugin(id string) (apphost.HTTPPlugin, apphost.PluginManifest, error)
}

type Plugin struct {
	root     string
	manifest apphost.PluginManifest
	base     apphost.HTTPPlugin
}

func Load(root string, resolver baseResolver) (Plugin, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return Plugin{}, fmt.Errorf("plugin directory is required")
	}
	if resolver == nil {
		return Plugin{}, fmt.Errorf("plugin resolver is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return Plugin{}, err
	}
	data, err := os.ReadFile(filepath.Join(root, "aip2p.plugin.json"))
	if err != nil {
		return Plugin{}, err
	}
	manifest, err := apphost.LoadPluginManifestJSON(data)
	if err != nil {
		return Plugin{}, err
	}
	baseID := strings.TrimSpace(manifest.BasePlugin)
	if baseID == "" {
		return Plugin{}, fmt.Errorf("plugin %q is missing base_plugin", manifest.ID)
	}
	base, _, err := resolver.ResolvePlugin(baseID)
	if err != nil {
		return Plugin{}, fmt.Errorf("plugin %q base_plugin %q: %w", manifest.ID, baseID, err)
	}
	return Plugin{
		root:     root,
		manifest: manifest,
		base:     base,
	}, nil
}

func (p Plugin) Manifest() apphost.PluginManifest {
	return p.manifest
}

func (p Plugin) Build(ctx context.Context, cfg apphost.Config, theme apphost.WebTheme) (*apphost.Site, error) {
	site, err := p.base.Build(ctx, cfg, theme)
	if err != nil {
		return nil, err
	}
	if site == nil {
		return nil, nil
	}
	cloned := *site
	cloned.Manifest = p.manifest
	if cloned.Theme.ID == "" && theme != nil {
		cloned.Theme = theme.Manifest()
	}
	return &cloned, nil
}
