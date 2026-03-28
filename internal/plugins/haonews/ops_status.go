package newsplugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	corehaonews "hao.news/internal/haonews"
)

const nodeStatusCacheTTL = 3 * time.Second

func (a *App) nodeStatus(index Index) NodeStatus {
	now := time.Now()
	a.nodeStatusMu.Lock()
	if a.nodeStatusCache.ready && now.Before(a.nodeStatusCache.expiresAt) {
		status := a.nodeStatusCache.status
		a.nodeStatusMu.Unlock()
		return status
	}
	a.nodeStatusMu.Unlock()

	status := a.buildNodeStatus(index)

	a.nodeStatusMu.Lock()
	a.nodeStatusCache = cachedNodeStatusState{
		status:    status,
		expiresAt: now.Add(nodeStatusCacheTTL),
		ready:     true,
	}
	a.nodeStatusMu.Unlock()
	return status
}

func (a *App) buildNodeStatus(index Index) NodeStatus {
	storeState := "ready"
	storeTone := "good"
	if _, err := os.Stat(filepath.Join(a.storeRoot, "data")); err != nil {
		storeState = "missing"
		storeTone = "warn"
	}
	torrentCount := 0
	store := &corehaonews.Store{TorrentDir: filepath.Join(a.storeRoot, "torrents")}
	if count, err := store.TorrentCount(); err == nil {
		torrentCount = count
	}
	netCfg, netErr := a.networkBootstrap()
	syncStatus, syncErr := a.syncRuntimeStatus()

	if syncErr == nil && !syncStatus.UpdatedAt.IsZero() {
		return buildLiveNodeStatus(index, storeState, storeTone, torrentCount, netCfg, syncStatus, a.httpListenAddr())
	}

	discoveryValue := "not loaded"
	discoveryTone := "warn"
	discoveryDetail := "Add hao_news_net.inf to declare bootstrap peers and routers."
	if netErr != nil {
		discoveryValue = "config error"
		discoveryTone = "bad"
		discoveryDetail = "hao_news_net.inf could not be read."
	} else if netCfg.Exists {
		discoveryValue = netCfg.FileName() + " loaded"
		discoveryTone = "good"
		discoveryDetail = "Bootstrap profile is present on this node."
	}
	httpFallbackValue := "enabled"
	httpFallbackTone := "good"
	httpFallbackDetail := "Hao.News now uses libp2p direct transfer with HTTP fallback for bundle delivery."
	libp2pValue := "not configured"
	libp2pTone := "warn"
	libp2pDetail := "Add libp2p_bootstrap peers to prepare the live control plane."
	if netErr != nil {
		libp2pValue = "config error"
		libp2pTone = "bad"
		libp2pDetail = "libp2p bootstrap config could not be parsed."
	} else if len(netCfg.LibP2PBootstrap) > 0 {
		libp2pValue = fmt.Sprintf("%d bootstrap peers configured", len(netCfg.LibP2PBootstrap))
		libp2pTone = "good"
		libp2pDetail = "Peer list is ready. Live peer dialing will come from the sync daemon."
	}
	rendezvousValue := "not configured"
	rendezvousTone := "warn"
	rendezvousDetail := "Add libp2p_rendezvous topics so peers can meet on shared namespaces."
	if netErr != nil {
		rendezvousValue = "config error"
		rendezvousTone = "bad"
		rendezvousDetail = "Rendezvous config could not be parsed."
	} else if len(netCfg.LibP2PRendezvous) > 0 {
		rendezvousValue = fmt.Sprintf("%d rendezvous topics configured", len(netCfg.LibP2PRendezvous))
		rendezvousTone = "good"
		rendezvousDetail = "Namespaces are ready for peer discovery."
	}
	networkIDValue := "not configured"
	networkIDTone := "warn"
	networkIDDetail := "Add a stable 256-bit network_id so same-name projects do not share the same Hao.News discovery space."
	if netErr != nil {
		networkIDValue = "config error"
		networkIDTone = "bad"
		networkIDDetail = "network_id could not be parsed from hao_news_net.inf."
	} else if netCfg.NetworkID != "" {
		networkIDValue = netCfg.NetworkID
		networkIDTone = "good"
		networkIDDetail = "This node is pinned to one Hao.News network namespace even if other projects reuse the same human-readable name."
	}

	summary := "offline"
	summaryTone := "warn"
	summaryDetail := "No bootstrap transports are configured yet."
	switch {
	case netErr != nil:
		summary = "config error"
		summaryTone = "bad"
		summaryDetail = "hao_news_net.inf exists but could not be parsed."
	case len(netCfg.LibP2PBootstrap) > 0:
		summary = "bootstrap ready"
		summaryTone = "good"
		summaryDetail = "libp2p discovery is configured. Hao.News Public is still in UI/index mode until the sync daemon is running."
	}
	return NodeStatus{
		Summary:       summary,
		SummaryTone:   summaryTone,
		SummaryDetail: summaryDetail,
		NetworkStatus: "not connected",
		NetworkTone:   "warn",
		NetworkDetail: "The sync daemon is not online on this node yet.",
		Entries: []NodeStatusEntry{
			{Label: "Overall", Value: summary, Detail: summaryDetail, Tone: summaryTone},
			{Label: "HTTP UI", Value: "online " + a.httpListenAddr(), Detail: "The local dashboard is reachable on this node.", Tone: "good"},
			{Label: "Bundle store", Value: fmt.Sprintf("%s · %d bundles", storeState, len(index.Bundles)), Detail: "Hao.News News is reading from the local immutable bundle store.", Tone: storeTone},
			{Label: "Torrent refs", Value: fmt.Sprintf("%d available", torrentCount), Detail: "Immutable torrent references currently mirrored on this node.", Tone: "good"},
			{Label: "Sync daemon", Value: "not running", Detail: "Run `haonews sync` to turn bootstrap configuration into a live network session.", Tone: "warn"},
			{Label: "libp2p pubsub", Value: "not running", Detail: "Pubsub topic joins start when the sync daemon is running.", Tone: "warn"},
			{Label: "Discovery file", Value: discoveryValue, Detail: discoveryDetail, Tone: discoveryTone},
			{Label: "Network ID", Value: networkIDValue, Detail: networkIDDetail, Tone: networkIDTone},
			{Label: "LAN mDNS", Value: "not running", Detail: "Local network discovery starts when the sync daemon is running.", Tone: "warn"},
			{Label: "libp2p bootstrap", Value: libp2pValue, Detail: libp2pDetail, Tone: libp2pTone},
			{Label: "libp2p rendezvous", Value: rendezvousValue, Detail: rendezvousDetail, Tone: rendezvousTone},
			{Label: "HTTP fallback", Value: httpFallbackValue, Detail: httpFallbackDetail, Tone: httpFallbackTone},
		},
		Dashboard: []NodeStatusCard{
			{Label: "Node mode", Value: summary, Detail: summaryDetail, Tone: summaryTone},
			{Label: "libp2p pubsub", Value: "not running", Detail: "Pubsub topic joins start when the sync daemon is running.", Tone: "warn"},
			{Label: "LAN mDNS", Value: "not running", Detail: "Local network discovery starts when the sync daemon is running.", Tone: "warn"},
			{Label: "libp2p bootstrap", Value: libp2pValue, Detail: libp2pDetail, Tone: libp2pTone},
			{Label: "HTTP fallback", Value: httpFallbackValue, Detail: httpFallbackDetail, Tone: httpFallbackTone},
			{Label: "Discovery profile", Value: discoveryValue, Detail: discoveryDetail, Tone: discoveryTone},
			{Label: "Network ID", Value: networkIDValue, Detail: networkIDDetail, Tone: networkIDTone},
		},
	}
}

func buildLiveNodeStatus(index Index, storeState, storeTone string, torrentCount int, netCfg NetworkBootstrapConfig, syncStatus SyncRuntimeStatus, listenAddr string) NodeStatus {
	age := time.Since(syncStatus.UpdatedAt)
	queueStalled := false
	queueStallAge := time.Duration(0)
	if syncStatus.SyncActivity.QueueRefs > 0 {
		switch {
		case syncStatus.SyncActivity.LastEventAt != nil:
			queueStallAge = time.Since(*syncStatus.SyncActivity.LastEventAt)
		case !syncStatus.StartedAt.IsZero():
			queueStallAge = time.Since(syncStatus.StartedAt)
		default:
			queueStallAge = age
		}
		queueStalled = queueStallAge > 2*time.Minute
	}
	summary := "degraded"
	summaryTone := "warn"
	summaryDetail := fmt.Sprintf("Sync daemon heartbeat updated %s ago.", age.Truncate(time.Second))
	switch {
	case age > 2*time.Minute:
		summary = "stale"
		summaryTone = "warn"
		summaryDetail = fmt.Sprintf("Sync daemon status is stale. Last heartbeat was %s ago.", age.Truncate(time.Second))
	case queueStalled:
		summary = "backfill stalled"
		summaryTone = "warn"
		summaryDetail = fmt.Sprintf("Sync worker is alive, but queue refs have not moved for %s.", queueStallAge.Truncate(time.Second))
	case syncStatus.LibP2P.ReachableBootstrap > 0:
		summary = "online"
		summaryTone = "good"
		summaryDetail = "libp2p bootstrap peers are reachable."
	case syncStatus.LibP2P.ConnectedBootstrap > 0:
		summary = "partial"
		summaryTone = "warn"
		summaryDetail = "At least one libp2p path is online, but the full sync path is not yet healthy."
	}

	libp2pValue := fmt.Sprintf("%d/%d reachable · %d peers", syncStatus.LibP2P.ReachableBootstrap, syncStatus.LibP2P.ConfiguredBootstrap, syncStatus.LibP2P.ConnectedPeers)
	libp2pTone := "warn"
	libp2pDetail := "Live libp2p bootstrap reachability from the sync daemon."
	if syncStatus.LibP2P.LastError != "" {
		libp2pDetail = summarizeNetworkError(syncStatus.LibP2P.LastError, "libp2p bootstrap has transient dial noise.")
	}
	if syncStatus.LibP2P.ReachableBootstrap > 0 {
		libp2pTone = "good"
	}

	rendezvousValue := fmt.Sprintf("%d configured", syncStatus.LibP2P.ConfiguredRendezvous)
	rendezvousTone := "warn"
	rendezvousDetail := "Rendezvous namespaces declared for the live control plane."
	if syncStatus.LibP2P.ConfiguredRendezvous > 0 {
		rendezvousTone = "good"
	}

	httpFallbackValue := "enabled"
	httpFallbackTone := "good"
	httpFallbackDetail := "Sync now uses libp2p direct transfer and HTTP fallback."

	pubsubValue := "disabled"
	pubsubTone := "warn"
	pubsubDetail := "Pubsub announcement relay is not active."
	if syncStatus.PubSub.Enabled {
		pubsubValue = fmt.Sprintf("%d topics · %d rx · %d enqueued", len(syncStatus.PubSub.JoinedTopics), syncStatus.PubSub.Received, syncStatus.PubSub.Enqueued)
		pubsubDetail = fmt.Sprintf("%d local announcements published across %d discovery namespaces, %d discovery feeds, %d discovery topics.", syncStatus.PubSub.Published, len(syncStatus.PubSub.DiscoveryNamespaces), len(syncStatus.PubSub.DiscoveryFeeds), len(syncStatus.PubSub.DiscoveryTopics))
		if syncStatus.PubSub.LastError != "" {
			pubsubDetail = summarizeNetworkError(syncStatus.PubSub.LastError, "Pubsub relay is active but some peer announcements are noisy.")
		}
		pubsubTone = "good"
	}

	mdnsValue := "enabled"
	mdnsTone := "warn"
	mdnsDetail := "mDNS is listening for Hao.News peers on the local network."
	if !syncStatus.LibP2P.MDNS.Enabled {
		mdnsValue = "disabled"
		mdnsDetail = "Local network peer discovery is not active."
	} else {
		mdnsValue = fmt.Sprintf("%d discovered · %d connected", syncStatus.LibP2P.MDNS.DiscoveredPeers, syncStatus.LibP2P.MDNS.ConnectedPeers)
		if syncStatus.LibP2P.MDNS.LastError != "" {
			mdnsDetail = summarizeNetworkError(syncStatus.LibP2P.MDNS.LastError, "mDNS is active but local peer dialing is noisy.")
		} else if syncStatus.LibP2P.MDNS.DiscoveredPeers > 0 {
			mdnsTone = "good"
			mdnsDetail = "Local network peers have been discovered through mDNS."
		}
	}

	discoveryValue := "sync daemon active"
	discoveryTone := "good"
	discoveryDetail := fmt.Sprintf("%s loaded; sync status heartbeat is current.", netCfg.FileName())
	if !netCfg.Exists {
		discoveryValue = "status only"
		discoveryDetail = "Sync daemon is running, but hao_news_net.inf is not present on this node."
	}

	syncDaemonValue := fmt.Sprintf("pid %d · %s", syncStatus.PID, syncStatus.Mode)
	syncDaemonDetail := fmt.Sprintf("Queue refs %d, imported %d, skipped %d, failed %d.", syncStatus.SyncActivity.QueueRefs, syncStatus.SyncActivity.Imported, syncStatus.SyncActivity.Skipped, syncStatus.SyncActivity.Failed)
	if syncStatus.HistoryBootstrap.Mode != "" {
		syncDaemonDetail = fmt.Sprintf("%s History bootstrap: %s.", syncDaemonDetail, syncStatus.HistoryBootstrap.Mode)
		if !syncStatus.HistoryBootstrap.FirstSyncCompleted && syncStatus.HistoryBootstrap.RecentRefsLimit > 0 {
			syncDaemonDetail = fmt.Sprintf("%s Recent window %d refs / %d pages.", syncDaemonDetail, syncStatus.HistoryBootstrap.RecentRefsLimit, syncStatus.HistoryBootstrap.RecentPagesLimit)
		}
	}
	if syncStatus.SyncActivity.LastStatus != "" {
		syncDaemonDetail = fmt.Sprintf("%s Last result: %s.", syncDaemonDetail, syncStatus.SyncActivity.LastStatus)
	}
	if queueStalled {
		syncDaemonDetail = fmt.Sprintf("%s Queue activity has been idle for %s.", syncDaemonDetail, queueStallAge.Truncate(time.Second))
	}
	networkIDValue := syncStatus.NetworkID
	networkIDTone := "warn"
	networkIDDetail := "No network_id is active; this node may still be using older shared discovery namespaces."
	if networkIDValue != "" {
		networkIDTone = "good"
		networkIDDetail = "Active 256-bit namespace used for pubsub topics, rendezvous discovery, and announcement filtering."
	} else if netCfg.NetworkID != "" {
		networkIDValue = netCfg.NetworkID
	}

	return NodeStatus{
		Summary:       summary,
		SummaryTone:   summaryTone,
		SummaryDetail: summaryDetail,
		NetworkStatus: func() string {
			switch summary {
			case "online":
				return "connected"
			case "partial":
				return "partial"
			default:
				return "not connected"
			}
		}(),
		NetworkTone: func() string {
			switch summary {
			case "online":
				return "good"
			case "partial":
				return "warn"
			default:
				return "warn"
			}
		}(),
		NetworkDetail: func() string {
			switch summary {
			case "online":
				return "At least one live network path is working."
			case "partial":
				return "Some network paths are up, but the node is not fully healthy yet."
			default:
				return "The node is not connected to a healthy live network path."
			}
		}(),
		Entries: []NodeStatusEntry{
			{Label: "Overall", Value: summary, Detail: summaryDetail, Tone: summaryTone},
			{Label: "HTTP UI", Value: "online " + listenAddr, Detail: "The local dashboard is reachable on this node.", Tone: "good"},
			{Label: "Bundle store", Value: fmt.Sprintf("%s · %d bundles", storeState, len(index.Bundles)), Detail: "Hao.News News is reading from the local immutable bundle store.", Tone: storeTone},
			{Label: "Torrent refs", Value: fmt.Sprintf("%d available", torrentCount), Detail: "Immutable torrent references currently mirrored on this node.", Tone: "good"},
			{Label: "Sync daemon", Value: syncDaemonValue, Detail: syncDaemonDetail, Tone: "good"},
			{Label: "libp2p pubsub", Value: pubsubValue, Detail: pubsubDetail, Tone: pubsubTone},
			{Label: "Discovery file", Value: discoveryValue, Detail: discoveryDetail, Tone: discoveryTone},
			{Label: "Network ID", Value: networkIDValue, Detail: networkIDDetail, Tone: networkIDTone},
			{Label: "LAN mDNS", Value: mdnsValue, Detail: mdnsDetail, Tone: mdnsTone},
			{Label: "libp2p bootstrap", Value: libp2pValue, Detail: libp2pDetail, Tone: libp2pTone},
			{Label: "libp2p rendezvous", Value: rendezvousValue, Detail: rendezvousDetail, Tone: rendezvousTone},
			{Label: "HTTP fallback", Value: httpFallbackValue, Detail: httpFallbackDetail, Tone: httpFallbackTone},
		},
		Dashboard: []NodeStatusCard{
			{Label: "Node mode", Value: summary, Detail: summaryDetail, Tone: summaryTone},
			{Label: "libp2p pubsub", Value: pubsubValue, Detail: pubsubDetail, Tone: pubsubTone},
			{Label: "LAN mDNS", Value: mdnsValue, Detail: mdnsDetail, Tone: mdnsTone},
			{Label: "libp2p bootstrap", Value: libp2pValue, Detail: libp2pDetail, Tone: libp2pTone},
			{Label: "HTTP fallback", Value: httpFallbackValue, Detail: httpFallbackDetail, Tone: httpFallbackTone},
			{Label: "Sync daemon", Value: syncDaemonValue, Detail: syncDaemonDetail, Tone: "good"},
			{Label: "Network ID", Value: networkIDValue, Detail: networkIDDetail, Tone: networkIDTone},
		},
	}
}

func summarizeNetworkError(raw, fallback string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return fallback
	}
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "dial to self attempted"):
		return "A discovered address points back to this node. This is noisy but harmless."
	case strings.Contains(lower, "peer id mismatch"):
		return "At least one peer advertised an address with the wrong peer identity. The node skipped it."
	case strings.Contains(lower, "no addresses"):
		return "A peer was discovered without dialable addresses. Discovery still worked, but that peer could not be contacted."
	case strings.Contains(lower, "context deadline exceeded"):
		return "A network dial timed out. The node will keep retrying healthy peers."
	case strings.Contains(lower, "connection refused"):
		return "A peer address was reachable at the network layer but refused the connection."
	case strings.Contains(lower, "all dials failed"):
		return "Some peer dial attempts failed. Other reachable peers may still be healthy."
	case strings.Contains(lower, "timed out waiting for metadata"):
		return "Torrent metadata retrieval timed out for at least one queued ref."
	default:
		if len(text) > 180 {
			return fallback
		}
		return text
	}
}

func (a *App) networkBootstrap() (NetworkBootstrapConfig, error) {
	if a.loadNet == nil {
		return NetworkBootstrapConfig{}, nil
	}
	return a.loadNet(a.netPath)
}

func (a *App) syncRuntimeStatus() (SyncRuntimeStatus, error) {
	if a.loadSync == nil {
		return SyncRuntimeStatus{}, nil
	}
	return a.loadSync(a.storeRoot)
}

func (a *App) syncSupervisorStatus() (SyncSupervisorState, error) {
	if a.loadSuper == nil {
		return SyncSupervisorState{}, nil
	}
	paths, err := DefaultRuntimePaths()
	if err != nil {
		return SyncSupervisorState{}, err
	}
	return a.loadSuper(paths.SupervisorStatePath)
}

func (a *App) lanPeerHealth() ([]LANPeerHealthStatus, []LANPeerHealthStatus, error) {
	cfg, err := corehaonews.LoadNetworkBootstrapConfig(a.netPath)
	if err != nil {
		return nil, nil, err
	}
	lanPeers, lanBTPeers, err := corehaonews.ReadLANPeerHealthStatus(cfg)
	if err != nil {
		return nil, nil, err
	}
	return mapLANPeerHealthStatus(lanPeers), mapLANPeerHealthStatus(lanBTPeers), nil
}

func (a *App) knownGoodLibP2PPeers() ([]KnownGoodLibP2PPeerStatus, error) {
	cfg, err := corehaonews.LoadNetworkBootstrapConfig(a.netPath)
	if err != nil {
		return nil, err
	}
	items, err := corehaonews.ReadKnownGoodLibP2PPeerStatus(cfg)
	if err != nil {
		return nil, err
	}
	out := make([]KnownGoodLibP2PPeerStatus, 0, len(items))
	for _, item := range items {
		out = append(out, KnownGoodLibP2PPeerStatus{
			PeerID:        item.PeerID,
			LastSuccessAt: item.LastSuccessAt,
			Addrs:         append([]string(nil), item.Addrs...),
		})
	}
	return out, nil
}

func (a *App) advertiseHostHealth() ([]AdvertiseHostHealthStatus, error) {
	cfg, err := LoadNetworkBootstrapConfig(a.netPath)
	if err != nil {
		return nil, err
	}
	items, err := ReadAdvertiseHostHealth(cfg)
	if err != nil {
		return nil, err
	}
	out := make([]AdvertiseHostHealthStatus, 0, len(items))
	for _, item := range items {
		out = append(out, AdvertiseHostHealthStatus{
			Host:          item.Host,
			SuccessCount:  item.SuccessCount,
			FailureCount:  item.FailureCount,
			LastSuccessAt: item.LastSuccessAt,
			LastFailureAt: item.LastFailureAt,
		})
	}
	return out, nil
}

func mapLANPeerHealthStatus(values []corehaonews.LANPeerHealthStatus) []LANPeerHealthStatus {
	out := make([]LANPeerHealthStatus, 0, len(values))
	for _, value := range values {
		out = append(out, LANPeerHealthStatus{
			Peer:                value.Peer,
			State:               value.State,
			Reason:              lanPeerHealthReason(value),
			ObservedPrimaryHost: value.ObservedPrimaryHost,
			ObservedPrimaryFrom: value.ObservedPrimaryFrom,
			LastSuccessAt:       value.LastSuccessAt,
			LastFailureAt:       value.LastFailureAt,
			ConsecutiveFailure:  value.ConsecutiveFailure,
			LastError:           value.LastError,
		})
	}
	return out
}

func lanPeerHealthReason(value corehaonews.LANPeerHealthStatus) string {
	observed := strings.TrimSpace(value.ObservedPrimaryHost)
	source := strings.TrimSpace(value.ObservedPrimaryFrom)
	observedSuffix := ""
	if observed != "" && observed != strings.TrimSpace(value.Peer) {
		observedSuffix = "；最近学习远端主地址为 " + observed
		if source != "" {
			observedSuffix += "（来源：" + source + "）"
		}
	}
	switch strings.TrimSpace(value.State) {
	case "preferred":
		if observedSuffix != "" {
			return "最近成功，当前优先作为局域网锚点" + observedSuffix
		}
		return "最近成功，当前优先作为局域网锚点"
	case "cooldown":
		if strings.TrimSpace(value.LastError) != "" {
			if observedSuffix != "" {
				return "最近失败，冷却后排：" + strings.TrimSpace(value.LastError) + observedSuffix
			}
			return "最近失败，冷却后排：" + strings.TrimSpace(value.LastError)
		}
		if observedSuffix != "" {
			return "最近失败，当前处于冷却后排" + observedSuffix
		}
		return "最近失败，当前处于冷却后排"
	case "degraded":
		if strings.TrimSpace(value.LastError) != "" {
			if observedSuffix != "" {
				return "历史失败较多：" + strings.TrimSpace(value.LastError) + observedSuffix
			}
			return "历史失败较多：" + strings.TrimSpace(value.LastError)
		}
		if observedSuffix != "" {
			return "历史上出现过失败，当前不是优先地址" + observedSuffix
		}
		return "历史上出现过失败，当前不是优先地址"
	default:
		if observedSuffix != "" {
			return "尚无健康记录，等待首次探测" + observedSuffix
		}
		return "尚无健康记录，等待首次探测"
	}
}
