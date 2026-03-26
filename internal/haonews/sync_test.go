package haonews

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestParseSyncRefMagnet(t *testing.T) {
	t.Parallel()

	ref, err := ParseSyncRef("magnet:?xt=urn:btih:93a71a010a59022c8670e06e2c92fa279f98d974&dn=test")
	if err != nil {
		t.Fatalf("ParseSyncRef error = %v", err)
	}
	if ref.InfoHash != "93a71a010a59022c8670e06e2c92fa279f98d974" {
		t.Fatalf("infohash = %q", ref.InfoHash)
	}
}

func TestParseSyncRefInfoHash(t *testing.T) {
	t.Parallel()

	ref, err := ParseSyncRef("93a71a010a59022c8670e06e2c92fa279f98d974")
	if err != nil {
		t.Fatalf("ParseSyncRef error = %v", err)
	}
	if ref.Magnet != "magnet:?xt=urn:btih:93a71a010a59022c8670e06e2c92fa279f98d974" {
		t.Fatalf("magnet = %q", ref.Magnet)
	}
}

func TestCollectSyncRefsQueueAndDirect(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	queue := filepath.Join(root, "magnets.txt")
	data := "# comment\n93a71a010a59022c8670e06e2c92fa279f98d974\nmagnet:?xt=urn:btih:93a71a010a59022c8670e06e2c92fa279f98d974&dn=test\n"
	if err := os.WriteFile(queue, []byte(data), 0o644); err != nil {
		t.Fatalf("write queue: %v", err)
	}
	refs, err := collectSyncRefs([]string{"90498b9d42e081acee4165af5f5a2554b5276cbb"}, queue)
	if err != nil {
		t.Fatalf("collect refs: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("refs len = %d, want 2", len(refs))
	}
}

func TestLoadNetworkBootstrapConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "haonews_net.inf")
	content := `network_id=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
dht_router=router.bittorrent.com:6881
dht_router=router.utorrent.com:6881
libp2p_transfer_max_size=123456
lan_peer=192.168.102.74
lan_peer=192.168.102.76
lan_peer=192.168.102.75
lan_bt_peer=192.168.102.74
lan_bt_peer=192.168.102.76
lan_bt_peer=192.168.102.75
libp2p_bootstrap=/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write net config: %v", err)
	}
	cfg, err := LoadNetworkBootstrapConfig(path)
	if err != nil {
		t.Fatalf("load network config: %v", err)
	}
	if len(cfg.DHTRouters) != 2 {
		t.Fatalf("dht routers = %d, want 2", len(cfg.DHTRouters))
	}
	if len(cfg.LibP2PBootstrap) != 1 {
		t.Fatalf("libp2p peers = %d, want 1", len(cfg.LibP2PBootstrap))
	}
	if cfg.LibP2PTransferMaxSize != 123456 {
		t.Fatalf("libp2p transfer max size = %d, want 123456", cfg.LibP2PTransferMaxSize)
	}
	if cfg.NetworkID == "" {
		t.Fatal("expected network id to load")
	}
	if len(cfg.LANPeers) != 3 {
		t.Fatalf("lan peers = %d, want 3", len(cfg.LANPeers))
	}
	if len(cfg.LANTorrentPeers) != 3 {
		t.Fatalf("lan bt peers = %d, want 3", len(cfg.LANTorrentPeers))
	}
}

func TestLANBootstrapEndpointDefaultsToLatestPort(t *testing.T) {
	t.Parallel()

	value, err := lanBootstrapEndpoint("192.168.102.74")
	if err != nil {
		t.Fatalf("lanBootstrapEndpoint error = %v", err)
	}
	if value != "http://192.168.102.74:51818/api/network/bootstrap" {
		t.Fatalf("endpoint = %q", value)
	}
}

func TestLANHistoryManifestEndpointDefaultsToLatestPort(t *testing.T) {
	t.Parallel()

	value, err := lanHistoryManifestEndpoint("192.168.102.74")
	if err != nil {
		t.Fatalf("lanHistoryManifestEndpoint error = %v", err)
	}
	if value != "http://192.168.102.74:51818/api/history/list" {
		t.Fatalf("endpoint = %q", value)
	}
}

func TestRemoveSyncRef(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	queue := filepath.Join(root, "magnets.txt")
	content := "# magnet:?xt=urn:btih:...\nmagnet:?xt=urn:btih:93a71a010a59022c8670e06e2c92fa279f98d974&dn=test\n"
	if err := os.WriteFile(queue, []byte(content), 0o644); err != nil {
		t.Fatalf("write queue: %v", err)
	}
	ref, err := ParseSyncRef("93a71a010a59022c8670e06e2c92fa279f98d974")
	if err != nil {
		t.Fatalf("parse ref: %v", err)
	}
	if err := removeSyncRef(queue, ref); err != nil {
		t.Fatalf("remove ref: %v", err)
	}
	data, err := os.ReadFile(queue)
	if err != nil {
		t.Fatalf("read queue: %v", err)
	}
	if string(data) != "# magnet:?xt=urn:btih:...\n" {
		t.Fatalf("queue contents = %q", string(data))
	}
}

func TestSanitizeSyncQueueFileRemovesDirtyPeerHints(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	queue := filepath.Join(root, "magnets.txt")
	content := "# magnet:?xt=urn:btih:...\nmagnet:?xt=urn:btih:93a71a010a59022c8670e06e2c92fa279f98d974&x.pe=192.168.102.74:55369&x.pe=100.168.102.74:55369\n"
	if err := os.WriteFile(queue, []byte(content), 0o644); err != nil {
		t.Fatalf("write queue: %v", err)
	}
	changed, err := sanitizeSyncQueueFile(queue, []string{"192.168.102.74"})
	if err != nil {
		t.Fatalf("sanitize queue: %v", err)
	}
	if changed != 1 {
		t.Fatalf("changed = %d, want 1", changed)
	}
	data, err := os.ReadFile(queue)
	if err != nil {
		t.Fatalf("read queue: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "x.pe=192.168.102.74%3A55369") {
		t.Fatalf("queue missing LAN peer: %q", text)
	}
	if strings.Contains(text, "100.168.102.74") {
		t.Fatalf("queue still contains dirty x.pe: %q", text)
	}
}

func TestResolveEffectiveDHTRoutersPrefersLANBTAnchors(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/network/bootstrap" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(lanBootstrapResponse{
			NetworkID:       latestOrgNetworkID,
			BitTorrentNodes: []string{"192.168.102.74:53396"},
		})
	}))
	defer srv.Close()

	cfg := NetworkBootstrapConfig{
		NetworkID:       latestOrgNetworkID,
		LANTorrentPeers: []string{srv.URL},
		DHTRouters:      []string{"router.bittorrent.com:6881", "router.utorrent.com:6881"},
	}
	routers, err := resolveEffectiveDHTRouters(context.Background(), cfg)
	if err != nil {
		t.Fatalf("resolveEffectiveDHTRouters error = %v", err)
	}
	if len(routers) < 3 {
		t.Fatalf("routers = %v, want LAN node plus public routers", routers)
	}
	if routers[0] != "192.168.102.74:53396" {
		t.Fatalf("first router = %q, want LAN BT node first", routers[0])
	}
}

func TestProbeLANAnchorsWritesHealthCache(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	netPath := filepath.Join(root, "hao_news_net.inf")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/network/bootstrap" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(lanBootstrapResponse{
			NetworkID:       latestOrgNetworkID,
			PeerID:          "QmTestPeer",
			DialAddrs:       []string{"/ip4/192.168.102.75/tcp/50584"},
			BitTorrentNodes: []string{"192.168.102.75:50585"},
		})
	}))
	defer srv.Close()

	if err := os.WriteFile(netPath, []byte("network_id="+latestOrgNetworkID+"\n"), 0o644); err != nil {
		t.Fatalf("write net config: %v", err)
	}

	runtime := &syncRuntime{
		netCfg: NetworkBootstrapConfig{
			Path:            netPath,
			NetworkID:       latestOrgNetworkID,
			LANPeers:        []string{srv.URL},
			LANTorrentPeers: []string{srv.URL},
		},
	}
	if err := runtime.probeLANAnchors(context.Background(), nil); err != nil {
		t.Fatalf("probeLANAnchors() error = %v", err)
	}

	cache, err := loadLANPeerHealthCache(runtime.netCfg)
	if err != nil {
		t.Fatalf("loadLANPeerHealthCache() error = %v", err)
	}
	if cache.entry("lan_peer", srv.URL).LastSuccessAt.IsZero() {
		t.Fatal("expected lan_peer success to be cached")
	}
}

func TestLoadTrackerListParsesDefaultStyleFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "Trackerlist.inf")
	content := "# comment\ntracker=https://tracker.example.com/announce\nudp://tracker.opentrackr.org:1337/announce\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write tracker list: %v", err)
	}
	trackers, err := LoadTrackerList(path)
	if err != nil {
		t.Fatalf("LoadTrackerList error = %v", err)
	}
	if len(trackers) != 2 {
		t.Fatalf("trackers len = %d, want 2", len(trackers))
	}
	if trackers[0] != "https://tracker.example.com/announce" {
		t.Fatalf("first tracker = %q", trackers[0])
	}
}

func TestHasCompleteLocalBundleRequiresBundleFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	result, err := PublishMessage(store, MessageInput{
		Kind:    "post",
		Author:  "agent://test/main",
		Title:   "bundle completeness",
		Body:    "body",
		Channel: "latest.org/world",
		Extensions: map[string]any{
			"project": "latest.org",
		},
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !hasCompleteLocalBundle(store, result.InfoHash) {
		t.Fatalf("expected complete local bundle")
	}
	mi, err := metainfo.LoadFromFile(store.TorrentPath(result.InfoHash))
	if err != nil {
		t.Fatalf("load torrent: %v", err)
	}
	info, err := mi.UnmarshalInfo()
	if err != nil {
		t.Fatalf("unmarshal info: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(store.DataDir, info.BestName())); err != nil {
		t.Fatalf("remove content dir: %v", err)
	}
	if hasCompleteLocalBundle(store, result.InfoHash) {
		t.Fatalf("expected incomplete bundle after deleting content dir")
	}
}

func TestHandleAnnouncementRemembersDirectPeer(t *testing.T) {
	t.Parallel()

	queue := filepath.Join(t.TempDir(), "magnets.txt")
	store, err := OpenStore(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	host, err := newTestLibP2PHost(context.Background())
	if err != nil {
		t.Fatalf("newTestLibP2PHost error = %v", err)
	}
	defer host.Close()

	runtime := &syncRuntime{
		store:         store,
		queuePath:     queue,
		netCfg:        NetworkBootstrapConfig{NetworkID: latestOrgNetworkID},
		directPeers:   make(map[string][]peer.ID),
		subscriptions: SyncSubscriptions{},
	}
	enqueued, err := runtime.handleAnnouncement(SyncAnnouncement{
		InfoHash:     "93a71a010a59022c8670e06e2c92fa279f98d974",
		Magnet:       "magnet:?xt=urn:btih:93a71a010a59022c8670e06e2c92fa279f98d974&dn=test",
		NetworkID:    latestOrgNetworkID,
		LibP2PPeerID: host.ID().String(),
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("handleAnnouncement error = %v", err)
	}
	if !enqueued {
		t.Fatal("expected announcement to enqueue")
	}
	peers := runtime.directPeerIDs("93a71a010a59022c8670e06e2c92fa279f98d974")
	if len(peers) != 1 || peers[0] != host.ID() {
		t.Fatalf("direct peers = %#v, want %s", peers, host.ID())
	}
}

func TestSyncRefImportsViaLibP2PDirectTransfer(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	providerHost, err := newTestLibP2PHost(ctx)
	if err != nil {
		t.Fatalf("newTestLibP2PHost(provider) error = %v", err)
	}
	defer providerHost.Close()
	requesterHost, err := newTestLibP2PHost(ctx)
	if err != nil {
		t.Fatalf("newTestLibP2PHost(requester) error = %v", err)
	}
	defer requesterHost.Close()

	addrInfo := peer.AddrInfo{ID: providerHost.ID(), Addrs: providerHost.Addrs()}
	if err := requesterHost.Connect(ctx, addrInfo); err != nil {
		t.Fatalf("requesterHost.Connect() error = %v", err)
	}

	providerStore, err := OpenStore(filepath.Join(t.TempDir(), "provider"))
	if err != nil {
		t.Fatalf("OpenStore(provider) error = %v", err)
	}
	requesterStore, err := OpenStore(filepath.Join(t.TempDir(), "requester"))
	if err != nil {
		t.Fatalf("OpenStore(requester) error = %v", err)
	}
	published, err := PublishMessage(providerStore, MessageInput{
		Kind:    "post",
		Author:  "agent://test/main",
		Title:   "direct-transfer",
		Body:    "libp2p first",
		Channel: "latest.org/world",
		Extensions: map[string]any{
			"project":    "latest.org",
			"network_id": latestOrgNetworkID,
		},
	})
	if err != nil {
		t.Fatalf("PublishMessage error = %v", err)
	}
	provider := newBundleTransferProvider(providerHost, providerStore, defaultLibP2PTransferMaxSize)
	defer provider.Close()

	result := syncRef(
		ctx,
		nil,
		requesterStore,
		SyncRef{Raw: published.Magnet, Magnet: published.Magnet, InfoHash: published.InfoHash},
		5*time.Second,
		nil,
		nil,
		SyncSubscriptions{},
		true,
		&libp2pRuntime{host: requesterHost, transferMaxSize: defaultLibP2PTransferMaxSize},
		[]peer.ID{providerHost.ID()},
	)
	if result.Status != "imported" {
		t.Fatalf("status = %q, want imported (%s)", result.Status, result.Message)
	}
	if result.Transport != "libp2p" {
		t.Fatalf("transport = %q, want libp2p", result.Transport)
	}
	if !hasCompleteLocalBundle(requesterStore, published.InfoHash) {
		t.Fatal("expected transferred bundle to exist locally")
	}
	if _, err := requesterStore.ExistingTorrentPath(published.InfoHash); err != nil {
		t.Fatalf("ExistingTorrentPath() error = %v", err)
	}
}
