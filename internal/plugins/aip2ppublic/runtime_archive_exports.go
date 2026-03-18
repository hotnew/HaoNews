package newsplugin

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (a *App) LatestHistoryListPayload() (HistoryManifestAPIResponse, error) {
	index, err := a.index()
	if err != nil {
		return HistoryManifestAPIResponse{}, err
	}
	if len(index.Bundles) == 0 {
		return HistoryManifestAPIResponse{}, os.ErrNotExist
	}
	var networkID string
	if syncStatus, err := a.syncRuntimeStatus(); err == nil {
		networkID = strings.TrimSpace(syncStatus.NetworkID)
	}
	entries := make([]HistoryManifestEntry, 0, len(index.Bundles))
	for _, bundle := range index.Bundles {
		originAuthor, originAgentID, originKeyType, originPublicKey, originSigned := originSummary(bundle.Message.Origin)
		delegated, parentAgentID, parentKeyType, parentPublicKey := delegationSummary(bundle.Delegation)
		entries = append(entries, HistoryManifestEntry{
			Protocol:          "aip2p-sync/0.1",
			InfoHash:          strings.ToLower(strings.TrimSpace(bundle.InfoHash)),
			Magnet:            strings.TrimSpace(bundle.Magnet),
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
	return HistoryManifestAPIResponse{
		Project:     a.project,
		Version:     a.version,
		NetworkID:   networkID,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		EntryCount:  len(entries),
		Entries:     entries,
	}, nil
}

func BuildArchiveDays(index Index) []ArchiveDay {
	dayMap := make(map[string]*ArchiveDay)
	for _, bundle := range index.Bundles {
		day := bundle.CreatedAt.UTC().Format("2006-01-02")
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
		if bundle.ArchiveMD == "" || bundle.CreatedAt.UTC().Format("2006-01-02") != day {
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
	day := bundle.CreatedAt.UTC().Format("2006-01-02")
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
