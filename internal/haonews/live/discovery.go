package live

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"hao.news/internal/haonews"

	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	mdns "github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	routingdisc "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	discutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	"github.com/multiformats/go-multiaddr"
)

type AnnouncementWatcher struct {
	cancel    context.CancelFunc
	done      chan struct{}
	store     *LocalStore
	host      host.Host
	dht       *kaddht.IpfsDHT
	mdns      mdns.Service
	discovery *routingdisc.RoutingDiscovery
	topic     *pubsub.Topic
	sub       *pubsub.Subscription
	networkID string
}

type BootstrapStatus struct {
	NetworkID  string   `json:"network_id,omitempty"`
	PeerID     string   `json:"peer_id,omitempty"`
	ListenPort int      `json:"listen_port,omitempty"`
	DialAddrs  []string `json:"dial_addrs,omitempty"`
}

func StartAnnouncementWatcher(parent context.Context, storeRoot, netPath string) (*AnnouncementWatcher, error) {
	if strings.TrimSpace(netPath) != "" {
		if err := haonews.EnsureDefaultNetworkBootstrapConfig(netPath); err != nil {
			return nil, err
		}
	}
	netCfg, err := haonews.LoadNetworkBootstrapConfig(netPath)
	if err != nil {
		return nil, err
	}
	store, err := OpenLocalStoreWithRedis(storeRoot, netCfg.Redis)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	h, dhtRuntime, mdnsService, discoveryRuntime, ps, err := startTransport(ctx, netCfg)
	if err != nil {
		cancel()
		return nil, err
	}
	topic, err := ps.Join(RoomAnnounceTopic())
	if err != nil {
		cancel()
		_ = mdnsService.Close()
		_ = dhtRuntime.Close()
		_ = h.Close()
		return nil, fmt.Errorf("join room announce topic: %w", err)
	}
	sub, err := topic.Subscribe()
	if err != nil {
		cancel()
		_ = topic.Close()
		_ = mdnsService.Close()
		_ = dhtRuntime.Close()
		_ = h.Close()
		return nil, fmt.Errorf("subscribe room announce topic: %w", err)
	}
	watcher := &AnnouncementWatcher{
		cancel:    cancel,
		done:      make(chan struct{}),
		store:     store,
		host:      h,
		dht:       dhtRuntime,
		mdns:      mdnsService,
		discovery: discoveryRuntime,
		topic:     topic,
		sub:       sub,
		networkID: strings.TrimSpace(netCfg.NetworkID),
	}
	if discoveryRuntime != nil {
		discutil.Advertise(ctx, discoveryRuntime, GlobalNamespace)
		go watcher.findPeersLoop(ctx, GlobalNamespace)
	}
	go watcher.run(ctx)
	return watcher, nil
}

func (w *AnnouncementWatcher) BootstrapStatus() *BootstrapStatus {
	if w == nil || w.host == nil {
		return nil
	}
	addrs := w.host.Addrs()
	dialAddrs := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		value := strings.TrimSpace(addr.String())
		if value == "" {
			continue
		}
		if !strings.Contains(value, "/p2p/") {
			value += "/p2p/" + w.host.ID().String()
		}
		dialAddrs = append(dialAddrs, value)
	}
	return &BootstrapStatus{
		NetworkID:  w.networkID,
		PeerID:     w.host.ID().String(),
		ListenPort: firstLiveListenPort(addrs),
		DialAddrs:  dialAddrs,
	}
}

func firstLiveListenPort(addrs []multiaddr.Multiaddr) int {
	for _, addr := range addrs {
		port, ok := parseMultiaddrPort(addr.String())
		if ok {
			return port
		}
	}
	return 0
}

func parseMultiaddrPort(addr string) (int, bool) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return 0, false
	}
	for _, prefix := range []string{"/tcp/", "/udp/"} {
		idx := strings.LastIndex(addr, prefix)
		if idx < 0 {
			continue
		}
		rest := addr[idx+len(prefix):]
		if rest == "" {
			continue
		}
		parts := strings.Split(rest, "/")
		if len(parts) == 0 {
			continue
		}
		port, err := strconv.Atoi(parts[0])
		if err != nil || port <= 0 {
			continue
		}
		return port, true
	}
	return 0, false
}

func (w *AnnouncementWatcher) Close() error {
	if w == nil {
		return nil
	}
	w.cancel()
	<-w.done
	return nil
}

func (w *AnnouncementWatcher) run(ctx context.Context) {
	defer close(w.done)
	defer w.shutdown()
	for {
		msg, err := w.sub.Next(ctx)
		if err != nil {
			return
		}
		if msg.ReceivedFrom == w.host.ID() {
			continue
		}
		var event LiveMessage
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			continue
		}
		if err := VerifyMessage(event); err != nil {
			continue
		}
		if err := w.handleEvent(event); err != nil {
			continue
		}
	}
}

func (w *AnnouncementWatcher) handleEvent(event LiveMessage) error {
	switch event.Type {
	case TypeRoomAnnounce:
		info := roomInfoFromAnnouncement(event)
		if strings.TrimSpace(info.RoomID) == "" {
			return nil
		}
		return w.store.SaveRoomAuthoritative(info)
	case TypeArchiveNotice:
		info := roomInfoFromAnnouncement(event)
		if strings.TrimSpace(info.RoomID) == "" {
			return nil
		}
		if err := w.store.SaveRoomAuthoritative(info); err != nil {
			return err
		}
		if err := w.store.AppendEvent(info.RoomID, event); err != nil {
			return err
		}
		if result, ok := archiveResultFromNotice(event); ok {
			if err := w.store.SaveArchiveResult(info.RoomID, result); err != nil {
				return err
			}
			if ref := archiveSyncRefFromNotice(event); strings.TrimSpace(ref) != "" {
				_, _ = haonews.QueueSyncRefForStore(filepath.Dir(w.store.Root), ref)
			}
		}
	case TypeJoin, TypeLeave, TypeHeartbeat, TypeMessage, TypeTaskUpdate:
		if strings.TrimSpace(event.RoomID) == "" {
			return nil
		}
		if _, err := w.store.LoadRoom(strings.TrimSpace(event.RoomID)); err != nil {
			if !os.IsNotExist(err) {
				return err
			}
			if err := w.store.SaveRoom(RoomInfo{
				RoomID:    strings.TrimSpace(event.RoomID),
				CreatedAt: strings.TrimSpace(event.Timestamp),
			}); err != nil {
				return err
			}
		}
		if err := w.store.AppendEvent(strings.TrimSpace(event.RoomID), event); err != nil {
			return err
		}
	}
	return nil
}

func (w *AnnouncementWatcher) shutdown() {
	if w.sub != nil {
		w.sub.Cancel()
	}
	if w.topic != nil {
		_ = w.topic.Close()
	}
	if w.mdns != nil {
		_ = w.mdns.Close()
	}
	if w.dht != nil {
		_ = w.dht.Close()
	}
	if w.host != nil {
		_ = w.host.Close()
	}
	if w.store != nil {
		_ = w.store.Close()
	}
}

func (w *AnnouncementWatcher) findPeersLoop(ctx context.Context, namespace string) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		w.findPeersOnce(ctx, namespace)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *AnnouncementWatcher) findPeersOnce(ctx context.Context, namespace string) {
	if w.discovery == nil || w.host == nil {
		return
	}
	findCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	peers, err := w.discovery.FindPeers(findCtx, namespace)
	if err != nil {
		cancel()
		return
	}
	for info := range peers {
		if info.ID == "" || info.ID == w.host.ID() {
			continue
		}
		connectCtx, connectCancel := context.WithTimeout(ctx, 5*time.Second)
		_ = w.host.Connect(connectCtx, info)
		connectCancel()
	}
	cancel()
}
