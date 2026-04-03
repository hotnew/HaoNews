package haonewsteam

import (
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
	var (
		members   []teamcore.Member
		policy    teamcore.Policy
		messages  []teamcore.Message
		tasks     []teamcore.Task
		artifacts []teamcore.Artifact
		history   []teamcore.ChangeEvent
		channels  []teamcore.ChannelSummary
		index     newsplugin.Index
	)
	var (
		loadErr error
		errOnce sync.Once
		wg      sync.WaitGroup
	)
	captureErr := func(err error) {
		if err == nil {
			return
		}
		errOnce.Do(func() {
			loadErr = err
		})
	}
	wg.Add(8)
	go func() {
		defer wg.Done()
		items, err := store.LoadMembers(teamID)
		if err != nil {
			captureErr(err)
			return
		}
		members = items
	}()
	go func() {
		defer wg.Done()
		value, err := store.LoadPolicy(teamID)
		if err != nil {
			captureErr(err)
			return
		}
		policy = value
	}()
	go func() {
		defer wg.Done()
		items, err := store.LoadMessages(teamID, "main", 20)
		if err != nil {
			captureErr(err)
			return
		}
		messages = items
	}()
	go func() {
		defer wg.Done()
		items, err := store.LoadTasks(teamID, 20)
		if err != nil {
			captureErr(err)
			return
		}
		tasks = items
	}()
	go func() {
		defer wg.Done()
		items, err := store.LoadArtifacts(teamID, 20)
		if err != nil {
			captureErr(err)
			return
		}
		artifacts = items
	}()
	go func() {
		defer wg.Done()
		items, err := store.LoadHistory(teamID, 20)
		if err != nil {
			captureErr(err)
			return
		}
		history = items
	}()
	go func() {
		defer wg.Done()
		items, err := store.ListChannels(teamID)
		if err != nil {
			captureErr(err)
			return
		}
		channels = items
	}()
	go func() {
		defer wg.Done()
		value, err := app.Index()
		if err != nil {
			captureErr(err)
			return
		}
		index = value
	}()
	wg.Wait()
	if loadErr != nil {
		http.Error(w, loadErr.Error(), http.StatusInternalServerError)
		return
	}
	data := teamPageData{
		Project:            app.ProjectName(),
		Version:            app.VersionString(),
		PageNav:            app.PageNav("/teams"),
		NodeStatus:         app.NodeStatus(index),
		Now:                time.Now(),
		Team:               info,
		Policy:             policy,
		Members:            members,
		ActiveMembers:      filterMembersByStatus(members, "active"),
		PendingMembers:     filterMembersByStatus(members, "pending"),
		MutedMembers:       filterMembersByStatus(members, "muted"),
		RemovedMembers:     filterMembersByStatus(members, "removed"),
		Owners:             filterMembersByRole(members, "owner"),
		Maintainers:        filterMembersByRole(members, "maintainer"),
		Observers:          filterMembersByRole(members, "observer"),
		Messages:           messages,
		Tasks:              tasks,
		Channels:           channels,
		Artifacts:          artifacts,
		History:            history,
		TaskStatusCounts:   taskStatusCounts(tasks),
		ArtifactKindCounts: artifactKindCounts(artifacts),
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

func handleTeamMembers(app *newsplugin.App, store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
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
	members, err := store.LoadMembers(teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filterStatus := strings.TrimSpace(r.URL.Query().Get("status"))
	filterRole := strings.TrimSpace(r.URL.Query().Get("role"))
	filterAgent := strings.TrimSpace(r.URL.Query().Get("agent"))
	statusCounts := memberStatusCounts(members)
	roleCounts := memberRoleCounts(members)
	filtered := filterMembers(members, filterStatus, filterRole, filterAgent)
	data := teamMembersPageData{
		Project:        app.ProjectName(),
		Version:        app.VersionString(),
		PageNav:        app.PageNav("/teams"),
		NodeStatus:     app.NodeStatus(index),
		Now:            time.Now(),
		Team:           info,
		Policy:         policy,
		Members:        filtered,
		PendingMembers: filterMembersByStatus(members, "pending"),
		FilterStatus:   filterStatus,
		FilterRole:     filterRole,
		FilterAgent:    filterAgent,
		AppliedFilters: appliedTeamFilters(
			labeledTeamFilter("状态", filterStatus),
			labeledTeamFilter("角色", filterRole),
			labeledTeamFilter("Agent", filterAgent),
		),
		Statuses:     memberStatuses(members),
		Roles:        memberRoles(members),
		StatusCounts: statusCounts,
		RoleCounts:   roleCounts,
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "成员", Value: formatTeamCount(len(filtered))},
			{Label: "active", Value: formatTeamCount(statusCounts["active"])},
			{Label: "pending", Value: formatTeamCount(statusCounts["pending"])},
			{Label: "muted", Value: formatTeamCount(statusCounts["muted"])},
			{Label: "owner", Value: formatTeamCount(roleCounts["owner"])},
		},
	}
	if err := app.Templates().ExecuteTemplate(w, "team_members.html", data); err != nil {
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
		Scopes:       scopes,
		Sources:      sources,
		ScopeCounts:  scopeCounts,
		SourceCounts: sourceCounts,
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
	tasks, err := store.LoadTasks(teamID, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	artifacts, err := store.LoadArtifacts(teamID, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	history, err := store.LoadHistory(teamID, 80)
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
		Project:        app.ProjectName(),
		Version:        app.VersionString(),
		PageNav:        app.PageNav("/teams"),
		NodeStatus:     app.NodeStatus(index),
		Now:            time.Now(),
		Team:           info,
		Channel:        current,
		ChannelID:      channelID,
		Channels:       channels,
		Messages:       messages,
		Tasks:          relatedTasksByChannel(tasks, channelID, 12),
		Artifacts:      relatedArtifactsByChannel(artifacts, channelID, 12),
		RelatedHistory: channelHistory(history, channelID, 12),
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "频道", Value: current.Title},
			{Label: "消息", Value: formatTeamCount(len(messages))},
			{Label: "任务", Value: formatTeamCount(countTasksByChannel(tasks, channelID))},
			{Label: "产物", Value: formatTeamCount(countArtifactsByChannel(artifacts, channelID))},
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
	if err := requireTeamAction(store, teamID, strings.TrimSpace(r.FormValue("actor_agent_id")), "channel.create"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
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
	if err := requireTeamAction(store, teamID, strings.TrimSpace(r.FormValue("actor_agent_id")), "channel.update"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
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
	if err := requireTeamAction(store, teamID, strings.TrimSpace(r.FormValue("actor_agent_id")), "channel.hide"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
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
	artifacts, err := store.LoadArtifacts(teamID, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	channels, err := store.ListChannels(teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
	filterStatus := strings.TrimSpace(r.URL.Query().Get("status"))
	filterAssignee := strings.TrimSpace(r.URL.Query().Get("assignee"))
	filterLabel := strings.TrimSpace(r.URL.Query().Get("label"))
	filterChannel := normalizeTeamChannel(r.URL.Query().Get("channel"))
	statuses := taskStatuses(tasks)
	assignees := taskAssignees(tasks)
	labels := taskLabels(tasks)
	tasks = filterTasks(tasks, filterStatus, filterAssignee, filterLabel, filterChannel)
	data := teamTasksPageData{
		Project:        app.ProjectName(),
		Version:        app.VersionString(),
		PageNav:        app.PageNav("/teams"),
		NodeStatus:     app.NodeStatus(index),
		Now:            time.Now(),
		Team:           info,
		Tasks:          tasks,
		ArtifactCounts: artifactCountsByTask(artifacts),
		HistoryCounts:  historyCountsByTask(history),
		FilterStatus:   filterStatus,
		FilterAssignee: filterAssignee,
		FilterLabel:    filterLabel,
		FilterChannel:  filterChannel,
		AppliedFilters: appliedTeamFilters(
			labeledTeamFilter("状态", filterStatus),
			labeledTeamFilter("负责者", filterAssignee),
			labeledTeamFilter("标签", filterLabel),
			labeledTeamFilter("频道", filterChannel),
		),
		Statuses:  statuses,
		Assignees: assignees,
		Labels:    labels,
		Channels:  channels,
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
	artifacts, err := store.LoadArtifacts(teamID, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	channels, err := store.ListChannels(teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	history, err := store.LoadHistory(teamID, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var relatedChannel *teamcore.ChannelSummary
	if strings.TrimSpace(task.ChannelID) != "" {
		for _, channel := range channels {
			if normalizeTeamChannel(channel.ChannelID) == normalizeTeamChannel(task.ChannelID) {
				channelCopy := channel
				relatedChannel = &channelCopy
				break
			}
		}
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := teamTaskPageData{
		Project:            app.ProjectName(),
		Version:            app.VersionString(),
		PageNav:            app.PageNav("/teams"),
		NodeStatus:         app.NodeStatus(index),
		Now:                time.Now(),
		Team:               info,
		Task:               task,
		Tasks:              tasks,
		Channels:           channels,
		Messages:           messages,
		Artifacts:          relatedArtifacts(artifacts, taskID, 20),
		RelatedChannel:     relatedChannel,
		RelatedHistory:     taskHistory(history, taskID, 10),
		DefaultCommentType: "comment",
		DefaultChannelID:   preferredTaskCommentChannel(task, channels),
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "状态", Value: task.Status},
			{Label: "优先级", Value: blankDash(task.Priority)},
			{Label: "评论", Value: formatTeamCount(len(messages))},
			{Label: "产物", Value: formatTeamCount(countArtifactsByTask(artifacts, taskID))},
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
		ChannelID:       normalizeTeamChannel(r.FormValue("channel_id")),
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
	if err := requireTeamAction(store, teamID, payload.CreatedBy, "task.create"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
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

func handleTeamTaskStatus(store *teamcore.Store, teamID, taskID string, w http.ResponseWriter, r *http.Request) {
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
	updated.Status = strings.TrimSpace(r.FormValue("status"))
	updated.UpdatedAt = time.Now().UTC()
	if err := requireTeamAction(store, teamID, strings.TrimSpace(r.FormValue("actor_agent_id")), "task.transition"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if err := store.SaveTask(teamID, updated); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	after, err := store.LoadTask(teamID, taskID)
	if err != nil {
		after = updated
	}
	_ = appendTeamHistory(store, historyActor{
		AgentID:         after.CreatedBy,
		OriginPublicKey: after.OriginPublicKey,
		ParentPublicKey: after.ParentPublicKey,
		Source:          "page",
	}, teamID, "task", "status", taskID, "更新 Team Task 状态", taskHistoryMetadata(existing, after))
	http.Redirect(w, r, "/teams/"+teamID+"/tasks/"+taskID, http.StatusSeeOther)
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
	updated.ChannelID = normalizeTeamChannel(r.FormValue("channel_id"))
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
	if err := requireTeamAction(store, teamID, strings.TrimSpace(r.FormValue("actor_agent_id")), "task.update"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
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
	existing, err := store.LoadTask(teamID, taskID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := requireTeamAction(store, teamID, strings.TrimSpace(r.FormValue("actor_agent_id")), "task.delete"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
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
	_ = appendTeamHistory(store, historyActor{
		AgentID:         existing.CreatedBy,
		OriginPublicKey: existing.OriginPublicKey,
		ParentPublicKey: existing.ParentPublicKey,
		Source:          "page",
	}, teamID, "task", "delete", taskID, "删除 Team Task", map[string]any{
		"diff_summary": "删除任务",
	})
	http.Redirect(w, r, "/teams/"+teamID+"/tasks", http.StatusSeeOther)
}

func handleTeamTaskCommentCreate(store *teamcore.Store, teamID, taskID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team task comment is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	task, err := store.LoadTask(teamID, taskID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	channelID := normalizeTeamChannel(r.FormValue("channel_id"))
	if channelID == "" {
		channelID = preferredTaskCommentChannel(task, nil)
	}
	structuredData, err := parseOptionalStructuredData(r.FormValue("structured_data"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if structuredData == nil {
		structuredData = make(map[string]any, 2)
	}
	structuredData["task_id"] = taskID
	if strings.TrimSpace(task.ContextID) != "" {
		structuredData["context_id"] = task.ContextID
	}
	msg := teamcore.Message{
		TeamID:          teamID,
		ChannelID:       channelID,
		ContextID:       strings.TrimSpace(task.ContextID),
		AuthorAgentID:   strings.TrimSpace(r.FormValue("author_agent_id")),
		OriginPublicKey: strings.TrimSpace(r.FormValue("origin_public_key")),
		ParentPublicKey: strings.TrimSpace(r.FormValue("parent_public_key")),
		MessageType:     strings.TrimSpace(r.FormValue("message_type")),
		Content:         strings.TrimSpace(r.FormValue("content")),
		StructuredData:  structuredData,
		CreatedAt:       time.Now().UTC(),
	}
	if err := requireTeamAction(store, teamID, msg.AuthorAgentID, "message.send"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
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
	}, teamID, "task", "comment", taskID, "追加 Team Task 评论", map[string]any{
		"task_id":       taskID,
		"channel_id":    channelID,
		"message_type":  blankDash(msg.MessageType),
		"author_agent":  msg.AuthorAgentID,
		"diff_summary":  "任务评论已追加到 Team Channel",
		"message_scope": "team-message",
	})
	http.Redirect(w, r, "/teams/"+teamID+"/tasks/"+taskID, http.StatusSeeOther)
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
	tasks, err := store.LoadTasks(teamID, 100)
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
	filterKind := strings.TrimSpace(r.URL.Query().Get("kind"))
	filterChannel := normalizeTeamChannel(r.URL.Query().Get("channel"))
	filterTask := strings.TrimSpace(r.URL.Query().Get("task"))
	kinds := artifactKinds(artifacts)
	filtered := filterArtifacts(artifacts, filterKind, filterChannel, filterTask)
	data := teamArtifactsPageData{
		Project:       app.ProjectName(),
		Version:       app.VersionString(),
		PageNav:       app.PageNav("/teams"),
		NodeStatus:    app.NodeStatus(index),
		Now:           time.Now(),
		Team:          info,
		Artifacts:     filtered,
		FilterKind:    filterKind,
		FilterChannel: filterChannel,
		FilterTask:    filterTask,
		AppliedFilters: appliedTeamFilters(
			labeledTeamFilter("类型", filterKind),
			labeledTeamFilter("频道", filterChannel),
			labeledTeamFilter("任务", filterTask),
		),
		Kinds:    kinds,
		Channels: channels,
		Tasks:    artifactFilterTasks(tasks, artifacts),
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "产物", Value: formatTeamCount(len(filtered))},
			{Label: "Markdown", Value: formatTeamCount(countArtifactsByKind(filtered, "markdown"))},
			{Label: "链接", Value: formatTeamCount(countArtifactsByKind(filtered, "link"))},
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
	channels, err := store.ListChannels(teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	history, err := store.LoadHistory(teamID, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var relatedTask *teamcore.Task
	if strings.TrimSpace(artifact.TaskID) != "" {
		task, err := store.LoadTask(teamID, artifact.TaskID)
		if err == nil {
			relatedTask = &task
		}
	}
	var relatedChannel *teamcore.ChannelSummary
	if strings.TrimSpace(artifact.ChannelID) != "" {
		for _, channel := range channels {
			if normalizeTeamChannel(channel.ChannelID) == normalizeTeamChannel(artifact.ChannelID) {
				channelCopy := channel
				relatedChannel = &channelCopy
				break
			}
		}
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := teamArtifactPageData{
		Project:        app.ProjectName(),
		Version:        app.VersionString(),
		PageNav:        app.PageNav("/teams"),
		NodeStatus:     app.NodeStatus(index),
		Now:            time.Now(),
		Team:           info,
		Artifact:       artifact,
		Artifacts:      artifacts,
		RelatedTask:    relatedTask,
		RelatedChannel: relatedChannel,
		RelatedHistory: artifactHistory(history, artifactID, 8),
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
		TaskID:          strings.TrimSpace(r.FormValue("task_id")),
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
	if err := requireTeamAction(store, teamID, payload.CreatedBy, "artifact.create"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
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
	updated.TaskID = strings.TrimSpace(r.FormValue("task_id"))
	updated.Title = strings.TrimSpace(r.FormValue("title"))
	updated.Kind = strings.TrimSpace(r.FormValue("kind"))
	updated.Summary = strings.TrimSpace(r.FormValue("summary"))
	updated.Content = strings.TrimSpace(r.FormValue("content"))
	updated.LinkURL = strings.TrimSpace(r.FormValue("link_url"))
	updated.Labels = parseCSVStrings(r.FormValue("labels"))
	updated.UpdatedAt = time.Now().UTC()
	if err := requireTeamAction(store, teamID, strings.TrimSpace(r.FormValue("actor_agent_id")), "artifact.update"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
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
	existing, err := store.LoadArtifact(teamID, artifactID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := requireTeamAction(store, teamID, strings.TrimSpace(r.FormValue("actor_agent_id")), "artifact.delete"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
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
	_ = appendTeamHistory(store, historyActor{
		AgentID:         existing.CreatedBy,
		OriginPublicKey: existing.OriginPublicKey,
		ParentPublicKey: existing.ParentPublicKey,
		Source:          "page",
	}, teamID, "artifact", "delete", artifactID, "删除 Team Artifact", map[string]any{
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
	filterStatus := strings.TrimSpace(r.URL.Query().Get("status"))
	filterRole := strings.TrimSpace(r.URL.Query().Get("role"))
	filterAgent := strings.TrimSpace(r.URL.Query().Get("agent"))
	filtered := filterMembers(members, filterStatus, filterRole, filterAgent)
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":        "team-members",
		"team_id":      info.TeamID,
		"member_count": len(filtered),
		"members":      filtered,
		"applied_filters": map[string]string{
			"status": filterStatus,
			"role":   filterRole,
			"agent":  filterAgent,
		},
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
	limit := clampTeamListLimit(r.URL.Query().Get("limit"), 50, 100)
	messages, err := store.LoadMessages(teamID, channelID, limit)
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
		configs, err := store.LoadWebhookConfigs(teamID)
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
		if err := store.SaveWebhookConfigs(teamID, payload.Webhooks); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		configs, err := store.LoadWebhookConfigs(teamID)
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
	if _, err := store.LoadTeam(teamID); err != nil {
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
		cards, err := store.ListAgentCards(teamID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		taskID := strings.TrimSpace(r.URL.Query().Get("task"))
		var matched []teamcore.AgentCard
		if taskID != "" {
			task, err := store.LoadTask(teamID, taskID)
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
		if err := store.SaveAgentCard(teamID, card); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		loaded, err := store.LoadAgentCard(teamID, card.AgentID)
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
	card, err := store.LoadAgentCard(teamID, agentID)
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
	limit := clampTeamListLimit(r.URL.Query().Get("limit"), 50, 100)
	messages, err := store.LoadMessages(teamID, channelID, limit)
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
	limit := clampTeamListLimit(r.URL.Query().Get("limit"), 50, 100)
	messages, err := store.LoadMessages(teamID, channelID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":         "team-channel-messages",
		"team_id":       info.TeamID,
		"channel_id":    channelID,
		"limit":         limit,
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
	if err := requireTeamAction(store, teamID, payload.AuthorAgentID, "message.send"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
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
	var payload struct {
		teamcore.Channel
		ActorAgentID string `json:"actor_agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := requireTeamAction(store, teamID, payload.ActorAgentID, "channel.create"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	payload.Channel.CreatedAt = time.Now().UTC()
	payload.Channel.UpdatedAt = payload.Channel.CreatedAt
	if err := store.SaveChannel(teamID, payload.Channel); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	channel, err := store.LoadChannel(teamID, payload.ChannelID)
	if err != nil {
		channel = payload.Channel
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
		Title        string `json:"title"`
		Description  string `json:"description"`
		Hidden       *bool  `json:"hidden"`
		ActorAgentID string `json:"actor_agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := requireTeamAction(store, teamID, payload.ActorAgentID, "channel.update"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
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
	var payload struct {
		ActorAgentID string `json:"actor_agent_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)
	if err := requireTeamAction(store, teamID, payload.ActorAgentID, "channel.hide"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
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
	limit := clampTeamListLimit(r.URL.Query().Get("limit"), 100, 200)
	tasks, err := store.LoadTasks(teamID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filterStatus := strings.TrimSpace(r.URL.Query().Get("status"))
	filterAssignee := strings.TrimSpace(r.URL.Query().Get("assignee"))
	filterLabel := strings.TrimSpace(r.URL.Query().Get("label"))
	filterChannel := normalizeTeamChannel(r.URL.Query().Get("channel"))
	tasks = filterTasks(tasks, filterStatus, filterAssignee, filterLabel, filterChannel)
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":      "team-tasks",
		"team_id":    info.TeamID,
		"limit":      limit,
		"task_count": len(tasks),
		"tasks":      tasks,
		"applied_filters": map[string]string{
			"status":   filterStatus,
			"assignee": filterAssignee,
			"label":    filterLabel,
			"channel":  filterChannel,
		},
	})
}

func handleAPITeamTask(store *teamcore.Store, teamID, taskID string, w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost && strings.TrimSpace(r.URL.Query().Get("action")) == "comment" {
		handleAPITeamTaskCommentCreate(store, teamID, taskID, w, r)
		return
	}
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

func handleAPITeamTaskCommentCreate(store *teamcore.Store, teamID, taskID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team task comment is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	task, err := store.LoadTask(teamID, taskID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var payload teamcore.Message
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload.TeamID = teamID
	payload.ChannelID = normalizeTeamChannel(payload.ChannelID)
	if payload.ChannelID == "" {
		payload.ChannelID = "main"
	}
	if payload.StructuredData == nil {
		payload.StructuredData = make(map[string]any, 2)
	}
	payload.StructuredData["task_id"] = taskID
	if strings.TrimSpace(task.ContextID) != "" {
		payload.ContextID = task.ContextID
		payload.StructuredData["context_id"] = task.ContextID
	}
	payload.CreatedAt = time.Now().UTC()
	if err := requireTeamAction(store, teamID, payload.AuthorAgentID, "message.send"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if err := store.AppendMessage(teamID, payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = appendTeamHistory(store, historyActor{
		AgentID:         payload.AuthorAgentID,
		OriginPublicKey: payload.OriginPublicKey,
		ParentPublicKey: payload.ParentPublicKey,
		Source:          "api",
	}, teamID, "task", "comment", taskID, "追加 Team Task 评论", map[string]any{
		"task_id":       taskID,
		"channel_id":    payload.ChannelID,
		"message_type":  blankDash(payload.MessageType),
		"author_agent":  payload.AuthorAgentID,
		"diff_summary":  "任务评论已追加到 Team Channel",
		"message_scope": "team-message",
	})
	newsplugin.WriteJSON(w, http.StatusCreated, map[string]any{
		"scope":   "team-task-comment",
		"team_id": teamID,
		"task_id": taskID,
		"message": payload,
	})
}

func handleAPITeamContext(store *teamcore.Store, teamID, contextID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, err := store.LoadTeam(teamID); err != nil {
		http.NotFound(w, r)
		return
	}
	limit := clampTeamListLimit(r.URL.Query().Get("limit"), 100, 200)
	tasks, err := store.LoadTasksByContext(teamID, contextID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	messages, err := store.LoadMessagesByContext(teamID, contextID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":         "team-context",
		"team_id":       teamID,
		"context_id":    strings.TrimSpace(contextID),
		"limit":         limit,
		"task_count":    len(tasks),
		"message_count": len(messages),
		"tasks":         tasks,
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
	payload.ChannelID = normalizeTeamChannel(payload.ChannelID)
	payload.CreatedAt = time.Now().UTC()
	payload.UpdatedAt = payload.CreatedAt
	if err := requireTeamAction(store, teamID, payload.CreatedBy, "task.create"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
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
	if strings.TrimSpace(payload.ChannelID) == "" {
		payload.ChannelID = existing.ChannelID
	}
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
	if err := requireTeamAction(store, teamID, payload.CreatedBy, "task.update"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
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

func handleAPITeamTaskStatus(store *teamcore.Store, teamID, taskID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !teamRequestTrusted(r) {
		http.Error(w, "team task update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	existing, err := store.LoadTask(teamID, taskID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var payload struct {
		Status       string `json:"status"`
		ActorAgentID string `json:"actor_agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	updated := existing
	updated.Status = payload.Status
	updated.UpdatedAt = time.Now().UTC()
	if err := requireTeamAction(store, teamID, payload.ActorAgentID, "task.transition"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if err := store.SaveTask(teamID, updated); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	task, err := store.LoadTask(teamID, taskID)
	if err != nil {
		task = updated
	}
	_ = appendTeamHistory(store, historyActor{
		AgentID:         task.CreatedBy,
		OriginPublicKey: task.OriginPublicKey,
		ParentPublicKey: task.ParentPublicKey,
		Source:          "api",
	}, teamID, "task", "status", taskID, "更新 Team Task 状态", taskHistoryMetadata(existing, task))
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
	existing, err := store.LoadTask(teamID, taskID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var payload struct {
		ActorAgentID string `json:"actor_agent_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)
	if err := requireTeamAction(store, teamID, payload.ActorAgentID, "task.delete"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
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
	_ = appendTeamHistory(store, historyActor{
		AgentID:         existing.CreatedBy,
		OriginPublicKey: existing.OriginPublicKey,
		ParentPublicKey: existing.ParentPublicKey,
		Source:          "api",
	}, teamID, "task", "delete", taskID, "删除 Team Task", map[string]any{
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
	limit := clampTeamListLimit(r.URL.Query().Get("limit"), 100, 200)
	artifacts, err := store.LoadArtifacts(teamID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filterKind := strings.TrimSpace(r.URL.Query().Get("kind"))
	filterChannel := normalizeTeamChannel(r.URL.Query().Get("channel"))
	filterTask := strings.TrimSpace(r.URL.Query().Get("task"))
	artifacts = filterArtifacts(artifacts, filterKind, filterChannel, filterTask)
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":          "team-artifacts",
		"team_id":        info.TeamID,
		"limit":          limit,
		"artifact_count": len(artifacts),
		"artifacts":      artifacts,
		"applied_filters": map[string]string{
			"kind":    filterKind,
			"channel": filterChannel,
			"task":    filterTask,
		},
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
	before, err := store.LoadPolicy(teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := requireTeamAction(store, teamID, strings.TrimSpace(r.FormValue("actor_agent_id")), "policy.update"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	policy := teamcore.Policy{
		MessageRoles:     parseCSVStrings(r.FormValue("message_roles")),
		TaskRoles:        parseCSVStrings(r.FormValue("task_roles")),
		SystemNoteRoles:  parseCSVStrings(r.FormValue("system_note_roles")),
		Permissions:      before.Permissions,
		RequireSignature: teamFormBool(r.FormValue("require_signature")),
		TaskTransitions:  before.TaskTransitions,
		UpdatedAt:        time.Now().UTC(),
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
	if err := requireTeamAction(store, teamID, strings.TrimSpace(r.FormValue("actor_agent_id")), "member.update"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
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
	if err := requireTeamAction(store, teamID, strings.TrimSpace(r.FormValue("actor_agent_id")), "member.transition"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
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

func handleTeamMemberBulkAction(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team member action is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	action := strings.TrimSpace(r.FormValue("action"))
	agentIDs := parseCSVStrings(r.FormValue("agent_ids"))
	if err := requireTeamAction(store, teamID, strings.TrimSpace(r.FormValue("actor_agent_id")), "member.bulk-transition"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if len(agentIDs) == 0 {
		http.Error(w, "empty agent_ids", http.StatusBadRequest)
		return
	}
	for _, agentID := range agentIDs {
		member, summary, metadata, err := applyTeamMemberAction(store, teamID, agentID, action)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		metadata["batch"] = true
		metadata["batch_size"] = len(agentIDs)
		_ = appendTeamHistory(store, historyActor{
			AgentID:         member.AgentID,
			OriginPublicKey: member.OriginPublicKey,
			ParentPublicKey: member.ParentPublicKey,
			Source:          "page",
		}, teamID, "member", "bulk-transition", member.AgentID, summary, metadata)
	}
	http.Redirect(w, r, "/teams/"+teamID, http.StatusSeeOther)
}

func handleAPITeamPolicyUpdate(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team policy update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	var payload struct {
		teamcore.Policy
		ActorAgentID string `json:"actor_agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	before, err := store.LoadPolicy(teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := requireTeamAction(store, teamID, payload.ActorAgentID, "policy.update"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if payload.Permissions == nil {
		payload.Permissions = before.Permissions
	}
	if payload.TaskTransitions == nil {
		payload.TaskTransitions = before.TaskTransitions
	}
	payload.UpdatedAt = time.Now().UTC()
	if err := store.SavePolicy(teamID, payload.Policy); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = appendTeamHistory(store, historyActor{AgentID: strings.TrimSpace(payload.ActorAgentID), Source: "api"}, teamID, "policy", "update", "team-policy", "更新 Team Policy", policyHistoryMetadata(before, payload.Policy))
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":   "team-policy",
		"team_id": teamID,
		"policy":  payload.Policy,
	})
}

func handleAPITeamMemberUpdate(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team member update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	var payload struct {
		teamcore.Member
		ActorAgentID string `json:"actor_agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := requireTeamAction(store, teamID, payload.ActorAgentID, "member.update"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	before, _ := loadTeamMember(store, teamID, payload.AgentID)
	if err := upsertTeamMember(store, teamID, payload.Member); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	after, _ := loadTeamMember(store, teamID, payload.AgentID)
	_ = appendTeamHistory(store, historyActor{
		AgentID:         strings.TrimSpace(payload.ActorAgentID),
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
		AgentID      string `json:"agent_id"`
		Action       string `json:"action"`
		ActorAgentID string `json:"actor_agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := requireTeamAction(store, teamID, payload.ActorAgentID, "member.transition"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
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

func handleAPITeamMemberBulkAction(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !teamRequestTrusted(r) {
		http.Error(w, "team member action is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	var payload struct {
		AgentIDs     []string `json:"agent_ids"`
		Action       string   `json:"action"`
		ActorAgentID string   `json:"actor_agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := requireTeamAction(store, teamID, payload.ActorAgentID, "member.bulk-transition"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	agentIDs := make([]string, 0, len(payload.AgentIDs))
	seen := make(map[string]struct{}, len(payload.AgentIDs))
	for _, agentID := range payload.AgentIDs {
		agentID = strings.TrimSpace(agentID)
		if agentID == "" {
			continue
		}
		if _, ok := seen[agentID]; ok {
			continue
		}
		seen[agentID] = struct{}{}
		agentIDs = append(agentIDs, agentID)
	}
	if len(agentIDs) == 0 {
		http.Error(w, "empty agent_ids", http.StatusBadRequest)
		return
	}
	applied := make([]teamcore.Member, 0, len(agentIDs))
	for _, agentID := range agentIDs {
		member, summary, metadata, err := applyTeamMemberAction(store, teamID, agentID, payload.Action)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		metadata["batch"] = true
		metadata["batch_size"] = len(agentIDs)
		_ = appendTeamHistory(store, historyActor{
			AgentID:         member.AgentID,
			OriginPublicKey: member.OriginPublicKey,
			ParentPublicKey: member.ParentPublicKey,
			Source:          "api",
		}, teamID, "member", "bulk-transition", member.AgentID, summary, metadata)
		applied = append(applied, member)
	}
	members, err := store.LoadMembers(teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":         "team-member-bulk-action",
		"team_id":       teamID,
		"applied":       applied,
		"applied_count": len(applied),
		"members":       members,
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
	if err := requireTeamAction(store, teamID, payload.CreatedBy, "artifact.create"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
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
	if payload.ChannelID == "" {
		payload.ChannelID = existing.ChannelID
	}
	if payload.TaskID == "" {
		payload.TaskID = existing.TaskID
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
	if err := requireTeamAction(store, teamID, payload.CreatedBy, "artifact.update"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
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
	existing, err := store.LoadArtifact(teamID, artifactID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var payload struct {
		ActorAgentID string `json:"actor_agent_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)
	if err := requireTeamAction(store, teamID, payload.ActorAgentID, "artifact.delete"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
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
	_ = appendTeamHistory(store, historyActor{
		AgentID:         existing.CreatedBy,
		OriginPublicKey: existing.OriginPublicKey,
		ParentPublicKey: existing.ParentPublicKey,
		Source:          "api",
	}, teamID, "artifact", "delete", artifactID, "删除 Team Artifact", map[string]any{
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
		"assignees_before": before.Assignees,
		"assignees_after":  after.Assignees,
		"labels_before":    before.Labels,
		"labels_after":     after.Labels,
		"diff_summary":     "任务频道/标题/状态/优先级/指派/标签已更新",
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

func handleTeamArchiveIndex(app *newsplugin.App, store *teamcore.Store, w http.ResponseWriter, r *http.Request) {
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
	archivedTeams := 0
	for _, item := range teams {
		archives, err := store.ListArchives(item.TeamID)
		if err == nil && len(archives) > 0 {
			archivedTeams++
		}
	}
	data := teamArchiveIndexPageData{
		Project:    app.ProjectName(),
		Version:    app.VersionString(),
		PageNav:    app.PageNav("/archive"),
		NodeStatus: app.NodeStatus(index),
		Now:        time.Now(),
		Teams:      teams,
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "团队", Value: formatTeamCount(len(teams))},
			{Label: "已归档团队", Value: formatTeamCount(archivedTeams)},
			{Label: "最近更新", Value: latestTeamValue(teams)},
		},
	}
	if err := app.Templates().ExecuteTemplate(w, "team_archive_index.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleTeamArchive(app *newsplugin.App, store *teamcore.Store, teamID, archiveID string, w http.ResponseWriter, r *http.Request) {
	info, err := store.LoadTeam(teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	archives, err := store.ListArchives(teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var selected *teamcore.ArchiveSnapshot
	if archiveID != "" {
		item, err := store.LoadArchive(teamID, archiveID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		selected = &item
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := teamArchivePageData{
		Project:    app.ProjectName(),
		Version:    app.VersionString(),
		PageNav:    app.PageNav("/archive"),
		NodeStatus: app.NodeStatus(index),
		Now:        time.Now(),
		Team:       info,
		Archives:   archives,
		Archive:    selected,
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "归档批次", Value: formatTeamCount(len(archives))},
			{Label: "任务", Value: archiveTaskValue(selected)},
			{Label: "产物", Value: archiveArtifactValue(selected)},
		},
	}
	if err := app.Templates().ExecuteTemplate(w, "team_archive.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleTeamArchiveCreate(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !teamRequestTrusted(r) {
		http.Error(w, "team archive writes are limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := requireTeamAction(store, teamID, strings.TrimSpace(r.FormValue("actor_agent_id")), "archive.create"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	record, err := store.CreateManualArchive(teamID, time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/archive/team/"+teamID+"/"+record.ArchiveID, http.StatusSeeOther)
}

func handleAPITeamArchiveIndex(store *teamcore.Store, w http.ResponseWriter, r *http.Request) {
	teams, err := store.ListTeams()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]map[string]any, 0, len(teams))
	for _, item := range teams {
		archives, err := store.ListArchives(item.TeamID)
		if err != nil {
			continue
		}
		summary := map[string]any{
			"team_id":       item.TeamID,
			"title":         item.Title,
			"archive_count": len(archives),
		}
		if len(archives) > 0 {
			summary["last_archive_id"] = archives[0].ArchiveID
			summary["last_archived_at"] = archives[0].ArchivedAt
		}
		items = append(items, summary)
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope": "team-archive-index",
		"teams": items,
	})
}

func handleAPITeamArchive(store *teamcore.Store, teamID, archiveID string, w http.ResponseWriter, r *http.Request) {
	if archiveID == "" {
		archives, err := store.ListArchives(teamID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
			"scope":    "team-archive-list",
			"team_id":  teamID,
			"archives": archives,
		})
		return
	}
	record, err := store.LoadArchive(teamID, archiveID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":   "team-archive-detail",
		"team_id": teamID,
		"archive": record,
	})
}

func handleAPITeamArchiveCreate(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !teamRequestTrusted(r) {
		http.Error(w, "team archive writes are limited to local or LAN requests", http.StatusForbidden)
		return
	}
	var payload struct {
		ActorAgentID string `json:"actor_agent_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)
	if err := requireTeamAction(store, teamID, payload.ActorAgentID, "archive.create"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	record, err := store.CreateManualArchive(teamID, time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":   "team-archive-create",
		"team_id": teamID,
		"archive": record,
	})
}

func archiveTaskValue(item *teamcore.ArchiveSnapshot) string {
	if item == nil {
		return "0"
	}
	return formatTeamCount(item.TaskCount)
}

func archiveArtifactValue(item *teamcore.ArchiveSnapshot) string {
	if item == nil {
		return "0"
	}
	return formatTeamCount(item.ArtifactCount)
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
	info, err := store.LoadTeam(teamID)
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
	policy, err := store.LoadPolicy(teamID)
	if err != nil {
		return err
	}
	if !policy.Allows(action, role) {
		return fmt.Errorf("team policy denied action %q for role %q", action, role)
	}
	return nil
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
