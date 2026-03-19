package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"hao.news/internal/apphost"
	"hao.news/internal/plugins/directoryplugin"
	"hao.news/internal/themes/directorytheme"
)

type AppBundle struct {
	Root            string
	App             apphost.AppManifest
	Config          AppConfig
	ThemeManifests  []apphost.ThemeManifest
	PluginManifests []apphost.PluginManifest
	PluginConfigs   map[string]map[string]any
	PluginRoots     map[string]string
	Themes          []directorytheme.Theme
}

type PluginBundle struct {
	Root     string
	Manifest apphost.PluginManifest
	Config   map[string]any
}

type PluginResolver interface {
	ResolvePlugin(id string) (apphost.HTTPPlugin, apphost.PluginManifest, error)
}

func LoadAppBundle(root string) (AppBundle, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return AppBundle{}, fmt.Errorf("app directory is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return AppBundle{}, err
	}
	data, err := os.ReadFile(filepath.Join(root, appManifestName))
	if err != nil {
		return AppBundle{}, err
	}
	app, err := apphost.LoadAppManifestJSON(data)
	if err != nil {
		return AppBundle{}, err
	}
	config, err := LoadAppConfig(root)
	if err != nil {
		return AppBundle{}, err
	}
	themes, themeManifests, err := loadThemes(filepath.Join(root, "themes"))
	if err != nil {
		return AppBundle{}, err
	}
	pluginBundles, err := loadPluginBundles(filepath.Join(root, "plugins"))
	if err != nil {
		return AppBundle{}, err
	}
	pluginManifests := make([]apphost.PluginManifest, 0, len(pluginBundles))
	pluginConfigs := make(map[string]map[string]any, len(pluginBundles))
	pluginRoots := make(map[string]string, len(pluginBundles))
	for _, bundle := range pluginBundles {
		pluginManifests = append(pluginManifests, bundle.Manifest)
		pluginRoots[bundle.Manifest.ID] = bundle.Root
		if len(bundle.Config) > 0 {
			pluginConfigs[bundle.Manifest.ID] = bundle.Config
		}
	}
	return AppBundle{
		Root:            root,
		App:             app,
		Config:          config,
		ThemeManifests:  themeManifests,
		PluginManifests: pluginManifests,
		PluginConfigs:   pluginConfigs,
		PluginRoots:     pluginRoots,
		Themes:          themes,
	}, nil
}

func LoadPluginManifestDir(root string) (apphost.PluginManifest, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return apphost.PluginManifest{}, fmt.Errorf("plugin directory is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return apphost.PluginManifest{}, err
	}
	data, err := os.ReadFile(filepath.Join(root, pluginManifestName))
	if err != nil {
		return apphost.PluginManifest{}, err
	}
	return apphost.LoadPluginManifestJSON(data)
}

func LoadPluginDir(root string, resolver PluginResolver) (apphost.HTTPPlugin, apphost.PluginManifest, error) {
	plugin, err := directoryplugin.Load(root, resolver)
	if err != nil {
		return nil, apphost.PluginManifest{}, err
	}
	return plugin, plugin.Manifest(), nil
}

func LoadPlugins(root string, resolver PluginResolver) ([]apphost.HTTPPlugin, []apphost.PluginManifest, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	plugins := make([]apphost.HTTPPlugin, 0, len(entries))
	manifests := make([]apphost.PluginManifest, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		plugin, manifest, err := LoadPluginDir(filepath.Join(root, entry.Name()), resolver)
		if err != nil {
			return nil, nil, err
		}
		plugins = append(plugins, plugin)
		manifests = append(manifests, manifest)
	}
	sort.SliceStable(plugins, func(i, j int) bool {
		return plugins[i].Manifest().ID < plugins[j].Manifest().ID
	})
	sort.Slice(manifests, func(i, j int) bool {
		return manifests[i].ID < manifests[j].ID
	})
	return plugins, manifests, nil
}

func loadThemes(root string) ([]directorytheme.Theme, []apphost.ThemeManifest, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	themes := make([]directorytheme.Theme, 0, len(entries))
	manifests := make([]apphost.ThemeManifest, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		theme, err := directorytheme.Load(filepath.Join(root, entry.Name()))
		if err != nil {
			return nil, nil, err
		}
		themes = append(themes, theme)
		manifests = append(manifests, theme.Manifest())
	}
	sort.Slice(manifests, func(i, j int) bool {
		return manifests[i].ID < manifests[j].ID
	})
	sort.Slice(themes, func(i, j int) bool {
		return themes[i].Manifest().ID < themes[j].Manifest().ID
	})
	return themes, manifests, nil
}

func LoadPluginBundleDir(root string) (PluginBundle, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return PluginBundle{}, fmt.Errorf("plugin directory is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return PluginBundle{}, err
	}
	manifest, err := LoadPluginManifestDir(root)
	if err != nil {
		return PluginBundle{}, err
	}
	config, err := LoadPluginConfig(root)
	if err != nil {
		return PluginBundle{}, err
	}
	return PluginBundle{
		Root:     root,
		Manifest: manifest,
		Config:   config,
	}, nil
}

func loadPluginBundles(root string) ([]PluginBundle, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	bundles := make([]PluginBundle, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		bundle, err := LoadPluginBundleDir(filepath.Join(root, entry.Name()))
		if err != nil {
			return nil, err
		}
		bundles = append(bundles, bundle)
	}
	sort.Slice(bundles, func(i, j int) bool {
		return bundles[i].Manifest.ID < bundles[j].Manifest.ID
	})
	return bundles, nil
}
