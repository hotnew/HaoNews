package workspace

import (
	"fmt"

	"hao.news/internal/apphost"
)

type ThemeResolver interface {
	ResolveTheme(id string) (apphost.WebTheme, apphost.ThemeManifest, error)
}

type ResolvedPlugin struct {
	RequestedID string                  `json:"requested_id"`
	Root        string                  `json:"root,omitempty"`
	Manifest    apphost.PluginManifest  `json:"manifest"`
	Base        *apphost.PluginManifest `json:"base,omitempty"`
	Config      map[string]any          `json:"config,omitempty"`
}

type ValidationReport struct {
	Root    string                `json:"root"`
	App     apphost.AppManifest   `json:"app"`
	Config  AppConfig             `json:"config"`
	Plugins []ResolvedPlugin      `json:"plugins"`
	Theme   apphost.ThemeManifest `json:"theme"`
	Valid   bool                  `json:"valid"`
}

func ValidatePluginManifest(manifest apphost.PluginManifest, resolver PluginResolver) (ResolvedPlugin, error) {
	report := ResolvedPlugin{
		RequestedID: manifest.ID,
		Manifest:    manifest,
	}
	if resolver == nil {
		return report, nil
	}
	if manifest.BasePlugin == "" {
		return report, nil
	}
	_, baseManifest, err := resolver.ResolvePlugin(manifest.BasePlugin)
	if err != nil {
		return ResolvedPlugin{}, fmt.Errorf("resolve base plugin for %q: %w", manifest.ID, err)
	}
	report.Base = &baseManifest
	return report, nil
}

func ValidateAppBundle(bundle AppBundle, pluginResolver PluginResolver, themeResolver ThemeResolver) (ValidationReport, error) {
	report := ValidationReport{
		Root:   bundle.Root,
		App:    bundle.App,
		Config: bundle.Config,
		Valid:  false,
	}
	plugins := make([]ResolvedPlugin, 0, len(bundle.App.Plugins))
	pluginManifests := make([]apphost.PluginManifest, 0, len(bundle.App.Plugins))
	for _, id := range bundle.App.Plugins {
		plugin, manifest, err := pluginResolver.ResolvePlugin(id)
		if err != nil {
			return ValidationReport{}, fmt.Errorf("resolve app plugin %q: %w", id, err)
		}
		resolved, err := ValidatePluginManifest(manifest, pluginResolver)
		if err != nil {
			return ValidationReport{}, err
		}
		if resolved.RequestedID == "" {
			resolved.RequestedID = id
		}
		if resolved.Manifest.ID == "" {
			resolved.Manifest = plugin.Manifest()
		}
		if root, ok := bundle.PluginRoots[resolved.Manifest.ID]; ok {
			resolved.Root = root
		}
		if cfg, ok := bundle.PluginConfigs[resolved.Manifest.ID]; ok {
			resolved.Config = cfg
		}
		plugins = append(plugins, resolved)
		pluginManifests = append(pluginManifests, resolved.Manifest)
	}
	_, themeManifest, err := themeResolver.ResolveTheme(bundle.App.Theme)
	if err != nil {
		return ValidationReport{}, fmt.Errorf("resolve app theme %q: %w", bundle.App.Theme, err)
	}
	if err := apphost.ValidateSelection(pluginManifests, themeManifest); err != nil {
		return ValidationReport{}, err
	}
	report.Plugins = plugins
	report.Theme = themeManifest
	report.Valid = true
	return report, nil
}
