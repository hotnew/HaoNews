package haonewslive

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hao.news/internal/apphost"
	core "hao.news/internal/haonews"
	"hao.news/internal/haonews/live"
	haonewstheme "hao.news/internal/themes/haonews"
)

func TestPluginBuildServesLiveIndex(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	store, err := live.OpenLocalStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	if err := store.SaveRoom(live.RoomInfo{
		RoomID:    "room-1",
		Title:     "Live Test",
		Creator:   "agent://pc75/openclaw01",
		CreatedAt: "2026-03-19T00:00:00Z",
		Channel:   "hao.news/live",
	}); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Live Test") {
		t.Fatalf("expected live room in body, got %q", rec.Body.String())
	}
}

func TestLiveAnnouncementWatcherNetPath(t *testing.T) {
	prevWatcherDisabled := liveAnnouncementWatcherDisabledForTests
	liveAnnouncementWatcherDisabledForTests = false
	defer func() {
		liveAnnouncementWatcherDisabledForTests = prevWatcherDisabled
	}()
	prevEnv, hadEnv := os.LookupEnv("HAONEWS_DISABLE_LIVE_ANNOUNCEMENT_WATCHER")
	if err := os.Unsetenv("HAONEWS_DISABLE_LIVE_ANNOUNCEMENT_WATCHER"); err != nil {
		t.Fatalf("Unsetenv error = %v", err)
	}
	defer func() {
		if hadEnv {
			_ = os.Setenv("HAONEWS_DISABLE_LIVE_ANNOUNCEMENT_WATCHER", prevEnv)
			return
		}
		_ = os.Unsetenv("HAONEWS_DISABLE_LIVE_ANNOUNCEMENT_WATCHER")
	}()

	root := t.TempDir()
	netPath := filepath.Join(root, "haonews_net.inf")
	if err := os.WriteFile(netPath, []byte("network_mode=lan\n"), 0o644); err != nil {
		t.Fatalf("WriteFile netPath error = %v", err)
	}
	if path, ok := liveAnnouncementWatcherNetPath(apphost.Config{}); ok || path != "" {
		t.Fatalf("expected default config to keep watcher disabled in managed mode, got %q %v", path, ok)
	}
	if path, ok := liveAnnouncementWatcherNetPath(apphost.Config{SyncMode: "managed", NetPath: netPath}); ok || path != "" {
		t.Fatalf("expected managed sync without live net to disable watcher, got %q %v", path, ok)
	}
	if path, ok := liveAnnouncementWatcherNetPath(apphost.Config{SyncMode: "off", NetPath: netPath}); !ok || path != netPath {
		t.Fatalf("expected non-sync standalone mode to use main net path, got %q %v", path, ok)
	}
	liveNetPath := filepath.Join(root, "hao_news_live_net.inf")
	if err := os.WriteFile(liveNetPath, []byte("network_mode=lan\n"), 0o644); err != nil {
		t.Fatalf("WriteFile liveNetPath error = %v", err)
	}
	if path, ok := liveAnnouncementWatcherNetPath(apphost.Config{SyncMode: "managed", NetPath: netPath}); !ok || path != liveNetPath {
		t.Fatalf("expected managed sync to use live net path, got %q %v", path, ok)
	}
	if path, ok := liveAnnouncementWatcherNetPath(apphost.Config{SyncMode: "external", NetPath: netPath}); !ok || path != liveNetPath {
		t.Fatalf("expected external sync to use live net path, got %q %v", path, ok)
	}
}

func TestPluginBuildServesLiveAPI(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	store, err := live.OpenLocalStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	if err := store.SaveRoom(live.RoomInfo{
		RoomID:    "room-2",
		Title:     "Room API",
		Creator:   "agent://pc75/openclaw01",
		CreatedAt: "2026-03-19T00:00:00Z",
		Channel:   "hao.news/live",
	}); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/live/rooms", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "room-2") {
		t.Fatalf("expected room id in API body, got %q", rec.Body.String())
	}
}

func TestNewHandlerServesLiveBootstrap(t *testing.T) {
	t.Parallel()

	handler := newHandler(nil, nil, os.DirFS(t.TempDir()), func() *live.BootstrapStatus {
		return &live.BootstrapStatus{
			NetworkID:  "net-live",
			PeerID:     "12D3KooWLiveBootstrap",
			ListenPort: 51585,
			DialAddrs:  []string{"/ip4/192.168.102.74/tcp/51584/p2p/12D3KooWLiveBootstrap"},
		}
	}, "", "")
	req := httptest.NewRequest(http.MethodGet, "/api/live/bootstrap", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "\"network_id\": \"net-live\"") || !strings.Contains(body, "\"peer_id\": \"12D3KooWLiveBootstrap\"") {
		t.Fatalf("unexpected bootstrap body = %q", body)
	}
}

func TestNewHandlerServesLiveStatusAPI(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := live.OpenLocalStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	room := live.RoomInfo{
		RoomID:    "public-live-time",
		Title:     "Live-Time",
		Creator:   "agent://pc75/now-time",
		CreatedAt: "2026-04-04T00:00:00Z",
		Channel:   "hao.news/live/public",
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	if err := store.AppendEvent(room.RoomID, live.LiveMessage{
		Protocol:     live.ProtocolVersion,
		Type:         live.TypeMessage,
		RoomID:       room.RoomID,
		Sender:       room.Creator,
		SenderPubKey: strings.Repeat("a", 64),
		Seq:          1,
		Timestamp:    "2026-04-04T07:37:14Z",
		Payload:      live.LivePayload{Content: "当前时间：2026-04-04 15:37:06 CST"},
		Signature:    strings.Repeat("1", 128),
	}); err != nil {
		t.Fatalf("AppendEvent error = %v", err)
	}
	senderNetPath := filepath.Join(root, "config", "hao_news_live_sender_net.inf")
	if err := os.MkdirAll(filepath.Dir(senderNetPath), 0o755); err != nil {
		t.Fatalf("MkdirAll sender config dir error = %v", err)
	}
	if err := os.WriteFile(senderNetPath, []byte("network_mode=lan\nlibp2p_listen=/ip4/127.0.0.1/tcp/51585\nredis_enabled=true\n"), 0o644); err != nil {
		t.Fatalf("WriteFile sender net error = %v", err)
	}
	senderIdentityPath := filepath.Join(root, "config", "identities", "pc75-now-time.json")
	if err := os.MkdirAll(filepath.Dir(senderIdentityPath), 0o755); err != nil {
		t.Fatalf("MkdirAll sender identity dir error = %v", err)
	}
	identity, err := core.NewAgentIdentity("agent://pc75/now-time", "agent://pc75/now-time", time.Date(2026, 4, 4, 15, 37, 6, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewAgentIdentity error = %v", err)
	}
	if err := core.SaveAgentIdentity(senderIdentityPath, identity); err != nil {
		t.Fatalf("SaveAgentIdentity error = %v", err)
	}
	handler := newHandler(nil, store, os.DirFS(root), func() *live.BootstrapStatus {
		return &live.BootstrapStatus{
			NetworkID:  "net-live",
			PeerID:     "12D3KooWLiveBootstrap",
			ListenPort: 51584,
			DialAddrs:  []string{"/ip4/192.168.102.74/tcp/51584/p2p/12D3KooWLiveBootstrap"},
		}
	}, senderNetPath, senderIdentityPath)
	req := httptest.NewRequest(http.MethodGet, "/api/live/status/public-live-time", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal error = %v", err)
	}
	if body["watcher_peer_id"] != "12D3KooWLiveBootstrap" {
		t.Fatalf("watcher_peer_id = %v", body["watcher_peer_id"])
	}
	if got := int(body["watcher_listen_port"].(float64)); got != 51584 {
		t.Fatalf("watcher_listen_port = %d", got)
	}
	if body["sender_peer_id"] != "agent://pc75/now-time" {
		t.Fatalf("sender_peer_id = %v", body["sender_peer_id"])
	}
	if got := int(body["sender_listen_port"].(float64)); got != 51585 {
		t.Fatalf("sender_listen_port = %d", got)
	}
	if body["latest_non_heartbeat_at"] != "2026-04-04 15:37:14 CST" {
		t.Fatalf("latest_non_heartbeat_at = %v", body["latest_non_heartbeat_at"])
	}
	senderIdentity, ok := body["sender_identity"].(map[string]any)
	if !ok {
		t.Fatalf("sender_identity = %#v", body["sender_identity"])
	}
	if senderIdentity["agent_id"] != "agent://pc75/now-time" {
		t.Fatalf("sender_identity.agent_id = %v", senderIdentity["agent_id"])
	}
	archiveStats, ok := body["archive_stats"].(map[string]any)
	if !ok {
		t.Fatalf("archive_stats = %#v", body["archive_stats"])
	}
	if got := int(archiveStats["archive_count"].(float64)); got != 0 {
		t.Fatalf("archive_count = %d", got)
	}
	if body["latest_cache_refresh_at"] == nil || body["latest_cache_refresh_at"] == "" {
		t.Fatal("expected latest_cache_refresh_at")
	}
}

func TestPluginBuildServesLiveStatus(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	store, err := live.OpenLocalStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	room := live.RoomInfo{
		RoomID:    "public-live-time",
		Title:     "Live-Time",
		Creator:   "agent://pc75/now-time",
		CreatedAt: "2026-04-04T00:00:00Z",
		Channel:   "hao.news/live/public",
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	if err := store.AppendEvent(room.RoomID, live.LiveMessage{
		Protocol:     live.ProtocolVersion,
		Type:         live.TypeMessage,
		RoomID:       room.RoomID,
		Sender:       room.Creator,
		SenderPubKey: strings.Repeat("a", 64),
		Seq:          1,
		Timestamp:    "2026-04-04T07:37:14Z",
		Payload:      live.LivePayload{Content: "当前时间：2026-04-04 15:37:06 CST"},
		Signature:    strings.Repeat("1", 128),
	}); err != nil {
		t.Fatalf("AppendEvent error = %v", err)
	}
	senderNetPath := filepath.Join(root, "config", "hao_news_live_sender_net.inf")
	if err := os.MkdirAll(filepath.Dir(senderNetPath), 0o755); err != nil {
		t.Fatalf("MkdirAll sender config dir error = %v", err)
	}
	if err := os.WriteFile(senderNetPath, []byte("network_mode=lan\nlibp2p_listen=/ip4/127.0.0.1/tcp/51585\nredis_enabled=true\n"), 0o644); err != nil {
		t.Fatalf("WriteFile sender net error = %v", err)
	}
	senderIdentityPath := filepath.Join(root, "config", "identities", "pc75-now-time.json")
	if err := os.MkdirAll(filepath.Dir(senderIdentityPath), 0o755); err != nil {
		t.Fatalf("MkdirAll sender identity dir error = %v", err)
	}
	identity, err := core.NewAgentIdentity("agent://pc75/now-time", "agent://pc75/now-time", time.Date(2026, 4, 4, 15, 37, 6, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewAgentIdentity error = %v", err)
	}
	if err := core.SaveAgentIdentity(senderIdentityPath, identity); err != nil {
		t.Fatalf("SaveAgentIdentity error = %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/live/status/public-live-time", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal error = %v", err)
	}
	if got := int(body["visible_event_count"].(float64)); got != 1 {
		t.Fatalf("visible_event_count = %d", got)
	}
	if body["latest_non_heartbeat_at"] != "2026-04-04 15:37:14 CST" {
		t.Fatalf("latest_non_heartbeat_at = %v", body["latest_non_heartbeat_at"])
	}
	if body["sender_peer_id"] != "agent://pc75/now-time" {
		t.Fatalf("sender_peer_id = %v", body["sender_peer_id"])
	}
	if got := int(body["sender_listen_port"].(float64)); got != 51585 {
		t.Fatalf("sender_listen_port = %d", got)
	}
	if got := int(body["total_event_count"].(float64)); got != 1 {
		t.Fatalf("total_event_count = %d", got)
	}
	if body["latest_cache_refresh_at"] == nil || body["latest_cache_refresh_at"] == "" {
		t.Fatal("expected latest_cache_refresh_at")
	}
}

func TestPluginBuildServesLiveStatusPage(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	store, err := live.OpenLocalStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	room := live.RoomInfo{
		RoomID:    "public-live-time",
		Title:     "Live-Time",
		Creator:   "agent://pc75/now-time",
		CreatedAt: "2026-04-04T00:00:00Z",
		Channel:   "hao.news/live/public",
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	if err := store.AppendEvent(room.RoomID, live.LiveMessage{
		Protocol:     live.ProtocolVersion,
		Type:         live.TypeMessage,
		RoomID:       room.RoomID,
		Sender:       room.Creator,
		SenderPubKey: strings.Repeat("a", 64),
		Seq:          1,
		Timestamp:    "2026-04-04T07:37:14Z",
		Payload:      live.LivePayload{Content: "当前时间：2026-04-04 15:37:06 CST"},
		Signature:    strings.Repeat("1", 128),
	}); err != nil {
		t.Fatalf("AppendEvent error = %v", err)
	}
	senderNetPath := filepath.Join(root, "config", "hao_news_live_sender_net.inf")
	if err := os.MkdirAll(filepath.Dir(senderNetPath), 0o755); err != nil {
		t.Fatalf("MkdirAll sender config dir error = %v", err)
	}
	if err := os.WriteFile(senderNetPath, []byte("network_mode=lan\nlibp2p_listen=/ip4/127.0.0.1/tcp/51585\nredis_enabled=true\n"), 0o644); err != nil {
		t.Fatalf("WriteFile sender net error = %v", err)
	}
	senderIdentityPath := filepath.Join(root, "config", "identities", "pc75-now-time.json")
	if err := os.MkdirAll(filepath.Dir(senderIdentityPath), 0o755); err != nil {
		t.Fatalf("MkdirAll sender identity dir error = %v", err)
	}
	identity, err := core.NewAgentIdentity("agent://pc75/now-time", "agent://pc75/now-time", time.Date(2026, 4, 4, 15, 37, 6, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewAgentIdentity error = %v", err)
	}
	if err := core.SaveAgentIdentity(senderIdentityPath, identity); err != nil {
		t.Fatalf("SaveAgentIdentity error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/live/status/public-live-time", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Live 状态") ||
		!strings.Contains(body, "传输状态") ||
		!strings.Contains(body, "Sender") ||
		!strings.Contains(body, "latest cache refresh") ||
		!strings.Contains(body, "agent://pc75/now-time") ||
		!strings.Contains(body, "listen port：51585") ||
		!strings.Contains(body, "latest non-heartbeat") {
		t.Fatalf("unexpected status page body = %q", body)
	}
}

func TestPluginBuildLimitsVisibleLiveEventsButCanShowAll(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	store, err := live.OpenLocalStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	room := live.RoomInfo{
		RoomID:    "room-window",
		Title:     "Window Test",
		Creator:   "agent://pc75/openclaw01",
		CreatedAt: "2026-03-19T00:00:00Z",
		Channel:   "hao.news/live",
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	for idx := 0; idx < live.LiveRoomDisplayNonHeartbeatEvents+5; idx++ {
		if err := store.AppendEvent(room.RoomID, live.LiveMessage{
			Protocol:     live.ProtocolVersion,
			Type:         live.TypeMessage,
			RoomID:       room.RoomID,
			Sender:       room.Creator,
			SenderPubKey: strings.Repeat("a", 64),
			Seq:          uint64(idx + 1),
			Timestamp:    fmt.Sprintf("2026-03-19T00:%02d:00Z", idx%60),
			Payload:      live.LivePayload{Content: fmt.Sprintf("window-event-%03d", idx)},
			Signature:    fmt.Sprintf("%0128d", idx+1),
		}); err != nil {
			t.Fatalf("AppendEvent %d error = %v", idx, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/live/room-window", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "查看全部") {
		t.Fatalf("expected visible window hint in body, got %q", body)
	}
	if strings.Contains(body, "window-event-000") {
		t.Fatalf("expected oldest event hidden by default, got %q", body)
	}
	expectedNewest := fmt.Sprintf("window-event-%03d", live.LiveRoomDisplayNonHeartbeatEvents+4)
	if !strings.Contains(body, expectedNewest) {
		t.Fatalf("expected newest event visible, got %q", body)
	}

	allReq := httptest.NewRequest(http.MethodGet, "/api/live/rooms/room-window?show_all=1", nil)
	allRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(allRec, allReq)
	if allRec.Code != http.StatusOK {
		t.Fatalf("show all api status = %d, body = %s", allRec.Code, allRec.Body.String())
	}
	expectedTotal := live.LiveRoomDisplayNonHeartbeatEvents + 5
	if !strings.Contains(allRec.Body.String(), "\"show_all\": true") || !strings.Contains(allRec.Body.String(), fmt.Sprintf("\"total_event_count\": %d", expectedTotal)) || !strings.Contains(allRec.Body.String(), "window-event-000") {
		t.Fatalf("expected full event stream in show_all api, got %q", allRec.Body.String())
	}
}

func TestPluginBuildServesPublicLiveRoomDespiteBlockedOrigin(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	store, err := live.OpenLocalStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	blockedKey := strings.Repeat("b", 64)
	rulesBody := []byte("{\n  \"live_blocked_origin_public_keys\": [\"" + blockedKey + "\"]\n}\n")
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatalf("MkdirAll config error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "subscriptions.json"), rulesBody, 0o644); err != nil {
		t.Fatalf("WriteFile subscriptions error = %v", err)
	}
	room := live.RoomInfo{
		RoomID:          "public-new-agents",
		Title:           "Public New Agents",
		Creator:         "agent://pc75/public",
		CreatorPubKey:   blockedKey,
		ParentPublicKey: strings.Repeat("c", 64),
		CreatedAt:       "2026-03-30T00:00:00Z",
		Channel:         "hao.news/live",
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	if err := store.AppendEvent(room.RoomID, live.LiveMessage{
		Protocol:     live.ProtocolVersion,
		Type:         live.TypeMessage,
		RoomID:       room.RoomID,
		Sender:       room.Creator,
		SenderPubKey: blockedKey,
		Seq:          1,
		Timestamp:    "2026-03-30T00:00:10Z",
		Payload: live.LivePayload{
			Content: "public hello",
			Metadata: map[string]any{
				"origin_public_key": blockedKey,
				"parent_public_key": strings.Repeat("c", 64),
			},
		},
	}); err != nil {
		t.Fatalf("AppendEvent error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/live/public/new-agents", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Public New Agents") || !strings.Contains(body, "public hello") {
		t.Fatalf("expected public room body, got %q", body)
	}
}

func TestPluginBuildServesDefaultPublicLiveRoomWithoutStoredRoom(t *testing.T) {
	t.Parallel()

	site, _ := buildLiveSite(t)
	req := httptest.NewRequest(http.MethodGet, "/live/public", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Live Public") ||
		!strings.Contains(body, "agent://system/live-public") ||
		!strings.Contains(body, "/live/public/new-agents") ||
		!strings.Contains(body, "/live/public/help") ||
		!strings.Contains(body, "/live/public/world") {
		t.Fatalf("expected default public live room body, got %q", body)
	}
}

func TestPluginBuildServesPublicNewAgentsTemplateWithoutStoredRoom(t *testing.T) {
	t.Parallel()

	site, _ := buildLiveSite(t)
	req := httptest.NewRequest(http.MethodGet, "/live/public/new-agents", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "报到模板") || !strings.Contains(body, "Parent public key") || !strings.Contains(body, "申请加入") || !strings.Contains(body, "复制报到消息") {
		t.Fatalf("expected public new agents template, got %q", body)
	}
}

func TestPluginBuildServesPublicLiveRoomAPIDespiteBlockedOrigin(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	store, err := live.OpenLocalStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	blockedKey := strings.Repeat("d", 64)
	rulesBody := []byte("{\n  \"live_blocked_origin_public_keys\": [\"" + blockedKey + "\"]\n}\n")
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatalf("MkdirAll config error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "subscriptions.json"), rulesBody, 0o644); err != nil {
		t.Fatalf("WriteFile subscriptions error = %v", err)
	}
	room := live.RoomInfo{
		RoomID:          "public",
		Title:           "Live Public",
		Creator:         "agent://pc75/public",
		CreatorPubKey:   blockedKey,
		ParentPublicKey: strings.Repeat("e", 64),
		CreatedAt:       "2026-03-30T00:00:00Z",
		Channel:         "hao.news/live",
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/live/public", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "\"room_id\": \"public\"") || !strings.Contains(body, "\"room_visibility\": \"public\"") {
		t.Fatalf("expected public room API body, got %q", body)
	}
}

func TestPluginBuildServesDefaultPublicLiveRoomAPIWithoutStoredRoom(t *testing.T) {
	t.Parallel()

	site, _ := buildLiveSite(t)
	req := httptest.NewRequest(http.MethodGet, "/api/live/public/new-agents", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "\"room_id\": \"public-new-agents\"") ||
		!strings.Contains(body, "\"room_visibility\": \"public\"") ||
		!strings.Contains(body, "Live Public / New Agents") {
		t.Fatalf("expected default public room API body, got %q", body)
	}
}

func TestPluginBuildServesLivePublicModerationAPI(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	rulesBody := []byte("{\n  \"live_public_muted_origin_public_keys\": [\"" + strings.Repeat("1", 64) + "\"],\n  \"live_public_muted_parent_public_keys\": [\"" + strings.Repeat("2", 64) + "\"],\n  \"live_public_rate_limit_messages\": 3,\n  \"live_public_rate_limit_window_seconds\": 90\n}\n")
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatalf("MkdirAll config error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "subscriptions.json"), rulesBody, 0o644); err != nil {
		t.Fatalf("WriteFile subscriptions error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/live/public/moderation", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "\"scope\": \"live-public-moderation\"") ||
		!strings.Contains(body, strings.Repeat("1", 64)) ||
		!strings.Contains(body, strings.Repeat("2", 64)) ||
		!strings.Contains(body, "\"public_rate_limit_messages\": 3") ||
		!strings.Contains(body, "\"public_rate_limit_window_seconds\": 90") {
		t.Fatalf("expected moderation API body, got %q", body)
	}
}

func TestPluginBuildServesLivePublicModerationPage(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	rulesBody := []byte("{\n  \"live_public_muted_origin_public_keys\": [\"" + strings.Repeat("3", 64) + "\"],\n  \"live_public_muted_parent_public_keys\": [\"" + strings.Repeat("4", 64) + "\"],\n  \"live_public_rate_limit_messages\": 2,\n  \"live_public_rate_limit_window_seconds\": 60\n}\n")
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatalf("MkdirAll config error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "subscriptions.json"), rulesBody, 0o644); err != nil {
		t.Fatalf("WriteFile subscriptions error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/live/public/moderation", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "本地公共区防护") ||
		!strings.Contains(body, "静音子公钥") ||
		!strings.Contains(body, "静音父公钥") ||
		!strings.Contains(body, "Live Public 管理") {
		t.Fatalf("expected moderation page body, got %q", body)
	}
}

func TestPluginBuildUpdatesLivePublicModerationRules(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	form := url.Values{
		"muted_origin_public_keys":         {strings.Repeat("a", 64) + "\nnot-a-key"},
		"muted_parent_public_keys":         {strings.Repeat("b", 64)},
		"public_rate_limit_messages":       {"4"},
		"public_rate_limit_window_seconds": {"120"},
	}
	req := httptest.NewRequest(http.MethodPost, "/live/public/moderation", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if !strings.Contains(location, "/live/public/moderation?saved=1") {
		t.Fatalf("unexpected redirect = %q", location)
	}
	data, err := os.ReadFile(filepath.Join(root, "config", "subscriptions.json"))
	if err != nil {
		t.Fatalf("ReadFile subscriptions error = %v", err)
	}
	body := string(data)
	if !strings.Contains(body, strings.Repeat("a", 64)) ||
		!strings.Contains(body, strings.Repeat("b", 64)) ||
		!strings.Contains(body, "\"live_public_rate_limit_messages\": 4") ||
		!strings.Contains(body, "\"live_public_rate_limit_window_seconds\": 120") {
		t.Fatalf("expected updated subscriptions, got %q", body)
	}
	if strings.Contains(body, "not-a-key") {
		t.Fatalf("expected invalid key removed, got %q", body)
	}
}

func TestPluginBuildRejectsSpoofedForwardedForOnLivePublicModeration(t *testing.T) {
	t.Parallel()

	site, _ := buildLiveSite(t)
	form := url.Values{
		"muted_origin_public_keys": {strings.Repeat("a", 64)},
	}
	req := httptest.NewRequest(http.MethodPost, "/live/public/moderation", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	req.RemoteAddr = "198.51.100.20:23456"
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); !strings.Contains(location, "/live/public/moderation?error=untrusted") {
		t.Fatalf("unexpected redirect = %q", location)
	}
}

func TestPluginBuildPublicLiveRoomAppliesMutedOriginRules(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	store, err := live.OpenLocalStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	mutedKey := strings.Repeat("a", 64)
	rulesBody := []byte("{\n  \"live_public_muted_origin_public_keys\": [\"" + mutedKey + "\"]\n}\n")
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatalf("MkdirAll config error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "subscriptions.json"), rulesBody, 0o644); err != nil {
		t.Fatalf("WriteFile subscriptions error = %v", err)
	}
	room := defaultPublicLiveRoom("public-new-agents")
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	if err := store.AppendEvent(room.RoomID, live.LiveMessage{
		Protocol:     live.ProtocolVersion,
		Type:         live.TypeMessage,
		RoomID:       room.RoomID,
		Sender:       "agent://pc75/muted",
		SenderPubKey: mutedKey,
		Seq:          1,
		Timestamp:    "2026-03-30T00:00:10Z",
		Payload: live.LivePayload{
			Content: "muted hello",
			Metadata: map[string]any{
				"origin_public_key": mutedKey,
				"parent_public_key": strings.Repeat("b", 64),
			},
		},
	}); err != nil {
		t.Fatalf("AppendEvent error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/live/public/new-agents", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "muted hello") || !strings.Contains(body, "\"public_muted_events\": 1") {
		t.Fatalf("expected muted public event hidden, got %q", body)
	}
}

func TestPluginBuildPublicLiveRoomAppliesRateLimit(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	store, err := live.OpenLocalStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	key := strings.Repeat("c", 64)
	rulesBody := []byte("{\n  \"live_public_rate_limit_messages\": 2,\n  \"live_public_rate_limit_window_seconds\": 60\n}\n")
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatalf("MkdirAll config error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "subscriptions.json"), rulesBody, 0o644); err != nil {
		t.Fatalf("WriteFile subscriptions error = %v", err)
	}
	room := defaultPublicLiveRoom("public")
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	for idx, content := range []string{"one", "two", "three"} {
		if err := store.AppendEvent(room.RoomID, live.LiveMessage{
			Protocol:     live.ProtocolVersion,
			Type:         live.TypeMessage,
			RoomID:       room.RoomID,
			Sender:       "agent://pc75/spam",
			SenderPubKey: key,
			Seq:          uint64(idx + 1),
			Timestamp:    fmt.Sprintf("2026-03-30T00:00:%02dZ", 10+idx),
			Payload: live.LivePayload{
				Content: content,
				Metadata: map[string]any{
					"origin_public_key": key,
					"parent_public_key": strings.Repeat("d", 64),
				},
			},
		}); err != nil {
			t.Fatalf("AppendEvent error = %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/live/public", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "three") || !strings.Contains(body, "\"public_rate_limited_events\": 1") {
		t.Fatalf("expected rate-limited third public event hidden, got %q", body)
	}
}

func TestPluginBuildServesLiveRoomDetails(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	store, err := live.OpenLocalStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	room := live.RoomInfo{
		RoomID:    "room-3",
		Title:     "Room Detail",
		Creator:   "agent://pc75/openclaw01",
		CreatedAt: "2026-03-19T00:00:00Z",
		Channel:   "hao.news/live",
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	if err := store.AppendEvent(room.RoomID, live.LiveMessage{
		Protocol:     live.ProtocolVersion,
		Type:         live.TypeHeartbeat,
		RoomID:       room.RoomID,
		Sender:       room.Creator,
		SenderPubKey: strings.Repeat("a", 64),
		Seq:          0,
		Timestamp:    "2026-03-19T00:00:05Z",
	}); err != nil {
		t.Fatalf("AppendEvent heartbeat error = %v", err)
	}
	if err := store.AppendEvent(room.RoomID, live.LiveMessage{
		Protocol:     live.ProtocolVersion,
		Type:         live.TypeTaskUpdate,
		RoomID:       room.RoomID,
		Sender:       room.Creator,
		SenderPubKey: strings.Repeat("a", 64),
		Seq:          1,
		Timestamp:    "2026-03-19T00:00:10Z",
		Payload: live.LivePayload{
			Metadata: map[string]any{
				"task_id":     "task-1",
				"status":      "doing",
				"description": "同步直播任务状态",
				"assigned_to": "agent://pc76/openclaw01",
				"progress":    60,
			},
		},
	}); err != nil {
		t.Fatalf("AppendEvent error = %v", err)
	}
	if err := store.AppendEvent(room.RoomID, live.LiveMessage{
		Protocol:     live.ProtocolVersion,
		Type:         live.TypeArchiveNotice,
		RoomID:       room.RoomID,
		Sender:       room.Creator,
		SenderPubKey: strings.Repeat("a", 64),
		Seq:          2,
		Timestamp:    "2026-03-19T00:00:20Z",
		Payload: live.LivePayload{
			Content: "/posts/abc123",
			Metadata: map[string]any{
				"archive.infohash":    "abc123",
				"archive.viewer_url":  "/posts/abc123",
				"archive.archived_at": "2026-03-19T00:05:00Z",
			},
		},
	}); err != nil {
		t.Fatalf("AppendEvent archive notice error = %v", err)
	}
	if err := store.SaveArchiveResult(room.RoomID, live.ArchiveResult{
		RoomID:     room.RoomID,
		Channel:    "hao.news/live",
		Events:     1,
		ArchivedAt: "2026-03-19T00:05:00Z",
		ViewerURL:  "/posts/abc123",
		Published:  core.PublishResult{InfoHash: "abc123"},
	}); err != nil {
		t.Fatalf("SaveArchiveResult error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/live/"+room.RoomID, nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "打开归档文章") {
		t.Fatalf("expected archive link in body, got %q", body)
	}
	if !strings.Contains(body, "任务 ID") || !strings.Contains(body, "task-1") {
		t.Fatalf("expected task summary in body, got %q", body)
	}
	if !strings.Contains(body, "任务概览") || !strings.Contains(body, "更新次数") {
		t.Fatalf("expected task aggregate in body, got %q", body)
	}
	if !strings.Contains(body, "任务分组") || !strings.Contains(body, "按状态") || !strings.Contains(body, "按负责人") {
		t.Fatalf("expected task group panels in body, got %q", body)
	}
	if strings.Contains(body, "<span>heartbeat</span>") {
		t.Fatalf("expected heartbeats hidden by default, got %q", body)
	}
	if strings.Contains(body, "<span>archive_notice</span>") || strings.Contains(body, "archive.archived_at") {
		t.Fatalf("expected archive notices hidden by default, got %q", body)
	}
	if !strings.Contains(body, "显示心跳") || !strings.Contains(body, "关闭自动更新") {
		t.Fatalf("expected spectator controls in body, got %q", body)
	}
}

func TestPluginBuildServesLiveRoomHistoryAfterPrune(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	store, err := live.OpenLocalStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	room := live.RoomInfo{
		RoomID:    "room-history",
		Title:     "Room History",
		Creator:   "agent://pc75/openclaw01",
		CreatedAt: "2026-03-19T00:00:00Z",
		Channel:   "hao.news/live",
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	for idx := 0; idx < 105; idx++ {
		timestamp := fmt.Sprintf("2026-03-19T00:01:%02dZ", idx%60)
		if err := store.AppendEvent(room.RoomID, live.LiveMessage{
			Protocol:     live.ProtocolVersion,
			Type:         live.TypeMessage,
			RoomID:       room.RoomID,
			Sender:       room.Creator,
			SenderPubKey: strings.Repeat("a", 64),
			Seq:          uint64(idx + 1),
			Timestamp:    timestamp,
			Payload:      live.LivePayload{Content: fmt.Sprintf("history-%03d", idx)},
			Signature:    fmt.Sprintf("%064x", idx+1),
		}); err != nil {
			t.Fatalf("AppendEvent error = %v", err)
		}
	}
	historyArchives, err := store.ListHistoryArchives(room.RoomID)
	if err != nil {
		t.Fatalf("ListHistoryArchives error = %v", err)
	}
	if len(historyArchives) != 0 {
		t.Fatalf("len(historyArchives) = %d, want 0 without daily archive", len(historyArchives))
	}

	req := httptest.NewRequest(http.MethodGet, "/live/"+room.RoomID, nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "本地历史归档") || !strings.Contains(rec.Body.String(), "当前还没有本地历史归档") {
		t.Fatalf("expected local history section, got %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/live/history/"+room.RoomID, nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("history status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "当前还没有 Live 归档") {
		t.Fatalf("expected history page body, got %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/live/history/"+room.RoomID, nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("history API status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "\"api_scope\": \"live-room-history\"") {
		t.Fatalf("expected history API body, got %q", rec.Body.String())
	}
}

func TestPluginBuildServesLiveRoomAPIIncludesTaskSummaries(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	store, err := live.OpenLocalStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	room := live.RoomInfo{
		RoomID:    "room-4",
		Title:     "Room API Detail",
		Creator:   "agent://pc75/openclaw01",
		CreatedAt: "2026-03-19T00:00:00Z",
		Channel:   "hao.news/live",
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	for idx, status := range []string{"todo", "doing"} {
		if err := store.AppendEvent(room.RoomID, live.LiveMessage{
			Protocol:     live.ProtocolVersion,
			Type:         live.TypeTaskUpdate,
			RoomID:       room.RoomID,
			Sender:       room.Creator,
			SenderPubKey: strings.Repeat("a", 64),
			Seq:          uint64(idx + 1),
			Timestamp:    fmt.Sprintf("2026-03-19T00:00:1%dZ", idx),
			Payload: live.LivePayload{
				Metadata: map[string]any{
					"task_id":  "task-api",
					"status":   status,
					"progress": 25 + idx*25,
				},
			},
		}); err != nil {
			t.Fatalf("AppendEvent error = %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/live/rooms/"+room.RoomID, nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "\"task_summaries\"") || !strings.Contains(body, "\"task-api\"") {
		t.Fatalf("expected task summaries in API body, got %q", body)
	}
	if !strings.Contains(body, "\"update_count\": 2") {
		t.Fatalf("expected update_count in API body, got %q", body)
	}
	if !strings.Contains(body, "\"task_by_status\"") || !strings.Contains(body, "\"task_by_assignee\"") {
		t.Fatalf("expected grouped task fields in API body, got %q", body)
	}
}

func TestPluginBuildCreatesManualArchiveAndServesArchiveLiveRoutes(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	store, err := live.OpenLocalStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	room := live.RoomInfo{
		RoomID:    "room-archive-ui",
		Title:     "Archive UI",
		Creator:   "agent://pc75/openclaw01",
		CreatedAt: "2026-04-02T00:00:00Z",
		Channel:   "hao.news/live",
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	if err := store.AppendEvent(room.RoomID, live.LiveMessage{
		Protocol:     live.ProtocolVersion,
		Type:         live.TypeMessage,
		RoomID:       room.RoomID,
		Sender:       room.Creator,
		SenderPubKey: strings.Repeat("a", 64),
		Seq:          1,
		Timestamp:    "2026-04-02T01:00:00Z",
		Payload:      live.LivePayload{Content: "archive me"},
		Signature:    strings.Repeat("4", 128),
	}); err != nil {
		t.Fatalf("AppendEvent error = %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/live/archive/"+room.RoomID, nil)
	createReq.RemoteAddr = "127.0.0.1:12345"
	createRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("archive create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}
	if !strings.Contains(createRec.Body.String(), "\"scope\": \"live-room-archive-create\"") || !strings.Contains(createRec.Body.String(), "\"kind\": \"manual\"") {
		t.Fatalf("expected archive create payload, got %q", createRec.Body.String())
	}

	indexReq := httptest.NewRequest(http.MethodGet, "/archive/live", nil)
	indexRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(indexRec, indexReq)
	if indexRec.Code != http.StatusOK {
		t.Fatalf("archive live index status = %d, body = %s", indexRec.Code, indexRec.Body.String())
	}
	if !strings.Contains(indexRec.Body.String(), "Live 归档") || !strings.Contains(indexRec.Body.String(), room.Title) {
		t.Fatalf("expected archive live index body, got %q", indexRec.Body.String())
	}
	if !strings.Contains(indexRec.Body.String(), "正文 1 条") || !strings.Contains(indexRec.Body.String(), "手动") {
		t.Fatalf("expected archive stats in index body, got %q", indexRec.Body.String())
	}

	apiIndexReq := httptest.NewRequest(http.MethodGet, "/api/archive/live", nil)
	apiIndexRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(apiIndexRec, apiIndexReq)
	if apiIndexRec.Code != http.StatusOK {
		t.Fatalf("archive live API status = %d, body = %s", apiIndexRec.Code, apiIndexRec.Body.String())
	}
	apiBody := apiIndexRec.Body.String()
	if !strings.Contains(apiBody, "\"archive_stats\"") || !strings.Contains(apiBody, "\"latest_archive_kind\": \"manual\"") || !strings.Contains(apiBody, "\"latest_archive_message_count\": 1") {
		t.Fatalf("expected archive API stats, got %q", apiBody)
	}

	historyListReq := httptest.NewRequest(http.MethodGet, "/archive/live/"+room.RoomID, nil)
	historyListRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(historyListRec, historyListReq)
	if historyListRec.Code != http.StatusOK {
		t.Fatalf("archive live room status = %d, body = %s", historyListRec.Code, historyListRec.Body.String())
	}
	if !strings.Contains(historyListRec.Body.String(), "手动归档") {
		t.Fatalf("expected manual archive label in history page, got %q", historyListRec.Body.String())
	}

	compatReq := httptest.NewRequest(http.MethodGet, "/live/history/"+room.RoomID, nil)
	compatRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(compatRec, compatReq)
	if compatRec.Code != http.StatusOK {
		t.Fatalf("compat history status = %d, body = %s", compatRec.Code, compatRec.Body.String())
	}
	if !strings.Contains(compatRec.Body.String(), "archive me") {
		t.Fatalf("expected compat history body, got %q", compatRec.Body.String())
	}
}

func TestPluginBuildServesLiveRoomAPIIncludesHeartbeatsWhenRequested(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	store, err := live.OpenLocalStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	room := live.RoomInfo{
		RoomID:    "room-5",
		Title:     "Heartbeat API",
		Creator:   "agent://pc75/openclaw01",
		CreatedAt: "2026-03-19T00:00:00Z",
		Channel:   "hao.news/live",
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	if err := store.AppendEvent(room.RoomID, live.LiveMessage{
		Protocol:     live.ProtocolVersion,
		Type:         live.TypeHeartbeat,
		RoomID:       room.RoomID,
		Sender:       room.Creator,
		SenderPubKey: strings.Repeat("a", 64),
		Seq:          1,
		Timestamp:    "2026-03-19T00:00:10Z",
	}); err != nil {
		t.Fatalf("AppendEvent error = %v", err)
	}
	if err := store.AppendEvent(room.RoomID, live.LiveMessage{
		Protocol:     live.ProtocolVersion,
		Type:         live.TypeArchiveNotice,
		RoomID:       room.RoomID,
		Sender:       room.Creator,
		SenderPubKey: strings.Repeat("a", 64),
		Seq:          2,
		Timestamp:    "2026-03-19T00:00:20Z",
		Payload: live.LivePayload{
			Content: "/posts/archive-1",
			Metadata: map[string]any{
				"archive.infohash": "archive-1",
			},
		},
	}); err != nil {
		t.Fatalf("AppendEvent archive notice error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/live/rooms/"+room.RoomID+"?show_heartbeats=1", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "\"show_heartbeats\": true") {
		t.Fatalf("expected show_heartbeats flag in API body, got %q", body)
	}
	if !strings.Contains(body, "\"type\": \"heartbeat\"") {
		t.Fatalf("expected heartbeat event in API body, got %q", body)
	}
	if !strings.Contains(body, "\"type\": \"archive_notice\"") {
		t.Fatalf("expected archive_notice event in API body when controls shown, got %q", body)
	}
}

func TestPluginBuildFiltersBlockedLiveRoomByOriginPublicKey(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	store, err := live.OpenLocalStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	blockedKey := strings.Repeat("b", 64)
	rulesBody := []byte("{\n  \"live_blocked_origin_public_keys\": [\"" + blockedKey + "\"]\n}\n")
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatalf("MkdirAll config error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "subscriptions.json"), rulesBody, 0o644); err != nil {
		t.Fatalf("WriteFile subscriptions error = %v", err)
	}
	if err := store.SaveRoom(live.RoomInfo{
		RoomID:          "room-blocked",
		Title:           "Blocked Room",
		Creator:         "agent://pc75/blocked",
		CreatorPubKey:   blockedKey,
		ParentPublicKey: strings.Repeat("c", 64),
		CreatedAt:       "2026-03-19T00:00:00Z",
		Channel:         "hao.news/live",
	}); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Blocked Room") {
		t.Fatalf("expected blocked room hidden from live index, got %q", rec.Body.String())
	}
}

func TestPluginBuildFiltersBlockedLiveRoomEventsByOriginPublicKey(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	store, err := live.OpenLocalStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	blockedKey := strings.Repeat("b", 64)
	rulesBody := []byte("{\n  \"live_blocked_origin_public_keys\": [\"" + blockedKey + "\"]\n}\n")
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatalf("MkdirAll config error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "subscriptions.json"), rulesBody, 0o644); err != nil {
		t.Fatalf("WriteFile subscriptions error = %v", err)
	}
	room := live.RoomInfo{
		RoomID:          "room-events",
		Title:           "Room Events",
		Creator:         "agent://pc75/openclaw01",
		CreatorPubKey:   strings.Repeat("a", 64),
		ParentPublicKey: strings.Repeat("d", 64),
		CreatedAt:       "2026-03-19T00:00:00Z",
		Channel:         "hao.news/live",
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	if err := store.AppendEvent(room.RoomID, live.LiveMessage{
		Protocol:     live.ProtocolVersion,
		Type:         live.TypeMessage,
		RoomID:       room.RoomID,
		Sender:       "agent://pc75/allowed",
		SenderPubKey: strings.Repeat("a", 64),
		Seq:          1,
		Timestamp:    "2026-03-19T00:00:10Z",
		Payload: live.LivePayload{
			Content: "allowed event",
			Metadata: map[string]any{
				"origin_public_key": strings.Repeat("a", 64),
				"parent_public_key": strings.Repeat("d", 64),
			},
		},
	}); err != nil {
		t.Fatalf("AppendEvent allowed error = %v", err)
	}
	if err := store.AppendEvent(room.RoomID, live.LiveMessage{
		Protocol:     live.ProtocolVersion,
		Type:         live.TypeMessage,
		RoomID:       room.RoomID,
		Sender:       "agent://pc75/blocked",
		SenderPubKey: blockedKey,
		Seq:          2,
		Timestamp:    "2026-03-19T00:00:20Z",
		Payload: live.LivePayload{
			Content: "blocked event",
			Metadata: map[string]any{
				"origin_public_key": blockedKey,
				"parent_public_key": strings.Repeat("c", 64),
			},
		},
	}); err != nil {
		t.Fatalf("AppendEvent blocked error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/live/rooms/"+room.RoomID+"?show_heartbeats=1", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "allowed event") {
		t.Fatalf("expected allowed live event in API body, got %q", body)
	}
	if strings.Contains(body, "blocked event") {
		t.Fatalf("expected blocked live event hidden from API body, got %q", body)
	}
}

func TestPluginBuildServesLiveRoomAPIVisibility(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	store, err := live.OpenLocalStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	allowedParent := strings.Repeat("d", 64)
	rulesBody := []byte("{\n  \"live_allowed_parent_public_keys\": [\"" + allowedParent + "\"]\n}\n")
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatalf("MkdirAll config error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "subscriptions.json"), rulesBody, 0o644); err != nil {
		t.Fatalf("WriteFile subscriptions error = %v", err)
	}
	room := live.RoomInfo{
		RoomID:          "room-visibility",
		Title:           "Room Visibility",
		Creator:         "agent://pc75/openclaw01",
		CreatorPubKey:   strings.Repeat("a", 64),
		ParentPublicKey: allowedParent,
		CreatedAt:       "2026-03-19T00:00:00Z",
		Channel:         "hao.news/live",
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/live/rooms/"+room.RoomID, nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "\"room_visibility\": \"allowed_parent\"") {
		t.Fatalf("expected room_visibility in API body, got %q", body)
	}
}

func TestPluginBuildServesLivePendingIndexForBlockedRoom(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	store, err := live.OpenLocalStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	blockedKey := strings.Repeat("e", 64)
	rulesBody := []byte("{\n  \"live_blocked_origin_public_keys\": [\"" + blockedKey + "\"]\n}\n")
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatalf("MkdirAll config error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "subscriptions.json"), rulesBody, 0o644); err != nil {
		t.Fatalf("WriteFile subscriptions error = %v", err)
	}
	if err := store.SaveRoom(live.RoomInfo{
		RoomID:          "room-pending",
		Title:           "Pending Room",
		Creator:         "agent://pc75/pending",
		CreatorPubKey:   blockedKey,
		ParentPublicKey: strings.Repeat("f", 64),
		CreatedAt:       "2026-03-19T00:00:00Z",
		Channel:         "hao.news/live",
	}); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/live/pending", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Pending Room") || !strings.Contains(body, "blocked_origin") {
		t.Fatalf("expected blocked room in pending live index, got %q", body)
	}
}

func TestPluginBuildServesLivePendingRoomAPIForBlockedEvents(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	store, err := live.OpenLocalStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	blockedKey := strings.Repeat("b", 64)
	rulesBody := []byte("{\n  \"live_blocked_origin_public_keys\": [\"" + blockedKey + "\"]\n}\n")
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatalf("MkdirAll config error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "subscriptions.json"), rulesBody, 0o644); err != nil {
		t.Fatalf("WriteFile subscriptions error = %v", err)
	}
	room := live.RoomInfo{
		RoomID:          "room-pending-events",
		Title:           "Pending Events",
		Creator:         "agent://pc75/openclaw01",
		CreatorPubKey:   strings.Repeat("a", 64),
		ParentPublicKey: strings.Repeat("d", 64),
		CreatedAt:       "2026-03-19T00:00:00Z",
		Channel:         "hao.news/live",
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	if err := store.AppendEvent(room.RoomID, live.LiveMessage{
		Protocol:     live.ProtocolVersion,
		Type:         live.TypeMessage,
		RoomID:       room.RoomID,
		Sender:       "agent://pc75/allowed",
		SenderPubKey: strings.Repeat("a", 64),
		Seq:          1,
		Timestamp:    "2026-03-19T00:00:10Z",
		Payload: live.LivePayload{
			Content: "allowed event",
			Metadata: map[string]any{
				"origin_public_key": strings.Repeat("a", 64),
				"parent_public_key": strings.Repeat("d", 64),
			},
		},
	}); err != nil {
		t.Fatalf("AppendEvent allowed error = %v", err)
	}
	if err := store.AppendEvent(room.RoomID, live.LiveMessage{
		Protocol:     live.ProtocolVersion,
		Type:         live.TypeMessage,
		RoomID:       room.RoomID,
		Sender:       "agent://pc75/blocked",
		SenderPubKey: blockedKey,
		Seq:          2,
		Timestamp:    "2026-03-19T00:00:20Z",
		Payload: live.LivePayload{
			Content: "blocked event",
			Metadata: map[string]any{
				"origin_public_key": blockedKey,
				"parent_public_key": strings.Repeat("c", 64),
			},
		},
	}); err != nil {
		t.Fatalf("AppendEvent blocked error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/live/pending/"+room.RoomID, nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "\"scope\": \"live-pending-room\"") {
		t.Fatalf("expected pending room scope in API body, got %q", body)
	}
	if !strings.Contains(body, "blocked event") || strings.Contains(body, "allowed event") {
		t.Fatalf("expected only blocked live event in pending room API, got %q", body)
	}
}

func TestPluginBuildServesLiveRoomAPIIncludesPendingBlockedEvents(t *testing.T) {
	t.Parallel()

	site, root := buildLiveSite(t)
	store, err := live.OpenLocalStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	blockedKey := strings.Repeat("b", 64)
	rulesBody := []byte("{\n  \"live_blocked_origin_public_keys\": [\"" + blockedKey + "\"]\n}\n")
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatalf("MkdirAll config error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "subscriptions.json"), rulesBody, 0o644); err != nil {
		t.Fatalf("WriteFile subscriptions error = %v", err)
	}
	room := live.RoomInfo{
		RoomID:          "room-api-pending-count",
		Title:           "API Pending Count",
		Creator:         "agent://pc75/openclaw01",
		CreatorPubKey:   strings.Repeat("a", 64),
		ParentPublicKey: strings.Repeat("d", 64),
		CreatedAt:       "2026-03-19T00:00:00Z",
		Channel:         "hao.news/live",
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	if err := store.AppendEvent(room.RoomID, live.LiveMessage{
		Protocol:     live.ProtocolVersion,
		Type:         live.TypeMessage,
		RoomID:       room.RoomID,
		Sender:       "agent://pc75/blocked",
		SenderPubKey: blockedKey,
		Seq:          1,
		Timestamp:    "2026-03-19T00:00:20Z",
		Payload: live.LivePayload{
			Content: "blocked event",
			Metadata: map[string]any{
				"origin_public_key": blockedKey,
				"parent_public_key": strings.Repeat("c", 64),
			},
		},
	}); err != nil {
		t.Fatalf("AppendEvent blocked error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/live/rooms/"+room.RoomID, nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "\"pending_blocked_events\": 1") {
		t.Fatalf("expected pending_blocked_events in live room API body, got %q", body)
	}
}

func buildLiveSite(t *testing.T) (*apphost.Site, string) {
	t.Helper()
	liveAnnouncementWatcherDisabledForTests = true
	liveArchiveLoopDisabledForTests = true

	root := t.TempDir()
	cfg := apphost.Config{
		RuntimeRoot:      filepath.Join(root, "runtime"),
		StoreRoot:        filepath.Join(root, "store"),
		ArchiveRoot:      filepath.Join(root, "archive"),
		RulesPath:        filepath.Join(root, "config", "subscriptions.json"),
		WriterPolicyPath: filepath.Join(root, "config", "writer_policy.json"),
		NetPath:          filepath.Join(root, "config", "haonews_net.inf"),
		Project:          "hao.news",
		Version:          "test",
		Plugin:           "hao-news-live",
		Plugins:          []string{"hao-news-content", "hao-news-live"},
	}
	site, err := Plugin{}.Build(context.Background(), cfg, haonewstheme.Theme{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	t.Cleanup(func() {
		_ = site.Close(context.Background())
	})
	return site, root
}
