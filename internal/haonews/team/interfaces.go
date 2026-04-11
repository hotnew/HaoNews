package team

import "context"

type TeamReader interface {
	LoadTeamCtx(ctx context.Context, teamID string) (Info, error)
	LoadChannelCtx(ctx context.Context, teamID, channelID string) (Channel, error)
	LoadChannelConfigCtx(ctx context.Context, teamID, channelID string) (ChannelConfig, error)
	LoadMembersCtx(ctx context.Context, teamID string) ([]Member, error)
	ComputeMemberStatsCtx(ctx context.Context, teamID string) (map[string]MemberStats, error)
	LoadPolicyCtx(ctx context.Context, teamID string) (Policy, error)
	ListChannelsCtx(ctx context.Context, teamID string) ([]ChannelSummary, error)
	ListTasksCtx(ctx context.Context, teamID string, filter TaskFilter) ([]Task, error)
	ListMessagesCtx(ctx context.Context, teamID string, filter MessageFilter) ([]Message, error)
	LoadTasksCtx(ctx context.Context, teamID string, limit int) ([]Task, error)
	LoadTaskCtx(ctx context.Context, teamID, taskID string) (Task, error)
	LoadArtifactsCtx(ctx context.Context, teamID string, limit int) ([]Artifact, error)
	LoadHistoryCtx(ctx context.Context, teamID string, limit int) ([]ChangeEvent, error)
	LoadMessagesCtx(ctx context.Context, teamID, channelID string, limit int) ([]Message, error)
	LoadAllMessagesCtx(ctx context.Context, teamID, channelID string) ([]Message, error)
	LoadTasksByContextCtx(ctx context.Context, teamID, contextID string) ([]Task, error)
	LoadTaskThreadCtx(ctx context.Context, teamID, taskID string, limit int) (TaskThread, error)
	ListNotificationsCtx(ctx context.Context, teamID string, filter NotificationFilter) ([]Notification, error)
	Subscribe(teamID string) (<-chan TeamEvent, func(), error)
}

type TeamWriter interface {
	SaveTeamCtx(ctx context.Context, info Info) error
	SaveMembersCtx(ctx context.Context, teamID string, members []Member) error
	SavePolicyCtx(ctx context.Context, teamID string, policy Policy) error
	SaveChannelCtx(ctx context.Context, teamID string, channel Channel) error
	SaveChannelConfigCtx(ctx context.Context, teamID string, cfg ChannelConfig) error
	AppendMessageCtx(ctx context.Context, teamID string, msg Message) error
	AppendTaskCtx(ctx context.Context, teamID string, task Task) error
	SaveTaskCtx(ctx context.Context, teamID string, task Task) error
	AppendArtifactCtx(ctx context.Context, teamID string, artifact Artifact) error
	AppendHistoryCtx(ctx context.Context, teamID string, event ChangeEvent) error
	SaveMilestoneCtx(ctx context.Context, teamID string, milestone Milestone) error
	DeleteMilestoneCtx(ctx context.Context, teamID, milestoneID string) error
}

type TeamStore interface {
	TeamReader
	TeamWriter
}
