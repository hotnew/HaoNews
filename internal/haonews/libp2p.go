package haonews

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	crypto "github.com/libp2p/go-libp2p/core/crypto"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	mdns "github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"
)

type libp2pRuntime struct {
	host               host.Host
	dht                *kaddht.IpfsDHT
	ping               *ping.PingService
	mdns               mdns.Service
	mdnsTracker        *mdnsTracker
	transfer           *bundleTransferProvider
	transferMaxSize    int64
	netCfg             NetworkBootstrapConfig
	networkID          string
	configuredListen   []string
	mdnsServiceName    string
	bootstraps         []peer.AddrInfo
	rendezvous         []string
	bootstrapWarning   string
	lastBootstrappedAt *time.Time
}

const (
	knownGoodLibP2PPeerTTL     = 7 * 24 * time.Hour
	knownGoodLibP2PRefresh     = 5 * time.Minute
	knownGoodLibP2PPeerMaxSize = 24
)

type knownGoodLibP2PPeerCache struct {
	NetworkID string                             `json:"network_id,omitempty"`
	Entries   map[string]knownGoodLibP2PPeerInfo `json:"entries,omitempty"`
}

type knownGoodLibP2PPeerInfo struct {
	LastSuccessAt time.Time `json:"last_success_at,omitempty"`
	Addrs         []string  `json:"addrs,omitempty"`
}

type KnownGoodLibP2PPeerStatus struct {
	PeerID        string     `json:"peer_id"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	Addrs         []string   `json:"addrs,omitempty"`
}

func startLibP2PRuntime(ctx context.Context, cfg NetworkBootstrapConfig, store *Store) (*libp2pRuntime, error) {
	knownGoodPeers, knownGoodErr := LoadKnownGoodLibP2PBootstrapPeers(cfg)
	if len(cfg.LibP2PBootstrap) == 0 && len(cfg.LibP2PRendezvous) == 0 && len(cfg.LANPeers) == 0 && len(knownGoodPeers) == 0 {
		return nil, nil
	}

	hostOptions := []libp2p.Option{libp2p.Ping(true)}
	hostKey, err := loadOrCreateLibP2PHostKey(cfg)
	if err != nil {
		return nil, fmt.Errorf("load libp2p host key: %w", err)
	}
	hostOptions = append(hostOptions, libp2p.Identity(hostKey))
	configuredListen := append([]string(nil), cfg.LibP2PListen...)
	if len(configuredListen) > 0 {
		resolvedListen, err := resolveLibP2PListenAddrs(configuredListen)
		if err != nil {
			return nil, fmt.Errorf("resolve libp2p listen addrs: %w", err)
		}
		hostOptions = append(hostOptions, libp2p.ListenAddrStrings(resolvedListen...))
	}
	h, err := libp2p.New(hostOptions...)
	if err != nil {
		return nil, fmt.Errorf("create libp2p host: %w", err)
	}

	resolvedLANPeers, lanErr := resolveLANBootstrapPeers(ctx, cfg)
	bootstrapValues := EffectiveLibP2PBootstrapPeersWithKnownGood(resolvedLANPeers, knownGoodPeers, cfg.LibP2PBootstrap)
	peers, err := parseBootstrapPeers(bootstrapValues)
	if err != nil {
		_ = h.Close()
		return nil, err
	}

	dhtOptions := []kaddht.Option{kaddht.Mode(kaddht.ModeAutoServer)}
	if len(peers) > 0 {
		dhtOptions = append(dhtOptions, kaddht.BootstrapPeers(peers...))
	}
	dht, err := kaddht.New(ctx, h, dhtOptions...)
	if err != nil {
		_ = h.Close()
		return nil, fmt.Errorf("create libp2p dht: %w", err)
	}
	if err := dht.Bootstrap(ctx); err != nil {
		_ = dht.Close()
		_ = h.Close()
		return nil, fmt.Errorf("bootstrap libp2p dht: %w", err)
	}
	mdnsTracker := newMDNSTracker(h)
	serviceName := mdnsServiceName(cfg.NetworkID)
	mdnsService := mdns.NewMdnsService(h, serviceName, mdnsTracker)
	if err := mdnsService.Start(); err != nil {
		_ = dht.Close()
		_ = h.Close()
		return nil, fmt.Errorf("start libp2p mdns: %w", err)
	}
	now := time.Now().UTC()
	transferMaxSize := effectiveLibP2PTransferMaxSize(cfg.LibP2PTransferMaxSize)
	return &libp2pRuntime{
		host:             h,
		dht:              dht,
		ping:             ping.NewPingService(h),
		mdns:             mdnsService,
		mdnsTracker:      mdnsTracker,
		transfer:         newBundleTransferProvider(h, store, transferMaxSize),
		transferMaxSize:  transferMaxSize,
		netCfg:           cfg,
		networkID:        cfg.NetworkID,
		configuredListen: configuredListen,
		mdnsServiceName:  serviceName,
		bootstraps:       peers,
		rendezvous:       append([]string(nil), cfg.LibP2PRendezvous...),
		bootstrapWarning: func() string {
			var warns []string
			if lanErr != nil {
				warns = append(warns, lanErr.Error())
			}
			if knownGoodErr != nil {
				warns = append(warns, "load known-good peers: "+knownGoodErr.Error())
			}
			return strings.Join(warns, "; ")
		}(),
		lastBootstrappedAt: &now,
	}, nil
}

func (r *libp2pRuntime) Close() error {
	if r == nil {
		return nil
	}
	if r.mdns != nil {
		_ = r.mdns.Close()
	}
	if r.transfer != nil {
		r.transfer.Close()
	}
	if r.dht != nil {
		_ = r.dht.Close()
	}
	if r.host != nil {
		return r.host.Close()
	}
	return nil
}

func (r *libp2pRuntime) Status(ctx context.Context) SyncLibP2PStatus {
	if r == nil {
		return SyncLibP2PStatus{}
	}

	status := SyncLibP2PStatus{
		Enabled:               true,
		PeerID:                r.host.ID().String(),
		ConfiguredListen:      append([]string(nil), r.configuredListen...),
		DirectTransferEnabled: r.transfer != nil,
		TransferMaxSize:       r.transferMaxSize,
		ConfiguredBootstrap:   len(r.bootstraps),
		ConfiguredRendezvous:  len(r.rendezvous),
		MDNS: SyncMDNSStatus{
			Enabled:     r.mdns != nil,
			ServiceName: r.mdnsServiceName,
		},
		LastBootstrapAt: r.lastBootstrappedAt,
	}
	if r.bootstrapWarning != "" {
		status.LastError = r.bootstrapWarning
	}
	for _, addr := range r.host.Addrs() {
		status.ListenAddrs = append(status.ListenAddrs, addr.String())
	}

	peerStates := make([]SyncPeerRef, 0, len(r.bootstraps))
	cache, cacheErr := loadKnownGoodLibP2PPeerCache(r.netCfg)
	cacheChanged := false
	for _, info := range r.bootstraps {
		state := SyncPeerRef{
			PeerID:  info.ID.String(),
			Address: firstPeerAddr(info),
		}
		if len(r.host.Network().ConnsToPeer(info.ID)) == 0 {
			connectCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			err := r.host.Connect(connectCtx, info)
			cancel()
			if err != nil {
				state.Error = err.Error()
				peerStates = append(peerStates, state)
				continue
			}
			now := time.Now().UTC()
			r.lastBootstrappedAt = &now
			status.LastBootstrapAt = r.lastBootstrappedAt
		}
		state.Connected = true
		status.ConnectedBootstrap++
		if cache != nil && cache.recordSuccess(info.ID.String(), collectKnownGoodLibP2PPeerAddrs(r.host, info), time.Now().UTC()) {
			cacheChanged = true
		}

		pingCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		result := <-r.ping.Ping(pingCtx, info.ID)
		cancel()
		if result.Error != nil {
			state.Error = result.Error.Error()
		} else {
			state.Reachable = true
			state.RTT = result.RTT.String()
			status.ReachableBootstrap++
		}
		peerStates = append(peerStates, state)
	}
	if cacheChanged {
		if err := saveKnownGoodLibP2PPeerCache(r.netCfg, cache); err != nil {
			cacheErr = err
		}
	}

	status.ConnectedPeers = len(r.host.Network().Peers())
	if r.dht != nil {
		status.RoutingTablePeers = r.dht.RoutingTable().Size()
	}
	status.Peers = peerStates
	if r.mdnsTracker != nil {
		status.MDNS = r.mdnsTracker.status(r.host)
	}
	if cacheErr != nil {
		if status.LastError != "" {
			status.LastError += "; "
		}
		status.LastError += "known-good peer cache: " + cacheErr.Error()
	}
	return status
}

func parseBootstrapPeers(values []string) ([]peer.AddrInfo, error) {
	out := make([]peer.AddrInfo, 0, len(values))
	seen := make(map[string]struct{})
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		info, err := peer.AddrInfoFromString(value)
		if err != nil {
			return nil, fmt.Errorf("parse libp2p bootstrap peer %q: %w", value, err)
		}
		key := info.ID.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, *info)
	}
	return out, nil
}

func EffectiveLibP2PBootstrapPeers(lanPeers, publicPeers []string) []string {
	return EffectiveLibP2PBootstrapPeersWithKnownGood(lanPeers, nil, publicPeers)
}

func EffectiveLibP2PBootstrapPeersWithKnownGood(lanPeers, knownGoodPeers, publicPeers []string) []string {
	values := make([]string, 0, len(lanPeers)+len(knownGoodPeers)+len(publicPeers))
	seen := make(map[string]struct{}, len(lanPeers)+len(publicPeers))
	for _, group := range [][]string{lanPeers, knownGoodPeers, publicPeers} {
		for _, value := range group {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			values = append(values, value)
		}
	}
	return values
}

func ResolveLANBootstrapPeers(ctx context.Context, cfg NetworkBootstrapConfig) ([]string, error) {
	return resolveLANBootstrapPeers(ctx, cfg)
}

func LoadKnownGoodLibP2PBootstrapPeers(cfg NetworkBootstrapConfig) ([]string, error) {
	return loadKnownGoodLibP2PBootstrapPeers(cfg, time.Now().UTC())
}

func ReadKnownGoodLibP2PPeerStatus(cfg NetworkBootstrapConfig) ([]KnownGoodLibP2PPeerStatus, error) {
	cache, err := loadKnownGoodLibP2PPeerCache(cfg)
	if err != nil {
		return nil, err
	}
	type cachedPeer struct {
		peerID string
		entry  knownGoodLibP2PPeerInfo
	}
	items := make([]cachedPeer, 0, len(cache.Entries))
	for peerID, entry := range cache.Entries {
		if strings.TrimSpace(peerID) == "" || entry.LastSuccessAt.IsZero() {
			continue
		}
		items = append(items, cachedPeer{
			peerID: peerID,
			entry:  entry,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].entry.LastSuccessAt.After(items[j].entry.LastSuccessAt)
	})
	out := make([]KnownGoodLibP2PPeerStatus, 0, len(items))
	for _, item := range items {
		ts := item.entry.LastSuccessAt
		out = append(out, KnownGoodLibP2PPeerStatus{
			PeerID:        item.peerID,
			LastSuccessAt: &ts,
			Addrs:         append([]string(nil), normalizeKnownGoodLibP2PPeerAddrs(item.peerID, item.entry.Addrs)...),
		})
	}
	return out, nil
}

func firstPeerAddr(info peer.AddrInfo) string {
	if len(info.Addrs) == 0 {
		return ""
	}
	return info.Addrs[0].String()
}

func mdnsServiceName(networkID string) string {
	networkID = normalizeNetworkID(networkID)
	if len(networkID) >= 12 {
		return "_haonews-" + networkID[:12] + "._udp"
	}
	return "_haonews._udp"
}

func knownGoodLibP2PPeerCachePath(cfg NetworkBootstrapConfig) string {
	if strings.TrimSpace(cfg.Path) == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(cfg.Path), "known_good_libp2p_peers.json")
}

func libp2pHostKeyPath(cfg NetworkBootstrapConfig) string {
	if strings.TrimSpace(cfg.Path) == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(cfg.Path), "libp2p_host.key")
}

func loadOrCreateLibP2PHostKey(cfg NetworkBootstrapConfig) (crypto.PrivKey, error) {
	path := libp2pHostKeyPath(cfg)
	if path == "" {
		priv, _, err := crypto.GenerateEd25519Key(nil)
		return priv, err
	}
	data, err := os.ReadFile(path)
	if err == nil {
		priv, err := crypto.UnmarshalPrivateKey(data)
		if err == nil {
			return priv, nil
		}
		backupPath := path + ".corrupt"
		_ = os.Rename(path, backupPath)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	priv, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		return nil, err
	}
	encoded, err := crypto.MarshalPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return nil, err
	}
	return priv, nil
}

func loadKnownGoodLibP2PPeerCache(cfg NetworkBootstrapConfig) (*knownGoodLibP2PPeerCache, error) {
	path := knownGoodLibP2PPeerCachePath(cfg)
	if path == "" {
		return &knownGoodLibP2PPeerCache{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &knownGoodLibP2PPeerCache{}, nil
		}
		return nil, err
	}
	var cache knownGoodLibP2PPeerCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	if cache.Entries == nil {
		cache.Entries = make(map[string]knownGoodLibP2PPeerInfo)
	}
	if normalizeNetworkID(cfg.NetworkID) != "" && cache.NetworkID != "" && cache.NetworkID != cfg.NetworkID {
		return &knownGoodLibP2PPeerCache{NetworkID: cfg.NetworkID, Entries: make(map[string]knownGoodLibP2PPeerInfo)}, nil
	}
	return &cache, nil
}

func saveKnownGoodLibP2PPeerCache(cfg NetworkBootstrapConfig, cache *knownGoodLibP2PPeerCache) error {
	path := knownGoodLibP2PPeerCachePath(cfg)
	if path == "" || cache == nil {
		return nil
	}
	if cache.Entries == nil {
		cache.Entries = make(map[string]knownGoodLibP2PPeerInfo)
	}
	if normalizeNetworkID(cfg.NetworkID) != "" {
		cache.NetworkID = cfg.NetworkID
	}
	trimKnownGoodLibP2PPeerCache(cache)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func loadKnownGoodLibP2PBootstrapPeers(cfg NetworkBootstrapConfig, now time.Time) ([]string, error) {
	cache, err := loadKnownGoodLibP2PPeerCache(cfg)
	if err != nil {
		return nil, err
	}
	type cachedPeer struct {
		peerID        string
		lastSuccessAt time.Time
		addrs         []string
	}
	peers := make([]cachedPeer, 0, len(cache.Entries))
	for peerID, entry := range cache.Entries {
		if strings.TrimSpace(peerID) == "" || entry.LastSuccessAt.IsZero() || now.Sub(entry.LastSuccessAt) > knownGoodLibP2PPeerTTL {
			continue
		}
		addrs := normalizeKnownGoodLibP2PPeerAddrs(peerID, entry.Addrs)
		if len(addrs) == 0 {
			continue
		}
		peers = append(peers, cachedPeer{
			peerID:        peerID,
			lastSuccessAt: entry.LastSuccessAt,
			addrs:         addrs,
		})
	}
	sort.SliceStable(peers, func(i, j int) bool {
		return peers[i].lastSuccessAt.After(peers[j].lastSuccessAt)
	})
	out := make([]string, 0, len(peers))
	seen := make(map[string]struct{}, len(peers))
	for _, item := range peers {
		for _, addr := range item.addrs {
			if _, ok := seen[addr]; ok {
				continue
			}
			seen[addr] = struct{}{}
			out = append(out, addr)
		}
	}
	return out, nil
}

func (c *knownGoodLibP2PPeerCache) recordSuccess(peerID string, addrs []string, now time.Time) bool {
	if c == nil || strings.TrimSpace(peerID) == "" {
		return false
	}
	if c.Entries == nil {
		c.Entries = make(map[string]knownGoodLibP2PPeerInfo)
	}
	addrs = normalizeKnownGoodLibP2PPeerAddrs(peerID, addrs)
	if len(addrs) == 0 {
		return false
	}
	prev := c.Entries[peerID]
	if prev.LastSuccessAt.Add(knownGoodLibP2PRefresh).After(now) && stringSliceEqual(prev.Addrs, addrs) {
		return false
	}
	c.Entries[peerID] = knownGoodLibP2PPeerInfo{
		LastSuccessAt: now,
		Addrs:         addrs,
	}
	return true
}

func trimKnownGoodLibP2PPeerCache(cache *knownGoodLibP2PPeerCache) {
	if cache == nil || len(cache.Entries) <= knownGoodLibP2PPeerMaxSize {
		return
	}
	type peerStamp struct {
		peerID string
		at     time.Time
	}
	items := make([]peerStamp, 0, len(cache.Entries))
	for peerID, entry := range cache.Entries {
		items = append(items, peerStamp{peerID: peerID, at: entry.LastSuccessAt})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].at.After(items[j].at)
	})
	keep := make(map[string]struct{}, knownGoodLibP2PPeerMaxSize)
	for i := 0; i < len(items) && i < knownGoodLibP2PPeerMaxSize; i++ {
		keep[items[i].peerID] = struct{}{}
	}
	for peerID := range cache.Entries {
		if _, ok := keep[peerID]; ok {
			continue
		}
		delete(cache.Entries, peerID)
	}
}

func collectKnownGoodLibP2PPeerAddrs(h host.Host, info peer.AddrInfo) []string {
	values := make([]string, 0, len(info.Addrs)+len(h.Peerstore().Addrs(info.ID)))
	for _, addr := range info.Addrs {
		values = append(values, addr.String())
	}
	for _, addr := range h.Peerstore().Addrs(info.ID) {
		values = append(values, addr.String())
	}
	return normalizeKnownGoodLibP2PPeerAddrs(info.ID.String(), values)
}

func normalizeKnownGoodLibP2PPeerAddrs(peerID string, values []string) []string {
	peerID = strings.TrimSpace(peerID)
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !strings.Contains(value, "/p2p/") && peerID != "" {
			value += "/p2p/" + peerID
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func stringSliceEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

type mdnsPeerState struct {
	SyncPeerRef
	LastSeen time.Time
}

type mdnsTracker struct {
	host        host.Host
	mu          sync.RWMutex
	lastError   string
	lastFoundAt *time.Time
	peers       map[string]mdnsPeerState
}

func newMDNSTracker(h host.Host) *mdnsTracker {
	return &mdnsTracker{
		host:  h,
		peers: make(map[string]mdnsPeerState),
	}
}

func (m *mdnsTracker) HandlePeerFound(info peer.AddrInfo) {
	if info.ID == m.host.ID() {
		return
	}
	now := time.Now().UTC()
	state := mdnsPeerState{
		SyncPeerRef: SyncPeerRef{
			PeerID:  info.ID.String(),
			Address: firstPeerAddr(info),
		},
		LastSeen: now,
	}

	connectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err := m.host.Connect(connectCtx, info)
	cancel()
	if err != nil {
		state.Error = err.Error()
	} else {
		state.Connected = true
		state.Reachable = true
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if prev, ok := m.peers[state.PeerID]; ok && state.Address == "" {
		state.Address = prev.Address
	}
	m.peers[state.PeerID] = state
	m.lastFoundAt = &now
	if err != nil {
		m.lastError = err.Error()
	}
}

func (m *mdnsTracker) status(h host.Host) SyncMDNSStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := SyncMDNSStatus{
		Enabled:          true,
		ServiceName:      "_haonews._udp",
		DiscoveredPeers:  len(m.peers),
		LastDiscoveredAt: m.lastFoundAt,
		LastError:        m.lastError,
		Peers:            make([]SyncPeerRef, 0, len(m.peers)),
	}
	for _, state := range m.peers {
		ref := state.SyncPeerRef
		if len(h.Network().ConnsToPeer(peer.ID(state.PeerID))) > 0 {
			ref.Connected = true
			ref.Reachable = true
			status.ConnectedPeers++
		}
		status.Peers = append(status.Peers, ref)
	}
	return status
}
