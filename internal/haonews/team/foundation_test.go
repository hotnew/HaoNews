package team

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTypedErrorsExposeStableCodes(t *testing.T) {
	t.Parallel()

	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	if _, err := store.LoadTeamCtx(context.Background(), ""); !errors.Is(err, ErrEmptyID) {
		t.Fatalf("LoadTeamCtx error = %v, want ErrEmptyID", err)
	}

	var nilStore *Store
	if _, err := nilStore.LoadTeamCtx(context.Background(), "team-alpha"); !errors.Is(err, ErrNilStore) {
		t.Fatalf("nil store LoadTeamCtx error = %v, want ErrNilStore", err)
	}
}

func TestPolicyEnforcerAllow(t *testing.T) {
	t.Parallel()

	store := openTeamStoreWithFixture(t, "team-enforcer")

	enforcer := NewPolicyEnforcer(store)
	if err := enforcer.Allow(context.Background(), "team-enforcer", "agent-owner", ActionPolicyUpdate); err != nil {
		t.Fatalf("owner Allow(policy.update) error = %v", err)
	}
	if err := enforcer.Allow(context.Background(), "team-enforcer", "agent-member", ActionMessageSend); err != nil {
		t.Fatalf("member Allow(message.send) error = %v", err)
	}
	if err := enforcer.Allow(context.Background(), "team-enforcer", "agent-member", ActionPolicyUpdate); !errors.Is(err, ErrForbidden) {
		t.Fatalf("member Allow(policy.update) error = %v, want ErrForbidden", err)
	}
}

func TestStoreListTasksCtxFilterAndChannelContextProvider(t *testing.T) {
	t.Parallel()

	store := openTeamStoreWithFixture(t, "team-context")
	ctx := context.Background()
	if err := store.SaveChannelCtx(ctx, "team-context", Channel{ChannelID: "research", Title: "Research"}); err != nil {
		t.Fatalf("SaveChannelCtx error = %v", err)
	}
	if err := store.SaveChannelConfigCtx(ctx, "team-context", ChannelConfig{
		ChannelID:       "research",
		Plugin:          "review-room@1.0",
		Theme:           "focus",
		AgentOnboarding: "Focus on critical path.",
	}); err != nil {
		t.Fatalf("SaveChannelConfigCtx error = %v", err)
	}

	taskOpen := Task{
		TaskID:    "task-open",
		TeamID:    "team-context",
		ChannelID: "research",
		ContextID: "ctx-1",
		Title:     "Open task",
		Status:    TaskStateOpen,
		Priority:  TaskPriorityHigh,
		CreatedBy: "agent-owner",
		Assignees: []string{"agent-member"},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := store.AppendTaskCtx(ctx, "team-context", taskOpen); err != nil {
		t.Fatalf("AppendTaskCtx(open) error = %v", err)
	}
	taskDone := Task{
		TaskID:    "task-done",
		TeamID:    "team-context",
		ChannelID: "research",
		ContextID: "ctx-2",
		Title:     "Done task",
		Status:    TaskStateDone,
		Priority:  TaskPriorityLow,
		CreatedBy: "agent-owner",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		ClosedAt:  time.Now().UTC(),
	}
	if err := store.AppendTaskCtx(ctx, "team-context", taskDone); err != nil {
		t.Fatalf("AppendTaskCtx(done) error = %v", err)
	}
	if err := store.AppendMessageCtx(ctx, "team-context", Message{
		TeamID:        "team-context",
		ChannelID:     "research",
		ContextID:     "ctx-1",
		AuthorAgentID: "agent-member",
		MessageType:   "review",
		Content:       "Need follow-up",
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AppendMessageCtx error = %v", err)
	}

	tasks, err := store.ListTasksCtx(ctx, "team-context", TaskFilter{
		Statuses:        []string{TaskStateOpen},
		AssigneeAgentID: "agent-member",
		ChannelID:       "research",
	})
	if err != nil {
		t.Fatalf("ListTasksCtx error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].TaskID != "task-open" {
		t.Fatalf("ListTasksCtx = %#v, want task-open only", tasks)
	}

	provider := NewChannelContextProvider(store)
	snapshot, err := provider.GetChannelContext(ctx, "team-context", "research")
	if err != nil {
		t.Fatalf("GetChannelContext error = %v", err)
	}
	if snapshot.Channel.ChannelID != "research" || snapshot.AgentOnboarding == "" {
		t.Fatalf("unexpected snapshot channel = %#v", snapshot)
	}
	if len(snapshot.ActiveTasks) != 1 || snapshot.ActiveTasks[0].TaskID != "task-open" {
		t.Fatalf("ActiveTasks = %#v", snapshot.ActiveTasks)
	}
	if len(snapshot.RecentMessages) != 1 || snapshot.RecentMessages[0].ContextID != "ctx-1" {
		t.Fatalf("RecentMessages = %#v", snapshot.RecentMessages)
	}
	if len(snapshot.Threads) != 1 || !strings.Contains(snapshot.AgentPrompt, "Focus on critical path.") || !strings.Contains(snapshot.AgentPrompt, "Open task") {
		t.Fatalf("expected thread summaries and prompt, got %#v", snapshot)
	}
}

func TestTaskHooksFireOnStatusTransition(t *testing.T) {
	t.Parallel()

	store := openTeamStoreWithFixture(t, "team-hooks")
	ctx := context.Background()
	if err := store.AppendTaskCtx(ctx, "team-hooks", Task{
		TaskID:    "task-hook",
		TeamID:    "team-hooks",
		ChannelID: "main",
		Title:     "Hook me",
		Status:    TaskStateOpen,
		CreatedBy: "agent-owner",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AppendTaskCtx error = %v", err)
	}

	events := make(chan TaskTransitionEvent, 1)
	store.TaskHooks = &HookRegistry{}
	store.TaskHooks.Register(TaskLifecycleHookFunc(func(_ context.Context, event TaskTransitionEvent) {
		events <- event
	}))

	if err := store.SaveTaskCtx(ctx, "team-hooks", Task{
		TaskID:    "task-hook",
		TeamID:    "team-hooks",
		ChannelID: "main",
		Title:     "Hook me",
		Status:    TaskStateDoing,
		CreatedBy: "agent-owner",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveTaskCtx error = %v", err)
	}

	select {
	case event := <-events:
		if event.FromState != TaskStateOpen || event.ToState != TaskStateDoing {
			t.Fatalf("event = %#v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected task hook event")
	}
}

func TestTaskDependencyAndMilestoneLifecycle(t *testing.T) {
	t.Parallel()

	store := openTeamStoreWithFixture(t, "team-phase5")
	ctx := context.Background()
	if err := store.SaveMilestoneCtx(ctx, "team-phase5", Milestone{
		MilestoneID: "m1",
		TeamID:      "team-phase5",
		Title:       "Ship phase 5",
		Status:      MilestoneStateOpen,
	}); err != nil {
		t.Fatalf("SaveMilestoneCtx error = %v", err)
	}
	if err := store.AppendTaskCtx(ctx, "team-phase5", Task{
		TaskID:      "dep-1",
		TeamID:      "team-phase5",
		ChannelID:   "main",
		Title:       "Dependency",
		Status:      TaskStateOpen,
		CreatedBy:   "agent-owner",
		MilestoneID: "m1",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AppendTaskCtx(dep) error = %v", err)
	}
	if err := store.AppendTaskCtx(ctx, "team-phase5", Task{
		TaskID:       "task-2",
		TeamID:       "team-phase5",
		ChannelID:    "main",
		Title:        "Blocked by dep",
		Status:       TaskStateOpen,
		CreatedBy:    "agent-owner",
		DependsOn:    []string{"dep-1"},
		ParentTaskID: "dep-1",
		MilestoneID:  "m1",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AppendTaskCtx(task-2) error = %v", err)
	}
	task, err := store.LoadTaskCtx(ctx, "team-phase5", "task-2")
	if err != nil {
		t.Fatalf("LoadTaskCtx(task-2) error = %v", err)
	}
	task.Status = TaskStateDoing
	task.UpdatedAt = time.Now().UTC()
	if err := store.SaveTaskCtx(ctx, "team-phase5", task); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("SaveTaskCtx dependency gate error = %v, want ErrInvalidState", err)
	}
	dep, err := store.LoadTaskCtx(ctx, "team-phase5", "dep-1")
	if err != nil {
		t.Fatalf("LoadTaskCtx(dep-1) error = %v", err)
	}
	dep.Status = TaskStateDone
	dep.UpdatedAt = time.Now().UTC()
	if err := store.SaveTaskCtx(ctx, "team-phase5", dep); err != nil {
		t.Fatalf("SaveTaskCtx(dep done) error = %v", err)
	}
	task.UpdatedAt = time.Now().UTC()
	if err := store.SaveTaskCtx(ctx, "team-phase5", task); err != nil {
		t.Fatalf("SaveTaskCtx(task-2 doing) error = %v", err)
	}
	progress, err := store.ListMilestoneProgressCtx(ctx, "team-phase5")
	if err != nil {
		t.Fatalf("ListMilestoneProgressCtx error = %v", err)
	}
	if len(progress) != 1 || progress[0].TaskCount != 2 || progress[0].Completed != 1 || progress[0].Active != 1 {
		t.Fatalf("progress = %#v", progress)
	}
}

func TestCreateTeamFromTemplateCtx(t *testing.T) {
	t.Parallel()

	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	info, template, err := store.CreateTeamFromTemplateCtx(context.Background(), Info{
		TeamID:       "planning-team",
		Title:        "Planning Team",
		OwnerAgentID: "agent-owner",
	}, "planning", map[string]string{
		"planner": "agent-planner",
	})
	if err != nil {
		t.Fatalf("CreateTeamFromTemplateCtx error = %v", err)
	}
	if template.TemplateID != "planning" || info.TeamID != "planning-team" {
		t.Fatalf("unexpected template/info = %#v %#v", template, info)
	}
	configs, err := store.ListChannelConfigsCtx(context.Background(), "planning-team")
	if err != nil {
		t.Fatalf("ListChannelConfigsCtx error = %v", err)
	}
	if len(configs) < 2 {
		t.Fatalf("expected template channel configs, got %#v", configs)
	}
	members, err := store.LoadMembersCtx(context.Background(), "planning-team")
	if err != nil {
		t.Fatalf("LoadMembersCtx error = %v", err)
	}
	if len(members) < 2 {
		t.Fatalf("expected template role bindings, got %#v", members)
	}
}

func TestCreateTeamFromSpecPackageTemplateCtx(t *testing.T) {
	t.Parallel()

	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	info, template, err := store.CreateTeamFromTemplateCtx(context.Background(), Info{
		TeamID:       "spec-team",
		Title:        "Spec Team",
		OwnerAgentID: "agent-owner",
	}, "spec-package", map[string]string{
		"proposer": "agent-proposer",
		"reviewer": "agent-reviewer",
		"editor":   "agent-editor",
	})
	if err != nil {
		t.Fatalf("CreateTeamFromTemplateCtx(spec-package) error = %v", err)
	}
	if template.TemplateID != "spec-package" || info.TeamID != "spec-team" {
		t.Fatalf("unexpected spec template/info = %#v %#v", template, info)
	}
	configs, err := store.ListChannelConfigsCtx(context.Background(), "spec-team")
	if err != nil {
		t.Fatalf("ListChannelConfigsCtx error = %v", err)
	}
	if len(configs) != 4 {
		t.Fatalf("expected 4 spec template channel configs, got %#v", configs)
	}
	members, err := store.LoadMembersCtx(context.Background(), "spec-team")
	if err != nil {
		t.Fatalf("LoadMembersCtx error = %v", err)
	}
	if len(members) != 4 {
		t.Fatalf("expected spec template role bindings, got %#v", members)
	}
	progress, err := store.ListMilestoneProgressCtx(context.Background(), "spec-team")
	if err != nil {
		t.Fatalf("ListMilestoneProgressCtx error = %v", err)
	}
	if len(progress) != 1 || progress[0].Milestone.MilestoneID != "spec-package-ready" {
		t.Fatalf("unexpected milestone progress = %#v", progress)
	}
}

func TestDetectReplicatedTaskStatusConflictAutoResolvable(t *testing.T) {
	t.Parallel()

	store := openTeamStoreWithFixture(t, "team-sync-auto")
	ctx := context.Background()
	current := Task{
		TaskID:    "task-sync-1",
		TeamID:    "team-sync-auto",
		ContextID: "ctx-sync-1",
		Title:     "Review release plan",
		Status:    TaskStateDoing,
		Priority:  TaskPriorityHigh,
		CreatedBy: "agent-owner",
		Assignees: []string{"agent-member"},
		CreatedAt: time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 10, 10, 5, 0, 0, time.UTC),
	}
	if err := store.AppendTaskCtx(ctx, "team-sync-auto", current); err != nil {
		t.Fatalf("AppendTaskCtx(current) error = %v", err)
	}

	conflict, ok, err := store.DetectReplicatedConflict(NewTaskSyncMsg("team-sync-auto", "node-74", Task{
		TaskID:    "task-sync-1",
		TeamID:    "team-sync-auto",
		ContextID: "ctx-sync-1",
		Title:     "Review release plan",
		Status:    TaskStateOpen,
		Priority:  TaskPriorityHigh,
		CreatedBy: "agent-owner",
		Assignees: []string{"agent-member"},
		CreatedAt: current.CreatedAt,
		UpdatedAt: time.Date(2026, 4, 10, 10, 4, 0, 0, time.UTC),
	}))
	if err != nil {
		t.Fatalf("DetectReplicatedConflict(status-only) error = %v", err)
	}
	if !ok || !conflict.AutoResolvable {
		t.Fatalf("status-only conflict = %#v, ok = %v, want auto_resolvable", conflict, ok)
	}

	conflict, ok, err = store.DetectReplicatedConflict(NewTaskSyncMsg("team-sync-auto", "node-74", Task{
		TaskID:    "task-sync-1",
		TeamID:    "team-sync-auto",
		ContextID: "ctx-sync-1",
		Title:     "Review release checklist",
		Status:    TaskStateOpen,
		Priority:  TaskPriorityHigh,
		CreatedBy: "agent-owner",
		Assignees: []string{"agent-member"},
		CreatedAt: current.CreatedAt,
		UpdatedAt: time.Date(2026, 4, 10, 10, 4, 0, 0, time.UTC),
	}))
	if err != nil {
		t.Fatalf("DetectReplicatedConflict(content-change) error = %v", err)
	}
	if !ok || conflict.AutoResolvable {
		t.Fatalf("content-change conflict = %#v, ok = %v, want manual review", conflict, ok)
	}
}

func TestTeamSyncValidateAndBuilders(t *testing.T) {
	t.Parallel()

	msg := NewMessageSyncMsg("team-sync", "node-a", Message{
		MessageID:     "msg-1",
		TeamID:        "team-sync",
		ChannelID:     "main",
		AuthorAgentID: "agent-owner",
		MessageType:   "chat",
		Content:       "hello",
		CreatedAt:     time.Now().UTC(),
	})
	if err := msg.Validate(); err != nil {
		t.Fatalf("Validate(message) error = %v", err)
	}
	taskMsg := NewTaskSyncMsg("team-sync", "node-a", Task{
		TaskID:    "task-1",
		TeamID:    "team-sync",
		ChannelID: "main",
		Title:     "Ship it",
		Status:    TaskStateOpen,
	})
	if err := taskMsg.Validate(); err != nil {
		t.Fatalf("Validate(task) error = %v", err)
	}
	memberMsg := NewMemberSyncMsg("team-sync", "node-a", []Member{
		{AgentID: "agent-owner", Role: MemberRoleOwner, Status: MemberStatusActive},
	})
	if err := memberMsg.Validate(); err != nil {
		t.Fatalf("Validate(member) error = %v", err)
	}
	if err := (TeamSyncMessage{Type: TeamSyncTypeTask, TeamID: "team-sync"}).Validate(); err == nil {
		t.Fatal("expected missing task payload error")
	}
}

func TestTaskDispatchAndThreadCtx(t *testing.T) {
	t.Parallel()

	store := openTeamStoreWithFixture(t, "team-thread")
	ctx := context.Background()
	if err := store.AppendTaskCtx(ctx, "team-thread", Task{
		TaskID:    "task-thread",
		TeamID:    "team-thread",
		ChannelID: "main",
		ContextID: "ctx-thread",
		Title:     "Thread task",
		Status:    TaskStateOpen,
		CreatedBy: "agent-owner",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AppendTaskCtx error = %v", err)
	}
	if err := store.AppendMessageCtx(ctx, "team-thread", Message{
		TeamID:          "team-thread",
		ChannelID:       "main",
		ContextID:       "ctx-thread",
		ParentMessageID: "parent-1",
		AuthorAgentID:   "agent-owner",
		MessageType:     "decision",
		Content:         "Primary task message",
		StructuredData:  map[string]any{"task_id": "task-thread"},
		CreatedAt:       time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AppendMessageCtx(task) error = %v", err)
	}
	if err := store.AppendMessageCtx(ctx, "team-thread", Message{
		TeamID:        "team-thread",
		ChannelID:     "main",
		ContextID:     "ctx-thread",
		AuthorAgentID: "agent-member",
		MessageType:   "comment",
		Content:       "Context-only follow-up",
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AppendMessageCtx(context) error = %v", err)
	}
	if err := store.SaveTaskDispatchCtx(ctx, "team-thread", TaskDispatch{
		TaskID:          "task-thread",
		AssignedAgentID: "agent-member",
		MatchReason:     "label:review",
		Status:          TaskDispatchStatusQueued,
		TimeoutSeconds:  90,
	}); err != nil {
		t.Fatalf("SaveTaskDispatchCtx error = %v", err)
	}

	dispatch, err := store.LoadTaskDispatchCtx(ctx, "team-thread", "task-thread")
	if err != nil {
		t.Fatalf("LoadTaskDispatchCtx error = %v", err)
	}
	if dispatch.AssignedAgentID != "agent-member" || dispatch.TimeoutSeconds != 90 {
		t.Fatalf("dispatch = %#v", dispatch)
	}

	thread, err := store.LoadTaskThreadCtx(ctx, "team-thread", "task-thread", 10)
	if err != nil {
		t.Fatalf("LoadTaskThreadCtx error = %v", err)
	}
	if thread.Dispatch == nil || thread.Dispatch.AssignedAgentID != "agent-member" {
		t.Fatalf("thread.Dispatch = %#v", thread.Dispatch)
	}
	if len(thread.Messages) != 2 {
		t.Fatalf("thread.Messages = %#v", thread.Messages)
	}
	if thread.Messages[0].ParentMessageID != "parent-1" && thread.Messages[1].ParentMessageID != "parent-1" {
		t.Fatalf("expected parent_message_id to survive thread load, got %#v", thread.Messages)
	}
}

func openTeamStoreWithFixture(t *testing.T, teamID string) *Store {
	t.Helper()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", teamID)
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{
  "team_id":"`+teamID+`",
  "title":"Test Team",
  "owner_agent_id":"agent-owner",
  "channels":["main","research"]
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "members.json"), []byte(`[
  {"agent_id":"agent-owner","role":"owner","status":"active"},
  {"agent_id":"agent-member","role":"member","status":"active"},
  {"agent_id":"agent-muted","role":"observer","status":"muted"}
]`), 0o644); err != nil {
		t.Fatalf("WriteFile(members.json) error = %v", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	return store
}
