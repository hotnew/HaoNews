package haonews

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/libp2p/go-libp2p/core/crypto"
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
	if ref.Magnet != "haonews-sync://bundle/93a71a010a59022c8670e06e2c92fa279f98d974?dn=test" {
		t.Fatalf("ref = %q", ref.Magnet)
	}
}

func TestParseSyncRefMagnetPreservesDirectPeerHint(t *testing.T) {
	t.Parallel()

	ref, err := ParseSyncRef("magnet:?xt=urn:btih:93a71a010a59022c8670e06e2c92fa279f98d974&dn=test&x.hn.peer=12D3KooWPeerHint")
	if err != nil {
		t.Fatalf("ParseSyncRef error = %v", err)
	}
	if ref.DirectPeerHint != "12D3KooWPeerHint" {
		t.Fatalf("direct peer hint = %q", ref.DirectPeerHint)
	}
	if !strings.Contains(ref.Magnet, "peer=12D3KooWPeerHint") {
		t.Fatalf("ref missing peer hint: %q", ref.Magnet)
	}
}

func TestParseSyncRefInfoHash(t *testing.T) {
	t.Parallel()

	ref, err := ParseSyncRef("93a71a010a59022c8670e06e2c92fa279f98d974")
	if err != nil {
		t.Fatalf("ParseSyncRef error = %v", err)
	}
	if ref.Magnet != "haonews-sync://bundle/93a71a010a59022c8670e06e2c92fa279f98d974" {
		t.Fatalf("ref = %q", ref.Magnet)
	}
}

func TestSanitizeQueuedSyncRefUpgradesLegacyMagnetToNewRef(t *testing.T) {
	t.Parallel()

	raw := "magnet:?xt=urn:btih:93a71a010a59022c8670e06e2c92fa279f98d974&dn=test&x.hn.peer=12D3KooWPeerHint"
	got, changed, err := sanitizeQueuedSyncRef(raw, nil)
	if err != nil {
		t.Fatalf("sanitizeQueuedSyncRef error = %v", err)
	}
	if !changed {
		t.Fatalf("expected queue ref to be rewritten")
	}
	if got != "haonews-sync://bundle/93a71a010a59022c8670e06e2c92fa279f98d974?dn=test&peer=12D3KooWPeerHint" {
		t.Fatalf("sanitized ref = %q", got)
	}
}

func TestMigrateHistoryManifestQueueRefsMovesToHistoryQueue(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	realtimePath := filepath.Join(root, "realtime.txt")
	historyPath := filepath.Join(root, "history.txt")
	realtime := strings.Join([]string{
		"haonews-sync://bundle/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa?dn=20260327T080000Z-hao.news-history-manifest&peer=12D3KooWHist",
		"haonews-sync://bundle/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb?dn=test-post&peer=12D3KooWPost",
		"",
	}, "\n")
	if err := os.WriteFile(realtimePath, []byte(realtime), 0o644); err != nil {
		t.Fatalf("write realtime queue: %v", err)
	}
	if err := os.WriteFile(historyPath, nil, 0o644); err != nil {
		t.Fatalf("write history queue: %v", err)
	}
	moved, err := migrateHistoryManifestQueueRefs(realtimePath, historyPath)
	if err != nil {
		t.Fatalf("migrateHistoryManifestQueueRefs error = %v", err)
	}
	if moved != 1 {
		t.Fatalf("moved = %d, want 1", moved)
	}
	realtimeData, err := os.ReadFile(realtimePath)
	if err != nil {
		t.Fatalf("read realtime queue: %v", err)
	}
	if strings.Contains(string(realtimeData), "history-manifest") {
		t.Fatalf("realtime queue still contains history manifest: %q", string(realtimeData))
	}
	historyData, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("read history queue: %v", err)
	}
	if !strings.Contains(string(historyData), "history-manifest") {
		t.Fatalf("history queue missing migrated history manifest: %q", string(historyData))
	}
}

func TestRememberDirectPeerCapsPeerListPerInfoHash(t *testing.T) {
	t.Parallel()

	runtime := &syncRuntime{directPeers: make(map[string][]peer.ID)}
	const infoHash = "93a71a010a59022c8670e06e2c92fa279f98d974"
	for i := 0; i < maxDirectPeersPerInfoHash+3; i++ {
		priv, _, err := crypto.GenerateEd25519Key(nil)
		if err != nil {
			t.Fatalf("GenerateEd25519Key(%d) error = %v", i, err)
		}
		id, err := peer.IDFromPrivateKey(priv)
		if err != nil {
			t.Fatalf("IDFromPrivateKey(%d) error = %v", i, err)
		}
		runtime.rememberDirectPeer(infoHash, id.String())
	}

	got := runtime.directPeerIDs(infoHash)
	if len(got) != maxDirectPeersPerInfoHash {
		t.Fatalf("len(directPeerIDs) = %d, want %d", len(got), maxDirectPeersPerInfoHash)
	}
}

func TestCollectSyncRefsQueueAndDirect(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	realtimeQueue := filepath.Join(root, "realtime.txt")
	historyQueue := filepath.Join(root, "history.txt")
	if err := os.WriteFile(realtimeQueue, []byte("# comment\n93a71a010a59022c8670e06e2c92fa279f98d974\n"), 0o644); err != nil {
		t.Fatalf("write realtime queue: %v", err)
	}
	if err := os.WriteFile(historyQueue, []byte("magnet:?xt=urn:btih:93a71a010a59022c8670e06e2c92fa279f98d974&dn=test\nmagnet:?xt=urn:btih:1111111111111111111111111111111111111111&dn=history-manifest\n"), 0o644); err != nil {
		t.Fatalf("write history queue: %v", err)
	}
	realtimeRefs, historyRefs, err := collectSyncRefs([]string{"90498b9d42e081acee4165af5f5a2554b5276cbb"}, realtimeQueue, historyQueue)
	if err != nil {
		t.Fatalf("collect refs: %v", err)
	}
	if len(realtimeRefs) != 2 {
		t.Fatalf("realtime refs len = %d, want 2", len(realtimeRefs))
	}
	if len(historyRefs) != 1 {
		t.Fatalf("history refs len = %d, want 1", len(historyRefs))
	}
	if historyRefs[0].Queue != historyQueue {
		t.Fatalf("history queue path = %q, want %q", historyRefs[0].Queue, historyQueue)
	}
}

func TestEnsureSyncLayoutMigratesLegacyQueueToHistory(t *testing.T) {
	t.Parallel()

	store, err := OpenStore(filepath.Join(t.TempDir(), ".haonews"))
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	legacyPath := filepath.Join(store.Root, "sync", "magnets.txt")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("mkdir sync dir: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte("magnet:?xt=urn:btih:93a71a010a59022c8670e06e2c92fa279f98d974&dn=test\n"), 0o644); err != nil {
		t.Fatalf("write legacy queue: %v", err)
	}
	layout, err := ensureSyncLayout(store, "")
	if err != nil {
		t.Fatalf("ensureSyncLayout error = %v", err)
	}
	historyData, err := os.ReadFile(layout.HistoryPath)
	if err != nil {
		t.Fatalf("read history queue: %v", err)
	}
	if !strings.Contains(string(historyData), "93a71a010a59022c8670e06e2c92fa279f98d974") {
		t.Fatalf("history queue missing migrated ref: %q", string(historyData))
	}
	legacyData, err := os.ReadFile(layout.LegacyPath)
	if err != nil {
		t.Fatalf("read legacy queue: %v", err)
	}
	if strings.Contains(string(legacyData), "93a71a010a59022c8670e06e2c92fa279f98d974") {
		t.Fatalf("legacy queue still contains migrated ref: %q", string(legacyData))
	}
}

func TestLoadNetworkBootstrapConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "haonews_net.inf")
	content := `network_mode=shared
libp2p_transfer_max_size=123456
lan_peer=192.168.102.74
lan_peer=192.168.102.76
lan_peer=192.168.102.75
public_peer=ai.jie.news
relay_peer=relay.jie.news
libp2p_bootstrap=/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write net config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, networkIDFileName), []byte("network_id=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n"), 0o644); err != nil {
		t.Fatalf("write network_id.inf: %v", err)
	}
	cfg, err := LoadNetworkBootstrapConfig(path)
	if err != nil {
		t.Fatalf("load network config: %v", err)
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
	if cfg.NetworkMode != networkModeShared {
		t.Fatalf("network mode = %q, want shared", cfg.NetworkMode)
	}
	if len(cfg.LANPeers) != 3 {
		t.Fatalf("lan peers = %d, want 3", len(cfg.LANPeers))
	}
	if got := len(cfg.PublicPeers); got != 1 || cfg.PublicPeers[0] != "ai.jie.news" {
		t.Fatalf("public peers = %#v", cfg.PublicPeers)
	}
	if got := len(cfg.RelayPeers); got != 1 || cfg.RelayPeers[0] != "relay.jie.news" {
		t.Fatalf("relay peers = %#v", cfg.RelayPeers)
	}
	if !cfg.AllowsLANDiscovery() {
		t.Fatal("shared mode should allow LAN discovery")
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

	value, err := lanHistoryManifestEndpoint("192.168.102.74", "")
	if err != nil {
		t.Fatalf("lanHistoryManifestEndpoint error = %v", err)
	}
	if value != "http://192.168.102.74:51818/api/history/list" {
		t.Fatalf("endpoint = %q", value)
	}
}

func TestLANHistoryManifestEndpointIncludesCursor(t *testing.T) {
	t.Parallel()

	value, err := lanHistoryManifestEndpoint("192.168.102.74", "2")
	if err != nil {
		t.Fatalf("lanHistoryManifestEndpoint error = %v", err)
	}
	if value != "http://192.168.102.74:51818/api/history/list?cursor=2" {
		t.Fatalf("endpoint = %q", value)
	}
}

func TestPublicBootstrapEndpointDefaultsToHTTPS(t *testing.T) {
	t.Parallel()

	value, err := lanBootstrapEndpoint("ai.jie.news")
	if err != nil {
		t.Fatalf("lanBootstrapEndpoint error = %v", err)
	}
	if value != "https://ai.jie.news/api/network/bootstrap" {
		t.Fatalf("endpoint = %q", value)
	}
}

func TestPublicHistoryManifestEndpointDefaultsToHTTPS(t *testing.T) {
	t.Parallel()

	value, err := lanHistoryManifestEndpoint("ai.jie.news", "2")
	if err != nil {
		t.Fatalf("lanHistoryManifestEndpoint error = %v", err)
	}
	if value != "https://ai.jie.news/api/history/list?cursor=2" {
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

func TestSanitizeSyncQueueFileRemovesPrivatePeerHintsWhenOnlyPublicPeersConfigured(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	queue := filepath.Join(root, "history.txt")
	content := "magnet:?xt=urn:btih:93a71a010a59022c8670e06e2c92fa279f98d974&x.pe=192.168.102.75:50585\n"
	if err := os.WriteFile(queue, []byte(content), 0o644); err != nil {
		t.Fatalf("write queue: %v", err)
	}
	changed, err := sanitizeSyncQueueFile(queue, []string{"ai.jie.news"})
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
	if strings.Contains(text, "192.168.102.75") {
		t.Fatalf("queue still contains private x.pe: %q", text)
	}
	ref, err := ParseSyncRef(strings.TrimSpace(text))
	if err != nil {
		t.Fatalf("parse sanitized queue ref: %v", err)
	}
	if ref.InfoHash != "93a71a010a59022c8670e06e2c92fa279f98d974" {
		t.Fatalf("sanitized queue infohash = %q", ref.InfoHash)
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
			NetworkID: latestOrgNetworkID,
			PeerID:    "QmTestPeer",
			DialAddrs: []string{"/ip4/192.168.102.75/tcp/50584"},
		})
	}))
	defer srv.Close()

	if err := os.WriteFile(netPath, []byte("network_id="+latestOrgNetworkID+"\n"), 0o644); err != nil {
		t.Fatalf("write net config: %v", err)
	}

	runtime := &syncRuntime{
		netCfg: NetworkBootstrapConfig{
			Path:      netPath,
			NetworkID: latestOrgNetworkID,
			LANPeers:  []string{srv.URL},
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

func TestEnqueueHistoryFromLANPeersRecentBootstrapLimitsPages(t *testing.T) {
	t.Parallel()

	store, err := OpenStore(filepath.Join(t.TempDir(), ".haonews"))
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	queues, err := ensureSyncLayout(store, "")
	if err != nil {
		t.Fatalf("ensureSyncLayout error = %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
		page := 1
		if cursor != "" {
			switch cursor {
			case "2":
				page = 2
			case "3":
				page = 3
			case "4":
				page = 4
			}
		}
		infohash := strings.Repeat(strconv.Itoa(page), 40)
		nextCursor := ""
		hasMore := false
		if page < 4 {
			nextCursor = strconv.Itoa(page + 1)
			hasMore = true
		}
		_ = json.NewEncoder(w).Encode(HistoryManifest{
			Protocol:     ProtocolVersion,
			Type:         historyManifestType,
			Project:      "hao.news",
			NetworkID:    latestOrgNetworkID,
			Page:         page,
			PageSize:     1,
			EntryCount:   1,
			TotalEntries: 4,
			TotalPages:   4,
			Cursor:       strconv.Itoa(page),
			NextCursor:   nextCursor,
			HasMore:      hasMore,
			Entries: []SyncAnnouncement{{
				InfoHash:  infohash,
				Magnet:    "magnet:?xt=urn:btih:" + infohash + "&dn=page-" + strconv.Itoa(page),
				Kind:      "post",
				Author:    "agent://pc75/main",
				Project:   "hao.news",
				NetworkID: latestOrgNetworkID,
				CreatedAt: time.Now().UTC().Format(time.RFC3339),
			}},
		})
	}))
	defer srv.Close()

	runtime := &syncRuntime{
		store:            store,
		queuePath:        queues.RealtimePath,
		historyQueuePath: queues.HistoryPath,
		netCfg: NetworkBootstrapConfig{
			NetworkID: latestOrgNetworkID,
			LANPeers:  []string{srv.URL},
		},
		subscriptions: SyncSubscriptions{},
	}
	added, err := runtime.enqueueHistoryFromLANPeers(context.Background(), nil)
	if err != nil {
		t.Fatalf("enqueueHistoryFromLANPeers error = %v", err)
	}
	wantPages := max(1, (defaultHistoryMaxItems+historyManifestPageSize-1)/historyManifestPageSize)
	if added != wantPages {
		t.Fatalf("added = %d, want %d", added, wantPages)
	}
	queueData, err := os.ReadFile(queues.HistoryPath)
	if err != nil {
		t.Fatalf("read history queue: %v", err)
	}
	text := string(queueData)
	if strings.Contains(text, "dn=page-1") {
		t.Fatalf("history queue should not include recent page 1 entry: %q", text)
	}
	if !strings.Contains(text, "dn=page-2") || !strings.Contains(text, "dn=page-3") {
		t.Fatalf("history queue missing recent pages: %q", text)
	}
	if strings.Contains(text, "dn=page-4") {
		t.Fatalf("history queue should not include page 4 during bootstrap: %q", text)
	}
	realtimeData, err := os.ReadFile(queues.RealtimePath)
	if err != nil {
		t.Fatalf("read realtime queue: %v", err)
	}
	if !strings.Contains(string(realtimeData), "dn=page-1") {
		t.Fatalf("realtime queue missing recent page 1 entry: %q", string(realtimeData))
	}
	state, err := loadHistoryBootstrapState(store)
	if err != nil {
		t.Fatalf("loadHistoryBootstrapState error = %v", err)
	}
	if state.HistoryBootstrapMode != "recent" || state.FirstSyncCompleted {
		t.Fatalf("bootstrap state = %#v, want recent incomplete", state)
	}
}

func TestMaybeCompleteHistoryBootstrapMarksSteadyMode(t *testing.T) {
	t.Parallel()

	store, err := OpenStore(filepath.Join(t.TempDir(), ".haonews"))
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	queues, err := ensureSyncLayout(store, "")
	if err != nil {
		t.Fatalf("ensureSyncLayout error = %v", err)
	}
	runtime := &syncRuntime{
		store:            store,
		queuePath:        queues.RealtimePath,
		historyQueuePath: queues.HistoryPath,
		historyBootstrap: historyBootstrapState{
			HistoryBootstrapMode: "recent",
			RecentPagesLimit:     max(1, (defaultHistoryMaxItems+historyManifestPageSize-1)/historyManifestPageSize),
			RecentRefsLimit:      defaultHistoryMaxItems,
		},
	}
	if err := os.WriteFile(queues.HistoryPath, []byte("# history sync refs\nmagnet:?xt=urn:btih:"+strings.Repeat("1", 40)+"&dn=older\n"), 0o644); err != nil {
		t.Fatalf("write history queue: %v", err)
	}
	if err := runtime.maybeCompleteHistoryBootstrap(nil); err != nil {
		t.Fatalf("maybeCompleteHistoryBootstrap error = %v", err)
	}
	state, err := loadHistoryBootstrapState(store)
	if err != nil {
		t.Fatalf("loadHistoryBootstrapState error = %v", err)
	}
	if !state.FirstSyncCompleted || state.HistoryBootstrapMode != "steady" {
		t.Fatalf("bootstrap state = %#v, want steady completed", state)
	}
}

func TestEnqueueHistoryFromLANPeersRecentBootstrapRespectsHistoryDays(t *testing.T) {
	t.Parallel()

	store, err := OpenStore(filepath.Join(t.TempDir(), ".haonews"))
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	queues, err := ensureSyncLayout(store, "")
	if err != nil {
		t.Fatalf("ensureSyncLayout error = %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(HistoryManifest{
			Protocol:     ProtocolVersion,
			Type:         historyManifestType,
			Project:      "hao.news",
			NetworkID:    latestOrgNetworkID,
			Page:         1,
			PageSize:     1,
			EntryCount:   1,
			TotalEntries: 1,
			TotalPages:   1,
			Cursor:       "1",
			HasMore:      false,
			Entries: []SyncAnnouncement{{
				InfoHash:  strings.Repeat("a", 40),
				Magnet:    "magnet:?xt=urn:btih:" + strings.Repeat("a", 40) + "&dn=old",
				Kind:      "post",
				Author:    "agent://pc75/main",
				Project:   "hao.news",
				NetworkID: latestOrgNetworkID,
				CreatedAt: time.Now().UTC().Add(-72 * time.Hour).Format(time.RFC3339),
			}},
		})
	}))
	defer srv.Close()

	runtime := &syncRuntime{
		store:            store,
		queuePath:        queues.RealtimePath,
		historyQueuePath: queues.HistoryPath,
		netCfg: NetworkBootstrapConfig{
			NetworkID: latestOrgNetworkID,
			LANPeers:  []string{srv.URL},
		},
		subscriptions: SyncSubscriptions{
			HistoryDays: 1,
		},
	}
	added, err := runtime.enqueueHistoryFromLANPeers(context.Background(), nil)
	if err != nil {
		t.Fatalf("enqueueHistoryFromLANPeers error = %v", err)
	}
	if added != 0 {
		t.Fatalf("added = %d, want 0 for stale history entry", added)
	}
	queueData, err := os.ReadFile(queues.HistoryPath)
	if err != nil {
		t.Fatalf("read history queue: %v", err)
	}
	if strings.Contains(string(queueData), "dn=old") {
		t.Fatalf("history queue should not include stale entry: %q", string(queueData))
	}
}

func TestEnqueueHistoryFromLANPeersPromotesRecentPageOneEntriesToRealtime(t *testing.T) {
	t.Parallel()

	store, err := OpenStore(filepath.Join(t.TempDir(), ".haonews"))
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	queues, err := ensureSyncLayout(store, "")
	if err != nil {
		t.Fatalf("ensureSyncLayout error = %v", err)
	}
	recentInfoHash := strings.Repeat("b", 40)
	olderInfoHash := strings.Repeat("c", 40)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := strings.TrimSpace(r.URL.Query().Get("cursor"))
		if page == "" {
			page = "1"
		}
		payload := HistoryManifest{
			Protocol:     ProtocolVersion,
			Type:         historyManifestType,
			Project:      "hao.news",
			NetworkID:    latestOrgNetworkID,
			Page:         1,
			PageSize:     2,
			EntryCount:   2,
			TotalEntries: 2,
			TotalPages:   1,
			Cursor:       "1",
			HasMore:      false,
			Entries: []SyncAnnouncement{
				{
					InfoHash:  recentInfoHash,
					Magnet:    "magnet:?xt=urn:btih:" + recentInfoHash + "&dn=recent",
					Kind:      "post",
					Author:    "agent://pc75/main",
					Project:   "hao.news",
					NetworkID: latestOrgNetworkID,
					CreatedAt: time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339),
				},
				{
					InfoHash:  olderInfoHash,
					Magnet:    "magnet:?xt=urn:btih:" + olderInfoHash + "&dn=older",
					Kind:      "post",
					Author:    "agent://pc75/main",
					Project:   "hao.news",
					NetworkID: latestOrgNetworkID,
					CreatedAt: time.Now().UTC().Add(-6 * time.Hour).Format(time.RFC3339),
				},
			},
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	runtime := &syncRuntime{
		store:            store,
		queuePath:        queues.RealtimePath,
		historyQueuePath: queues.HistoryPath,
		netCfg: NetworkBootstrapConfig{
			NetworkID: latestOrgNetworkID,
			LANPeers:  []string{srv.URL},
		},
		subscriptions: SyncSubscriptions{},
	}
	added, err := runtime.enqueueHistoryFromLANPeers(context.Background(), nil)
	if err != nil {
		t.Fatalf("enqueueHistoryFromLANPeers error = %v", err)
	}
	if added != 2 {
		t.Fatalf("added = %d, want 2", added)
	}
	realtimeData, err := os.ReadFile(queues.RealtimePath)
	if err != nil {
		t.Fatalf("read realtime queue: %v", err)
	}
	if !strings.Contains(string(realtimeData), "dn=recent") {
		t.Fatalf("realtime queue missing recent entry: %q", string(realtimeData))
	}
	if strings.Contains(string(realtimeData), "dn=older") {
		t.Fatalf("realtime queue should not include older entry: %q", string(realtimeData))
	}
	historyData, err := os.ReadFile(queues.HistoryPath)
	if err != nil {
		t.Fatalf("read history queue: %v", err)
	}
	if !strings.Contains(string(historyData), "dn=older") {
		t.Fatalf("history queue missing older entry: %q", string(historyData))
	}
	if strings.Contains(string(historyData), "dn=recent") {
		t.Fatalf("history queue should not include promoted recent entry: %q", string(historyData))
	}
}

func TestEnqueueHistoryFromLANPeersPromotesRecentDuplicateOutOfHistoryQueue(t *testing.T) {
	t.Parallel()

	store, err := OpenStore(filepath.Join(t.TempDir(), ".haonews"))
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	queues, err := ensureSyncLayout(store, "")
	if err != nil {
		t.Fatalf("ensureSyncLayout error = %v", err)
	}
	recentInfoHash := strings.Repeat("d", 40)
	oldHistoryLine := "magnet:?xt=urn:btih:" + recentInfoHash + "&dn=stale-history-copy"
	if err := os.WriteFile(queues.HistoryPath, []byte("# history.txt\n"+oldHistoryLine+"\n"), 0o644); err != nil {
		t.Fatalf("write history queue: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := HistoryManifest{
			Protocol:     ProtocolVersion,
			Type:         historyManifestType,
			Project:      "hao.news",
			NetworkID:    latestOrgNetworkID,
			Page:         1,
			PageSize:     1,
			EntryCount:   1,
			TotalEntries: 1,
			TotalPages:   1,
			Cursor:       "1",
			HasMore:      false,
			Entries: []SyncAnnouncement{{
				InfoHash:  recentInfoHash,
				Magnet:    "magnet:?xt=urn:btih:" + recentInfoHash + "&dn=recent-promoted",
				Kind:      "post",
				Author:    "agent://pc75/main",
				Project:   "hao.news",
				NetworkID: latestOrgNetworkID,
				CreatedAt: time.Now().UTC().Add(-15 * time.Minute).Format(time.RFC3339),
			}},
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	runtime := &syncRuntime{
		store:            store,
		queuePath:        queues.RealtimePath,
		historyQueuePath: queues.HistoryPath,
		netCfg: NetworkBootstrapConfig{
			NetworkID: latestOrgNetworkID,
			LANPeers:  []string{srv.URL},
		},
		subscriptions: SyncSubscriptions{},
	}
	added, err := runtime.enqueueHistoryFromLANPeers(context.Background(), nil)
	if err != nil {
		t.Fatalf("enqueueHistoryFromLANPeers error = %v", err)
	}
	if added != 1 {
		t.Fatalf("added = %d, want 1", added)
	}

	realtimeData, err := os.ReadFile(queues.RealtimePath)
	if err != nil {
		t.Fatalf("read realtime queue: %v", err)
	}
	if !strings.Contains(string(realtimeData), "dn=recent-promoted") {
		t.Fatalf("realtime queue missing promoted entry: %q", string(realtimeData))
	}

	historyData, err := os.ReadFile(queues.HistoryPath)
	if err != nil {
		t.Fatalf("read history queue: %v", err)
	}
	if strings.Contains(string(historyData), recentInfoHash) {
		t.Fatalf("history queue still contains promoted duplicate: %q", string(historyData))
	}
}

func TestEnqueueHistoryFromLANPeersUsesConfiguredPublicPeers(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	const sourcePeerID = "12D3KooWQYdFguTJNhMWvYr5MZYqeg4RZZ9tx6c1nw7cX7cGCGzb"
	announcement := SyncAnnouncement{
		Magnet:       "magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		InfoHash:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Title:        "public peer history",
		CreatedAt:    now.Format(time.RFC3339),
		NetworkID:    latestOrgNetworkID,
		LibP2PPeerID: sourcePeerID,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/history/list" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(lanHistoryManifestResponse{
			NetworkID: latestOrgNetworkID,
			Entries:   []SyncAnnouncement{announcement},
		})
	}))
	defer srv.Close()

	store, err := OpenStore(filepath.Join(t.TempDir(), ".haonews"))
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	runtime := &syncRuntime{
		store:            store,
		queuePath:        filepath.Join(store.Root, "sync", "realtime.txt"),
		historyQueuePath: filepath.Join(store.Root, "sync", "history.txt"),
		netCfg: NetworkBootstrapConfig{
			NetworkID:   latestOrgNetworkID,
			PublicPeers: []string{srv.URL},
		},
		subscriptions: SyncSubscriptions{
			Topics: []string{"all"},
		},
	}
	if err := os.MkdirAll(filepath.Dir(runtime.queuePath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	added, err := runtime.enqueueHistoryFromLANPeers(context.Background(), nil)
	if err != nil {
		t.Fatalf("enqueueHistoryFromLANPeers error = %v", err)
	}
	if added != 1 {
		t.Fatalf("added = %d, want 1", added)
	}
	realtimeRefs, historyRefs, err := collectSyncRefs(nil, runtime.queuePath, runtime.historyQueuePath)
	if err != nil {
		t.Fatalf("collectSyncRefs error = %v", err)
	}
	if len(realtimeRefs) != 1 {
		t.Fatalf("realtime refs = %d, want 1", len(realtimeRefs))
	}
	peers := runtime.directPeerIDs(announcement.InfoHash)
	if len(peers) != 1 || peers[0].String() != announcement.LibP2PPeerID {
		t.Fatalf("direct peers = %v", peers)
	}
	if len(historyRefs) != 0 {
		t.Fatalf("history refs = %d, want 0", len(historyRefs))
	}
}

func TestSyncPeerSourcesIncludesLANPublicAndRelayPeers(t *testing.T) {
	t.Parallel()

	got := syncPeerSourcesWithLocalHosts(NetworkBootstrapConfig{
		LANPeers:    []string{"192.168.102.75"},
		PublicPeers: []string{"https://ai.jie.news"},
		RelayPeers:  []string{"relay.jie.news", "192.168.102.75"},
	}, nil)

	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	if got[0] != "192.168.102.75" {
		t.Fatalf("got[0] = %q, want LAN peer first", got[0])
	}
	if got[1] != "https://ai.jie.news" {
		t.Fatalf("got[1] = %q, want public peer second", got[1])
	}
	if got[2] != "relay.jie.news" {
		t.Fatalf("got[2] = %q, want relay peer third", got[2])
	}
}

func TestSyncPeerSourcesExcludesSelfPublicPeersInPublicMode(t *testing.T) {
	t.Parallel()

	got := syncPeerSourcesWithLocalHosts(NetworkBootstrapConfig{
		NetworkMode: networkModePublic,
		PublicPeers: []string{"https://ai.jie.news"},
		RelayPeers:  []string{"relay.jie.news"},
	}, nil)

	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0] != "relay.jie.news" {
		t.Fatalf("got[0] = %q, want relay peer only", got[0])
	}
}

func TestSyncPeerSourcesExcludesLocalLANPeerHosts(t *testing.T) {
	t.Parallel()

	got := syncPeerSourcesWithLocalHosts(NetworkBootstrapConfig{
		NetworkMode: networkModeLAN,
		LANPeers:    []string{"192.168.102.74", "192.168.102.75", "192.168.102.76"},
	}, []string{"192.168.102.76"})

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0] != "192.168.102.74" {
		t.Fatalf("got[0] = %q, want 192.168.102.74", got[0])
	}
	if got[1] != "192.168.102.75" {
		t.Fatalf("got[1] = %q, want 192.168.102.75", got[1])
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

	queueRoot := t.TempDir()
	queue := filepath.Join(queueRoot, "realtime.txt")
	historyQueue := filepath.Join(queueRoot, "history.txt")
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
		store:            store,
		queuePath:        queue,
		historyQueuePath: historyQueue,
		netCfg:           NetworkBootstrapConfig{NetworkID: latestOrgNetworkID},
		directPeers:      make(map[string][]peer.ID),
		subscriptions:    SyncSubscriptions{},
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
	data, err := os.ReadFile(queue)
	if err != nil {
		t.Fatalf("read realtime queue: %v", err)
	}
	if !strings.Contains(string(data), "93a71a010a59022c8670e06e2c92fa279f98d974") {
		t.Fatalf("realtime queue missing announcement ref: %q", string(data))
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
		requesterStore,
		SyncRef{Raw: published.Magnet, Magnet: published.Magnet, InfoHash: published.InfoHash},
		5*time.Second,
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
