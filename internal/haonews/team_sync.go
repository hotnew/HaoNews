package haonews

import (
	"context"
	"strings"
	"sync"
	"time"

	teamcore "hao.news/internal/haonews/team"
)

const teamSyncRecentScanLimit = 500

type teamSyncTransport interface {
	PublishTeamSync(context.Context, teamcore.TeamSyncMessage) error
	SubscribeTeamSync(context.Context, string, func(teamcore.TeamSyncMessage) (bool, error)) error
}

type teamPubSubRuntime struct {
	store     *teamcore.Store
	transport teamSyncTransport
	nodeID    string
	startedAt time.Time

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
	status          SyncTeamSyncStatus
}

func startTeamPubSubRuntime(storeRoot string, transport teamSyncTransport, nodeID string) (*teamPubSubRuntime, error) {
	if transport == nil {
		return nil, nil
	}
	store, err := teamcore.OpenStore(storeRoot)
	if err != nil {
		return nil, err
	}
	return &teamPubSubRuntime{
		store:           store,
		transport:       transport,
		nodeID:          strings.TrimSpace(nodeID),
		startedAt:       time.Now().UTC(),
		primedChannels:  make(map[string]struct{}),
		primedHistory:   make(map[string]struct{}),
		primedTasks:     make(map[string]struct{}),
		primedArtifacts: make(map[string]struct{}),
		primedMembers:   make(map[string]struct{}),
		primedPolicies:  make(map[string]struct{}),
		primedConfig:    make(map[string]struct{}),
		subscribed:      make(map[string]struct{}),
		seen:            make(map[string]time.Time),
		status: SyncTeamSyncStatus{
			Enabled: true,
			NodeID:  strings.TrimSpace(nodeID),
		},
	}, nil
}

func (r *teamPubSubRuntime) SyncOnce(ctx context.Context, logf func(string, ...any)) error {
	if r == nil || r.store == nil || r.transport == nil {
		return nil
	}
	teams, err := r.store.ListTeams()
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
		applied, err := r.store.ApplyReplicatedSync(syncMsg)
		if applied {
			r.rememberSeen(syncMsg.Key())
			r.recordApplied(syncMsg)
		} else if err == nil {
			r.recordSkipped(syncMsg)
		} else {
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
	channels, err := r.store.ListChannels(teamID)
	if err != nil {
		return err
	}
	for _, channel := range channels {
		channelID := strings.TrimSpace(channel.ChannelID)
		if channelID == "" {
			continue
		}
		items, err := r.store.LoadMessages(teamID, channelID, teamSyncRecentScanLimit)
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
			if firstScan && !r.shouldPublishSinceStart(msg.CreatedAt) {
				r.rememberSeen(syncKey)
				continue
			}
			if r.seenKey(syncKey) {
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
			r.recordPublished(syncMsg)
		}
		if firstScan {
			r.markPrimedChannel(key)
		}
	}
	return nil
}

func (r *teamPubSubRuntime) syncTeamHistory(ctx context.Context, teamID string, logf func(string, ...any)) error {
	items, err := r.store.LoadHistory(teamID, teamSyncRecentScanLimit)
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
		if firstScan && !r.shouldPublishSinceStart(event.CreatedAt) {
			r.rememberSeen(syncKey)
			continue
		}
		if r.seenKey(syncKey) {
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
		r.recordPublished(syncMsg)
	}
	if firstScan {
		r.markPrimedHistory(teamID)
	}
	return nil
}

func (r *teamPubSubRuntime) syncTeamTasks(ctx context.Context, teamID string, logf func(string, ...any)) error {
	items, err := r.store.LoadTasks(teamID, teamSyncRecentScanLimit)
	if err != nil {
		return err
	}
	firstScan := !r.isPrimedTasks(teamID)
	for i := len(items) - 1; i >= 0; i-- {
		task := items[i]
		r.recordScannedTask(teamID, task.TaskID)
		syncMsg := teamcore.TeamSyncMessage{
			Type:       teamcore.TeamSyncTypeTask,
			TeamID:     teamID,
			Task:       &task,
			SourceNode: r.nodeID,
			CreatedAt:  time.Now().UTC(),
		}.Normalize()
		syncKey := syncMsg.Key()
		if firstScan && !r.shouldPublishSinceStart(taskSyncVersion(task)) {
			r.rememberSeen(syncKey)
			continue
		}
		if r.seenKey(syncKey) {
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
		r.recordPublished(syncMsg)
	}
	if firstScan {
		r.markPrimedTasks(teamID)
	}
	return nil
}

func (r *teamPubSubRuntime) syncTeamMembers(ctx context.Context, teamID string, logf func(string, ...any)) error {
	members, version, err := r.store.LoadMembersSnapshot(teamID)
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
		if firstScan && !r.shouldPublishSinceStart(version) {
			r.rememberSeen(syncKey)
		} else if !r.seenKey(syncKey) {
			if err := r.transport.PublishTeamSync(ctx, syncMsg); err != nil {
				r.recordError(teamID, err)
				if logf != nil {
					logf("team sync publish members %s: %v", teamID, err)
				}
			} else {
				r.rememberSeen(syncKey)
				r.recordPublished(syncMsg)
			}
		}
	}
	if firstScan {
		r.markPrimedMembers(teamID)
	}
	return nil
}

func (r *teamPubSubRuntime) syncTeamPolicy(ctx context.Context, teamID string, logf func(string, ...any)) error {
	policy, version, err := r.store.LoadPolicySnapshot(teamID)
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
		if firstScan && !r.shouldPublishSinceStart(version) {
			r.rememberSeen(syncKey)
		} else if !r.seenKey(syncKey) {
			if err := r.transport.PublishTeamSync(ctx, syncMsg); err != nil {
				r.recordError(teamID, err)
				if logf != nil {
					logf("team sync publish policy %s: %v", teamID, err)
				}
			} else {
				r.rememberSeen(syncKey)
				r.recordPublished(syncMsg)
			}
		}
	}
	if firstScan {
		r.markPrimedPolicy(teamID)
	}
	return nil
}

func (r *teamPubSubRuntime) syncTeamChannels(ctx context.Context, teamID string, logf func(string, ...any)) error {
	items, err := r.store.ListChannels(teamID)
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
		if firstScan && !r.shouldPublishSinceStart(version) {
			r.rememberSeen(syncKey)
			r.markPrimedConfigChannel(teamID, channel.ChannelID)
			continue
		}
		if r.seenKey(syncKey) {
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
		r.recordPublished(syncMsg)
		if firstScan {
			r.markPrimedConfigChannel(teamID, channel.ChannelID)
		}
	}
	return nil
}

func (r *teamPubSubRuntime) syncTeamArtifacts(ctx context.Context, teamID string, logf func(string, ...any)) error {
	items, err := r.store.LoadArtifacts(teamID, teamSyncRecentScanLimit)
	if err != nil {
		return err
	}
	firstScan := !r.isPrimedArtifacts(teamID)
	for i := len(items) - 1; i >= 0; i-- {
		artifact := items[i]
		r.recordScannedArtifact(teamID, artifact.ArtifactID)
		syncMsg := teamcore.TeamSyncMessage{
			Type:       teamcore.TeamSyncTypeArtifact,
			TeamID:     teamID,
			Artifact:   &artifact,
			SourceNode: r.nodeID,
			CreatedAt:  time.Now().UTC(),
		}.Normalize()
		syncKey := syncMsg.Key()
		if firstScan && !r.shouldPublishSinceStart(artifactSyncVersion(artifact)) {
			r.rememberSeen(syncKey)
			continue
		}
		if r.seenKey(syncKey) {
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
		r.recordPublished(syncMsg)
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

func (r *teamPubSubRuntime) Status() SyncTeamSyncStatus {
	if r == nil {
		return SyncTeamSyncStatus{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
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
		}
	case "applied":
		r.status.LastAppliedKey = syncMsg.Key()
		r.status.LastAppliedAt = &now
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

func channelSyncVersion(channel teamcore.Channel) time.Time {
	if !channel.UpdatedAt.IsZero() {
		return channel.UpdatedAt.UTC()
	}
	return channel.CreatedAt.UTC()
}
