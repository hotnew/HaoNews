package team

import (
	"errors"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"
)

const (
	TeamSyncTypeMessage       = "message"
	TeamSyncTypeHistory       = "history"
	TeamSyncTypeTask          = "task"
	TeamSyncTypeArtifact      = "artifact"
	TeamSyncTypeMember        = "member"
	TeamSyncTypePolicy        = "policy"
	TeamSyncTypeChannel       = "channel"
	TeamSyncTypeChannelConfig = "channel_config"
	TeamSyncTypeAck           = "ack"
)

func normalizeTeamSyncType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

type TeamSyncAck struct {
	AckedKey   string    `json:"acked_key"`
	AckedBy    string    `json:"acked_by,omitempty"`
	TargetNode string    `json:"target_node,omitempty"`
	AppliedAt  time.Time `json:"applied_at,omitempty"`
}

type TeamSyncConflict struct {
	Type          string    `json:"type"`
	TeamID        string    `json:"team_id"`
	SubjectID     string    `json:"subject_id,omitempty"`
	SourceNode    string    `json:"source_node,omitempty"`
	Reason        string    `json:"reason,omitempty"`
	LocalVersion  time.Time `json:"local_version,omitempty"`
	RemoteVersion time.Time `json:"remote_version,omitempty"`
	Resolution    string    `json:"resolution,omitempty"`
	ResolvedBy    string    `json:"resolved_by,omitempty"`
	ResolvedAt    time.Time `json:"resolved_at,omitempty"`
}

type TeamSyncMessage struct {
	Type          string         `json:"type"`
	TeamID        string         `json:"team_id"`
	Message       *Message       `json:"message,omitempty"`
	History       *ChangeEvent   `json:"history,omitempty"`
	Task          *Task          `json:"task,omitempty"`
	Artifact      *Artifact      `json:"artifact,omitempty"`
	Members       []Member       `json:"members,omitempty"`
	Policy        *Policy        `json:"policy,omitempty"`
	Channel       *Channel       `json:"channel,omitempty"`
	ChannelConfig *ChannelConfig `json:"channel_config,omitempty"`
	Ack           *TeamSyncAck   `json:"ack,omitempty"`
	SourceNode    string         `json:"source_node,omitempty"`
	CreatedAt     time.Time      `json:"created_at,omitempty"`
}

func (m TeamSyncMessage) Normalize() TeamSyncMessage {
	m.Type = normalizeTeamSyncType(m.Type)
	m.TeamID = NormalizeTeamID(m.TeamID)
	m.SourceNode = strings.TrimSpace(m.SourceNode)
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	if m.Message != nil {
		msg := *m.Message
		msg.TeamID = m.TeamID
		msg.ChannelID = normalizeChannelID(msg.ChannelID)
		msg.ContextID = normalizeContextID(msg.ContextID)
		if strings.TrimSpace(msg.MessageID) == "" {
			msg.MessageID = buildMessageID(msg)
		}
		m.Message = &msg
	}
	if m.History != nil {
		event := *m.History
		event.TeamID = m.TeamID
		event.Scope = strings.TrimSpace(event.Scope)
		event.Action = strings.TrimSpace(event.Action)
		event.Source = strings.TrimSpace(event.Source)
		event.Diff = normalizeFieldDiffs(event.Diff)
		if event.CreatedAt.IsZero() {
			event.CreatedAt = m.CreatedAt
		}
		if strings.TrimSpace(event.EventID) == "" {
			event.EventID = buildChangeEventID(event)
		}
		m.History = &event
	}
	if m.Task != nil {
		task := normalizeReplicatedTask(m.TeamID, *m.Task)
		m.Task = &task
	}
	if m.Artifact != nil {
		artifact := normalizeReplicatedArtifact(m.TeamID, *m.Artifact)
		m.Artifact = &artifact
	}
	if len(m.Members) > 0 {
		m.Members = normalizeReplicatedMembers(m.Members)
	}
	if m.Policy != nil {
		policy := normalizePolicy(*m.Policy)
		if policy.UpdatedAt.IsZero() {
			policy.UpdatedAt = m.CreatedAt
		}
		m.Policy = &policy
	}
	if m.Channel != nil {
		channel := normalizeChannel(*m.Channel)
		if channel.UpdatedAt.IsZero() {
			channel.UpdatedAt = m.CreatedAt
		}
		m.Channel = &channel
	}
	if m.ChannelConfig != nil {
		cfg := normalizeChannelConfig(*m.ChannelConfig)
		if cfg.UpdatedAt.IsZero() {
			cfg.UpdatedAt = m.CreatedAt
		}
		if cfg.CreatedAt.IsZero() {
			cfg.CreatedAt = cfg.UpdatedAt
		}
		m.ChannelConfig = &cfg
	}
	if m.Ack != nil {
		ack := *m.Ack
		ack.AckedKey = strings.TrimSpace(ack.AckedKey)
		ack.AckedBy = strings.TrimSpace(ack.AckedBy)
		ack.TargetNode = strings.TrimSpace(ack.TargetNode)
		if ack.AppliedAt.IsZero() {
			ack.AppliedAt = m.CreatedAt
		}
		m.Ack = &ack
	}
	return m
}

func (m TeamSyncMessage) Key() string {
	switch normalizeTeamSyncType(m.Type) {
	case TeamSyncTypeMessage:
		if m.Message != nil {
			return TeamSyncTypeMessage + ":" + strings.TrimSpace(m.Message.MessageID)
		}
	case TeamSyncTypeHistory:
		if m.History != nil {
			return TeamSyncTypeHistory + ":" + strings.TrimSpace(m.History.EventID)
		}
	case TeamSyncTypeTask:
		if m.Task != nil {
			return teamSyncTaskKey(*m.Task)
		}
	case TeamSyncTypeArtifact:
		if m.Artifact != nil {
			return teamSyncArtifactKey(*m.Artifact)
		}
	case TeamSyncTypeMember:
		return teamSyncMembersKey(m.TeamID, m.Members)
	case TeamSyncTypePolicy:
		if m.Policy != nil {
			return teamSyncPolicyKey(m.TeamID, *m.Policy)
		}
	case TeamSyncTypeChannel:
		if m.Channel != nil {
			return teamSyncChannelKey(m.TeamID, *m.Channel)
		}
	case TeamSyncTypeChannelConfig:
		if m.ChannelConfig != nil {
			return teamSyncChannelConfigKey(m.TeamID, *m.ChannelConfig)
		}
	case TeamSyncTypeAck:
		if m.Ack != nil && m.Ack.AckedKey != "" {
			key := TeamSyncTypeAck + ":" + m.Ack.AckedKey
			if m.Ack.AckedBy != "" {
				key += ":" + m.Ack.AckedBy
			}
			return key
		}
	}
	return ""
}

func (s *Store) ApplyReplicatedSync(sync TeamSyncMessage) (bool, error) {
	if s == nil {
		return false, errors.New("nil team store")
	}
	sync = sync.Normalize()
	if sync.TeamID == "" {
		return false, errors.New("empty team id")
	}
	switch sync.Type {
	case TeamSyncTypeMessage:
		if sync.Message == nil {
			return false, errors.New("missing team sync message payload")
		}
		return s.ApplyReplicatedMessage(sync.TeamID, *sync.Message)
	case TeamSyncTypeHistory:
		if sync.History == nil {
			return false, errors.New("missing team sync history payload")
		}
		return s.ApplyReplicatedHistory(sync.TeamID, *sync.History)
	case TeamSyncTypeTask:
		if sync.Task == nil {
			return false, errors.New("missing team sync task payload")
		}
		return s.ApplyReplicatedTask(sync.TeamID, *sync.Task)
	case TeamSyncTypeArtifact:
		if sync.Artifact == nil {
			return false, errors.New("missing team sync artifact payload")
		}
		return s.ApplyReplicatedArtifact(sync.TeamID, *sync.Artifact)
	case TeamSyncTypeMember:
		return s.ApplyReplicatedMembers(sync.TeamID, sync.Members, sync.CreatedAt)
	case TeamSyncTypePolicy:
		if sync.Policy == nil {
			return false, errors.New("missing team sync policy payload")
		}
		return s.ApplyReplicatedPolicy(sync.TeamID, *sync.Policy, sync.CreatedAt)
	case TeamSyncTypeChannel:
		if sync.Channel == nil {
			return false, errors.New("missing team sync channel payload")
		}
		return s.ApplyReplicatedChannel(sync.TeamID, *sync.Channel, sync.CreatedAt)
	case TeamSyncTypeChannelConfig:
		if sync.ChannelConfig == nil {
			return false, errors.New("missing team sync channel config payload")
		}
		return s.ApplyReplicatedChannelConfig(sync.TeamID, *sync.ChannelConfig, sync.CreatedAt)
	case TeamSyncTypeAck:
		return false, nil
	default:
		return false, errors.New("unsupported team sync type")
	}
}

func (s *Store) DetectReplicatedConflict(sync TeamSyncMessage) (TeamSyncConflict, bool, error) {
	if s == nil {
		return TeamSyncConflict{}, false, errors.New("nil team store")
	}
	sync = sync.Normalize()
	if sync.TeamID == "" {
		return TeamSyncConflict{}, false, errors.New("empty team id")
	}
	switch sync.Type {
	case TeamSyncTypeTask:
		if sync.Task == nil {
			return TeamSyncConflict{}, false, nil
		}
		current, err := s.loadTaskNoCtx(sync.TeamID, sync.Task.TaskID)
		switch {
		case errors.Is(err, os.ErrNotExist):
			return TeamSyncConflict{}, false, nil
		case err != nil:
			return TeamSyncConflict{}, false, err
		}
		remote := normalizeReplicatedTask(sync.TeamID, *sync.Task)
		localVersion := taskVersion(current)
		remoteVersion := taskVersion(remote)
		if !replicatedVersionAfter(remoteVersion, localVersion) && !reflect.DeepEqual(current, remote) {
			reason := "local_newer"
			if remoteVersion.Equal(localVersion) {
				reason = "same_version_diverged"
			}
			return TeamSyncConflict{
				Type:          sync.Type,
				TeamID:        sync.TeamID,
				SubjectID:     remote.TaskID,
				SourceNode:    sync.SourceNode,
				Reason:        reason,
				LocalVersion:  localVersion,
				RemoteVersion: remoteVersion,
			}, true, nil
		}
	case TeamSyncTypeArtifact:
		if sync.Artifact == nil {
			return TeamSyncConflict{}, false, nil
		}
		current, err := s.loadArtifactNoCtx(sync.TeamID, sync.Artifact.ArtifactID)
		switch {
		case errors.Is(err, os.ErrNotExist):
			return TeamSyncConflict{}, false, nil
		case err != nil:
			return TeamSyncConflict{}, false, err
		}
		remote := normalizeReplicatedArtifact(sync.TeamID, *sync.Artifact)
		localVersion := artifactVersion(current)
		remoteVersion := artifactVersion(remote)
		if !replicatedVersionAfter(remoteVersion, localVersion) && !reflect.DeepEqual(current, remote) {
			reason := "local_newer"
			if remoteVersion.Equal(localVersion) {
				reason = "same_version_diverged"
			}
			return TeamSyncConflict{
				Type:          sync.Type,
				TeamID:        sync.TeamID,
				SubjectID:     remote.ArtifactID,
				SourceNode:    sync.SourceNode,
				Reason:        reason,
				LocalVersion:  localVersion,
				RemoteVersion: remoteVersion,
			}, true, nil
		}
	case TeamSyncTypeMember:
		current, currentVersion, err := s.loadMembersSnapshotNoCtx(sync.TeamID)
		if err != nil {
			return TeamSyncConflict{}, false, err
		}
		remote := normalizeReplicatedMembers(sync.Members)
		remoteVersion := sync.CreatedAt.UTC()
		if remoteVersion.IsZero() {
			remoteVersion = membersSnapshotVersion(remote)
		}
		if !replicatedVersionAfter(remoteVersion, currentVersion) && !reflect.DeepEqual(current, remote) {
			reason := "local_newer"
			if remoteVersion.Equal(currentVersion) {
				reason = "same_version_diverged"
			}
			return TeamSyncConflict{
				Type:          sync.Type,
				TeamID:        sync.TeamID,
				SubjectID:     sync.TeamID,
				SourceNode:    sync.SourceNode,
				Reason:        reason,
				LocalVersion:  currentVersion,
				RemoteVersion: remoteVersion,
			}, true, nil
		}
	case TeamSyncTypePolicy:
		if sync.Policy == nil {
			return TeamSyncConflict{}, false, nil
		}
		current, currentVersion, err := s.loadPolicySnapshotNoCtx(sync.TeamID)
		if err != nil {
			return TeamSyncConflict{}, false, err
		}
		remote := normalizePolicy(*sync.Policy)
		remoteVersion := sync.CreatedAt.UTC()
		if remoteVersion.IsZero() {
			remoteVersion = policySnapshotVersion(remote)
		}
		if !replicatedVersionAfter(remoteVersion, currentVersion) && !reflect.DeepEqual(current, remote) {
			reason := "local_newer"
			if remoteVersion.Equal(currentVersion) {
				reason = "same_version_diverged"
			}
			return TeamSyncConflict{
				Type:          sync.Type,
				TeamID:        sync.TeamID,
				SubjectID:     sync.TeamID,
				SourceNode:    sync.SourceNode,
				Reason:        reason,
				LocalVersion:  currentVersion,
				RemoteVersion: remoteVersion,
			}, true, nil
		}
	case TeamSyncTypeChannel:
		if sync.Channel == nil {
			return TeamSyncConflict{}, false, nil
		}
		current, _, err := s.loadChannelSnapshotNoCtx(sync.TeamID, sync.Channel.ChannelID)
		switch {
		case errors.Is(err, os.ErrNotExist):
			return TeamSyncConflict{}, false, nil
		case err != nil:
			return TeamSyncConflict{}, false, err
		}
		remote := normalizeChannel(*sync.Channel)
		localVersion := channelSnapshotVersion(current)
		remoteVersion := sync.CreatedAt.UTC()
		if remoteVersion.IsZero() {
			remoteVersion = channelSnapshotVersion(remote)
		}
		if !replicatedVersionAfter(remoteVersion, localVersion) && !reflect.DeepEqual(current, remote) {
			reason := "local_newer"
			if remoteVersion.Equal(localVersion) {
				reason = "same_version_diverged"
			}
			return TeamSyncConflict{
				Type:          sync.Type,
				TeamID:        sync.TeamID,
				SubjectID:     remote.ChannelID,
				SourceNode:    sync.SourceNode,
				Reason:        reason,
				LocalVersion:  localVersion,
				RemoteVersion: remoteVersion,
			}, true, nil
		}
	case TeamSyncTypeChannelConfig:
		if sync.ChannelConfig == nil {
			return TeamSyncConflict{}, false, nil
		}
		current, err := s.loadChannelConfigNoCtx(sync.TeamID, sync.ChannelConfig.ChannelID)
		if err != nil {
			return TeamSyncConflict{}, false, err
		}
		remote := normalizeChannelConfig(*sync.ChannelConfig)
		localVersion := channelConfigSnapshotVersion(current)
		remoteVersion := sync.CreatedAt.UTC()
		if remoteVersion.IsZero() {
			remoteVersion = channelConfigSnapshotVersion(remote)
		}
		if !replicatedVersionAfter(remoteVersion, localVersion) && !reflect.DeepEqual(current, remote) {
			reason := "local_newer"
			if remoteVersion.Equal(localVersion) {
				reason = "same_version_diverged"
			}
			return TeamSyncConflict{
				Type:          sync.Type,
				TeamID:        sync.TeamID,
				SubjectID:     remote.ChannelID,
				SourceNode:    sync.SourceNode,
				Reason:        reason,
				LocalVersion:  localVersion,
				RemoteVersion: remoteVersion,
			}, true, nil
		}
	}
	return TeamSyncConflict{}, false, nil
}

func (s *Store) ForceApplyReplicatedSync(sync TeamSyncMessage) (bool, error) {
	if s == nil {
		return false, errors.New("nil team store")
	}
	sync = sync.Normalize()
	switch sync.Type {
	case TeamSyncTypeTask:
		if sync.Task == nil {
			return false, errors.New("missing task payload")
		}
		return true, s.upsertReplicatedTask(sync.TeamID, normalizeReplicatedTask(sync.TeamID, *sync.Task))
	case TeamSyncTypeArtifact:
		if sync.Artifact == nil {
			return false, errors.New("missing artifact payload")
		}
		return true, s.upsertReplicatedArtifact(sync.TeamID, normalizeReplicatedArtifact(sync.TeamID, *sync.Artifact))
	case TeamSyncTypeMember:
		if err := s.saveMembersNoCtx(sync.TeamID, normalizeReplicatedMembers(sync.Members)); err != nil {
			return false, err
		}
		return true, nil
	case TeamSyncTypePolicy:
		if sync.Policy == nil {
			return false, errors.New("missing policy payload")
		}
		if err := s.savePolicyNoCtx(sync.TeamID, normalizePolicy(*sync.Policy)); err != nil {
			return false, err
		}
		return true, nil
	case TeamSyncTypeChannel:
		if sync.Channel == nil {
			return false, errors.New("missing channel payload")
		}
		if err := s.saveChannelNoCtx(sync.TeamID, normalizeChannel(*sync.Channel)); err != nil {
			return false, err
		}
		return true, nil
	case TeamSyncTypeChannelConfig:
		if sync.ChannelConfig == nil {
			return false, errors.New("missing channel config payload")
		}
		if err := s.saveChannelConfigNoCtx(sync.TeamID, normalizeChannelConfig(*sync.ChannelConfig)); err != nil {
			return false, err
		}
		return true, nil
	default:
		return false, errors.New("conflict accept_remote unsupported for sync type")
	}
}

func (s *Store) ApplyReplicatedMessage(teamID string, msg Message) (bool, error) {
	if s == nil {
		return false, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return false, errors.New("empty team id")
	}
	msg.TeamID = teamID
	msg.ChannelID = normalizeChannelID(msg.ChannelID)
	msg.ContextID = normalizeContextID(msg.ContextID)
	if msg.ContextID == "" && len(msg.StructuredData) > 0 {
		msg.ContextID = structuredDataContextID(msg.StructuredData)
	}
	if strings.TrimSpace(msg.MessageID) == "" {
		msg.MessageID = buildMessageID(msg)
	}
	if !verifyMessageSignature(msg) {
		return false, errors.New("replicated team message signature verification failed")
	}
	exists, err := s.HasMessageID(teamID, msg.ChannelID, msg.MessageID)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	if err := s.appendMessageNoCtx(teamID, msg); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ApplyReplicatedHistory(teamID string, event ChangeEvent) (bool, error) {
	if s == nil {
		return false, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return false, errors.New("empty team id")
	}
	event.TeamID = teamID
	event.Scope = strings.TrimSpace(event.Scope)
	if event.Scope == "" {
		return false, errors.New("replicated team history requires scope")
	}
	if strings.TrimSpace(event.EventID) == "" {
		event.EventID = buildChangeEventID(event)
	}
	exists, err := s.HasHistoryEventID(teamID, event.EventID)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	if err := s.appendHistoryNoCtx(teamID, event); err != nil {
		return false, err
	}
	switch event.Scope + ":" + event.Action {
	case "task:delete":
		if err := s.deleteTaskNoCtx(teamID, event.SubjectID); err != nil && !errors.Is(err, os.ErrNotExist) {
			return true, err
		}
	case "artifact:delete":
		if err := s.deleteArtifactNoCtx(teamID, event.SubjectID); err != nil && !errors.Is(err, os.ErrNotExist) {
			return true, err
		}
	}
	return true, nil
}

func (s *Store) ApplyReplicatedTask(teamID string, task Task) (bool, error) {
	if s == nil {
		return false, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return false, errors.New("empty team id")
	}
	task = normalizeReplicatedTask(teamID, task)
	if strings.TrimSpace(task.TaskID) == "" {
		return false, errors.New("empty replicated task id")
	}
	current, err := s.loadTaskNoCtx(teamID, task.TaskID)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := s.upsertReplicatedTask(teamID, task); err != nil {
			return false, err
		}
		return true, nil
	case err != nil:
		return false, err
	}
	if !replicatedVersionAfter(taskVersion(task), taskVersion(current)) {
		return false, nil
	}
	if err := s.upsertReplicatedTask(teamID, task); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ApplyReplicatedArtifact(teamID string, artifact Artifact) (bool, error) {
	if s == nil {
		return false, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return false, errors.New("empty team id")
	}
	artifact = normalizeReplicatedArtifact(teamID, artifact)
	if strings.TrimSpace(artifact.ArtifactID) == "" {
		return false, errors.New("empty replicated artifact id")
	}
	current, err := s.loadArtifactNoCtx(teamID, artifact.ArtifactID)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := s.upsertReplicatedArtifact(teamID, artifact); err != nil {
			return false, err
		}
		return true, nil
	case err != nil:
		return false, err
	}
	if !replicatedVersionAfter(artifactVersion(artifact), artifactVersion(current)) {
		return false, nil
	}
	if err := s.upsertReplicatedArtifact(teamID, artifact); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ApplyReplicatedMembers(teamID string, members []Member, version time.Time) (bool, error) {
	if s == nil {
		return false, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return false, errors.New("empty team id")
	}
	members = normalizeReplicatedMembers(members)
	current, currentVersion, err := s.loadMembersSnapshotNoCtx(teamID)
	if err != nil {
		return false, err
	}
	if len(members) == 0 && len(current) == 0 {
		return false, nil
	}
	if version.IsZero() {
		version = membersSnapshotVersion(members)
	}
	if !replicatedVersionAfter(version, currentVersion) {
		return false, nil
	}
	if err := s.saveMembersNoCtx(teamID, members); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ApplyReplicatedPolicy(teamID string, policy Policy, version time.Time) (bool, error) {
	if s == nil {
		return false, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return false, errors.New("empty team id")
	}
	policy = normalizePolicy(policy)
	if version.IsZero() {
		version = policySnapshotVersion(policy)
	}
	current, currentVersion, err := s.loadPolicySnapshotNoCtx(teamID)
	if err != nil {
		return false, err
	}
	if !replicatedVersionAfter(version, currentVersion) {
		return false, nil
	}
	if reflect.DeepEqual(current, policy) {
		return false, nil
	}
	if err := s.savePolicyNoCtx(teamID, policy); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ApplyReplicatedChannel(teamID string, channel Channel, version time.Time) (bool, error) {
	if s == nil {
		return false, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return false, errors.New("empty team id")
	}
	channel = normalizeChannel(channel)
	if channel.ChannelID == "" {
		return false, errors.New("empty replicated channel id")
	}
	if version.IsZero() {
		version = channelSnapshotVersion(channel)
	}
	current, _, err := s.loadChannelSnapshotNoCtx(teamID, channel.ChannelID)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := s.saveChannelNoCtx(teamID, channel); err != nil {
			return false, err
		}
		return true, nil
	case err != nil:
		return false, err
	}
	if !replicatedVersionAfter(version, channelSnapshotVersion(current)) {
		return false, nil
	}
	if reflect.DeepEqual(current, channel) {
		return false, nil
	}
	if err := s.saveChannelNoCtx(teamID, channel); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ApplyReplicatedChannelConfig(teamID string, cfg ChannelConfig, version time.Time) (bool, error) {
	if s == nil {
		return false, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return false, errors.New("empty team id")
	}
	cfg = normalizeChannelConfig(cfg)
	if strings.TrimSpace(cfg.ChannelID) == "" {
		return false, errors.New("empty replicated channel config channel_id")
	}
	current, err := s.loadChannelConfigNoCtx(teamID, cfg.ChannelID)
	if err != nil {
		return false, err
	}
	if version.IsZero() {
		version = channelConfigSnapshotVersion(cfg)
	}
	if !replicatedVersionAfter(version, channelConfigSnapshotVersion(current)) && !reflect.DeepEqual(current, cfg) {
		return false, nil
	}
	cfg.ChannelID = normalizeChannelID(cfg.ChannelID)
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = version
	}
	cfg.UpdatedAt = version
	if err := s.saveChannelConfigNoCtx(teamID, cfg); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) HasMessageID(teamID, channelID, messageID string) (bool, error) {
	if s == nil {
		return false, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	channelID = normalizeChannelID(channelID)
	messageID = strings.TrimSpace(messageID)
	if teamID == "" || channelID == "" || messageID == "" {
		return false, nil
	}
	items, err := s.loadMessagesNoCtx(teamID, channelID, 0)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if strings.TrimSpace(item.MessageID) == messageID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) HasHistoryEventID(teamID, eventID string) (bool, error) {
	if s == nil {
		return false, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	eventID = strings.TrimSpace(eventID)
	if teamID == "" || eventID == "" {
		return false, nil
	}
	items, err := s.loadHistoryNoCtx(teamID, 0)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if strings.TrimSpace(item.EventID) == eventID {
			return true, nil
		}
	}
	return false, nil
}

func normalizeReplicatedTask(teamID string, task Task) Task {
	task.TeamID = NormalizeTeamID(teamID)
	task.TaskID = strings.TrimSpace(task.TaskID)
	task.ChannelID = normalizeChannelID(task.ChannelID)
	task.ContextID = normalizeContextID(task.ContextID)
	task.Title = strings.TrimSpace(task.Title)
	task.Description = strings.TrimSpace(task.Description)
	task.CreatedBy = strings.TrimSpace(task.CreatedBy)
	task.Status = normalizeTaskStatus(task.Status)
	if task.Status == "" {
		task.Status = "open"
	}
	task.Priority = normalizeTaskPriority(task.Priority)
	if !task.DueAt.IsZero() {
		task.DueAt = task.DueAt.UTC()
	}
	task.Assignees = normalizeNonEmptyStrings(task.Assignees)
	task.Labels = normalizeNonEmptyStrings(task.Labels)
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now().UTC()
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = task.CreatedAt
	}
	if task.ContextID == "" {
		task.ContextID = generateContextID(task.TeamID)
	}
	if task.TaskID == "" {
		task.TaskID = buildTaskID(task)
	}
	if IsTerminalState(task.Status) && task.ClosedAt.IsZero() {
		task.ClosedAt = task.UpdatedAt
	}
	if !IsTerminalState(task.Status) {
		task.ClosedAt = time.Time{}
	}
	return task
}

func normalizeReplicatedMembers(members []Member) []Member {
	if len(members) == 0 {
		return nil
	}
	out := make([]Member, 0, len(members))
	seen := make(map[string]struct{}, len(members))
	for _, member := range members {
		member.AgentID = strings.TrimSpace(member.AgentID)
		if member.AgentID == "" {
			continue
		}
		if _, ok := seen[member.AgentID]; ok {
			continue
		}
		seen[member.AgentID] = struct{}{}
		member.Role = normalizeMemberRole(member.Role)
		member.Status = normalizeMemberStatus(member.Status)
		if member.JoinedAt.IsZero() {
			member.JoinedAt = time.Now().UTC()
		}
		if member.UpdatedAt.IsZero() {
			member.UpdatedAt = member.JoinedAt
		}
		out = append(out, member)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Role != out[j].Role {
			return out[i].Role < out[j].Role
		}
		return out[i].AgentID < out[j].AgentID
	})
	return out
}

func normalizeReplicatedArtifact(teamID string, artifact Artifact) Artifact {
	artifact.TeamID = NormalizeTeamID(teamID)
	artifact.ArtifactID = strings.TrimSpace(artifact.ArtifactID)
	artifact.ChannelID = normalizeChannelID(artifact.ChannelID)
	artifact.TaskID = strings.TrimSpace(artifact.TaskID)
	artifact.Title = strings.TrimSpace(artifact.Title)
	artifact.Kind = normalizeArtifactKind(artifact.Kind)
	artifact.Summary = strings.TrimSpace(artifact.Summary)
	artifact.Content = strings.TrimSpace(artifact.Content)
	artifact.LinkURL = strings.TrimSpace(artifact.LinkURL)
	artifact.CreatedBy = strings.TrimSpace(artifact.CreatedBy)
	artifact.Labels = normalizeNonEmptyStrings(artifact.Labels)
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = time.Now().UTC()
	}
	if artifact.UpdatedAt.IsZero() {
		artifact.UpdatedAt = artifact.CreatedAt
	}
	if artifact.ArtifactID == "" {
		artifact.ArtifactID = buildArtifactID(artifact)
	}
	return artifact
}

func taskVersion(task Task) time.Time {
	if !task.UpdatedAt.IsZero() {
		return task.UpdatedAt.UTC()
	}
	return task.CreatedAt.UTC()
}

func membersSnapshotVersion(members []Member) time.Time {
	var latest time.Time
	for _, member := range members {
		if !member.UpdatedAt.IsZero() && member.UpdatedAt.After(latest) {
			latest = member.UpdatedAt.UTC()
		}
		if member.UpdatedAt.IsZero() && !member.JoinedAt.IsZero() && member.JoinedAt.After(latest) {
			latest = member.JoinedAt.UTC()
		}
	}
	return latest
}

func policySnapshotVersion(policy Policy) time.Time {
	return policy.UpdatedAt.UTC()
}

func channelSnapshotVersion(channel Channel) time.Time {
	if !channel.UpdatedAt.IsZero() {
		return channel.UpdatedAt.UTC()
	}
	return channel.CreatedAt.UTC()
}

func channelConfigSnapshotVersion(cfg ChannelConfig) time.Time {
	if !cfg.UpdatedAt.IsZero() {
		return cfg.UpdatedAt.UTC()
	}
	return cfg.CreatedAt.UTC()
}

func artifactVersion(artifact Artifact) time.Time {
	if !artifact.UpdatedAt.IsZero() {
		return artifact.UpdatedAt.UTC()
	}
	return artifact.CreatedAt.UTC()
}

func replicatedVersionAfter(next, current time.Time) bool {
	if current.IsZero() {
		return !next.IsZero()
	}
	if next.IsZero() {
		return false
	}
	return next.After(current)
}

func teamSyncTaskKey(task Task) string {
	taskID := strings.TrimSpace(task.TaskID)
	if taskID == "" {
		return ""
	}
	return "task:" + taskID + ":" + taskVersion(task).Format(time.RFC3339Nano)
}

func teamSyncArtifactKey(artifact Artifact) string {
	artifactID := strings.TrimSpace(artifact.ArtifactID)
	if artifactID == "" {
		return ""
	}
	return "artifact:" + artifactID + ":" + artifactVersion(artifact).Format(time.RFC3339Nano)
}

func teamSyncMembersKey(teamID string, members []Member) string {
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return ""
	}
	version := membersSnapshotVersion(members)
	if version.IsZero() {
		return ""
	}
	return "member:" + teamID + ":" + version.Format(time.RFC3339Nano)
}

func teamSyncPolicyKey(teamID string, policy Policy) string {
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return ""
	}
	version := policySnapshotVersion(policy)
	if version.IsZero() {
		return ""
	}
	return "policy:" + teamID + ":" + version.Format(time.RFC3339Nano)
}

func teamSyncChannelKey(teamID string, channel Channel) string {
	teamID = NormalizeTeamID(teamID)
	channel.ChannelID = normalizeChannelID(channel.ChannelID)
	if teamID == "" || channel.ChannelID == "" {
		return ""
	}
	version := channelSnapshotVersion(channel)
	if version.IsZero() {
		return ""
	}
	return "channel:" + channel.ChannelID + ":" + version.Format(time.RFC3339Nano)
}

func teamSyncChannelConfigKey(teamID string, cfg ChannelConfig) string {
	teamID = NormalizeTeamID(teamID)
	cfg = normalizeChannelConfig(cfg)
	if teamID == "" || cfg.ChannelID == "" {
		return ""
	}
	version := channelConfigSnapshotVersion(cfg)
	if version.IsZero() {
		return ""
	}
	return "channel_config:" + cfg.ChannelID + ":" + version.Format(time.RFC3339Nano)
}

func (s *Store) upsertReplicatedTask(teamID string, task Task) error {
	return s.withTeamLock(teamID, func() error {
		return s.upsertReplicatedTaskCurrentLocked(teamID, task)
	})
}

func (s *Store) upsertReplicatedArtifact(teamID string, artifact Artifact) error {
	return s.withTeamLock(teamID, func() error {
		return s.upsertReplicatedArtifactCurrentLocked(teamID, artifact)
	})
}
