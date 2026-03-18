package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"aip2p.org/internal/aip2p"
	"aip2p.org/internal/apphost"
	"aip2p.org/internal/builtin"
	"aip2p.org/internal/extensions"
	"aip2p.org/internal/host"
	"aip2p.org/internal/scaffold"
	"aip2p.org/internal/themes/directorytheme"
	"aip2p.org/internal/workspace"
)

type boolFlag interface {
	IsBoolFlag() bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usageError()
	}
	switch args[0] {
	case "identity":
		return runIdentity(args[1:])
	case "publish":
		return runPublish(args[1:])
	case "verify":
		return runVerify(args[1:])
	case "show":
		return runShow(args[1:])
	case "sync":
		return runSync(args[1:])
	case "serve":
		return runServe(args[1:])
	case "plugins":
		return runPlugins(args[1:])
	case "themes":
		return runThemes(args[1:])
	case "apps":
		return runApps(args[1:])
	case "create":
		return runCreate(args[1:])
	default:
		return usageError()
	}
}

func runPublish(args []string) error {
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	storeRoot := fs.String("store", ".aip2p", "store root")
	author := fs.String("author", "", "agent author id")
	identityFile := fs.String("identity-file", "", "path to a signing identity JSON file")
	kind := fs.String("kind", "post", "message kind")
	channel := fs.String("channel", "", "message channel")
	title := fs.String("title", "", "message title")
	body := fs.String("body", "", "message body")
	replyInfoHash := fs.String("reply-infohash", "", "reply target infohash")
	replyMagnet := fs.String("reply-magnet", "", "reply target magnet")
	tagsCSV := fs.String("tags", "", "comma-separated tags")
	extensionsJSON := fs.String("extensions-json", "", "inline JSON object for message extensions")
	extensionsFile := fs.String("extensions-file", "", "path to JSON object file for message extensions")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*identityFile) == "" {
		return errors.New("identity-file is required; all new posts and replies must be signed")
	}

	store, err := aip2p.OpenStore(*storeRoot)
	if err != nil {
		return err
	}
	identity, err := aip2p.LoadAgentIdentity(strings.TrimSpace(*identityFile))
	if err != nil {
		return err
	}
	if strings.TrimSpace(*author) == "" && strings.TrimSpace(identity.Author) != "" {
		*author = strings.TrimSpace(identity.Author)
	}
	if strings.TrimSpace(*author) == "" {
		return errors.New("author is required; set --author or store author in identity-file")
	}
	if strings.TrimSpace(identity.Author) != "" && strings.TrimSpace(*author) != strings.TrimSpace(identity.Author) {
		return errors.New("author does not match identity-file author")
	}

	var replyTo *aip2p.MessageLink
	if strings.TrimSpace(*replyInfoHash) != "" || strings.TrimSpace(*replyMagnet) != "" {
		replyTo = &aip2p.MessageLink{
			InfoHash: strings.TrimSpace(*replyInfoHash),
			Magnet:   strings.TrimSpace(*replyMagnet),
		}
	}
	extensions, err := loadJSONObject(*extensionsJSON, *extensionsFile)
	if err != nil {
		return err
	}

	result, err := aip2p.PublishMessage(store, aip2p.MessageInput{
		Kind:       *kind,
		Author:     *author,
		Channel:    *channel,
		Title:      *title,
		Body:       *body,
		ReplyTo:    replyTo,
		Tags:       splitCSV(*tagsCSV),
		Identity:   &identity,
		Extensions: extensions,
		CreatedAt:  time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	return writeJSON(result)
}

func runIdentity(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: aip2p identity init [flags]")
	}
	switch args[0] {
	case "init":
		return runIdentityInit(args[1:])
	default:
		return errors.New("usage: aip2p identity init [flags]")
	}
}

func runIdentityInit(args []string) error {
	fs := flag.NewFlagSet("identity init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	agentID := fs.String("agent-id", "", "stable agent id")
	author := fs.String("author", "", "default author for this identity")
	out := fs.String("out", "", "identity file output path; defaults to ~/.aip2p-public/identities/<sanitized-agent-id>.json")
	force := fs.Bool("force", false, "overwrite output file if it exists")
	if err := fs.Parse(args); err != nil {
		return err
	}
	outputPath, err := defaultIdentityOutputPath(*agentID, *out)
	if err != nil {
		return err
	}
	if !*force {
		if _, err := os.Stat(outputPath); err == nil {
			return fmt.Errorf("identity file already exists: %s", outputPath)
		}
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	identity, err := aip2p.NewAgentIdentity(*agentID, *author, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := aip2p.SaveAgentIdentity(outputPath, identity); err != nil {
		return err
	}
	return writeJSON(map[string]any{
		"agent_id":   identity.AgentID,
		"author":     identity.Author,
		"key_type":   identity.KeyType,
		"public_key": identity.PublicKey,
		"created_at": identity.CreatedAt,
		"file":       outputPath,
	})
}

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dir := fs.String("dir", "", "content directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*dir) == "" {
		return errors.New("dir is required")
	}
	msg, body, err := aip2p.LoadMessage(*dir)
	if err != nil {
		return err
	}
	return writeJSON(struct {
		Valid   bool          `json:"valid"`
		Message aip2p.Message `json:"message"`
		BodyLen int           `json:"body_len"`
	}{
		Valid:   true,
		Message: msg,
		BodyLen: len(body),
	})
}

func runShow(args []string) error {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dir := fs.String("dir", "", "content directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*dir) == "" {
		return errors.New("dir is required")
	}
	msg, body, err := aip2p.LoadMessage(*dir)
	if err != nil {
		return err
	}
	return writeJSON(struct {
		Message aip2p.Message `json:"message"`
		Body    string        `json:"body"`
	}{
		Message: msg,
		Body:    body,
	})
}

func runSync(args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	storeRoot := fs.String("store", ".aip2p", "store root")
	queuePath := fs.String("queue", "", "line-based magnet/infohash queue file")
	netPath := fs.String("net", "./aip2p_net.inf", "network bootstrap config")
	trackersPath := fs.String("trackers", "", "tracker list file; defaults to Trackerlist.inf next to the net config")
	subscriptionsPath := fs.String("subscriptions", "", "subscription rules file for pubsub topic joins")
	listenAddr := fs.String("listen", "0.0.0.0:0", "bittorrent listen address")
	magnets := fs.String("magnet", "", "comma-separated magnets or infohashes to sync immediately")
	poll := fs.Duration("poll", 30*time.Second, "queue polling interval")
	timeout := fs.Duration("timeout", 20*time.Second, "per-ref sync timeout")
	once := fs.Bool("once", false, "run one sync pass and exit")
	seed := fs.Bool("seed", true, "seed after download while daemon is running")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return aip2p.RunSync(ctx, aip2p.SyncOptions{
		StoreRoot:         *storeRoot,
		QueuePath:         *queuePath,
		NetPath:           *netPath,
		TrackerListPath:   *trackersPath,
		SubscriptionsPath: *subscriptionsPath,
		ListenAddr:        *listenAddr,
		Refs:              splitCSV(*magnets),
		PollInterval:      *poll,
		Timeout:           *timeout,
		Once:              *once,
		Seed:              *seed,
	}, log.Printf)
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	listenAddr := fs.String("listen", "0.0.0.0:51818", "http listen address")
	appID := fs.String("app", "", "built-in application id; defaults to the built-in sample app")
	appDir := fs.String("app-dir", "", "application directory containing aip2p.app.json and optional themes/plugins folders")
	extensionsRoot := fs.String("extensions-root", "", "installed extensions root; defaults to ~/.aip2p/extensions")
	pluginID := fs.String("plugin", "", "single built-in plugin id; ignored when --plugins is set")
	pluginsCSV := fs.String("plugins", "", "comma-separated built-in plugin ids to compose; overrides --plugin")
	pluginDirsCSV := fs.String("plugin-dir", "", "comma-separated external plugin directories containing aip2p.plugin.json")
	themeID := fs.String("theme", "", "theme id; defaults to the plugin default theme")
	themeDir := fs.String("theme-dir", "", "directory theme override; expects aip2p.theme.json plus templates/static")
	project := fs.String("project", "", "project id override")
	version := fs.String("version", "dev", "host version label")
	runtimeRoot := fs.String("runtime-root", "", "application runtime root")
	storeRoot := fs.String("store", "", "store root override")
	archiveRoot := fs.String("archive", "", "archive root override")
	rulesPath := fs.String("subscriptions", "", "subscription rules path override")
	writerPolicy := fs.String("writer-policy", "", "writer policy path override")
	netPath := fs.String("net", "", "network bootstrap config override")
	trackersPath := fs.String("trackers", "", "tracker list override")
	syncMode := fs.String("sync-mode", "", "sync mode override")
	syncBinary := fs.String("sync-binary", "", "managed sync binary override")
	syncStaleAfter := fs.Duration("sync-stale-after", 2*time.Minute, "managed sync stale restart threshold")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	instance, err := host.New(ctx, host.Config{
		App:              *appID,
		AppDir:           *appDir,
		ExtensionsRoot:   *extensionsRoot,
		Plugin:           *pluginID,
		Plugins:          splitCSV(*pluginsCSV),
		PluginDirs:       splitCSV(*pluginDirsCSV),
		Theme:            *themeID,
		ThemeDir:         *themeDir,
		Project:          *project,
		Version:          *version,
		ListenAddr:       *listenAddr,
		RuntimeRoot:      *runtimeRoot,
		StoreRoot:        *storeRoot,
		ArchiveRoot:      *archiveRoot,
		RulesPath:        *rulesPath,
		WriterPolicyPath: *writerPolicy,
		NetPath:          *netPath,
		TrackerPath:      *trackersPath,
		SyncMode:         *syncMode,
		SyncBinaryPath:   *syncBinary,
		SyncStaleAfter:   *syncStaleAfter,
		Logf:             log.Printf,
	})
	if err != nil {
		return err
	}
	log.Printf("AiP2P host serving plugin=%s theme=%s on http://%s", instance.Site().Manifest.ID, instance.Site().Theme.ID, instance.ListenAddr())
	return instance.ListenAndServe(ctx)
}

func runPlugins(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: aip2p plugins <list|inspect|install|link|remove>")
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("plugins list", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		registry := builtin.DefaultRegistry()
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		installed, err := store.ListPlugins()
		if err != nil {
			return err
		}
		plugins := make([]any, 0, len(registry.PluginManifests())+len(installed))
		for _, manifest := range registry.PluginManifests() {
			plugins = append(plugins, map[string]any{
				"source":   "builtin",
				"manifest": manifest,
			})
		}
		for _, entry := range installed {
			plugins = append(plugins, map[string]any{
				"source":   "installed",
				"root":     entry.Root,
				"manifest": entry.Manifest,
				"config":   entry.Config,
				"metadata": entry.Metadata,
			})
		}
		return writeJSON(struct {
			Plugins []any `json:"plugins"`
		}{
			Plugins: plugins,
		})
	case "inspect":
		fs := flag.NewFlagSet("plugins inspect", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		dir := fs.String("dir", "", "plugin directory containing aip2p.plugin.json")
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		registry := builtin.DefaultRegistry()
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		if _, err := store.RegisterIntoRegistry(registry, "", "", ""); err != nil {
			return err
		}
		if strings.TrimSpace(*dir) != "" {
			bundle, err := workspace.LoadPluginBundleDir(*dir)
			if err != nil {
				return err
			}
			_, manifest, err := workspace.LoadPluginDir(*dir, registry)
			if err != nil {
				return err
			}
			resolved, err := workspace.ValidatePluginManifest(manifest, registry)
			if err != nil {
				return err
			}
			resolved.Root = bundle.Root
			resolved.Config = bundle.Config
			return writeJSON(struct {
				Dir      string                   `json:"dir"`
				Manifest apphost.PluginManifest   `json:"manifest"`
				Config   map[string]any           `json:"config,omitempty"`
				Resolved workspace.ResolvedPlugin `json:"resolved"`
			}{
				Dir:      *dir,
				Manifest: manifest,
				Config:   bundle.Config,
				Resolved: resolved,
			})
		}
		if fs.NArg() == 0 {
			return errors.New("plugin id or --dir is required")
		}
		id := fs.Arg(0)
		if entry, err := store.GetPlugin(id); err == nil {
			resolved, err := workspace.ValidatePluginManifest(entry.Manifest, registry)
			if err != nil {
				return err
			}
			resolved.Root = entry.Root
			resolved.Config = entry.Config
			return writeJSON(struct {
				Source   string                     `json:"source"`
				Root     string                     `json:"root"`
				Manifest apphost.PluginManifest     `json:"manifest"`
				Config   map[string]any             `json:"config,omitempty"`
				Metadata extensions.InstallMetadata `json:"metadata"`
				Resolved workspace.ResolvedPlugin   `json:"resolved"`
			}{
				Source:   "installed",
				Root:     entry.Root,
				Manifest: entry.Manifest,
				Config:   entry.Config,
				Metadata: entry.Metadata,
				Resolved: resolved,
			})
		}
		_, manifest, err := registry.ResolvePlugin(id)
		if err != nil {
			return err
		}
		return writeJSON(struct {
			Source   string                 `json:"source"`
			Manifest apphost.PluginManifest `json:"manifest"`
		}{
			Source:   "builtin",
			Manifest: manifest,
		})
	case "install", "link":
		fs := flag.NewFlagSet("plugins "+args[0], flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		dir := fs.String("dir", "", "plugin directory containing aip2p.plugin.json")
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		entry, err := store.InstallPlugin(*dir, args[0] == "link")
		if err != nil {
			return err
		}
		return writeJSON(entry)
	case "remove":
		fs := flag.NewFlagSet("plugins remove", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			return errors.New("plugin id is required")
		}
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		if err := store.RemovePlugin(fs.Arg(0)); err != nil {
			return err
		}
		return writeJSON(map[string]any{"removed": fs.Arg(0)})
	default:
		return errors.New("usage: aip2p plugins <list|inspect|install|link|remove>")
	}
}

func runApps(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: aip2p apps <list|inspect|validate|install|link|remove>")
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("apps list", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		installed, err := store.ListApps()
		if err != nil {
			return err
		}
		apps := make([]any, 0, len(builtin.DefaultApps())+len(installed))
		for _, app := range builtin.DefaultApps() {
			apps = append(apps, map[string]any{
				"source": "builtin",
				"app":    app,
			})
		}
		for _, entry := range installed {
			apps = append(apps, map[string]any{
				"source":   "installed",
				"root":     entry.Root,
				"app":      entry.Manifest,
				"config":   entry.Config,
				"metadata": entry.Metadata,
			})
		}
		return writeJSON(struct {
			Apps []any `json:"apps"`
		}{
			Apps: apps,
		})
	case "inspect":
		fs := flag.NewFlagSet("apps inspect", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		dir := fs.String("dir", "", "application directory containing aip2p.app.json")
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*dir) != "" {
			bundle, report, err := inspectAppDir(*dir, *root)
			if err != nil {
				return err
			}
			return writeJSON(struct {
				Dir        string                     `json:"dir"`
				App        apphost.AppManifest        `json:"app"`
				Config     workspace.AppConfig        `json:"config"`
				Plugins    []apphost.PluginManifest   `json:"plugins"`
				Themes     []apphost.ThemeManifest    `json:"themes"`
				Validation workspace.ValidationReport `json:"validation"`
			}{
				Dir:        *dir,
				App:        bundle.App,
				Config:     bundle.Config,
				Plugins:    bundle.PluginManifests,
				Themes:     bundle.ThemeManifests,
				Validation: report,
			})
		}
		if fs.NArg() == 0 {
			return errors.New("app id or --dir is required")
		}
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		entry, err := store.GetApp(fs.Arg(0))
		if err != nil {
			return err
		}
		bundle, report, err := inspectAppDir(entry.Root, *root)
		if err != nil {
			return err
		}
		return writeJSON(struct {
			Source     string                     `json:"source"`
			Root       string                     `json:"root"`
			Metadata   extensions.InstallMetadata `json:"metadata"`
			App        apphost.AppManifest        `json:"app"`
			Config     workspace.AppConfig        `json:"config"`
			Plugins    []apphost.PluginManifest   `json:"plugins"`
			Themes     []apphost.ThemeManifest    `json:"themes"`
			Validation workspace.ValidationReport `json:"validation"`
		}{
			Source:     "installed",
			Root:       entry.Root,
			Metadata:   entry.Metadata,
			App:        bundle.App,
			Config:     bundle.Config,
			Plugins:    bundle.PluginManifests,
			Themes:     bundle.ThemeManifests,
			Validation: report,
		})
	case "validate":
		fs := flag.NewFlagSet("apps validate", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		dir := fs.String("dir", "", "application directory containing aip2p.app.json")
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		target := strings.TrimSpace(*dir)
		if target == "" {
			if fs.NArg() == 0 {
				return errors.New("app id or --dir is required")
			}
			store, err := extensions.Open(*root)
			if err != nil {
				return err
			}
			entry, err := store.GetApp(fs.Arg(0))
			if err != nil {
				return err
			}
			target = entry.Root
		}
		_, report, err := inspectAppDir(target, *root)
		if err != nil {
			return err
		}
		return writeJSON(report)
	case "install", "link":
		fs := flag.NewFlagSet("apps "+args[0], flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		dir := fs.String("dir", "", "application directory containing aip2p.app.json")
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		entry, err := store.InstallApp(*dir, args[0] == "link")
		if err != nil {
			return err
		}
		return writeJSON(entry)
	case "remove":
		fs := flag.NewFlagSet("apps remove", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			return errors.New("app id is required")
		}
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		if err := store.RemoveApp(fs.Arg(0)); err != nil {
			return err
		}
		return writeJSON(map[string]any{"removed": fs.Arg(0)})
	default:
		return errors.New("usage: aip2p apps <list|inspect|validate|install|link|remove>")
	}
}

func runCreate(args []string) error {
	if len(args) < 2 {
		return errors.New("usage: aip2p create <plugin|theme|app> <name> [--out dir]")
	}
	kind := strings.TrimSpace(args[0])
	target := strings.TrimSpace(args[1])
	if target == "" {
		return errors.New("name is required")
	}
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	outDir := fs.String("out", "", "output directory")
	if err := fs.Parse(args[2:]); err != nil {
		return err
	}
	name, resolvedOut, err := resolveCreateTarget(target, *outDir)
	if err != nil {
		return err
	}

	var (
		files []scaffold.File
	)
	switch kind {
	case "plugin":
		files, err = scaffold.PluginFiles(name)
	case "theme":
		files, err = scaffold.ThemeFiles(name)
	case "app":
		files, err = scaffold.AppFiles(name)
	default:
		return errors.New("usage: aip2p create <plugin|theme|app> <name> [--out dir]")
	}
	if err != nil {
		return err
	}
	if err := scaffold.WriteFiles(resolvedOut, files); err != nil {
		return err
	}
	return writeJSON(map[string]any{
		"kind":   kind,
		"name":   name,
		"output": resolvedOut,
		"files":  filePaths(files),
	})
}

func resolveCreateTarget(target, explicitOut string) (string, string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", "", errors.New("name is required")
	}
	explicitOut = strings.TrimSpace(explicitOut)
	if explicitOut != "" {
		return targetBaseName(target), explicitOut, nil
	}
	if looksLikePath(target) {
		return targetBaseName(target), target, nil
	}
	return target, scaffold.Slug(target), nil
}

func looksLikePath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if filepath.IsAbs(value) {
		return true
	}
	switch value {
	case ".", "..":
		return true
	}
	return strings.Contains(value, "/") || strings.Contains(value, `\`)
}

func targetBaseName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	base := filepath.Base(filepath.Clean(value))
	base = strings.TrimSpace(base)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return value
	}
	return base
}

func defaultIdentityOutputPath(agentID, explicitOut string) (string, error) {
	explicitOut = strings.TrimSpace(explicitOut)
	if explicitOut != "" {
		return explicitOut, nil
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return "", errors.New("agent-id is required")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return "", errors.New("user home directory is empty")
	}
	return filepath.Join(home, ".aip2p-public", "identities", sanitizeAgentIDForFilename(agentID)+".json"), nil
}

func sanitizeAgentIDForFilename(agentID string) string {
	agentID = strings.ToLower(strings.TrimSpace(agentID))
	if agentID == "" {
		return "identity"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range agentID {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlnum {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	value := strings.Trim(b.String(), "-")
	if value == "" {
		return "identity"
	}
	return value
}

func parseFlagSetInterspersed(fs *flag.FlagSet, args []string) error {
	reordered := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}
		reordered = append(reordered, arg)
		if strings.Contains(arg, "=") {
			continue
		}
		name := strings.TrimLeft(arg, "-")
		if name == "" {
			continue
		}
		info := fs.Lookup(name)
		if info == nil {
			continue
		}
		if bf, ok := info.Value.(boolFlag); ok && bf.IsBoolFlag() {
			continue
		}
		if i+1 < len(args) {
			i++
			reordered = append(reordered, args[i])
		}
	}
	reordered = append(reordered, positionals...)
	return fs.Parse(reordered)
}

func filePaths(files []scaffold.File) []string {
	out := make([]string, 0, len(files))
	for _, file := range files {
		out = append(out, file.Path)
	}
	return out
}

func runThemes(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: aip2p themes <list|inspect|install|link|remove>")
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("themes list", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		registry := builtin.DefaultRegistry()
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		installed, err := store.ListThemes()
		if err != nil {
			return err
		}
		themes := make([]any, 0, len(registry.ThemeManifests())+len(installed))
		for _, manifest := range registry.ThemeManifests() {
			themes = append(themes, map[string]any{
				"source":   "builtin",
				"manifest": manifest,
			})
		}
		for _, entry := range installed {
			themes = append(themes, map[string]any{
				"source":   "installed",
				"root":     entry.Root,
				"manifest": entry.Manifest,
				"metadata": entry.Metadata,
			})
		}
		return writeJSON(struct {
			Themes []any `json:"themes"`
		}{
			Themes: themes,
		})
	case "inspect":
		fs := flag.NewFlagSet("themes inspect", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		dir := fs.String("dir", "", "theme directory containing aip2p.theme.json")
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*dir) != "" {
			theme, err := directorytheme.Load(*dir)
			if err != nil {
				return err
			}
			return writeJSON(struct {
				Dir      string                `json:"dir"`
				Manifest apphost.ThemeManifest `json:"manifest"`
			}{
				Dir:      *dir,
				Manifest: theme.Manifest(),
			})
		}
		if fs.NArg() == 0 {
			return errors.New("theme id or --dir is required")
		}
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		if entry, err := store.GetTheme(fs.Arg(0)); err == nil {
			return writeJSON(struct {
				Source   string                     `json:"source"`
				Root     string                     `json:"root"`
				Manifest apphost.ThemeManifest      `json:"manifest"`
				Metadata extensions.InstallMetadata `json:"metadata"`
			}{
				Source:   "installed",
				Root:     entry.Root,
				Manifest: entry.Manifest,
				Metadata: entry.Metadata,
			})
		}
		registry := builtin.DefaultRegistry()
		_, manifest, err := registry.ResolveTheme(fs.Arg(0))
		if err != nil {
			return err
		}
		return writeJSON(struct {
			Source   string                `json:"source"`
			Manifest apphost.ThemeManifest `json:"manifest"`
		}{
			Source:   "builtin",
			Manifest: manifest,
		})
	case "install", "link":
		fs := flag.NewFlagSet("themes "+args[0], flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		dir := fs.String("dir", "", "theme directory containing aip2p.theme.json")
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		entry, err := store.InstallTheme(*dir, args[0] == "link")
		if err != nil {
			return err
		}
		return writeJSON(entry)
	case "remove":
		fs := flag.NewFlagSet("themes remove", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			return errors.New("theme id is required")
		}
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		if err := store.RemoveTheme(fs.Arg(0)); err != nil {
			return err
		}
		return writeJSON(map[string]any{"removed": fs.Arg(0)})
	default:
		return errors.New("usage: aip2p themes <list|inspect|install|link|remove>")
	}
}

func manifestsToAny[T any](items []T) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	return out
}

func inspectAppDir(dir, extensionsRoot string) (workspace.AppBundle, workspace.ValidationReport, error) {
	bundle, err := workspace.LoadAppBundle(dir)
	if err != nil {
		return workspace.AppBundle{}, workspace.ValidationReport{}, err
	}
	registry := builtin.DefaultRegistry()
	store, err := extensions.Open(extensionsRoot)
	if err != nil {
		return workspace.AppBundle{}, workspace.ValidationReport{}, err
	}
	if _, err := store.RegisterIntoRegistry(registry, "", "", bundle.App.ID); err != nil {
		return workspace.AppBundle{}, workspace.ValidationReport{}, err
	}
	plugins, _, err := workspace.LoadPlugins(filepath.Join(bundle.Root, "plugins"), registry)
	if err != nil {
		return workspace.AppBundle{}, workspace.ValidationReport{}, err
	}
	for _, plugin := range plugins {
		if err := registry.RegisterPlugin(plugin); err != nil {
			return workspace.AppBundle{}, workspace.ValidationReport{}, err
		}
	}
	for _, theme := range bundle.Themes {
		if err := registry.RegisterTheme(theme); err != nil {
			return workspace.AppBundle{}, workspace.ValidationReport{}, err
		}
	}
	report, err := workspace.ValidateAppBundle(bundle, registry, registry)
	if err != nil {
		return workspace.AppBundle{}, workspace.ValidationReport{}, err
	}
	return bundle, report, nil
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func usageError() error {
	return errors.New("usage: aip2p <identity|publish|verify|show|sync|serve|plugins|themes|apps|create> [flags]")
}

func loadJSONObject(inline, path string) (map[string]any, error) {
	inline = strings.TrimSpace(inline)
	path = strings.TrimSpace(path)
	if inline != "" && path != "" {
		return nil, errors.New("use only one of extensions-json or extensions-file")
	}
	if inline == "" && path == "" {
		return map[string]any{}, nil
	}
	var data []byte
	var err error
	if inline != "" {
		data = []byte(inline)
	} else {
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, err
		}
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("parse extensions json: %w", err)
	}
	if value == nil {
		value = map[string]any{}
	}
	return value, nil
}
