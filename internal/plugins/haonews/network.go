package newsplugin

import (
	"encoding/hex"
	"fmt"
	corehaonews "hao.news/internal/haonews"

	"os"
	"path/filepath"
	"strings"
)

const (
	networkModeLAN    = "lan"
	networkModePublic = "public"
	networkModeShared = "shared"
)

const networkIDFileName = "network_id.inf"

type NetworkBootstrapConfig struct {
	Path             string
	Exists           bool
	NetworkMode      string
	NetworkID        string
	LibP2PListen     []string
	Redis            corehaonews.RedisConfig
	LANPeers         []string
	PublicPeers      []string
	RelayPeers       []string
	LibP2PBootstrap  []string
	LibP2PRendezvous []string
}

func LoadNetworkBootstrapConfig(path string) (NetworkBootstrapConfig, error) {
	coreCfg, err := corehaonews.LoadNetworkBootstrapConfig(path)
	if err != nil {
		return NetworkBootstrapConfig{}, err
	}
	return NetworkBootstrapConfig{
		Path:             coreCfg.Path,
		Exists:           coreCfg.Exists,
		NetworkMode:      coreCfg.NetworkMode,
		NetworkID:        coreCfg.NetworkID,
		LibP2PListen:     append([]string(nil), coreCfg.LibP2PListen...),
		Redis:            coreCfg.Redis,
		LANPeers:         append([]string(nil), coreCfg.LANPeers...),
		PublicPeers:      append([]string(nil), coreCfg.PublicPeers...),
		RelayPeers:       append([]string(nil), coreCfg.RelayPeers...),
		LibP2PBootstrap:  append([]string(nil), coreCfg.LibP2PBootstrap...),
		LibP2PRendezvous: append([]string(nil), coreCfg.LibP2PRendezvous...),
	}, nil
}

func (c NetworkBootstrapConfig) AllowsLANDiscovery() bool {
	mode := normalizeNetworkMode(c.NetworkMode)
	return mode == "" || mode == networkModeLAN || mode == networkModeShared
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
