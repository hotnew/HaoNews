package haonews

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const syncStatusRedisTTL = 30 * time.Second

type SyncRuntimeStatus struct {
	UpdatedAt        time.Time                  `json:"updated_at"`
	StartedAt        time.Time                  `json:"started_at"`
	PID              int                        `json:"pid"`
	StoreRoot        string                     `json:"store_root"`
	QueuePath        string                     `json:"queue_path"`
	Mode             string                     `json:"mode"`
	Seed             bool                       `json:"seed"`
	NetworkID        string                     `json:"network_id"`
	LibP2P           SyncLibP2PStatus           `json:"libp2p"`
	PubSub           SyncPubSubStatus           `json:"pubsub"`
	TeamSync         SyncTeamSyncStatus         `json:"team_sync"`
	SyncActivity     SyncActivityStatus         `json:"sync_activity"`
	HistoryBootstrap SyncHistoryBootstrapStatus `json:"history_bootstrap"`
}

type SyncLibP2PStatus struct {
	Enabled                bool           `json:"enabled"`
	PeerID                 string         `json:"peer_id,omitempty"`
	ConfiguredListen       []string       `json:"configured_listen,omitempty"`
	ListenAddrs            []string       `json:"listen_addrs,omitempty"`
	DirectTransferEnabled  bool           `json:"direct_transfer_enabled"`
	TransferMaxSize        int64          `json:"transfer_max_size,omitempty"`
	AutoNATv2Enabled       bool           `json:"autonatv2_enabled,omitempty"`
	AutoRelayEnabled       bool           `json:"autorelay_enabled,omitempty"`
	HolePunchingEnabled    bool           `json:"hole_punching_enabled,omitempty"`
	ForcedReachability     string         `json:"forced_reachability,omitempty"`
	Reachability           string         `json:"reachability,omitempty"`
	ReachableAddrs         []string       `json:"reachable_addrs,omitempty"`
	LastReachableAddrsAt   *time.Time     `json:"last_reachable_addrs_at,omitempty"`
	ConfiguredPublicPeers  int            `json:"configured_public_peers,omitempty"`
	ConfiguredRelayPeers   int            `json:"configured_relay_peers,omitempty"`
	ResolvedRelayPeers     int            `json:"resolved_relay_peers,omitempty"`
	RelayReservationActive bool           `json:"relay_reservation_active,omitempty"`
	RelayReservationCount  int            `json:"relay_reservation_count,omitempty"`
	RelayReservationPeers  []string       `json:"relay_reservation_peers,omitempty"`
	RelayAddrs             []string       `json:"relay_addrs,omitempty"`
	LastReachabilityAt     *time.Time     `json:"last_reachability_at,omitempty"`
	LastRelayAt            *time.Time     `json:"last_relay_at,omitempty"`
	ConfiguredBootstrap    int            `json:"configured_bootstrap"`
	ConfiguredRendezvous   int            `json:"configured_rendezvous"`
	ConnectedBootstrap     int            `json:"connected_bootstrap"`
	ReachableBootstrap     int            `json:"reachable_bootstrap"`
	ConnectedPeers         int            `json:"connected_peers"`
	RoutingTablePeers      int            `json:"routing_table_peers"`
	MDNS                   SyncMDNSStatus `json:"mdns"`
	LastBootstrapAt        *time.Time     `json:"last_bootstrap_at,omitempty"`
	LastError              string         `json:"last_error,omitempty"`
	Peers                  []SyncPeerRef  `json:"peers,omitempty"`
}

type SyncMDNSStatus struct {
	Enabled          bool          `json:"enabled"`
	ServiceName      string        `json:"service_name,omitempty"`
	DiscoveredPeers  int           `json:"discovered_peers"`
	ConnectedPeers   int           `json:"connected_peers"`
	LastDiscoveredAt *time.Time    `json:"last_discovered_at,omitempty"`
	LastError        string        `json:"last_error,omitempty"`
	Peers            []SyncPeerRef `json:"peers,omitempty"`
}

type SyncPeerRef struct {
	PeerID    string `json:"peer_id"`
	Address   string `json:"address"`
	Connected bool   `json:"connected"`
	Reachable bool   `json:"reachable"`
	RTT       string `json:"rtt,omitempty"`
	Error     string `json:"error,omitempty"`
}

type SyncPubSubStatus struct {
	Enabled                          bool       `json:"enabled"`
	JoinedTopics                     []string   `json:"joined_topics,omitempty"`
	DiscoveryNamespaces              []string   `json:"discovery_namespaces,omitempty"`
	DiscoveryFeeds                   []string   `json:"discovery_feeds,omitempty"`
	DiscoveryTopics                  []string   `json:"discovery_topics,omitempty"`
	TopicWhitelist                   []string   `json:"topic_whitelist,omitempty"`
	TopicAliasPairs                  []string   `json:"topic_alias_pairs,omitempty"`
	AllowedOriginKeys                []string   `json:"allowed_origin_public_keys,omitempty"`
	BlockedOriginKeys                []string   `json:"blocked_origin_public_keys,omitempty"`
	AllowedParentKeys                []string   `json:"allowed_parent_public_keys,omitempty"`
	BlockedParentKeys                []string   `json:"blocked_parent_public_keys,omitempty"`
	LiveAllowedOriginKeys            []string   `json:"live_allowed_origin_public_keys,omitempty"`
	LiveBlockedOriginKeys            []string   `json:"live_blocked_origin_public_keys,omitempty"`
	LiveAllowedParentKeys            []string   `json:"live_allowed_parent_public_keys,omitempty"`
	LiveBlockedParentKeys            []string   `json:"live_blocked_parent_public_keys,omitempty"`
	LivePublicMutedOriginKeys        []string   `json:"live_public_muted_origin_public_keys,omitempty"`
	LivePublicMutedParentKeys        []string   `json:"live_public_muted_parent_public_keys,omitempty"`
	LivePublicRateLimitMessages      int        `json:"live_public_rate_limit_messages,omitempty"`
	LivePublicRateLimitWindowSeconds int        `json:"live_public_rate_limit_window_seconds,omitempty"`
	Published                        int        `json:"published"`
	Received                         int        `json:"received"`
	Enqueued                         int        `json:"enqueued"`
	CreditPublished                  int        `json:"credit_published"`
	CreditReceived                   int        `json:"credit_received"`
	CreditSaved                      int        `json:"credit_saved"`
	LastTopic                        string     `json:"last_topic,omitempty"`
	LastInfoHash                     string     `json:"last_infohash,omitempty"`
	LastPublishedAt                  *time.Time `json:"last_published_at,omitempty"`
	LastReceivedAt                   *time.Time `json:"last_received_at,omitempty"`
	LastCreditProofID                string     `json:"last_credit_proof_id,omitempty"`
	LastCreditAt                     *time.Time `json:"last_credit_at,omitempty"`
	LastError                        string     `json:"last_error,omitempty"`
}

type SyncTeamSyncStatus struct {
	Enabled                 bool       `json:"enabled"`
	NodeID                  string     `json:"node_id,omitempty"`
	StateLoaded             bool       `json:"state_loaded,omitempty"`
	StatePath               string     `json:"state_path,omitempty"`
	PersistedCursors        int        `json:"persisted_cursors,omitempty"`
	PersistedPeerAcks       int        `json:"persisted_peer_acks,omitempty"`
	AckPeers                int        `json:"ack_peers,omitempty"`
	Conflicts               int        `json:"conflicts,omitempty"`
	ResolvedConflicts       int        `json:"resolved_conflicts,omitempty"`
	PeerAckPrunes           int        `json:"peer_ack_prunes,omitempty"`
	ConflictPrunes          int        `json:"conflict_prunes,omitempty"`
	ExpiredPending          int        `json:"expired_pending,omitempty"`
	SupersededPending       int        `json:"superseded_pending,omitempty"`
	SubscribedTeams         int        `json:"subscribed_teams"`
	PrimedChannels          int        `json:"primed_channels"`
	PrimedHistoryTeams      int        `json:"primed_history_teams"`
	PrimedTaskTeams         int        `json:"primed_task_teams"`
	PrimedArtifactTeams     int        `json:"primed_artifact_teams"`
	PrimedMemberTeams       int        `json:"primed_member_teams"`
	PrimedPolicyTeams       int        `json:"primed_policy_teams"`
	PrimedConfigChannels    int        `json:"primed_config_channels"`
	ScannedMessages         int        `json:"scanned_messages"`
	ScannedHistory          int        `json:"scanned_history"`
	ScannedTasks            int        `json:"scanned_tasks"`
	ScannedArtifacts        int        `json:"scanned_artifacts"`
	ScannedMembers          int        `json:"scanned_members"`
	ScannedPolicies         int        `json:"scanned_policies"`
	ScannedConfigChannels   int        `json:"scanned_config_channels"`
	PublishedMessages       int        `json:"published_messages"`
	PublishedHistory        int        `json:"published_history"`
	PublishedTasks          int        `json:"published_tasks"`
	PublishedArtifacts      int        `json:"published_artifacts"`
	PublishedMembers        int        `json:"published_members"`
	PublishedPolicies       int        `json:"published_policies"`
	PublishedConfigChannels int        `json:"published_config_channels"`
	PublishedAcks           int        `json:"published_acks"`
	ReceivedMessages        int        `json:"received_messages"`
	ReceivedHistory         int        `json:"received_history"`
	ReceivedTasks           int        `json:"received_tasks"`
	ReceivedArtifacts       int        `json:"received_artifacts"`
	ReceivedMembers         int        `json:"received_members"`
	ReceivedPolicies        int        `json:"received_policies"`
	ReceivedConfigChannels  int        `json:"received_config_channels"`
	ReceivedAcks            int        `json:"received_acks"`
	AppliedMessages         int        `json:"applied_messages"`
	AppliedHistory          int        `json:"applied_history"`
	AppliedTasks            int        `json:"applied_tasks"`
	AppliedArtifacts        int        `json:"applied_artifacts"`
	AppliedMembers          int        `json:"applied_members"`
	AppliedPolicies         int        `json:"applied_policies"`
	AppliedConfigChannels   int        `json:"applied_config_channels"`
	AppliedAcks             int        `json:"applied_acks"`
	PendingAcks             int        `json:"pending_acks"`
	RetriedPublishes        int        `json:"retried_publishes"`
	SkippedMessages         int        `json:"skipped_messages"`
	SkippedHistory          int        `json:"skipped_history"`
	SkippedTasks            int        `json:"skipped_tasks"`
	SkippedArtifacts        int        `json:"skipped_artifacts"`
	SkippedMembers          int        `json:"skipped_members"`
	SkippedPolicies         int        `json:"skipped_policies"`
	SkippedConfigChannels   int        `json:"skipped_config_channels"`
	SkippedAcks             int        `json:"skipped_acks"`
	LastTeamID              string     `json:"last_team_id,omitempty"`
	LastPublishedKey        string     `json:"last_published_key,omitempty"`
	LastReceivedKey         string     `json:"last_received_key,omitempty"`
	LastAppliedKey          string     `json:"last_applied_key,omitempty"`
	LastAckedKey            string     `json:"last_acked_key,omitempty"`
	LastRetriedKey          string     `json:"last_retried_key,omitempty"`
	LastConflictKey         string     `json:"last_conflict_key,omitempty"`
	LastConflictReason      string     `json:"last_conflict_reason,omitempty"`
	LastPrunedAckPeer       string     `json:"last_pruned_ack_peer,omitempty"`
	LastPrunedAckKey        string     `json:"last_pruned_ack_key,omitempty"`
	LastPrunedConflictKey   string     `json:"last_pruned_conflict_key,omitempty"`
	LastPrunedConflictAt    *time.Time `json:"last_pruned_conflict_at,omitempty"`
	LastScannedChannelID    string     `json:"last_scanned_channel_id,omitempty"`
	LastScannedMessageID    string     `json:"last_scanned_message_id,omitempty"`
	LastScannedEventID      string     `json:"last_scanned_event_id,omitempty"`
	LastScannedTaskID       string     `json:"last_scanned_task_id,omitempty"`
	LastScannedArtifactID   string     `json:"last_scanned_artifact_id,omitempty"`
	LastPublishedAt         *time.Time `json:"last_published_at,omitempty"`
	LastReceivedAt          *time.Time `json:"last_received_at,omitempty"`
	LastAppliedAt           *time.Time `json:"last_applied_at,omitempty"`
	LastScannedAt           *time.Time `json:"last_scanned_at,omitempty"`
	LastSubscriptionAt      *time.Time `json:"last_subscription_at,omitempty"`
	LastSubscriptionTeam    string     `json:"last_subscription_team,omitempty"`
	LastStateWriteAt        *time.Time `json:"last_state_write_at,omitempty"`
	LastError               string     `json:"last_error,omitempty"`
}

type SyncActivityStatus struct {
	QueueRefs         int        `json:"queue_refs"`
	RealtimeQueueRefs int        `json:"realtime_queue_refs,omitempty"`
	HistoryQueueRefs  int        `json:"history_queue_refs,omitempty"`
	Imported          int        `json:"imported"`
	DirectImported    int        `json:"direct_imported"`
	Skipped           int        `json:"skipped"`
	Failed            int        `json:"failed"`
	LastRef           string     `json:"last_ref,omitempty"`
	LastInfoHash      string     `json:"last_infohash,omitempty"`
	LastStatus        string     `json:"last_status,omitempty"`
	LastTransport     string     `json:"last_transport,omitempty"`
	LastMessage       string     `json:"last_message,omitempty"`
	LastEventAt       *time.Time `json:"last_event_at,omitempty"`
}

type SyncHistoryBootstrapStatus struct {
	FirstSyncCompleted     bool       `json:"first_sync_completed"`
	Mode                   string     `json:"mode,omitempty"`
	LastHistoryBootstrapAt *time.Time `json:"last_history_bootstrap_at,omitempty"`
	RecentPagesLimit       int        `json:"recent_pages_limit,omitempty"`
	RecentRefsLimit        int        `json:"recent_refs_limit,omitempty"`
}

func syncStatusPath(store *Store) string {
	return filepath.Join(store.Root, "sync", "status.json")
}

func syncStatusRedisKey(rc *RedisClient) string {
	if rc == nil {
		return ""
	}
	return rc.Key("meta", "node_status")
}

func writeSyncStatus(store *Store, status SyncRuntimeStatus) error {
	path := syncStatusPath(store)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	status.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeSyncStatusCache(ctx context.Context, rc *RedisClient, status SyncRuntimeStatus) error {
	if rc == nil || !rc.Enabled() {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	status.UpdatedAt = time.Now().UTC()
	return rc.SetJSON(ctx, syncStatusRedisKey(rc), status, syncStatusRedisTTL)
}
