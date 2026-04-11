package team

import (
	"context"
	"time"
)

// Deprecated: this file contains compatibility wrappers that call the ctx-aware
// Store API with context.Background(). New code should always use the ...Ctx
// variants directly. These wrappers remain only to keep older callers building
// during the v0.5.x line and should be removed after the compatibility window.

func (s *Store) ListTeams() ([]Summary, error) {
	return s.ListTeamsCtx(context.Background())
}

func (s *Store) LoadChannelMessages(teamID, channelID string, limit int) ([]Message, error) {
	return s.LoadMessagesCtx(context.Background(), teamID, channelID, limit)
}

func (s *Store) LoadAllMessages(teamID, channelID string) ([]Message, error) {
	return s.LoadAllMessagesCtx(context.Background(), teamID, channelID)
}

func (s *Store) LoadTeam(teamID string) (Info, error) {
	return s.LoadTeamCtx(context.Background(), teamID)
}

func (s *Store) LoadMembers(teamID string) ([]Member, error) {
	return s.LoadMembersCtx(context.Background(), teamID)
}

func (s *Store) SaveMembers(teamID string, members []Member) error {
	return s.SaveMembersCtx(context.Background(), teamID, members)
}

func (s *Store) LoadMembersSnapshot(teamID string) ([]Member, time.Time, error) {
	return s.LoadMembersSnapshotCtx(context.Background(), teamID)
}

func (s *Store) LoadPolicySnapshot(teamID string) (Policy, time.Time, error) {
	return s.LoadPolicySnapshotCtx(context.Background(), teamID)
}

func (s *Store) LoadChannelSnapshot(teamID, channelID string) (Channel, time.Time, error) {
	return s.LoadChannelSnapshotCtx(context.Background(), teamID, channelID)
}

func (s *Store) LoadChannelConfig(teamID, channelID string) (ChannelConfig, error) {
	return s.LoadChannelConfigCtx(context.Background(), teamID, channelID)
}

func (s *Store) LoadWebhookConfigs(teamID string) ([]PushNotificationConfig, error) {
	return s.LoadWebhookConfigsCtx(context.Background(), teamID)
}

func (s *Store) SaveWebhookConfigs(teamID string, configs []PushNotificationConfig) error {
	return s.SaveWebhookConfigsCtx(context.Background(), teamID, configs)
}

func (s *Store) LoadPolicy(teamID string) (Policy, error) {
	return s.LoadPolicyCtx(context.Background(), teamID)
}

func (s *Store) SavePolicy(teamID string, policy Policy) error {
	return s.SavePolicyCtx(context.Background(), teamID, policy)
}

func (s *Store) AppendMessage(teamID string, msg Message) error {
	return s.AppendMessageCtx(context.Background(), teamID, msg)
}

func (s *Store) LoadMessages(teamID, channelID string, limit int) ([]Message, error) {
	return s.LoadMessagesCtx(context.Background(), teamID, channelID, limit)
}

func (s *Store) LoadChannel(teamID, channelID string) (Channel, error) {
	return s.LoadChannelCtx(context.Background(), teamID, channelID)
}

func (s *Store) SaveChannel(teamID string, channel Channel) error {
	return s.SaveChannelCtx(context.Background(), teamID, channel)
}

func (s *Store) HideChannel(teamID, channelID string) error {
	return s.HideChannelCtx(context.Background(), teamID, channelID)
}

func (s *Store) ListChannels(teamID string) ([]ChannelSummary, error) {
	return s.ListChannelsCtx(context.Background(), teamID)
}

func (s *Store) ListChannelConfigs(teamID string) ([]ChannelConfig, error) {
	return s.ListChannelConfigsCtx(context.Background(), teamID)
}

func (s *Store) AppendTask(teamID string, task Task) error {
	return s.AppendTaskCtx(context.Background(), teamID, task)
}

func (s *Store) LoadTasks(teamID string, limit int) ([]Task, error) {
	return s.LoadTasksCtx(context.Background(), teamID, limit)
}

func (s *Store) LoadTask(teamID, taskID string) (Task, error) {
	return s.LoadTaskCtx(context.Background(), teamID, taskID)
}

func (s *Store) SaveTask(teamID string, task Task) error {
	return s.SaveTaskCtx(context.Background(), teamID, task)
}

func (s *Store) SaveChannelConfig(teamID string, cfg ChannelConfig) error {
	return s.SaveChannelConfigCtx(context.Background(), teamID, cfg)
}

func (s *Store) DeleteTask(teamID, taskID string) error {
	return s.DeleteTaskCtx(context.Background(), teamID, taskID)
}

func (s *Store) AppendArtifact(teamID string, artifact Artifact) error {
	return s.AppendArtifactCtx(context.Background(), teamID, artifact)
}

func (s *Store) LoadArtifacts(teamID string, limit int) ([]Artifact, error) {
	return s.LoadArtifactsCtx(context.Background(), teamID, limit)
}

func (s *Store) LoadArtifact(teamID, artifactID string) (Artifact, error) {
	return s.LoadArtifactCtx(context.Background(), teamID, artifactID)
}

func (s *Store) AppendHistory(teamID string, event ChangeEvent) error {
	return s.AppendHistoryCtx(context.Background(), teamID, event)
}

func (s *Store) LoadHistory(teamID string, limit int) ([]ChangeEvent, error) {
	return s.LoadHistoryCtx(context.Background(), teamID, limit)
}

func (s *Store) SaveArtifact(teamID string, artifact Artifact) error {
	return s.SaveArtifactCtx(context.Background(), teamID, artifact)
}

func (s *Store) DeleteArtifact(teamID, artifactID string) error {
	return s.DeleteArtifactCtx(context.Background(), teamID, artifactID)
}

func (s *Store) LoadTaskMessages(teamID, taskID string, limit int) ([]Message, error) {
	return s.LoadTaskMessagesCtx(context.Background(), teamID, taskID, limit)
}

func (s *Store) LoadTaskDispatch(teamID, taskID string) (TaskDispatch, error) {
	return s.LoadTaskDispatchCtx(context.Background(), teamID, taskID)
}

func (s *Store) SaveTaskDispatch(teamID string, dispatch TaskDispatch) error {
	return s.SaveTaskDispatchCtx(context.Background(), teamID, dispatch)
}

func (s *Store) LoadTaskThread(teamID, taskID string, limit int) (TaskThread, error) {
	return s.LoadTaskThreadCtx(context.Background(), teamID, taskID, limit)
}

func (s *Store) LoadMessagesByContext(teamID, contextID string, limit int) ([]Message, error) {
	return s.LoadMessagesByContextCtx(context.Background(), teamID, contextID, limit)
}

func (s *Store) LoadTasksByContext(teamID, contextID string) ([]Task, error) {
	return s.LoadTasksByContextCtx(context.Background(), teamID, contextID)
}

func (s *Store) CreateManualArchive(teamID string, now time.Time) (*ArchiveSnapshot, error) {
	return s.CreateManualArchiveCtx(context.Background(), teamID, now)
}

func (s *Store) ListArchives(teamID string) ([]ArchiveSnapshot, error) {
	return s.ListArchivesCtx(context.Background(), teamID)
}

func (s *Store) LoadArchive(teamID, archiveID string) (ArchiveSnapshot, error) {
	return s.LoadArchiveCtx(context.Background(), teamID, archiveID)
}

func (s *Store) SaveAgentCard(teamID string, card AgentCard) error {
	return s.SaveAgentCardCtx(context.Background(), teamID, card)
}

func (s *Store) LoadAgentCard(teamID, agentID string) (AgentCard, error) {
	return s.LoadAgentCardCtx(context.Background(), teamID, agentID)
}

func (s *Store) ListAgentCards(teamID string) ([]AgentCard, error) {
	return s.ListAgentCardsCtx(context.Background(), teamID)
}
