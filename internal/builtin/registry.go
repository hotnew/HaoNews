package builtin

import (
	_ "embed"
	"fmt"
	"strings"

	"hao.news/internal/apphost"
	haonewsarchive "hao.news/internal/plugins/haonewsarchive"
	haonewscontent "hao.news/internal/plugins/haonewscontent"
	haonewsgovernance "hao.news/internal/plugins/haonewsgovernance"
	haonewslive "hao.news/internal/plugins/haonewslive"
	haonewsops "hao.news/internal/plugins/haonewsops"
	"hao.news/internal/themes/haonews"
)

//go:embed hao-news-app.app.json
var publicAppJSON []byte

func DefaultRegistry() *apphost.Registry {
	registry := apphost.NewRegistry()
	registry.MustRegisterTheme(haonews.Theme{})
	registry.MustRegisterPlugin(haonewscontent.Plugin{})
	registry.MustRegisterPlugin(haonewslive.Plugin{})
	registry.MustRegisterPlugin(haonewsarchive.Plugin{})
	registry.MustRegisterPlugin(haonewsgovernance.Plugin{})
	registry.MustRegisterPlugin(haonewsops.Plugin{})
	return registry
}

func DefaultApps() []apphost.AppManifest {
	return []apphost.AppManifest{
		apphost.MustLoadAppManifestJSON(publicAppJSON),
	}
}

func ResolveApp(id string) (apphost.AppManifest, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, app := range DefaultApps() {
		if strings.ToLower(strings.TrimSpace(app.ID)) == id {
			return app, nil
		}
	}
	return apphost.AppManifest{}, fmt.Errorf("app %q not found", id)
}
