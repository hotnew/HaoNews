package haonewsteam

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	corehaonews "hao.news/internal/haonews"
	teamcore "hao.news/internal/haonews/team"
	newsplugin "hao.news/internal/plugins/haonews"
)

func handleTeamSync(app *newsplugin.App, store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	info, err := store.LoadTeamCtx(r.Context(), teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	conflicts, err := corehaonews.LoadTeamSyncConflicts(app.StoreRoot(), teamID, corehaonews.TeamSyncConflictFilter{Limit: 10})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	status, err := loadTeamSyncRuntimeStatus(app.StoreRoot())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := teamSyncPageData{
		Project:         app.ProjectName(),
		Version:         app.VersionString(),
		PageNav:         app.PageNav("/teams"),
		NodeStatus:      app.NodeStatus(index),
		Now:             time.Now(),
		Team:            info,
		SyncNotice:      strings.TrimSpace(r.URL.Query().Get("resolved")),
		SyncStatus:      status.TeamSync,
		RecentConflicts: conflicts,
		ConflictViews:   buildTeamSyncConflictViews(conflicts),
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "已订阅 Team", Value: formatTeamCount(status.TeamSync.SubscribedTeams)},
			{Label: "pending ack", Value: formatTeamCount(status.TeamSync.PendingAcks)},
			{Label: "ack peers", Value: formatTeamCount(status.TeamSync.AckPeers)},
			{Label: "冲突", Value: formatTeamCount(status.TeamSync.Conflicts)},
			{Label: "已处理冲突", Value: formatTeamCount(status.TeamSync.ResolvedConflicts)},
			{Label: "冲突清理", Value: formatTeamCount(status.TeamSync.ConflictPrunes)},
			{Label: "最近 publish", Value: formatTeamTimePtr(status.TeamSync.LastPublishedAt)},
			{Label: "最近 apply", Value: formatTeamTimePtr(status.TeamSync.LastAppliedAt)},
		},
	}
	if err := app.Templates().ExecuteTemplate(w, "team_sync.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleTeamSyncConflictResolvePage(app *newsplugin.App, store *teamcore.Store, teamID, conflictKey string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	record, err := resolveTeamSyncConflict(app, store, teamID, conflictKey, r.RemoteAddr, r.FormValue("actor_agent_id"), r.FormValue("action"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	redirectURL := "/teams/" + teamID + "/sync"
	if strings.TrimSpace(record.Resolution) != "" {
		redirectURL += "?resolved=" + record.Resolution
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

func handleAPITeamSync(app *newsplugin.App, store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	info, err := store.LoadTeamCtx(r.Context(), teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	status, err := loadTeamSyncRuntimeStatus(app.StoreRoot())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	limit := clampTeamListLimit(r.URL.Query().Get("limit"), 10, 100)
	conflicts, err := corehaonews.LoadTeamSyncConflicts(app.StoreRoot(), teamID, corehaonews.TeamSyncConflictFilter{Limit: limit})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":            "team-sync-health",
		"team_id":          info.TeamID,
		"team_sync":        status.TeamSync,
		"conflict_count":   len(conflicts),
		"recent_conflicts": conflicts,
		"conflict_views":   buildTeamSyncConflictViews(conflicts),
	})
}

func handleAPITeamSyncConflicts(app *newsplugin.App, store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := clampTeamListLimit(r.URL.Query().Get("limit"), 50, 200)
		filter := corehaonews.TeamSyncConflictFilter{
			Type:       strings.TrimSpace(r.URL.Query().Get("type")),
			SubjectID:  strings.TrimSpace(r.URL.Query().Get("subject_id")),
			SourceNode: strings.TrimSpace(r.URL.Query().Get("source_node")),
			Limit:      limit,
		}
		conflicts, err := corehaonews.LoadTeamSyncConflicts(app.StoreRoot(), teamID, filter)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
			"scope":          "team-sync-conflicts",
			"team_id":        teamID,
			"conflict_count": len(conflicts),
			"conflicts":      conflicts,
			"applied_filters": map[string]any{
				"type":        filter.Type,
				"subject_id":  filter.SubjectID,
				"source_node": filter.SourceNode,
				"limit":       limit,
			},
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func loadTeamSyncRuntimeStatus(storeRoot string) (corehaonews.SyncRuntimeStatus, error) {
	path := filepath.Join(strings.TrimSpace(storeRoot), "sync", "status.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return corehaonews.SyncRuntimeStatus{}, nil
	}
	if err != nil {
		return corehaonews.SyncRuntimeStatus{}, err
	}
	var status corehaonews.SyncRuntimeStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return corehaonews.SyncRuntimeStatus{}, err
	}
	return status, nil
}

func resolveTeamSyncConflict(app *newsplugin.App, store *teamcore.Store, teamID, conflictKey, remoteAddr, actorAgentID, action string) (corehaonews.TeamSyncConflictRecord, error) {
	request := &http.Request{RemoteAddr: remoteAddr}
	if !teamRequestTrusted(request) {
		return corehaonews.TeamSyncConflictRecord{}, errors.New("team sync conflict resolution is limited to local or LAN requests")
	}
	actorAgentID = strings.TrimSpace(actorAgentID)
	action = strings.TrimSpace(action)
	if err := requireTeamConflictAction(store, teamID, actorAgentID); err != nil {
		return corehaonews.TeamSyncConflictRecord{}, err
	}
	record, err := corehaonews.ResolveTeamSyncConflict(app.StoreRoot(), teamID, conflictKey, action, actorAgentID)
	if err != nil {
		return corehaonews.TeamSyncConflictRecord{}, err
	}
	_ = appendTeamHistory(store, historyActor{AgentID: actorAgentID, Source: "api"}, teamID, "sync-conflict", "resolve", conflictKey, "处理 Team 复制冲突", map[string]any{
		"diff_summary":      "复制冲突已处理",
		"reason_before":     record.Reason,
		"resolution_after":  record.Resolution,
		"subject_id_after":  record.SubjectID,
		"source_node_after": record.SourceNode,
		"sync_type_after":   record.SyncType,
	})
	return record, nil
}

func buildTeamSyncConflictViews(records []corehaonews.TeamSyncConflictRecord) []teamSyncConflictView {
	views := make([]teamSyncConflictView, 0, len(records))
	for _, record := range records {
		syncType := strings.TrimSpace(record.SyncType)
		if syncType == "" {
			syncType = strings.TrimSpace(record.Type)
		}
		reasonLabel, actionHint, suggestedAction := describeTeamSyncConflict(record, supportsAcceptRemoteConflict(syncType))
		views = append(views, teamSyncConflictView{
			Record:            record,
			AllowAcceptRemote: supportsAcceptRemoteConflict(syncType),
			SuggestedAction:   suggestedAction,
			ReasonLabel:       reasonLabel,
			ActionHint:        actionHint,
		})
	}
	return views
}

func supportsAcceptRemoteConflict(syncType string) bool {
	switch strings.TrimSpace(syncType) {
	case "task", "artifact", "member", "policy", "channel":
		return true
	default:
		return false
	}
}

func describeTeamSyncConflict(record corehaonews.TeamSyncConflictRecord, allowAcceptRemote bool) (string, string, string) {
	reason := strings.TrimSpace(record.Reason)
	switch {
	case reason == "local_newer":
		if allowAcceptRemote {
			return "本地版本更新较新", "建议保留本地版本，除非你明确要覆盖为远端。", "keep_local"
		}
		return "本地版本更新较新", "建议人工复核后再决定是否保留本地版本。", "dismiss"
	case reason == "same_version_diverged":
		if allowAcceptRemote {
			return "同版本分叉", "建议先选一个方向，通常接收远端即可恢复一致性。", "accept_remote"
		}
		return "同版本分叉", "建议人工复核差异后再处理。", "dismiss"
	case reason == "signature_rejected":
		return "签名校验失败", "建议驳回并检查消息签名或来源节点。", "dismiss"
	case reason == "policy_rejected":
		return "策略拒绝", "建议先修订策略，再重新发起同步。", "dismiss"
	case reason == "remote_newer":
		if allowAcceptRemote {
			return "远端版本较新", "建议接收远端版本并检查本地是否存在未同步写入。", "accept_remote"
		}
		return "远端版本较新", "建议人工复核后再决定。", "dismiss"
	case reason == "":
		if allowAcceptRemote {
			return "待人工复核", "建议先比对本地/远端差异，再决定 keep_local 或 accept_remote。", "review_accept_remote"
		}
		return "待人工复核", "建议先人工复核差异，再决定处理动作。", "dismiss"
	default:
		if allowAcceptRemote {
			return reason, "建议人工复核后再决定是否接收远端。", "review_accept_remote"
		}
		return reason, "建议人工复核后再决定。", "dismiss"
	}
}

func countUnresolvedTeamConflicts(conflicts []corehaonews.TeamSyncConflictRecord) int {
	count := 0
	for _, record := range conflicts {
		if strings.TrimSpace(record.Resolution) == "" {
			count++
		}
	}
	return count
}

func countResolvedTeamConflicts(storeRoot, teamID string) int {
	conflicts, err := corehaonews.LoadTeamSyncConflicts(storeRoot, teamID, corehaonews.TeamSyncConflictFilter{
		IncludeResolved: true,
		Limit:           200,
	})
	if err != nil {
		return 0
	}
	count := 0
	for _, record := range conflicts {
		if strings.TrimSpace(record.Resolution) != "" {
			count++
		}
	}
	return count
}

func handleAPITeamSyncConflictResolve(app *newsplugin.App, store *teamcore.Store, teamID, conflictKey string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		ActorAgentID string `json:"actor_agent_id"`
		Action       string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	record, err := resolveTeamSyncConflict(app, store, teamID, conflictKey, r.RemoteAddr, payload.ActorAgentID, payload.Action)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":    "team-sync-conflict-resolve",
		"team_id":  teamID,
		"conflict": record,
	})
}
