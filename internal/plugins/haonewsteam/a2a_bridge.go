package haonewsteam

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	teamcore "hao.news/internal/haonews/team"
	newsplugin "hao.news/internal/plugins/haonews"
)

func handleA2AWellKnownAgent(app *newsplugin.App, store *teamcore.Store, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	teams, err := store.ListTeams()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type agentSkill struct {
		ID   string `json:"id"`
		Name string `json:"name,omitempty"`
	}
	type agentCard struct {
		AgentID      string             `json:"agent_id"`
		Name         string             `json:"name,omitempty"`
		Description  string             `json:"description,omitempty"`
		Version      string             `json:"version,omitempty"`
		PublicKey    string             `json:"public_key,omitempty"`
		Endpoint     string             `json:"endpoint,omitempty"`
		Capabilities teamcore.AgentCaps `json:"capabilities,omitempty"`
		Skills       []agentSkill       `json:"skills,omitempty"`
	}
	aggregated := make([]agentCard, 0)
	skillSeen := make(map[string]agentSkill)
	teamIDs := make([]string, 0, len(teams))
	for _, summary := range teams {
		teamIDs = append(teamIDs, summary.TeamID)
		cards, err := store.ListAgentCards(summary.TeamID)
		if err != nil {
			continue
		}
		for _, card := range cards {
			mapped := agentCard{
				AgentID:      card.AgentID,
				Name:         card.Name,
				Description:  card.Description,
				Version:      card.Version,
				PublicKey:    card.PublicKey,
				Endpoint:     card.Endpoint,
				Capabilities: card.Capabilities,
			}
			if len(card.Skills) > 0 {
				mapped.Skills = make([]agentSkill, 0, len(card.Skills))
				for _, skill := range card.Skills {
					item := agentSkill{ID: skill.ID, Name: skill.Name}
					mapped.Skills = append(mapped.Skills, item)
					if item.ID != "" {
						skillSeen[item.ID] = item
					}
				}
			}
			aggregated = append(aggregated, mapped)
		}
	}
	sort.Strings(teamIDs)
	skills := make([]agentSkill, 0, len(skillSeen))
	for _, item := range skillSeen {
		skills = append(skills, item)
	}
	sort.SliceStable(skills, func(i, j int) bool { return skills[i].ID < skills[j].ID })
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"name":         app.ProjectName() + " Team Bridge",
		"version":      app.VersionString(),
		"capabilities": map[string]any{"streaming": true},
		"teams":        teamIDs,
		"agent_count":  len(aggregated),
		"agents":       aggregated,
		"skills":       skills,
	})
}

func handleA2ATeam(app *newsplugin.App, store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	trimmed := strings.Trim(strings.TrimPrefix(r.URL.Path, "/a2a/teams/"+teamID+"/"), "/")
	switch {
	case trimmed == "message:stream":
		handleAPITeamEvents(store, teamID, w, r)
		return
	case trimmed == "message:send":
		handleA2AMessageSend(store, teamID, w, r)
		return
	case trimmed == "tasks":
		handleA2ATasks(store, teamID, w, r)
		return
	case strings.HasSuffix(trimmed, ":cancel") && strings.HasPrefix(trimmed, "tasks/"):
		taskID := strings.TrimSuffix(strings.TrimPrefix(trimmed, "tasks/"), ":cancel")
		handleA2ATaskCancel(store, teamID, taskID, w, r)
		return
	case strings.HasPrefix(trimmed, "tasks/"):
		taskID := strings.TrimPrefix(trimmed, "tasks/")
		handleA2ATask(store, teamID, taskID, w, r)
		return
	default:
		http.NotFound(w, r)
	}
}

func handleA2ATasks(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tasks, err := store.LoadTasks(teamID, clampTeamListLimit(r.URL.Query().Get("limit"), 50, 100))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, mapTaskToA2A(task))
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":   "a2a-tasks",
		"team_id": teamID,
		"count":   len(out),
		"tasks":   out,
	})
}

func handleA2ATask(store *teamcore.Store, teamID, taskID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	task, err := store.LoadTask(teamID, taskID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":   "a2a-task",
		"team_id": teamID,
		"task":    mapTaskToA2A(task),
	})
}

func handleA2AMessageSend(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !teamRequestTrusted(r) {
		http.Error(w, "a2a message send is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	var payload teamcore.Message
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload.TeamID = teamID
	payload.CreatedAt = time.Now().UTC()
	if err := requireTeamAction(store, teamID, payload.AuthorAgentID, "message.send"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if err := store.AppendMessage(teamID, payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	newsplugin.WriteJSON(w, http.StatusCreated, map[string]any{
		"scope":   "a2a-message",
		"team_id": teamID,
		"message": mapMessageToA2A(payload),
	})
}

func handleA2ATaskCancel(store *teamcore.Store, teamID, taskID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !teamRequestTrusted(r) {
		http.Error(w, "a2a task cancel is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	task, err := store.LoadTask(teamID, taskID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var payload struct {
		ActorAgentID string `json:"actor_agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := requireTeamAction(store, teamID, payload.ActorAgentID, "task.transition"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	task.Status = "cancelled"
	task.UpdatedAt = time.Now().UTC()
	if err := store.SaveTask(teamID, task); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	updated, err := store.LoadTask(teamID, taskID)
	if err != nil {
		updated = task
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":   "a2a-task",
		"team_id": teamID,
		"task":    mapTaskToA2A(updated),
	})
}

func mapTeamStateToA2A(state string) string {
	switch strings.TrimSpace(state) {
	case "open":
		return "submitted"
	case "doing":
		return "working"
	case "blocked":
		return "input-required"
	case "done":
		return "completed"
	case "failed":
		return "failed"
	case "cancelled":
		return "canceled"
	case "rejected":
		return "rejected"
	default:
		return "submitted"
	}
}

func mapTaskToA2A(task teamcore.Task) map[string]any {
	return map[string]any{
		"id":          task.TaskID,
		"team_id":     task.TeamID,
		"context_id":  task.ContextID,
		"channel_id":  task.ChannelID,
		"title":       task.Title,
		"description": task.Description,
		"status":      mapTeamStateToA2A(task.Status),
		"priority":    task.Priority,
		"assignees":   task.Assignees,
		"labels":      task.Labels,
		"created_by":  task.CreatedBy,
		"created_at":  task.CreatedAt,
		"updated_at":  task.UpdatedAt,
		"closed_at":   task.ClosedAt,
	}
}

func mapMessageToA2A(msg teamcore.Message) map[string]any {
	return map[string]any{
		"id":              msg.MessageID,
		"team_id":         msg.TeamID,
		"channel_id":      msg.ChannelID,
		"context_id":      msg.ContextID,
		"author_agent_id": msg.AuthorAgentID,
		"message_type":    msg.MessageType,
		"content":         msg.Content,
		"parts":           msg.Parts,
		"references":      msg.References,
		"created_at":      msg.CreatedAt,
	}
}
