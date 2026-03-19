package live

import (
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
