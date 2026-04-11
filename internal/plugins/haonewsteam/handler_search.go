package haonewsteam

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	teamcore "hao.news/internal/haonews/team"
	newsplugin "hao.news/internal/plugins/haonews"
)

const teamSearchSectionLimit = 12

func handleTeamSearch(app *newsplugin.App, store teamcore.TeamReader, teamID string, w http.ResponseWriter, r *http.Request) {
	info, err := store.LoadTeamCtx(r.Context(), teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	scope := normalizeTeamSearchScope(r.URL.Query().Get("scope"))
	sections, scopeOptions, resultSummary, err := buildTeamSearchSections(r.Context(), store, teamID, query, scope)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := teamSearchPageData{
		Project:       app.ProjectName(),
		Version:       app.VersionString(),
		PageNav:       app.PageNav("/teams"),
		NodeStatus:    app.NodeStatus(index),
		Now:           time.Now(),
		Team:          info,
		Query:         query,
		Scope:         scope,
		ScopeOptions:  scopeOptions,
		Sections:      sections,
		ResultSummary: resultSummary,
		SearchTips: []string{
			"任务和产物按当前 Team 对象检索；消息会按现有频道扫描；历史会按 Team 自己的 change events 检索。",
			"如果你已经知道 task_id 或 context_id，先搜任务标题或上下文关键词，再跳回相关频道/历史会更快。",
		},
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "检索范围", Value: teamSearchScopeLabel(scope)},
			{Label: "命中分组", Value: formatTeamCount(len(nonEmptySearchSections(sections)))},
			{Label: "展示上限", Value: formatTeamCount(teamSearchSectionLimit)},
		},
	}
	if err := app.Templates().ExecuteTemplate(w, "team_search.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleAPITeamSearch(store teamcore.TeamReader, teamID string, w http.ResponseWriter, r *http.Request) {
	if _, err := store.LoadTeamCtx(r.Context(), teamID); err != nil {
		http.NotFound(w, r)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	scope := normalizeTeamSearchScope(r.URL.Query().Get("scope"))
	sections, scopeOptions, resultSummary, err := buildTeamSearchSections(r.Context(), store, teamID, query, scope)
	if err != nil {
		if resp, ok := classifyTeamAPIError(teamID, err); ok {
			writeTeamAPIError(w, http.StatusBadRequest, resp)
			return
		}
		writeTeamAPIError(w, http.StatusInternalServerError, teamErrorResponse{
			Error:   "team_search_failed",
			Message: "Team 搜索失败。",
			Help:    "请稍后重试；如果持续失败，先确认 Team 数据文件可读。",
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"team_id":        teamID,
		"query":          query,
		"scope":          scope,
		"scope_options":  scopeOptions,
		"result_summary": resultSummary,
		"sections":       sections,
	})
}

func buildTeamSearchSections(ctx context.Context, store teamcore.TeamReader, teamID, query, scope string) ([]teamSearchSectionView, []teamSearchScopeOption, string, error) {
	query = strings.TrimSpace(query)
	scope = normalizeTeamSearchScope(scope)
	sections := []teamSearchSectionView{
		{Key: "tasks", Title: "任务", Hint: "任务标题、描述、负责者、标签、优先级"},
		{Key: "artifacts", Title: "产物", Hint: "产物标题、摘要、正文、标签、链接"},
		{Key: "messages", Title: "消息", Hint: "Team 频道消息内容、作者、上下文"},
		{Key: "history", Title: "历史", Hint: "治理、任务、产物、成员和同步变更"},
	}
	if query == "" {
		return sections, buildTeamSearchScopeOptions(scope, sections), "输入关键词后开始检索 Team 内的任务、产物、消息和历史。", nil
	}
	if scope == "all" || scope == "tasks" {
		items, err := store.LoadTasksCtx(ctx, teamID, 0)
		if err != nil {
			return nil, nil, "", err
		}
		sections[0].Results, sections[0].Count = searchTeamTasks(items, teamID, query)
	}
	if scope == "all" || scope == "artifacts" {
		items, err := store.LoadArtifactsCtx(ctx, teamID, 0)
		if err != nil {
			return nil, nil, "", err
		}
		sections[1].Results, sections[1].Count = searchTeamArtifacts(items, teamID, query)
	}
	if scope == "all" || scope == "messages" {
		channels, err := store.ListChannelsCtx(ctx, teamID)
		if err != nil {
			return nil, nil, "", err
		}
		results, count, err := searchTeamMessages(ctx, store, teamID, channels, query)
		if err != nil {
			return nil, nil, "", err
		}
		sections[2].Results, sections[2].Count = results, count
	}
	if scope == "all" || scope == "history" {
		items, err := store.LoadHistoryCtx(ctx, teamID, 0)
		if err != nil {
			return nil, nil, "", err
		}
		sections[3].Results, sections[3].Count = searchTeamHistory(items, teamID, query)
	}
	scopeOptions := buildTeamSearchScopeOptions(scope, sections)
	return sections, scopeOptions, buildTeamSearchResultSummary(query, sections), nil
}

func searchTeamTasks(tasks []teamcore.Task, teamID, query string) ([]teamSearchResultView, int) {
	needle := normalizeTeamSearchQuery(query)
	results := make([]teamSearchResultView, 0, teamSearchSectionLimit)
	matches := make([]teamSearchResultView, 0)
	for _, task := range tasks {
		if !teamSearchMatch(needle,
			task.TaskID,
			task.Title,
			task.Description,
			task.ChannelID,
			task.ContextID,
			task.Status,
			task.Priority,
			strings.Join(task.Assignees, " "),
			strings.Join(task.Labels, " "),
		) {
			continue
		}
		matches = append(matches, teamSearchResultView{
			Kind:      "task",
			ID:        task.TaskID,
			Title:     task.Title,
			Summary:   firstNonEmpty(task.Description, "任务状态 "+task.Status),
			URL:       "/teams/" + teamID + "/tasks/" + task.TaskID,
			Meta:      strings.TrimSpace(strings.Join(compactTeamSearchParts(task.Status, task.Priority, formatTaskDue(task.DueAt), strings.Join(task.Assignees, ", ")), " · ")),
			CreatedAt: latestTaskTime(task),
		})
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].CreatedAt.After(matches[j].CreatedAt) })
	results = trimTeamSearchResults(matches, teamSearchSectionLimit)
	return results, len(matches)
}

func searchTeamArtifacts(items []teamcore.Artifact, teamID, query string) ([]teamSearchResultView, int) {
	needle := normalizeTeamSearchQuery(query)
	matches := make([]teamSearchResultView, 0)
	for _, artifact := range items {
		if !teamSearchMatch(needle,
			artifact.ArtifactID,
			artifact.Title,
			artifact.Kind,
			artifact.Summary,
			artifact.Content,
			artifact.ChannelID,
			artifact.TaskID,
			artifact.LinkURL,
			strings.Join(artifact.Labels, " "),
		) {
			continue
		}
		matches = append(matches, teamSearchResultView{
			Kind:      "artifact",
			ID:        artifact.ArtifactID,
			Title:     artifact.Title,
			Summary:   firstNonEmpty(artifact.Summary, trimTeamSearchText(artifact.Content, 120)),
			URL:       "/teams/" + teamID + "/artifacts/" + artifact.ArtifactID,
			Meta:      strings.TrimSpace(strings.Join(compactTeamSearchParts(artifact.Kind, artifact.ChannelID, artifact.TaskID), " · ")),
			CreatedAt: latestArtifactTime(artifact),
		})
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].CreatedAt.After(matches[j].CreatedAt) })
	return trimTeamSearchResults(matches, teamSearchSectionLimit), len(matches)
}

func searchTeamMessages(ctx context.Context, store teamcore.TeamReader, teamID string, channels []teamcore.ChannelSummary, query string) ([]teamSearchResultView, int, error) {
	needle := normalizeTeamSearchQuery(query)
	matches := make([]teamSearchResultView, 0)
	for _, channel := range channels {
		messages, err := store.LoadAllMessagesCtx(ctx, teamID, channel.ChannelID)
		if err != nil {
			return nil, 0, err
		}
		for _, msg := range messages {
			if !teamSearchMatch(needle,
				msg.MessageID,
				msg.ChannelID,
				msg.ContextID,
				msg.AuthorAgentID,
				msg.MessageType,
				msg.Content,
			) {
				continue
			}
			title := msg.AuthorAgentID
			if title == "" {
				title = "消息"
			}
			matches = append(matches, teamSearchResultView{
				Kind:      "message",
				ID:        msg.MessageID,
				Title:     title,
				Summary:   trimTeamSearchText(msg.Content, 120),
				URL:       "/teams/" + teamID + "/channels/" + firstNonEmpty(msg.ChannelID, "main"),
				Meta:      strings.TrimSpace(strings.Join(compactTeamSearchParts(msg.ChannelID, msg.MessageType, msg.ContextID), " · ")),
				CreatedAt: msg.CreatedAt,
			})
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].CreatedAt.After(matches[j].CreatedAt) })
	return trimTeamSearchResults(matches, teamSearchSectionLimit), len(matches), nil
}

func searchTeamHistory(items []teamcore.ChangeEvent, teamID, query string) ([]teamSearchResultView, int) {
	needle := normalizeTeamSearchQuery(query)
	matches := make([]teamSearchResultView, 0)
	for _, event := range items {
		if !teamSearchMatch(needle,
			event.EventID,
			event.Scope,
			event.Action,
			event.SubjectID,
			event.Summary,
			event.ActorAgentID,
			event.Source,
		) {
			continue
		}
		matches = append(matches, teamSearchResultView{
			Kind:      "history",
			ID:        event.EventID,
			Title:     firstNonEmpty(event.Summary, event.Scope+"."+event.Action),
			Summary:   strings.TrimSpace(strings.Join(compactTeamSearchParts(event.Scope, event.Action, event.SubjectID), " · ")),
			URL:       "/teams/" + teamID + "/history?scope=" + event.Scope,
			Meta:      strings.TrimSpace(strings.Join(compactTeamSearchParts(event.ActorAgentID, event.Source), " · ")),
			CreatedAt: event.CreatedAt,
		})
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].CreatedAt.After(matches[j].CreatedAt) })
	return trimTeamSearchResults(matches, teamSearchSectionLimit), len(matches)
}

func normalizeTeamSearchScope(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "tasks", "artifacts", "messages", "history":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "all"
	}
}

func teamSearchScopeLabel(scope string) string {
	switch scope {
	case "tasks":
		return "仅任务"
	case "artifacts":
		return "仅产物"
	case "messages":
		return "仅消息"
	case "history":
		return "仅历史"
	default:
		return "全部"
	}
}

func buildTeamSearchScopeOptions(active string, sections []teamSearchSectionView) []teamSearchScopeOption {
	counts := map[string]int{}
	for _, section := range sections {
		counts[section.Key] = section.Count
	}
	options := []teamSearchScopeOption{
		{Value: "all", Label: "全部", Active: active == "all", Count: totalSearchSectionCounts(sections)},
		{Value: "tasks", Label: "任务", Active: active == "tasks", Count: counts["tasks"]},
		{Value: "artifacts", Label: "产物", Active: active == "artifacts", Count: counts["artifacts"]},
		{Value: "messages", Label: "消息", Active: active == "messages", Count: counts["messages"]},
		{Value: "history", Label: "历史", Active: active == "history", Count: counts["history"]},
	}
	return options
}

func buildTeamSearchResultSummary(query string, sections []teamSearchSectionView) string {
	if strings.TrimSpace(query) == "" {
		return "输入关键词后开始检索。"
	}
	total := totalSearchSectionCounts(sections)
	if total == 0 {
		return "没有找到匹配结果。建议换一个更具体的任务标题、成员、频道或上下文关键词。"
	}
	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		if section.Count == 0 {
			continue
		}
		parts = append(parts, section.Title+" "+formatTeamCount(section.Count))
	}
	return "关键词 “" + query + "” 共命中 " + formatTeamCount(total) + " 条，分布在：" + strings.Join(parts, "、") + "。"
}

func normalizeTeamSearchQuery(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func teamSearchMatch(needle string, fields ...string) bool {
	if needle == "" {
		return false
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(strings.TrimSpace(field)), needle) {
			return true
		}
	}
	return false
}

func trimTeamSearchResults(items []teamSearchResultView, limit int) []teamSearchResultView {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func totalSearchSectionCounts(sections []teamSearchSectionView) int {
	total := 0
	for _, section := range sections {
		total += section.Count
	}
	return total
}

func nonEmptySearchSections(sections []teamSearchSectionView) []teamSearchSectionView {
	out := make([]teamSearchSectionView, 0, len(sections))
	for _, section := range sections {
		if section.Count > 0 {
			out = append(out, section)
		}
	}
	return out
}

func trimTeamSearchText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit]) + "..."
}

func compactTeamSearchParts(parts ...string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func latestTaskTime(task teamcore.Task) time.Time {
	if task.UpdatedAt.After(task.CreatedAt) {
		return task.UpdatedAt
	}
	return task.CreatedAt
}

func latestArtifactTime(artifact teamcore.Artifact) time.Time {
	if artifact.UpdatedAt.After(artifact.CreatedAt) {
		return artifact.UpdatedAt
	}
	return artifact.CreatedAt
}

func firstNonEmpty(parts ...string) string {
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			return part
		}
	}
	return ""
}
