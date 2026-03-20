package haonewslive

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

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
	if !strings.Contains(body, "显示心跳") || !strings.Contains(body, "关闭自动更新") {
		t.Fatalf("expected spectator controls in body, got %q", body)
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
}

func buildLiveSite(t *testing.T) (*apphost.Site, string) {
	t.Helper()

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
	return site, root
}
