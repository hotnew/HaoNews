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
