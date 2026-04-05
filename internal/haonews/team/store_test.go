package team

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestStoreListTeams(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-alpha")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{
  "team_id": "project-alpha",
  "title": "Project Alpha",
  "description": "Long-running multi-agent project",
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

	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	teams, err := store.ListTeams()
	if err != nil {
		t.Fatalf("ListTeams error = %v", err)
	}
	if len(teams) != 1 {
		t.Fatalf("expected 1 team, got %d", len(teams))
	}
	if teams[0].Title != "Project Alpha" || teams[0].MemberCount != 2 || teams[0].ChannelCount != 2 {
		t.Fatalf("unexpected team summary: %#v", teams[0])
	}
}

func TestStoreLoadTeamDefaults(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "demo-team")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{
  "title": "Demo Team"
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	info, err := store.LoadTeam("demo-team")
	if err != nil {
		t.Fatalf("LoadTeam error = %v", err)
	}
	if info.TeamID != "demo-team" || info.Slug != "demo-team" || info.Visibility != "team" {
		t.Fatalf("unexpected info defaults: %#v", info)
	}
	if len(info.Channels) != 1 || info.Channels[0] != "main" {
		t.Fatalf("expected default main channel, got %#v", info.Channels)
	}
}

func TestStoreLoadPolicyDefaultsAndNormalize(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "policy-team")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"policy-team","title":"Policy Team"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	defaults, err := store.LoadPolicy("policy-team")
	if err != nil {
		t.Fatalf("LoadPolicy(defaults) error = %v", err)
	}
	if strings.Join(defaults.MessageRoles, ",") != "owner,maintainer,member" {
		t.Fatalf("unexpected default message roles: %#v", defaults)
	}
	if err := store.SavePolicy("policy-team", Policy{
		MessageRoles:    []string{"Maintainer", "observer", "maintainer"},
		TaskRoles:       []string{"owner", "member"},
		SystemNoteRoles: []string{"owner", "owner", "Maintainer"},
	}); err != nil {
		t.Fatalf("SavePolicy error = %v", err)
	}
	policy, err := store.LoadPolicy("policy-team")
	if err != nil {
		t.Fatalf("LoadPolicy error = %v", err)
	}
	if strings.Join(policy.MessageRoles, ",") != "maintainer,observer" {
		t.Fatalf("unexpected normalized message roles: %#v", policy.MessageRoles)
	}
	if strings.Join(policy.SystemNoteRoles, ",") != "owner,maintainer" {
		t.Fatalf("unexpected normalized system note roles: %#v", policy.SystemNoteRoles)
	}
}

func TestPolicyAllowsUsesLegacyAndExplicitPermissions(t *testing.T) {
	t.Parallel()

	policy := normalizePolicy(Policy{})
	if !policy.Allows("message.send", "member") {
		t.Fatalf("expected default legacy policy to allow member message.send")
	}
	if policy.Allows("policy.update", "member") {
		t.Fatalf("expected default legacy policy to deny member policy.update")
	}

	policy = normalizePolicy(Policy{
		Permissions: map[string][]string{
			"message.send":  {"owner"},
			"policy.update": {"owner"},
		},
	})
	if policy.Allows("message.send", "member") {
		t.Fatalf("expected explicit permissions to override legacy message.send")
	}
	if !policy.Allows("message.send", "owner") {
		t.Fatalf("expected explicit permissions to allow owner message.send")
	}
	if !policy.Allows("task.create", "member") {
		t.Fatalf("expected unspecified action to fall back to legacy task.create")
	}
}

func TestAgentCardCRUDAndMatchAgentsForTask(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "agent-test")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"agent-test","title":"Agent Test"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}

	card := AgentCard{
		AgentID:     "agent://pc75/coder",
		Name:        "Code Agent",
		Description: "Writes and reviews code",
		Skills: []AgentSkill{
			{ID: "code-review", Name: "Code Review", Tags: []string{"review", "code"}},
			{ID: "code-write", Name: "Code Writing", Tags: []string{"coding", "implementation"}},
		},
	}
	if err := store.SaveAgentCard("agent-test", card); err != nil {
		t.Fatalf("SaveAgentCard error = %v", err)
	}

	loaded, err := store.LoadAgentCard("agent-test", "agent://pc75/coder")
	if err != nil {
		t.Fatalf("LoadAgentCard error = %v", err)
	}
	if loaded.Name != "Code Agent" {
		t.Fatalf("loaded.Name = %q", loaded.Name)
	}

	cards, err := store.ListAgentCards("agent-test")
	if err != nil {
		t.Fatalf("ListAgentCards error = %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("ListAgentCards = %d, want 1", len(cards))
	}

	matched := MatchAgentsForTask(cards, Task{Labels: []string{"implementation"}})
	if len(matched) != 1 || matched[0].AgentID != "agent://pc75/coder" {
		t.Fatalf("MatchAgentsForTask = %#v", matched)
	}
}

func TestStoreSubscribePublishesMessageEvent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "event-team")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	events, unsubscribe, err := store.Subscribe("event-team")
	if err != nil {
		t.Fatalf("Subscribe error = %v", err)
	}
	defer unsubscribe()

	if err := store.AppendMessage("event-team", Message{
		ChannelID:     "main",
		AuthorAgentID: "agent://pc75/live-alpha",
		MessageType:   "chat",
		Content:       "hello sse",
		CreatedAt:     time.Date(2026, 4, 3, 13, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendMessage error = %v", err)
	}

	select {
	case event := <-events:
		if event.TeamID != "event-team" || event.Kind != "message" || event.Action != "create" {
			t.Fatalf("unexpected event: %#v", event)
		}
		if event.ChannelID != "main" {
			t.Fatalf("event.ChannelID = %q", event.ChannelID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for team event")
	}
}

func TestStoreWebhookReceivesPublishedEvent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "webhook-team")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	received := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	if err := store.SaveWebhookConfigs("webhook-team", []PushNotificationConfig{{
		WebhookID: "hook-1",
		URL:       server.URL,
		Token:     "secret-token",
		Events:    []string{"message.create"},
	}}); err != nil {
		t.Fatalf("SaveWebhookConfigs error = %v", err)
	}
	if err := store.AppendMessage("webhook-team", Message{
		ChannelID:     "main",
		AuthorAgentID: "agent://pc75/live-alpha",
		MessageType:   "chat",
		Content:       "hello webhook",
		CreatedAt:     time.Date(2026, 4, 3, 16, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendMessage error = %v", err)
	}
	select {
	case req := <-received:
		if got := req.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("Authorization = %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for webhook delivery")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		status, err := store.LoadWebhookDeliveryStatusCtx(context.Background(), "webhook-team")
		if err == nil && status.DeliveredCount == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for webhook delivery status: status=%#v err=%v", status, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestStoreWebhookRetriesRetriableStatus(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "team", "webhook-retry"), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	var mu sync.Mutex
	attempts := 0
	done := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		current := attempts
		mu.Unlock()
		if current == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		done <- struct{}{}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	store.webhookClient = &http.Client{Timeout: 2 * time.Second}
	store.sendWebhook(PushNotificationConfig{URL: server.URL}, TeamEvent{
		TeamID: "webhook-retry",
		Kind:   "message",
		Action: "create",
	})
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for webhook retry")
	}
	mu.Lock()
	gotAttempts := attempts
	mu.Unlock()
	if gotAttempts < 2 {
		t.Fatalf("attempts = %d, want at least 2", gotAttempts)
	}
}

func TestStoreWebhookPersistsDeadLetterAndReplay(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "team", "webhook-dead"), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	var mu sync.Mutex
	mode := "fail"
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		currentMode := mode
		mu.Unlock()
		if currentMode == "fail" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	store.sendWebhook(PushNotificationConfig{WebhookID: "hook-dead", URL: server.URL}, TeamEvent{
		TeamID:  "webhook-dead",
		EventID: "evt-dead",
		Kind:    "message",
		Action:  "create",
	})

	status, err := store.LoadWebhookDeliveryStatusCtx(context.Background(), "webhook-dead")
	if err != nil {
		t.Fatalf("LoadWebhookDeliveryStatusCtx error = %v", err)
	}
	if status.DeadLetterCount != 1 {
		t.Fatalf("DeadLetterCount = %d, want 1", status.DeadLetterCount)
	}
	if len(status.RecentDead) != 1 {
		t.Fatalf("RecentDead len = %d, want 1", len(status.RecentDead))
	}

	mu.Lock()
	mode = "ok"
	mu.Unlock()
	replayed, err := store.ReplayWebhookDeliveryCtx(context.Background(), "webhook-dead", status.RecentDead[0].DeliveryID)
	if err != nil {
		t.Fatalf("ReplayWebhookDeliveryCtx error = %v", err)
	}
	if replayed.ReplayedFrom != status.RecentDead[0].DeliveryID {
		t.Fatalf("ReplayedFrom = %q, want %q", replayed.ReplayedFrom, status.RecentDead[0].DeliveryID)
	}
	status, err = store.LoadWebhookDeliveryStatusCtx(context.Background(), "webhook-dead")
	if err != nil {
		t.Fatalf("LoadWebhookDeliveryStatusCtx(after replay) error = %v", err)
	}
	if status.DeliveredCount < 1 {
		t.Fatalf("DeliveredCount = %d, want at least 1", status.DeliveredCount)
	}
	mu.Lock()
	gotAttempts := attempts
	mu.Unlock()
	if gotAttempts < 4 {
		t.Fatalf("attempts = %d, want at least 4", gotAttempts)
	}
}

func TestStoreWebhookPersistsNonRetriableFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "team", "webhook-failed"), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	store.sendWebhook(PushNotificationConfig{WebhookID: "hook-failed", URL: server.URL}, TeamEvent{
		TeamID:  "webhook-failed",
		EventID: "evt-failed",
		Kind:    "message",
		Action:  "create",
	})

	status, err := store.LoadWebhookDeliveryStatusCtx(context.Background(), "webhook-failed")
	if err != nil {
		t.Fatalf("LoadWebhookDeliveryStatusCtx error = %v", err)
	}
	if status.FailedCount != 1 {
		t.Fatalf("FailedCount = %d, want 1", status.FailedCount)
	}
	if status.DeadLetterCount != 0 {
		t.Fatalf("DeadLetterCount = %d, want 0", status.DeadLetterCount)
	}
}

func TestStoreLoadMembersNormalizesRoleAndStatus(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "member-team")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"member-team","title":"Member Team"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "members.json"), []byte(`[
  {"agent_id":"agent://pc75/a","role":"Maintainer","status":"Pending"},
  {"agent_id":"agent://pc75/b","role":"weird","status":"odd"}
]`), 0o644); err != nil {
		t.Fatalf("WriteFile(members.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	members, err := store.LoadMembers("member-team")
	if err != nil {
		t.Fatalf("LoadMembers error = %v", err)
	}
	if members[0].Role != "maintainer" || members[0].Status != "pending" {
		t.Fatalf("unexpected normalized member[0]: %#v", members[0])
	}
	if members[1].Role != "member" || members[1].Status != "active" {
		t.Fatalf("unexpected normalized member[1]: %#v", members[1])
	}
}

func TestNormalizeTeamID(t *testing.T) {
	t.Parallel()

	got := NormalizeTeamID("  Project / Alpha_Test  ")
	if got != "project-alpha-test" {
		t.Fatalf("NormalizeTeamID = %q", got)
	}
	if strings.Contains(got, "/") || strings.Contains(got, "_") {
		t.Fatalf("expected normalized team id, got %q", got)
	}
}

func TestNormalizeTeamIDAndSanitizeArchiveIDRejectTraversal(t *testing.T) {
	t.Parallel()

	if got := NormalizeTeamID("../Team%2FAlpha"); got != "team-alpha" {
		t.Fatalf("NormalizeTeamID traversal sanitize = %q", got)
	}
	if got := NormalizeTeamID("/../../"); got != "" {
		t.Fatalf("NormalizeTeamID absolute traversal = %q, want empty", got)
	}
	if got := sanitizeArchiveID("../archive%2F2026-04-04"); got != "archive-2026-04-04" {
		t.Fatalf("sanitizeArchiveID traversal sanitize = %q", got)
	}
}

func TestReadLastJSONLLinesReturnsNewestNonEmptyLines(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "messages.jsonl")
	var builder strings.Builder
	for i := 0; i < 300; i++ {
		if i%57 == 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(fmt.Sprintf("{\"message_id\":\"msg-%03d\"}\n", i))
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	lines, err := readLastJSONLLines(path, 3)
	if err != nil {
		t.Fatalf("readLastJSONLLines error = %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("len(lines) = %d, want 3", len(lines))
	}
	if !strings.Contains(lines[0], "msg-299") || !strings.Contains(lines[1], "msg-298") || !strings.Contains(lines[2], "msg-297") {
		t.Fatalf("unexpected newest lines: %#v", lines)
	}
}

func TestStoreChannelSummaryCountsMessagesAndLatestTimestamp(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "channel-stats")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"channel-stats","title":"Channel Stats","channels":["main"]}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	t1 := time.Date(2026, 4, 4, 8, 0, 0, 0, time.UTC)
	t2 := t1.Add(2 * time.Minute)
	if err := store.AppendMessage("channel-stats", Message{ChannelID: "main", AuthorAgentID: "agent://pc75/a", Content: "one", CreatedAt: t1}); err != nil {
		t.Fatalf("AppendMessage(one) error = %v", err)
	}
	if err := store.AppendMessage("channel-stats", Message{ChannelID: "main", AuthorAgentID: "agent://pc75/b", Content: "two", CreatedAt: t2}); err != nil {
		t.Fatalf("AppendMessage(two) error = %v", err)
	}
	summary, err := store.channelSummary("channel-stats", "main")
	if err != nil {
		t.Fatalf("channelSummary error = %v", err)
	}
	if summary.MessageCount != 2 {
		t.Fatalf("summary.MessageCount = %d, want 2", summary.MessageCount)
	}
	if !summary.LastMessageAt.Equal(t2) {
		t.Fatalf("summary.LastMessageAt = %v, want %v", summary.LastMessageAt, t2)
	}
}

func TestWithTeamLockTimeoutReturnsDeadlineExceededStyleError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "lock-timeout")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	lockFile, err := os.OpenFile(filepath.Join(teamRoot, ".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(lock) error = %v", err)
	}
	defer lockFile.Close()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("Flock lock holder error = %v", err)
	}
	defer func() { _ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN) }()

	start := time.Now()
	err = store.withTeamLockTimeout("lock-timeout", 150*time.Millisecond, func() error { return nil })
	if err == nil || !strings.Contains(err.Error(), "team lock timeout") {
		t.Fatalf("withTeamLockTimeout error = %v, want timeout", err)
	}
	if time.Since(start) < 150*time.Millisecond {
		t.Fatalf("withTeamLockTimeout returned too early: %v", time.Since(start))
	}
}

func TestWithTeamLockCtxCancelsWhileWaiting(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "lock-ctx-timeout")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	lockFile, err := os.OpenFile(filepath.Join(teamRoot, ".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(lock) error = %v", err)
	}
	defer lockFile.Close()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("Flock lock holder error = %v", err)
	}
	defer func() { _ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN) }()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = store.withTeamLockCtx(ctx, "lock-ctx-timeout", 5*time.Second, func() error { return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("withTeamLockCtx error = %v, want context deadline exceeded", err)
	}
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond || elapsed > 700*time.Millisecond {
		t.Fatalf("withTeamLockCtx elapsed = %v, want around context timeout", elapsed)
	}
}

func TestMergeChannelPreservesEarliestCreatedAt(t *testing.T) {
	t.Parallel()

	baseCreated := time.Date(2026, 4, 4, 8, 0, 0, 0, time.UTC)
	overrideCreated := baseCreated.Add(10 * time.Minute)
	merged := mergeChannel(
		Channel{ChannelID: "main", Title: "Main", CreatedAt: baseCreated},
		Channel{ChannelID: "main", Title: "Renamed", CreatedAt: overrideCreated, UpdatedAt: overrideCreated},
	)
	if !merged.CreatedAt.Equal(baseCreated) {
		t.Fatalf("merged.CreatedAt = %v, want %v", merged.CreatedAt, baseCreated)
	}
	if merged.Title != "Renamed" {
		t.Fatalf("merged.Title = %q, want Renamed", merged.Title)
	}
	if !merged.UpdatedAt.Equal(overrideCreated) {
		t.Fatalf("merged.UpdatedAt = %v, want %v", merged.UpdatedAt, overrideCreated)
	}
}

func TestStoreAppendAndLoadMessages(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-gamma")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-gamma","title":"Project Gamma"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	if err := store.AppendMessage("project-gamma", Message{
		ChannelID:     "main",
		AuthorAgentID: "agent://pc75/live-alpha",
		MessageType:   "chat",
		Content:       "first team message",
		CreatedAt:     time.Date(2026, 4, 1, 1, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendMessage(first) error = %v", err)
	}
	if err := store.AppendMessage("project-gamma", Message{
		ChannelID:     "main",
		AuthorAgentID: "agent://pc75/live-bravo",
		MessageType:   "decision",
		Content:       "second team message",
		CreatedAt:     time.Date(2026, 4, 1, 2, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendMessage(second) error = %v", err)
	}

	messages, err := store.LoadMessages("project-gamma", "main", 10)
	if err != nil {
		t.Fatalf("LoadMessages error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].Content != "second team message" || messages[1].Content != "first team message" {
		t.Fatalf("unexpected message order: %#v", messages)
	}
}

func TestStoreLoadMessagesLimitReadsLatestMessages(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-limit")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-limit","title":"Project Limit"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	for i := 1; i <= 5; i++ {
		if err := store.AppendMessage("project-limit", Message{
			ChannelID:     "main",
			AuthorAgentID: "agent://pc75/live-alpha",
			MessageType:   "chat",
			Content:       fmt.Sprintf("message-%d", i),
			CreatedAt:     time.Date(2026, 4, 1, i, 0, 0, 0, time.UTC),
		}); err != nil {
			t.Fatalf("AppendMessage(%d) error = %v", i, err)
		}
	}

	messages, err := store.LoadMessages("project-limit", "main", 2)
	if err != nil {
		t.Fatalf("LoadMessages error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].Content != "message-5" || messages[1].Content != "message-4" {
		t.Fatalf("unexpected limited message order: %#v", messages)
	}
}

func TestStoreMigrateChannelToShardsKeepsMessagesReadable(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-shards")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-shards","title":"Project Shards"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	firstAt := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	secondAt := time.Date(2026, 4, 8, 10, 0, 0, 0, time.UTC)
	if err := store.AppendMessage("project-shards", Message{
		ChannelID:     "research",
		AuthorAgentID: "agent://pc75/live-alpha",
		Content:       "week one",
		CreatedAt:     firstAt,
	}); err != nil {
		t.Fatalf("AppendMessage(first) error = %v", err)
	}
	if err := store.AppendMessage("project-shards", Message{
		ChannelID:     "research",
		AuthorAgentID: "agent://pc75/live-bravo",
		Content:       "week two",
		CreatedAt:     secondAt,
	}); err != nil {
		t.Fatalf("AppendMessage(second) error = %v", err)
	}

	before, err := store.LoadMessages("project-shards", "research", 0)
	if err != nil {
		t.Fatalf("LoadMessages(before migrate) error = %v", err)
	}
	if len(before) != 2 || before[0].Content != "week two" || before[1].Content != "week one" {
		t.Fatalf("unexpected messages before migrate: %#v", before)
	}
	if err := store.MigrateChannelToShards("project-shards", "research"); err != nil {
		t.Fatalf("MigrateChannelToShards error = %v", err)
	}

	if _, err := os.Stat(store.channelLegacyBackupPath("project-shards", "research")); err != nil {
		t.Fatalf("expected legacy backup to exist, got %v", err)
	}
	if !store.isShardedChannel("project-shards", "research") {
		t.Fatalf("expected research channel to be sharded after migration")
	}

	after, err := store.LoadMessages("project-shards", "research", 0)
	if err != nil {
		t.Fatalf("LoadMessages(after migrate) error = %v", err)
	}
	if len(after) != 2 || after[0].Content != "week two" || after[1].Content != "week one" {
		t.Fatalf("unexpected messages after migrate: %#v", after)
	}
}

func TestStoreMigrateChannelToShardsAppendsNewMessagesToLatestShard(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-shard-append")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-shard-append","title":"Project Shard Append"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	legacyAt := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	newAt := time.Date(2026, 4, 15, 9, 0, 0, 0, time.UTC)
	if err := store.AppendMessage("project-shard-append", Message{
		ChannelID:     "planning",
		AuthorAgentID: "agent://pc75/live-alpha",
		Content:       "legacy planning note",
		CreatedAt:     legacyAt,
	}); err != nil {
		t.Fatalf("AppendMessage(legacy) error = %v", err)
	}
	if err := store.MigrateChannelToShards("project-shard-append", "planning"); err != nil {
		t.Fatalf("MigrateChannelToShards error = %v", err)
	}
	if err := store.AppendMessage("project-shard-append", Message{
		ChannelID:     "planning",
		AuthorAgentID: "agent://pc75/live-bravo",
		Content:       "new planning note",
		CreatedAt:     newAt,
	}); err != nil {
		t.Fatalf("AppendMessage(new) error = %v", err)
	}

	if _, err := os.Stat(store.channelPath("project-shard-append", "planning")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected legacy channel file to stay absent after shard append, got %v", err)
	}
	if _, err := os.Stat(store.channelShardPath("project-shard-append", "planning", newAt)); err != nil {
		t.Fatalf("expected latest shard file to exist, got %v", err)
	}

	messages, err := store.LoadMessages("project-shard-append", "planning", 10)
	if err != nil {
		t.Fatalf("LoadMessages error = %v", err)
	}
	if len(messages) != 2 || messages[0].Content != "new planning note" || messages[1].Content != "legacy planning note" {
		t.Fatalf("unexpected sharded messages: %#v", messages)
	}
}

func TestStoreCreateManualArchive(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "archive-team")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"archive-team","title":"Archive Team"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	if err := store.AppendMessage("archive-team", Message{
		ChannelID:     "main",
		AuthorAgentID: "agent://pc75/tester",
		MessageType:   "chat",
		Content:       "archived message",
		CreatedAt:     time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendMessage error = %v", err)
	}
	if err := store.AppendTask("archive-team", Task{
		TaskID:    "task-a",
		Title:     "Archive Task",
		Status:    "doing",
		CreatedAt: time.Date(2026, 4, 2, 10, 5, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 2, 10, 5, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendTask error = %v", err)
	}
	record, err := store.CreateManualArchive("archive-team", time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("CreateManualArchive error = %v", err)
	}
	if record.Kind != "manual" || record.MessageCount != 1 || record.TaskCount != 1 {
		t.Fatalf("unexpected archive record: %#v", record)
	}
	archives, err := store.ListArchives("archive-team")
	if err != nil {
		t.Fatalf("ListArchives error = %v", err)
	}
	if len(archives) != 1 {
		t.Fatalf("expected 1 archive, got %d", len(archives))
	}
	loaded, err := store.LoadArchive("archive-team", record.ArchiveID)
	if err != nil {
		t.Fatalf("LoadArchive error = %v", err)
	}
	if loaded.ArchiveID != record.ArchiveID || loaded.MessageCount != 1 {
		t.Fatalf("unexpected loaded archive: %#v", loaded)
	}
}

func TestStoreListChannelsIncludesConfiguredAndDiskChannels(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-epsilon")
	if err := os.MkdirAll(filepath.Join(teamRoot, "channels"), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{
  "team_id": "project-epsilon",
  "title": "Project Epsilon",
  "channels": ["main", "research"]
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	if err := store.AppendMessage("project-epsilon", Message{
		ChannelID:     "research",
		AuthorAgentID: "agent://pc75/live-alpha",
		Content:       "research note",
		CreatedAt:     time.Date(2026, 4, 1, 3, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendMessage(research) error = %v", err)
	}
	if err := store.AppendMessage("project-epsilon", Message{
		ChannelID:     "ops",
		AuthorAgentID: "agent://pc75/live-bravo",
		Content:       "ops note",
		CreatedAt:     time.Date(2026, 4, 1, 4, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendMessage(ops) error = %v", err)
	}

	channels, err := store.ListChannels("project-epsilon")
	if err != nil {
		t.Fatalf("ListChannels error = %v", err)
	}
	if len(channels) != 3 {
		t.Fatalf("expected 3 channels, got %d: %#v", len(channels), channels)
	}
	if channels[0].ChannelID != "ops" || channels[0].MessageCount != 1 {
		t.Fatalf("expected ops first, got %#v", channels[0])
	}
	if channels[1].ChannelID != "research" || channels[1].MessageCount != 1 {
		t.Fatalf("expected research second, got %#v", channels[1])
	}
	if channels[2].ChannelID != "main" || channels[2].MessageCount != 0 {
		t.Fatalf("expected main last, got %#v", channels[2])
	}
}

func TestStoreLoadChannelMessagesAlias(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-zeta")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-zeta","title":"Project Zeta","channels":["main","design"]}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	if err := store.AppendMessage("project-zeta", Message{
		ChannelID:     "design",
		AuthorAgentID: "agent://pc75/live-charlie",
		Content:       "design one",
		CreatedAt:     time.Date(2026, 4, 1, 5, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendMessage(first) error = %v", err)
	}
	if err := store.AppendMessage("project-zeta", Message{
		ChannelID:     "design",
		AuthorAgentID: "agent://pc75/live-delta",
		Content:       "design two",
		CreatedAt:     time.Date(2026, 4, 1, 6, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendMessage(second) error = %v", err)
	}

	messages, err := store.LoadChannelMessages("project-zeta", "design", 1)
	if err != nil {
		t.Fatalf("LoadChannelMessages error = %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].Content != "design two" || messages[0].ChannelID != "design" {
		t.Fatalf("unexpected channel message: %#v", messages[0])
	}
}

func TestStoreSaveLoadAndHideChannel(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-channel-write")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{
  "team_id":"project-channel-write",
  "title":"Project Channel Write",
  "channels":["main","research"]
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}

	if err := store.SaveChannel("project-channel-write", Channel{
		ChannelID:   "planning",
		Title:       "Planning Board",
		Description: "Long-running team planning",
		CreatedAt:   time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveChannel(create) error = %v", err)
	}

	channel, err := store.LoadChannel("project-channel-write", "planning")
	if err != nil {
		t.Fatalf("LoadChannel error = %v", err)
	}
	if channel.Title != "Planning Board" || channel.Description != "Long-running team planning" || channel.Hidden {
		t.Fatalf("unexpected saved channel: %#v", channel)
	}

	if err := store.SaveChannel("project-channel-write", Channel{
		ChannelID:   "planning",
		Title:       "Planning Updated",
		Description: "",
		CreatedAt:   channel.CreatedAt,
		UpdatedAt:   time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveChannel(update) error = %v", err)
	}

	channel, err = store.LoadChannel("project-channel-write", "planning")
	if err != nil {
		t.Fatalf("LoadChannel(updated) error = %v", err)
	}
	if channel.Title != "Planning Updated" || channel.Description != "" {
		t.Fatalf("unexpected updated channel: %#v", channel)
	}

	if err := store.HideChannel("project-channel-write", "planning"); err != nil {
		t.Fatalf("HideChannel error = %v", err)
	}

	channel, err = store.LoadChannel("project-channel-write", "planning")
	if err != nil {
		t.Fatalf("LoadChannel(hidden) error = %v", err)
	}
	if !channel.Hidden {
		t.Fatalf("expected hidden channel, got %#v", channel)
	}

	channels, err := store.ListChannels("project-channel-write")
	if err != nil {
		t.Fatalf("ListChannels error = %v", err)
	}
	found := false
	for _, summary := range channels {
		if summary.ChannelID == "planning" {
			found = true
			if !summary.Hidden || summary.Title != "Planning Updated" {
				t.Fatalf("unexpected channel summary: %#v", summary)
			}
		}
	}
	if !found {
		t.Fatalf("expected planning channel in summaries, got %#v", channels)
	}
}

func TestStoreAppendAndLoadTasks(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-delta")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-delta","title":"Project Delta"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	if err := store.AppendTask("project-delta", Task{
		CreatedBy: "agent://pc75/openclaw01",
		Title:     "Prepare task model",
		Status:    "doing",
		UpdatedAt: time.Date(2026, 4, 1, 5, 0, 0, 0, time.UTC),
		CreatedAt: time.Date(2026, 4, 1, 4, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendTask(first) error = %v", err)
	}
	if err := store.AppendTask("project-delta", Task{
		CreatedBy: "agent://pc75/live-alpha",
		Title:     "Review team message design",
		Status:    "open",
		UpdatedAt: time.Date(2026, 4, 1, 6, 0, 0, 0, time.UTC),
		CreatedAt: time.Date(2026, 4, 1, 5, 30, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendTask(second) error = %v", err)
	}

	tasks, err := store.LoadTasks("project-delta", 10)
	if err != nil {
		t.Fatalf("LoadTasks error = %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].Title != "Review team message design" || tasks[1].Title != "Prepare task model" {
		t.Fatalf("unexpected task order: %#v", tasks)
	}
}

func TestStoreLoadTasksLimitReadsLatestTasks(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-task-limit")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-task-limit","title":"Project Task Limit"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	for i := 1; i <= 5; i++ {
		if err := store.AppendTask("project-task-limit", Task{
			Title:     fmt.Sprintf("task-%d", i),
			Status:    "open",
			CreatedAt: time.Date(2026, 4, 1, i, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 4, 1, i, 0, 0, 0, time.UTC),
		}); err != nil {
			t.Fatalf("AppendTask(%d) error = %v", i, err)
		}
	}
	tasks, err := store.LoadTasks("project-task-limit", 2)
	if err != nil {
		t.Fatalf("LoadTasks error = %v", err)
	}
	if len(tasks) != 2 || tasks[0].Title != "task-5" || tasks[1].Title != "task-4" {
		t.Fatalf("unexpected limited tasks: %#v", tasks)
	}
}

func TestStoreSaveAndDeleteTask(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-task-write")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-task-write","title":"Project Task Write"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	createdAt := time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)
	if err := store.AppendTask("project-task-write", Task{
		TaskID:      "task-write-1",
		CreatedBy:   "agent://pc75/live-alpha",
		Title:       "Original task",
		ChannelID:   " research ",
		Status:      "open",
		Priority:    "low",
		Assignees:   []string{"agent://pc75/live-bravo"},
		Description: "before update",
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	}); err != nil {
		t.Fatalf("AppendTask error = %v", err)
	}

	if err := store.SaveTask("project-task-write", Task{
		TaskID:      "task-write-1",
		Title:       "Updated task",
		ChannelID:   " planning ",
		Status:      "doing",
		Priority:    "high",
		Assignees:   []string{"agent://pc75/live-charlie", "agent://pc75/live-charlie"},
		Labels:      []string{"urgent", "", "urgent", "team"},
		Description: "after update",
		CreatedBy:   "agent://pc75/live-alpha",
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt.Add(30 * time.Minute),
	}); err != nil {
		t.Fatalf("SaveTask error = %v", err)
	}

	task, err := store.LoadTask("project-task-write", "task-write-1")
	if err != nil {
		t.Fatalf("LoadTask error = %v", err)
	}
	if task.Title != "Updated task" || task.Status != "doing" || task.Priority != "high" {
		t.Fatalf("unexpected saved task core fields: %#v", task)
	}
	if task.ChannelID != "planning" {
		t.Fatalf("unexpected saved channel: %#v", task.ChannelID)
	}
	if strings.Join(task.Assignees, ",") != "agent://pc75/live-charlie" {
		t.Fatalf("unexpected saved assignees: %#v", task.Assignees)
	}
	if strings.Join(task.Labels, ",") != "urgent,team" {
		t.Fatalf("unexpected saved labels: %#v", task.Labels)
	}

	if err := store.DeleteTask("project-task-write", "task-write-1"); err != nil {
		t.Fatalf("DeleteTask error = %v", err)
	}
	if _, err := store.LoadTask("project-task-write", "task-write-1"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected deleted task to be missing, got %v", err)
	}
}

func TestStoreMigrateTasksToIndexKeepsTasksReadable(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-task-index")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	if err := store.AppendTask("project-task-index", Task{
		TaskID:    "task-index-1",
		CreatedBy: "agent://pc75/live-alpha",
		Title:     "Indexed Task One",
		Status:    "open",
		Priority:  "normal",
		CreatedAt: time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendTask(first) error = %v", err)
	}
	if err := store.AppendTask("project-task-index", Task{
		TaskID:    "task-index-2",
		CreatedBy: "agent://pc75/live-bravo",
		Title:     "Indexed Task Two",
		Status:    "doing",
		Priority:  "high",
		CreatedAt: time.Date(2026, 4, 3, 12, 10, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 3, 12, 10, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendTask(second) error = %v", err)
	}
	if err := store.MigrateTasksToIndex("project-task-index"); err != nil {
		t.Fatalf("MigrateTasksToIndex error = %v", err)
	}
	if !store.hasTaskIndex("project-task-index") {
		t.Fatal("expected task index to exist after migration")
	}

	tasks, err := store.LoadTasks("project-task-index", 2)
	if err != nil {
		t.Fatalf("LoadTasks(indexed) error = %v", err)
	}
	if len(tasks) != 2 || tasks[0].TaskID != "task-index-2" {
		t.Fatalf("unexpected indexed tasks: %#v", tasks)
	}

	task, err := store.LoadTask("project-task-index", "task-index-1")
	if err != nil {
		t.Fatalf("LoadTask(indexed) error = %v", err)
	}
	if task.Title != "Indexed Task One" {
		t.Fatalf("task.Title = %q", task.Title)
	}
}

func TestStoreIndexedTaskCRUDAndCompact(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-task-compact")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	if err := store.MigrateTasksToIndex("project-task-compact"); err != nil {
		t.Fatalf("MigrateTasksToIndex(empty) error = %v", err)
	}
	if err := store.AppendTask("project-task-compact", Task{
		TaskID:    "task-compact-1",
		CreatedBy: "agent://pc75/live-alpha",
		Title:     "Compact Task",
		Status:    "open",
		CreatedAt: time.Date(2026, 4, 3, 13, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 3, 13, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendTask error = %v", err)
	}
	if err := store.SaveTask("project-task-compact", Task{
		TaskID:    "task-compact-1",
		CreatedBy: "agent://pc75/live-alpha",
		Title:     "Compact Task Updated",
		Status:    "doing",
		Priority:  "high",
		CreatedAt: time.Date(2026, 4, 3, 13, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 3, 13, 5, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveTask(indexed) error = %v", err)
	}
	task, err := store.LoadTask("project-task-compact", "task-compact-1")
	if err != nil {
		t.Fatalf("LoadTask error = %v", err)
	}
	if task.Status != "doing" || task.Title != "Compact Task Updated" {
		t.Fatalf("unexpected indexed task after update: %#v", task)
	}
	before, err := os.Stat(store.taskDataPath("project-task-compact"))
	if err != nil {
		t.Fatalf("Stat(before compact) error = %v", err)
	}
	if err := store.CompactTasks("project-task-compact"); err != nil {
		t.Fatalf("CompactTasks error = %v", err)
	}
	after, err := os.Stat(store.taskDataPath("project-task-compact"))
	if err != nil {
		t.Fatalf("Stat(after compact) error = %v", err)
	}
	if after.Size() >= before.Size() {
		t.Fatalf("expected compacted task data to shrink: before=%d after=%d", before.Size(), after.Size())
	}
	if err := store.DeleteTask("project-task-compact", "task-compact-1"); err != nil {
		t.Fatalf("DeleteTask(indexed) error = %v", err)
	}
	if _, err := store.LoadTask("project-task-compact", "task-compact-1"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("LoadTask after delete error = %v, want not exist", err)
	}
}

func TestStoreTaskStateMachineAllowsValidTransitions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-task-state")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-task-state","title":"Project Task State"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	createdAt := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	if err := store.AppendTask("project-task-state", Task{
		TaskID:    "task-state-1",
		Title:     "Track valid transitions",
		Status:    "open",
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}); err != nil {
		t.Fatalf("AppendTask error = %v", err)
	}
	if err := store.SaveTask("project-task-state", Task{
		TaskID:    "task-state-1",
		Title:     "Track valid transitions",
		Status:    "doing",
		CreatedAt: createdAt,
		UpdatedAt: createdAt.Add(30 * time.Minute),
	}); err != nil {
		t.Fatalf("SaveTask(doing) error = %v", err)
	}
	if err := store.SaveTask("project-task-state", Task{
		TaskID:    "task-state-1",
		Title:     "Track valid transitions",
		Status:    "done",
		CreatedAt: createdAt,
		UpdatedAt: createdAt.Add(60 * time.Minute),
	}); err != nil {
		t.Fatalf("SaveTask(done) error = %v", err)
	}
	task, err := store.LoadTask("project-task-state", "task-state-1")
	if err != nil {
		t.Fatalf("LoadTask error = %v", err)
	}
	if task.Status != TaskStateDone {
		t.Fatalf("expected done task, got %#v", task)
	}
	if task.ClosedAt.IsZero() {
		t.Fatalf("expected terminal task to have closed_at, got %#v", task)
	}
}

func TestStoreTaskStateMachineRejectsInvalidTransition(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-task-invalid")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-task-invalid","title":"Project Task Invalid"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	createdAt := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	if err := store.AppendTask("project-task-invalid", Task{
		TaskID:    "task-invalid-1",
		Title:     "Reject reopen",
		Status:    "done",
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}); err != nil {
		t.Fatalf("AppendTask error = %v", err)
	}
	err = store.SaveTask("project-task-invalid", Task{
		TaskID:    "task-invalid-1",
		Title:     "Reject reopen",
		Status:    "doing",
		CreatedAt: createdAt,
		UpdatedAt: createdAt.Add(15 * time.Minute),
	})
	if err == nil || !strings.Contains(err.Error(), "invalid task status transition") {
		t.Fatalf("expected invalid transition error, got %v", err)
	}
}

func TestStoreTaskStateMachineUsesPolicyOverrides(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-task-policy")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-task-policy","title":"Project Task Policy"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	if err := store.SavePolicy("project-task-policy", Policy{
		TaskTransitions: map[string]TaskTransitionRule{
			TaskStateOpen: {Allowed: []string{TaskStateReview}},
		},
	}); err != nil {
		t.Fatalf("SavePolicy error = %v", err)
	}
	createdAt := time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC)
	if err := store.AppendTask("project-task-policy", Task{
		TaskID:    "task-policy-1",
		Title:     "Policy controlled task",
		Status:    "open",
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}); err != nil {
		t.Fatalf("AppendTask error = %v", err)
	}
	err = store.SaveTask("project-task-policy", Task{
		TaskID:    "task-policy-1",
		Title:     "Policy controlled task",
		Status:    "doing",
		CreatedAt: createdAt,
		UpdatedAt: createdAt.Add(15 * time.Minute),
	})
	if err == nil || !strings.Contains(err.Error(), "invalid task status transition") {
		t.Fatalf("expected policy transition error, got %v", err)
	}
	if err := store.SaveTask("project-task-policy", Task{
		TaskID:    "task-policy-1",
		Title:     "Policy controlled task",
		Status:    "review",
		CreatedAt: createdAt,
		UpdatedAt: createdAt.Add(30 * time.Minute),
	}); err != nil {
		t.Fatalf("SaveTask(review) error = %v", err)
	}
}

func TestStoreContextIDAutoGeneratedAndQueryable(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-context")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-context","title":"Project Context"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	createdAt := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	if err := store.AppendTask("project-context", Task{
		TaskID:    "task-context-1",
		Title:     "Link context",
		ChannelID: "research",
		Status:    "open",
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}); err != nil {
		t.Fatalf("AppendTask error = %v", err)
	}
	task, err := store.LoadTask("project-context", "task-context-1")
	if err != nil {
		t.Fatalf("LoadTask error = %v", err)
	}
	if strings.TrimSpace(task.ContextID) == "" {
		t.Fatalf("expected task context id, got %#v", task)
	}
	if err := store.AppendMessage("project-context", Message{
		ChannelID:     "research",
		ContextID:     task.ContextID,
		AuthorAgentID: "agent://pc75/live-alpha",
		Content:       "context research note",
		CreatedAt:     createdAt.Add(10 * time.Minute),
	}); err != nil {
		t.Fatalf("AppendMessage(context field) error = %v", err)
	}
	if err := store.AppendMessage("project-context", Message{
		ChannelID:     "main",
		AuthorAgentID: "agent://pc75/live-bravo",
		Content:       "context from structured data",
		StructuredData: map[string]any{
			"context_id": task.ContextID,
		},
		CreatedAt: createdAt.Add(20 * time.Minute),
	}); err != nil {
		t.Fatalf("AppendMessage(context structured data) error = %v", err)
	}

	tasks, err := store.LoadTasksByContext("project-context", task.ContextID)
	if err != nil {
		t.Fatalf("LoadTasksByContext error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].TaskID != "task-context-1" {
		t.Fatalf("unexpected context tasks: %#v", tasks)
	}
	messages, err := store.LoadMessagesByContext("project-context", task.ContextID, 10)
	if err != nil {
		t.Fatalf("LoadMessagesByContext error = %v", err)
	}
	if len(messages) != 2 || messages[0].Content != "context from structured data" || messages[1].Content != "context research note" {
		t.Fatalf("unexpected context messages: %#v", messages)
	}
}

func TestStoreLoadMessagesByContextFallsBackBeyondPreferredChannels(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-context-fallback")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-context-fallback","title":"Project Context Fallback","channels":["main","research","ops"]}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	base := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	if err := store.AppendTask("project-context-fallback", Task{
		TaskID:    "task-context-fallback",
		Title:     "Cross-channel context",
		ChannelID: "research",
		Status:    "open",
		CreatedAt: base,
		UpdatedAt: base,
	}); err != nil {
		t.Fatalf("AppendTask error = %v", err)
	}
	task, err := store.LoadTask("project-context-fallback", "task-context-fallback")
	if err != nil {
		t.Fatalf("LoadTask error = %v", err)
	}
	if err := store.AppendMessage("project-context-fallback", Message{
		ChannelID:     "ops",
		AuthorAgentID: "agent://pc75/live-charlie",
		Content:       "fallback context note",
		ContextID:     task.ContextID,
		CreatedAt:     base.Add(30 * time.Minute),
	}); err != nil {
		t.Fatalf("AppendMessage(fallback context) error = %v", err)
	}

	messages, err := store.LoadMessagesByContext("project-context-fallback", task.ContextID, 10)
	if err != nil {
		t.Fatalf("LoadMessagesByContext error = %v", err)
	}
	if len(messages) != 1 || messages[0].Content != "fallback context note" {
		t.Fatalf("unexpected context fallback messages: %#v", messages)
	}
}

func TestStoreAppendMessageRequireSignature(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-signature")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-signature","title":"Project Signature"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	if err := store.SavePolicy("project-signature", Policy{RequireSignature: true}); err != nil {
		t.Fatalf("SavePolicy error = %v", err)
	}
	msg := Message{
		ChannelID:       "main",
		AuthorAgentID:   "agent://pc75/live-alpha",
		OriginPublicKey: strings.Repeat("a", 64),
		Content:         "unsigned message",
		CreatedAt:       time.Date(2026, 4, 1, 13, 0, 0, 0, time.UTC),
	}
	err = store.AppendMessage("project-signature", msg)
	if err == nil || !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("expected signature verification error, got %v", err)
	}
}

func TestStoreAppendMessageAcceptsValidSignatureAndRejectsTamper(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-signed-message")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-signed-message","title":"Project Signed Message"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	if err := store.SavePolicy("project-signed-message", Policy{RequireSignature: true}); err != nil {
		t.Fatalf("SavePolicy error = %v", err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey error = %v", err)
	}
	msg := Message{
		TeamID:          "project-signed-message",
		ChannelID:       "research",
		ContextID:       "ctx-signed",
		AuthorAgentID:   "agent://pc75/live-alpha",
		OriginPublicKey: hex.EncodeToString(publicKey),
		ParentPublicKey: strings.Repeat("b", 64),
		MessageType:     "decision",
		Content:         "signed message content",
		StructuredData: map[string]any{
			"task_id": "task-1",
		},
		Parts:      []MessagePart{{Kind: "text", Text: "signed body"}},
		References: []Reference{{RefType: "task", TargetID: "task-1"}},
		CreatedAt:  time.Date(2026, 4, 1, 14, 0, 0, 0, time.UTC),
	}
	payload, err := messageSignaturePayload(msg)
	if err != nil {
		t.Fatalf("messageSignaturePayload error = %v", err)
	}
	msg.Signature = hex.EncodeToString(ed25519.Sign(privateKey, payload))
	if err := store.AppendMessage("project-signed-message", msg); err != nil {
		t.Fatalf("AppendMessage(valid signature) error = %v", err)
	}
	loaded, err := store.LoadMessages("project-signed-message", "research", 10)
	if err != nil {
		t.Fatalf("LoadMessages error = %v", err)
	}
	if len(loaded) != 1 || loaded[0].Content != "signed message content" {
		t.Fatalf("unexpected signed messages: %#v", loaded)
	}

	msg.Content = "tampered content"
	err = store.AppendMessage("project-signed-message", msg)
	if err == nil || !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("expected tamper signature error, got %v", err)
	}
}

func TestStoreListChannels(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-epsilon")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{
  "team_id":"project-epsilon",
  "title":"Project Epsilon",
  "channels":["main","research"]
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	if err := store.AppendMessage("project-epsilon", Message{
		ChannelID:     "research",
		AuthorAgentID: "agent://pc75/live-alpha",
		Content:       "research update",
		CreatedAt:     time.Date(2026, 4, 1, 7, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendMessage(research) error = %v", err)
	}
	if err := store.AppendMessage("project-epsilon", Message{
		ChannelID:     "ops",
		AuthorAgentID: "agent://pc75/live-bravo",
		Content:       "ops update",
		CreatedAt:     time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendMessage(ops) error = %v", err)
	}
	channels, err := store.ListChannels("project-epsilon")
	if err != nil {
		t.Fatalf("ListChannels error = %v", err)
	}
	if len(channels) != 3 {
		t.Fatalf("expected 3 channels, got %d", len(channels))
	}
	if channels[0].ChannelID != "ops" || channels[0].MessageCount != 1 {
		t.Fatalf("unexpected first channel summary: %#v", channels[0])
	}
}

func TestStoreLoadTaskAndTaskMessagesAcrossChannels(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-theta")
	if err := os.MkdirAll(filepath.Join(teamRoot, "channels"), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{
  "team_id": "project-theta",
  "title": "Project Theta",
  "channels": ["main"]
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	task := Task{
		TaskID:      "theta-task-1",
		CreatedBy:   "agent://pc75/live-alpha",
		Title:       "Trace task messages",
		Status:      "doing",
		CreatedAt:   time.Date(2026, 4, 1, 6, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 4, 1, 6, 10, 0, 0, time.UTC),
		Assignees:   []string{"agent://pc75/live-bravo"},
		Description: "Messages can live in any Team channel.",
	}
	if err := store.AppendTask("project-theta", task); err != nil {
		t.Fatalf("AppendTask error = %v", err)
	}
	if err := store.AppendMessage("project-theta", Message{
		ChannelID:     "ops",
		AuthorAgentID: "agent://pc75/live-alpha",
		MessageType:   "comment",
		Content:       "ops comment for theta task",
		StructuredData: map[string]any{
			"task_id": task.TaskID,
		},
		CreatedAt: time.Date(2026, 4, 1, 6, 20, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendMessage(ops) error = %v", err)
	}
	if err := store.AppendMessage("project-theta", Message{
		ChannelID:     "main",
		AuthorAgentID: "agent://pc75/live-bravo",
		MessageType:   "comment",
		Content:       "main comment for theta task",
		StructuredData: map[string]any{
			"team_task_id": task.TaskID,
		},
		CreatedAt: time.Date(2026, 4, 1, 6, 21, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendMessage(main) error = %v", err)
	}

	loadedTask, err := store.LoadTask("project-theta", task.TaskID)
	if err != nil {
		t.Fatalf("LoadTask error = %v", err)
	}
	if loadedTask.Title != task.Title {
		t.Fatalf("unexpected loaded task: %#v", loadedTask)
	}

	messages, err := store.LoadTaskMessages("project-theta", task.TaskID, 10)
	if err != nil {
		t.Fatalf("LoadTaskMessages error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 task messages, got %d: %#v", len(messages), messages)
	}
	if messages[0].Content != "main comment for theta task" || messages[1].Content != "ops comment for theta task" {
		t.Fatalf("unexpected task message order: %#v", messages)
	}
}

func TestStoreLoadTaskMessagesFallsBackBeyondPreferredChannels(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-task-fallback")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-task-fallback","title":"Project Task Fallback","channels":["main","research","ops"]}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	task := Task{
		TaskID:    "task-fallback-1",
		Title:     "Fallback task messages",
		ChannelID: "research",
		Status:    "doing",
		CreatedAt: time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 1, 8, 5, 0, 0, time.UTC),
	}
	if err := store.AppendTask("project-task-fallback", task); err != nil {
		t.Fatalf("AppendTask error = %v", err)
	}
	if err := store.AppendMessage("project-task-fallback", Message{
		ChannelID:     "ops",
		AuthorAgentID: "agent://pc75/live-delta",
		MessageType:   "comment",
		Content:       "ops fallback task comment",
		StructuredData: map[string]any{
			"task_id": task.TaskID,
		},
		CreatedAt: time.Date(2026, 4, 1, 8, 10, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendMessage(ops) error = %v", err)
	}

	messages, err := store.LoadTaskMessages("project-task-fallback", task.TaskID, 10)
	if err != nil {
		t.Fatalf("LoadTaskMessages error = %v", err)
	}
	if len(messages) != 1 || messages[0].Content != "ops fallback task comment" {
		t.Fatalf("unexpected task fallback messages: %#v", messages)
	}
}

func TestStoreSaveMembersAndLoadArtifacts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-iota")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-iota","title":"Project Iota"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	if err := store.SaveMembers("project-iota", []Member{
		{AgentID: "agent://pc75/live-alpha", Role: "Maintainer", Status: "Pending"},
		{AgentID: "agent://pc75/live-alpha", Role: "owner", Status: "active"},
		{AgentID: "agent://pc75/live-bravo", Role: "odd", Status: "weird"},
	}); err != nil {
		t.Fatalf("SaveMembers error = %v", err)
	}
	members, err := store.LoadMembers("project-iota")
	if err != nil {
		t.Fatalf("LoadMembers error = %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}
	if members[0].Role != "maintainer" || members[0].Status != "pending" {
		t.Fatalf("unexpected normalized saved member: %#v", members[0])
	}
	if err := store.AppendArtifact("project-iota", Artifact{
		ArtifactID: "artifact-iota-1",
		Title:      "Iota Summary",
		Kind:       "link",
		LinkURL:    "https://example.com/iota",
		CreatedBy:  "agent://pc75/live-alpha",
		CreatedAt:  time.Date(2026, 4, 1, 7, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 4, 1, 7, 10, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendArtifact error = %v", err)
	}
	artifact, err := store.LoadArtifact("project-iota", "artifact-iota-1")
	if err != nil {
		t.Fatalf("LoadArtifact error = %v", err)
	}
	if artifact.Kind != "link" || artifact.LinkURL != "https://example.com/iota" {
		t.Fatalf("unexpected artifact: %#v", artifact)
	}
	artifacts, err := store.LoadArtifacts("project-iota", 10)
	if err != nil {
		t.Fatalf("LoadArtifacts error = %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].ArtifactID != "artifact-iota-1" {
		t.Fatalf("unexpected artifacts: %#v", artifacts)
	}
	if err := store.SaveArtifact("project-iota", Artifact{
		ArtifactID: "artifact-iota-1",
		Title:      "Iota Summary Updated",
		Kind:       "markdown",
		Content:    "Updated markdown body",
		CreatedBy:  "agent://pc75/live-alpha",
		CreatedAt:  artifact.CreatedAt,
		UpdatedAt:  time.Date(2026, 4, 1, 7, 20, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveArtifact error = %v", err)
	}
	artifact, err = store.LoadArtifact("project-iota", "artifact-iota-1")
	if err != nil {
		t.Fatalf("LoadArtifact(updated) error = %v", err)
	}
	if artifact.Title != "Iota Summary Updated" || artifact.Kind != "markdown" || artifact.Content != "Updated markdown body" {
		t.Fatalf("unexpected updated artifact: %#v", artifact)
	}
	if err := store.DeleteArtifact("project-iota", "artifact-iota-1"); err != nil {
		t.Fatalf("DeleteArtifact error = %v", err)
	}
	artifacts, err = store.LoadArtifacts("project-iota", 10)
	if err != nil {
		t.Fatalf("LoadArtifacts(after delete) error = %v", err)
	}
	if len(artifacts) != 0 {
		t.Fatalf("expected empty artifacts after delete, got %#v", artifacts)
	}
	if err := store.AppendHistory("project-iota", ChangeEvent{
		Scope:     "artifact",
		Action:    "delete",
		SubjectID: "artifact-iota-1",
		Summary:   "删除 Team Artifact",
		CreatedAt: time.Date(2026, 4, 1, 7, 30, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendHistory error = %v", err)
	}
	history, err := store.LoadHistory("project-iota", 10)
	if err != nil {
		t.Fatalf("LoadHistory error = %v", err)
	}
	if len(history) != 1 || history[0].SubjectID != "artifact-iota-1" || history[0].Action != "delete" {
		t.Fatalf("unexpected history: %#v", history)
	}
	if err := store.AppendHistory("project-iota", ChangeEvent{
		Scope:     "policy",
		Action:    "update",
		SubjectID: "policy",
		Summary:   "更新 Team Policy",
		Diff: map[string]FieldDiff{
			"require_signature": {Before: false, After: true},
			"noop":              {Before: "same", After: "same"},
		},
		CreatedAt: time.Date(2026, 4, 1, 7, 45, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendHistory(with diff) error = %v", err)
	}
	history, err = store.LoadHistory("project-iota", 10)
	if err != nil {
		t.Fatalf("LoadHistory(with diff) error = %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries, got %#v", history)
	}
	if _, ok := history[0].Diff["require_signature"]; !ok {
		t.Fatalf("expected require_signature diff, got %#v", history[0].Diff)
	}
	if _, ok := history[0].Diff["noop"]; ok {
		t.Fatalf("expected identical diff entry to be dropped, got %#v", history[0].Diff)
	}
}

func TestStoreMigrateArtifactsToIndexAndCompact(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-artifact-index")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	if err := store.AppendArtifact("project-artifact-index", Artifact{
		ArtifactID: "artifact-index-1",
		CreatedBy:  "agent://pc75/live-alpha",
		Title:      "Artifact One",
		Kind:       "markdown",
		CreatedAt:  time.Date(2026, 4, 3, 14, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 4, 3, 14, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendArtifact(first) error = %v", err)
	}
	if err := store.AppendArtifact("project-artifact-index", Artifact{
		ArtifactID: "artifact-index-2",
		CreatedBy:  "agent://pc75/live-bravo",
		Title:      "Artifact Two",
		Kind:       "json",
		CreatedAt:  time.Date(2026, 4, 3, 14, 10, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 4, 3, 14, 10, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AppendArtifact(second) error = %v", err)
	}
	if err := store.MigrateArtifactsToIndex("project-artifact-index"); err != nil {
		t.Fatalf("MigrateArtifactsToIndex error = %v", err)
	}
	if !store.hasArtifactIndex("project-artifact-index") {
		t.Fatal("expected artifact index to exist after migration")
	}
	artifacts, err := store.LoadArtifacts("project-artifact-index", 2)
	if err != nil {
		t.Fatalf("LoadArtifacts(indexed) error = %v", err)
	}
	if len(artifacts) != 2 || artifacts[0].ArtifactID != "artifact-index-2" {
		t.Fatalf("unexpected indexed artifacts: %#v", artifacts)
	}
	if err := store.SaveArtifact("project-artifact-index", Artifact{
		ArtifactID: "artifact-index-1",
		CreatedBy:  "agent://pc75/live-alpha",
		Title:      "Artifact One Updated",
		Kind:       "post",
		CreatedAt:  time.Date(2026, 4, 3, 14, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 4, 3, 14, 20, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SaveArtifact(indexed) error = %v", err)
	}
	before, err := os.Stat(store.artifactDataPath("project-artifact-index"))
	if err != nil {
		t.Fatalf("Stat(before compact) error = %v", err)
	}
	if err := store.CompactArtifacts("project-artifact-index"); err != nil {
		t.Fatalf("CompactArtifacts error = %v", err)
	}
	after, err := os.Stat(store.artifactDataPath("project-artifact-index"))
	if err != nil {
		t.Fatalf("Stat(after compact) error = %v", err)
	}
	if after.Size() >= before.Size() {
		t.Fatalf("expected compacted artifact data to shrink: before=%d after=%d", before.Size(), after.Size())
	}
	if err := store.DeleteArtifact("project-artifact-index", "artifact-index-2"); err != nil {
		t.Fatalf("DeleteArtifact(indexed) error = %v", err)
	}
	if _, err := store.LoadArtifact("project-artifact-index", "artifact-index-2"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("LoadArtifact after delete error = %v, want not exist", err)
	}
}

func TestStoreLoadArtifactsAndHistoryLimitReadLatestEntries(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-limit-tail")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-limit-tail","title":"Project Limit Tail"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	for i := 1; i <= 5; i++ {
		if err := store.AppendArtifact("project-limit-tail", Artifact{
			Title:     fmt.Sprintf("artifact-%d", i),
			Kind:      "markdown",
			CreatedAt: time.Date(2026, 4, 1, i, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 4, 1, i, 0, 0, 0, time.UTC),
		}); err != nil {
			t.Fatalf("AppendArtifact(%d) error = %v", i, err)
		}
		if err := store.AppendHistory("project-limit-tail", ChangeEvent{
			Scope:     "artifact",
			Action:    "create",
			SubjectID: fmt.Sprintf("artifact-%d", i),
			Summary:   fmt.Sprintf("history-%d", i),
			CreatedAt: time.Date(2026, 4, 1, i, 30, 0, 0, time.UTC),
		}); err != nil {
			t.Fatalf("AppendHistory(%d) error = %v", i, err)
		}
	}

	artifacts, err := store.LoadArtifacts("project-limit-tail", 2)
	if err != nil {
		t.Fatalf("LoadArtifacts error = %v", err)
	}
	if len(artifacts) != 2 || artifacts[0].Title != "artifact-5" || artifacts[1].Title != "artifact-4" {
		t.Fatalf("unexpected limited artifacts: %#v", artifacts)
	}

	history, err := store.LoadHistory("project-limit-tail", 2)
	if err != nil {
		t.Fatalf("LoadHistory error = %v", err)
	}
	if len(history) != 2 || history[0].Summary != "history-5" || history[1].Summary != "history-4" {
		t.Fatalf("unexpected limited history: %#v", history)
	}
}

func TestStoreApplyReplicatedSyncMessageAndHistoryAreIdempotent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	teamRoot := filepath.Join(root, "team", "project-sync")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(teamRoot) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-sync","title":"Project Sync"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey error = %v", err)
	}
	msg := Message{
		TeamID:          "project-sync",
		ChannelID:       "main",
		ContextID:       "ctx-sync",
		AuthorAgentID:   "agent://remote/alpha",
		OriginPublicKey: hex.EncodeToString(publicKey),
		MessageType:     "note",
		Content:         "replicated message",
		CreatedAt:       time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC),
	}
	payload, err := MessageSignaturePayload(msg)
	if err != nil {
		t.Fatalf("MessageSignaturePayload error = %v", err)
	}
	msg.Signature = hex.EncodeToString(ed25519.Sign(privateKey, payload))
	syncMsg := TeamSyncMessage{
		Type:    TeamSyncTypeMessage,
		TeamID:  "project-sync",
		Message: &msg,
	}.Normalize()

	applied, err := store.ApplyReplicatedSync(syncMsg)
	if err != nil {
		t.Fatalf("ApplyReplicatedSync(message) error = %v", err)
	}
	if !applied {
		t.Fatalf("expected first replicated message to apply")
	}
	applied, err = store.ApplyReplicatedSync(syncMsg)
	if err != nil {
		t.Fatalf("ApplyReplicatedSync(message duplicate) error = %v", err)
	}
	if applied {
		t.Fatalf("expected duplicate replicated message to be skipped")
	}
	messages, err := store.LoadMessages("project-sync", "main", 10)
	if err != nil {
		t.Fatalf("LoadMessages error = %v", err)
	}
	if len(messages) != 1 || messages[0].Content != "replicated message" {
		t.Fatalf("unexpected replicated messages: %#v", messages)
	}

	event := ChangeEvent{
		TeamID:    "project-sync",
		Scope:     "message",
		Action:    "create",
		SubjectID: msg.MessageID,
		Summary:   "replicated history",
		Source:    "p2p",
		CreatedAt: time.Date(2026, 4, 3, 12, 0, 1, 0, time.UTC),
	}
	syncHistory := TeamSyncMessage{
		Type:    TeamSyncTypeHistory,
		TeamID:  "project-sync",
		History: &event,
	}.Normalize()
	applied, err = store.ApplyReplicatedSync(syncHistory)
	if err != nil {
		t.Fatalf("ApplyReplicatedSync(history) error = %v", err)
	}
	if !applied {
		t.Fatalf("expected first replicated history to apply")
	}
	applied, err = store.ApplyReplicatedSync(syncHistory)
	if err != nil {
		t.Fatalf("ApplyReplicatedSync(history duplicate) error = %v", err)
	}
	if applied {
		t.Fatalf("expected duplicate replicated history to be skipped")
	}
	history, err := store.LoadHistory("project-sync", 10)
	if err != nil {
		t.Fatalf("LoadHistory error = %v", err)
	}
	if len(history) != 1 || history[0].Scope != "message" {
		t.Fatalf("unexpected replicated history: %#v", history)
	}
}

func TestStoreApplyReplicatedSyncTaskArtifactAndDeleteHistory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	teamRoot := filepath.Join(root, "team", "project-sync-objects")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(teamRoot) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-sync-objects","title":"Project Sync Objects"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}

	task := Task{
		TeamID:      "project-sync-objects",
		TaskID:      "task-sync-1",
		ChannelID:   "main",
		ContextID:   "ctx-sync-task",
		Title:       "replicated task",
		Status:      "doing",
		Priority:    "high",
		Description: "remote task",
		CreatedBy:   "agent://remote/task",
		CreatedAt:   time.Date(2026, 4, 3, 12, 10, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 4, 3, 12, 10, 1, 0, time.UTC),
	}
	taskSync := TeamSyncMessage{Type: TeamSyncTypeTask, TeamID: "project-sync-objects", Task: &task}.Normalize()
	applied, err := store.ApplyReplicatedSync(taskSync)
	if err != nil {
		t.Fatalf("ApplyReplicatedSync(task) error = %v", err)
	}
	if !applied {
		t.Fatalf("expected replicated task to apply")
	}
	loadedTask, err := store.LoadTask("project-sync-objects", "task-sync-1")
	if err != nil {
		t.Fatalf("LoadTask error = %v", err)
	}
	if loadedTask.Status != "doing" || loadedTask.Priority != "high" {
		t.Fatalf("unexpected replicated task: %#v", loadedTask)
	}

	staleTask := task
	staleTask.Status = "open"
	staleTask.UpdatedAt = time.Date(2026, 4, 3, 12, 9, 59, 0, time.UTC)
	applied, err = store.ApplyReplicatedSync(TeamSyncMessage{Type: TeamSyncTypeTask, TeamID: "project-sync-objects", Task: &staleTask}.Normalize())
	if err != nil {
		t.Fatalf("ApplyReplicatedSync(stale task) error = %v", err)
	}
	if applied {
		t.Fatalf("expected stale replicated task to be skipped")
	}

	artifact := Artifact{
		TeamID:     "project-sync-objects",
		ArtifactID: "artifact-sync-1",
		ChannelID:  "main",
		TaskID:     "task-sync-1",
		Title:      "replicated artifact",
		Kind:       "markdown",
		Summary:    "remote artifact",
		Content:    "hello",
		CreatedBy:  "agent://remote/task",
		CreatedAt:  time.Date(2026, 4, 3, 12, 11, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 4, 3, 12, 11, 1, 0, time.UTC),
	}
	artifactSync := TeamSyncMessage{Type: TeamSyncTypeArtifact, TeamID: "project-sync-objects", Artifact: &artifact}.Normalize()
	applied, err = store.ApplyReplicatedSync(artifactSync)
	if err != nil {
		t.Fatalf("ApplyReplicatedSync(artifact) error = %v", err)
	}
	if !applied {
		t.Fatalf("expected replicated artifact to apply")
	}
	loadedArtifact, err := store.LoadArtifact("project-sync-objects", "artifact-sync-1")
	if err != nil {
		t.Fatalf("LoadArtifact error = %v", err)
	}
	if loadedArtifact.TaskID != "task-sync-1" || loadedArtifact.Title != "replicated artifact" {
		t.Fatalf("unexpected replicated artifact: %#v", loadedArtifact)
	}

	deleteTaskHistory := ChangeEvent{
		EventID:   "task-delete-event-1",
		TeamID:    "project-sync-objects",
		Scope:     "task",
		Action:    "delete",
		SubjectID: "task-sync-1",
		Summary:   "task deleted remotely",
		Source:    "p2p",
		CreatedAt: time.Date(2026, 4, 3, 12, 12, 0, 0, time.UTC),
	}
	applied, err = store.ApplyReplicatedSync(TeamSyncMessage{Type: TeamSyncTypeHistory, TeamID: "project-sync-objects", History: &deleteTaskHistory}.Normalize())
	if err != nil {
		t.Fatalf("ApplyReplicatedSync(task delete history) error = %v", err)
	}
	if !applied {
		t.Fatalf("expected task delete history to apply")
	}
	if _, err := store.LoadTask("project-sync-objects", "task-sync-1"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected deleted replicated task to disappear, got err=%v", err)
	}

	deleteArtifactHistory := ChangeEvent{
		EventID:   "artifact-delete-event-1",
		TeamID:    "project-sync-objects",
		Scope:     "artifact",
		Action:    "delete",
		SubjectID: "artifact-sync-1",
		Summary:   "artifact deleted remotely",
		Source:    "p2p",
		CreatedAt: time.Date(2026, 4, 3, 12, 12, 1, 0, time.UTC),
	}
	applied, err = store.ApplyReplicatedSync(TeamSyncMessage{Type: TeamSyncTypeHistory, TeamID: "project-sync-objects", History: &deleteArtifactHistory}.Normalize())
	if err != nil {
		t.Fatalf("ApplyReplicatedSync(artifact delete history) error = %v", err)
	}
	if !applied {
		t.Fatalf("expected artifact delete history to apply")
	}
	if _, err := store.LoadArtifact("project-sync-objects", "artifact-sync-1"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected deleted replicated artifact to disappear, got err=%v", err)
	}
}

func TestStoreApplyReplicatedSyncMemberPolicyChannel(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "replicate-team")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(teamRoot) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"replicate-team","title":"Replicate Team"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}

	memberVersion := time.Date(2026, 4, 3, 13, 10, 0, 0, time.UTC)
	members := []Member{
		{AgentID: "agent://pc75/owner", Role: "owner", Status: "active", JoinedAt: memberVersion.Add(-time.Hour), UpdatedAt: memberVersion},
		{AgentID: "agent://pc76/member", Role: "member", Status: "pending", JoinedAt: memberVersion.Add(-30 * time.Minute), UpdatedAt: memberVersion},
	}
	applied, err := store.ApplyReplicatedSync(TeamSyncMessage{
		Type:      TeamSyncTypeMember,
		TeamID:    "replicate-team",
		Members:   members,
		CreatedAt: memberVersion,
	})
	if err != nil || !applied {
		t.Fatalf("ApplyReplicatedSync(members) = (%v, %v), want (true, nil)", applied, err)
	}
	gotMembers, err := store.LoadMembers("replicate-team")
	if err != nil {
		t.Fatalf("LoadMembers error = %v", err)
	}
	if len(gotMembers) != 2 || gotMembers[1].UpdatedAt.IsZero() {
		t.Fatalf("unexpected replicated members: %#v", gotMembers)
	}
	applied, err = store.ApplyReplicatedSync(TeamSyncMessage{
		Type:      TeamSyncTypeMember,
		TeamID:    "replicate-team",
		Members:   members,
		CreatedAt: memberVersion.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("ApplyReplicatedSync(stale members) error = %v", err)
	}
	if applied {
		t.Fatalf("expected stale member snapshot to be skipped")
	}

	policyVersion := time.Date(2026, 4, 3, 13, 15, 0, 0, time.UTC)
	applied, err = store.ApplyReplicatedSync(TeamSyncMessage{
		Type:   TeamSyncTypePolicy,
		TeamID: "replicate-team",
		Policy: &Policy{
			MessageRoles:     []string{"owner", "member"},
			TaskRoles:        []string{"owner", "maintainer"},
			SystemNoteRoles:  []string{"owner"},
			RequireSignature: true,
			UpdatedAt:        policyVersion,
		},
		CreatedAt: policyVersion,
	})
	if err != nil || !applied {
		t.Fatalf("ApplyReplicatedSync(policy) = (%v, %v), want (true, nil)", applied, err)
	}
	gotPolicy, err := store.LoadPolicy("replicate-team")
	if err != nil {
		t.Fatalf("LoadPolicy error = %v", err)
	}
	if !gotPolicy.RequireSignature || len(gotPolicy.MessageRoles) != 2 {
		t.Fatalf("unexpected replicated policy: %#v", gotPolicy)
	}

	channelVersion := time.Date(2026, 4, 3, 13, 20, 0, 0, time.UTC)
	applied, err = store.ApplyReplicatedSync(TeamSyncMessage{
		Type:   TeamSyncTypeChannel,
		TeamID: "replicate-team",
		Channel: &Channel{
			ChannelID:   "research",
			Title:       "Research",
			Description: "deep work",
			Hidden:      true,
			CreatedAt:   channelVersion.Add(-time.Minute),
			UpdatedAt:   channelVersion,
		},
		CreatedAt: channelVersion,
	})
	if err != nil || !applied {
		t.Fatalf("ApplyReplicatedSync(channel) = (%v, %v), want (true, nil)", applied, err)
	}
	gotChannel, err := store.LoadChannel("replicate-team", "research")
	if err != nil {
		t.Fatalf("LoadChannel error = %v", err)
	}
	if gotChannel.Title != "Research" || !gotChannel.Hidden {
		t.Fatalf("unexpected replicated channel: %#v", gotChannel)
	}
	applied, err = store.ApplyReplicatedSync(TeamSyncMessage{
		Type:   TeamSyncTypeChannel,
		TeamID: "replicate-team",
		Channel: &Channel{
			ChannelID: "research",
			Title:     "Old Research",
			UpdatedAt: channelVersion.Add(-time.Minute),
		},
		CreatedAt: channelVersion.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("ApplyReplicatedSync(stale channel) error = %v", err)
	}
	if applied {
		t.Fatalf("expected stale channel snapshot to be skipped")
	}

	configVersion := time.Date(2026, 4, 3, 13, 25, 0, 0, time.UTC)
	applied, err = store.ApplyReplicatedSync(TeamSyncMessage{
		Type:   TeamSyncTypeChannelConfig,
		TeamID: "replicate-team",
		ChannelConfig: &ChannelConfig{
			ChannelID:       "research",
			Plugin:          "plan-exchange@1.0",
			Theme:           "minimal",
			AgentOnboarding: "Use plan mode first.",
			Rules:           []string{"Keep decisions explicit"},
			UpdatedAt:       configVersion,
		},
		CreatedAt: configVersion,
	})
	if err != nil || !applied {
		t.Fatalf("ApplyReplicatedSync(channel_config) = (%v, %v), want (true, nil)", applied, err)
	}
	gotConfig, err := store.LoadChannelConfig("replicate-team", "research")
	if err != nil {
		t.Fatalf("LoadChannelConfig error = %v", err)
	}
	if gotConfig.Plugin != "plan-exchange@1.0" || gotConfig.Theme != "minimal" {
		t.Fatalf("unexpected replicated channel config: %#v", gotConfig)
	}
	applied, err = store.ApplyReplicatedSync(TeamSyncMessage{
		Type:   TeamSyncTypeChannelConfig,
		TeamID: "replicate-team",
		ChannelConfig: &ChannelConfig{
			ChannelID: "research",
			Plugin:    "old-plugin@0.1",
			Theme:     "legacy",
			UpdatedAt: configVersion.Add(-time.Minute),
		},
		CreatedAt: configVersion.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("ApplyReplicatedSync(stale channel_config) error = %v", err)
	}
	if applied {
		t.Fatalf("expected stale channel config snapshot to be skipped")
	}
}

func TestStoreSaveMembersNormalizesStatusesForApprovalFlow(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-kappa")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-kappa","title":"Project Kappa"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	if err := store.SaveMembers("project-kappa", []Member{
		{AgentID: "agent://pc75/live-pending", Role: "member", Status: "pending"},
		{AgentID: "agent://pc75/live-muted", Role: "member", Status: "muted"},
		{AgentID: "agent://pc75/live-removed", Role: "member", Status: "removed"},
	}); err != nil {
		t.Fatalf("SaveMembers error = %v", err)
	}
	members, err := store.LoadMembers("project-kappa")
	if err != nil {
		t.Fatalf("LoadMembers error = %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(members))
	}
	statuses := map[string]string{}
	for _, member := range members {
		statuses[member.AgentID] = member.Status
	}
	if statuses["agent://pc75/live-pending"] != "pending" || statuses["agent://pc75/live-muted"] != "muted" || statuses["agent://pc75/live-removed"] != "removed" {
		t.Fatalf("unexpected statuses after save/load: %#v", statuses)
	}
}

func TestStoreNormalizesTaskStatusPriorityAndArtifactTaskID(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-normalize")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-normalize","title":"Project Normalize"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}

	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	if err := store.AppendTask("project-normalize", Task{
		TaskID:    "normalize-task-1",
		Title:     "Normalize task values",
		ChannelID: " research ",
		Status:    "Completed",
		Priority:  "urgent",
		Assignees: []string{" agent://pc75/live-alpha "},
		Labels:    []string{" qa "},
	}); err != nil {
		t.Fatalf("AppendTask error = %v", err)
	}
	task, err := store.LoadTask("project-normalize", "normalize-task-1")
	if err != nil {
		t.Fatalf("LoadTask error = %v", err)
	}
	if task.Status != "done" || task.Priority != "high" {
		t.Fatalf("unexpected normalized task: %#v", task)
	}
	if task.ChannelID != "research" {
		t.Fatalf("unexpected task channel: %#v", task.ChannelID)
	}
	if len(task.Assignees) != 1 || task.Assignees[0] != "agent://pc75/live-alpha" {
		t.Fatalf("unexpected assignees: %#v", task.Assignees)
	}
	if err := store.AppendArtifact("project-normalize", Artifact{
		ArtifactID: "normalize-artifact-1",
		Title:      "Normalize artifact relations",
		ChannelID:  " main ",
		TaskID:     " normalize-task-1 ",
	}); err != nil {
		t.Fatalf("AppendArtifact error = %v", err)
	}
	artifact, err := store.LoadArtifact("project-normalize", "normalize-artifact-1")
	if err != nil {
		t.Fatalf("LoadArtifact error = %v", err)
	}
	if artifact.ChannelID != "main" || artifact.TaskID != "normalize-task-1" {
		t.Fatalf("unexpected normalized artifact: %#v", artifact)
	}
}

func TestStoreAppendTaskConcurrentPreservesAllTasks(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-concurrent")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-concurrent","title":"Project Concurrent"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}

	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}

	const writers = 12
	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		i := i
		go func() {
			defer wg.Done()
			if err := store.AppendTask("project-concurrent", Task{
				TaskID:    "task-" + strings.TrimSpace(time.Unix(int64(i), 0).UTC().Format("150405")),
				Title:     "Concurrent Task " + time.Unix(int64(i), 0).UTC().Format("150405"),
				Status:    "open",
				Labels:    []string{"load"},
				CreatedBy: "agent://pc75/test",
			}); err != nil {
				t.Errorf("AppendTask(%d) error = %v", i, err)
			}
		}()
	}
	wg.Wait()

	tasks, err := store.LoadTasks("project-concurrent", 0)
	if err != nil {
		t.Fatalf("LoadTasks error = %v", err)
	}
	if len(tasks) != writers {
		t.Fatalf("expected %d tasks, got %d", writers, len(tasks))
	}
}
