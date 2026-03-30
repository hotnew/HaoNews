package newsplugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

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
	PeerID                 string         `json:"peer_id"`
	ConfiguredListen       []string       `json:"configured_listen"`
	ListenAddrs            []string       `json:"listen_addrs"`
	AutoNATv2Enabled       bool           `json:"autonatv2_enabled"`
	AutoRelayEnabled       bool           `json:"autorelay_enabled"`
	HolePunchingEnabled    bool           `json:"hole_punching_enabled"`
	ForcedReachability     string         `json:"forced_reachability"`
	Reachability           string         `json:"reachability"`
	ReachableAddrs         []string       `json:"reachable_addrs"`
	LastReachableAddrsAt   *time.Time     `json:"last_reachable_addrs_at"`
	ConfiguredPublicPeers  int            `json:"configured_public_peers"`
	ConfiguredRelayPeers   int            `json:"configured_relay_peers"`
	ResolvedRelayPeers     int            `json:"resolved_relay_peers"`
	RelayReservationActive bool           `json:"relay_reservation_active"`
	RelayReservationCount  int            `json:"relay_reservation_count"`
	RelayReservationPeers  []string       `json:"relay_reservation_peers"`
	RelayAddrs             []string       `json:"relay_addrs"`
	LastReachabilityAt     *time.Time     `json:"last_reachability_at"`
	LastRelayAt            *time.Time     `json:"last_relay_at"`
	ConfiguredBootstrap    int            `json:"configured_bootstrap"`
	ConfiguredRendezvous   int            `json:"configured_rendezvous"`
	ConnectedBootstrap     int            `json:"connected_bootstrap"`
	ReachableBootstrap     int            `json:"reachable_bootstrap"`
	ConnectedPeers         int            `json:"connected_peers"`
	RoutingTablePeers      int            `json:"routing_table_peers"`
	MDNS                   SyncMDNSStatus `json:"mdns"`
	LastBootstrapAt        *time.Time     `json:"last_bootstrap_at"`
	LastError              string         `json:"last_error"`
	Peers                  []SyncPeerRef  `json:"peers"`
}

type SyncMDNSStatus struct {
	Enabled          bool          `json:"enabled"`
	ServiceName      string        `json:"service_name"`
	DiscoveredPeers  int           `json:"discovered_peers"`
	ConnectedPeers   int           `json:"connected_peers"`
	LastDiscoveredAt *time.Time    `json:"last_discovered_at"`
	LastError        string        `json:"last_error"`
	Peers            []SyncPeerRef `json:"peers"`
}

type SyncPeerRef struct {
	PeerID    string `json:"peer_id"`
	Address   string `json:"address"`
	Connected bool   `json:"connected"`
	Reachable bool   `json:"reachable"`
	RTT       string `json:"rtt"`
	Error     string `json:"error"`
}

type SyncPubSubStatus struct {
	Enabled                          bool       `json:"enabled"`
	JoinedTopics                     []string   `json:"joined_topics"`
	DiscoveryNamespaces              []string   `json:"discovery_namespaces"`
	DiscoveryFeeds                   []string   `json:"discovery_feeds"`
	DiscoveryTopics                  []string   `json:"discovery_topics"`
	TopicWhitelist                   []string   `json:"topic_whitelist"`
	TopicAliasPairs                  []string   `json:"topic_alias_pairs"`
	AllowedOriginKeys                []string   `json:"allowed_origin_public_keys"`
	BlockedOriginKeys                []string   `json:"blocked_origin_public_keys"`
	AllowedParentKeys                []string   `json:"allowed_parent_public_keys"`
	BlockedParentKeys                []string   `json:"blocked_parent_public_keys"`
	LiveAllowedOriginKeys            []string   `json:"live_allowed_origin_public_keys"`
	LiveBlockedOriginKeys            []string   `json:"live_blocked_origin_public_keys"`
	LiveAllowedParentKeys            []string   `json:"live_allowed_parent_public_keys"`
	LiveBlockedParentKeys            []string   `json:"live_blocked_parent_public_keys"`
	LivePublicMutedOriginKeys        []string   `json:"live_public_muted_origin_public_keys"`
	LivePublicMutedParentKeys        []string   `json:"live_public_muted_parent_public_keys"`
	LivePublicRateLimitMessages      int        `json:"live_public_rate_limit_messages"`
	LivePublicRateLimitWindowSeconds int        `json:"live_public_rate_limit_window_seconds"`
	Published                        int        `json:"published"`
	Received                         int        `json:"received"`
	Enqueued                         int        `json:"enqueued"`
	LastTopic                        string     `json:"last_topic"`
	LastInfoHash                     string     `json:"last_infohash"`
	LastPublishedAt                  *time.Time `json:"last_published_at"`
	LastReceivedAt                   *time.Time `json:"last_received_at"`
	LastError                        string     `json:"last_error"`
}

type SyncActivityStatus struct {
	QueueRefs         int        `json:"queue_refs"`
	RealtimeQueueRefs int        `json:"realtime_queue_refs"`
	HistoryQueueRefs  int        `json:"history_queue_refs"`
	Imported          int        `json:"imported"`
	Skipped           int        `json:"skipped"`
	Failed            int        `json:"failed"`
	LastRef           string     `json:"last_ref"`
	LastInfoHash      string     `json:"last_infohash"`
	LastStatus        string     `json:"last_status"`
	LastMessage       string     `json:"last_message"`
	LastEventAt       *time.Time `json:"last_event_at"`
}

type SyncHistoryBootstrapStatus struct {
	FirstSyncCompleted     bool       `json:"first_sync_completed"`
	Mode                   string     `json:"mode"`
	LastHistoryBootstrapAt *time.Time `json:"last_history_bootstrap_at"`
	RecentPagesLimit       int        `json:"recent_pages_limit"`
	RecentRefsLimit        int        `json:"recent_refs_limit"`
}

func loadSyncRuntimeStatus(storeRoot string) (SyncRuntimeStatus, error) {
	path := filepath.Join(storeRoot, "sync", "status.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return SyncRuntimeStatus{}, nil
		}
		return SyncRuntimeStatus{}, err
	}
	var status SyncRuntimeStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return SyncRuntimeStatus{}, err
	}
	return status, nil
}
