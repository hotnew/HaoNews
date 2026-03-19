package haonewsgovernance

import (
	"context"
	_ "embed"

	"hao.news/internal/apphost"
	newsplugin "hao.news/internal/plugins/haonews"
)

type Plugin struct{}

//go:embed aip2p.plugin.json
var pluginManifestJSON []byte

func (Plugin) Manifest() apphost.PluginManifest {
	return apphost.MustLoadPluginManifestJSON(pluginManifestJSON)
}

func (Plugin) Build(_ context.Context, cfg apphost.Config, theme apphost.WebTheme) (*apphost.Site, error) {
	cfg = newsplugin.ApplyDefaultConfig(cfg)
	app, err := newsplugin.NewWithThemeAndOptions(
		cfg.StoreRoot,
		cfg.Project,
		cfg.Version,
		cfg.ArchiveRoot,
		cfg.RulesPath,
		cfg.WriterPolicyPath,
		cfg.NetPath,
		theme,
		newsplugin.OptionsForPlugins(newsplugin.GovernanceOnlyAppOptions(), cfg),
	)
	if err != nil {
		return nil, err
	}
	staticFS, err := theme.StaticFS()
	if err != nil {
		return nil, err
	}
	return &apphost.Site{
		Manifest: Plugin{}.Manifest(),
		Theme:    theme.Manifest(),
		Handler:  newHandler(app, staticFS),
	}, nil
}
