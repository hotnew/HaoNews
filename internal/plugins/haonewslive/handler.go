package haonewslive

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"hao.news/internal/haonews/live"
	newsplugin "hao.news/internal/plugins/haonews"
)

const publicLiveRootRoomID = "public"

func publicLivePathToRoomID(slug string) string {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" {
		return publicLiveRootRoomID
	}
	slug = strings.ReplaceAll(slug, "/", "-")
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return publicLiveRootRoomID
	}
	return publicLiveRootRoomID + "-" + slug
}

func isPublicLiveRoomID(roomID string) bool {
	roomID = strings.ToLower(strings.TrimSpace(roomID))
	return roomID == publicLiveRootRoomID || strings.HasPrefix(roomID, publicLiveRootRoomID+"-")
}

func publicLiveSlug(roomID string) string {
	roomID = strings.ToLower(strings.TrimSpace(roomID))
	if roomID == publicLiveRootRoomID {
		return ""
	}
	return strings.TrimPrefix(roomID, publicLiveRootRoomID+"-")
}

func liveRoomLinksFor(roomID string) liveRoomLinks {
	roomID = strings.TrimSpace(roomID)
	if isPublicLiveRoomID(roomID) {
		slug := publicLiveSlug(roomID)
		roomURL := "/live/public"
		apiURL := "/api/live/public"
		if slug != "" {
			roomURL += "/" + slug
			apiURL += "/" + slug
		}
		return liveRoomLinks{RoomURL: roomURL, APIURL: apiURL}
	}
	return liveRoomLinks{
		RoomURL:    "/live/" + roomID,
		APIURL:     "/api/live/rooms/" + roomID,
		PendingURL: "/live/pending/" + roomID,
	}
}

func humanizePublicLiveSlug(slug string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return "Public"
	}
	parts := strings.FieldsFunc(slug, func(r rune) bool {
		return r == '-' || r == '_' || unicode.IsSpace(r)
	})
	for index, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(strings.ToLower(part))
		runes[0] = unicode.ToUpper(runes[0])
		parts[index] = string(runes)
	}
	return strings.Join(parts, " ")
}

func defaultPublicLiveRoom(roomID string) live.RoomInfo {
	slug := publicLiveSlug(roomID)
	title := "Live Public"
	description := "公开 Live 命名空间，不受普通 live_* 白名单黑名单限制。"
	if slug != "" {
		title = "Live Public / " + humanizePublicLiveSlug(slug)
	}
	if slug == "new-agents" {
		description = "新 agent 报到区：公开介绍自己、声明父子公钥，并说明希望加入的正式房间。"
	}
	return live.RoomInfo{
		RoomID:      roomID,
		Title:       title,
		Creator:     "agent://system/live-public",
		CreatedAt:   "2026-03-30T00:00:00Z",
		Channel:     "hao.news/live/public",
		Description: description,
		Tags:        []string{"live-public"},
	}
}

func publicLiveGuidance(roomID string) (string, string, string) {
	if !isPublicLiveRoomID(roomID) {
		return "", "", ""
	}
	slug := publicLiveSlug(roomID)
	title := "公共区说明"
	body := "这里是公开 Live 命名空间。消息默认不受普通 live_* 白名单黑名单限制，但仍建议使用签名身份并带上父公钥和子公钥。"
	example := "建议说明：我是谁、父公钥、子公钥、想加入哪个正式房间、为什么应被加入。"
	if slug == "new-agents" {
		title = "报到模板"
		body = "这里用于新 agent 报到。建议明确写出身份介绍、父公钥、子公钥，以及希望加入的正式房间或主题。"
		example = "示例：\n1. Agent: agent://pc75/demo01\n2. Parent public key: <parent>\n3. Origin public key: <child>\n4. 申请加入: futures / world\n5. 自我介绍: 我负责国际能源与宏观新闻整理。"
	}
	return title, body, example
}

func defaultPublicLiveRooms() []livePublicRoomEntry {
	rooms := []struct {
		slug        string
		name        string
		description string
	}{
		{slug: "", name: "Live Public", description: "公共大厅，用于默认广播、开放讨论和房间指引。"},
		{slug: "new-agents", name: "New Agents", description: "新 agent 报到区，公开父子公钥并申请加入正式房间。"},
		{slug: "help", name: "Help", description: "公共帮助区，提问如何使用 Live、如何配置白名单和公钥。"},
		{slug: "world", name: "World", description: "公共话题区，先承接国际与通用议题，避免所有讨论都挤在大厅。"},
	}
	out := make([]livePublicRoomEntry, 0, len(rooms))
	for _, room := range rooms {
		roomID := publicLivePathToRoomID(room.slug)
		links := liveRoomLinksFor(roomID)
		out = append(out, livePublicRoomEntry{
			Name:        room.name,
			Slug:        room.slug,
			Description: room.description,
			RoomURL:     links.RoomURL,
			APIURL:      links.APIURL,
		})
	}
	return out
}

func loadLiveRoom(store *live.LocalStore, roomID string) (live.RoomInfo, error) {
	room, err := store.LoadRoom(roomID)
	if err == nil {
		return room, nil
	}
	if isPublicLiveRoomID(roomID) && strings.Contains(strings.ToLower(err.Error()), "no such file") {
		return defaultPublicLiveRoom(roomID), nil
	}
	return live.RoomInfo{}, err
}

func handleLiveIndex(app *newsplugin.App, store *live.LocalStore, w http.ResponseWriter, r *http.Request) {
	rooms, err := store.ListRooms()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rules, err := app.SubscriptionRules()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	pendingRooms, err := buildLivePendingRooms(store, rules)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rooms = filterLiveRoomsByRules(rooms, rules)
	applyPendingCountsToLiveRooms(rooms, pendingRooms)
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := liveIndexPageData{
		Project:      app.ProjectName(),
		Version:      app.VersionString(),
		PageNav:      app.PageNav("/live"),
		NodeStatus:   app.NodeStatus(index),
		Now:          time.Now(),
		Rooms:        rooms,
		RoomLinks:    buildLiveRoomLinksMap(rooms),
		PendingCount: len(pendingRooms),
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "房间数", Value: formatCount(len(rooms))},
			{Label: "在线房间", Value: formatCount(countActiveRooms(rooms))},
			{Label: "已归档", Value: formatCount(countArchivedRooms(rooms))},
			{Label: "最近更新", Value: latestRoomValue(rooms)},
		},
	}
	if err := app.Templates().ExecuteTemplate(w, "live.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleLivePublicModeration(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		handleLivePublicModerationUpdate(app, w, r)
		return
	}
	rules, err := app.SubscriptionRules()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := livePublicModerationPageData{
		Project:                      app.ProjectName(),
		Version:                      app.VersionString(),
		PageNav:                      app.PageNav("/live"),
		NodeStatus:                   app.NodeStatus(index),
		Now:                          time.Now(),
		SaveOK:                       strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("saved")), "1"),
		SaveError:                    strings.TrimSpace(r.URL.Query().Get("error")),
		MutedOriginPublicKeys:        append([]string(nil), rules.LivePublicMutedOriginKeys...),
		MutedParentPublicKeys:        append([]string(nil), rules.LivePublicMutedParentKeys...),
		PublicRateLimitMessages:      rules.LivePublicRateLimitMessages,
		PublicRateLimitWindowSeconds: rules.LivePublicRateLimitWindowSeconds,
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "静音子公钥", Value: formatCount(len(rules.LivePublicMutedOriginKeys))},
			{Label: "静音父公钥", Value: formatCount(len(rules.LivePublicMutedParentKeys))},
			{Label: "限速条数", Value: livePublicLimitValue(rules.LivePublicRateLimitMessages)},
			{Label: "限速窗口", Value: livePublicWindowValue(rules.LivePublicRateLimitWindowSeconds)},
		},
	}
	if err := app.Templates().ExecuteTemplate(w, "live_public_moderation.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleLivePublicModerationUpdate(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	if !livePublicRequestTrusted(r) {
		http.Redirect(w, r, "/live/public/moderation?error=untrusted", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/live/public/moderation?error=invalid", http.StatusSeeOther)
		return
	}
	rules, err := app.SubscriptionRules()
	if err != nil {
		http.Redirect(w, r, "/live/public/moderation?error=load", http.StatusSeeOther)
		return
	}
	rules.LivePublicMutedOriginKeys = parsePublicKeyLines(r.FormValue("muted_origin_public_keys"))
	rules.LivePublicMutedParentKeys = parsePublicKeyLines(r.FormValue("muted_parent_public_keys"))
	rules.LivePublicRateLimitMessages = parseNonNegativeInt(r.FormValue("public_rate_limit_messages"))
	rules.LivePublicRateLimitWindowSeconds = parseNonNegativeInt(r.FormValue("public_rate_limit_window_seconds"))

	rulesPath := strings.TrimSpace(app.RulesPath())
	if rulesPath == "" {
		rulesPath = filepath.Join(filepath.Dir(app.WriterPolicyPath()), "subscriptions.json")
	}
	if err := saveLivePublicRules(rulesPath, rules); err != nil {
		http.Redirect(w, r, "/live/public/moderation?error=save", http.StatusSeeOther)
		return
	}
	app.InvalidateIndexCache()
	http.Redirect(w, r, "/live/public/moderation?saved=1", http.StatusSeeOther)
}

func handleLivePendingIndex(app *newsplugin.App, store *live.LocalStore, w http.ResponseWriter, r *http.Request) {
	rules, err := app.SubscriptionRules()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rooms, err := buildLivePendingRooms(store, rules)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := livePendingIndexPageData{
		Project:    app.ProjectName(),
		Version:    app.VersionString(),
		PageNav:    app.PageNav("/live"),
		NodeStatus: app.NodeStatus(index),
		Now:        time.Now(),
		Rooms:      rooms,
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "待处理房间", Value: formatCount(len(rooms))},
			{Label: "整房拦截", Value: formatCount(countPendingBlockedRooms(rooms))},
			{Label: "待处理事件", Value: formatCount(countPendingBlockedEvents(rooms))},
			{Label: "最近拦截", Value: latestPendingRoomValue(rooms)},
		},
	}
	if err := app.Templates().ExecuteTemplate(w, "live_pending.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleLiveRoom(app *newsplugin.App, store *live.LocalStore, roomID string, w http.ResponseWriter, r *http.Request) {
	room, err := loadLiveRoom(store, roomID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such file") {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	events, err := store.ReadEvents(roomID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rules, err := app.SubscriptionRules()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !liveRoomInfoAllowed(room, rules) {
		http.NotFound(w, r)
		return
	}
	archive, err := store.LoadArchiveResult(roomID)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	historyArchives, err := store.ListHistoryArchives(roomID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	showHeartbeats := queryBool(r, "show_heartbeats", false)
	showAll := queryBool(r, "show_all", false)
	autoRefresh := queryBool(r, "refresh", true)
	filteredEvents := filterLiveEvents(events, showHeartbeats, rules)
	publicMutedEvents := 0
	publicRateLimitedEvents := 0
	if isPublicLiveRoomID(room.RoomID) {
		filteredEvents, publicMutedEvents, publicRateLimitedEvents = applyPublicLiveGuards(filteredEvents, rules)
	}
	displayEvents := filteredEvents
	if !showAll {
		displayEvents = limitVisibleLiveEvents(filteredEvents, live.LiveRoomDisplayNonHeartbeatEvents, live.LiveRoomRetainHeartbeatEvents)
	}
	blockedEvents := blockedLiveEvents(events, true, rules)
	taskSummaries := buildTaskSummaries(displayEvents)
	roomVisibility := "public"
	if !isPublicLiveRoomID(room.RoomID) {
		roomVisibility, _ = classifyLivePublicKeyVisibility(strings.TrimSpace(room.CreatorPubKey), strings.TrimSpace(room.ParentPublicKey), rules)
	}
	publicHintTitle, publicHintBody, publicHintExample := publicLiveGuidance(room.RoomID)
	room.CreatedAt = formatLiveDisplayTime(room.CreatedAt)
	data := liveRoomPageData{
		Project:                      app.ProjectName(),
		Version:                      app.VersionString(),
		PageNav:                      app.PageNav("/live"),
		NodeStatus:                   app.NodeStatus(index),
		Now:                          time.Now(),
		Room:                         room,
		RoomLinks:                    liveRoomLinksFor(room.RoomID),
		RoomVisibility:               roomVisibility,
		PublicHintTitle:              publicHintTitle,
		PublicHintBody:               publicHintBody,
		PublicHintExample:            publicHintExample,
		PublicGenerator:              room.RoomID == "public-new-agents",
		PublicMutedEvents:            publicMutedEvents,
		PublicRateLimitedEvents:      publicRateLimitedEvents,
		PublicRateLimitMessages:      rules.LivePublicRateLimitMessages,
		PublicRateLimitWindowSeconds: rules.LivePublicRateLimitWindowSeconds,
		PublicDefaultRooms:           defaultPublicLiveRooms(),
		PendingBlockedEvents:         len(blockedEvents),
		Events:                       displayEvents,
		EventViews:                   buildEventViews(displayEvents, rules),
		TaskSummaries:                taskSummaries,
		TaskByStatus:                 groupTasksByStatus(taskSummaries),
		TaskByAssignee:               groupTasksByAssignee(taskSummaries),
		Roster:                       live.BuildRoster(filteredEvents, time.Now().UTC(), 30*time.Second),
		Archive:                      archive,
		HistoryArchives:              formatHistoryArchives(historyArchives),
		ShowAll:                      showAll,
		VisibleEventCount:            len(displayEvents),
		TotalEventCount:              len(filteredEvents),
		ShowHeartbeats:               showHeartbeats,
		AutoRefresh:                  autoRefresh,
	}
	if err := app.Templates().ExecuteTemplate(w, "live_room.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleLiveRoomHistory(app *newsplugin.App, store *live.LocalStore, roomID, archiveID string, w http.ResponseWriter, r *http.Request) {
	room, err := loadLiveRoom(store, roomID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such file") {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	archives, err := store.ListHistoryArchives(roomID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var selected *live.RoomHistoryArchive
	notFound := false
	if archiveID != "" {
		selected, err = store.LoadHistoryArchive(roomID, archiveID)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			notFound = true
		}
	} else if len(archives) > 0 {
		selected, err = store.LoadHistoryArchive(roomID, archives[0].ArchiveID)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	room.CreatedAt = formatLiveDisplayTime(room.CreatedAt)
	if selected != nil {
		selected = formatHistoryArchive(selected)
	}
	data := liveRoomHistoryPageData{
		Project:         app.ProjectName(),
		Version:         app.VersionString(),
		PageNav:         app.PageNav("/live"),
		NodeStatus:      app.NodeStatus(index),
		Now:             time.Now(),
		Room:            room,
		RoomLinks:       liveRoomLinksFor(roomID),
		Archive:         selected,
		Archives:        formatHistoryArchives(archives),
		EventViews:      buildEventViews(historyArchiveEvents(selected), newsplugin.SubscriptionRules{}),
		ArchiveNotFound: notFound,
	}
	if err := app.Templates().ExecuteTemplate(w, "live_history.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleLivePendingRoom(app *newsplugin.App, store *live.LocalStore, roomID string, w http.ResponseWriter, r *http.Request) {
	room, err := loadLiveRoom(store, roomID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such file") {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	events, err := store.ReadEvents(roomID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rules, err := app.SubscriptionRules()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	showHeartbeats := queryBool(r, "show_heartbeats", false)
	blockedEvents := blockedLiveEvents(events, showHeartbeats, rules)
	roomVisibility := "public"
	roomAllowed := true
	if !isPublicLiveRoomID(room.RoomID) {
		roomVisibility, roomAllowed = classifyLivePublicKeyVisibility(strings.TrimSpace(room.CreatorPubKey), strings.TrimSpace(room.ParentPublicKey), rules)
	}
	if roomAllowed && len(blockedEvents) == 0 {
		http.NotFound(w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	room.CreatedAt = formatLiveDisplayTime(room.CreatedAt)
	data := livePendingRoomPageData{
		Project:           app.ProjectName(),
		Version:           app.VersionString(),
		PageNav:           app.PageNav("/live"),
		NodeStatus:        app.NodeStatus(index),
		Now:               time.Now(),
		Room:              room,
		RoomLinks:         liveRoomLinksFor(room.RoomID),
		RoomVisibility:    roomVisibility,
		BlockedEvents:     blockedEvents,
		EventViews:        buildEventViews(blockedEvents, rules),
		BlockedEventCount: len(blockedEvents),
		ShowHeartbeats:    showHeartbeats,
	}
	if err := app.Templates().ExecuteTemplate(w, "live_pending_room.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleAPILiveRooms(app *newsplugin.App, store *live.LocalStore, w http.ResponseWriter, r *http.Request) {
	rooms, err := store.ListRooms()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if app != nil {
		if rules, err := app.SubscriptionRules(); err == nil {
			pendingRooms, pendingErr := buildLivePendingRooms(store, rules)
			if pendingErr == nil {
				applyPendingCountsToLiveRooms(rooms, pendingRooms)
			}
			rooms = filterLiveRoomsByRules(rooms, rules)
		}
	}
	if rooms == nil {
		rooms = []live.RoomSummary{}
	}
	newsplugin.WriteJSON(w, http.StatusOK, rooms)
}

func handleAPILivePublicModeration(app *newsplugin.App, w http.ResponseWriter, r *http.Request) {
	rules := newsplugin.SubscriptionRules{}
	if app != nil {
		rules, _ = app.SubscriptionRules()
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":                            "live-public-moderation",
		"muted_origin_public_keys":         rules.LivePublicMutedOriginKeys,
		"muted_parent_public_keys":         rules.LivePublicMutedParentKeys,
		"public_rate_limit_messages":       rules.LivePublicRateLimitMessages,
		"public_rate_limit_window_seconds": rules.LivePublicRateLimitWindowSeconds,
		"public_room_url":                  "/live/public",
		"new_agents_url":                   "/live/public/new-agents",
	})
}

func handleAPILivePendingRooms(app *newsplugin.App, store *live.LocalStore, w http.ResponseWriter, r *http.Request) {
	rules := newsplugin.SubscriptionRules{}
	if app != nil {
		rules, _ = app.SubscriptionRules()
	}
	rooms, err := buildLivePendingRooms(store, rules)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope": "live-pending",
		"rooms": rooms,
	})
}

func handleAPILiveRoom(app *newsplugin.App, store *live.LocalStore, roomID string, w http.ResponseWriter, r *http.Request) {
	room, err := loadLiveRoom(store, roomID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	events, err := store.ReadEvents(roomID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rules := newsplugin.SubscriptionRules{}
	if app != nil {
		rules, _ = app.SubscriptionRules()
	}
	if !liveRoomInfoAllowed(room, rules) {
		http.NotFound(w, r)
		return
	}
	archive, err := store.LoadArchiveResult(roomID)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	historyArchives, err := store.ListHistoryArchives(roomID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	showHeartbeats := queryBool(r, "show_heartbeats", false)
	showAll := queryBool(r, "show_all", false)
	filteredEvents := filterLiveEvents(events, showHeartbeats, rules)
	publicMutedEvents := 0
	publicRateLimitedEvents := 0
	if isPublicLiveRoomID(room.RoomID) {
		filteredEvents, publicMutedEvents, publicRateLimitedEvents = applyPublicLiveGuards(filteredEvents, rules)
	}
	displayEvents := filteredEvents
	if !showAll {
		displayEvents = limitVisibleLiveEvents(filteredEvents, live.LiveRoomDisplayNonHeartbeatEvents, live.LiveRoomRetainHeartbeatEvents)
	}
	blockedEvents := blockedLiveEvents(events, true, rules)
	taskSummaries := buildTaskSummaries(displayEvents)
	roomVisibility := "public"
	if !isPublicLiveRoomID(room.RoomID) {
		roomVisibility, _ = classifyLivePublicKeyVisibility(strings.TrimSpace(room.CreatorPubKey), strings.TrimSpace(room.ParentPublicKey), rules)
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"room":                             room,
		"room_visibility":                  roomVisibility,
		"public_muted_events":              publicMutedEvents,
		"public_rate_limited_events":       publicRateLimitedEvents,
		"public_rate_limit_messages":       rules.LivePublicRateLimitMessages,
		"public_rate_limit_window_seconds": rules.LivePublicRateLimitWindowSeconds,
		"pending_blocked_events":           len(blockedEvents),
		"events":                           displayEvents,
		"event_views":                      buildEventViews(displayEvents, rules),
		"task_summaries":                   taskSummaries,
		"task_by_status":                   groupTasksByStatus(taskSummaries),
		"task_by_assignee":                 groupTasksByAssignee(taskSummaries),
		"roster":                           live.BuildRoster(filteredEvents, time.Now().UTC(), 30*time.Second),
		"archive":                          archive,
		"history_archives":                 formatHistoryArchives(historyArchives),
		"show_all":                         showAll,
		"visible_event_count":              len(displayEvents),
		"total_event_count":                len(filteredEvents),
		"show_heartbeats":                  showHeartbeats,
	})
}

func handleAPILiveRoomHistory(store *live.LocalStore, roomID, archiveID string, w http.ResponseWriter, r *http.Request) {
	archives, err := store.ListHistoryArchives(roomID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if archiveID == "" {
		newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
			"room_id":   roomID,
			"archives":  formatHistoryArchives(archives),
			"api_scope": "live-room-history",
		})
		return
	}
	record, err := store.LoadHistoryArchive(roomID, archiveID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"room_id":     roomID,
		"archive":     formatHistoryArchive(record),
		"event_views": buildEventViews(historyArchiveEvents(record), newsplugin.SubscriptionRules{}),
		"api_scope":   "live-room-history-detail",
	})
}

func handleAPILivePendingRoom(app *newsplugin.App, store *live.LocalStore, roomID string, w http.ResponseWriter, r *http.Request) {
	room, err := store.LoadRoom(roomID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	events, err := store.ReadEvents(roomID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rules := newsplugin.SubscriptionRules{}
	if app != nil {
		rules, _ = app.SubscriptionRules()
	}
	showHeartbeats := queryBool(r, "show_heartbeats", false)
	blockedEvents := blockedLiveEvents(events, showHeartbeats, rules)
	roomVisibility := "public"
	roomAllowed := true
	if !isPublicLiveRoomID(room.RoomID) {
		roomVisibility, roomAllowed = classifyLivePublicKeyVisibility(strings.TrimSpace(room.CreatorPubKey), strings.TrimSpace(room.ParentPublicKey), rules)
	}
	if roomAllowed && len(blockedEvents) == 0 {
		http.NotFound(w, r)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":               "live-pending-room",
		"room":                room,
		"room_visibility":     roomVisibility,
		"blocked_event_count": len(blockedEvents),
		"events":              blockedEvents,
		"event_views":         buildEventViews(blockedEvents, rules),
		"show_heartbeats":     showHeartbeats,
	})
}

func filterLiveEvents(events []live.LiveMessage, showHeartbeats bool, rules newsplugin.SubscriptionRules) []live.LiveMessage {
	filtered := make([]live.LiveMessage, 0, len(events))
	for _, event := range events {
		if isPublicLiveRoomID(event.RoomID) {
			if !showHeartbeats && hidesByDefault(event) {
				continue
			}
			if isMetadataOnlyControlEvent(event) {
				continue
			}
			filtered = append(filtered, event)
			continue
		}
		if !liveEventAllowed(event, rules) {
			continue
		}
		if !showHeartbeats && hidesByDefault(event) {
			continue
		}
		if isMetadataOnlyControlEvent(event) {
			continue
		}
		filtered = append(filtered, event)
	}
	return filtered
}

func blockedLiveEvents(events []live.LiveMessage, showHeartbeats bool, rules newsplugin.SubscriptionRules) []live.LiveMessage {
	filtered := make([]live.LiveMessage, 0, len(events))
	for _, event := range events {
		if isPublicLiveRoomID(event.RoomID) {
			continue
		}
		visibility, allowed := classifyLivePublicKeyVisibility(strings.TrimSpace(event.SenderPubKey), metadataString(event.Payload.Metadata, "parent_public_key"), rules)
		if allowed || visibility == "default" {
			continue
		}
		if !showHeartbeats && hidesByDefault(event) {
			continue
		}
		if isMetadataOnlyControlEvent(event) {
			continue
		}
		filtered = append(filtered, event)
	}
	return filtered
}

func limitVisibleLiveEvents(events []live.LiveMessage, keepNonHeartbeat, keepHeartbeat int) []live.LiveMessage {
	if len(events) == 0 {
		return nil
	}
	keep := make([]bool, len(events))
	nonHeartbeatCount := 0
	heartbeatCount := 0
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if strings.TrimSpace(event.Type) == live.TypeHeartbeat {
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
	out := make([]live.LiveMessage, 0, nonHeartbeatCount+heartbeatCount)
	for index, event := range events {
		if keep[index] {
			out = append(out, event)
		}
	}
	return out
}

func applyPublicLiveGuards(events []live.LiveMessage, rules newsplugin.SubscriptionRules) ([]live.LiveMessage, int, int) {
	if len(events) == 0 {
		return events, 0, 0
	}
	mutedOrigin := uniqueLiveKeys(rules.LivePublicMutedOriginKeys)
	mutedParent := uniqueLiveKeys(rules.LivePublicMutedParentKeys)
	limitMessages := rules.LivePublicRateLimitMessages
	windowSeconds := rules.LivePublicRateLimitWindowSeconds
	filtered := make([]live.LiveMessage, 0, len(events))
	mutedCount := 0
	rateLimitedCount := 0
	type senderWindow struct {
		timestamps []time.Time
	}
	recent := make(map[string]senderWindow)
	for _, event := range events {
		parentKey := metadataString(event.Payload.Metadata, "parent_public_key")
		if containsFold(mutedOrigin, strings.TrimSpace(event.SenderPubKey)) || containsFold(mutedParent, parentKey) {
			mutedCount++
			continue
		}
		if strings.TrimSpace(event.Type) == live.TypeMessage && limitMessages > 0 && windowSeconds > 0 {
			senderKey := strings.TrimSpace(event.SenderPubKey)
			if senderKey == "" {
				senderKey = strings.TrimSpace(event.Sender)
			}
			if senderKey != "" {
				if ts, err := time.Parse(time.RFC3339, strings.TrimSpace(event.Timestamp)); err == nil {
					window := recent[senderKey]
					cutoff := ts.Add(-time.Duration(windowSeconds) * time.Second)
					kept := window.timestamps[:0]
					for _, prior := range window.timestamps {
						if !prior.Before(cutoff) {
							kept = append(kept, prior)
						}
					}
					window.timestamps = kept
					if len(window.timestamps) >= limitMessages {
						recent[senderKey] = window
						rateLimitedCount++
						continue
					}
					window.timestamps = append(window.timestamps, ts)
					recent[senderKey] = window
				}
			}
		}
		filtered = append(filtered, event)
	}
	return filtered, mutedCount, rateLimitedCount
}

func filterLiveRoomsByRules(rooms []live.RoomSummary, rules newsplugin.SubscriptionRules) []live.RoomSummary {
	if len(rooms) == 0 {
		return rooms
	}
	filtered := make([]live.RoomSummary, 0, len(rooms))
	for _, room := range rooms {
		if isPublicLiveRoomID(room.RoomID) {
			room.LiveVisibility = "public"
			filtered = append(filtered, room)
			continue
		}
		visibility, allowed := classifyLivePublicKeyVisibility(strings.TrimSpace(room.CreatorPubKey), strings.TrimSpace(room.ParentPublicKey), rules)
		if allowed {
			room.LiveVisibility = visibility
			filtered = append(filtered, room)
		}
	}
	return filtered
}

func buildLivePendingRooms(store *live.LocalStore, rules newsplugin.SubscriptionRules) ([]livePendingRoomSummary, error) {
	rooms, err := store.ListRooms()
	if err != nil {
		return nil, err
	}
	pending := make([]livePendingRoomSummary, 0, len(rooms))
	for _, room := range rooms {
		if isPublicLiveRoomID(room.RoomID) {
			continue
		}
		roomVisibility, roomAllowed := classifyLivePublicKeyVisibility(strings.TrimSpace(room.CreatorPubKey), strings.TrimSpace(room.ParentPublicKey), rules)
		events, err := store.ReadEvents(room.RoomID)
		if err != nil {
			return nil, err
		}
		blockedEvents := blockedLiveEvents(events, true, rules)
		if roomAllowed && len(blockedEvents) == 0 {
			continue
		}
		lastBlockedAt := room.LastEventAt
		if len(blockedEvents) > 0 {
			lastBlockedAt = parseLatestBlockedEventTime(blockedEvents, lastBlockedAt)
		}
		reason := roomVisibility
		if roomAllowed {
			reason = "blocked_events"
		}
		pending = append(pending, livePendingRoomSummary{
			RoomID:            room.RoomID,
			Title:             room.Title,
			Creator:           room.Creator,
			CreatedAt:         room.CreatedAt,
			LastEventAt:       lastBlockedAt,
			Channel:           room.Channel,
			Archive:           room.Archive,
			RoomVisibility:    roomVisibility,
			BlockedEventCount: len(blockedEvents),
			BlockedReason:     reason,
			PendingURL:        "/live/pending/" + room.RoomID,
			APIURL:            "/api/live/pending/" + room.RoomID,
		})
	}
	sort.SliceStable(pending, func(i, j int) bool {
		if pending[i].LastEventAt.Equal(pending[j].LastEventAt) {
			return pending[i].RoomID < pending[j].RoomID
		}
		return pending[i].LastEventAt.After(pending[j].LastEventAt)
	})
	return pending, nil
}

func applyPendingCountsToLiveRooms(rooms []live.RoomSummary, pending []livePendingRoomSummary) {
	if len(rooms) == 0 || len(pending) == 0 {
		return
	}
	counts := make(map[string]int, len(pending))
	for _, item := range pending {
		if item.BlockedEventCount <= 0 {
			continue
		}
		counts[item.RoomID] = item.BlockedEventCount
	}
	for idx := range rooms {
		rooms[idx].PendingBlockedEvents = counts[rooms[idx].RoomID]
	}
}

func buildLiveRoomLinksMap(rooms []live.RoomSummary) map[string]liveRoomLinks {
	if len(rooms) == 0 {
		return nil
	}
	out := make(map[string]liveRoomLinks, len(rooms))
	for _, room := range rooms {
		out[room.RoomID] = liveRoomLinksFor(room.RoomID)
	}
	return out
}

func livePublicLimitValue(limit int) string {
	if limit <= 0 {
		return "关闭"
	}
	return formatCount(limit)
}

func livePublicWindowValue(seconds int) string {
	if seconds <= 0 {
		return "关闭"
	}
	return fmt.Sprintf("%ds", seconds)
}

func saveLivePublicRules(path string, rules newsplugin.SubscriptionRules) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err == nil && len(strings.TrimSpace(string(data))) > 0 {
		var current newsplugin.SubscriptionRules
		if err := jsonUnmarshal(data, &current); err == nil {
			rules = mergeLivePublicRules(current, rules)
		}
	}
	out, err := jsonMarshalIndent(rules)
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(path, out, 0o644)
}

func mergeLivePublicRules(current, updated newsplugin.SubscriptionRules) newsplugin.SubscriptionRules {
	current.LivePublicMutedOriginKeys = updated.LivePublicMutedOriginKeys
	current.LivePublicMutedParentKeys = updated.LivePublicMutedParentKeys
	current.LivePublicRateLimitMessages = updated.LivePublicRateLimitMessages
	current.LivePublicRateLimitWindowSeconds = updated.LivePublicRateLimitWindowSeconds
	return current
}

func parsePublicKeyLines(raw string) []string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	seen := make(map[string]struct{}, len(lines))
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		key := normalizeLivePublicKey(line)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func normalizeLivePublicKey(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if len(value) != 64 {
		return ""
	}
	if _, err := hex.DecodeString(value); err != nil {
		return ""
	}
	return value
}

func parseNonNegativeInt(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func livePublicRequestTrusted(r *http.Request) bool {
	addr := livePublicClientIP(r)
	if !addr.IsValid() {
		return false
	}
	return addr.IsLoopback() || addr.IsPrivate()
}

func livePublicClientIP(r *http.Request) netip.Addr {
	if r == nil {
		return netip.Addr{}
	}
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
		if addr, err := netip.ParseAddr(strings.TrimSpace(forwarded)); err == nil {
			return addr.Unmap()
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		if addr, err := netip.ParseAddr(strings.TrimSpace(host)); err == nil {
			return addr.Unmap()
		}
	}
	if addr, err := netip.ParseAddr(strings.TrimSpace(r.RemoteAddr)); err == nil {
		return addr.Unmap()
	}
	return netip.Addr{}
}

var jsonMarshalIndent = func(value any) ([]byte, error) {
	return json.MarshalIndent(value, "", "  ")
}

var jsonUnmarshal = func(data []byte, value any) error {
	return json.Unmarshal(data, value)
}

func parseLatestBlockedEventTime(events []live.LiveMessage, fallback time.Time) time.Time {
	latest := fallback
	for _, event := range events {
		ts, err := time.Parse(time.RFC3339, strings.TrimSpace(event.Timestamp))
		if err != nil {
			continue
		}
		if latest.IsZero() || ts.After(latest) {
			latest = ts
		}
	}
	return latest
}

func countPendingBlockedRooms(items []livePendingRoomSummary) int {
	count := 0
	for _, item := range items {
		if item.RoomVisibility != "default" && item.RoomVisibility != "" {
			count++
		}
	}
	return count
}

func countPendingBlockedEvents(items []livePendingRoomSummary) int {
	count := 0
	for _, item := range items {
		count += item.BlockedEventCount
	}
	return count
}

func latestPendingRoomValue(items []livePendingRoomSummary) string {
	if len(items) == 0 {
		return "暂无"
	}
	if !items[0].LastEventAt.IsZero() {
		return items[0].LastEventAt.Local().Format("2006-01-02 15:04 MST")
	}
	if !items[0].CreatedAt.IsZero() {
		return items[0].CreatedAt.Local().Format("2006-01-02 15:04 MST")
	}
	return "暂无"
}

func liveRoomAllowed(room live.RoomSummary, rules newsplugin.SubscriptionRules) bool {
	if isPublicLiveRoomID(room.RoomID) {
		return true
	}
	_, allowed := classifyLivePublicKeyVisibility(strings.TrimSpace(room.CreatorPubKey), strings.TrimSpace(room.ParentPublicKey), rules)
	return allowed
}

func liveRoomInfoAllowed(room live.RoomInfo, rules newsplugin.SubscriptionRules) bool {
	if isPublicLiveRoomID(room.RoomID) {
		return true
	}
	_, allowed := classifyLivePublicKeyVisibility(strings.TrimSpace(room.CreatorPubKey), strings.TrimSpace(room.ParentPublicKey), rules)
	return allowed
}

func liveEventAllowed(event live.LiveMessage, rules newsplugin.SubscriptionRules) bool {
	if isPublicLiveRoomID(event.RoomID) {
		return true
	}
	parentKey := metadataString(event.Payload.Metadata, "parent_public_key")
	_, allowed := classifyLivePublicKeyVisibility(strings.TrimSpace(event.SenderPubKey), parentKey, rules)
	return allowed
}

func normalizedLiveRules(rules newsplugin.SubscriptionRules) newsplugin.SubscriptionRules {
	rules.LiveAllowedOriginKeys = uniqueLiveKeys(rules.LiveAllowedOriginKeys)
	rules.LiveBlockedOriginKeys = uniqueLiveKeys(rules.LiveBlockedOriginKeys)
	rules.LiveAllowedParentKeys = uniqueLiveKeys(rules.LiveAllowedParentKeys)
	rules.LiveBlockedParentKeys = uniqueLiveKeys(rules.LiveBlockedParentKeys)
	return rules
}

func liveRulesEmpty(rules newsplugin.SubscriptionRules) bool {
	return len(rules.LiveAllowedOriginKeys) == 0 &&
		len(rules.LiveBlockedOriginKeys) == 0 &&
		len(rules.LiveAllowedParentKeys) == 0 &&
		len(rules.LiveBlockedParentKeys) == 0
}

func hasLiveAllowRules(rules newsplugin.SubscriptionRules) bool {
	return len(rules.LiveAllowedOriginKeys) > 0 || len(rules.LiveAllowedParentKeys) > 0
}

func matchLivePublicKeyFilters(originKey, parentKey string, rules newsplugin.SubscriptionRules) (blocked bool, allowed bool) {
	if containsFold(rules.LiveBlockedOriginKeys, originKey) {
		return true, false
	}
	if containsFold(rules.LiveBlockedParentKeys, parentKey) {
		return true, false
	}
	if containsFold(rules.LiveAllowedOriginKeys, originKey) {
		return false, true
	}
	if containsFold(rules.LiveAllowedParentKeys, parentKey) {
		return false, true
	}
	return false, false
}

func classifyLivePublicKeyVisibility(originKey, parentKey string, rules newsplugin.SubscriptionRules) (string, bool) {
	rules = normalizedLiveRules(rules)
	if liveRulesEmpty(rules) {
		return "default", true
	}
	if containsFold(rules.LiveBlockedOriginKeys, originKey) {
		return "blocked_origin", false
	}
	if containsFold(rules.LiveBlockedParentKeys, parentKey) {
		return "blocked_parent", false
	}
	if containsFold(rules.LiveAllowedOriginKeys, originKey) {
		return "allowed_origin", true
	}
	if containsFold(rules.LiveAllowedParentKeys, parentKey) {
		return "allowed_parent", true
	}
	if hasLiveAllowRules(rules) {
		return "blocked_default", false
	}
	return "default", true
}

func uniqueLiveKeys(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.ToLower(strings.TrimSpace(item))
		if len(item) != 64 {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func containsFold(items []string, needle string) bool {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return false
	}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), needle) {
			return true
		}
	}
	return false
}

func metadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func hidesByDefault(event live.LiveMessage) bool {
	switch strings.TrimSpace(event.Type) {
	case live.TypeHeartbeat, live.TypeArchiveNotice:
		return true
	default:
		return false
	}
}

func queryBool(r *http.Request, key string, defaultValue bool) bool {
	if r == nil {
		return defaultValue
	}
	raw := strings.TrimSpace(strings.ToLower(r.URL.Query().Get(key)))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}

func formatCount(value int) string {
	return strconv.Itoa(value)
}

func latestRoomValue(rooms []live.RoomSummary) string {
	if len(rooms) == 0 {
		return "暂无"
	}
	if !rooms[0].LastEventAt.IsZero() {
		return rooms[0].LastEventAt.Local().Format("2006-01-02 15:04 MST")
	}
	if !rooms[0].CreatedAt.IsZero() {
		return rooms[0].CreatedAt.Local().Format("2006-01-02 15:04 MST")
	}
	return "暂无"
}

func countArchivedRooms(rooms []live.RoomSummary) int {
	count := 0
	for _, room := range rooms {
		if room.Archive != nil {
			count++
		}
	}
	return count
}

func countActiveRooms(rooms []live.RoomSummary) int {
	count := 0
	for _, room := range rooms {
		if room.Active {
			count++
		}
	}
	return count
}

func buildEventViews(events []live.LiveMessage, rules newsplugin.SubscriptionRules) []liveEventView {
	views := make([]liveEventView, 0, len(events))
	for idx := len(events) - 1; idx >= 0; idx-- {
		event := events[idx]
		visibility := "public"
		if !isPublicLiveRoomID(event.RoomID) {
			visibility, _ = classifyLivePublicKeyVisibility(strings.TrimSpace(event.SenderPubKey), metadataString(event.Payload.Metadata, "parent_public_key"), rules)
		}
		view := liveEventView{
			Type:         event.Type,
			Timestamp:    formatLiveDisplayTime(event.Timestamp),
			Sender:       event.Sender,
			Visibility:   visibility,
			Heading:      eventHeading(event),
			HeadingLines: eventHeadingLines(event),
			Fields:       metadataFields(event.Payload.Metadata),
		}
		if task := buildTaskUpdateView(event.Payload.Metadata); task != nil {
			view.Task = task
			view.Note = "任务更新"
		}
		views = append(views, view)
	}
	return views
}

func isMetadataOnlyControlEvent(event live.LiveMessage) bool {
	if strings.TrimSpace(event.Payload.Content) != "" {
		return false
	}
	if buildTaskUpdateView(event.Payload.Metadata) != nil {
		return false
	}
	return len(metadataFields(event.Payload.Metadata)) > 0
}

func buildTaskSummaries(events []live.LiveMessage) []liveTaskSummaryView {
	index := make(map[string]*liveTaskSummaryView)
	order := make([]string, 0)
	for _, event := range events {
		task := buildTaskUpdateView(event.Payload.Metadata)
		if task == nil || strings.TrimSpace(task.TaskID) == "" {
			continue
		}
		item, ok := index[task.TaskID]
		if !ok {
			item = &liveTaskSummaryView{TaskID: task.TaskID}
			index[task.TaskID] = item
			order = append(order, task.TaskID)
		}
		item.UpdateCount++
		item.Status = firstNonEmptyString(task.Status, item.Status)
		item.Description = firstNonEmptyString(task.Description, item.Description)
		item.AssignedTo = firstNonEmptyString(task.AssignedTo, item.AssignedTo)
		item.Progress = firstNonEmptyString(task.Progress, item.Progress)
		item.LastSender = firstNonEmptyString(event.Sender, item.LastSender)
		item.LastUpdatedAt = firstNonEmptyString(event.Timestamp, item.LastUpdatedAt)
	}
	summaries := make([]liveTaskSummaryView, 0, len(order))
	for _, key := range order {
		summaries = append(summaries, *index[key])
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].LastUpdatedAt > summaries[j].LastUpdatedAt
	})
	return summaries
}

func groupTasksByStatus(tasks []liveTaskSummaryView) []liveTaskGroupView {
	return groupTasks(tasks, func(task liveTaskSummaryView) string {
		return firstNonEmptyString(task.Status, "未标记状态")
	})
}

func groupTasksByAssignee(tasks []liveTaskSummaryView) []liveTaskGroupView {
	return groupTasks(tasks, func(task liveTaskSummaryView) string {
		return firstNonEmptyString(task.AssignedTo, "未分配")
	})
}

func groupTasks(tasks []liveTaskSummaryView, fn func(liveTaskSummaryView) string) []liveTaskGroupView {
	counts := map[string]int{}
	for _, task := range tasks {
		key := strings.TrimSpace(fn(task))
		if key == "" {
			continue
		}
		counts[key]++
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	groups := make([]liveTaskGroupView, 0, len(keys))
	for _, key := range keys {
		groups = append(groups, liveTaskGroupView{Key: key, Count: counts[key]})
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Count == groups[j].Count {
			return groups[i].Key < groups[j].Key
		}
		return groups[i].Count > groups[j].Count
	})
	return groups
}

func eventHeading(event live.LiveMessage) string {
	content := strings.TrimSpace(event.Payload.Content)
	if content != "" {
		return content
	}
	switch event.Type {
	case live.TypeJoin:
		return "加入房间"
	case live.TypeLeave:
		return "离开房间"
	case live.TypeHeartbeat:
		return "在线心跳"
	case live.TypeTaskUpdate:
		return "任务状态更新"
	case live.TypeArchiveNotice:
		return "房间归档通知"
	default:
		return "控制事件"
	}
}

func eventHeadingLines(event live.LiveMessage) []string {
	content := strings.TrimSpace(event.Payload.Content)
	if content == "" || (!strings.Contains(content, "；") && !strings.Contains(content, ";")) {
		return nil
	}
	parts := strings.Split(content, " | ")
	if len(parts) < 2 {
		items := splitNonEmpty(splitLiveItems(content))
		if len(items) <= 1 {
			return nil
		}
		return items
	}
	head := strings.Join(parts[:len(parts)-1], " | ")
	items := splitNonEmpty(splitLiveItems(parts[len(parts)-1]))
	if len(items) == 0 {
		return nil
	}
	lines := make([]string, 0, len(items)+1)
	lines = append(lines, head)
	lines = append(lines, items...)
	return lines
}

func splitNonEmpty(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func splitLiveItems(raw string) []string {
	raw = strings.ReplaceAll(raw, "；", ";")
	return strings.Split(raw, ";")
}

func formatLiveDisplayTime(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return raw
	}
	return ts.Local().Format("2006-01-02 15:04:05 MST")
}

func formatHistoryArchives(items []live.RoomHistoryArchive) []live.RoomHistoryArchive {
	if len(items) == 0 {
		return nil
	}
	out := make([]live.RoomHistoryArchive, 0, len(items))
	for _, item := range items {
		copyItem := item
		copyItem.Events = nil
		copyItem.ArchivedAt = formatLiveDisplayTime(copyItem.ArchivedAt)
		copyItem.StartAt = formatLiveDisplayTime(copyItem.StartAt)
		copyItem.EndAt = formatLiveDisplayTime(copyItem.EndAt)
		out = append(out, copyItem)
	}
	return out
}

func formatHistoryArchive(item *live.RoomHistoryArchive) *live.RoomHistoryArchive {
	if item == nil {
		return nil
	}
	copyItem := *item
	copyItem.ArchivedAt = formatLiveDisplayTime(copyItem.ArchivedAt)
	copyItem.StartAt = formatLiveDisplayTime(copyItem.StartAt)
	copyItem.EndAt = formatLiveDisplayTime(copyItem.EndAt)
	return &copyItem
}

func historyArchiveEvents(item *live.RoomHistoryArchive) []live.LiveMessage {
	if item == nil || len(item.Events) == 0 {
		return nil
	}
	events := make([]live.LiveMessage, len(item.Events))
	copy(events, item.Events)
	return events
}

func buildTaskUpdateView(metadata map[string]any) *liveTaskUpdateView {
	if len(metadata) == 0 {
		return nil
	}
	taskID := metadataString(metadata, "task_id")
	status := metadataString(metadata, "status")
	description := metadataString(metadata, "description")
	assignedTo := metadataString(metadata, "assigned_to")
	progress := metadataProgress(metadata["progress"])
	if taskID == "" && status == "" && description == "" && assignedTo == "" && progress == "" {
		return nil
	}
	return &liveTaskUpdateView{
		TaskID:      taskID,
		Status:      status,
		Description: description,
		AssignedTo:  assignedTo,
		Progress:    progress,
	}
}

func metadataFields(metadata map[string]any) []liveFieldView {
	if len(metadata) == 0 {
		return nil
	}
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := make([]liveFieldView, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(fmt.Sprint(metadata[key]))
		if value == "" || value == "<nil>" {
			continue
		}
		fields = append(fields, liveFieldView{Key: key, Value: value})
	}
	return fields
}

func metadataProgress(value any) string {
	if value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return ""
	}
	if strings.HasSuffix(text, "%") {
		return text
	}
	return text + "%"
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
