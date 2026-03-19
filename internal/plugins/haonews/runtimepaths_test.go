package newsplugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testDefaultLatestNetINF() (string, error) {
	return fmt.Sprintf(`network_id=%s
libp2p_listen=/ip4/0.0.0.0/tcp/41001
libp2p_listen=/ip4/0.0.0.0/udp/41001/quic-v1
bittorrent_listen=0.0.0.0:41002
lan_peer=%s
lan_bt_peer=%s
`, latestOrgNetworkID, defaultLANPeer, defaultLANPeer), nil
}

func TestDefaultRuntimePathsFromHome(t *testing.T) {
	paths := DefaultRuntimePathsFromHome("/tmp/example-home")
	if paths.Root != "/tmp/example-home/.hao-news" {
		t.Fatalf("root = %q", paths.Root)
	}
	if paths.StoreRoot != "/tmp/example-home/.hao-news/aip2p/.aip2p" {
		t.Fatalf("store = %q", paths.StoreRoot)
	}
	if paths.IdentitiesRoot != "/tmp/example-home/.hao-news/identities" {
		t.Fatalf("identities = %q", paths.IdentitiesRoot)
	}
	if paths.DelegationsRoot != "/tmp/example-home/.hao-news/delegations" {
		t.Fatalf("delegations = %q", paths.DelegationsRoot)
	}
	if paths.RevocationsRoot != "/tmp/example-home/.hao-news/revocations" {
		t.Fatalf("revocations = %q", paths.RevocationsRoot)
	}
	if paths.ArchiveRoot != "/tmp/example-home/.hao-news/archive" {
		t.Fatalf("archive = %q", paths.ArchiveRoot)
	}
	if paths.RulesPath != "/tmp/example-home/.hao-news/subscriptions.json" {
		t.Fatalf("rules = %q", paths.RulesPath)
	}
	if paths.WriterPolicyPath != "/tmp/example-home/.hao-news/writer_policy.json" {
		t.Fatalf("writer policy = %q", paths.WriterPolicyPath)
	}
	if paths.WriterWhitelistPath != "/tmp/example-home/.hao-news/WriterWhitelist.inf" {
		t.Fatalf("writer whitelist = %q", paths.WriterWhitelistPath)
	}
	if paths.WriterBlacklistPath != "/tmp/example-home/.hao-news/WriterBlacklist.inf" {
		t.Fatalf("writer blacklist = %q", paths.WriterBlacklistPath)
	}
	if paths.NetPath != "/tmp/example-home/.hao-news/hao_news_net.inf" {
		t.Fatalf("net = %q", paths.NetPath)
	}
	if paths.TrackerPath != "/tmp/example-home/.hao-news/Trackerlist.inf" {
		t.Fatalf("tracker = %q", paths.TrackerPath)
	}
}

func TestEnsureRuntimeLayoutCreatesDefaultConfigFiles(t *testing.T) {
	previous := buildDefaultLatestNetINF
	buildDefaultLatestNetINF = testDefaultLatestNetINF
	defer func() { buildDefaultLatestNetINF = previous }()

	root := t.TempDir()
	store := filepath.Join(root, "aip2p", ".aip2p")
	archive := filepath.Join(root, "archive")
	rules := filepath.Join(root, "subscriptions.json")
	writerPolicy := filepath.Join(root, "writer_policy.json")
	netPath := filepath.Join(root, "hao_news_net.inf")
	if err := ensureRuntimeLayout(store, archive, rules, writerPolicy, netPath); err != nil {
		t.Fatalf("ensureRuntimeLayout() error = %v", err)
	}
	for _, path := range []string{
		filepath.Join(store, "data"),
		filepath.Join(store, "torrents"),
		archive,
		filepath.Join(root, "identities"),
		filepath.Join(root, "delegations"),
		filepath.Join(root, "revocations"),
		rules,
		writerPolicy,
		filepath.Join(root, "WriterWhitelist.inf"),
		filepath.Join(root, "WriterBlacklist.inf"),
		netPath,
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}
	data, err := os.ReadFile(rules)
	if err != nil {
		t.Fatalf("ReadFile(rules) error = %v", err)
	}
	if string(data) != defaultSubscriptionsJSON {
		t.Fatalf("unexpected rules content: %q", string(data))
	}
	writerData, err := os.ReadFile(writerPolicy)
	if err != nil {
		t.Fatalf("ReadFile(writerPolicy) error = %v", err)
	}
	if string(writerData) != defaultWriterPolicyJSON {
		t.Fatalf("unexpected writer policy content: %q", string(writerData))
	}
	whitelistData, err := os.ReadFile(filepath.Join(root, "WriterWhitelist.inf"))
	if err != nil {
		t.Fatalf("ReadFile(WriterWhitelist.inf) error = %v", err)
	}
	if string(whitelistData) != defaultWriterWhitelistINF {
		t.Fatalf("unexpected whitelist content: %q", string(whitelistData))
	}
	blacklistData, err := os.ReadFile(filepath.Join(root, "WriterBlacklist.inf"))
	if err != nil {
		t.Fatalf("ReadFile(WriterBlacklist.inf) error = %v", err)
	}
	if string(blacklistData) != defaultWriterBlacklistINF {
		t.Fatalf("unexpected blacklist content: %q", string(blacklistData))
	}
	netData, err := os.ReadFile(netPath)
	if err != nil {
		t.Fatalf("ReadFile(netPath) error = %v", err)
	}
	netText := string(netData)
	if !strings.Contains(netText, "libp2p_listen=/ip4/0.0.0.0/tcp/") {
		t.Fatalf("missing libp2p listen in net config: %q", netText)
	}
	if !strings.Contains(netText, "bittorrent_listen=0.0.0.0:") {
		t.Fatalf("missing bittorrent listen in net config: %q", netText)
	}
	if !strings.Contains(netText, "\nlan_peer=192.168.102.74") {
		t.Fatalf("missing default lan_peer in net config: %q", netText)
	}
	if !strings.Contains(netText, "\nlan_bt_peer=192.168.102.74") {
		t.Fatalf("missing default lan_bt_peer in net config: %q", netText)
	}
	if !strings.Contains(netText, "network_id="+latestOrgNetworkID) {
		t.Fatalf("missing hao.news network id in net config: %q", netText)
	}
	trackerData, err := os.ReadFile(filepath.Join(root, "Trackerlist.inf"))
	if err != nil {
		t.Fatalf("ReadFile(Trackerlist.inf) error = %v", err)
	}
	if !strings.Contains(string(trackerData), "udp://tracker.opentrackr.org:1337/announce") {
		t.Fatalf("missing default trackers in Trackerlist.inf: %q", string(trackerData))
	}
}

func TestEnsureRuntimeLayoutPreservesExistingRules(t *testing.T) {
	previous := buildDefaultLatestNetINF
	buildDefaultLatestNetINF = testDefaultLatestNetINF
	defer func() { buildDefaultLatestNetINF = previous }()

	root := t.TempDir()
	store := filepath.Join(root, "aip2p", ".aip2p")
	archive := filepath.Join(root, "archive")
	rules := filepath.Join(root, "subscriptions.json")
	writerPolicy := filepath.Join(root, "writer_policy.json")
	netPath := filepath.Join(root, "hao_news_net.inf")
	if err := os.MkdirAll(filepath.Dir(rules), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(rules, []byte("{\"topics\":[\"pc75\"]}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := ensureRuntimeLayout(store, archive, rules, writerPolicy, netPath); err != nil {
		t.Fatalf("ensureRuntimeLayout() error = %v", err)
	}
	data, err := os.ReadFile(rules)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "{\"topics\":[\"pc75\"]}\n" {
		t.Fatalf("rules file was overwritten: %q", string(data))
	}
}

func TestEnsureRuntimeLayoutMigratesLegacyWriterPolicyTemplate(t *testing.T) {
	previous := buildDefaultLatestNetINF
	buildDefaultLatestNetINF = testDefaultLatestNetINF
	defer func() { buildDefaultLatestNetINF = previous }()

	root := t.TempDir()
	store := filepath.Join(root, "aip2p", ".aip2p")
	archive := filepath.Join(root, "archive")
	rules := filepath.Join(root, "subscriptions.json")
	writerPolicy := filepath.Join(root, "writer_policy.json")
	netPath := filepath.Join(root, "hao_news_net.inf")
	if err := os.MkdirAll(filepath.Dir(writerPolicy), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(writerPolicy, []byte(legacyWriterPolicyJSONMixedAllowUnsigned), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := ensureRuntimeLayout(store, archive, rules, writerPolicy, netPath); err != nil {
		t.Fatalf("ensureRuntimeLayout() error = %v", err)
	}
	data, err := os.ReadFile(writerPolicy)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != defaultWriterPolicyJSON {
		t.Fatalf("writer policy was not migrated to current default: %q", string(data))
	}
}

func TestEnsureRuntimeLayoutPreservesCustomWriterPolicy(t *testing.T) {
	previous := buildDefaultLatestNetINF
	buildDefaultLatestNetINF = testDefaultLatestNetINF
	defer func() { buildDefaultLatestNetINF = previous }()

	root := t.TempDir()
	store := filepath.Join(root, "aip2p", ".aip2p")
	archive := filepath.Join(root, "archive")
	rules := filepath.Join(root, "subscriptions.json")
	writerPolicy := filepath.Join(root, "writer_policy.json")
	netPath := filepath.Join(root, "hao_news_net.inf")
	if err := os.MkdirAll(filepath.Dir(writerPolicy), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	custom := "{\n  \"sync_mode\": \"whitelist\",\n  \"allow_unsigned\": true,\n  \"default_capability\": \"read_only\"\n}\n"
	if err := os.WriteFile(writerPolicy, []byte(custom), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := ensureRuntimeLayout(store, archive, rules, writerPolicy, netPath); err != nil {
		t.Fatalf("ensureRuntimeLayout() error = %v", err)
	}
	data, err := os.ReadFile(writerPolicy)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var policy WriterPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if policy.SyncMode != WriterSyncModeWhitelist {
		t.Fatalf("sync_mode = %q, want %q", policy.SyncMode, WriterSyncModeWhitelist)
	}
	if policy.DefaultCapability != WriterCapabilityReadOnly {
		t.Fatalf("default_capability = %q, want %q", policy.DefaultCapability, WriterCapabilityReadOnly)
	}
	if policy.AllowUnsigned {
		t.Fatalf("allow_unsigned = %t, want false after forced upgrade", policy.AllowUnsigned)
	}
}

func TestEnsureRuntimeLayoutAppendsLatestNetworkIDToExistingNetConfig(t *testing.T) {
	previous := buildDefaultLatestNetINF
	buildDefaultLatestNetINF = testDefaultLatestNetINF
	defer func() { buildDefaultLatestNetINF = previous }()

	root := t.TempDir()
	store := filepath.Join(root, "aip2p", ".aip2p")
	archive := filepath.Join(root, "archive")
	rules := filepath.Join(root, "subscriptions.json")
	writerPolicy := filepath.Join(root, "writer_policy.json")
	netPath := filepath.Join(root, "hao_news_net.inf")
	if err := os.MkdirAll(filepath.Dir(netPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(netPath, []byte("libp2p_rendezvous=hao.news/global\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := ensureRuntimeLayout(store, archive, rules, writerPolicy, netPath); err != nil {
		t.Fatalf("ensureRuntimeLayout() error = %v", err)
	}
	data, err := os.ReadFile(netPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "network_id="+latestOrgNetworkID) {
		t.Fatalf("expected network_id append, got %q", string(data))
	}
}
