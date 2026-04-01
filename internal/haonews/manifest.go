package haonews

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anacrolix/torrent/metainfo"
)

const (
	historyManifestKind     = "manifest"
	historyManifestType     = "history"
	historyManifestAuthor   = "agent://haonews-sync/history-manifest"
	historyManifestPageSize = 200
)

type HistoryManifest struct {
	Protocol     string             `json:"protocol"`
	Type         string             `json:"type"`
	Project      string             `json:"project,omitempty"`
	NetworkID    string             `json:"network_id,omitempty"`
	GeneratedAt  string             `json:"generated_at"`
	Page         int                `json:"page,omitempty"`
	PageSize     int                `json:"page_size,omitempty"`
	TotalEntries int                `json:"total_entries,omitempty"`
	TotalPages   int                `json:"total_pages,omitempty"`
	Cursor       string             `json:"cursor,omitempty"`
	NextCursor   string             `json:"next_cursor,omitempty"`
	HasMore      bool               `json:"has_more,omitempty"`
	EntryCount   int                `json:"entry_count"`
	Entries      []SyncAnnouncement `json:"entries"`
}

type historyManifestState struct {
	Project     string `json:"project"`
	NetworkID   string `json:"network_id"`
	BodySHA256  string `json:"body_sha256"`
	InfoHash    string `json:"infohash"`
	ContentDir  string `json:"content_dir"`
	TorrentFile string `json:"torrent_file"`
}

func ensureHistoryManifests(store *Store, netCfg NetworkBootstrapConfig, listenAddrs []net.Addr, localPeerID string, rc *RedisClient) error {
	announcements, err := localAnnouncements(store)
	if err != nil {
		return err
	}
	grouped := map[string][]SyncAnnouncement{}
	for _, announcement := range announcements {
		announcement = normalizeAnnouncement(announcement)
		if announcement.InfoHash == "" || announcement.Ref == "" {
			continue
		}
		if strings.EqualFold(announcement.Kind, historyManifestKind) {
			continue
		}
		if netCfg.NetworkID != "" && announcement.NetworkID != "" && !strings.EqualFold(announcement.NetworkID, netCfg.NetworkID) {
			continue
		}
		project := strings.TrimSpace(announcement.Project)
		if project == "" {
			continue
		}
		if announcement.NetworkID == "" {
			announcement.NetworkID = netCfg.NetworkID
		}
		if strings.TrimSpace(localPeerID) != "" {
			announcement.LibP2PPeerID = strings.TrimSpace(localPeerID)
		}
		announcement.Ref = withPeerHints(announcement.Ref, listenAddrs, netCfg.LANPeers)
		_ = cacheSyncAnnouncement(context.Background(), rc, announcement)
		grouped[project] = append(grouped[project], announcement)
	}
	for project, entries := range grouped {
		if err := ensureHistoryManifestPages(store, project, netCfg.NetworkID, entries); err != nil {
			return err
		}
	}
	return nil
}

func ensureHistoryManifestPages(store *Store, project, networkID string, entries []SyncAnnouncement) error {
	if len(entries) == 0 {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].CreatedAt != entries[j].CreatedAt {
			return entries[i].CreatedAt > entries[j].CreatedAt
		}
		return strings.ToLower(strings.TrimSpace(entries[i].InfoHash)) < strings.ToLower(strings.TrimSpace(entries[j].InfoHash))
	})
	totalEntries := len(entries)
	totalPages := totalEntries / historyManifestPageSize
	if totalEntries%historyManifestPageSize != 0 {
		totalPages++
	}
	for page := 1; page <= totalPages; page++ {
		start := (page - 1) * historyManifestPageSize
		end := start + historyManifestPageSize
		if end > totalEntries {
			end = totalEntries
		}
		if err := ensureHistoryManifest(store, project, networkID, entries[start:end], page, totalEntries, totalPages); err != nil {
			return err
		}
	}
	return cleanupHistoryManifestStatePages(store, strings.TrimSpace(project), normalizeNetworkID(networkID), totalPages)
}

func ensureHistoryManifest(store *Store, project, networkID string, entries []SyncAnnouncement, page, totalEntries, totalPages int) error {
	if len(entries) == 0 {
		return nil
	}
	manifest := HistoryManifest{
		Protocol:     ProtocolVersion,
		Type:         historyManifestType,
		Project:      strings.TrimSpace(project),
		NetworkID:    normalizeNetworkID(networkID),
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		Page:         page,
		PageSize:     historyManifestPageSize,
		TotalEntries: totalEntries,
		TotalPages:   totalPages,
		Cursor:       strconv.Itoa(page),
		HasMore:      page < totalPages,
		EntryCount:   len(entries),
		Entries:      make([]SyncAnnouncement, 0, len(entries)),
	}
	if manifest.HasMore {
		manifest.NextCursor = strconv.Itoa(page + 1)
	}
	topicsSeen := map[string]struct{}{reservedTopicAll: {}}
	topics := []string{reservedTopicAll}
	for _, entry := range entries {
		entry.NetworkID = manifest.NetworkID
		entry = normalizeAnnouncement(entry)
		manifest.Entries = append(manifest.Entries, entry)
		for _, topic := range entry.Topics {
			key := strings.ToLower(strings.TrimSpace(topic))
			if key == "" {
				continue
			}
			if _, ok := topicsSeen[key]; ok {
				continue
			}
			topicsSeen[key] = struct{}{}
			topics = append(topics, topic)
		}
	}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	bodySHA := sha256.Sum256(body)
	bodyHash := hex.EncodeToString(bodySHA[:])
	statePath := historyManifestStatePath(store, manifest.Project, manifest.NetworkID, page)
	state, _ := loadHistoryManifestState(statePath)
	if state.BodySHA256 == bodyHash && state.ContentDir != "" && state.TorrentFile != "" {
		if _, err := os.Stat(state.ContentDir); err == nil {
			if _, err := os.Stat(state.TorrentFile); err == nil {
				return nil
			}
		}
	}
	result, err := PublishMessage(store, MessageInput{
		Kind:      historyManifestKind,
		Author:    historyManifestAuthor,
		Channel:   manifest.Project + "/history",
		Title:     historyManifestTitle(manifest.Project, page, totalPages),
		Body:      string(body),
		Tags:      []string{"history-manifest"},
		CreatedAt: time.Now().UTC(),
		Extensions: map[string]any{
			"project":                manifest.Project,
			"network_id":             manifest.NetworkID,
			"manifest_type":          historyManifestType,
			"entry_count":            manifest.EntryCount,
			"manifest_page":          manifest.Page,
			"manifest_page_size":     manifest.PageSize,
			"manifest_total_pages":   manifest.TotalPages,
			"manifest_total_entries": manifest.TotalEntries,
			"manifest_cursor":        manifest.Cursor,
			"manifest_next_cursor":   manifest.NextCursor,
			"manifest_has_more":      manifest.HasMore,
			"topics":                 topics,
		},
	})
	if err != nil {
		return err
	}
	if state.ContentDir != "" && state.ContentDir != result.ContentDir {
		_ = os.RemoveAll(state.ContentDir)
	}
	if state.TorrentFile != "" && state.TorrentFile != result.TorrentFile {
		_ = os.Remove(state.TorrentFile)
	}
	return writeHistoryManifestState(statePath, historyManifestState{
		Project:     manifest.Project,
		NetworkID:   manifest.NetworkID,
		BodySHA256:  bodyHash,
		InfoHash:    result.InfoHash,
		ContentDir:  result.ContentDir,
		TorrentFile: result.TorrentFile,
	})
}

func enqueueHistoryManifestRefs(store *Store, queuePath string, subscriptions SyncSubscriptions, networkID string, maxAdds int, shouldEnqueue func(SyncAnnouncement, SyncRef) bool, onEnqueue func(SyncAnnouncement, SyncRef)) (int, error) {
	entries, err := os.ReadDir(store.DataDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	added := 0
	dayCounts := localBundleDayCounts(store, "")
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(store.DataDir, entry.Name())
		msg, body, err := LoadMessage(dir)
		if err != nil || !isHistoryManifestMessage(msg) {
			continue
		}
		manifest, err := parseHistoryManifest(body, msg)
		if err != nil {
			continue
		}
		if networkID != "" && manifest.NetworkID != "" && !strings.EqualFold(manifest.NetworkID, networkID) {
			continue
		}
		for _, announcement := range manifest.Entries {
			announcement = normalizeAnnouncement(announcement)
			if announcement.NetworkID == "" {
				announcement.NetworkID = manifest.NetworkID
			}
			if networkID != "" && announcement.NetworkID != "" && !strings.EqualFold(announcement.NetworkID, networkID) {
				continue
			}
			if !matchesHistoryAnnouncement(announcement, subscriptions) {
				continue
			}
			ref, err := syncRefFromAnnouncement(announcement)
			if err != nil || ref.InfoHash == "" {
				continue
			}
			if shouldEnqueue != nil && !shouldEnqueue(announcement, ref) {
				continue
			}
			if hasLocalTorrent(store, ref.InfoHash) {
				continue
			}
			if !reserveDailyQuota(dayCounts, announcement.CreatedAt, subscriptions.MaxItemsPerDay) {
				continue
			}
			enqueued, err := enqueueSyncRef(queuePath, ref)
			if err != nil {
				return added, err
			}
			if onEnqueue != nil {
				onEnqueue(announcement, ref)
			}
			if enqueued {
				added++
				if maxAdds > 0 && added >= maxAdds {
					return added, nil
				}
			}
		}
	}
	return added, nil
}

func isHistoryManifestMessage(msg Message) bool {
	if !strings.EqualFold(strings.TrimSpace(msg.Kind), historyManifestKind) {
		return false
	}
	return strings.EqualFold(nestedString(msg.Extensions, "manifest_type"), historyManifestType)
}

func parseHistoryManifest(body string, msg Message) (HistoryManifest, error) {
	var manifest HistoryManifest
	if err := json.Unmarshal([]byte(body), &manifest); err != nil {
		return HistoryManifest{}, err
	}
	manifest.Protocol = strings.TrimSpace(manifest.Protocol)
	manifest.Type = strings.TrimSpace(manifest.Type)
	manifest.Project = strings.TrimSpace(manifest.Project)
	manifest.NetworkID = normalizeNetworkID(manifest.NetworkID)
	if manifest.Project == "" {
		manifest.Project = nestedString(msg.Extensions, "project")
	}
	if manifest.NetworkID == "" {
		manifest.NetworkID = nestedString(msg.Extensions, "network_id")
	}
	if manifest.Page <= 0 {
		manifest.Page = nestedInt(msg.Extensions, "manifest_page")
	}
	if manifest.Page <= 0 {
		manifest.Page = 1
	}
	if manifest.PageSize <= 0 {
		manifest.PageSize = nestedInt(msg.Extensions, "manifest_page_size")
	}
	if manifest.PageSize <= 0 {
		manifest.PageSize = historyManifestPageSize
	}
	if manifest.TotalPages <= 0 {
		manifest.TotalPages = nestedInt(msg.Extensions, "manifest_total_pages")
	}
	if manifest.TotalEntries <= 0 {
		manifest.TotalEntries = nestedInt(msg.Extensions, "manifest_total_entries")
	}
	if manifest.Cursor == "" {
		manifest.Cursor = nestedString(msg.Extensions, "manifest_cursor")
	}
	if manifest.Cursor == "" {
		manifest.Cursor = strconv.Itoa(manifest.Page)
	}
	if manifest.NextCursor == "" {
		manifest.NextCursor = nestedString(msg.Extensions, "manifest_next_cursor")
	}
	if !manifest.HasMore {
		manifest.HasMore = nestedBool(msg.Extensions, "manifest_has_more")
	}
	if !strings.EqualFold(manifest.Type, historyManifestType) {
		return HistoryManifest{}, errors.New("unsupported manifest type")
	}
	for index := range manifest.Entries {
		manifest.Entries[index].Project = manifest.Project
		if manifest.Entries[index].NetworkID == "" {
			manifest.Entries[index].NetworkID = manifest.NetworkID
		}
		manifest.Entries[index] = normalizeAnnouncement(manifest.Entries[index])
	}
	return manifest, nil
}

func historyManifestStatePath(store *Store, project, networkID string, page int) string {
	return filepath.Join(historyManifestStateDir(store, project, networkID), historyManifestStateFile(page))
}

func historyManifestStateDir(store *Store, project, networkID string) string {
	name := slugify(project)
	if name == "" {
		name = "project"
	}
	if networkID != "" {
		name += "-" + networkID[:12]
	}
	return filepath.Join(store.Root, "sync", "manifests", name)
}

func historyManifestStateFile(page int) string {
	if page <= 0 {
		page = 1
	}
	return fmt.Sprintf("page-%04d.json", page)
}

func loadHistoryManifestState(path string) (historyManifestState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return historyManifestState{}, err
	}
	var state historyManifestState
	if err := json.Unmarshal(data, &state); err != nil {
		return historyManifestState{}, err
	}
	return state, nil
}

func writeHistoryManifestState(path string, state historyManifestState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func cleanupHistoryManifestStatePages(store *Store, project, networkID string, keepPages int) error {
	dir := historyManifestStateDir(store, project, networkID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		page := historyManifestPageFromFile(entry.Name())
		if page <= 0 || page <= keepPages {
			continue
		}
		statePath := filepath.Join(dir, entry.Name())
		state, err := loadHistoryManifestState(statePath)
		if err == nil {
			if state.ContentDir != "" {
				_ = os.RemoveAll(state.ContentDir)
			}
			if state.TorrentFile != "" {
				_ = os.Remove(state.TorrentFile)
			}
		}
		_ = os.Remove(statePath)
	}
	return nil
}

func historyManifestPageFromFile(name string) int {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "page-")
	name = strings.TrimSuffix(name, ".json")
	value, err := strconv.Atoi(name)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func historyManifestTitle(project string, page, totalPages int) string {
	project = strings.TrimSpace(project)
	if project == "" {
		project = "hao.news"
	}
	if totalPages <= 1 || page <= 1 {
		return project + " history manifest"
	}
	return fmt.Sprintf("%s history manifest page %d", project, page)
}

func nestedInt(value map[string]any, key string) int {
	if value == nil {
		return 0
	}
	raw, ok := value[key]
	if !ok {
		return 0
	}
	switch v := raw.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return n
	default:
		return 0
	}
}

func nestedBool(value map[string]any, key string) bool {
	if value == nil {
		return false
	}
	raw, ok := value[key]
	if !ok {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(v))
		return parsed
	default:
		return false
	}
}

func syncRefFromAnnouncement(announcement SyncAnnouncement) (SyncRef, error) {
	if strings.TrimSpace(announcement.Ref) != "" {
		ref, err := ParseSyncRef(announcement.Ref)
		if err != nil {
			return SyncRef{}, err
		}
		ref.Magnet = withSourcePeerHint(ref.Magnet, announcement.SourceHost)
		ref.Magnet = withLibP2PPeerHint(ref.Magnet, announcement.LibP2PPeerID)
		ref.Raw = ref.Magnet
		ref.DirectPeerHint = strings.TrimSpace(announcement.LibP2PPeerID)
		return ref, nil
	}
	if strings.TrimSpace(announcement.Magnet) != "" {
		ref, err := ParseSyncRef(announcement.Magnet)
		if err != nil {
			return SyncRef{}, err
		}
		ref.Magnet = withSourcePeerHint(ref.Magnet, announcement.SourceHost)
		ref.Magnet = withLibP2PPeerHint(ref.Magnet, announcement.LibP2PPeerID)
		ref.Raw = ref.Magnet
		ref.DirectPeerHint = strings.TrimSpace(announcement.LibP2PPeerID)
		return ref, nil
	}
	ref, err := ParseSyncRef(announcement.InfoHash)
	if err != nil {
		return SyncRef{}, err
	}
	ref.Magnet = withSourcePeerHint(ref.Magnet, announcement.SourceHost)
	ref.Magnet = withLibP2PPeerHint(ref.Magnet, announcement.LibP2PPeerID)
	ref.Raw = ref.Magnet
	ref.DirectPeerHint = strings.TrimSpace(announcement.LibP2PPeerID)
	return ref, nil
}

func hasLocalTorrent(store *Store, infoHash string) bool {
	if strings.TrimSpace(infoHash) == "" {
		return false
	}
	_, err := store.ExistingTorrentPath(infoHash)
	return err == nil
}

func hasCompleteLocalBundle(store *Store, infoHash string) bool {
	infoHash = strings.TrimSpace(strings.ToLower(infoHash))
	if infoHash == "" {
		return false
	}
	torrentPath, err := store.ExistingTorrentPath(infoHash)
	if err != nil {
		return false
	}
	mi, err := metainfo.LoadFromFile(torrentPath)
	if err != nil {
		return false
	}
	info, err := mi.UnmarshalInfo()
	if err != nil {
		return false
	}
	contentDir := filepath.Join(store.DataDir, info.BestName())
	_, _, err = LoadMessage(contentDir)
	return err == nil
}
