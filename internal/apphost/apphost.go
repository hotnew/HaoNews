package apphost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type PluginManifest struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Version      string `json:"version,omitempty"`
	Kind         string `json:"plugin_kind,omitempty"`
	Description  string `json:"description"`
	BasePlugin   string `json:"base_plugin,omitempty"`
	DefaultTheme string `json:"default_theme"`
}

type ThemeManifest struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Version          string   `json:"version,omitempty"`
	Description      string   `json:"description"`
	SupportedPlugins []string `json:"supported_plugins,omitempty"`
	RequiredPlugins  []string `json:"required_plugins,omitempty"`
}

type AppManifest struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version,omitempty"`
	Description string   `json:"description"`
	Plugins     []string `json:"plugins"`
	Theme       string   `json:"theme"`
}

type Config struct {
	Plugin           string
	Plugins          []string
	PluginConfig     map[string]any
	PluginConfigs    map[string]map[string]any
	Theme            string
	Project          string
	Version          string
	ListenAddr       string
	RuntimeRoot      string
	StoreRoot        string
	ArchiveRoot      string
	RulesPath        string
	WriterPolicyPath string
	NetPath          string
	TrackerPath      string
	SyncMode         string
	SyncBinaryPath   string
	SyncStaleAfter   time.Duration
	Logf             func(string, ...any)
}

type WebTheme interface {
	Manifest() ThemeManifest
	ParseTemplates(template.FuncMap) (*template.Template, error)
	StaticFS() (fs.FS, error)
}

type HTTPPlugin interface {
	Manifest() PluginManifest
	Build(context.Context, Config, WebTheme) (*Site, error)
}

type Site struct {
	Manifest PluginManifest
	Theme    ThemeManifest
	Handler  http.Handler
	Close    func(context.Context) error
}

func (s *Site) Shutdown(ctx context.Context) error {
	if s == nil || s.Close == nil {
		return nil
	}
	return s.Close(ctx)
}

type Registry struct {
	plugins map[string]HTTPPlugin
	themes  map[string]WebTheme
}

func NewRegistry() *Registry {
	return &Registry{
		plugins: map[string]HTTPPlugin{},
		themes:  map[string]WebTheme{},
	}
}

func (r *Registry) RegisterPlugin(plugin HTTPPlugin) error {
	if plugin == nil {
		return errors.New("plugin is nil")
	}
	manifest := plugin.Manifest()
	id := normalizeID(manifest.ID)
	if id == "" {
		return errors.New("plugin id is required")
	}
	if _, exists := r.plugins[id]; exists {
		return fmt.Errorf("plugin %q already registered", manifest.ID)
	}
	r.plugins[id] = plugin
	return nil
}

func (r *Registry) RegisterTheme(theme WebTheme) error {
	if theme == nil {
		return errors.New("theme is nil")
	}
	manifest := theme.Manifest()
	id := normalizeID(manifest.ID)
	if id == "" {
		return errors.New("theme id is required")
	}
	if _, exists := r.themes[id]; exists {
		return fmt.Errorf("theme %q already registered", manifest.ID)
	}
	r.themes[id] = theme
	return nil
}

func (r *Registry) MustRegisterPlugin(plugin HTTPPlugin) {
	if err := r.RegisterPlugin(plugin); err != nil {
		panic(err)
	}
}

func (r *Registry) MustRegisterTheme(theme WebTheme) {
	if err := r.RegisterTheme(theme); err != nil {
		panic(err)
	}
}

func (r *Registry) PluginIDs() []string {
	return sortedKeys(r.plugins)
}

func (r *Registry) ThemeIDs() []string {
	return sortedKeys(r.themes)
}

func (r *Registry) PluginManifests() []PluginManifest {
	ids := r.PluginIDs()
	out := make([]PluginManifest, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.plugins[id].Manifest())
	}
	return out
}

func (r *Registry) ThemeManifests() []ThemeManifest {
	ids := r.ThemeIDs()
	out := make([]ThemeManifest, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.themes[id].Manifest())
	}
	return out
}

func (r *Registry) Build(ctx context.Context, cfg Config) (*Site, error) {
	if r == nil {
		return nil, errors.New("registry is nil")
	}
	plugins, manifests, err := r.lookupPlugins(cfg)
	if err != nil {
		return nil, err
	}
	themeID := cfg.Theme
	if strings.TrimSpace(themeID) == "" {
		themeID = manifests[0].DefaultTheme
	}
	theme, themeManifest, err := r.lookupTheme(themeID)
	if err != nil {
		return nil, err
	}
	cfg.Plugin = manifests[0].ID
	cfg.Theme = themeManifest.ID
	logf := cfg.Logf
	if logf == nil {
		logf = log.Printf
	}
	for _, manifest := range manifests {
		if err := validateThemeCompatibility(manifest, themeManifest); err != nil {
			return nil, err
		}
	}
	if err := validateThemeRequirements(manifests, themeManifest); err != nil {
		return nil, err
	}
	sites := make([]*Site, 0, len(plugins))
	for idx, plugin := range plugins {
		pluginCfg := cfg
		pluginCfg.Plugin = manifests[idx].ID
		if len(cfg.PluginConfigs) > 0 {
			pluginCfg.PluginConfig = cfg.PluginConfigs[manifests[idx].ID]
		}
		pluginCfg = scopeConfigForPlugin(pluginCfg, manifests[idx])
		site, err := buildPlugin(ctx, plugin, pluginCfg, theme)
		if err != nil {
			return nil, err
		}
		if site == nil {
			return nil, fmt.Errorf("plugin %q returned no site", manifests[idx].ID)
		}
		if site.Handler == nil {
			return nil, fmt.Errorf("plugin %q returned a nil handler", manifests[idx].ID)
		}
		if site.Manifest.ID == "" {
			site.Manifest = manifests[idx]
		}
		if site.Theme.ID == "" {
			site.Theme = themeManifest
		}
		sites = append(sites, site)
	}
	site, err := mergeSites(sites, manifests, themeManifest)
	if err != nil {
		return nil, err
	}
	site.Handler = recoverMiddleware(site.Handler, site.Manifest.ID, logf)
	return site, nil
}

func buildPlugin(ctx context.Context, plugin HTTPPlugin, cfg Config, theme WebTheme) (site *Site, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("plugin %q panicked during startup: %v", plugin.Manifest().ID, rec)
		}
	}()
	return plugin.Build(ctx, cfg, theme)
}

func recoverMiddleware(next http.Handler, pluginID string, logf func(string, ...any)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logf("plugin %s panic on %s %s: %v", pluginID, r.Method, r.URL.Path, rec)
				http.Error(w, "plugin handler panic", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (r *Registry) lookupPlugin(id string) (HTTPPlugin, PluginManifest, error) {
	id = normalizeID(id)
	if id == "" {
		return nil, PluginManifest{}, fmt.Errorf("plugin id is required; available plugins: %s", strings.Join(r.PluginIDs(), ", "))
	}
	plugin, ok := r.plugins[id]
	if !ok {
		return nil, PluginManifest{}, fmt.Errorf("plugin %q not found; available plugins: %s", id, strings.Join(r.PluginIDs(), ", "))
	}
	return plugin, plugin.Manifest(), nil
}

func (r *Registry) ResolvePlugin(id string) (HTTPPlugin, PluginManifest, error) {
	return r.lookupPlugin(id)
}

func (r *Registry) ResolveTheme(id string) (WebTheme, ThemeManifest, error) {
	return r.lookupTheme(id)
}

func (r *Registry) lookupPlugins(cfg Config) ([]HTTPPlugin, []PluginManifest, error) {
	ids := make([]string, 0, len(cfg.Plugins)+1)
	for _, id := range cfg.Plugins {
		id = normalizeID(id)
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		ids = append(ids, normalizeID(cfg.Plugin))
	}
	plugins := make([]HTTPPlugin, 0, len(ids))
	manifests := make([]PluginManifest, 0, len(ids))
	seen := map[string]struct{}{}
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			return nil, nil, fmt.Errorf("plugin %q was requested more than once", id)
		}
		seen[id] = struct{}{}
		plugin, manifest, err := r.lookupPlugin(id)
		if err != nil {
			return nil, nil, err
		}
		plugins = append(plugins, plugin)
		manifests = append(manifests, manifest)
	}
	return plugins, manifests, nil
}

func (r *Registry) lookupTheme(id string) (WebTheme, ThemeManifest, error) {
	id = normalizeID(id)
	if id == "" {
		return nil, ThemeManifest{}, fmt.Errorf("theme id is required; available themes: %s", strings.Join(r.ThemeIDs(), ", "))
	}
	theme, ok := r.themes[id]
	if !ok {
		return nil, ThemeManifest{}, fmt.Errorf("theme %q not found; available themes: %s", id, strings.Join(r.ThemeIDs(), ", "))
	}
	return theme, theme.Manifest(), nil
}

func normalizeID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

func MustLoadPluginManifestJSON(data []byte) PluginManifest {
	manifest, err := LoadPluginManifestJSON(data)
	if err != nil {
		panic(err)
	}
	return manifest
}

func LoadPluginManifestJSON(data []byte) (PluginManifest, error) {
	var manifest PluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return PluginManifest{}, err
	}
	if normalizeID(manifest.ID) == "" {
		return PluginManifest{}, errors.New("plugin manifest id is required")
	}
	if strings.TrimSpace(manifest.Name) == "" {
		return PluginManifest{}, errors.New("plugin manifest name is required")
	}
	return manifest, nil
}

func MustLoadThemeManifestJSON(data []byte) ThemeManifest {
	manifest, err := LoadThemeManifestJSON(data)
	if err != nil {
		panic(err)
	}
	return manifest
}

func MustLoadAppManifestJSON(data []byte) AppManifest {
	manifest, err := LoadAppManifestJSON(data)
	if err != nil {
		panic(err)
	}
	return manifest
}

func LoadAppManifestJSON(data []byte) (AppManifest, error) {
	var manifest AppManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return AppManifest{}, err
	}
	if normalizeID(manifest.ID) == "" {
		return AppManifest{}, errors.New("app manifest id is required")
	}
	if strings.TrimSpace(manifest.Name) == "" {
		return AppManifest{}, errors.New("app manifest name is required")
	}
	if len(manifest.Plugins) == 0 {
		return AppManifest{}, errors.New("app manifest plugins are required")
	}
	if strings.TrimSpace(manifest.Theme) == "" {
		return AppManifest{}, errors.New("app manifest theme is required")
	}
	return manifest, nil
}

func LoadThemeManifestJSON(data []byte) (ThemeManifest, error) {
	var manifest ThemeManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ThemeManifest{}, err
	}
	if normalizeID(manifest.ID) == "" {
		return ThemeManifest{}, errors.New("theme manifest id is required")
	}
	if strings.TrimSpace(manifest.Name) == "" {
		return ThemeManifest{}, errors.New("theme manifest name is required")
	}
	return manifest, nil
}

func validateThemeCompatibility(plugin PluginManifest, theme ThemeManifest) error {
	if len(theme.SupportedPlugins) == 0 {
		return nil
	}
	capabilities := pluginCapabilities(plugin)
	for _, supported := range theme.SupportedPlugins {
		supported = normalizeID(supported)
		for _, capability := range capabilities {
			if supported == capability {
				return nil
			}
		}
	}
	return fmt.Errorf("theme %q does not support plugin %q", theme.ID, plugin.ID)
}

func ValidateSelection(plugins []PluginManifest, theme ThemeManifest) error {
	for _, plugin := range plugins {
		if err := validateThemeCompatibility(plugin, theme); err != nil {
			return err
		}
	}
	return validateThemeRequirements(plugins, theme)
}

func validateThemeRequirements(plugins []PluginManifest, theme ThemeManifest) error {
	if len(theme.RequiredPlugins) == 0 {
		return nil
	}
	selected := make(map[string]struct{}, len(plugins))
	for _, plugin := range plugins {
		for _, capability := range pluginCapabilities(plugin) {
			selected[capability] = struct{}{}
		}
	}
	missing := make([]string, 0, len(theme.RequiredPlugins))
	for _, required := range theme.RequiredPlugins {
		required = normalizeID(required)
		if required == "" {
			continue
		}
		if _, ok := selected[required]; ok {
			continue
		}
		missing = append(missing, required)
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("theme %q requires plugins: %s", theme.ID, strings.Join(missing, ", "))
}

func pluginCapabilities(plugin PluginManifest) []string {
	out := []string{normalizeID(plugin.ID)}
	if base := normalizeID(plugin.BasePlugin); base != "" && base != out[0] {
		out = append(out, base)
	}
	return out
}

func mergeSites(sites []*Site, manifests []PluginManifest, theme ThemeManifest) (*Site, error) {
	if len(sites) == 0 {
		return nil, errors.New("no plugin sites to merge")
	}
	if len(sites) == 1 {
		return sites[0], nil
	}
	names := make([]string, 0, len(manifests))
	ids := make([]string, 0, len(manifests))
	for _, manifest := range manifests {
		names = append(names, manifest.Name)
		ids = append(ids, manifest.ID)
	}
	return &Site{
		Manifest: PluginManifest{
			ID:           strings.Join(ids, "+"),
			Name:         strings.Join(names, " + "),
			Description:  "Composite Hao.News application built from multiple plugins.",
			DefaultTheme: theme.ID,
		},
		Theme:   theme,
		Handler: chainHandlers(sites),
		Close: func(ctx context.Context) error {
			for _, site := range sites {
				if err := site.Shutdown(ctx); err != nil {
					return err
				}
			}
			return nil
		},
	}, nil
}

func chainHandlers(sites []*Site) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, site := range sites {
			rec := newBufferedResponseRecorder()
			site.Handler.ServeHTTP(rec, r)
			if rec.statusCode != http.StatusNotFound {
				rec.writeTo(w)
				return
			}
		}
		http.NotFound(w, r)
	})
}

type bufferedResponseRecorder struct {
	header     http.Header
	body       bytes.Buffer
	statusCode int
}

func newBufferedResponseRecorder() *bufferedResponseRecorder {
	return &bufferedResponseRecorder{
		header:     make(http.Header),
		statusCode: http.StatusOK,
	}
}

func (r *bufferedResponseRecorder) Header() http.Header {
	return r.header
}

func (r *bufferedResponseRecorder) Write(value []byte) (int, error) {
	if r.statusCode == 0 {
		r.statusCode = http.StatusOK
	}
	return r.body.Write(value)
}

func (r *bufferedResponseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
}

func (r *bufferedResponseRecorder) writeTo(w http.ResponseWriter) {
	for key, values := range r.header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	if r.statusCode == 0 {
		r.statusCode = http.StatusOK
	}
	w.WriteHeader(r.statusCode)
	_, _ = w.Write(r.body.Bytes())
}

func sortedKeys[T any](value map[string]T) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func scopeConfigForPlugin(cfg Config, manifest PluginManifest) Config {
	scope := normalizeID(manifest.ID)
	if scope == "" {
		return cfg
	}
	cfg.RuntimeRoot = scopeDir(cfg.RuntimeRoot, scope, true)
	cfg.StoreRoot = scopeDir(cfg.StoreRoot, scope, false)
	cfg.ArchiveRoot = scopeDir(cfg.ArchiveRoot, scope, false)
	cfg.RulesPath = scopeFile(cfg.RulesPath, scope)
	cfg.WriterPolicyPath = scopeFile(cfg.WriterPolicyPath, scope)
	cfg.NetPath = scopeFile(cfg.NetPath, scope)
	cfg.TrackerPath = scopeFile(cfg.TrackerPath, scope)
	return cfg
}

func scopeDir(root, scope string, nestedPlugins bool) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	if nestedPlugins {
		return filepath.Join(root, "plugins", scope)
	}
	return filepath.Join(root, scope)
}

func scopeFile(path, scope string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(path), scope, filepath.Base(path))
}
