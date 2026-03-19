package extensions

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"hao.news/internal/apphost"
	"hao.news/internal/builtin"
	"hao.news/internal/themes/directorytheme"
	"hao.news/internal/workspace"
)

const (
	metaSuffix     = ".aip2p.install.json"
	defaultEnvRoot = "AIP2P_EXTENSIONS_ROOT"
	kindPlugin     = "plugin"
	kindTheme      = "theme"
	kindApp        = "app"
)

type Paths struct {
	Root       string
	PluginsDir string
	ThemesDir  string
	AppsDir    string
}

type InstallMetadata struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Version     string `json:"version,omitempty"`
	Source      string `json:"source,omitempty"`
	Linked      bool   `json:"linked"`
	InstalledAt string `json:"installed_at"`
}

type PluginEntry struct {
	Root     string                 `json:"root"`
	Manifest apphost.PluginManifest `json:"manifest"`
	Config   map[string]any         `json:"config,omitempty"`
	Metadata InstallMetadata        `json:"metadata"`
}

type ThemeEntry struct {
	Root     string                `json:"root"`
	Manifest apphost.ThemeManifest `json:"manifest"`
	Metadata InstallMetadata       `json:"metadata"`
}

type AppEntry struct {
	Root     string              `json:"root"`
	Manifest apphost.AppManifest `json:"manifest"`
	Config   workspace.AppConfig `json:"config"`
	Metadata InstallMetadata     `json:"metadata"`
}

type Store struct {
	Paths Paths
}

func Open(root string) (Store, error) {
	paths, err := resolvePaths(root)
	if err != nil {
		return Store{}, err
	}
	return Store{Paths: paths}, nil
}

func resolvePaths(root string) (Paths, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = strings.TrimSpace(os.Getenv(defaultEnvRoot))
	}
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Paths{}, err
		}
		root = filepath.Join(home, ".aip2p", "extensions")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return Paths{}, err
	}
	return Paths{
		Root:       root,
		PluginsDir: filepath.Join(root, "plugins"),
		ThemesDir:  filepath.Join(root, "themes"),
		AppsDir:    filepath.Join(root, "apps"),
	}, nil
}

func (s Store) EnsureDirs() error {
	for _, dir := range []string{s.Paths.Root, s.Paths.PluginsDir, s.Paths.ThemesDir, s.Paths.AppsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (s Store) InstallPlugin(src string, link bool) (PluginEntry, error) {
	if err := s.EnsureDirs(); err != nil {
		return PluginEntry{}, err
	}
	bundle, err := workspace.LoadPluginBundleDir(src)
	if err != nil {
		return PluginEntry{}, err
	}
	registry := builtin.DefaultRegistry()
	if _, err := s.RegisterIntoRegistry(registry, bundle.Manifest.ID, "", ""); err != nil {
		return PluginEntry{}, err
	}
	if _, _, err := workspace.LoadPluginDir(src, registry); err != nil {
		return PluginEntry{}, err
	}
	if _, _, err := registry.ResolvePlugin(bundle.Manifest.ID); err == nil {
		return PluginEntry{}, fmt.Errorf("plugin %q conflicts with a built-in plugin", bundle.Manifest.ID)
	}
	dest := filepath.Join(s.Paths.PluginsDir, bundle.Manifest.ID)
	if err := replaceInstalledPath(src, dest, link); err != nil {
		return PluginEntry{}, err
	}
	meta := newMetadata(kindPlugin, bundle.Manifest.ID, bundle.Manifest.Version, src, link)
	if err := writeMetadata(s.pluginMetaPath(bundle.Manifest.ID), meta); err != nil {
		return PluginEntry{}, err
	}
	return s.GetPlugin(bundle.Manifest.ID)
}

func (s Store) InstallTheme(src string, link bool) (ThemeEntry, error) {
	if err := s.EnsureDirs(); err != nil {
		return ThemeEntry{}, err
	}
	theme, err := directorytheme.Load(src)
	if err != nil {
		return ThemeEntry{}, err
	}
	registry := builtin.DefaultRegistry()
	if _, err := s.RegisterIntoRegistry(registry, "", theme.Manifest().ID, ""); err != nil {
		return ThemeEntry{}, err
	}
	if _, _, err := registry.ResolveTheme(theme.Manifest().ID); err == nil {
		return ThemeEntry{}, fmt.Errorf("theme %q conflicts with a built-in theme", theme.Manifest().ID)
	}
	dest := filepath.Join(s.Paths.ThemesDir, theme.Manifest().ID)
	if err := replaceInstalledPath(src, dest, link); err != nil {
		return ThemeEntry{}, err
	}
	meta := newMetadata(kindTheme, theme.Manifest().ID, theme.Manifest().Version, src, link)
	if err := writeMetadata(s.themeMetaPath(theme.Manifest().ID), meta); err != nil {
		return ThemeEntry{}, err
	}
	return s.GetTheme(theme.Manifest().ID)
}

func (s Store) InstallApp(src string, link bool) (AppEntry, error) {
	if err := s.EnsureDirs(); err != nil {
		return AppEntry{}, err
	}
	bundle, err := workspace.LoadAppBundle(src)
	if err != nil {
		return AppEntry{}, err
	}
	if _, err := builtin.ResolveApp(bundle.App.ID); err == nil {
		return AppEntry{}, fmt.Errorf("app %q conflicts with a built-in app", bundle.App.ID)
	}
	registry := builtin.DefaultRegistry()
	if _, err := s.RegisterIntoRegistry(registry, "", "", bundle.App.ID); err != nil {
		return AppEntry{}, err
	}
	plugins, _, err := workspace.LoadPlugins(filepath.Join(bundle.Root, "plugins"), registry)
	if err != nil {
		return AppEntry{}, err
	}
	for _, plugin := range plugins {
		if err := registry.RegisterPlugin(plugin); err != nil {
			return AppEntry{}, err
		}
	}
	for _, theme := range bundle.Themes {
		if err := registry.RegisterTheme(theme); err != nil {
			return AppEntry{}, err
		}
	}
	if _, err := workspace.ValidateAppBundle(bundle, registry, registry); err != nil {
		return AppEntry{}, err
	}
	dest := filepath.Join(s.Paths.AppsDir, bundle.App.ID)
	if err := replaceInstalledPath(src, dest, link); err != nil {
		return AppEntry{}, err
	}
	meta := newMetadata(kindApp, bundle.App.ID, bundle.App.Version, src, link)
	if err := writeMetadata(s.appMetaPath(bundle.App.ID), meta); err != nil {
		return AppEntry{}, err
	}
	return s.GetApp(bundle.App.ID)
}

func (s Store) RemovePlugin(id string) error {
	return s.removeInstalled(filepath.Join(s.Paths.PluginsDir, strings.TrimSpace(id)), s.pluginMetaPath(id))
}

func (s Store) RemoveTheme(id string) error {
	return s.removeInstalled(filepath.Join(s.Paths.ThemesDir, strings.TrimSpace(id)), s.themeMetaPath(id))
}

func (s Store) RemoveApp(id string) error {
	return s.removeInstalled(filepath.Join(s.Paths.AppsDir, strings.TrimSpace(id)), s.appMetaPath(id))
}

func (s Store) GetPlugin(id string) (PluginEntry, error) {
	root := filepath.Join(s.Paths.PluginsDir, strings.TrimSpace(id))
	bundle, err := workspace.LoadPluginBundleDir(root)
	if err != nil {
		return PluginEntry{}, err
	}
	return PluginEntry{
		Root:     root,
		Manifest: bundle.Manifest,
		Config:   bundle.Config,
		Metadata: s.readMetadataOrInfer(kindPlugin, root, bundle.Manifest.ID, bundle.Manifest.Version),
	}, nil
}

func (s Store) GetTheme(id string) (ThemeEntry, error) {
	root := filepath.Join(s.Paths.ThemesDir, strings.TrimSpace(id))
	theme, err := directorytheme.Load(root)
	if err != nil {
		return ThemeEntry{}, err
	}
	return ThemeEntry{
		Root:     root,
		Manifest: theme.Manifest(),
		Metadata: s.readMetadataOrInfer(kindTheme, root, theme.Manifest().ID, theme.Manifest().Version),
	}, nil
}

func (s Store) GetApp(id string) (AppEntry, error) {
	root := filepath.Join(s.Paths.AppsDir, strings.TrimSpace(id))
	bundle, err := workspace.LoadAppBundle(root)
	if err != nil {
		return AppEntry{}, err
	}
	return AppEntry{
		Root:     root,
		Manifest: bundle.App,
		Config:   bundle.Config,
		Metadata: s.readMetadataOrInfer(kindApp, root, bundle.App.ID, bundle.App.Version),
	}, nil
}

func (s Store) ListPlugins() ([]PluginEntry, error) {
	names, err := installedNames(s.Paths.PluginsDir)
	if err != nil {
		return nil, err
	}
	items := make([]PluginEntry, 0, len(names))
	for _, name := range names {
		entry, err := s.GetPlugin(name)
		if err != nil {
			return nil, err
		}
		items = append(items, entry)
	}
	return items, nil
}

func (s Store) ListThemes() ([]ThemeEntry, error) {
	names, err := installedNames(s.Paths.ThemesDir)
	if err != nil {
		return nil, err
	}
	items := make([]ThemeEntry, 0, len(names))
	for _, name := range names {
		entry, err := s.GetTheme(name)
		if err != nil {
			return nil, err
		}
		items = append(items, entry)
	}
	return items, nil
}

func (s Store) ListApps() ([]AppEntry, error) {
	names, err := installedNames(s.Paths.AppsDir)
	if err != nil {
		return nil, err
	}
	items := make([]AppEntry, 0, len(names))
	for _, name := range names {
		entry, err := s.GetApp(name)
		if err != nil {
			return nil, err
		}
		items = append(items, entry)
	}
	return items, nil
}

func (s Store) RegisterIntoRegistry(registry *apphost.Registry, skipPluginID, skipThemeID, skipAppID string) (map[string]AppEntry, error) {
	themes, err := s.ListThemes()
	if err != nil {
		return nil, err
	}
	for _, entry := range themes {
		if strings.EqualFold(entry.Manifest.ID, strings.TrimSpace(skipThemeID)) {
			continue
		}
		theme, err := directorytheme.Load(entry.Root)
		if err != nil {
			return nil, err
		}
		if err := registry.RegisterTheme(theme); err != nil && !strings.Contains(err.Error(), "already registered") {
			return nil, err
		}
	}
	plugins, err := s.ListPlugins()
	if err != nil {
		return nil, err
	}
	pending := make(map[string]PluginEntry, len(plugins))
	for _, entry := range plugins {
		if strings.EqualFold(entry.Manifest.ID, strings.TrimSpace(skipPluginID)) {
			continue
		}
		pending[entry.Manifest.ID] = entry
	}
	for len(pending) > 0 {
		progress := false
		ids := make([]string, 0, len(pending))
		for id := range pending {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			entry := pending[id]
			if base := strings.TrimSpace(entry.Manifest.BasePlugin); base != "" {
				if _, _, err := registry.ResolvePlugin(base); err != nil {
					continue
				}
			}
			plugin, _, err := workspace.LoadPluginDir(entry.Root, registry)
			if err != nil {
				return nil, err
			}
			if err := registry.RegisterPlugin(plugin); err != nil && !strings.Contains(err.Error(), "already registered") {
				return nil, err
			}
			delete(pending, id)
			progress = true
		}
		if !progress {
			return nil, fmt.Errorf("unable to resolve installed plugin dependencies")
		}
	}
	apps, err := s.ListApps()
	if err != nil {
		return nil, err
	}
	out := make(map[string]AppEntry, len(apps))
	for _, entry := range apps {
		if strings.EqualFold(entry.Manifest.ID, strings.TrimSpace(skipAppID)) {
			continue
		}
		out[strings.ToLower(strings.TrimSpace(entry.Manifest.ID))] = entry
	}
	return out, nil
}

func (s Store) pluginMetaPath(id string) string {
	return filepath.Join(s.Paths.PluginsDir, strings.TrimSpace(id)+metaSuffix)
}

func (s Store) themeMetaPath(id string) string {
	return filepath.Join(s.Paths.ThemesDir, strings.TrimSpace(id)+metaSuffix)
}

func (s Store) appMetaPath(id string) string {
	return filepath.Join(s.Paths.AppsDir, strings.TrimSpace(id)+metaSuffix)
}

func (s Store) readMetadataOrInfer(kind, root, id, version string) InstallMetadata {
	var meta InstallMetadata
	switch kind {
	case kindPlugin:
		meta = readMetadata(s.pluginMetaPath(id))
	case kindTheme:
		meta = readMetadata(s.themeMetaPath(id))
	case kindApp:
		meta = readMetadata(s.appMetaPath(id))
	}
	if meta.ID == "" {
		meta = newMetadata(kind, id, version, root, false)
	}
	if info, err := os.Lstat(root); err == nil && info.Mode()&os.ModeSymlink != 0 {
		meta.Linked = true
		if target, err := filepath.EvalSymlinks(root); err == nil {
			meta.Source = target
		}
	}
	return meta
}

func (s Store) removeInstalled(root, metaPath string) error {
	if err := os.RemoveAll(root); err != nil {
		return err
	}
	if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func installedNames(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, metaSuffix) {
			continue
		}
		path := filepath.Join(root, name)
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func replaceInstalledPath(src, dest string, link bool) error {
	src = strings.TrimSpace(src)
	dest = strings.TrimSpace(dest)
	if src == "" || dest == "" {
		return fmt.Errorf("source and destination are required")
	}
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	if link {
		return os.Symlink(src, dest)
	}
	return copyTree(src, dest)
}

func copyTree(src, dest string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dest string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

func newMetadata(kind, id, version, source string, linked bool) InstallMetadata {
	return InstallMetadata{
		Kind:        kind,
		ID:          strings.TrimSpace(id),
		Version:     strings.TrimSpace(version),
		Source:      strings.TrimSpace(source),
		Linked:      linked,
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
	}
}

func writeMetadata(path string, meta InstallMetadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func readMetadata(path string) InstallMetadata {
	data, err := os.ReadFile(path)
	if err != nil {
		return InstallMetadata{}
	}
	var meta InstallMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return InstallMetadata{}
	}
	return meta
}
