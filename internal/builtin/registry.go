package builtin

import (
	_ "embed"
	"fmt"
	"strings"

	"aip2p.org/internal/apphost"
	aip2ppublicarchive "aip2p.org/internal/plugins/aip2ppublicarchive"
	aip2ppubliccontent "aip2p.org/internal/plugins/aip2ppubliccontent"
	aip2ppublicgovernance "aip2p.org/internal/plugins/aip2ppublicgovernance"
	aip2ppublicops "aip2p.org/internal/plugins/aip2ppublicops"
	"aip2p.org/internal/themes/aip2ppublic"
)

//go:embed aip2p-public-app.app.json
var publicAppJSON []byte

func DefaultRegistry() *apphost.Registry {
	registry := apphost.NewRegistry()
	registry.MustRegisterTheme(aip2ppublic.Theme{})
	registry.MustRegisterPlugin(aip2ppubliccontent.Plugin{})
	registry.MustRegisterPlugin(aip2ppublicarchive.Plugin{})
	registry.MustRegisterPlugin(aip2ppublicgovernance.Plugin{})
	registry.MustRegisterPlugin(aip2ppublicops.Plugin{})
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
