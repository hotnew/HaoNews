package haonews

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/discovery"
	routingdisc "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	discutil "github.com/libp2p/go-libp2p/p2p/discovery/util"

	"github.com/anacrolix/torrent/metainfo"
)

const (
	syncAnnouncementProtocol   = "haonews-sync/0.1"
	syncPubSubTopicPrefix      = "haonews/announce"
	syncPubSubGlobalTopic      = syncPubSubTopicPrefix + "/global"
	syncPubSubDiscoveryDefault = "haonews/sync"
	creditProofTopicPrefix     = "haonews/credit/proofs"
	reservedTopicAll           = "all"
)

type SyncAnnouncement struct {
	Protocol        string   `json:"protocol"`
	InfoHash        string   `json:"infohash"`
	Ref             string   `json:"ref,omitempty"`
	Magnet          string   `json:"magnet,omitempty"`
	SizeBytes       int64    `json:"size_bytes,omitempty"`
	Kind            string   `json:"kind,omitempty"`
	Channel         string   `json:"channel,omitempty"`
	Title           string   `json:"title,omitempty"`
	Author          string   `json:"author,omitempty"`
	CreatedAt       string   `json:"created_at,omitempty"`
	Project         string   `json:"project,omitempty"`
	NetworkID       string   `json:"network_id,omitempty"`
	Topics          []string `json:"topics,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	OriginPublicKey string   `json:"origin_public_key,omitempty"`
	ParentPublicKey string   `json:"parent_public_key,omitempty"`
	LibP2PPeerID    string   `json:"libp2p_peer_id,omitempty"`
	SourceHost      string   `json:"source_host,omitempty"`
}

type pubsubRuntime struct {
	host      *libp2pRuntime
	pubsub    *pubsub.PubSub
	discovery *routingdisc.RoutingDiscovery

	mu                  sync.Mutex
	topics              map[string]*pubsub.Topic
	subscriptions       map[string]*pubsub.Subscription
	joinedTopics        []string
	discoveryNamespaces []string
	status              SyncPubSubStatus
}

func startPubSubRuntime(
	ctx context.Context,
	hostRuntime *libp2pRuntime,
	rules SyncSubscriptions,
	onAnnouncement func(SyncAnnouncement) (bool, error),
	onCreditProof func(OnlineProof) (bool, error),
) (*pubsubRuntime, error) {
	if hostRuntime == nil || hostRuntime.host == nil {
		return nil, nil
	}

	ps, err := pubsub.NewGossipSub(ctx, hostRuntime.host)
	if err != nil {
		return nil, fmt.Errorf("create libp2p pubsub: %w", err)
	}

	rules.Normalize()
	joinedTopics := subscribedAnnouncementTopics(hostRuntime.networkID, rules)
	runtime := &pubsubRuntime{
		host:                hostRuntime,
		pubsub:              ps,
		topics:              make(map[string]*pubsub.Topic),
		subscriptions:       make(map[string]*pubsub.Subscription),
		joinedTopics:        joinedTopics,
		discoveryNamespaces: discoveryNamespaces(hostRuntime.networkID, hostRuntime.rendezvous, rules),
		status: SyncPubSubStatus{
			Enabled:                          true,
			JoinedTopics:                     append([]string(nil), joinedTopics...),
			DiscoveryNamespaces:              discoveryNamespaces(hostRuntime.networkID, hostRuntime.rendezvous, rules),
			DiscoveryFeeds:                   append([]string(nil), rules.discoveryFeeds()...),
			DiscoveryTopics:                  append([]string(nil), rules.discoveryTopics()...),
			TopicWhitelist:                   append([]string(nil), rules.TopicWhitelist...),
			TopicAliasPairs:                  topicAliasPairs(rules.TopicAliases),
			AllowedOriginKeys:                append([]string(nil), rules.AllowedOriginKeys...),
			BlockedOriginKeys:                append([]string(nil), rules.BlockedOriginKeys...),
			AllowedParentKeys:                append([]string(nil), rules.AllowedParentKeys...),
			BlockedParentKeys:                append([]string(nil), rules.BlockedParentKeys...),
			LiveAllowedOriginKeys:            append([]string(nil), rules.LiveAllowedOriginKeys...),
			LiveBlockedOriginKeys:            append([]string(nil), rules.LiveBlockedOriginKeys...),
			LiveAllowedParentKeys:            append([]string(nil), rules.LiveAllowedParentKeys...),
			LiveBlockedParentKeys:            append([]string(nil), rules.LiveBlockedParentKeys...),
			LivePublicMutedOriginKeys:        append([]string(nil), rules.LivePublicMutedOriginKeys...),
			LivePublicMutedParentKeys:        append([]string(nil), rules.LivePublicMutedParentKeys...),
			LivePublicRateLimitMessages:      rules.LivePublicRateLimitMessages,
			LivePublicRateLimitWindowSeconds: rules.LivePublicRateLimitWindowSeconds,
		},
	}

	if hostRuntime.dht != nil {
		runtime.discovery = routingdisc.NewRoutingDiscovery(hostRuntime.dht)
		runtime.startDiscoveryLoops(ctx)
	}

	for _, topicName := range joinedTopics {
		if err := runtime.subscribe(ctx, topicName, onAnnouncement); err != nil {
			runtime.Close()
			return nil, err
		}
	}
	if onCreditProof != nil {
		if err := runtime.subscribeCreditProofs(ctx, creditProofTopic(hostRuntime.networkID), onCreditProof); err != nil {
			runtime.Close()
			return nil, err
		}
	}

	return runtime, nil
}

func (r *pubsubRuntime) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, sub := range r.subscriptions {
		sub.Cancel()
		delete(r.subscriptions, name)
	}
	for name, topic := range r.topics {
		_ = topic.Close()
		delete(r.topics, name)
	}
	return nil
}

func (r *pubsubRuntime) Status() SyncPubSubStatus {
	if r == nil {
		return SyncPubSubStatus{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	status := r.status
	status.JoinedTopics = append([]string(nil), r.joinedTopics...)
	status.DiscoveryNamespaces = append([]string(nil), r.discoveryNamespaces...)
	status.DiscoveryFeeds = append([]string(nil), r.status.DiscoveryFeeds...)
	status.DiscoveryTopics = append([]string(nil), r.status.DiscoveryTopics...)
	status.TopicWhitelist = append([]string(nil), r.status.TopicWhitelist...)
	status.TopicAliasPairs = append([]string(nil), r.status.TopicAliasPairs...)
	status.AllowedOriginKeys = append([]string(nil), r.status.AllowedOriginKeys...)
	status.BlockedOriginKeys = append([]string(nil), r.status.BlockedOriginKeys...)
	status.AllowedParentKeys = append([]string(nil), r.status.AllowedParentKeys...)
	status.BlockedParentKeys = append([]string(nil), r.status.BlockedParentKeys...)
	status.LiveAllowedOriginKeys = append([]string(nil), r.status.LiveAllowedOriginKeys...)
	status.LiveBlockedOriginKeys = append([]string(nil), r.status.LiveBlockedOriginKeys...)
	status.LiveAllowedParentKeys = append([]string(nil), r.status.LiveAllowedParentKeys...)
	status.LiveBlockedParentKeys = append([]string(nil), r.status.LiveBlockedParentKeys...)
	status.LivePublicMutedOriginKeys = append([]string(nil), r.status.LivePublicMutedOriginKeys...)
	status.LivePublicMutedParentKeys = append([]string(nil), r.status.LivePublicMutedParentKeys...)
	status.LivePublicRateLimitMessages = r.status.LivePublicRateLimitMessages
	status.LivePublicRateLimitWindowSeconds = r.status.LivePublicRateLimitWindowSeconds
	return status
}

func (r *pubsubRuntime) PublishAnnouncement(ctx context.Context, announcement SyncAnnouncement) error {
	if r == nil {
		return nil
	}
	announcement = normalizeAnnouncement(announcement)
	if announcement.InfoHash == "" || announcement.Ref == "" {
		return fmt.Errorf("announcement requires both infohash and ref")
	}
	if r.host != nil && r.host.host != nil && announcement.LibP2PPeerID == "" {
		announcement.LibP2PPeerID = r.host.host.ID().String()
	}
	body, err := json.Marshal(announcement)
	if err != nil {
		return err
	}
	for _, topicName := range announcementTopics(r.host.networkID, announcement) {
		topic, err := r.ensureTopic(topicName)
		if err != nil {
			r.recordError(err)
			return err
		}
		publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = topic.Publish(publishCtx, body)
		cancel()
		if err != nil {
			r.recordError(err)
			return fmt.Errorf("publish to %s: %w", topicName, err)
		}
		r.recordPublished(topicName, announcement.InfoHash)
	}
	return nil
}

func (r *pubsubRuntime) PublishCreditProof(ctx context.Context, proof OnlineProof) error {
	if r == nil {
		return nil
	}
	body, err := json.Marshal(proof)
	if err != nil {
		return err
	}
	topicName := creditProofTopic(r.host.networkID)
	topic, err := r.ensureTopic(topicName)
	if err != nil {
		r.recordError(err)
		return err
	}
	publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = topic.Publish(publishCtx, body)
	cancel()
	if err != nil {
		r.recordError(err)
		return fmt.Errorf("publish credit proof to %s: %w", topicName, err)
	}
	r.recordCreditPublished(proof.ProofID)
	return nil
}

func (r *pubsubRuntime) subscribe(ctx context.Context, topicName string, onAnnouncement func(SyncAnnouncement) (bool, error)) error {
	topic, err := r.ensureTopic(topicName)
	if err != nil {
		return err
	}
	sub, err := topic.Subscribe()
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", topicName, err)
	}
	r.mu.Lock()
	r.subscriptions[topicName] = sub
	r.mu.Unlock()

	go func() {
		for {
			msg, err := sub.Next(ctx)
			if err != nil {
				if ctx.Err() == nil {
					r.recordError(err)
				}
				return
			}
			if msg.ReceivedFrom == r.host.host.ID() {
				continue
			}
			var announcement SyncAnnouncement
			if err := json.Unmarshal(msg.Data, &announcement); err != nil {
				r.recordError(fmt.Errorf("decode pubsub message on %s: %w", topicName, err))
				continue
			}
			announcement = normalizeAnnouncement(announcement)
			if announcement.InfoHash == "" || announcement.Ref == "" {
				r.recordError(fmt.Errorf("ignore incomplete pubsub announcement on %s", topicName))
				continue
			}
			enqueued := false
			if onAnnouncement != nil {
				enqueued, err = onAnnouncement(announcement)
				if err != nil {
					r.recordError(fmt.Errorf("handle pubsub announcement on %s: %w", topicName, err))
					continue
				}
			}
			r.recordReceived(topicName, announcement.InfoHash, enqueued)
		}
	}()
	return nil
}

func (r *pubsubRuntime) subscribeCreditProofs(ctx context.Context, topicName string, onCreditProof func(OnlineProof) (bool, error)) error {
	topic, err := r.ensureTopic(topicName)
	if err != nil {
		return err
	}
	sub, err := topic.Subscribe()
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", topicName, err)
	}
	r.mu.Lock()
	r.subscriptions[topicName] = sub
	r.joinedTopics = uniqueStrings(append(r.joinedTopics, topicName))
	r.status.JoinedTopics = append([]string(nil), r.joinedTopics...)
	r.mu.Unlock()

	go func() {
		for {
			msg, err := sub.Next(ctx)
			if err != nil {
				if ctx.Err() == nil {
					r.recordError(err)
				}
				return
			}
			if msg.ReceivedFrom == r.host.host.ID() {
				continue
			}
			var proof OnlineProof
			if err := json.Unmarshal(msg.Data, &proof); err != nil {
				r.recordError(fmt.Errorf("decode credit proof on %s: %w", topicName, err))
				continue
			}
			saved := false
			if onCreditProof != nil {
				saved, err = onCreditProof(proof)
				if err != nil {
					r.recordError(fmt.Errorf("handle credit proof on %s: %w", topicName, err))
					continue
				}
			}
			r.recordCreditReceived(proof.ProofID, saved)
		}
	}()
	return nil
}

func (r *pubsubRuntime) ensureTopic(topicName string) (*pubsub.Topic, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if topic, ok := r.topics[topicName]; ok {
		return topic, nil
	}
	topic, err := r.pubsub.Join(topicName)
	if err != nil {
		return nil, fmt.Errorf("join pubsub topic %s: %w", topicName, err)
	}
	r.topics[topicName] = topic
	return topic, nil
}

func (r *pubsubRuntime) startDiscoveryLoops(ctx context.Context) {
	if r.discovery == nil {
		return
	}
	for _, namespace := range r.discoveryNamespaces {
		discutil.Advertise(ctx, r.discovery, namespace)
		go r.findPeersLoop(ctx, namespace)
	}
}

func (r *pubsubRuntime) findPeersLoop(ctx context.Context, namespace string) {
	ticker := time.NewTicker(45 * time.Second)
	defer ticker.Stop()
	for {
		r.findPeersOnce(ctx, namespace)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *pubsubRuntime) findPeersOnce(ctx context.Context, namespace string) {
	findCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	peers, err := discutil.FindPeers(findCtx, r.discovery, namespace, discovery.Limit(24))
	if err != nil {
		r.recordError(fmt.Errorf("find peers for %s: %w", namespace, err))
		return
	}
	for _, info := range peers {
		if info.ID == "" || info.ID == r.host.host.ID() {
			continue
		}
		if len(r.host.host.Network().ConnsToPeer(info.ID)) > 0 {
			continue
		}
		connectCtx, connectCancel := context.WithTimeout(ctx, 8*time.Second)
		err := r.host.host.Connect(connectCtx, info)
		connectCancel()
		if err != nil {
			r.recordError(fmt.Errorf("connect discovered peer %s: %w", info.ID, err))
		}
	}
}

func (r *pubsubRuntime) recordPublished(topicName, infoHash string) {
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.Published++
	r.status.LastTopic = topicName
	r.status.LastInfoHash = infoHash
	r.status.LastPublishedAt = &now
}

func (r *pubsubRuntime) recordReceived(topicName, infoHash string, enqueued bool) {
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.Received++
	if enqueued {
		r.status.Enqueued++
	}
	r.status.LastTopic = topicName
	r.status.LastInfoHash = infoHash
	r.status.LastReceivedAt = &now
}

func (r *pubsubRuntime) recordCreditPublished(proofID string) {
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.CreditPublished++
	r.status.LastCreditProofID = proofID
	r.status.LastCreditAt = &now
}

func (r *pubsubRuntime) recordCreditReceived(proofID string, saved bool) {
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.CreditReceived++
	if saved {
		r.status.CreditSaved++
	}
	r.status.LastCreditProofID = proofID
	r.status.LastCreditAt = &now
}

func (r *pubsubRuntime) recordError(err error) {
	if err == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.LastError = err.Error()
}

func subscribedAnnouncementTopics(networkID string, rules SyncSubscriptions) []string {
	rules.Normalize()
	topics := []string{namespacedGlobalTopic(networkID)}
	if !rules.Empty() {
		topics = topics[:0]
		if containsFold(rules.Topics, reservedTopicAll) {
			topics = append(topics, namespacedGlobalTopic(networkID))
		}
		for _, channel := range rules.Channels {
			topics = append(topics, namespacedTopic(networkID, "channel", channel))
		}
		for _, topic := range rules.Topics {
			if strings.EqualFold(strings.TrimSpace(topic), reservedTopicAll) {
				continue
			}
			topics = append(topics, namespacedTopic(networkID, "topic", topic))
		}
		for _, tag := range rules.Tags {
			topics = append(topics, namespacedTopic(networkID, "tag", tag))
		}
	}
	for _, feed := range rules.discoveryFeeds() {
		if strings.EqualFold(strings.TrimSpace(feed), "global") {
			topics = append(topics, namespacedGlobalTopic(networkID))
			continue
		}
		if channel := feedToChannel(feed); channel != "" {
			topics = append(topics, namespacedTopic(networkID, "channel", channel))
		}
	}
	for _, topic := range rules.discoveryTopics() {
		if strings.EqualFold(strings.TrimSpace(topic), reservedTopicAll) {
			topics = append(topics, namespacedGlobalTopic(networkID))
			continue
		}
		topics = append(topics, namespacedTopic(networkID, "topic", topic))
	}
	return uniqueStrings(topics)
}

func discoveryNamespaces(networkID string, namespaces []string, rules SyncSubscriptions) []string {
	values := []string{namespacedDiscoveryNamespace(networkID, syncPubSubDiscoveryDefault)}
	for _, namespace := range namespaces {
		values = append(values, namespacedDiscoveryNamespace(networkID, namespace))
	}
	for _, feed := range rules.discoveryFeeds() {
		values = append(values, namespacedDiscoveryNamespace(networkID, "feed/"+strings.ToLower(strings.TrimSpace(feed))))
	}
	for _, topic := range rules.discoveryTopics() {
		values = append(values, namespacedDiscoveryNamespace(networkID, "topic/"+strings.ToLower(strings.TrimSpace(topic))))
	}
	return uniqueStrings(values)
}

func announcementTopics(networkID string, announcement SyncAnnouncement) []string {
	topics := []string{namespacedGlobalTopic(networkID)}
	topics = append(topics, namespacedTopic(networkID, "topic", reservedTopicAll))
	if announcement.Channel != "" {
		topics = append(topics, namespacedTopic(networkID, "channel", announcement.Channel))
	}
	for _, topic := range announcement.Topics {
		topics = append(topics, namespacedTopic(networkID, "topic", topic))
	}
	for _, tag := range announcement.Tags {
		topics = append(topics, namespacedTopic(networkID, "tag", tag))
	}
	return uniqueStrings(topics)
}

func namespacedGlobalTopic(networkID string) string {
	if networkID == "" {
		return syncPubSubGlobalTopic
	}
	return syncPubSubTopicPrefix + "/" + networkID + "/global"
}

func namespacedTopic(networkID, kind, value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if networkID == "" {
		return syncPubSubTopicPrefix + "/" + kind + "/" + url.PathEscape(value)
	}
	return syncPubSubTopicPrefix + "/" + networkID + "/" + kind + "/" + url.PathEscape(value)
}

func namespacedDiscoveryNamespace(networkID, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if networkID == "" {
		return value
	}
	return "haonews/discovery/" + networkID + "/" + url.PathEscape(value)
}

func feedToChannel(feed string) string {
	feed = strings.TrimSpace(strings.ToLower(feed))
	if feed == "" {
		return ""
	}
	if strings.Contains(feed, "/") {
		return feed
	}
	return "hao.news/" + feed
}

func creditProofTopic(networkID string) string {
	networkID = normalizeNetworkID(networkID)
	if networkID == "" {
		return creditProofTopicPrefix
	}
	return creditProofTopicPrefix + "/" + url.PathEscape(networkID)
}

func normalizeAnnouncement(announcement SyncAnnouncement) SyncAnnouncement {
	announcement.Protocol = syncAnnouncementProtocol
	announcement.InfoHash = strings.ToLower(strings.TrimSpace(announcement.InfoHash))
	announcement.Ref = strings.TrimSpace(announcement.Ref)
	announcement.Magnet = strings.TrimSpace(announcement.Magnet)
	if announcement.InfoHash == "" && announcement.Ref != "" {
		if ref, err := ParseSyncRef(announcement.Ref); err == nil {
			announcement.InfoHash = ref.InfoHash
			announcement.Ref = ref.Magnet
		}
	}
	if announcement.Magnet != "" {
		if ref, err := ParseSyncRef(announcement.Magnet); err == nil {
			if announcement.InfoHash == "" {
				announcement.InfoHash = ref.InfoHash
			}
			announcement.Magnet = ref.Magnet
			if announcement.Ref == "" {
				announcement.Ref = ref.Magnet
			}
		}
	}
	if announcement.Ref == "" && announcement.InfoHash != "" {
		announcement.Ref = CanonicalSyncRef(announcement.InfoHash, announcement.Title)
	}
	if announcement.Magnet == "" && announcement.InfoHash != "" {
		announcement.Magnet = CanonicalMagnet(announcement.InfoHash, announcement.Title)
	}
	announcement.Channel = strings.TrimSpace(announcement.Channel)
	announcement.Title = strings.TrimSpace(announcement.Title)
	announcement.Author = strings.TrimSpace(announcement.Author)
	announcement.Project = strings.TrimSpace(announcement.Project)
	announcement.NetworkID = normalizeNetworkID(announcement.NetworkID)
	announcement.OriginPublicKey = normalizePublicKey(announcement.OriginPublicKey)
	announcement.ParentPublicKey = normalizePublicKey(announcement.ParentPublicKey)
	announcement.LibP2PPeerID = strings.TrimSpace(announcement.LibP2PPeerID)
	announcement.Topics = uniqueCanonicalTopics(announcement.Topics)
	announcement.Tags = uniqueFold(announcement.Tags)
	return announcement
}

func localAnnouncements(store *Store) ([]SyncAnnouncement, error) {
	var out []SyncAnnouncement
	if err := store.WalkTorrentFiles(func(_ string, refPath string) error {
		mi, err := metainfo.LoadFromFile(refPath)
		if err != nil {
			return nil
		}
		info, err := mi.UnmarshalInfo()
		if err != nil {
			return nil
		}
		contentDir := filepath.Join(store.DataDir, info.BestName())
		msg, _, err := LoadMessage(contentDir)
		if err != nil {
			return nil
		}
		out = append(out, buildAnnouncement(msg, mi, info))
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt < out[j].CreatedAt
	})
	return out, nil
}

func buildAnnouncement(msg Message, mi *metainfo.MetaInfo, info metainfo.Info) SyncAnnouncement {
	infoHash := strings.ToLower(mi.HashInfoBytes().HexString())
	displayName := strings.TrimSpace(info.BestName())
	if displayName == "" {
		displayName = strings.TrimSpace(msg.Title)
	}
	return normalizeAnnouncement(SyncAnnouncement{
		InfoHash:        infoHash,
		Ref:             CanonicalSyncRef(infoHash, displayName),
		Magnet:          CanonicalMagnet(infoHash, displayName),
		SizeBytes:       info.TotalLength(),
		Kind:            msg.Kind,
		Channel:         msg.Channel,
		Title:           msg.Title,
		Author:          msg.Author,
		CreatedAt:       msg.CreatedAt,
		Project:         nestedString(msg.Extensions, "project"),
		NetworkID:       nestedString(msg.Extensions, "network_id"),
		Topics:          stringSlice(msg.Extensions["topics"]),
		Tags:            append([]string(nil), msg.Tags...),
		OriginPublicKey: announcementOriginPublicKey(msg),
		ParentPublicKey: announcementParentPublicKey(msg),
	})
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func nestedString(value map[string]any, path ...string) string {
	current := any(value)
	for _, key := range path {
		obj, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current, ok = obj[key]
		if !ok {
			return ""
		}
	}
	switch v := current.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return ""
	}
}

func stringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		if typed, ok := value.([]string); ok {
			return uniqueCanonicalTopics(typed)
		}
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			continue
		}
		out = append(out, text)
	}
	return uniqueCanonicalTopics(out)
}

func matchesAnnouncement(announcement SyncAnnouncement, rules SyncSubscriptions) bool {
	announcement = normalizeAnnouncement(announcement)
	rules.Normalize()
	if blocked, allowed := matchPublicKeyFilters(announcement.OriginPublicKey, announcement.ParentPublicKey, rules); blocked {
		return false
	} else if allowed {
		return true
	}
	whitelist := topicWhitelistSet(rules.TopicWhitelist, rules.TopicAliases)
	announcement.Topics = uniqueCanonicalTopicsWithAliases(announcement.Topics, rules.TopicAliases, whitelist)
	if !withinMaxAge(announcement.CreatedAt, rules.MaxAgeDays) {
		return false
	}
	if !withinMaxBundleSize(announcement.SizeBytes, rules.MaxBundleMB) {
		return false
	}
	if rules.Empty() {
		return true
	}
	if containsFold(rules.Topics, reservedTopicAll) {
		return true
	}
	if containsFold(rules.Channels, announcement.Channel) {
		return true
	}
	if containsFold(rules.Authors, announcement.Author) {
		return true
	}
	for _, topic := range announcement.Topics {
		if containsFold(rules.Topics, topic) {
			return true
		}
	}
	for _, tag := range announcement.Tags {
		if containsFold(rules.Tags, tag) {
			return true
		}
	}
	return false
}

func announcementOriginPublicKey(msg Message) string {
	if value := nestedString(msg.Extensions, "origin_public_key"); value != "" {
		return value
	}
	if msg.Origin != nil {
		return strings.TrimSpace(msg.Origin.PublicKey)
	}
	return ""
}

func announcementParentPublicKey(msg Message) string {
	if value := nestedString(msg.Extensions, "parent_public_key"); value != "" {
		return value
	}
	return nestedString(msg.Extensions, "hd.parent_pubkey")
}
