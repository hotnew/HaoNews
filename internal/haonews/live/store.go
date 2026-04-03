package live

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"hao.news/internal/haonews"
)

type LocalStore struct {
	Root  string
	redis *haonews.RedisClient
}

type RoomSummary struct {
	RoomID               string         `json:"room_id"`
	Title                string         `json:"title"`
	Creator              string         `json:"creator"`
	CreatorPubKey        string         `json:"creator_pubkey,omitempty"`
	ParentPublicKey      string         `json:"parent_public_key,omitempty"`
	LiveVisibility       string         `json:"live_visibility,omitempty"`
	PendingBlockedEvents int            `json:"pending_blocked_events,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
	LastEventAt          time.Time      `json:"last_event_at,omitempty"`
	EventCount           int            `json:"event_count"`
	Channel              string         `json:"channel,omitempty"`
	Active               bool           `json:"active"`
	ActiveParticipants   int            `json:"active_participants"`
	TotalParticipants    int            `json:"total_participants"`
	Archive              *ArchiveRecord `json:"archive,omitempty"`
	Path                 string         `json:"path"`
}

type ArchiveRecord struct {
	RoomID      string `json:"room_id"`
	Channel     string `json:"channel"`
	InfoHash    string `json:"infohash"`
	Ref         string `json:"ref,omitempty"`
	Magnet      string `json:"magnet,omitempty"`
	TorrentFile string `json:"torrent_file,omitempty"`
	ContentDir  string `json:"content_dir,omitempty"`
	ViewerURL   string `json:"viewer_url,omitempty"`
	Events      int    `json:"events"`
	ArchivedAt  string `json:"archived_at"`
}

type RoomHistoryArchive struct {
	ArchiveID       string        `json:"archive_id"`
	RoomID          string        `json:"room_id"`
	Kind            string        `json:"kind,omitempty"`
	Label           string        `json:"label,omitempty"`
	ArchivedAt      string        `json:"archived_at"`
	StartAt         string        `json:"start_at,omitempty"`
	EndAt           string        `json:"end_at,omitempty"`
	EventCount      int           `json:"event_count"`
	MessageCount    int           `json:"message_count,omitempty"`
	TaskUpdateCount int           `json:"task_update_count,omitempty"`
	Participants    []string      `json:"participants,omitempty"`
	Events          []LiveMessage `json:"events,omitempty"`
}

type roomRecord struct {
	Info               RoomInfo `json:"info"`
	EventCount         int      `json:"event_count"`
	LastEventAt        string   `json:"last_event_at,omitempty"`
	Active             bool     `json:"active,omitempty"`
	ActiveParticipants int      `json:"active_participants,omitempty"`
	TotalParticipants  int      `json:"total_participants,omitempty"`
}

func OpenLocalStore(storeRoot string) (*LocalStore, error) {
	return OpenLocalStoreWithRedis(storeRoot, haonews.RedisConfig{})
}

func OpenLocalStoreWithRedis(storeRoot string, redisCfg haonews.RedisConfig) (*LocalStore, error) {
	store, err := haonews.OpenStore(storeRoot)
	if err != nil {
		return nil, err
	}
	root := filepath.Join(store.Root, "live")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	redisClient, err := haonews.NewRedisClient(redisCfg)
	if err != nil {
		return &LocalStore{Root: root}, nil
	}
	return &LocalStore{Root: root, redis: redisClient}, nil
}

func (s *LocalStore) Close() error {
	if s == nil || s.redis == nil {
		return nil
	}
	return s.redis.Close()
}

func (s *LocalStore) RoomDir(roomID string) string {
	return filepath.Join(s.Root, strings.TrimSpace(roomID))
}

func (s *LocalStore) archivePath(roomID string) string {
	return filepath.Join(s.RoomDir(roomID), "archive.json")
}

func (s *LocalStore) historyDir(roomID string) string {
	return filepath.Join(s.RoomDir(roomID), "history")
}

func (s *LocalStore) historyPath(roomID, archiveID string) string {
	return filepath.Join(s.historyDir(roomID), strings.TrimSpace(archiveID)+".json")
}

func (s *LocalStore) SaveRoom(info RoomInfo) error {
	if s == nil {
		return fmt.Errorf("local store is required")
	}
	dir := s.RoomDir(info.RoomID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return s.withRoomLock(info.RoomID, func() error {
		record := roomRecord{Info: info}
		if current, err := os.ReadFile(filepath.Join(dir, "room.json")); err == nil {
			var existing roomRecord
			if err := json.Unmarshal(current, &existing); err == nil {
				record.EventCount = existing.EventCount
				record.LastEventAt = existing.LastEventAt
				record.Active = existing.Active
				record.ActiveParticipants = existing.ActiveParticipants
				record.TotalParticipants = existing.TotalParticipants
				record.Info = mergeRoomInfo(existing.Info, info)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		data, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
		if err := writeFileAtomic(filepath.Join(dir, "room.json"), data, 0o644); err != nil {
			return err
		}
		s.redisSetJSON(s.cacheRoomKey(info.RoomID), record.Info, s.redisTTL())
		s.redisDelete(s.cacheRoomsKey())
		return nil
	})
}

func (s *LocalStore) SaveRoomAuthoritative(info RoomInfo) error {
	if s == nil {
		return fmt.Errorf("local store is required")
	}
	dir := s.RoomDir(info.RoomID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return s.withRoomLock(info.RoomID, func() error {
		record := roomRecord{Info: info}
		if current, err := os.ReadFile(filepath.Join(dir, "room.json")); err == nil {
			var existing roomRecord
			if err := json.Unmarshal(current, &existing); err == nil {
				record.EventCount = existing.EventCount
				record.LastEventAt = existing.LastEventAt
				record.Active = existing.Active
				record.ActiveParticipants = existing.ActiveParticipants
				record.TotalParticipants = existing.TotalParticipants
				record.Info = mergeRoomInfoAuthoritative(existing.Info, info)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		data, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
		if err := writeFileAtomic(filepath.Join(dir, "room.json"), data, 0o644); err != nil {
			return err
		}
		s.redisSetJSON(s.cacheRoomKey(info.RoomID), record.Info, s.redisTTL())
		s.redisDelete(s.cacheRoomsKey())
		return nil
	})
}

func (s *LocalStore) LoadRoom(roomID string) (RoomInfo, error) {
	if s == nil {
		return RoomInfo{}, fmt.Errorf("local store is required")
	}
	var cached RoomInfo
	if ok, err := s.redisGetJSON(s.cacheRoomKey(roomID), &cached); err == nil && ok {
		return cached, nil
	}
	data, err := os.ReadFile(filepath.Join(s.RoomDir(roomID), "room.json"))
	if err != nil {
		return RoomInfo{}, err
	}
	var record roomRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return RoomInfo{}, err
	}
	s.redisSetJSON(s.cacheRoomKey(roomID), record.Info, s.redisTTL())
	return record.Info, nil
}

func (s *LocalStore) AppendEvent(roomID string, msg LiveMessage) error {
	if s == nil {
		return fmt.Errorf("local store is required")
	}
	dir := s.RoomDir(roomID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return s.withRoomLock(roomID, func() error {
		path := filepath.Join(dir, "events.jsonl")
		if duplicate, err := isImmediateDuplicateEvent(path, msg); err == nil && duplicate {
			return nil
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer file.Close()
		body, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		if _, err := file.Write(append(body, '\n')); err != nil {
			return err
		}
		if err := s.pruneRoomEvents(roomID); err != nil {
			return err
		}
		if err := s.refreshRoomIndex(roomID); err != nil {
			return err
		}
		s.redisDelete(s.cacheEventsKey(roomID), s.cacheRoomsKey())
		return nil
	})
}

func isImmediateDuplicateEvent(path string, msg LiveMessage) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	if info.Size() == 0 {
		return false, nil
	}
	const tailBytes int64 = 262144
	size := info.Size()
	readSize := size
	if readSize > tailBytes {
		readSize = tailBytes
	}
	buf := make([]byte, readSize)
	if _, err := file.ReadAt(buf, size-readSize); err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	lines := strings.Split(strings.TrimSpace(string(buf)), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		if line == "" {
			continue
		}
		var last LiveMessage
		if err := json.Unmarshal([]byte(line), &last); err != nil {
			continue
		}
		if liveEventKey(last) == liveEventKey(msg) {
			return true, nil
		}
	}
	return false, nil
}

func liveEventKey(msg LiveMessage) string {
	if signature := strings.TrimSpace(msg.Signature); signature != "" {
		return "sig:" + signature
	}
	return strings.Join([]string{
		strings.TrimSpace(msg.RoomID),
		strings.TrimSpace(msg.Type),
		strings.TrimSpace(msg.Sender),
		strings.TrimSpace(msg.SenderPubKey),
		fmt.Sprintf("%d", msg.Seq),
		strings.TrimSpace(msg.Timestamp),
		strings.TrimSpace(msg.Payload.Content),
	}, "\x00")
}

func (s *LocalStore) ReadEvents(roomID string) ([]LiveMessage, error) {
	var cached []LiveMessage
	if ok, err := s.redisGetJSON(s.cacheEventsKey(roomID), &cached); err == nil && ok {
		return cached, nil
	}
	path := filepath.Join(s.RoomDir(roomID), "events.jsonl")
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()
	var out []LiveMessage
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg LiveMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		out = append(out, msg)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	s.redisSetJSON(s.cacheEventsKey(roomID), out, s.redisTTL())
	return out, nil
}

func (s *LocalStore) ListRooms() ([]RoomSummary, error) {
	if s == nil {
		return nil, nil
	}
	var cached []RoomSummary
	if ok, err := s.redisGetJSON(s.cacheRoomsKey(), &cached); err == nil && ok {
		return cached, nil
	}
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var rooms []RoomSummary
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.Root, entry.Name(), "room.json"))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		var record roomRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, err
		}
		summary := RoomSummary{
			RoomID:             record.Info.RoomID,
			Title:              record.Info.Title,
			Creator:            record.Info.Creator,
			CreatorPubKey:      record.Info.CreatorPubKey,
			ParentPublicKey:    record.Info.ParentPublicKey,
			EventCount:         record.EventCount,
			Channel:            record.Info.Channel,
			Active:             record.Active,
			ActiveParticipants: record.ActiveParticipants,
			TotalParticipants:  record.TotalParticipants,
			Path:               filepath.Join(s.Root, entry.Name()),
		}
		archive, err := s.LoadArchiveResult(entry.Name())
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		summary.Archive = archive
		if createdAt, err := time.Parse(time.RFC3339, record.Info.CreatedAt); err == nil {
			summary.CreatedAt = createdAt
		}
		if record.LastEventAt != "" {
			if lastEventAt, err := time.Parse(time.RFC3339, record.LastEventAt); err == nil {
				summary.LastEventAt = lastEventAt
			}
		}
		if summary.TotalParticipants == 0 && summary.ActiveParticipants == 0 && summary.EventCount > 0 {
			events, err := s.ReadEvents(entry.Name())
			if err != nil {
				return nil, err
			}
			roster := BuildRoster(events, time.Now().UTC(), 30*time.Second)
			summary.TotalParticipants = len(roster)
			for _, participant := range roster {
				if participant.Online {
					summary.ActiveParticipants++
				}
			}
			summary.Active = summary.ActiveParticipants > 0
		}
		rooms = append(rooms, summary)
	}
	sort.Slice(rooms, func(i, j int) bool {
		if rooms[i].LastEventAt.Equal(rooms[j].LastEventAt) {
			return rooms[i].CreatedAt.After(rooms[j].CreatedAt)
		}
		return rooms[i].LastEventAt.After(rooms[j].LastEventAt)
	})
	s.redisSetJSON(s.cacheRoomsKey(), rooms, s.redisShortTTL())
	return rooms, nil
}

func (s *LocalStore) refreshRoomIndex(roomID string) error {
	path := filepath.Join(s.RoomDir(roomID), "room.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var record roomRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return err
	}
	events, err := s.ReadEvents(roomID)
	if err != nil {
		return err
	}
	record.EventCount = countIndexedLiveEvents(events)
	record.LastEventAt = latestEventTimestamp(events)
	roster := BuildRoster(events, time.Now().UTC(), 30*time.Second)
	record.TotalParticipants = len(roster)
	record.ActiveParticipants = 0
	for _, participant := range roster {
		if participant.Online {
			record.ActiveParticipants++
		}
	}
	record.Active = record.ActiveParticipants > 0
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return writeFileAtomic(path, body, 0o644)
}

func (s *LocalStore) pruneRoomEvents(roomID string) error {
	path := filepath.Join(s.RoomDir(roomID), "events.jsonl")
	events, err := s.ReadEvents(roomID)
	if err != nil {
		return err
	}
	pruned, dropped := retainRecentLiveEvents(events, 0, LiveRoomRetainHeartbeatEvents)
	if len(pruned) == len(events) {
		return nil
	}
	if err := s.savePrunedHistory(roomID, dropped); err != nil {
		return err
	}
	var body []byte
	for _, event := range pruned {
		line, err := json.Marshal(event)
		if err != nil {
			return err
		}
		body = append(body, line...)
		body = append(body, '\n')
	}
	return writeFileAtomic(path, body, 0o644)
}

func retainRecentLiveEvents(events []LiveMessage, keepNonHeartbeat, keepHeartbeat int) ([]LiveMessage, []LiveMessage) {
	if len(events) == 0 {
		return nil, nil
	}
	keep := make([]bool, len(events))
	nonHeartbeatCount := 0
	heartbeatCount := 0
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if strings.TrimSpace(event.Type) == TypeHeartbeat {
			if keepHeartbeat > 0 && heartbeatCount < keepHeartbeat {
				keep[index] = true
				heartbeatCount++
			}
			continue
		}
		if keepNonHeartbeat <= 0 || nonHeartbeatCount < keepNonHeartbeat {
			keep[index] = true
			nonHeartbeatCount++
		}
	}
	out := make([]LiveMessage, 0, nonHeartbeatCount+heartbeatCount)
	dropped := make([]LiveMessage, 0, len(events)-len(out))
	for index, event := range events {
		if keep[index] {
			out = append(out, event)
			continue
		}
		dropped = append(dropped, event)
	}
	return out, dropped
}

func countIndexedLiveEvents(events []LiveMessage) int {
	count := 0
	for _, event := range events {
		if strings.TrimSpace(event.Type) == TypeHeartbeat {
			continue
		}
		count++
	}
	return count
}

func latestEventTimestamp(events []LiveMessage) string {
	for index := len(events) - 1; index >= 0; index-- {
		ts := strings.TrimSpace(events[index].Timestamp)
		if ts != "" {
			return ts
		}
	}
	return ""
}

func (s *LocalStore) SaveArchiveResult(roomID string, result ArchiveResult) error {
	if s == nil {
		return fmt.Errorf("local store is required")
	}
	record := ArchiveRecord{
		RoomID:     strings.TrimSpace(roomID),
		Channel:    strings.TrimSpace(result.Channel),
		InfoHash:   strings.TrimSpace(result.Published.InfoHash),
		Ref:        firstNonEmpty(strings.TrimSpace(result.Published.Ref), strings.TrimSpace(result.Published.Magnet)),
		ContentDir: strings.TrimSpace(result.Published.ContentDir),
		ViewerURL:  strings.TrimSpace(result.ViewerURL),
		Events:     result.Events,
		ArchivedAt: strings.TrimSpace(result.ArchivedAt),
	}
	if record.ViewerURL == "" && record.InfoHash != "" {
		record.ViewerURL = "/posts/" + record.InfoHash
	}
	if err := os.MkdirAll(s.RoomDir(roomID), 0o755); err != nil {
		return err
	}
	return s.withRoomLock(roomID, func() error {
		body, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			return err
		}
		body = append(body, '\n')
		if err := writeFileAtomic(s.archivePath(roomID), body, 0o644); err != nil {
			return err
		}
		s.redisSetJSON(s.cacheArchiveKey(roomID), record, 30*24*time.Hour)
		s.redisDelete(s.cacheRoomsKey())
		return nil
	})
}

func (s *LocalStore) LoadArchiveResult(roomID string) (*ArchiveRecord, error) {
	if s == nil {
		return nil, fmt.Errorf("local store is required")
	}
	var cached ArchiveRecord
	if ok, err := s.redisGetJSON(s.cacheArchiveKey(roomID), &cached); err == nil && ok {
		if strings.TrimSpace(cached.ViewerURL) == "" && strings.TrimSpace(cached.InfoHash) != "" {
			cached.ViewerURL = "/posts/" + strings.TrimSpace(cached.InfoHash)
		}
		if strings.TrimSpace(cached.Ref) == "" {
			cached.Ref = firstNonEmpty(strings.TrimSpace(cached.Magnet), strings.TrimSpace(cached.InfoHash))
		}
		return &cached, nil
	}
	body, err := os.ReadFile(s.archivePath(roomID))
	if err != nil {
		return nil, err
	}
	var record ArchiveRecord
	if err := json.Unmarshal(body, &record); err != nil {
		return nil, err
	}
	if strings.TrimSpace(record.ViewerURL) == "" && strings.TrimSpace(record.InfoHash) != "" {
		record.ViewerURL = "/posts/" + strings.TrimSpace(record.InfoHash)
	}
	if strings.TrimSpace(record.Ref) == "" {
		record.Ref = firstNonEmpty(strings.TrimSpace(record.Magnet), strings.TrimSpace(record.InfoHash))
	}
	s.redisSetJSON(s.cacheArchiveKey(roomID), record, 30*24*time.Hour)
	return &record, nil
}

func (s *LocalStore) redisContext() context.Context {
	return context.Background()
}

func (s *LocalStore) redisTTL() time.Duration {
	if s == nil || s.redis == nil {
		return 0
	}
	return s.redis.DefaultTTL()
}

func (s *LocalStore) redisShortTTL() time.Duration {
	if s == nil || s.redis == nil {
		return 0
	}
	return s.redis.Config().ShortTTL()
}

func (s *LocalStore) cacheRoomKey(roomID string) string {
	if s == nil || s.redis == nil {
		return ""
	}
	return s.redis.Key("live", "room", strings.TrimSpace(roomID))
}

func (s *LocalStore) cacheEventsKey(roomID string) string {
	if s == nil || s.redis == nil {
		return ""
	}
	return s.redis.Key("live", "room", strings.TrimSpace(roomID), "events")
}

func (s *LocalStore) cacheArchiveKey(roomID string) string {
	if s == nil || s.redis == nil {
		return ""
	}
	return s.redis.Key("live", "room", strings.TrimSpace(roomID), "archive")
}

func (s *LocalStore) cacheRoomsKey() string {
	if s == nil || s.redis == nil {
		return ""
	}
	return s.redis.Key("live", "rooms", "list")
}

func (s *LocalStore) redisGetJSON(key string, dest any) (bool, error) {
	if s == nil || s.redis == nil || key == "" {
		return false, nil
	}
	return s.redis.GetJSON(s.redisContext(), key, dest)
}

func (s *LocalStore) redisSetJSON(key string, value any, ttl time.Duration) {
	if s == nil || s.redis == nil || key == "" || ttl <= 0 {
		return
	}
	_ = s.redis.SetJSON(s.redisContext(), key, value, ttl)
}

func (s *LocalStore) redisDelete(keys ...string) {
	if s == nil || s.redis == nil {
		return
	}
	filtered := make([]string, 0, len(keys))
	for _, key := range keys {
		if strings.TrimSpace(key) != "" {
			filtered = append(filtered, key)
		}
	}
	if len(filtered) == 0 {
		return
	}
	_ = s.redis.Delete(s.redisContext(), filtered...)
}

func (s *LocalStore) savePrunedHistory(roomID string, dropped []LiveMessage) error {
	visible := archiveDisplayEvents(dropped)
	if len(visible) == 0 {
		return nil
	}
	_, err := s.saveHistoryArchiveRecord(strings.TrimSpace(roomID), RoomHistoryArchive{
		ArchiveID: time.Now().UTC().Format("legacy-20060102T150405.000000000Z0700"),
		Kind:      "legacy-window",
		Label:     "旧窗口裁剪",
		Events:    visible,
	}, time.Now().UTC())
	return err
}

func (s *LocalStore) CreateManualHistoryArchive(roomID string, now time.Time) (*RoomHistoryArchive, error) {
	if s == nil {
		return nil, fmt.Errorf("local store is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return s.withHistoryArchive(roomID, "manual-"+sanitizeArchiveID(now.UTC().Format("20060102T150405Z0700")), "manual", "手动归档", time.Time{}, time.Time{}, now)
}

func (s *LocalStore) EnsureDailyHistoryArchives(roomID string, now time.Time) ([]RoomHistoryArchive, error) {
	if s == nil {
		return nil, fmt.Errorf("local store is required")
	}
	loc := liveArchiveLocation()
	if now.IsZero() {
		now = time.Now().In(loc)
	} else {
		now = now.In(loc)
	}
	events, err := s.ReadEvents(roomID)
	if err != nil {
		return nil, err
	}
	visible := archiveDisplayEvents(events)
	if len(visible) == 0 {
		return nil, nil
	}
	firstEventAt, err := parseLiveTimestamp(strings.TrimSpace(visible[0].Timestamp))
	if err != nil {
		return nil, nil
	}
	firstEventAt = firstEventAt.In(loc)
	latestCutoff := liveArchiveWindowEnd(now)
	if latestCutoff.IsZero() || latestCutoff.Before(firstEventAt) {
		return nil, nil
	}
	firstWindowEnd := liveArchiveWindowEnd(firstEventAt)
	if firstWindowEnd.Before(firstEventAt) {
		firstWindowEnd = firstWindowEnd.Add(24 * time.Hour)
	}
	if firstWindowEnd.After(latestCutoff) {
		return nil, nil
	}
	created := make([]RoomHistoryArchive, 0)
	for windowEnd := firstWindowEnd; !windowEnd.After(latestCutoff); windowEnd = windowEnd.Add(24 * time.Hour) {
		windowStart := windowEnd.Add(-24 * time.Hour)
		archiveID := liveDailyArchiveID(windowEnd)
		record, err := s.withHistoryArchive(roomID, archiveID, "daily", liveDailyArchiveLabel(windowStart, windowEnd), windowStart, windowEnd, windowEnd)
		if err != nil {
			return created, err
		}
		if record != nil {
			created = append(created, *record)
		}
	}
	return created, nil
}

func (s *LocalStore) withHistoryArchive(roomID, archiveID, kind, label string, start, end, archivedAt time.Time) (*RoomHistoryArchive, error) {
	if s == nil {
		return nil, fmt.Errorf("local store is required")
	}
	events, err := s.ReadEvents(roomID)
	if err != nil {
		return nil, err
	}
	visible := archiveDisplayEvents(events)
	if !start.IsZero() || !end.IsZero() {
		visible = filterArchiveEventsByWindow(visible, start, end)
	}
	if len(visible) == 0 {
		return nil, nil
	}
	if _, err := os.Stat(s.historyPath(roomID, archiveID)); err == nil {
		return nil, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	record := RoomHistoryArchive{
		ArchiveID: archiveID,
		RoomID:    strings.TrimSpace(roomID),
		Kind:      strings.TrimSpace(kind),
		Label:     strings.TrimSpace(label),
		Events:    visible,
	}
	return s.saveHistoryArchiveRecord(roomID, record, archivedAt)
}

func (s *LocalStore) saveHistoryArchiveRecord(roomID string, record RoomHistoryArchive, archivedAt time.Time) (*RoomHistoryArchive, error) {
	if s == nil {
		return nil, fmt.Errorf("local store is required")
	}
	roomID = strings.TrimSpace(roomID)
	record.RoomID = roomID
	record.ArchiveID = sanitizeArchiveID(record.ArchiveID)
	if record.ArchiveID == "" {
		record.ArchiveID = sanitizeArchiveID(archivedAt.UTC().Format("20060102T150405.000000000Z0700"))
	}
	if archivedAt.IsZero() {
		archivedAt = time.Now().UTC()
	}
	returnRecord := record
	err := s.withRoomLock(roomID, func() error {
		if err := os.MkdirAll(s.historyDir(roomID), 0o755); err != nil {
			return err
		}
		path := s.historyPath(roomID, record.ArchiveID)
		if body, err := os.ReadFile(path); err == nil {
			var existing RoomHistoryArchive
			if err := json.Unmarshal(body, &existing); err == nil {
				returnRecord = existing
				return nil
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		startAt, endAt := archiveEventRange(record.Events)
		record.ArchivedAt = archivedAt.UTC().Format(time.RFC3339)
		record.StartAt = firstNonEmpty(record.StartAt, startAt)
		record.EndAt = firstNonEmpty(record.EndAt, endAt)
		record.EventCount = len(record.Events)
		record.MessageCount = archiveCountByType(record.Events, TypeMessage)
		record.TaskUpdateCount = archiveCountByType(record.Events, TypeTaskUpdate)
		record.Participants = archiveParticipants(record.Events)
		body, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			return err
		}
		body = append(body, '\n')
		if err := writeFileAtomic(path, body, 0o644); err != nil {
			return err
		}
		returnRecord = record
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &returnRecord, nil
}

func (s *LocalStore) ListHistoryArchives(roomID string) ([]RoomHistoryArchive, error) {
	if s == nil {
		return nil, fmt.Errorf("local store is required")
	}
	entries, err := os.ReadDir(s.historyDir(roomID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]RoomHistoryArchive, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(s.historyDir(roomID), entry.Name()))
		if err != nil {
			return nil, err
		}
		var record RoomHistoryArchive
		if err := json.Unmarshal(body, &record); err != nil {
			return nil, err
		}
		record.Events = nil
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ArchivedAt > out[j].ArchivedAt
	})
	return out, nil
}

func (s *LocalStore) LoadHistoryArchive(roomID, archiveID string) (*RoomHistoryArchive, error) {
	if s == nil {
		return nil, fmt.Errorf("local store is required")
	}
	body, err := os.ReadFile(s.historyPath(roomID, archiveID))
	if err != nil {
		return nil, err
	}
	var record RoomHistoryArchive
	if err := json.Unmarshal(body, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func filterArchiveEventsByWindow(events []LiveMessage, start, end time.Time) []LiveMessage {
	if len(events) == 0 {
		return nil
	}
	out := make([]LiveMessage, 0, len(events))
	for _, event := range events {
		ts, err := parseLiveTimestamp(strings.TrimSpace(event.Timestamp))
		if err != nil {
			continue
		}
		if !start.IsZero() && ts.Before(start) {
			continue
		}
		if !end.IsZero() && !ts.Before(end) {
			continue
		}
		out = append(out, event)
	}
	return out
}

func parseLiveTimestamp(raw string) (time.Time, error) {
	return time.Parse(time.RFC3339, strings.TrimSpace(raw))
}

func liveArchiveLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return loc
}

func liveArchiveWindowEnd(now time.Time) time.Time {
	loc := liveArchiveLocation()
	now = now.In(loc)
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 5, 30, 0, 0, loc)
	if now.Before(cutoff) {
		return cutoff.Add(-24 * time.Hour)
	}
	return cutoff
}

func liveDailyArchiveID(windowEnd time.Time) string {
	windowEnd = windowEnd.In(liveArchiveLocation())
	return fmt.Sprintf("daily-%s-0530", windowEnd.Format("20060102"))
}

func liveDailyArchiveLabel(windowStart, windowEnd time.Time) string {
	windowStart = windowStart.In(liveArchiveLocation())
	windowEnd = windowEnd.In(liveArchiveLocation())
	return fmt.Sprintf("%s 至 %s", windowStart.Format("2006-01-02 05:30 MST"), windowEnd.Format("2006-01-02 05:30 MST"))
}

func sanitizeArchiveID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "", "+", "", " ", "-", ".", "")
	value = replacer.Replace(value)
	value = strings.Trim(value, "-")
	return value
}

func mergeRoomInfo(existing, incoming RoomInfo) RoomInfo {
	return RoomInfo{
		RoomID:          firstNonEmptyInfo(existing.RoomID, incoming.RoomID),
		Title:           firstNonEmptyInfo(existing.Title, incoming.Title),
		Creator:         firstNonEmptyInfo(existing.Creator, incoming.Creator),
		CreatorPubKey:   firstNonEmptyInfo(existing.CreatorPubKey, incoming.CreatorPubKey),
		ParentPublicKey: firstNonEmptyInfo(existing.ParentPublicKey, incoming.ParentPublicKey),
		CreatedAt:       firstNonEmptyInfo(existing.CreatedAt, incoming.CreatedAt),
		NetworkID:       firstNonEmptyInfo(existing.NetworkID, incoming.NetworkID),
		Channel:         firstNonEmptyInfo(existing.Channel, incoming.Channel),
		Tags:            firstNonEmptySlice(incoming.Tags, existing.Tags),
		Description:     firstNonEmptyInfo(existing.Description, incoming.Description),
	}
}

func mergeRoomInfoAuthoritative(existing, incoming RoomInfo) RoomInfo {
	return RoomInfo{
		RoomID:          firstNonEmptyInfo(existing.RoomID, incoming.RoomID),
		Title:           firstNonEmptyInfo(incoming.Title, existing.Title),
		Creator:         firstNonEmptyInfo(incoming.Creator, existing.Creator),
		CreatorPubKey:   firstNonEmptyInfo(incoming.CreatorPubKey, existing.CreatorPubKey),
		ParentPublicKey: firstNonEmptyInfo(incoming.ParentPublicKey, existing.ParentPublicKey),
		CreatedAt:       firstNonEmptyInfo(incoming.CreatedAt, existing.CreatedAt),
		NetworkID:       firstNonEmptyInfo(incoming.NetworkID, existing.NetworkID),
		Channel:         firstNonEmptyInfo(incoming.Channel, existing.Channel),
		Tags:            firstNonEmptySlice(incoming.Tags, existing.Tags),
		Description:     firstNonEmptyInfo(incoming.Description, existing.Description),
	}
}

func firstNonEmptyInfo(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmptySlice(values ...[]string) []string {
	for _, items := range values {
		if len(items) > 0 {
			out := make([]string, len(items))
			copy(out, items)
			return out
		}
	}
	return nil
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func (s *LocalStore) withRoomLock(roomID string, fn func() error) error {
	if s == nil {
		return fmt.Errorf("local store is required")
	}
	dir := s.RoomDir(roomID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	lockFile, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lockFile.Close()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN) }()
	return fn()
}
