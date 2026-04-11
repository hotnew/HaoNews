package team

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type ChannelThreadSummary struct {
	TaskID               string `json:"task_id"`
	Title                string `json:"title"`
	Status               string `json:"status"`
	ContextID            string `json:"context_id,omitempty"`
	AssignedAgentID      string `json:"assigned_agent_id,omitempty"`
	DispatchStatus       string `json:"dispatch_status,omitempty"`
	MessageCount         int    `json:"message_count"`
	LatestMessageType    string `json:"latest_message_type,omitempty"`
	LatestMessagePreview string `json:"latest_message_preview,omitempty"`
}

type ChannelContext struct {
	Team            Info                   `json:"team"`
	Channel         Channel                `json:"channel"`
	AgentOnboarding string                 `json:"agent_onboarding,omitempty"`
	ActiveTasks     []Task                 `json:"active_tasks,omitempty"`
	RecentMessages  []Message              `json:"recent_messages,omitempty"`
	Threads         []ChannelThreadSummary `json:"threads,omitempty"`
	ActiveMembers   []Member               `json:"active_members,omitempty"`
	Policy          Policy                 `json:"policy"`
	AgentPrompt     string                 `json:"agent_prompt,omitempty"`
	SnapshotAt      string                 `json:"snapshot_at"`
}

type ChannelContextProvider interface {
	GetChannelContext(ctx context.Context, teamID, channelID string) (ChannelContext, error)
}

type StoreChannelContextProvider struct {
	store TeamReader
}

func NewChannelContextProvider(store TeamReader) ChannelContextProvider {
	return &StoreChannelContextProvider{store: store}
}

func (p *StoreChannelContextProvider) GetChannelContext(ctx context.Context, teamID, channelID string) (ChannelContext, error) {
	if p == nil || p.store == nil {
		return ChannelContext{}, NewNilStoreError("ChannelContextProvider")
	}
	info, err := p.store.LoadTeamCtx(ctx, teamID)
	if err != nil {
		return ChannelContext{}, err
	}
	channel, err := p.store.LoadChannelCtx(ctx, teamID, channelID)
	if err != nil {
		return ChannelContext{}, err
	}
	cfg, err := p.store.LoadChannelConfigCtx(ctx, teamID, channelID)
	if err != nil {
		return ChannelContext{}, err
	}
	members, err := p.store.LoadMembersCtx(ctx, teamID)
	if err != nil {
		return ChannelContext{}, err
	}
	policy, err := p.store.LoadPolicyCtx(ctx, teamID)
	if err != nil {
		return ChannelContext{}, err
	}
	tasks, err := p.store.ListTasksCtx(ctx, teamID, TaskFilter{
		ChannelID: channelID,
		Statuses:  []string{TaskStateOpen, TaskStateDispatched, TaskStateDoing, TaskStateBlocked, TaskStateReview},
	})
	if err != nil {
		return ChannelContext{}, err
	}
	messages, err := p.store.ListMessagesCtx(ctx, teamID, MessageFilter{
		ChannelID: channelID,
		Limit:     20,
	})
	if err != nil {
		return ChannelContext{}, err
	}
	snapshot := ChannelContext{
		Team:            info,
		Channel:         channel,
		AgentOnboarding: cfg.AgentOnboarding,
		ActiveTasks:     tasks,
		RecentMessages:  messages,
		Threads:         buildChannelThreadSummaries(ctx, p.store, teamID, tasks),
		ActiveMembers:   filterActiveMembers(members),
		Policy:          policy,
		SnapshotAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	snapshot.AgentPrompt = buildChannelAgentPrompt(snapshot)
	return snapshot, nil
}

func filterActiveMembers(members []Member) []Member {
	out := make([]Member, 0, len(members))
	for _, member := range members {
		if normalizeMemberStatus(member.Status) != MemberStatusActive {
			continue
		}
		out = append(out, member)
	}
	return out
}

func buildChannelThreadSummaries(ctx context.Context, store TeamReader, teamID string, tasks []Task) []ChannelThreadSummary {
	if store == nil || len(tasks) == 0 {
		return nil
	}
	limit := len(tasks)
	if limit > 5 {
		limit = 5
	}
	out := make([]ChannelThreadSummary, 0, limit)
	for _, task := range tasks[:limit] {
		thread, err := store.LoadTaskThreadCtx(ctx, teamID, task.TaskID, 8)
		if err != nil {
			continue
		}
		summary := ChannelThreadSummary{
			TaskID:       task.TaskID,
			Title:        task.Title,
			Status:       task.Status,
			ContextID:    task.ContextID,
			MessageCount: len(thread.Messages),
		}
		if thread.Dispatch != nil {
			summary.AssignedAgentID = thread.Dispatch.AssignedAgentID
			summary.DispatchStatus = thread.Dispatch.Status
		}
		if len(thread.Messages) > 0 {
			summary.LatestMessageType = thread.Messages[0].MessageType
			summary.LatestMessagePreview = summarizePromptText(thread.Messages[0].Content, 120)
		}
		out = append(out, summary)
	}
	return out
}

func buildChannelAgentPrompt(snapshot ChannelContext) string {
	lines := make([]string, 0, 16)
	if onboarding := strings.TrimSpace(snapshot.AgentOnboarding); onboarding != "" {
		lines = append(lines, "Agent Onboarding:")
		lines = append(lines, onboarding, "")
	}
	lines = append(lines, fmt.Sprintf("Team=%s Channel=%s Visibility=%s", snapshot.Team.TeamID, snapshot.Channel.ChannelID, dashIfBlank(snapshot.Team.Visibility)))
	if len(snapshot.ActiveTasks) > 0 {
		lines = append(lines, "Active Tasks:")
		taskLimit := len(snapshot.ActiveTasks)
		if taskLimit > 3 {
			taskLimit = 3
		}
		for _, task := range snapshot.ActiveTasks[:taskLimit] {
			lines = append(lines, fmt.Sprintf("- [%s] %s (%s)", dashIfBlank(task.Status), summarizePromptText(task.Title, 80), dashIfBlank(task.TaskID)))
		}
	}
	if len(snapshot.Threads) > 0 {
		lines = append(lines, "Thread Summary:")
		threadLimit := len(snapshot.Threads)
		if threadLimit > 3 {
			threadLimit = 3
		}
		for _, thread := range snapshot.Threads[:threadLimit] {
			line := fmt.Sprintf("- %s [%s]", summarizePromptText(thread.Title, 80), dashIfBlank(thread.Status))
			if strings.TrimSpace(thread.AssignedAgentID) != "" {
				line += " -> " + thread.AssignedAgentID
			}
			if strings.TrimSpace(thread.LatestMessagePreview) != "" {
				line += " | " + thread.LatestMessagePreview
			}
			lines = append(lines, line)
		}
	}
	if len(snapshot.RecentMessages) > 0 {
		lines = append(lines, "Recent Messages:")
		messageLimit := len(snapshot.RecentMessages)
		if messageLimit > 3 {
			messageLimit = 3
		}
		for _, message := range snapshot.RecentMessages[:messageLimit] {
			lines = append(lines, fmt.Sprintf("- %s: %s", dashIfBlank(message.AuthorAgentID), summarizePromptText(message.Content, 100)))
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func summarizePromptText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 1 {
		return value[:limit]
	}
	return value[:limit-1] + "…"
}

func dashIfBlank(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}
