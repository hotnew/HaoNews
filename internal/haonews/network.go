package haonews

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const latestOrgNetworkID = "b2090347cee0ff1a577b1101d4adbd664c309932d3c2578971c11997fdd2164e"
const defaultLANPeer = "192.168.102.74"

const (
	networkModeLAN    = "lan"
	networkModePublic = "public"
	networkModeShared = "shared"
)

func defaultNetworkBootstrapConfig(path string) (string, error) {
	libp2pPort, err := pickFreeTCPAndUDPPort()
	if err != nil {
		return "", err
	}
	bitTorrentPort, err := pickFreeTCPPort()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`# latest.org bootstrap configuration
# Plaintext file loaded by --net %s
#
# Supported keys:
#   network_mode=lan|public|shared
#   network_id=<64 hex chars>
#   libp2p_listen=/ip4/.../tcp/<port>
#   bittorrent_listen=0.0.0.0:<port>
#   lan_peer=<host-or-ip>
#   lan_bt_peer=<host-or-ip>
#   public_peer=<host-or-domain>
#   relay_peer=<host-or-domain>
#   libp2p_bootstrap=/dnsaddr/.../p2p/<peer-id>
#   libp2p_rendezvous=latest.org/<topic>
#   libp2p_transfer_max_size=<bytes>
#   dht_router=host:port
#
# Generated on first start. Reuse these ports on later restarts unless you intentionally change them.
network_mode=lan
network_id=%s
libp2p_listen=/ip4/0.0.0.0/tcp/%d
libp2p_listen=/ip4/0.0.0.0/udp/%d/quic-v1
libp2p_transfer_max_size=%d
bittorrent_listen=0.0.0.0:%d

# Optional LAN anchor. Hao.News will query http://<lan_peer>:51818/api/network/bootstrap
# so a plain IP can become a dialable libp2p peer with the current peer_id and listen ports.
lan_peer=192.168.102.74
lan_peer=192.168.102.76
lan_peer=192.168.102.75

# Optional LAN BitTorrent/DHT anchor. Hao.News will query the same bootstrap endpoint and
# reuse the current bittorrent_listen port from that peer as a LAN-local BT/DHT starting node.
lan_bt_peer=192.168.102.74
lan_bt_peer=192.168.102.76
lan_bt_peer=192.168.102.75

libp2p_bootstrap=/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN
libp2p_bootstrap=/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa
libp2p_bootstrap=/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb
libp2p_bootstrap=/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ
libp2p_rendezvous=latest.org/global
libp2p_rendezvous=latest.org/world

dht_router=router.bittorrent.com:6881
dht_router=router.utorrent.com:6881
dht_router=dht.transmissionbt.com:6881
`, path, latestOrgNetworkID, libp2pPort, libp2pPort, defaultLibP2PTransferMaxSize, bitTorrentPort), nil
}

type NetworkBootstrapConfig struct {
	Path                  string
	Exists                bool
	NetworkMode           string
	NetworkID             string
	BitTorrentListen      string
	LibP2PListen          []string
	LibP2PTransferMaxSize int64
	LANPeers              []string
	LANTorrentPeers       []string
	PublicPeers           []string
	RelayPeers            []string
	DHTRouters            []string
	LibP2PBootstrap       []string
	LibP2PRendezvous      []string
}

func LoadNetworkBootstrapConfig(path string) (NetworkBootstrapConfig, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return NetworkBootstrapConfig{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NetworkBootstrapConfig{Path: path, NetworkMode: networkModeLAN}, nil
		}
		return NetworkBootstrapConfig{}, err
	}
	cfg := NetworkBootstrapConfig{
		Path:   path,
		Exists: true,
	}
	seenListen := make(map[string]struct{})
	seenLAN := make(map[string]struct{})
	seenLANTorrent := make(map[string]struct{})
	seenPublic := make(map[string]struct{})
	seenRelay := make(map[string]struct{})
	seenDHT := make(map[string]struct{})
	seenLibP2P := make(map[string]struct{})
	seenRendezvous := make(map[string]struct{})
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		switch key {
		case "network_mode":
			if cfg.NetworkMode == "" {
				cfg.NetworkMode = normalizeNetworkMode(value)
			}
		case "network_id":
			if cfg.NetworkID == "" {
				cfg.NetworkID = normalizeNetworkID(value)
			}
		case "bittorrent_listen", "bt_listen":
			if cfg.BitTorrentListen == "" {
				cfg.BitTorrentListen = value
			}
		case "libp2p_listen":
			if _, ok := seenListen[value]; ok {
				continue
			}
			seenListen[value] = struct{}{}
			cfg.LibP2PListen = append(cfg.LibP2PListen, value)
		case "libp2p_transfer_max_size":
			size, err := strconv.ParseInt(value, 10, 64)
			if err != nil || size <= 0 {
				continue
			}
			cfg.LibP2PTransferMaxSize = size
		case "lan_peer":
			if _, ok := seenLAN[value]; ok {
				continue
			}
			seenLAN[value] = struct{}{}
			cfg.LANPeers = append(cfg.LANPeers, value)
		case "lan_bt_peer", "lan_torrent_peer", "lan_dht_peer":
			if _, ok := seenLANTorrent[value]; ok {
				continue
			}
			seenLANTorrent[value] = struct{}{}
			cfg.LANTorrentPeers = append(cfg.LANTorrentPeers, value)
		case "public_peer", "public_http_peer", "public_sync_peer":
			if _, ok := seenPublic[value]; ok {
				continue
			}
			seenPublic[value] = struct{}{}
			cfg.PublicPeers = append(cfg.PublicPeers, value)
		case "relay_peer":
			if _, ok := seenRelay[value]; ok {
				continue
			}
			seenRelay[value] = struct{}{}
			cfg.RelayPeers = append(cfg.RelayPeers, value)
		case "dht_router":
			if _, ok := seenDHT[value]; ok {
				continue
			}
			seenDHT[value] = struct{}{}
			cfg.DHTRouters = append(cfg.DHTRouters, value)
		case "libp2p_bootstrap":
			if _, ok := seenLibP2P[value]; ok {
				continue
			}
			seenLibP2P[value] = struct{}{}
			cfg.LibP2PBootstrap = append(cfg.LibP2PBootstrap, value)
		case "libp2p_rendezvous", "rendezvous":
			if _, ok := seenRendezvous[value]; ok {
				continue
			}
			seenRendezvous[value] = struct{}{}
			cfg.LibP2PRendezvous = append(cfg.LibP2PRendezvous, value)
		}
	}
	if cfg.NetworkMode == "" {
		cfg.NetworkMode = networkModeLAN
	}
	return cfg, nil
}

func (c NetworkBootstrapConfig) AllowsLANDiscovery() bool {
	mode := normalizeNetworkMode(c.NetworkMode)
	return mode == "" || mode == networkModeLAN || mode == networkModeShared
}

func effectiveLibP2PTransferMaxSize(value int64) int64 {
	if value <= 0 {
		return defaultLibP2PTransferMaxSize
	}
	return value
}

func EnsureDefaultNetworkBootstrapConfig(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	content, err := defaultNetworkBootstrapConfig(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}
	return nil
}

func (c NetworkBootstrapConfig) FileName() string {
	if c.Path == "" {
		return ""
	}
	return filepath.Base(c.Path)
}

func normalizeNetworkID(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if len(value) != 64 {
		return ""
	}
	if _, err := hex.DecodeString(value); err != nil {
		return ""
	}
	return value
}

func normalizeNetworkMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case networkModePublic:
		return networkModePublic
	case networkModeShared:
		return networkModeShared
	case networkModeLAN:
		return networkModeLAN
	default:
		return ""
	}
}

func ensureNetworkID(path, networkID string) error {
	path = strings.TrimSpace(path)
	networkID = normalizeNetworkID(networkID)
	if path == "" || networkID == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	cfg, err := LoadNetworkBootstrapConfig(path)
	if err != nil {
		return err
	}
	if cfg.NetworkID != "" {
		return nil
	}
	body := strings.TrimRight(string(data), "\n")
	body += "\n\n# Stable 256-bit Hao.News network namespace for latest.org.\n"
	body += "network_id=" + networkID + "\n"
	return os.WriteFile(path, []byte(body), 0o644)
}

func ensureLANPeer(path, lanPeer string) error {
	path = strings.TrimSpace(path)
	lanPeer = strings.TrimSpace(lanPeer)
	if path == "" || lanPeer == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	cfg, err := LoadNetworkBootstrapConfig(path)
	if err != nil {
		return err
	}
	if !cfg.AllowsLANDiscovery() {
		return nil
	}
	if len(cfg.LANPeers) > 0 {
		return nil
	}
	body := strings.TrimRight(string(data), "\n")
	body += "\n\n# Optional LAN anchor for faster local discovery.\n"
	body += "lan_peer=" + lanPeer + "\n"
	return os.WriteFile(path, []byte(body), 0o644)
}

func ensureLANTorrentPeer(path, lanPeer string) error {
	path = strings.TrimSpace(path)
	lanPeer = strings.TrimSpace(lanPeer)
	if path == "" || lanPeer == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	cfg, err := LoadNetworkBootstrapConfig(path)
	if err != nil {
		return err
	}
	if !cfg.AllowsLANDiscovery() {
		return nil
	}
	if len(cfg.LANTorrentPeers) > 0 {
		return nil
	}
	body := strings.TrimRight(string(data), "\n")
	body += "\n\n# Optional LAN BitTorrent/DHT anchor for faster local backfill.\n"
	body += "lan_bt_peer=" + lanPeer + "\n"
	return os.WriteFile(path, []byte(body), 0o644)
}
