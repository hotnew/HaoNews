package live

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"hao.news/internal/haonews"
)

type LocalStore struct {
	Root string
}

type RoomSummary struct {
	RoomID             string         `json:"room_id"`
	Title              string         `json:"title"`
	Creator            string         `json:"creator"`
	CreatedAt          time.Time      `json:"created_at"`
	LastEventAt        time.Time      `json:"last_event_at,omitempty"`
	EventCount         int            `json:"event_count"`
	Channel            string         `json:"channel,omitempty"`
	Active             bool           `json:"active"`
	ActiveParticipants int            `json:"active_participants"`
	TotalParticipants  int            `json:"total_participants"`
	Archive            *ArchiveRecord `json:"archive,omitempty"`
	Path               string         `json:"path"`
}

type ArchiveRecord struct {
	RoomID      string `json:"room_id"`
	Channel     string `json:"channel"`
	InfoHash    string `json:"infohash"`
	Magnet      string `json:"magnet,omitempty"`
	TorrentFile string `json:"torrent_file,omitempty"`
	ContentDir  string `json:"content_dir,omitempty"`
	ViewerURL   string `json:"viewer_url,omitempty"`
	Events      int    `json:"events"`
	ArchivedAt  string `json:"archived_at"`
}

type roomRecord struct {
	Info        RoomInfo `json:"info"`
	EventCount  int      `json:"event_count"`
	LastEventAt string   `json:"last_event_at,omitempty"`
}

func OpenLocalStore(storeRoot string) (*LocalStore, error) {
	store, err := haonews.OpenStore(storeRoot)
	if err != nil {
		return nil, err
	}
	root := filepath.Join(store.Root, "live")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &LocalStore{Root: root}, nil
}

func (s *LocalStore) RoomDir(roomID string) string {
	return filepath.Join(s.Root, strings.TrimSpace(roomID))
}

func (s *LocalStore) archivePath(roomID string) string {
	return filepath.Join(s.RoomDir(roomID), "archive.json")
}

func (s *LocalStore) SaveRoom(info RoomInfo) error {
	if s == nil {
		return fmt.Errorf("local store is required")
	}
	dir := s.RoomDir(info.RoomID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	record := roomRecord{Info: info}
	if current, err := os.ReadFile(filepath.Join(dir, "room.json")); err == nil {
		var existing roomRecord
		if err := json.Unmarshal(current, &existing); err == nil {
			record.EventCount = existing.EventCount
			record.LastEventAt = existing.LastEventAt
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
	return os.WriteFile(filepath.Join(dir, "room.json"), data, 0o644)
}

func (s *LocalStore) LoadRoom(roomID string) (RoomInfo, error) {
	if s == nil {
		return RoomInfo{}, fmt.Errorf("local store is required")
	}
	data, err := os.ReadFile(filepath.Join(s.RoomDir(roomID), "room.json"))
	if err != nil {
		return RoomInfo{}, err
	}
	var record roomRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return RoomInfo{}, err
	}
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
	file, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
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
	return s.updateRoomIndex(roomID, msg)
}

func (s *LocalStore) ReadEvents(roomID string) ([]LiveMessage, error) {
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
			return nil, err
		}
		out = append(out, msg)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *LocalStore) ListRooms() ([]RoomSummary, error) {
	if s == nil {
		return nil, nil
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
			RoomID:     record.Info.RoomID,
			Title:      record.Info.Title,
			Creator:    record.Info.Creator,
			EventCount: record.EventCount,
			Channel:    record.Info.Channel,
			Path:       filepath.Join(s.Root, entry.Name()),
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
		rooms = append(rooms, summary)
	}
	sort.Slice(rooms, func(i, j int) bool {
		if rooms[i].LastEventAt.Equal(rooms[j].LastEventAt) {
			return rooms[i].CreatedAt.After(rooms[j].CreatedAt)
		}
		return rooms[i].LastEventAt.After(rooms[j].LastEventAt)
	})
	return rooms, nil
}

func (s *LocalStore) updateRoomIndex(roomID string, msg LiveMessage) error {
	path := filepath.Join(s.RoomDir(roomID), "room.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var record roomRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return err
	}
	record.EventCount++
	record.LastEventAt = msg.Timestamp
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}

func (s *LocalStore) SaveArchiveResult(roomID string, result ArchiveResult) error {
	if s == nil {
		return fmt.Errorf("local store is required")
	}
	record := ArchiveRecord{
		RoomID:      strings.TrimSpace(roomID),
		Channel:     strings.TrimSpace(result.Channel),
		InfoHash:    strings.TrimSpace(result.Published.InfoHash),
		Magnet:      strings.TrimSpace(result.Published.Magnet),
		TorrentFile: strings.TrimSpace(result.Published.TorrentFile),
		ContentDir:  strings.TrimSpace(result.Published.ContentDir),
		ViewerURL:   strings.TrimSpace(result.ViewerURL),
		Events:      result.Events,
		ArchivedAt:  strings.TrimSpace(result.ArchivedAt),
	}
	if record.ViewerURL == "" && record.InfoHash != "" {
		record.ViewerURL = "/posts/" + record.InfoHash
	}
	if err := os.MkdirAll(s.RoomDir(roomID), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(s.archivePath(roomID), body, 0o644)
}

func (s *LocalStore) LoadArchiveResult(roomID string) (*ArchiveRecord, error) {
	if s == nil {
		return nil, fmt.Errorf("local store is required")
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
	return &record, nil
}

func mergeRoomInfo(existing, incoming RoomInfo) RoomInfo {
	return RoomInfo{
		RoomID:      firstNonEmptyInfo(incoming.RoomID, existing.RoomID),
		Title:       firstNonEmptyInfo(incoming.Title, existing.Title),
		Creator:     firstNonEmptyInfo(incoming.Creator, existing.Creator),
		CreatedAt:   firstNonEmptyInfo(incoming.CreatedAt, existing.CreatedAt),
		NetworkID:   firstNonEmptyInfo(incoming.NetworkID, existing.NetworkID),
		Channel:     firstNonEmptyInfo(incoming.Channel, existing.Channel),
		Tags:        firstNonEmptySlice(incoming.Tags, existing.Tags),
		Description: firstNonEmptyInfo(incoming.Description, existing.Description),
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
