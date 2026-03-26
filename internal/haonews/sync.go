package haonews

import (
	"context"
	"encoding/json"
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
	"github.com/libp2p/go-libp2p/core/peer"
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
	DirectTransfer     bool
}

type SyncRef struct {
	Raw      string
	Magnet   string
	InfoHash string
	Queue    string
}

type SyncItemResult struct {
	Ref        string `json:"ref"`
	InfoHash   string `json:"infohash,omitempty"`
	ContentDir string `json:"content_dir,omitempty"`
	Status     string `json:"status"`
	Transport  string `json:"transport,omitempty"`
	Message    string `json:"message,omitempty"`
}

const (
	defaultSyncRefTimeout = 20 * time.Second
	maxSyncRefsPerPass    = 3
	lanHealthProbeEvery   = 60 * time.Second
	recentRealtimeWindow  = 2 * time.Hour
)

func RunSync(ctx context.Context, opts SyncOptions, logf func(string, ...any)) error {
	store, err := OpenStore(opts.StoreRoot)
	if err != nil {
		return err
	}
	queues, err := ensureSyncLayout(store, opts.QueuePath)
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

	libp2pRuntime, err := startLibP2PRuntime(ctx, netCfg, store)
	if err != nil {
		return err
	}
	defer libp2pRuntime.Close()

	runtime := &syncRuntime{
		store:            store,
		queuePath:        queues.RealtimePath,
		historyQueuePath: queues.HistoryPath,
		mode:             syncMode(opts.Once),
		seed:             opts.Seed,
		startedAt:        time.Now().UTC(),
		libp2p:           libp2pRuntime,
		netCfg:           netCfg,
		trackers:         trackers,
		subscriptions:    subscriptions,
		announced:        make(map[string]struct{}),
		announcedProofs:  make(map[string]struct{}),
		seeded:           make(map[string]struct{}),
		directTransfer:   opts.DirectTransfer,
		directPeers:      make(map[string][]peer.ID),
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
	if state, err := loadHistoryBootstrapState(store); err == nil {
		runtime.historyBootstrap = state
	} else if logf != nil {
		logf("load history bootstrap state: %v", err)
	}
	if err := runtime.writeStatus(ctx); err != nil && logf != nil {
		logf("write sync status: %v", err)
	}
	if logf != nil {
		logf("sync queue: realtime=%s history=%s", queues.RealtimePath, queues.HistoryPath)
		if netCfg.Exists {
			logf("network bootstrap file: %s", netCfg.FileName())
			logf("configured libp2p peers: %d", len(netCfg.LibP2PBootstrap))
			logf("configured libp2p rendezvous namespaces: %d", len(netCfg.LibP2PRendezvous))
			logf("bittorrent transport: disabled")
		} else if strings.TrimSpace(opts.NetPath) != "" {
			logf("network bootstrap file not found: %s", opts.NetPath)
		}
		if !subscriptions.Empty() {
			logf("subscription filters: %d channels, %d topics, %d tags", len(subscriptions.Channels), len(subscriptions.Topics), len(subscriptions.Tags))
		}
	}

	if opts.Once {
		if err := runtime.probeLANAnchors(ctx, logf); err != nil && logf != nil {
			logf("probe LAN anchors: %v", err)
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
		if err := runtime.maybeProbeLANAnchors(ctx, logf); err != nil && logf != nil {
			logf("probe LAN anchors: %v", err)
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
	mu                 sync.Mutex
	store              *Store
	queuePath          string
	historyQueuePath   string
	mode               string
	seed               bool
	startedAt          time.Time
	torrentClient      *torrent.Client
	libp2p             *libp2pRuntime
	pubsub             *pubsubRuntime
	creditStore        *CreditStore
	creditIdentity     *AgentIdentity
	netCfg             NetworkBootstrapConfig
	trackers           []string
	subscriptions      SyncSubscriptions
	announced          map[string]struct{}
	announcedProofs    map[string]struct{}
	seeded             map[string]struct{}
	directTransfer     bool
	directPeers        map[string][]peer.ID
	activity           SyncActivityStatus
	configuredBTListen string
	lastLANProbeAt     time.Time
	historyBootstrap   historyBootstrapState
}

type historyBootstrapState struct {
	FirstSyncCompleted     bool       `json:"first_sync_completed"`
	HistoryBootstrapMode   string     `json:"history_bootstrap_mode,omitempty"`
	LastHistoryBootstrapAt *time.Time `json:"last_history_bootstrap_at,omitempty"`
	RecentPagesLimit       int        `json:"recent_pages_limit,omitempty"`
	RecentRefsLimit        int        `json:"recent_refs_limit,omitempty"`
}

type syncQueueLayout struct {
	RealtimePath string
	HistoryPath  string
	LegacyPath   string
}

func (r *syncRuntime) setQueueRefs(realtime, history int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.activity.RealtimeQueueRefs = realtime
	r.activity.HistoryQueueRefs = history
	r.activity.QueueRefs = realtime + history
}

func (r *syncRuntime) recordResult(result SyncItemResult) {
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.activity.LastRef = result.Ref
	r.activity.LastInfoHash = result.InfoHash
	r.activity.LastStatus = result.Status
	r.activity.LastTransport = result.Transport
	r.activity.LastMessage = result.Message
	r.activity.LastEventAt = &now
	switch result.Status {
	case "imported":
		r.activity.Imported++
		switch result.Transport {
		case "libp2p":
			r.activity.DirectImported++
		case "bittorrent":
			r.activity.BitTorrentImported++
		}
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

func (r *syncRuntime) rememberDirectPeer(infoHash, peerValue string) {
	if r == nil {
		return
	}
	infoHash = normalizeInfoHash(infoHash)
	peerValue = strings.TrimSpace(peerValue)
	if infoHash == "" || peerValue == "" {
		return
	}
	peerID, err := peer.Decode(peerValue)
	if err != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.directPeers[infoHash]
	for _, existing := range current {
		if existing == peerID {
			return
		}
	}
	r.directPeers[infoHash] = append(current, peerID)
}

func (r *syncRuntime) directPeerIDs(infoHash string) []peer.ID {
	if r == nil {
		return nil
	}
	infoHash = normalizeInfoHash(infoHash)
	if infoHash == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.directPeers[infoHash]
	if len(current) == 0 {
		return nil
	}
	out := make([]peer.ID, len(current))
	copy(out, current)
	return out
}

func (r *syncRuntime) clearDirectPeers(infoHash string) {
	if r == nil {
		return
	}
	infoHash = normalizeInfoHash(infoHash)
	if infoHash == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.directPeers, infoHash)
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
		HistoryBootstrap: SyncHistoryBootstrapStatus{
			FirstSyncCompleted:     r.historyBootstrap.FirstSyncCompleted,
			Mode:                   r.historyBootstrap.HistoryBootstrapMode,
			LastHistoryBootstrapAt: r.historyBootstrap.LastHistoryBootstrapAt,
			RecentPagesLimit:       r.historyBootstrap.RecentPagesLimit,
			RecentRefsLimit:        r.historyBootstrap.RecentRefsLimit,
		},
	}
	status.LibP2P = r.libp2p.Status(ctx)
	status.BitTorrentDHT = torrentStatus(nil, 0, "")
	status.PubSub = r.pubsub.Status()
	return writeSyncStatus(r.store, status)
}

func (r *syncRuntime) processQueue(ctx context.Context, direct []string, timeout time.Duration, logf func(string, ...any)) error {
	realtimeRefs, historyRefs, err := collectSyncRefs(direct, r.queuePath, r.historyQueuePath)
	if err != nil {
		return err
	}
	refs := append([]SyncRef(nil), realtimeRefs...)
	remainingSlots := maxSyncRefsPerPass - len(refs)
	if remainingSlots > 0 && len(historyRefs) > 0 {
		historyBatch := historyRefs
		if len(historyBatch) > remainingSlots {
			historyBatch = historyBatch[:remainingSlots]
		}
		refs = append(refs, historyBatch...)
		if !r.historyBootstrap.FirstSyncCompleted && logf != nil {
			logf("history bootstrap recent mode: process %d realtime refs and %d history refs", len(realtimeRefs), len(historyBatch))
		}
	}
	if len(refs) > maxSyncRefsPerPass {
		refs = refs[:maxSyncRefsPerPass]
	}
	r.setQueueRefs(len(realtimeRefs), len(historyRefs))
	if err := r.writeStatus(ctx); err != nil && logf != nil {
		logf("write sync status: %v", err)
	}
	for _, ref := range refs {
		result := syncRef(ctx, nil, r.store, ref, timeout, r.netCfg.LANPeers, r.trackers, r.subscriptions, r.directTransfer, r.libp2p, r.directPeerIDs(ref.InfoHash))
		if result.Status == "imported" && result.ContentDir != "" {
			if err := r.importCreditBundle(result.ContentDir, logf); err != nil && logf != nil {
				logf("import credit bundle: %v", err)
			}
		}
		r.recordResult(result)
		if result.Status == "imported" || result.Status == "skipped" {
			r.clearDirectPeers(ref.InfoHash)
			if err := removeSyncRef(ref.Queue, ref); err != nil && logf != nil {
				logf("remove sync ref: %v", err)
			}
		} else if result.Status == "failed" {
			if isTerminalSyncFailure(ref, result) {
				if err := removeSyncRef(ref.Queue, ref); err != nil && logf != nil {
					logf("drop terminal sync ref: %v", err)
				}
			} else if err := rotateSyncRef(ref.Queue, ref); err != nil && logf != nil {
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
		logf("sanitized %d realtime magnet refs", changed)
	}
	if changed, err := sanitizeSyncQueueFile(r.historyQueuePath, r.netCfg.LANPeers); err != nil {
		return err
	} else if changed > 0 && logf != nil {
		logf("sanitized %d history magnet refs", changed)
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
			return r.maybeCompleteHistoryBootstrap(logf)
		}
		direct = nil
	}
	return r.maybeCompleteHistoryBootstrap(logf)
}

func (r *syncRuntime) announceLocalBundles(ctx context.Context, logf func(string, ...any)) error {
	if r.pubsub == nil {
		return nil
	}
	if err := ensureHistoryManifests(r.store, r.netCfg, nil); err != nil {
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
	if r == nil || r.torrentClient == nil {
		return nil
	}
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
	r.rememberDirectPeer(ref.InfoHash, announcement.LibP2PPeerID)
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
	maxAdds := 0
	if r.inRecentHistoryBootstrap() {
		maxAdds = r.subscriptions.historyMaxItems()
	}
	added, err := enqueueHistoryManifestRefs(r.store, r.historyQueuePath, r.subscriptions, r.netCfg.NetworkID, maxAdds)
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
	maxPages := 32
	remainingRefs := 0
	recentDays := 0
	if r.inRecentHistoryBootstrap() {
		remainingRefs = r.subscriptions.historyMaxItems()
		maxPages = max(1, (remainingRefs+historyManifestPageSize-1)/historyManifestPageSize)
		recentDays = r.subscriptions.historyDays()
		if err := r.ensureHistoryBootstrapStarted(); err != nil && logf != nil {
			logf("write history bootstrap state: %v", err)
		}
	}
	for _, peerValue := range r.netCfg.LANPeers {
		cursor := ""
		for page := 1; page <= maxPages; page++ {
			if remainingRefs == 0 && r.inRecentHistoryBootstrap() {
				return added, nil
			}
			payload, err := fetchLANHistoryManifest(ctx, peerValue, cursor, r.netCfg.NetworkID)
			if err != nil {
				if logf != nil {
					logf("fetch lan history manifest from %s cursor=%q: %v", peerValue, cursor, err)
				}
				break
			}
			for _, announcement := range payload.Entries {
				announcement = normalizeAnnouncement(announcement)
				if announcement.NetworkID == "" {
					announcement.NetworkID = payload.NetworkID
				}
				if r.netCfg.NetworkID != "" && announcement.NetworkID != "" && !strings.EqualFold(announcement.NetworkID, r.netCfg.NetworkID) {
					continue
				}
				if recentDays > 0 && !withinMaxAge(announcement.CreatedAt, recentDays) {
					continue
				}
				if !matchesHistoryAnnouncement(announcement, r.subscriptions) {
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
				queuePath := r.historyQueuePath
				if shouldPromoteHistoryAnnouncementToRealtime(page, announcement) {
					queuePath = r.queuePath
				}
				enqueued, err := enqueueSyncRef(queuePath, ref)
				if err != nil {
					return added, err
				}
				if enqueued {
					added++
					if remainingRefs > 0 {
						remainingRefs--
					}
				}
			}
			if strings.TrimSpace(payload.NextCursor) == "" || !payload.HasMore {
				break
			}
			cursor = payload.NextCursor
		}
	}
	return added, nil
}

func (r *syncRuntime) inRecentHistoryBootstrap() bool {
	return !r.historyBootstrap.FirstSyncCompleted
}

func shouldPromoteHistoryAnnouncementToRealtime(page int, announcement SyncAnnouncement) bool {
	if page != 1 {
		return false
	}
	createdAt := strings.TrimSpace(announcement.CreatedAt)
	if createdAt == "" {
		return false
	}
	parsed, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return false
	}
	age := time.Since(parsed.UTC())
	return age >= 0 && age <= recentRealtimeWindow
}

func (r *syncRuntime) ensureHistoryBootstrapStarted() error {
	if r == nil || r.store == nil || r.historyBootstrap.FirstSyncCompleted {
		return nil
	}
	now := time.Now().UTC()
	r.historyBootstrap.HistoryBootstrapMode = "recent"
	r.historyBootstrap.RecentRefsLimit = r.subscriptions.historyMaxItems()
	r.historyBootstrap.RecentPagesLimit = max(1, (r.historyBootstrap.RecentRefsLimit+historyManifestPageSize-1)/historyManifestPageSize)
	r.historyBootstrap.LastHistoryBootstrapAt = &now
	return writeHistoryBootstrapState(r.store, r.historyBootstrap)
}

func (r *syncRuntime) maybeCompleteHistoryBootstrap(logf func(string, ...any)) error {
	if r == nil || r.store == nil || r.historyBootstrap.FirstSyncCompleted {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(r.historyBootstrap.HistoryBootstrapMode), "recent") {
		return nil
	}
	realtimeRefs, historyRefs, err := collectSyncRefs(nil, r.queuePath, r.historyQueuePath)
	if err != nil {
		return err
	}
	r.setQueueRefs(len(realtimeRefs), len(historyRefs))
	if len(realtimeRefs) > 0 {
		return nil
	}
	now := time.Now().UTC()
	r.historyBootstrap.FirstSyncCompleted = true
	r.historyBootstrap.HistoryBootstrapMode = "steady"
	r.historyBootstrap.LastHistoryBootstrapAt = &now
	if logf != nil {
		logf("history bootstrap completed; switching to steady mode")
	}
	return writeHistoryBootstrapState(r.store, r.historyBootstrap)
}

func (r *syncRuntime) maybeProbeLANAnchors(ctx context.Context, logf func(string, ...any)) error {
	if r == nil {
		return nil
	}
	if !r.lastLANProbeAt.IsZero() && time.Since(r.lastLANProbeAt) < lanHealthProbeEvery {
		return nil
	}
	return r.probeLANAnchors(ctx, logf)
}

func (r *syncRuntime) probeLANAnchors(ctx context.Context, logf func(string, ...any)) error {
	if r == nil {
		return nil
	}
	r.lastLANProbeAt = time.Now().UTC()
	var errs []string

	if len(r.netCfg.LANPeers) > 0 {
		probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		peers, err := resolveLANBootstrapPeers(probeCtx, r.netCfg)
		cancel()
		if err != nil {
			errs = append(errs, err.Error())
		} else if logf != nil {
			logf("LAN libp2p anchors healthy: %d", len(peers))
		}
	}

	if len(r.netCfg.LANTorrentPeers) > 0 {
		if logf != nil {
			logf("LAN BT anchors: disabled")
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func torrentStatus(client *torrent.Client, configuredRouters int, configuredListen string) SyncBitTorrentStatus {
	if client == nil {
		return SyncBitTorrentStatus{
			Enabled:           false,
			ConfiguredListen:  configuredListen,
			ConfiguredRouters: configuredRouters,
			LastError:         "disabled",
		}
	}
	listenAddrs := make([]string, 0, len(client.ListenAddrs()))
	for _, addr := range client.ListenAddrs() {
		listenAddrs = append(listenAddrs, addr.String())
	}
	status := SyncBitTorrentStatus{
		Enabled:           len(client.DhtServers()) > 0,
		ConfiguredListen:  configuredListen,
		ListenAddrs:       listenAddrs,
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
	if strings.TrimSpace(queuePath) == "" {
		return nil
	}
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
	if strings.TrimSpace(queuePath) == "" {
		return nil
	}
	if err := removeSyncRef(queuePath, ref); err != nil {
		return err
	}
	_, err := enqueueSyncRef(queuePath, ref)
	return err
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

func ensureSyncLayout(store *Store, queuePath string) (syncQueueLayout, error) {
	syncDir := filepath.Join(store.Root, "sync")
	if err := os.MkdirAll(syncDir, 0o755); err != nil {
		return syncQueueLayout{}, err
	}
	queuePath = strings.TrimSpace(queuePath)
	layout := syncQueueLayout{}
	if queuePath == "" {
		layout.RealtimePath = filepath.Join(syncDir, "realtime.txt")
		layout.HistoryPath = filepath.Join(syncDir, "history.txt")
		layout.LegacyPath = filepath.Join(syncDir, "magnets.txt")
	} else {
		layout.RealtimePath = queuePath
		layout.HistoryPath = queuePath + ".history"
	}
	if err := ensureQueueFile(layout.RealtimePath, "# realtime sync refs\n"); err != nil {
		return syncQueueLayout{}, err
	}
	if err := ensureQueueFile(layout.HistoryPath, "# history sync refs\n"); err != nil {
		return syncQueueLayout{}, err
	}
	if layout.LegacyPath != "" {
		if err := migrateLegacySyncQueue(layout); err != nil {
			return syncQueueLayout{}, err
		}
	}
	return layout, nil
}

func historyBootstrapStatePath(store *Store) string {
	return filepath.Join(store.Root, "sync", "bootstrap_history_state.json")
}

func loadHistoryBootstrapState(store *Store) (historyBootstrapState, error) {
	path := historyBootstrapStatePath(store)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return historyBootstrapState{}, nil
		}
		return historyBootstrapState{}, err
	}
	var state historyBootstrapState
	if err := json.Unmarshal(data, &state); err != nil {
		return historyBootstrapState{}, err
	}
	return state, nil
}

func writeHistoryBootstrapState(store *Store, state historyBootstrapState) error {
	path := historyBootstrapStatePath(store)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func ensureQueueFile(path, header string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.WriteFile(path, []byte(header), 0o644)
	} else if err != nil {
		return err
	}
	return nil
}

func migrateLegacySyncQueue(layout syncQueueLayout) error {
	legacyPath := strings.TrimSpace(layout.LegacyPath)
	if legacyPath == "" {
		return nil
	}
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	migrated := 0
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
			continue
		}
		ref, err := ParseSyncRef(line)
		if err != nil {
			continue
		}
		if _, err := enqueueSyncRef(layout.HistoryPath, ref); err != nil {
			return err
		}
		migrated++
	}
	if migrated == 0 {
		return nil
	}
	return os.WriteFile(legacyPath, []byte("# legacy queue migrated to history.txt\n"), 0o644)
}

func QueueSyncRefForStore(storeRoot, raw string) (bool, error) {
	store, err := OpenStore(storeRoot)
	if err != nil {
		return false, err
	}
	layout, err := ensureSyncLayout(store, "")
	if err != nil {
		return false, err
	}
	ref, err := ParseSyncRef(raw)
	if err != nil {
		return false, err
	}
	return enqueueSyncRef(layout.RealtimePath, ref)
}

func collectSyncRefs(direct []string, realtimeQueuePath, historyQueuePath string) ([]SyncRef, []SyncRef, error) {
	seen := make(map[string]struct{})
	realtime := make([]SyncRef, 0)
	history := make([]SyncRef, 0)
	add := func(raw string, queuePath string, target *[]SyncRef) error {
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
			ref.Queue = strings.TrimSpace(queuePath)
			*target = append(*target, ref)
		}
		return nil
	}
	for _, raw := range direct {
		if err := add(raw, "", &realtime); err != nil {
			return nil, nil, err
		}
	}
	readQueue := func(queuePath string, target *[]SyncRef) error {
		if strings.TrimSpace(queuePath) == "" {
			return nil
		}
		data, err := os.ReadFile(queuePath)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		for lineNo, rawLine := range strings.Split(string(data), "\n") {
			line := strings.TrimSpace(rawLine)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
				continue
			}
			ref, err := ParseSyncRef(line)
			if err != nil {
				return fmt.Errorf("%s line %d: %w", filepath.Base(queuePath), lineNo+1, err)
			}
			key := ref.Magnet
			if ref.InfoHash != "" {
				key = ref.InfoHash
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			ref.Queue = queuePath
			*target = append(*target, ref)
		}
		return nil
	}
	if err := readQueue(realtimeQueuePath, &realtime); err != nil {
		return nil, nil, err
	}
	if err := readQueue(historyQueuePath, &history); err != nil {
		return nil, nil, err
	}
	sort.SliceStable(history, func(i, j int) bool {
		return syncRefPriority(history[i]) < syncRefPriority(history[j])
	})
	return realtime, history, nil
}

func syncRefPriority(ref SyncRef) int {
	if isHistoryManifestRef(ref) {
		return 1
	}
	return 0
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

func syncRef(
	ctx context.Context,
	client *torrent.Client,
	store *Store,
	ref SyncRef,
	timeout time.Duration,
	lanPeers []string,
	trackers []string,
	rules SyncSubscriptions,
	directTransfer bool,
	libp2pRuntime *libp2pRuntime,
	directPeerIDs []peer.ID,
) SyncItemResult {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var (
		directAttempted    bool
		directFailureNotes []string
	)
	if ref.InfoHash != "" && hasCompleteLocalBundle(store, ref.InfoHash) {
		return SyncItemResult{
			Ref:      ref.Raw,
			InfoHash: ref.InfoHash,
			Status:   "skipped",
			Message:  "bundle already present in local store",
		}
	}
	if directTransfer && ref.InfoHash != "" && libp2pRuntime != nil && libp2pRuntime.host != nil && len(directPeerIDs) > 0 {
		directAttempted = true
		for _, peerID := range directPeerIDs {
			contentDir, fetchErr := FetchBundleViaLibP2P(runCtx, libp2pRuntime.host, peerID, ref.InfoHash, store, libp2pRuntime.transferMaxSize)
			if fetchErr != nil {
				directFailureNotes = append(directFailureNotes, peerID.String()+": "+fetchErr.Error())
				continue
			}
			msg, _, loadErr := LoadMessage(contentDir)
			if loadErr != nil {
				return SyncItemResult{
					Ref:       ref.Raw,
					InfoHash:  ref.InfoHash,
					Status:    "failed",
					Transport: "libp2p",
					Message:   fmt.Sprintf("validate transferred bundle: %v", loadErr),
				}
			}
			dayCounts := localBundleDayCounts(store, contentDir)
			if !reserveDailyQuota(dayCounts, msg.CreatedAt, rules.MaxItemsPerDay) {
				_ = os.RemoveAll(contentDir)
				_ = store.RemoveTorrent(ref.InfoHash)
				return SyncItemResult{
					Ref:       ref.Raw,
					InfoHash:  ref.InfoHash,
					Status:    "skipped",
					Transport: "libp2p",
					Message:   fmt.Sprintf("bundle exceeds max_items_per_day limit (%d)", rules.MaxItemsPerDay),
				}
			}
			return SyncItemResult{
				Ref:        ref.Raw,
				InfoHash:   ref.InfoHash,
				ContentDir: contentDir,
				Status:     "imported",
				Transport:  "libp2p",
				Message:    "bundle transferred via libp2p direct stream from " + peerID.String(),
			}
		}
	}
	contentDir, fallbackErr := fetchBundleFallback(runCtx, store, ref, lanPeers, rules.MaxBundleMB)
	if fallbackErr == nil {
		message := "bundle imported via HTTP fallback"
		if directAttempted && len(directFailureNotes) > 0 {
			message = "libp2p direct transfer failed; bundle imported via HTTP fallback"
		}
		return SyncItemResult{
			Ref:        ref.Raw,
			InfoHash:   ref.InfoHash,
			ContentDir: contentDir,
			Status:     "imported",
			Transport:  "http",
			Message:    message,
		}
	}
	message := "http fallback failed: " + fallbackErr.Error()
	if directAttempted && len(directFailureNotes) > 0 {
		message = "libp2p direct transfer failed; " + message
	}
	return SyncItemResult{
		Ref:       ref.Raw,
		InfoHash:  ref.InfoHash,
		Status:    "failed",
		Transport: "http",
		Message:   message,
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
