package haonewsteam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	corehaonews "hao.news/internal/haonews"
	teamcore "hao.news/internal/haonews/team"
	newsplugin "hao.news/internal/plugins/haonews"
	"hao.news/internal/plugins/haonewsteam/roomplugin"
	roomthemes "hao.news/internal/themes/room-themes"
)

func handleTeamIndex(app *newsplugin.App, store *teamcore.Store, w http.ResponseWriter, r *http.Request) {
	teams, err := store.ListTeamsCtx(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	memberCount := 0
	for _, team := range teams {
		memberCount += team.MemberCount
	}
	digests := buildTeamActivityDigests(r.Context(), app, store, teams)
	unresolved := 0
	deadLetters := 0
	for _, digest := range digests {
		unresolved += digest.UnresolvedConflicts
		deadLetters += digest.WebhookDeadLetters
	}
	data := teamIndexPageData{
		Project:    app.ProjectName(),
		Version:    app.VersionString(),
		PageNav:    app.PageNav("/teams"),
		NodeStatus: app.NodeStatus(index),
		Now:        time.Now(),
		Teams:      teams,
		Digests:    digests,
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "团队数", Value: formatTeamCount(len(teams))},
			{Label: "成员总数", Value: formatTeamCount(memberCount)},
			{Label: "待处理冲突", Value: formatTeamCount(unresolved)},
			{Label: "webhook dead-letter", Value: formatTeamCount(deadLetters)},
			{Label: "最近更新", Value: latestTeamValue(teams)},
		},
	}
	if err := app.Templates().ExecuteTemplate(w, "team.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleTeam(app *newsplugin.App, store *teamcore.Store, themeRegistry *roomthemes.Registry, teamID string, w http.ResponseWriter, r *http.Request) {
	info, err := store.LoadTeamCtx(r.Context(), teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var (
		members        []teamcore.Member
		policy         teamcore.Policy
		messages       []teamcore.Message
		tasks          []teamcore.Task
		artifacts      []teamcore.Artifact
		history        []teamcore.ChangeEvent
		channels       []teamcore.ChannelSummary
		channelConfigs []teamcore.ChannelConfig
		conflicts      []corehaonews.TeamSyncConflictRecord
		webhooks       teamcore.WebhookDeliveryStatus
		index          newsplugin.Index
	)
	g, _ := errgroup.WithContext(r.Context())
	g.Go(func() error {
		items, err := store.LoadMembersCtx(r.Context(), teamID)
		if err != nil {
			return err
		}
		members = items
		return nil
	})
	g.Go(func() error {
		value, err := store.LoadPolicyCtx(r.Context(), teamID)
		if err != nil {
			return err
		}
		policy = value
		return nil
	})
	g.Go(func() error {
		items, err := store.LoadMessagesCtx(r.Context(), teamID, "main", 20)
		if err != nil {
			return err
		}
		messages = items
		return nil
	})
	g.Go(func() error {
		items, err := store.LoadTasksCtx(r.Context(), teamID, 20)
		if err != nil {
			return err
		}
		tasks = items
		return nil
	})
	g.Go(func() error {
		items, err := store.LoadArtifactsCtx(r.Context(), teamID, 20)
		if err != nil {
			return err
		}
		artifacts = items
		return nil
	})
	g.Go(func() error {
		items, err := store.LoadHistoryCtx(r.Context(), teamID, 20)
		if err != nil {
			return err
		}
		history = items
		return nil
	})
	g.Go(func() error {
		items, err := store.ListChannelsCtx(r.Context(), teamID)
		if err != nil {
			return err
		}
		channels = items
		return nil
	})
	g.Go(func() error {
		items, err := store.ListChannelConfigsCtx(r.Context(), teamID)
		if err != nil {
			return err
		}
		channelConfigs = items
		return nil
	})
	g.Go(func() error {
		items, err := corehaonews.LoadTeamSyncConflicts(app.StoreRoot(), teamID, corehaonews.TeamSyncConflictFilter{Limit: 5})
		if err != nil {
			return err
		}
		conflicts = items
		return nil
	})
	g.Go(func() error {
		value, err := store.LoadWebhookDeliveryStatusCtx(r.Context(), teamID)
		if err != nil {
			return err
		}
		webhooks = value
		return nil
	})
	g.Go(func() error {
		value, err := app.Index()
		if err != nil {
			return err
		}
		index = value
		return nil
	})
	if err := g.Wait(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := teamPageData{
		Project:             app.ProjectName(),
		Version:             app.VersionString(),
		PageNav:             app.PageNav("/teams"),
		NodeStatus:          app.NodeStatus(index),
		Now:                 time.Now(),
		Team:                info,
		Policy:              policy,
		Members:             members,
		ActiveMembers:       filterMembersByStatus(members, "active"),
		PendingMembers:      filterMembersByStatus(members, "pending"),
		MutedMembers:        filterMembersByStatus(members, "muted"),
		RemovedMembers:      filterMembersByStatus(members, "removed"),
		Owners:              filterMembersByRole(members, "owner"),
		Maintainers:         filterMembersByRole(members, "maintainer"),
		Observers:           filterMembersByRole(members, "observer"),
		Messages:            messages,
		Tasks:               tasks,
		Channels:            channels,
		Artifacts:           artifacts,
		History:             history,
		RecentConflicts:     conflicts,
		UnresolvedConflicts: countUnresolvedTeamConflicts(conflicts),
		ResolvedConflicts:   countResolvedTeamConflicts(app.StoreRoot(), teamID),
		WebhookStatus:       webhooks,
		TaskStatusCounts:    taskStatusCounts(tasks),
		ArtifactKindCounts:  artifactKindCounts(artifacts),
		DefaultActorAgentID: teamDefaultActor(info, members),
		CanQuickPost:        !policy.RequireSignature,
		PolicyNotice:        teamPolicyNotice(policy),
		FocusTasks:          buildTeamFocusTasks(tasks, artifacts, history, 4),
		RecentMessageItems:  buildTeamMessagePreviews(messages, 5),
		RecentChangeItems:   buildTeamChangePreviews(history, 5),
		DashboardAlerts:     buildTeamDashboardAlerts(policy, conflicts, webhooks),
		RoomEntries:         buildTeamRoomEntries(info.TeamID, channels, channelConfigs),
		AvailableRoomThemes: buildRoomThemeSummaries(themeRegistry),
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "成员", Value: formatTeamCount(countMembersByStatus(members, "active"))},
			{Label: "频道", Value: formatTeamCount(len(channels))},
			{Label: "任务", Value: formatTeamCount(len(tasks))},
			{Label: "产物", Value: formatTeamCount(len(artifacts))},
			{Label: "待审批", Value: formatTeamCount(countMembersByStatus(members, "pending"))},
			{Label: "冲突", Value: formatTeamCount(countUnresolvedTeamConflicts(conflicts))},
			{Label: "dead-letter", Value: formatTeamCount(webhooks.DeadLetterCount)},
		},
	}
	if err := app.Templates().ExecuteTemplate(w, "team_detail.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func buildRoomThemeSummaries(themeRegistry *roomthemes.Registry) []teamRoomThemeSummary {
	if themeRegistry == nil {
		return []teamRoomThemeSummary{}
	}
	manifests := themeRegistry.Manifests()
	out := make([]teamRoomThemeSummary, 0, len(manifests))
	for _, manifest := range manifests {
		out = append(out, teamRoomThemeSummary{
			ID:           manifest.ID,
			Name:         manifest.Name,
			Version:      manifest.Version,
			Description:  manifest.Description,
			Overrides:    append([]string(nil), manifest.Overrides...),
			PreviewClass: manifest.PreviewClass,
		})
	}
	return out
}

func buildRoomPluginSummaries(roomRegistry *roomplugin.Registry) []teamRoomPluginSummary {
	if roomRegistry == nil {
		return nil
	}
	manifests := roomRegistry.Manifests()
	out := make([]teamRoomPluginSummary, 0, len(manifests))
	for _, manifest := range manifests {
		out = append(out, teamRoomPluginSummary{
			ID:            manifest.ID,
			Name:          manifest.Name,
			Version:       manifest.Version,
			ConfigValue:   roomPluginConfigValue(manifest),
			Description:   manifest.Description,
			MinTeamVer:    manifest.MinTeamVersion,
			MessageKinds:  append([]string(nil), manifest.MessageKinds...),
			ArtifactKinds: append([]string(nil), manifest.ArtifactKinds...),
		})
	}
	return out
}

func roomPluginSummaryMap(roomRegistry *roomplugin.Registry) map[string]teamRoomPluginSummary {
	items := buildRoomPluginSummaries(roomRegistry)
	out := make(map[string]teamRoomPluginSummary, len(items))
	for _, item := range items {
		out[item.ID] = item
		out[item.ConfigValue] = item
	}
	return out
}

func roomThemeSummaryMap(themeRegistry *roomthemes.Registry) map[string]teamRoomThemeSummary {
	items := buildRoomThemeSummaries(themeRegistry)
	out := make(map[string]teamRoomThemeSummary, len(items))
	for _, item := range items {
		out[item.ID] = item
	}
	return out
}

func roomPluginConfigValue(manifest roomplugin.Manifest) string {
	version := strings.TrimSpace(manifest.Version)
	if version == "" {
		return manifest.ID
	}
	parts := strings.Split(version, ".")
	if len(parts) >= 2 {
		version = parts[0] + "." + parts[1]
	}
	return strings.TrimSpace(manifest.ID) + "@" + version
}

func clampTeamListLimit(raw string, fallback, max int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	if value <= 0 {
		return fallback
	}
	if max > 0 && value > max {
		return max
	}
	return value
}

func handleTeamHistory(app *newsplugin.App, store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	info, err := store.LoadTeamCtx(r.Context(), teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	history, err := store.LoadHistoryCtx(r.Context(), teamID, 200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	conflicts, err := corehaonews.LoadTeamSyncConflicts(app.StoreRoot(), teamID, corehaonews.TeamSyncConflictFilter{Limit: 8})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filterScope := strings.TrimSpace(r.URL.Query().Get("scope"))
	filterSource := strings.TrimSpace(r.URL.Query().Get("source"))
	filterActor := strings.TrimSpace(r.URL.Query().Get("actor"))
	scopeCounts := historyScopeCounts(history)
	sourceCounts := historySourceCounts(history)
	scopes := historyScopes(history)
	sources := historySources(history)
	history = filterHistory(history, filterScope, filterSource, filterActor)
	data := teamHistoryPageData{
		Project:      app.ProjectName(),
		Version:      app.VersionString(),
		PageNav:      app.PageNav("/teams"),
		NodeStatus:   app.NodeStatus(index),
		Now:          time.Now(),
		Team:         info,
		History:      history,
		FilterScope:  filterScope,
		FilterSource: filterSource,
		FilterActor:  filterActor,
		AppliedFilters: appliedTeamFilters(
			labeledTeamFilter("Scope", filterScope),
			labeledTeamFilter("Source", filterSource),
			labeledTeamFilter("Actor", filterActor),
		),
		Scopes:              scopes,
		Sources:             sources,
		RecentConflicts:     conflicts,
		UnresolvedConflicts: countUnresolvedTeamConflicts(conflicts),
		ResolvedConflicts:   countResolvedTeamConflicts(app.StoreRoot(), teamID),
		ScopeCounts:         scopeCounts,
		SourceCounts:        sourceCounts,
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "变更", Value: formatTeamCount(len(history))},
			{Label: "最近来源", Value: latestHistorySource(history)},
			{Label: "最近时间", Value: latestHistoryValue(history)},
		},
	}
	if err := app.Templates().ExecuteTemplate(w, "team_history.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func relatedArtifacts(artifacts []teamcore.Artifact, taskID string, limit int) []teamcore.Artifact {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || len(artifacts) == 0 {
		return nil
	}
	capHint := len(artifacts)
	if limit > 0 && limit < capHint {
		capHint = limit
	}
	out := make([]teamcore.Artifact, 0, capHint)
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.TaskID) != taskID {
			continue
		}
		out = append(out, artifact)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func relatedTasksByChannel(tasks []teamcore.Task, channelID string, limit int) []teamcore.Task {
	channelID = normalizeTeamChannel(channelID)
	if channelID == "" || len(tasks) == 0 {
		return nil
	}
	capHint := len(tasks)
	if limit > 0 && limit < capHint {
		capHint = limit
	}
	out := make([]teamcore.Task, 0, capHint)
	for _, task := range tasks {
		if normalizeTeamChannel(task.ChannelID) != channelID {
			continue
		}
		out = append(out, task)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func relatedArtifactsByChannel(artifacts []teamcore.Artifact, channelID string, limit int) []teamcore.Artifact {
	channelID = normalizeTeamChannel(channelID)
	if channelID == "" || len(artifacts) == 0 {
		return nil
	}
	capHint := len(artifacts)
	if limit > 0 && limit < capHint {
		capHint = limit
	}
	out := make([]teamcore.Artifact, 0, capHint)
	for _, artifact := range artifacts {
		if normalizeTeamChannel(artifact.ChannelID) != channelID {
			continue
		}
		out = append(out, artifact)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func channelHistory(history []teamcore.ChangeEvent, channelID string, limit int) []teamcore.ChangeEvent {
	channelID = normalizeTeamChannel(channelID)
	if channelID == "" || len(history) == 0 {
		return nil
	}
	out := make([]teamcore.ChangeEvent, 0, min(limit, len(history)))
	for _, event := range history {
		switch event.Scope {
		case "channel":
			if normalizeTeamChannel(strings.TrimSpace(event.SubjectID)) == channelID {
				out = append(out, event)
			}
		case "task":
			if taskChannelFromHistory(event) == channelID {
				out = append(out, event)
			}
		case "artifact":
			if artifactChannelFromHistory(event) == channelID {
				out = append(out, event)
			}
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func taskChannelFromHistory(event teamcore.ChangeEvent) string {
	if event.Scope != "task" {
		return ""
	}
	if channel := historyMetadataText(event.Metadata, "channel_after"); channel != "" {
		return normalizeTeamChannel(channel)
	}
	return normalizeTeamChannel(historyMetadataText(event.Metadata, "channel_before"))
}

func artifactChannelFromHistory(event teamcore.ChangeEvent) string {
	if event.Scope != "artifact" {
		return ""
	}
	if channel := historyMetadataText(event.Metadata, "channel_after"); channel != "" {
		return normalizeTeamChannel(channel)
	}
	return normalizeTeamChannel(historyMetadataText(event.Metadata, "channel_before"))
}

func historyMetadataText(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	raw, ok := metadata[key]
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func artifactKinds(artifacts []teamcore.Artifact) []string {
	seen := make(map[string]struct{}, len(artifacts))
	out := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		kind := strings.TrimSpace(artifact.Kind)
		if kind == "" {
			continue
		}
		if _, ok := seen[kind]; ok {
			continue
		}
		seen[kind] = struct{}{}
		out = append(out, kind)
	}
	sort.Strings(out)
	return out
}

func artifactFilterTasks(tasks []teamcore.Task, artifacts []teamcore.Artifact) []teamcore.Task {
	if len(tasks) == 0 || len(artifacts) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(artifacts))
	for _, artifact := range artifacts {
		taskID := strings.TrimSpace(artifact.TaskID)
		if taskID != "" {
			seen[taskID] = struct{}{}
		}
	}
	out := make([]teamcore.Task, 0, len(seen))
	for _, task := range tasks {
		if _, ok := seen[strings.TrimSpace(task.TaskID)]; ok {
			out = append(out, task)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Title != out[j].Title {
			return out[i].Title < out[j].Title
		}
		return out[i].TaskID < out[j].TaskID
	})
	return out
}

func filterArtifacts(artifacts []teamcore.Artifact, kind, channelID, taskID string) []teamcore.Artifact {
	if kind == "" && channelID == "" && taskID == "" {
		return artifacts
	}
	out := make([]teamcore.Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		if kind != "" && !strings.EqualFold(strings.TrimSpace(artifact.Kind), kind) {
			continue
		}
		if channelID != "" && normalizeTeamChannel(artifact.ChannelID) != channelID {
			continue
		}
		if taskID != "" && strings.TrimSpace(artifact.TaskID) != taskID {
			continue
		}
		out = append(out, artifact)
	}
	return out
}

func artifactHistory(history []teamcore.ChangeEvent, artifactID string, limit int) []teamcore.ChangeEvent {
	artifactID = strings.TrimSpace(artifactID)
	if artifactID == "" || len(history) == 0 {
		return nil
	}
	out := make([]teamcore.ChangeEvent, 0, min(limit, len(history)))
	for _, event := range history {
		if event.Scope != "artifact" {
			continue
		}
		if strings.TrimSpace(event.SubjectID) != artifactID {
			continue
		}
		out = append(out, event)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func countArtifactsByTask(artifacts []teamcore.Artifact, taskID string) int {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return 0
	}
	count := 0
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.TaskID) == taskID {
			count++
		}
	}
	return count
}

func artifactKindCounts(artifacts []teamcore.Artifact) map[string]int {
	out := map[string]int{
		"markdown": 0,
		"link":     0,
		"json":     0,
		"post":     0,
	}
	for _, artifact := range artifacts {
		kind := strings.TrimSpace(artifact.Kind)
		if kind == "" {
			continue
		}
		out[kind]++
	}
	return out
}

func artifactCountsByTask(artifacts []teamcore.Artifact) map[string]int {
	if len(artifacts) == 0 {
		return nil
	}
	out := make(map[string]int, len(artifacts))
	for _, artifact := range artifacts {
		taskID := strings.TrimSpace(artifact.TaskID)
		if taskID == "" {
			continue
		}
		out[taskID]++
	}
	return out
}

func historyCountsByTask(history []teamcore.ChangeEvent) map[string]int {
	if len(history) == 0 {
		return nil
	}
	out := make(map[string]int, len(history))
	for _, event := range history {
		if event.Scope != "task" {
			continue
		}
		taskID := strings.TrimSpace(event.SubjectID)
		if taskID == "" {
			continue
		}
		out[taskID]++
	}
	return out
}

func taskHistory(history []teamcore.ChangeEvent, taskID string, limit int) []teamcore.ChangeEvent {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || len(history) == 0 {
		return nil
	}
	out := make([]teamcore.ChangeEvent, 0, min(limit, len(history)))
	for _, event := range history {
		if event.Scope != "task" {
			continue
		}
		if strings.TrimSpace(event.SubjectID) != taskID {
			continue
		}
		out = append(out, event)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func handleAPITeamIndex(store *teamcore.Store, w http.ResponseWriter, r *http.Request) {
	teams, err := store.ListTeamsCtx(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope": "team-index",
		"count": len(teams),
		"teams": teams,
	})
}

func handleAPITeam(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	info, err := store.LoadTeamCtx(r.Context(), teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	members, err := store.LoadMembersCtx(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	policy, err := store.LoadPolicyCtx(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	channelConfigs, err := store.ListChannelConfigsCtx(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	channelConfigSummary := summarizeTeamChannelConfigs(channelConfigs)
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":                "team-detail",
		"team_id":              info.TeamID,
		"team":                 info,
		"policy":               policy,
		"member_count":         len(members),
		"members":              members,
		"channel_config_count": len(channelConfigs),
		"channel_configs":      channelConfigs,
		"channels_config":      channelConfigSummary,
	})
}

func summarizeTeamChannelConfigs(configs []teamcore.ChannelConfig) []teamChannelConfigSummary {
	if len(configs) == 0 {
		return []teamChannelConfigSummary{}
	}
	out := make([]teamChannelConfigSummary, 0, len(configs))
	for _, cfg := range configs {
		out = append(out, teamChannelConfigSummary{
			ChannelID:       cfg.ChannelID,
			Plugin:          cfg.Plugin,
			PluginID:        cfg.PluginID(),
			Theme:           cfg.Theme,
			AgentOnboarding: cfg.AgentOnboarding,
			Rules:           append([]string(nil), cfg.Rules...),
			UpdatedAt:       cfg.UpdatedAt,
		})
	}
	return out
}

func buildTeamRoomEntries(teamID string, channels []teamcore.ChannelSummary, configs []teamcore.ChannelConfig) []teamRoomEntry {
	if len(channels) == 0 {
		return []teamRoomEntry{}
	}
	configByChannel := make(map[string]teamcore.ChannelConfig, len(configs))
	for _, cfg := range configs {
		configByChannel[normalizeTeamChannel(cfg.ChannelID)] = cfg
	}
	out := make([]teamRoomEntry, 0, len(channels))
	for _, channel := range channels {
		cfg, ok := configByChannel[normalizeTeamChannel(channel.ChannelID)]
		out = append(out, buildTeamRoomEntry(teamID, channel, cfg, ok))
	}
	return out
}

func buildTeamRoomEntry(teamID string, channel teamcore.ChannelSummary, cfg teamcore.ChannelConfig, configured bool) teamRoomEntry {
	entry := teamRoomEntry{
		ChannelID:           channel.ChannelID,
		Plugin:              cfg.Plugin,
		PluginID:            cfg.PluginID(),
		Theme:               cfg.Theme,
		Configured:          configured,
		ChannelPath:         "/teams/" + teamID + "/channels/" + channel.ChannelID,
		ConfigAPIPath:       "/api/teams/" + teamID + "/channels/" + channel.ChannelID + "/config",
		AgentOnboarding:     cfg.AgentOnboarding,
		RuleCount:           len(cfg.Rules),
		UpdatedAt:           cfg.UpdatedAt,
		ChannelTitle:        channel.Title,
		ChannelDescription:  channel.Description,
		ChannelHidden:       channel.Hidden,
		ChannelMessageCount: channel.MessageCount,
	}
	if entry.PluginID != "" {
		entry.RoomWebPath = "/teams/" + teamID + "/r/" + entry.PluginID + "/?channel_id=" + channel.ChannelID
		entry.RoomAPIPath = "/api/teams/" + teamID + "/r/" + entry.PluginID + "/?channel_id=" + channel.ChannelID
	}
	return entry
}

func handleAPITeamHistory(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	info, err := store.LoadTeamCtx(r.Context(), teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	history, err := store.LoadHistoryCtx(r.Context(), teamID, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filterScope := strings.TrimSpace(r.URL.Query().Get("scope"))
	filterSource := strings.TrimSpace(r.URL.Query().Get("source"))
	filterActor := strings.TrimSpace(r.URL.Query().Get("actor"))
	history = filterHistory(history, filterScope, filterSource, filterActor)
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":         "team-history",
		"team_id":       info.TeamID,
		"history_count": len(history),
		"history":       history,
		"applied_filters": map[string]string{
			"scope":  filterScope,
			"source": filterSource,
			"actor":  filterActor,
		},
	})
}

func handleAPITeamPolicy(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		handleAPITeamPolicyUpdate(store, teamID, w, r)
		return
	}
	info, err := store.LoadTeamCtx(r.Context(), teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	policy, err := store.LoadPolicyCtx(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":   "team-policy",
		"team_id": info.TeamID,
		"policy":  policy,
	})
}

func handleAPITeamMessages(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	info, err := store.LoadTeamCtx(r.Context(), teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	channelID := strings.TrimSpace(r.URL.Query().Get("channel"))
	limit := clampTeamListLimit(r.URL.Query().Get("limit"), 50, 100)
	messages, err := store.LoadMessagesCtx(r.Context(), teamID, channelID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if channelID == "" {
		channelID = "main"
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":         "team-messages",
		"team_id":       info.TeamID,
		"channel_id":    channelID,
		"limit":         limit,
		"message_count": len(messages),
		"messages":      messages,
	})
}

func handleAPITeamWebhooks(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		configs, err := store.LoadWebhookConfigsCtx(r.Context(), teamID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
			"scope":    "team-webhooks",
			"team_id":  teamID,
			"count":    len(configs),
			"webhooks": configs,
		})
	case http.MethodPost:
		if !teamRequestTrusted(r) {
			http.Error(w, "team webhook update is limited to local or LAN requests", http.StatusForbidden)
			return
		}
		var payload struct {
			ActorAgentID string                            `json:"actor_agent_id"`
			Webhooks     []teamcore.PushNotificationConfig `json:"webhooks"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireTeamAction(store, teamID, payload.ActorAgentID, "policy.update"); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		if err := store.SaveWebhookConfigsCtx(r.Context(), teamID, payload.Webhooks); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		configs, err := store.LoadWebhookConfigsCtx(r.Context(), teamID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
			"scope":    "team-webhooks",
			"team_id":  teamID,
			"count":    len(configs),
			"webhooks": configs,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAPITeamEvents(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, err := store.LoadTeamCtx(r.Context(), teamID); err != nil {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	events, unsubscribe, err := store.Subscribe(teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer unsubscribe()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, ": team-events\n\n")
	flusher.Flush()
	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()
	writeEvent := func(event teamcore.TeamEvent) error {
		body, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: team\ndata: %s\n\n", body); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-events:
			if err := writeEvent(event); err != nil {
				return
			}
		case <-keepalive.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func handleAPITeamAgents(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cards, err := store.ListAgentCardsCtx(r.Context(), teamID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		taskID := strings.TrimSpace(r.URL.Query().Get("task"))
		var matched []teamcore.AgentCard
		if taskID != "" {
			task, err := store.LoadTaskCtx(r.Context(), teamID, taskID)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					http.NotFound(w, r)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			matched = teamcore.MatchAgentsForTask(cards, task)
		}
		body := map[string]any{
			"scope":       "team-agents",
			"team_id":     teamID,
			"agent_count": len(cards),
			"agents":      cards,
		}
		if taskID != "" {
			body["task_id"] = taskID
			body["matched_count"] = len(matched)
			body["matched_agents"] = matched
		}
		newsplugin.WriteJSON(w, http.StatusOK, body)
	case http.MethodPost:
		if !teamRequestTrusted(r) {
			http.Error(w, "team agent card writes are limited to local or LAN requests", http.StatusForbidden)
			return
		}
		var payload struct {
			Card         teamcore.AgentCard `json:"card"`
			ActorAgentID string             `json:"actor_agent_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		card := payload.Card
		if card.AgentID == "" {
			http.Error(w, "empty agent card", http.StatusBadRequest)
			return
		}
		if err := requireTeamAction(store, teamID, payload.ActorAgentID, "agent_card.register"); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		if err := store.SaveAgentCardCtx(r.Context(), teamID, card); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		loaded, err := store.LoadAgentCardCtx(r.Context(), teamID, card.AgentID)
		if err != nil {
			loaded = card
		}
		newsplugin.WriteJSON(w, http.StatusCreated, map[string]any{
			"scope":   "team-agent-card",
			"team_id": teamID,
			"agent":   loaded,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAPITeamAgent(store *teamcore.Store, teamID, agentID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	card, err := store.LoadAgentCardCtx(r.Context(), teamID, agentID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":   "team-agent-card",
		"team_id": teamID,
		"agent":   card,
	})
}

func formatTeamCount(value int) string {
	return strconv.Itoa(value)
}

func buildTeamActivityDigests(ctx context.Context, app *newsplugin.App, store *teamcore.Store, teams []teamcore.Summary) []teamActivityDigest {
	if len(teams) == 0 || app == nil || store == nil {
		return nil
	}
	out := make([]teamActivityDigest, len(teams))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for i, team := range teams {
		i, team := i, team
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			digest := teamActivityDigest{
				TeamID:       team.TeamID,
				Title:        team.Title,
				Description:  team.Description,
				Visibility:   team.Visibility,
				MemberCount:  team.MemberCount,
				ChannelCount: team.ChannelCount,
				LastActivityAt: func() time.Time {
					if !team.UpdatedAt.IsZero() {
						return team.UpdatedAt
					}
					return team.CreatedAt
				}(),
				TopAction:    "进入 Team",
				TopActionURL: "/teams/" + team.TeamID,
			}

			taskCounts, err := store.CountTasksByStatusCtx(ctx, team.TeamID)
			if err == nil {
				digest.OpenTasks = taskCounts[teamcore.TaskStateOpen] + taskCounts[teamcore.TaskStateDoing] + taskCounts[teamcore.TaskStateBlocked] + taskCounts[teamcore.TaskStateReview]
			}
			channels, err := store.ListChannelsCtx(ctx, team.TeamID)
			if err == nil {
				for _, channel := range channels {
					if channel.Hidden {
						continue
					}
					if !channel.LastMessageAt.IsZero() && channel.LastMessageAt.After(digest.LastActivityAt) {
						digest.LastActivityAt = channel.LastMessageAt
					}
					recent, countErr := store.CountRecentMessagesCtx(ctx, team.TeamID, channel.ChannelID, time.Now().Add(-72*time.Hour))
					if countErr == nil {
						digest.RecentMessages += recent
					}
				}
			}
			conflicts, err := corehaonews.LoadTeamSyncConflicts(app.StoreRoot(), team.TeamID, corehaonews.TeamSyncConflictFilter{Limit: 20})
			if err == nil {
				digest.UnresolvedConflicts = countUnresolvedTeamConflicts(conflicts)
			}
			webhookStatus, err := store.LoadWebhookDeliveryStatusCtx(ctx, team.TeamID)
			if err == nil {
				digest.WebhookDeadLetters = webhookStatus.DeadLetterCount
			}
			switch {
			case digest.WebhookDeadLetters > 0:
				digest.TopAction = "处理 webhook 失败"
				digest.TopActionURL = "/teams/" + team.TeamID + "/webhooks"
				digest.HealthLabel = "需要处理"
			case digest.UnresolvedConflicts > 0:
				digest.TopAction = "处理同步冲突"
				digest.TopActionURL = "/teams/" + team.TeamID + "/sync"
				digest.HealthLabel = "需要处理"
			case digest.OpenTasks > 0:
				digest.TopAction = "查看进行中任务"
				digest.TopActionURL = "/teams/" + team.TeamID + "/tasks?status=open"
				digest.HealthLabel = "进行中"
			case digest.RecentMessages > 0:
				digest.TopAction = "查看最近消息"
				digest.TopActionURL = "/teams/" + team.TeamID + "/channels/main"
				digest.HealthLabel = "活跃"
			default:
				digest.TopAction = "进入 Team"
				digest.TopActionURL = "/teams/" + team.TeamID
				digest.HealthLabel = "稳定"
			}
			out[i] = digest
		}()
	}
	wg.Wait()
	return out
}

func teamDefaultActor(info teamcore.Info, members []teamcore.Member) string {
	if strings.TrimSpace(info.OwnerAgentID) != "" {
		return strings.TrimSpace(info.OwnerAgentID)
	}
	for _, member := range members {
		if member.Status == "active" && strings.TrimSpace(member.AgentID) != "" {
			return strings.TrimSpace(member.AgentID)
		}
	}
	return ""
}

func teamPolicyNotice(policy teamcore.Policy) string {
	if policy.RequireSignature {
		return "本 Team 要求消息签名。网页快捷发消息会被禁用，请改用带签名的 API 消息。"
	}
	if len(policy.Permissions) > 0 {
		return "本 Team 已启用细粒度权限规则。若某些动作被拒绝，请先查看 Policy 摘要。"
	}
	return "当前 Team 可以直接用页面完成高频消息、任务、同步和归档操作。"
}

func buildTeamFocusTasks(tasks []teamcore.Task, artifacts []teamcore.Artifact, history []teamcore.ChangeEvent, limit int) []teamTaskFocusItem {
	if len(tasks) == 0 || limit <= 0 {
		return nil
	}
	copied := append([]teamcore.Task(nil), tasks...)
	sort.SliceStable(copied, func(i, j int) bool { return compareTeamTasks(copied[i], copied[j]) })
	out := make([]teamTaskFocusItem, 0, min(limit, len(copied)))
	for _, task := range copied {
		if len(out) >= limit {
			break
		}
		out = append(out, teamTaskFocusItem{
			TaskID:        task.TaskID,
			Title:         task.Title,
			Status:        task.Status,
			Priority:      task.Priority,
			DueAt:         task.DueAt,
			DueLabel:      describeTaskDue(task),
			ChannelID:     task.ChannelID,
			Assignees:     task.Assignees,
			ArtifactCount: countArtifactsByTask(artifacts, task.TaskID),
			HistoryCount:  countHistoryByTask(history, task.TaskID),
		})
	}
	return out
}

func buildTeamMessagePreviews(messages []teamcore.Message, limit int) []teamMessagePreview {
	if len(messages) == 0 || limit <= 0 {
		return nil
	}
	out := make([]teamMessagePreview, 0, min(limit, len(messages)))
	for _, message := range messages {
		if len(out) >= limit {
			break
		}
		content := strings.TrimSpace(message.Content)
		if len(content) > 88 {
			content = content[:88] + "..."
		}
		out = append(out, teamMessagePreview{
			MessageID:     message.MessageID,
			ChannelID:     message.ChannelID,
			AuthorAgentID: message.AuthorAgentID,
			Content:       content,
			CreatedAt:     message.CreatedAt,
		})
	}
	return out
}

func buildTeamChangePreviews(history []teamcore.ChangeEvent, limit int) []teamChangePreview {
	if len(history) == 0 || limit <= 0 {
		return nil
	}
	out := make([]teamChangePreview, 0, min(limit, len(history)))
	for _, item := range history {
		if len(out) >= limit {
			break
		}
		out = append(out, teamChangePreview{
			EventID:    item.EventID,
			Scope:      item.Scope,
			Action:     item.Action,
			Summary:    item.Summary,
			SubjectID:  item.SubjectID,
			ActorAgent: item.ActorAgentID,
			CreatedAt:  item.CreatedAt,
		})
	}
	return out
}

func buildTeamDashboardAlerts(policy teamcore.Policy, conflicts []corehaonews.TeamSyncConflictRecord, webhook teamcore.WebhookDeliveryStatus) []string {
	alerts := make([]string, 0, 3)
	if policy.RequireSignature {
		alerts = append(alerts, "当前 Team 开启了消息签名要求，网页快捷发消息只在允许的 Team 中可用。")
	}
	if unresolved := countUnresolvedTeamConflicts(conflicts); unresolved > 0 {
		alerts = append(alerts, "当前还有 "+formatTeamCount(unresolved)+" 个未处理同步冲突，建议优先处理。")
	}
	if webhook.DeadLetterCount > 0 {
		alerts = append(alerts, "当前还有 "+formatTeamCount(webhook.DeadLetterCount)+" 条 webhook dead-letter，需要回放或修复。")
	}
	return alerts
}

func describeTaskDue(task teamcore.Task) string {
	if task.DueAt.IsZero() {
		return "未设置截止时间"
	}
	now := time.Now().UTC()
	switch {
	case teamcore.IsTerminalState(task.Status):
		return "已完成任务"
	case task.DueAt.Before(now):
		return "已逾期"
	case task.DueAt.Before(now.Add(24 * time.Hour)):
		return "24 小时内到期"
	case task.DueAt.Before(now.Add(72 * time.Hour)):
		return "3 天内到期"
	default:
		return "已设置截止时间"
	}
}

func countHistoryByTask(history []teamcore.ChangeEvent, taskID string) int {
	count := 0
	for _, item := range history {
		if strings.TrimSpace(item.SubjectID) == strings.TrimSpace(taskID) {
			count++
		}
	}
	return count
}

func latestTeamValue(teams []teamcore.Summary) string {
	for _, team := range teams {
		if !team.UpdatedAt.IsZero() {
			return formatTeamTime(team.UpdatedAt)
		}
		if !team.CreatedAt.IsZero() {
			return formatTeamTime(team.CreatedAt)
		}
	}
	return "暂无"
}

func latestHistoryValue(history []teamcore.ChangeEvent) string {
	for _, event := range history {
		if !event.CreatedAt.IsZero() {
			return formatTeamTime(event.CreatedAt)
		}
	}
	return "暂无"
}

func latestHistorySource(history []teamcore.ChangeEvent) string {
	for _, event := range history {
		if strings.TrimSpace(event.Source) != "" {
			return event.Source
		}
	}
	return "system"
}

func historyScopes(history []teamcore.ChangeEvent) []string {
	seen := make(map[string]struct{}, len(history))
	out := make([]string, 0, len(history))
	for _, event := range history {
		scope := strings.TrimSpace(event.Scope)
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	sort.Strings(out)
	return out
}

func historySources(history []teamcore.ChangeEvent) []string {
	seen := make(map[string]struct{}, len(history))
	out := make([]string, 0, len(history))
	for _, event := range history {
		source := strings.TrimSpace(event.Source)
		if source == "" {
			continue
		}
		if _, ok := seen[source]; ok {
			continue
		}
		seen[source] = struct{}{}
		out = append(out, source)
	}
	sort.Strings(out)
	return out
}

func historyScopeCounts(history []teamcore.ChangeEvent) map[string]int {
	out := make(map[string]int, len(history))
	for _, entry := range history {
		scope := strings.TrimSpace(entry.Scope)
		if scope == "" {
			scope = "unknown"
		}
		out[scope]++
	}
	return out
}

func historySourceCounts(history []teamcore.ChangeEvent) map[string]int {
	out := make(map[string]int, len(history))
	for _, entry := range history {
		source := strings.TrimSpace(entry.Source)
		if source == "" {
			source = "unknown"
		}
		out[source]++
	}
	return out
}

func filterHistory(history []teamcore.ChangeEvent, scope, source, actor string) []teamcore.ChangeEvent {
	scope = strings.TrimSpace(scope)
	source = strings.TrimSpace(source)
	actor = strings.TrimSpace(actor)
	if scope == "" && source == "" && actor == "" {
		return history
	}
	out := make([]teamcore.ChangeEvent, 0, len(history))
	for _, event := range history {
		if scope != "" && !strings.EqualFold(strings.TrimSpace(event.Scope), scope) {
			continue
		}
		if source != "" && !strings.EqualFold(strings.TrimSpace(event.Source), source) {
			continue
		}
		if actor != "" && !strings.Contains(strings.ToLower(strings.TrimSpace(event.ActorAgentID)), strings.ToLower(actor)) {
			continue
		}
		out = append(out, event)
	}
	return out
}

func formatTeamTime(value time.Time) string {
	if value.IsZero() {
		return "暂无"
	}
	return value.In(time.Local).Format("2006-01-02 15:04")
}

func formatTeamTimePtr(value *time.Time) string {
	if value == nil {
		return "暂无"
	}
	return formatTeamTime(*value)
}

func normalizeTeamChannel(value string) string {
	value = teamcore.NormalizeTeamID(value)
	if value == "" {
		return ""
	}
	return value
}

func countMembersByStatus(members []teamcore.Member, status string) int {
	count := 0
	for _, member := range members {
		if member.Status == status {
			count++
		}
	}
	return count
}

func countMembersByRole(members []teamcore.Member, role string) int {
	count := 0
	for _, member := range members {
		if member.Role == role {
			count++
		}
	}
	return count
}

func memberStatusCounts(members []teamcore.Member) map[string]int {
	return map[string]int{
		"active":  countMembersByStatus(members, "active"),
		"pending": countMembersByStatus(members, "pending"),
		"muted":   countMembersByStatus(members, "muted"),
		"removed": countMembersByStatus(members, "removed"),
	}
}

func memberRoleCounts(members []teamcore.Member) map[string]int {
	return map[string]int{
		"owner":      countMembersByRole(members, "owner"),
		"maintainer": countMembersByRole(members, "maintainer"),
		"member":     countMembersByRole(members, "member"),
		"observer":   countMembersByRole(members, "observer"),
	}
}

func memberStatuses(members []teamcore.Member) []string {
	seen := make(map[string]struct{}, len(members))
	out := make([]string, 0, len(members))
	for _, member := range members {
		status := strings.TrimSpace(member.Status)
		if status == "" {
			continue
		}
		if _, ok := seen[status]; ok {
			continue
		}
		seen[status] = struct{}{}
		out = append(out, status)
	}
	sort.Strings(out)
	return out
}

func memberRoles(members []teamcore.Member) []string {
	seen := make(map[string]struct{}, len(members))
	out := make([]string, 0, len(members))
	for _, member := range members {
		role := strings.TrimSpace(member.Role)
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		out = append(out, role)
	}
	sort.Strings(out)
	return out
}

func filterMembersByStatus(members []teamcore.Member, status string) []teamcore.Member {
	out := make([]teamcore.Member, 0, len(members))
	for _, member := range members {
		if member.Status == status {
			out = append(out, member)
		}
	}
	return out
}

func filterMembers(members []teamcore.Member, status, role, agent string) []teamcore.Member {
	status = strings.TrimSpace(status)
	role = strings.TrimSpace(role)
	agent = strings.TrimSpace(agent)
	if status == "" && role == "" && agent == "" {
		return members
	}
	out := make([]teamcore.Member, 0, len(members))
	agent = strings.ToLower(agent)
	for _, member := range members {
		if status != "" && !strings.EqualFold(strings.TrimSpace(member.Status), status) {
			continue
		}
		if role != "" && !strings.EqualFold(strings.TrimSpace(member.Role), role) {
			continue
		}
		if agent != "" && !strings.Contains(strings.ToLower(strings.TrimSpace(member.AgentID)), agent) {
			continue
		}
		out = append(out, member)
	}
	return out
}

func filterMembersByRole(members []teamcore.Member, role string) []teamcore.Member {
	out := make([]teamcore.Member, 0, len(members))
	for _, member := range members {
		if member.Role == role {
			out = append(out, member)
		}
	}
	return out
}

func countTasksByStatus(tasks []teamcore.Task, status string) int {
	count := 0
	for _, task := range tasks {
		if strings.EqualFold(strings.TrimSpace(task.Status), status) {
			count++
		}
	}
	return count
}

func taskStatusCounts(tasks []teamcore.Task) map[string]int {
	out := map[string]int{
		"open":    0,
		"doing":   0,
		"review":  0,
		"blocked": 0,
		"done":    0,
	}
	for _, task := range tasks {
		status := strings.TrimSpace(task.Status)
		if status == "" {
			status = "open"
		}
		out[status]++
	}
	return out
}

func taskStatuses(tasks []teamcore.Task) []string {
	seen := make(map[string]struct{}, len(tasks))
	out := make([]string, 0, len(tasks))
	for _, task := range tasks {
		status := strings.TrimSpace(task.Status)
		if status == "" {
			continue
		}
		if _, ok := seen[status]; ok {
			continue
		}
		seen[status] = struct{}{}
		out = append(out, status)
	}
	sort.Strings(out)
	return out
}

func taskAssignees(tasks []teamcore.Task) []string {
	seen := make(map[string]struct{}, len(tasks))
	out := make([]string, 0, len(tasks))
	for _, task := range tasks {
		for _, assignee := range task.Assignees {
			assignee = strings.TrimSpace(assignee)
			if assignee == "" {
				continue
			}
			if _, ok := seen[assignee]; ok {
				continue
			}
			seen[assignee] = struct{}{}
			out = append(out, assignee)
		}
	}
	sort.Strings(out)
	return out
}

func taskLabels(tasks []teamcore.Task) []string {
	seen := make(map[string]struct{}, len(tasks))
	out := make([]string, 0, len(tasks))
	for _, task := range tasks {
		for _, label := range task.Labels {
			label = strings.TrimSpace(label)
			if label == "" {
				continue
			}
			if _, ok := seen[label]; ok {
				continue
			}
			seen[label] = struct{}{}
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

func countTasksByChannel(tasks []teamcore.Task, channelID string) int {
	channelID = normalizeTeamChannel(channelID)
	if channelID == "" {
		return 0
	}
	count := 0
	for _, task := range tasks {
		if normalizeTeamChannel(task.ChannelID) == channelID {
			count++
		}
	}
	return count
}

func filterTasks(tasks []teamcore.Task, status, assignee, label, channelID string) []teamcore.Task {
	status = strings.TrimSpace(status)
	assignee = strings.TrimSpace(assignee)
	label = strings.TrimSpace(label)
	channelID = normalizeTeamChannel(channelID)
	if status == "" && assignee == "" && label == "" && channelID == "" {
		return tasks
	}
	out := make([]teamcore.Task, 0, len(tasks))
	for _, task := range tasks {
		if status != "" && !strings.EqualFold(strings.TrimSpace(task.Status), status) {
			continue
		}
		if channelID != "" && normalizeTeamChannel(task.ChannelID) != channelID {
			continue
		}
		if assignee != "" && !containsFolded(task.Assignees, assignee) {
			continue
		}
		if label != "" && !containsFolded(task.Labels, label) {
			continue
		}
		out = append(out, task)
	}
	return out
}

func countArtifactsByChannel(artifacts []teamcore.Artifact, channelID string) int {
	channelID = normalizeTeamChannel(channelID)
	if channelID == "" {
		return 0
	}
	count := 0
	for _, artifact := range artifacts {
		if normalizeTeamChannel(artifact.ChannelID) == channelID {
			count++
		}
	}
	return count
}

func containsFolded(values []string, needle string) bool {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return false
	}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), needle) {
			return true
		}
	}
	return false
}

func preferredTaskCommentChannel(task teamcore.Task, channels []teamcore.ChannelSummary) string {
	taskChannel := normalizeTeamChannel(task.ChannelID)
	if taskChannel != "" {
		if len(channels) == 0 {
			return taskChannel
		}
		for _, channel := range channels {
			if normalizeTeamChannel(channel.ChannelID) == taskChannel {
				return channel.ChannelID
			}
		}
	}
	for _, channel := range channels {
		if channel.ChannelID == "main" {
			return "main"
		}
	}
	if len(channels) > 0 {
		return channels[0].ChannelID
	}
	return "main"
}

func blankDash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func channelStateLabel(hidden bool) string {
	if hidden {
		return "hidden"
	}
	return "active"
}

func parseCSVStrings(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func teamFormBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

func parseOptionalStructuredData(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		return nil, nil
	}
	return payload, nil
}

func loadTeamMember(store *teamcore.Store, teamID, agentID string) (teamcore.Member, error) {
	members, err := store.LoadMembersCtx(context.Background(), teamID)
	if err != nil {
		return teamcore.Member{}, err
	}
	agentID = strings.TrimSpace(agentID)
	for _, member := range members {
		if member.AgentID == agentID {
			return member, nil
		}
	}
	return teamcore.Member{}, os.ErrNotExist
}

func policyHistoryMetadata(before, after teamcore.Policy) map[string]any {
	return map[string]any{
		"message_roles_before":     before.MessageRoles,
		"message_roles_after":      after.MessageRoles,
		"task_roles_before":        before.TaskRoles,
		"task_roles_after":         after.TaskRoles,
		"system_note_roles_before": before.SystemNoteRoles,
		"system_note_roles_after":  after.SystemNoteRoles,
		"require_signature_before": before.RequireSignature,
		"require_signature_after":  after.RequireSignature,
		"diff_summary":             "消息角色/任务角色/系统说明角色已更新",
	}
}

func memberHistoryMetadata(before, after teamcore.Member) map[string]any {
	return map[string]any{
		"agent_id":      after.AgentID,
		"role_before":   strings.TrimSpace(before.Role),
		"role_after":    strings.TrimSpace(after.Role),
		"status_before": strings.TrimSpace(before.Status),
		"status_after":  strings.TrimSpace(after.Status),
		"origin_before": strings.TrimSpace(before.OriginPublicKey),
		"origin_after":  strings.TrimSpace(after.OriginPublicKey),
		"parent_before": strings.TrimSpace(before.ParentPublicKey),
		"parent_after":  strings.TrimSpace(after.ParentPublicKey),
		"diff_summary":  "成员角色/状态已更新",
	}
}

func taskHistoryMetadata(before, after teamcore.Task) map[string]any {
	return map[string]any{
		"channel_before":   strings.TrimSpace(before.ChannelID),
		"channel_after":    strings.TrimSpace(after.ChannelID),
		"title_before":     strings.TrimSpace(before.Title),
		"title_after":      strings.TrimSpace(after.Title),
		"status_before":    strings.TrimSpace(before.Status),
		"status_after":     strings.TrimSpace(after.Status),
		"priority_before":  strings.TrimSpace(before.Priority),
		"priority_after":   strings.TrimSpace(after.Priority),
		"due_before":       before.DueAt,
		"due_after":        after.DueAt,
		"assignees_before": before.Assignees,
		"assignees_after":  after.Assignees,
		"labels_before":    before.Labels,
		"labels_after":     after.Labels,
		"diff_summary":     "任务频道/标题/状态/优先级/截止时间/指派/标签已更新",
	}
}

func artifactHistoryMetadata(before, after teamcore.Artifact) map[string]any {
	return map[string]any{
		"title_before":   strings.TrimSpace(before.Title),
		"title_after":    strings.TrimSpace(after.Title),
		"kind_before":    strings.TrimSpace(before.Kind),
		"kind_after":     strings.TrimSpace(after.Kind),
		"channel_before": strings.TrimSpace(before.ChannelID),
		"channel_after":  strings.TrimSpace(after.ChannelID),
		"task_before":    strings.TrimSpace(before.TaskID),
		"task_after":     strings.TrimSpace(after.TaskID),
		"labels_before":  before.Labels,
		"labels_after":   after.Labels,
		"diff_summary":   "产物标题/类型/频道/任务/标签已更新",
	}
}

func channelHistoryMetadata(before, after teamcore.Channel) map[string]any {
	return map[string]any{
		"title_before":       strings.TrimSpace(before.Title),
		"title_after":        strings.TrimSpace(after.Title),
		"description_before": strings.TrimSpace(before.Description),
		"description_after":  strings.TrimSpace(after.Description),
		"hidden_before":      before.Hidden,
		"hidden_after":       after.Hidden,
		"diff_summary":       "频道标题/说明/隐藏状态已更新",
	}
}

func labeledTeamFilter(label, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return label + "：" + value
}

func appliedTeamFilters(values ...string) []string {
	filters := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		filters = append(filters, value)
	}
	return filters
}

func countArtifactsByKind(artifacts []teamcore.Artifact, kind string) int {
	count := 0
	for _, artifact := range artifacts {
		if strings.EqualFold(strings.TrimSpace(artifact.Kind), kind) {
			count++
		}
	}
	return count
}

func upsertTeamMemberCtx(ctx context.Context, store *teamcore.Store, teamID string, member teamcore.Member) error {
	member.AgentID = strings.TrimSpace(member.AgentID)
	if member.AgentID == "" {
		return errors.New("empty agent_id")
	}
	members, err := store.LoadMembersCtx(ctx, teamID)
	if err != nil {
		return err
	}
	updated := false
	for i := range members {
		if members[i].AgentID != member.AgentID {
			continue
		}
		if strings.TrimSpace(member.OriginPublicKey) != "" {
			members[i].OriginPublicKey = strings.TrimSpace(member.OriginPublicKey)
		}
		if strings.TrimSpace(member.ParentPublicKey) != "" {
			members[i].ParentPublicKey = strings.TrimSpace(member.ParentPublicKey)
		}
		if strings.TrimSpace(member.Role) != "" {
			members[i].Role = member.Role
		}
		if strings.TrimSpace(member.Status) != "" {
			members[i].Status = member.Status
		}
		updated = true
		break
	}
	if !updated {
		member.JoinedAt = time.Now().UTC()
		members = append(members, member)
	}
	return store.SaveMembersCtx(ctx, teamID, members)
}

func applyTeamMemberActionCtx(ctx context.Context, store *teamcore.Store, teamID, agentID, action string) (teamcore.Member, string, map[string]any, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return teamcore.Member{}, "", nil, errors.New("empty agent_id")
	}
	nextStatus, summary, err := normalizeMemberAction(action)
	if err != nil {
		return teamcore.Member{}, "", nil, err
	}
	members, err := store.LoadMembersCtx(ctx, teamID)
	if err != nil {
		return teamcore.Member{}, "", nil, err
	}
	for i := range members {
		if members[i].AgentID != agentID {
			continue
		}
		before := members[i].Status
		members[i].Status = nextStatus
		if err := store.SaveMembersCtx(ctx, teamID, members); err != nil {
			return teamcore.Member{}, "", nil, err
		}
		return members[i], summary, map[string]any{
			"agent_id":      members[i].AgentID,
			"role":          members[i].Role,
			"status_before": before,
			"status_after":  nextStatus,
		}, nil
	}
	return teamcore.Member{}, "", nil, os.ErrNotExist
}

func normalizeMemberAction(value string) (string, string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "approve", "activate", "active":
		return "active", "审批通过 Team 成员", nil
	case "mute", "muted":
		return "muted", "静音 Team 成员", nil
	case "remove", "removed":
		return "removed", "移除 Team 成员", nil
	case "pending":
		return "pending", "重新标记 Team 成员为待审批", nil
	default:
		return "", "", errors.New("unknown team member action")
	}
}

type historyActor struct {
	AgentID         string
	OriginPublicKey string
	ParentPublicKey string
	Source          string
}

func appendTeamHistory(store *teamcore.Store, actor historyActor, teamID, scope, action, subjectID, summary string, metadata map[string]any) error {
	return appendTeamHistoryCtx(context.Background(), store, actor, teamID, scope, action, subjectID, summary, metadata)
}

func appendTeamHistoryCtx(ctx context.Context, store *teamcore.Store, actor historyActor, teamID, scope, action, subjectID, summary string, metadata map[string]any) error {
	return store.AppendHistoryCtx(ctx, teamID, teamcore.ChangeEvent{
		Scope:                scope,
		Action:               action,
		SubjectID:            subjectID,
		Summary:              summary,
		ActorAgentID:         strings.TrimSpace(actor.AgentID),
		ActorOriginPublicKey: strings.TrimSpace(actor.OriginPublicKey),
		ActorParentPublicKey: strings.TrimSpace(actor.ParentPublicKey),
		Source:               strings.TrimSpace(actor.Source),
		Diff:                 buildTeamHistoryDiff(scope, metadata),
		Metadata:             metadata,
		CreatedAt:            time.Now().UTC(),
	})
}

func buildTeamHistoryDiff(scope string, metadata map[string]any) map[string]teamcore.FieldDiff {
	if len(metadata) == 0 {
		return nil
	}
	var pairs map[string][2]string
	switch strings.TrimSpace(scope) {
	case "policy":
		pairs = map[string][2]string{
			"message_roles":     {"message_roles_before", "message_roles_after"},
			"task_roles":        {"task_roles_before", "task_roles_after"},
			"system_note_roles": {"system_note_roles_before", "system_note_roles_after"},
			"require_signature": {"require_signature_before", "require_signature_after"},
		}
	case "member":
		pairs = map[string][2]string{
			"role":   {"role_before", "role_after"},
			"status": {"status_before", "status_after"},
			"origin": {"origin_before", "origin_after"},
			"parent": {"parent_before", "parent_after"},
		}
	case "task":
		pairs = map[string][2]string{
			"channel":   {"channel_before", "channel_after"},
			"title":     {"title_before", "title_after"},
			"status":    {"status_before", "status_after"},
			"priority":  {"priority_before", "priority_after"},
			"due":       {"due_before", "due_after"},
			"assignees": {"assignees_before", "assignees_after"},
			"labels":    {"labels_before", "labels_after"},
		}
	case "artifact":
		pairs = map[string][2]string{
			"title":   {"title_before", "title_after"},
			"kind":    {"kind_before", "kind_after"},
			"channel": {"channel_before", "channel_after"},
			"task":    {"task_before", "task_after"},
			"labels":  {"labels_before", "labels_after"},
		}
	case "channel":
		pairs = map[string][2]string{
			"title":       {"title_before", "title_after"},
			"description": {"description_before", "description_after"},
			"hidden":      {"hidden_before", "hidden_after"},
		}
	default:
		return nil
	}
	diff := make(map[string]teamcore.FieldDiff, len(pairs))
	for name, pair := range pairs {
		before, beforeOK := metadata[pair[0]]
		after, afterOK := metadata[pair[1]]
		if !beforeOK && !afterOK {
			continue
		}
		diff[name] = teamcore.FieldDiff{Before: before, After: after}
	}
	if len(diff) == 0 {
		return nil
	}
	return diff
}

func teamRequestTrusted(r *http.Request) bool {
	addr := teamClientIP(r)
	if !addr.IsValid() {
		return false
	}
	return addr.IsLoopback() || addr.IsPrivate()
}

func requireTeamAction(store *teamcore.Store, teamID, actorAgentID, action string) error {
	actorAgentID = strings.TrimSpace(actorAgentID)
	info, err := store.LoadTeamCtx(context.Background(), teamID)
	if err != nil {
		return err
	}
	fallbackRole := ""
	if actorAgentID == "" {
		actorAgentID = strings.TrimSpace(info.OwnerAgentID)
		if actorAgentID == "" {
			fallbackRole = "owner"
		}
	}
	role := fallbackRole
	if actorAgentID != "" {
		role, err = teamActorRole(store, teamID, actorAgentID, info)
		if err != nil {
			return err
		}
	}
	policy, err := store.LoadPolicyCtx(context.Background(), teamID)
	if err != nil {
		return err
	}
	if !policy.Allows(action, role) {
		return fmt.Errorf("team policy denied action %q for role %q", action, role)
	}
	return nil
}

func requireTeamConflictAction(store *teamcore.Store, teamID, actorAgentID string) error {
	return requireTeamAction(store, teamID, actorAgentID, "sync.conflict.resolve")
}

func teamActorRole(store *teamcore.Store, teamID, actorAgentID string, info teamcore.Info) (string, error) {
	actorAgentID = strings.TrimSpace(actorAgentID)
	if actorAgentID == "" {
		return "", errors.New("empty actor_agent_id")
	}
	member, err := loadTeamMember(store, teamID, actorAgentID)
	if err == nil {
		return member.Role, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if actorAgentID == strings.TrimSpace(info.OwnerAgentID) {
		return "owner", nil
	}
	return "", fmt.Errorf("team actor %q not found", actorAgentID)
}

func teamClientIP(r *http.Request) netip.Addr {
	if r == nil {
		return netip.Addr{}
	}
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
		if addr, err := netip.ParseAddr(strings.TrimSpace(forwarded)); err == nil {
			return addr.Unmap()
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		if addr, err := netip.ParseAddr(strings.TrimSpace(host)); err == nil {
			return addr.Unmap()
		}
	}
	if addr, err := netip.ParseAddr(strings.TrimSpace(r.RemoteAddr)); err == nil {
		return addr.Unmap()
	}
	return netip.Addr{}
}
