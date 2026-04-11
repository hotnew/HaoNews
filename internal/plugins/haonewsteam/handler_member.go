package haonewsteam

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	teamcore "hao.news/internal/haonews/team"
	newsplugin "hao.news/internal/plugins/haonews"
)

func handleTeamMembers(app *newsplugin.App, store teamcore.TeamReader, teamID string, w http.ResponseWriter, r *http.Request) {
	info, err := store.LoadTeamCtx(r.Context(), teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	policy, err := store.LoadPolicyCtx(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	members, err := store.LoadMembersCtx(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filterStatus := strings.TrimSpace(r.URL.Query().Get("status"))
	filterRole := strings.TrimSpace(r.URL.Query().Get("role"))
	filterAgent := strings.TrimSpace(r.URL.Query().Get("agent"))
	statusCounts := memberStatusCounts(members)
	roleCounts := memberRoleCounts(members)
	memberStats, err := store.ComputeMemberStatsCtx(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filtered := filterMembers(members, filterStatus, filterRole, filterAgent)
	data := teamMembersPageData{
		Project:        app.ProjectName(),
		Version:        app.VersionString(),
		PageNav:        app.PageNav("/teams"),
		NodeStatus:     app.NodeStatus(index),
		Now:            time.Now(),
		Team:           info,
		Policy:         policy,
		Members:        filtered,
		PendingMembers: filterMembersByStatus(members, "pending"),
		FilterStatus:   filterStatus,
		FilterRole:     filterRole,
		FilterAgent:    filterAgent,
		AppliedFilters: appliedTeamFilters(
			labeledTeamFilter("状态", filterStatus),
			labeledTeamFilter("角色", filterRole),
			labeledTeamFilter("Agent", filterAgent),
		),
		Statuses:     memberStatuses(members),
		Roles:        memberRoles(members),
		MemberStats:  memberStats,
		StatusCounts: statusCounts,
		RoleCounts:   roleCounts,
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "成员", Value: formatTeamCount(len(filtered))},
			{Label: "active", Value: formatTeamCount(statusCounts["active"])},
			{Label: "pending", Value: formatTeamCount(statusCounts["pending"])},
			{Label: "muted", Value: formatTeamCount(statusCounts["muted"])},
			{Label: "owner", Value: formatTeamCount(roleCounts["owner"])},
		},
	}
	if err := app.Templates().ExecuteTemplate(w, "team_members.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleTeamPolicyUpdate(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team policy update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	before, err := store.LoadPolicyCtx(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := requireTeamAction(store, teamID, strings.TrimSpace(r.FormValue("actor_agent_id")), "policy.update"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	policy := teamcore.Policy{
		MessageRoles:     parseCSVStrings(r.FormValue("message_roles")),
		TaskRoles:        parseCSVStrings(r.FormValue("task_roles")),
		SystemNoteRoles:  parseCSVStrings(r.FormValue("system_note_roles")),
		Permissions:      before.Permissions,
		RequireSignature: teamFormBool(r.FormValue("require_signature")),
		TaskTransitions:  before.TaskTransitions,
		UpdatedAt:        time.Now().UTC(),
	}
	if err := store.SavePolicyCtx(r.Context(), teamID, policy); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{Source: "page"}, teamID, "policy", "update", "team-policy", "更新 Team Policy", policyHistoryMetadata(before, policy))
	http.Redirect(w, r, "/teams/"+teamID, http.StatusSeeOther)
}

func handleTeamMemberUpdate(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team member update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	member := teamcore.Member{
		AgentID:         strings.TrimSpace(r.FormValue("agent_id")),
		OriginPublicKey: strings.TrimSpace(r.FormValue("origin_public_key")),
		ParentPublicKey: strings.TrimSpace(r.FormValue("parent_public_key")),
		Role:            strings.TrimSpace(r.FormValue("role")),
		Status:          strings.TrimSpace(r.FormValue("status")),
	}
	if err := requireTeamAction(store, teamID, strings.TrimSpace(r.FormValue("actor_agent_id")), "member.update"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	before, _ := loadTeamMember(store, teamID, member.AgentID)
	if err := upsertTeamMemberCtx(r.Context(), store, teamID, member); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	after, _ := loadTeamMember(store, teamID, member.AgentID)
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{
		AgentID:         member.AgentID,
		OriginPublicKey: member.OriginPublicKey,
		ParentPublicKey: member.ParentPublicKey,
		Source:          "page",
	}, teamID, "member", "update", member.AgentID, "更新 Team 成员角色或状态", memberHistoryMetadata(before, after))
	http.Redirect(w, r, "/teams/"+teamID, http.StatusSeeOther)
}

func handleTeamMemberAction(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team member action is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := requireTeamAction(store, teamID, strings.TrimSpace(r.FormValue("actor_agent_id")), "member.transition"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	member, summary, metadata, err := applyTeamMemberActionCtx(r.Context(), store, teamID, strings.TrimSpace(r.FormValue("agent_id")), strings.TrimSpace(r.FormValue("action")))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{
		AgentID:         member.AgentID,
		OriginPublicKey: member.OriginPublicKey,
		ParentPublicKey: member.ParentPublicKey,
		Source:          "page",
	}, teamID, "member", "transition", member.AgentID, summary, metadata)
	http.Redirect(w, r, "/teams/"+teamID, http.StatusSeeOther)
}

func handleTeamMemberBulkAction(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team member action is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	action := strings.TrimSpace(r.FormValue("action"))
	agentIDs := parseCSVStrings(r.FormValue("agent_ids"))
	if err := requireTeamAction(store, teamID, strings.TrimSpace(r.FormValue("actor_agent_id")), "member.bulk-transition"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if len(agentIDs) == 0 {
		http.Error(w, "empty agent_ids", http.StatusBadRequest)
		return
	}
	for _, agentID := range agentIDs {
		member, summary, metadata, err := applyTeamMemberActionCtx(r.Context(), store, teamID, agentID, action)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		metadata["batch"] = true
		metadata["batch_size"] = len(agentIDs)
		_ = appendTeamHistoryCtx(r.Context(), store, historyActor{
			AgentID:         member.AgentID,
			OriginPublicKey: member.OriginPublicKey,
			ParentPublicKey: member.ParentPublicKey,
			Source:          "page",
		}, teamID, "member", "bulk-transition", member.AgentID, summary, metadata)
	}
	http.Redirect(w, r, "/teams/"+teamID, http.StatusSeeOther)
}

func handleAPITeamMembers(store teamcore.TeamReader, teamID string, w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		writable, ok := store.(*teamcore.Store)
		if !ok {
			http.Error(w, "team member write path requires writable store", http.StatusInternalServerError)
			return
		}
		handleAPITeamMemberUpdate(writable, teamID, w, r)
		return
	}
	info, err := store.LoadTeamCtx(r.Context(), teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	members, err := store.LoadMembersCtx(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filterStatus := strings.TrimSpace(r.URL.Query().Get("status"))
	filterRole := strings.TrimSpace(r.URL.Query().Get("role"))
	filterAgent := strings.TrimSpace(r.URL.Query().Get("agent"))
	filtered := filterMembers(members, filterStatus, filterRole, filterAgent)
	memberStats, err := store.ComputeMemberStatsCtx(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":        "team-members",
		"team_id":      info.TeamID,
		"member_count": len(filtered),
		"members":      filtered,
		"member_stats": memberStats,
		"applied_filters": map[string]string{
			"status": filterStatus,
			"role":   filterRole,
			"agent":  filterAgent,
		},
		"counts": map[string]int{
			"active":     countMembersByStatus(members, "active"),
			"pending":    countMembersByStatus(members, "pending"),
			"muted":      countMembersByStatus(members, "muted"),
			"removed":    countMembersByStatus(members, "removed"),
			"owner":      countMembersByRole(members, "owner"),
			"maintainer": countMembersByRole(members, "maintainer"),
			"member":     countMembersByRole(members, "member"),
			"observer":   countMembersByRole(members, "observer"),
		},
	})
}

func handleAPITeamPolicyUpdate(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team policy update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	var payload struct {
		teamcore.Policy
		ActorAgentID string `json:"actor_agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	before, err := store.LoadPolicyCtx(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := requireTeamAction(store, teamID, payload.ActorAgentID, "policy.update"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if payload.Permissions == nil {
		payload.Permissions = before.Permissions
	}
	if payload.TaskTransitions == nil {
		payload.TaskTransitions = before.TaskTransitions
	}
	payload.UpdatedAt = time.Now().UTC()
	if err := store.SavePolicyCtx(r.Context(), teamID, payload.Policy); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{AgentID: strings.TrimSpace(payload.ActorAgentID), Source: "api"}, teamID, "policy", "update", "team-policy", "更新 Team Policy", policyHistoryMetadata(before, payload.Policy))
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":   "team-policy",
		"team_id": teamID,
		"policy":  payload.Policy,
	})
}

func handleAPITeamMemberUpdate(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team member update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	var payload struct {
		teamcore.Member
		ActorAgentID string `json:"actor_agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := requireTeamAction(store, teamID, payload.ActorAgentID, "member.update"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	before, _ := loadTeamMember(store, teamID, payload.AgentID)
	if err := upsertTeamMemberCtx(r.Context(), store, teamID, payload.Member); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	after, _ := loadTeamMember(store, teamID, payload.AgentID)
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{
		AgentID:         strings.TrimSpace(payload.ActorAgentID),
		OriginPublicKey: payload.OriginPublicKey,
		ParentPublicKey: payload.ParentPublicKey,
		Source:          "api",
	}, teamID, "member", "update", payload.AgentID, "更新 Team 成员角色或状态", memberHistoryMetadata(before, after))
	members, err := store.LoadMembersCtx(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":   "team-members",
		"team_id": teamID,
		"members": members,
	})
}

func handleAPITeamMemberAction(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !teamRequestTrusted(r) {
		http.Error(w, "team member action is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	var payload struct {
		AgentID      string `json:"agent_id"`
		Action       string `json:"action"`
		ActorAgentID string `json:"actor_agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := requireTeamAction(store, teamID, payload.ActorAgentID, "member.transition"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	member, summary, metadata, err := applyTeamMemberActionCtx(r.Context(), store, teamID, payload.AgentID, payload.Action)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{
		AgentID:         member.AgentID,
		OriginPublicKey: member.OriginPublicKey,
		ParentPublicKey: member.ParentPublicKey,
		Source:          "api",
	}, teamID, "member", "transition", member.AgentID, summary, metadata)
	members, err := store.LoadMembersCtx(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":   "team-member-action",
		"team_id": teamID,
		"member":  member,
		"members": members,
		"summary": summary,
	})
}

func handleAPITeamMemberBulkAction(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !teamRequestTrusted(r) {
		http.Error(w, "team member action is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	var payload struct {
		AgentIDs     []string `json:"agent_ids"`
		Action       string   `json:"action"`
		ActorAgentID string   `json:"actor_agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := requireTeamAction(store, teamID, payload.ActorAgentID, "member.bulk-transition"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	agentIDs := make([]string, 0, len(payload.AgentIDs))
	seen := make(map[string]struct{}, len(payload.AgentIDs))
	for _, agentID := range payload.AgentIDs {
		agentID = strings.TrimSpace(agentID)
		if agentID == "" {
			continue
		}
		if _, ok := seen[agentID]; ok {
			continue
		}
		seen[agentID] = struct{}{}
		agentIDs = append(agentIDs, agentID)
	}
	if len(agentIDs) == 0 {
		http.Error(w, "empty agent_ids", http.StatusBadRequest)
		return
	}
	applied := make([]teamcore.Member, 0, len(agentIDs))
	for _, agentID := range agentIDs {
		member, summary, metadata, err := applyTeamMemberActionCtx(r.Context(), store, teamID, agentID, payload.Action)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		metadata["batch"] = true
		metadata["batch_size"] = len(agentIDs)
		_ = appendTeamHistoryCtx(r.Context(), store, historyActor{
			AgentID:         member.AgentID,
			OriginPublicKey: member.OriginPublicKey,
			ParentPublicKey: member.ParentPublicKey,
			Source:          "api",
		}, teamID, "member", "bulk-transition", member.AgentID, summary, metadata)
		applied = append(applied, member)
	}
	members, err := store.LoadMembersCtx(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":         "team-member-bulk-action",
		"team_id":       teamID,
		"applied":       applied,
		"applied_count": len(applied),
		"members":       members,
	})
}
