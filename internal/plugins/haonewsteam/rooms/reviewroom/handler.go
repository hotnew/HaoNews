package reviewroom

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
	"strings"
	"time"

	teamcore "hao.news/internal/haonews/team"
)

//go:embed templates/*.html
var templatesFS embed.FS

type reviewRoomCardView struct {
	MessageID          string
	Kind               string
	KindLabel          string
	Title              string
	Content            string
	Summary            string
	DetailLabel        string
	Detail             string
	ItemsLabel         string
	Items              []string
	Distilled          bool
	StatusLabel        string
	StatusGroup        string
	DecisionRef        string
	TaskID             string
	TaskLink           string
	LinkedArtifactID   string
	LinkedArtifactLink string
	AuthorAgentID      string
	CreatedAt          time.Time
	ArtifactTitle      string
	ArtifactLink       string
	StructuredJSON     string
}

type reviewRoomPageData struct {
	TeamID              string
	ChannelID           string
	FilterKind          string
	ActorAgentID        string
	Notice              string
	Cards               []reviewRoomCardView
	MessageCount        int
	ReviewCount         int
	RiskCount           int
	DecisionCount       int
	DistilledCount      int
	OpenDecisionCount   int
	OpenRiskCount       int
	ArtifactLink        string
	HistoryLink         string
	DecisionCards       []reviewRoomCardView
	RiskCards           []reviewRoomCardView
	ReviewCards         []reviewRoomCardView
	OpenDecisionCards   []reviewRoomCardView
	OpenRiskCards       []reviewRoomCardView
	DistilledCards      []reviewRoomCardView
	DecisionDigests     []reviewRoomDecisionDigest
	ArtifactDigests     []reviewRoomArtifactDigest
	DecisionThreads     []reviewRoomDecisionThreadDigest
	ThreadWorkbench     reviewRoomThreadWorkbench
	RecentBatchRuns     []reviewRoomBatchRun
	CrossChannelDigests []reviewRoomCrossChannelDigest
	ContextDigests      []reviewRoomContextDigest
}

type reviewRoomDecisionDigest struct {
	Decision            string    `json:"decision"`
	Title               string    `json:"title"`
	CardCount           int       `json:"card_count"`
	OpenCount           int       `json:"open_count"`
	DistilledCount      int       `json:"distilled_count"`
	ArtifactCount       int       `json:"artifact_count"`
	LatestAt            time.Time `json:"latest_at"`
	LatestArtifactTitle string    `json:"latest_artifact_title,omitempty"`
	LatestArtifactLink  string    `json:"latest_artifact_link,omitempty"`
}

type reviewRoomArtifactDigest struct {
	ArtifactID    string    `json:"artifact_id"`
	ArtifactTitle string    `json:"artifact_title"`
	ArtifactLink  string    `json:"artifact_link"`
	SourceTitle   string    `json:"source_title"`
	StatusGroup   string    `json:"status_group"`
	CreatedAt     time.Time `json:"created_at"`
}

type reviewRoomDecisionThreadDigest struct {
	Decision            string    `json:"decision"`
	Title               string    `json:"title"`
	DecisionCount       int       `json:"decision_count"`
	RiskCount           int       `json:"risk_count"`
	ReviewCount         int       `json:"review_count"`
	OpenRiskCount       int       `json:"open_risk_count"`
	PendingReview       int       `json:"pending_review_count"`
	DistilledCount      int       `json:"distilled_count"`
	LatestAt            time.Time `json:"latest_at"`
	LatestSummary       string    `json:"latest_summary,omitempty"`
	LatestArtifact      string    `json:"latest_artifact_link,omitempty"`
	BoundTaskID         string    `json:"bound_task_id,omitempty"`
	BoundTaskLink       string    `json:"bound_task_link,omitempty"`
	BoundArtifactID     string    `json:"bound_artifact_id,omitempty"`
	BoundArtifactLink   string    `json:"bound_artifact_link,omitempty"`
	WorkflowState       string    `json:"workflow_state,omitempty"`
	WorkflowLabel       string    `json:"workflow_label,omitempty"`
	SuggestedTaskStatus string    `json:"suggested_task_status,omitempty"`
	TaskSearchLink      string    `json:"task_search_link,omitempty"`
	ArtifactSearchLink  string    `json:"artifact_search_link,omitempty"`
	HistorySearchLink   string    `json:"history_search_link,omitempty"`
}

type reviewRoomSummary struct {
	TeamID              string                           `json:"team_id"`
	ChannelID           string                           `json:"channel_id"`
	ReviewCount         int                              `json:"review_count"`
	RiskCount           int                              `json:"risk_count"`
	DecisionCount       int                              `json:"decision_count"`
	DistilledCount      int                              `json:"distilled_count"`
	OpenDecisionCount   int                              `json:"open_decision_count"`
	OpenRiskCount       int                              `json:"open_risk_count"`
	DecisionCards       []reviewRoomCardView             `json:"decision_cards,omitempty"`
	RiskCards           []reviewRoomCardView             `json:"risk_cards,omitempty"`
	ReviewCards         []reviewRoomCardView             `json:"review_cards,omitempty"`
	OpenDecisionCards   []reviewRoomCardView             `json:"open_decision_cards,omitempty"`
	OpenRiskCards       []reviewRoomCardView             `json:"open_risk_cards,omitempty"`
	DistilledCards      []reviewRoomCardView             `json:"distilled_cards,omitempty"`
	DecisionDigests     []reviewRoomDecisionDigest       `json:"decision_digests,omitempty"`
	ArtifactDigests     []reviewRoomArtifactDigest       `json:"artifact_digests,omitempty"`
	DecisionThreads     []reviewRoomDecisionThreadDigest `json:"decision_threads,omitempty"`
	ThreadWorkbench     reviewRoomThreadWorkbench        `json:"thread_workbench,omitempty"`
	RecentBatchRuns     []reviewRoomBatchRun             `json:"recent_batch_runs,omitempty"`
	CrossChannelDigests []reviewRoomCrossChannelDigest   `json:"cross_channel_digests,omitempty"`
	ContextDigests      []reviewRoomContextDigest        `json:"context_digests,omitempty"`
}

type reviewRoomThreadWorkbench struct {
	TotalThreads               int                              `json:"total_threads"`
	BoundTaskCount             int                              `json:"bound_task_count"`
	BoundArtifactCount         int                              `json:"bound_artifact_count"`
	MissingTaskCount           int                              `json:"missing_task_count"`
	MissingArtifactCount       int                              `json:"missing_artifact_count"`
	NeedsRiskFollowupCount     int                              `json:"needs_risk_followup_count"`
	NeedsReviewCount           int                              `json:"needs_review_count"`
	ReadyToDistillCount        int                              `json:"ready_to_distill_count"`
	DistilledUnassignedCount   int                              `json:"distilled_unassigned_count"`
	CompletedCount             int                              `json:"completed_count"`
	SuggestedBlockedCount      int                              `json:"suggested_blocked_count"`
	SuggestedDoingCount        int                              `json:"suggested_doing_count"`
	SuggestedDoneCount         int                              `json:"suggested_done_count"`
	AutoCreateTaskThreads      []reviewRoomDecisionThreadDigest `json:"auto_create_task_threads,omitempty"`
	MissingArtifactThreads     []reviewRoomDecisionThreadDigest `json:"missing_artifact_threads,omitempty"`
	NeedsRiskFollowupThreads   []reviewRoomDecisionThreadDigest `json:"needs_risk_followup_threads,omitempty"`
	NeedsReviewThreads         []reviewRoomDecisionThreadDigest `json:"needs_review_threads,omitempty"`
	ReadyToDistillThreads      []reviewRoomDecisionThreadDigest `json:"ready_to_distill_threads,omitempty"`
	DistilledUnassignedThreads []reviewRoomDecisionThreadDigest `json:"distilled_unassigned_threads,omitempty"`
	CompletedThreads           []reviewRoomDecisionThreadDigest `json:"completed_threads,omitempty"`
	SuggestedBlockedThreads    []reviewRoomDecisionThreadDigest `json:"suggested_blocked_threads,omitempty"`
	SuggestedDoingThreads      []reviewRoomDecisionThreadDigest `json:"suggested_doing_threads,omitempty"`
	SuggestedDoneThreads       []reviewRoomDecisionThreadDigest `json:"suggested_done_threads,omitempty"`
}

type reviewRoomBatchRun struct {
	CreatedAt                time.Time `json:"created_at"`
	ActorAgentID             string    `json:"actor_agent_id,omitempty"`
	SyncedThreads            int       `json:"synced_threads"`
	TaskCreated              int       `json:"task_created"`
	ArtifactCreated          int       `json:"artifact_created"`
	TotalThreads             int       `json:"total_threads"`
	SuggestedBlockedCount    int       `json:"suggested_blocked_count"`
	SuggestedDoingCount      int       `json:"suggested_doing_count"`
	SuggestedDoneCount       int       `json:"suggested_done_count"`
	NeedsRiskFollowupCount   int       `json:"needs_risk_followup_count"`
	NeedsReviewCount         int       `json:"needs_review_count"`
	ReadyToDistillCount      int       `json:"ready_to_distill_count"`
	DistilledUnassignedCount int       `json:"distilled_unassigned_count"`
	CompletedCount           int       `json:"completed_count"`
	HistoryLink              string    `json:"history_link,omitempty"`
	CreatedTaskIDs           []string  `json:"created_task_ids,omitempty"`
	CreatedArtifactIDs       []string  `json:"created_artifact_ids,omitempty"`
	CreatedTaskLinks         []string  `json:"created_task_links,omitempty"`
	CreatedArtifactLinks     []string  `json:"created_artifact_links,omitempty"`
}

type reviewRoomChannelLink struct {
	ChannelID    string `json:"channel_id"`
	ChannelTitle string `json:"channel_title,omitempty"`
	RoomLink     string `json:"room_link,omitempty"`
}

type reviewRoomCrossChannelDigest struct {
	Decision           string                  `json:"decision"`
	Title              string                  `json:"title,omitempty"`
	ThreadCount        int                     `json:"thread_count"`
	ChannelCount       int                     `json:"channel_count"`
	OpenRiskCount      int                     `json:"open_risk_count"`
	PendingReviewCount int                     `json:"pending_review_count"`
	Channels           []reviewRoomChannelLink `json:"channels,omitempty"`
}

type reviewRoomContextDigest struct {
	ContextID          string                  `json:"context_id"`
	ThreadCount        int                     `json:"thread_count"`
	TaskCount          int                     `json:"task_count"`
	OpenRiskCount      int                     `json:"open_risk_count"`
	PendingReviewCount int                     `json:"pending_review_count"`
	TaskLinks          []string                `json:"task_links,omitempty"`
	TaskIDs            []string                `json:"task_ids,omitempty"`
	HistorySearchLink  string                  `json:"history_search_link,omitempty"`
	TaskSearchLink     string                  `json:"task_search_link,omitempty"`
	Channels           []reviewRoomChannelLink `json:"channels,omitempty"`
}

type postReviewRoomRequest struct {
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

type threadTaskStatusRequest struct {
	ChannelID    string `json:"channel_id"`
	DecisionRef  string `json:"decision_ref"`
	TaskID       string `json:"task_id,omitempty"`
	ActorAgentID string `json:"actor_agent_id"`
	Status       string `json:"status"`
}

type threadArtifactRequest struct {
	ChannelID    string `json:"channel_id"`
	DecisionRef  string `json:"decision_ref"`
	TaskID       string `json:"task_id,omitempty"`
	ActorAgentID string `json:"actor_agent_id"`
	Title        string `json:"title,omitempty"`
}

type threadSyncRequest struct {
	ChannelID    string `json:"channel_id"`
	DecisionRef  string `json:"decision_ref"`
	TaskID       string `json:"task_id,omitempty"`
	ActorAgentID string `json:"actor_agent_id"`
}

type threadSyncAllRequest struct {
	ChannelID    string `json:"channel_id"`
	ActorAgentID string `json:"actor_agent_id"`
}

type threadSyncResult struct {
	TaskID          string
	TaskStatus      string
	TaskCreated     bool
	ArtifactID      string
	ArtifactCreated bool
}

func newHandler(store *teamcore.Store, teamID string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "" {
			http.NotFound(w, r)
			return
		}
		handleListReviewRoomMessages(store, teamID, w, r)
	})
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handlePostReviewRoomMessage(store, teamID, w, r)
	})
	mux.HandleFunc("/distill", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleDistillReviewSummary(store, teamID, w, r)
	})
	mux.HandleFunc("/summary", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleReviewRoomSummary(store, teamID, w, r)
	})
	mux.HandleFunc("/thread-task-status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleUpdateReviewRoomThreadTask(store, teamID, w, r)
	})
	mux.HandleFunc("/thread-artifact", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleCreateReviewRoomThreadArtifact(store, teamID, w, r)
	})
	mux.HandleFunc("/thread-sync", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleSyncReviewRoomThread(store, teamID, w, r)
	})
	mux.HandleFunc("/thread-sync-all", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleSyncAllReviewRoomThreads(store, teamID, w, r)
	})
	return mux
}

func handleListReviewRoomMessages(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
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
	artifacts, err := store.LoadArtifactsCtx(r.Context(), teamID, 200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	history, err := store.LoadHistoryCtx(r.Context(), teamID, 200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filtered := filterReviewRoomMessages(messages, kind)
	if isAPIRequest(r) {
		if filtered == nil {
			filtered = []teamcore.Message{}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(filtered)
		return
	}
	data := buildReviewRoomPageData(teamID, channelID, actorAgentID, kind, strings.TrimSpace(r.URL.Query().Get("notice")), messages, filtered, artifacts, history)
	crossChannelDigests, contextDigests := buildReviewRoomGlobalDigests(r.Context(), store, teamID)
	data.CrossChannelDigests = crossChannelDigests
	data.ContextDigests = contextDigests
	if err := renderReviewRoomPage(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleReviewRoomSummary(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	channelID := strings.TrimSpace(r.URL.Query().Get("channel_id"))
	if channelID == "" {
		channelID = "main"
	}
	messages, err := store.LoadMessagesCtx(r.Context(), teamID, channelID, 200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	artifacts, err := store.LoadArtifactsCtx(r.Context(), teamID, 200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	history, err := store.LoadHistoryCtx(r.Context(), teamID, 200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := buildReviewRoomPageData(teamID, channelID, strings.TrimSpace(r.URL.Query().Get("actor_agent_id")), "", "", messages, filterReviewRoomMessages(messages, ""), artifacts, history)
	crossChannelDigests, contextDigests := buildReviewRoomGlobalDigests(r.Context(), store, teamID)
	data.CrossChannelDigests = crossChannelDigests
	data.ContextDigests = contextDigests
	summary := reviewRoomSummary{
		TeamID:              teamID,
		ChannelID:           channelID,
		ReviewCount:         data.ReviewCount,
		RiskCount:           data.RiskCount,
		DecisionCount:       data.DecisionCount,
		DistilledCount:      data.DistilledCount,
		OpenDecisionCount:   data.OpenDecisionCount,
		OpenRiskCount:       data.OpenRiskCount,
		DecisionCards:       limitReviewRoomCards(data.DecisionCards, 3),
		RiskCards:           limitReviewRoomCards(data.RiskCards, 3),
		ReviewCards:         limitReviewRoomCards(data.ReviewCards, 3),
		OpenDecisionCards:   limitReviewRoomCards(data.OpenDecisionCards, 3),
		OpenRiskCards:       limitReviewRoomCards(data.OpenRiskCards, 3),
		DistilledCards:      limitReviewRoomCards(data.DistilledCards, 3),
		DecisionDigests:     limitReviewRoomDecisionDigests(data.DecisionDigests, 4),
		ArtifactDigests:     limitReviewRoomArtifactDigests(data.ArtifactDigests, 4),
		DecisionThreads:     limitReviewRoomDecisionThreads(data.DecisionThreads, 4),
		ThreadWorkbench:     limitReviewRoomThreadWorkbench(data.ThreadWorkbench, 4),
		RecentBatchRuns:     limitReviewRoomBatchRuns(data.RecentBatchRuns, 4),
		CrossChannelDigests: limitReviewRoomCrossChannelDigests(data.CrossChannelDigests, 4),
		ContextDigests:      limitReviewRoomContextDigests(data.ContextDigests, 4),
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(summary)
}

func handlePostReviewRoomMessage(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !requestTrusted(r) {
		http.Error(w, "review-room write is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	req, err := decodePostReviewRoomRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	validKinds := map[string]bool{"review": true, "risk": true, "decision": true}
	if !validKinds[req.Kind] {
		http.Error(w, "kind must be review, risk, or decision", http.StatusBadRequest)
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
		req.Content = reviewRoomStructuredTitle(req.StructuredData)
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
		Summary:   "发送 review-room 消息",
		Metadata: map[string]any{
			"channel_id":    req.ChannelID,
			"message_type":  req.Kind,
			"author_agent":  req.AuthorAgentID,
			"message_scope": "review-room",
		},
		CreatedAt: msg.CreatedAt,
	})
	if isAPIRequest(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "created", "kind": req.Kind})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%s/r/review-room/?channel_id=%s&kind=%s&actor_agent_id=%s&notice=created",
		teamID, req.ChannelID, url.QueryEscape(req.Kind), url.QueryEscape(req.AuthorAgentID)), http.StatusSeeOther)
}

func handleDistillReviewSummary(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !requestTrusted(r) {
		http.Error(w, "review-room distill is limited to local or LAN requests", http.StatusForbidden)
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
	var source *teamcore.Message
	for i := range messages {
		if messages[i].MessageID == req.MessageID && isReviewRoomKind(messages[i].MessageType) {
			source = &messages[i]
			break
		}
	}
	if source == nil {
		http.Error(w, "review-room message not found", http.StatusNotFound)
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = reviewRoomStructuredTitle(source.StructuredData)
		if title == "" {
			title = strings.TrimSpace(source.Content)
		}
	}
	content, _ := json.MarshalIndent(source.StructuredData, "", "  ")
	artifact := teamcore.Artifact{
		TeamID:    teamID,
		ChannelID: req.ChannelID,
		Title:     title,
		Kind:      "review-summary",
		Summary:   source.Content,
		Content:   string(content),
		CreatedBy: req.ActorAgentID,
		Labels:    []string{"distilled", "review-room", "source-message:" + req.MessageID},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := store.AppendArtifactCtx(r.Context(), teamID, artifact); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = store.AppendHistoryCtx(r.Context(), teamID, teamcore.ChangeEvent{
		TeamID:    teamID,
		Scope:     "artifact",
		Action:    "create",
		SubjectID: artifact.ArtifactID,
		Summary:   "提炼 review-room 消息为 Artifact",
		Metadata: map[string]any{
			"channel_id":      req.ChannelID,
			"artifact_kind":   "review-summary",
			"source_message":  req.MessageID,
			"artifact_source": "review-room",
		},
		CreatedAt: artifact.CreatedAt,
	})
	if isAPIRequest(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "distilled", "artifact_kind": "review-summary"})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%s/r/review-room/?channel_id=%s&actor_agent_id=%s&notice=distilled",
		teamID, req.ChannelID, url.QueryEscape(req.ActorAgentID)), http.StatusSeeOther)
}

func handleUpdateReviewRoomThreadTask(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !requestTrusted(r) {
		http.Error(w, "review-room task action is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	req, err := decodeThreadTaskStatusRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ChannelID == "" || req.DecisionRef == "" || req.ActorAgentID == "" || req.Status == "" {
		http.Error(w, "channel_id, decision_ref, actor_agent_id, and status are required", http.StatusBadRequest)
		return
	}
	if !validTaskStatus(req.Status) {
		http.Error(w, "status must be open, doing, blocked, or done", http.StatusBadRequest)
		return
	}
	if err := requireAction(store, teamID, req.ActorAgentID, "task.update"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	thread, err := loadReviewRoomThread(r.Context(), store, teamID, req.ChannelID, req.DecisionRef)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	taskID := firstNonEmpty(req.TaskID, thread.BoundTaskID)
	if taskID == "" {
		http.Error(w, "review-room thread has no bound task", http.StatusBadRequest)
		return
	}
	task, err := store.LoadTaskCtx(r.Context(), teamID, taskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	updated := task
	updated.Status = req.Status
	updated.UpdatedAt = time.Now().UTC()
	if err := store.SaveTaskCtx(r.Context(), teamID, updated); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = store.AppendHistoryCtx(r.Context(), teamID, teamcore.ChangeEvent{
		TeamID:    teamID,
		Scope:     "task",
		Action:    "update",
		SubjectID: taskID,
		Summary:   "从 review-room 推进任务状态",
		Metadata: map[string]any{
			"channel_id":    req.ChannelID,
			"decision_ref":  req.DecisionRef,
			"task_id":       taskID,
			"status":        req.Status,
			"actor_agent":   req.ActorAgentID,
			"message_scope": "review-room",
		},
		CreatedAt: updated.UpdatedAt,
	})
	if isAPIRequest(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "updated",
			"task_id": taskID,
		})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%s/r/review-room/?channel_id=%s&actor_agent_id=%s&notice=task-updated",
		teamID, req.ChannelID, url.QueryEscape(req.ActorAgentID)), http.StatusSeeOther)
}

func handleCreateReviewRoomThreadArtifact(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !requestTrusted(r) {
		http.Error(w, "review-room thread distill is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	req, err := decodeThreadArtifactRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ChannelID == "" || req.DecisionRef == "" || req.ActorAgentID == "" {
		http.Error(w, "channel_id, decision_ref, and actor_agent_id are required", http.StatusBadRequest)
		return
	}
	if err := requireAction(store, teamID, req.ActorAgentID, "artifact.create"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	artifact, created, err := ensureReviewRoomThreadArtifact(r.Context(), store, teamID, req.ChannelID, req.DecisionRef, req.TaskID, req.ActorAgentID, req.Title)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if isAPIRequest(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if created {
			w.WriteHeader(http.StatusCreated)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":      map[bool]string{true: "created", false: "existing"}[created],
			"artifact_id": artifact.ArtifactID,
		})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%s/r/review-room/?channel_id=%s&actor_agent_id=%s&notice=thread-distilled",
		teamID, req.ChannelID, url.QueryEscape(req.ActorAgentID)), http.StatusSeeOther)
}

func handleSyncReviewRoomThread(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !requestTrusted(r) {
		http.Error(w, "review-room thread sync is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	req, err := decodeThreadSyncRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ChannelID == "" || req.DecisionRef == "" || req.ActorAgentID == "" {
		http.Error(w, "channel_id, decision_ref, and actor_agent_id are required", http.StatusBadRequest)
		return
	}
	result, err := syncReviewRoomThread(r.Context(), store, teamID, req.ChannelID, req.DecisionRef, req.TaskID, req.ActorAgentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if isAPIRequest(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":                "synced",
			"task_id":               result.TaskID,
			"task_status":           result.TaskStatus,
			"task_created":          fmt.Sprintf("%t", result.TaskCreated),
			"artifact_id":           result.ArtifactID,
			"artifact_created":      fmt.Sprintf("%t", result.ArtifactCreated),
			"suggested_task_status": result.TaskStatus,
		})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%s/r/review-room/?channel_id=%s&actor_agent_id=%s&notice=thread-synced",
		teamID, req.ChannelID, url.QueryEscape(req.ActorAgentID)), http.StatusSeeOther)
}

func handleSyncAllReviewRoomThreads(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !requestTrusted(r) {
		http.Error(w, "review-room batch sync is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	req, err := decodeThreadSyncAllRequest(r)
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
	artifacts, err := store.LoadArtifactsCtx(r.Context(), teamID, 200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	history, err := store.LoadHistoryCtx(r.Context(), teamID, 200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := buildReviewRoomPageData(teamID, req.ChannelID, req.ActorAgentID, "", "", messages, filterReviewRoomMessages(messages, ""), artifacts, history)
	synced := 0
	taskCreated := 0
	artifactCreated := 0
	createdTaskIDs := make([]string, 0, 8)
	createdArtifactIDs := make([]string, 0, 8)
	for _, thread := range data.DecisionThreads {
		result, err := syncReviewRoomThread(r.Context(), store, teamID, req.ChannelID, thread.Decision, thread.BoundTaskID, req.ActorAgentID)
		if err != nil {
			continue
		}
		synced++
		if result.TaskCreated {
			taskCreated++
			if strings.TrimSpace(result.TaskID) != "" {
				createdTaskIDs = append(createdTaskIDs, strings.TrimSpace(result.TaskID))
			}
		}
		if result.ArtifactCreated {
			artifactCreated++
			if strings.TrimSpace(result.ArtifactID) != "" {
				createdArtifactIDs = append(createdArtifactIDs, strings.TrimSpace(result.ArtifactID))
			}
		}
	}
	messages, err = store.LoadAllMessagesCtx(r.Context(), teamID, req.ChannelID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	artifacts, err = store.LoadArtifactsCtx(r.Context(), teamID, 200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	history, err = store.LoadHistoryCtx(r.Context(), teamID, 200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	updated := buildReviewRoomPageData(teamID, req.ChannelID, req.ActorAgentID, "", "", messages, filterReviewRoomMessages(messages, ""), artifacts, history)
	_ = store.AppendHistoryCtx(r.Context(), teamID, teamcore.ChangeEvent{
		TeamID:       teamID,
		Scope:        "room",
		Action:       "sync",
		SubjectID:    req.ChannelID,
		Summary:      "批量同步 review-room 结论线程",
		ActorAgentID: req.ActorAgentID,
		Source:       "review-room",
		Metadata: map[string]any{
			"channel_id":                 req.ChannelID,
			"message_scope":              "review-room",
			"batch_action":               "thread-sync-all",
			"synced_threads":             synced,
			"task_created":               taskCreated,
			"artifact_created":           artifactCreated,
			"created_task_ids":           createdTaskIDs,
			"created_artifact_ids":       createdArtifactIDs,
			"total_threads":              updated.ThreadWorkbench.TotalThreads,
			"suggested_blocked_count":    updated.ThreadWorkbench.SuggestedBlockedCount,
			"suggested_doing_count":      updated.ThreadWorkbench.SuggestedDoingCount,
			"suggested_done_count":       updated.ThreadWorkbench.SuggestedDoneCount,
			"needs_risk_followup_count":  updated.ThreadWorkbench.NeedsRiskFollowupCount,
			"needs_review_count":         updated.ThreadWorkbench.NeedsReviewCount,
			"ready_to_distill_count":     updated.ThreadWorkbench.ReadyToDistillCount,
			"distilled_unassigned_count": updated.ThreadWorkbench.DistilledUnassignedCount,
			"completed_count":            updated.ThreadWorkbench.CompletedCount,
		},
		CreatedAt: time.Now().UTC(),
	})
	if isAPIRequest(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":           "synced",
			"synced_threads":   synced,
			"task_created":     taskCreated,
			"artifact_created": artifactCreated,
		})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%s/r/review-room/?channel_id=%s&actor_agent_id=%s&notice=threads-synced",
		teamID, req.ChannelID, url.QueryEscape(req.ActorAgentID)), http.StatusSeeOther)
}

func filterReviewRoomMessages(messages []teamcore.Message, kind string) []teamcore.Message {
	filtered := make([]teamcore.Message, 0, len(messages))
	for _, msg := range messages {
		if !isReviewRoomKind(msg.MessageType) {
			continue
		}
		if kind != "" && strings.TrimSpace(msg.MessageType) != kind {
			continue
		}
		filtered = append(filtered, msg)
	}
	return filtered
}

func buildReviewRoomPageData(teamID, channelID, actorAgentID, kind, notice string, allMessages, messages []teamcore.Message, artifacts []teamcore.Artifact, history []teamcore.ChangeEvent) reviewRoomPageData {
	cards := make([]reviewRoomCardView, 0, len(messages))
	decisionCards := make([]reviewRoomCardView, 0, len(messages))
	riskCards := make([]reviewRoomCardView, 0, len(messages))
	reviewCards := make([]reviewRoomCardView, 0, len(messages))
	openDecisionCards := make([]reviewRoomCardView, 0, len(messages))
	openRiskCards := make([]reviewRoomCardView, 0, len(messages))
	distilledCards := make([]reviewRoomCardView, 0, len(messages))
	decisionDigests := make([]reviewRoomDecisionDigest, 0, len(messages))
	artifactDigests := make([]reviewRoomArtifactDigest, 0, len(messages))
	decisionThreads := make([]reviewRoomDecisionThreadDigest, 0, len(messages))
	threadWorkbench := reviewRoomThreadWorkbench{}
	recentBatchRuns := make([]reviewRoomBatchRun, 0, 4)
	distilledSources := reviewRoomArtifactSources(teamID, channelID, artifacts)
	reviewCount := 0
	riskCount := 0
	decisionCount := 0
	for _, msg := range allMessages {
		switch strings.TrimSpace(msg.MessageType) {
		case "review":
			reviewCount++
		case "risk":
			riskCount++
		case "decision":
			decisionCount++
		}
	}
	for _, msg := range messages {
		summary, detailLabel, detail, itemsLabel, items := reviewRoomCardDetails(msg.MessageType, msg.StructuredData)
		distilledArtifact, distilled := distilledSources[msg.MessageID]
		card := reviewRoomCardView{
			MessageID:          msg.MessageID,
			Kind:               msg.MessageType,
			KindLabel:          reviewRoomKindLabel(msg.MessageType),
			Title:              reviewRoomStructuredTitle(msg.StructuredData),
			Content:            msg.Content,
			Summary:            summary,
			DetailLabel:        detailLabel,
			Detail:             detail,
			ItemsLabel:         itemsLabel,
			Items:              items,
			Distilled:          distilled,
			StatusLabel:        reviewRoomStatusLabel(msg.MessageType, distilled),
			StatusGroup:        reviewRoomStatusGroup(msg.MessageType, distilled),
			DecisionRef:        reviewRoomDecisionRef(msg.MessageType, msg.StructuredData),
			TaskID:             strings.TrimSpace(stringField(msg.StructuredData, "task_id")),
			TaskLink:           reviewRoomTaskLink(teamID, strings.TrimSpace(stringField(msg.StructuredData, "task_id"))),
			LinkedArtifactID:   strings.TrimSpace(stringField(msg.StructuredData, "artifact_id")),
			LinkedArtifactLink: reviewRoomArtifactLink(teamID, strings.TrimSpace(stringField(msg.StructuredData, "artifact_id"))),
			AuthorAgentID:      msg.AuthorAgentID,
			CreatedAt:          msg.CreatedAt,
			ArtifactTitle:      distilledArtifact.Title,
			ArtifactLink:       reviewRoomArtifactLink(teamID, distilledArtifact.ArtifactID),
			StructuredJSON:     formatStructuredJSON(msg.StructuredData),
		}
		cards = append(cards, card)
		switch strings.TrimSpace(msg.MessageType) {
		case "decision":
			decisionCards = append(decisionCards, card)
			if !card.Distilled {
				openDecisionCards = append(openDecisionCards, card)
			}
		case "risk":
			riskCards = append(riskCards, card)
			if !card.Distilled {
				openRiskCards = append(openRiskCards, card)
			}
		case "review":
			reviewCards = append(reviewCards, card)
		}
		if card.Distilled {
			distilledCards = append(distilledCards, card)
		}
	}
	distilledCount := 0
	openDecisionCount := 0
	openRiskCount := 0
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.ChannelID) == strings.TrimSpace(channelID) && strings.TrimSpace(artifact.Kind) == "review-summary" {
			distilledCount++
		}
	}
	for _, card := range decisionCards {
		if !card.Distilled {
			openDecisionCount++
		}
	}
	for _, card := range riskCards {
		if !card.Distilled {
			openRiskCount++
		}
	}
	decisionDigests = buildReviewRoomDecisionDigests(decisionCards)
	artifactDigests = buildReviewRoomArtifactDigests(distilledCards)
	decisionThreads = reviewRoomThreadLinks(teamID, reviewRoomApplyThreadArtifactBindings(teamID, channelID, buildReviewRoomDecisionThreads(cards), artifacts))
	threadWorkbench = buildReviewRoomThreadWorkbench(decisionThreads)
	recentBatchRuns = buildReviewRoomBatchRuns(teamID, channelID, history)
	return reviewRoomPageData{
		TeamID:            teamID,
		ChannelID:         channelID,
		FilterKind:        kind,
		ActorAgentID:      actorAgentID,
		Notice:            notice,
		Cards:             cards,
		MessageCount:      len(cards),
		ReviewCount:       reviewCount,
		RiskCount:         riskCount,
		DecisionCount:     decisionCount,
		DistilledCount:    distilledCount,
		OpenDecisionCount: openDecisionCount,
		OpenRiskCount:     openRiskCount,
		ArtifactLink:      fmt.Sprintf("/teams/%s/artifacts?channel=%s&kind=review-summary", teamID, url.QueryEscape(channelID)),
		HistoryLink:       fmt.Sprintf("/teams/%s/history?scope=message", teamID),
		DecisionCards:     decisionCards,
		RiskCards:         riskCards,
		ReviewCards:       reviewCards,
		OpenDecisionCards: openDecisionCards,
		OpenRiskCards:     openRiskCards,
		DistilledCards:    distilledCards,
		DecisionDigests:   decisionDigests,
		ArtifactDigests:   artifactDigests,
		DecisionThreads:   decisionThreads,
		ThreadWorkbench:   threadWorkbench,
		RecentBatchRuns:   recentBatchRuns,
	}
}

func reviewRoomArtifactSources(teamID, channelID string, artifacts []teamcore.Artifact) map[string]teamcore.Artifact {
	out := make(map[string]teamcore.Artifact)
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.ChannelID) != strings.TrimSpace(channelID) {
			continue
		}
		if strings.TrimSpace(artifact.Kind) != "review-summary" {
			continue
		}
		for _, label := range artifact.Labels {
			label = strings.TrimSpace(label)
			if !strings.HasPrefix(label, "source-message:") {
				continue
			}
			messageID := strings.TrimSpace(strings.TrimPrefix(label, "source-message:"))
			if messageID != "" {
				existing, ok := out[messageID]
				if !ok || artifact.CreatedAt.After(existing.CreatedAt) {
					out[messageID] = artifact
				}
			}
		}
	}
	return out
}

func reviewRoomStatusLabel(kind string, distilled bool) string {
	if distilled {
		return "已提炼"
	}
	switch strings.TrimSpace(kind) {
	case "decision":
		return "待沉淀"
	case "risk":
		return "待跟进"
	default:
		return "进行中"
	}
}

func reviewRoomStatusGroup(kind string, distilled bool) string {
	if distilled {
		return "已提炼"
	}
	switch strings.TrimSpace(kind) {
	case "decision":
		return "待沉淀"
	case "risk":
		return "待跟进"
	default:
		return "进行中"
	}
}

func reviewRoomArtifactLink(teamID, artifactID string) string {
	artifactID = strings.TrimSpace(artifactID)
	if artifactID == "" {
		return ""
	}
	return fmt.Sprintf("/teams/%s/artifacts/%s", teamID, url.PathEscape(artifactID))
}

func limitReviewRoomCards(cards []reviewRoomCardView, limit int) []reviewRoomCardView {
	if limit <= 0 || len(cards) <= limit {
		return cards
	}
	return append([]reviewRoomCardView(nil), cards[:limit]...)
}

func limitReviewRoomDecisionDigests(items []reviewRoomDecisionDigest, limit int) []reviewRoomDecisionDigest {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return append([]reviewRoomDecisionDigest(nil), items[:limit]...)
}

func limitReviewRoomArtifactDigests(items []reviewRoomArtifactDigest, limit int) []reviewRoomArtifactDigest {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return append([]reviewRoomArtifactDigest(nil), items[:limit]...)
}

func limitReviewRoomDecisionThreads(items []reviewRoomDecisionThreadDigest, limit int) []reviewRoomDecisionThreadDigest {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return append([]reviewRoomDecisionThreadDigest(nil), items[:limit]...)
}

func limitReviewRoomThreadWorkbench(workbench reviewRoomThreadWorkbench, limit int) reviewRoomThreadWorkbench {
	workbench.AutoCreateTaskThreads = limitReviewRoomDecisionThreads(workbench.AutoCreateTaskThreads, limit)
	workbench.MissingArtifactThreads = limitReviewRoomDecisionThreads(workbench.MissingArtifactThreads, limit)
	workbench.NeedsRiskFollowupThreads = limitReviewRoomDecisionThreads(workbench.NeedsRiskFollowupThreads, limit)
	workbench.NeedsReviewThreads = limitReviewRoomDecisionThreads(workbench.NeedsReviewThreads, limit)
	workbench.ReadyToDistillThreads = limitReviewRoomDecisionThreads(workbench.ReadyToDistillThreads, limit)
	workbench.DistilledUnassignedThreads = limitReviewRoomDecisionThreads(workbench.DistilledUnassignedThreads, limit)
	workbench.CompletedThreads = limitReviewRoomDecisionThreads(workbench.CompletedThreads, limit)
	workbench.SuggestedBlockedThreads = limitReviewRoomDecisionThreads(workbench.SuggestedBlockedThreads, limit)
	workbench.SuggestedDoingThreads = limitReviewRoomDecisionThreads(workbench.SuggestedDoingThreads, limit)
	workbench.SuggestedDoneThreads = limitReviewRoomDecisionThreads(workbench.SuggestedDoneThreads, limit)
	return workbench
}

func limitReviewRoomBatchRuns(items []reviewRoomBatchRun, limit int) []reviewRoomBatchRun {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return append([]reviewRoomBatchRun(nil), items[:limit]...)
}

func limitReviewRoomCrossChannelDigests(items []reviewRoomCrossChannelDigest, limit int) []reviewRoomCrossChannelDigest {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return append([]reviewRoomCrossChannelDigest(nil), items[:limit]...)
}

func limitReviewRoomContextDigests(items []reviewRoomContextDigest, limit int) []reviewRoomContextDigest {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return append([]reviewRoomContextDigest(nil), items[:limit]...)
}

func buildReviewRoomDecisionDigests(cards []reviewRoomCardView) []reviewRoomDecisionDigest {
	type agg struct {
		title               string
		decision            string
		cardCount           int
		openCount           int
		distilledCount      int
		artifactCount       int
		latestAt            time.Time
		latestArtifactTitle string
		latestArtifactLink  string
	}
	byDecision := make(map[string]*agg)
	order := make([]string, 0, len(cards))
	for _, card := range cards {
		key := strings.TrimSpace(card.Detail)
		if key == "" {
			key = strings.TrimSpace(card.Title)
		}
		if key == "" {
			key = strings.TrimSpace(card.Content)
		}
		if key == "" {
			key = "未命名决策"
		}
		item := byDecision[key]
		if item == nil {
			item = &agg{title: card.Title, decision: key}
			byDecision[key] = item
			order = append(order, key)
		}
		item.cardCount++
		if card.Distilled {
			item.distilledCount++
			item.artifactCount++
		} else {
			item.openCount++
		}
		if card.CreatedAt.After(item.latestAt) {
			item.latestAt = card.CreatedAt
			if strings.TrimSpace(card.Title) != "" {
				item.title = card.Title
			}
		}
		if card.Distilled && strings.TrimSpace(card.ArtifactLink) != "" {
			item.latestArtifactLink = card.ArtifactLink
			item.latestArtifactTitle = strings.TrimSpace(card.ArtifactTitle)
		}
	}
	out := make([]reviewRoomDecisionDigest, 0, len(order))
	for _, key := range order {
		item := byDecision[key]
		out = append(out, reviewRoomDecisionDigest{
			Decision:            item.decision,
			Title:               strings.TrimSpace(item.title),
			CardCount:           item.cardCount,
			OpenCount:           item.openCount,
			DistilledCount:      item.distilledCount,
			ArtifactCount:       item.artifactCount,
			LatestAt:            item.latestAt,
			LatestArtifactTitle: item.latestArtifactTitle,
			LatestArtifactLink:  item.latestArtifactLink,
		})
	}
	return out
}

func buildReviewRoomArtifactDigests(cards []reviewRoomCardView) []reviewRoomArtifactDigest {
	out := make([]reviewRoomArtifactDigest, 0, len(cards))
	for _, card := range cards {
		if strings.TrimSpace(card.ArtifactLink) == "" {
			continue
		}
		out = append(out, reviewRoomArtifactDigest{
			ArtifactTitle: strings.TrimSpace(card.ArtifactTitle),
			ArtifactLink:  card.ArtifactLink,
			SourceTitle:   firstNonEmpty(card.Title, card.Content),
			StatusGroup:   card.StatusGroup,
			CreatedAt:     card.CreatedAt,
		})
	}
	return out
}

func buildReviewRoomDecisionThreads(cards []reviewRoomCardView) []reviewRoomDecisionThreadDigest {
	type agg struct {
		title             string
		decision          string
		decisionCount     int
		riskCount         int
		reviewCount       int
		openRiskCount     int
		pendingReview     int
		distilledCount    int
		latestAt          time.Time
		latestSummary     string
		latestArtifact    string
		boundTaskID       string
		boundTaskLink     string
		boundArtifactID   string
		boundArtifactLink string
	}
	order := make([]string, 0, len(cards))
	byDecision := make(map[string]*agg)
	for _, card := range cards {
		ref := strings.TrimSpace(card.DecisionRef)
		if ref == "" {
			continue
		}
		item := byDecision[ref]
		if item == nil {
			item = &agg{decision: ref, title: firstNonEmpty(card.Title, ref)}
			byDecision[ref] = item
			order = append(order, ref)
		}
		switch strings.TrimSpace(card.Kind) {
		case "decision":
			item.decisionCount++
		case "risk":
			item.riskCount++
			if !card.Distilled {
				item.openRiskCount++
			}
		case "review":
			item.reviewCount++
			if !card.Distilled {
				item.pendingReview++
			}
		}
		if card.Distilled {
			item.distilledCount++
			if strings.TrimSpace(card.ArtifactLink) != "" {
				item.latestArtifact = card.ArtifactLink
			}
		}
		if item.boundTaskLink == "" && strings.TrimSpace(card.TaskLink) != "" {
			item.boundTaskID = card.TaskID
			item.boundTaskLink = card.TaskLink
		}
		if item.boundArtifactLink == "" && strings.TrimSpace(card.LinkedArtifactLink) != "" {
			item.boundArtifactID = card.LinkedArtifactID
			item.boundArtifactLink = card.LinkedArtifactLink
		}
		if card.CreatedAt.After(item.latestAt) {
			item.latestAt = card.CreatedAt
			item.latestSummary = firstNonEmpty(card.Summary, card.Detail, card.Content)
			if strings.TrimSpace(card.Title) != "" {
				item.title = card.Title
			}
		}
	}
	out := make([]reviewRoomDecisionThreadDigest, 0, len(order))
	for _, key := range order {
		item := byDecision[key]
		thread := reviewRoomDecisionThreadDigest{
			Decision:          item.decision,
			Title:             item.title,
			DecisionCount:     item.decisionCount,
			RiskCount:         item.riskCount,
			ReviewCount:       item.reviewCount,
			OpenRiskCount:     item.openRiskCount,
			PendingReview:     item.pendingReview,
			DistilledCount:    item.distilledCount,
			LatestAt:          item.latestAt,
			LatestSummary:     item.latestSummary,
			LatestArtifact:    item.latestArtifact,
			BoundTaskID:       item.boundTaskID,
			BoundTaskLink:     item.boundTaskLink,
			BoundArtifactID:   item.boundArtifactID,
			BoundArtifactLink: item.boundArtifactLink,
			SuggestedTaskStatus: reviewRoomSuggestedTaskStatus(reviewRoomDecisionThreadDigest{
				DecisionCount:  item.decisionCount,
				RiskCount:      item.riskCount,
				ReviewCount:    item.reviewCount,
				OpenRiskCount:  item.openRiskCount,
				PendingReview:  item.pendingReview,
				DistilledCount: item.distilledCount,
			}),
		}
		thread.WorkflowState = reviewRoomWorkflowState(thread)
		thread.WorkflowLabel = reviewRoomWorkflowLabel(thread.WorkflowState)
		out = append(out, thread)
	}
	return out
}

func buildReviewRoomThreadWorkbench(threads []reviewRoomDecisionThreadDigest) reviewRoomThreadWorkbench {
	workbench := reviewRoomThreadWorkbench{
		TotalThreads: len(threads),
	}
	for _, thread := range threads {
		if strings.TrimSpace(thread.BoundTaskID) != "" {
			workbench.BoundTaskCount++
		} else {
			workbench.MissingTaskCount++
			workbench.AutoCreateTaskThreads = append(workbench.AutoCreateTaskThreads, thread)
		}
		if strings.TrimSpace(thread.BoundArtifactID) != "" || strings.TrimSpace(thread.LatestArtifact) != "" {
			workbench.BoundArtifactCount++
		} else {
			workbench.MissingArtifactCount++
			workbench.MissingArtifactThreads = append(workbench.MissingArtifactThreads, thread)
		}
		switch strings.TrimSpace(thread.WorkflowState) {
		case "needs-risk-followup":
			workbench.NeedsRiskFollowupCount++
			workbench.NeedsRiskFollowupThreads = append(workbench.NeedsRiskFollowupThreads, thread)
		case "needs-review":
			workbench.NeedsReviewCount++
			workbench.NeedsReviewThreads = append(workbench.NeedsReviewThreads, thread)
		case "ready-to-distill":
			workbench.ReadyToDistillCount++
			workbench.ReadyToDistillThreads = append(workbench.ReadyToDistillThreads, thread)
		case "distilled-unassigned":
			workbench.DistilledUnassignedCount++
			workbench.DistilledUnassignedThreads = append(workbench.DistilledUnassignedThreads, thread)
		case "completed":
			workbench.CompletedCount++
			workbench.CompletedThreads = append(workbench.CompletedThreads, thread)
		}
		switch strings.TrimSpace(thread.SuggestedTaskStatus) {
		case "blocked":
			workbench.SuggestedBlockedCount++
			workbench.SuggestedBlockedThreads = append(workbench.SuggestedBlockedThreads, thread)
		case "doing":
			workbench.SuggestedDoingCount++
			workbench.SuggestedDoingThreads = append(workbench.SuggestedDoingThreads, thread)
		case "done":
			workbench.SuggestedDoneCount++
			workbench.SuggestedDoneThreads = append(workbench.SuggestedDoneThreads, thread)
		}
	}
	return workbench
}

type reviewRoomThreadRef struct {
	ChannelID    string
	ChannelTitle string
	Thread       reviewRoomDecisionThreadDigest
}

func buildReviewRoomGlobalDigests(ctx context.Context, store *teamcore.Store, teamID string) ([]reviewRoomCrossChannelDigest, []reviewRoomContextDigest) {
	if store == nil || strings.TrimSpace(teamID) == "" {
		return nil, nil
	}
	channels, err := store.ListChannelsCtx(ctx, teamID)
	if err != nil {
		return nil, nil
	}
	artifacts, err := store.LoadArtifactsCtx(ctx, teamID, 200)
	if err != nil {
		return nil, nil
	}
	tasks, err := store.LoadTasksCtx(ctx, teamID, 200)
	if err != nil {
		return nil, nil
	}
	taskByID := make(map[string]teamcore.Task, len(tasks))
	for _, task := range tasks {
		taskByID[strings.TrimSpace(task.TaskID)] = task
	}
	refs := make([]reviewRoomThreadRef, 0, len(channels)*2)
	for _, channel := range channels {
		messages, err := store.LoadAllMessagesCtx(ctx, teamID, channel.ChannelID)
		if err != nil {
			continue
		}
		filtered := filterReviewRoomMessages(messages, "")
		if len(filtered) == 0 {
			continue
		}
		data := buildReviewRoomPageData(teamID, channel.ChannelID, "", "", "", messages, filtered, artifacts, nil)
		for _, thread := range data.DecisionThreads {
			refs = append(refs, reviewRoomThreadRef{
				ChannelID:    channel.ChannelID,
				ChannelTitle: firstNonEmpty(channel.Title, channel.ChannelID),
				Thread:       thread,
			})
		}
	}
	return buildReviewRoomCrossChannelDigests(teamID, refs), buildReviewRoomContextDigests(teamID, refs, taskByID)
}

func buildReviewRoomCrossChannelDigests(teamID string, refs []reviewRoomThreadRef) []reviewRoomCrossChannelDigest {
	type agg struct {
		title              string
		decision           string
		threadCount        int
		openRiskCount      int
		pendingReviewCount int
		channels           []reviewRoomChannelLink
		seenChannels       map[string]struct{}
	}
	order := make([]string, 0, len(refs))
	byDecision := make(map[string]*agg)
	for _, ref := range refs {
		key := strings.TrimSpace(ref.Thread.Decision)
		if key == "" {
			continue
		}
		item := byDecision[key]
		if item == nil {
			item = &agg{
				title:        firstNonEmpty(ref.Thread.Title, key),
				decision:     key,
				seenChannels: make(map[string]struct{}),
			}
			byDecision[key] = item
			order = append(order, key)
		}
		item.threadCount++
		item.openRiskCount += ref.Thread.OpenRiskCount
		item.pendingReviewCount += ref.Thread.PendingReview
		if _, ok := item.seenChannels[ref.ChannelID]; !ok {
			item.seenChannels[ref.ChannelID] = struct{}{}
			item.channels = append(item.channels, reviewRoomChannelLink{
				ChannelID:    ref.ChannelID,
				ChannelTitle: ref.ChannelTitle,
				RoomLink:     fmt.Sprintf("/teams/%s/r/review-room/?channel_id=%s", teamID, url.QueryEscape(ref.ChannelID)),
			})
		}
	}
	out := make([]reviewRoomCrossChannelDigest, 0, len(order))
	for _, key := range order {
		item := byDecision[key]
		out = append(out, reviewRoomCrossChannelDigest{
			Decision:           item.decision,
			Title:              item.title,
			ThreadCount:        item.threadCount,
			ChannelCount:       len(item.channels),
			OpenRiskCount:      item.openRiskCount,
			PendingReviewCount: item.pendingReviewCount,
			Channels:           item.channels,
		})
	}
	return out
}

func buildReviewRoomContextDigests(teamID string, refs []reviewRoomThreadRef, taskByID map[string]teamcore.Task) []reviewRoomContextDigest {
	type agg struct {
		contextID          string
		threadCount        int
		openRiskCount      int
		pendingReviewCount int
		taskIDs            []string
		taskLinks          []string
		seenTasks          map[string]struct{}
		channels           []reviewRoomChannelLink
		seenChannels       map[string]struct{}
	}
	order := make([]string, 0, len(refs))
	byContext := make(map[string]*agg)
	for _, ref := range refs {
		taskID := strings.TrimSpace(ref.Thread.BoundTaskID)
		if taskID == "" {
			continue
		}
		task, ok := taskByID[taskID]
		if !ok {
			continue
		}
		contextID := strings.TrimSpace(task.ContextID)
		if contextID == "" {
			continue
		}
		item := byContext[contextID]
		if item == nil {
			item = &agg{
				contextID:    contextID,
				seenTasks:    make(map[string]struct{}),
				seenChannels: make(map[string]struct{}),
			}
			byContext[contextID] = item
			order = append(order, contextID)
		}
		item.threadCount++
		item.openRiskCount += ref.Thread.OpenRiskCount
		item.pendingReviewCount += ref.Thread.PendingReview
		if _, ok := item.seenTasks[taskID]; !ok {
			item.seenTasks[taskID] = struct{}{}
			item.taskIDs = append(item.taskIDs, taskID)
			if link := reviewRoomTaskLink(teamID, taskID); link != "" {
				item.taskLinks = append(item.taskLinks, link)
			}
		}
		if _, ok := item.seenChannels[ref.ChannelID]; !ok {
			item.seenChannels[ref.ChannelID] = struct{}{}
			item.channels = append(item.channels, reviewRoomChannelLink{
				ChannelID:    ref.ChannelID,
				ChannelTitle: ref.ChannelTitle,
				RoomLink:     fmt.Sprintf("/teams/%s/r/review-room/?channel_id=%s", teamID, url.QueryEscape(ref.ChannelID)),
			})
		}
	}
	out := make([]reviewRoomContextDigest, 0, len(order))
	for _, key := range order {
		item := byContext[key]
		out = append(out, reviewRoomContextDigest{
			ContextID:          item.contextID,
			ThreadCount:        item.threadCount,
			TaskCount:          len(item.taskIDs),
			OpenRiskCount:      item.openRiskCount,
			PendingReviewCount: item.pendingReviewCount,
			TaskLinks:          item.taskLinks,
			TaskIDs:            item.taskIDs,
			TaskSearchLink:     reviewRoomScopedSearchLink(teamID, item.contextID, "tasks"),
			HistorySearchLink:  reviewRoomScopedSearchLink(teamID, item.contextID, "history"),
			Channels:           item.channels,
		})
	}
	return out
}

func reviewRoomThreadLinks(teamID string, threads []reviewRoomDecisionThreadDigest) []reviewRoomDecisionThreadDigest {
	for i := range threads {
		query := strings.TrimSpace(threads[i].Decision)
		if query == "" {
			query = strings.TrimSpace(threads[i].Title)
		}
		if query == "" {
			continue
		}
		threads[i].TaskSearchLink = reviewRoomScopedSearchLink(teamID, query, "tasks")
		threads[i].ArtifactSearchLink = reviewRoomScopedSearchLink(teamID, query, "artifacts")
		threads[i].HistorySearchLink = reviewRoomScopedSearchLink(teamID, query, "history")
	}
	return threads
}

func reviewRoomScopedSearchLink(teamID, query, scope string) string {
	query = strings.TrimSpace(query)
	scope = strings.TrimSpace(scope)
	if teamID == "" || query == "" || scope == "" {
		return ""
	}
	return fmt.Sprintf("/teams/%s/search?q=%s&scope=%s", teamID, url.QueryEscape(query), url.QueryEscape(scope))
}

func reviewRoomTaskLink(teamID, taskID string) string {
	taskID = strings.TrimSpace(taskID)
	if teamID == "" || taskID == "" {
		return ""
	}
	return fmt.Sprintf("/teams/%s/tasks/%s", teamID, url.PathEscape(taskID))
}

func reviewRoomSuggestedTaskStatus(thread reviewRoomDecisionThreadDigest) string {
	switch {
	case thread.OpenRiskCount > 0:
		return "blocked"
	case thread.PendingReview > 0:
		return "doing"
	case thread.DistilledCount > 0 && thread.DecisionCount > 0:
		return "done"
	case thread.DecisionCount > 0 || thread.ReviewCount > 0:
		return "doing"
	default:
		return ""
	}
}

func reviewRoomWorkflowState(thread reviewRoomDecisionThreadDigest) string {
	switch {
	case thread.OpenRiskCount > 0:
		return "needs-risk-followup"
	case thread.PendingReview > 0:
		return "needs-review"
	case thread.DecisionCount > 0 && thread.DistilledCount == 0:
		return "ready-to-distill"
	case thread.DistilledCount > 0 && strings.TrimSpace(thread.BoundTaskID) == "":
		return "distilled-unassigned"
	case thread.DistilledCount > 0 && strings.TrimSpace(thread.SuggestedTaskStatus) == "done":
		return "completed"
	default:
		return "active"
	}
}

func reviewRoomWorkflowLabel(state string) string {
	switch strings.TrimSpace(state) {
	case "needs-risk-followup":
		return "待风险跟进"
	case "needs-review":
		return "待评审"
	case "ready-to-distill":
		return "待沉淀"
	case "distilled-unassigned":
		return "已沉淀待挂接"
	case "completed":
		return "已完成"
	default:
		return "进行中"
	}
}

func reviewRoomApplyThreadArtifactBindings(teamID, channelID string, threads []reviewRoomDecisionThreadDigest, artifacts []teamcore.Artifact) []reviewRoomDecisionThreadDigest {
	for i := range threads {
		artifact, ok := reviewRoomDecisionArtifact(channelID, threads[i].Decision, artifacts)
		if ok {
			if threads[i].BoundTaskID == "" && strings.TrimSpace(artifact.TaskID) != "" {
				threads[i].BoundTaskID = strings.TrimSpace(artifact.TaskID)
				threads[i].BoundTaskLink = reviewRoomTaskLink(teamID, threads[i].BoundTaskID)
			}
			if threads[i].BoundArtifactID == "" && strings.TrimSpace(artifact.ArtifactID) != "" {
				threads[i].BoundArtifactID = strings.TrimSpace(artifact.ArtifactID)
				threads[i].BoundArtifactLink = reviewRoomArtifactLink(teamID, threads[i].BoundArtifactID)
			}
			if threads[i].LatestArtifact == "" {
				threads[i].LatestArtifact = reviewRoomArtifactLink(teamID, artifact.ArtifactID)
			}
		}
		threads[i].WorkflowState = reviewRoomWorkflowState(threads[i])
		threads[i].WorkflowLabel = reviewRoomWorkflowLabel(threads[i].WorkflowState)
	}
	return threads
}

func syncReviewRoomThread(ctx context.Context, store *teamcore.Store, teamID, channelID, decisionRef, requestedTaskID, actorAgentID string) (threadSyncResult, error) {
	thread, _, err := loadReviewRoomThreadWithLatestCard(ctx, store, teamID, channelID, decisionRef)
	if err != nil {
		return threadSyncResult{}, err
	}
	suggestedStatus := reviewRoomSuggestedTaskStatus(thread)
	taskID := firstNonEmpty(requestedTaskID, thread.BoundTaskID)
	taskCreated := false
	if taskID == "" {
		if err := requireAction(store, teamID, actorAgentID, "task.create"); err != nil {
			return threadSyncResult{}, err
		}
		now := time.Now().UTC()
		task := teamcore.Task{
			TeamID:      teamID,
			ChannelID:   channelID,
			Title:       firstNonEmpty(thread.Title, thread.Decision),
			Description: thread.LatestSummary,
			CreatedBy:   actorAgentID,
			Status:      firstNonEmpty(suggestedStatus, "open"),
			Labels:      []string{"review-room", "decision-thread"},
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := store.AppendTaskCtx(ctx, teamID, task); err != nil {
			return threadSyncResult{}, err
		}
		tasks, err := store.LoadTasksCtx(ctx, teamID, 1)
		if err != nil || len(tasks) == 0 {
			return threadSyncResult{}, errors.New("failed to load created task")
		}
		taskID = tasks[0].TaskID
		taskCreated = true
		_ = store.AppendHistoryCtx(ctx, teamID, teamcore.ChangeEvent{
			TeamID:    teamID,
			Scope:     "task",
			Action:    "create",
			SubjectID: taskID,
			Summary:   "从 review-room 自动创建绑定任务",
			Metadata: map[string]any{
				"channel_id":      channelID,
				"decision_ref":    decisionRef,
				"task_id":         taskID,
				"actor_agent":     actorAgentID,
				"message_scope":   "review-room",
				"auto_sync":       true,
				"suggested_state": firstNonEmpty(suggestedStatus, "open"),
			},
			CreatedAt: now,
		})
	}
	if taskID != "" && suggestedStatus != "" {
		if err := requireAction(store, teamID, actorAgentID, "task.update"); err != nil {
			return threadSyncResult{}, err
		}
		task, err := store.LoadTaskCtx(ctx, teamID, taskID)
		if err != nil {
			return threadSyncResult{}, err
		}
		if strings.TrimSpace(task.Status) != suggestedStatus {
			updated := task
			updated.Status = suggestedStatus
			updated.UpdatedAt = time.Now().UTC()
			if err := store.SaveTaskCtx(ctx, teamID, updated); err != nil {
				return threadSyncResult{}, err
			}
			_ = store.AppendHistoryCtx(ctx, teamID, teamcore.ChangeEvent{
				TeamID:    teamID,
				Scope:     "task",
				Action:    "update",
				SubjectID: taskID,
				Summary:   "从 review-room 自动同步任务状态",
				Metadata: map[string]any{
					"channel_id":      channelID,
					"decision_ref":    decisionRef,
					"task_id":         taskID,
					"status":          suggestedStatus,
					"actor_agent":     actorAgentID,
					"message_scope":   "review-room",
					"auto_sync":       true,
					"suggested_state": suggestedStatus,
				},
				CreatedAt: updated.UpdatedAt,
			})
		}
	}
	if err := requireAction(store, teamID, actorAgentID, "artifact.create"); err != nil {
		return threadSyncResult{}, err
	}
	artifact, created, err := ensureReviewRoomThreadArtifact(ctx, store, teamID, channelID, decisionRef, taskID, actorAgentID, "")
	if err != nil {
		return threadSyncResult{}, err
	}
	return threadSyncResult{
		TaskID:          taskID,
		TaskStatus:      suggestedStatus,
		TaskCreated:     taskCreated,
		ArtifactID:      artifact.ArtifactID,
		ArtifactCreated: created,
	}, nil
}

func loadReviewRoomThread(ctx context.Context, store *teamcore.Store, teamID, channelID, decisionRef string) (reviewRoomDecisionThreadDigest, error) {
	thread, _, err := loadReviewRoomThreadWithLatestCard(ctx, store, teamID, channelID, decisionRef)
	return thread, err
}

func loadReviewRoomThreadWithLatestCard(ctx context.Context, store *teamcore.Store, teamID, channelID, decisionRef string) (reviewRoomDecisionThreadDigest, *reviewRoomCardView, error) {
	decisionRef = strings.TrimSpace(decisionRef)
	if decisionRef == "" {
		return reviewRoomDecisionThreadDigest{}, nil, errors.New("empty decision_ref")
	}
	messages, err := store.LoadAllMessagesCtx(ctx, teamID, channelID)
	if err != nil {
		return reviewRoomDecisionThreadDigest{}, nil, err
	}
	artifacts, err := store.LoadArtifactsCtx(ctx, teamID, 200)
	if err != nil {
		return reviewRoomDecisionThreadDigest{}, nil, err
	}
	data := buildReviewRoomPageData(teamID, channelID, "", "", "", messages, filterReviewRoomMessages(messages, ""), artifacts, nil)
	var latest *reviewRoomCardView
	for i := range data.Cards {
		if strings.TrimSpace(data.Cards[i].DecisionRef) != decisionRef {
			continue
		}
		if latest == nil || data.Cards[i].CreatedAt.After(latest.CreatedAt) {
			cardCopy := data.Cards[i]
			latest = &cardCopy
		}
	}
	for _, thread := range data.DecisionThreads {
		if strings.TrimSpace(thread.Decision) == decisionRef {
			return thread, latest, nil
		}
	}
	return reviewRoomDecisionThreadDigest{}, nil, errors.New("review-room thread not found")
}

func buildReviewRoomBatchRuns(teamID, channelID string, history []teamcore.ChangeEvent) []reviewRoomBatchRun {
	channelID = strings.TrimSpace(channelID)
	out := make([]reviewRoomBatchRun, 0, 4)
	for _, event := range history {
		if strings.TrimSpace(event.Scope) != "room" || strings.TrimSpace(event.Action) != "sync" {
			continue
		}
		if strings.TrimSpace(stringMetadata(event.Metadata, "message_scope")) != "review-room" {
			continue
		}
		if strings.TrimSpace(stringMetadata(event.Metadata, "batch_action")) != "thread-sync-all" {
			continue
		}
		if strings.TrimSpace(stringMetadata(event.Metadata, "channel_id")) != channelID {
			continue
		}
		out = append(out, reviewRoomBatchRun{
			CreatedAt:                event.CreatedAt,
			ActorAgentID:             strings.TrimSpace(event.ActorAgentID),
			SyncedThreads:            intMetadata(event.Metadata, "synced_threads"),
			TaskCreated:              intMetadata(event.Metadata, "task_created"),
			ArtifactCreated:          intMetadata(event.Metadata, "artifact_created"),
			TotalThreads:             intMetadata(event.Metadata, "total_threads"),
			SuggestedBlockedCount:    intMetadata(event.Metadata, "suggested_blocked_count"),
			SuggestedDoingCount:      intMetadata(event.Metadata, "suggested_doing_count"),
			SuggestedDoneCount:       intMetadata(event.Metadata, "suggested_done_count"),
			NeedsRiskFollowupCount:   intMetadata(event.Metadata, "needs_risk_followup_count"),
			NeedsReviewCount:         intMetadata(event.Metadata, "needs_review_count"),
			ReadyToDistillCount:      intMetadata(event.Metadata, "ready_to_distill_count"),
			DistilledUnassignedCount: intMetadata(event.Metadata, "distilled_unassigned_count"),
			CompletedCount:           intMetadata(event.Metadata, "completed_count"),
			HistoryLink:              reviewRoomBatchHistoryLink(teamID, channelID),
			CreatedTaskIDs:           stringSliceMetadata(event.Metadata, "created_task_ids"),
			CreatedArtifactIDs:       stringSliceMetadata(event.Metadata, "created_artifact_ids"),
		})
		last := &out[len(out)-1]
		last.CreatedTaskLinks = buildReviewRoomTaskLinks(teamID, last.CreatedTaskIDs)
		last.CreatedArtifactLinks = buildReviewRoomArtifactLinks(teamID, last.CreatedArtifactIDs)
	}
	return out
}

func buildReviewRoomTaskLinks(teamID string, taskIDs []string) []string {
	out := make([]string, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		if link := reviewRoomTaskLink(teamID, taskID); link != "" {
			out = append(out, link)
		}
	}
	return out
}

func buildReviewRoomArtifactLinks(teamID string, artifactIDs []string) []string {
	out := make([]string, 0, len(artifactIDs))
	for _, artifactID := range artifactIDs {
		if link := reviewRoomArtifactLink(teamID, artifactID); link != "" {
			out = append(out, link)
		}
	}
	return out
}

func reviewRoomBatchHistoryLink(teamID, channelID string) string {
	channelID = strings.TrimSpace(channelID)
	if teamID == "" || channelID == "" {
		return ""
	}
	return fmt.Sprintf("/teams/%s/history?scope=room&q=%s", teamID, url.QueryEscape(channelID))
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

func ensureReviewRoomThreadArtifact(ctx context.Context, store *teamcore.Store, teamID, channelID, decisionRef, taskID, actorAgentID, title string) (teamcore.Artifact, bool, error) {
	thread, latestCard, err := loadReviewRoomThreadWithLatestCard(ctx, store, teamID, channelID, decisionRef)
	if err != nil {
		return teamcore.Artifact{}, false, err
	}
	artifacts, err := store.LoadArtifactsCtx(ctx, teamID, 200)
	if err != nil {
		return teamcore.Artifact{}, false, err
	}
	if existing, ok := reviewRoomDecisionArtifact(channelID, decisionRef, artifacts); ok {
		return existing, false, nil
	}
	body, _ := json.MarshalIndent(map[string]any{
		"decision":              thread.Decision,
		"title":                 thread.Title,
		"decision_count":        thread.DecisionCount,
		"risk_count":            thread.RiskCount,
		"review_count":          thread.ReviewCount,
		"open_risk_count":       thread.OpenRiskCount,
		"pending_review_count":  thread.PendingReview,
		"distilled_count":       thread.DistilledCount,
		"latest_summary":        thread.LatestSummary,
		"bound_task_id":         firstNonEmpty(taskID, thread.BoundTaskID),
		"bound_artifact_id":     thread.BoundArtifactID,
		"latest_artifact_link":  thread.LatestArtifact,
		"task_search_link":      thread.TaskSearchLink,
		"artifact_search_link":  thread.ArtifactSearchLink,
		"history_search_link":   thread.HistorySearchLink,
		"suggested_task_status": reviewRoomSuggestedTaskStatus(thread),
	}, "", "  ")
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Review Summary: " + firstNonEmpty(thread.Title, thread.Decision)
	}
	labels := []string{
		"distilled",
		"review-room",
		"source-decision:" + decisionRef,
	}
	if latestCard != nil && strings.TrimSpace(latestCard.MessageID) != "" {
		labels = append(labels, "source-message:"+latestCard.MessageID)
	}
	artifact := teamcore.Artifact{
		TeamID:    teamID,
		ChannelID: channelID,
		TaskID:    firstNonEmpty(taskID, thread.BoundTaskID),
		Title:     title,
		Kind:      "review-summary",
		Summary:   thread.LatestSummary,
		Content:   string(body),
		CreatedBy: actorAgentID,
		Labels:    labels,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := store.AppendArtifactCtx(ctx, teamID, artifact); err != nil {
		return teamcore.Artifact{}, false, err
	}
	if strings.TrimSpace(artifact.ArtifactID) == "" {
		artifacts, err = store.LoadArtifactsCtx(ctx, teamID, 200)
		if err != nil {
			return teamcore.Artifact{}, false, err
		}
		if persisted, ok := reviewRoomDecisionArtifact(channelID, decisionRef, artifacts); ok {
			artifact = persisted
		}
	}
	_ = store.AppendHistoryCtx(ctx, teamID, teamcore.ChangeEvent{
		TeamID:    teamID,
		Scope:     "artifact",
		Action:    "create",
		SubjectID: artifact.ArtifactID,
		Summary:   "从 review-room 结论线程沉淀 Artifact",
		Metadata: map[string]any{
			"channel_id":      channelID,
			"decision_ref":    decisionRef,
			"artifact_kind":   artifact.Kind,
			"artifact_source": "review-room-thread",
			"actor_agent":     actorAgentID,
			"task_id":         artifact.TaskID,
		},
		CreatedAt: artifact.CreatedAt,
	})
	return artifact, true, nil
}

func reviewRoomDecisionArtifact(channelID, decisionRef string, artifacts []teamcore.Artifact) (teamcore.Artifact, bool) {
	channelID = strings.TrimSpace(channelID)
	decisionRef = strings.TrimSpace(decisionRef)
	var latest teamcore.Artifact
	found := false
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.ChannelID) != channelID || strings.TrimSpace(artifact.Kind) != "review-summary" {
			continue
		}
		match := false
		for _, label := range artifact.Labels {
			if strings.TrimSpace(label) == "source-decision:"+decisionRef {
				match = true
				break
			}
		}
		if !match {
			continue
		}
		if !found || artifact.CreatedAt.After(latest.CreatedAt) {
			latest = artifact
			found = true
		}
	}
	return latest, found
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

func reviewRoomCardDetails(kind string, data map[string]any) (summary, detailLabel, detail, itemsLabel string, items []string) {
	summary = strings.TrimSpace(stringField(data, "summary"))
	switch strings.TrimSpace(kind) {
	case "review":
		detailLabel = "建议"
		detail = strings.TrimSpace(stringField(data, "recommendation"))
		itemsLabel = "检查项"
		items = stringSliceField(data, "checklist")
	case "decision":
		detailLabel = "决策"
		detail = strings.TrimSpace(stringField(data, "decision"))
		itemsLabel = "后续动作"
		items = stringSliceField(data, "next_steps")
	case "risk":
		detailLabel = "影响"
		detail = strings.TrimSpace(stringField(data, "impact"))
		itemsLabel = "缓解动作"
		items = stringSliceField(data, "mitigation")
	}
	return summary, detailLabel, detail, itemsLabel, items
}

func reviewRoomDecisionRef(kind string, data map[string]any) string {
	ref := strings.TrimSpace(stringField(data, "decision_ref"))
	if ref != "" {
		return ref
	}
	if strings.TrimSpace(kind) == "decision" {
		return firstNonEmpty(stringField(data, "decision"), stringField(data, "title"))
	}
	return ""
}

func stringField(data map[string]any, key string) string {
	if len(data) == 0 {
		return ""
	}
	value, _ := data[key].(string)
	return value
}

func stringSliceField(data map[string]any, key string) []string {
	if len(data) == 0 {
		return nil
	}
	raw, ok := data[key].([]any)
	if ok {
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, strings.TrimSpace(text))
			}
		}
		return out
	}
	if cast, ok := data[key].([]string); ok {
		out := make([]string, 0, len(cast))
		for _, item := range cast {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	}
	return nil
}

func renderReviewRoomPage(w http.ResponseWriter, data reviewRoomPageData) error {
	tmpl, err := template.New("channel.html").ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return err
	}
	return tmpl.ExecuteTemplate(w, "channel.html", data)
}

func decodePostReviewRoomRequest(r *http.Request) (postReviewRoomRequest, error) {
	if isJSONRequest(r) {
		var req postReviewRoomRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return postReviewRoomRequest{}, err
		}
		return req, nil
	}
	if err := r.ParseForm(); err != nil {
		return postReviewRoomRequest{}, err
	}
	req := postReviewRoomRequest{
		ChannelID:     strings.TrimSpace(r.FormValue("channel_id")),
		AuthorAgentID: strings.TrimSpace(r.FormValue("author_agent_id")),
		Kind:          strings.TrimSpace(r.FormValue("kind")),
	}
	req.StructuredData = buildStructuredDataFromForm(req.Kind, r)
	req.Content = buildContentFromForm(req.StructuredData, r)
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

func decodeThreadTaskStatusRequest(r *http.Request) (threadTaskStatusRequest, error) {
	if isJSONRequest(r) {
		var req threadTaskStatusRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return threadTaskStatusRequest{}, err
		}
		return req, nil
	}
	if err := r.ParseForm(); err != nil {
		return threadTaskStatusRequest{}, err
	}
	return threadTaskStatusRequest{
		ChannelID:    strings.TrimSpace(r.FormValue("channel_id")),
		DecisionRef:  strings.TrimSpace(r.FormValue("decision_ref")),
		TaskID:       strings.TrimSpace(r.FormValue("task_id")),
		ActorAgentID: strings.TrimSpace(r.FormValue("actor_agent_id")),
		Status:       strings.TrimSpace(r.FormValue("status")),
	}, nil
}

func decodeThreadArtifactRequest(r *http.Request) (threadArtifactRequest, error) {
	if isJSONRequest(r) {
		var req threadArtifactRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return threadArtifactRequest{}, err
		}
		return req, nil
	}
	if err := r.ParseForm(); err != nil {
		return threadArtifactRequest{}, err
	}
	return threadArtifactRequest{
		ChannelID:    strings.TrimSpace(r.FormValue("channel_id")),
		DecisionRef:  strings.TrimSpace(r.FormValue("decision_ref")),
		TaskID:       strings.TrimSpace(r.FormValue("task_id")),
		ActorAgentID: strings.TrimSpace(r.FormValue("actor_agent_id")),
		Title:        strings.TrimSpace(r.FormValue("title")),
	}, nil
}

func decodeThreadSyncRequest(r *http.Request) (threadSyncRequest, error) {
	if isJSONRequest(r) {
		var req threadSyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return threadSyncRequest{}, err
		}
		return req, nil
	}
	if err := r.ParseForm(); err != nil {
		return threadSyncRequest{}, err
	}
	return threadSyncRequest{
		ChannelID:    strings.TrimSpace(r.FormValue("channel_id")),
		DecisionRef:  strings.TrimSpace(r.FormValue("decision_ref")),
		TaskID:       strings.TrimSpace(r.FormValue("task_id")),
		ActorAgentID: strings.TrimSpace(r.FormValue("actor_agent_id")),
	}, nil
}

func decodeThreadSyncAllRequest(r *http.Request) (threadSyncAllRequest, error) {
	if isJSONRequest(r) {
		var req threadSyncAllRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return threadSyncAllRequest{}, err
		}
		return req, nil
	}
	if err := r.ParseForm(); err != nil {
		return threadSyncAllRequest{}, err
	}
	return threadSyncAllRequest{
		ChannelID:    strings.TrimSpace(r.FormValue("channel_id")),
		ActorAgentID: strings.TrimSpace(r.FormValue("actor_agent_id")),
	}, nil
}

func validTaskStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "open", "doing", "blocked", "done":
		return true
	default:
		return false
	}
}

func buildStructuredDataFromForm(kind string, r *http.Request) map[string]any {
	items := parseLines(r.FormValue("items"))
	switch strings.TrimSpace(kind) {
	case "review":
		return map[string]any{
			"kind":           "review",
			"title":          strings.TrimSpace(r.FormValue("title")),
			"decision_ref":   strings.TrimSpace(r.FormValue("decision_ref")),
			"task_id":        strings.TrimSpace(r.FormValue("task_id")),
			"artifact_id":    strings.TrimSpace(r.FormValue("artifact_id")),
			"summary":        strings.TrimSpace(r.FormValue("summary")),
			"checklist":      items,
			"recommendation": strings.TrimSpace(r.FormValue("detail")),
		}
	case "decision":
		return map[string]any{
			"kind":        "decision",
			"title":       strings.TrimSpace(r.FormValue("title")),
			"task_id":     strings.TrimSpace(r.FormValue("task_id")),
			"artifact_id": strings.TrimSpace(r.FormValue("artifact_id")),
			"summary":     strings.TrimSpace(r.FormValue("summary")),
			"decision":    strings.TrimSpace(r.FormValue("detail")),
			"next_steps":  items,
		}
	case "risk":
		return map[string]any{
			"kind":         "risk",
			"title":        strings.TrimSpace(r.FormValue("title")),
			"decision_ref": strings.TrimSpace(r.FormValue("decision_ref")),
			"task_id":      strings.TrimSpace(r.FormValue("task_id")),
			"artifact_id":  strings.TrimSpace(r.FormValue("artifact_id")),
			"summary":      strings.TrimSpace(r.FormValue("summary")),
			"impact":       strings.TrimSpace(r.FormValue("detail")),
			"mitigation":   items,
		}
	default:
		return nil
	}
}

func buildContentFromForm(structuredData map[string]any, r *http.Request) string {
	if content := strings.TrimSpace(r.FormValue("content")); content != "" {
		return content
	}
	if summary, _ := structuredData["summary"].(string); strings.TrimSpace(summary) != "" {
		return strings.TrimSpace(summary)
	}
	return reviewRoomStructuredTitle(structuredData)
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

func reviewRoomStructuredTitle(data map[string]any) string {
	if len(data) == 0 {
		return ""
	}
	if title, _ := data["title"].(string); strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title)
	}
	return ""
}

func reviewRoomKindLabel(kind string) string {
	switch strings.TrimSpace(kind) {
	case "review":
		return "[REVIEW]"
	case "risk":
		return "[RISK]"
	case "decision":
		return "[DECISION]"
	default:
		return "[MESSAGE]"
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

func isReviewRoomKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "review", "risk", "decision":
		return true
	default:
		return false
	}
}

func isJSONRequest(r *http.Request) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))), "application/json")
}

func isAPIRequest(r *http.Request) bool {
	return strings.HasPrefix(strings.TrimSpace(r.RequestURI), "/api/teams/")
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
	if actorAgentID == strings.TrimSpace(info.OwnerAgentID) {
		return "owner", nil
	}
	members, err := store.LoadMembersCtx(context.Background(), teamID)
	if err != nil {
		return "", err
	}
	for _, member := range members {
		if strings.EqualFold(strings.TrimSpace(member.AgentID), actorAgentID) {
			role := strings.TrimSpace(member.Role)
			if role == "" {
				return "member", nil
			}
			return role, nil
		}
	}
	return "observer", nil
}
