package incidentroom

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"

	teamcore "hao.news/internal/haonews/team"
)

//go:embed templates/*.html
var templatesFS embed.FS

type incidentRoomCardView struct {
	MessageID       string
	Kind            string
	KindLabel       string
	Title           string
	Content         string
	Summary         string
	StatusLabel     string
	Severity        string
	IncidentRef     string
	TaskID          string
	TaskLink        string
	AuthorAgentID   string
	CreatedAt       time.Time
	StructuredJSON  string
	Distilled       bool
	DistillArtifact string
	DistillTitle    string
}

type incidentRoomSummary struct {
	TeamID                string                 `json:"team_id"`
	ChannelID             string                 `json:"channel_id"`
	IncidentCount         int                    `json:"incident_count"`
	UpdateCount           int                    `json:"update_count"`
	RecoveryCount         int                    `json:"recovery_count"`
	DistilledCount        int                    `json:"distilled_count"`
	BoundTaskCount        int                    `json:"bound_task_count"`
	UnboundTaskCount      int                    `json:"unbound_task_count"`
	SuggestedBlockedCount int                    `json:"suggested_blocked_count"`
	SuggestedDoingCount   int                    `json:"suggested_doing_count"`
	SuggestedDoneCount    int                    `json:"suggested_done_count"`
	RecentBatchRuns       []incidentRoomBatchRun `json:"recent_batch_runs,omitempty"`
	Cards                 []incidentRoomCardView `json:"cards,omitempty"`
}

type incidentRoomBatchRun struct {
	CreatedAt             time.Time `json:"created_at"`
	ActorAgentID          string    `json:"actor_agent_id,omitempty"`
	SyncedItems           int       `json:"synced_items"`
	TaskCreated           int       `json:"task_created"`
	ArtifactCreated       int       `json:"artifact_created"`
	TotalMessages         int       `json:"total_messages"`
	SuggestedBlockedCount int       `json:"suggested_blocked_count"`
	SuggestedDoingCount   int       `json:"suggested_doing_count"`
	SuggestedDoneCount    int       `json:"suggested_done_count"`
	HistoryLink           string    `json:"history_link,omitempty"`
	CreatedTaskIDs        []string  `json:"created_task_ids,omitempty"`
	CreatedArtifactIDs    []string  `json:"created_artifact_ids,omitempty"`
	CreatedTaskLinks      []string  `json:"created_task_links,omitempty"`
	CreatedArtifactLinks  []string  `json:"created_artifact_links,omitempty"`
}

type incidentRoomPageData struct {
	TeamID                string
	ChannelID             string
	FilterKind            string
	ActorAgentID          string
	Notice                string
	Cards                 []incidentRoomCardView
	IncidentCount         int
	UpdateCount           int
	RecoveryCount         int
	DistilledCount        int
	BoundTaskCount        int
	UnboundTaskCount      int
	SuggestedBlockedCount int
	SuggestedDoingCount   int
	SuggestedDoneCount    int
	RecentBatchRuns       []incidentRoomBatchRun
	ArtifactLink          string
	HistoryLink           string
}

type postIncidentRoomRequest struct {
	ChannelID      string         `json:"channel_id"`
	AuthorAgentID  string         `json:"author_agent_id"`
	Kind           string         `json:"kind"`
	Content        string         `json:"content"`
	StructuredData map[string]any `json:"structured_data"`
}

type distillRequest struct {
	ChannelID    string `json:"channel_id"`
	MessageID    string `json:"message_id"`
	ActorAgentID string `json:"actor_agent_id"`
	Title        string `json:"title,omitempty"`
}

type taskSyncRequest struct {
	ChannelID    string `json:"channel_id"`
	MessageID    string `json:"message_id"`
	TaskID       string `json:"task_id,omitempty"`
	ActorAgentID string `json:"actor_agent_id"`
}

type taskSyncAllRequest struct {
	ChannelID    string `json:"channel_id"`
	ActorAgentID string `json:"actor_agent_id"`
}

func newHandler(store *teamcore.Store, teamID string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "" {
			http.NotFound(w, r)
			return
		}
		handleListIncidentRoom(store, teamID, w, r)
	})
	mux.HandleFunc("/summary", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleIncidentRoomSummary(store, teamID, w, r)
	})
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handlePostIncidentRoomMessage(store, teamID, w, r)
	})
	mux.HandleFunc("/distill", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleDistillIncidentRoom(store, teamID, w, r)
	})
	mux.HandleFunc("/task-sync", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleSyncIncidentRoomTask(store, teamID, w, r)
	})
	mux.HandleFunc("/task-sync-all", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleSyncAllIncidentRoomTasks(store, teamID, w, r)
	})
	return mux
}

func handleListIncidentRoom(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	channelID := strings.TrimSpace(r.URL.Query().Get("channel_id"))
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	actorAgentID := strings.TrimSpace(r.URL.Query().Get("actor_agent_id"))
	if channelID == "" {
		channelID = "main"
	}
	messages, err := store.LoadMessagesCtx(r.Context(), teamID, channelID, 200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	history, err := store.LoadHistoryCtx(r.Context(), teamID, 200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filtered := filterIncidentRoomMessages(messages, kind)
	if isAPIRequest(r) {
		if filtered == nil {
			filtered = []teamcore.Message{}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(filtered)
		return
	}
	data, err := buildIncidentRoomPageData(r.Context(), store, teamID, channelID, actorAgentID, kind, strings.TrimSpace(r.URL.Query().Get("notice")), filtered, history)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := renderIncidentRoomPage(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleIncidentRoomSummary(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	channelID := strings.TrimSpace(r.URL.Query().Get("channel_id"))
	if channelID == "" {
		channelID = "main"
	}
	messages, err := store.LoadMessagesCtx(r.Context(), teamID, channelID, 200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	history, err := store.LoadHistoryCtx(r.Context(), teamID, 200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := buildIncidentRoomPageData(r.Context(), store, teamID, channelID, "", "", "", filterIncidentRoomMessages(messages, ""), history)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(incidentRoomSummary{
		TeamID:                teamID,
		ChannelID:             channelID,
		IncidentCount:         data.IncidentCount,
		UpdateCount:           data.UpdateCount,
		RecoveryCount:         data.RecoveryCount,
		DistilledCount:        data.DistilledCount,
		BoundTaskCount:        data.BoundTaskCount,
		UnboundTaskCount:      data.UnboundTaskCount,
		SuggestedBlockedCount: data.SuggestedBlockedCount,
		SuggestedDoingCount:   data.SuggestedDoingCount,
		SuggestedDoneCount:    data.SuggestedDoneCount,
		RecentBatchRuns:       limitIncidentRoomBatchRuns(data.RecentBatchRuns, 4),
		Cards:                 data.Cards,
	})
}

func handlePostIncidentRoomMessage(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !requestTrusted(r) {
		http.Error(w, "incident-room write is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	req, err := decodePostIncidentRoomRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	validKinds := map[string]bool{"incident": true, "update": true, "recovery": true}
	if !validKinds[req.Kind] {
		http.Error(w, "kind must be incident, update, or recovery", http.StatusBadRequest)
		return
	}
	if req.ChannelID == "" {
		req.ChannelID = "main"
	}
	if req.AuthorAgentID == "" {
		http.Error(w, "author_agent_id is required", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		req.Content = incidentRoomStructuredTitle(req.StructuredData)
	}
	if err := requireAction(store, teamID, req.AuthorAgentID, "message.send"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	msg := teamcore.Message{
		TeamID:         teamID,
		ChannelID:      req.ChannelID,
		AuthorAgentID:  req.AuthorAgentID,
		MessageType:    req.Kind,
		Content:        req.Content,
		StructuredData: req.StructuredData,
		CreatedAt:      time.Now().UTC(),
	}
	if err := store.AppendMessageCtx(r.Context(), teamID, msg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = store.AppendHistoryCtx(r.Context(), teamID, teamcore.ChangeEvent{
		TeamID:    teamID,
		Scope:     "message",
		Action:    "create",
		SubjectID: req.ChannelID + ":" + msg.CreatedAt.UTC().Format(time.RFC3339Nano),
		Summary:   "发送 incident-room 消息",
		Metadata: map[string]any{
			"channel_id":    req.ChannelID,
			"message_type":  req.Kind,
			"author_agent":  req.AuthorAgentID,
			"message_scope": "incident-room",
		},
		CreatedAt: msg.CreatedAt,
	})
	if isAPIRequest(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "created", "kind": req.Kind})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%s/r/incident-room/?channel_id=%s&kind=%s&actor_agent_id=%s&notice=created",
		teamID, req.ChannelID, url.QueryEscape(req.Kind), url.QueryEscape(req.AuthorAgentID)), http.StatusSeeOther)
}

func handleDistillIncidentRoom(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !requestTrusted(r) {
		http.Error(w, "incident-room distill is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	req, err := decodeDistillRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ChannelID == "" || req.MessageID == "" || req.ActorAgentID == "" {
		http.Error(w, "channel_id, message_id, and actor_agent_id are required", http.StatusBadRequest)
		return
	}
	if err := requireAction(store, teamID, req.ActorAgentID, "artifact.create"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	messages, err := store.LoadAllMessagesCtx(r.Context(), teamID, req.ChannelID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var roomMsg *teamcore.Message
	for i := range messages {
		if messages[i].MessageID == req.MessageID && isIncidentRoomKind(messages[i].MessageType) {
			roomMsg = &messages[i]
			break
		}
	}
	if roomMsg == nil {
		http.Error(w, "incident-room message not found", http.StatusNotFound)
		return
	}
	artifact, _, err := ensureIncidentRoomArtifact(r.Context(), store, teamID, req.ChannelID, *roomMsg, req.ActorAgentID, req.Title)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if isAPIRequest(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "distilled", "artifact_kind": "incident-summary", "artifact_id": artifact.ArtifactID})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%s/r/incident-room/?channel_id=%s&actor_agent_id=%s&notice=distilled",
		teamID, req.ChannelID, url.QueryEscape(req.ActorAgentID)), http.StatusSeeOther)
}

func handleSyncIncidentRoomTask(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !requestTrusted(r) {
		http.Error(w, "incident-room task sync is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	req, err := decodeTaskSyncRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ChannelID == "" || req.MessageID == "" || req.ActorAgentID == "" {
		http.Error(w, "channel_id, message_id, and actor_agent_id are required", http.StatusBadRequest)
		return
	}
	msg, err := loadIncidentRoomMessage(r.Context(), store, teamID, req.ChannelID, req.MessageID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	taskID, taskStatus, taskCreated, err := syncIncidentRoomTask(r.Context(), store, teamID, req.ChannelID, *msg, req.TaskID, req.ActorAgentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if isAPIRequest(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":       "synced",
			"task_id":      taskID,
			"task_status":  taskStatus,
			"task_created": taskCreated,
		})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%s/r/incident-room/?channel_id=%s&actor_agent_id=%s&notice=task-synced",
		teamID, req.ChannelID, url.QueryEscape(req.ActorAgentID)), http.StatusSeeOther)
}

func handleSyncAllIncidentRoomTasks(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !requestTrusted(r) {
		http.Error(w, "incident-room batch sync is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	req, err := decodeTaskSyncAllRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ChannelID == "" || req.ActorAgentID == "" {
		http.Error(w, "channel_id and actor_agent_id are required", http.StatusBadRequest)
		return
	}
	messages, err := store.LoadAllMessagesCtx(r.Context(), teamID, req.ChannelID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filtered := filterIncidentRoomMessages(messages, "")
	synced, taskCreated, artifactCreated := 0, 0, 0
	createdTaskIDs := make([]string, 0, len(filtered))
	createdArtifactIDs := make([]string, 0, len(filtered))
	suggestedBlockedCount, suggestedDoingCount, suggestedDoneCount := 0, 0, 0
	for _, msg := range filtered {
		switch incidentRoomSuggestedTaskStatus(msg.MessageType) {
		case "blocked":
			suggestedBlockedCount++
		case "doing":
			suggestedDoingCount++
		case "done":
			suggestedDoneCount++
		}
		taskID, _, created, err := syncIncidentRoomTask(r.Context(), store, teamID, req.ChannelID, msg, strings.TrimSpace(stringField(msg.StructuredData, "task_id")), req.ActorAgentID)
		if err != nil {
			continue
		}
		synced++
		if created {
			taskCreated++
			if strings.TrimSpace(taskID) != "" {
				createdTaskIDs = append(createdTaskIDs, strings.TrimSpace(taskID))
			}
		}
		if strings.TrimSpace(msg.MessageType) == "recovery" {
			artifact, createdArtifact, err := ensureIncidentRoomArtifact(r.Context(), store, teamID, req.ChannelID, msg, req.ActorAgentID, "")
			if err == nil && createdArtifact {
				artifactCreated++
				if strings.TrimSpace(artifact.ArtifactID) == "" {
					if recovered, ok := incidentRoomArtifactByMessageID(r.Context(), store, teamID, req.ChannelID, msg.MessageID); ok {
						artifact = recovered
					}
				}
				if strings.TrimSpace(artifact.ArtifactID) != "" {
					createdArtifactIDs = append(createdArtifactIDs, strings.TrimSpace(artifact.ArtifactID))
				}
			}
		}
	}
	_ = store.AppendHistoryCtx(r.Context(), teamID, teamcore.ChangeEvent{
		TeamID:       teamID,
		Scope:        "room",
		Action:       "sync",
		SubjectID:    req.ChannelID,
		Summary:      "批量同步 incident-room 到任务主链",
		ActorAgentID: req.ActorAgentID,
		Source:       "incident-room",
		Metadata: map[string]any{
			"channel_id":              req.ChannelID,
			"message_scope":           "incident-room",
			"batch_action":            "task-sync-all",
			"synced_items":            synced,
			"task_created":            taskCreated,
			"artifact_created":        artifactCreated,
			"total_messages":          len(filtered),
			"suggested_blocked_count": suggestedBlockedCount,
			"suggested_doing_count":   suggestedDoingCount,
			"suggested_done_count":    suggestedDoneCount,
			"created_task_ids":        createdTaskIDs,
			"created_artifact_ids":    createdArtifactIDs,
		},
		CreatedAt: time.Now().UTC(),
	})
	if isAPIRequest(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":           "synced",
			"synced_items":     synced,
			"task_created":     taskCreated,
			"artifact_created": artifactCreated,
		})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%s/r/incident-room/?channel_id=%s&actor_agent_id=%s&notice=tasks-synced",
		teamID, req.ChannelID, url.QueryEscape(req.ActorAgentID)), http.StatusSeeOther)
}

func filterIncidentRoomMessages(messages []teamcore.Message, kind string) []teamcore.Message {
	filtered := make([]teamcore.Message, 0, len(messages))
	for _, msg := range messages {
		msgType := strings.TrimSpace(msg.MessageType)
		if !isIncidentRoomKind(msgType) {
			continue
		}
		if kind != "" && msgType != kind {
			continue
		}
		filtered = append(filtered, msg)
	}
	return filtered
}

func buildIncidentRoomPageData(ctx context.Context, store *teamcore.Store, teamID, channelID, actorAgentID, kind, notice string, messages []teamcore.Message, history []teamcore.ChangeEvent) (incidentRoomPageData, error) {
	artifacts, err := store.LoadArtifactsCtx(ctx, teamID, 200)
	if err != nil {
		return incidentRoomPageData{}, err
	}
	taskBindings := buildIncidentRoomTaskBindings(channelID, history)
	distilledByMessage := map[string]teamcore.Artifact{}
	for _, artifact := range artifacts {
		for _, label := range artifact.Labels {
			if strings.HasPrefix(label, "source-message:") {
				distilledByMessage[strings.TrimPrefix(label, "source-message:")] = artifact
			}
		}
	}
	cards := make([]incidentRoomCardView, 0, len(messages))
	incidentCount, updateCount, recoveryCount, distilledCount := 0, 0, 0, 0
	boundTaskCount, unboundTaskCount := 0, 0
	suggestedBlockedCount, suggestedDoingCount, suggestedDoneCount := 0, 0, 0
	for _, msg := range messages {
		distilledArtifact, distilled := distilledByMessage[msg.MessageID]
		if distilled {
			distilledCount++
		}
		switch strings.TrimSpace(msg.MessageType) {
		case "incident":
			incidentCount++
		case "update":
			updateCount++
		case "recovery":
			recoveryCount++
		}
		taskID := firstNonEmpty(strings.TrimSpace(stringField(msg.StructuredData, "task_id")), taskBindings[msg.MessageID])
		if taskID != "" {
			boundTaskCount++
		} else {
			unboundTaskCount++
		}
		switch incidentRoomSuggestedTaskStatus(msg.MessageType) {
		case "blocked":
			suggestedBlockedCount++
		case "doing":
			suggestedDoingCount++
		case "done":
			suggestedDoneCount++
		}
		cards = append(cards, incidentRoomCardView{
			MessageID:       msg.MessageID,
			Kind:            msg.MessageType,
			KindLabel:       incidentRoomKindLabel(msg.MessageType),
			Title:           incidentRoomStructuredTitle(msg.StructuredData),
			Content:         msg.Content,
			Summary:         incidentRoomSummaryLine(msg.StructuredData),
			StatusLabel:     incidentRoomStatusLabel(msg.MessageType, distilled),
			Severity:        strings.TrimSpace(stringField(msg.StructuredData, "severity")),
			IncidentRef:     firstNonEmpty(strings.TrimSpace(stringField(msg.StructuredData, "incident_ref")), strings.TrimSpace(stringField(msg.StructuredData, "title"))),
			TaskID:          taskID,
			TaskLink:        incidentRoomTaskLink(teamID, taskID),
			AuthorAgentID:   msg.AuthorAgentID,
			CreatedAt:       msg.CreatedAt,
			StructuredJSON:  formatStructuredJSON(msg.StructuredData),
			Distilled:       distilled,
			DistillArtifact: incidentRoomArtifactLink(teamID, distilledArtifact.ArtifactID),
			DistillTitle:    distilledArtifact.Title,
		})
	}
	return incidentRoomPageData{
		TeamID:                teamID,
		ChannelID:             channelID,
		FilterKind:            kind,
		ActorAgentID:          actorAgentID,
		Notice:                notice,
		Cards:                 cards,
		IncidentCount:         incidentCount,
		UpdateCount:           updateCount,
		RecoveryCount:         recoveryCount,
		DistilledCount:        distilledCount,
		BoundTaskCount:        boundTaskCount,
		UnboundTaskCount:      unboundTaskCount,
		SuggestedBlockedCount: suggestedBlockedCount,
		SuggestedDoingCount:   suggestedDoingCount,
		SuggestedDoneCount:    suggestedDoneCount,
		RecentBatchRuns:       limitIncidentRoomBatchRuns(buildIncidentRoomBatchRuns(teamID, channelID, history), 4),
		ArtifactLink:          fmt.Sprintf("/teams/%s/artifacts?channel=%s&kind=incident-summary", teamID, url.QueryEscape(channelID)),
		HistoryLink:           fmt.Sprintf("/teams/%s/history?scope=message", teamID),
	}, nil
}

func renderIncidentRoomPage(w http.ResponseWriter, data incidentRoomPageData) error {
	tmpl, err := template.New("channel.html").Funcs(template.FuncMap{
		"kindSelected": func(filterKind, kind string) bool {
			return strings.TrimSpace(filterKind) == strings.TrimSpace(kind)
		},
	}).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return err
	}
	return tmpl.ExecuteTemplate(w, "channel.html", data)
}

func decodePostIncidentRoomRequest(r *http.Request) (postIncidentRoomRequest, error) {
	if isJSONRequest(r) {
		var req postIncidentRoomRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return postIncidentRoomRequest{}, err
		}
		return req, nil
	}
	if err := r.ParseForm(); err != nil {
		return postIncidentRoomRequest{}, err
	}
	req := postIncidentRoomRequest{
		ChannelID:     strings.TrimSpace(r.FormValue("channel_id")),
		AuthorAgentID: strings.TrimSpace(r.FormValue("author_agent_id")),
		Kind:          strings.TrimSpace(r.FormValue("kind")),
	}
	req.StructuredData = buildIncidentStructuredDataFromForm(req.Kind, r)
	req.Content = buildIncidentContentFromForm(req.Kind, req.StructuredData, r)
	return req, nil
}

func decodeDistillRequest(r *http.Request) (distillRequest, error) {
	if isJSONRequest(r) {
		var req distillRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return distillRequest{}, err
		}
		return req, nil
	}
	if err := r.ParseForm(); err != nil {
		return distillRequest{}, err
	}
	return distillRequest{
		ChannelID:    strings.TrimSpace(r.FormValue("channel_id")),
		MessageID:    strings.TrimSpace(r.FormValue("message_id")),
		ActorAgentID: strings.TrimSpace(r.FormValue("actor_agent_id")),
		Title:        strings.TrimSpace(r.FormValue("title")),
	}, nil
}

func decodeTaskSyncRequest(r *http.Request) (taskSyncRequest, error) {
	if isJSONRequest(r) {
		var req taskSyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return taskSyncRequest{}, err
		}
		return req, nil
	}
	if err := r.ParseForm(); err != nil {
		return taskSyncRequest{}, err
	}
	return taskSyncRequest{
		ChannelID:    strings.TrimSpace(r.FormValue("channel_id")),
		MessageID:    strings.TrimSpace(r.FormValue("message_id")),
		TaskID:       strings.TrimSpace(r.FormValue("task_id")),
		ActorAgentID: strings.TrimSpace(r.FormValue("actor_agent_id")),
	}, nil
}

func decodeTaskSyncAllRequest(r *http.Request) (taskSyncAllRequest, error) {
	if isJSONRequest(r) {
		var req taskSyncAllRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return taskSyncAllRequest{}, err
		}
		return req, nil
	}
	if err := r.ParseForm(); err != nil {
		return taskSyncAllRequest{}, err
	}
	return taskSyncAllRequest{
		ChannelID:    strings.TrimSpace(r.FormValue("channel_id")),
		ActorAgentID: strings.TrimSpace(r.FormValue("actor_agent_id")),
	}, nil
}

func buildIncidentStructuredDataFromForm(kind string, r *http.Request) map[string]any {
	switch strings.TrimSpace(kind) {
	case "incident":
		return map[string]any{
			"kind":       "incident",
			"title":      strings.TrimSpace(r.FormValue("title")),
			"severity":   strings.TrimSpace(r.FormValue("severity")),
			"task_id":    strings.TrimSpace(r.FormValue("task_id")),
			"summary":    strings.TrimSpace(r.FormValue("summary")),
			"impact":     strings.TrimSpace(r.FormValue("impact")),
			"owner":      strings.TrimSpace(r.FormValue("owner")),
			"next_steps": parseLines(r.FormValue("next_steps")),
		}
	case "update":
		return map[string]any{
			"kind":         "update",
			"title":        strings.TrimSpace(r.FormValue("title")),
			"incident_ref": strings.TrimSpace(r.FormValue("incident_ref")),
			"task_id":      strings.TrimSpace(r.FormValue("task_id")),
			"status":       strings.TrimSpace(r.FormValue("status")),
			"summary":      strings.TrimSpace(r.FormValue("summary")),
			"findings":     parseLines(r.FormValue("findings")),
		}
	case "recovery":
		return map[string]any{
			"kind":         "recovery",
			"title":        strings.TrimSpace(r.FormValue("title")),
			"incident_ref": strings.TrimSpace(r.FormValue("incident_ref")),
			"task_id":      strings.TrimSpace(r.FormValue("task_id")),
			"resolution":   strings.TrimSpace(r.FormValue("resolution")),
			"summary":      strings.TrimSpace(r.FormValue("summary")),
			"followups":    parseLines(r.FormValue("followups")),
		}
	default:
		return nil
	}
}

func buildIncidentContentFromForm(kind string, structuredData map[string]any, r *http.Request) string {
	if content := strings.TrimSpace(r.FormValue("content")); content != "" {
		return content
	}
	if summary := strings.TrimSpace(stringField(structuredData, "summary")); summary != "" {
		return summary
	}
	return incidentRoomStructuredTitle(structuredData)
}

func incidentRoomStructuredTitle(data map[string]any) string {
	if len(data) == 0 {
		return ""
	}
	return strings.TrimSpace(stringField(data, "title"))
}

func incidentRoomSummaryLine(data map[string]any) string {
	if len(data) == 0 {
		return ""
	}
	if summary := strings.TrimSpace(stringField(data, "summary")); summary != "" {
		return summary
	}
	if resolution := strings.TrimSpace(stringField(data, "resolution")); resolution != "" {
		return resolution
	}
	return ""
}

func incidentRoomKindLabel(kind string) string {
	switch strings.TrimSpace(kind) {
	case "incident":
		return "[INCIDENT]"
	case "update":
		return "[UPDATE]"
	case "recovery":
		return "[RECOVERY]"
	default:
		return "[MESSAGE]"
	}
}

func incidentRoomStatusLabel(kind string, distilled bool) string {
	if distilled {
		return "已沉淀"
	}
	switch strings.TrimSpace(kind) {
	case "incident":
		return "处理中"
	case "update":
		return "跟进中"
	case "recovery":
		return "待沉淀"
	default:
		return "进行中"
	}
}

func incidentRoomArtifactLink(teamID, artifactID string) string {
	artifactID = strings.TrimSpace(artifactID)
	if teamID == "" || artifactID == "" {
		return ""
	}
	return fmt.Sprintf("/teams/%s/artifacts/%s", teamID, url.PathEscape(artifactID))
}

func incidentRoomTaskLink(teamID, taskID string) string {
	taskID = strings.TrimSpace(taskID)
	if teamID == "" || taskID == "" {
		return ""
	}
	return fmt.Sprintf("/teams/%s/tasks/%s", teamID, url.PathEscape(taskID))
}

func buildIncidentRoomBatchRuns(teamID, channelID string, history []teamcore.ChangeEvent) []incidentRoomBatchRun {
	channelID = strings.TrimSpace(channelID)
	out := make([]incidentRoomBatchRun, 0, 4)
	for _, event := range history {
		if strings.TrimSpace(event.Scope) != "room" || strings.TrimSpace(event.Action) != "sync" {
			continue
		}
		if strings.TrimSpace(stringMetadata(event.Metadata, "message_scope")) != "incident-room" {
			continue
		}
		if strings.TrimSpace(stringMetadata(event.Metadata, "batch_action")) != "task-sync-all" {
			continue
		}
		if strings.TrimSpace(stringMetadata(event.Metadata, "channel_id")) != channelID {
			continue
		}
		run := incidentRoomBatchRun{
			CreatedAt:             event.CreatedAt,
			ActorAgentID:          strings.TrimSpace(event.ActorAgentID),
			SyncedItems:           intMetadata(event.Metadata, "synced_items"),
			TaskCreated:           intMetadata(event.Metadata, "task_created"),
			ArtifactCreated:       intMetadata(event.Metadata, "artifact_created"),
			TotalMessages:         intMetadata(event.Metadata, "total_messages"),
			SuggestedBlockedCount: intMetadata(event.Metadata, "suggested_blocked_count"),
			SuggestedDoingCount:   intMetadata(event.Metadata, "suggested_doing_count"),
			SuggestedDoneCount:    intMetadata(event.Metadata, "suggested_done_count"),
			HistoryLink:           incidentRoomBatchHistoryLink(teamID, channelID),
			CreatedTaskIDs:        stringSliceMetadata(event.Metadata, "created_task_ids"),
			CreatedArtifactIDs:    stringSliceMetadata(event.Metadata, "created_artifact_ids"),
		}
		run.CreatedTaskLinks = buildIncidentRoomTaskLinks(teamID, run.CreatedTaskIDs)
		run.CreatedArtifactLinks = buildIncidentRoomArtifactLinks(teamID, run.CreatedArtifactIDs)
		out = append(out, run)
	}
	return out
}

func buildIncidentRoomTaskBindings(channelID string, history []teamcore.ChangeEvent) map[string]string {
	channelID = strings.TrimSpace(channelID)
	out := make(map[string]string)
	for _, event := range history {
		if strings.TrimSpace(event.Scope) != "task" {
			continue
		}
		if scope := strings.TrimSpace(stringMetadata(event.Metadata, "message_scope")); scope != "incident-room" {
			continue
		}
		if strings.TrimSpace(stringMetadata(event.Metadata, "channel_id")) != channelID {
			continue
		}
		messageID := strings.TrimSpace(stringMetadata(event.Metadata, "message_id"))
		taskID := strings.TrimSpace(stringMetadata(event.Metadata, "task_id"))
		if messageID == "" || taskID == "" {
			continue
		}
		out[messageID] = taskID
	}
	return out
}

func buildIncidentRoomTaskLinks(teamID string, taskIDs []string) []string {
	out := make([]string, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		if link := incidentRoomTaskLink(teamID, taskID); link != "" {
			out = append(out, link)
		}
	}
	return out
}

func buildIncidentRoomArtifactLinks(teamID string, artifactIDs []string) []string {
	out := make([]string, 0, len(artifactIDs))
	for _, artifactID := range artifactIDs {
		if link := incidentRoomArtifactLink(teamID, artifactID); link != "" {
			out = append(out, link)
		}
	}
	return out
}

func incidentRoomBatchHistoryLink(teamID, channelID string) string {
	channelID = strings.TrimSpace(channelID)
	if teamID == "" || channelID == "" {
		return ""
	}
	return fmt.Sprintf("/teams/%s/history?scope=room&q=%s", teamID, url.QueryEscape(channelID))
}

func limitIncidentRoomBatchRuns(items []incidentRoomBatchRun, limit int) []incidentRoomBatchRun {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return append([]incidentRoomBatchRun(nil), items[:limit]...)
}

func incidentRoomSuggestedTaskStatus(kind string) string {
	switch strings.TrimSpace(kind) {
	case "incident":
		return "blocked"
	case "update":
		return "doing"
	case "recovery":
		return "done"
	default:
		return "open"
	}
}

func loadIncidentRoomMessage(ctx context.Context, store *teamcore.Store, teamID, channelID, messageID string) (*teamcore.Message, error) {
	messages, err := store.LoadAllMessagesCtx(ctx, teamID, channelID)
	if err != nil {
		return nil, err
	}
	for i := range messages {
		if messages[i].MessageID == messageID && isIncidentRoomKind(messages[i].MessageType) {
			return &messages[i], nil
		}
	}
	return nil, errors.New("incident-room message not found")
}

func syncIncidentRoomTask(ctx context.Context, store *teamcore.Store, teamID, channelID string, msg teamcore.Message, requestedTaskID, actorAgentID string) (string, string, bool, error) {
	taskID := firstNonEmpty(requestedTaskID, strings.TrimSpace(stringField(msg.StructuredData, "task_id")))
	status := incidentRoomSuggestedTaskStatus(msg.MessageType)
	taskCreated := false
	if taskID == "" {
		if err := requireAction(store, teamID, actorAgentID, "task.create"); err != nil {
			return "", "", false, err
		}
		now := time.Now().UTC()
		task := teamcore.Task{
			TeamID:      teamID,
			ChannelID:   channelID,
			Title:       firstNonEmpty(incidentRoomStructuredTitle(msg.StructuredData), msg.Content),
			Description: incidentRoomSummaryLine(msg.StructuredData),
			CreatedBy:   actorAgentID,
			Status:      status,
			Labels:      []string{"incident-room", strings.TrimSpace(msg.MessageType)},
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := store.AppendTaskCtx(ctx, teamID, task); err != nil {
			return "", "", false, err
		}
		tasks, err := store.LoadTasksCtx(ctx, teamID, 1)
		if err != nil || len(tasks) == 0 {
			return "", "", false, errors.New("failed to load created incident task")
		}
		taskID = tasks[0].TaskID
		taskCreated = true
		_ = store.AppendHistoryCtx(ctx, teamID, teamcore.ChangeEvent{
			TeamID:    teamID,
			Scope:     "task",
			Action:    "create",
			SubjectID: taskID,
			Summary:   "从 incident-room 自动创建绑定任务",
			Metadata: map[string]any{
				"channel_id":    channelID,
				"message_id":    msg.MessageID,
				"task_id":       taskID,
				"actor_agent":   actorAgentID,
				"message_scope": "incident-room",
				"auto_sync":     true,
				"status":        status,
			},
			CreatedAt: now,
		})
	}
	if taskID != "" {
		if err := requireAction(store, teamID, actorAgentID, "task.update"); err != nil {
			return "", "", false, err
		}
		task, err := store.LoadTaskCtx(ctx, teamID, taskID)
		if err != nil {
			return "", "", false, err
		}
		if strings.TrimSpace(task.Status) != status {
			task.Status = status
			task.UpdatedAt = time.Now().UTC()
			if err := store.SaveTaskCtx(ctx, teamID, task); err != nil {
				return "", "", false, err
			}
			_ = store.AppendHistoryCtx(ctx, teamID, teamcore.ChangeEvent{
				TeamID:    teamID,
				Scope:     "task",
				Action:    "update",
				SubjectID: taskID,
				Summary:   "从 incident-room 自动同步任务状态",
				Metadata: map[string]any{
					"channel_id":    channelID,
					"message_id":    msg.MessageID,
					"task_id":       taskID,
					"actor_agent":   actorAgentID,
					"message_scope": "incident-room",
					"auto_sync":     true,
					"status":        status,
				},
				CreatedAt: task.UpdatedAt,
			})
		}
	}
	return taskID, status, taskCreated, nil
}

func ensureIncidentRoomArtifact(ctx context.Context, store *teamcore.Store, teamID, channelID string, msg teamcore.Message, actorAgentID, requestedTitle string) (teamcore.Artifact, bool, error) {
	artifacts, err := store.LoadArtifactsCtx(ctx, teamID, 200)
	if err != nil {
		return teamcore.Artifact{}, false, err
	}
	if artifact, ok := incidentRoomArtifactByMessageIDFromSlice(artifacts, channelID, msg.MessageID); ok {
		return artifact, false, nil
	}
	title := strings.TrimSpace(requestedTitle)
	if title == "" {
		title = incidentRoomStructuredTitle(msg.StructuredData)
		if title == "" {
			title = strings.TrimSpace(msg.Content)
		}
		if len(title) > 80 {
			title = title[:80]
		}
	}
	content, _ := json.MarshalIndent(msg.StructuredData, "", "  ")
	artifact := teamcore.Artifact{
		TeamID:    teamID,
		ChannelID: channelID,
		TaskID:    strings.TrimSpace(stringField(msg.StructuredData, "task_id")),
		Title:     title,
		Kind:      "incident-summary",
		Summary:   msg.Content,
		Content:   string(content),
		CreatedBy: actorAgentID,
		Labels:    []string{"distilled", "incident-room", "source-message:" + msg.MessageID},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if strings.TrimSpace(artifact.ArtifactID) == "" {
		artifact.ArtifactID = incidentRoomBuildArtifactID(artifact)
	}
	if err := store.AppendArtifactCtx(ctx, teamID, artifact); err != nil {
		return teamcore.Artifact{}, false, err
	}
	artifacts, err = store.LoadArtifactsCtx(ctx, teamID, 200)
	if err != nil {
		return teamcore.Artifact{}, false, err
	}
	if candidate, ok := incidentRoomArtifactByMessageIDFromSlice(artifacts, channelID, msg.MessageID); ok {
		artifact = candidate
	}
	_ = store.AppendHistoryCtx(ctx, teamID, teamcore.ChangeEvent{
		TeamID:    teamID,
		Scope:     "artifact",
		Action:    "create",
		SubjectID: artifact.ArtifactID,
		Summary:   "提炼 incident-room 消息为 Artifact",
		Metadata: map[string]any{
			"channel_id":      channelID,
			"artifact_kind":   "incident-summary",
			"source_message":  msg.MessageID,
			"artifact_source": "incident-room",
		},
		CreatedAt: artifact.CreatedAt,
	})
	return artifact, true, nil
}

func incidentRoomArtifactByMessageID(ctx context.Context, store *teamcore.Store, teamID, channelID, messageID string) (teamcore.Artifact, bool) {
	artifacts, err := store.LoadArtifactsCtx(ctx, teamID, 200)
	if err != nil {
		return teamcore.Artifact{}, false
	}
	return incidentRoomArtifactByMessageIDFromSlice(artifacts, channelID, messageID)
}

func incidentRoomArtifactByMessageIDFromSlice(artifacts []teamcore.Artifact, channelID, messageID string) (teamcore.Artifact, bool) {
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.ChannelID) != strings.TrimSpace(channelID) {
			continue
		}
		if strings.TrimSpace(artifact.Kind) != "incident-summary" {
			continue
		}
		for _, label := range artifact.Labels {
			if strings.TrimSpace(label) == "source-message:"+messageID {
				return artifact, true
			}
		}
	}
	return teamcore.Artifact{}, false
}

func isIncidentRoomKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "incident", "update", "recovery":
		return true
	default:
		return false
	}
}

func parseLines(raw string) []string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	parts := strings.Split(raw, "\n")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func stringField(data map[string]any, key string) string {
	if len(data) == 0 {
		return ""
	}
	if value, ok := data[key]; ok {
		return strings.TrimSpace(fmt.Sprint(value))
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func incidentRoomBuildArtifactID(artifact teamcore.Artifact) string {
	return strings.Join([]string{
		strings.TrimSpace(artifact.TeamID),
		strings.TrimSpace(artifact.CreatedBy),
		artifact.CreatedAt.UTC().Format(time.RFC3339Nano),
		strings.TrimSpace(artifact.Title),
	}, ":")
}

func stringMetadata(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, _ := metadata[key]
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func intMetadata(metadata map[string]any, key string) int {
	if len(metadata) == 0 {
		return 0
	}
	switch value := metadata[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		n, _ := value.Int64()
		return int(n)
	case string:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(value), "%d", &parsed)
		return parsed
	default:
		return 0
	}
}

func stringSliceMetadata(metadata map[string]any, key string) []string {
	if len(metadata) == 0 {
		return nil
	}
	raw, ok := metadata[key]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func formatStructuredJSON(value map[string]any) string {
	if len(value) == 0 {
		return ""
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(body)
}

func isJSONRequest(r *http.Request) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))), "application/json")
}

func isAPIRequest(r *http.Request) bool {
	requestURI := strings.TrimSpace(r.RequestURI)
	return strings.HasPrefix(requestURI, "/api/teams/")
}

func requestTrusted(r *http.Request) bool {
	addr := clientIP(r)
	return addr.IsValid() && (addr.IsLoopback() || addr.IsPrivate())
}

func clientIP(r *http.Request) netip.Addr {
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

func requireAction(store *teamcore.Store, teamID, actorAgentID, action string) error {
	return teamcore.RequireAction(context.Background(), store, teamID, actorAgentID, action)
}

func actorRole(store *teamcore.Store, teamID, actorAgentID string, info teamcore.Info) (string, error) {
	actorAgentID = strings.TrimSpace(actorAgentID)
	if actorAgentID == "" {
		return "", errors.New("empty actor_agent_id")
	}
	members, err := store.LoadMembersCtx(context.Background(), teamID)
	if err == nil {
		for _, member := range members {
			if strings.TrimSpace(member.AgentID) == actorAgentID {
				return member.Role, nil
			}
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if actorAgentID == strings.TrimSpace(info.OwnerAgentID) {
		return "owner", nil
	}
	return "", fmt.Errorf("team actor %q not found", actorAgentID)
}
