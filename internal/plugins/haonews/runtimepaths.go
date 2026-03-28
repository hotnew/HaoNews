package newsplugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const reservedTopicAll = "all"
const latestOrgNetworkID = "2c2d6cf7b255ba20d6ad01135654933851b02bd00c65c2a6a54b97ab56590475"
const defaultLANPeer = "192.168.102.74"
const projectSyncBinaryName = "hao-news-syncd"
const defaultWriterWhitelistINF = "# WriterWhitelist.inf\n# One writer entry per line. Lines starting with #, ;, or // are ignored.\n# Supported forms:\n# agent://news/publisher-01\n# agent_id=agent://news/editor-02\n# public_key=aaaaaaaa...\n"
const defaultWriterBlacklistINF = "# WriterBlacklist.inf\n# One writer entry per line. Lines starting with #, ;, or // are ignored.\n# Supported forms:\n# agent://spam/bot-99\n# agent_id=agent://spam/bot-99\n# public_key=deadbeef...\n"

const defaultSubscriptionsJSON = "{\n  \"channels\": [],\n  \"topics\": [\"all\"],\n  \"tags\": [],\n  \"whitelist_mode\": \"strict\",\n  \"approval_feed\": \"pending-approval\",\n  \"auto_route_pending\": false,\n  \"discovery_feeds\": [\"global\", \"news\"],\n  \"discovery_topics\": [],\n  \"topic_whitelist\": [\"world\", \"news\", \"futures\"],\n  \"topic_aliases\": {\n    \"世界\": \"world\",\n    \"国际\": \"world\",\n    \"新闻\": \"news\",\n    \"期货\": \"futures\"\n  },\n  \"max_age_days\": 99999999,\n  \"max_bundle_mb\": 10,\n  \"max_items_per_day\": 999999999999\n}\n"
const defaultWriterPolicyJSON = "{\n  \"sync_mode\": \"all\",\n  \"trust_mode\": \"exact\",\n  \"allow_unsigned\": false,\n  \"default_capability\": \"read_write\",\n  \"trusted_authorities\": {},\n  \"shared_registries\": [],\n  \"relay_default_trust\": \"neutral\",\n  \"relay_peer_trust\": {},\n  \"relay_host_trust\": {},\n  \"agent_capabilities\": {},\n  \"public_key_capabilities\": {},\n  \"allowed_agent_ids\": [],\n  \"allowed_public_keys\": [],\n  \"blocked_agent_ids\": [],\n  \"blocked_public_keys\": []\n}\n"
const legacyWriterPolicyJSONMixedAllowUnsigned = "{\n  \"sync_mode\": \"mixed\",\n  \"allow_unsigned\": true,\n  \"default_capability\": \"read_write\",\n  \"trusted_authorities\": {},\n  \"shared_registries\": [],\n  \"relay_default_trust\": \"neutral\",\n  \"relay_peer_trust\": {},\n  \"relay_host_trust\": {},\n  \"agent_capabilities\": {},\n  \"public_key_capabilities\": {},\n  \"allowed_agent_ids\": [],\n  \"allowed_public_keys\": [],\n  \"blocked_agent_ids\": [],\n  \"blocked_public_keys\": []\n}\n"
const legacyWriterPolicyJSONAllAllowUnsigned = "{\n  \"sync_mode\": \"all\",\n  \"allow_unsigned\": true,\n  \"default_capability\": \"read_write\",\n  \"trusted_authorities\": {},\n  \"shared_registries\": [],\n  \"relay_default_trust\": \"neutral\",\n  \"relay_peer_trust\": {},\n  \"relay_host_trust\": {},\n  \"agent_capabilities\": {},\n  \"public_key_capabilities\": {},\n  \"allowed_agent_ids\": [],\n  \"allowed_public_keys\": [],\n  \"blocked_agent_ids\": [],\n  \"blocked_public_keys\": []\n}\n"

var buildDefaultLatestNetINF = defaultLatestNetINF

func defaultLatestNetINF() (string, error) {
	libp2pPort, err := pickFreeTCPAndUDPPort()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`# Hao.News News bootstrap configuration
# Plaintext file loaded by --net ~/.hao-news/hao_news_net.inf
#
# Supported keys:
#   network_mode=lan|public|shared
#   network_id=<64 hex chars>
#   libp2p_listen=/ip4/.../tcp/<port>
#   lan_peer=<host-or-ip>
#   public_peer=<host-or-domain>
#   relay_peer=<host-or-domain>
#   libp2p_bootstrap=/dnsaddr/.../p2p/<peer-id>
#   libp2p_rendezvous=hao.news/<topic>
#
# Generated on first start. Reuse these ports on later restarts unless you intentionally change them.
network_mode=lan
network_id=%s
libp2p_listen=/ip4/0.0.0.0/tcp/%d
libp2p_listen=/ip4/0.0.0.0/udp/%d/quic-v1

# Default LAN anchor. This matches the reference latest.org setup and gives
# Hao.News Public uses the same shared LAN libp2p entrypoint by default.
lan_peer=192.168.102.74
lan_peer=192.168.102.76
lan_peer=192.168.102.75

# Public libp2p helper write-back section. After the public helper node is
# deployed, replace <peer-id> and uncomment these entries.
# libp2p_bootstrap=/dns4/free001.haonews.org/tcp/4001/p2p/<peer-id>
# libp2p_bootstrap=/dns4/free001.haonews.org/udp/4001/quic-v1/p2p/<peer-id>

# hao.news should treat libp2p as the primary control plane for discovery and subscriptions.
libp2p_bootstrap=/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN
libp2p_bootstrap=/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa
libp2p_bootstrap=/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb
libp2p_bootstrap=/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ
libp2p_rendezvous=hao.news/global
libp2p_rendezvous=hao.news/world
`, latestOrgNetworkID, libp2pPort, libp2pPort), nil
}

type RuntimePaths struct {
	Root                string
	BinRoot             string
	IdentitiesRoot      string
	DelegationsRoot     string
	RevocationsRoot     string
	StoreRoot           string
	ArchiveRoot         string
	RulesPath           string
	WriterPolicyPath    string
	WriterWhitelistPath string
	WriterBlacklistPath string
	NetPath             string
	StatusPath          string
	MagnetsPath         string
	SyncLogPath         string
	SyncBinPath         string
	SupervisorStatePath string
}

func DefaultRuntimePaths() (RuntimePaths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return RuntimePaths{}, err
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return RuntimePaths{}, errors.New("user home directory is empty")
	}
	return DefaultRuntimePathsFromHome(home), nil
}

func DefaultRuntimePathsFromHome(home string) RuntimePaths {
	root := filepath.Join(strings.TrimSpace(home), ".hao-news")
	return RuntimePathsFromRoot(root)
}

func RuntimePathsFromRoot(root string) RuntimePaths {
	root = strings.TrimSpace(root)
	if root == "" {
		root = ".hao-news"
	}
	storeRoot := filepath.Join(root, "haonews", ".haonews")
	binRoot := filepath.Join(root, "bin")
	return RuntimePaths{
		Root:                root,
		BinRoot:             binRoot,
		IdentitiesRoot:      filepath.Join(root, "identities"),
		DelegationsRoot:     filepath.Join(root, "delegations"),
		RevocationsRoot:     filepath.Join(root, "revocations"),
		StoreRoot:           storeRoot,
		ArchiveRoot:         filepath.Join(root, "archive"),
		RulesPath:           filepath.Join(root, "subscriptions.json"),
		WriterPolicyPath:    filepath.Join(root, "writer_policy.json"),
		WriterWhitelistPath: filepath.Join(root, writerWhitelistINFName),
		WriterBlacklistPath: filepath.Join(root, writerBlacklistINFName),
		NetPath:             filepath.Join(root, "hao_news_net.inf"),
		StatusPath:          filepath.Join(storeRoot, "sync", "status.json"),
		MagnetsPath:         filepath.Join(storeRoot, "sync", "magnets.txt"),
		SyncLogPath:         filepath.Join(root, "hao-news-sync.log"),
		SyncBinPath:         filepath.Join(binRoot, projectSyncBinaryName+platformExecutableSuffix()),
		SupervisorStatePath: filepath.Join(root, "sync-supervisor.json"),
	}
}

func platformExecutableSuffix() string {
	if os.PathSeparator == '\\' {
		return ".exe"
	}
	return ""
}

func ProjectSyncBinaryNameForLogs() string {
	return projectSyncBinaryName
}

func ensureFileIfMissing(path string, content []byte) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func ensureWriterPolicyFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return os.WriteFile(path, []byte(defaultWriterPolicyJSON), 0o644)
	}
	if err != nil {
		return err
	}
	text := string(data)
	switch text {
	case legacyWriterPolicyJSONMixedAllowUnsigned, legacyWriterPolicyJSONAllAllowUnsigned:
		return os.WriteFile(path, []byte(defaultWriterPolicyJSON), 0o644)
	}
	var policy WriterPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		return nil
	}
	if !policy.AllowUnsigned {
		return nil
	}
	policy.AllowUnsigned = false
	policy.normalize()
	updated, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return err
	}
	updated = append(updated, '\n')
	return os.WriteFile(path, updated, 0o644)
}

func appendNetworkIDIfMissing(path, networkID string) error {
	path = strings.TrimSpace(path)
	networkID = normalizeNetworkID(networkID)
	if path == "" || networkID == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
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
	body += "\n\n# Stable 256-bit Hao.News network namespace for hao.news.\n"
	body += "network_id=" + networkID + "\n"
	return os.WriteFile(path, []byte(body), 0o644)
}

func appendLANPeerIfMissing(path, lanPeer string) error {
	path = strings.TrimSpace(path)
	lanPeer = strings.TrimSpace(lanPeer)
	if path == "" || lanPeer == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
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

func appendLANTorrentPeerIfMissing(path, lanPeer string) error {
	path = strings.TrimSpace(path)
	lanPeer = strings.TrimSpace(lanPeer)
	if path == "" || lanPeer == "" {
		return nil
	}
	if _, err := os.ReadFile(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
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
	return nil
}
