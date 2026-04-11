package artifactroom

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

type artifactRoomCardView struct {
	MessageID       string
	Kind            string
	KindLabel       string
	Title           string
	Content         string
	Summary         string
	Owner           string
	ArtifactKind    string
	TaskID          string
	TaskLink        string
	AuthorAgentID   string
	CreatedAt       time.Time
	StructuredJSON  string
	Distilled       bool
	DistillArtifact string
	DistillTitle    string
}

type artifactRoomSummary struct {
	TeamID              string                 `json:"team_id"`
	ChannelID           string                 `json:"channel_id"`
	ProposalCount       int                    `json:"proposal_count"`
	RevisionCount       int                    `json:"revision_count"`
	PublishCount        int                    `json:"publish_count"`
	DistilledCount      int                    `json:"distilled_count"`
	BoundTaskCount      int                    `json:"bound_task_count"`
	UnboundTaskCount    int                    `json:"unbound_task_count"`
	SuggestedDoingCount int                    `json:"suggested_doing_count"`
	SuggestedDoneCount  int                    `json:"suggested_done_count"`
	RecentBatchRuns     []artifactRoomBatchRun `json:"recent_batch_runs,omitempty"`
	Cards               []artifactRoomCardView `json:"cards,omitempty"`
}

type artifactRoomBatchRun struct {
	CreatedAt            time.Time `json:"created_at"`
	ActorAgentID         string    `json:"actor_agent_id,omitempty"`
	SyncedItems          int       `json:"synced_items"`
	TaskCreated          int       `json:"task_created"`
	ArtifactCreated      int       `json:"artifact_created"`
	TotalMessages        int       `json:"total_messages"`
	SuggestedDoingCount  int       `json:"suggested_doing_count"`
	SuggestedDoneCount   int       `json:"suggested_done_count"`
	HistoryLink          string    `json:"history_link,omitempty"`
	CreatedTaskIDs       []string  `json:"created_task_ids,omitempty"`
	CreatedArtifactIDs   []string  `json:"created_artifact_ids,omitempty"`
	CreatedTaskLinks     []string  `json:"created_task_links,omitempty"`
	CreatedArtifactLinks []string  `json:"created_artifact_links,omitempty"`
}

type artifactRoomPageData struct {
	TeamID              string
	ChannelID           string
	FilterKind          string
	ActorAgentID        string
	Notice              string
	Cards               []artifactRoomCardView
	ProposalCount       int
	RevisionCount       int
	PublishCount        int
	DistilledCount      int
	BoundTaskCount      int
	UnboundTaskCount    int
	SuggestedDoingCount int
	SuggestedDoneCount  int
	RecentBatchRuns     []artifactRoomBatchRun
	ArtifactLink        string
	HistoryLink         string
}

type postArtifactRoomRequest struct {
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
		handleListArtifactRoom(store, teamID, w, r)
	})
	mux.HandleFunc("/summary", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleArtifactRoomSummary(store, teamID, w, r)
	})
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handlePostArtifactRoomMessage(store, teamID, w, r)
	})
	mux.HandleFunc("/distill", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleDistillArtifactRoom(store, teamID, w, r)
	})
	mux.HandleFunc("/task-sync", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleSyncArtifactRoomTask(store, teamID, w, r)
	})
	mux.HandleFunc("/task-sync-all", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleSyncAllArtifactRoomTasks(store, teamID, w, r)
	})
	return mux
}

func handleListArtifactRoom(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
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
	filtered := filterArtifactRoomMessages(messages, kind)
	if isAPIRequest(r) {
		if filtered == nil {
			filtered = []teamcore.Message{}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(filtered)
		return
	}
	data, err := buildArtifactRoomPageData(r.Context(), store, teamID, channelID, actorAgentID, kind, strings.TrimSpace(r.URL.Query().Get("notice")), filtered, history)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := renderArtifactRoomPage(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleArtifactRoomSummary(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
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
	data, err := buildArtifactRoomPageData(r.Context(), store, teamID, channelID, "", "", "", filterArtifactRoomMessages(messages, ""), history)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(artifactRoomSummary{
		TeamID:              teamID,
		ChannelID:           channelID,
		ProposalCount:       data.ProposalCount,
		RevisionCount:       data.RevisionCount,
		PublishCount:        data.PublishCount,
		DistilledCount:      data.DistilledCount,
		BoundTaskCount:      data.BoundTaskCount,
		UnboundTaskCount:    data.UnboundTaskCount,
		SuggestedDoingCount: data.SuggestedDoingCount,
		SuggestedDoneCount:  data.SuggestedDoneCount,
		RecentBatchRuns:     limitArtifactRoomBatchRuns(data.RecentBatchRuns, 4),
		Cards:               data.Cards,
	})
}

func handlePostArtifactRoomMessage(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !requestTrusted(r) {
		http.Error(w, "artifact-room write is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	req, err := decodePostArtifactRoomRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	validKinds := map[string]bool{"proposal": true, "revision": true, "publish": true}
	if !validKinds[req.Kind] {
		http.Error(w, "kind must be proposal, revision, or publish", http.StatusBadRequest)
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
		req.Content = artifactRoomStructuredTitle(req.StructuredData)
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
		Summary:   "发送 artifact-room 消息",
		Metadata: map[string]any{
			"channel_id":    req.ChannelID,
			"message_type":  req.Kind,
			"author_agent":  req.AuthorAgentID,
			"message_scope": "artifact-room",
		},
		CreatedAt: msg.CreatedAt,
	})
	if isAPIRequest(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "created", "kind": req.Kind})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%s/r/artifact-room/?channel_id=%s&kind=%s&actor_agent_id=%s&notice=created",
		teamID, req.ChannelID, url.QueryEscape(req.Kind), url.QueryEscape(req.AuthorAgentID)), http.StatusSeeOther)
}

func handleDistillArtifactRoom(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !requestTrusted(r) {
		http.Error(w, "artifact-room distill is limited to local or LAN requests", http.StatusForbidden)
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
	roomMsg, err := loadArtifactRoomMessage(r.Context(), store, teamID, req.ChannelID, req.MessageID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	artifact, _, err := ensureArtifactRoomArtifact(r.Context(), store, teamID, req.ChannelID, *roomMsg, req.ActorAgentID, req.Title)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if isAPIRequest(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "distilled", "artifact_kind": "artifact-brief", "artifact_id": artifact.ArtifactID})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%s/r/artifact-room/?channel_id=%s&actor_agent_id=%s&notice=distilled",
		teamID, req.ChannelID, url.QueryEscape(req.ActorAgentID)), http.StatusSeeOther)
}

func handleSyncArtifactRoomTask(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !requestTrusted(r) {
		http.Error(w, "artifact-room task sync is limited to local or LAN requests", http.StatusForbidden)
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
	msg, err := loadArtifactRoomMessage(r.Context(), store, teamID, req.ChannelID, req.MessageID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	taskID, taskStatus, taskCreated, err := syncArtifactRoomTask(r.Context(), store, teamID, req.ChannelID, *msg, req.TaskID, req.ActorAgentID)
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
	http.Redirect(w, r, fmt.Sprintf("/teams/%s/r/artifact-room/?channel_id=%s&actor_agent_id=%s&notice=task-synced",
		teamID, req.ChannelID, url.QueryEscape(req.ActorAgentID)), http.StatusSeeOther)
}

func handleSyncAllArtifactRoomTasks(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !requestTrusted(r) {
		http.Error(w, "artifact-room batch sync is limited to local or LAN requests", http.StatusForbidden)
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
	filtered := filterArtifactRoomMessages(messages, "")
	synced, taskCreated, artifactCreated := 0, 0, 0
	suggestedDoingCount, suggestedDoneCount := 0, 0
	createdTaskIDs := make([]string, 0, len(filtered))
	createdArtifactIDs := make([]string, 0, len(filtered))
	for _, msg := range filtered {
		switch artifactRoomSuggestedTaskStatus(msg.MessageType) {
		case "doing":
			suggestedDoingCount++
		case "done":
			suggestedDoneCount++
		}
		taskID, _, created, err := syncArtifactRoomTask(r.Context(), store, teamID, req.ChannelID, msg, strings.TrimSpace(stringField(msg.StructuredData, "task_id")), req.ActorAgentID)
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
		if strings.TrimSpace(msg.MessageType) == "publish" {
			artifact, createdArtifact, err := ensureArtifactRoomArtifact(r.Context(), store, teamID, req.ChannelID, msg, req.ActorAgentID, "")
			if err == nil && createdArtifact {
				artifactCreated++
				if strings.TrimSpace(artifact.ArtifactID) == "" {
					if recovered, ok := artifactRoomArtifactByMessageID(r.Context(), store, teamID, req.ChannelID, msg.MessageID); ok {
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
		Summary:      "批量同步 artifact-room 到任务主链",
		ActorAgentID: req.ActorAgentID,
		Source:       "artifact-room",
		Metadata: map[string]any{
			"channel_id":            req.ChannelID,
			"message_scope":         "artifact-room",
			"batch_action":          "task-sync-all",
			"synced_items":          synced,
			"task_created":          taskCreated,
			"artifact_created":      artifactCreated,
			"total_messages":        len(filtered),
			"suggested_doing_count": suggestedDoingCount,
			"suggested_done_count":  suggestedDoneCount,
			"created_task_ids":      createdTaskIDs,
			"created_artifact_ids":  createdArtifactIDs,
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
	http.Redirect(w, r, fmt.Sprintf("/teams/%s/r/artifact-room/?channel_id=%s&actor_agent_id=%s&notice=tasks-synced",
		teamID, req.ChannelID, url.QueryEscape(req.ActorAgentID)), http.StatusSeeOther)
}

func filterArtifactRoomMessages(messages []teamcore.Message, kind string) []teamcore.Message {
	filtered := make([]teamcore.Message, 0, len(messages))
	for _, msg := range messages {
		msgType := strings.TrimSpace(msg.MessageType)
		if !isArtifactRoomKind(msgType) {
			continue
		}
		if kind != "" && msgType != kind {
			continue
		}
		filtered = append(filtered, msg)
	}
	return filtered
}

func buildArtifactRoomPageData(ctx context.Context, store *teamcore.Store, teamID, channelID, actorAgentID, kind, notice string, messages []teamcore.Message, history []teamcore.ChangeEvent) (artifactRoomPageData, error) {
	artifacts, err := store.LoadArtifactsCtx(ctx, teamID, 200)
	if err != nil {
		return artifactRoomPageData{}, err
	}
	taskBindings := buildArtifactRoomTaskBindings(channelID, history)
	distilledByMessage := map[string]teamcore.Artifact{}
	for _, artifact := range artifacts {
		for _, label := range artifact.Labels {
			if strings.HasPrefix(label, "source-message:") {
				distilledByMessage[strings.TrimPrefix(label, "source-message:")] = artifact
			}
		}
	}
	cards := make([]artifactRoomCardView, 0, len(messages))
	proposalCount, revisionCount, publishCount, distilledCount := 0, 0, 0, 0
	boundTaskCount, unboundTaskCount := 0, 0
	suggestedDoingCount, suggestedDoneCount := 0, 0
	for _, msg := range messages {
		distilledArtifact, distilled := distilledByMessage[msg.MessageID]
		if distilled {
			distilledCount++
		}
		switch strings.TrimSpace(msg.MessageType) {
		case "proposal":
			proposalCount++
		case "revision":
			revisionCount++
		case "publish":
			publishCount++
		}
		taskID := firstNonEmpty(strings.TrimSpace(stringField(msg.StructuredData, "task_id")), taskBindings[msg.MessageID])
		if taskID != "" {
			boundTaskCount++
		} else {
			unboundTaskCount++
		}
		switch artifactRoomSuggestedTaskStatus(msg.MessageType) {
		case "doing":
			suggestedDoingCount++
		case "done":
			suggestedDoneCount++
		}
		cards = append(cards, artifactRoomCardView{
			MessageID:       msg.MessageID,
			Kind:            msg.MessageType,
			KindLabel:       artifactRoomKindLabel(msg.MessageType),
			Title:           artifactRoomStructuredTitle(msg.StructuredData),
			Content:         msg.Content,
			Summary:         artifactRoomSummaryLine(msg.StructuredData),
			Owner:           strings.TrimSpace(stringField(msg.StructuredData, "owner")),
			ArtifactKind:    strings.TrimSpace(stringField(msg.StructuredData, "artifact_kind")),
			TaskID:          taskID,
			TaskLink:        artifactRoomTaskLink(teamID, taskID),
			AuthorAgentID:   msg.AuthorAgentID,
			CreatedAt:       msg.CreatedAt,
			StructuredJSON:  formatStructuredJSON(msg.StructuredData),
			Distilled:       distilled,
			DistillArtifact: artifactRoomArtifactLink(teamID, distilledArtifact.ArtifactID),
			DistillTitle:    distilledArtifact.Title,
		})
	}
	return artifactRoomPageData{
		TeamID:              teamID,
		ChannelID:           channelID,
		FilterKind:          kind,
		ActorAgentID:        actorAgentID,
		Notice:              notice,
		Cards:               cards,
		ProposalCount:       proposalCount,
		RevisionCount:       revisionCount,
		PublishCount:        publishCount,
		DistilledCount:      distilledCount,
		BoundTaskCount:      boundTaskCount,
		UnboundTaskCount:    unboundTaskCount,
		SuggestedDoingCount: suggestedDoingCount,
		SuggestedDoneCount:  suggestedDoneCount,
		RecentBatchRuns:     limitArtifactRoomBatchRuns(buildArtifactRoomBatchRuns(teamID, channelID, history), 4),
		ArtifactLink:        fmt.Sprintf("/teams/%s/artifacts?channel=%s&kind=artifact-brief", teamID, url.QueryEscape(channelID)),
		HistoryLink:         fmt.Sprintf("/teams/%s/history?scope=message", teamID),
	}, nil
}

func renderArtifactRoomPage(w http.ResponseWriter, data artifactRoomPageData) error {
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

func decodePostArtifactRoomRequest(r *http.Request) (postArtifactRoomRequest, error) {
	if isJSONRequest(r) {
		var req postArtifactRoomRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return postArtifactRoomRequest{}, err
		}
		return req, nil
	}
	if err := r.ParseForm(); err != nil {
		return postArtifactRoomRequest{}, err
	}
	req := postArtifactRoomRequest{
		ChannelID:     strings.TrimSpace(r.FormValue("channel_id")),
		AuthorAgentID: strings.TrimSpace(r.FormValue("author_agent_id")),
		Kind:          strings.TrimSpace(r.FormValue("kind")),
	}
	req.StructuredData = buildArtifactStructuredDataFromForm(req.Kind, r)
	req.Content = buildArtifactContentFromForm(req.Kind, req.StructuredData, r)
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

func buildArtifactStructuredDataFromForm(kind string, r *http.Request) map[string]any {
	switch strings.TrimSpace(kind) {
	case "proposal":
		return map[string]any{
			"kind":          "proposal",
			"title":         strings.TrimSpace(r.FormValue("title")),
			"artifact_kind": strings.TrimSpace(r.FormValue("artifact_kind")),
			"task_id":       strings.TrimSpace(r.FormValue("task_id")),
			"owner":         strings.TrimSpace(r.FormValue("owner")),
			"summary":       strings.TrimSpace(r.FormValue("summary")),
			"outline":       parseLines(r.FormValue("outline")),
		}
	case "revision":
		return map[string]any{
			"kind":          "revision",
			"title":         strings.TrimSpace(r.FormValue("title")),
			"artifact_kind": strings.TrimSpace(r.FormValue("artifact_kind")),
			"task_id":       strings.TrimSpace(r.FormValue("task_id")),
			"owner":         strings.TrimSpace(r.FormValue("owner")),
			"summary":       strings.TrimSpace(r.FormValue("summary")),
			"changes":       parseLines(r.FormValue("changes")),
		}
	case "publish":
		return map[string]any{
			"kind":          "publish",
			"title":         strings.TrimSpace(r.FormValue("title")),
			"artifact_kind": strings.TrimSpace(r.FormValue("artifact_kind")),
			"task_id":       strings.TrimSpace(r.FormValue("task_id")),
			"owner":         strings.TrimSpace(r.FormValue("owner")),
			"summary":       strings.TrimSpace(r.FormValue("summary")),
			"result":        strings.TrimSpace(r.FormValue("result")),
			"followups":     parseLines(r.FormValue("followups")),
		}
	default:
		return nil
	}
}

func buildArtifactContentFromForm(kind string, structuredData map[string]any, r *http.Request) string {
	if content := strings.TrimSpace(r.FormValue("content")); content != "" {
		return content
	}
	if summary := strings.TrimSpace(stringField(structuredData, "summary")); summary != "" {
		return summary
	}
	return artifactRoomStructuredTitle(structuredData)
}

func artifactRoomStructuredTitle(data map[string]any) string {
	return strings.TrimSpace(stringField(data, "title"))
}
func artifactRoomSummaryLine(data map[string]any) string {
	return strings.TrimSpace(stringField(data, "summary"))
}

func artifactRoomKindLabel(kind string) string {
	switch strings.TrimSpace(kind) {
	case "proposal":
		return "[PROPOSAL]"
	case "revision":
		return "[REVISION]"
	case "publish":
		return "[PUBLISH]"
	default:
		return "[MESSAGE]"
	}
}

func artifactRoomArtifactLink(teamID, artifactID string) string {
	artifactID = strings.TrimSpace(artifactID)
	if teamID == "" || artifactID == "" {
		return ""
	}
	return fmt.Sprintf("/teams/%s/artifacts/%s", teamID, url.PathEscape(artifactID))
}

func artifactRoomTaskLink(teamID, taskID string) string {
	taskID = strings.TrimSpace(taskID)
	if teamID == "" || taskID == "" {
		return ""
	}
	return fmt.Sprintf("/teams/%s/tasks/%s", teamID, url.PathEscape(taskID))
}

func buildArtifactRoomBatchRuns(teamID, channelID string, history []teamcore.ChangeEvent) []artifactRoomBatchRun {
	channelID = strings.TrimSpace(channelID)
	out := make([]artifactRoomBatchRun, 0, 4)
	for _, event := range history {
		if strings.TrimSpace(event.Scope) != "room" || strings.TrimSpace(event.Action) != "sync" {
			continue
		}
		if strings.TrimSpace(stringMetadata(event.Metadata, "message_scope")) != "artifact-room" {
			continue
		}
		if strings.TrimSpace(stringMetadata(event.Metadata, "batch_action")) != "task-sync-all" {
			continue
		}
		if strings.TrimSpace(stringMetadata(event.Metadata, "channel_id")) != channelID {
			continue
		}
		run := artifactRoomBatchRun{
			CreatedAt:           event.CreatedAt,
			ActorAgentID:        strings.TrimSpace(event.ActorAgentID),
			SyncedItems:         intMetadata(event.Metadata, "synced_items"),
			TaskCreated:         intMetadata(event.Metadata, "task_created"),
			ArtifactCreated:     intMetadata(event.Metadata, "artifact_created"),
			TotalMessages:       intMetadata(event.Metadata, "total_messages"),
			SuggestedDoingCount: intMetadata(event.Metadata, "suggested_doing_count"),
			SuggestedDoneCount:  intMetadata(event.Metadata, "suggested_done_count"),
			HistoryLink:         artifactRoomBatchHistoryLink(teamID, channelID),
			CreatedTaskIDs:      stringSliceMetadata(event.Metadata, "created_task_ids"),
			CreatedArtifactIDs:  stringSliceMetadata(event.Metadata, "created_artifact_ids"),
		}
		run.CreatedTaskLinks = buildArtifactRoomTaskLinks(teamID, run.CreatedTaskIDs)
		run.CreatedArtifactLinks = buildArtifactRoomArtifactLinks(teamID, run.CreatedArtifactIDs)
		out = append(out, run)
	}
	return out
}

func buildArtifactRoomTaskBindings(channelID string, history []teamcore.ChangeEvent) map[string]string {
	channelID = strings.TrimSpace(channelID)
	out := make(map[string]string)
	for _, event := range history {
		if strings.TrimSpace(event.Scope) != "task" {
			continue
		}
		if scope := strings.TrimSpace(stringMetadata(event.Metadata, "message_scope")); scope != "artifact-room" {
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

func buildArtifactRoomTaskLinks(teamID string, taskIDs []string) []string {
	out := make([]string, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		if link := artifactRoomTaskLink(teamID, taskID); link != "" {
			out = append(out, link)
		}
	}
	return out
}

func buildArtifactRoomArtifactLinks(teamID string, artifactIDs []string) []string {
	out := make([]string, 0, len(artifactIDs))
	for _, artifactID := range artifactIDs {
		if link := artifactRoomArtifactLink(teamID, artifactID); link != "" {
			out = append(out, link)
		}
	}
	return out
}

func artifactRoomBatchHistoryLink(teamID, channelID string) string {
	channelID = strings.TrimSpace(channelID)
	if teamID == "" || channelID == "" {
		return ""
	}
	return fmt.Sprintf("/teams/%s/history?scope=room&q=%s", teamID, url.QueryEscape(channelID))
}

func limitArtifactRoomBatchRuns(items []artifactRoomBatchRun, limit int) []artifactRoomBatchRun {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return append([]artifactRoomBatchRun(nil), items[:limit]...)
}

func artifactRoomSuggestedTaskStatus(kind string) string {
	switch strings.TrimSpace(kind) {
	case "proposal", "revision":
		return "doing"
	case "publish":
		return "done"
	default:
		return "open"
	}
}

func loadArtifactRoomMessage(ctx context.Context, store *teamcore.Store, teamID, channelID, messageID string) (*teamcore.Message, error) {
	messages, err := store.LoadAllMessagesCtx(ctx, teamID, channelID)
	if err != nil {
		return nil, err
	}
	for i := range messages {
		if messages[i].MessageID == messageID && isArtifactRoomKind(messages[i].MessageType) {
			return &messages[i], nil
		}
	}
	return nil, errors.New("artifact-room message not found")
}

func ensureArtifactRoomArtifact(ctx context.Context, store *teamcore.Store, teamID, channelID string, msg teamcore.Message, actorAgentID, requestedTitle string) (teamcore.Artifact, bool, error) {
	artifacts, err := store.LoadArtifactsCtx(ctx, teamID, 200)
	if err != nil {
		return teamcore.Artifact{}, false, err
	}
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.ChannelID) != strings.TrimSpace(channelID) || strings.TrimSpace(artifact.Kind) != "artifact-brief" {
			continue
		}
		for _, label := range artifact.Labels {
			if strings.TrimSpace(label) == "source-message:"+msg.MessageID {
				return artifact, false, nil
			}
		}
	}
	title := strings.TrimSpace(requestedTitle)
	if title == "" {
		title = artifactRoomStructuredTitle(msg.StructuredData)
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
		Kind:      "artifact-brief",
		Summary:   msg.Content,
		Content:   string(content),
		CreatedBy: actorAgentID,
		Labels:    []string{"distilled", "artifact-room", "source-message:" + msg.MessageID},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if strings.TrimSpace(artifact.ArtifactID) == "" {
		artifact.ArtifactID = buildRoomArtifactID(artifact)
	}
	if err := store.AppendArtifactCtx(ctx, teamID, artifact); err != nil {
		return teamcore.Artifact{}, false, err
	}
	_ = store.AppendHistoryCtx(ctx, teamID, teamcore.ChangeEvent{
		TeamID:    teamID,
		Scope:     "artifact",
		Action:    "create",
		SubjectID: artifact.ArtifactID,
		Summary:   "提炼 artifact-room 消息为 Artifact",
		Metadata: map[string]any{
			"channel_id":      channelID,
			"artifact_kind":   "artifact-brief",
			"source_message":  msg.MessageID,
			"artifact_source": "artifact-room",
		},
		CreatedAt: artifact.CreatedAt,
	})
	return artifact, true, nil
}

func artifactRoomArtifactByMessageID(ctx context.Context, store *teamcore.Store, teamID, channelID, messageID string) (teamcore.Artifact, bool) {
	artifacts, err := store.LoadArtifactsCtx(ctx, teamID, 200)
	if err != nil {
		return teamcore.Artifact{}, false
	}
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.ChannelID) != strings.TrimSpace(channelID) {
			continue
		}
		if strings.TrimSpace(artifact.Kind) != "artifact-brief" {
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

func syncArtifactRoomTask(ctx context.Context, store *teamcore.Store, teamID, channelID string, msg teamcore.Message, requestedTaskID, actorAgentID string) (string, string, bool, error) {
	taskID := firstNonEmpty(requestedTaskID, strings.TrimSpace(stringField(msg.StructuredData, "task_id")))
	status := artifactRoomSuggestedTaskStatus(msg.MessageType)
	taskCreated := false
	if taskID == "" {
		if err := requireAction(store, teamID, actorAgentID, "task.create"); err != nil {
			return "", "", false, err
		}
		now := time.Now().UTC()
		task := teamcore.Task{
			TeamID:      teamID,
			ChannelID:   channelID,
			Title:       firstNonEmpty(artifactRoomStructuredTitle(msg.StructuredData), msg.Content),
			Description: artifactRoomSummaryLine(msg.StructuredData),
			CreatedBy:   actorAgentID,
			Status:      status,
			Labels:      []string{"artifact-room", strings.TrimSpace(msg.MessageType)},
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := store.AppendTaskCtx(ctx, teamID, task); err != nil {
			return "", "", false, err
		}
		tasks, err := store.LoadTasksCtx(ctx, teamID, 1)
		if err != nil || len(tasks) == 0 {
			return "", "", false, errors.New("failed to load created artifact task")
		}
		taskID = tasks[0].TaskID
		taskCreated = true
		_ = store.AppendHistoryCtx(ctx, teamID, teamcore.ChangeEvent{
			TeamID:    teamID,
			Scope:     "task",
			Action:    "create",
			SubjectID: taskID,
			Summary:   "从 artifact-room 自动创建绑定任务",
			Metadata: map[string]any{
				"channel_id":    channelID,
				"message_id":    msg.MessageID,
				"task_id":       taskID,
				"actor_agent":   actorAgentID,
				"message_scope": "artifact-room",
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
				Summary:   "从 artifact-room 自动同步任务状态",
				Metadata: map[string]any{
					"channel_id":    channelID,
					"message_id":    msg.MessageID,
					"task_id":       taskID,
					"actor_agent":   actorAgentID,
					"message_scope": "artifact-room",
					"auto_sync":     true,
					"status":        status,
				},
				CreatedAt: task.UpdatedAt,
			})
		}
	}
	return taskID, status, taskCreated, nil
}

func isArtifactRoomKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "proposal", "revision", "publish":
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

func buildRoomArtifactID(artifact teamcore.Artifact) string {
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
	return strings.TrimSpace(fmt.Sprint(metadata[key]))
}

func intMetadata(metadata map[string]any, key string) int {
	if len(metadata) == 0 {
		return 0
	}
	switch value := metadata[key].(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		n, _ := value.Int64()
		return int(n)
	default:
		text := strings.TrimSpace(fmt.Sprint(metadata[key]))
		if text == "" {
			return 0
		}
		var out int
		_, _ = fmt.Sscanf(text, "%d", &out)
		return out
	}
}

func stringSliceMetadata(metadata map[string]any, key string) []string {
	if len(metadata) == 0 {
		return nil
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
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
