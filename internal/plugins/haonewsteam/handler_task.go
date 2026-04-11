package haonewsteam

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	teamcore "hao.news/internal/haonews/team"
	newsplugin "hao.news/internal/plugins/haonews"
)

func handleTeamTasks(app *newsplugin.App, store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	info, err := store.LoadTeamCtx(r.Context(), teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	tasks, err := store.LoadTasksCtx(r.Context(), teamID, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	artifacts, err := store.LoadArtifactsCtx(r.Context(), teamID, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	channels, err := store.ListChannelsCtx(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	history, err := store.LoadHistoryCtx(r.Context(), teamID, 200)
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
	defaultActor := teamDefaultActor(info, nil)
	taskLanes := buildTeamTaskLanes(tasks)
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
		Statuses:            statuses,
		Assignees:           assignees,
		Labels:              labels,
		Channels:            channels,
		DefaultActorAgentID: defaultActor,
		OverdueCount:        countOverdueTasks(tasks, time.Now()),
		DueSoonCount:        countDueSoonTasks(tasks, time.Now(), 72*time.Hour),
		MyOpenTaskCount:     countTasksAssignedTo(tasks, defaultActor),
		TaskLanes:           taskLanes,
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "任务", Value: formatTeamCount(len(tasks))},
			{Label: "进行中", Value: formatTeamCount(countTasksByStatus(tasks, "doing"))},
			{Label: "已完成", Value: formatTeamCount(countTasksByStatus(tasks, "done"))},
			{Label: "即将到期", Value: formatTeamCount(countDueSoonTasks(tasks, time.Now(), 72*time.Hour))},
			{Label: "已逾期", Value: formatTeamCount(countOverdueTasks(tasks, time.Now()))},
		},
	}
	if err := app.Templates().ExecuteTemplate(w, "team_tasks.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleTeamTask(app *newsplugin.App, store *teamcore.Store, teamID, taskID string, w http.ResponseWriter, r *http.Request) {
	info, err := store.LoadTeamCtx(r.Context(), teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	task, err := store.LoadTaskCtx(r.Context(), teamID, taskID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	tasks, err := store.LoadTasksCtx(r.Context(), teamID, 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	messages, err := store.LoadTaskMessagesCtx(r.Context(), teamID, taskID, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	artifacts, err := store.LoadArtifactsCtx(r.Context(), teamID, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	channels, err := store.ListChannelsCtx(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	history, err := store.LoadHistoryCtx(r.Context(), teamID, 100)
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
			{Label: "截止时间", Value: formatTaskDue(task.DueAt)},
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
		DueAt:           parseTaskDueAt(r.FormValue("due_at")),
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
	if err := store.AppendTaskCtx(r.Context(), teamID, payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	targetID := payload.TaskID
	if targetID == "" {
		tasks, err := store.LoadTasksCtx(r.Context(), teamID, 1)
		if err == nil && len(tasks) > 0 {
			targetID = tasks[0].TaskID
		}
	}
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{
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
	existing, err := store.LoadTaskCtx(r.Context(), teamID, taskID)
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
	if err := store.SaveTaskCtx(r.Context(), teamID, updated); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	after, err := store.LoadTaskCtx(r.Context(), teamID, taskID)
	if err != nil {
		after = updated
	}
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{
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
	existing, err := store.LoadTaskCtx(r.Context(), teamID, taskID)
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
	updated.DueAt = parseTaskDueAt(r.FormValue("due_at"))
	updated.Labels = parseCSVStrings(r.FormValue("labels"))
	updated.UpdatedAt = time.Now().UTC()
	if err := requireTeamAction(store, teamID, strings.TrimSpace(r.FormValue("actor_agent_id")), "task.update"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if err := store.SaveTaskCtx(r.Context(), teamID, updated); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{
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
	existing, err := store.LoadTaskCtx(r.Context(), teamID, taskID)
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
	if err := store.DeleteTaskCtx(r.Context(), teamID, taskID); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{
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
	task, err := store.LoadTaskCtx(r.Context(), teamID, taskID)
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
	if err := store.AppendMessageCtx(r.Context(), teamID, msg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{
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

func handleAPITeamTasks(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		handleAPITeamTaskCreate(store, teamID, w, r)
		return
	}
	info, err := store.LoadTeamCtx(r.Context(), teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	limit := clampTeamListLimit(r.URL.Query().Get("limit"), 100, 200)
	tasks, err := store.LoadTasksCtx(r.Context(), teamID, limit)
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
	info, err := store.LoadTeamCtx(r.Context(), teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	task, err := store.LoadTaskCtx(r.Context(), teamID, taskID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	thread, err := store.LoadTaskThreadCtx(r.Context(), teamID, taskID, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":         "team-task",
		"team_id":       info.TeamID,
		"task_id":       task.TaskID,
		"task":          task,
		"dispatch":      thread.Dispatch,
		"message_count": len(thread.Messages),
		"messages":      thread.Messages,
	})
}

func handleAPITeamTaskCommentCreate(store *teamcore.Store, teamID, taskID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team task comment is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	task, err := store.LoadTaskCtx(r.Context(), teamID, taskID)
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
	if err := store.AppendMessageCtx(r.Context(), teamID, payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{
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
	if _, err := store.LoadTeamCtx(r.Context(), teamID); err != nil {
		http.NotFound(w, r)
		return
	}
	limit := clampTeamListLimit(r.URL.Query().Get("limit"), 100, 200)
	tasks, err := store.LoadTasksByContextCtx(r.Context(), teamID, contextID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	messages, err := store.LoadMessagesByContextCtx(r.Context(), teamID, contextID, limit)
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

func handleAPITeamTaskThread(store *teamcore.Store, teamID, taskID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := clampTeamListLimit(r.URL.Query().Get("limit"), 100, 200)
	thread, err := store.LoadTaskThreadCtx(r.Context(), teamID, taskID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":         "team-task-thread",
		"team_id":       teamID,
		"task_id":       taskID,
		"limit":         limit,
		"task":          thread.Task,
		"dispatch":      thread.Dispatch,
		"message_count": len(thread.Messages),
		"messages":      thread.Messages,
	})
}

func handleAPITeamTaskDispatch(store *teamcore.Store, teamID, taskID string, w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		dispatch, err := store.LoadTaskDispatchCtx(r.Context(), teamID, taskID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
			"scope":    "team-task-dispatch",
			"team_id":  teamID,
			"task_id":  taskID,
			"dispatch": dispatch,
		})
	case http.MethodPost, http.MethodPut:
		if !teamRequestTrusted(r) {
			http.Error(w, "team task update is limited to local or LAN requests", http.StatusForbidden)
			return
		}
		var payload struct {
			ActorAgentID     string `json:"actor_agent_id"`
			AssignedAgentID  string `json:"assigned_agent_id"`
			MatchReason      string `json:"match_reason"`
			Status           string `json:"status"`
			RetryCount       int    `json:"retry_count"`
			TimeoutSeconds   int    `json:"timeout_seconds"`
			CurrentQueueSize int    `json:"current_queue_size"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireTeamAction(store, teamID, payload.ActorAgentID, teamcore.ActionTaskTransition); err != nil {
			if resp, ok := classifyTeamAPIError(teamID, err); ok {
				writeTeamAPIError(w, http.StatusForbidden, resp)
				return
			}
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		task, err := store.LoadTaskCtx(r.Context(), teamID, taskID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		dispatch := teamcore.TaskDispatch{
			TaskID:           taskID,
			AssignedAgentID:  payload.AssignedAgentID,
			MatchReason:      payload.MatchReason,
			Status:           payload.Status,
			RetryCount:       payload.RetryCount,
			TimeoutSeconds:   payload.TimeoutSeconds,
			CurrentQueueSize: payload.CurrentQueueSize,
			LastResponseAt:   time.Now().UTC(),
		}
		if err := store.SaveTaskDispatchCtx(r.Context(), teamID, dispatch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !teamcore.IsTerminalState(task.Status) && teamcore.NormalizeTeamID(task.TeamID) != "" && normalizeTeamChannel(task.ChannelID) != "" {
			updated := task
			updated.Status = teamcore.TaskStateDispatched
			updated.UpdatedAt = time.Now().UTC()
			if err := store.SaveTaskCtx(r.Context(), teamID, updated); err != nil {
				if !errors.Is(err, teamcore.ErrInvalidState) {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			} else {
				task = updated
			}
		}
		saved, err := store.LoadTaskDispatchCtx(r.Context(), teamID, taskID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
			"scope":    "team-task-dispatch",
			"team_id":  teamID,
			"task_id":  taskID,
			"task":     task,
			"dispatch": saved,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
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
	if err := store.AppendTaskCtx(r.Context(), teamID, payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	task, err := store.LoadTaskCtx(r.Context(), teamID, payload.TaskID)
	if err != nil {
		task = payload
	}
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{
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

func parseTaskDueAt(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if value, err := time.Parse(layout, raw); err == nil {
			return value.UTC()
		}
	}
	return time.Time{}
}

func formatTaskDue(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Local().Format("2006-01-02 15:04")
}

func countOverdueTasks(tasks []teamcore.Task, now time.Time) int {
	count := 0
	for _, task := range tasks {
		if task.DueAt.IsZero() || teamcore.IsTerminalState(task.Status) {
			continue
		}
		if task.DueAt.Before(now) {
			count++
		}
	}
	return count
}

func countDueSoonTasks(tasks []teamcore.Task, now time.Time, within time.Duration) int {
	count := 0
	limit := now.Add(within)
	for _, task := range tasks {
		if task.DueAt.IsZero() || teamcore.IsTerminalState(task.Status) {
			continue
		}
		if (task.DueAt.Equal(now) || task.DueAt.After(now)) && task.DueAt.Before(limit) {
			count++
		}
	}
	return count
}

func countTasksAssignedTo(tasks []teamcore.Task, agentID string) int {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return 0
	}
	count := 0
	for _, task := range tasks {
		if teamcore.IsTerminalState(task.Status) {
			continue
		}
		if containsFolded(task.Assignees, agentID) {
			count++
		}
	}
	return count
}

func buildTeamTaskLanes(tasks []teamcore.Task) []teamTaskLane {
	copied := append([]teamcore.Task(nil), tasks...)
	sort.SliceStable(copied, func(i, j int) bool {
		return compareTeamTasks(copied[i], copied[j])
	})
	definitions := []struct {
		key   string
		title string
		hint  string
	}{
		{key: "doing", title: "推进中", hint: "优先处理正在推进和被阻塞的任务。"},
		{key: "review", title: "待确认", hint: "这些任务已接近完成，需要复核或确认。"},
		{key: "open", title: "待开始", hint: "还没启动的任务，适合作为下一步入口。"},
		{key: "done", title: "已完成", hint: "已完成任务保留在最后，便于回看结果。"},
	}
	lanes := make([]teamTaskLane, 0, len(definitions))
	for _, definition := range definitions {
		laneTasks := make([]teamcore.Task, 0)
		for _, task := range copied {
			if teamTaskLaneKey(task) != definition.key {
				continue
			}
			laneTasks = append(laneTasks, task)
		}
		lanes = append(lanes, teamTaskLane{
			Key:   definition.key,
			Title: definition.title,
			Hint:  definition.hint,
			Count: len(laneTasks),
			Tasks: laneTasks,
		})
	}
	return lanes
}

func teamTaskLaneKey(task teamcore.Task) string {
	switch strings.TrimSpace(task.Status) {
	case "doing", "blocked":
		return "doing"
	case "review":
		return "review"
	case "done", "closed", "cancelled":
		return "done"
	default:
		return "open"
	}
}

func compareTeamTasks(left, right teamcore.Task) bool {
	if dueScore(left) != dueScore(right) {
		return dueScore(left) > dueScore(right)
	}
	if priorityScore(left.Priority) != priorityScore(right.Priority) {
		return priorityScore(left.Priority) > priorityScore(right.Priority)
	}
	if !left.DueAt.Equal(right.DueAt) {
		if left.DueAt.IsZero() {
			return false
		}
		if right.DueAt.IsZero() {
			return true
		}
		return left.DueAt.Before(right.DueAt)
	}
	if !left.UpdatedAt.Equal(right.UpdatedAt) {
		return left.UpdatedAt.After(right.UpdatedAt)
	}
	return left.TaskID > right.TaskID
}

func dueScore(task teamcore.Task) int {
	if task.DueAt.IsZero() || teamcore.IsTerminalState(task.Status) {
		return 0
	}
	now := time.Now().UTC()
	switch {
	case task.DueAt.Before(now):
		return 3
	case task.DueAt.Before(now.Add(24 * time.Hour)):
		return 2
	default:
		return 1
	}
}

func priorityScore(value string) int {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "high", "urgent":
		return 3
	case "normal", "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func handleAPITeamTaskUpdate(store *teamcore.Store, teamID, taskID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team task update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	existing, err := store.LoadTaskCtx(r.Context(), teamID, taskID)
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
	if strings.TrimSpace(payload.ParentTaskID) == "" {
		payload.ParentTaskID = existing.ParentTaskID
	}
	if len(payload.DependsOn) == 0 {
		payload.DependsOn = append([]string(nil), existing.DependsOn...)
	}
	if strings.TrimSpace(payload.MilestoneID) == "" {
		payload.MilestoneID = existing.MilestoneID
	}
	payload.UpdatedAt = time.Now().UTC()
	if err := requireTeamAction(store, teamID, payload.CreatedBy, "task.update"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if err := store.SaveTaskCtx(r.Context(), teamID, payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	task, err := store.LoadTaskCtx(r.Context(), teamID, taskID)
	if err != nil {
		task = payload
	}
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{
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
	existing, err := store.LoadTaskCtx(r.Context(), teamID, taskID)
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
		if resp, ok := classifyTeamAPIError(teamID, err); ok {
			writeTeamAPIError(w, http.StatusForbidden, resp)
			return
		}
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if err := store.SaveTaskCtx(r.Context(), teamID, updated); err != nil {
		if resp, ok := classifyTeamAPIError(teamID, err); ok {
			writeTeamAPIError(w, http.StatusBadRequest, resp)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	task, err := store.LoadTaskCtx(r.Context(), teamID, taskID)
	if err != nil {
		task = updated
	}
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{
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
	existing, err := store.LoadTaskCtx(r.Context(), teamID, taskID)
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
	if err := store.DeleteTaskCtx(r.Context(), teamID, taskID); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{
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
