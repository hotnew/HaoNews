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
const networkIDFileName = "network_id.inf"

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
	return fmt.Sprintf(`# latest.org bootstrap configuration
# Plaintext file loaded by --net %s
#
# Supported keys:
#   network_mode=lan|public|shared
#   libp2p_listen=/ip4/.../tcp/<port>
#   lan_peer=<host-or-ip>
#   public_peer=<host-or-domain>
#   relay_peer=<host-or-domain>
#   libp2p_bootstrap=/dnsaddr/.../p2p/<peer-id>
#   libp2p_rendezvous=latest.org/<topic>
#   libp2p_transfer_max_size=<bytes>
#   redis_enabled=true|false
#   redis_addr=127.0.0.1:6379
#   redis_password=
#   redis_db=0
#   redis_key_prefix=haonews-
#   redis_max_retries=3
#   redis_dial_timeout_ms=3000
#   redis_read_timeout_ms=2000
#   redis_write_timeout_ms=2000
#   redis_pool_size=10
#   redis_min_idle_conns=2
#   redis_hot_window_days=7
#
# Generated on first start. Reuse these ports on later restarts unless you intentionally change them.
# Stable 256-bit network namespace now lives in %s beside this file.
network_mode=lan
libp2p_listen=/ip4/0.0.0.0/tcp/%d
libp2p_listen=/ip4/0.0.0.0/udp/%d/quic-v1
libp2p_transfer_max_size=%d

# Optional LAN anchor. Hao.News will query http://<lan_peer>:51818/api/network/bootstrap
# so a plain IP can become a dialable libp2p peer with the current peer_id and listen ports.
lan_peer=192.168.102.74
lan_peer=192.168.102.76
lan_peer=192.168.102.75

libp2p_bootstrap=/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN
libp2p_bootstrap=/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa
libp2p_bootstrap=/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb
libp2p_bootstrap=/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ
libp2p_rendezvous=latest.org/global
libp2p_rendezvous=latest.org/world

# Optional Redis hot cache. File storage remains authoritative.
# redis_enabled=true
# redis_addr=127.0.0.1:6379
# redis_password=
# redis_db=0
# redis_key_prefix=haonews-
# redis_max_retries=3
# redis_dial_timeout_ms=3000
# redis_read_timeout_ms=2000
# redis_write_timeout_ms=2000
# redis_pool_size=10
# redis_min_idle_conns=2
# redis_hot_window_days=7
`, path, networkIDFileName, libp2pPort, libp2pPort, defaultLibP2PTransferMaxSize), nil
}

type NetworkBootstrapConfig struct {
	Path                  string
	Exists                bool
	NetworkMode           string
	NetworkID             string
	LibP2PListen          []string
	LibP2PTransferMaxSize int64
	Redis                 RedisConfig
	LANPeers              []string
	PublicPeers           []string
	RelayPeers            []string
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
			return NetworkBootstrapConfig{Path: path, NetworkMode: networkModeLAN, Redis: DefaultRedisConfig()}, nil
		}
		return NetworkBootstrapConfig{}, err
	}
	cfg := NetworkBootstrapConfig{
		Path:   path,
		Exists: true,
		Redis:  DefaultRedisConfig(),
	}
	seenListen := make(map[string]struct{})
	seenLAN := make(map[string]struct{})
	seenPublic := make(map[string]struct{})
	seenRelay := make(map[string]struct{})
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
		case "redis_enabled", "redis_addr", "redis_password", "redis_db", "redis_key_prefix",
			"redis_max_retries", "redis_dial_timeout_ms", "redis_read_timeout_ms", "redis_write_timeout_ms",
			"redis_pool_size", "redis_min_idle_conns", "redis_hot_window_days":
			applyRedisConfigValue(&cfg.Redis, key, value)
		}
	}
	if cfg.NetworkMode == "" {
		cfg.NetworkMode = networkModeLAN
	}
	cfg.Redis = normalizeRedisConfig(cfg.Redis)
	fileNetworkID, err := loadNetworkIDFile(networkIDFilePath(path))
	if err != nil {
		return NetworkBootstrapConfig{}, err
	}
	if fileNetworkID != "" {
		cfg.NetworkID = fileNetworkID
	}
	return cfg, nil
}

func applyRedisConfigValue(cfg *RedisConfig, key, value string) {
	if cfg == nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "redis_enabled":
		cfg.Enabled = strings.EqualFold(strings.TrimSpace(value), "true") ||
			strings.EqualFold(strings.TrimSpace(value), "1") ||
			strings.EqualFold(strings.TrimSpace(value), "yes")
	case "redis_addr":
		cfg.Addr = strings.TrimSpace(value)
	case "redis_password":
		cfg.Password = value
	case "redis_db":
		if db, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && db >= 0 {
			cfg.DB = db
		}
	case "redis_key_prefix":
		cfg.KeyPrefix = strings.TrimSpace(value)
	case "redis_max_retries":
		if v, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && v > 0 {
			cfg.MaxRetries = v
		}
	case "redis_dial_timeout_ms":
		if v, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && v > 0 {
			cfg.DialTimeoutMs = v
		}
	case "redis_read_timeout_ms":
		if v, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && v > 0 {
			cfg.ReadTimeoutMs = v
		}
	case "redis_write_timeout_ms":
		if v, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && v > 0 {
			cfg.WriteTimeoutMs = v
		}
	case "redis_pool_size":
		if v, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && v > 0 {
			cfg.PoolSize = v
		}
	case "redis_min_idle_conns":
		if v, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && v >= 0 {
			cfg.MinIdleConns = v
		}
	case "redis_hot_window_days":
		if v, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && v > 0 {
			cfg.HotWindowDays = v
		}
	}
}

func (c NetworkBootstrapConfig) AllowsLANDiscovery() bool {
	mode := normalizeNetworkMode(c.NetworkMode)
	return mode == "" || mode == networkModeLAN || mode == networkModeShared
}

func (c NetworkBootstrapConfig) IsSharedMode() bool {
	return normalizeNetworkMode(c.NetworkMode) == networkModeShared
}

func (c NetworkBootstrapConfig) IsPublicMode() bool {
	return normalizeNetworkMode(c.NetworkMode) == networkModePublic
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
	inlineNetworkID, err := loadInlineNetworkID(path)
	if err != nil {
		return err
	}
	if inlineNetworkID != "" {
		networkID = inlineNetworkID
	}
	if err := ensureNetworkIDFile(networkIDFilePath(path), networkID); err != nil {
		return err
	}
	return stripInlineNetworkID(path)
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
	return nil
}

func networkIDFilePath(netPath string) string {
	netPath = strings.TrimSpace(netPath)
	if netPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(netPath), networkIDFileName)
}

func loadNetworkIDFile(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
			continue
		}
		if key, value, ok := strings.Cut(line, "="); ok {
			if strings.EqualFold(strings.TrimSpace(key), "network_id") {
				line = strings.TrimSpace(value)
			}
		}
		networkID := normalizeNetworkID(line)
		if networkID != "" {
			return networkID, nil
		}
		return "", fmt.Errorf("network_id could not be parsed from %s", filepath.Base(path))
	}
	return "", nil
}

func ensureNetworkIDFile(path, networkID string) error {
	path = strings.TrimSpace(path)
	networkID = normalizeNetworkID(networkID)
	if path == "" || networkID == "" {
		return nil
	}
	current, err := loadNetworkIDFile(path)
	if err != nil {
		return err
	}
	if current != "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content := "# Stable 256-bit Hao.News network namespace.\nnetwork_id=" + networkID + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

func loadInlineNetworkID(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), "network_id") {
			return normalizeNetworkID(strings.TrimSpace(value)), nil
		}
	}
	return "", nil
}

func stripInlineNetworkID(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	lines := strings.Split(string(data), "\n")
	filtered := make([]string, 0, len(lines))
	changed := false
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if key, _, ok := strings.Cut(line, "="); ok && strings.EqualFold(strings.TrimSpace(key), "network_id") {
			changed = true
			continue
		}
		filtered = append(filtered, rawLine)
	}
	if !changed {
		return nil
	}
	content := strings.TrimRight(strings.Join(filtered, "\n"), "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}
