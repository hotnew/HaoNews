package aip2p

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	anacrolixdht "github.com/anacrolix/dht/v2"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

type SyncOptions struct {
	StoreRoot          string
	QueuePath          string
	NetPath            string
	TrackerListPath    string
	SubscriptionsPath  string
	CreditIdentityFile string
	ListenAddr         string
	Refs               []string
	PollInterval       time.Duration
	Timeout            time.Duration
	Once               bool
	Seed               bool
}

type SyncRef struct {
	Raw      string
	Magnet   string
	InfoHash string
}

type SyncItemResult struct {
	Ref        string `json:"ref"`
	InfoHash   string `json:"infohash,omitempty"`
	ContentDir string `json:"content_dir,omitempty"`
	Status     string `json:"status"`
	Message    string `json:"message,omitempty"`
}

const (
	defaultSyncRefTimeout = 20 * time.Second
	maxSyncRefsPerPass    = 3
)

func RunSync(ctx context.Context, opts SyncOptions, logf func(string, ...any)) error {
	store, err := OpenStore(opts.StoreRoot)
	if err != nil {
		return err
	}
	queuePath, err := ensureSyncLayout(store, opts.QueuePath)
	if err != nil {
		return err
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = 30 * time.Second
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaultSyncRefTimeout
	}
	if strings.TrimSpace(opts.TrackerListPath) == "" {
		opts.TrackerListPath = defaultTrackerListPath(opts.NetPath)
	}
	if err := EnsureDefaultTrackerList(opts.TrackerListPath); err != nil {
		return fmt.Errorf("ensure tracker list: %w", err)
	}
	if err := ensureNetworkID(opts.NetPath, latestOrgNetworkID); err != nil {
		return fmt.Errorf("ensure latest.org network id: %w", err)
	}
	if err := ensureLANPeer(opts.NetPath, defaultLANPeer); err != nil {
		return fmt.Errorf("ensure lan peer: %w", err)
	}
	if err := ensureLANTorrentPeer(opts.NetPath, defaultLANPeer); err != nil {
		return fmt.Errorf("ensure lan bt peer: %w", err)
	}
	netCfg, err := LoadNetworkBootstrapConfig(opts.NetPath)
	if err != nil {
		return fmt.Errorf("load network bootstrap config: %w", err)
	}
	subscriptions, err := LoadSyncSubscriptions(opts.SubscriptionsPath)
	if err != nil {
		return fmt.Errorf("load subscriptions: %w", err)
	}
	trackers, err := LoadTrackerList(opts.TrackerListPath)
	if err != nil {
		return fmt.Errorf("load tracker list: %w", err)
	}
	dhtRouters, err := resolveEffectiveDHTRouters(ctx, netCfg)
	if err != nil && logf != nil {
		logf("resolve LAN BT/DHT peers: %v", err)
	}

	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = store.DataDir
	cfg.Seed = opts.Seed
	cfg.NoDefaultPortForwarding = true
	cfg.DisableAcceptRateLimiting = true
	cfg.DhtStartingNodes = func(network string) anacrolixdht.StartingNodesGetter {
		return func() ([]anacrolixdht.Addr, error) {
			return resolveDHTRouters(network, dhtRouters)
		}
	}
	if strings.TrimSpace(opts.ListenAddr) != "" {
		cfg.SetListenAddr(opts.ListenAddr)
	} else if strings.TrimSpace(netCfg.BitTorrentListen) != "" {
		cfg.SetListenAddr(normalizeBitTorrentListen(netCfg.BitTorrentListen))
	}
	client, err := torrent.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("create torrent client: %w", err)
	}
	defer client.Close()
	if err := bootstrapTorrentDHT(client, dhtRouters); err != nil && logf != nil {
		logf("bootstrap torrent dht: %v", err)
	}
	if len(dhtRouters) > 0 {
		client.AddDhtNodes(dhtRouters)
	}

	libp2pRuntime, err := startLibP2PRuntime(ctx, netCfg)
	if err != nil {
		return err
	}
	defer libp2pRuntime.Close()

	runtime := &syncRuntime{
		store:           store,
		queuePath:       queuePath,
		mode:            syncMode(opts.Once),
		seed:            opts.Seed,
		startedAt:       time.Now().UTC(),
		torrentClient:   client,
		libp2p:          libp2pRuntime,
		netCfg:          netCfg,
		trackers:        trackers,
		subscriptions:   subscriptions,
		announced:       make(map[string]struct{}),
		announcedProofs: make(map[string]struct{}),
		seeded:          make(map[string]struct{}),
	}
	runtime.creditStore, err = OpenCreditStore(store.Root)
	if err != nil {
		return err
	}
	runtime.creditIdentity, err = loadSyncCreditIdentity(opts.CreditIdentityFile)
	if err != nil {
		if logf != nil {
			logf("load credit identity: %v", err)
		}
	}
	if runtime.creditIdentity != nil && libp2pRuntime != nil {
		if err := registerCreditWitnessHandler(libp2pRuntime.host, *runtime.creditIdentity, runtime.seededInfohashes); err != nil && logf != nil {
			logf("register credit witness handler: %v", err)
		}
	}
	runtime.pubsub, err = startPubSubRuntime(ctx, libp2pRuntime, subscriptions, runtime.handleAnnouncement, runtime.handleCreditProof)
	if err != nil {
		return err
	}
	defer runtime.pubsub.Close()
	if err := runtime.writeStatus(ctx); err != nil && logf != nil {
		logf("write sync status: %v", err)
	}
	if logf != nil {
		logf("sync queue: %s", queuePath)
		if netCfg.Exists {
			logf("network bootstrap file: %s", netCfg.FileName())
			logf("configured DHT routers: %d", len(dhtRouters))
			logf("configured libp2p peers: %d", len(netCfg.LibP2PBootstrap))
			logf("configured libp2p rendezvous namespaces: %d", len(netCfg.LibP2PRendezvous))
		} else if strings.TrimSpace(opts.NetPath) != "" {
			logf("network bootstrap file not found: %s", opts.NetPath)
		}
		if !subscriptions.Empty() {
			logf("subscription filters: %d channels, %d topics, %d tags", len(subscriptions.Channels), len(subscriptions.Topics), len(subscriptions.Tags))
		}
	}

	if opts.Once {
		if err := runtime.seedLocalTorrents(logf); err != nil && logf != nil {
			logf("seed local torrents: %v", err)
		}
		if err := runtime.announceLocalBundles(ctx, logf); err != nil && logf != nil {
			logf("announce local bundles: %v", err)
		}
		if err := runtime.generateLocalCreditProof(time.Now().UTC(), logf); err != nil && logf != nil {
			logf("generate local credit proof: %v", err)
		}
		if err := runtime.ensureLocalCreditBundle(time.Now().UTC(), logf); err != nil && logf != nil {
			logf("ensure local credit bundle: %v", err)
		}
		if err := runtime.announceLocalCreditProofs(ctx, logf); err != nil && logf != nil {
			logf("announce local credit proofs: %v", err)
		}
		if err := runtime.reconcileQueue(ctx, opts.Refs, opts.Timeout, logf); err != nil {
			return err
		}
		if runtime.queueRefs() == 0 {
			return errors.New("no magnet or infohash refs found")
		}
		return nil
	}

	ticker := time.NewTicker(opts.PollInterval)
	defer ticker.Stop()
	for {
		if err := runtime.seedLocalTorrents(logf); err != nil && logf != nil {
			logf("seed local torrents: %v", err)
		}
		if err := runtime.announceLocalBundles(ctx, logf); err != nil && logf != nil {
			logf("announce local bundles: %v", err)
		}
		if err := runtime.generateLocalCreditProof(time.Now().UTC(), logf); err != nil && logf != nil {
			logf("generate local credit proof: %v", err)
		}
		if err := runtime.ensureLocalCreditBundle(time.Now().UTC(), logf); err != nil && logf != nil {
			logf("ensure local credit bundle: %v", err)
		}
		if err := runtime.announceLocalCreditProofs(ctx, logf); err != nil && logf != nil {
			logf("announce local credit proofs: %v", err)
		}
		if err := runtime.reconcileQueue(ctx, opts.Refs, opts.Timeout, logf); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

type syncRuntime struct {
	mu              sync.Mutex
	store           *Store
	queuePath       string
	mode            string
	seed            bool
	startedAt       time.Time
	torrentClient   *torrent.Client
	libp2p          *libp2pRuntime
	pubsub          *pubsubRuntime
	creditStore     *CreditStore
	creditIdentity  *AgentIdentity
	netCfg          NetworkBootstrapConfig
	trackers        []string
	subscriptions   SyncSubscriptions
	announced       map[string]struct{}
	announcedProofs map[string]struct{}
	seeded          map[string]struct{}
	activity        SyncActivityStatus
}

func (r *syncRuntime) setQueueRefs(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.activity.QueueRefs = n
}

func (r *syncRuntime) recordResult(result SyncItemResult) {
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.activity.LastRef = result.Ref
	r.activity.LastInfoHash = result.InfoHash
	r.activity.LastStatus = result.Status
	r.activity.LastMessage = result.Message
	r.activity.LastEventAt = &now
	switch result.Status {
	case "imported":
		r.activity.Imported++
	case "skipped":
		r.activity.Skipped++
	default:
		r.activity.Failed++
	}
}

func (r *syncRuntime) queueRefs() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.activity.QueueRefs
}

func (r *syncRuntime) writeStatus(ctx context.Context) error {
	r.mu.Lock()
	activity := r.activity
	r.mu.Unlock()
	status := SyncRuntimeStatus{
		StartedAt:    r.startedAt,
		PID:          os.Getpid(),
		StoreRoot:    r.store.Root,
		QueuePath:    r.queuePath,
		Mode:         r.mode,
		Seed:         r.seed,
		NetworkID:    r.netCfg.NetworkID,
		SyncActivity: activity,
	}
	status.LibP2P = r.libp2p.Status(ctx)
	status.BitTorrentDHT = torrentStatus(r.torrentClient, effectiveDHTRouterCount(r.netCfg))
	status.PubSub = r.pubsub.Status()
	return writeSyncStatus(r.store, status)
}

func (r *syncRuntime) processQueue(ctx context.Context, direct []string, timeout time.Duration, logf func(string, ...any)) error {
	refs, err := collectSyncRefs(direct, r.queuePath)
	if err != nil {
		return err
	}
	sortSyncRefsByPriority(refs)
	if len(refs) > maxSyncRefsPerPass {
		refs = refs[:maxSyncRefsPerPass]
	}
	r.setQueueRefs(len(refs))
	if err := r.writeStatus(ctx); err != nil && logf != nil {
		logf("write sync status: %v", err)
	}
	for _, ref := range refs {
		result := syncRef(ctx, r.torrentClient, r.store, ref, timeout, r.netCfg.LANPeers, r.trackers, r.subscriptions)
		if result.Status == "imported" && result.ContentDir != "" {
			if err := r.importCreditBundle(result.ContentDir, logf); err != nil && logf != nil {
				logf("import credit bundle: %v", err)
			}
		}
		r.recordResult(result)
		if result.Status == "imported" || result.Status == "skipped" {
			if err := removeSyncRef(r.queuePath, ref); err != nil && logf != nil {
				logf("remove sync ref: %v", err)
			}
		} else if result.Status == "failed" {
			if isTerminalSyncFailure(ref, result) {
				if err := removeSyncRef(r.queuePath, ref); err != nil && logf != nil {
					logf("drop terminal sync ref: %v", err)
				}
			} else if err := rotateSyncRef(r.queuePath, ref); err != nil && logf != nil {
				logf("rotate failed sync ref: %v", err)
			}
		}
		if err := r.writeStatus(ctx); err != nil && logf != nil {
			logf("write sync status: %v", err)
		}
		if logf != nil {
			logf("%s: %s", result.Status, result.Ref)
			if result.Message != "" {
				logf("  %s", result.Message)
			}
		}
	}
	return nil
}

func (r *syncRuntime) reconcileQueue(ctx context.Context, direct []string, timeout time.Duration, logf func(string, ...any)) error {
	if changed, err := sanitizeSyncQueueFile(r.queuePath, r.netCfg.LANPeers); err != nil {
		return err
	} else if changed > 0 && logf != nil {
		logf("sanitized %d queued magnet refs", changed)
	}
	if added, err := r.enqueueHistoryFromLANPeers(ctx, logf); err != nil {
		return err
	} else if added > 0 && logf != nil {
		logf("lan history head queued %d refs", added)
	}
	for round := 0; round < 3; round++ {
		if err := r.processQueue(ctx, direct, timeout, logf); err != nil {
			return err
		}
		added, err := r.enqueueHistoryFromLocalManifests(logf)
		if err != nil {
			return err
		}
		if added == 0 {
			return nil
		}
		direct = nil
	}
	return nil
}

func (r *syncRuntime) announceLocalBundles(ctx context.Context, logf func(string, ...any)) error {
	if r.pubsub == nil {
		return nil
	}
	if err := ensureHistoryManifests(r.store, r.netCfg, r.torrentClient.ListenAddrs()); err != nil {
		return err
	}
	if err := r.seedLocalTorrents(logf); err != nil {
		return err
	}
	announcements, err := localAnnouncements(r.store)
	if err != nil {
		return err
	}
	for _, announcement := range announcements {
		if announcement.InfoHash == "" {
			continue
		}
		if announcement.NetworkID == "" {
			announcement.NetworkID = r.netCfg.NetworkID
		}
		announcement.Magnet = withPeerHints(announcement.Magnet, r.torrentClient.ListenAddrs(), r.netCfg.LANPeers)
		alwaysPublish := strings.EqualFold(announcement.Kind, historyManifestKind)
		if !alwaysPublish {
			r.mu.Lock()
			_, seen := r.announced[announcement.InfoHash]
			if !seen {
				r.announced[announcement.InfoHash] = struct{}{}
			}
			r.mu.Unlock()
			if seen {
				continue
			}
		}
		if err := r.pubsub.PublishAnnouncement(ctx, announcement); err != nil {
			if !alwaysPublish {
				r.mu.Lock()
				delete(r.announced, announcement.InfoHash)
				r.mu.Unlock()
			}
			return err
		}
		if logf != nil {
			logf("announced: %s (%s)", announcement.InfoHash, announcement.Title)
		}
	}
	if err := r.writeStatus(ctx); err != nil && logf != nil {
		logf("write sync status: %v", err)
	}
	return nil
}

func (r *syncRuntime) seedLocalTorrents(logf func(string, ...any)) error {
	return r.store.WalkTorrentFiles(func(infoHash, path string) error {
		r.mu.Lock()
		_, seen := r.seeded[infoHash]
		if !seen {
			r.seeded[infoHash] = struct{}{}
		}
		r.mu.Unlock()
		if seen {
			return nil
		}
		if _, err := r.torrentClient.AddTorrentFromFile(path); err != nil {
			r.mu.Lock()
			delete(r.seeded, infoHash)
			r.mu.Unlock()
			return err
		}
		if logf != nil {
			logf("seeding: %s", infoHash)
		}
		return nil
	})
}

func (r *syncRuntime) handleAnnouncement(announcement SyncAnnouncement) (bool, error) {
	if r.netCfg.NetworkID != "" && !strings.EqualFold(strings.TrimSpace(announcement.NetworkID), r.netCfg.NetworkID) {
		return false, nil
	}
	if !matchesAnnouncement(announcement, r.subscriptions) {
		return false, nil
	}
	ref, err := ParseSyncRef(announcement.Magnet)
	if err != nil {
		return false, err
	}
	if hasCompleteLocalBundle(r.store, ref.InfoHash) {
		return false, nil
	}
	dayCounts := localBundleDayCounts(r.store, "")
	if !reserveDailyQuota(dayCounts, announcement.CreatedAt, r.subscriptions.MaxItemsPerDay) {
		return false, nil
	}
	return enqueueSyncRef(r.queuePath, ref)
}

func (r *syncRuntime) handleCreditProof(proof OnlineProof) (bool, error) {
	if r.creditStore == nil {
		return false, nil
	}
	if r.netCfg.NetworkID != "" && !strings.EqualFold(strings.TrimSpace(proof.NetworkID), r.netCfg.NetworkID) {
		return false, nil
	}
	err := r.creditStore.SaveProof(proof)
	if errors.Is(err, ErrDuplicateProof) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *syncRuntime) generateLocalCreditProof(now time.Time, logf func(string, ...any)) error {
	if r.creditStore == nil || r.creditIdentity == nil {
		return nil
	}
	infohashes := r.seededInfohashes()
	if len(infohashes) == 0 {
		return nil
	}
	windowStart := AlignToWindow(now.UTC()).Add(-ProofWindowMinutes * time.Minute)
	if windowStart.IsZero() {
		return nil
	}
	proof, err := NewOnlineProof(*r.creditIdentity, windowStart, infohashes, r.netCfg.NetworkID)
	if err != nil {
		return err
	}
	if err := SignProof(proof, *r.creditIdentity); err != nil {
		return err
	}
	witnessCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	witnesses, err := collectRemoteWitnesses(witnessCtx, r.libp2p, *proof, MinWitnesses)
	cancel()
	if err != nil && logf != nil {
		logf("collect remote witnesses: %v", err)
	}
	if len(witnesses) < MinWitnesses {
		if logf != nil {
			logf("skip credit proof: no remote witness available for %s", proof.WindowStart)
		}
		return nil
	}
	proof.Witnesses = append(proof.Witnesses, witnesses...)
	err = r.creditStore.SaveProof(*proof)
	if errors.Is(err, ErrDuplicateProof) {
		return nil
	}
	if err != nil {
		return err
	}
	if logf != nil {
		logf("credit proof saved: %s (%s)", proof.ProofID, proof.WindowStart)
	}
	return nil
}

func (r *syncRuntime) ensureLocalCreditBundle(now time.Time, logf func(string, ...any)) error {
	if r.store == nil || r.creditStore == nil {
		return nil
	}
	result, err := EnsureCreditProofBundle(r.store, r.creditStore, now, r.netCfg.NetworkID)
	if err != nil {
		return err
	}
	if result.InfoHash != "" && logf != nil {
		logf("credit daily bundle ready: %s", result.InfoHash)
	}
	return nil
}

func (r *syncRuntime) importCreditBundle(contentDir string, logf func(string, ...any)) error {
	if r.creditStore == nil {
		return nil
	}
	imported, err := ImportCreditProofsFromBundle(contentDir, r.creditStore, r.netCfg.NetworkID)
	if err != nil {
		return err
	}
	if imported > 0 && logf != nil {
		logf("imported %d credit proofs from bundle", imported)
	}
	return nil
}

func (r *syncRuntime) announceLocalCreditProofs(ctx context.Context, logf func(string, ...any)) error {
	if r.pubsub == nil || r.creditStore == nil {
		return nil
	}
	proofs, err := r.creditStore.GetProofsSince(time.Now().UTC().Add(-ProofMaxAge))
	if err != nil {
		return err
	}
	published := 0
	for _, proof := range proofs {
		if r.netCfg.NetworkID != "" && !strings.EqualFold(strings.TrimSpace(proof.NetworkID), r.netCfg.NetworkID) {
			continue
		}
		r.mu.Lock()
		_, seen := r.announcedProofs[proof.ProofID]
		r.mu.Unlock()
		if seen {
			continue
		}
		if err := r.pubsub.PublishCreditProof(ctx, proof); err != nil {
			return err
		}
		r.mu.Lock()
		r.announcedProofs[proof.ProofID] = struct{}{}
		r.mu.Unlock()
		published++
	}
	if published > 0 && logf != nil {
		logf("published %d local credit proofs", published)
	}
	return nil
}

func (r *syncRuntime) seededInfohashes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.seeded))
	for infohash := range r.seeded {
		infohash = strings.ToLower(strings.TrimSpace(infohash))
		if infohash == "" {
			continue
		}
		out = append(out, infohash)
	}
	sort.Strings(out)
	return out
}

func loadSyncCreditIdentity(explicitPath string) (*AgentIdentity, error) {
	explicitPath = strings.TrimSpace(explicitPath)
	if explicitPath != "" {
		identity, err := LoadAgentIdentity(explicitPath)
		if err != nil {
			return nil, err
		}
		if !isCreditOnlineIdentity(identity) {
			return nil, errors.New("credit identity must use /credit/online author")
		}
		return &identity, nil
	}
	identityDir, err := defaultSyncIdentityDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(identityDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(identityDir, name)
		identity, err := LoadAgentIdentity(path)
		if err != nil {
			continue
		}
		if !isCreditOnlineIdentity(identity) {
			continue
		}
		return &identity, nil
	}
	return nil, nil
}

func defaultSyncIdentityDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return "", errors.New("user home directory is empty")
	}
	return filepath.Join(home, ".hao-news", "identities"), nil
}

func isCreditOnlineIdentity(identity AgentIdentity) bool {
	if err := identity.ValidatePrivate(); err != nil {
		return false
	}
	return strings.HasSuffix(strings.TrimSpace(identity.Author), "/credit/online")
}

func (r *syncRuntime) enqueueHistoryFromLocalManifests(logf func(string, ...any)) (int, error) {
	added, err := enqueueHistoryManifestRefs(r.store, r.queuePath, r.subscriptions, r.netCfg.NetworkID)
	if err != nil {
		return 0, err
	}
	if added > 0 && logf != nil {
		logf("history manifest queued %d refs", added)
	}
	return added, nil
}

func (r *syncRuntime) enqueueHistoryFromLANPeers(ctx context.Context, logf func(string, ...any)) (int, error) {
	added := 0
	dayCounts := localBundleDayCounts(r.store, "")
	for _, peerValue := range r.netCfg.LANPeers {
		payload, err := fetchLANHistoryManifest(ctx, peerValue, r.netCfg.NetworkID)
		if err != nil {
			if logf != nil {
				logf("fetch lan history manifest from %s: %v", peerValue, err)
			}
			continue
		}
		for _, announcement := range payload.Entries {
			announcement = normalizeAnnouncement(announcement)
			if announcement.NetworkID == "" {
				announcement.NetworkID = payload.NetworkID
			}
			if r.netCfg.NetworkID != "" && announcement.NetworkID != "" && !strings.EqualFold(announcement.NetworkID, r.netCfg.NetworkID) {
				continue
			}
			if !matchesAnnouncement(announcement, r.subscriptions) {
				continue
			}
			ref, err := syncRefFromAnnouncement(announcement)
			if err != nil || ref.InfoHash == "" {
				continue
			}
			if hasCompleteLocalBundle(r.store, ref.InfoHash) {
				continue
			}
			if !reserveDailyQuota(dayCounts, announcement.CreatedAt, r.subscriptions.MaxItemsPerDay) {
				continue
			}
			enqueued, err := enqueueSyncRef(r.queuePath, ref)
			if err != nil {
				return added, err
			}
			if enqueued {
				added++
			}
		}
	}
	return added, nil
}

func torrentStatus(client *torrent.Client, configuredRouters int) SyncBitTorrentStatus {
	status := SyncBitTorrentStatus{
		Enabled:           len(client.DhtServers()) > 0,
		ConfiguredRouters: configuredRouters,
		Servers:           len(client.DhtServers()),
	}
	for _, server := range client.DhtServers() {
		stats, ok := server.Stats().(anacrolixdht.ServerStats)
		if !ok {
			continue
		}
		status.GoodNodes += stats.GoodNodes
		status.Nodes += stats.Nodes
		status.OutstandingTransactions += stats.OutstandingTransactions
	}
	return status
}

func normalizeBitTorrentListen(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		return value
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		return ":" + port
	}
	return value
}

func enqueueSyncRef(queuePath string, ref SyncRef) (bool, error) {
	if strings.TrimSpace(queuePath) == "" {
		return false, errors.New("queue path is required")
	}
	data, err := os.ReadFile(queuePath)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
			continue
		}
		queued, err := ParseSyncRef(line)
		if err != nil {
			continue
		}
		if queued.InfoHash != "" && queued.InfoHash == ref.InfoHash {
			return false, nil
		}
		if queued.Magnet != "" && queued.Magnet == ref.Magnet {
			return false, nil
		}
	}
	file, err := os.OpenFile(queuePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	defer file.Close()
	if _, err := file.WriteString(ref.Magnet + "\n"); err != nil {
		return false, err
	}
	return true, nil
}

func removeSyncRef(queuePath string, ref SyncRef) error {
	data, err := os.ReadFile(queuePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	lines := strings.Split(string(data), "\n")
	out := make([]string, 0, len(lines))
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
			out = append(out, rawLine)
			continue
		}
		queued, err := ParseSyncRef(line)
		if err != nil {
			out = append(out, rawLine)
			continue
		}
		if queued.InfoHash != "" && queued.InfoHash == ref.InfoHash {
			continue
		}
		if queued.Magnet != "" && queued.Magnet == ref.Magnet {
			continue
		}
		out = append(out, rawLine)
	}
	content := strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n"
	return os.WriteFile(queuePath, []byte(content), 0o644)
}

func rotateSyncRef(queuePath string, ref SyncRef) error {
	if err := removeSyncRef(queuePath, ref); err != nil {
		return err
	}
	_, err := enqueueSyncRef(queuePath, ref)
	return err
}

func sortSyncRefsByPriority(refs []SyncRef) {
	sort.SliceStable(refs, func(i, j int) bool {
		return syncRefPriority(refs[i]) < syncRefPriority(refs[j])
	})
}

func syncRefPriority(ref SyncRef) int {
	if isHistoryManifestRef(ref) {
		return 1
	}
	return 0
}

func isHistoryManifestRef(ref SyncRef) bool {
	if strings.Contains(strings.ToLower(ref.Raw), "history-manifest") {
		return true
	}
	if strings.Contains(strings.ToLower(ref.Magnet), "history-manifest") {
		return true
	}
	uri, err := url.Parse(strings.TrimSpace(ref.Magnet))
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(uri.Query().Get("dn")), "history-manifest")
}

func isTerminalSyncFailure(ref SyncRef, result SyncItemResult) bool {
	message := strings.ToLower(strings.TrimSpace(result.Message))
	if !strings.Contains(message, "status 404") {
		return false
	}
	if isHistoryManifestRef(ref) {
		return true
	}
	return strings.Contains(message, "/api/torrents/")
}

func withPeerHints(magnet string, addrs []net.Addr, lanPeers []string) string {
	if strings.TrimSpace(magnet) == "" || len(addrs) == 0 {
		return magnet
	}
	uri, err := url.Parse(magnet)
	if err != nil {
		return magnet
	}
	query := uri.Query()
	seen := make(map[string]struct{})
	for _, existing := range query["x.pe"] {
		seen[existing] = struct{}{}
	}
	ports := make(map[string]struct{})
	for _, addr := range addrs {
		_, port, err := net.SplitHostPort(addr.String())
		if err != nil || strings.TrimSpace(port) == "" {
			continue
		}
		ports[port] = struct{}{}
	}
	if len(ports) == 0 {
		return magnet
	}
	hosts := localPeerHosts(lanPeers)
	for port := range ports {
		for _, host := range hosts {
			peerAddr := net.JoinHostPort(host, port)
			if _, ok := seen[peerAddr]; ok {
				continue
			}
			seen[peerAddr] = struct{}{}
			query.Add("x.pe", peerAddr)
		}
	}
	uri.RawQuery = query.Encode()
	return uri.String()
}

func localPeerHosts(lanPeers []string) []string {
	out := make([]string, 0, 4)
	seen := make(map[string]struct{})
	preferredSubnets := privateIPv4Subnets(lanPeers)
	fallback := make([]string, 0, 4)
	ifaces, err := net.InterfaceAddrs()
	if err != nil {
		return out
	}
	for _, iface := range ifaces {
		ipNet, ok := iface.(*net.IPNet)
		if !ok || ipNet.IP == nil {
			continue
		}
		ip := ipNet.IP
		if ip.IsUnspecified() || ip.IsMulticast() {
			continue
		}
		if ip4 := ip.To4(); ip4 != nil {
			if !isRFC1918IPv4(ip4) {
				continue
			}
			text := ip4.String()
			if _, ok := seen[text]; ok {
				continue
			}
			seen[text] = struct{}{}
			if len(preferredSubnets) == 0 || matchesAnyPrivateSubnet(ip4, preferredSubnets) {
				out = append(out, text)
				continue
			}
			fallback = append(fallback, text)
		}
	}
	if len(out) == 0 {
		out = append(out, fallback...)
	}
	return out
}

func syncMode(once bool) string {
	if once {
		return "once"
	}
	return "daemon"
}

func resolveEffectiveDHTRouters(ctx context.Context, cfg NetworkBootstrapConfig) ([]string, error) {
	merged := make([]string, 0, len(cfg.LANTorrentPeers)+len(cfg.DHTRouters))
	lanRouters, err := resolveLANTorrentRouters(ctx, cfg)
	seen := make(map[string]struct{}, len(cfg.LANTorrentPeers)+len(cfg.DHTRouters))
	for _, value := range lanRouters {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		merged = append(merged, value)
	}
	for _, value := range cfg.DHTRouters {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		merged = append(merged, value)
	}
	return merged, err
}

func effectiveDHTRouterCount(cfg NetworkBootstrapConfig) int {
	count := len(cfg.DHTRouters)
	if len(cfg.LANTorrentPeers) > 0 {
		count += len(cfg.LANTorrentPeers)
	}
	return count
}

func resolveDHTRouters(network string, routers []string) ([]anacrolixdht.Addr, error) {
	if len(routers) == 0 {
		return anacrolixdht.GlobalBootstrapAddrs(network)
	}
	out := make([]anacrolixdht.Addr, 0, len(routers))
	seen := make(map[string]struct{})
	for _, raw := range routers {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		host, port, err := net.SplitHostPort(raw)
		if err != nil {
			return nil, fmt.Errorf("parse dht router %q: %w", raw, err)
		}
		addrs, err := net.LookupIP(host)
		if err != nil {
			return nil, fmt.Errorf("resolve dht router %q: %w", raw, err)
		}
		for _, ip := range addrs {
			addr := net.JoinHostPort(ip.String(), port)
			if _, ok := seen[addr]; ok {
				continue
			}
			seen[addr] = struct{}{}
			udpAddr, err := net.ResolveUDPAddr(network, addr)
			if err != nil {
				return nil, fmt.Errorf("resolve udp addr %q: %w", addr, err)
			}
			out = append(out, anacrolixdht.NewAddr(udpAddr))
		}
	}
	if len(out) > 0 {
		return out, nil
	}
	return anacrolixdht.GlobalBootstrapAddrs(network)
}

func bootstrapTorrentDHT(client *torrent.Client, routers []string) error {
	addrs, err := resolveRouterUDPAddrs(routers)
	if err != nil {
		return err
	}
	for _, server := range client.DhtServers() {
		for _, addr := range addrs {
			server.Ping(addr)
		}
	}
	return nil
}

func resolveRouterUDPAddrs(routers []string) ([]*net.UDPAddr, error) {
	if len(routers) == 0 {
		return nil, nil
	}
	out := make([]*net.UDPAddr, 0, len(routers))
	seen := make(map[string]struct{})
	for _, raw := range routers {
		host, port, err := net.SplitHostPort(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("parse dht router %q: %w", raw, err)
		}
		ips, err := net.LookupIP(host)
		if err != nil {
			return nil, fmt.Errorf("resolve dht router %q: %w", raw, err)
		}
		for _, ip := range ips {
			addr := net.JoinHostPort(ip.String(), port)
			if _, ok := seen[addr]; ok {
				continue
			}
			seen[addr] = struct{}{}
			udpAddr, err := net.ResolveUDPAddr("udp", addr)
			if err != nil {
				return nil, fmt.Errorf("resolve udp dht router %q: %w", addr, err)
			}
			out = append(out, udpAddr)
		}
	}
	return out, nil
}

func ensureSyncLayout(store *Store, queuePath string) (string, error) {
	syncDir := filepath.Join(store.Root, "sync")
	if err := os.MkdirAll(syncDir, 0o755); err != nil {
		return "", err
	}
	queuePath = strings.TrimSpace(queuePath)
	if queuePath == "" {
		queuePath = filepath.Join(syncDir, "magnets.txt")
	}
	if err := os.MkdirAll(filepath.Dir(queuePath), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(queuePath); os.IsNotExist(err) {
		if err := os.WriteFile(queuePath, []byte("# magnet:?xt=urn:btih:...\n"), 0o644); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	return queuePath, nil
}

func collectSyncRefs(direct []string, queuePath string) ([]SyncRef, error) {
	seen := make(map[string]struct{})
	out := make([]SyncRef, 0)
	add := func(raw string) error {
		for _, part := range splitCommaRefs(raw) {
			ref, err := ParseSyncRef(part)
			if err != nil {
				return err
			}
			key := ref.Magnet
			if ref.InfoHash != "" {
				key = ref.InfoHash
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, ref)
		}
		return nil
	}
	for _, raw := range direct {
		if err := add(raw); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(queuePath) != "" {
		data, err := os.ReadFile(queuePath)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		for lineNo, rawLine := range strings.Split(string(data), "\n") {
			line := strings.TrimSpace(rawLine)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
				continue
			}
			ref, err := ParseSyncRef(line)
			if err != nil {
				return nil, fmt.Errorf("queue line %d: %w", lineNo+1, err)
			}
			key := ref.Magnet
			if ref.InfoHash != "" {
				key = ref.InfoHash
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, ref)
		}
	}
	return out, nil
}

func ParseSyncRef(raw string) (SyncRef, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return SyncRef{}, errors.New("empty sync ref")
	}
	if strings.HasPrefix(strings.ToLower(raw), "magnet:?") {
		spec, err := torrent.TorrentSpecFromMagnetUri(raw)
		if err != nil {
			return SyncRef{}, fmt.Errorf("parse magnet: %w", err)
		}
		return SyncRef{
			Raw:      raw,
			Magnet:   raw,
			InfoHash: strings.ToLower(spec.InfoHash.HexString()),
		}, nil
	}
	if isHexInfoHash(raw) {
		infoHash := strings.ToLower(raw)
		return SyncRef{
			Raw:      raw,
			Magnet:   "magnet:?xt=urn:btih:" + infoHash,
			InfoHash: infoHash,
		}, nil
	}
	return SyncRef{}, fmt.Errorf("unsupported sync ref %q", raw)
}

func sanitizeSyncQueueFile(queuePath string, lanPeers []string) (int, error) {
	queuePath = strings.TrimSpace(queuePath)
	if queuePath == "" {
		return 0, nil
	}
	data, err := os.ReadFile(queuePath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	lines := strings.Split(string(data), "\n")
	changed := 0
	for i, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
			continue
		}
		sanitized, lineChanged, err := sanitizeQueuedSyncRef(line, lanPeers)
		if err != nil {
			return changed, fmt.Errorf("sanitize queue line %d: %w", i+1, err)
		}
		if !lineChanged {
			continue
		}
		lines[i] = sanitized
		changed++
	}
	if changed == 0 {
		return 0, nil
	}
	content := strings.Join(lines, "\n")
	return changed, os.WriteFile(queuePath, []byte(content), 0o644)
}

func sanitizeQueuedSyncRef(raw string, lanPeers []string) (string, bool, error) {
	ref, err := ParseSyncRef(raw)
	if err != nil {
		return "", false, err
	}
	if strings.TrimSpace(ref.Magnet) == "" {
		return raw, false, nil
	}
	uri, err := url.Parse(ref.Magnet)
	if err != nil {
		return "", false, fmt.Errorf("parse magnet: %w", err)
	}
	query := uri.Query()
	values := query["x.pe"]
	if len(values) == 0 {
		return raw, false, nil
	}
	kept := make([]string, 0, len(values))
	for _, value := range values {
		host, _, err := net.SplitHostPort(value)
		if err != nil {
			continue
		}
		if allowTorrentHTTPHost(host, lanPeers) {
			kept = append(kept, value)
		}
	}
	if len(kept) == len(values) {
		return raw, false, nil
	}
	delete(query, "x.pe")
	for _, value := range kept {
		query.Add("x.pe", value)
	}
	uri.RawQuery = query.Encode()
	return uri.String(), true, nil
}

func syncRef(ctx context.Context, client *torrent.Client, store *Store, ref SyncRef, timeout time.Duration, lanPeers []string, trackers []string, rules SyncSubscriptions) SyncItemResult {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var (
		t   *torrent.Torrent
		err error
	)
	if ref.InfoHash != "" && hasCompleteLocalBundle(store, ref.InfoHash) {
		return SyncItemResult{
			Ref:      ref.Raw,
			InfoHash: ref.InfoHash,
			Status:   "skipped",
			Message:  "bundle already present in local store",
		}
	}
	if ref.InfoHash != "" && hasLocalTorrent(store, ref.InfoHash) {
		torrentPath, pathErr := store.ExistingTorrentPath(ref.InfoHash)
		if pathErr != nil {
			return SyncItemResult{
				Ref:      ref.Raw,
				InfoHash: ref.InfoHash,
				Status:   "failed",
				Message:  fmt.Sprintf("locate existing torrent file: %v", pathErr),
			}
		}
		t, err = addTorrentFileWithTrackers(client, torrentPath, trackers)
		if err != nil {
			return SyncItemResult{
				Ref:      ref.Raw,
				InfoHash: ref.InfoHash,
				Status:   "failed",
				Message:  fmt.Sprintf("load existing torrent file: %v", err),
			}
		}
	}
	if t == nil {
		t, err = addMagnetWithTrackers(client, ref.Magnet, trackers)
		if err != nil {
			return SyncItemResult{
				Ref:     ref.Raw,
				Status:  "failed",
				Message: fmt.Sprintf("add magnet: %v", err),
			}
		}
	}

	select {
	case <-runCtx.Done():
		path, fallbackErr := fetchTorrentFallback(ctx, store, ref, lanPeers)
		if fallbackErr != nil {
			return SyncItemResult{
				Ref:      ref.Raw,
				InfoHash: ref.InfoHash,
				Status:   "failed",
				Message:  "timed out waiting for metadata; torrent fallback failed: " + fallbackErr.Error(),
			}
		}
		t, err = addTorrentFileWithTrackers(client, path, trackers)
		if err != nil {
			return SyncItemResult{
				Ref:      ref.Raw,
				InfoHash: ref.InfoHash,
				Status:   "failed",
				Message:  fmt.Sprintf("load fallback torrent file: %v", err),
			}
		}
	case <-t.GotInfo():
	}

	infoHash := strings.ToLower(t.InfoHash().HexString())
	if info := t.Info(); info != nil && !withinMaxBundleSize(info.TotalLength(), rules.MaxBundleMB) {
		t.Drop()
		return SyncItemResult{
			Ref:      ref.Raw,
			InfoHash: infoHash,
			Status:   "skipped",
			Message:  fmt.Sprintf("bundle exceeds max_bundle_mb limit (%d MB)", rules.MaxBundleMB),
		}
	}
	t.DownloadAll()

	select {
	case <-runCtx.Done():
		return SyncItemResult{
			Ref:      ref.Raw,
			InfoHash: infoHash,
			Status:   "failed",
			Message:  "timed out waiting for bundle download",
		}
	case <-t.Complete().On():
	}

	contentDir := filepath.Join(store.DataDir, t.Name())
	msg, _, err := LoadMessage(contentDir)
	if err != nil {
		return SyncItemResult{
			Ref:      ref.Raw,
			InfoHash: infoHash,
			Status:   "failed",
			Message:  fmt.Sprintf("validate downloaded bundle: %v", err),
		}
	}
	dayCounts := localBundleDayCounts(store, contentDir)
	if !reserveDailyQuota(dayCounts, msg.CreatedAt, rules.MaxItemsPerDay) {
		t.Drop()
		_ = os.RemoveAll(contentDir)
		_ = store.RemoveTorrent(infoHash)
		return SyncItemResult{
			Ref:      ref.Raw,
			InfoHash: infoHash,
			Status:   "skipped",
			Message:  fmt.Sprintf("bundle exceeds max_items_per_day limit (%d)", rules.MaxItemsPerDay),
		}
	}
	if err := writeTorrentFile(store.TorrentPath(infoHash), t.Metainfo()); err != nil {
		return SyncItemResult{
			Ref:      ref.Raw,
			InfoHash: infoHash,
			Status:   "failed",
			Message:  fmt.Sprintf("write torrent file: %v", err),
		}
	}
	return SyncItemResult{
		Ref:        ref.Raw,
		InfoHash:   infoHash,
		ContentDir: contentDir,
		Status:     "imported",
		Message:    "bundle downloaded and indexed in local store",
	}
}

func writeTorrentFile(path string, mi metainfo.MetaInfo) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return mi.Write(file)
}

func isHexInfoHash(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, r := range value {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}

func splitCommaRefs(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}
