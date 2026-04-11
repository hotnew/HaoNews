package haonews

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	teamcore "hao.news/internal/haonews/team"
)

const (
	teamSyncRecentScanLimit     = 500
	teamSyncAckRetryAfter       = 15 * time.Second
	teamSyncPeerAckTTL          = 24 * time.Hour
	teamSyncPeerAckPerPeer      = 128
	teamSyncPendingMaxRetry     = 8
	teamSyncPendingMaxAge       = 6 * time.Hour
	teamSyncResolvedTTL         = 2 * time.Hour
	teamSyncConflictResolvedTTL = 24 * time.Hour
)

type teamSyncTransport interface {
	PublishTeamSync(context.Context, teamcore.TeamSyncMessage) error
	SubscribeTeamSync(context.Context, string, func(teamcore.TeamSyncMessage) (bool, error)) error
}

type teamPubSubRuntime struct {
	store     *teamcore.Store
	transport teamSyncTransport
	nodeID    string
	startedAt time.Time
	statePath string

	mu              sync.Mutex
	primedChannels  map[string]struct{}
	primedHistory   map[string]struct{}
	primedTasks     map[string]struct{}
	primedArtifacts map[string]struct{}
	primedMembers   map[string]struct{}
	primedPolicies  map[string]struct{}
	primedConfig    map[string]struct{}
	subscribed      map[string]struct{}
	seen            map[string]time.Time
	state           teamSyncPersistedState
	status          SyncTeamSyncStatus
}

type teamSyncCheckpoint struct {
	VersionAt time.Time `json:"version_at,omitempty"`
	Key       string    `json:"key,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type teamSyncPeerAck struct {
	AckedKey  string    `json:"acked_key,omitempty"`
	AckedBy   string    `json:"acked_by,omitempty"`
	AppliedAt time.Time `json:"applied_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type teamSyncConflict struct {
	Key            string                   `json:"key,omitempty"`
	Type           string                   `json:"type,omitempty"`
	TeamID         string                   `json:"team_id,omitempty"`
	SubjectID      string                   `json:"subject_id,omitempty"`
	SourceNode     string                   `json:"source_node,omitempty"`
	Reason         string                   `json:"reason,omitempty"`
	AutoResolvable bool                     `json:"auto_resolvable,omitempty"`
	LocalVersion   time.Time                `json:"local_version,omitempty"`
	RemoteVersion  time.Time                `json:"remote_version,omitempty"`
	Resolution     string                   `json:"resolution,omitempty"`
	ResolvedBy     string                   `json:"resolved_by,omitempty"`
	ResolvedAt     time.Time                `json:"resolved_at,omitempty"`
	Sync           teamcore.TeamSyncMessage `json:"sync,omitempty"`
	UpdatedAt      time.Time                `json:"updated_at,omitempty"`
}

type teamSyncPendingState struct {
	VersionAt  time.Time `json:"version_at,omitempty"`
	Key        string    `json:"key,omitempty"`
	StateKey   string    `json:"state_key,omitempty"`
	Status     string    `json:"status,omitempty"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
	RetryCount int       `json:"retry_count,omitempty"`
}

type teamSyncPersistedState struct {
	Messages  map[string]teamSyncCheckpoint         `json:"messages,omitempty"`
	History   map[string]teamSyncCheckpoint         `json:"history,omitempty"`
	Tasks     map[string]teamSyncCheckpoint         `json:"tasks,omitempty"`
	Artifacts map[string]teamSyncCheckpoint         `json:"artifacts,omitempty"`
	Members   map[string]teamSyncCheckpoint         `json:"members,omitempty"`
	Policies  map[string]teamSyncCheckpoint         `json:"policies,omitempty"`
	Channels  map[string]teamSyncCheckpoint         `json:"channels,omitempty"`
	Applied   map[string]teamSyncCheckpoint         `json:"applied,omitempty"`
	Acks      map[string]teamSyncCheckpoint         `json:"acks,omitempty"`
	Pending   map[string]teamSyncPendingState       `json:"pending,omitempty"`
	PeerAcks  map[string]map[string]teamSyncPeerAck `json:"peer_acks,omitempty"`
	Conflicts map[string]teamSyncConflict           `json:"conflicts,omitempty"`
}

func startTeamPubSubRuntime(storeRoot string, transport teamSyncTransport, nodeID string) (*teamPubSubRuntime, error) {
	if transport == nil {
		return nil, nil
	}
	store, err := teamcore.OpenStore(storeRoot)
	if err != nil {
		return nil, err
	}
	statePath := filepath.Join(storeRoot, "sync", "team_sync_state.json")
	state, err := loadTeamSyncState(statePath)
	if err != nil {
		return nil, err
	}
	return &teamPubSubRuntime{
		store:           store,
		transport:       transport,
		nodeID:          strings.TrimSpace(nodeID),
		startedAt:       time.Now().UTC(),
		statePath:       statePath,
		primedChannels:  make(map[string]struct{}),
		primedHistory:   make(map[string]struct{}),
		primedTasks:     make(map[string]struct{}),
		primedArtifacts: make(map[string]struct{}),
		primedMembers:   make(map[string]struct{}),
		primedPolicies:  make(map[string]struct{}),
		primedConfig:    make(map[string]struct{}),
		subscribed:      make(map[string]struct{}),
		seen:            make(map[string]time.Time),
		state:           state,
		status: SyncTeamSyncStatus{
			Enabled:           true,
			NodeID:            strings.TrimSpace(nodeID),
			StatePath:         statePath,
			PersistedCursors:  countPersistedStateEntries(state),
			PersistedPeerAcks: countPersistedPeerAckEntries(state),
			AckPeers:          len(state.PeerAcks),
			Conflicts:         len(state.Conflicts),
			ResolvedConflicts: countResolvedConflicts(state),
			PendingAcks:       countPendingStatus(state, "pending"),
			ExpiredPending:    countPendingStatus(state, "expired"),
			SupersededPending: countPendingStatus(state, "superseded"),
			StateLoaded:       true,
		},
	}, nil
}

func (r *teamPubSubRuntime) SyncOnce(ctx context.Context, logf func(string, ...any)) error {
	if r == nil || r.store == nil || r.transport == nil {
		return nil
	}
	teams, err := r.store.ListTeamsCtx(ctx)
	if err != nil {
		return err
	}
	for _, summary := range teams {
		teamID := teamcore.NormalizeTeamID(summary.TeamID)
		if teamID == "" {
			continue
		}
		if err := r.ensureSubscription(ctx, teamID); err != nil {
			if logf != nil {
				logf("team sync subscribe %s: %v", teamID, err)
			}
			continue
		}
		if err := r.syncTeamMessages(ctx, teamID, logf); err != nil && logf != nil {
			logf("team sync messages %s: %v", teamID, err)
		}
		if err := r.syncTeamMembers(ctx, teamID, logf); err != nil && logf != nil {
			logf("team sync members %s: %v", teamID, err)
		}
		if err := r.syncTeamPolicy(ctx, teamID, logf); err != nil && logf != nil {
			logf("team sync policy %s: %v", teamID, err)
		}
		if err := r.syncTeamChannels(ctx, teamID, logf); err != nil && logf != nil {
			logf("team sync channels %s: %v", teamID, err)
		}
		if err := r.syncTeamChannelConfigs(ctx, teamID, logf); err != nil && logf != nil {
			logf("team sync channel configs %s: %v", teamID, err)
		}
		if err := r.syncTeamTasks(ctx, teamID, logf); err != nil && logf != nil {
			logf("team sync tasks %s: %v", teamID, err)
		}
		if err := r.syncTeamArtifacts(ctx, teamID, logf); err != nil && logf != nil {
			logf("team sync artifacts %s: %v", teamID, err)
		}
		if err := r.syncTeamHistory(ctx, teamID, logf); err != nil && logf != nil {
			logf("team sync history %s: %v", teamID, err)
		}
	}
	r.pruneSeen(30 * time.Minute)
	return nil
}

func (r *teamPubSubRuntime) ensureSubscription(ctx context.Context, teamID string) error {
	r.mu.Lock()
	if _, ok := r.subscribed[teamID]; ok {
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()
	if err := r.transport.SubscribeTeamSync(ctx, teamID, func(syncMsg teamcore.TeamSyncMessage) (bool, error) {
		r.recordReceived(syncMsg)
		if syncMsg.Type == teamcore.TeamSyncTypeAck {
			handled, err := r.handleAck(syncMsg)
			if handled {
				r.recordApplied(syncMsg)
			} else if err == nil {
				r.recordSkipped(syncMsg)
			} else {
				r.recordError(syncMsg.TeamID, err)
			}
			return handled, err
		}
		applied, err := r.store.ApplyReplicatedSync(syncMsg)
		if applied {
			r.rememberSeen(syncMsg.Key())
			r.recordApplied(syncMsg)
			_ = r.persistApplied(syncMsg)
			_ = r.publishAck(ctx, syncMsg)
		} else if err == nil {
			r.recordConflictIfNeeded(syncMsg)
			r.recordSkipped(syncMsg)
		} else {
			r.recordConflictFromError(syncMsg, err)
			r.recordError(syncMsg.TeamID, err)
		}
		return applied, err
	}); err != nil {
		r.recordError(teamID, err)
		return err
	}
	r.mu.Lock()
	r.subscribed[teamID] = struct{}{}
	now := time.Now().UTC()
	r.status.SubscribedTeams = len(r.subscribed)
	r.status.LastSubscriptionTeam = teamID
	r.status.LastSubscriptionAt = &now
	r.mu.Unlock()
	return nil
}

func (r *teamPubSubRuntime) syncTeamMessages(ctx context.Context, teamID string, logf func(string, ...any)) error {
	channels, err := r.store.ListChannelsCtx(ctx, teamID)
	if err != nil {
		return err
	}
	for _, channel := range channels {
		channelID := strings.TrimSpace(channel.ChannelID)
		if channelID == "" {
			continue
		}
		items, err := r.store.LoadMessagesCtx(ctx, teamID, channelID, teamSyncRecentScanLimit)
		if err != nil {
			return err
		}
		key := teamID + ":" + channelID
		firstScan := !r.isPrimedChannel(key)
		for i := len(items) - 1; i >= 0; i-- {
			msg := items[i]
			if strings.TrimSpace(msg.MessageID) == "" {
				msg.MessageID = buildMessageIDForSync(msg)
			}
			r.recordScannedMessage(teamID, channelID, msg.MessageID)
			syncKey := teamSyncMessageKey(msg.MessageID)
			retryPending := r.shouldRetryPending(syncKey)
			if firstScan && !retryPending && !r.shouldPublishSnapshot(teamSyncStateMessageKey(teamID, channelID), msg.CreatedAt) {
				r.rememberSeen(syncKey)
				continue
			}
			if !retryPending && r.seenKey(syncKey) {
				continue
			}
			syncMsg := teamcore.TeamSyncMessage{
				Type:       teamcore.TeamSyncTypeMessage,
				TeamID:     teamID,
				Message:    &msg,
				SourceNode: r.nodeID,
				CreatedAt:  time.Now().UTC(),
			}.Normalize()
			if err := r.transport.PublishTeamSync(ctx, syncMsg); err != nil {
				r.recordError(teamID, err)
				if logf != nil {
					logf("team sync publish message %s/%s: %v", teamID, channelID, err)
				}
				continue
			}
			r.rememberSeen(syncKey)
			if retryPending {
				r.recordRetried(syncKey)
			}
			r.recordPublished(syncMsg)
			_ = r.persistPublished(syncMsg, msg.CreatedAt)
		}
		if firstScan {
			r.markPrimedChannel(key)
		}
	}
	return nil
}

func (r *teamPubSubRuntime) syncTeamHistory(ctx context.Context, teamID string, logf func(string, ...any)) error {
	items, err := r.store.LoadHistoryCtx(ctx, teamID, teamSyncRecentScanLimit)
	if err != nil {
		return err
	}
	firstScan := !r.isPrimedHistory(teamID)
	for i := len(items) - 1; i >= 0; i-- {
		event := items[i]
		if strings.TrimSpace(event.Scope) == "" {
			continue
		}
		if strings.TrimSpace(event.EventID) == "" {
			continue
		}
		r.recordScannedHistory(teamID, event.EventID)
		syncKey := teamSyncHistoryKey(event.EventID)
		retryPending := r.shouldRetryPending(syncKey)
		if firstScan && !retryPending && !r.shouldPublishSnapshot(teamSyncStateHistoryKey(teamID), event.CreatedAt) {
			r.rememberSeen(syncKey)
			continue
		}
		if !retryPending && r.seenKey(syncKey) {
			continue
		}
		syncMsg := teamcore.TeamSyncMessage{
			Type:       teamcore.TeamSyncTypeHistory,
			TeamID:     teamID,
			History:    &event,
			SourceNode: r.nodeID,
			CreatedAt:  time.Now().UTC(),
		}.Normalize()
		if err := r.transport.PublishTeamSync(ctx, syncMsg); err != nil {
			r.recordError(teamID, err)
			if logf != nil {
				logf("team sync publish history %s: %v", teamID, err)
			}
			continue
		}
		r.rememberSeen(syncKey)
		if retryPending {
			r.recordRetried(syncKey)
		}
		r.recordPublished(syncMsg)
		_ = r.persistPublished(syncMsg, event.CreatedAt)
	}
	if firstScan {
		r.markPrimedHistory(teamID)
	}
	return nil
}

func (r *teamPubSubRuntime) syncTeamTasks(ctx context.Context, teamID string, logf func(string, ...any)) error {
	items, err := r.store.LoadTasksCtx(ctx, teamID, teamSyncRecentScanLimit)
	if err != nil {
		return err
	}
	firstScan := !r.isPrimedTasks(teamID)
	for i := len(items) - 1; i >= 0; i-- {
		task := items[i]
		r.recordScannedTask(teamID, task.TaskID)
		version := taskSyncVersion(task)
		syncMsg := teamcore.TeamSyncMessage{
			Type:       teamcore.TeamSyncTypeTask,
			TeamID:     teamID,
			Task:       &task,
			SourceNode: r.nodeID,
			CreatedAt:  time.Now().UTC(),
		}.Normalize()
		syncKey := syncMsg.Key()
		retryPending := r.shouldRetryPending(syncKey)
		if firstScan && !retryPending && !r.shouldPublishSnapshot(teamSyncStateTaskKey(teamID, task.TaskID), version) {
			r.rememberSeen(syncKey)
			continue
		}
		if !retryPending && r.seenKey(syncKey) {
			continue
		}
		if err := r.transport.PublishTeamSync(ctx, syncMsg); err != nil {
			r.recordError(teamID, err)
			if logf != nil {
				logf("team sync publish task %s/%s: %v", teamID, task.TaskID, err)
			}
			continue
		}
		r.rememberSeen(syncKey)
		if retryPending {
			r.recordRetried(syncKey)
		}
		r.recordPublished(syncMsg)
		_ = r.persistPublished(syncMsg, version)
	}
	if firstScan {
		r.markPrimedTasks(teamID)
	}
	return nil
}

func (r *teamPubSubRuntime) syncTeamMembers(ctx context.Context, teamID string, logf func(string, ...any)) error {
	members, version, err := r.store.LoadMembersSnapshotCtx(ctx, teamID)
	if err != nil {
		return err
	}
	firstScan := !r.isPrimedMembers(teamID)
	r.recordScannedMember(teamID)
	if version.IsZero() {
		if firstScan {
			r.markPrimedMembers(teamID)
		}
		return nil
	}
	syncMsg := teamcore.TeamSyncMessage{
		Type:       teamcore.TeamSyncTypeMember,
		TeamID:     teamID,
		Members:    append([]teamcore.Member(nil), members...),
		SourceNode: r.nodeID,
		CreatedAt:  version,
	}.Normalize()
	syncKey := syncMsg.Key()
	if syncKey != "" {
		retryPending := r.shouldRetryPending(syncKey)
		if firstScan && !retryPending && !r.shouldPublishSnapshot(teamSyncStateMembersKey(teamID), version) {
			r.rememberSeen(syncKey)
		} else if retryPending || !r.seenKey(syncKey) {
			if err := r.transport.PublishTeamSync(ctx, syncMsg); err != nil {
				r.recordError(teamID, err)
				if logf != nil {
					logf("team sync publish members %s: %v", teamID, err)
				}
			} else {
				r.rememberSeen(syncKey)
				if retryPending {
					r.recordRetried(syncKey)
				}
				r.recordPublished(syncMsg)
				_ = r.persistPublished(syncMsg, version)
			}
		}
	}
	if firstScan {
		r.markPrimedMembers(teamID)
	}
	return nil
}

func (r *teamPubSubRuntime) syncTeamPolicy(ctx context.Context, teamID string, logf func(string, ...any)) error {
	policy, version, err := r.store.LoadPolicySnapshotCtx(ctx, teamID)
	if err != nil {
		return err
	}
	firstScan := !r.isPrimedPolicy(teamID)
	r.recordScannedPolicy(teamID)
	if version.IsZero() {
		if firstScan {
			r.markPrimedPolicy(teamID)
		}
		return nil
	}
	syncMsg := teamcore.TeamSyncMessage{
		Type:       teamcore.TeamSyncTypePolicy,
		TeamID:     teamID,
		Policy:     &policy,
		SourceNode: r.nodeID,
		CreatedAt:  version,
	}.Normalize()
	syncKey := syncMsg.Key()
	if syncKey != "" {
		retryPending := r.shouldRetryPending(syncKey)
		if firstScan && !retryPending && !r.shouldPublishSnapshot(teamSyncStatePolicyKey(teamID), version) {
			r.rememberSeen(syncKey)
		} else if retryPending || !r.seenKey(syncKey) {
			if err := r.transport.PublishTeamSync(ctx, syncMsg); err != nil {
				r.recordError(teamID, err)
				if logf != nil {
					logf("team sync publish policy %s: %v", teamID, err)
				}
			} else {
				r.rememberSeen(syncKey)
				if retryPending {
					r.recordRetried(syncKey)
				}
				r.recordPublished(syncMsg)
				_ = r.persistPublished(syncMsg, version)
			}
		}
	}
	if firstScan {
		r.markPrimedPolicy(teamID)
	}
	return nil
}

func (r *teamPubSubRuntime) syncTeamChannels(ctx context.Context, teamID string, logf func(string, ...any)) error {
	items, err := r.store.ListChannelsCtx(ctx, teamID)
	if err != nil {
		return err
	}
	for _, summary := range items {
		channel := summary.Channel
		r.recordScannedChannel(teamID, channel.ChannelID)
		version := channelSyncVersion(channel)
		if version.IsZero() {
			if !r.isPrimedConfigChannel(teamID, channel.ChannelID) {
				r.markPrimedConfigChannel(teamID, channel.ChannelID)
			}
			continue
		}
		syncMsg := teamcore.TeamSyncMessage{
			Type:       teamcore.TeamSyncTypeChannel,
			TeamID:     teamID,
			Channel:    &channel,
			SourceNode: r.nodeID,
			CreatedAt:  version,
		}.Normalize()
		syncKey := syncMsg.Key()
		if syncKey == "" {
			continue
		}
		firstScan := !r.isPrimedConfigChannel(teamID, channel.ChannelID)
		retryPending := r.shouldRetryPending(syncKey)
		if firstScan && !retryPending && !r.shouldPublishSnapshot(teamSyncStateChannelKey(teamID, channel.ChannelID), version) {
			r.rememberSeen(syncKey)
			r.markPrimedConfigChannel(teamID, channel.ChannelID)
			continue
		}
		if !retryPending && r.seenKey(syncKey) {
			if firstScan {
				r.markPrimedConfigChannel(teamID, channel.ChannelID)
			}
			continue
		}
		if err := r.transport.PublishTeamSync(ctx, syncMsg); err != nil {
			r.recordError(teamID, err)
			if logf != nil {
				logf("team sync publish channel %s/%s: %v", teamID, channel.ChannelID, err)
			}
			continue
		}
		r.rememberSeen(syncKey)
		if retryPending {
			r.recordRetried(syncKey)
		}
		r.recordPublished(syncMsg)
		_ = r.persistPublished(syncMsg, version)
		if firstScan {
			r.markPrimedConfigChannel(teamID, channel.ChannelID)
		}
	}
	return nil
}

func (r *teamPubSubRuntime) syncTeamChannelConfigs(ctx context.Context, teamID string, logf func(string, ...any)) error {
	items, err := r.store.ListChannelConfigsCtx(ctx, teamID)
	if err != nil {
		return err
	}
	for _, cfg := range items {
		channelID := strings.TrimSpace(cfg.ChannelID)
		if channelID == "" {
			continue
		}
		r.recordScannedChannel(teamID, channelID)
		version := cfg.UpdatedAt.UTC()
		if version.IsZero() {
			version = cfg.CreatedAt.UTC()
		}
		primedKey := channelID + ":config"
		if version.IsZero() {
			if !r.isPrimedConfigChannel(teamID, primedKey) {
				r.markPrimedConfigChannel(teamID, primedKey)
			}
			continue
		}
		syncMsg := teamcore.TeamSyncMessage{
			Type:          teamcore.TeamSyncTypeChannelConfig,
			TeamID:        teamID,
			ChannelConfig: &cfg,
			SourceNode:    r.nodeID,
			CreatedAt:     version,
		}.Normalize()
		syncKey := syncMsg.Key()
		if syncKey == "" {
			continue
		}
		firstScan := !r.isPrimedConfigChannel(teamID, primedKey)
		retryPending := r.shouldRetryPending(syncKey)
		if firstScan && !retryPending && !r.shouldPublishSnapshot(teamSyncStateChannelConfigKey(teamID, channelID), version) {
			r.rememberSeen(syncKey)
			r.markPrimedConfigChannel(teamID, primedKey)
			continue
		}
		if !retryPending && r.seenKey(syncKey) {
			if firstScan {
				r.markPrimedConfigChannel(teamID, primedKey)
			}
			continue
		}
		if err := r.transport.PublishTeamSync(ctx, syncMsg); err != nil {
			r.recordError(teamID, err)
			if logf != nil {
				logf("team sync publish channel config %s/%s: %v", teamID, channelID, err)
			}
			continue
		}
		r.rememberSeen(syncKey)
		if retryPending {
			r.recordRetried(syncKey)
		}
		r.recordPublished(syncMsg)
		_ = r.persistPublished(syncMsg, version)
		if firstScan {
			r.markPrimedConfigChannel(teamID, primedKey)
		}
	}
	return nil
}

func (r *teamPubSubRuntime) syncTeamArtifacts(ctx context.Context, teamID string, logf func(string, ...any)) error {
	items, err := r.store.LoadArtifactsCtx(ctx, teamID, teamSyncRecentScanLimit)
	if err != nil {
		return err
	}
	firstScan := !r.isPrimedArtifacts(teamID)
	for i := len(items) - 1; i >= 0; i-- {
		artifact := items[i]
		r.recordScannedArtifact(teamID, artifact.ArtifactID)
		version := artifactSyncVersion(artifact)
		syncMsg := teamcore.TeamSyncMessage{
			Type:       teamcore.TeamSyncTypeArtifact,
			TeamID:     teamID,
			Artifact:   &artifact,
			SourceNode: r.nodeID,
			CreatedAt:  time.Now().UTC(),
		}.Normalize()
		syncKey := syncMsg.Key()
		retryPending := r.shouldRetryPending(syncKey)
		if firstScan && !retryPending && !r.shouldPublishSnapshot(teamSyncStateArtifactKey(teamID, artifact.ArtifactID), version) {
			r.rememberSeen(syncKey)
			continue
		}
		if !retryPending && r.seenKey(syncKey) {
			continue
		}
		if err := r.transport.PublishTeamSync(ctx, syncMsg); err != nil {
			r.recordError(teamID, err)
			if logf != nil {
				logf("team sync publish artifact %s/%s: %v", teamID, artifact.ArtifactID, err)
			}
			continue
		}
		r.rememberSeen(syncKey)
		if retryPending {
			r.recordRetried(syncKey)
		}
		r.recordPublished(syncMsg)
		_ = r.persistPublished(syncMsg, version)
	}
	if firstScan {
		r.markPrimedArtifacts(teamID)
	}
	return nil
}

func (r *teamPubSubRuntime) isPrimedChannel(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.primedChannels[key]
	return ok
}

func (r *teamPubSubRuntime) markPrimedChannel(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.primedChannels[key] = struct{}{}
	r.status.PrimedChannels = len(r.primedChannels)
}

func (r *teamPubSubRuntime) isPrimedHistory(teamID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.primedHistory[teamID]
	return ok
}

func (r *teamPubSubRuntime) isPrimedTasks(teamID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.primedTasks[teamID]
	return ok
}

func (r *teamPubSubRuntime) isPrimedMembers(teamID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.primedMembers[teamID]
	return ok
}

func (r *teamPubSubRuntime) markPrimedMembers(teamID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.primedMembers[teamID] = struct{}{}
	r.status.PrimedMemberTeams = len(r.primedMembers)
}

func (r *teamPubSubRuntime) isPrimedPolicy(teamID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.primedPolicies[teamID]
	return ok
}

func (r *teamPubSubRuntime) markPrimedPolicy(teamID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.primedPolicies[teamID] = struct{}{}
	r.status.PrimedPolicyTeams = len(r.primedPolicies)
}

func (r *teamPubSubRuntime) isPrimedConfigChannel(teamID, channelID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.primedConfig[teamID+":"+strings.TrimSpace(channelID)]
	return ok
}

func (r *teamPubSubRuntime) markPrimedConfigChannel(teamID, channelID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.primedConfig[teamID+":"+strings.TrimSpace(channelID)] = struct{}{}
	r.status.PrimedConfigChannels = len(r.primedConfig)
}

func (r *teamPubSubRuntime) markPrimedTasks(teamID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.primedTasks[teamID] = struct{}{}
	r.status.PrimedTaskTeams = len(r.primedTasks)
}

func (r *teamPubSubRuntime) isPrimedArtifacts(teamID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.primedArtifacts[teamID]
	return ok
}

func (r *teamPubSubRuntime) markPrimedArtifacts(teamID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.primedArtifacts[teamID] = struct{}{}
	r.status.PrimedArtifactTeams = len(r.primedArtifacts)
}

func (r *teamPubSubRuntime) markPrimedHistory(teamID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.primedHistory[teamID] = struct{}{}
	r.status.PrimedHistoryTeams = len(r.primedHistory)
}

func (r *teamPubSubRuntime) seenKey(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.seen[key]
	return ok
}

func (r *teamPubSubRuntime) rememberSeen(key string) {
	if strings.TrimSpace(key) == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen[key] = time.Now().UTC()
}

func (r *teamPubSubRuntime) pruneSeen(maxAge time.Duration) {
	if maxAge <= 0 {
		return
	}
	cutoff := time.Now().UTC().Add(-maxAge)
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, ts := range r.seen {
		if ts.Before(cutoff) {
			delete(r.seen, key)
		}
	}
}

func (r *teamPubSubRuntime) shouldPublishSinceStart(at time.Time) bool {
	if r == nil {
		return false
	}
	if at.IsZero() {
		return false
	}
	return at.UTC().After(r.startedAt)
}

func (r *teamPubSubRuntime) shouldPublishSnapshot(stateKey string, version time.Time) bool {
	if version.IsZero() {
		return false
	}
	if checkpoint, ok := r.publishedCheckpoint(stateKey); ok {
		return teamSyncVersionAfter(version, checkpoint.VersionAt)
	}
	return r.shouldPublishSinceStart(version)
}

func (r *teamPubSubRuntime) handleAck(syncMsg teamcore.TeamSyncMessage) (bool, error) {
	if r == nil {
		return false, nil
	}
	syncMsg = syncMsg.Normalize()
	if syncMsg.Ack == nil || strings.TrimSpace(syncMsg.Ack.AckedKey) == "" {
		return false, nil
	}
	if target := strings.TrimSpace(syncMsg.Ack.TargetNode); target != "" && target != r.nodeID {
		return false, nil
	}
	if strings.TrimSpace(syncMsg.Ack.AckedBy) == "" || syncMsg.Ack.AckedBy == r.nodeID {
		return false, nil
	}
	syncKey := syncMsg.Key()
	if syncKey == "" {
		return false, nil
	}
	if r.seenKey(syncKey) {
		return false, nil
	}
	if checkpoint, ok := r.publishedCheckpoint(teamSyncStateKey(syncMsg)); ok && !teamSyncVersionAfter(teamSyncMessageVersion(syncMsg), checkpoint.VersionAt) {
		r.rememberSeen(syncKey)
		return false, nil
	}
	r.rememberSeen(syncKey)
	return true, r.persistAck(syncMsg)
}

func (r *teamPubSubRuntime) shouldRetryPending(syncKey string) bool {
	if r == nil || strings.TrimSpace(syncKey) == "" {
		return false
	}
	syncKey = strings.TrimSpace(syncKey)
	r.mu.Lock()
	state := normalizeTeamSyncState(r.state)
	item, ok := state.Pending[syncKey]
	r.mu.Unlock()
	if !ok {
		return false
	}
	if strings.TrimSpace(item.Status) == "" {
		item.Status = "pending"
	}
	if item.Status != "pending" {
		return false
	}
	now := time.Now().UTC()
	if !item.UpdatedAt.IsZero() && item.UpdatedAt.Before(now.Add(-teamSyncPendingMaxAge)) {
		_ = r.markPendingStatus(syncKey, "expired")
		return false
	}
	if item.RetryCount >= teamSyncPendingMaxRetry {
		_ = r.markPendingStatus(syncKey, "expired")
		return false
	}
	if item.UpdatedAt.IsZero() {
		return true
	}
	return now.After(item.UpdatedAt.UTC().Add(teamSyncAckRetryAfter))
}

func (r *teamPubSubRuntime) publishAck(ctx context.Context, appliedSync teamcore.TeamSyncMessage) error {
	if r == nil || r.transport == nil {
		return nil
	}
	appliedSync = appliedSync.Normalize()
	if appliedSync.Type == teamcore.TeamSyncTypeAck {
		return nil
	}
	ackedKey := strings.TrimSpace(appliedSync.Key())
	targetNode := strings.TrimSpace(appliedSync.SourceNode)
	if ackedKey == "" || targetNode == "" {
		return nil
	}
	ack := teamcore.TeamSyncMessage{
		Type:   teamcore.TeamSyncTypeAck,
		TeamID: appliedSync.TeamID,
		Ack: &teamcore.TeamSyncAck{
			AckedKey:   ackedKey,
			AckedBy:    r.nodeID,
			TargetNode: targetNode,
			AppliedAt:  time.Now().UTC(),
		},
		SourceNode: r.nodeID,
		CreatedAt:  time.Now().UTC(),
	}.Normalize()
	if err := r.transport.PublishTeamSync(ctx, ack); err != nil {
		r.recordError(appliedSync.TeamID, err)
		return err
	}
	r.rememberSeen(ack.Key())
	r.recordPublished(ack)
	return nil
}

func (r *teamPubSubRuntime) Status() SyncTeamSyncStatus {
	if r == nil {
		return SyncTeamSyncStatus{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

func (r *teamPubSubRuntime) recordRetried(syncKey string) {
	if r == nil || strings.TrimSpace(syncKey) == "" {
		return
	}
	_ = r.bumpPendingRetry(strings.TrimSpace(syncKey))
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.RetriedPublishes++
	r.status.LastRetriedKey = strings.TrimSpace(syncKey)
}

func (r *teamPubSubRuntime) recordPublished(syncMsg teamcore.TeamSyncMessage) {
	r.recordTransition(syncMsg, "published")
}

func (r *teamPubSubRuntime) recordReceived(syncMsg teamcore.TeamSyncMessage) {
	r.recordTransition(syncMsg, "received")
}

func (r *teamPubSubRuntime) recordApplied(syncMsg teamcore.TeamSyncMessage) {
	r.recordTransition(syncMsg, "applied")
}

func (r *teamPubSubRuntime) recordSkipped(syncMsg teamcore.TeamSyncMessage) {
	r.recordTransition(syncMsg, "skipped")
}

func (r *teamPubSubRuntime) recordConflictIfNeeded(syncMsg teamcore.TeamSyncMessage) {
	if r == nil || r.store == nil {
		return
	}
	conflict, ok, err := r.store.DetectReplicatedConflict(syncMsg)
	if err != nil {
		r.recordError(syncMsg.TeamID, err)
		return
	}
	if !ok {
		return
	}
	_ = r.persistConflict(syncMsg, conflict)
}

func (r *teamPubSubRuntime) recordConflictFromError(syncMsg teamcore.TeamSyncMessage, err error) {
	if r == nil || err == nil {
		return
	}
	reason := classifyTeamSyncError(err)
	if reason == "" {
		return
	}
	_ = r.persistConflict(syncMsg.Normalize(), teamcore.TeamSyncConflict{
		Type:          syncMsg.Type,
		TeamID:        syncMsg.TeamID,
		SubjectID:     teamSyncSubjectID(syncMsg),
		SourceNode:    syncMsg.SourceNode,
		Reason:        reason,
		LocalVersion:  time.Time{},
		RemoteVersion: teamSyncMessageVersion(syncMsg),
	})
}

func classifyTeamSyncError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "signature verification failed"):
		return "signature_rejected"
	case strings.Contains(message, "policy denied"):
		return "policy_rejected"
	default:
		return ""
	}
}

func teamSyncSubjectID(syncMsg teamcore.TeamSyncMessage) string {
	syncMsg = syncMsg.Normalize()
	switch syncMsg.Type {
	case teamcore.TeamSyncTypeMessage:
		if syncMsg.Message != nil {
			return strings.TrimSpace(syncMsg.Message.MessageID)
		}
	case teamcore.TeamSyncTypeHistory:
		if syncMsg.History != nil {
			return strings.TrimSpace(syncMsg.History.EventID)
		}
	case teamcore.TeamSyncTypeTask:
		if syncMsg.Task != nil {
			return strings.TrimSpace(syncMsg.Task.TaskID)
		}
	case teamcore.TeamSyncTypeArtifact:
		if syncMsg.Artifact != nil {
			return strings.TrimSpace(syncMsg.Artifact.ArtifactID)
		}
	case teamcore.TeamSyncTypeMember, teamcore.TeamSyncTypePolicy:
		return syncMsg.TeamID
	case teamcore.TeamSyncTypeChannel:
		if syncMsg.Channel != nil {
			return strings.TrimSpace(syncMsg.Channel.ChannelID)
		}
	case teamcore.TeamSyncTypeChannelConfig:
		if syncMsg.ChannelConfig != nil {
			return strings.TrimSpace(syncMsg.ChannelConfig.ChannelID)
		}
	}
	return ""
}

func (r *teamPubSubRuntime) recordTransition(syncMsg teamcore.TeamSyncMessage, stage string) {
	if r == nil {
		return
	}
	syncMsg = syncMsg.Normalize()
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.LastTeamID = syncMsg.TeamID
	switch stage {
	case "published":
		r.status.LastPublishedKey = syncMsg.Key()
		r.status.LastPublishedAt = &now
		switch syncMsg.Type {
		case teamcore.TeamSyncTypeMessage:
			r.status.PublishedMessages++
		case teamcore.TeamSyncTypeHistory:
			r.status.PublishedHistory++
		case teamcore.TeamSyncTypeTask:
			r.status.PublishedTasks++
		case teamcore.TeamSyncTypeArtifact:
			r.status.PublishedArtifacts++
		case teamcore.TeamSyncTypeMember:
			r.status.PublishedMembers++
		case teamcore.TeamSyncTypePolicy:
			r.status.PublishedPolicies++
		case teamcore.TeamSyncTypeChannel:
			r.status.PublishedConfigChannels++
		case teamcore.TeamSyncTypeChannelConfig:
			r.status.PublishedConfigChannels++
		case teamcore.TeamSyncTypeAck:
			r.status.PublishedAcks++
		}
	case "received":
		r.status.LastReceivedKey = syncMsg.Key()
		r.status.LastReceivedAt = &now
		switch syncMsg.Type {
		case teamcore.TeamSyncTypeMessage:
			r.status.ReceivedMessages++
		case teamcore.TeamSyncTypeHistory:
			r.status.ReceivedHistory++
		case teamcore.TeamSyncTypeTask:
			r.status.ReceivedTasks++
		case teamcore.TeamSyncTypeArtifact:
			r.status.ReceivedArtifacts++
		case teamcore.TeamSyncTypeMember:
			r.status.ReceivedMembers++
		case teamcore.TeamSyncTypePolicy:
			r.status.ReceivedPolicies++
		case teamcore.TeamSyncTypeChannel:
			r.status.ReceivedConfigChannels++
		case teamcore.TeamSyncTypeChannelConfig:
			r.status.ReceivedConfigChannels++
		case teamcore.TeamSyncTypeAck:
			r.status.ReceivedAcks++
		}
	case "applied":
		r.status.LastAppliedKey = syncMsg.Key()
		r.status.LastAppliedAt = &now
		if syncMsg.Type == teamcore.TeamSyncTypeAck && syncMsg.Ack != nil {
			r.status.LastAckedKey = syncMsg.Ack.AckedKey
		}
		switch syncMsg.Type {
		case teamcore.TeamSyncTypeMessage:
			r.status.AppliedMessages++
		case teamcore.TeamSyncTypeHistory:
			r.status.AppliedHistory++
		case teamcore.TeamSyncTypeTask:
			r.status.AppliedTasks++
		case teamcore.TeamSyncTypeArtifact:
			r.status.AppliedArtifacts++
		case teamcore.TeamSyncTypeMember:
			r.status.AppliedMembers++
		case teamcore.TeamSyncTypePolicy:
			r.status.AppliedPolicies++
		case teamcore.TeamSyncTypeChannel:
			r.status.AppliedConfigChannels++
		case teamcore.TeamSyncTypeChannelConfig:
			r.status.AppliedConfigChannels++
		case teamcore.TeamSyncTypeAck:
			r.status.AppliedAcks++
		}
	case "skipped":
		switch syncMsg.Type {
		case teamcore.TeamSyncTypeMessage:
			r.status.SkippedMessages++
		case teamcore.TeamSyncTypeHistory:
			r.status.SkippedHistory++
		case teamcore.TeamSyncTypeTask:
			r.status.SkippedTasks++
		case teamcore.TeamSyncTypeArtifact:
			r.status.SkippedArtifacts++
		case teamcore.TeamSyncTypeMember:
			r.status.SkippedMembers++
		case teamcore.TeamSyncTypePolicy:
			r.status.SkippedPolicies++
		case teamcore.TeamSyncTypeChannel:
			r.status.SkippedConfigChannels++
		case teamcore.TeamSyncTypeChannelConfig:
			r.status.SkippedConfigChannels++
		case teamcore.TeamSyncTypeAck:
			r.status.SkippedAcks++
		}
	}
}

func (r *teamPubSubRuntime) recordError(teamID string, err error) {
	if r == nil || err == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.LastTeamID = teamcore.NormalizeTeamID(teamID)
	r.status.LastError = err.Error()
}

func (r *teamPubSubRuntime) recordScannedMessage(teamID, channelID, messageID string) {
	if r == nil {
		return
	}
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.LastTeamID = teamcore.NormalizeTeamID(teamID)
	r.status.LastScannedChannelID = strings.TrimSpace(channelID)
	r.status.LastScannedMessageID = strings.TrimSpace(messageID)
	r.status.LastScannedAt = &now
	r.status.ScannedMessages++
}

func (r *teamPubSubRuntime) recordScannedHistory(teamID, eventID string) {
	if r == nil {
		return
	}
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.LastTeamID = teamcore.NormalizeTeamID(teamID)
	r.status.LastScannedEventID = strings.TrimSpace(eventID)
	r.status.LastScannedAt = &now
	r.status.ScannedHistory++
}

func (r *teamPubSubRuntime) recordScannedTask(teamID, taskID string) {
	if r == nil {
		return
	}
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.LastTeamID = teamcore.NormalizeTeamID(teamID)
	r.status.LastScannedTaskID = strings.TrimSpace(taskID)
	r.status.LastScannedAt = &now
	r.status.ScannedTasks++
}

func (r *teamPubSubRuntime) recordScannedArtifact(teamID, artifactID string) {
	if r == nil {
		return
	}
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.LastTeamID = teamcore.NormalizeTeamID(teamID)
	r.status.LastScannedArtifactID = strings.TrimSpace(artifactID)
	r.status.LastScannedAt = &now
	r.status.ScannedArtifacts++
}

func (r *teamPubSubRuntime) recordScannedMember(teamID string) {
	if r == nil {
		return
	}
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.LastTeamID = teamcore.NormalizeTeamID(teamID)
	r.status.LastScannedAt = &now
	r.status.ScannedMembers++
}

func (r *teamPubSubRuntime) recordScannedPolicy(teamID string) {
	if r == nil {
		return
	}
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.LastTeamID = teamcore.NormalizeTeamID(teamID)
	r.status.LastScannedAt = &now
	r.status.ScannedPolicies++
}

func (r *teamPubSubRuntime) recordScannedChannel(teamID, channelID string) {
	if r == nil {
		return
	}
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.LastTeamID = teamcore.NormalizeTeamID(teamID)
	r.status.LastScannedChannelID = strings.TrimSpace(channelID)
	r.status.LastScannedAt = &now
	r.status.ScannedConfigChannels++
}

func teamSyncMessageKey(messageID string) string {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return ""
	}
	return "message:" + messageID
}

func teamSyncHistoryKey(eventID string) string {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return ""
	}
	return "history:" + eventID
}

func buildMessageIDForSync(msg teamcore.Message) string {
	return strings.Join([]string{
		strings.TrimSpace(msg.TeamID),
		strings.TrimSpace(msg.ChannelID),
		strings.TrimSpace(msg.AuthorAgentID),
		msg.CreatedAt.UTC().Format(time.RFC3339Nano),
		strings.TrimSpace(msg.Content),
	}, ":")
}

func taskSyncVersion(task teamcore.Task) time.Time {
	if !task.UpdatedAt.IsZero() {
		return task.UpdatedAt.UTC()
	}
	return task.CreatedAt.UTC()
}

func artifactSyncVersion(artifact teamcore.Artifact) time.Time {
	if !artifact.UpdatedAt.IsZero() {
		return artifact.UpdatedAt.UTC()
	}
	return artifact.CreatedAt.UTC()
}

func loadTeamSyncState(path string) (teamSyncPersistedState, error) {
	state := newTeamSyncPersistedState()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	return normalizeTeamSyncState(state), nil
}

func newTeamSyncPersistedState() teamSyncPersistedState {
	return teamSyncPersistedState{
		Messages:  make(map[string]teamSyncCheckpoint),
		History:   make(map[string]teamSyncCheckpoint),
		Tasks:     make(map[string]teamSyncCheckpoint),
		Artifacts: make(map[string]teamSyncCheckpoint),
		Members:   make(map[string]teamSyncCheckpoint),
		Policies:  make(map[string]teamSyncCheckpoint),
		Channels:  make(map[string]teamSyncCheckpoint),
		Applied:   make(map[string]teamSyncCheckpoint),
		Acks:      make(map[string]teamSyncCheckpoint),
		Pending:   make(map[string]teamSyncPendingState),
		PeerAcks:  make(map[string]map[string]teamSyncPeerAck),
		Conflicts: make(map[string]teamSyncConflict),
	}
}

func normalizeTeamSyncState(state teamSyncPersistedState) teamSyncPersistedState {
	if state.Messages == nil || state.History == nil || state.Tasks == nil || state.Artifacts == nil || state.Members == nil || state.Policies == nil || state.Channels == nil || state.Applied == nil || state.Acks == nil || state.Pending == nil || state.PeerAcks == nil || state.Conflicts == nil {
		normalized := newTeamSyncPersistedState()
		copyTeamSyncStateMap(normalized.Messages, state.Messages)
		copyTeamSyncStateMap(normalized.History, state.History)
		copyTeamSyncStateMap(normalized.Tasks, state.Tasks)
		copyTeamSyncStateMap(normalized.Artifacts, state.Artifacts)
		copyTeamSyncStateMap(normalized.Members, state.Members)
		copyTeamSyncStateMap(normalized.Policies, state.Policies)
		copyTeamSyncStateMap(normalized.Channels, state.Channels)
		copyTeamSyncStateMap(normalized.Applied, state.Applied)
		copyTeamSyncStateMap(normalized.Acks, state.Acks)
		copyTeamSyncPendingStateMap(normalized.Pending, state.Pending)
		copyTeamSyncPeerAckStateMap(normalized.PeerAcks, state.PeerAcks)
		copyTeamSyncConflictStateMap(normalized.Conflicts, state.Conflicts)
		return normalized
	}
	return state
}

func copyTeamSyncStateMap(dst, src map[string]teamSyncCheckpoint) {
	for key, value := range src {
		dst[key] = value
	}
}

func copyTeamSyncPendingStateMap(dst, src map[string]teamSyncPendingState) {
	for key, value := range src {
		dst[key] = value
	}
}

func copyTeamSyncPeerAckStateMap(dst, src map[string]map[string]teamSyncPeerAck) {
	for peerID, entries := range src {
		if _, ok := dst[peerID]; !ok {
			dst[peerID] = make(map[string]teamSyncPeerAck)
		}
		for ackedKey, value := range entries {
			dst[peerID][ackedKey] = value
		}
	}
}

func copyTeamSyncConflictStateMap(dst, src map[string]teamSyncConflict) {
	for key, value := range src {
		dst[key] = value
	}
}

func mergeTeamSyncPeerAckState(dst, src map[string]map[string]teamSyncPeerAck) {
	for peerID, entries := range src {
		if _, ok := dst[peerID]; !ok {
			dst[peerID] = make(map[string]teamSyncPeerAck)
		}
		for ackedKey, incoming := range entries {
			current, ok := dst[peerID][ackedKey]
			if !ok || incoming.UpdatedAt.After(current.UpdatedAt) {
				dst[peerID][ackedKey] = incoming
			}
		}
	}
}

func mergeTeamSyncConflictState(dst, src map[string]teamSyncConflict) {
	for key, incoming := range src {
		current, ok := dst[key]
		if !ok {
			continue
		}
		if incoming.UpdatedAt.After(current.UpdatedAt) {
			if strings.TrimSpace(incoming.Resolution) != "" && strings.TrimSpace(current.Resolution) == "" {
				incoming.UpdatedAt = time.Now().UTC()
			}
			dst[key] = incoming
			continue
		}
		if strings.TrimSpace(current.Resolution) == "" && strings.TrimSpace(incoming.Resolution) != "" {
			incoming.UpdatedAt = time.Now().UTC()
			dst[key] = incoming
		}
	}
}

func mergeTeamSyncState(existing, incoming teamSyncPersistedState) teamSyncPersistedState {
	merged := normalizeTeamSyncState(incoming)
	existing = normalizeTeamSyncState(existing)
	mergeTeamSyncPeerAckState(merged.PeerAcks, existing.PeerAcks)
	mergeTeamSyncConflictState(merged.Conflicts, existing.Conflicts)
	return merged
}

func countPersistedStateEntries(state teamSyncPersistedState) int {
	return len(state.Messages) + len(state.History) + len(state.Tasks) + len(state.Artifacts) + len(state.Members) + len(state.Policies) + len(state.Channels) + len(state.Applied) + len(state.Acks) + len(state.Pending)
}

func countPersistedPeerAckEntries(state teamSyncPersistedState) int {
	total := 0
	for _, entries := range state.PeerAcks {
		total += len(entries)
	}
	return total
}

func countPendingStatus(state teamSyncPersistedState, status string) int {
	status = strings.TrimSpace(status)
	total := 0
	for _, item := range state.Pending {
		if strings.TrimSpace(item.Status) == status {
			total++
		}
	}
	return total
}

func countResolvedConflicts(state teamSyncPersistedState) int {
	total := 0
	for _, item := range state.Conflicts {
		if strings.TrimSpace(item.Resolution) != "" {
			total++
		}
	}
	return total
}

func applyTeamSyncCompactionResult(status *SyncTeamSyncStatus, now time.Time, peerPruned int, prunedPeer, prunedKey string, conflictPruned int, conflictPrunedKey string) {
	if status == nil {
		return
	}
	if peerPruned > 0 {
		status.PeerAckPrunes += peerPruned
		if strings.TrimSpace(prunedPeer) != "" {
			status.LastPrunedAckPeer = strings.TrimSpace(prunedPeer)
		}
		if strings.TrimSpace(prunedKey) != "" {
			status.LastPrunedAckKey = strings.TrimSpace(prunedKey)
		}
	}
	if conflictPruned > 0 {
		status.ConflictPrunes += conflictPruned
		if strings.TrimSpace(conflictPrunedKey) != "" {
			status.LastPrunedConflictKey = strings.TrimSpace(conflictPrunedKey)
		}
		status.LastPrunedConflictAt = &now
	}
}

func (r *teamPubSubRuntime) publishedCheckpoint(stateKey string) (teamSyncCheckpoint, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := teamSyncStateBucket(r.state, stateKey)[stateKey]
	return value, ok
}

func (r *teamPubSubRuntime) persistPublished(syncMsg teamcore.TeamSyncMessage, version time.Time) error {
	if err := r.persistCheckpoint(syncMsg, version, false); err != nil {
		return err
	}
	if syncMsg.Normalize().Type == teamcore.TeamSyncTypeAck {
		return nil
	}
	return r.persistPending(syncMsg, version)
}

func (r *teamPubSubRuntime) persistApplied(syncMsg teamcore.TeamSyncMessage) error {
	version := teamSyncMessageVersion(syncMsg)
	return r.persistCheckpoint(syncMsg, version, true)
}

func (r *teamPubSubRuntime) persistAck(syncMsg teamcore.TeamSyncMessage) error {
	version := teamSyncMessageVersion(syncMsg)
	if err := r.persistCheckpoint(syncMsg, version, false); err != nil {
		return err
	}
	if syncMsg.Ack != nil {
		if err := r.persistPeerAck(syncMsg); err != nil {
			return err
		}
		return r.clearPending(syncMsg.Ack.AckedKey)
	}
	return nil
}

func (r *teamPubSubRuntime) persistCheckpoint(syncMsg teamcore.TeamSyncMessage, version time.Time, applied bool) error {
	if r == nil || strings.TrimSpace(r.statePath) == "" {
		return nil
	}
	stateKey := teamSyncStateKey(syncMsg)
	if stateKey == "" || version.IsZero() {
		return nil
	}
	r.mu.Lock()
	state := normalizeTeamSyncState(r.state)
	checkpoint := teamSyncCheckpoint{VersionAt: version.UTC(), Key: syncMsg.Key()}
	if applied {
		state.Applied[stateKey] = checkpoint
	} else {
		teamSyncStateBucket(state, stateKey)[stateKey] = checkpoint
	}
	peerPruned, prunedPeer, prunedKey, conflictPruned, conflictPrunedKey := compactTeamSyncState(&state)
	r.state = state
	now := time.Now().UTC()
	updateTeamSyncStatusFromState(&r.status, state)
	applyTeamSyncCompactionResult(&r.status, now, peerPruned, prunedPeer, prunedKey, conflictPruned, conflictPrunedKey)
	r.status.LastStateWriteAt = &now
	r.mu.Unlock()
	return writeTeamSyncState(r.statePath, state)
}

func (r *teamPubSubRuntime) persistPending(syncMsg teamcore.TeamSyncMessage, version time.Time) error {
	if r == nil || strings.TrimSpace(r.statePath) == "" {
		return nil
	}
	syncKey := strings.TrimSpace(syncMsg.Key())
	if syncKey == "" || version.IsZero() {
		return nil
	}
	r.mu.Lock()
	state := normalizeTeamSyncState(r.state)
	now := time.Now().UTC()
	stateKey := teamSyncStateKey(syncMsg)
	for key, item := range state.Pending {
		if key == syncKey {
			continue
		}
		if strings.TrimSpace(item.StateKey) == stateKey && strings.TrimSpace(item.Status) == "pending" {
			item.Status = "superseded"
			item.UpdatedAt = now
			state.Pending[key] = item
		}
	}
	item := state.Pending[syncKey]
	item.VersionAt = version.UTC()
	item.Key = syncKey
	item.StateKey = stateKey
	item.Status = "pending"
	item.UpdatedAt = now
	if item.RetryCount < 0 {
		item.RetryCount = 0
	}
	state.Pending[syncKey] = item
	peerPruned, prunedPeer, prunedKey, conflictPruned, conflictPrunedKey := compactTeamSyncState(&state)
	r.state = state
	updateTeamSyncStatusFromState(&r.status, state)
	applyTeamSyncCompactionResult(&r.status, now, peerPruned, prunedPeer, prunedKey, conflictPruned, conflictPrunedKey)
	r.status.LastStateWriteAt = &now
	r.mu.Unlock()
	return writeTeamSyncState(r.statePath, state)
}

func (r *teamPubSubRuntime) persistPeerAck(syncMsg teamcore.TeamSyncMessage) error {
	if r == nil || strings.TrimSpace(r.statePath) == "" {
		return nil
	}
	syncMsg = syncMsg.Normalize()
	if syncMsg.Ack == nil || strings.TrimSpace(syncMsg.Ack.AckedBy) == "" || strings.TrimSpace(syncMsg.Ack.AckedKey) == "" {
		return nil
	}
	r.mu.Lock()
	state := normalizeTeamSyncState(r.state)
	peerID := strings.TrimSpace(syncMsg.Ack.AckedBy)
	if _, ok := state.PeerAcks[peerID]; !ok {
		state.PeerAcks[peerID] = make(map[string]teamSyncPeerAck)
	}
	now := time.Now().UTC()
	state.PeerAcks[peerID][strings.TrimSpace(syncMsg.Ack.AckedKey)] = teamSyncPeerAck{
		AckedKey:  strings.TrimSpace(syncMsg.Ack.AckedKey),
		AckedBy:   peerID,
		AppliedAt: syncMsg.Ack.AppliedAt.UTC(),
		UpdatedAt: now,
	}
	pruned, prunedPeer, prunedKey, conflictPruned, conflictPrunedKey := compactTeamSyncState(&state)
	r.state = state
	updateTeamSyncStatusFromState(&r.status, state)
	applyTeamSyncCompactionResult(&r.status, now, pruned, prunedPeer, prunedKey, conflictPruned, conflictPrunedKey)
	r.status.LastStateWriteAt = &now
	r.mu.Unlock()
	return writeTeamSyncState(r.statePath, state)
}

func (r *teamPubSubRuntime) clearPending(syncKey string) error {
	if r == nil || strings.TrimSpace(r.statePath) == "" || strings.TrimSpace(syncKey) == "" {
		return nil
	}
	r.mu.Lock()
	state := normalizeTeamSyncState(r.state)
	item, ok := state.Pending[strings.TrimSpace(syncKey)]
	if !ok {
		r.mu.Unlock()
		return nil
	}
	item.Status = "acked"
	item.UpdatedAt = time.Now().UTC()
	state.Pending[strings.TrimSpace(syncKey)] = item
	peerPruned, prunedPeer, prunedKey, conflictPruned, conflictPrunedKey := compactTeamSyncState(&state)
	r.state = state
	now := time.Now().UTC()
	updateTeamSyncStatusFromState(&r.status, state)
	applyTeamSyncCompactionResult(&r.status, now, peerPruned, prunedPeer, prunedKey, conflictPruned, conflictPrunedKey)
	r.status.LastStateWriteAt = &now
	r.mu.Unlock()
	return writeTeamSyncState(r.statePath, state)
}

func (r *teamPubSubRuntime) persistConflict(syncMsg teamcore.TeamSyncMessage, conflict teamcore.TeamSyncConflict) error {
	if r == nil || strings.TrimSpace(r.statePath) == "" {
		return nil
	}
	syncMsg = syncMsg.Normalize()
	conflictKey := strings.TrimSpace(syncMsg.Key())
	if conflictKey == "" {
		return nil
	}
	r.mu.Lock()
	state := normalizeTeamSyncState(r.state)
	now := time.Now().UTC()
	state.Conflicts[conflictKey] = teamSyncConflict{
		Key:            conflictKey,
		Type:           strings.TrimSpace(conflict.Type),
		TeamID:         teamcore.NormalizeTeamID(conflict.TeamID),
		SubjectID:      strings.TrimSpace(conflict.SubjectID),
		SourceNode:     strings.TrimSpace(conflict.SourceNode),
		Reason:         strings.TrimSpace(conflict.Reason),
		AutoResolvable: teamSyncConflictAutoResolvable(conflict, syncMsg),
		LocalVersion:   conflict.LocalVersion.UTC(),
		RemoteVersion:  conflict.RemoteVersion.UTC(),
		Resolution:     strings.TrimSpace(conflict.Resolution),
		ResolvedBy:     strings.TrimSpace(conflict.ResolvedBy),
		ResolvedAt:     conflict.ResolvedAt.UTC(),
		Sync:           syncMsg,
		UpdatedAt:      now,
	}
	peerPruned, prunedPeer, prunedKey, conflictPruned, conflictPrunedKey := compactTeamSyncState(&state)
	r.state = state
	updateTeamSyncStatusFromState(&r.status, state)
	applyTeamSyncCompactionResult(&r.status, now, peerPruned, prunedPeer, prunedKey, conflictPruned, conflictPrunedKey)
	r.status.LastConflictKey = conflictKey
	r.status.LastConflictReason = strings.TrimSpace(conflict.Reason)
	r.status.LastStateWriteAt = &now
	r.mu.Unlock()
	return writeTeamSyncState(r.statePath, state)
}

func updateTeamSyncStatusFromState(status *SyncTeamSyncStatus, state teamSyncPersistedState) {
	if status == nil {
		return
	}
	status.PersistedCursors = countPersistedStateEntries(state)
	status.PersistedPeerAcks = countPersistedPeerAckEntries(state)
	status.AckPeers = len(state.PeerAcks)
	status.Conflicts = len(state.Conflicts)
	status.ResolvedConflicts = countResolvedConflicts(state)
	status.PendingAcks = countPendingStatus(state, "pending")
	status.ExpiredPending = countPendingStatus(state, "expired")
	status.SupersededPending = countPendingStatus(state, "superseded")
}

func teamSyncConflictAutoResolvable(conflict teamcore.TeamSyncConflict, sync teamcore.TeamSyncMessage) bool {
	switch strings.TrimSpace(sync.Type) {
	case teamcore.TeamSyncTypeMessage:
		return true
	case teamcore.TeamSyncTypeTask:
		return conflict.AutoResolvable
	case teamcore.TeamSyncTypePolicy:
		return false
	default:
		return conflict.AutoResolvable
	}
}

func teamSyncConflictAutoAction(conflict teamSyncConflict) (string, error) {
	if !conflict.AutoResolvable {
		return "", os.ErrInvalid
	}
	switch conflict.Sync.Normalize().Type {
	case teamcore.TeamSyncTypeTask:
		if conflict.RemoteVersion.After(conflict.LocalVersion) {
			return "accept_remote", nil
		}
		return "keep_local", nil
	case teamcore.TeamSyncTypeMessage:
		return "keep_local", nil
	default:
		return "keep_local", nil
	}
}

func compactTeamSyncState(state *teamSyncPersistedState) (int, string, string, int, string) {
	if state == nil {
		return 0, "", "", 0, ""
	}
	now := time.Now().UTC()
	stateValue := normalizeTeamSyncState(*state)
	pruned := 0
	prunedPeer := ""
	prunedKey := ""
	conflictPruned := 0
	conflictPrunedKey := ""
	for peerID, entries := range stateValue.PeerAcks {
		type ackPair struct {
			key   string
			entry teamSyncPeerAck
		}
		pairs := make([]ackPair, 0, len(entries))
		for key, entry := range entries {
			pairs = append(pairs, ackPair{key: key, entry: entry})
		}
		sort.SliceStable(pairs, func(i, j int) bool {
			return pairs[i].entry.UpdatedAt.After(pairs[j].entry.UpdatedAt)
		})
		trimmed := make(map[string]teamSyncPeerAck, len(entries))
		for idx, pair := range pairs {
			if idx >= teamSyncPeerAckPerPeer || (!pair.entry.UpdatedAt.IsZero() && pair.entry.UpdatedAt.Before(now.Add(-teamSyncPeerAckTTL))) {
				pruned++
				prunedPeer = peerID
				prunedKey = pair.key
				continue
			}
			trimmed[pair.key] = pair.entry
		}
		if len(trimmed) == 0 {
			delete(stateValue.PeerAcks, peerID)
			continue
		}
		stateValue.PeerAcks[peerID] = trimmed
	}
	for key, item := range stateValue.Pending {
		status := strings.TrimSpace(item.Status)
		if status == "" {
			status = "pending"
			item.Status = status
		}
		if status == "pending" {
			if (!item.UpdatedAt.IsZero() && item.UpdatedAt.Before(now.Add(-teamSyncPendingMaxAge))) || item.RetryCount >= teamSyncPendingMaxRetry {
				item.Status = "expired"
				item.UpdatedAt = now
				stateValue.Pending[key] = item
			}
			continue
		}
		if !item.UpdatedAt.IsZero() && item.UpdatedAt.Before(now.Add(-teamSyncResolvedTTL)) {
			delete(stateValue.Pending, key)
		}
	}
	for key, item := range stateValue.Conflicts {
		if strings.TrimSpace(item.Resolution) == "" {
			continue
		}
		resolvedAt := item.ResolvedAt.UTC()
		if resolvedAt.IsZero() || item.UpdatedAt.After(resolvedAt) {
			resolvedAt = item.UpdatedAt.UTC()
		}
		if resolvedAt.IsZero() || resolvedAt.After(now.Add(-teamSyncConflictResolvedTTL)) {
			continue
		}
		delete(stateValue.Conflicts, key)
		conflictPruned++
		conflictPrunedKey = key
	}
	*state = stateValue
	return pruned, prunedPeer, prunedKey, conflictPruned, conflictPrunedKey
}

func (r *teamPubSubRuntime) markPendingStatus(syncKey, status string) error {
	if r == nil || strings.TrimSpace(r.statePath) == "" || strings.TrimSpace(syncKey) == "" || strings.TrimSpace(status) == "" {
		return nil
	}
	r.mu.Lock()
	state := normalizeTeamSyncState(r.state)
	item, ok := state.Pending[strings.TrimSpace(syncKey)]
	if !ok {
		r.mu.Unlock()
		return nil
	}
	item.Status = strings.TrimSpace(status)
	item.UpdatedAt = time.Now().UTC()
	state.Pending[strings.TrimSpace(syncKey)] = item
	peerPruned, prunedPeer, prunedKey, conflictPruned, conflictPrunedKey := compactTeamSyncState(&state)
	r.state = state
	now := time.Now().UTC()
	updateTeamSyncStatusFromState(&r.status, state)
	applyTeamSyncCompactionResult(&r.status, now, peerPruned, prunedPeer, prunedKey, conflictPruned, conflictPrunedKey)
	r.status.LastStateWriteAt = &now
	r.mu.Unlock()
	return writeTeamSyncState(r.statePath, state)
}

func (r *teamPubSubRuntime) bumpPendingRetry(syncKey string) error {
	if r == nil || strings.TrimSpace(r.statePath) == "" || strings.TrimSpace(syncKey) == "" {
		return nil
	}
	r.mu.Lock()
	state := normalizeTeamSyncState(r.state)
	item, ok := state.Pending[strings.TrimSpace(syncKey)]
	if !ok {
		r.mu.Unlock()
		return nil
	}
	item.Status = "pending"
	item.RetryCount++
	item.UpdatedAt = time.Now().UTC()
	state.Pending[strings.TrimSpace(syncKey)] = item
	peerPruned, prunedPeer, prunedKey, conflictPruned, conflictPrunedKey := compactTeamSyncState(&state)
	r.state = state
	now := time.Now().UTC()
	updateTeamSyncStatusFromState(&r.status, state)
	applyTeamSyncCompactionResult(&r.status, now, peerPruned, prunedPeer, prunedKey, conflictPruned, conflictPrunedKey)
	r.status.LastStateWriteAt = &now
	r.mu.Unlock()
	return writeTeamSyncState(r.statePath, state)
}

func writeTeamSyncState(path string, state teamSyncPersistedState) error {
	state = normalizeTeamSyncState(state)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if current, err := loadTeamSyncState(path); err == nil {
		state = mergeTeamSyncState(current, state)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

type TeamSyncConflictFilter struct {
	Type            string
	SubjectID       string
	SourceNode      string
	Limit           int
	IncludeResolved bool
}

type TeamSyncConflictRecord struct {
	Key            string    `json:"key"`
	Type           string    `json:"type"`
	TeamID         string    `json:"team_id"`
	SubjectID      string    `json:"subject_id,omitempty"`
	SourceNode     string    `json:"source_node,omitempty"`
	Reason         string    `json:"reason,omitempty"`
	AutoResolvable bool      `json:"auto_resolvable,omitempty"`
	LocalVersion   time.Time `json:"local_version,omitempty"`
	RemoteVersion  time.Time `json:"remote_version,omitempty"`
	Resolution     string    `json:"resolution,omitempty"`
	ResolvedBy     string    `json:"resolved_by,omitempty"`
	ResolvedAt     time.Time `json:"resolved_at,omitempty"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
	SyncType       string    `json:"sync_type,omitempty"`
}

func LoadTeamSyncConflicts(storeRoot, teamID string, filter TeamSyncConflictFilter) ([]TeamSyncConflictRecord, error) {
	state, err := loadTeamSyncState(filepath.Join(strings.TrimSpace(storeRoot), "sync", "team_sync_state.json"))
	if err != nil {
		return nil, err
	}
	teamID = teamcore.NormalizeTeamID(teamID)
	items := make([]TeamSyncConflictRecord, 0, len(state.Conflicts))
	for _, item := range state.Conflicts {
		if teamID != "" && teamcore.NormalizeTeamID(item.TeamID) != teamID {
			continue
		}
		if !filter.IncludeResolved && strings.TrimSpace(item.Resolution) != "" {
			continue
		}
		if filter.Type != "" && strings.TrimSpace(item.Type) != strings.TrimSpace(filter.Type) {
			continue
		}
		if filter.SubjectID != "" && strings.TrimSpace(item.SubjectID) != strings.TrimSpace(filter.SubjectID) {
			continue
		}
		if filter.SourceNode != "" && strings.TrimSpace(item.SourceNode) != strings.TrimSpace(filter.SourceNode) {
			continue
		}
		items = append(items, TeamSyncConflictRecord{
			Key:            item.Key,
			Type:           item.Type,
			TeamID:         item.TeamID,
			SubjectID:      item.SubjectID,
			SourceNode:     item.SourceNode,
			Reason:         item.Reason,
			AutoResolvable: item.AutoResolvable,
			LocalVersion:   item.LocalVersion,
			RemoteVersion:  item.RemoteVersion,
			Resolution:     item.Resolution,
			ResolvedBy:     item.ResolvedBy,
			ResolvedAt:     item.ResolvedAt,
			UpdatedAt:      item.UpdatedAt,
			SyncType:       item.Sync.Type,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	if filter.Limit > 0 && len(items) > filter.Limit {
		items = items[:filter.Limit]
	}
	return items, nil
}

func ResolveTeamSyncConflict(storeRoot, teamID, key, action, actorAgentID string) (TeamSyncConflictRecord, error) {
	path := filepath.Join(strings.TrimSpace(storeRoot), "sync", "team_sync_state.json")
	state, err := loadTeamSyncState(path)
	if err != nil {
		return TeamSyncConflictRecord{}, err
	}
	key = strings.TrimSpace(key)
	teamID = teamcore.NormalizeTeamID(teamID)
	conflict, ok := state.Conflicts[key]
	if !ok || (teamID != "" && teamcore.NormalizeTeamID(conflict.TeamID) != teamID) {
		return TeamSyncConflictRecord{}, os.ErrNotExist
	}
	action = strings.TrimSpace(action)
	if action == "auto" {
		resolvedAction, err := teamSyncConflictAutoAction(conflict)
		if err != nil {
			return TeamSyncConflictRecord{}, err
		}
		action = resolvedAction
	}
	switch action {
	case "dismiss", "keep_local":
	case "accept_remote":
		if !teamSyncConflictReplaySafe(conflict) {
			return TeamSyncConflictRecord{}, os.ErrInvalid
		}
		store, err := teamcore.OpenStore(strings.TrimSpace(storeRoot))
		if err != nil {
			return TeamSyncConflictRecord{}, err
		}
		if _, err := store.ForceApplyReplicatedSync(conflict.Sync); err != nil {
			return TeamSyncConflictRecord{}, err
		}
	default:
		return TeamSyncConflictRecord{}, os.ErrInvalid
	}
	now := time.Now().UTC()
	conflict.Resolution = action
	conflict.ResolvedBy = strings.TrimSpace(actorAgentID)
	conflict.ResolvedAt = now
	conflict.UpdatedAt = now
	state.Conflicts[key] = conflict
	if err := writeTeamSyncState(path, state); err != nil {
		return TeamSyncConflictRecord{}, err
	}
	return TeamSyncConflictRecord{
		Key:            conflict.Key,
		Type:           conflict.Type,
		TeamID:         conflict.TeamID,
		SubjectID:      conflict.SubjectID,
		SourceNode:     conflict.SourceNode,
		Reason:         conflict.Reason,
		AutoResolvable: conflict.AutoResolvable,
		LocalVersion:   conflict.LocalVersion,
		RemoteVersion:  conflict.RemoteVersion,
		Resolution:     conflict.Resolution,
		ResolvedBy:     conflict.ResolvedBy,
		ResolvedAt:     conflict.ResolvedAt,
		UpdatedAt:      conflict.UpdatedAt,
		SyncType:       conflict.Sync.Type,
	}, nil
}

func teamSyncConflictReplaySafe(conflict teamSyncConflict) bool {
	switch conflict.Sync.Normalize().Type {
	case teamcore.TeamSyncTypeTask, teamcore.TeamSyncTypeArtifact, teamcore.TeamSyncTypeMember, teamcore.TeamSyncTypePolicy, teamcore.TeamSyncTypeChannel, teamcore.TeamSyncTypeChannelConfig:
		return true
	default:
		return false
	}
}

func teamSyncStateBucket(state teamSyncPersistedState, stateKey string) map[string]teamSyncCheckpoint {
	switch {
	case strings.HasPrefix(stateKey, "message:"):
		return state.Messages
	case strings.HasPrefix(stateKey, "history:"):
		return state.History
	case strings.HasPrefix(stateKey, "task:"):
		return state.Tasks
	case strings.HasPrefix(stateKey, "artifact:"):
		return state.Artifacts
	case strings.HasPrefix(stateKey, "member:"):
		return state.Members
	case strings.HasPrefix(stateKey, "policy:"):
		return state.Policies
	case strings.HasPrefix(stateKey, "channel:"):
		return state.Channels
	case strings.HasPrefix(stateKey, "channel_config:"):
		return state.Channels
	case strings.HasPrefix(stateKey, "ack:"):
		return state.Acks
	default:
		return state.Messages
	}
}

func teamSyncStateKey(syncMsg teamcore.TeamSyncMessage) string {
	syncMsg = syncMsg.Normalize()
	switch syncMsg.Type {
	case teamcore.TeamSyncTypeMessage:
		if syncMsg.Message != nil {
			return teamSyncStateMessageKey(syncMsg.TeamID, syncMsg.Message.ChannelID)
		}
	case teamcore.TeamSyncTypeHistory:
		return teamSyncStateHistoryKey(syncMsg.TeamID)
	case teamcore.TeamSyncTypeTask:
		if syncMsg.Task != nil {
			return teamSyncStateTaskKey(syncMsg.TeamID, syncMsg.Task.TaskID)
		}
	case teamcore.TeamSyncTypeArtifact:
		if syncMsg.Artifact != nil {
			return teamSyncStateArtifactKey(syncMsg.TeamID, syncMsg.Artifact.ArtifactID)
		}
	case teamcore.TeamSyncTypeMember:
		return teamSyncStateMembersKey(syncMsg.TeamID)
	case teamcore.TeamSyncTypePolicy:
		return teamSyncStatePolicyKey(syncMsg.TeamID)
	case teamcore.TeamSyncTypeChannel:
		if syncMsg.Channel != nil {
			return teamSyncStateChannelKey(syncMsg.TeamID, syncMsg.Channel.ChannelID)
		}
	case teamcore.TeamSyncTypeChannelConfig:
		if syncMsg.ChannelConfig != nil {
			return teamSyncStateChannelConfigKey(syncMsg.TeamID, syncMsg.ChannelConfig.ChannelID)
		}
	case teamcore.TeamSyncTypeAck:
		if syncMsg.Ack != nil {
			return teamSyncStateAckKey(syncMsg.TeamID, syncMsg.Ack.AckedKey, syncMsg.Ack.AckedBy)
		}
	}
	return ""
}

func teamSyncMessageVersion(syncMsg teamcore.TeamSyncMessage) time.Time {
	syncMsg = syncMsg.Normalize()
	switch syncMsg.Type {
	case teamcore.TeamSyncTypeMessage:
		if syncMsg.Message != nil {
			return syncMsg.Message.CreatedAt.UTC()
		}
	case teamcore.TeamSyncTypeHistory:
		if syncMsg.History != nil {
			return syncMsg.History.CreatedAt.UTC()
		}
	case teamcore.TeamSyncTypeTask:
		if syncMsg.Task != nil {
			return taskSyncVersion(*syncMsg.Task)
		}
	case teamcore.TeamSyncTypeArtifact:
		if syncMsg.Artifact != nil {
			return artifactSyncVersion(*syncMsg.Artifact)
		}
	case teamcore.TeamSyncTypeMember:
		return syncMsg.CreatedAt.UTC()
	case teamcore.TeamSyncTypePolicy:
		return syncMsg.CreatedAt.UTC()
	case teamcore.TeamSyncTypeChannel:
		if syncMsg.Channel != nil {
			return channelSyncVersion(*syncMsg.Channel)
		}
	case teamcore.TeamSyncTypeChannelConfig:
		if syncMsg.ChannelConfig != nil {
			if !syncMsg.ChannelConfig.UpdatedAt.IsZero() {
				return syncMsg.ChannelConfig.UpdatedAt.UTC()
			}
			return syncMsg.ChannelConfig.CreatedAt.UTC()
		}
	case teamcore.TeamSyncTypeAck:
		if syncMsg.Ack != nil {
			return syncMsg.Ack.AppliedAt.UTC()
		}
	}
	return time.Time{}
}

func teamSyncVersionAfter(next, current time.Time) bool {
	if next.IsZero() {
		return false
	}
	if current.IsZero() {
		return true
	}
	return next.UTC().After(current.UTC())
}

func teamSyncStateMessageKey(teamID, channelID string) string {
	return "message:" + teamcore.NormalizeTeamID(teamID) + ":" + strings.TrimSpace(channelID)
}

func teamSyncStateHistoryKey(teamID string) string {
	return "history:" + teamcore.NormalizeTeamID(teamID)
}

func teamSyncStateTaskKey(teamID, taskID string) string {
	return "task:" + teamcore.NormalizeTeamID(teamID) + ":" + strings.TrimSpace(taskID)
}

func teamSyncStateArtifactKey(teamID, artifactID string) string {
	return "artifact:" + teamcore.NormalizeTeamID(teamID) + ":" + strings.TrimSpace(artifactID)
}

func teamSyncStateMembersKey(teamID string) string {
	return "member:" + teamcore.NormalizeTeamID(teamID)
}

func teamSyncStatePolicyKey(teamID string) string {
	return "policy:" + teamcore.NormalizeTeamID(teamID)
}

func teamSyncStateChannelKey(teamID, channelID string) string {
	return "channel:" + teamcore.NormalizeTeamID(teamID) + ":" + strings.TrimSpace(channelID)
}

func teamSyncStateChannelConfigKey(teamID, channelID string) string {
	return "channel_config:" + teamcore.NormalizeTeamID(teamID) + ":" + strings.TrimSpace(channelID)
}

func teamSyncStateAckKey(teamID, ackedKey, ackedBy string) string {
	return "ack:" + teamcore.NormalizeTeamID(teamID) + ":" + strings.TrimSpace(ackedKey) + ":" + strings.TrimSpace(ackedBy)
}

func channelSyncVersion(channel teamcore.Channel) time.Time {
	if !channel.UpdatedAt.IsZero() {
		return channel.UpdatedAt.UTC()
	}
	return channel.CreatedAt.UTC()
}
