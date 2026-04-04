package newsplugin

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPreferredAdvertiseHostFallsBackFromLoopbackToLANIP(t *testing.T) {
	t.Parallel()

	prev := listLocalUnicastCandidates
	listLocalUnicastCandidates = func() []localIPCandidate {
		return []localIPCandidate{
			{IP: net.ParseIP("192.168.102.75"), InterfaceName: "en0"},
			{IP: net.ParseIP("10.0.0.15"), InterfaceName: "utun4"},
		}
	}
	t.Cleanup(func() {
		listLocalUnicastCandidates = prev
	})

	status := SyncRuntimeStatus{
		LibP2P: SyncLibP2PStatus{
			ListenAddrs: []string{
				"/ip4/0.0.0.0/tcp/50584",
			},
		},
	}

	got := PreferredAdvertiseHost(status, "127.0.0.1")
	if got != "192.168.102.75" {
		t.Fatalf("preferredAdvertiseHost() = %q, want 192.168.102.75", got)
	}
}

func TestPreferredAdvertiseHostKeepsExplicitLANRequestHost(t *testing.T) {
	t.Parallel()

	status := SyncRuntimeStatus{}
	got := PreferredAdvertiseHost(status, "192.168.102.76")
	if got != "192.168.102.76" {
		t.Fatalf("preferredAdvertiseHost() = %q, want request host", got)
	}
}

func TestPreferredAdvertiseHostPublicModePrefersConfiguredPublicPeerFromLoopback(t *testing.T) {
	t.Parallel()

	status := SyncRuntimeStatus{}
	cfg := NetworkBootstrapConfig{
		NetworkMode: "public",
		PublicPeers: []string{"ai.jie.news"},
	}
	got := PreferredAdvertiseHostForConfig(status, "127.0.0.1", cfg)
	if got != "ai.jie.news" {
		t.Fatalf("preferredAdvertiseHost() = %q, want ai.jie.news", got)
	}
}

func TestPreferredAdvertiseHostSharedModePrefersRoutedLANHost(t *testing.T) {
	t.Parallel()

	prevGateway := defaultGatewayTarget
	prevRoute := routedSourceIPForTarget
	defaultGatewayTarget = func() string {
		return "192.168.102.1"
	}
	routedSourceIPForTarget = func(target string) string {
		switch target {
		case "192.168.102.1":
			return "192.168.102.75"
		case "192.168.102.74", "192.168.102.76":
			return "192.168.102.75"
		case "192.168.100.1":
			return "192.168.100.75"
		default:
			return ""
		}
	}
	t.Cleanup(func() {
		defaultGatewayTarget = prevGateway
		routedSourceIPForTarget = prevRoute
	})

	cfg := NetworkBootstrapConfig{
		NetworkMode: "shared",
		LANPeers:    []string{"192.168.102.74", "192.168.102.76", "192.168.100.1"},
	}
	got := PreferredAdvertiseHostForConfig(SyncRuntimeStatus{}, "127.0.0.1", cfg)
	if got != "192.168.102.75" {
		t.Fatalf("preferredAdvertiseHost() = %q, want routed LAN host 192.168.102.75", got)
	}
}

func TestPreferredAdvertiseHostPrefersPhysicalInterfaceOverVirtual(t *testing.T) {
	t.Parallel()

	prev := listLocalUnicastCandidates
	listLocalUnicastCandidates = func() []localIPCandidate {
		return []localIPCandidate{
			{IP: net.ParseIP("10.10.0.2"), InterfaceName: "utun5"},
			{IP: net.ParseIP("192.168.102.75"), InterfaceName: "en0"},
			{IP: net.ParseIP("172.18.0.1"), InterfaceName: "bridge100"},
		}
	}
	t.Cleanup(func() {
		listLocalUnicastCandidates = prev
	})

	got := PreferredAdvertiseHost(SyncRuntimeStatus{}, "127.0.0.1")
	if got != "192.168.102.75" {
		t.Fatalf("preferredAdvertiseHost() = %q, want physical LAN interface first", got)
	}
}

func TestAdvertiseHostHistoryScoreUsesSuccessHistory(t *testing.T) {
	t.Parallel()

	cache := &advertiseHostHealthCache{
		Entries: map[string]advertiseHostHealthEntry{
			"192.168.102.76": {
				SuccessCount:  3,
				LastSuccessAt: time.Now().UTC(),
			},
			"192.168.102.75": {
				FailureCount:  2,
				LastFailureAt: time.Now().UTC(),
			},
		},
	}
	successScore := advertiseHostHistoryScore(net.ParseIP("192.168.102.76"), cache, time.Now().UTC())
	failureScore := advertiseHostHistoryScore(net.ParseIP("192.168.102.75"), cache, time.Now().UTC())
	if successScore <= failureScore {
		t.Fatalf("successScore = %d, failureScore = %d, want success-biased host score", successScore, failureScore)
	}
}

func TestRecordAdvertiseHostResultRoundTrip(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := NetworkBootstrapConfig{Path: filepath.Join(root, "haonews_net.inf")}
	if err := RecordAdvertiseHostResult(cfg, "192.168.102.75", true); err != nil {
		t.Fatalf("RecordAdvertiseHostResult(success) error = %v", err)
	}
	if err := RecordAdvertiseHostResult(cfg, "192.168.102.75", false); err != nil {
		t.Fatalf("RecordAdvertiseHostResult(failure) error = %v", err)
	}

	cache, err := loadAdvertiseHostHealthCache(cfg)
	if err != nil {
		t.Fatalf("loadAdvertiseHostHealthCache() error = %v", err)
	}
	entry := cache.Entries["192.168.102.75"]
	if entry.SuccessCount != 1 {
		t.Fatalf("entry.SuccessCount = %d, want 1", entry.SuccessCount)
	}
	if entry.FailureCount != 1 {
		t.Fatalf("entry.FailureCount = %d, want 1", entry.FailureCount)
	}
	if entry.LastSuccessAt.IsZero() {
		t.Fatal("expected LastSuccessAt to be recorded")
	}
	if entry.LastFailureAt.IsZero() {
		t.Fatal("expected LastFailureAt to be recorded")
	}
}

func TestLoadAdvertiseHostHealthCacheIgnoresTrailingGarbage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := NetworkBootstrapConfig{Path: filepath.Join(root, "haonews_net.inf")}
	cachePath := advertiseHostHealthCachePath(cfg)
	data := []byte("{\n  \"entries\": {\n    \"192.168.102.75\": {\n      \"success_count\": 1\n    }\n  }\n}\n}\n")
	if err := os.WriteFile(cachePath, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cache, err := loadAdvertiseHostHealthCache(cfg)
	if err != nil {
		t.Fatalf("loadAdvertiseHostHealthCache() error = %v", err)
	}
	entry, ok := cache.Entries["192.168.102.75"]
	if !ok {
		t.Fatal("expected entry for 192.168.102.75")
	}
	if entry.SuccessCount != 1 {
		t.Fatalf("entry.SuccessCount = %d, want 1", entry.SuccessCount)
	}
}

func TestDialableLibP2PAddrsForConfigPublicDomainRewritesPrivateAddrs(t *testing.T) {
	t.Parallel()

	status := SyncRuntimeStatus{
		LibP2P: SyncLibP2PStatus{
			PeerID: "12D3KooWKqit8ESTPbk9mrutVWpJwNPshMfN7tnQtrQVwLzz1L1r",
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
	cfg := NetworkBootstrapConfig{NetworkMode: "public"}
	got := DialableLibP2PAddrsForConfig(status, "ai.jie.news", cfg)
	if len(got) == 0 {
		t.Fatal("expected rewritten dial addrs")
	}
	for _, value := range got {
		if strings.Contains(value, "10.219.147.1") || strings.Contains(value, "127.0.0.1") || strings.Contains(value, "0.0.0.0") {
			t.Fatalf("dial addr leaked private/local host: %q", value)
		}
		if !strings.Contains(value, "/dns/ai.jie.news/") {
			t.Fatalf("dial addr %q does not use public dns host", value)
		}
	}
}

func TestDialableLibP2PAddrsForConfigSharedModePrefersConfiguredStablePort(t *testing.T) {
	t.Parallel()

	status := SyncRuntimeStatus{
		LibP2P: SyncLibP2PStatus{
			PeerID: "12D3KooWKqit8ESTPbk9mrutVWpJwNPshMfN7tnQtrQVwLzz1L1r",
			ListenAddrs: []string{
				"/ip4/192.168.102.75/tcp/25472",
				"/ip4/192.168.102.75/tcp/50584",
			},
			ConfiguredListen: []string{
				"/ip4/0.0.0.0/tcp/50584",
				"/ip4/0.0.0.0/udp/50584/quic-v1",
			},
		},
	}
	cfg := NetworkBootstrapConfig{NetworkMode: "shared"}
	got := DialableLibP2PAddrsForConfig(status, "192.168.102.75", cfg)
	if len(got) < 2 {
		t.Fatalf("DialableLibP2PAddrsForConfig() = %#v, want configured port first", got)
	}
	if !strings.Contains(got[0], "/tcp/50584/") {
		t.Fatalf("DialableLibP2PAddrsForConfig() first addr = %q, want stable configured tcp/50584", got[0])
	}
}
