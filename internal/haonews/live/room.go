package live

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"hao.news/internal/haonews"

	libp2p "github.com/libp2p/go-libp2p"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	mdns "github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	routingdisc "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	discutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
)

var errSessionExitRequested = errors.New("live session exit requested")

type SessionOptions struct {
	StoreRoot         string
	NetPath           string
	IdentityFile      string
	Author            string
	RoomID            string
	Title             string
	Channel           string
	Role              string
	AutoArchive       bool
	HeartbeatInterval time.Duration
}

type session struct {
	info     RoomInfo
	identity haonews.AgentIdentity
	store    *LocalStore

	host      host.Host
	dht       *kaddht.IpfsDHT
	mdns      mdns.Service
	discovery *routingdisc.RoutingDiscovery
	pubsub    *pubsub.PubSub
	topic     *pubsub.Topic
	sub       *pubsub.Subscription

	seq             atomic.Uint64
	mu              sync.Mutex
	remoteReady     chan struct{}
	remoteReadyOnce sync.Once
	role            string
	autoArchive     bool
	storeRoot       string
	netPath         string
	identityFile    string
	author          string
}

func Host(ctx context.Context, opts SessionOptions, stdin io.Reader, stdout io.Writer) (RoomInfo, error) {
	s, err := startSession(ctx, opts)
	if err != nil {
		return RoomInfo{}, err
	}
	defer s.close()
	if err := s.publishControl(ctx, TypeJoin, LivePayload{Metadata: map[string]any{"role": s.roleValue("host")}}); err != nil {
		return RoomInfo{}, err
	}
	if err := s.run(ctx, stdin, stdout); err != nil {
		return RoomInfo{}, err
	}
	if err := s.archiveOnExit(stdout); err != nil {
		return RoomInfo{}, err
	}
	return s.info, nil
}

func Join(ctx context.Context, opts SessionOptions, stdin io.Reader, stdout io.Writer) (RoomInfo, error) {
	if strings.TrimSpace(opts.RoomID) == "" {
		return RoomInfo{}, fmt.Errorf("room-id is required")
	}
	s, err := startSession(ctx, opts)
	if err != nil {
		return RoomInfo{}, err
	}
	defer s.close()
	if err := s.publishControl(ctx, TypeJoin, LivePayload{Metadata: map[string]any{"role": s.roleValue("participant")}}); err != nil {
		return RoomInfo{}, err
	}
	if err := s.run(ctx, stdin, stdout); err != nil {
		return RoomInfo{}, err
	}
	if err := s.archiveOnExit(stdout); err != nil {
		return RoomInfo{}, err
	}
	return s.info, nil
}

func List(storeRoot string) ([]RoomSummary, error) {
	store, err := OpenLocalStore(storeRoot)
	if err != nil {
		return nil, err
	}
	return store.ListRooms()
}

func PublishTaskUpdate(ctx context.Context, opts SessionOptions, metadata map[string]any) (RoomInfo, error) {
	if strings.TrimSpace(opts.RoomID) == "" {
		return RoomInfo{}, fmt.Errorf("room-id is required")
	}
	s, err := startSession(ctx, opts)
	if err != nil {
		return RoomInfo{}, err
	}
	defer s.close()
	if err := s.publishControl(ctx, TypeJoin, LivePayload{Metadata: map[string]any{"role": "task_updater"}}); err != nil {
		return RoomInfo{}, err
	}
	errCh := make(chan error, 1)
	go s.receiveLoop(ctx, nil, errCh)
	presenceCancel := s.startPresenceLoop(ctx, 2*time.Second)
	defer presenceCancel()
	s.waitForTopicPeers(ctx, 1, 6*time.Second)
	s.waitForRemoteTraffic(ctx, 10*time.Second)
	if err := s.publishControl(ctx, TypeTaskUpdate, LivePayload{
		ContentType: "application/json",
		Metadata:    metadata,
	}); err != nil {
		return RoomInfo{}, err
	}
	select {
	case <-ctx.Done():
	case <-time.After(4 * time.Second):
	}
	_ = s.publishControl(context.Background(), TypeLeave, LivePayload{})
	return s.info, nil
}

func startSession(ctx context.Context, opts SessionOptions) (*session, error) {
	store, err := OpenLocalStore(opts.StoreRoot)
	if err != nil {
		return nil, err
	}
	identity, err := haonews.LoadAgentIdentity(strings.TrimSpace(opts.IdentityFile))
	if err != nil {
		return nil, err
	}
	author := strings.TrimSpace(opts.Author)
	if author == "" {
		author = strings.TrimSpace(identity.Author)
	}
	if author == "" {
		return nil, fmt.Errorf("author is required")
	}
	signingIdentity, _, err := haonews.ResolveSigningIdentity(identity, author, nil)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(opts.NetPath) != "" {
		if err := haonews.EnsureDefaultNetworkBootstrapConfig(opts.NetPath); err != nil {
			return nil, err
		}
	}
	netCfg, err := haonews.LoadNetworkBootstrapConfig(opts.NetPath)
	if err != nil {
		return nil, err
	}
	h, dhtRuntime, mdnsService, discoveryRuntime, ps, err := startTransport(ctx, netCfg)
	if err != nil {
		return nil, err
	}
	roomID := strings.TrimSpace(opts.RoomID)
	if roomID == "" {
		roomID, err = GenerateRoomID(author)
		if err != nil {
			return nil, err
		}
	}
	topic, err := ps.Join(RoomAnnounceTopic())
	if err != nil {
		return nil, fmt.Errorf("join live bus topic: %w", err)
	}
	sub, err := topic.Subscribe()
	if err != nil {
		_ = topic.Close()
		return nil, fmt.Errorf("subscribe live bus topic: %w", err)
	}
	info := RoomInfo{
		RoomID:          roomID,
		Title:           strings.TrimSpace(opts.Title),
		Creator:         author,
		CreatorPubKey:   strings.ToLower(strings.TrimSpace(signingIdentity.PublicKey)),
		ParentPublicKey: firstNonEmpty(strings.ToLower(strings.TrimSpace(signingIdentity.ParentPublicKey)), strings.ToLower(strings.TrimSpace(signingIdentity.PublicKey))),
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		NetworkID:       netCfg.NetworkID,
		Channel:         firstNonEmpty(strings.TrimSpace(opts.Channel), "hao.news/live"),
	}
	if normalizeRole(opts.Role) != "host" {
		if existing, err := store.LoadRoom(roomID); err == nil {
			info = mergeRoomInfo(existing, info)
		}
	}
	if err := store.SaveRoom(info); err != nil {
		sub.Cancel()
		_ = topic.Close()
		return nil, err
	}
	s := &session{
		info:         info,
		identity:     signingIdentity,
		store:        store,
		host:         h,
		dht:          dhtRuntime,
		mdns:         mdnsService,
		discovery:    discoveryRuntime,
		pubsub:       ps,
		topic:        topic,
		sub:          sub,
		remoteReady:  make(chan struct{}),
		role:         normalizeRole(opts.Role),
		autoArchive:  opts.AutoArchive,
		storeRoot:    opts.StoreRoot,
		netPath:      strings.TrimSpace(opts.NetPath),
		identityFile: strings.TrimSpace(opts.IdentityFile),
		author:       author,
	}
	if discoveryRuntime != nil {
		discutil.Advertise(ctx, discoveryRuntime, GlobalNamespace)
		discutil.Advertise(ctx, discoveryRuntime, RoomNamespace(roomID))
		go s.findPeersLoop(ctx, GlobalNamespace)
		go s.findPeersLoop(ctx, RoomNamespace(roomID))
	}
	if err := s.publishRoomAnnouncement(ctx); err != nil {
		s.close()
		return nil, err
	}
	return s, nil
}

func (s *session) run(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	errCh := make(chan error, 2)
	go s.receiveLoop(ctx, stdout, errCh)
	_ = s.publishControl(ctx, TypeHeartbeat, LivePayload{})
	_ = s.publishRoomAnnouncement(ctx)
	go s.stdinLoop(ctx, stdin, errCh)

	heartbeatEvery := 10 * time.Second
	ticker := time.NewTicker(heartbeatEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = s.publishControl(context.Background(), TypeLeave, LivePayload{})
			return nil
		case err := <-errCh:
			if errors.Is(err, errSessionExitRequested) {
				_ = s.publishControl(context.Background(), TypeLeave, LivePayload{})
				return nil
			}
			if err != nil && err != io.EOF {
				return err
			}
		case <-ticker.C:
			_ = s.publishControl(ctx, TypeHeartbeat, LivePayload{})
			_ = s.publishRoomAnnouncement(ctx)
		}
	}
}

func (s *session) stdinLoop(ctx context.Context, stdin io.Reader, errCh chan<- error) {
	if stdin == nil {
		return
	}
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		exitRequested, err := s.handleInputLine(ctx, line)
		if err != nil {
			errCh <- err
			return
		}
		if exitRequested {
			errCh <- errSessionExitRequested
			return
		}
	}
	if err := scanner.Err(); err != nil {
		errCh <- err
	}
}

func (s *session) handleInputLine(ctx context.Context, line string) (bool, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return false, nil
	}
	if isSessionExitCommand(line) {
		return true, nil
	}
	return false, s.publishMessage(ctx, line)
}

func (s *session) receiveLoop(ctx context.Context, stdout io.Writer, errCh chan<- error) {
	for {
		msg, err := s.sub.Next(ctx)
		if err != nil {
			if ctx.Err() == nil {
				errCh <- err
			}
			return
		}
		if msg.ReceivedFrom == s.host.ID() {
			continue
		}
		var event LiveMessage
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			continue
		}
		if err := VerifyMessage(event); err != nil {
			continue
		}
		s.markRemoteReady()
		if event.Type == TypeRoomAnnounce {
			info := roomInfoFromAnnouncement(event)
			if strings.TrimSpace(info.RoomID) != "" {
				_ = s.store.SaveRoom(info)
			}
			continue
		}
		if strings.TrimSpace(event.RoomID) != strings.TrimSpace(s.info.RoomID) {
			continue
		}
		if err := s.store.AppendEvent(s.info.RoomID, event); err != nil {
			errCh <- err
			return
		}
		if stdout != nil {
			fmt.Fprintf(stdout, "[%s] %s %s: %s\n", event.Timestamp, event.Type, event.Sender, strings.TrimSpace(event.Payload.Content))
		}
	}
}

func (s *session) publishMessage(ctx context.Context, content string) error {
	if err := s.prepareForManualMessage(ctx); err != nil {
		return err
	}
	msg, err := NewSignedMessage(s.identity, s.identity.Author, s.info.RoomID, TypeMessage, s.nextSeq(), 0, LivePayload{
		Content:     content,
		ContentType: "text/plain",
	})
	if err != nil {
		return err
	}
	return s.publishEvent(ctx, msg)
}

func (s *session) prepareForManualMessage(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.waitForTopicPeers(ctx, 1, 6*time.Second)
	select {
	case <-s.remoteReady:
		return nil
	default:
	}
	s.waitForRemoteTraffic(ctx, 8*time.Second)
	return ctx.Err()
}

func (s *session) publishControl(ctx context.Context, messageType string, payload LivePayload) error {
	msg, err := NewSignedMessage(s.identity, s.identity.Author, s.info.RoomID, messageType, s.nextSeq(), 0, payload)
	if err != nil {
		return err
	}
	return s.publishEvent(ctx, msg)
}

func (s *session) publishRoomAnnouncement(ctx context.Context) error {
	if s == nil || s.roleValue("participant") != "host" {
		return nil
	}
	msg, err := NewSignedMessage(s.identity, s.identity.Author, s.info.RoomID, TypeRoomAnnounce, s.nextSeq(), 0, LivePayload{
		ContentType: "application/json",
		Metadata: map[string]any{
			"title":       s.info.Title,
			"creator":     s.info.Creator,
			"created_at":  s.info.CreatedAt,
			"network_id":  s.info.NetworkID,
			"channel":     s.info.Channel,
			"description": s.info.Description,
		},
	})
	if err != nil {
		return err
	}
	return s.publishRaw(ctx, msg)
}

func (s *session) publishEvent(ctx context.Context, msg LiveMessage) error {
	if err := s.publishRaw(ctx, msg); err != nil {
		return err
	}
	return s.store.AppendEvent(s.info.RoomID, msg)
}

func (s *session) publishRaw(ctx context.Context, msg LiveMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.topic.Publish(publishCtx, body); err != nil {
		return err
	}
	return nil
}

func (s *session) waitForTopicPeers(ctx context.Context, minPeers int, waitFor time.Duration) bool {
	if s == nil || s.topic == nil || minPeers <= 0 {
		return false
	}
	if len(s.topic.ListPeers()) >= minPeers {
		return true
	}
	waitCtx, cancel := context.WithTimeout(ctx, waitFor)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-waitCtx.Done():
			return len(s.topic.ListPeers()) >= minPeers
		case <-ticker.C:
			if len(s.topic.ListPeers()) >= minPeers {
				return true
			}
		}
	}
}

func (s *session) startPresenceLoop(ctx context.Context, interval time.Duration) context.CancelFunc {
	loopCtx, cancel := context.WithCancel(ctx)
	go func() {
		_ = s.publishControl(loopCtx, TypeHeartbeat, LivePayload{})
		_ = s.publishRoomAnnouncement(loopCtx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-loopCtx.Done():
				return
			case <-ticker.C:
				_ = s.publishControl(loopCtx, TypeHeartbeat, LivePayload{})
				_ = s.publishRoomAnnouncement(loopCtx)
			}
		}
	}()
	return cancel
}

func (s *session) roleValue(defaultRole string) string {
	if s == nil {
		return strings.TrimSpace(defaultRole)
	}
	if strings.TrimSpace(s.role) != "" {
		return strings.TrimSpace(s.role)
	}
	return strings.TrimSpace(defaultRole)
}

func (s *session) archiveOnExit(stdout io.Writer) error {
	if s == nil || !s.autoArchive {
		return nil
	}
	result, err := Archive(ArchiveOptions{
		StoreRoot:    firstNonEmpty(strings.TrimSpace(s.storeRoot), filepath.Dir(filepath.Dir(s.store.Root))),
		IdentityFile: strings.TrimSpace(s.identityFile),
		Author:       firstNonEmpty(strings.TrimSpace(s.author), strings.TrimSpace(s.identity.Author)),
		RoomID:       s.info.RoomID,
		Channel:      s.info.Channel,
	})
	if err != nil {
		if stdout != nil {
			fmt.Fprintf(stdout, "[auto-archive] failed: %v\n", err)
		}
		return err
	}
	if stdout != nil {
		fmt.Fprintf(stdout, "[auto-archive] %s -> %s\n", s.info.RoomID, result.ViewerURL)
	}
	if err := s.publishArchiveNotice(context.Background(), result); err != nil {
		if stdout != nil {
			fmt.Fprintf(stdout, "[auto-archive-notice] inline publish failed: %v\n", err)
		}
		if fallbackErr := publishArchiveNoticeDetached(s.netPath, s.identity, s.info, result); fallbackErr != nil {
			if stdout != nil {
				fmt.Fprintf(stdout, "[auto-archive-notice] detached publish failed: %v\n", fallbackErr)
			}
		} else if stdout != nil {
			fmt.Fprintf(stdout, "[auto-archive-notice] detached publish ok: %s\n", result.ViewerURL)
		}
	}
	return nil
}

func (s *session) publishArchiveNotice(ctx context.Context, result ArchiveResult) error {
	if s == nil {
		return nil
	}
	msg, err := NewSignedMessage(s.identity, s.identity.Author, s.info.RoomID, TypeArchiveNotice, s.nextSeq(), 0, LivePayload{
		Content:     firstNonEmpty(strings.TrimSpace(result.ViewerURL), "/posts/"+strings.TrimSpace(result.Published.InfoHash)),
		ContentType: "application/json",
		Metadata:    archiveNoticeMetadata(s.info, result),
	})
	if err != nil {
		return err
	}
	return s.publishEvent(ctx, msg)
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "":
		return ""
	case "participant":
		return "participant"
	case "host":
		return "host"
	case "viewer":
		return "viewer"
	default:
		return "participant"
	}
}

func isSessionExitCommand(line string) bool {
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "/exit", "/quit":
		return true
	default:
		return false
	}
}

func (s *session) waitForRemoteTraffic(ctx context.Context, waitFor time.Duration) bool {
	if s == nil || s.remoteReady == nil {
		return false
	}
	select {
	case <-s.remoteReady:
		return true
	default:
	}
	waitCtx, cancel := context.WithTimeout(ctx, waitFor)
	defer cancel()
	select {
	case <-waitCtx.Done():
		select {
		case <-s.remoteReady:
			return true
		default:
			return false
		}
	case <-s.remoteReady:
		return true
	}
}

func (s *session) markRemoteReady() {
	if s == nil || s.remoteReady == nil {
		return
	}
	s.remoteReadyOnce.Do(func() {
		close(s.remoteReady)
	})
}

func (s *session) nextSeq() uint64 {
	return s.seq.Add(1)
}

func (s *session) close() {
	if s.sub != nil {
		s.sub.Cancel()
	}
	if s.topic != nil {
		_ = s.topic.Close()
	}
	if s.mdns != nil {
		_ = s.mdns.Close()
	}
	if s.dht != nil {
		_ = s.dht.Close()
	}
	if s.host != nil {
		_ = s.host.Close()
	}
}

func roomInfoFromAnnouncement(event LiveMessage) RoomInfo {
	return RoomInfo{
		RoomID:      strings.TrimSpace(event.RoomID),
		Title:       metadataStringValue(event.Payload.Metadata, "title"),
		Creator:     strings.TrimSpace(event.Sender),
		CreatorPubKey: firstNonEmpty(metadataStringValue(event.Payload.Metadata, "origin_public_key"), strings.TrimSpace(event.SenderPubKey)),
		ParentPublicKey: metadataStringValue(event.Payload.Metadata, "parent_public_key"),
		CreatedAt:   firstNonEmpty(metadataStringValue(event.Payload.Metadata, "created_at"), strings.TrimSpace(event.Timestamp)),
		NetworkID:   metadataStringValue(event.Payload.Metadata, "network_id"),
		Channel:     metadataStringValue(event.Payload.Metadata, "channel"),
		Description: metadataStringValue(event.Payload.Metadata, "description"),
	}
}

func metadataStringValue(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func (s *session) findPeersLoop(ctx context.Context, namespace string) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		s.findPeersOnce(ctx, namespace)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *session) findPeersOnce(ctx context.Context, namespace string) {
	if s.discovery == nil {
		return
	}
	findCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	peers, err := s.discovery.FindPeers(findCtx, namespace)
	if err != nil {
		cancel()
		return
	}
	for info := range peers {
		if info.ID == "" || info.ID == s.host.ID() {
			continue
		}
		connectCtx, connectCancel := context.WithTimeout(ctx, 5*time.Second)
		_ = s.host.Connect(connectCtx, info)
		connectCancel()
	}
	cancel()
}

func startTransport(ctx context.Context, cfg haonews.NetworkBootstrapConfig) (host.Host, *kaddht.IpfsDHT, mdns.Service, *routingdisc.RoutingDiscovery, *pubsub.PubSub, error) {
	var options []libp2p.Option
	resolvedLANPeers, _ := haonews.ResolveLANBootstrapPeers(ctx, cfg)
	resolvedPublicPeers, _ := haonews.ResolveExplicitBootstrapPeers(ctx, cfg.PublicPeers, cfg.NetworkID, "public_peer")
	resolvedRelayPeers, _ := haonews.ResolveExplicitBootstrapPeers(ctx, cfg.RelayPeers, cfg.NetworkID, "relay_peer")
	if factory := haonews.BuildLibP2PAddrsFactory(cfg); factory != nil {
		options = append(options, libp2p.AddrsFactory(factory))
	}
	if cfg.IsPublicMode() {
		options = append(options, libp2p.EnableRelayService())
	}
	if cfg.IsSharedMode() {
		if relayBootstrapPeers, err := parseBootstrapPeers(resolvedRelayPeers); err == nil && len(relayBootstrapPeers) > 0 {
			options = append(options,
				libp2p.EnableAutoNATv2(),
				libp2p.EnableAutoRelayWithStaticRelays(relayBootstrapPeers),
				libp2p.EnableHolePunching(),
				libp2p.ForceReachabilityPrivate(),
			)
		}
	}
	if len(cfg.LibP2PListen) > 0 {
		resolved, err := haonews.ResolveLibP2PListenAddrs(cfg.LibP2PListen)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		options = append(options, libp2p.ListenAddrStrings(resolved...))
	}
	h, err := libp2p.New(options...)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	knownGoodPeers, _ := haonews.LoadKnownGoodLibP2PBootstrapPeers(cfg)
	bootstrapPeers, err := parseBootstrapPeers(haonews.EffectiveLibP2PBootstrapPeersWithKnownGood(
		append(append([]string{}, resolvedLANPeers...), append(resolvedPublicPeers, resolvedRelayPeers...)...),
		knownGoodPeers,
		cfg.LibP2PBootstrap,
	))
	if err != nil {
		_ = h.Close()
		return nil, nil, nil, nil, nil, err
	}
	dhtOptions := []kaddht.Option{kaddht.Mode(haonews.DHTModeForConfig(cfg))}
	if len(bootstrapPeers) > 0 {
		dhtOptions = append(dhtOptions, kaddht.BootstrapPeers(bootstrapPeers...))
	}
	dhtRuntime, err := kaddht.New(ctx, h, dhtOptions...)
	if err != nil {
		_ = h.Close()
		return nil, nil, nil, nil, nil, err
	}
	go func() {
		bootstrapCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		_ = dhtRuntime.Bootstrap(bootstrapCtx)
		cancel()
	}()
	for _, info := range bootstrapPeers {
		connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_ = h.Connect(connectCtx, info)
		cancel()
	}
	serviceName := "_haonews-live._udp"
	if strings.TrimSpace(cfg.NetworkID) != "" && len(cfg.NetworkID) >= 12 {
		serviceName = "_haonews-live-" + cfg.NetworkID[:12] + "._udp"
	}
	mdnsService := mdns.NewMdnsService(h, serviceName, mdns.Notifee(&liveMDNSNotifee{host: h}))
	if err := mdnsService.Start(); err != nil {
		_ = dhtRuntime.Close()
		_ = h.Close()
		return nil, nil, nil, nil, nil, err
	}
	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		_ = mdnsService.Close()
		_ = dhtRuntime.Close()
		_ = h.Close()
		return nil, nil, nil, nil, nil, err
	}
	return h, dhtRuntime, mdnsService, routingdisc.NewRoutingDiscovery(dhtRuntime), ps, nil
}

type liveMDNSNotifee struct {
	host host.Host
}

func (n *liveMDNSNotifee) HandlePeerFound(info peer.AddrInfo) {
	if info.ID == "" || n.host == nil || info.ID == n.host.ID() {
		return
	}
	connectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = n.host.Connect(connectCtx, info)
}

func parseBootstrapPeers(values []string) ([]peer.AddrInfo, error) {
	out := make([]peer.AddrInfo, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		info, err := peer.AddrInfoFromString(value)
		if err != nil {
			return nil, fmt.Errorf("parse live bootstrap peer %q: %w", value, err)
		}
		if _, ok := seen[info.ID.String()]; ok {
			continue
		}
		seen[info.ID.String()] = struct{}{}
		out = append(out, *info)
	}
	return out, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
