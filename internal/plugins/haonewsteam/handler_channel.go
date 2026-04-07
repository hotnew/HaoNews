package haonewsteam

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	teamcore "hao.news/internal/haonews/team"
	newsplugin "hao.news/internal/plugins/haonews"
	"hao.news/internal/plugins/haonewsteam/roomplugin"
	roomthemes "hao.news/internal/themes/room-themes"
)

func handleTeamChannel(app *newsplugin.App, store *teamcore.Store, roomRegistry *roomplugin.Registry, themeRegistry *roomthemes.Registry, teamID, channelID string, w http.ResponseWriter, r *http.Request) {
	info, err := store.LoadTeamCtx(r.Context(), teamID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	current, err := store.LoadChannelCtx(r.Context(), teamID, channelID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	messages, err := store.LoadMessagesCtx(r.Context(), teamID, channelID, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tasks, err := store.LoadTasksCtx(r.Context(), teamID, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	artifacts, err := store.LoadArtifactsCtx(r.Context(), teamID, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	channels, err := store.ListChannelsCtx(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	currentSummary := teamcore.ChannelSummary{Channel: current}
	for _, channel := range channels {
		if normalizeTeamChannel(channel.ChannelID) == normalizeTeamChannel(channelID) {
			currentSummary = channel
			break
		}
	}
	history, err := store.LoadHistoryCtx(r.Context(), teamID, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := teamChannelPageData{
		Project:              app.ProjectName(),
		Version:              app.VersionString(),
		PageNav:              app.PageNav("/teams"),
		NodeStatus:           app.NodeStatus(index),
		Now:                  time.Now(),
		Team:                 info,
		Channel:              currentSummary,
		ChannelID:            channelID,
		ViewMode:             strings.TrimSpace(r.URL.Query().Get("view")),
		Channels:             channels,
		Messages:             messages,
		Tasks:                relatedTasksByChannel(tasks, channelID, 12),
		Artifacts:            relatedArtifactsByChannel(artifacts, channelID, 12),
		ChannelConfig:        loadChannelConfigSafe(r.Context(), store, teamID, channelID),
		AvailableRoomPlugins: buildRoomPluginSummaries(roomRegistry),
		AvailableRoomThemes:  buildRoomThemeSummaries(themeRegistry),
		ConfigNotice:         roomConfigNoticeLabel(strings.TrimSpace(r.URL.Query().Get("room_notice"))),
		RelatedHistory:       channelHistory(history, channelID, 12),
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "频道", Value: current.Title},
			{Label: "消息", Value: formatTeamCount(len(messages))},
			{Label: "任务", Value: formatTeamCount(countTasksByChannel(tasks, channelID))},
			{Label: "产物", Value: formatTeamCount(countArtifactsByChannel(artifacts, channelID))},
			{Label: "可见性", Value: info.Visibility},
			{Label: "状态", Value: channelStateLabel(current.Hidden)},
		},
	}
	pluginByID := roomPluginSummaryMap(roomRegistry)
	themeByID := roomThemeSummaryMap(themeRegistry)
	data.RoomEntry = buildTeamRoomEntry(teamID, currentSummary, data.ChannelConfig, strings.TrimSpace(data.ChannelConfig.Plugin) != "" || strings.TrimSpace(data.ChannelConfig.Theme) != "")
	data.CurrentRoomPlugin = pluginByID[data.ChannelConfig.PluginID()]
	data.CurrentRoomTheme = themeByID[strings.TrimSpace(data.ChannelConfig.Theme)]
	if err := renderTeamChannelPage(app, themeRegistry, w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func renderTeamChannelPage(app *newsplugin.App, themeRegistry *roomthemes.Registry, w http.ResponseWriter, data teamChannelPageData) error {
	themeID := strings.TrimSpace(data.ChannelConfig.Theme)
	if themeID != "" && themeRegistry != nil && strings.TrimSpace(data.ViewMode) != "channel" {
		if theme, ok := themeRegistry.Get(themeID); ok {
			tmpl, err := theme.Templates(template.FuncMap{
				"structuredJSON": func(value map[string]any) string {
					body, err := json.MarshalIndent(value, "", "  ")
					if err != nil {
						return fmt.Sprintf("%v", value)
					}
					return string(body)
				},
				"roomMessageKindLabel": func(messageType string) string {
					switch strings.TrimSpace(messageType) {
					case "plan":
						return "[PLAN]"
					case "skill":
						return "[SKILL]"
					case "snippet":
						return "[SNIPPET]"
					default:
						return "[MESSAGE]"
					}
				},
			})
			if err == nil {
				if err := tmpl.ExecuteTemplate(w, "room_channel.html", data); err == nil {
					return nil
				}
			}
		}
	}
	if err := app.Templates().ExecuteTemplate(w, "room_channel_default.html", data); err == nil {
		return nil
	}
	return app.Templates().ExecuteTemplate(w, "team_channel.html", data)
}

func loadChannelConfigSafe(ctx context.Context, store *teamcore.Store, teamID, channelID string) teamcore.ChannelConfig {
	if store == nil {
		return teamcore.ChannelConfig{ChannelID: channelID}
	}
	cfg, err := store.LoadChannelConfigCtx(ctx, teamID, channelID)
	if err != nil {
		return teamcore.ChannelConfig{ChannelID: channelID}
	}
	if strings.TrimSpace(cfg.ChannelID) == "" {
		cfg.ChannelID = channelID
	}
	return cfg
}

func handleTeamChannelCreate(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team channel update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	channel := teamcore.Channel{
		ChannelID:   strings.TrimSpace(r.FormValue("channel_id")),
		Title:       strings.TrimSpace(r.FormValue("title")),
		Description: strings.TrimSpace(r.FormValue("description")),
		Hidden:      false,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := requireTeamAction(store, teamID, strings.TrimSpace(r.FormValue("actor_agent_id")), "channel.create"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if err := store.SaveChannelCtx(r.Context(), teamID, channel); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	channel, _ = store.LoadChannelCtx(r.Context(), teamID, channel.ChannelID)
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{Source: "page"}, teamID, "channel", "create", channel.ChannelID, "创建 Team Channel", channelHistoryMetadata(teamcore.Channel{}, channel))
	http.Redirect(w, r, "/teams/"+teamID+"/channels/"+channel.ChannelID, http.StatusSeeOther)
}

func handleTeamChannelUpdate(store *teamcore.Store, teamID, channelID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team channel update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	before, err := store.LoadChannelCtx(r.Context(), teamID, channelID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	updated := before
	updated.Title = strings.TrimSpace(r.FormValue("title"))
	updated.Description = strings.TrimSpace(r.FormValue("description"))
	updated.Hidden = r.FormValue("hidden") == "true" || r.FormValue("hidden") == "on"
	updated.UpdatedAt = time.Now().UTC()
	if err := requireTeamAction(store, teamID, strings.TrimSpace(r.FormValue("actor_agent_id")), "channel.update"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if err := store.SaveChannelCtx(r.Context(), teamID, updated); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	after, _ := store.LoadChannelCtx(r.Context(), teamID, channelID)
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{Source: "page"}, teamID, "channel", "update", channelID, "更新 Team Channel", channelHistoryMetadata(before, after))
	http.Redirect(w, r, "/teams/"+teamID+"/channels/"+channelID, http.StatusSeeOther)
}

func handleTeamChannelHide(store *teamcore.Store, teamID, channelID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team channel update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	before, err := store.LoadChannelCtx(r.Context(), teamID, channelID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := requireTeamAction(store, teamID, strings.TrimSpace(r.FormValue("actor_agent_id")), "channel.hide"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if err := store.HideChannelCtx(r.Context(), teamID, channelID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	after, _ := store.LoadChannelCtx(r.Context(), teamID, channelID)
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{Source: "page"}, teamID, "channel", "hide", channelID, "隐藏 Team Channel", channelHistoryMetadata(before, after))
	http.Redirect(w, r, "/teams/"+teamID, http.StatusSeeOther)
}

func handleAPITeamChannels(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		channels, err := store.ListChannelsCtx(r.Context(), teamID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
			"scope":         "team-channels",
			"team_id":       teamID,
			"channel_count": len(channels),
			"channels":      channels,
		})
	case http.MethodPost:
		handleAPITeamChannelCreate(store, teamID, w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAPITeamChannel(store *teamcore.Store, roomRegistry *roomplugin.Registry, themeRegistry *roomthemes.Registry, teamID, channelID string, w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		channel, err := store.LoadChannelCtx(r.Context(), teamID, channelID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		cfg := loadChannelConfigSafe(r.Context(), store, teamID, channelID)
		roomEntry := buildTeamRoomEntry(teamID, teamcore.ChannelSummary{Channel: channel}, cfg, strings.TrimSpace(cfg.Plugin) != "" || strings.TrimSpace(cfg.Theme) != "")
		newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
			"scope":                  "team-channel",
			"team_id":                teamID,
			"channel":                channel,
			"room_plugin_id":         roomEntry.PluginID,
			"room_plugin_route":      roomEntry.RoomWebPath,
			"room_plugin_api":        roomEntry.RoomAPIPath,
			"available_room_plugins": buildRoomPluginSummaries(roomRegistry),
			"room_theme_id":          roomEntry.Theme,
			"available_room_themes":  buildRoomThemeSummaries(themeRegistry),
			"channel_config_state":   map[bool]string{true: "configured", false: "unconfigured"}[roomEntry.Configured],
			"room_entry":             roomEntry,
		})
	case http.MethodPut:
		handleAPITeamChannelUpdate(store, teamID, channelID, w, r)
	case http.MethodDelete:
		handleAPITeamChannelDelete(store, teamID, channelID, w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleTeamChannelConfigUpdate(store *teamcore.Store, roomRegistry *roomplugin.Registry, themeRegistry *roomthemes.Registry, teamID, channelID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "channel config update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	actorAgentID := strings.TrimSpace(r.FormValue("actor_agent_id"))
	if err := requireTeamAction(store, teamID, actorAgentID, "channel.update"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	cfg := loadChannelConfigSafe(r.Context(), store, teamID, channelID)
	action := strings.TrimSpace(r.FormValue("action"))
	if action == "clear" {
		cfg.Plugin = ""
		cfg.Theme = ""
		cfg.AgentOnboarding = ""
		cfg.Rules = nil
		cfg.ThemeConfig = nil
		cfg.Metadata = nil
	} else {
		pluginChoice := strings.TrimSpace(r.FormValue("plugin"))
		if pluginChoice != "" {
			summary, ok := roomPluginSummaryMap(roomRegistry)[pluginChoice]
			if !ok {
				http.Error(w, "unknown room plugin", http.StatusBadRequest)
				return
			}
			cfg.Plugin = summary.ConfigValue
		} else {
			cfg.Plugin = ""
		}
		themeChoice := strings.TrimSpace(r.FormValue("theme"))
		if themeChoice != "" {
			if _, ok := roomThemeSummaryMap(themeRegistry)[themeChoice]; !ok {
				http.Error(w, "unknown room theme", http.StatusBadRequest)
				return
			}
		}
		cfg.Theme = themeChoice
		cfg.AgentOnboarding = strings.TrimSpace(r.FormValue("agent_onboarding"))
		cfg.Rules = parseConfigLines(r.FormValue("rules"))
	}
	cfg.ChannelID = channelID
	if err := store.SaveChannelConfigCtx(r.Context(), teamID, cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	notice := "saved"
	if action == "clear" {
		notice = "cleared"
	}
	http.Redirect(w, r, "/teams/"+teamID+"/channels/"+channelID+"?room_notice="+url.QueryEscape(notice), http.StatusSeeOther)
}

func parseConfigLines(raw string) []string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	parts := strings.Split(raw, "\n")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func roomConfigNoticeLabel(value string) string {
	switch value {
	case "saved":
		return "Room Plugin 配置已更新。当前频道会按新的 plugin/theme 入口渲染。"
	case "cleared":
		return "Room Plugin 配置已清空，频道已回退为普通 Team Channel。"
	default:
		return ""
	}
}

func handleAPITeamChannelMessages(store *teamcore.Store, teamID, channelID string, w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		handleAPITeamChannelMessageCreate(store, teamID, channelID, w, r)
		return
	}
	limit := clampTeamListLimit(r.URL.Query().Get("limit"), 100, 200)
	messages, err := store.LoadMessagesCtx(r.Context(), teamID, channelID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":         "team-channel-messages",
		"team_id":       teamID,
		"channel_id":    channelID,
		"limit":         limit,
		"message_count": len(messages),
		"messages":      messages,
	})
}

func handleAPITeamChannelConfig(store *teamcore.Store, teamID, channelID string, w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := store.LoadChannelConfigCtx(r.Context(), teamID, channelID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		newsplugin.WriteJSON(w, http.StatusOK, cfg)
	case http.MethodPut:
		if !teamRequestTrusted(r) {
			http.Error(w, "channel config update is limited to local or LAN requests", http.StatusForbidden)
			return
		}
		actorAgentID := strings.TrimSpace(r.Header.Get("X-Actor-Agent-ID"))
		if actorAgentID == "" {
			actorAgentID = strings.TrimSpace(r.URL.Query().Get("actor_agent_id"))
		}
		if err := requireTeamAction(store, teamID, actorAgentID, "channel.update"); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		var cfg teamcore.ChannelConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cfg.ChannelID = channelID
		if err := store.SaveChannelConfigCtx(r.Context(), teamID, cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		saved, err := store.LoadChannelConfigCtx(r.Context(), teamID, channelID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		newsplugin.WriteJSON(w, http.StatusOK, saved)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAPITeamChannelConfigs(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	configs, err := store.ListChannelConfigsCtx(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if configs == nil {
		configs = []teamcore.ChannelConfig{}
	}
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":   "team-channel-configs",
		"team_id": teamID,
		"count":   len(configs),
		"configs": configs,
	})
}

func handleTeamChannelMessageCreate(store *teamcore.Store, teamID, channelID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team channel message is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	structuredData, err := parseOptionalStructuredData(r.FormValue("structured_data"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	msg := teamcore.Message{
		TeamID:          teamID,
		ChannelID:       channelID,
		ContextID:       strings.TrimSpace(r.FormValue("context_id")),
		AuthorAgentID:   strings.TrimSpace(r.FormValue("author_agent_id")),
		OriginPublicKey: strings.TrimSpace(r.FormValue("origin_public_key")),
		ParentPublicKey: strings.TrimSpace(r.FormValue("parent_public_key")),
		MessageType:     strings.TrimSpace(r.FormValue("message_type")),
		Content:         strings.TrimSpace(r.FormValue("content")),
		StructuredData:  structuredData,
		CreatedAt:       time.Now().UTC(),
	}
	if err := requireTeamAction(store, teamID, msg.AuthorAgentID, "message.send"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if err := store.AppendMessageCtx(r.Context(), teamID, msg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{
		AgentID:         msg.AuthorAgentID,
		OriginPublicKey: msg.OriginPublicKey,
		ParentPublicKey: msg.ParentPublicKey,
		Source:          "page",
	}, teamID, "message", "create", channelID+":"+msg.CreatedAt.UTC().Format(time.RFC3339Nano), "发送 TeamMessage", map[string]any{
		"channel_id":    channelID,
		"message_type":  blankDash(msg.MessageType),
		"author_agent":  msg.AuthorAgentID,
		"diff_summary":  "Team 消息已发送到频道",
		"message_scope": "team-message",
	})
	http.Redirect(w, r, "/teams/"+teamID+"/channels/"+channelID, http.StatusSeeOther)
}

func handleAPITeamChannelMessageCreate(store *teamcore.Store, teamID, channelID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team channel message is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	var payload teamcore.Message
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload.TeamID = teamID
	payload.ChannelID = channelID
	payload.CreatedAt = time.Now().UTC()
	if err := requireTeamAction(store, teamID, payload.AuthorAgentID, "message.send"); err != nil {
		if resp, ok := classifyTeamAPIError(teamID, err); ok {
			writeTeamAPIError(w, http.StatusForbidden, resp)
			return
		}
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if err := store.AppendMessageCtx(r.Context(), teamID, payload); err != nil {
		if resp, ok := classifyTeamAPIError(teamID, err); ok {
			writeTeamAPIError(w, http.StatusBadRequest, resp)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{
		AgentID:         payload.AuthorAgentID,
		OriginPublicKey: payload.OriginPublicKey,
		ParentPublicKey: payload.ParentPublicKey,
		Source:          "api",
	}, teamID, "message", "create", channelID+":"+payload.CreatedAt.UTC().Format(time.RFC3339Nano), "发送 TeamMessage", map[string]any{
		"channel_id":    channelID,
		"message_type":  blankDash(payload.MessageType),
		"author_agent":  payload.AuthorAgentID,
		"diff_summary":  "Team 消息已发送到频道",
		"message_scope": "team-message",
	})
	newsplugin.WriteJSON(w, http.StatusCreated, map[string]any{
		"scope":   "team-message",
		"team_id": teamID,
		"message": payload,
	})
}

func handleAPITeamChannelCreate(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team channel update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	var payload struct {
		teamcore.Channel
		ActorAgentID string `json:"actor_agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload.CreatedAt = time.Now().UTC()
	payload.UpdatedAt = payload.CreatedAt
	if err := requireTeamAction(store, teamID, payload.ActorAgentID, "channel.create"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if err := store.SaveChannelCtx(r.Context(), teamID, payload.Channel); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	channel, _ := store.LoadChannelCtx(r.Context(), teamID, payload.ChannelID)
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{Source: "api"}, teamID, "channel", "create", channel.ChannelID, "创建 Team Channel", channelHistoryMetadata(teamcore.Channel{}, channel))
	newsplugin.WriteJSON(w, http.StatusCreated, map[string]any{
		"scope":   "team-channel",
		"team_id": teamID,
		"channel": channel,
	})
}

func handleAPITeamChannelUpdate(store *teamcore.Store, teamID, channelID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team channel update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	before, err := store.LoadChannelCtx(r.Context(), teamID, channelID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var payload struct {
		teamcore.Channel
		ActorAgentID string `json:"actor_agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload.ChannelID = channelID
	payload.UpdatedAt = time.Now().UTC()
	if err := requireTeamAction(store, teamID, payload.ActorAgentID, "channel.update"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if err := store.SaveChannelCtx(r.Context(), teamID, payload.Channel); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	after, _ := store.LoadChannelCtx(r.Context(), teamID, channelID)
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{Source: "api"}, teamID, "channel", "update", channelID, "更新 Team Channel", channelHistoryMetadata(before, after))
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":   "team-channel",
		"team_id": teamID,
		"channel": after,
	})
}

func handleAPITeamChannelDelete(store *teamcore.Store, teamID, channelID string, w http.ResponseWriter, r *http.Request) {
	if !teamRequestTrusted(r) {
		http.Error(w, "team channel update is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	before, err := store.LoadChannelCtx(r.Context(), teamID, channelID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var payload struct {
		ActorAgentID string `json:"actor_agent_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)
	if err := requireTeamAction(store, teamID, payload.ActorAgentID, "channel.hide"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if err := store.HideChannelCtx(r.Context(), teamID, channelID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	after, _ := store.LoadChannelCtx(r.Context(), teamID, channelID)
	_ = appendTeamHistoryCtx(r.Context(), store, historyActor{Source: "api"}, teamID, "channel", "hide", channelID, "隐藏 Team Channel", channelHistoryMetadata(before, after))
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"scope":   "team-channel",
		"team_id": teamID,
		"channel": after,
		"deleted": true,
	})
}
