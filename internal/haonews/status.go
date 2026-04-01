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
