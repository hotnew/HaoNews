package team

import (
	"context"
	"sort"
	"strings"
	"time"
)

type TaskFilter struct {
	Statuses         []string  `json:"statuses,omitempty"`
	AssigneeAgentID  string    `json:"assignee_agent_id,omitempty"`
	CreatedByAgentID string    `json:"created_by_agent_id,omitempty"`
	Priorities       []string  `json:"priorities,omitempty"`
	Labels           []string  `json:"labels,omitempty"`
	ChannelID        string    `json:"channel_id,omitempty"`
	DueAfter         time.Time `json:"due_after,omitempty"`
	DueBefore        time.Time `json:"due_before,omitempty"`
	Limit            int       `json:"limit,omitempty"`
	Offset           int       `json:"offset,omitempty"`
}

type ArtifactFilter struct {
	Kinds     []string `json:"kinds,omitempty"`
	TaskID    string   `json:"task_id,omitempty"`
	ChannelID string   `json:"channel_id,omitempty"`
	Labels    []string `json:"labels,omitempty"`
	CreatedBy string   `json:"created_by,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	Offset    int      `json:"offset,omitempty"`
}

type MessageFilter struct {
	ChannelID   string    `json:"channel_id,omitempty"`
	ContextID   string    `json:"context_id,omitempty"`
	AuthorID    string    `json:"author_id,omitempty"`
	MessageType string    `json:"message_type,omitempty"`
	After       time.Time `json:"after,omitempty"`
	Limit       int       `json:"limit,omitempty"`
	Offset      int       `json:"offset,omitempty"`
}

func (s *Store) ListTasksCtx(ctx context.Context, teamID string, filter TaskFilter) ([]Task, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	tasks, err := s.LoadTasksCtx(ctx, teamID, 0)
	if err != nil {
		return nil, err
	}
	filter.AssigneeAgentID = strings.TrimSpace(filter.AssigneeAgentID)
	filter.CreatedByAgentID = strings.TrimSpace(filter.CreatedByAgentID)
	filter.ChannelID = normalizeChannelID(filter.ChannelID)
	statuses := normalizeStatusList(filter.Statuses)
	priorities := normalizePriorityList(filter.Priorities)
	labels := normalizeStringList(filter.Labels)
	out := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		if len(statuses) > 0 && !containsString(statuses, normalizeTaskStatus(task.Status)) {
			continue
		}
		if filter.AssigneeAgentID != "" && !containsNormalized(task.Assignees, filter.AssigneeAgentID) {
			continue
		}
		if filter.CreatedByAgentID != "" && strings.TrimSpace(task.CreatedBy) != filter.CreatedByAgentID {
			continue
		}
		if len(priorities) > 0 && !containsString(priorities, normalizeTaskPriority(task.Priority)) {
			continue
		}
		if len(labels) > 0 && !intersectsNormalized(task.Labels, labels) {
			continue
		}
		if filter.ChannelID != "main" && strings.TrimSpace(filter.ChannelID) != "" && normalizeChannelID(task.ChannelID) != filter.ChannelID {
			continue
		}
		if !filter.DueAfter.IsZero() && (task.DueAt.IsZero() || task.DueAt.Before(filter.DueAfter)) {
			continue
		}
		if !filter.DueBefore.IsZero() && (task.DueAt.IsZero() || task.DueAt.After(filter.DueBefore)) {
			continue
		}
		out = append(out, task)
	}
	return applyWindow(out, filter.Offset, filter.Limit), nil
}

func (s *Store) ListArtifactsCtx(ctx context.Context, teamID string, filter ArtifactFilter) ([]Artifact, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	artifacts, err := s.LoadArtifactsCtx(ctx, teamID, 0)
	if err != nil {
		return nil, err
	}
	filter.TaskID = strings.TrimSpace(filter.TaskID)
	filter.ChannelID = normalizeChannelID(filter.ChannelID)
	filter.CreatedBy = strings.TrimSpace(filter.CreatedBy)
	kinds := normalizeArtifactKinds(filter.Kinds)
	labels := normalizeStringList(filter.Labels)
	out := make([]Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		if len(kinds) > 0 && !containsString(kinds, normalizeArtifactKind(artifact.Kind)) {
			continue
		}
		if filter.TaskID != "" && strings.TrimSpace(artifact.TaskID) != filter.TaskID {
			continue
		}
		if filter.ChannelID != "main" && strings.TrimSpace(filter.ChannelID) != "" && normalizeChannelID(artifact.ChannelID) != filter.ChannelID {
			continue
		}
		if len(labels) > 0 && !intersectsNormalized(artifact.Labels, labels) {
			continue
		}
		if filter.CreatedBy != "" && strings.TrimSpace(artifact.CreatedBy) != filter.CreatedBy {
			continue
		}
		out = append(out, artifact)
	}
	return applyWindow(out, filter.Offset, filter.Limit), nil
}

func (s *Store) ListMessagesCtx(ctx context.Context, teamID string, filter MessageFilter) ([]Message, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	filter.ChannelID = strings.TrimSpace(filter.ChannelID)
	filter.ContextID = normalizeContextID(filter.ContextID)
	filter.AuthorID = strings.TrimSpace(filter.AuthorID)
	filter.MessageType = strings.TrimSpace(filter.MessageType)
	channelIDs := []string{}
	if filter.ChannelID != "" {
		channelIDs = append(channelIDs, normalizeChannelID(filter.ChannelID))
	} else {
		channels, err := s.ListChannelsCtx(ctx, teamID)
		if err != nil {
			return nil, err
		}
		channelIDs = orderedChannelIDs(channels, "main")
	}
	messages, err := loadMessagesMatchingChannelsCtx(ctx, channelIDs, 0, func(message Message) bool {
		if filter.ContextID != "" && normalizeContextID(message.ContextID) != filter.ContextID && structuredDataContextID(message.StructuredData) != filter.ContextID {
			return false
		}
		if filter.AuthorID != "" && strings.TrimSpace(message.AuthorAgentID) != filter.AuthorID {
			return false
		}
		if filter.MessageType != "" && strings.TrimSpace(message.MessageType) != filter.MessageType {
			return false
		}
		if !filter.After.IsZero() && (message.CreatedAt.IsZero() || message.CreatedAt.Before(filter.After)) {
			return false
		}
		return true
	}, func(channelID string) ([]Message, error) {
		return s.LoadAllMessagesCtx(ctx, teamID, channelID)
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(messages, func(i, j int) bool {
		if !messages[i].CreatedAt.Equal(messages[j].CreatedAt) {
			return messages[i].CreatedAt.After(messages[j].CreatedAt)
		}
		return messages[i].MessageID > messages[j].MessageID
	})
	return applyWindow(messages, filter.Offset, filter.Limit), nil
}

func normalizeStatusList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = normalizeTaskStatus(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizePriorityList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = normalizeTaskPriority(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeArtifactKinds(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = normalizeArtifactKind(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func containsNormalized(values []string, needle string) bool {
	needle = strings.TrimSpace(needle)
	for _, value := range values {
		if strings.TrimSpace(value) == needle {
			return true
		}
	}
	return false
}

func intersectsNormalized(values []string, needles []string) bool {
	if len(values) == 0 || len(needles) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	for _, needle := range needles {
		needle = strings.TrimSpace(strings.ToLower(needle))
		if needle == "" {
			continue
		}
		if _, ok := set[needle]; ok {
			return true
		}
	}
	return false
}

func applyWindow[T any](values []T, offset, limit int) []T {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(values) {
		return nil
	}
	values = values[offset:]
	if limit > 0 && len(values) > limit {
		values = values[:limit]
	}
	return append([]T(nil), values...)
}
