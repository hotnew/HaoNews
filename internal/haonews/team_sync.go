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

	mu             sync.Mutex
	primedChannels map[string]struct{}
	primedHistory  map[string]struct{}
	subscribed     map[string]struct{}
	seen           map[string]time.Time
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
		store:          store,
		transport:      transport,
		nodeID:         strings.TrimSpace(nodeID),
		primedChannels: make(map[string]struct{}),
		primedHistory:  make(map[string]struct{}),
		subscribed:     make(map[string]struct{}),
		seen:           make(map[string]time.Time),
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
		applied, err := r.store.ApplyReplicatedSync(syncMsg)
		if applied {
			r.rememberSeen(syncMsg.Key())
		}
		return applied, err
	}); err != nil {
		return err
	}
	r.mu.Lock()
	r.subscribed[teamID] = struct{}{}
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
		if !r.isPrimedChannel(key) {
			for _, item := range items {
				r.rememberSeen(teamSyncMessageKey(item.MessageID))
			}
			r.markPrimedChannel(key)
			continue
		}
		for i := len(items) - 1; i >= 0; i-- {
			msg := items[i]
			if strings.TrimSpace(msg.MessageID) == "" {
				msg.MessageID = buildMessageIDForSync(msg)
			}
			syncKey := teamSyncMessageKey(msg.MessageID)
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
				if logf != nil {
					logf("team sync publish message %s/%s: %v", teamID, channelID, err)
				}
				continue
			}
			r.rememberSeen(syncKey)
		}
	}
	return nil
}

func (r *teamPubSubRuntime) syncTeamHistory(ctx context.Context, teamID string, logf func(string, ...any)) error {
	items, err := r.store.LoadHistory(teamID, teamSyncRecentScanLimit)
	if err != nil {
		return err
	}
	if !r.isPrimedHistory(teamID) {
		for _, item := range items {
			if strings.TrimSpace(item.EventID) == "" {
				continue
			}
			r.rememberSeen(teamSyncHistoryKey(item.EventID))
		}
		r.markPrimedHistory(teamID)
		return nil
	}
	for i := len(items) - 1; i >= 0; i-- {
		event := items[i]
		if strings.TrimSpace(event.Scope) != "message" {
			continue
		}
		if strings.TrimSpace(event.EventID) == "" {
			continue
		}
		syncKey := teamSyncHistoryKey(event.EventID)
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
			if logf != nil {
				logf("team sync publish history %s: %v", teamID, err)
			}
			continue
		}
		r.rememberSeen(syncKey)
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
}

func (r *teamPubSubRuntime) isPrimedHistory(teamID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.primedHistory[teamID]
	return ok
}

func (r *teamPubSubRuntime) markPrimedHistory(teamID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.primedHistory[teamID] = struct{}{}
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
