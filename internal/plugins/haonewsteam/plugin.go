package haonewsteam

import (
	"context"
	_ "embed"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"hao.news/internal/apphost"
	teamcore "hao.news/internal/haonews/team"
	newsplugin "hao.news/internal/plugins/haonews"
)

type Plugin struct{}

//go:embed haonews.plugin.json
var pluginManifestJSON []byte

func (Plugin) Manifest() apphost.PluginManifest {
	return apphost.MustLoadPluginManifestJSON(pluginManifestJSON)
}

func (Plugin) Build(_ context.Context, cfg apphost.Config, theme apphost.WebTheme) (*apphost.Site, error) {
	ctx := context.Background()
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
		newsplugin.OptionsForPlugins(newsplugin.TeamOnlyAppOptions(), cfg),
	)
	if err != nil {
		return nil, err
	}
	store, err := teamcore.OpenStore(cfg.StoreRoot)
	if err != nil {
		return nil, err
	}
	staticFS, err := theme.StaticFS()
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(filepathBase(os.Args[0]), ".test") {
		startTeamWorkspaceWarmup(ctx, app, store)
	}
	return &apphost.Site{
		Manifest: Plugin{}.Manifest(),
		Theme:    theme.Manifest(),
		Handler:  newHandler(app, store, staticFS),
	}, nil
}

func startTeamWorkspaceWarmup(ctx context.Context, app *newsplugin.App, store *teamcore.Store) {
	const warmupInterval = 45 * time.Second
	go func() {
		ticker := time.NewTicker(warmupInterval)
		defer ticker.Stop()
		for {
			warmTeamWorkspace(app, store)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func warmTeamWorkspace(app *newsplugin.App, store *teamcore.Store) {
	if app == nil || store == nil {
		return
	}
	index, err := app.Index()
	if err == nil {
		_ = app.NodeStatus(index)
	}
	teams, err := store.ListTeams()
	if err != nil {
		return
	}
	const warmTeamLimit = 8
	for i, summary := range teams {
		if i >= warmTeamLimit {
			break
		}
		teamID := strings.TrimSpace(summary.TeamID)
		if teamID == "" {
			continue
		}
		_, _ = store.LoadTeam(teamID)
		_, _ = store.LoadMembers(teamID)
		_, _ = store.LoadPolicy(teamID)
		_, _ = store.LoadMessages(teamID, "main", 20)
		_, _ = store.LoadTasks(teamID, 20)
		_, _ = store.LoadArtifacts(teamID, 20)
		_, _ = store.LoadHistory(teamID, 20)
		_, _ = store.ListChannels(teamID)
		_, _ = store.ListArchives(teamID)
	}
}

func filepathBase(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

func newHandler(app *newsplugin.App, store *teamcore.Store, staticFS fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/agent.json", func(w http.ResponseWriter, r *http.Request) {
		handleA2AWellKnownAgent(app, store, w, r)
	})
	mux.HandleFunc("/a2a/teams/", func(w http.ResponseWriter, r *http.Request) {
		trimmed := strings.Trim(strings.TrimPrefix(r.URL.Path, "/a2a/teams/"), "/")
		if trimmed == "" {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(trimmed, "/")
		teamID := teamcore.NormalizeTeamID(parts[0])
		if teamID == "" || len(parts) < 2 {
			http.NotFound(w, r)
			return
		}
		handleA2ATeam(app, store, teamID, w, r)
	})
	mux.HandleFunc("/archive/team", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/archive/team" {
			http.NotFound(w, r)
			return
		}
		handleTeamArchiveIndex(app, store, w, r)
	})
	mux.HandleFunc("/archive/team/", func(w http.ResponseWriter, r *http.Request) {
		trimmed := strings.Trim(strings.TrimPrefix(r.URL.Path, "/archive/team/"), "/")
		if trimmed == "" {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(trimmed, "/")
		teamID := teamcore.NormalizeTeamID(parts[0])
		if teamID == "" {
			http.NotFound(w, r)
			return
		}
		if len(parts) == 1 {
			handleTeamArchive(app, store, teamID, "", w, r)
			return
		}
		if len(parts) == 2 {
			handleTeamArchive(app, store, teamID, parts[1], w, r)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/teams", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/teams" {
			http.NotFound(w, r)
			return
		}
		handleTeamIndex(app, store, w, r)
	})
	mux.HandleFunc("/teams/", func(w http.ResponseWriter, r *http.Request) {
		trimmed := strings.Trim(strings.TrimPrefix(r.URL.Path, "/teams/"), "/")
		if trimmed == "" {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(trimmed, "/")
		teamID := teamcore.NormalizeTeamID(parts[0])
		if teamID == "" {
			http.NotFound(w, r)
			return
		}
		if len(parts) == 1 {
			handleTeam(app, store, teamID, w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "tasks" {
			handleTeamTasks(app, store, teamID, w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "members" {
			handleTeamMembers(app, store, teamID, w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "history" {
			handleTeamHistory(app, store, teamID, w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "archive" && r.Method == http.MethodPost {
			handleTeamArchiveCreate(store, teamID, w, r)
			return
		}
		if len(parts) == 3 && parts[1] == "tasks" && parts[2] == "create" && r.Method == http.MethodPost {
			handleTeamTaskCreate(store, teamID, w, r)
			return
		}
		if len(parts) == 3 && parts[1] == "tasks" {
			handleTeamTask(app, store, teamID, parts[2], w, r)
			return
		}
		if len(parts) == 4 && parts[1] == "tasks" && parts[3] == "update" && r.Method == http.MethodPost {
			handleTeamTaskUpdate(store, teamID, parts[2], w, r)
			return
		}
		if len(parts) == 4 && parts[1] == "tasks" && parts[3] == "comment" && r.Method == http.MethodPost {
			handleTeamTaskCommentCreate(store, teamID, parts[2], w, r)
			return
		}
		if len(parts) == 4 && parts[1] == "tasks" && parts[3] == "status" && r.Method == http.MethodPost {
			handleTeamTaskStatus(store, teamID, parts[2], w, r)
			return
		}
		if len(parts) == 4 && parts[1] == "tasks" && parts[3] == "delete" && r.Method == http.MethodPost {
			handleTeamTaskDelete(store, teamID, parts[2], w, r)
			return
		}
		if len(parts) == 3 && parts[1] == "artifacts" && parts[2] == "create" && r.Method == http.MethodPost {
			handleTeamArtifactCreate(store, teamID, w, r)
			return
		}
		if len(parts) == 4 && parts[1] == "artifacts" && parts[3] == "update" && r.Method == http.MethodPost {
			handleTeamArtifactUpdate(store, teamID, parts[2], w, r)
			return
		}
		if len(parts) == 4 && parts[1] == "artifacts" && parts[3] == "delete" && r.Method == http.MethodPost {
			handleTeamArtifactDelete(store, teamID, parts[2], w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "artifacts" {
			handleTeamArtifacts(app, store, teamID, w, r)
			return
		}
		if len(parts) == 3 && parts[1] == "artifacts" {
			handleTeamArtifact(app, store, teamID, parts[2], w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "policy" && r.Method == http.MethodPost {
			handleTeamPolicyUpdate(store, teamID, w, r)
			return
		}
		if len(parts) == 3 && parts[1] == "members" && parts[2] == "action" && r.Method == http.MethodPost {
			handleTeamMemberAction(store, teamID, w, r)
			return
		}
		if len(parts) == 3 && parts[1] == "members" && parts[2] == "bulk-action" && r.Method == http.MethodPost {
			handleTeamMemberBulkAction(store, teamID, w, r)
			return
		}
		if len(parts) == 3 && parts[1] == "members" && parts[2] == "update" && r.Method == http.MethodPost {
			handleTeamMemberUpdate(store, teamID, w, r)
			return
		}
		if len(parts) == 3 && parts[1] == "channels" && parts[2] == "create" && r.Method == http.MethodPost {
			handleTeamChannelCreate(store, teamID, w, r)
			return
		}
		if len(parts) == 4 && parts[1] == "channels" && parts[3] == "update" && r.Method == http.MethodPost {
			channelID := normalizeTeamChannel(parts[2])
			if channelID == "" {
				http.NotFound(w, r)
				return
			}
			handleTeamChannelUpdate(store, teamID, channelID, w, r)
			return
		}
		if len(parts) == 4 && parts[1] == "channels" && parts[3] == "hide" && r.Method == http.MethodPost {
			channelID := normalizeTeamChannel(parts[2])
			if channelID == "" {
				http.NotFound(w, r)
				return
			}
			handleTeamChannelHide(store, teamID, channelID, w, r)
			return
		}
		if len(parts) == 3 && parts[1] == "channels" {
			channelID := normalizeTeamChannel(parts[2])
			if channelID == "" {
				http.NotFound(w, r)
				return
			}
			handleTeamChannel(app, store, teamID, channelID, w, r)
			return
		}
		if len(parts) == 5 && parts[1] == "channels" && parts[3] == "messages" && parts[4] == "create" && r.Method == http.MethodPost {
			channelID := normalizeTeamChannel(parts[2])
			if channelID == "" {
				http.NotFound(w, r)
				return
			}
			handleTeamChannelMessageCreate(store, teamID, channelID, w, r)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/api/teams", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/teams" {
			http.NotFound(w, r)
			return
		}
		handleAPITeamIndex(store, w, r)
	})
	mux.HandleFunc("/api/teams/", func(w http.ResponseWriter, r *http.Request) {
		trimmed := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/teams/"), "/")
		if trimmed == "" {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(trimmed, "/")
		if len(parts) > 5 && !(len(parts) >= 3 && parts[1] == "agents") {
			http.NotFound(w, r)
			return
		}
		teamID := teamcore.NormalizeTeamID(parts[0])
		if teamID == "" {
			http.NotFound(w, r)
			return
		}
		if len(parts) == 1 {
			handleAPITeam(store, teamID, w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "channels" {
			handleAPITeamChannels(store, teamID, w, r)
			return
		}
		if len(parts) == 3 && parts[1] == "channels" {
			channelID := normalizeTeamChannel(parts[2])
			if channelID == "" {
				http.NotFound(w, r)
				return
			}
			handleAPITeamChannel(store, teamID, channelID, w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "policy" {
			handleAPITeamPolicy(store, teamID, w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "history" {
			handleAPITeamHistory(store, teamID, w, r)
			return
		}
		if len(parts) == 3 && parts[1] == "sync" && parts[2] == "conflicts" {
			handleAPITeamSyncConflicts(app, store, teamID, w, r)
			return
		}
		if len(parts) == 5 && parts[1] == "sync" && parts[2] == "conflicts" && parts[4] == "resolve" {
			handleAPITeamSyncConflictResolve(app, store, teamID, parts[3], w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "archive" {
			handleAPITeamArchiveCreate(store, teamID, w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "members" {
			handleAPITeamMembers(store, teamID, w, r)
			return
		}
		if len(parts) == 3 && parts[1] == "members" && parts[2] == "action" {
			handleAPITeamMemberAction(store, teamID, w, r)
			return
		}
		if len(parts) == 3 && parts[1] == "members" && parts[2] == "bulk-action" {
			handleAPITeamMemberBulkAction(store, teamID, w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "messages" {
			handleAPITeamMessages(store, teamID, w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "webhooks" {
			handleAPITeamWebhooks(store, teamID, w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "events" {
			handleAPITeamEvents(store, teamID, w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "agents" {
			handleAPITeamAgents(store, teamID, w, r)
			return
		}
		if len(parts) >= 3 && parts[1] == "agents" {
			agentID, err := url.PathUnescape(strings.Join(parts[2:], "/"))
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			handleAPITeamAgent(store, teamID, agentID, w, r)
			return
		}
		if len(parts) == 3 && parts[1] == "contexts" {
			handleAPITeamContext(store, teamID, parts[2], w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "tasks" {
			handleAPITeamTasks(store, teamID, w, r)
			return
		}
		if len(parts) == 4 && parts[1] == "tasks" && parts[3] == "comment" {
			handleAPITeamTaskCommentCreate(store, teamID, parts[2], w, r)
			return
		}
		if len(parts) == 3 && parts[1] == "tasks" {
			handleAPITeamTask(store, teamID, parts[2], w, r)
			return
		}
		if len(parts) == 4 && parts[1] == "tasks" && parts[3] == "status" {
			handleAPITeamTaskStatus(store, teamID, parts[2], w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "artifacts" {
			handleAPITeamArtifacts(store, teamID, w, r)
			return
		}
		if len(parts) == 3 && parts[1] == "artifacts" {
			handleAPITeamArtifact(store, teamID, parts[2], w, r)
			return
		}
		if len(parts) == 4 && parts[1] == "channels" && parts[3] == "messages" {
			channelID := normalizeTeamChannel(parts[2])
			if channelID == "" {
				http.NotFound(w, r)
				return
			}
			handleAPITeamChannelMessages(store, teamID, channelID, w, r)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/api/archive/team", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/archive/team" {
			http.NotFound(w, r)
			return
		}
		handleAPITeamArchiveIndex(store, w, r)
	})
	mux.HandleFunc("/api/archive/team/", func(w http.ResponseWriter, r *http.Request) {
		trimmed := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/archive/team/"), "/")
		if trimmed == "" {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(trimmed, "/")
		teamID := teamcore.NormalizeTeamID(parts[0])
		if teamID == "" {
			http.NotFound(w, r)
			return
		}
		if len(parts) == 1 {
			handleAPITeamArchive(store, teamID, "", w, r)
			return
		}
		if len(parts) == 2 {
			handleAPITeamArchive(store, teamID, parts[1], w, r)
			return
		}
		http.NotFound(w, r)
	})
	mux.Handle("/static/", newsplugin.NoStoreStaticHandler(staticFS))
	return mux
}
