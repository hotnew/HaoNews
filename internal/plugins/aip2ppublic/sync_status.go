package newsplugin

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
	PeerID               string         `json:"peer_id"`
	ConfiguredListen     []string       `json:"configured_listen"`
	ListenAddrs          []string       `json:"listen_addrs"`
	ConfiguredBootstrap  int            `json:"configured_bootstrap"`
	ConfiguredRendezvous int            `json:"configured_rendezvous"`
	ConnectedBootstrap   int            `json:"connected_bootstrap"`
	ReachableBootstrap   int            `json:"reachable_bootstrap"`
	ConnectedPeers       int            `json:"connected_peers"`
	RoutingTablePeers    int            `json:"routing_table_peers"`
	MDNS                 SyncMDNSStatus `json:"mdns"`
	LastBootstrapAt      *time.Time     `json:"last_bootstrap_at"`
	LastError            string         `json:"last_error"`
	Peers                []SyncPeerRef  `json:"peers"`
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

type SyncBitTorrentStatus struct {
	Enabled                 bool     `json:"enabled"`
	ConfiguredListen        string   `json:"configured_listen"`
	ListenAddrs             []string `json:"listen_addrs"`
	ConfiguredRouters       int      `json:"configured_routers"`
	Servers                 int      `json:"servers"`
	GoodNodes               int      `json:"good_nodes"`
	Nodes                   int      `json:"nodes"`
	OutstandingTransactions int      `json:"outstanding_transactions"`
	LastError               string   `json:"last_error"`
}

type SyncPubSubStatus struct {
	Enabled             bool       `json:"enabled"`
	JoinedTopics        []string   `json:"joined_topics"`
	DiscoveryNamespaces []string   `json:"discovery_namespaces"`
	Published           int        `json:"published"`
	Received            int        `json:"received"`
	Enqueued            int        `json:"enqueued"`
	LastTopic           string     `json:"last_topic"`
	LastInfoHash        string     `json:"last_infohash"`
	LastPublishedAt     *time.Time `json:"last_published_at"`
	LastReceivedAt      *time.Time `json:"last_received_at"`
	LastError           string     `json:"last_error"`
}

type SyncActivityStatus struct {
	QueueRefs    int        `json:"queue_refs"`
	Imported     int        `json:"imported"`
	Skipped      int        `json:"skipped"`
	Failed       int        `json:"failed"`
	LastRef      string     `json:"last_ref"`
	LastInfoHash string     `json:"last_infohash"`
	LastStatus   string     `json:"last_status"`
	LastMessage  string     `json:"last_message"`
	LastEventAt  *time.Time `json:"last_event_at"`
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
