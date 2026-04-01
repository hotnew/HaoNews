package team

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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
