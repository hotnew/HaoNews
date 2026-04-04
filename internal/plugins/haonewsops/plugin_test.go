package haonewsops

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"

	"hao.news/internal/apphost"
	corehaonews "hao.news/internal/haonews"
	newsplugin "hao.news/internal/plugins/haonews"
	themehaonews "hao.news/internal/themes/haonews"
)

func TestPluginBuildServesNetworkPage(t *testing.T) {
	t.Parallel()

	site := buildOpsSite(t)
	req := httptest.NewRequest(http.MethodGet, "/network", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "实时网络遥测") {
		t.Fatalf("expected network page content, got %q", rec.Body.String())
	}
}

func TestPluginBuildServesLANPeerHealth(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	site := buildOpsSiteAtRoot(t, root)
	netPath := filepath.Join(root, "config", "haonews_net.inf")
	healthPath := filepath.Join(root, "config", "lan_peer_health.json")
	knownGoodPath := filepath.Join(root, "config", "known_good_libp2p_peers.json")
	advertiseHealthPath := filepath.Join(root, "config", "advertise_host_health.json")
	netText := `network_id=2c2d6cf7b255ba20d6ad01135654933851b02bd00c65c2a6a54b97ab56590475
lan_peer=192.168.102.74
`
	if err := os.WriteFile(netPath, []byte(netText), 0o644); err != nil {
		t.Fatalf("WriteFile(netPath) error = %v", err)
	}
	now := time.Now().UTC()
	healthText := fmt.Sprintf(`{
  "entries": {
    "lan_peer|192.168.102.74": {
      "last_success_at": %q,
      "observed_primary_host": "192.168.102.75",
      "observed_primary_from": "lan_peer"
    }
  }
}
`, now.Add(-2*time.Minute).Format(time.RFC3339))
	if err := os.WriteFile(healthPath, []byte(healthText), 0o644); err != nil {
		t.Fatalf("WriteFile(healthPath) error = %v", err)
	}
	knownGoodText := fmt.Sprintf(`{
  "network_id": "2c2d6cf7b255ba20d6ad01135654933851b02bd00c65c2a6a54b97ab56590475",
  "entries": {
    "QmKnownGood": {
      "last_success_at": %q,
      "addrs": [
        "/ip4/192.168.102.75/tcp/50584/p2p/QmKnownGood"
      ]
    }
  }
}
`, now.Add(-90*time.Second).Format(time.RFC3339))
	if err := os.WriteFile(knownGoodPath, []byte(knownGoodText), 0o644); err != nil {
		t.Fatalf("WriteFile(knownGoodPath) error = %v", err)
	}
	advertiseHealthText := fmt.Sprintf(`{
  "entries": {
    "192.168.102.75": {
      "success_count": 3,
      "failure_count": 1,
      "last_success_at": %q,
      "last_failure_at": %q
    }
  }
}
`, now.Add(-30*time.Second).Format(time.RFC3339), now.Add(-4*time.Minute).Format(time.RFC3339))
	if err := os.WriteFile(advertiseHealthPath, []byte(advertiseHealthText), 0o644); err != nil {
		t.Fatalf("WriteFile(advertiseHealthPath) error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/network", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"局域网锚点健康",
		"libp2p 锚点健康",
		"主通告 libp2p",
		"Relay Reservation",
		"实际可达地址",
		"主通告候选地址",
		"地址类型",
		"网卡类型",
		"已知好节点缓存",
		"当前主地址解释",
		"当前主通告地址",
		"主通告地址历史",
		"3 成功 / 1 失败",
		"192.168.102.75",
		"观察到的主地址",
		"来源：lan_peer",
		"known-good",
		"QmKnownGood",
		"preferred",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected network page to contain %q, got %q", want, body)
		}
	}
}

func TestPluginBuildReturnsBootstrapUnavailableWithoutSyncDaemon(t *testing.T) {
	t.Parallel()

	site := buildOpsSite(t)
	req := httptest.NewRequest(http.MethodGet, "/api/network/bootstrap", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestPluginBuildServesBootstrapExplainAPI(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	netPath := filepath.Join(root, "config", "haonews_net.inf")
	if err := os.MkdirAll(filepath.Dir(netPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(netPath) error = %v", err)
	}
	netText := `network_id=2c2d6cf7b255ba20d6ad01135654933851b02bd00c65c2a6a54b97ab56590475
lan_peer=192.168.102.75
`
	if err := os.WriteFile(netPath, []byte(netText), 0o644); err != nil {
		t.Fatalf("WriteFile(netPath) error = %v", err)
	}
	advertiseHealthPath := filepath.Join(root, "config", "advertise_host_health.json")
	advertiseHealthText := `{
  "entries": {
    "192.168.102.75": {
      "success_count": 2,
      "failure_count": 1,
      "last_success_at": "2026-03-20T10:03:00Z",
      "last_failure_at": "2026-03-20T10:01:30Z"
    }
  }
}
`
	if err := os.WriteFile(advertiseHealthPath, []byte(advertiseHealthText), 0o644); err != nil {
		t.Fatalf("WriteFile(advertiseHealthPath) error = %v", err)
	}
	healthPath := filepath.Join(root, "config", "lan_peer_health.json")
	healthText := `{
  "entries": {
    "lan_peer|192.168.102.75": {
      "last_success_at": "2026-03-20T10:00:00Z",
      "observed_primary_host": "192.168.102.76",
      "observed_primary_from": "lan_peer"
    }
  }
}
`
	if err := os.WriteFile(healthPath, []byte(healthText), 0o644); err != nil {
		t.Fatalf("WriteFile(healthPath) error = %v", err)
	}
	syncDir := filepath.Join(root, "store", "sync")
	if err := os.MkdirAll(syncDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(syncDir) error = %v", err)
	}
	status := newsplugin.SyncRuntimeStatus{
		UpdatedAt: time.Now().UTC(),
		NetworkID: "2c2d6cf7b255ba20d6ad01135654933851b02bd00c65c2a6a54b97ab56590475",
		LibP2P: newsplugin.SyncLibP2PStatus{
			Enabled:                true,
			PeerID:                 "QmBootstrapPeer",
			ListenAddrs:            []string{"/ip4/0.0.0.0/tcp/50584"},
			AutoNATv2Enabled:       true,
			AutoRelayEnabled:       true,
			HolePunchingEnabled:    true,
			Reachability:           "private",
			ReachableAddrs:         []string{"/dns4/ai.jie.news/tcp/50584/p2p/QmBootstrapPeer"},
			RelayReservationActive: true,
			RelayReservationCount:  2,
			RelayReservationPeers:  []string{"QmRelayA", "QmRelayB"},
			RelayAddrs: []string{
				"/dns4/relay.jie.news/tcp/4001/p2p/QmRelayA/p2p-circuit/p2p/QmBootstrapPeer",
				"/dns4/relay2.jie.news/tcp/4001/p2p/QmRelayB/p2p-circuit/p2p/QmBootstrapPeer",
			},
			LastError:      "",
			Peers:          nil,
			ConnectedPeers: 1,
		},
		TeamSync: newsplugin.SyncTeamSyncStatus{
			Enabled:           true,
			NodeID:            "12D3KooWTeamNode",
			SubscribedTeams:   1,
			PublishedMessages: 2,
			ReceivedMessages:  1,
			AppliedMessages:   1,
			LastTeamID:        "archive-demo",
			LastPublishedKey:  "message:msg-1",
		},
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("json.Marshal(status) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(syncDir, "status.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile(status.json) error = %v", err)
	}

	site := buildOpsSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/api/network/bootstrap", nil)
	req.Host = "127.0.0.1:51818"
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload newsplugin.NetworkBootstrapResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.PeerID != "QmBootstrapPeer" {
		t.Fatalf("payload.PeerID = %q", payload.PeerID)
	}
	if len(payload.Explain) == 0 {
		t.Fatalf("payload.Explain = %#v, want non-empty explanation", payload.Explain)
	}
	if !strings.Contains(strings.Join(payload.Explain, "\n"), "当前主通告地址") {
		t.Fatalf("payload.Explain = %#v, want primary explain text", payload.Explain)
	}
	if payload.ExplainDetail == nil {
		t.Fatal("payload.ExplainDetail = nil, want structured detail")
	}
	if payload.ExplainDetail.PrimaryHost != "192.168.102.75" {
		t.Fatalf("payload.ExplainDetail.PrimaryHost = %q", payload.ExplainDetail.PrimaryHost)
	}
	if !payload.ExplainDetail.RelayReservationActive || payload.ExplainDetail.RelayReservationCount != 2 {
		t.Fatalf("payload.ExplainDetail = %#v, want relay reservation status", payload.ExplainDetail)
	}
	if len(payload.ExplainDetail.RelayReservationPeers) != 2 {
		t.Fatalf("payload.ExplainDetail = %#v, want relay reservation peers", payload.ExplainDetail)
	}
	if len(payload.ExplainDetail.ReachableAddrs) != 1 {
		t.Fatalf("payload.ExplainDetail = %#v, want reachable addrs", payload.ExplainDetail)
	}
	if payload.ExplainDetail.SuccessCount < 2 || payload.ExplainDetail.FailureCount != 1 {
		t.Fatalf("payload.ExplainDetail = %#v", payload.ExplainDetail)
	}
	if payload.ExplainDetail.LANLibP2P == nil {
		t.Fatalf("payload.ExplainDetail = %#v, want lan detail", payload.ExplainDetail)
	}
	if payload.ExplainDetail.LANLibP2P.ObservedPrimaryHost != "192.168.102.76" {
		t.Fatalf("payload.ExplainDetail.LANLibP2P = %#v, want observed_primary_host", payload.ExplainDetail.LANLibP2P)
	}
	if payload.ExplainDetail.LANLibP2P.ObservedPrimaryFrom != "lan_peer" {
		t.Fatalf("payload.ExplainDetail.LANLibP2P = %#v, want observed_primary_from", payload.ExplainDetail.LANLibP2P)
	}
	if payload.TeamSync == nil || !payload.TeamSync.Enabled {
		t.Fatalf("payload.TeamSync = %#v, want enabled team sync status", payload.TeamSync)
	}
	if payload.TeamSync.LastTeamID != "archive-demo" || payload.TeamSync.PublishedMessages != 2 {
		t.Fatalf("payload.TeamSync = %#v", payload.TeamSync)
	}
}

func TestPluginBuildServesBootstrapReadinessDuringColdStart(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	syncDir := filepath.Join(root, "store", "sync")
	if err := os.MkdirAll(syncDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(syncDir) error = %v", err)
	}
	status := newsplugin.SyncRuntimeStatus{
		UpdatedAt: time.Now().UTC(),
		NetworkID: "test-network",
		LibP2P: newsplugin.SyncLibP2PStatus{
			Enabled:     true,
			PeerID:      "QmBootstrapPeer",
			ListenAddrs: []string{"/ip4/0.0.0.0/tcp/50584"},
		},
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("json.Marshal(status) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(syncDir, "status.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile(status.json) error = %v", err)
	}

	site := buildOpsSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/api/network/bootstrap", nil)
	req.Header.Set("X-HaoNews-Debug-ColdStart", "1")
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload newsplugin.NetworkBootstrapResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Readiness == nil {
		t.Fatal("payload.Readiness = nil, want readiness status")
	}
	if payload.Readiness.Stage != "warming_index" {
		t.Fatalf("payload.Readiness = %#v, want warming_index", payload.Readiness)
	}
	if !payload.Readiness.HTTPReady || payload.Readiness.IndexReady || payload.Readiness.WarmupReady || !payload.Readiness.ColdStarting {
		t.Fatalf("payload.Readiness = %#v, want http_ready=true index_ready=false warmup_ready=false cold_starting=true", payload.Readiness)
	}
}

func TestPluginBuildServesBootstrapReadinessReadyByDefault(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	syncDir := filepath.Join(root, "store", "sync")
	if err := os.MkdirAll(syncDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(syncDir) error = %v", err)
	}
	status := newsplugin.SyncRuntimeStatus{
		UpdatedAt: time.Now().UTC(),
		NetworkID: "test-network",
		LibP2P: newsplugin.SyncLibP2PStatus{
			Enabled:     true,
			PeerID:      "QmBootstrapPeer",
			ListenAddrs: []string{"/ip4/0.0.0.0/tcp/50584"},
		},
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("json.Marshal(status) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(syncDir, "status.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile(status.json) error = %v", err)
	}

	site := buildOpsSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/api/network/bootstrap", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload newsplugin.NetworkBootstrapResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Readiness == nil {
		t.Fatal("payload.Readiness = nil, want readiness status")
	}
	if payload.Readiness.Stage != "ready" {
		t.Fatalf("payload.Readiness = %#v, want ready", payload.Readiness)
	}
	if !payload.Readiness.HTTPReady || !payload.Readiness.IndexReady || !payload.Readiness.WarmupReady || payload.Readiness.ColdStarting {
		t.Fatalf("payload.Readiness = %#v, want http_ready=true index_ready=true warmup_ready=true cold_starting=false", payload.Readiness)
	}
}

func TestPluginBuildServesBootstrapRedisSummary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	netPath := filepath.Join(root, "config", "haonews_net.inf")
	if err := os.MkdirAll(filepath.Dir(netPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(netPath) error = %v", err)
	}
	mini, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run error = %v", err)
	}
	defer mini.Close()
	netText := "network_mode=lan\nredis_enabled=true\nredis_addr=" + mini.Addr() + "\nredis_db=2\nredis_key_prefix=haonews-test-\n"
	if err := os.WriteFile(netPath, []byte(netText), 0o644); err != nil {
		t.Fatalf("WriteFile(netPath) error = %v", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr(), DB: 2})
	defer rdb.Close()
	if err := rdb.Set(context.Background(), "haonews-test-sync:ann:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", `{"infohash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","channel":"news","topics":["world"]}`, time.Hour).Err(); err != nil {
		t.Fatalf("Set(sync ann) error = %v", err)
	}
	if err := rdb.ZAdd(context.Background(), "haonews-test-sync:channel:news", redis.Z{Score: 1711933200, Member: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}).Err(); err != nil {
		t.Fatalf("ZAdd(channel) error = %v", err)
	}
	if err := rdb.ZAdd(context.Background(), "haonews-test-sync:topic:world", redis.Z{Score: 1711933200, Member: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}).Err(); err != nil {
		t.Fatalf("ZAdd(topic) error = %v", err)
	}
	if err := rdb.RPush(context.Background(), "haonews-test-sync:queue:refs:realtime", "haonews-sync://bundle/aaa?dn=one").Err(); err != nil {
		t.Fatalf("RPush(realtime) error = %v", err)
	}
	if err := rdb.RPush(context.Background(), "haonews-test-sync:queue:refs:history", "haonews-sync://bundle/bbb?dn=two").Err(); err != nil {
		t.Fatalf("RPush(history) error = %v", err)
	}
	syncDir := filepath.Join(root, "store", "sync")
	if err := os.MkdirAll(syncDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(syncDir) error = %v", err)
	}
	status := newsplugin.SyncRuntimeStatus{
		UpdatedAt: time.Now().UTC(),
		NetworkID: "test-network",
		LibP2P: newsplugin.SyncLibP2PStatus{
			Enabled:     true,
			PeerID:      "QmBootstrapPeer",
			ListenAddrs: []string{"/ip4/0.0.0.0/tcp/50584"},
		},
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("json.Marshal(status) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(syncDir, "status.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile(status.json) error = %v", err)
	}

	site := buildOpsSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/api/network/bootstrap", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload newsplugin.NetworkBootstrapResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Redis == nil {
		t.Fatal("payload.Redis = nil, want redis summary")
	}
	if !payload.Redis.Enabled || !payload.Redis.Online {
		t.Fatalf("payload.Redis = %#v, want enabled+online", payload.Redis)
	}
	if payload.Redis.Addr != mini.Addr() || payload.Redis.Prefix != "haonews-test-" || payload.Redis.DB != 2 {
		t.Fatalf("payload.Redis = %#v, want addr/prefix/db", payload.Redis)
	}
	if payload.Redis.AnnouncementCount != 1 || payload.Redis.ChannelIndexCount != 1 || payload.Redis.TopicIndexCount != 1 {
		t.Fatalf("payload.Redis = %#v, want redis sync index counts", payload.Redis)
	}
	if payload.Redis.RealtimeQueueRefs != 1 || payload.Redis.HistoryQueueRefs != 1 {
		t.Fatalf("payload.Redis = %#v, want queue mirror counts", payload.Redis)
	}
}

func TestPluginBuildServesBootstrapExplainAPIPublicModeUsesPublicPeerDialAddrs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	netPath := filepath.Join(root, "config", "haonews_net.inf")
	if err := os.MkdirAll(filepath.Dir(netPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(netPath) error = %v", err)
	}
	netText := `network_mode=public
network_id=2c2d6cf7b255ba20d6ad01135654933851b02bd00c65c2a6a54b97ab56590475
public_peer=ai.jie.news
`
	if err := os.WriteFile(netPath, []byte(netText), 0o644); err != nil {
		t.Fatalf("WriteFile(netPath) error = %v", err)
	}
	syncDir := filepath.Join(root, "store", "sync")
	if err := os.MkdirAll(syncDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(syncDir) error = %v", err)
	}
	status := newsplugin.SyncRuntimeStatus{
		UpdatedAt: time.Now().UTC(),
		NetworkID: "2c2d6cf7b255ba20d6ad01135654933851b02bd00c65c2a6a54b97ab56590475",
		LibP2P: newsplugin.SyncLibP2PStatus{
			Enabled: true,
			PeerID:  "QmBootstrapPeer",
			ListenAddrs: []string{
				"/ip4/10.219.147.1/tcp/50584",
				"/ip4/127.0.0.1/tcp/50584",
			},
			ConfiguredListen: []string{
				"/ip4/0.0.0.0/tcp/50584",
				"/ip4/0.0.0.0/udp/50584/quic-v1",
			},
		},
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("json.Marshal(status) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(syncDir, "status.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile(status.json) error = %v", err)
	}

	site := buildOpsSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/api/network/bootstrap", nil)
	req.Host = "127.0.0.1:51818"
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload newsplugin.NetworkBootstrapResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.ExplainDetail == nil || payload.ExplainDetail.PrimaryHost != "ai.jie.news" {
		t.Fatalf("payload.ExplainDetail = %#v, want primary_host ai.jie.news", payload.ExplainDetail)
	}
	if len(payload.DialAddrs) == 0 {
		t.Fatal("expected dial addrs")
	}
	for _, value := range payload.DialAddrs {
		if strings.Contains(value, "10.219.147.1") || strings.Contains(value, "127.0.0.1") {
			t.Fatalf("payload.DialAddrs leaked local/private host: %#v", payload.DialAddrs)
		}
		if !strings.Contains(value, "/dns/ai.jie.news/") {
			t.Fatalf("payload.DialAddrs = %#v, want dns ai.jie.news", payload.DialAddrs)
		}
	}
}

func TestPluginBuildServesCreditPage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	seedCreditProof(t, root, "agent://alice/credit/online", corehaonews.AlignToWindow(time.Now().UTC()).Add(-10*time.Minute))

	site := buildOpsSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/credit", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"积分账本总览",
		"积分榜",
		"证明记录：",
		"活动快照",
		"见证角色分布",
		"agent://alice/credit/online",
		"/api/v1/credit/stats",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected credit page to contain %q, got %q", want, body)
		}
	}
}

func TestPluginBuildServesCreditAuthorView(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	seedCreditProof(t, root, "agent://alice/credit/online", corehaonews.AlignToWindow(time.Now().UTC()).Add(-20*time.Minute))
	seedCreditProof(t, root, "agent://alice/credit/online", corehaonews.AlignToWindow(time.Now().UTC()).Add(-10*time.Minute))

	site := buildOpsSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/credit?author=agent://alice/credit/online&start=2026-03-01&end=2026-03-31", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"证明记录：agent://alice/credit/online",
		"当前作者范围：",
		"选定作者",
		"value=\"agent://alice/credit/online\"",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected credit author page to contain %q, got %q", want, body)
		}
	}
}

func TestPluginBuildServesCreditPagePagination(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	day := corehaonews.AlignToWindow(time.Now().UTC()).Add(-10 * time.Minute)
	seedCreditProof(t, root, "agent://alice/credit/online", day)
	seedCreditProof(t, root, "agent://bob/credit/online", day)

	site := buildOpsSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/credit?date="+day.Format("2006-01-02")+"&page_size=1&page=2", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"证明分页",
		"显示第 2-2 条，共 2 条证明。",
		"上一页",
		"page=1",
		"filter-chip is-active\">2</span>",
		"见证者",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected credit pagination page to contain %q, got %q", want, body)
		}
	}
}

func TestPluginBuildServesCreditBalanceAPI(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	seedCreditProof(t, root, "agent://alice/credit/online", corehaonews.AlignToWindow(time.Now().UTC()).Add(-20*time.Minute))
	seedCreditProof(t, root, "agent://alice/credit/online", corehaonews.AlignToWindow(time.Now().UTC()).Add(-10*time.Minute))

	site := buildOpsSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/credit/balance?author=agent://alice/credit/online", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Scope   string                    `json:"scope"`
		Author  string                    `json:"author"`
		Balance corehaonews.CreditBalance `json:"balance"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Scope != "credit_balance" || payload.Author != "agent://alice/credit/online" {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.Balance.Credits != 2 {
		t.Fatalf("credits = %d", payload.Balance.Credits)
	}
}

func TestPluginBuildServesCreditProofsAPI(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	windowStart := corehaonews.AlignToWindow(time.Now().UTC()).Add(-10 * time.Minute)
	proof := seedCreditProof(t, root, "agent://alice/credit/online", windowStart)

	site := buildOpsSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/credit/proofs?date="+windowStart.Format("2006-01-02"), nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Scope      string                     `json:"scope"`
		Date       string                     `json:"date"`
		Proofs     []corehaonews.OnlineProof  `json:"proofs"`
		Total      int                        `json:"total"`
		Pagination newsplugin.PaginationState `json:"pagination"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Scope != "credit_proofs" || payload.Date != windowStart.Format("2006-01-02") {
		t.Fatalf("payload = %#v", payload)
	}
	if len(payload.Proofs) != 1 || payload.Proofs[0].ProofID != proof.ProofID {
		t.Fatalf("proofs = %#v", payload.Proofs)
	}
	if payload.Total != 1 || payload.Pagination.TotalItems != 1 {
		t.Fatalf("pagination payload = %#v", payload)
	}
}

func TestPluginBuildServesCreditProofsAPIPagination(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	day := corehaonews.AlignToWindow(time.Now().UTC()).Add(-10 * time.Minute)
	seedCreditProof(t, root, "agent://alice/credit/online", day)
	seedCreditProof(t, root, "agent://bob/credit/online", day)

	site := buildOpsSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/credit/proofs?date="+day.Format("2006-01-02")+"&page_size=1&page=2", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Proofs     []corehaonews.OnlineProof  `json:"proofs"`
		Total      int                        `json:"total"`
		Pagination newsplugin.PaginationState `json:"pagination"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Total != 2 || payload.Pagination.Page != 2 || payload.Pagination.PageSize != 1 {
		t.Fatalf("pagination payload = %#v", payload)
	}
	if payload.Pagination.PrevURL == "" || payload.Pagination.NextURL != "" {
		t.Fatalf("pagination urls = %#v", payload.Pagination)
	}
	if len(payload.Proofs) != 1 || payload.Proofs[0].ProofID == "" {
		t.Fatalf("proofs = %#v", payload.Proofs)
	}
}

func TestPluginBuildServesCreditStatsAPI(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	seedCreditProof(t, root, "agent://alice/credit/online", corehaonews.AlignToWindow(time.Now().UTC()).Add(-20*time.Minute))
	seedCreditProof(t, root, "agent://bob/credit/online", corehaonews.AlignToWindow(time.Now().UTC()).Add(-10*time.Minute))

	site := buildOpsSiteAtRoot(t, root)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/credit/stats", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Scope        string                              `json:"scope"`
		Totals       map[string]any                      `json:"totals"`
		Balances     []corehaonews.CreditBalance         `json:"balances"`
		Issues       []string                            `json:"issues"`
		Daily        []corehaonews.CreditDailyStat       `json:"daily"`
		WitnessRoles []corehaonews.CreditWitnessRoleStat `json:"witness_roles"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Scope != "credit_stats" {
		t.Fatalf("scope = %q", payload.Scope)
	}
	if len(payload.Balances) != 2 {
		t.Fatalf("balances = %#v", payload.Balances)
	}
	if got, _ := payload.Totals["authors"].(float64); got != 2 {
		t.Fatalf("totals = %#v", payload.Totals)
	}
	if got, _ := payload.Totals["proofs"].(float64); got != 2 {
		t.Fatalf("totals = %#v", payload.Totals)
	}
	if len(payload.Issues) != 0 {
		t.Fatalf("issues = %#v", payload.Issues)
	}
	if len(payload.Daily) == 0 {
		t.Fatalf("daily = %#v", payload.Daily)
	}
	if len(payload.WitnessRoles) == 0 || payload.WitnessRoles[0].Role != "dht_neighbor" {
		t.Fatalf("witness roles = %#v", payload.WitnessRoles)
	}
}

func buildOpsSite(t *testing.T) *apphost.Site {
	t.Helper()
	return buildOpsSiteAtRoot(t, t.TempDir())
}

func buildOpsSiteAtRoot(t *testing.T, root string) *apphost.Site {
	t.Helper()

	cfg := apphost.Config{
		RuntimeRoot:      filepath.Join(root, "runtime"),
		StoreRoot:        filepath.Join(root, "store"),
		ArchiveRoot:      filepath.Join(root, "archive"),
		RulesPath:        filepath.Join(root, "config", "subscriptions.json"),
		WriterPolicyPath: filepath.Join(root, "config", "writer_policy.json"),
		NetPath:          filepath.Join(root, "config", "haonews_net.inf"),
		Project:          "hao.news",
		Version:          "test",
	}
	site, err := Plugin{}.Build(context.Background(), cfg, themehaonews.Theme{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return site
}

func seedCreditProof(t *testing.T, root, author string, windowStart time.Time) corehaonews.OnlineProof {
	t.Helper()

	store, err := corehaonews.OpenCreditStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenCreditStore() error = %v", err)
	}
	node, err := corehaonews.NewAgentIdentity("agent://news/node-01", author, time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity(node) error = %v", err)
	}
	witness, err := corehaonews.NewAgentIdentity("agent://news/witness-01", "agent://witness/credit/online", time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity(witness) error = %v", err)
	}
	proof, err := corehaonews.NewOnlineProof(node, windowStart, []string{"abc123"}, "hao-news-mainnet")
	if err != nil {
		t.Fatalf("NewOnlineProof() error = %v", err)
	}
	if err := corehaonews.SignProof(proof, node); err != nil {
		t.Fatalf("SignProof() error = %v", err)
	}
	if err := corehaonews.AddWitnessSignature(proof, witness, "dht_neighbor"); err != nil {
		t.Fatalf("AddWitnessSignature() error = %v", err)
	}
	if err := store.SaveProof(*proof); err != nil {
		t.Fatalf("SaveProof() error = %v", err)
	}
	return *proof
}
