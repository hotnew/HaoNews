package newsplugin

import (
	"testing"

	"hao.news/internal/apphost"
)

func TestOptionsForPluginsExpandsNavigationForCombinedApp(t *testing.T) {
	t.Parallel()

	options := OptionsForPlugins(ArchiveOnlyAppOptions(), apphost.Config{
		Plugin: archivePluginID,
		Plugins: []string{
			contentPluginID,
			livePluginID,
			governancePluginID,
			archivePluginID,
			opsPluginID,
		},
	})
	if !options.ContentRoutes || !options.ContentAPIRoutes {
		t.Fatal("expected content routes in combined app options")
	}
	if !options.LiveRoutes {
		t.Fatal("expected live routes in combined app options")
	}
	if !options.ArchiveRoutes || !options.HistoryAPIRoutes {
		t.Fatal("expected archive routes in combined app options")
	}
	if !options.WriterPolicyRoutes {
		t.Fatal("expected governance routes in combined app options")
	}
	if !options.NetworkRoutes || !options.NetworkAPIRoutes {
		t.Fatal("expected ops routes in combined app options")
	}
}

func TestPageNavMatchesInstalledPluginSet(t *testing.T) {
	t.Parallel()

	archiveOnly := App{options: OptionsForPlugins(ArchiveOnlyAppOptions(), apphost.Config{Plugin: archivePluginID})}
	archiveNav := archiveOnly.pageNav("/archive")
	if hasNavItem(archiveNav, "首页") {
		t.Fatal("archive-only nav unexpectedly exposed feed link")
	}
	if !hasNavItem(archiveNav, "归档") {
		t.Fatal("archive-only nav should expose archive link")
	}

	combined := App{options: OptionsForPlugins(ArchiveOnlyAppOptions(), apphost.Config{
		Plugin: archivePluginID,
		Plugins: []string{
			contentPluginID,
			livePluginID,
			governancePluginID,
			archivePluginID,
			opsPluginID,
		},
	})}
	combinedNav := combined.pageNav("/archive")
	for _, name := range []string{"首页", "来源", "话题", "Live", "网络", "策略", "归档"} {
		if !hasNavItem(combinedNav, name) {
			t.Fatalf("combined nav missing %q", name)
		}
	}
}

func hasNavItem(items []NavItem, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}
