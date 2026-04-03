package live

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hao.news/internal/haonews"
)

func TestGenerateRoomID(t *testing.T) {
	roomID, err := GenerateRoomID("agent://pc75/openclaw01")
	if err != nil {
		t.Fatalf("GenerateRoomID error = %v", err)
	}
	if !strings.HasPrefix(roomID, "openclaw01-") {
		t.Fatalf("roomID = %q, want openclaw01-*", roomID)
	}
}

func TestBuildArchiveBody(t *testing.T) {
	body := buildArchiveBody(RoomInfo{
		RoomID:    "room-1",
		Title:     "Test Room",
		Creator:   "agent://pc75/openclaw01",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}, []LiveMessage{{
		Protocol:     ProtocolVersion,
		Type:         TypeMessage,
		RoomID:       "room-1",
		Sender:       "agent://pc75/openclaw01",
		SenderPubKey: strings.Repeat("a", 64),
		Seq:          1,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Payload:      LivePayload{Content: "hello"},
	}})
	if !strings.Contains(body, "Test Room") {
		t.Fatalf("archive body missing title: %q", body)
	}
	if !strings.Contains(body, "hello") {
		t.Fatalf("archive body missing content: %q", body)
	}
}

func TestBuildArchiveBodyHidesHeartbeatAndArchiveNotice(t *testing.T) {
	now := time.Now().UTC()
	body := buildArchiveBody(RoomInfo{
		RoomID:    "room-1",
		Title:     "Test Room",
		Creator:   "agent://pc75/openclaw01",
		CreatedAt: now.Format(time.RFC3339),
	}, []LiveMessage{
		{
			Protocol:     ProtocolVersion,
			Type:         TypeJoin,
			RoomID:       "room-1",
			Sender:       "agent://pc75/openclaw01",
			SenderPubKey: strings.Repeat("a", 64),
			Seq:          1,
			Timestamp:    now.Format(time.RFC3339),
		},
		{
			Protocol:     ProtocolVersion,
			Type:         TypeHeartbeat,
			RoomID:       "room-1",
			Sender:       "agent://pc75/openclaw01",
			SenderPubKey: strings.Repeat("a", 64),
			Seq:          2,
			Timestamp:    now.Add(time.Second).Format(time.RFC3339),
		},
		{
			Protocol:     ProtocolVersion,
			Type:         TypeArchiveNotice,
			RoomID:       "room-1",
			Sender:       "agent://pc75/openclaw01",
			SenderPubKey: strings.Repeat("a", 64),
			Seq:          3,
			Timestamp:    now.Add(2 * time.Second).Format(time.RFC3339),
			Payload: LivePayload{Metadata: map[string]any{
				"archive.infohash": "abc123",
			}},
		},
		{
			Protocol:     ProtocolVersion,
			Type:         TypeMessage,
			RoomID:       "room-1",
			Sender:       "agent://pc75/openclaw01",
			SenderPubKey: strings.Repeat("a", 64),
			Seq:          4,
			Timestamp:    now.Add(3 * time.Second).Format(time.RFC3339),
			Payload:      LivePayload{Content: "hello"},
		},
	})
	if strings.Contains(body, "`heartbeat`") {
		t.Fatalf("archive body should hide heartbeat events: %q", body)
	}
	if strings.Contains(body, "`archive_notice`") {
		t.Fatalf("archive body should hide archive notices: %q", body)
	}
	if !strings.Contains(body, "`message`") || !strings.Contains(body, "hello") {
		t.Fatalf("archive body should keep message events: %q", body)
	}
	if !strings.Contains(body, "- 事件数：2") {
		t.Fatalf("archive body should count visible events only: %q", body)
	}
}

func TestBuildRoster(t *testing.T) {
	now := time.Now().UTC()
	roster := BuildRoster([]LiveMessage{
		{
			Protocol:     ProtocolVersion,
			Type:         TypeJoin,
			RoomID:       "room-1",
			Sender:       "agent://pc75/openclaw01",
			SenderPubKey: strings.Repeat("a", 64),
			Timestamp:    now.Add(-5 * time.Second).Format(time.RFC3339),
		},
		{
			Protocol:     ProtocolVersion,
			Type:         TypeHeartbeat,
			RoomID:       "room-1",
			Sender:       "agent://pc75/openclaw01",
			SenderPubKey: strings.Repeat("a", 64),
			Timestamp:    now.Add(-2 * time.Second).Format(time.RFC3339),
		},
		{
			Protocol:     ProtocolVersion,
			Type:         TypeJoin,
			RoomID:       "room-1",
			Sender:       "agent://pc76/openclaw01",
			SenderPubKey: strings.Repeat("b", 64),
			Timestamp:    now.Add(-40 * time.Second).Format(time.RFC3339),
		},
	}, now, 30*time.Second)
	if len(roster) != 2 {
		t.Fatalf("len(roster) = %d, want 2", len(roster))
	}
	if !roster[0].Online {
		t.Fatalf("expected first roster entry online")
	}
	if roster[1].Online {
		t.Fatalf("expected second roster entry offline after timeout")
	}
}

func TestSignAndVerifyMessage(t *testing.T) {
	identity, err := haonews.NewAgentIdentity("agent://pc75", "agent://pc75/openclaw01", time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity error = %v", err)
	}
	msg, err := NewSignedMessage(identity, identity.Author, "room-1", TypeMessage, 1, 0, LivePayload{Content: "hello"})
	if err != nil {
		t.Fatalf("NewSignedMessage error = %v", err)
	}
	if err := VerifyMessage(msg); err != nil {
		t.Fatalf("VerifyMessage error = %v", err)
	}
}

func TestSignAndVerifyChildMessageWithDelegation(t *testing.T) {
	rootIdentity, err := haonews.RecoverHDIdentity(
		"pc75",
		"agent://pc75",
		"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
		time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("RecoverHDIdentity error = %v", err)
	}
	childIdentity, err := haonews.DeriveChildIdentity(rootIdentity, "agent://pc75/live-alpha", time.Date(2026, 3, 30, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DeriveChildIdentity error = %v", err)
	}
	msg, err := NewSignedMessage(childIdentity, childIdentity.Author, "room-1", TypeMessage, 1, 0, LivePayload{Content: "hello"})
	if err != nil {
		t.Fatalf("NewSignedMessage error = %v", err)
	}
	if _, ok := msg.Payload.Metadata["hd.delegation"]; !ok {
		t.Fatal("expected hd.delegation metadata")
	}
	if err := VerifyMessage(msg); err != nil {
		t.Fatalf("VerifyMessage error = %v", err)
	}
}

func TestOpenLocalStoreListRooms(t *testing.T) {
	store, err := OpenLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	info := RoomInfo{
		RoomID:    "room-1",
		Title:     "Test",
		Creator:   "agent://pc75/openclaw01",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Channel:   "hao.news/live",
	}
	if err := store.SaveRoom(info); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	if err := store.AppendEvent(info.RoomID, LiveMessage{
		Protocol:     ProtocolVersion,
		Type:         TypeJoin,
		RoomID:       info.RoomID,
		Sender:       info.Creator,
		SenderPubKey: strings.Repeat("a", 64),
		Seq:          1,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("AppendEvent error = %v", err)
	}
	rooms, err := store.ListRooms()
	if err != nil {
		t.Fatalf("ListRooms error = %v", err)
	}
	if len(rooms) != 1 {
		t.Fatalf("len(rooms) = %d, want 1", len(rooms))
	}
	if rooms[0].EventCount != 1 {
		t.Fatalf("rooms[0].EventCount = %d, want 1", rooms[0].EventCount)
	}
	if !rooms[0].Active || rooms[0].ActiveParticipants != 1 || rooms[0].TotalParticipants != 1 {
		t.Fatalf("rooms[0] active summary = %#v, want 1 online participant", rooms[0])
	}
}

func TestSaveArchiveResult(t *testing.T) {
	store, err := OpenLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	info := RoomInfo{
		RoomID:    "room-archive",
		Title:     "Archive Test",
		Creator:   "agent://pc75/openclaw01",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := store.SaveRoom(info); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	if err := store.SaveArchiveResult(info.RoomID, ArchiveResult{
		RoomID:     info.RoomID,
		Channel:    "hao.news/live",
		Events:     3,
		ArchivedAt: time.Now().UTC().Format(time.RFC3339),
		ViewerURL:  "/posts/abc123",
		Published: haonews.PublishResult{
			InfoHash: "abc123",
		},
	}); err != nil {
		t.Fatalf("SaveArchiveResult error = %v", err)
	}
	record, err := store.LoadArchiveResult(info.RoomID)
	if err != nil {
		t.Fatalf("LoadArchiveResult error = %v", err)
	}
	if record == nil || record.InfoHash != "abc123" {
		t.Fatalf("archive infohash = %#v, want abc123", record)
	}
	rooms, err := store.ListRooms()
	if err != nil {
		t.Fatalf("ListRooms error = %v", err)
	}
	if rooms[0].Archive == nil || rooms[0].Archive.ViewerURL != "/posts/abc123" {
		t.Fatalf("rooms[0].Archive = %#v, want viewer url", rooms[0].Archive)
	}
}

func TestAppendEventSkipsImmediateDuplicate(t *testing.T) {
	store, err := OpenLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	room := RoomInfo{
		RoomID:    "room-dedupe",
		Title:     "Dedupe Test",
		Creator:   "agent://pc75/openclaw01",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	event := LiveMessage{
		Protocol:     ProtocolVersion,
		Type:         TypeMessage,
		RoomID:       room.RoomID,
		Sender:       room.Creator,
		SenderPubKey: strings.Repeat("a", 64),
		Seq:          1,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Payload:      LivePayload{Content: "dedupe-check"},
		Signature:    strings.Repeat("b", 128),
	}
	if err := store.AppendEvent(room.RoomID, event); err != nil {
		t.Fatalf("first AppendEvent error = %v", err)
	}
	if err := store.AppendEvent(room.RoomID, event); err != nil {
		t.Fatalf("second AppendEvent error = %v", err)
	}
	events, err := store.ReadEvents(room.RoomID)
	if err != nil {
		t.Fatalf("ReadEvents error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	rooms, err := store.ListRooms()
	if err != nil {
		t.Fatalf("ListRooms error = %v", err)
	}
	if len(rooms) != 1 || rooms[0].EventCount != 1 {
		t.Fatalf("rooms = %#v, want single event indexed", rooms)
	}
}

func TestAppendEventSkipsRecentDuplicateEvenWhenNotImmediate(t *testing.T) {
	store, err := OpenLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	room := RoomInfo{
		RoomID:    "room-dedupe-recent",
		Title:     "Dedupe Recent Test",
		Creator:   "agent://pc75/openclaw01",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	baseTime := time.Now().UTC()
	first := LiveMessage{
		Protocol:     ProtocolVersion,
		Type:         TypeMessage,
		RoomID:       room.RoomID,
		Sender:       room.Creator,
		SenderPubKey: strings.Repeat("a", 64),
		Seq:          1,
		Timestamp:    baseTime.Format(time.RFC3339),
		Payload:      LivePayload{Content: "dedupe-check"},
		Signature:    strings.Repeat("b", 128),
	}
	second := LiveMessage{
		Protocol:     ProtocolVersion,
		Type:         TypeMessage,
		RoomID:       room.RoomID,
		Sender:       room.Creator,
		SenderPubKey: strings.Repeat("c", 64),
		Seq:          2,
		Timestamp:    baseTime.Add(time.Second).Format(time.RFC3339),
		Payload:      LivePayload{Content: "another-event"},
		Signature:    strings.Repeat("d", 128),
	}
	if err := store.AppendEvent(room.RoomID, first); err != nil {
		t.Fatalf("first AppendEvent error = %v", err)
	}
	if err := store.AppendEvent(room.RoomID, second); err != nil {
		t.Fatalf("second AppendEvent error = %v", err)
	}
	if err := store.AppendEvent(room.RoomID, first); err != nil {
		t.Fatalf("third AppendEvent error = %v", err)
	}
	events, err := store.ReadEvents(room.RoomID)
	if err != nil {
		t.Fatalf("ReadEvents error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
}

func TestSaveRoomPreservesIndexAndMergesInfo(t *testing.T) {
	store, err := OpenLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	room := RoomInfo{
		RoomID:    "room-merge",
		Title:     "Original Title",
		Creator:   "agent://pc75/openclaw01",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Channel:   "hao.news/live",
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	if err := store.AppendEvent(room.RoomID, LiveMessage{
		Protocol:     ProtocolVersion,
		Type:         TypeJoin,
		RoomID:       room.RoomID,
		Sender:       room.Creator,
		SenderPubKey: strings.Repeat("a", 64),
		Seq:          1,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("AppendEvent error = %v", err)
	}
	if err := store.SaveRoom(RoomInfo{
		RoomID:      room.RoomID,
		Description: "From announce",
	}); err != nil {
		t.Fatalf("second SaveRoom error = %v", err)
	}
	loaded, err := store.LoadRoom(room.RoomID)
	if err != nil {
		t.Fatalf("LoadRoom error = %v", err)
	}
	if loaded.Title != "Original Title" || loaded.Description != "From announce" {
		t.Fatalf("loaded room = %#v", loaded)
	}
	rooms, err := store.ListRooms()
	if err != nil {
		t.Fatalf("ListRooms error = %v", err)
	}
	if rooms[0].EventCount != 1 {
		t.Fatalf("rooms[0].EventCount = %d, want 1", rooms[0].EventCount)
	}
}

func TestReadEventsIgnoresPartialTrailingJSONLine(t *testing.T) {
	store, err := OpenLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	room := RoomInfo{
		RoomID:    "room-partial-json",
		Title:     "Partial JSON Test",
		Creator:   "agent://pc75/openclaw01",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	event := LiveMessage{
		Protocol:     ProtocolVersion,
		Type:         TypeMessage,
		RoomID:       room.RoomID,
		Sender:       room.Creator,
		SenderPubKey: strings.Repeat("a", 64),
		Seq:          1,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Payload:      LivePayload{Content: "valid-event"},
		Signature:    strings.Repeat("b", 128),
	}
	if err := store.AppendEvent(room.RoomID, event); err != nil {
		t.Fatalf("AppendEvent error = %v", err)
	}
	path := filepath.Join(store.RoomDir(room.RoomID), "events.jsonl")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile error = %v", err)
	}
	if _, err := file.WriteString("{\"protocol\":\"haonews-live/0.1\""); err != nil {
		_ = file.Close()
		t.Fatalf("WriteString error = %v", err)
	}
	_ = file.Close()
	events, err := store.ReadEvents(room.RoomID)
	if err != nil {
		t.Fatalf("ReadEvents error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
}

func TestAppendEventRetainsAllNonHeartbeatEventsButPrunesHeartbeats(t *testing.T) {
	store, err := OpenLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	room := RoomInfo{
		RoomID:    "room-prune",
		Title:     "Prune Test",
		Creator:   "agent://pc75/openclaw01",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	base := time.Date(2026, 3, 30, 0, 0, 0, 0, time.UTC)
	expectedNonHeartbeat := 0
	for idx := 0; idx < 125; idx++ {
		eventType := TypeMessage
		if idx%3 == 0 {
			eventType = TypeHeartbeat
		} else {
			expectedNonHeartbeat++
		}
		if err := store.AppendEvent(room.RoomID, LiveMessage{
			Protocol:     ProtocolVersion,
			Type:         eventType,
			RoomID:       room.RoomID,
			Sender:       room.Creator,
			SenderPubKey: strings.Repeat("a", 64),
			Seq:          uint64(idx + 1),
			Timestamp:    base.Add(time.Duration(idx) * time.Second).Format(time.RFC3339),
			Payload:      LivePayload{Content: fmt.Sprintf("event-%03d", idx)},
			Signature:    strings.Repeat(fmt.Sprintf("%02x", idx), 64),
		}); err != nil {
			t.Fatalf("AppendEvent %d error = %v", idx, err)
		}
	}
	events, err := store.ReadEvents(room.RoomID)
	if err != nil {
		t.Fatalf("ReadEvents error = %v", err)
	}
	nonHeartbeat := 0
	heartbeat := 0
	for _, event := range events {
		if event.Type == TypeHeartbeat {
			heartbeat++
			continue
		}
		nonHeartbeat++
	}
	if nonHeartbeat != expectedNonHeartbeat {
		t.Fatalf("nonHeartbeat = %d, want %d", nonHeartbeat, expectedNonHeartbeat)
	}
	if heartbeat > LiveRoomRetainHeartbeatEvents {
		t.Fatalf("heartbeat = %d, want <= %d", heartbeat, LiveRoomRetainHeartbeatEvents)
	}
	rooms, err := store.ListRooms()
	if err != nil {
		t.Fatalf("ListRooms error = %v", err)
	}
	if len(rooms) != 1 {
		t.Fatalf("len(rooms) = %d, want 1", len(rooms))
	}
	if rooms[0].EventCount != expectedNonHeartbeat {
		t.Fatalf("rooms[0].EventCount = %d, want %d", rooms[0].EventCount, expectedNonHeartbeat)
	}
	historyArchives, err := store.ListHistoryArchives(room.RoomID)
	if err != nil {
		t.Fatalf("ListHistoryArchives error = %v", err)
	}
	if len(historyArchives) != 0 {
		t.Fatalf("len(historyArchives) = %d, want 0 when only heartbeats are pruned", len(historyArchives))
	}
}

func TestRoomInfoFromAnnouncement(t *testing.T) {
	info := roomInfoFromAnnouncement(LiveMessage{
		Protocol:     ProtocolVersion,
		Type:         TypeRoomAnnounce,
		RoomID:       "room-announce",
		Sender:       "agent://pc75/openclaw01",
		SenderPubKey: strings.Repeat("a", 64),
		Timestamp:    "2026-03-19T00:00:00Z",
		Payload: LivePayload{
			Metadata: map[string]any{
				"title":       "Live Room",
				"created_at":  "2026-03-19T00:00:00Z",
				"channel":     "hao.news/live",
				"network_id":  "mainnet",
				"description": "room announcement",
			},
		},
	})
	if info.RoomID != "room-announce" || info.Title != "Live Room" || info.Creator != "agent://pc75/openclaw01" {
		t.Fatalf("room info = %#v", info)
	}
}

func TestSaveRoomAuthoritativeOverridesPlaceholderOwner(t *testing.T) {
	store, err := OpenLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	if err := store.SaveRoom(RoomInfo{
		RoomID:          "room-authoritative",
		Creator:         "agent://pc75/live-bravo",
		CreatorPubKey:   strings.Repeat("b", 64),
		ParentPublicKey: strings.Repeat("c", 64),
		CreatedAt:       "2026-03-30T01:18:07Z",
	}); err != nil {
		t.Fatalf("SaveRoom placeholder error = %v", err)
	}
	if err := store.SaveRoomAuthoritative(RoomInfo{
		RoomID:          "room-authoritative",
		Title:           "Authoritative Room",
		Creator:         "agent://pc75/openclaw01",
		CreatorPubKey:   strings.Repeat("a", 64),
		ParentPublicKey: strings.Repeat("c", 64),
		CreatedAt:       "2026-03-30T01:18:17Z",
		Channel:         "hao.news/live",
	}); err != nil {
		t.Fatalf("SaveRoomAuthoritative error = %v", err)
	}
	room, err := store.LoadRoom("room-authoritative")
	if err != nil {
		t.Fatalf("LoadRoom error = %v", err)
	}
	if room.Creator != "agent://pc75/openclaw01" || room.Title != "Authoritative Room" {
		t.Fatalf("room = %#v, want authoritative owner/title", room)
	}
}

func TestCreateManualAndDailyHistoryArchives(t *testing.T) {
	store, err := OpenLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	room := RoomInfo{
		RoomID:    "room-archive-daily",
		Title:     "Archive Daily",
		Creator:   "agent://pc75/openclaw01",
		CreatedAt: "2026-04-01T00:00:00Z",
		Channel:   "hao.news/live",
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	events := []LiveMessage{
		{
			Protocol:     ProtocolVersion,
			Type:         TypeMessage,
			RoomID:       room.RoomID,
			Sender:       room.Creator,
			SenderPubKey: strings.Repeat("a", 64),
			Seq:          1,
			Timestamp:    "2026-04-01T21:40:00Z", // 2026-04-02 05:40 CST
			Payload:      LivePayload{Content: "daily-window-a"},
			Signature:    strings.Repeat("1", 128),
		},
		{
			Protocol:     ProtocolVersion,
			Type:         TypeTaskUpdate,
			RoomID:       room.RoomID,
			Sender:       room.Creator,
			SenderPubKey: strings.Repeat("a", 64),
			Seq:          2,
			Timestamp:    "2026-04-02T05:00:00Z", // 2026-04-02 13:00 CST
			Payload: LivePayload{Metadata: map[string]any{
				"task_id": "task-daily",
				"status":  "doing",
			}},
			Signature: strings.Repeat("2", 128),
		},
		{
			Protocol:     ProtocolVersion,
			Type:         TypeHeartbeat,
			RoomID:       room.RoomID,
			Sender:       room.Creator,
			SenderPubKey: strings.Repeat("a", 64),
			Seq:          3,
			Timestamp:    "2026-04-02T05:05:00Z",
			Signature:    strings.Repeat("3", 128),
		},
	}
	for _, event := range events {
		if err := store.AppendEvent(room.RoomID, event); err != nil {
			t.Fatalf("AppendEvent error = %v", err)
		}
	}
	manual, err := store.CreateManualHistoryArchive(room.RoomID, time.Date(2026, 4, 2, 13, 6, 0, 0, time.FixedZone("CST", 8*60*60)))
	if err != nil {
		t.Fatalf("CreateManualHistoryArchive error = %v", err)
	}
	if manual == nil || manual.Kind != "manual" || manual.EventCount != 2 || manual.MessageCount != 1 || manual.TaskUpdateCount != 1 {
		t.Fatalf("manual archive = %#v", manual)
	}
	created, err := store.EnsureDailyHistoryArchives(room.RoomID, time.Date(2026, 4, 3, 6, 0, 0, 0, time.FixedZone("CST", 8*60*60)))
	if err != nil {
		t.Fatalf("EnsureDailyHistoryArchives error = %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("len(created) = %d, want 1", len(created))
	}
	if created[0].Kind != "daily" || created[0].ArchiveID != "daily-20260403-0530" {
		t.Fatalf("daily archive = %#v", created[0])
	}
	if created[0].EventCount != 2 || created[0].MessageCount != 1 || created[0].TaskUpdateCount != 1 {
		t.Fatalf("daily counters = %#v", created[0])
	}
	again, err := store.EnsureDailyHistoryArchives(room.RoomID, time.Date(2026, 4, 3, 6, 10, 0, 0, time.FixedZone("CST", 8*60*60)))
	if err != nil {
		t.Fatalf("EnsureDailyHistoryArchives again error = %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("len(again) = %d, want 0", len(again))
	}
}

func TestHistoryArchivesKeepAllVisibleEventsBeyondDisplayWindow(t *testing.T) {
	store, err := OpenLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	room := RoomInfo{
		RoomID:    "room-archive-full-visible",
		Title:     "Archive Full Visible",
		Creator:   "agent://pc75/openclaw01",
		CreatedAt: "2026-04-01T00:00:00Z",
		Channel:   "hao.news/live",
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	for idx := 0; idx < 105; idx++ {
		event := LiveMessage{
			Protocol:     ProtocolVersion,
			Type:         TypeMessage,
			RoomID:       room.RoomID,
			Sender:       room.Creator,
			SenderPubKey: strings.Repeat("a", 64),
			Seq:          uint64(idx + 1),
			Timestamp:    fmt.Sprintf("2026-04-02T%02d:%02d:00Z", idx/60, idx%60),
			Payload:      LivePayload{Content: fmt.Sprintf("archive-event-%03d", idx)},
			Signature:    fmt.Sprintf("%0128d", idx+1),
		}
		if err := store.AppendEvent(room.RoomID, event); err != nil {
			t.Fatalf("AppendEvent %d error = %v", idx, err)
		}
	}
	for hb := 0; hb < 30; hb++ {
		event := LiveMessage{
			Protocol:     ProtocolVersion,
			Type:         TypeHeartbeat,
			RoomID:       room.RoomID,
			Sender:       room.Creator,
			SenderPubKey: strings.Repeat("a", 64),
			Seq:          uint64(1000 + hb),
			Timestamp:    fmt.Sprintf("2026-04-02T02:%02d:30Z", hb%60),
			Signature:    fmt.Sprintf("%0128d", 2000+hb),
		}
		if err := store.AppendEvent(room.RoomID, event); err != nil {
			t.Fatalf("AppendEvent heartbeat %d error = %v", hb, err)
		}
	}

	events, err := store.ReadEvents(room.RoomID)
	if err != nil {
		t.Fatalf("ReadEvents error = %v", err)
	}
	visible := archiveDisplayEvents(events)
	if got := len(visible); got != 105 {
		t.Fatalf("len(archiveDisplayEvents(ReadEvents)) = %d, want 105", got)
	}

	manual, err := store.CreateManualHistoryArchive(room.RoomID, time.Date(2026, 4, 2, 13, 6, 0, 0, time.FixedZone("CST", 8*60*60)))
	if err != nil {
		t.Fatalf("CreateManualHistoryArchive error = %v", err)
	}
	if manual == nil {
		t.Fatal("manual archive = nil, want record")
	}
	if manual.EventCount != 105 || len(manual.Events) != 105 || manual.MessageCount != 105 {
		t.Fatalf("manual archive counts = %#v, want 105 visible events", manual)
	}

	created, err := store.EnsureDailyHistoryArchives(room.RoomID, time.Date(2026, 4, 3, 6, 0, 0, 0, time.FixedZone("CST", 8*60*60)))
	if err != nil {
		t.Fatalf("EnsureDailyHistoryArchives error = %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("len(created) = %d, want 1", len(created))
	}
	if created[0].EventCount != 105 || len(created[0].Events) != 105 || created[0].MessageCount != 105 {
		t.Fatalf("daily archive counts = %#v, want 105 visible events", created[0])
	}
}

func TestAnnouncementWatcherHandleArchiveNotice(t *testing.T) {
	root := t.TempDir()
	store, err := OpenLocalStore(root)
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	identity, err := haonews.NewAgentIdentity("agent://pc75", "agent://pc75/openclaw01", time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity error = %v", err)
	}
	info := RoomInfo{
		RoomID:      "room-archive-notice",
		Title:       "Archive Notice Test",
		Creator:     identity.Author,
		CreatedAt:   "2026-03-19T10:00:00Z",
		Channel:     "hao.news/live",
		Description: "archive notice propagation",
	}
	result := ArchiveResult{
		RoomID:     info.RoomID,
		Channel:    "hao.news/live",
		Events:     5,
		ArchivedAt: "2026-03-19T10:05:00Z",
		ViewerURL:  "/posts/338f294e18ac2fe39c0a5201845bc6e4d7cc33c0",
		Published: haonews.PublishResult{
			InfoHash: "338f294e18ac2fe39c0a5201845bc6e4d7cc33c0",
			Ref:      "haonews-sync://bundle/338f294e18ac2fe39c0a5201845bc6e4d7cc33c0?dn=live-archive",
		},
	}
	event, err := NewSignedMessage(identity, identity.Author, info.RoomID, TypeArchiveNotice, 1, 0, LivePayload{
		Content:     result.ViewerURL,
		ContentType: "application/json",
		Metadata:    archiveNoticeMetadata(info, result),
	})
	if err != nil {
		t.Fatalf("NewSignedMessage error = %v", err)
	}
	watcher := &AnnouncementWatcher{store: store}
	if err := watcher.handleEvent(event); err != nil {
		t.Fatalf("handleEvent error = %v", err)
	}
	room, err := store.LoadRoom(info.RoomID)
	if err != nil {
		t.Fatalf("LoadRoom error = %v", err)
	}
	if room.Title != info.Title || room.Channel != info.Channel {
		t.Fatalf("saved room = %#v, want title/channel from notice", room)
	}
	archive, err := store.LoadArchiveResult(info.RoomID)
	if err != nil {
		t.Fatalf("LoadArchiveResult error = %v", err)
	}
	if archive == nil || archive.InfoHash != result.Published.InfoHash || archive.ViewerURL != result.ViewerURL {
		t.Fatalf("archive = %#v, want infohash/viewer url", archive)
	}
	if archive.Ref != result.Published.Ref {
		t.Fatalf("archive ref = %#v, want %q", archive.Ref, result.Published.Ref)
	}
	events, err := store.ReadEvents(info.RoomID)
	if err != nil {
		t.Fatalf("ReadEvents error = %v", err)
	}
	if len(events) != 1 || events[0].Type != TypeArchiveNotice {
		t.Fatalf("events = %#v, want single archive_notice", events)
	}
	queueBody, err := os.ReadFile(filepath.Join(root, "sync", "realtime.txt"))
	if err != nil {
		t.Fatalf("read queue error = %v", err)
	}
	if !strings.Contains(string(queueBody), result.Published.InfoHash) {
		t.Fatalf("queue missing archive sync ref: %s", string(queueBody))
	}
}

func TestAnnouncementWatcherJoinDoesNotOverrideExistingRoomOwner(t *testing.T) {
	root := t.TempDir()
	store, err := OpenLocalStore(root)
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	if err := store.SaveRoomAuthoritative(RoomInfo{
		RoomID:          "room-owner-stable",
		Title:           "Stable Room",
		Creator:         "agent://pc75/openclaw01",
		CreatorPubKey:   strings.Repeat("a", 64),
		ParentPublicKey: strings.Repeat("c", 64),
		CreatedAt:       "2026-03-30T01:18:17Z",
		Channel:         "hao.news/live",
	}); err != nil {
		t.Fatalf("SaveRoomAuthoritative error = %v", err)
	}
	identity, err := haonews.NewAgentIdentity("agent://pc75", "agent://pc75/live-bravo", time.Now().UTC())
	if err != nil {
		t.Fatalf("NewAgentIdentity error = %v", err)
	}
	event, err := NewSignedMessage(identity, identity.Author, "room-owner-stable", TypeJoin, 1, 0, LivePayload{
		Metadata: map[string]any{
			"origin_public_key": strings.Repeat("b", 64),
			"parent_public_key": strings.Repeat("c", 64),
		},
	})
	if err != nil {
		t.Fatalf("NewSignedMessage error = %v", err)
	}
	watcher := &AnnouncementWatcher{store: store}
	if err := watcher.handleEvent(event); err != nil {
		t.Fatalf("handleEvent error = %v", err)
	}
	room, err := store.LoadRoom("room-owner-stable")
	if err != nil {
		t.Fatalf("LoadRoom error = %v", err)
	}
	if room.Creator != "agent://pc75/openclaw01" || room.Title != "Stable Room" {
		t.Fatalf("room = %#v, want original owner/title preserved", room)
	}
}

func TestWaitForTopicPeersWithoutTopic(t *testing.T) {
	s := &session{}
	start := time.Now()
	ok := s.waitForTopicPeers(context.Background(), 1, 20*time.Millisecond)
	if ok {
		t.Fatalf("waitForTopicPeers() = true, want false")
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatalf("waitForTopicPeers took too long without topic")
	}
}
