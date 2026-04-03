package haonews

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestSortLANPeerCandidatesPrefersRecentSuccess(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)
	cache := &lanPeerHealthCache{
		Entries: map[string]lanPeerHealthEntry{
			lanPeerHealthKey("lan_peer", "192.168.102.76"): {
				LastSuccessAt: now.Add(-2 * time.Minute),
			},
		},
	}

	got := sortLANPeerCandidates([]string{
		"192.168.102.74",
		"192.168.102.76",
		"192.168.102.75",
	}, cache, "lan_peer", now)

	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	if got[0] != "192.168.102.76" {
		t.Fatalf("got[0] = %q, want recent success first", got[0])
	}
}

func TestSortLANPeerCandidatesDeprioritizesRecentFailure(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)
	cache := &lanPeerHealthCache{
		Entries: map[string]lanPeerHealthEntry{
			lanPeerHealthKey("lan_peer", "192.168.102.74"): {
				LastFailureAt:      now.Add(-2 * time.Minute),
				ConsecutiveFailure: 2,
			},
		},
	}

	got := sortLANPeerCandidates([]string{
		"192.168.102.74",
		"192.168.102.76",
		"192.168.102.75",
	}, cache, "lan_peer", now)

	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	if got[len(got)-1] != "192.168.102.74" {
		t.Fatalf("got[last] = %q, want cooled failure at end", got[len(got)-1])
	}
}

func TestLANPeerHealthCacheRoundTrip(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := NetworkBootstrapConfig{
		Path: filepath.Join(root, "hao_news_net.inf"),
	}
	cache := &lanPeerHealthCache{}
	cache.recordSuccess("lan_peer", "192.168.102.75", "192.168.102.76")
	cache.recordFailure("lan_peer", "192.168.102.76", errString("timeout"))

	if err := saveLANPeerHealthCache(cfg, cache); err != nil {
		t.Fatalf("saveLANPeerHealthCache() error = %v", err)
	}

	loaded, err := loadLANPeerHealthCache(cfg)
	if err != nil {
		t.Fatalf("loadLANPeerHealthCache() error = %v", err)
	}
	if loaded.entry("lan_peer", "192.168.102.75").LastSuccessAt.IsZero() {
		t.Fatal("expected lan_peer success to persist")
	}
	if observed := loaded.entry("lan_peer", "192.168.102.75").ObservedPrimaryHost; observed != "192.168.102.76" {
		t.Fatalf("ObservedPrimaryHost = %q, want 192.168.102.76", observed)
	}
	if source := loaded.entry("lan_peer", "192.168.102.75").ObservedPrimaryFrom; source != "lan_peer" {
		t.Fatalf("ObservedPrimaryFrom = %q, want lan_peer", source)
	}
	entry := loaded.entry("lan_peer", "192.168.102.76")
	if entry.ConsecutiveFailure != 1 {
		t.Fatalf("entry.ConsecutiveFailure = %d, want 1", entry.ConsecutiveFailure)
	}
	if entry.LastError != "timeout" {
		t.Fatalf("entry.LastError = %q, want timeout", entry.LastError)
	}
}

func TestLANPeerBootstrapTargetsPreferObservedPrimaryHost(t *testing.T) {
	t.Parallel()

	cache := &lanPeerHealthCache{
		Entries: map[string]lanPeerHealthEntry{
			lanPeerHealthKey("lan_peer", "192.168.102.74"): {
				ObservedPrimaryHost: "192.168.102.75",
			},
		},
	}

	got := cache.bootstrapTargets("lan_peer", "192.168.102.74")
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0] != "192.168.102.75" {
		t.Fatalf("got[0] = %q, want observed primary host first", got[0])
	}
	if got[1] != "192.168.102.74" {
		t.Fatalf("got[1] = %q, want configured peer second", got[1])
	}
}

func TestNormalizeObservedPrimaryHostRejectsLoopback(t *testing.T) {
	t.Parallel()

	if got := normalizeObservedPrimaryHost("127.0.0.1"); got != "" {
		t.Fatalf("normalizeObservedPrimaryHost(loopback) = %q, want empty", got)
	}
	if got := normalizeObservedPrimaryHost("192.168.102.75"); got != "192.168.102.75" {
		t.Fatalf("normalizeObservedPrimaryHost(lan) = %q", got)
	}
}

func TestEffectiveLibP2PBootstrapPeersPrefersLAN(t *testing.T) {
	t.Parallel()

	got := EffectiveLibP2PBootstrapPeers(
		[]string{"/ip4/192.168.102.75/tcp/50584/p2p/lan-peer"},
		[]string{
			"/dnsaddr/bootstrap.libp2p.io/p2p/public-1",
			"/ip4/192.168.102.75/tcp/50584/p2p/lan-peer",
			"/dnsaddr/bootstrap.libp2p.io/p2p/public-2",
		},
	)

	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	if got[0] != "/ip4/192.168.102.75/tcp/50584/p2p/lan-peer" {
		t.Fatalf("got[0] = %q, want LAN peer first", got[0])
	}
	if got[1] != "/dnsaddr/bootstrap.libp2p.io/p2p/public-1" {
		t.Fatalf("got[1] = %q, want first public bootstrap second", got[1])
	}
}

func TestEffectiveLibP2PBootstrapPeersWithKnownGoodPrefersExplicitPublicBeforeKnownGood(t *testing.T) {
	t.Parallel()

	got := EffectiveLibP2PBootstrapPeersWithKnownGood(
		[]string{"/ip4/192.168.102.75/tcp/50584/p2p/lan-peer"},
		[]string{"/ip4/192.168.102.80/tcp/50584/p2p/cached-peer"},
		[]string{
			"/dnsaddr/bootstrap.libp2p.io/p2p/public-1",
			"/dnsaddr/bootstrap.libp2p.io/p2p/cached-peer",
		},
	)

	if len(got) != 4 {
		t.Fatalf("len(got) = %d, want 4", len(got))
	}
	if got[0] != "/ip4/192.168.102.75/tcp/50584/p2p/lan-peer" {
		t.Fatalf("got[0] = %q, want LAN peer first", got[0])
	}
	if got[1] != "/dnsaddr/bootstrap.libp2p.io/p2p/public-1" {
		t.Fatalf("got[1] = %q, want explicit public peer before known-good cache", got[1])
	}
	if got[2] != "/dnsaddr/bootstrap.libp2p.io/p2p/cached-peer" {
		t.Fatalf("got[2] = %q, want explicit public cached peer before known-good cache", got[2])
	}
}

func TestResolveExplicitBootstrapPeersUsesPublicPeerBootstrapEndpoint(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/network/bootstrap" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(lanBootstrapResponse{
			NetworkID: latestOrgNetworkID,
			PeerID:    "QmPublicPeer",
			DialAddrs: []string{"/ip4/203.0.113.20/tcp/50584"},
		})
	}))
	defer srv.Close()

	got, err := resolveExplicitBootstrapPeers(context.Background(), []string{srv.URL}, latestOrgNetworkID, "public_peer")
	if err != nil {
		t.Fatalf("resolveExplicitBootstrapPeers() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0] != "/ip4/203.0.113.20/tcp/50584/p2p/QmPublicPeer" {
		t.Fatalf("got[0] = %q", got[0])
	}
}

func TestResolveExplicitBootstrapPeersUsesRelayPeerBootstrapEndpoint(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/network/bootstrap" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(lanBootstrapResponse{
			NetworkID: latestOrgNetworkID,
			PeerID:    "QmRelayPeer",
			DialAddrs: []string{"/dns4/relay.jie.news/tcp/50584"},
		})
	}))
	defer srv.Close()

	got, err := resolveExplicitBootstrapPeers(context.Background(), []string{srv.URL}, latestOrgNetworkID, "relay_peer")
	if err != nil {
		t.Fatalf("resolveExplicitBootstrapPeers() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0] != "/dns4/relay.jie.news/tcp/50584/p2p/QmRelayPeer" {
		t.Fatalf("got[0] = %q", got[0])
	}
}

func TestResolveLANBootstrapPeersFetchesCandidatesInParallel(t *testing.T) {
	t.Parallel()

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(250 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(lanBootstrapResponse{
			NetworkID: latestOrgNetworkID,
			PeerID:    "QmSlowPeer",
			DialAddrs: []string{"/ip4/192.168.102.74/tcp/50584"},
		})
	}))
	defer slow.Close()

	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(25 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(lanBootstrapResponse{
			NetworkID: latestOrgNetworkID,
			PeerID:    "QmFastPeer",
			DialAddrs: []string{"/ip4/192.168.102.75/tcp/50584"},
		})
	}))
	defer fast.Close()

	root := t.TempDir()
	cfg := NetworkBootstrapConfig{
		Path:      filepath.Join(root, "hao_news_net.inf"),
		NetworkID: latestOrgNetworkID,
		LANPeers:  []string{slow.URL, fast.URL},
	}

	start := time.Now()
	peers, err := resolveLANBootstrapPeers(context.Background(), cfg)
	if err != nil {
		t.Fatalf("resolveLANBootstrapPeers error = %v", err)
	}
	if len(peers) != 2 {
		t.Fatalf("len(peers) = %d, want 2", len(peers))
	}
	if elapsed := time.Since(start); elapsed >= 450*time.Millisecond {
		t.Fatalf("resolveLANBootstrapPeers took too long: %s", elapsed)
	}
}

func TestResolveExplicitBootstrapPeersAppendsTargetPeerToRelayCircuitAddr(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/network/bootstrap" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(lanBootstrapResponse{
			NetworkID: latestOrgNetworkID,
			PeerID:    "QmSharedPeer",
			DialAddrs: []string{"/ip4/207.148.109.62/tcp/50584/p2p/QmRelayPeer/p2p-circuit"},
		})
	}))
	defer srv.Close()

	got, err := resolveExplicitBootstrapPeers(context.Background(), []string{srv.URL}, latestOrgNetworkID, "relay_peer")
	if err != nil {
		t.Fatalf("resolveExplicitBootstrapPeers() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0] != "/ip4/207.148.109.62/tcp/50584/p2p/QmRelayPeer/p2p-circuit/p2p/QmSharedPeer" {
		t.Fatalf("got[0] = %q", got[0])
	}
}

func TestKnownGoodLibP2PPeerCacheRoundTrip(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := NetworkBootstrapConfig{
		Path:      filepath.Join(root, "hao_news_net.inf"),
		NetworkID: latestOrgNetworkID,
	}
	cache := &knownGoodLibP2PPeerCache{
		Entries: map[string]knownGoodLibP2PPeerInfo{
			"QmKnownGood": {
				LastSuccessAt: time.Date(2026, 3, 20, 10, 5, 0, 0, time.UTC),
				Addrs:         []string{"/ip4/192.168.102.75/tcp/50584/p2p/QmKnownGood"},
			},
		},
	}

	if err := saveKnownGoodLibP2PPeerCache(cfg, cache); err != nil {
		t.Fatalf("saveKnownGoodLibP2PPeerCache() error = %v", err)
	}

	loaded, err := loadKnownGoodLibP2PPeerCache(cfg)
	if err != nil {
		t.Fatalf("loadKnownGoodLibP2PPeerCache() error = %v", err)
	}
	entry, ok := loaded.Entries["QmKnownGood"]
	if !ok {
		t.Fatal("expected known-good peer entry to persist")
	}
	if entry.LastSuccessAt.IsZero() {
		t.Fatal("expected known-good peer success timestamp to persist")
	}
	if len(entry.Addrs) != 1 || entry.Addrs[0] != "/ip4/192.168.102.75/tcp/50584/p2p/QmKnownGood" {
		t.Fatalf("entry.Addrs = %#v", entry.Addrs)
	}
}

func TestKnownGoodLibP2PPeerCacheNormalizesRelayCircuitAddr(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := NetworkBootstrapConfig{
		Path:      filepath.Join(root, "hao_news_net.inf"),
		NetworkID: latestOrgNetworkID,
	}
	cache := &knownGoodLibP2PPeerCache{
		Entries: map[string]knownGoodLibP2PPeerInfo{
			"QmSharedPeer": {
				LastSuccessAt: time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC),
				Addrs: []string{
					"/ip4/207.148.109.62/tcp/50584/p2p/QmRelayPeer/p2p-circuit",
				},
			},
		},
	}

	if err := saveKnownGoodLibP2PPeerCache(cfg, cache); err != nil {
		t.Fatalf("saveKnownGoodLibP2PPeerCache() error = %v", err)
	}

	loaded, err := loadKnownGoodLibP2PPeerCache(cfg)
	if err != nil {
		t.Fatalf("loadKnownGoodLibP2PPeerCache() error = %v", err)
	}
	entry, ok := loaded.Entries["QmSharedPeer"]
	if !ok {
		t.Fatal("expected known-good relay peer entry to persist")
	}
	if len(entry.Addrs) != 1 || entry.Addrs[0] != "/ip4/207.148.109.62/tcp/50584/p2p/QmRelayPeer/p2p-circuit/p2p/QmSharedPeer" {
		t.Fatalf("entry.Addrs = %#v", entry.Addrs)
	}
}

func TestLoadKnownGoodLibP2PBootstrapPeersDropsStaleEntries(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := NetworkBootstrapConfig{
		Path:      filepath.Join(root, "hao_news_net.inf"),
		NetworkID: latestOrgNetworkID,
	}
	cache := &knownGoodLibP2PPeerCache{
		Entries: map[string]knownGoodLibP2PPeerInfo{
			"QmFresh": {
				LastSuccessAt: time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC),
				Addrs:         []string{"/ip4/192.168.102.75/tcp/50584/p2p/QmFresh"},
			},
			"QmStale": {
				LastSuccessAt: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
				Addrs:         []string{"/ip4/192.168.102.74/tcp/50584/p2p/QmStale"},
			},
		},
	}
	if err := saveKnownGoodLibP2PPeerCache(cfg, cache); err != nil {
		t.Fatalf("saveKnownGoodLibP2PPeerCache() error = %v", err)
	}

	got, err := loadKnownGoodLibP2PBootstrapPeers(cfg, time.Date(2026, 3, 20, 10, 10, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("loadKnownGoodLibP2PBootstrapPeers() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0] != "/ip4/192.168.102.75/tcp/50584/p2p/QmFresh" {
		t.Fatalf("got[0] = %q, want fresh known-good peer", got[0])
	}
}

type errString string

func (e errString) Error() string {
	return string(e)
}
