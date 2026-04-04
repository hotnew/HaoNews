package haonewsteam

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"hao.news/internal/apphost"
	corehaonews "hao.news/internal/haonews"
	teamcore "hao.news/internal/haonews/team"
	themehaonews "hao.news/internal/themes/haonews"
)

func TestPluginBuildServesTeamIndex(t *testing.T) {
	t.Parallel()

	site, root := buildTeamSite(t)
	teamRoot := filepath.Join(root, "store", "team", "project-alpha")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{
  "team_id": "project-alpha",
  "title": "Project Alpha",
  "description": "Coordination team",
  "visibility": "team",
  "owner_agent_id": "agent://pc75/openclaw01",
  "channels": ["main", "research"],
  "created_at": "2026-04-01T00:00:00Z",
  "updated_at": "2026-04-01T01:00:00Z"
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "members.json"), []byte(`[
  {"agent_id":"agent://pc75/openclaw01","role":"owner","status":"active"},
  {"agent_id":"agent://pc75/live-alpha","role":"member","status":"active"}
]`), 0o644); err != nil {
		t.Fatalf("WriteFile(members.json) error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/teams", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Project Alpha") || !strings.Contains(body, "Coordination team") {
		t.Fatalf("expected team in body, got %q", body)
	}
}

func TestPluginBuildServesTeamArchiveRoutes(t *testing.T) {
	t.Parallel()

	site, root := buildTeamSite(t)
	store, err := teamcore.OpenStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	teamRoot := filepath.Join(root, "store", "team", "archive-project")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"archive-project","title":"Archive Project"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	record, err := store.CreateManualArchive("archive-project", time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("CreateManualArchive error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/archive/team", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Team 归档") {
		t.Fatalf("index status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/archive/team/archive-project/"+record.ArchiveID, nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), record.ArchiveID) {
		t.Fatalf("detail status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/archive/team/archive-project", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), record.ArchiveID) {
		t.Fatalf("api status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestPluginBuildCreatesTeamArchiveFromWorkspaceRoutes(t *testing.T) {
	t.Parallel()

	site, root := buildTeamSite(t)
	teamRoot := filepath.Join(root, "store", "team", "archive-create-project")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"archive-create-project","title":"Archive Create Project"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/teams/archive-create-project/archive", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("page archive create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); !strings.HasPrefix(location, "/archive/team/archive-create-project/manual-") {
		t.Fatalf("page archive location = %q", location)
	}

	apiReq := httptest.NewRequest(http.MethodPost, "/api/teams/archive-create-project/archive", nil)
	apiReq.RemoteAddr = "127.0.0.1:12345"
	apiRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(apiRec, apiReq)
	if apiRec.Code != http.StatusOK {
		t.Fatalf("api archive create status = %d, body = %s", apiRec.Code, apiRec.Body.String())
	}
	body := apiRec.Body.String()
	if !strings.Contains(body, `"scope": "team-archive-create"`) || !strings.Contains(body, `"team_id": "archive-create-project"`) || !strings.Contains(body, `"archive_id": "manual-`) {
		t.Fatalf("api archive create body = %s", body)
	}
}

func TestPluginBuildRejectsUnsignedMessageWhenTeamPolicyRequiresSignature(t *testing.T) {
	t.Parallel()

	site, root := buildTeamSite(t)
	teamRoot := filepath.Join(root, "store", "team", "signed-policy-team")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"signed-policy-team","title":"Signed Policy Team"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "members.json"), []byte(`[
  {"agent_id":"agent://pc75/live-alpha","role":"member","status":"active"}
]`), 0o644); err != nil {
		t.Fatalf("WriteFile(members.json) error = %v", err)
	}

	policyReq := httptest.NewRequest(http.MethodPost, "/api/teams/signed-policy-team/policy", strings.NewReader(`{
  "require_signature": true,
  "message_roles": ["owner","maintainer","member"],
  "task_roles": ["owner","maintainer","member"],
  "system_note_roles": ["owner","maintainer"]
}`))
	policyReq.RemoteAddr = "127.0.0.1:12345"
	policyRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(policyRec, policyReq)
	if policyRec.Code != http.StatusOK {
		t.Fatalf("policy update status = %d, body = %s", policyRec.Code, policyRec.Body.String())
	}

	msgReq := httptest.NewRequest(http.MethodPost, "/api/teams/signed-policy-team/channels/main/messages", strings.NewReader(`{
  "author_agent_id": "agent://pc75/live-alpha",
  "message_type": "chat",
  "content": "unsigned message should fail"
}`))
	msgReq.RemoteAddr = "127.0.0.1:12345"
	msgRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(msgRec, msgReq)
	if msgRec.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request for unsigned message, got %d body=%s", msgRec.Code, msgRec.Body.String())
	}
	if !strings.Contains(msgRec.Body.String(), "signature verification failed") {
		t.Fatalf("expected signature verification failure body, got %q", msgRec.Body.String())
	}
}

func TestPluginBuildServesWebhookStatusAndReplay(t *testing.T) {
	t.Parallel()

	site, root := buildTeamSite(t)
	teamRoot := filepath.Join(root, "store", "team", "webhook-status-team")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{
  "team_id":"webhook-status-team",
  "title":"Webhook Status Team",
  "owner_agent_id":"agent://pc75/openclaw01"
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "members.json"), []byte(`[
  {"agent_id":"agent://pc75/openclaw01","role":"owner","status":"active"}
]`), 0o644); err != nil {
		t.Fatalf("WriteFile(members.json) error = %v", err)
	}

	var mu sync.Mutex
	mode := "fail"
	delivered := 0
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		currentMode := mode
		if currentMode == "ok" {
			delivered++
		}
		mu.Unlock()
		if currentMode == "fail" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer hook.Close()

	configReq := httptest.NewRequest(http.MethodPost, "/api/teams/webhook-status-team/webhooks", strings.NewReader(`{
  "actor_agent_id":"agent://pc75/openclaw01",
  "webhooks":[{"webhook_id":"hook-status","url":"`+hook.URL+`","token":"token-status","events":["message.create"]}]
}`))
	configReq.RemoteAddr = "127.0.0.1:12345"
	configRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(configRec, configReq)
	if configRec.Code != http.StatusOK {
		t.Fatalf("webhook config status = %d, body = %s", configRec.Code, configRec.Body.String())
	}

	msgReq := httptest.NewRequest(http.MethodPost, "/api/teams/webhook-status-team/channels/main/messages", strings.NewReader(`{
  "author_agent_id":"agent://pc75/openclaw01",
  "message_type":"chat",
  "content":"dead letter me"
}`))
	msgReq.RemoteAddr = "127.0.0.1:12345"
	msgRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(msgRec, msgReq)
	if msgRec.Code != http.StatusCreated {
		t.Fatalf("message create status = %d, body = %s", msgRec.Code, msgRec.Body.String())
	}

	var deliveryID string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		statusReq := httptest.NewRequest(http.MethodGet, "/api/teams/webhook-status-team/webhooks/status", nil)
		statusRec := httptest.NewRecorder()
		site.Handler.ServeHTTP(statusRec, statusReq)
		if statusRec.Code != http.StatusOK {
			t.Fatalf("status api code = %d, body = %s", statusRec.Code, statusRec.Body.String())
		}
		var payload struct {
			DeadLetterCount  int `json:"dead_letter_count"`
			RecentDeadLetter []struct {
				DeliveryID string `json:"delivery_id"`
			} `json:"recent_dead_letters"`
		}
		if err := json.Unmarshal(statusRec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("status json decode error = %v", err)
		}
		if payload.DeadLetterCount > 0 && len(payload.RecentDeadLetter) > 0 {
			deliveryID = payload.RecentDeadLetter[0].DeliveryID
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if deliveryID == "" {
		t.Fatal("timed out waiting for dead-letter webhook status")
	}

	mu.Lock()
	mode = "ok"
	mu.Unlock()

	replayReq := httptest.NewRequest(http.MethodPost, "/api/teams/webhook-status-team/webhooks/replay/"+deliveryID, strings.NewReader(`{
  "actor_agent_id":"agent://pc75/openclaw01"
}`))
	replayReq.RemoteAddr = "127.0.0.1:12345"
	replayRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusOK {
		t.Fatalf("replay status = %d, body = %s", replayRec.Code, replayRec.Body.String())
	}

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		gotDelivered := delivered
		mu.Unlock()
		if gotDelivered > 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("timed out waiting for replayed webhook delivery")
}

func TestPluginBuildEnforcesTeamActionPermissions(t *testing.T) {
	t.Parallel()

	site, root := buildTeamSite(t)
	teamRoot := filepath.Join(root, "store", "team", "permission-team")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{
  "team_id":"permission-team",
  "title":"Permission Team",
  "owner_agent_id":"agent://pc75/live-bravo"
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "members.json"), []byte(`[
  {"agent_id":"agent://pc75/live-bravo","role":"owner","status":"active"},
  {"agent_id":"agent://pc75/live-charlie","role":"observer","status":"active"}
]`), 0o644); err != nil {
		t.Fatalf("WriteFile(members.json) error = %v", err)
	}

	policyReq := httptest.NewRequest(http.MethodPost, "/api/teams/permission-team/policy", strings.NewReader(`{
  "actor_agent_id": "agent://pc75/live-bravo",
  "permissions": {
    "message.send": ["owner"],
    "policy.update": ["owner"]
  }
}`))
	policyReq.RemoteAddr = "127.0.0.1:12345"
	policyRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(policyRec, policyReq)
	if policyRec.Code != http.StatusOK {
		t.Fatalf("policy update status = %d, body = %s", policyRec.Code, policyRec.Body.String())
	}

	msgReq := httptest.NewRequest(http.MethodPost, "/api/teams/permission-team/channels/main/messages", strings.NewReader(`{
  "author_agent_id": "agent://pc75/live-charlie",
  "message_type": "chat",
  "content": "observer should be denied"
}`))
	msgReq.RemoteAddr = "127.0.0.1:12345"
	msgRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(msgRec, msgReq)
	if msgRec.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden for message.send, got %d body=%s", msgRec.Code, msgRec.Body.String())
	}
	if !strings.Contains(msgRec.Body.String(), "policy denied action") {
		t.Fatalf("expected policy denied action body, got %q", msgRec.Body.String())
	}

	memberPolicyReq := httptest.NewRequest(http.MethodPost, "/api/teams/permission-team/policy", strings.NewReader(`{
  "actor_agent_id": "agent://pc75/live-charlie",
  "permissions": {
    "message.send": ["owner"],
    "policy.update": ["owner"]
  }
}`))
	memberPolicyReq.RemoteAddr = "127.0.0.1:12345"
	memberPolicyRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(memberPolicyRec, memberPolicyReq)
	if memberPolicyRec.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden for policy.update, got %d body=%s", memberPolicyRec.Code, memberPolicyRec.Body.String())
	}
}

func TestPluginBuildTeamAgentCardAPI(t *testing.T) {
	t.Parallel()

	site, root := buildTeamSite(t)
	teamRoot := filepath.Join(root, "store", "team", "agent-api-team")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{
  "team_id":"agent-api-team",
  "title":"Agent API Team",
  "owner_agent_id":"agent://pc75/live-bravo"
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "members.json"), []byte(`[
  {"agent_id":"agent://pc75/live-bravo","role":"owner","status":"active"}
]`), 0o644); err != nil {
		t.Fatalf("WriteFile(members.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "tasks.jsonl"), []byte("{\"task_id\":\"task-agent-match\",\"team_id\":\"agent-api-team\",\"title\":\"Task Agent Match\",\"labels\":[\"coding\"],\"status\":\"open\",\"created_at\":\"2026-04-03T00:00:00Z\",\"updated_at\":\"2026-04-03T00:00:00Z\"}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(tasks.jsonl) error = %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/teams/agent-api-team/agents", strings.NewReader(`{
  "actor_agent_id": "agent://pc75/live-bravo",
  "card": {
    "agent_id": "agent://pc75/coder",
    "name": "Code Agent",
    "skills": [
      {"id":"code-write","name":"Code Writing","tags":["coding","implementation"]}
    ]
  }
}`))
	createReq.RemoteAddr = "127.0.0.1:12345"
	createRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("agent create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/teams/agent-api-team/agents?task=task-agent-match", nil)
	listRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("agent list status = %d, body = %s", listRec.Code, listRec.Body.String())
	}
	if !strings.Contains(listRec.Body.String(), "\"scope\": \"team-agents\"") || !strings.Contains(listRec.Body.String(), "\"matched_count\": 1") || !strings.Contains(listRec.Body.String(), "agent://pc75/coder") {
		t.Fatalf("expected team agents body, got %q", listRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/teams/agent-api-team/agents/agent:%2F%2Fpc75%2Fcoder", nil)
	getRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("agent get status = %d, body = %s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), "\"scope\": \"team-agent-card\"") || !strings.Contains(getRec.Body.String(), "\"name\": \"Code Agent\"") {
		t.Fatalf("expected team agent card body, got %q", getRec.Body.String())
	}
}

func TestPluginBuildStreamsTeamEvents(t *testing.T) {
	t.Parallel()

	site, root := buildTeamSite(t)
	teamRoot := filepath.Join(root, "store", "team", "sse-team")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{
  "team_id":"sse-team",
  "title":"SSE Team",
  "owner_agent_id":"agent://pc75/live-bravo"
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "members.json"), []byte(`[
  {"agent_id":"agent://pc75/live-bravo","role":"owner","status":"active"}
]`), 0o644); err != nil {
		t.Fatalf("WriteFile(members.json) error = %v", err)
	}

	server := httptest.NewServer(site.Handler)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/teams/sse-team/events", nil)
	if err != nil {
		t.Fatalf("NewRequest error = %v", err)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("stream request error = %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q", got)
	}

	eventCh := make(chan teamcore.TeamEvent, 1)
	errCh := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				errCh <- err
				return
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var event teamcore.TeamEvent
			if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data: "))), &event); err != nil {
				errCh <- err
				return
			}
			eventCh <- event
			return
		}
	}()

	postReq, err := http.NewRequest(http.MethodPost, server.URL+"/api/teams/sse-team/channels/main/messages", strings.NewReader(`{
  "author_agent_id":"agent://pc75/live-bravo",
  "message_type":"chat",
  "content":"hello from sse"
}`))
	if err != nil {
		t.Fatalf("NewRequest(post) error = %v", err)
	}
	postReq.Header.Set("Content-Type", "application/json")
	postResp, err := server.Client().Do(postReq)
	if err != nil {
		t.Fatalf("post message error = %v", err)
	}
	defer postResp.Body.Close()
	if postResp.StatusCode != http.StatusCreated {
		t.Fatalf("post status = %d", postResp.StatusCode)
	}

	select {
	case event := <-eventCh:
		if event.TeamID != "sse-team" || event.Kind != "message" || event.Action != "create" {
			t.Fatalf("unexpected event: %#v", event)
		}
		if event.ChannelID != "main" {
			t.Fatalf("event.ChannelID = %q", event.ChannelID)
		}
	case err := <-errCh:
		t.Fatalf("stream read error = %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for sse event")
	}
}

func TestPluginBuildServesA2ABridge(t *testing.T) {
	t.Parallel()

	site, root := buildTeamSite(t)
	store, err := teamcore.OpenStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	teamRoot := filepath.Join(root, "store", "team", "a2a-team")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{
  "team_id":"a2a-team",
  "title":"A2A Team",
  "owner_agent_id":"agent://pc75/live-bravo"
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "members.json"), []byte(`[
  {"agent_id":"agent://pc75/live-bravo","role":"owner","status":"active"}
]`), 0o644); err != nil {
		t.Fatalf("WriteFile(members.json) error = %v", err)
	}
	if err := store.SaveAgentCard("a2a-team", teamcore.AgentCard{
		AgentID: "agent://pc75/coder",
		Name:    "Code Agent",
		Skills:  []teamcore.AgentSkill{{ID: "code-write", Name: "Code Writing"}},
		Capabilities: teamcore.AgentCaps{
			Streaming: true,
		},
	}); err != nil {
		t.Fatalf("SaveAgentCard error = %v", err)
	}
	if err := store.AppendTask("a2a-team", teamcore.Task{
		TaskID:    "a2a-task-1",
		CreatedBy: "agent://pc75/live-bravo",
		Title:     "Bridge Task",
		Status:    "doing",
		Priority:  "high",
		CreatedAt: time.Date(2026, 4, 3, 15, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 3, 15, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendTask error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("well-known status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"agent_count": 1`) || !strings.Contains(rec.Body.String(), "agent://pc75/coder") || !strings.Contains(rec.Body.String(), `"streaming": true`) {
		t.Fatalf("unexpected well-known body: %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/a2a/teams/a2a-team/tasks", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("a2a tasks status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"scope": "a2a-tasks"`) || !strings.Contains(rec.Body.String(), `"status": "working"`) {
		t.Fatalf("unexpected a2a tasks body: %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/a2a/teams/a2a-team/tasks/a2a-task-1", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("a2a task detail status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"scope": "a2a-task"`) || !strings.Contains(rec.Body.String(), `"id": "a2a-task-1"`) {
		t.Fatalf("unexpected a2a task detail body: %q", rec.Body.String())
	}

	msgReq := httptest.NewRequest(http.MethodPost, "/a2a/teams/a2a-team/message:send", strings.NewReader(`{
  "author_agent_id":"agent://pc75/live-bravo",
  "message_type":"chat",
  "content":"hello from a2a bridge"
}`))
	msgReq.RemoteAddr = "127.0.0.1:12345"
	msgReq.Header.Set("Content-Type", "application/json")
	msgRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(msgRec, msgReq)
	if msgRec.Code != http.StatusCreated {
		t.Fatalf("a2a message send status = %d, body = %s", msgRec.Code, msgRec.Body.String())
	}
	if !strings.Contains(msgRec.Body.String(), `"scope": "a2a-message"`) || !strings.Contains(msgRec.Body.String(), "hello from a2a bridge") {
		t.Fatalf("unexpected a2a message body: %q", msgRec.Body.String())
	}

	cancelReq := httptest.NewRequest(http.MethodPost, "/a2a/teams/a2a-team/tasks/a2a-task-1:cancel", strings.NewReader(`{
  "actor_agent_id":"agent://pc75/live-bravo"
}`))
	cancelReq.RemoteAddr = "127.0.0.1:12345"
	cancelReq.Header.Set("Content-Type", "application/json")
	cancelRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(cancelRec, cancelReq)
	if cancelRec.Code != http.StatusOK {
		t.Fatalf("a2a task cancel status = %d, body = %s", cancelRec.Code, cancelRec.Body.String())
	}
	if !strings.Contains(cancelRec.Body.String(), `"status": "canceled"`) {
		t.Fatalf("unexpected a2a cancel body: %q", cancelRec.Body.String())
	}

	server := httptest.NewServer(site.Handler)
	defer server.Close()
	streamReq, err := http.NewRequest(http.MethodGet, server.URL+"/a2a/teams/a2a-team/message:stream", nil)
	if err != nil {
		t.Fatalf("NewRequest(stream) error = %v", err)
	}
	streamResp, err := server.Client().Do(streamReq)
	if err != nil {
		t.Fatalf("stream request error = %v", err)
	}
	defer streamResp.Body.Close()
	if got := streamResp.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("stream Content-Type = %q", got)
	}
}

func TestPluginBuildConfiguresAndFiresTeamWebhook(t *testing.T) {
	t.Parallel()

	site, root := buildTeamSite(t)
	teamRoot := filepath.Join(root, "store", "team", "webhook-api-team")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{
  "team_id":"webhook-api-team",
  "title":"Webhook API Team",
  "owner_agent_id":"agent://pc75/live-bravo"
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "members.json"), []byte(`[
  {"agent_id":"agent://pc75/live-bravo","role":"owner","status":"active"}
]`), 0o644); err != nil {
		t.Fatalf("WriteFile(members.json) error = %v", err)
	}

	received := make(chan string, 1)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Get("Authorization")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer hook.Close()

	configReq := httptest.NewRequest(http.MethodPost, "/api/teams/webhook-api-team/webhooks", strings.NewReader(`{
  "actor_agent_id":"agent://pc75/live-bravo",
  "webhooks":[{"webhook_id":"hook-api","url":"`+hook.URL+`","token":"token-api","events":["message.create"]}]
}`))
	configReq.RemoteAddr = "127.0.0.1:12345"
	configReq.Header.Set("Content-Type", "application/json")
	configRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(configRec, configReq)
	if configRec.Code != http.StatusOK {
		t.Fatalf("webhook config status = %d, body = %s", configRec.Code, configRec.Body.String())
	}
	if !strings.Contains(configRec.Body.String(), `"scope": "team-webhooks"`) {
		t.Fatalf("unexpected webhook config body: %q", configRec.Body.String())
	}

	msgReq := httptest.NewRequest(http.MethodPost, "/api/teams/webhook-api-team/channels/main/messages", strings.NewReader(`{
  "author_agent_id":"agent://pc75/live-bravo",
  "message_type":"chat",
  "content":"hello webhook api"
}`))
	msgReq.RemoteAddr = "127.0.0.1:12345"
	msgReq.Header.Set("Content-Type", "application/json")
	msgRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(msgRec, msgReq)
	if msgRec.Code != http.StatusCreated {
		t.Fatalf("message create status = %d, body = %s", msgRec.Code, msgRec.Body.String())
	}

	select {
	case auth := <-received:
		if auth != "Bearer token-api" {
			t.Fatalf("Authorization = %q", auth)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for webhook api delivery")
	}
}

func TestPluginBuildServesEmptyTeamIndex(t *testing.T) {
	t.Parallel()

	site, _ := buildTeamSite(t)
	req := httptest.NewRequest(http.MethodGet, "/teams", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "暂无 Team") {
		t.Fatalf("expected empty state body, got %q", rec.Body.String())
	}
}

func TestPluginBuildServesTeamDetailAndAPI(t *testing.T) {
	t.Parallel()

	site, root := buildTeamSite(t)
	store, err := teamcore.OpenStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	teamRoot := filepath.Join(root, "store", "team", "project-beta")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{
  "team_id": "project-beta",
  "title": "Project Beta",
  "description": "Independent team module",
  "visibility": "private",
  "owner_agent_id": "agent://pc75/live-bravo",
  "owner_origin_public_key": "`+strings.Repeat("a", 64)+`",
  "owner_parent_public_key": "`+strings.Repeat("b", 64)+`",
  "channels": ["main", "research"],
  "created_at": "2026-04-01T02:00:00Z",
  "updated_at": "2026-04-01T03:00:00Z"
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "members.json"), []byte(`[
  {"agent_id":"agent://pc75/live-bravo","role":"owner","status":"active"},
  {"agent_id":"agent://pc75/live-charlie","role":"observer","status":"pending"}
]`), 0o644); err != nil {
		t.Fatalf("WriteFile(members.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "policy.json"), []byte(`{
  "message_roles": ["owner", "maintainer", "member"],
  "task_roles": ["owner", "maintainer"],
  "system_note_roles": ["owner"]
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(policy.json) error = %v", err)
	}
	if err := store.AppendMessage("project-beta", teamcore.Message{
		ChannelID:     "main",
		AuthorAgentID: "agent://pc75/live-bravo",
		MessageType:   "decision",
		Content:       "Team Beta decided to keep Team separate from Live.",
		CreatedAt:     time.Date(2026, 4, 1, 3, 30, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendMessage error = %v", err)
	}
	if err := store.AppendMessage("project-beta", teamcore.Message{
		ChannelID:     "research",
		AuthorAgentID: "agent://pc75/live-alpha",
		MessageType:   "note",
		Content:       "Research channel keeps long-running coordination notes.",
		CreatedAt:     time.Date(2026, 4, 1, 3, 35, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendMessage(research) error = %v", err)
	}
	if err := store.AppendTask("project-beta", teamcore.Task{
		TaskID:    "team-task-implement-teamtask",
		CreatedBy: "agent://pc75/live-bravo",
		Title:     "Implement TeamTask",
		ChannelID: "research",
		Status:    "doing",
		Priority:  "high",
		Assignees: []string{"agent://pc75/live-charlie"},
		UpdatedAt: time.Date(2026, 4, 1, 3, 45, 0, 0, time.UTC),
		CreatedAt: time.Date(2026, 4, 1, 3, 40, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendTask error = %v", err)
	}
	if err := store.AppendMessage("project-beta", teamcore.Message{
		ChannelID:     "research",
		AuthorAgentID: "agent://pc75/live-charlie",
		MessageType:   "comment",
		Content:       "Task comments stay inside TeamMessage, not Live.",
		StructuredData: map[string]any{
			"task_id": "team-task-implement-teamtask",
		},
		CreatedAt: time.Date(2026, 4, 1, 3, 46, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendMessage(task comment) error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/teams/project-beta", nil)
	rec := httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Project Beta") || !strings.Contains(rec.Body.String(), "成员") || !strings.Contains(rec.Body.String(), "Team Policy") || !strings.Contains(rec.Body.String(), "最近消息") || !strings.Contains(rec.Body.String(), "Team Beta decided to keep Team separate from Live.") || !strings.Contains(rec.Body.String(), "最近任务") || !strings.Contains(rec.Body.String(), "Implement TeamTask") || !strings.Contains(rec.Body.String(), "owner · agent://pc75/live-bravo") || !strings.Contains(rec.Body.String(), "工作入口") || !strings.Contains(rec.Body.String(), "/teams/project-beta/tasks?status=doing") || !strings.Contains(rec.Body.String(), "/teams/project-beta/artifacts?kind=markdown") {
		t.Fatalf("expected team detail body, got %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "/teams/project-beta/channels/main") || !strings.Contains(rec.Body.String(), "/teams/project-beta/channels/research") {
		t.Fatalf("expected team detail channel links, got %q", rec.Body.String())
	}

	channelReq := httptest.NewRequest(http.MethodGet, "/teams/project-beta/channels/research", nil)
	channelRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(channelRec, channelReq)
	if channelRec.Code != http.StatusOK {
		t.Fatalf("channel page status = %d, body = %s", channelRec.Code, channelRec.Body.String())
	}
	if !strings.Contains(channelRec.Body.String(), "Research channel keeps long-running coordination notes.") || !strings.Contains(channelRec.Body.String(), "research") {
		t.Fatalf("expected team channel page body, got %q", channelRec.Body.String())
	}

	tasksPageReq := httptest.NewRequest(http.MethodGet, "/teams/project-beta/tasks", nil)
	tasksPageRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(tasksPageRec, tasksPageReq)
	if tasksPageRec.Code != http.StatusOK {
		t.Fatalf("tasks page status = %d, body = %s", tasksPageRec.Code, tasksPageRec.Body.String())
	}
	if !strings.Contains(tasksPageRec.Body.String(), "Implement TeamTask") || !strings.Contains(tasksPageRec.Body.String(), "/teams/project-beta/tasks/team-task-implement-teamtask") {
		t.Fatalf("expected team tasks page body, got %q", tasksPageRec.Body.String())
	}

	taskPageReq := httptest.NewRequest(http.MethodGet, "/teams/project-beta/tasks/team-task-implement-teamtask", nil)
	taskPageRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(taskPageRec, taskPageReq)
	if taskPageRec.Code != http.StatusOK {
		t.Fatalf("task page status = %d, body = %s", taskPageRec.Code, taskPageRec.Body.String())
	}
	if !strings.Contains(taskPageRec.Body.String(), "Task comments stay inside TeamMessage, not Live.") || !strings.Contains(taskPageRec.Body.String(), "team-task-implement-teamtask") {
		t.Fatalf("expected team task page body, got %q", taskPageRec.Body.String())
	}

	contextTask, err := store.LoadTask("project-beta", "team-task-implement-teamtask")
	if err != nil {
		t.Fatalf("LoadTask(context) error = %v", err)
	}
	contextReq := httptest.NewRequest(http.MethodGet, "/api/teams/project-beta/contexts/"+url.PathEscape(contextTask.ContextID), nil)
	contextRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(contextRec, contextReq)
	if contextRec.Code != http.StatusOK {
		t.Fatalf("context api status = %d, body = %s", contextRec.Code, contextRec.Body.String())
	}
	if !strings.Contains(contextRec.Body.String(), `"scope": "team-context"`) || !strings.Contains(contextRec.Body.String(), `"task_count": 1`) || !strings.Contains(contextRec.Body.String(), contextTask.ContextID) {
		t.Fatalf("expected team context api body, got %q", contextRec.Body.String())
	}

	apiReq := httptest.NewRequest(http.MethodGet, "/api/teams/project-beta", nil)
	apiRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(apiRec, apiReq)
	if apiRec.Code != http.StatusOK {
		t.Fatalf("api status = %d, body = %s", apiRec.Code, apiRec.Body.String())
	}
	if !strings.Contains(apiRec.Body.String(), "\"scope\": \"team-detail\"") || !strings.Contains(apiRec.Body.String(), "\"team_id\": \"project-beta\"") {
		t.Fatalf("expected team detail api body, got %q", apiRec.Body.String())
	}
	if !strings.Contains(apiRec.Body.String(), "\"policy\"") {
		t.Fatalf("expected team detail policy body, got %q", apiRec.Body.String())
	}

	channelsReq := httptest.NewRequest(http.MethodGet, "/api/teams/project-beta/channels", nil)
	channelsRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(channelsRec, channelsReq)
	if channelsRec.Code != http.StatusOK {
		t.Fatalf("channels api status = %d, body = %s", channelsRec.Code, channelsRec.Body.String())
	}
	if !strings.Contains(channelsRec.Body.String(), "\"scope\": \"team-channels\"") || !strings.Contains(channelsRec.Body.String(), "\"channel_count\": 2") || !strings.Contains(channelsRec.Body.String(), "\"channel_id\": \"research\"") {
		t.Fatalf("expected team channels api body, got %q", channelsRec.Body.String())
	}

	channelCreateReq := httptest.NewRequest(http.MethodPost, "/api/teams/project-beta/channels", strings.NewReader(`{
  "channel_id": "planning",
  "title": "Planning Board",
  "description": "Long-running planning channel"
}`))
	channelCreateReq.RemoteAddr = "127.0.0.1:12345"
	channelCreateReq.Header.Set("Content-Type", "application/json")
	channelCreateRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(channelCreateRec, channelCreateReq)
	if channelCreateRec.Code != http.StatusCreated {
		t.Fatalf("channel create api status = %d, body = %s", channelCreateRec.Code, channelCreateRec.Body.String())
	}
	if !strings.Contains(channelCreateRec.Body.String(), "\"scope\": \"team-channel\"") || !strings.Contains(channelCreateRec.Body.String(), "Planning Board") {
		t.Fatalf("expected channel create api body, got %q", channelCreateRec.Body.String())
	}

	channelAPIReq := httptest.NewRequest(http.MethodGet, "/api/teams/project-beta/channels/planning", nil)
	channelAPIRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(channelAPIRec, channelAPIReq)
	if channelAPIRec.Code != http.StatusOK {
		t.Fatalf("channel api status = %d, body = %s", channelAPIRec.Code, channelAPIRec.Body.String())
	}
	if !strings.Contains(channelAPIRec.Body.String(), "\"scope\": \"team-channel\"") || !strings.Contains(channelAPIRec.Body.String(), "\"channel_id\": \"planning\"") {
		t.Fatalf("expected team channel api body, got %q", channelAPIRec.Body.String())
	}

	channelUpdateReq := httptest.NewRequest(http.MethodPut, "/api/teams/project-beta/channels/planning", strings.NewReader(`{
  "title": "Planning Updated",
  "description": "Updated planning channel",
  "hidden": false
}`))
	channelUpdateReq.RemoteAddr = "127.0.0.1:12345"
	channelUpdateReq.Header.Set("Content-Type", "application/json")
	channelUpdateRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(channelUpdateRec, channelUpdateReq)
	if channelUpdateRec.Code != http.StatusOK {
		t.Fatalf("channel update api status = %d, body = %s", channelUpdateRec.Code, channelUpdateRec.Body.String())
	}
	if !strings.Contains(channelUpdateRec.Body.String(), "Planning Updated") || !strings.Contains(channelUpdateRec.Body.String(), "Updated planning channel") {
		t.Fatalf("expected team channel update api body, got %q", channelUpdateRec.Body.String())
	}

	channelDeleteReq := httptest.NewRequest(http.MethodDelete, "/api/teams/project-beta/channels/planning", nil)
	channelDeleteReq.RemoteAddr = "127.0.0.1:12345"
	channelDeleteRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(channelDeleteRec, channelDeleteReq)
	if channelDeleteRec.Code != http.StatusOK {
		t.Fatalf("channel delete api status = %d, body = %s", channelDeleteRec.Code, channelDeleteRec.Body.String())
	}
	if !strings.Contains(channelDeleteRec.Body.String(), "\"deleted\": true") || !strings.Contains(channelDeleteRec.Body.String(), "\"hidden\": true") {
		t.Fatalf("expected team channel delete api body, got %q", channelDeleteRec.Body.String())
	}

	membersReq := httptest.NewRequest(http.MethodGet, "/api/teams/project-beta/members", nil)
	membersRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(membersRec, membersReq)
	if membersRec.Code != http.StatusOK {
		t.Fatalf("members api status = %d, body = %s", membersRec.Code, membersRec.Body.String())
	}
	if !strings.Contains(membersRec.Body.String(), "\"scope\": \"team-members\"") || !strings.Contains(membersRec.Body.String(), "\"member_count\": 2") || !strings.Contains(membersRec.Body.String(), "\"pending\": 1") {
		t.Fatalf("expected team members api body, got %q", membersRec.Body.String())
	}

	membersPageReq := httptest.NewRequest(http.MethodGet, "/teams/project-beta/members", nil)
	membersPageRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(membersPageRec, membersPageReq)
	if membersPageRec.Code != http.StatusOK {
		t.Fatalf("members page status = %d, body = %s", membersPageRec.Code, membersPageRec.Body.String())
	}
	if !strings.Contains(membersPageRec.Body.String(), "成员治理") || !strings.Contains(membersPageRec.Body.String(), "批量治理") || !strings.Contains(membersPageRec.Body.String(), "agent://pc75/live-charlie") {
		t.Fatalf("expected members page body, got %q", membersPageRec.Body.String())
	}

	filteredMembersReq := httptest.NewRequest(http.MethodGet, "/api/teams/project-beta/members?status=pending&role=observer&agent=charlie", nil)
	filteredMembersRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(filteredMembersRec, filteredMembersReq)
	if filteredMembersRec.Code != http.StatusOK {
		t.Fatalf("filtered members api status = %d, body = %s", filteredMembersRec.Code, filteredMembersRec.Body.String())
	}
	if !strings.Contains(filteredMembersRec.Body.String(), "\"member_count\": 1") || !strings.Contains(filteredMembersRec.Body.String(), "\"status\": \"pending\"") || !strings.Contains(filteredMembersRec.Body.String(), "\"role\": \"observer\"") || !strings.Contains(filteredMembersRec.Body.String(), "\"agent\": \"charlie\"") {
		t.Fatalf("expected filtered members api body, got %q", filteredMembersRec.Body.String())
	}

	memberActionReq := httptest.NewRequest(http.MethodPost, "/api/teams/project-beta/members/action", strings.NewReader(`{
  "agent_id": "agent://pc75/live-charlie",
  "action": "approve"
}`))
	memberActionReq.RemoteAddr = "127.0.0.1:12345"
	memberActionReq.Header.Set("Content-Type", "application/json")
	memberActionRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(memberActionRec, memberActionReq)
	if memberActionRec.Code != http.StatusOK {
		t.Fatalf("member action api status = %d, body = %s", memberActionRec.Code, memberActionRec.Body.String())
	}
	if !strings.Contains(memberActionRec.Body.String(), "\"scope\": \"team-member-action\"") || !strings.Contains(memberActionRec.Body.String(), "\"status\": \"active\"") || !strings.Contains(memberActionRec.Body.String(), "审批通过 Team 成员") {
		t.Fatalf("expected member action api body, got %q", memberActionRec.Body.String())
	}

	memberBulkReq := httptest.NewRequest(http.MethodPost, "/api/teams/project-beta/members/bulk-action", strings.NewReader(`{
  "agent_ids": ["agent://pc75/live-charlie"],
  "action": "mute"
}`))
	memberBulkReq.RemoteAddr = "127.0.0.1:12345"
	memberBulkReq.Header.Set("Content-Type", "application/json")
	memberBulkRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(memberBulkRec, memberBulkReq)
	if memberBulkRec.Code != http.StatusOK {
		t.Fatalf("member bulk action api status = %d, body = %s", memberBulkRec.Code, memberBulkRec.Body.String())
	}
	if !strings.Contains(memberBulkRec.Body.String(), "\"scope\": \"team-member-bulk-action\"") || !strings.Contains(memberBulkRec.Body.String(), "\"applied_count\": 1") || !strings.Contains(memberBulkRec.Body.String(), "\"status\": \"muted\"") {
		t.Fatalf("expected member bulk action api body, got %q", memberBulkRec.Body.String())
	}

	policyReq := httptest.NewRequest(http.MethodGet, "/api/teams/project-beta/policy", nil)
	policyRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(policyRec, policyReq)
	if policyRec.Code != http.StatusOK {
		t.Fatalf("policy api status = %d, body = %s", policyRec.Code, policyRec.Body.String())
	}
	if !strings.Contains(policyRec.Body.String(), "\"scope\": \"team-policy\"") || !strings.Contains(policyRec.Body.String(), "\"system_note_roles\"") {
		t.Fatalf("expected team policy api body, got %q", policyRec.Body.String())
	}

	policyUpdateReq := httptest.NewRequest(http.MethodPost, "/api/teams/project-beta/policy", strings.NewReader(`{
  "message_roles": ["owner", "member"],
  "task_roles": ["owner", "maintainer", "member"],
  "system_note_roles": ["owner", "maintainer"]
}`))
	policyUpdateReq.RemoteAddr = "127.0.0.1:12345"
	policyUpdateReq.Header.Set("Content-Type", "application/json")
	policyUpdateRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(policyUpdateRec, policyUpdateReq)
	if policyUpdateRec.Code != http.StatusOK {
		t.Fatalf("policy update api status = %d, body = %s", policyUpdateRec.Code, policyUpdateRec.Body.String())
	}
	if !strings.Contains(policyUpdateRec.Body.String(), "\"scope\": \"team-policy\"") || !strings.Contains(policyUpdateRec.Body.String(), "\"member\"") {
		t.Fatalf("expected updated policy api body, got %q", policyUpdateRec.Body.String())
	}

	historyAPIReq := httptest.NewRequest(http.MethodGet, "/api/teams/project-beta/history", nil)
	historyAPIRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(historyAPIRec, historyAPIReq)
	if historyAPIRec.Code != http.StatusOK {
		t.Fatalf("history api status = %d, body = %s", historyAPIRec.Code, historyAPIRec.Body.String())
	}
	if !strings.Contains(historyAPIRec.Body.String(), "\"scope\": \"team-history\"") || !strings.Contains(historyAPIRec.Body.String(), "更新 Team Policy") || !strings.Contains(historyAPIRec.Body.String(), "\"source\": \"api\"") || !strings.Contains(historyAPIRec.Body.String(), "\"message_roles_before\"") || !strings.Contains(historyAPIRec.Body.String(), "\"diff\":") || !strings.Contains(historyAPIRec.Body.String(), "\"message_roles\"") || !strings.Contains(historyAPIRec.Body.String(), "消息角色/任务角色/系统说明角色已更新") {
		t.Fatalf("expected history api body, got %q", historyAPIRec.Body.String())
	}

	historyPageReq := httptest.NewRequest(http.MethodGet, "/teams/project-beta/history", nil)
	historyPageRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(historyPageRec, historyPageReq)
	if historyPageRec.Code != http.StatusOK {
		t.Fatalf("history page status = %d, body = %s", historyPageRec.Code, historyPageRec.Body.String())
	}
	if !strings.Contains(historyPageRec.Body.String(), "全部变更") || !strings.Contains(historyPageRec.Body.String(), ">api<") || !strings.Contains(historyPageRec.Body.String(), "更新 Team Policy") || !strings.Contains(historyPageRec.Body.String(), "消息角色") || !strings.Contains(historyPageRec.Body.String(), "应用筛选") {
		t.Fatalf("expected history page body, got %q", historyPageRec.Body.String())
	}

	filteredHistoryReq := httptest.NewRequest(http.MethodGet, "/api/teams/project-beta/history?scope=channel", nil)
	filteredHistoryRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(filteredHistoryRec, filteredHistoryReq)
	if filteredHistoryRec.Code != http.StatusOK {
		t.Fatalf("filtered history api status = %d, body = %s", filteredHistoryRec.Code, filteredHistoryRec.Body.String())
	}
	if !strings.Contains(filteredHistoryRec.Body.String(), "\"scope\": \"channel\"") || !strings.Contains(filteredHistoryRec.Body.String(), "\"action\": \"hide\"") || strings.Contains(filteredHistoryRec.Body.String(), "更新 Team Policy") {
		t.Fatalf("expected filtered team history api body, got %q", filteredHistoryRec.Body.String())
	}

	filteredHistoryPageReq := httptest.NewRequest(http.MethodGet, "/teams/project-beta/history?scope=channel", nil)
	filteredHistoryPageRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(filteredHistoryPageRec, filteredHistoryPageReq)
	if filteredHistoryPageRec.Code != http.StatusOK {
		t.Fatalf("filtered history page status = %d, body = %s", filteredHistoryPageRec.Code, filteredHistoryPageRec.Body.String())
	}
	if !strings.Contains(filteredHistoryPageRec.Body.String(), "创建 Team Channel") || strings.Contains(filteredHistoryPageRec.Body.String(), "更新 Team Policy") {
		t.Fatalf("expected filtered team history page body, got %q", filteredHistoryPageRec.Body.String())
	}

	memberActionPageReq := httptest.NewRequest(http.MethodPost, "/teams/project-beta/members/action", strings.NewReader("agent_id=agent%3A%2F%2Fpc75%2Flive-charlie&action=pending"))
	memberActionPageReq.RemoteAddr = "127.0.0.1:12345"
	memberActionPageReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	memberActionPageRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(memberActionPageRec, memberActionPageReq)
	if memberActionPageRec.Code != http.StatusSeeOther {
		t.Fatalf("member action page status = %d, body = %s", memberActionPageRec.Code, memberActionPageRec.Body.String())
	}

	messagesReq := httptest.NewRequest(http.MethodGet, "/api/teams/project-beta/messages", nil)
	messagesRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(messagesRec, messagesReq)
	if messagesRec.Code != http.StatusOK {
		t.Fatalf("messages api status = %d, body = %s", messagesRec.Code, messagesRec.Body.String())
	}
	if !strings.Contains(messagesRec.Body.String(), "\"scope\": \"team-messages\"") || !strings.Contains(messagesRec.Body.String(), "Team Beta decided to keep Team separate from Live.") {
		t.Fatalf("expected team messages api body, got %q", messagesRec.Body.String())
	}
	limitedMessagesReq := httptest.NewRequest(http.MethodGet, "/api/teams/project-beta/messages?limit=1", nil)
	limitedMessagesRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(limitedMessagesRec, limitedMessagesReq)
	if limitedMessagesRec.Code != http.StatusOK {
		t.Fatalf("limited messages api status = %d, body = %s", limitedMessagesRec.Code, limitedMessagesRec.Body.String())
	}
	if !strings.Contains(limitedMessagesRec.Body.String(), "\"limit\": 1") || !strings.Contains(limitedMessagesRec.Body.String(), "\"message_count\": 1") {
		t.Fatalf("expected clamped team messages api body, got %q", limitedMessagesRec.Body.String())
	}

	channelMessagesReq := httptest.NewRequest(http.MethodGet, "/api/teams/project-beta/channels/research/messages", nil)
	channelMessagesRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(channelMessagesRec, channelMessagesReq)
	if channelMessagesRec.Code != http.StatusOK {
		t.Fatalf("channel messages api status = %d, body = %s", channelMessagesRec.Code, channelMessagesRec.Body.String())
	}
	if !strings.Contains(channelMessagesRec.Body.String(), "\"scope\": \"team-channel-messages\"") || !strings.Contains(channelMessagesRec.Body.String(), "\"channel_id\": \"research\"") || !strings.Contains(channelMessagesRec.Body.String(), "Research channel keeps long-running coordination notes.") {
		t.Fatalf("expected team channel messages api body, got %q", channelMessagesRec.Body.String())
	}

	channelMessageCreateReq := httptest.NewRequest(http.MethodPost, "/api/teams/project-beta/channels/research/messages", strings.NewReader(`{
  "author_agent_id": "agent://pc75/live-bravo",
  "origin_public_key": "`+strings.Repeat("c", 64)+`",
  "parent_public_key": "`+strings.Repeat("d", 64)+`",
  "message_type": "note",
  "content": "Team channel message stays inside Team.",
  "structured_data": {"task_id":"team-task-implement-teamtask"}
}`))
	channelMessageCreateReq.RemoteAddr = "127.0.0.1:12345"
	channelMessageCreateReq.Header.Set("Content-Type", "application/json")
	channelMessageCreateRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(channelMessageCreateRec, channelMessageCreateReq)
	if channelMessageCreateRec.Code != http.StatusCreated {
		t.Fatalf("channel message create api status = %d, body = %s", channelMessageCreateRec.Code, channelMessageCreateRec.Body.String())
	}
	if !strings.Contains(channelMessageCreateRec.Body.String(), "\"scope\": \"team-message\"") || !strings.Contains(channelMessageCreateRec.Body.String(), "Team channel message stays inside Team.") {
		t.Fatalf("expected team channel message create api body, got %q", channelMessageCreateRec.Body.String())
	}

	channelPagePostReq := httptest.NewRequest(http.MethodPost, "/teams/project-beta/channels/research/messages/create", strings.NewReader(
		"author_agent_id=agent%3A%2F%2Fpc75%2Flive-bravo&message_type=decision&content=Channel+page+form+message&structured_data=%7B%22kind%22%3A%22page%22%7D",
	))
	channelPagePostReq.RemoteAddr = "127.0.0.1:12345"
	channelPagePostReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	channelPagePostRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(channelPagePostRec, channelPagePostReq)
	if channelPagePostRec.Code != http.StatusSeeOther {
		t.Fatalf("channel message create page status = %d, body = %s", channelPagePostRec.Code, channelPagePostRec.Body.String())
	}

	channelReq = httptest.NewRequest(http.MethodGet, "/teams/project-beta/channels/research", nil)
	channelRec = httptest.NewRecorder()
	site.Handler.ServeHTTP(channelRec, channelReq)
	if channelRec.Code != http.StatusOK {
		t.Fatalf("channel page status(after create) = %d, body = %s", channelRec.Code, channelRec.Body.String())
	}
	if !strings.Contains(channelRec.Body.String(), "发送 TeamMessage") || !strings.Contains(channelRec.Body.String(), "Channel page form message") || !strings.Contains(channelRec.Body.String(), "Team channel message stays inside Team.") {
		t.Fatalf("expected team channel page body after create, got %q", channelRec.Body.String())
	}

	historyAPIReq = httptest.NewRequest(http.MethodGet, "/api/teams/project-beta/history", nil)
	historyAPIRec = httptest.NewRecorder()
	site.Handler.ServeHTTP(historyAPIRec, historyAPIReq)
	if historyAPIRec.Code != http.StatusOK {
		t.Fatalf("history api(after message create) status = %d, body = %s", historyAPIRec.Code, historyAPIRec.Body.String())
	}
	if !strings.Contains(historyAPIRec.Body.String(), "发送 TeamMessage") || !strings.Contains(historyAPIRec.Body.String(), "\"scope\": \"message\"") || !strings.Contains(historyAPIRec.Body.String(), "\"diff_summary\"") {
		t.Fatalf("expected history api with team message entry, got %q", historyAPIRec.Body.String())
	}

	tasksReq := httptest.NewRequest(http.MethodGet, "/api/teams/project-beta/tasks", nil)
	tasksRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(tasksRec, tasksReq)
	if tasksRec.Code != http.StatusOK {
		t.Fatalf("tasks api status = %d, body = %s", tasksRec.Code, tasksRec.Body.String())
	}
	if !strings.Contains(tasksRec.Body.String(), "\"scope\": \"team-tasks\"") || !strings.Contains(tasksRec.Body.String(), "Implement TeamTask") {
		t.Fatalf("expected team tasks api body, got %q", tasksRec.Body.String())
	}

	filteredTasksReq := httptest.NewRequest(http.MethodGet, "/api/teams/project-beta/tasks?status=doing&assignee=agent%3A%2F%2Fpc75%2Flive-charlie&channel=research", nil)
	filteredTasksRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(filteredTasksRec, filteredTasksReq)
	if filteredTasksRec.Code != http.StatusOK {
		t.Fatalf("filtered tasks api status = %d, body = %s", filteredTasksRec.Code, filteredTasksRec.Body.String())
	}
	if !strings.Contains(filteredTasksRec.Body.String(), "\"status\": \"doing\"") || !strings.Contains(filteredTasksRec.Body.String(), "\"assignee\": \"agent://pc75/live-charlie\"") || !strings.Contains(filteredTasksRec.Body.String(), "\"channel\": \"research\"") || !strings.Contains(filteredTasksRec.Body.String(), "Implement TeamTask") {
		t.Fatalf("expected filtered team tasks api body, got %q", filteredTasksRec.Body.String())
	}

	filteredTasksPageReq := httptest.NewRequest(http.MethodGet, "/teams/project-beta/tasks?status=doing&label=rollout&channel=research", nil)
	filteredTasksPageRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(filteredTasksPageRec, filteredTasksPageReq)
	if filteredTasksPageRec.Code != http.StatusOK {
		t.Fatalf("filtered tasks page status = %d, body = %s", filteredTasksPageRec.Code, filteredTasksPageRec.Body.String())
	}
	if !strings.Contains(filteredTasksPageRec.Body.String(), "筛选任务") || !strings.Contains(filteredTasksPageRec.Body.String(), "已筛选") {
		t.Fatalf("expected filtered team tasks page body, got %q", filteredTasksPageRec.Body.String())
	}

	taskCreateReq := httptest.NewRequest(http.MethodPost, "/api/teams/project-beta/tasks", strings.NewReader(`{
  "task_id": "team-task-beta-2",
  "title": "Prepare beta rollout",
  "description": "Rollout task inside Team only.",
  "created_by": "agent://pc75/live-bravo",
  "channel_id": "research",
  "status": "open",
  "priority": "medium",
  "assignees": ["agent://pc75/live-charlie"],
  "labels": ["rollout", "beta"]
}`))
	taskCreateReq.RemoteAddr = "127.0.0.1:12345"
	taskCreateReq.Header.Set("Content-Type", "application/json")
	taskCreateRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(taskCreateRec, taskCreateReq)
	if taskCreateRec.Code != http.StatusCreated {
		t.Fatalf("task create api status = %d, body = %s", taskCreateRec.Code, taskCreateRec.Body.String())
	}
	if !strings.Contains(taskCreateRec.Body.String(), "\"scope\": \"team-task\"") || !strings.Contains(taskCreateRec.Body.String(), "Prepare beta rollout") || !strings.Contains(taskCreateRec.Body.String(), "\"channel_id\": \"research\"") {
		t.Fatalf("expected task create api body, got %q", taskCreateRec.Body.String())
	}

	filteredPlanningTasksReq := httptest.NewRequest(http.MethodGet, "/api/teams/project-beta/tasks?channel=planning", nil)
	filteredPlanningTasksRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(filteredPlanningTasksRec, filteredPlanningTasksReq)
	if filteredPlanningTasksRec.Code != http.StatusOK {
		t.Fatalf("planning channel filtered tasks api status = %d, body = %s", filteredPlanningTasksRec.Code, filteredPlanningTasksRec.Body.String())
	}
	if !strings.Contains(filteredPlanningTasksRec.Body.String(), "\"channel\": \"planning\"") || !strings.Contains(filteredPlanningTasksRec.Body.String(), "\"task_count\": 0") {
		t.Fatalf("expected empty planning filtered task api body before update, got %q", filteredPlanningTasksRec.Body.String())
	}

	taskAPIReq := httptest.NewRequest(http.MethodGet, "/api/teams/project-beta/tasks/team-task-implement-teamtask", nil)
	taskAPIRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(taskAPIRec, taskAPIReq)
	if taskAPIRec.Code != http.StatusOK {
		t.Fatalf("task api status = %d, body = %s", taskAPIRec.Code, taskAPIRec.Body.String())
	}
	if !strings.Contains(taskAPIRec.Body.String(), "\"scope\": \"team-task\"") || !strings.Contains(taskAPIRec.Body.String(), "\"message_count\": 2") || !strings.Contains(taskAPIRec.Body.String(), "Task comments stay inside TeamMessage, not Live.") || !strings.Contains(taskAPIRec.Body.String(), "Team channel message stays inside Team.") {
		t.Fatalf("expected team task api body, got %q", taskAPIRec.Body.String())
	}

	taskCommentAPIReq := httptest.NewRequest(http.MethodPost, "/api/teams/project-beta/tasks/team-task-implement-teamtask/comment", strings.NewReader(`{
  "author_agent_id": "agent://pc75/live-bravo",
  "channel_id": "research",
  "message_type": "comment",
  "content": "API task comment stays inside Team channel."
}`))
	taskCommentAPIReq.RemoteAddr = "127.0.0.1:12345"
	taskCommentAPIReq.Header.Set("Content-Type", "application/json")
	taskCommentAPIRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(taskCommentAPIRec, taskCommentAPIReq)
	if taskCommentAPIRec.Code != http.StatusCreated {
		t.Fatalf("task comment api status = %d, body = %s", taskCommentAPIRec.Code, taskCommentAPIRec.Body.String())
	}
	if !strings.Contains(taskCommentAPIRec.Body.String(), "\"scope\": \"team-task-comment\"") || !strings.Contains(taskCommentAPIRec.Body.String(), "API task comment stays inside Team channel.") {
		t.Fatalf("expected task comment api body, got %q", taskCommentAPIRec.Body.String())
	}

	taskUpdateReq := httptest.NewRequest(http.MethodPut, "/api/teams/project-beta/tasks/team-task-beta-2", strings.NewReader(`{
  "title": "Prepare beta rollout updated",
  "description": "Updated rollout task inside Team only.",
  "created_by": "agent://pc75/live-bravo",
  "channel_id": "planning",
  "status": "doing",
  "priority": "high",
  "assignees": ["agent://pc75/live-alpha", "agent://pc75/live-charlie"],
  "labels": ["rollout", "beta", "urgent"]
}`))
	taskUpdateReq.RemoteAddr = "127.0.0.1:12345"
	taskUpdateReq.Header.Set("Content-Type", "application/json")
	taskUpdateRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(taskUpdateRec, taskUpdateReq)
	if taskUpdateRec.Code != http.StatusOK {
		t.Fatalf("task update api status = %d, body = %s", taskUpdateRec.Code, taskUpdateRec.Body.String())
	}
	if !strings.Contains(taskUpdateRec.Body.String(), "Prepare beta rollout updated") || !strings.Contains(taskUpdateRec.Body.String(), "\"doing\"") || !strings.Contains(taskUpdateRec.Body.String(), "\"channel_id\": \"planning\"") {
		t.Fatalf("expected task update api body, got %q", taskUpdateRec.Body.String())
	}

	taskPageReq = httptest.NewRequest(http.MethodGet, "/teams/project-beta/tasks/team-task-beta-2", nil)
	taskPageRec = httptest.NewRecorder()
	site.Handler.ServeHTTP(taskPageRec, taskPageReq)
	if taskPageRec.Code != http.StatusOK {
		t.Fatalf("task page(updated) status = %d, body = %s", taskPageRec.Code, taskPageRec.Body.String())
	}
	if !strings.Contains(taskPageRec.Body.String(), "Prepare beta rollout updated") || !strings.Contains(taskPageRec.Body.String(), "保存任务") || !strings.Contains(taskPageRec.Body.String(), "关联频道：Planning Updated") || !strings.Contains(taskPageRec.Body.String(), "去任务频道补充讨论") || !strings.Contains(taskPageRec.Body.String(), "<option value=\"planning\" selected>Planning Updated</option>") {
		t.Fatalf("expected updated task page body, got %q", taskPageRec.Body.String())
	}

	filteredPlanningTasksReq = httptest.NewRequest(http.MethodGet, "/api/teams/project-beta/tasks?channel=planning", nil)
	filteredPlanningTasksRec = httptest.NewRecorder()
	site.Handler.ServeHTTP(filteredPlanningTasksRec, filteredPlanningTasksReq)
	if filteredPlanningTasksRec.Code != http.StatusOK {
		t.Fatalf("planning channel filtered tasks api(after update) status = %d, body = %s", filteredPlanningTasksRec.Code, filteredPlanningTasksRec.Body.String())
	}
	if !strings.Contains(filteredPlanningTasksRec.Body.String(), "\"channel\": \"planning\"") || !strings.Contains(filteredPlanningTasksRec.Body.String(), "Prepare beta rollout updated") {
		t.Fatalf("expected planning filtered task api body after update, got %q", filteredPlanningTasksRec.Body.String())
	}

	taskCommentPageReq := httptest.NewRequest(http.MethodPost, "/teams/project-beta/tasks/team-task-implement-teamtask/comment", strings.NewReader(
		"author_agent_id=agent%3A%2F%2Fpc75%2Flive-bravo&channel_id=main&message_type=comment&content=Page+task+comment+stays+inside+Team",
	))
	taskCommentPageReq.RemoteAddr = "127.0.0.1:12345"
	taskCommentPageReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	taskCommentPageRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(taskCommentPageRec, taskCommentPageReq)
	if taskCommentPageRec.Code != http.StatusSeeOther {
		t.Fatalf("task comment page status = %d, body = %s", taskCommentPageRec.Code, taskCommentPageRec.Body.String())
	}

	taskPageReq = httptest.NewRequest(http.MethodGet, "/teams/project-beta/tasks/team-task-implement-teamtask", nil)
	taskPageRec = httptest.NewRecorder()
	site.Handler.ServeHTTP(taskPageRec, taskPageReq)
	if taskPageRec.Code != http.StatusOK {
		t.Fatalf("task page(after comment) status = %d, body = %s", taskPageRec.Code, taskPageRec.Body.String())
	}
	if !strings.Contains(taskPageRec.Body.String(), "追加 Task 评论") || !strings.Contains(taskPageRec.Body.String(), "API task comment stays inside Team channel.") || !strings.Contains(taskPageRec.Body.String(), "Page task comment stays inside Team") || !strings.Contains(taskPageRec.Body.String(), "最近相关变更") || !strings.Contains(taskPageRec.Body.String(), "同状态任务") {
		t.Fatalf("expected team task page with comments, got %q", taskPageRec.Body.String())
	}

	taskDeleteReq := httptest.NewRequest(http.MethodDelete, "/api/teams/project-beta/tasks/team-task-beta-2", nil)
	taskDeleteReq.RemoteAddr = "127.0.0.1:12345"
	taskDeleteRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(taskDeleteRec, taskDeleteReq)
	if taskDeleteRec.Code != http.StatusOK {
		t.Fatalf("task delete api status = %d, body = %s", taskDeleteRec.Code, taskDeleteRec.Body.String())
	}
	if !strings.Contains(taskDeleteRec.Body.String(), "\"deleted\": true") {
		t.Fatalf("expected task delete api body, got %q", taskDeleteRec.Body.String())
	}

	memberUpdateReq := httptest.NewRequest(http.MethodPost, "/api/teams/project-beta/members", strings.NewReader(`{
  "agent_id": "agent://pc75/live-charlie",
  "role": "member",
  "status": "active"
}`))
	memberUpdateReq.RemoteAddr = "127.0.0.1:12345"
	memberUpdateReq.Header.Set("Content-Type", "application/json")
	memberUpdateRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(memberUpdateRec, memberUpdateReq)
	if memberUpdateRec.Code != http.StatusOK {
		t.Fatalf("member update api status = %d, body = %s", memberUpdateRec.Code, memberUpdateRec.Body.String())
	}
	if !strings.Contains(memberUpdateRec.Body.String(), "\"status\": \"active\"") || !strings.Contains(memberUpdateRec.Body.String(), "\"role\": \"member\"") {
		t.Fatalf("expected updated member api body, got %q", memberUpdateRec.Body.String())
	}

	artifactCreateReq := httptest.NewRequest(http.MethodPost, "/api/teams/project-beta/artifacts", strings.NewReader(`{
  "artifact_id": "artifact-beta-1",
  "title": "Team Beta Summary",
  "kind": "markdown",
  "summary": "Weekly recap",
  "content": "Long-form notes stay inside Team Artifact.",
  "created_by": "agent://pc75/live-bravo",
  "channel_id": "research"
}`))
	artifactCreateReq.RemoteAddr = "127.0.0.1:12345"
	artifactCreateReq.Header.Set("Content-Type", "application/json")
	artifactCreateRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(artifactCreateRec, artifactCreateReq)
	if artifactCreateRec.Code != http.StatusCreated {
		t.Fatalf("artifact create api status = %d, body = %s", artifactCreateRec.Code, artifactCreateRec.Body.String())
	}
	if !strings.Contains(artifactCreateRec.Body.String(), "\"scope\": \"team-artifact\"") || !strings.Contains(artifactCreateRec.Body.String(), "Team Beta Summary") {
		t.Fatalf("expected artifact create api body, got %q", artifactCreateRec.Body.String())
	}

	artifactsPageReq := httptest.NewRequest(http.MethodGet, "/teams/project-beta/artifacts", nil)
	artifactsPageRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(artifactsPageRec, artifactsPageReq)
	if artifactsPageRec.Code != http.StatusOK {
		t.Fatalf("artifacts page status = %d, body = %s", artifactsPageRec.Code, artifactsPageRec.Body.String())
	}
	if !strings.Contains(artifactsPageRec.Body.String(), "Team Beta Summary") || !strings.Contains(artifactsPageRec.Body.String(), "/teams/project-beta/artifacts/artifact-beta-1") || !strings.Contains(artifactsPageRec.Body.String(), "筛选产物") {
		t.Fatalf("expected artifacts page body, got %q", artifactsPageRec.Body.String())
	}

	artifactsFilteredPageReq := httptest.NewRequest(http.MethodGet, "/teams/project-beta/artifacts?kind=markdown&channel=research", nil)
	artifactsFilteredPageRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(artifactsFilteredPageRec, artifactsFilteredPageReq)
	if artifactsFilteredPageRec.Code != http.StatusOK {
		t.Fatalf("filtered artifacts page status = %d, body = %s", artifactsFilteredPageRec.Code, artifactsFilteredPageRec.Body.String())
	}
	if !strings.Contains(artifactsFilteredPageRec.Body.String(), "已筛选") || !strings.Contains(artifactsFilteredPageRec.Body.String(), "Team Beta Summary") {
		t.Fatalf("expected filtered artifacts page body, got %q", artifactsFilteredPageRec.Body.String())
	}

	artifactPageReq := httptest.NewRequest(http.MethodGet, "/teams/project-beta/artifacts/artifact-beta-1", nil)
	artifactPageRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(artifactPageRec, artifactPageReq)
	if artifactPageRec.Code != http.StatusOK {
		t.Fatalf("artifact page status = %d, body = %s", artifactPageRec.Code, artifactPageRec.Body.String())
	}
	if !strings.Contains(artifactPageRec.Body.String(), "Long-form notes stay inside Team Artifact.") || !strings.Contains(artifactPageRec.Body.String(), "结果预览") || !strings.Contains(artifactPageRec.Body.String(), "最近相关变更") {
		t.Fatalf("expected artifact page body, got %q", artifactPageRec.Body.String())
	}

	artifactsAPIReq := httptest.NewRequest(http.MethodGet, "/api/teams/project-beta/artifacts", nil)
	artifactsAPIRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(artifactsAPIRec, artifactsAPIReq)
	if artifactsAPIRec.Code != http.StatusOK {
		t.Fatalf("artifacts api status = %d, body = %s", artifactsAPIRec.Code, artifactsAPIRec.Body.String())
	}
	if !strings.Contains(artifactsAPIRec.Body.String(), "\"scope\": \"team-artifacts\"") || !strings.Contains(artifactsAPIRec.Body.String(), "\"artifact_count\": 1") {
		t.Fatalf("expected artifacts api body, got %q", artifactsAPIRec.Body.String())
	}

	artifactsFilteredAPIReq := httptest.NewRequest(http.MethodGet, "/api/teams/project-beta/artifacts?kind=markdown&task=", nil)
	artifactsFilteredAPIRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(artifactsFilteredAPIRec, artifactsFilteredAPIReq)
	if artifactsFilteredAPIRec.Code != http.StatusOK {
		t.Fatalf("filtered artifacts api status = %d, body = %s", artifactsFilteredAPIRec.Code, artifactsFilteredAPIRec.Body.String())
	}
	if !strings.Contains(artifactsFilteredAPIRec.Body.String(), "\"applied_filters\"") || !strings.Contains(artifactsFilteredAPIRec.Body.String(), "\"kind\": \"markdown\"") {
		t.Fatalf("expected filtered artifacts api body, got %q", artifactsFilteredAPIRec.Body.String())
	}

	artifactAPIReq := httptest.NewRequest(http.MethodGet, "/api/teams/project-beta/artifacts/artifact-beta-1", nil)
	artifactAPIRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(artifactAPIRec, artifactAPIReq)
	if artifactAPIRec.Code != http.StatusOK {
		t.Fatalf("artifact api status = %d, body = %s", artifactAPIRec.Code, artifactAPIRec.Body.String())
	}
	if !strings.Contains(artifactAPIRec.Body.String(), "\"scope\": \"team-artifact\"") || !strings.Contains(artifactAPIRec.Body.String(), "artifact-beta-1") {
		t.Fatalf("expected artifact api body, got %q", artifactAPIRec.Body.String())
	}

	artifactUpdateReq := httptest.NewRequest(http.MethodPut, "/api/teams/project-beta/artifacts/artifact-beta-1", strings.NewReader(`{
  "title": "Team Beta Summary Updated",
  "kind": "link",
  "summary": "Updated recap",
  "link_url": "https://example.com/team-beta",
  "channel_id": "main",
  "labels": ["weekly", "beta"]
}`))
	artifactUpdateReq.RemoteAddr = "127.0.0.1:12345"
	artifactUpdateReq.Header.Set("Content-Type", "application/json")
	artifactUpdateRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(artifactUpdateRec, artifactUpdateReq)
	if artifactUpdateRec.Code != http.StatusOK {
		t.Fatalf("artifact update api status = %d, body = %s", artifactUpdateRec.Code, artifactUpdateRec.Body.String())
	}
	if !strings.Contains(artifactUpdateRec.Body.String(), "Team Beta Summary Updated") || !strings.Contains(artifactUpdateRec.Body.String(), "https://example.com/team-beta") {
		t.Fatalf("expected artifact update api body, got %q", artifactUpdateRec.Body.String())
	}

	artifactPageReq = httptest.NewRequest(http.MethodGet, "/teams/project-beta/artifacts/artifact-beta-1", nil)
	artifactPageRec = httptest.NewRecorder()
	site.Handler.ServeHTTP(artifactPageRec, artifactPageReq)
	if artifactPageRec.Code != http.StatusOK {
		t.Fatalf("artifact page(updated) status = %d, body = %s", artifactPageRec.Code, artifactPageRec.Body.String())
	}
	if !strings.Contains(artifactPageRec.Body.String(), "Team Beta Summary Updated") || !strings.Contains(artifactPageRec.Body.String(), "保存产物") || !strings.Contains(artifactPageRec.Body.String(), "关联上下文") || !strings.Contains(artifactPageRec.Body.String(), "最近相关变更") {
		t.Fatalf("expected updated artifact page body, got %q", artifactPageRec.Body.String())
	}

	artifactDeleteReq := httptest.NewRequest(http.MethodDelete, "/api/teams/project-beta/artifacts/artifact-beta-1", nil)
	artifactDeleteReq.RemoteAddr = "127.0.0.1:12345"
	artifactDeleteRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(artifactDeleteRec, artifactDeleteReq)
	if artifactDeleteRec.Code != http.StatusOK {
		t.Fatalf("artifact delete api status = %d, body = %s", artifactDeleteRec.Code, artifactDeleteRec.Body.String())
	}
	if !strings.Contains(artifactDeleteRec.Body.String(), "\"deleted\": true") {
		t.Fatalf("expected artifact delete api body, got %q", artifactDeleteRec.Body.String())
	}

	artifactsAPIReq = httptest.NewRequest(http.MethodGet, "/api/teams/project-beta/artifacts", nil)
	artifactsAPIRec = httptest.NewRecorder()
	site.Handler.ServeHTTP(artifactsAPIRec, artifactsAPIReq)
	if artifactsAPIRec.Code != http.StatusOK {
		t.Fatalf("artifacts api(after delete) status = %d, body = %s", artifactsAPIRec.Code, artifactsAPIRec.Body.String())
	}
	if !strings.Contains(artifactsAPIRec.Body.String(), "\"artifact_count\": 0") {
		t.Fatalf("expected empty artifacts api body after delete, got %q", artifactsAPIRec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/teams/project-beta", nil)
	rec = httptest.NewRecorder()
	site.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("team detail(after history) status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "最近变更") || !strings.Contains(rec.Body.String(), "删除 Team Artifact") || !strings.Contains(rec.Body.String(), "批量处理成员") {
		t.Fatalf("expected team detail history body, got %q", rec.Body.String())
	}
}

func TestPluginBuildHandlesTeamTaskFormWrites(t *testing.T) {
	t.Parallel()

	site, root := buildTeamSite(t)
	store, err := teamcore.OpenStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	teamRoot := filepath.Join(root, "store", "team", "project-forms")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{
  "team_id": "project-forms",
  "title": "Project Forms",
  "owner_agent_id": "agent://pc75/live-alpha",
  "channels": ["main"]
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	if err := store.AppendTask("project-forms", teamcore.Task{
		TaskID:    "form-task-1",
		CreatedBy: "agent://pc75/live-alpha",
		Title:     "Original form task",
		Status:    "open",
		Priority:  "medium",
		CreatedAt: time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendTask error = %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/teams/project-forms/tasks/create", strings.NewReader(
		"title=Created+via+form&status=doing&priority=high&assignees=agent%3A%2F%2Fpc75%2Flive-bravo&created_by=agent%3A%2F%2Fpc75%2Flive-alpha&description=form+task",
	))
	createReq.RemoteAddr = "127.0.0.1:23456"
	createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusSeeOther {
		t.Fatalf("task create form status = %d, body = %s", createRec.Code, createRec.Body.String())
	}
	location := createRec.Header().Get("Location")
	if !strings.Contains(location, "/teams/project-forms/tasks/") {
		t.Fatalf("expected redirect to task page, got %q", location)
	}

	tasks, err := store.LoadTasks("project-forms", 10)
	if err != nil {
		t.Fatalf("LoadTasks error = %v", err)
	}
	if len(tasks) != 2 || tasks[0].Title != "Created via form" {
		t.Fatalf("expected created task at top, got %#v", tasks)
	}

	updateReq := httptest.NewRequest(http.MethodPost, "/teams/project-forms/tasks/form-task-1/update", strings.NewReader(
		"title=Updated+via+form&status=done&priority=high&assignees=agent%3A%2F%2Fpc75%2Flive-charlie&labels=weekly%2Cdone&description=form+update&actor_agent_id=agent%3A%2F%2Fpc75%2Flive-alpha",
	))
	updateReq.RemoteAddr = "127.0.0.1:23456"
	updateReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	updateRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusSeeOther {
		t.Fatalf("task update form status = %d, body = %s", updateRec.Code, updateRec.Body.String())
	}

	updatedTask, err := store.LoadTask("project-forms", "form-task-1")
	if err != nil {
		t.Fatalf("LoadTask(updated) error = %v", err)
	}
	if updatedTask.Title != "Updated via form" || updatedTask.Status != "done" || updatedTask.ClosedAt.IsZero() {
		t.Fatalf("unexpected updated form task: %#v", updatedTask)
	}

	deleteReq := httptest.NewRequest(http.MethodPost, "/teams/project-forms/tasks/form-task-1/delete", strings.NewReader(
		"actor_agent_id=agent%3A%2F%2Fpc75%2Flive-alpha",
	))
	deleteReq.RemoteAddr = "127.0.0.1:23456"
	deleteReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	deleteRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusSeeOther {
		t.Fatalf("task delete form status = %d, body = %s", deleteRec.Code, deleteRec.Body.String())
	}
	if _, err := store.LoadTask("project-forms", "form-task-1"); !os.IsNotExist(err) {
		t.Fatalf("expected deleted form task to be missing, got %v", err)
	}
}

func TestPluginBuildHandlesTeamChannelFormWrites(t *testing.T) {
	t.Parallel()

	site, root := buildTeamSite(t)
	store, err := teamcore.OpenStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	teamRoot := filepath.Join(root, "store", "team", "project-channel-forms")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{
  "team_id":"project-channel-forms",
  "title":"Project Channel Forms",
  "channels":["main"]
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	if err := store.SaveChannel("project-channel-forms", teamcore.Channel{
		ChannelID:   "main",
		Title:       "Main Channel",
		Description: "Primary coordination channel",
		CreatedAt:   time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveChannel(main) error = %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/teams/project-channel-forms/channels/create", strings.NewReader(
		"channel_id=planning&title=Planning+Board&description=Planning+inside+Team",
	))
	createReq.RemoteAddr = "127.0.0.1:23456"
	createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusSeeOther {
		t.Fatalf("channel create form status = %d, body = %s", createRec.Code, createRec.Body.String())
	}
	if location := createRec.Header().Get("Location"); !strings.Contains(location, "/teams/project-channel-forms/channels/planning") {
		t.Fatalf("expected channel create redirect, got %q", location)
	}

	channel, err := store.LoadChannel("project-channel-forms", "planning")
	if err != nil {
		t.Fatalf("LoadChannel(created) error = %v", err)
	}
	if channel.Title != "Planning Board" || channel.Description != "Planning inside Team" {
		t.Fatalf("unexpected created channel: %#v", channel)
	}

	updateReq := httptest.NewRequest(http.MethodPost, "/teams/project-channel-forms/channels/planning/update", strings.NewReader(
		"title=Planning+Updated&description=Updated+planning+details",
	))
	updateReq.RemoteAddr = "127.0.0.1:23456"
	updateReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	updateRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusSeeOther {
		t.Fatalf("channel update form status = %d, body = %s", updateRec.Code, updateRec.Body.String())
	}

	channel, err = store.LoadChannel("project-channel-forms", "planning")
	if err != nil {
		t.Fatalf("LoadChannel(updated) error = %v", err)
	}
	if channel.Title != "Planning Updated" || channel.Description != "Updated planning details" || channel.Hidden {
		t.Fatalf("unexpected updated channel: %#v", channel)
	}

	hideReq := httptest.NewRequest(http.MethodPost, "/teams/project-channel-forms/channels/planning/hide", nil)
	hideReq.RemoteAddr = "127.0.0.1:23456"
	hideRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(hideRec, hideReq)
	if hideRec.Code != http.StatusSeeOther {
		t.Fatalf("channel hide form status = %d, body = %s", hideRec.Code, hideRec.Body.String())
	}

	channel, err = store.LoadChannel("project-channel-forms", "planning")
	if err != nil {
		t.Fatalf("LoadChannel(hidden) error = %v", err)
	}
	if !channel.Hidden {
		t.Fatalf("expected hidden channel, got %#v", channel)
	}
}

func TestPluginBuildHandlesTeamTaskStatusAndArtifactTaskRelation(t *testing.T) {
	t.Parallel()

	site, root := buildTeamSite(t)
	store, err := teamcore.OpenStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	teamRoot := filepath.Join(root, "store", "team", "project-task-artifact")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-task-artifact","title":"Project Task Artifact","channels":["main"]}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	if err := store.AppendTask("project-task-artifact", teamcore.Task{
		TaskID:    "task-artifact-1",
		Title:     "Wire task and artifact together",
		Status:    "open",
		Priority:  "medium",
		CreatedBy: "agent://pc75/live-alpha",
		CreatedAt: time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendTask error = %v", err)
	}

	statusReq := httptest.NewRequest(http.MethodPost, "/teams/project-task-artifact/tasks/task-artifact-1/status", strings.NewReader("status=review"))
	statusReq.RemoteAddr = "127.0.0.1:23456"
	statusReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	statusRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusSeeOther {
		t.Fatalf("task status form status = %d, body = %s", statusRec.Code, statusRec.Body.String())
	}
	task, err := store.LoadTask("project-task-artifact", "task-artifact-1")
	if err != nil {
		t.Fatalf("LoadTask error = %v", err)
	}
	if task.Status != "review" {
		t.Fatalf("expected task status review, got %#v", task)
	}

	artifactReq := httptest.NewRequest(http.MethodPost, "/teams/project-task-artifact/artifacts/create", strings.NewReader(
		"title=Artifact+with+Task&kind=markdown&channel_id=main&task_id=task-artifact-1&summary=Task+linked+artifact&content=Artifact+content",
	))
	artifactReq.RemoteAddr = "127.0.0.1:23456"
	artifactReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	artifactRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(artifactRec, artifactReq)
	if artifactRec.Code != http.StatusSeeOther {
		t.Fatalf("artifact create form status = %d, body = %s", artifactRec.Code, artifactRec.Body.String())
	}
	artifacts, err := store.LoadArtifacts("project-task-artifact", 10)
	if err != nil || len(artifacts) != 1 {
		t.Fatalf("LoadArtifacts error=%v artifacts=%#v", err, artifacts)
	}
	if artifacts[0].TaskID != "task-artifact-1" {
		t.Fatalf("expected artifact task relation, got %#v", artifacts[0])
	}

	taskPageReq := httptest.NewRequest(http.MethodGet, "/teams/project-task-artifact/tasks/task-artifact-1", nil)
	taskPageRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(taskPageRec, taskPageReq)
	if taskPageRec.Code != http.StatusOK {
		t.Fatalf("task page status = %d, body = %s", taskPageRec.Code, taskPageRec.Body.String())
	}
	if !strings.Contains(taskPageRec.Body.String(), "从任务创建产物") || !strings.Contains(taskPageRec.Body.String(), "关联产物") || !strings.Contains(taskPageRec.Body.String(), "Artifact with Task") || !strings.Contains(taskPageRec.Body.String(), "最近相关变更") {
		t.Fatalf("expected task page to show related artifact context, got %q", taskPageRec.Body.String())
	}

	tasksPageReq := httptest.NewRequest(http.MethodGet, "/teams/project-task-artifact/tasks", nil)
	tasksPageRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(tasksPageRec, tasksPageReq)
	if tasksPageRec.Code != http.StatusOK {
		t.Fatalf("tasks page status = %d, body = %s", tasksPageRec.Code, tasksPageRec.Body.String())
	}
	if !strings.Contains(tasksPageRec.Body.String(), "1 个产物") || !strings.Contains(tasksPageRec.Body.String(), "查看关联产物") {
		t.Fatalf("expected tasks page to show artifact context counts, got %q", tasksPageRec.Body.String())
	}

	artifactPageReq := httptest.NewRequest(http.MethodGet, "/teams/project-task-artifact/artifacts/"+url.PathEscape(artifacts[0].ArtifactID), nil)
	artifactPageRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(artifactPageRec, artifactPageReq)
	if artifactPageRec.Code != http.StatusOK {
		t.Fatalf("artifact page status = %d, body = %s", artifactPageRec.Code, artifactPageRec.Body.String())
	}
	if !strings.Contains(artifactPageRec.Body.String(), "task-artifact-1") || !strings.Contains(artifactPageRec.Body.String(), "关联任务摘要") || !strings.Contains(artifactPageRec.Body.String(), "Wire task and artifact together") {
		t.Fatalf("expected artifact page to show task relation, got %q", artifactPageRec.Body.String())
	}
}

func TestPluginBuildServesAndResolvesTeamSyncConflicts(t *testing.T) {
	t.Parallel()

	site, root := buildTeamSite(t)
	teamRoot := filepath.Join(root, "store", "team", "sync-conflict-team")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(teamRoot) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{
  "team_id":"sync-conflict-team",
  "title":"Sync Conflict Team",
  "owner_agent_id":"agent://pc75/live-alpha"
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "members.json"), []byte(`[
  {"agent_id":"agent://pc75/live-alpha","role":"owner","status":"active"}
]`), 0o644); err != nil {
		t.Fatalf("WriteFile(members.json) error = %v", err)
	}
	writeTeamSyncStateFixture(t, root, map[string]any{
		"conflicts": map[string]any{
			"task:task-conflict-1:2026-04-04T15:00:00Z": map[string]any{
				"key":         "task:task-conflict-1:2026-04-04T15:00:00Z",
				"type":        "task",
				"team_id":     "sync-conflict-team",
				"subject_id":  "task-conflict-1",
				"source_node": "node-75",
				"reason":      "local_newer",
				"sync": map[string]any{
					"type":        "task",
					"team_id":     "sync-conflict-team",
					"source_node": "node-75",
					"created_at":  "2026-04-04T15:00:00Z",
					"task": map[string]any{
						"team_id":    "sync-conflict-team",
						"task_id":    "task-conflict-1",
						"title":      "Remote conflicted task",
						"status":     "doing",
						"priority":   "high",
						"created_by": "agent://pc75/live-alpha",
						"created_at": "2026-04-04T14:59:00Z",
						"updated_at": "2026-04-04T15:00:00Z",
						"context_id": "ctx-sync-conflict-1",
						"channel_id": "main",
					},
				},
				"updated_at": "2026-04-04T15:00:01Z",
			},
		},
	})

	conflictsReq := httptest.NewRequest(http.MethodGet, "/api/teams/sync-conflict-team/sync/conflicts", nil)
	conflictsRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(conflictsRec, conflictsReq)
	if conflictsRec.Code != http.StatusOK {
		t.Fatalf("conflicts api status = %d, body = %s", conflictsRec.Code, conflictsRec.Body.String())
	}
	if !strings.Contains(conflictsRec.Body.String(), `"scope": "team-sync-conflicts"`) || !strings.Contains(conflictsRec.Body.String(), `"reason": "local_newer"`) {
		t.Fatalf("expected conflicts api body, got %q", conflictsRec.Body.String())
	}

	resolveReq := httptest.NewRequest(http.MethodPost, "/api/teams/sync-conflict-team/sync/conflicts/"+url.PathEscape("task:task-conflict-1:2026-04-04T15:00:00Z")+"/resolve", strings.NewReader(`{
  "actor_agent_id":"agent://pc75/live-alpha",
  "action":"dismiss"
}`))
	resolveReq.RemoteAddr = "127.0.0.1:12345"
	resolveRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(resolveRec, resolveReq)
	if resolveRec.Code != http.StatusOK {
		t.Fatalf("resolve api status = %d, body = %s", resolveRec.Code, resolveRec.Body.String())
	}
	if !strings.Contains(resolveRec.Body.String(), `"scope": "team-sync-conflict-resolve"`) || !strings.Contains(resolveRec.Body.String(), `"resolution": "dismiss"`) {
		t.Fatalf("expected resolve api body, got %q", resolveRec.Body.String())
	}

	historyReq := httptest.NewRequest(http.MethodGet, "/api/teams/sync-conflict-team/history?scope=sync-conflict", nil)
	historyRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(historyRec, historyReq)
	if historyRec.Code != http.StatusOK {
		t.Fatalf("history api status = %d, body = %s", historyRec.Code, historyRec.Body.String())
	}
	if !strings.Contains(historyRec.Body.String(), `"scope": "team-history"`) || !strings.Contains(historyRec.Body.String(), `"resolution_after": "dismiss"`) {
		t.Fatalf("expected conflict resolution history body, got %q", historyRec.Body.String())
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/teams/sync-conflict-team", nil)
	detailRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("team detail status = %d, body = %s", detailRec.Code, detailRec.Body.String())
	}
	if !strings.Contains(detailRec.Body.String(), "最近复制冲突") {
		t.Fatalf("expected conflict summary on team detail, got %q", detailRec.Body.String())
	}

	pageReq := httptest.NewRequest(http.MethodGet, "/teams/sync-conflict-team/history", nil)
	pageRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(pageRec, pageReq)
	if pageRec.Code != http.StatusOK {
		t.Fatalf("team history page status = %d, body = %s", pageRec.Code, pageRec.Body.String())
	}
	if !strings.Contains(pageRec.Body.String(), "最近复制冲突") || !strings.Contains(pageRec.Body.String(), "冲突 JSON") {
		t.Fatalf("expected conflict summary on team history page, got %q", pageRec.Body.String())
	}
}

func TestBuildTeamSyncConflictViewsExplainsSuggestions(t *testing.T) {
	t.Parallel()

	views := buildTeamSyncConflictViews([]corehaonews.TeamSyncConflictRecord{
		{
			Key:       "task:task-1:remote",
			Type:      "task",
			SyncType:  "task",
			TeamID:    "team-1",
			SubjectID: "task-1",
			Reason:    "local_newer",
		},
		{
			Key:       "artifact:artifact-1:remote",
			Type:      "artifact",
			SyncType:  "artifact",
			TeamID:    "team-1",
			SubjectID: "artifact-1",
			Reason:    "same_version_diverged",
		},
		{
			Key:       "policy:team-1:remote",
			Type:      "policy",
			SyncType:  "policy",
			TeamID:    "team-1",
			SubjectID: "team-1",
			Reason:    "signature_rejected",
		},
	})
	if len(views) != 3 {
		t.Fatalf("views = %d, want 3", len(views))
	}
	if views[0].SuggestedAction != "keep_local" || !strings.Contains(views[0].ReasonLabel, "本地") || !strings.Contains(views[0].ActionHint, "保留本地") {
		t.Fatalf("unexpected local_newer view: %#v", views[0])
	}
	if views[0].ConflictClass != "safe-local" || !views[0].AllowKeepLocal || views[0].SubjectLabel != "Task / task-1" {
		t.Fatalf("unexpected local_newer metadata: %#v", views[0])
	}
	if views[1].SuggestedAction != "accept_remote" || !strings.Contains(views[1].ReasonLabel, "分叉") || !strings.Contains(views[1].ActionHint, "接收远端") {
		t.Fatalf("unexpected same_version_diverged view: %#v", views[1])
	}
	if views[1].ConflictClass != "diverged" || !views[1].AllowAcceptRemote || len(views[1].Actions) < 3 {
		t.Fatalf("unexpected diverged actions: %#v", views[1])
	}
	if views[2].SuggestedAction != "dismiss" || !strings.Contains(views[2].ReasonLabel, "签名") || !strings.Contains(views[2].ActionHint, "驳回") {
		t.Fatalf("unexpected signature_rejected view: %#v", views[2])
	}
	if views[2].ConflictClass != "rejected" || views[2].AllowAcceptRemote {
		t.Fatalf("unexpected signature_rejected metadata: %#v", views[2])
	}
}

func TestPluginBuildServesTeamSyncHealthPageAndAPI(t *testing.T) {
	t.Parallel()

	site, root := buildTeamSite(t)
	teamRoot := filepath.Join(root, "store", "team", "sync-health-team")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(teamRoot) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{
  "team_id":"sync-health-team",
  "title":"Sync Health Team",
  "visibility":"private",
  "owner_agent_id":"agent://pc75/live-alpha"
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	writeSyncRuntimeStatusFixture(t, root, map[string]any{
		"team_sync": map[string]any{
			"enabled":              true,
			"node_id":              "node-75",
			"state_loaded":         true,
			"persisted_cursors":    9,
			"persisted_peer_acks":  4,
			"ack_peers":            2,
			"pending_acks":         3,
			"conflicts":            1,
			"subscribed_teams":     1,
			"published_messages":   7,
			"applied_messages":     6,
			"last_published_key":   "message:sync-health-team:main:msg-1",
			"last_conflict_key":    "task:sync-health-task:2026-04-04T15:00:00Z",
			"last_conflict_reason": "local_newer",
		},
	})
	writeTeamSyncStateFixture(t, root, map[string]any{
		"conflicts": map[string]any{
			"task:sync-health-task:2026-04-04T15:00:00Z": map[string]any{
				"key":         "task:sync-health-task:2026-04-04T15:00:00Z",
				"type":        "task",
				"team_id":     "sync-health-team",
				"subject_id":  "sync-health-task",
				"source_node": "node-74",
				"reason":      "local_newer",
				"updated_at":  "2026-04-04T15:01:00Z",
			},
		},
	})

	pageReq := httptest.NewRequest(http.MethodGet, "/teams/sync-health-team/sync", nil)
	pageRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(pageRec, pageReq)
	if pageRec.Code != http.StatusOK {
		t.Fatalf("team sync page status = %d, body = %s", pageRec.Code, pageRec.Body.String())
	}
	if !strings.Contains(pageRec.Body.String(), "Team Sync 健康") || !strings.Contains(pageRec.Body.String(), "pending ack") || !strings.Contains(pageRec.Body.String(), "最近复制冲突") || !strings.Contains(pageRec.Body.String(), "Webhook 投递") {
		t.Fatalf("expected sync health page content, got %q", pageRec.Body.String())
	}

	apiReq := httptest.NewRequest(http.MethodGet, "/api/teams/sync-health-team/sync", nil)
	apiRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(apiRec, apiReq)
	if apiRec.Code != http.StatusOK {
		t.Fatalf("team sync api status = %d, body = %s", apiRec.Code, apiRec.Body.String())
	}
	if !strings.Contains(apiRec.Body.String(), `"scope": "team-sync-health"`) || !strings.Contains(apiRec.Body.String(), `"pending_acks": 3`) || !strings.Contains(apiRec.Body.String(), `"conflict_count": 1`) || !strings.Contains(apiRec.Body.String(), `"allow_accept_remote": true`) || !strings.Contains(apiRec.Body.String(), `"webhook_status":`) {
		t.Fatalf("expected sync health api body, got %q", apiRec.Body.String())
	}

	resolveReq := httptest.NewRequest(http.MethodPost, "/teams/sync-health-team/sync/conflicts/"+url.PathEscape("task:sync-health-task:2026-04-04T15:00:00Z")+"/resolve", strings.NewReader("actor_agent_id=agent%3A%2F%2Fpc75%2Flive-alpha&action=dismiss"))
	resolveReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resolveReq.RemoteAddr = "127.0.0.1:12345"
	resolveRec := httptest.NewRecorder()
	site.Handler.ServeHTTP(resolveRec, resolveReq)
	if resolveRec.Code != http.StatusSeeOther {
		t.Fatalf("team sync page resolve status = %d, body = %s", resolveRec.Code, resolveRec.Body.String())
	}
	location := resolveRec.Header().Get("Location")
	if !strings.Contains(location, "/teams/sync-health-team/sync") || !strings.Contains(location, "resolved=dismiss") {
		t.Fatalf("expected sync page redirect, got %q", location)
	}
}

func buildTeamSite(t *testing.T) (*apphost.Site, string) {
	t.Helper()

	root := t.TempDir()
	site, err := Plugin{}.Build(context.Background(), apphost.Config{
		StoreRoot:        filepath.Join(root, "store"),
		Project:          "hao.news",
		Version:          "dev",
		ArchiveRoot:      filepath.Join(root, "archive"),
		RulesPath:        filepath.Join(root, "config", "subscriptions.json"),
		WriterPolicyPath: filepath.Join(root, "config", "writer_policy.json"),
		NetPath:          filepath.Join(root, "config", "haonews_net.inf"),
		Plugin:           "hao-news-team",
		Plugins:          []string{"hao-news-content", "hao-news-live", "hao-news-team", "hao-news-archive", "hao-news-governance", "hao-news-ops"},
	}, themehaonews.Theme{})
	if err != nil {
		t.Fatalf("Plugin.Build error = %v", err)
	}
	return site, root
}

func writeTeamSyncStateFixture(t *testing.T, root string, payload map[string]any) {
	t.Helper()

	syncRoot := filepath.Join(root, "store", "sync")
	if err := os.MkdirAll(syncRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(syncRoot) error = %v", err)
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent(teamSyncState) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(syncRoot, "team_sync_state.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile(team_sync_state.json) error = %v", err)
	}
}

func writeSyncRuntimeStatusFixture(t *testing.T, root string, payload map[string]any) {
	t.Helper()

	syncRoot := filepath.Join(root, "store", "sync")
	if err := os.MkdirAll(syncRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(syncRoot) error = %v", err)
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent(syncRuntimeStatus) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(syncRoot, "status.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile(status.json) error = %v", err)
	}
}
