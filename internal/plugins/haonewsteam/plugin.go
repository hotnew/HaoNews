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
	"hao.news/internal/plugins/haonewsteam/roomplugin"
	"hao.news/internal/plugins/haonewsteam/rooms/artifactroom"
	"hao.news/internal/plugins/haonewsteam/rooms/decisionroom"
	"hao.news/internal/plugins/haonewsteam/rooms/handoffroom"
	"hao.news/internal/plugins/haonewsteam/rooms/incidentroom"
	"hao.news/internal/plugins/haonewsteam/rooms/planexchange"
	"hao.news/internal/plugins/haonewsteam/rooms/reviewroom"
	roomthemes "hao.news/internal/themes/room-themes"
	roomboard "hao.news/internal/themes/room-themes/board"
	roomfocus "hao.news/internal/themes/room-themes/focus"
	roomminimal "hao.news/internal/themes/room-themes/minimal"
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
	registry := roomplugin.NewRegistry()
	registry.MustRegister(artifactroom.New())
	registry.MustRegister(decisionroom.New())
	registry.MustRegister(handoffroom.New())
	registry.MustRegister(incidentroom.New())
	registry.MustRegister(planexchange.New())
	registry.MustRegister(reviewroom.New())
	themeRegistry := roomthemes.NewRegistry()
	themeRegistry.MustRegister(roomboard.New())
	themeRegistry.MustRegister(roomfocus.New())
	themeRegistry.MustRegister(roomminimal.New())
	if !strings.HasSuffix(filepathBase(os.Args[0]), ".test") {
		startTeamWorkspaceWarmup(ctx, app, store)
	}
	return &apphost.Site{
		Manifest: Plugin{}.Manifest(),
		Theme:    theme.Manifest(),
		Handler:  newHandler(app, store, staticFS, registry, themeRegistry),
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
	teams, err := store.ListTeamsCtx(context.Background())
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
		_, _ = store.LoadTeamCtx(context.Background(), teamID)
		_, _ = store.LoadMembersCtx(context.Background(), teamID)
		_, _ = store.LoadPolicyCtx(context.Background(), teamID)
		_, _ = store.LoadMessagesCtx(context.Background(), teamID, "main", 20)
		_, _ = store.LoadTasksCtx(context.Background(), teamID, 20)
		_, _ = store.LoadArtifactsCtx(context.Background(), teamID, 20)
		_, _ = store.LoadHistoryCtx(context.Background(), teamID, 20)
		_, _ = store.ListChannelsCtx(context.Background(), teamID)
		_, _ = store.ListArchivesCtx(context.Background(), teamID)
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

func splitEscapedPathSegments(r *http.Request, prefix string) []string {
	if r == nil {
		return nil
	}
	trimmed := strings.Trim(strings.TrimPrefix(r.URL.EscapedPath(), prefix), "/")
	if trimmed == "" {
		return nil
	}
	rawParts := strings.Split(trimmed, "/")
	parts := make([]string, 0, len(rawParts))
	for _, raw := range rawParts {
		part, err := url.PathUnescape(raw)
		if err != nil {
			parts = append(parts, raw)
			continue
		}
		parts = append(parts, part)
	}
	return parts
}

func newHandler(app *newsplugin.App, store *teamcore.Store, staticFS fs.FS, roomRegistry *roomplugin.Registry, themeRegistry *roomthemes.Registry) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/agent.json", func(w http.ResponseWriter, r *http.Request) {
		handleA2AWellKnownAgent(app, store, w, r)
	})
	mux.HandleFunc("/a2a/teams/", func(w http.ResponseWriter, r *http.Request) {
		parts := splitEscapedPathSegments(r, "/a2a/teams/")
		if len(parts) == 0 {
			http.NotFound(w, r)
			return
		}
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
		parts := splitEscapedPathSegments(r, "/archive/team/")
		if len(parts) == 0 {
			http.NotFound(w, r)
			return
		}
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
		parts := splitEscapedPathSegments(r, "/teams/")
		if len(parts) == 0 {
			http.NotFound(w, r)
			return
		}
		teamID := teamcore.NormalizeTeamID(parts[0])
		if teamID == "" {
			http.NotFound(w, r)
			return
		}
		if len(parts) == 1 {
			handleTeam(app, store, themeRegistry, teamID, w, r)
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
		if len(parts) == 2 && parts[1] == "sync" {
			handleTeamSync(app, store, teamID, w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "search" {
			handleTeamSearch(app, store, teamID, w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "webhooks" {
			handleTeamWebhookPage(app, store, teamID, w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "a2a" {
			handleTeamA2APage(app, store, teamID, w, r)
			return
		}
		if len(parts) >= 3 && parts[1] == "r" {
			pluginID := strings.TrimSpace(parts[2])
			if pluginID == "" || roomRegistry == nil {
				http.NotFound(w, r)
				return
			}
			rp, ok := roomRegistry.Get(pluginID)
			if !ok {
				http.NotFound(w, r)
				return
			}
			prefix := "/teams/" + teamID + "/r/" + pluginID
			http.StripPrefix(prefix, rp.Handler(store, teamID)).ServeHTTP(w, r)
			return
		}
		if len(parts) == 5 && parts[1] == "sync" && parts[2] == "conflicts" && parts[4] == "resolve" && r.Method == http.MethodPost {
			handleTeamSyncConflictResolvePage(app, store, teamID, parts[3], w, r)
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
		if len(parts) == 5 && parts[1] == "channels" && parts[3] == "config" && parts[4] == "update" && r.Method == http.MethodPost {
			channelID := normalizeTeamChannel(parts[2])
			if channelID == "" {
				http.NotFound(w, r)
				return
			}
			handleTeamChannelConfigUpdate(store, roomRegistry, themeRegistry, teamID, channelID, w, r)
			return
		}
		if len(parts) == 3 && parts[1] == "channels" {
			channelID := normalizeTeamChannel(parts[2])
			if channelID == "" {
				http.NotFound(w, r)
				return
			}
			handleTeamChannel(app, store, roomRegistry, themeRegistry, teamID, channelID, w, r)
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
		parts := splitEscapedPathSegments(r, "/api/teams/")
		if len(parts) == 0 {
			http.NotFound(w, r)
			return
		}
		if len(parts) > 5 && !(len(parts) >= 3 && parts[1] == "agents") && !(len(parts) >= 3 && parts[1] == "r") {
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
		if len(parts) >= 3 && parts[1] == "r" {
			pluginID := strings.TrimSpace(parts[2])
			if pluginID == "" || roomRegistry == nil {
				http.NotFound(w, r)
				return
			}
			rp, ok := roomRegistry.Get(pluginID)
			if !ok {
				http.NotFound(w, r)
				return
			}
			prefix := "/api/teams/" + teamID + "/r/" + pluginID
			http.StripPrefix(prefix, rp.Handler(store, teamID)).ServeHTTP(w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "channels" {
			handleAPITeamChannels(store, teamID, w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "milestones" {
			handleAPITeamMilestones(store, teamID, w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "channel-configs" {
			handleAPITeamChannelConfigs(store, teamID, w, r)
			return
		}
		if len(parts) == 3 && parts[1] == "milestones" {
			handleAPITeamMilestone(store, teamID, parts[2], w, r)
			return
		}
		if len(parts) == 3 && parts[1] == "channels" {
			channelID := normalizeTeamChannel(parts[2])
			if channelID == "" {
				http.NotFound(w, r)
				return
			}
			handleAPITeamChannel(store, roomRegistry, themeRegistry, teamID, channelID, w, r)
			return
		}
		if len(parts) == 4 && parts[1] == "channels" && parts[3] == "config" {
			channelID := normalizeTeamChannel(parts[2])
			if channelID == "" {
				http.NotFound(w, r)
				return
			}
			handleAPITeamChannelConfig(store, teamID, channelID, w, r)
			return
		}
		if len(parts) == 4 && parts[1] == "channels" && parts[3] == "context" {
			channelID := normalizeTeamChannel(parts[2])
			if channelID == "" {
				http.NotFound(w, r)
				return
			}
			handleAPITeamChannelContext(store, teamID, channelID, w, r)
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
		if len(parts) == 2 && parts[1] == "sync" {
			handleAPITeamSync(app, store, teamID, w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "search" {
			handleAPITeamSearch(store, teamID, w, r)
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
		if len(parts) == 3 && parts[1] == "webhooks" && parts[2] == "status" {
			handleAPITeamWebhookStatus(store, teamID, w, r)
			return
		}
		if len(parts) == 4 && parts[1] == "webhooks" && parts[2] == "replay" {
			handleAPITeamWebhookReplay(store, teamID, parts[3], w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "events" {
			handleAPITeamEvents(store, teamID, w, r)
			return
		}
		if len(parts) == 2 && parts[1] == "notifications" {
			handleAPITeamNotifications(store, teamID, w, r)
			return
		}
		if len(parts) == 3 && parts[1] == "notifications" && parts[2] == "stream" {
			handleAPITeamNotificationsStream(store, teamID, w, r)
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
		if len(parts) == 4 && parts[1] == "tasks" && parts[3] == "thread" {
			handleAPITeamTaskThread(store, teamID, parts[2], w, r)
			return
		}
		if len(parts) == 4 && parts[1] == "tasks" && parts[3] == "dispatch" {
			handleAPITeamTaskDispatch(store, teamID, parts[2], w, r)
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
		if len(parts) == 3 && parts[1] == "artifacts" && parts[2] == "export" {
			handleAPITeamArtifactExport(store, teamID, w, r)
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
		parts := splitEscapedPathSegments(r, "/api/archive/team/")
		if len(parts) == 0 {
			http.NotFound(w, r)
			return
		}
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
