package planexchange

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"

	teamcore "hao.news/internal/haonews/team"
)

//go:embed templates/*.html
var templatesFS embed.FS

type planExchangeCardView struct {
	MessageID       string
	Kind            string
	KindLabel       string
	Title           string
	Content         string
	AuthorAgentID   string
	ChannelID       string
	CreatedAt       time.Time
	StructuredJSON  string
	Distilled       bool
	DistillArtifact string
}

type planExchangePageData struct {
	TeamID        string
	ChannelID     string
	FilterKind    string
	ActorAgentID  string
	Notice        string
	Cards         []planExchangeCardView
	MessageCount  int
	PluginVersion string
}

func newHandler(store *teamcore.Store, teamID string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "" {
			http.NotFound(w, r)
			return
		}
		handleListPlanExchangeMessages(store, teamID, w, r)
	})
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handlePostPlanExchangeMessage(store, teamID, w, r)
	})
	mux.HandleFunc("/distill", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleDistillSkillToArtifact(store, teamID, w, r)
	})
	return mux
}

func handleListPlanExchangeMessages(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	channelID := strings.TrimSpace(r.URL.Query().Get("channel_id"))
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	actorAgentID := strings.TrimSpace(r.URL.Query().Get("actor_agent_id"))
	if channelID == "" {
		channelID = "main"
	}
	messages, err := store.LoadMessagesCtx(r.Context(), teamID, channelID, 200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filtered := filterPlanExchangeMessages(messages, kind)
	if isAPIRequest(r) {
		if filtered == nil {
			filtered = []teamcore.Message{}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(filtered)
		return
	}
	data, err := buildPlanExchangePageData(r.Context(), store, teamID, channelID, actorAgentID, kind, strings.TrimSpace(r.URL.Query().Get("notice")), filtered)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := renderPlanExchangePage(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type postPlanExchangeRequest struct {
	ChannelID      string         `json:"channel_id"`
	AuthorAgentID  string         `json:"author_agent_id"`
	Kind           string         `json:"kind"`
	Content        string         `json:"content"`
	StructuredData map[string]any `json:"structured_data"`
}

func handlePostPlanExchangeMessage(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !requestTrusted(r) {
		http.Error(w, "plan-exchange message write is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	req, err := decodePostPlanExchangeRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	validKinds := map[string]bool{"plan": true, "skill": true, "snippet": true}
	if !validKinds[req.Kind] {
		http.Error(w, "kind must be plan, skill, or snippet", http.StatusBadRequest)
		return
	}
	if req.ChannelID == "" {
		req.ChannelID = "main"
	}
	if req.AuthorAgentID == "" {
		http.Error(w, "author_agent_id is required", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		req.Content = planExchangeStructuredTitle(req.StructuredData)
	}
	if err := requireAction(store, teamID, req.AuthorAgentID, "message.send"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	msg := teamcore.Message{
		TeamID:         teamID,
		ChannelID:      req.ChannelID,
		AuthorAgentID:  req.AuthorAgentID,
		MessageType:    req.Kind,
		Content:        req.Content,
		StructuredData: req.StructuredData,
		CreatedAt:      time.Now().UTC(),
	}
	if err := store.AppendMessageCtx(r.Context(), teamID, msg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = store.AppendHistoryCtx(r.Context(), teamID, teamcore.ChangeEvent{
		TeamID:    teamID,
		Scope:     "message",
		Action:    "create",
		SubjectID: req.ChannelID + ":" + msg.CreatedAt.UTC().Format(time.RFC3339Nano),
		Summary:   "发送 plan-exchange 消息",
		Metadata: map[string]any{
			"channel_id":    req.ChannelID,
			"message_type":  req.Kind,
			"author_agent":  req.AuthorAgentID,
			"message_scope": "plan-exchange",
		},
		CreatedAt: msg.CreatedAt,
	})
	if isAPIRequest(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "created", "kind": req.Kind})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%s/r/plan-exchange/?channel_id=%s&kind=%s&actor_agent_id=%s&notice=created",
		teamID,
		req.ChannelID,
		url.QueryEscape(req.Kind),
		url.QueryEscape(req.AuthorAgentID),
	), http.StatusSeeOther)
}

type distillRequest struct {
	ChannelID    string `json:"channel_id"`
	MessageID    string `json:"message_id"`
	ActorAgentID string `json:"actor_agent_id"`
	Title        string `json:"title,omitempty"`
}

func handleDistillSkillToArtifact(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if !requestTrusted(r) {
		http.Error(w, "plan-exchange distill is limited to local or LAN requests", http.StatusForbidden)
		return
	}
	req, err := decodeDistillRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ChannelID == "" || req.MessageID == "" || req.ActorAgentID == "" {
		http.Error(w, "channel_id, message_id, and actor_agent_id are required", http.StatusBadRequest)
		return
	}
	if err := requireAction(store, teamID, req.ActorAgentID, "artifact.create"); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	messages, err := store.LoadAllMessagesCtx(r.Context(), teamID, req.ChannelID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var skillMsg *teamcore.Message
	for i := range messages {
		if messages[i].MessageID == req.MessageID && messages[i].MessageType == "skill" {
			skillMsg = &messages[i]
			break
		}
	}
	if skillMsg == nil {
		http.Error(w, "skill message not found", http.StatusNotFound)
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = planExchangeStructuredTitle(skillMsg.StructuredData)
		if title == "" {
			title = strings.TrimSpace(skillMsg.Content)
		}
		if len(title) > 80 {
			title = title[:80]
		}
	}
	content, _ := json.MarshalIndent(skillMsg.StructuredData, "", "  ")
	artifact := teamcore.Artifact{
		TeamID:    teamID,
		ChannelID: req.ChannelID,
		Title:     title,
		Kind:      "skill-doc",
		Summary:   skillMsg.Content,
		Content:   string(content),
		CreatedBy: req.ActorAgentID,
		Labels:    []string{"distilled", "skill", "source-message:" + req.MessageID},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := store.AppendArtifactCtx(r.Context(), teamID, artifact); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = store.AppendHistoryCtx(r.Context(), teamID, teamcore.ChangeEvent{
		TeamID:    teamID,
		Scope:     "artifact",
		Action:    "create",
		SubjectID: artifact.ArtifactID,
		Summary:   "提炼 plan-exchange skill 为 Artifact",
		Metadata: map[string]any{
			"channel_id":      req.ChannelID,
			"artifact_kind":   "skill-doc",
			"source_message":  req.MessageID,
			"artifact_source": "plan-exchange",
		},
		CreatedAt: artifact.CreatedAt,
	})
	if isAPIRequest(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "distilled", "artifact_kind": "skill-doc"})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/teams/%s/r/plan-exchange/?channel_id=%s&kind=skill&actor_agent_id=%s&notice=distilled",
		teamID,
		req.ChannelID,
		url.QueryEscape(req.ActorAgentID),
	), http.StatusSeeOther)
}

func filterPlanExchangeMessages(messages []teamcore.Message, kind string) []teamcore.Message {
	validKinds := map[string]bool{"plan": true, "skill": true, "snippet": true}
	filtered := make([]teamcore.Message, 0, len(messages))
	for _, msg := range messages {
		msgType := strings.TrimSpace(msg.MessageType)
		if !validKinds[msgType] {
			continue
		}
		if kind != "" && msgType != kind {
			continue
		}
		filtered = append(filtered, msg)
	}
	return filtered
}

func buildPlanExchangePageData(ctx context.Context, store *teamcore.Store, teamID, channelID, actorAgentID, kind, notice string, messages []teamcore.Message) (planExchangePageData, error) {
	artifacts, err := store.LoadArtifactsCtx(ctx, teamID, 200)
	if err != nil {
		return planExchangePageData{}, err
	}
	distilledByMessage := map[string]string{}
	for _, artifact := range artifacts {
		for _, label := range artifact.Labels {
			if strings.HasPrefix(label, "source-message:") {
				distilledByMessage[strings.TrimPrefix(label, "source-message:")] = artifact.ArtifactID
			}
		}
	}
	cards := make([]planExchangeCardView, 0, len(messages))
	for _, msg := range messages {
		cards = append(cards, planExchangeCardView{
			MessageID:       msg.MessageID,
			Kind:            msg.MessageType,
			KindLabel:       planExchangeKindLabel(msg.MessageType),
			Title:           planExchangeStructuredTitle(msg.StructuredData),
			Content:         msg.Content,
			AuthorAgentID:   msg.AuthorAgentID,
			ChannelID:       msg.ChannelID,
			CreatedAt:       msg.CreatedAt,
			StructuredJSON:  formatStructuredJSON(msg.StructuredData),
			Distilled:       distilledByMessage[msg.MessageID] != "",
			DistillArtifact: distilledByMessage[msg.MessageID],
		})
	}
	return planExchangePageData{
		TeamID:        teamID,
		ChannelID:     channelID,
		FilterKind:    kind,
		ActorAgentID:  actorAgentID,
		Notice:        notice,
		Cards:         cards,
		MessageCount:  len(cards),
		PluginVersion: "1.0.0",
	}, nil
}

func renderPlanExchangePage(w http.ResponseWriter, data planExchangePageData) error {
	tmpl, err := template.New("channel.html").Funcs(template.FuncMap{
		"kindSelected": func(filterKind, kind string) bool {
			return strings.TrimSpace(filterKind) == strings.TrimSpace(kind)
		},
	}).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return err
	}
	return tmpl.ExecuteTemplate(w, "channel.html", data)
}

func decodePostPlanExchangeRequest(r *http.Request) (postPlanExchangeRequest, error) {
	if isJSONRequest(r) {
		var req postPlanExchangeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return postPlanExchangeRequest{}, err
		}
		return req, nil
	}
	if err := r.ParseForm(); err != nil {
		return postPlanExchangeRequest{}, err
	}
	req := postPlanExchangeRequest{
		ChannelID:     strings.TrimSpace(r.FormValue("channel_id")),
		AuthorAgentID: strings.TrimSpace(r.FormValue("author_agent_id")),
		Kind:          strings.TrimSpace(r.FormValue("kind")),
	}
	req.StructuredData = buildStructuredDataFromForm(req.Kind, r)
	req.Content = buildContentFromForm(req.Kind, req.StructuredData, r)
	return req, nil
}

func decodeDistillRequest(r *http.Request) (distillRequest, error) {
	if isJSONRequest(r) {
		var req distillRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return distillRequest{}, err
		}
		return req, nil
	}
	if err := r.ParseForm(); err != nil {
		return distillRequest{}, err
	}
	return distillRequest{
		ChannelID:    strings.TrimSpace(r.FormValue("channel_id")),
		MessageID:    strings.TrimSpace(r.FormValue("message_id")),
		ActorAgentID: strings.TrimSpace(r.FormValue("actor_agent_id")),
		Title:        strings.TrimSpace(r.FormValue("title")),
	}, nil
}

func buildStructuredDataFromForm(kind string, r *http.Request) map[string]any {
	switch strings.TrimSpace(kind) {
	case "plan":
		return map[string]any{
			"kind":       "plan",
			"title":      strings.TrimSpace(r.FormValue("title")),
			"goal":       strings.TrimSpace(r.FormValue("goal")),
			"steps":      parseLines(r.FormValue("steps")),
			"abandoned":  parseAbandoned(r.FormValue("abandoned")),
			"interfaces": parseLines(r.FormValue("interfaces")),
			"ready_for":  parseLines(r.FormValue("ready_for")),
		}
	case "skill":
		return map[string]any{
			"kind":         "skill",
			"title":        strings.TrimSpace(r.FormValue("title")),
			"summary":      strings.TrimSpace(r.FormValue("summary")),
			"steps":        parseLines(r.FormValue("steps")),
			"traps":        parseLines(r.FormValue("traps")),
			"validated_by": strings.TrimSpace(r.FormValue("validated_by")),
			"language":     strings.TrimSpace(r.FormValue("language")),
		}
	case "snippet":
		examples := map[string]string{}
		for _, language := range []string{"go", "python", "typescript"} {
			value := strings.TrimSpace(r.FormValue("examples_" + language))
			if value != "" {
				examples[language] = value
			}
		}
		return map[string]any{
			"kind":       "snippet",
			"title":      strings.TrimSpace(r.FormValue("title")),
			"summary":    strings.TrimSpace(r.FormValue("summary")),
			"pseudocode": strings.TrimSpace(r.FormValue("pseudocode")),
			"examples":   examples,
			"language":   strings.TrimSpace(r.FormValue("language")),
			"related_to": strings.TrimSpace(r.FormValue("related_to")),
		}
	default:
		return nil
	}
}

func buildContentFromForm(kind string, structuredData map[string]any, r *http.Request) string {
	if content := strings.TrimSpace(r.FormValue("content")); content != "" {
		return content
	}
	title := planExchangeStructuredTitle(structuredData)
	switch strings.TrimSpace(kind) {
	case "plan":
		return title
	case "skill":
		if summary, _ := structuredData["summary"].(string); strings.TrimSpace(summary) != "" {
			return strings.TrimSpace(summary)
		}
		return title
	case "snippet":
		if summary, _ := structuredData["summary"].(string); strings.TrimSpace(summary) != "" {
			return strings.TrimSpace(summary)
		}
		return title
	default:
		return title
	}
}

func parseLines(raw string) []string {
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

func parseAbandoned(raw string) []map[string]string {
	lines := parseLines(raw)
	out := make([]map[string]string, 0, len(lines))
	for _, line := range lines {
		option := line
		reason := ""
		if left, right, ok := strings.Cut(line, "::"); ok {
			option = strings.TrimSpace(left)
			reason = strings.TrimSpace(right)
		}
		out = append(out, map[string]string{
			"option": option,
			"reason": reason,
		})
	}
	return out
}

func planExchangeStructuredTitle(data map[string]any) string {
	if len(data) == 0 {
		return ""
	}
	if title, _ := data["title"].(string); strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title)
	}
	return ""
}

func planExchangeKindLabel(kind string) string {
	switch strings.TrimSpace(kind) {
	case "plan":
		return "[PLAN]"
	case "skill":
		return "[SKILL]"
	case "snippet":
		return "[SNIPPET]"
	default:
		return "[MESSAGE]"
	}
}

func formatStructuredJSON(value map[string]any) string {
	if len(value) == 0 {
		return ""
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(body)
}

func isJSONRequest(r *http.Request) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))), "application/json")
}

func isAPIRequest(r *http.Request) bool {
	requestURI := strings.TrimSpace(r.RequestURI)
	return strings.HasPrefix(requestURI, "/api/teams/")
}

func requestTrusted(r *http.Request) bool {
	addr := clientIP(r)
	return addr.IsValid() && (addr.IsLoopback() || addr.IsPrivate())
}

func clientIP(r *http.Request) netip.Addr {
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

func requireAction(store *teamcore.Store, teamID, actorAgentID, action string) error {
	return teamcore.RequireAction(context.Background(), store, teamID, actorAgentID, action)
}

func actorRole(store *teamcore.Store, teamID, actorAgentID string, info teamcore.Info) (string, error) {
	actorAgentID = strings.TrimSpace(actorAgentID)
	if actorAgentID == "" {
		return "", errors.New("empty actor_agent_id")
	}
	members, err := store.LoadMembersCtx(context.Background(), teamID)
	if err == nil {
		for _, member := range members {
			if strings.TrimSpace(member.AgentID) == actorAgentID {
				return member.Role, nil
			}
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if actorAgentID == strings.TrimSpace(info.OwnerAgentID) {
		return "owner", nil
	}
	return "", fmt.Errorf("team actor %q not found", actorAgentID)
}
