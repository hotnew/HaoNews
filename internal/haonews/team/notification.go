package team

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	NotificationKindMention      = "mention"
	NotificationKindTaskAssigned = "task_assigned"
	NotificationKindTaskBlocked  = "task_blocked"
	NotificationKindReviewNeeded = "review_needed"
)

type Notification struct {
	NotificationID  string    `json:"notification_id"`
	TeamID          string    `json:"team_id"`
	AgentID         string    `json:"agent_id"`
	Kind            string    `json:"kind"`
	ChannelID       string    `json:"channel_id,omitempty"`
	TaskID          string    `json:"task_id,omitempty"`
	ContextID       string    `json:"context_id,omitempty"`
	SourceMessageID string    `json:"source_message_id,omitempty"`
	Summary         string    `json:"summary,omitempty"`
	CreatedAt       time.Time `json:"created_at,omitempty"`
	ReadAt          time.Time `json:"read_at,omitempty"`
}

type NotificationFilter struct {
	AgentID    string
	Kinds      []string
	UnreadOnly bool
	Limit      int
}

func normalizeNotification(value Notification) Notification {
	value.NotificationID = strings.TrimSpace(value.NotificationID)
	value.TeamID = NormalizeTeamID(value.TeamID)
	value.AgentID = strings.TrimSpace(value.AgentID)
	value.Kind = strings.TrimSpace(value.Kind)
	value.ChannelID = normalizeChannelID(value.ChannelID)
	value.TaskID = strings.TrimSpace(value.TaskID)
	value.ContextID = normalizeContextID(value.ContextID)
	value.SourceMessageID = strings.TrimSpace(value.SourceMessageID)
	value.Summary = strings.TrimSpace(value.Summary)
	if !value.CreatedAt.IsZero() {
		value.CreatedAt = value.CreatedAt.UTC()
	}
	if !value.ReadAt.IsZero() {
		value.ReadAt = value.ReadAt.UTC()
	}
	return value
}

func (s *Store) notificationPath(teamID string) string {
	return filepath.Join(s.root, NormalizeTeamID(teamID), "notifications.jsonl")
}

func buildNotificationID(notification Notification) string {
	at := notification.CreatedAt
	if at.IsZero() {
		at = time.Now().UTC()
	}
	subject := notification.SourceMessageID
	if subject == "" {
		subject = notification.TaskID
	}
	if subject == "" {
		subject = notification.AgentID
	}
	subject = strings.NewReplacer("/", "-", ":", "-", " ", "-").Replace(subject)
	return NormalizeTeamID(notification.TeamID) + ":" + notification.Kind + ":" + subject + ":" + at.UTC().Format(time.RFC3339Nano)
}

func (s *Store) appendNotificationNoCtx(teamID string, notification Notification) error {
	if s == nil {
		return NewNilStoreError("Store")
	}
	teamID = NormalizeTeamID(teamID)
	notification = normalizeNotification(notification)
	if teamID == "" {
		return NewEmptyIDError("team_id")
	}
	if notification.AgentID == "" {
		return NewEmptyIDError("agent_id")
	}
	if notification.Kind == "" {
		return NewEmptyIDError("kind")
	}
	if notification.CreatedAt.IsZero() {
		notification.CreatedAt = time.Now().UTC()
	}
	notification.TeamID = teamID
	if notification.NotificationID == "" {
		notification.NotificationID = buildNotificationID(notification)
	}
	err := s.withTeamLock(teamID, func() error {
		path := s.notificationPath(teamID)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer file.Close()
		body, err := json.Marshal(notification)
		if err != nil {
			return err
		}
		_, err = file.Write(append(body, '\n'))
		return err
	})
	if err == nil {
		s.publish(TeamEvent{
			EventID:   notification.NotificationID,
			TeamID:    teamID,
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
		})
	}
	return err
}

func (s *Store) AppendNotificationCtx(ctx context.Context, teamID string, notification Notification) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	return s.appendNotificationNoCtx(teamID, notification)
}

func (s *Store) ListNotificationsCtx(ctx context.Context, teamID string, filter NotificationFilter) ([]Notification, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, NewNilStoreError("Store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, NewEmptyIDError("team_id")
	}
	data, err := os.Open(s.notificationPath(teamID))
	if err != nil {
		if os.IsNotExist(err) {
			return []Notification{}, nil
		}
		return nil, err
	}
	defer data.Close()
	kindSet := make(map[string]struct{}, len(filter.Kinds))
	for _, kind := range filter.Kinds {
		kind = strings.TrimSpace(kind)
		if kind != "" {
			kindSet[kind] = struct{}{}
		}
	}
	scanner := bufio.NewScanner(data)
	out := make([]Notification, 0, 32)
	for scanner.Scan() {
		if err := ctxErr(ctx); err != nil {
			return nil, err
		}
		var notification Notification
		if err := json.Unmarshal(scanner.Bytes(), &notification); err != nil {
			continue
		}
		notification = normalizeNotification(notification)
		if filter.AgentID != "" && notification.AgentID != strings.TrimSpace(filter.AgentID) {
			continue
		}
		if filter.UnreadOnly && !notification.ReadAt.IsZero() {
			continue
		}
		if len(kindSet) > 0 {
			if _, ok := kindSet[notification.Kind]; !ok {
				continue
			}
		}
		out = append(out, notification)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].NotificationID > out[j].NotificationID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = append([]Notification(nil), out[:filter.Limit]...)
	}
	return out, nil
}

func extractAgentMentions(content string) []string {
	fields := strings.Fields(content)
	if len(fields) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(fields))
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if !strings.HasPrefix(field, "@") {
			continue
		}
		agentID := strings.TrimPrefix(field, "@")
		agentID = strings.TrimRight(agentID, ".,;:!?)]}")
		agentID = strings.TrimSpace(agentID)
		if agentID == "" || !strings.HasPrefix(agentID, "agent://") {
			continue
		}
		if _, ok := seen[agentID]; ok {
			continue
		}
		seen[agentID] = struct{}{}
		out = append(out, agentID)
	}
	return out
}

func buildMentionNotifications(teamID string, msg Message) []Notification {
	mentions := extractAgentMentions(msg.Content)
	if len(mentions) == 0 {
		return nil
	}
	out := make([]Notification, 0, len(mentions))
	for _, agentID := range mentions {
		if strings.TrimSpace(agentID) == strings.TrimSpace(msg.AuthorAgentID) {
			continue
		}
		out = append(out, Notification{
			TeamID:          teamID,
			AgentID:         agentID,
			Kind:            NotificationKindMention,
			ChannelID:       msg.ChannelID,
			ContextID:       msg.ContextID,
			SourceMessageID: msg.MessageID,
			Summary:         summarizePromptText(msg.Content, 160),
			CreatedAt:       msg.CreatedAt,
		})
	}
	return out
}

func buildTaskNotifications(teamID string, before *Task, after Task, created bool) []Notification {
	assignees := normalizeStringList(after.Assignees)
	if len(assignees) == 0 {
		return nil
	}
	beforeAssignees := map[string]struct{}{}
	if before != nil {
		for _, assignee := range normalizeStringList(before.Assignees) {
			beforeAssignees[assignee] = struct{}{}
		}
	}
	out := make([]Notification, 0, len(assignees))
	appendForAll := func(kind string, summary string) {
		for _, assignee := range assignees {
			out = append(out, Notification{
				TeamID:    teamID,
				AgentID:   assignee,
				Kind:      kind,
				ChannelID: after.ChannelID,
				TaskID:    after.TaskID,
				ContextID: after.ContextID,
				Summary:   summary,
				CreatedAt: after.UpdatedAt,
			})
		}
	}
	if created {
		appendForAll(NotificationKindTaskAssigned, after.Title)
		return out
	}
	for _, assignee := range assignees {
		if _, ok := beforeAssignees[assignee]; !ok {
			out = append(out, Notification{
				TeamID:    teamID,
				AgentID:   assignee,
				Kind:      NotificationKindTaskAssigned,
				ChannelID: after.ChannelID,
				TaskID:    after.TaskID,
				ContextID: after.ContextID,
				Summary:   after.Title,
				CreatedAt: after.UpdatedAt,
			})
		}
	}
	if before == nil || normalizeTaskStatus(before.Status) != normalizeTaskStatus(after.Status) {
		switch normalizeTaskStatus(after.Status) {
		case TaskStateBlocked:
			appendForAll(NotificationKindTaskBlocked, after.Title)
		case TaskStateReview:
			appendForAll(NotificationKindReviewNeeded, after.Title)
		}
	}
	return out
}
