package haonewsteam

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"

	teamcore "hao.news/internal/haonews/team"
	newsplugin "hao.news/internal/plugins/haonews"
)

func handleAPITeamMilestones(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := store.ListMilestoneProgressCtx(r.Context(), teamID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
			"scope":      "team-milestones",
			"team_id":    teamID,
			"count":      len(items),
			"milestones": items,
		})
	case http.MethodPost:
		if !teamRequestTrusted(r) {
			http.Error(w, "team milestone update is limited to local or LAN requests", http.StatusForbidden)
			return
		}
		var milestone teamcore.Milestone
		if err := json.NewDecoder(r.Body).Decode(&milestone); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		milestone.TeamID = teamID
		milestone.CreatedAt = time.Now().UTC()
		milestone.UpdatedAt = milestone.CreatedAt
		if err := store.SaveMilestoneCtx(r.Context(), teamID, milestone); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		saved, err := store.LoadMilestoneCtx(r.Context(), teamID, milestone.MilestoneID)
		if err != nil {
			saved = milestone
		}
		progress, _ := store.ListMilestoneProgressCtx(r.Context(), teamID)
		_ = appendTeamHistoryCtx(r.Context(), store, historyActor{Source: "api"}, teamID, "milestone", "create", saved.MilestoneID, "创建 Milestone", map[string]any{
			"title_after":  saved.Title,
			"status_after": saved.Status,
			"due_after":    saved.DueAt,
		})
		newsplugin.WriteJSON(w, http.StatusCreated, map[string]any{
			"scope":      "team-milestone",
			"team_id":    teamID,
			"milestone":  saved,
			"milestones": progress,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAPITeamMilestone(store *teamcore.Store, teamID, milestoneID string, w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		item, err := store.LoadMilestoneCtx(r.Context(), teamID, milestoneID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		progresses, err := store.ListMilestoneProgressCtx(r.Context(), teamID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, progress := range progresses {
			if progress.Milestone.MilestoneID == item.MilestoneID {
				newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
					"scope":     "team-milestone",
					"team_id":   teamID,
					"milestone": progress,
				})
				return
			}
		}
		newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
			"scope":     "team-milestone",
			"team_id":   teamID,
			"milestone": item,
		})
	case http.MethodPut:
		if !teamRequestTrusted(r) {
			http.Error(w, "team milestone update is limited to local or LAN requests", http.StatusForbidden)
			return
		}
		existing, err := store.LoadMilestoneCtx(r.Context(), teamID, milestoneID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var milestone teamcore.Milestone
		if err := json.NewDecoder(r.Body).Decode(&milestone); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		milestone.TeamID = teamID
		milestone.MilestoneID = milestoneID
		if milestone.Title == "" {
			milestone.Title = existing.Title
		}
		if milestone.CreatedAt.IsZero() {
			milestone.CreatedAt = existing.CreatedAt
		}
		milestone.UpdatedAt = time.Now().UTC()
		if err := store.SaveMilestoneCtx(r.Context(), teamID, milestone); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		saved, err := store.LoadMilestoneCtx(r.Context(), teamID, milestoneID)
		if err != nil {
			saved = milestone
		}
		_ = appendTeamHistoryCtx(r.Context(), store, historyActor{Source: "api"}, teamID, "milestone", "update", saved.MilestoneID, "更新 Milestone", map[string]any{
			"title_before":  existing.Title,
			"title_after":   saved.Title,
			"status_before": existing.Status,
			"status_after":  saved.Status,
			"due_before":    existing.DueAt,
			"due_after":     saved.DueAt,
		})
		newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
			"scope":     "team-milestone",
			"team_id":   teamID,
			"milestone": saved,
		})
	case http.MethodDelete:
		if !teamRequestTrusted(r) {
			http.Error(w, "team milestone update is limited to local or LAN requests", http.StatusForbidden)
			return
		}
		if err := store.DeleteMilestoneCtx(r.Context(), teamID, milestoneID); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = appendTeamHistoryCtx(r.Context(), store, historyActor{Source: "api"}, teamID, "milestone", "delete", milestoneID, "删除 Milestone", map[string]any{
			"diff_summary": "删除 Milestone",
		})
		newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
			"scope":        "team-milestone",
			"team_id":      teamID,
			"milestone_id": milestoneID,
			"deleted":      true,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
