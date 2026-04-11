package team

import (
	"context"
	"strings"
	"time"
)

type MemberStats struct {
	AgentID       string    `json:"agent_id"`
	MessageCount  int       `json:"message_count"`
	TaskCreated   int       `json:"task_created"`
	TaskClosed    int       `json:"task_closed"`
	ArtifactCount int       `json:"artifact_count"`
	LastActiveAt  time.Time `json:"last_active_at,omitempty"`
}

func (s *Store) ComputeMemberStatsCtx(ctx context.Context, teamID string) (map[string]MemberStats, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	members, err := s.LoadMembersCtx(ctx, teamID)
	if err != nil {
		return nil, err
	}
	tasks, err := s.LoadTasksCtx(ctx, teamID, 0)
	if err != nil {
		return nil, err
	}
	artifacts, err := s.LoadArtifactsCtx(ctx, teamID, 0)
	if err != nil {
		return nil, err
	}
	channels, err := s.ListChannelsCtx(ctx, teamID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]MemberStats, len(members))
	ensure := func(agentID string) MemberStats {
		stats := out[agentID]
		if stats.AgentID == "" {
			stats.AgentID = agentID
		}
		return stats
	}
	touch := func(stats MemberStats, at time.Time) MemberStats {
		if !at.IsZero() && stats.LastActiveAt.Before(at) {
			stats.LastActiveAt = at.UTC()
		}
		return stats
	}
	for _, member := range members {
		member.AgentID = strings.TrimSpace(member.AgentID)
		if member.AgentID == "" {
			continue
		}
		out[member.AgentID] = ensure(member.AgentID)
	}
	for _, channel := range channels {
		messages, err := s.LoadMessagesCtx(ctx, teamID, channel.ChannelID, 0)
		if err != nil {
			return nil, err
		}
		for _, message := range messages {
			agentID := strings.TrimSpace(message.AuthorAgentID)
			if agentID == "" {
				continue
			}
			stats := ensure(agentID)
			stats.MessageCount++
			stats = touch(stats, message.CreatedAt)
			out[agentID] = stats
		}
	}
	for _, task := range tasks {
		if agentID := strings.TrimSpace(task.CreatedBy); agentID != "" {
			stats := ensure(agentID)
			stats.TaskCreated++
			stats = touch(stats, task.CreatedAt)
			out[agentID] = stats
		}
		if IsTerminalState(task.Status) {
			targets := normalizeStringList(task.Assignees)
			if len(targets) == 0 && strings.TrimSpace(task.CreatedBy) != "" {
				targets = []string{strings.TrimSpace(task.CreatedBy)}
			}
			for _, agentID := range targets {
				stats := ensure(agentID)
				stats.TaskClosed++
				stats = touch(stats, nonZeroTaskCloseTime(task))
				out[agentID] = stats
			}
		}
	}
	for _, artifact := range artifacts {
		agentID := strings.TrimSpace(artifact.CreatedBy)
		if agentID == "" {
			continue
		}
		stats := ensure(agentID)
		stats.ArtifactCount++
		stats = touch(stats, artifact.UpdatedAt)
		out[agentID] = stats
	}
	return out, nil
}

func nonZeroTaskCloseTime(task Task) time.Time {
	if !task.ClosedAt.IsZero() {
		return task.ClosedAt
	}
	if !task.UpdatedAt.IsZero() {
		return task.UpdatedAt
	}
	return task.CreatedAt
}
