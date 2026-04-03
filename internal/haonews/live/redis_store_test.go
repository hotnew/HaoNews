package live

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"hao.news/internal/haonews"

	miniredis "github.com/alicebob/miniredis/v2"
)

func TestLocalStoreUsesRedisCacheForRoomAndEvents(t *testing.T) {
	t.Parallel()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run error = %v", err)
	}
	defer mr.Close()

	store, err := OpenLocalStoreWithRedis(t.TempDir(), haonews.RedisConfig{
		Enabled:   true,
		Addr:      mr.Addr(),
		KeyPrefix: "haonews-test:",
	})
	if err != nil {
		t.Fatalf("OpenLocalStoreWithRedis error = %v", err)
	}
	defer store.Close()

	room := RoomInfo{
		RoomID:    "redis-room",
		Title:     "Redis Room",
		Creator:   "agent://pc75/test",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Channel:   "hao.news/live",
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	event := LiveMessage{
		RoomID:       room.RoomID,
		Type:         TypeMessage,
		Sender:       room.Creator,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Seq:          1,
		Signature:    "sig-1",
		SenderPubKey: "pub-1",
		Payload: LivePayload{
			Content: "hello redis",
		},
	}
	if err := store.AppendEvent(room.RoomID, event); err != nil {
		t.Fatalf("AppendEvent error = %v", err)
	}

	if _, err := store.LoadRoom(room.RoomID); err != nil {
		t.Fatalf("LoadRoom(warm) error = %v", err)
	}
	if _, err := store.ReadEvents(room.RoomID); err != nil {
		t.Fatalf("ReadEvents(warm) error = %v", err)
	}
	if _, err := store.ListRooms(); err != nil {
		t.Fatalf("ListRooms(warm) error = %v", err)
	}

	if err := os.Remove(filepath.Join(store.RoomDir(room.RoomID), "room.json")); err != nil {
		t.Fatalf("remove room.json: %v", err)
	}
	if err := os.Remove(filepath.Join(store.RoomDir(room.RoomID), "events.jsonl")); err != nil {
		t.Fatalf("remove events.jsonl: %v", err)
	}

	loadedRoom, err := store.LoadRoom(room.RoomID)
	if err != nil {
		t.Fatalf("LoadRoom(cache) error = %v", err)
	}
	if loadedRoom.Title != room.Title {
		t.Fatalf("cached room title = %q", loadedRoom.Title)
	}
	events, err := store.ReadEvents(room.RoomID)
	if err != nil {
		t.Fatalf("ReadEvents(cache) error = %v", err)
	}
	if len(events) != 1 || events[0].Payload.Content != "hello redis" {
		t.Fatalf("cached events = %+v", events)
	}
	rooms, err := store.ListRooms()
	if err != nil {
		t.Fatalf("ListRooms(cache) error = %v", err)
	}
	if len(rooms) != 1 || rooms[0].RoomID != room.RoomID {
		t.Fatalf("cached rooms = %+v", rooms)
	}
}

func TestListRoomsFallsBackToRoomSummaryWithoutEventsScan(t *testing.T) {
	t.Parallel()

	store, err := OpenLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenLocalStore error = %v", err)
	}
	room := RoomInfo{
		RoomID:    "summary-room",
		Title:     "Summary Room",
		Creator:   "agent://pc75/test",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Channel:   "hao.news/live",
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	if err := store.AppendEvent(room.RoomID, LiveMessage{
		RoomID:       room.RoomID,
		Type:         TypeMessage,
		Sender:       room.Creator,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Seq:          1,
		Signature:    "summary-sig-1",
		SenderPubKey: "pub-summary-1",
		Payload:      LivePayload{Content: "hello summary"},
	}); err != nil {
		t.Fatalf("AppendEvent error = %v", err)
	}

	if err := os.Remove(filepath.Join(store.RoomDir(room.RoomID), "events.jsonl")); err != nil {
		t.Fatalf("remove events.jsonl: %v", err)
	}
	rooms, err := store.ListRooms()
	if err != nil {
		t.Fatalf("ListRooms error = %v", err)
	}
	if len(rooms) != 1 {
		t.Fatalf("expected 1 room, got %d", len(rooms))
	}
	if rooms[0].EventCount != 1 {
		t.Fatalf("EventCount = %d, want 1", rooms[0].EventCount)
	}
}

func TestListHistoryArchivesUsesRedisCache(t *testing.T) {
	t.Parallel()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run error = %v", err)
	}
	defer mr.Close()

	store, err := OpenLocalStoreWithRedis(t.TempDir(), haonews.RedisConfig{
		Enabled:   true,
		Addr:      mr.Addr(),
		KeyPrefix: "haonews-test:",
	})
	if err != nil {
		t.Fatalf("OpenLocalStoreWithRedis error = %v", err)
	}
	defer store.Close()

	room := RoomInfo{
		RoomID:    "history-cache-room",
		Title:     "History Cache Room",
		Creator:   "agent://pc75/test",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Channel:   "hao.news/live",
	}
	if err := store.SaveRoom(room); err != nil {
		t.Fatalf("SaveRoom error = %v", err)
	}
	for i := 1; i <= 2; i++ {
		if err := store.AppendEvent(room.RoomID, LiveMessage{
			RoomID:       room.RoomID,
			Type:         TypeMessage,
			Sender:       room.Creator,
			Timestamp:    time.Now().UTC().Add(time.Duration(i) * time.Minute).Format(time.RFC3339),
			Seq:          uint64(i),
			Signature:    "history-cache-sig-" + string(rune('0'+i)),
			SenderPubKey: "pub-history-cache",
			Payload:      LivePayload{Content: "msg"},
		}); err != nil {
			t.Fatalf("AppendEvent(%d) error = %v", i, err)
		}
	}
	record, err := store.CreateManualHistoryArchive(room.RoomID, time.Now().UTC())
	if err != nil {
		t.Fatalf("CreateManualHistoryArchive error = %v", err)
	}
	if record == nil {
		t.Fatal("expected manual archive")
	}

	archives, err := store.ListHistoryArchives(room.RoomID)
	if err != nil {
		t.Fatalf("ListHistoryArchives(warm) error = %v", err)
	}
	if len(archives) != 1 {
		t.Fatalf("warm archives len = %d, want 1", len(archives))
	}
	if err := os.Remove(filepath.Join(store.historyDir(room.RoomID), record.ArchiveID+".json")); err != nil {
		t.Fatalf("remove history archive: %v", err)
	}

	cached, err := store.ListHistoryArchives(room.RoomID)
	if err != nil {
		t.Fatalf("ListHistoryArchives(cache) error = %v", err)
	}
	if len(cached) != 1 || cached[0].ArchiveID != record.ArchiveID {
		t.Fatalf("cached archives = %+v", cached)
	}
}
