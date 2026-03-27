package newsplugin

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	haonews "hao.news/internal/haonews"
)

const defaultHistoryListPageSize = 200

func (a *App) LatestHistoryListPayload() (HistoryManifestAPIResponse, error) {
	return a.HistoryListPayload("", defaultHistoryListPageSize)
}

func (a *App) HistoryListPayload(cursor string, pageSize int) (HistoryManifestAPIResponse, error) {
	index, err := a.index()
	if err != nil {
		return HistoryManifestAPIResponse{}, err
	}
	if len(index.Bundles) == 0 {
		return HistoryManifestAPIResponse{}, os.ErrNotExist
	}
	var networkID string
	var localPeerID string
	var sourceHost string
	if syncStatus, err := a.syncRuntimeStatus(); err == nil {
		networkID = strings.TrimSpace(syncStatus.NetworkID)
		localPeerID = strings.TrimSpace(syncStatus.LibP2P.PeerID)
		if netCfg, cfgErr := a.networkBootstrap(); cfgErr == nil {
			sourceHost = strings.TrimSpace(PreferredAdvertiseHostForConfig(syncStatus, "", netCfg))
		}
	}
	page, pageSize := normalizeHistoryPage(cursor, pageSize)
	bundles := make([]Bundle, len(index.Bundles))
	copy(bundles, index.Bundles)
	sort.Slice(bundles, func(i, j int) bool {
		if !bundles[i].CreatedAt.Equal(bundles[j].CreatedAt) {
			return bundles[i].CreatedAt.After(bundles[j].CreatedAt)
		}
		return strings.ToLower(strings.TrimSpace(bundles[i].InfoHash)) < strings.ToLower(strings.TrimSpace(bundles[j].InfoHash))
	})
	totalEntries := len(bundles)
	totalPages := totalEntries / pageSize
	if totalEntries%pageSize != 0 {
		totalPages++
	}
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * pageSize
	if start > totalEntries {
		start = totalEntries
	}
	end := start + pageSize
	if end > totalEntries {
		end = totalEntries
	}
	entries := make([]HistoryManifestEntry, 0, end-start)
	for _, bundle := range bundles[start:end] {
		ref := haonews.CanonicalSyncRef(strings.ToLower(strings.TrimSpace(bundle.InfoHash)), strings.TrimSpace(bundle.Message.Title))
		ref = haonews.WithSourcePeerHintForSyncRef(ref, sourceHost)
		ref = haonews.WithLibP2PPeerHintForSyncRef(ref, localPeerID)
		originAuthor, originAgentID, originKeyType, originPublicKey, originSigned := originSummary(bundle.Message.Origin)
		delegated, parentAgentID, parentKeyType, parentPublicKey := delegationSummary(bundle.Delegation)
		entries = append(entries, HistoryManifestEntry{
			Protocol:          "haonews-sync/0.1",
			InfoHash:          strings.ToLower(strings.TrimSpace(bundle.InfoHash)),
			Ref:               ref,
			Magnet:            strings.TrimSpace(bundle.Magnet),
			LibP2PPeerID:      localPeerID,
			SourceHost:        sourceHost,
			SizeBytes:         bundle.SizeBytes,
			Kind:              strings.TrimSpace(bundle.Message.Kind),
			Channel:           strings.TrimSpace(bundle.Message.Channel),
			Title:             strings.TrimSpace(bundle.Message.Title),
			Author:            strings.TrimSpace(bundle.Message.Author),
			CreatedAt:         strings.TrimSpace(bundle.Message.CreatedAt),
			Project:           a.project,
			NetworkID:         networkID,
			Topics:            stringSlice(bundle.Message.Extensions["topics"]),
			Tags:              append([]string(nil), bundle.Message.Tags...),
			OriginAuthor:      originAuthor,
			OriginAgentID:     originAgentID,
			OriginKeyType:     originKeyType,
			OriginPublicKey:   originPublicKey,
			OriginSigned:      originSigned,
			Delegated:         delegated,
			ParentAgentID:     parentAgentID,
			ParentKeyType:     parentKeyType,
			ParentPublicKey:   parentPublicKey,
			SharedByLocalNode: bundle.SharedByLocalNode,
		})
	}
	nextCursor := ""
	hasMore := end < totalEntries
	if hasMore {
		nextCursor = strconv.Itoa(page + 1)
	}
	return HistoryManifestAPIResponse{
		Project:      a.project,
		Version:      a.version,
		NetworkID:    networkID,
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		Page:         page,
		PageSize:     pageSize,
		TotalEntries: totalEntries,
		TotalPages:   totalPages,
		Cursor:       strconv.Itoa(page),
		NextCursor:   nextCursor,
		HasMore:      hasMore,
		EntryCount:   len(entries),
		Entries:      entries,
	}, nil
}

func normalizeHistoryPage(cursor string, pageSize int) (int, int) {
	if pageSize <= 0 {
		pageSize = defaultHistoryListPageSize
	}
	if pageSize > 1000 {
		pageSize = 1000
	}
	page := 1
	if value, err := strconv.Atoi(strings.TrimSpace(cursor)); err == nil && value > 0 {
		page = value
	}
	return page, pageSize
}

func BuildArchiveDays(index Index) []ArchiveDay {
	dayMap := make(map[string]*ArchiveDay)
	for _, bundle := range index.Bundles {
		day := defaultDisplayDate(bundle.CreatedAt)
		item := dayMap[day]
		if item == nil {
			item = &ArchiveDay{
				Date: day,
				URL:  "/archive/" + day,
			}
			dayMap[day] = item
		}
		switch bundle.Message.Kind {
		case "post":
			item.StoryCount++
		case "reply":
			item.ReplyCount++
		case "reaction":
			item.ReactionCount++
		}
	}
	out := make([]ArchiveDay, 0, len(dayMap))
	for _, item := range dayMap {
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Date > out[j].Date
	})
	return out
}

func MarkArchiveDayActive(days []ArchiveDay, active string) []ArchiveDay {
	out := make([]ArchiveDay, 0, len(days))
	for _, day := range days {
		day.Active = day.Date == active
		out = append(out, day)
	}
	return out
}

func HasArchiveDay(days []ArchiveDay, target string) bool {
	for _, day := range days {
		if day.Date == target {
			return true
		}
	}
	return false
}

func BuildArchiveSummaryStats(days []ArchiveDay, bundles int) []SummaryStat {
	return []SummaryStat{
		{Label: "Archive days", Value: strconv.Itoa(len(days))},
		{Label: "Mirrored bundles", Value: strconv.Itoa(bundles)},
	}
}

func BuildArchiveDayStats(entries []ArchiveEntry) []SummaryStat {
	stories := 0
	replies := 0
	reactions := 0
	for _, entry := range entries {
		switch entry.Kind {
		case "post":
			stories++
		case "reply":
			replies++
		case "reaction":
			reactions++
		}
	}
	return []SummaryStat{
		{Label: "Entries", Value: strconv.Itoa(len(entries))},
		{Label: "Stories", Value: strconv.Itoa(stories)},
		{Label: "Replies", Value: strconv.Itoa(replies)},
		{Label: "Reactions", Value: strconv.Itoa(reactions)},
	}
}

func BuildArchiveEntries(index Index, day string) []ArchiveEntry {
	entries := make([]ArchiveEntry, 0)
	for _, bundle := range index.Bundles {
		if bundle.ArchiveMD == "" || defaultDisplayDate(bundle.CreatedAt) != day {
			continue
		}
		entries = append(entries, archiveEntry(bundle))
	}
	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].CreatedAt.Equal(entries[j].CreatedAt) {
			return entries[i].CreatedAt.After(entries[j].CreatedAt)
		}
		return entries[i].InfoHash < entries[j].InfoHash
	})
	return entries
}

func FindArchiveEntry(index Index, infoHash string) (ArchiveEntry, bool) {
	infoHash = strings.ToLower(infoHash)
	for _, bundle := range index.Bundles {
		if strings.ToLower(bundle.InfoHash) == infoHash && bundle.ArchiveMD != "" {
			return archiveEntry(bundle), true
		}
	}
	return ArchiveEntry{}, false
}

func archiveEntry(bundle Bundle) ArchiveEntry {
	title := strings.TrimSpace(bundle.Message.Title)
	if title == "" {
		title = strings.ToUpper(bundle.Message.Kind) + " " + bundle.InfoHash
	}
	day := defaultDisplayDate(bundle.CreatedAt)
	return ArchiveEntry{
		InfoHash:   bundle.InfoHash,
		Kind:       bundle.Message.Kind,
		Title:      title,
		Author:     bundle.Message.Author,
		CreatedAt:  bundle.CreatedAt,
		ArchiveMD:  bundle.ArchiveMD,
		Day:        day,
		ThreadURL:  bundleThreadURL(bundle),
		ViewerURL:  "/archive/messages/" + bundle.InfoHash,
		RawURL:     "/archive/raw/" + bundle.InfoHash,
		Channel:    bundle.Message.Channel,
		SourceName: nestedString(bundle.Message.Extensions, "source", "name"),
	}
}

func bundleThreadURL(bundle Bundle) string {
	switch bundle.Message.Kind {
	case "post":
		return "/posts/" + bundle.InfoHash
	case "reply":
		if bundle.Message.ReplyTo != nil && bundle.Message.ReplyTo.InfoHash != "" {
			return "/posts/" + bundle.Message.ReplyTo.InfoHash
		}
	case "reaction":
		if infoHash := nestedString(bundle.Message.Extensions, "subject", "infohash"); infoHash != "" {
			return "/posts/" + infoHash
		}
	}
	return ""
}
