package haonewsteam

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	teamcore "hao.news/internal/haonews/team"
	newsplugin "hao.news/internal/plugins/haonews"
)

func handleTeamIndex(app *newsplugin.App, store *teamcore.Store, w http.ResponseWriter, r *http.Request) {
	teams, err := store.ListTeams()
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
	data := teamIndexPageData{
		Project:    app.ProjectName(),
		Version:    app.VersionString(),
		PageNav:    app.PageNav("/teams"),
		NodeStatus: app.NodeStatus(index),
		Now:        time.Now(),
		Teams:      teams,
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "团队数", Value: formatTeamCount(len(teams))},
			{Label: "成员总数", Value: formatTeamCount(memberCount)},
			{Label: "最近更新", Value: latestTeamValue(teams)},
		},
	}
	if err := app.Templates().ExecuteTemplate(w, "team.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleTeam(app *newsplugin.App, store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	info, err := store.LoadTeam(teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	members, err := store.LoadMembers(teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	policy, err := store.LoadPolicy(teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	messages, err := store.LoadMessages(teamID, "main", 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tasks, err := store.LoadTasks(teamID, 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	artifacts, err := store.LoadArtifacts(teamID, 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	history, err := store.LoadHistory(teamID, 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	channels, err := store.ListChannels(teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := teamPageData{
		Project:        app.ProjectName(),
		Version:        app.VersionString(),
		PageNav:        app.PageNav("/teams"),
		NodeStatus:     app.NodeStatus(index),
		Now:            time.Now(),
		Team:           info,
		Policy:         policy,
		Members:        members,
		PendingMembers: filterMembersByStatus(members, "pending"),
		Messages:       messages,
		Tasks:          tasks,
		Channels:       channels,
		Artifacts:      artifacts,
		History:        history,
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "成员", Value: formatTeamCount(countMembersByStatus(members, "active"))},
			{Label: "频道", Value: formatTeamCount(len(channels))},
			{Label: "任务", Value: formatTeamCount(len(tasks))},
			{Label: "产物", Value: formatTeamCount(len(artifacts))},
			{Label: "待审批", Value: formatTeamCount(countMembersByStatus(members, "pending"))},
		},
	}
	if err := app.Templates().ExecuteTemplate(w, "team_detail.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleTeamHistory(app *newsplugin.App, store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	info, err := store.LoadTeam(teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	history, err := store.LoadHistory(teamID, 200)
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
		Scopes:       scopes,
		Sources:      sources,
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

func handleTeamChannel(app *newsplugin.App, store *teamcore.Store, teamID, channelID string, w http.ResponseWriter, r *http.Request) {
	info, err := store.LoadTeam(teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	channels, err := store.ListChannels(teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	channelID = normalizeTeamChannel(channelID)
	var current teamcore.ChannelSummary
	found := false
	for _, channel := range channels {
		if channel.ChannelID == channelID {
			current = channel
			found = true
			break
		}
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	messages, err := store.LoadChannelMessages(teamID, channelID, 40)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := teamChannelPageData{
		Project:    app.ProjectName(),
		Version:    app.VersionString(),
		PageNav:    app.PageNav("/teams"),
		NodeStatus: app.NodeStatus(index),
		Now:        time.Now(),
		Team:       info,
		Channel:    current,
		ChannelID:  channelID,
		Channels:   channels,
		Messages:   messages,
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "频道", Value: current.Title},
			{Label: "消息", Value: formatTeamCount(len(messages))},
			{Label: "可见性", Value: info.Visibility},
			{Label: "状态", Value: channelStateLabel(current.Hidden)},
		},
	}
	if err := app.Templates().ExecuteTemplate(w, "team_channel.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleTeamChannelCreate(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team channel update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	channel := teamcore.Channel{
		ChannelID:   strings.TrimSpace(r.FormValue("channel_id")),
		Title:       strings.TrimSpace(r.FormValue("title")),
		Description: strings.TrimSpace(r.FormValue("description")),
		Hidden:      false,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := store.SaveChannel(teamID, channel); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	channel, _ = store.LoadChannel(teamID, channel.ChannelID)
	_ = appendTeamHistory(store, historyActor{Source: "page"}, teamID, "channel", "create", channel.ChannelID, "创建 Team Channel", channelHistoryMetadata(teamcore.Channel{}, channel))
	http.Redirect(w, r, "/teams/"+teamID+"/channels/"+channel.ChannelID, http.StatusSeeOther)
}

func handleTeamChannelUpdate(store *teamcore.Store, teamID, channelID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team channel update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	before, err := store.LoadChannel(teamID, channelID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	updated := before
	updated.Title = strings.TrimSpace(r.FormValue("title"))
	updated.Description = strings.TrimSpace(r.FormValue("description"))
	updated.Hidden = r.FormValue("hidden") == "true" || r.FormValue("hidden") == "on"
	updated.UpdatedAt = time.Now().UTC()
	if err := store.SaveChannel(teamID, updated); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	after, _ := store.LoadChannel(teamID, channelID)
	_ = appendTeamHistory(store, historyActor{Source: "page"}, teamID, "channel", "update", channelID, "更新 Team Channel", channelHistoryMetadata(before, after))
	http.Redirect(w, r, "/teams/"+teamID+"/channels/"+channelID, http.StatusSeeOther)
}

func handleTeamChannelHide(store *teamcore.Store, teamID, channelID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team channel update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	before, err := store.LoadChannel(teamID, channelID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := store.HideChannel(teamID, channelID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	after, _ := store.LoadChannel(teamID, channelID)
	_ = appendTeamHistory(store, historyActor{Source: "page"}, teamID, "channel", "hide", channelID, "隐藏 Team Channel", channelHistoryMetadata(before, after))
	http.Redirect(w, r, "/teams/"+teamID, http.StatusSeeOther)
}

func handleTeamTasks(app *newsplugin.App, store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	info, err := store.LoadTeam(teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	tasks, err := store.LoadTasks(teamID, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := teamTasksPageData{
		Project:    app.ProjectName(),
		Version:    app.VersionString(),
		PageNav:    app.PageNav("/teams"),
		NodeStatus: app.NodeStatus(index),
		Now:        time.Now(),
		Team:       info,
		Tasks:      tasks,
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "任务", Value: formatTeamCount(len(tasks))},
			{Label: "进行中", Value: formatTeamCount(countTasksByStatus(tasks, "doing"))},
			{Label: "已完成", Value: formatTeamCount(countTasksByStatus(tasks, "done"))},
		},
	}
	if err := app.Templates().ExecuteTemplate(w, "team_tasks.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleTeamTask(app *newsplugin.App, store *teamcore.Store, teamID, taskID string, w http.ResponseWriter, r *http.Request) {
	info, err := store.LoadTeam(teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	task, err := store.LoadTask(teamID, taskID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	tasks, err := store.LoadTasks(teamID, 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	messages, err := store.LoadTaskMessages(teamID, taskID, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := teamTaskPageData{
		Project:    app.ProjectName(),
		Version:    app.VersionString(),
		PageNav:    app.PageNav("/teams"),
		NodeStatus: app.NodeStatus(index),
		Now:        time.Now(),
		Team:       info,
		Task:       task,
		Tasks:      tasks,
		Messages:   messages,
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "状态", Value: task.Status},
			{Label: "优先级", Value: blankDash(task.Priority)},
			{Label: "评论", Value: formatTeamCount(len(messages))},
		},
	}
	if err := app.Templates().ExecuteTemplate(w, "team_task.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleTeamTaskCreate(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team task update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload := teamcore.Task{
		TaskID:          strings.TrimSpace(r.FormValue("task_id")),
		Title:           strings.TrimSpace(r.FormValue("title")),
		Description:     strings.TrimSpace(r.FormValue("description")),
		CreatedBy:       strings.TrimSpace(r.FormValue("created_by")),
		Assignees:       parseCSVStrings(r.FormValue("assignees")),
		Status:          strings.TrimSpace(r.FormValue("status")),
		Priority:        strings.TrimSpace(r.FormValue("priority")),
		Labels:          parseCSVStrings(r.FormValue("labels")),
		OriginPublicKey: strings.TrimSpace(r.FormValue("origin_public_key")),
		ParentPublicKey: strings.TrimSpace(r.FormValue("parent_public_key")),
		CreatedAt:       time.Now().UTC(),
	}
	payload.UpdatedAt = payload.CreatedAt
	if err := store.AppendTask(teamID, payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	targetID := payload.TaskID
	if targetID == "" {
		tasks, err := store.LoadTasks(teamID, 1)
		if err == nil && len(tasks) > 0 {
			targetID = tasks[0].TaskID
		}
	}
	_ = appendTeamHistory(store, historyActor{
		AgentID:         payload.CreatedBy,
		OriginPublicKey: payload.OriginPublicKey,
		ParentPublicKey: payload.ParentPublicKey,
		Source:          "page",
	}, teamID, "task", "create", targetID, "创建 Team Task", taskHistoryMetadata(teamcore.Task{}, payload))
	http.Redirect(w, r, "/teams/"+teamID+"/tasks/"+targetID, http.StatusSeeOther)
}

func handleTeamTaskUpdate(store *teamcore.Store, teamID, taskID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team task update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	existing, err := store.LoadTask(teamID, taskID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	updated := existing
	updated.Title = strings.TrimSpace(r.FormValue("title"))
	updated.Description = strings.TrimSpace(r.FormValue("description"))
	updated.Assignees = parseCSVStrings(r.FormValue("assignees"))
	updated.Status = strings.TrimSpace(r.FormValue("status"))
	updated.Priority = strings.TrimSpace(r.FormValue("priority"))
	updated.Labels = parseCSVStrings(r.FormValue("labels"))
	if updated.Status == "done" && updated.ClosedAt.IsZero() {
		updated.ClosedAt = time.Now().UTC()
	}
	if updated.Status != "done" {
		updated.ClosedAt = time.Time{}
	}
	updated.UpdatedAt = time.Now().UTC()
	if err := store.SaveTask(teamID, updated); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = appendTeamHistory(store, historyActor{
		AgentID:         updated.CreatedBy,
		OriginPublicKey: updated.OriginPublicKey,
		ParentPublicKey: updated.ParentPublicKey,
		Source:          "page",
	}, teamID, "task", "update", taskID, "更新 Team Task", taskHistoryMetadata(existing, updated))
	http.Redirect(w, r, "/teams/"+teamID+"/tasks/"+taskID, http.StatusSeeOther)
}

func handleTeamTaskDelete(store *teamcore.Store, teamID, taskID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team task update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := store.DeleteTask(teamID, taskID); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = appendTeamHistory(store, historyActor{Source: "page"}, teamID, "task", "delete", taskID, "删除 Team Task", map[string]any{
		"diff_summary": "删除任务",
	})
	http.Redirect(w, r, "/teams/"+teamID+"/tasks", http.StatusSeeOther)
}

func handleTeamArtifacts(app *newsplugin.App, store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	info, err := store.LoadTeam(teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	artifacts, err := store.LoadArtifacts(teamID, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := teamArtifactsPageData{
		Project:    app.ProjectName(),
		Version:    app.VersionString(),
		PageNav:    app.PageNav("/teams"),
		NodeStatus: app.NodeStatus(index),
		Now:        time.Now(),
		Team:       info,
		Artifacts:  artifacts,
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "产物", Value: formatTeamCount(len(artifacts))},
			{Label: "Markdown", Value: formatTeamCount(countArtifactsByKind(artifacts, "markdown"))},
			{Label: "链接", Value: formatTeamCount(countArtifactsByKind(artifacts, "link"))},
		},
	}
	if err := app.Templates().ExecuteTemplate(w, "team_artifacts.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleTeamArtifact(app *newsplugin.App, store *teamcore.Store, teamID, artifactID string, w http.ResponseWriter, r *http.Request) {
	info, err := store.LoadTeam(teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	artifact, err := store.LoadArtifact(teamID, artifactID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	artifacts, err := store.LoadArtifacts(teamID, 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := teamArtifactPageData{
		Project:    app.ProjectName(),
		Version:    app.VersionString(),
		PageNav:    app.PageNav("/teams"),
		NodeStatus: app.NodeStatus(index),
		Now:        time.Now(),
		Team:       info,
		Artifact:   artifact,
		Artifacts:  artifacts,
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "类型", Value: artifact.Kind},
			{Label: "频道", Value: artifact.ChannelID},
			{Label: "标签", Value: formatTeamCount(len(artifact.Labels))},
		},
	}
	if err := app.Templates().ExecuteTemplate(w, "team_artifact.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleTeamArtifactCreate(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team artifact update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload := teamcore.Artifact{
		ArtifactID:      strings.TrimSpace(r.FormValue("artifact_id")),
		ChannelID:       strings.TrimSpace(r.FormValue("channel_id")),
		Title:           strings.TrimSpace(r.FormValue("title")),
		Kind:            strings.TrimSpace(r.FormValue("kind")),
		Summary:         strings.TrimSpace(r.FormValue("summary")),
		Content:         strings.TrimSpace(r.FormValue("content")),
		LinkURL:         strings.TrimSpace(r.FormValue("link_url")),
		CreatedBy:       strings.TrimSpace(r.FormValue("created_by")),
		OriginPublicKey: strings.TrimSpace(r.FormValue("origin_public_key")),
		ParentPublicKey: strings.TrimSpace(r.FormValue("parent_public_key")),
		Labels:          parseCSVStrings(r.FormValue("labels")),
		CreatedAt:       time.Now().UTC(),
	}
	payload.UpdatedAt = payload.CreatedAt
	if err := store.AppendArtifact(teamID, payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	targetID := payload.ArtifactID
	if targetID == "" {
		artifact, err := store.LoadArtifacts(teamID, 1)
		if err == nil && len(artifact) > 0 {
			targetID = artifact[0].ArtifactID
		}
	}
	_ = appendTeamHistory(store, historyActor{
		AgentID:         payload.CreatedBy,
		OriginPublicKey: payload.OriginPublicKey,
		ParentPublicKey: payload.ParentPublicKey,
		Source:          "page",
	}, teamID, "artifact", "create", targetID, "创建 Team Artifact", artifactHistoryMetadata(teamcore.Artifact{}, payload))
	http.Redirect(w, r, "/teams/"+teamID+"/artifacts/"+targetID, http.StatusSeeOther)
}

func handleTeamArtifactUpdate(store *teamcore.Store, teamID, artifactID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team artifact update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	existing, err := store.LoadArtifact(teamID, artifactID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	updated := existing
	updated.ChannelID = strings.TrimSpace(r.FormValue("channel_id"))
	updated.Title = strings.TrimSpace(r.FormValue("title"))
	updated.Kind = strings.TrimSpace(r.FormValue("kind"))
	updated.Summary = strings.TrimSpace(r.FormValue("summary"))
	updated.Content = strings.TrimSpace(r.FormValue("content"))
	updated.LinkURL = strings.TrimSpace(r.FormValue("link_url"))
	updated.Labels = parseCSVStrings(r.FormValue("labels"))
	updated.UpdatedAt = time.Now().UTC()
	if err := store.SaveArtifact(teamID, updated); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = appendTeamHistory(store, historyActor{
		AgentID:         updated.CreatedBy,
		OriginPublicKey: updated.OriginPublicKey,
		ParentPublicKey: updated.ParentPublicKey,
		Source:          "page",
	}, teamID, "artifact", "update", artifactID, "更新 Team Artifact", artifactHistoryMetadata(existing, updated))
	http.Redirect(w, r, "/teams/"+teamID+"/artifacts/"+artifactID, http.StatusSeeOther)
}

func handleTeamArtifactDelete(store *teamcore.Store, teamID, artifactID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team artifact update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := store.DeleteArtifact(teamID, artifactID); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = appendTeamHistory(store, historyActor{Source: "page"}, teamID, "artifact", "delete", artifactID, "删除 Team Artifact", map[string]any{
		"diff_summary": "删除产物",
	})
	http.Redirect(w, r, "/teams/"+teamID+"/artifacts", http.StatusSeeOther)
}

func handleAPITeamIndex(store *teamcore.Store, w http.ResponseWriter, _ *http.Request) {
	teams, err := store.ListTeams()
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
	info, err := store.LoadTeam(teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	members, err := store.LoadMembers(teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	policy, err := store.LoadPolicy(teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":        "team-detail",
		"team_id":      info.TeamID,
		"team":         info,
		"policy":       policy,
		"member_count": len(members),
		"members":      members,
	})
}

func handleAPITeamMembers(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		handleAPITeamMemberUpdate(store, teamID, w, r)
		return
	}
	info, err := store.LoadTeam(teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	members, err := store.LoadMembers(teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":        "team-members",
		"team_id":      info.TeamID,
		"member_count": len(members),
		"members":      members,
		"counts": map[string]int{
			"active":     countMembersByStatus(members, "active"),
			"pending":    countMembersByStatus(members, "pending"),
			"muted":      countMembersByStatus(members, "muted"),
			"removed":    countMembersByStatus(members, "removed"),
			"owner":      countMembersByRole(members, "owner"),
			"maintainer": countMembersByRole(members, "maintainer"),
			"member":     countMembersByRole(members, "member"),
			"observer":   countMembersByRole(members, "observer"),
		},
	})
}

func handleAPITeamHistory(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	info, err := store.LoadTeam(teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	history, err := store.LoadHistory(teamID, 100)
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
	info, err := store.LoadTeam(teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	policy, err := store.LoadPolicy(teamID)
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
	info, err := store.LoadTeam(teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	channelID := strings.TrimSpace(r.URL.Query().Get("channel"))
	messages, err := store.LoadMessages(teamID, channelID, 50)
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
		"message_count": len(messages),
		"messages":      messages,
	})
}

func handleAPITeamChannels(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		handleAPITeamChannelCreate(store, teamID, w, r)
		return
	}
	info, err := store.LoadTeam(teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	channels, err := store.ListChannels(teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":         "team-channels",
		"team_id":       info.TeamID,
		"channel_count": len(channels),
		"channels":      channels,
	})
}

func handleAPITeamChannel(store *teamcore.Store, teamID, channelID string, w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		handleAPITeamChannelUpdate(store, teamID, channelID, w, r)
		return
	}
	if r.Method == http.MethodDelete {
		handleAPITeamChannelDelete(store, teamID, channelID, w, r)
		return
	}
	info, err := store.LoadTeam(teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	channel, err := store.LoadChannel(teamID, channelID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	messages, err := store.LoadMessages(teamID, channelID, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":         "team-channel",
		"team_id":       info.TeamID,
		"channel":       channel,
		"message_count": len(messages),
	})
}

func handleAPITeamChannelMessages(store *teamcore.Store, teamID, channelID string, w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		handleAPITeamChannelMessageCreate(store, teamID, channelID, w, r)
		return
	}
	info, err := store.LoadTeam(teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	channelID = normalizeTeamChannel(channelID)
	if channelID == "" {
		channelID = "main"
	}
	messages, err := store.LoadMessages(teamID, channelID, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":         "team-channel-messages",
		"team_id":       info.TeamID,
		"channel_id":    channelID,
		"message_count": len(messages),
		"messages":      messages,
	})
}

func handleTeamChannelMessageCreate(store *teamcore.Store, teamID, channelID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team message create is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	structuredData, err := parseOptionalStructuredData(r.FormValue("structured_data"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	msg := teamcore.Message{
		TeamID:          teamID,
		ChannelID:       channelID,
		AuthorAgentID:   strings.TrimSpace(r.FormValue("author_agent_id")),
		OriginPublicKey: strings.TrimSpace(r.FormValue("origin_public_key")),
		ParentPublicKey: strings.TrimSpace(r.FormValue("parent_public_key")),
		MessageType:     strings.TrimSpace(r.FormValue("message_type")),
		Content:         strings.TrimSpace(r.FormValue("content")),
		StructuredData:  structuredData,
		CreatedAt:       time.Now().UTC(),
	}
	if err := store.AppendMessage(teamID, msg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = appendTeamHistory(store, historyActor{
		AgentID:         msg.AuthorAgentID,
		OriginPublicKey: msg.OriginPublicKey,
		ParentPublicKey: msg.ParentPublicKey,
		Source:          "page",
	}, teamID, "message", "create", msg.ChannelID, "发送 TeamMessage", map[string]any{
		"channel_id":   msg.ChannelID,
		"message_type": blankDash(msg.MessageType),
		"author_agent": msg.AuthorAgentID,
		"has_metadata": len(msg.StructuredData) > 0,
	})
	http.Redirect(w, r, "/teams/"+teamID+"/channels/"+channelID, http.StatusSeeOther)
}

func handleAPITeamChannelMessageCreate(store *teamcore.Store, teamID, channelID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team message create is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	var payload teamcore.Message
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload.TeamID = teamID
	payload.ChannelID = channelID
	payload.CreatedAt = time.Now().UTC()
	if err := store.AppendMessage(teamID, payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = appendTeamHistory(store, historyActor{
		AgentID:         payload.AuthorAgentID,
		OriginPublicKey: payload.OriginPublicKey,
		ParentPublicKey: payload.ParentPublicKey,
		Source:          "api",
	}, teamID, "message", "create", channelID, "发送 TeamMessage", map[string]any{
		"channel_id":   channelID,
		"message_type": blankDash(payload.MessageType),
		"author_agent": payload.AuthorAgentID,
		"has_metadata": len(payload.StructuredData) > 0,
	})
	newsplugin.WriteJSON(w, http.StatusCreated, map[string]any{
		"scope":   "team-message",
		"team_id": teamID,
		"message": payload,
	})
}

func handleAPITeamChannelCreate(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team channel update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	var payload teamcore.Channel
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload.CreatedAt = time.Now().UTC()
	payload.UpdatedAt = payload.CreatedAt
	if err := store.SaveChannel(teamID, payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	channel, err := store.LoadChannel(teamID, payload.ChannelID)
	if err != nil {
		channel = payload
	}
	_ = appendTeamHistory(store, historyActor{Source: "api"}, teamID, "channel", "create", channel.ChannelID, "创建 Team Channel", channelHistoryMetadata(teamcore.Channel{}, channel))
	newsplugin.WriteJSON(w, http.StatusCreated, map[string]any{
		"scope":   "team-channel",
		"team_id": teamID,
		"channel": channel,
	})
}

func handleAPITeamChannelUpdate(store *teamcore.Store, teamID, channelID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team channel update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	before, err := store.LoadChannel(teamID, channelID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var payload struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Hidden      *bool  `json:"hidden"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	channel := before
	if strings.TrimSpace(payload.Title) != "" {
		channel.Title = strings.TrimSpace(payload.Title)
	}
	channel.Description = strings.TrimSpace(payload.Description)
	if payload.Hidden != nil {
		channel.Hidden = *payload.Hidden
	}
	channel.CreatedAt = before.CreatedAt
	channel.UpdatedAt = time.Now().UTC()
	if err := store.SaveChannel(teamID, channel); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	after, err := store.LoadChannel(teamID, channelID)
	if err != nil {
		after = channel
	}
	_ = appendTeamHistory(store, historyActor{Source: "api"}, teamID, "channel", "update", channelID, "更新 Team Channel", channelHistoryMetadata(before, after))
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":   "team-channel",
		"team_id": teamID,
		"channel": after,
	})
}

func handleAPITeamChannelDelete(store *teamcore.Store, teamID, channelID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team channel update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	before, err := store.LoadChannel(teamID, channelID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := store.HideChannel(teamID, channelID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	after, _ := store.LoadChannel(teamID, channelID)
	_ = appendTeamHistory(store, historyActor{Source: "api"}, teamID, "channel", "hide", channelID, "隐藏 Team Channel", channelHistoryMetadata(before, after))
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":   "team-channel",
		"team_id": teamID,
		"channel": after,
		"deleted": true,
	})
}

func handleAPITeamTasks(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		handleAPITeamTaskCreate(store, teamID, w, r)
		return
	}
	info, err := store.LoadTeam(teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	tasks, err := store.LoadTasks(teamID, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":      "team-tasks",
		"team_id":    info.TeamID,
		"task_count": len(tasks),
		"tasks":      tasks,
	})
}

func handleAPITeamTask(store *teamcore.Store, teamID, taskID string, w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		handleAPITeamTaskUpdate(store, teamID, taskID, w, r)
		return
	}
	if r.Method == http.MethodDelete {
		handleAPITeamTaskDelete(store, teamID, taskID, w, r)
		return
	}
	info, err := store.LoadTeam(teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	task, err := store.LoadTask(teamID, taskID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	messages, err := store.LoadTaskMessages(teamID, taskID, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":         "team-task",
		"team_id":       info.TeamID,
		"task_id":       task.TaskID,
		"task":          task,
		"message_count": len(messages),
		"messages":      messages,
	})
}

func handleAPITeamTaskCreate(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team task update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	var payload teamcore.Task
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload.CreatedAt = time.Now().UTC()
	payload.UpdatedAt = payload.CreatedAt
	if err := store.AppendTask(teamID, payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	task, err := store.LoadTask(teamID, payload.TaskID)
	if err != nil {
		task = payload
	}
	_ = appendTeamHistory(store, historyActor{
		AgentID:         task.CreatedBy,
		OriginPublicKey: task.OriginPublicKey,
		ParentPublicKey: task.ParentPublicKey,
		Source:          "api",
	}, teamID, "task", "create", task.TaskID, "创建 Team Task", taskHistoryMetadata(teamcore.Task{}, task))
	newsplugin.WriteJSON(w, http.StatusCreated, map[string]any{
		"scope":   "team-task",
		"team_id": teamID,
		"task":    task,
	})
}

func handleAPITeamTaskUpdate(store *teamcore.Store, teamID, taskID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team task update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	existing, err := store.LoadTask(teamID, taskID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var payload teamcore.Task
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload.TeamID = teamID
	payload.TaskID = taskID
	if payload.Title == "" {
		payload.Title = existing.Title
	}
	if payload.CreatedBy == "" {
		payload.CreatedBy = existing.CreatedBy
	}
	if payload.OriginPublicKey == "" {
		payload.OriginPublicKey = existing.OriginPublicKey
	}
	if payload.ParentPublicKey == "" {
		payload.ParentPublicKey = existing.ParentPublicKey
	}
	if payload.CreatedAt.IsZero() {
		payload.CreatedAt = existing.CreatedAt
	}
	if payload.Status == "done" && payload.ClosedAt.IsZero() {
		payload.ClosedAt = time.Now().UTC()
	}
	if payload.Status != "done" && payload.Status != "" {
		payload.ClosedAt = time.Time{}
	}
	payload.UpdatedAt = time.Now().UTC()
	if err := store.SaveTask(teamID, payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	task, err := store.LoadTask(teamID, taskID)
	if err != nil {
		task = payload
	}
	_ = appendTeamHistory(store, historyActor{
		AgentID:         task.CreatedBy,
		OriginPublicKey: task.OriginPublicKey,
		ParentPublicKey: task.ParentPublicKey,
		Source:          "api",
	}, teamID, "task", "update", taskID, "更新 Team Task", taskHistoryMetadata(existing, task))
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":   "team-task",
		"team_id": teamID,
		"task":    task,
	})
}

func handleAPITeamTaskDelete(store *teamcore.Store, teamID, taskID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team task update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := store.DeleteTask(teamID, taskID); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = appendTeamHistory(store, historyActor{Source: "api"}, teamID, "task", "delete", taskID, "删除 Team Task", map[string]any{
		"diff_summary": "删除任务",
	})
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":   "team-task",
		"team_id": teamID,
		"task_id": taskID,
		"deleted": true,
	})
}

func handleAPITeamArtifacts(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		handleAPITeamArtifactCreate(store, teamID, w, r)
		return
	}
	info, err := store.LoadTeam(teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	artifacts, err := store.LoadArtifacts(teamID, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":          "team-artifacts",
		"team_id":        info.TeamID,
		"artifact_count": len(artifacts),
		"artifacts":      artifacts,
	})
}

func handleAPITeamArtifact(store *teamcore.Store, teamID, artifactID string, w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		handleAPITeamArtifactUpdate(store, teamID, artifactID, w, r)
		return
	}
	if r.Method == http.MethodDelete {
		handleAPITeamArtifactDelete(store, teamID, artifactID, w, r)
		return
	}
	info, err := store.LoadTeam(teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	artifact, err := store.LoadArtifact(teamID, artifactID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":    "team-artifact",
		"team_id":  info.TeamID,
		"artifact": artifact,
	})
}

func handleTeamPolicyUpdate(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team policy update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	policy := teamcore.Policy{
		MessageRoles:    parseCSVStrings(r.FormValue("message_roles")),
		TaskRoles:       parseCSVStrings(r.FormValue("task_roles")),
		SystemNoteRoles: parseCSVStrings(r.FormValue("system_note_roles")),
		UpdatedAt:       time.Now().UTC(),
	}
	before, err := store.LoadPolicy(teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := store.SavePolicy(teamID, policy); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = appendTeamHistory(store, historyActor{Source: "page"}, teamID, "policy", "update", "team-policy", "更新 Team Policy", policyHistoryMetadata(before, policy))
	http.Redirect(w, r, "/teams/"+teamID, http.StatusSeeOther)
}

func handleTeamMemberUpdate(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team member update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	member := teamcore.Member{
		AgentID:         strings.TrimSpace(r.FormValue("agent_id")),
		OriginPublicKey: strings.TrimSpace(r.FormValue("origin_public_key")),
		ParentPublicKey: strings.TrimSpace(r.FormValue("parent_public_key")),
		Role:            strings.TrimSpace(r.FormValue("role")),
		Status:          strings.TrimSpace(r.FormValue("status")),
	}
	before, _ := loadTeamMember(store, teamID, member.AgentID)
	if err := upsertTeamMember(store, teamID, member); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	after, _ := loadTeamMember(store, teamID, member.AgentID)
	_ = appendTeamHistory(store, historyActor{
		AgentID:         member.AgentID,
		OriginPublicKey: member.OriginPublicKey,
		ParentPublicKey: member.ParentPublicKey,
		Source:          "page",
	}, teamID, "member", "update", member.AgentID, "更新 Team 成员角色或状态", memberHistoryMetadata(before, after))
	http.Redirect(w, r, "/teams/"+teamID, http.StatusSeeOther)
}

func handleTeamMemberAction(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team member action is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	member, summary, metadata, err := applyTeamMemberAction(store, teamID, strings.TrimSpace(r.FormValue("agent_id")), strings.TrimSpace(r.FormValue("action")))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = appendTeamHistory(store, historyActor{
		AgentID:         member.AgentID,
		OriginPublicKey: member.OriginPublicKey,
		ParentPublicKey: member.ParentPublicKey,
		Source:          "page",
	}, teamID, "member", "transition", member.AgentID, summary, metadata)
	http.Redirect(w, r, "/teams/"+teamID, http.StatusSeeOther)
}

func handleAPITeamPolicyUpdate(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team policy update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	var payload teamcore.Policy
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	before, err := store.LoadPolicy(teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	payload.UpdatedAt = time.Now().UTC()
	if err := store.SavePolicy(teamID, payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = appendTeamHistory(store, historyActor{Source: "api"}, teamID, "policy", "update", "team-policy", "更新 Team Policy", policyHistoryMetadata(before, payload))
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":   "team-policy",
		"team_id": teamID,
		"policy":  payload,
	})
}

func handleAPITeamMemberUpdate(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team member update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	var payload teamcore.Member
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	before, _ := loadTeamMember(store, teamID, payload.AgentID)
	if err := upsertTeamMember(store, teamID, payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	after, _ := loadTeamMember(store, teamID, payload.AgentID)
	_ = appendTeamHistory(store, historyActor{
		AgentID:         payload.AgentID,
		OriginPublicKey: payload.OriginPublicKey,
		ParentPublicKey: payload.ParentPublicKey,
		Source:          "api",
	}, teamID, "member", "update", payload.AgentID, "更新 Team 成员角色或状态", memberHistoryMetadata(before, after))
	members, err := store.LoadMembers(teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":   "team-members",
		"team_id": teamID,
		"members": members,
	})
}

func handleAPITeamMemberAction(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !teamRequestTrusted(r) {
		http.Error(w, "team member action is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	var payload struct {
		AgentID string `json:"agent_id"`
		Action  string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	member, summary, metadata, err := applyTeamMemberAction(store, teamID, payload.AgentID, payload.Action)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = appendTeamHistory(store, historyActor{
		AgentID:         member.AgentID,
		OriginPublicKey: member.OriginPublicKey,
		ParentPublicKey: member.ParentPublicKey,
		Source:          "api",
	}, teamID, "member", "transition", member.AgentID, summary, metadata)
	members, err := store.LoadMembers(teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":   "team-member-action",
		"team_id": teamID,
		"member":  member,
		"members": members,
		"summary": summary,
	})
}

func handleAPITeamArtifactCreate(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team artifact update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	var payload teamcore.Artifact
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload.CreatedAt = time.Now().UTC()
	payload.UpdatedAt = payload.CreatedAt
	if err := store.AppendArtifact(teamID, payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	artifact, err := store.LoadArtifact(teamID, payload.ArtifactID)
	if err != nil {
		artifact = payload
	}
	_ = appendTeamHistory(store, historyActor{
		AgentID:         artifact.CreatedBy,
		OriginPublicKey: artifact.OriginPublicKey,
		ParentPublicKey: artifact.ParentPublicKey,
		Source:          "api",
	}, teamID, "artifact", "create", artifact.ArtifactID, "创建 Team Artifact", artifactHistoryMetadata(teamcore.Artifact{}, artifact))
	newsplugin.WriteJSON(w, http.StatusCreated, map[string]any{
		"scope":    "team-artifact",
		"team_id":  teamID,
		"artifact": artifact,
	})
}

func handleAPITeamArtifactUpdate(store *teamcore.Store, teamID, artifactID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team artifact update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	existing, err := store.LoadArtifact(teamID, artifactID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var payload teamcore.Artifact
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload.TeamID = teamID
	payload.ArtifactID = artifactID
	if payload.Title == "" {
		payload.Title = existing.Title
	}
	if payload.CreatedBy == "" {
		payload.CreatedBy = existing.CreatedBy
	}
	if payload.OriginPublicKey == "" {
		payload.OriginPublicKey = existing.OriginPublicKey
	}
	if payload.ParentPublicKey == "" {
		payload.ParentPublicKey = existing.ParentPublicKey
	}
	if payload.CreatedAt.IsZero() {
		payload.CreatedAt = existing.CreatedAt
	}
	payload.UpdatedAt = time.Now().UTC()
	if err := store.SaveArtifact(teamID, payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	artifact, err := store.LoadArtifact(teamID, artifactID)
	if err != nil {
		artifact = payload
	}
	_ = appendTeamHistory(store, historyActor{
		AgentID:         artifact.CreatedBy,
		OriginPublicKey: artifact.OriginPublicKey,
		ParentPublicKey: artifact.ParentPublicKey,
		Source:          "api",
	}, teamID, "artifact", "update", artifactID, "更新 Team Artifact", artifactHistoryMetadata(existing, artifact))
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":    "team-artifact",
		"team_id":  teamID,
		"artifact": artifact,
	})
}

func handleAPITeamArtifactDelete(store *teamcore.Store, teamID, artifactID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team artifact update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := store.DeleteArtifact(teamID, artifactID); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = appendTeamHistory(store, historyActor{Source: "api"}, teamID, "artifact", "delete", artifactID, "删除 Team Artifact", map[string]any{
		"diff_summary": "删除产物",
	})
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":       "team-artifact",
		"team_id":     teamID,
		"artifact_id": artifactID,
		"deleted":     true,
	})
}

func formatTeamCount(value int) string {
	return strconv.Itoa(value)
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

func filterMembersByStatus(members []teamcore.Member, status string) []teamcore.Member {
	out := make([]teamcore.Member, 0, len(members))
	for _, member := range members {
		if member.Status == status {
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
	members, err := store.LoadMembers(teamID)
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
		"title_before":     strings.TrimSpace(before.Title),
		"title_after":      strings.TrimSpace(after.Title),
		"status_before":    strings.TrimSpace(before.Status),
		"status_after":     strings.TrimSpace(after.Status),
		"priority_before":  strings.TrimSpace(before.Priority),
		"priority_after":   strings.TrimSpace(after.Priority),
		"assignees_before": before.Assignees,
		"assignees_after":  after.Assignees,
		"labels_before":    before.Labels,
		"labels_after":     after.Labels,
		"diff_summary":     "任务标题/状态/优先级/指派已更新",
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
		"labels_before":  before.Labels,
		"labels_after":   after.Labels,
		"diff_summary":   "产物标题/类型/频道/标签已更新",
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

func countArtifactsByKind(artifacts []teamcore.Artifact, kind string) int {
	count := 0
	for _, artifact := range artifacts {
		if strings.EqualFold(strings.TrimSpace(artifact.Kind), kind) {
			count++
		}
	}
	return count
}

func upsertTeamMember(store *teamcore.Store, teamID string, member teamcore.Member) error {
	member.AgentID = strings.TrimSpace(member.AgentID)
	if member.AgentID == "" {
		return errors.New("empty agent_id")
	}
	members, err := store.LoadMembers(teamID)
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
	return store.SaveMembers(teamID, members)
}

func applyTeamMemberAction(store *teamcore.Store, teamID, agentID, action string) (teamcore.Member, string, map[string]any, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return teamcore.Member{}, "", nil, errors.New("empty agent_id")
	}
	nextStatus, summary, err := normalizeMemberAction(action)
	if err != nil {
		return teamcore.Member{}, "", nil, err
	}
	members, err := store.LoadMembers(teamID)
	if err != nil {
		return teamcore.Member{}, "", nil, err
	}
	for i := range members {
		if members[i].AgentID != agentID {
			continue
		}
		before := members[i].Status
		members[i].Status = nextStatus
		if err := store.SaveMembers(teamID, members); err != nil {
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
	return store.AppendHistory(teamID, teamcore.ChangeEvent{
		Scope:                scope,
		Action:               action,
		SubjectID:            subjectID,
		Summary:              summary,
		ActorAgentID:         strings.TrimSpace(actor.AgentID),
		ActorOriginPublicKey: strings.TrimSpace(actor.OriginPublicKey),
		ActorParentPublicKey: strings.TrimSpace(actor.ParentPublicKey),
		Source:               strings.TrimSpace(actor.Source),
		Metadata:             metadata,
		CreatedAt:            time.Now().UTC(),
	})
}

func teamRequestTrusted(r *http.Request) bool {
	addr := teamClientIP(r)
	if !addr.IsValid() {
		return false
	}
	return addr.IsLoopback() || addr.IsPrivate()
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
