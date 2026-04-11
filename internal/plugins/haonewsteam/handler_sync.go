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
	webhookStatus, err := store.LoadWebhookDeliveryStatusCtx(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		Project:               app.ProjectName(),
		Version:               app.VersionString(),
		PageNav:               app.PageNav("/teams"),
		NodeStatus:            app.NodeStatus(index),
		Now:                   time.Now(),
		Team:                  info,
		SyncNotice:            strings.TrimSpace(r.URL.Query().Get("resolved")),
		SyncStatus:            status.TeamSync,
		WebhookStatus:         webhookStatus,
		RecentConflicts:       conflicts,
		ConflictViews:         buildTeamSyncConflictViews(conflicts),
		OpenConflictViews:     buildOpenTeamSyncConflictViews(conflicts),
		ResolvedConflictViews: buildResolvedTeamSyncConflictViews(conflicts),
		StatusGroups:          buildTeamSyncStatusGroups(status.TeamSync, webhookStatus),
		HealthLevel:           teamSyncHealthLevel(status.TeamSync, webhookStatus),
		HealthTitle:           teamSyncHealthTitle(status.TeamSync, webhookStatus),
		HealthHint:            teamSyncHealthHint(status.TeamSync, webhookStatus),
		ResolvedTitle:         teamSyncResolvedTitle(strings.TrimSpace(r.URL.Query().Get("resolved"))),
		ResolvedHint:          teamSyncResolvedHint(strings.TrimSpace(r.URL.Query().Get("resolved"))),
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "已订阅 Team", Value: formatTeamCount(status.TeamSync.SubscribedTeams)},
			{Label: "pending ack", Value: formatTeamCount(status.TeamSync.PendingAcks)},
			{Label: "ack peers", Value: formatTeamCount(status.TeamSync.AckPeers)},
			{Label: "冲突", Value: formatTeamCount(status.TeamSync.Conflicts)},
			{Label: "已处理冲突", Value: formatTeamCount(status.TeamSync.ResolvedConflicts)},
			{Label: "冲突清理", Value: formatTeamCount(status.TeamSync.ConflictPrunes)},
			{Label: "webhook dead-letter", Value: formatTeamCount(webhookStatus.DeadLetterCount)},
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
	webhookStatus, err := store.LoadWebhookDeliveryStatusCtx(r.Context(), teamID)
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
		"webhook_status":   webhookStatus,
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
		allowKeepLocal, allowAcceptRemote := conflictActionPermissions(record, supportsAcceptRemoteConflict(syncType))
		views = append(views, teamSyncConflictView{
			Record:             record,
			AutoResolvable:     record.AutoResolvable,
			AutoResolutionHint: describeTeamSyncConflictAutoResolution(record),
			AllowAcceptRemote:  allowAcceptRemote,
			AllowKeepLocal:     allowKeepLocal,
			SuggestedAction:    suggestedAction,
			ReasonLabel:        reasonLabel,
			ActionHint:         actionHint,
			SubjectLabel:       describeTeamSyncConflictSubject(record),
			ConflictClass:      classifyTeamSyncConflict(record),
			SeverityLabel:      describeTeamSyncConflictSeverity(record),
			ConsequenceHint:    describeTeamSyncConflictConsequence(record),
			LocalVersionLabel:  formatSyncConflictVersion(record.LocalVersion),
			RemoteVersionLabel: formatSyncConflictVersion(record.RemoteVersion),
			Actions:            buildTeamSyncConflictActions(record, allowKeepLocal, allowAcceptRemote, suggestedAction),
		})
	}
	return views
}

func buildOpenTeamSyncConflictViews(records []corehaonews.TeamSyncConflictRecord) []teamSyncConflictView {
	views := buildTeamSyncConflictViews(records)
	out := make([]teamSyncConflictView, 0, len(views))
	for _, view := range views {
		if strings.TrimSpace(view.Record.Resolution) == "" {
			out = append(out, view)
		}
	}
	return out
}

func buildResolvedTeamSyncConflictViews(records []corehaonews.TeamSyncConflictRecord) []teamSyncConflictView {
	views := buildTeamSyncConflictViews(records)
	out := make([]teamSyncConflictView, 0, len(views))
	for _, view := range views {
		if strings.TrimSpace(view.Record.Resolution) != "" {
			out = append(out, view)
		}
	}
	return out
}

func conflictActionPermissions(record corehaonews.TeamSyncConflictRecord, allowAcceptRemote bool) (bool, bool) {
	if strings.TrimSpace(record.Resolution) != "" {
		return false, false
	}
	switch strings.TrimSpace(record.Reason) {
	case "signature_rejected", "policy_rejected":
		return false, false
	default:
		return true, allowAcceptRemote
	}
}

func buildTeamSyncConflictActions(record corehaonews.TeamSyncConflictRecord, allowKeepLocal, allowAcceptRemote bool, suggestedAction string) []teamSyncConflictActionView {
	if strings.TrimSpace(record.Resolution) != "" {
		return nil
	}
	actions := make([]teamSyncConflictActionView, 0, 4)
	if record.AutoResolvable {
		actions = append(actions, teamSyncConflictActionView{
			Value:   "auto",
			Label:   "自动收敛",
			Primary: suggestedAction == "auto",
		})
	}
	actions = append(actions, teamSyncConflictActionView{
		Value:   "dismiss",
		Label:   "忽略",
		Primary: suggestedAction == "dismiss",
	})
	if allowKeepLocal {
		actions = append(actions, teamSyncConflictActionView{
			Value:   "keep_local",
			Label:   "保留本地",
			Primary: suggestedAction == "keep_local",
		})
	}
	if allowAcceptRemote {
		actions = append(actions, teamSyncConflictActionView{
			Value:   "accept_remote",
			Label:   "接受远端",
			Primary: suggestedAction == "accept_remote" || suggestedAction == "review_accept_remote",
		})
	}
	return actions
}

func describeTeamSyncConflictSubject(record corehaonews.TeamSyncConflictRecord) string {
	subject := strings.TrimSpace(record.SubjectID)
	if subject == "" {
		subject = strings.TrimSpace(record.Key)
	}
	if subject == "" {
		subject = strings.TrimSpace(record.TeamID)
	}
	switch strings.TrimSpace(record.SyncType) {
	case "task":
		return "Task / " + subject
	case "artifact":
		return "Artifact / " + subject
	case "member":
		return "Member / " + subject
	case "policy":
		return "Policy / " + subject
	case "channel":
		return "Channel / " + subject
	default:
		if strings.TrimSpace(record.Type) != "" {
			return titleSyncKind(strings.TrimSpace(record.Type)) + " / " + subject
		}
		return subject
	}
}

func titleSyncKind(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func classifyTeamSyncConflict(record corehaonews.TeamSyncConflictRecord) string {
	switch strings.TrimSpace(record.Reason) {
	case "remote_newer":
		return "safe-remote"
	case "local_newer":
		return "safe-local"
	case "same_version_diverged":
		return "diverged"
	case "signature_rejected", "policy_rejected":
		return "rejected"
	default:
		return "review"
	}
}

func describeTeamSyncConflictSeverity(record corehaonews.TeamSyncConflictRecord) string {
	switch strings.TrimSpace(record.Reason) {
	case "signature_rejected", "policy_rejected":
		return "blocked"
	case "same_version_diverged":
		return "risky"
	case "remote_newer", "local_newer":
		return "attention"
	default:
		return "info"
	}
}

func describeTeamSyncConflictConsequence(record corehaonews.TeamSyncConflictRecord) string {
	switch strings.TrimSpace(record.Reason) {
	case "remote_newer":
		return "如果继续保留本地，远端较新的内容不会自动进入当前节点。"
	case "local_newer":
		return "如果直接接受远端，当前节点已经更新过的内容会被旧版本覆盖。"
	case "same_version_diverged":
		return "两个节点现在都认为自己是当前版本，不先统一结果就会持续分叉。"
	case "signature_rejected":
		return "这条记录因签名校验失败被拒绝，先修签名或来源再考虑重放。"
	case "policy_rejected":
		return "这条记录被 Team Policy 拒绝，直接接受远端不会生效。"
	default:
		return "建议先查看本地和远端版本，再决定用哪个动作收敛。"
	}
}

func formatSyncConflictVersion(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Local().Format("2006-01-02 15:04:05")
}

func buildTeamSyncStatusGroups(status corehaonews.SyncTeamSyncStatus, webhook teamcore.WebhookDeliveryStatus) []teamSyncStatusGroup {
	return []teamSyncStatusGroup{
		{
			Title:    "订阅与游标",
			Subtitle: "看 team 订阅、cursor 和 state 是否稳定前进。",
			Metrics: []teamSyncMetricValue{
				{Label: "subscribed teams", Value: formatTeamCount(status.SubscribedTeams)},
				{Label: "persisted cursors", Value: formatTeamCount(status.PersistedCursors)},
				{Label: "ack peers", Value: formatTeamCount(status.AckPeers)},
			},
			Details: []string{
				"persisted peer acks：" + formatTeamCount(status.PersistedPeerAcks),
				"primed channels：" + formatTeamCount(status.PrimedChannels) + " · primed history：" + formatTeamCount(status.PrimedHistoryTeams),
				"last subscription：" + emptyIfZero(status.LastSubscriptionTeam, "暂无") + " / " + formatTeamTimePtr(status.LastSubscriptionAt),
			},
		},
		{
			Title:    "主线吞吐",
			Subtitle: "看 publish / receive / apply 是否一致前进。",
			Metrics: []teamSyncMetricValue{
				{Label: "published", Value: formatTeamCount(status.PublishedMessages + status.PublishedHistory + status.PublishedTasks + status.PublishedArtifacts + status.PublishedMembers + status.PublishedPolicies + status.PublishedConfigChannels)},
				{Label: "received", Value: formatTeamCount(status.ReceivedMessages + status.ReceivedHistory + status.ReceivedTasks + status.ReceivedArtifacts + status.ReceivedMembers + status.ReceivedPolicies + status.ReceivedConfigChannels)},
				{Label: "applied", Value: formatTeamCount(status.AppliedMessages + status.AppliedHistory + status.AppliedTasks + status.AppliedArtifacts + status.AppliedMembers + status.AppliedPolicies + status.AppliedConfigChannels)},
			},
			Details: []string{
				"message/history：" + formatTeamCount(status.PublishedMessages) + " / " + formatTeamCount(status.AppliedMessages) + " · " + formatTeamCount(status.PublishedHistory) + " / " + formatTeamCount(status.AppliedHistory),
				"task/artifact：" + formatTeamCount(status.PublishedTasks) + " / " + formatTeamCount(status.AppliedTasks) + " · " + formatTeamCount(status.PublishedArtifacts) + " / " + formatTeamCount(status.AppliedArtifacts),
				"member/policy/channel：" + formatTeamCount(status.PublishedMembers) + " / " + formatTeamCount(status.AppliedMembers) + " · " + formatTeamCount(status.PublishedPolicies) + " / " + formatTeamCount(status.AppliedPolicies) + " · " + formatTeamCount(status.PublishedConfigChannels) + " / " + formatTeamCount(status.AppliedConfigChannels),
			},
		},
		{
			Title:    "Ack 与重试",
			Subtitle: "看 pending、retry 和压缩是否失控。",
			Metrics: []teamSyncMetricValue{
				{Label: "pending", Value: formatTeamCount(status.PendingAcks)},
				{Label: "retried", Value: formatTeamCount(status.RetriedPublishes)},
				{Label: "expired", Value: formatTeamCount(status.ExpiredPending)},
			},
			Details: []string{
				"published / received / applied acks：" + formatTeamCount(status.PublishedAcks) + " / " + formatTeamCount(status.ReceivedAcks) + " / " + formatTeamCount(status.AppliedAcks),
				"superseded pending：" + formatTeamCount(status.SupersededPending),
				"last acked key：" + emptyIfZero(status.LastAckedKey, "暂无"),
			},
		},
		{
			Title:    "Webhook 投递",
			Subtitle: "直接看 delivered / failed / dead-letter，不再只靠日志猜。",
			Metrics: []teamSyncMetricValue{
				{Label: "delivered", Value: formatTeamCount(webhook.DeliveredCount)},
				{Label: "failed", Value: formatTeamCount(webhook.FailedCount)},
				{Label: "dead_letter", Value: formatTeamCount(webhook.DeadLetterCount)},
			},
			Details: []string{
				"retrying：" + formatTeamCount(webhook.RetryingCount),
				"recent delivered：" + formatTeamCount(len(webhook.RecentDelivered)),
				"recent dead letters：" + formatTeamCount(len(webhook.RecentDead)),
			},
		},
	}
}

func emptyIfZero(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
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
		if record.AutoResolvable {
			return "本地版本更新较新", "这类冲突只涉及任务状态或追加语义，可以直接自动收敛。", "auto"
		}
		if allowAcceptRemote {
			return "本地版本更新较新", "建议保留本地版本，除非你明确要覆盖为远端。", "keep_local"
		}
		return "本地版本更新较新", "建议人工复核后再决定是否保留本地版本。", "dismiss"
	case reason == "same_version_diverged":
		if record.AutoResolvable {
			return "版本相同但状态分叉", "这类任务状态分叉可按时间戳自动收敛，不需要人工逐条判断。", "auto"
		}
		if allowAcceptRemote {
			return "版本相同但内容不同", "两个节点在同一版本号上出现了不同内容。通常先接受远端或保留本地，统一到一个结果。", "accept_remote"
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

func describeTeamSyncConflictAutoResolution(record corehaonews.TeamSyncConflictRecord) string {
	if !record.AutoResolvable {
		return ""
	}
	switch strings.TrimSpace(record.SyncType) {
	case "task":
		return "任务状态冲突可按更新时间自动收敛，避免把人工冲突留给页面处理。"
	case "message":
		return "消息同步默认按追加语义处理，可自动收敛。"
	default:
		return "该冲突可按默认策略自动收敛。"
	}
}

func teamSyncHealthLevel(status corehaonews.SyncTeamSyncStatus, webhook teamcore.WebhookDeliveryStatus) string {
	if status.SubscribedTeams == 0 {
		return "disabled"
	}
	if status.Conflicts > status.ResolvedConflicts || webhook.DeadLetterCount > 0 {
		return "attention"
	}
	last := mostRecentTime(status.LastAppliedAt, status.LastReceivedAt, status.LastPublishedAt)
	if !last.IsZero() && time.Since(last) > time.Hour {
		return "stale"
	}
	return "healthy"
}

func teamSyncHealthTitle(status corehaonews.SyncTeamSyncStatus, webhook teamcore.WebhookDeliveryStatus) string {
	switch teamSyncHealthLevel(status, webhook) {
	case "disabled":
		return "未启用同步"
	case "attention":
		return "需要处理"
	case "stale":
		return "同步可能停滞"
	default:
		return "同步正常运行中"
	}
}

func teamSyncHealthHint(status corehaonews.SyncTeamSyncStatus, webhook teamcore.WebhookDeliveryStatus) string {
	switch teamSyncHealthLevel(status, webhook) {
	case "disabled":
		return "当前节点没有订阅任何 Team，同步链路还没有启动。"
	case "attention":
		parts := make([]string, 0, 2)
		if status.Conflicts > status.ResolvedConflicts {
			parts = append(parts, "有未处理的复制冲突")
		}
		if webhook.DeadLetterCount > 0 {
			parts = append(parts, "webhook 存在 dead-letter")
		}
		return strings.Join(parts, "，")
	case "stale":
		return "最近超过 1 小时没有新的 publish / receive / apply，建议检查对端节点和 sync daemon。"
	default:
		return "最近的 publish / receive / apply 都在前进，可以把细节指标折叠起来只在需要时查看。"
	}
}

func teamSyncResolvedTitle(action string) string {
	switch action {
	case "dismiss":
		return "冲突已忽略"
	case "accept_remote":
		return "已接受远端版本"
	case "keep_local":
		return "已保留本地版本"
	default:
		return ""
	}
}

func teamSyncResolvedHint(action string) string {
	switch action {
	case "dismiss":
		return "这次冲突已标记为忽略；后续如果再次出现同类分叉，还会重新进入列表。"
	case "accept_remote":
		return "本地将以远端内容为准；如果后面继续分叉，再回到这里处理。"
	case "keep_local":
		return "本地版本已经保留；远端内容本次不会覆盖当前结果。"
	default:
		return ""
	}
}

func mostRecentTime(values ...*time.Time) time.Time {
	var out time.Time
	for _, value := range values {
		if value == nil || value.IsZero() {
			continue
		}
		if out.IsZero() || value.After(out) {
			out = *value
		}
	}
	return out
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
