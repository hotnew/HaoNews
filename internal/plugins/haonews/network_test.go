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
network_id=2c2d6cf7b255ba20d6ad01135654933851b02bd00c65c2a6a54b97ab56590475
bittorrent_listen=0.0.0.0:51413
libp2p_listen=/ip4/0.0.0.0/tcp/4001
libp2p_listen=/ip4/0.0.0.0/udp/4001/quic-v1
lan_peer=192.168.102.74
lan_bt_peer=192.168.102.74
dht_router=router.bittorrent.com:6881
dht_router=router.utorrent.com:6881
dht_router=router.bittorrent.com:6881
libp2p_bootstrap=/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN
libp2p_bootstrap=/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa
libp2p_rendezvous=hao.news/global
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write net config: %v", err)
	}

	cfg, err := LoadNetworkBootstrapConfig(path)
	if err != nil {
		t.Fatalf("load network config: %v", err)
	}
	if got := len(cfg.DHTRouters); got != 2 {
		t.Fatalf("dht routers = %d, want 2", got)
	}
	if got := len(cfg.LibP2PBootstrap); got != 2 {
		t.Fatalf("libp2p bootstrap = %d, want 2", got)
	}
	if got := len(cfg.LibP2PRendezvous); got != 1 {
		t.Fatalf("libp2p rendezvous = %d, want 1", got)
	}
	if cfg.BitTorrentListen != "0.0.0.0:51413" {
		t.Fatalf("bittorrent listen = %q", cfg.BitTorrentListen)
	}
	if cfg.NetworkID != latestOrgNetworkID {
		t.Fatalf("network id = %q", cfg.NetworkID)
	}
	if got := len(cfg.LibP2PListen); got != 2 {
		t.Fatalf("libp2p listen = %d, want 2", got)
	}
	if got := len(cfg.LANPeers); got != 1 {
		t.Fatalf("lan peers = %d, want 1", got)
	}
	if got := len(cfg.LANTorrentPeers); got != 1 {
		t.Fatalf("lan bt peers = %d, want 1", got)
	}
	if cfg.FileName() != "hao_news_net.inf" {
		t.Fatalf("file name = %q, want hao_news_net.inf", cfg.FileName())
	}
}

func TestLoadNetworkBootstrapConfigMissingFileReturnsEmpty(t *testing.T) {
	t.Parallel()

	cfg, err := LoadNetworkBootstrapConfig(filepath.Join(t.TempDir(), "missing.inf"))
	if err != nil {
		t.Fatalf("load network config: %v", err)
	}
	if len(cfg.DHTRouters) != 0 || len(cfg.LibP2PBootstrap) != 0 || len(cfg.LibP2PRendezvous) != 0 {
		t.Fatalf("unexpected bootstrap entries: %+v", cfg)
	}
}
