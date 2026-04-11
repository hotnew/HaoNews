package haonewsteam

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	teamcore "hao.news/internal/haonews/team"
	newsplugin "hao.news/internal/plugins/haonews"
)

func handleAPITeamNotifications(store teamcore.TeamReader, teamID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	filter := teamcore.NotificationFilter{
		AgentID:    strings.TrimSpace(r.URL.Query().Get("agent_id")),
		UnreadOnly: strings.TrimSpace(r.URL.Query().Get("status")) == "unread",
		Limit:      clampTeamListLimit(r.URL.Query().Get("limit"), 50, 200),
	}
	if kinds := strings.TrimSpace(r.URL.Query().Get("kind")); kinds != "" {
		filter.Kinds = strings.Split(kinds, ",")
	}
	notifications, err := store.ListNotificationsCtx(r.Context(), teamID, filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":              "team-notifications",
		"team_id":            teamID,
		"agent_id":           filter.AgentID,
		"status":             strings.TrimSpace(r.URL.Query().Get("status")),
		"count":              len(notifications),
		"notifications":      notifications,
		"notification_kinds": filter.Kinds,
	})
}

func handleAPITeamNotificationsStream(store teamcore.TeamReader, teamID string, w http.ResponseWriter, r *http.Request) {
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
	agentID := strings.TrimSpace(r.URL.Query().Get("agent_id"))
	initialNotifications, err := store.ListNotificationsCtx(r.Context(), teamID, teamcore.NotificationFilter{
		AgentID: agentID,
		Limit:   20,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
	_, _ = fmt.Fprint(w, ": team-notifications\n\n")
	flusher.Flush()
	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()
	writeEvent := func(event teamcore.TeamEvent) error {
		body, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: notification\ndata: %s\n\n", body); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	for _, notification := range initialNotifications {
		if err := writeEvent(teamcore.TeamEvent{
			EventID:   notification.NotificationID,
			TeamID:    notification.TeamID,
			Kind:      "notification",
			Action:    "create",
			SubjectID: notification.NotificationID,
			ChannelID: notification.ChannelID,
			ContextID: notification.ContextID,
			Metadata: map[string]any{
				"agent_id":          notification.AgentID,
				"notification_kind": notification.Kind,
				"task_id":           notification.TaskID,
			},
			CreatedAt: notification.CreatedAt,
		}); err != nil {
			return
		}
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-events:
			if event.Kind != "notification" {
				continue
			}
			if agentID != "" {
				if got := strings.TrimSpace(fmt.Sprint(event.Metadata["agent_id"])); got != agentID {
					continue
				}
			}
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
