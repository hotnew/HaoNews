package newsplugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadNetworkBootstrapConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "hao_news_net.inf")
	content := `# default bootstrap nodes
network_mode=public
libp2p_listen=/ip4/0.0.0.0/tcp/4001
libp2p_listen=/ip4/0.0.0.0/udp/4001/quic-v1
lan_peer=192.168.102.74
lan_peer=192.168.102.76
lan_peer=192.168.102.75
public_peer=ai.jie.news
relay_peer=relay.jie.news
libp2p_bootstrap=/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN
libp2p_bootstrap=/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa
libp2p_rendezvous=hao.news/global
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write net config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, networkIDFileName), []byte("network_id="+latestOrgNetworkID+"\n"), 0o644); err != nil {
		t.Fatalf("write network id file: %v", err)
	}

	cfg, err := LoadNetworkBootstrapConfig(path)
	if err != nil {
		t.Fatalf("load network config: %v", err)
	}
	if got := len(cfg.LibP2PBootstrap); got != 2 {
		t.Fatalf("libp2p bootstrap = %d, want 2", got)
	}
	if got := len(cfg.LibP2PRendezvous); got != 1 {
		t.Fatalf("libp2p rendezvous = %d, want 1", got)
	}
	if cfg.NetworkID != latestOrgNetworkID {
		t.Fatalf("network id = %q", cfg.NetworkID)
	}
	if cfg.NetworkMode != networkModePublic {
		t.Fatalf("network mode = %q, want public", cfg.NetworkMode)
	}
	if got := len(cfg.LibP2PListen); got != 2 {
		t.Fatalf("libp2p listen = %d, want 2", got)
	}
	if got := len(cfg.LANPeers); got != 3 {
		t.Fatalf("lan peers = %d, want 3", got)
	}
	if got := len(cfg.PublicPeers); got != 1 || cfg.PublicPeers[0] != "ai.jie.news" {
		t.Fatalf("public peers = %#v", cfg.PublicPeers)
	}
	if cfg.Redis.Enabled {
		t.Fatalf("redis should default disabled, got %+v", cfg.Redis)
	}
	if got := len(cfg.RelayPeers); got != 1 || cfg.RelayPeers[0] != "relay.jie.news" {
		t.Fatalf("relay peers = %#v", cfg.RelayPeers)
	}
	if cfg.FileName() != "hao_news_net.inf" {
		t.Fatalf("file name = %q, want hao_news_net.inf", cfg.FileName())
	}
	if cfg.AllowsLANDiscovery() {
		t.Fatal("public mode should not allow implicit LAN discovery")
	}
}

func TestLoadNetworkBootstrapConfigReadsInlineNetworkIDAsFallback(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "hao_news_net.inf")
	content := "network_mode=shared\nnetwork_id=" + latestOrgNetworkID + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write net config: %v", err)
	}

	cfg, err := LoadNetworkBootstrapConfig(path)
	if err != nil {
		t.Fatalf("load network config: %v", err)
	}
	if cfg.NetworkID != latestOrgNetworkID {
		t.Fatalf("network id = %q", cfg.NetworkID)
	}
}

func TestLoadNetworkBootstrapConfigMissingFileReturnsEmpty(t *testing.T) {
	t.Parallel()

	cfg, err := LoadNetworkBootstrapConfig(filepath.Join(t.TempDir(), "missing.inf"))
	if err != nil {
		t.Fatalf("load network config: %v", err)
	}
	if len(cfg.LibP2PBootstrap) != 0 || len(cfg.LibP2PRendezvous) != 0 {
		t.Fatalf("unexpected bootstrap entries: %+v", cfg)
	}
	if cfg.NetworkMode != networkModeLAN {
		t.Fatalf("default network mode = %q, want lan", cfg.NetworkMode)
	}
}

func TestLoadNetworkBootstrapConfigReadsRedisConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "hao_news_net.inf")
	content := "network_mode=lan\nredis_enabled=true\nredis_addr=127.0.0.1:6380\nredis_db=2\nredis_key_prefix=haonews-redis:\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write net config: %v", err)
	}

	cfg, err := LoadNetworkBootstrapConfig(path)
	if err != nil {
		t.Fatalf("load network config: %v", err)
	}
	if !cfg.Redis.Enabled {
		t.Fatalf("redis should be enabled: %+v", cfg.Redis)
	}
	if cfg.Redis.Addr != "127.0.0.1:6380" {
		t.Fatalf("redis addr = %q", cfg.Redis.Addr)
	}
	if cfg.Redis.DB != 2 {
		t.Fatalf("redis db = %d", cfg.Redis.DB)
	}
	if cfg.Redis.KeyPrefix != "haonews-redis:" {
		t.Fatalf("redis key prefix = %q", cfg.Redis.KeyPrefix)
	}
}
