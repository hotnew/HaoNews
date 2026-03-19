package aip2p

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type SyncRuntimeStatus struct {
	UpdatedAt     time.Time            `json:"updated_at"`
	StartedAt     time.Time            `json:"started_at"`
	PID           int                  `json:"pid"`
	StoreRoot     string               `json:"store_root"`
	QueuePath     string               `json:"queue_path"`
	Mode          string               `json:"mode"`
	Seed          bool                 `json:"seed"`
	NetworkID     string               `json:"network_id"`
	LibP2P        SyncLibP2PStatus     `json:"libp2p"`
	BitTorrentDHT SyncBitTorrentStatus `json:"bittorrent_dht"`
	PubSub        SyncPubSubStatus     `json:"pubsub"`
	SyncActivity  SyncActivityStatus   `json:"sync_activity"`
}

type SyncLibP2PStatus struct {
	Enabled              bool           `json:"enabled"`
	PeerID               string         `json:"peer_id,omitempty"`
	ListenAddrs          []string       `json:"listen_addrs,omitempty"`
	ConfiguredBootstrap  int            `json:"configured_bootstrap"`
	ConfiguredRendezvous int            `json:"configured_rendezvous"`
	ConnectedBootstrap   int            `json:"connected_bootstrap"`
	ReachableBootstrap   int            `json:"reachable_bootstrap"`
	ConnectedPeers       int            `json:"connected_peers"`
	RoutingTablePeers    int            `json:"routing_table_peers"`
	MDNS                 SyncMDNSStatus `json:"mdns"`
	LastBootstrapAt      *time.Time     `json:"last_bootstrap_at,omitempty"`
	LastError            string         `json:"last_error,omitempty"`
	Peers                []SyncPeerRef  `json:"peers,omitempty"`
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

type SyncBitTorrentStatus struct {
	Enabled                 bool   `json:"enabled"`
	ConfiguredRouters       int    `json:"configured_routers"`
	Servers                 int    `json:"servers"`
	GoodNodes               int    `json:"good_nodes"`
	Nodes                   int    `json:"nodes"`
	OutstandingTransactions int    `json:"outstanding_transactions"`
	LastError               string `json:"last_error,omitempty"`
}

type SyncPubSubStatus struct {
	Enabled             bool       `json:"enabled"`
	JoinedTopics        []string   `json:"joined_topics,omitempty"`
	DiscoveryNamespaces []string   `json:"discovery_namespaces,omitempty"`
	Published           int        `json:"published"`
	Received            int        `json:"received"`
	Enqueued            int        `json:"enqueued"`
	CreditPublished     int        `json:"credit_published"`
	CreditReceived      int        `json:"credit_received"`
	CreditSaved         int        `json:"credit_saved"`
	LastTopic           string     `json:"last_topic,omitempty"`
	LastInfoHash        string     `json:"last_infohash,omitempty"`
	LastPublishedAt     *time.Time `json:"last_published_at,omitempty"`
	LastReceivedAt      *time.Time `json:"last_received_at,omitempty"`
	LastCreditProofID   string     `json:"last_credit_proof_id,omitempty"`
	LastCreditAt        *time.Time `json:"last_credit_at,omitempty"`
	LastError           string     `json:"last_error,omitempty"`
}

type SyncActivityStatus struct {
	QueueRefs    int        `json:"queue_refs"`
	Imported     int        `json:"imported"`
	Skipped      int        `json:"skipped"`
	Failed       int        `json:"failed"`
	LastRef      string     `json:"last_ref,omitempty"`
	LastInfoHash string     `json:"last_infohash,omitempty"`
	LastStatus   string     `json:"last_status,omitempty"`
	LastMessage  string     `json:"last_message,omitempty"`
	LastEventAt  *time.Time `json:"last_event_at,omitempty"`
}

func syncStatusPath(store *Store) string {
	return filepath.Join(store.Root, "sync", "status.json")
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
