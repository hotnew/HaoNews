package haonews

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	teamcore "hao.news/internal/haonews/team"
)

type fakeTeamSyncTransport struct {
	published []teamcore.TeamSyncMessage
	handlers  map[string]func(teamcore.TeamSyncMessage) (bool, error)
}

func (f *fakeTeamSyncTransport) PublishTeamSync(_ context.Context, sync teamcore.TeamSyncMessage) error {
	f.published = append(f.published, sync.Normalize())
	return nil
}

func (f *fakeTeamSyncTransport) SubscribeTeamSync(_ context.Context, teamID string, handler func(teamcore.TeamSyncMessage) (bool, error)) error {
	if f.handlers == nil {
		f.handlers = make(map[string]func(teamcore.TeamSyncMessage) (bool, error))
	}
	f.handlers[teamcore.NormalizeTeamID(teamID)] = handler
	return nil
}

func TestTeamPubSubRuntimePrimesThenPublishesNewMessageAndHistory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-team-sync")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(teamRoot) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-team-sync","title":"Team Sync"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := teamcore.OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	msg1 := signedTeamMessage(t, "project-team-sync", "first", time.Date(2026, 4, 3, 13, 0, 0, 0, time.UTC))
	if err := store.AppendMessage("project-team-sync", msg1); err != nil {
		t.Fatalf("AppendMessage(first) error = %v", err)
	}
	if err := store.AppendHistory("project-team-sync", teamcore.ChangeEvent{
		TeamID:    "project-team-sync",
		Scope:     "message",
		Action:    "create",
		SubjectID: msg1.MessageID,
		Source:    "api",
		CreatedAt: msg1.CreatedAt.Add(time.Second),
	}); err != nil {
		t.Fatalf("AppendHistory(first) error = %v", err)
	}

	transport := &fakeTeamSyncTransport{}
	runtime, err := startTeamPubSubRuntime(root, transport, "node-1")
	if err != nil {
		t.Fatalf("startTeamPubSubRuntime error = %v", err)
	}
	runtime.startedAt = time.Date(2026, 4, 3, 13, 0, 2, 0, time.UTC)
	if err := runtime.SyncOnce(context.Background(), nil); err != nil {
		t.Fatalf("SyncOnce(prime) error = %v", err)
	}
	if len(transport.published) != 0 {
		t.Fatalf("expected priming scan to avoid publish, got %#v", transport.published)
	}

	msg2 := signedTeamMessage(t, "project-team-sync", "second", time.Date(2026, 4, 3, 13, 0, 5, 0, time.UTC))
	if err := store.AppendMessage("project-team-sync", msg2); err != nil {
		t.Fatalf("AppendMessage(second) error = %v", err)
	}
	if err := store.AppendHistory("project-team-sync", teamcore.ChangeEvent{
		TeamID:    "project-team-sync",
		Scope:     "message",
		Action:    "create",
		SubjectID: msg2.MessageID,
		Source:    "api",
		CreatedAt: msg2.CreatedAt.Add(time.Second),
	}); err != nil {
		t.Fatalf("AppendHistory(second) error = %v", err)
	}

	if err := runtime.SyncOnce(context.Background(), nil); err != nil {
		t.Fatalf("SyncOnce(publish) error = %v", err)
	}
	if len(transport.published) != 2 {
		t.Fatalf("published = %d, want 2", len(transport.published))
	}
	if transport.published[0].Type != teamcore.TeamSyncTypeMessage || transport.published[0].Message == nil || transport.published[0].Message.MessageID != msg2.MessageID {
		t.Fatalf("unexpected first sync payload: %#v", transport.published[0])
	}
	if transport.published[1].Type != teamcore.TeamSyncTypeHistory || transport.published[1].History == nil || transport.published[1].History.SubjectID != msg2.MessageID {
		t.Fatalf("unexpected second sync payload: %#v", transport.published[1])
	}
	status := runtime.Status()
	if status.PublishedMessages != 1 || status.PublishedHistory != 1 {
		t.Fatalf("status = %+v, want one published message and history", status)
	}
	if status.SubscribedTeams != 1 || status.PrimedChannels == 0 || status.PrimedHistoryTeams != 1 {
		t.Fatalf("status = %+v, want primed/subscribed counters", status)
	}
	if status.ScannedMessages == 0 || status.ScannedHistory == 0 {
		t.Fatalf("status = %+v, want scanned counters", status)
	}
}

func TestTeamPubSubRuntimeAppliesInboundReplicatedMessage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-team-sync")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(teamRoot) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-team-sync","title":"Team Sync"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	transport := &fakeTeamSyncTransport{}
	runtime, err := startTeamPubSubRuntime(root, transport, "node-1")
	if err != nil {
		t.Fatalf("startTeamPubSubRuntime error = %v", err)
	}
	if err := runtime.SyncOnce(context.Background(), nil); err != nil {
		t.Fatalf("SyncOnce(prime) error = %v", err)
	}
	handler := transport.handlers["project-team-sync"]
	if handler == nil {
		t.Fatalf("expected team subscription to be installed")
	}

	remoteMsg := signedTeamMessage(t, "project-team-sync", "remote", time.Date(2026, 4, 3, 14, 0, 0, 0, time.UTC))
	applied, err := handler(teamcore.TeamSyncMessage{
		Type:       teamcore.TeamSyncTypeMessage,
		TeamID:     "project-team-sync",
		Message:    &remoteMsg,
		SourceNode: "node-2",
		CreatedAt:  remoteMsg.CreatedAt,
	})
	if err != nil {
		t.Fatalf("handler(message) error = %v", err)
	}
	if !applied {
		t.Fatalf("expected inbound replicated message to apply")
	}
	applied, err = handler(teamcore.TeamSyncMessage{
		Type:       teamcore.TeamSyncTypeMessage,
		TeamID:     "project-team-sync",
		Message:    &remoteMsg,
		SourceNode: "node-2",
		CreatedAt:  remoteMsg.CreatedAt,
	})
	if err != nil {
		t.Fatalf("handler(message duplicate) error = %v", err)
	}
	if applied {
		t.Fatalf("expected duplicate inbound replicated message to be skipped")
	}

	store, err := teamcore.OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	messages, err := store.LoadMessages("project-team-sync", "main", 10)
	if err != nil {
		t.Fatalf("LoadMessages error = %v", err)
	}
	if len(messages) != 1 || messages[0].Content != "remote" {
		t.Fatalf("unexpected replicated runtime messages: %#v", messages)
	}
	status := runtime.Status()
	if status.ReceivedMessages != 2 || status.AppliedMessages != 1 || status.SkippedMessages != 1 {
		t.Fatalf("status = %+v, want receive/apply/skip counters", status)
	}
	if status.PublishedAcks != 1 {
		t.Fatalf("status = %+v, want one published ack", status)
	}
	if len(transport.published) == 0 {
		t.Fatalf("expected ack to be published")
	}
	ack := transport.published[len(transport.published)-1]
	if ack.Type != teamcore.TeamSyncTypeAck || ack.Ack == nil || ack.Ack.AckedKey != teamcore.TeamSyncTypeMessage+":"+remoteMsg.MessageID {
		t.Fatalf("unexpected ack payload: %#v", ack)
	}
	if ack.Ack.TargetNode != "node-2" {
		t.Fatalf("unexpected ack target: %#v", ack)
	}
}

func TestTeamPubSubRuntimePublishesAndAppliesTaskAndArtifact(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-team-sync")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(teamRoot) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-team-sync","title":"Team Sync"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := teamcore.OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	transport := &fakeTeamSyncTransport{}
	runtime, err := startTeamPubSubRuntime(root, transport, "node-1")
	if err != nil {
		t.Fatalf("startTeamPubSubRuntime error = %v", err)
	}
	if err := runtime.SyncOnce(context.Background(), nil); err != nil {
		t.Fatalf("SyncOnce(prime) error = %v", err)
	}

	task := teamcore.Task{
		TeamID:    "project-team-sync",
		TaskID:    "task-sync-1",
		ChannelID: "main",
		ContextID: "ctx-task-sync",
		Title:     "runtime replicated task",
		CreatedBy: "agent://remote/task",
		Status:    "doing",
		Priority:  "high",
		CreatedAt: time.Date(2026, 4, 3, 14, 10, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 3, 14, 10, 1, 0, time.UTC),
	}
	if err := store.AppendTask("project-team-sync", task); err != nil {
		t.Fatalf("AppendTask error = %v", err)
	}
	artifact := teamcore.Artifact{
		TeamID:     "project-team-sync",
		ArtifactID: "artifact-sync-1",
		ChannelID:  "main",
		TaskID:     "task-sync-1",
		Title:      "runtime replicated artifact",
		Kind:       "markdown",
		Content:    "artifact content",
		CreatedBy:  "agent://remote/task",
		CreatedAt:  time.Date(2026, 4, 3, 14, 11, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 4, 3, 14, 11, 1, 0, time.UTC),
	}
	if err := store.AppendArtifact("project-team-sync", artifact); err != nil {
		t.Fatalf("AppendArtifact error = %v", err)
	}

	if err := runtime.SyncOnce(context.Background(), nil); err != nil {
		t.Fatalf("SyncOnce(publish objects) error = %v", err)
	}
	if len(transport.published) != 2 {
		t.Fatalf("published = %d, want 2", len(transport.published))
	}
	if transport.published[0].Type != teamcore.TeamSyncTypeTask || transport.published[0].Task == nil || transport.published[0].Task.TaskID != "task-sync-1" {
		t.Fatalf("unexpected task sync payload: %#v", transport.published[0])
	}
	if transport.published[1].Type != teamcore.TeamSyncTypeArtifact || transport.published[1].Artifact == nil || transport.published[1].Artifact.ArtifactID != "artifact-sync-1" {
		t.Fatalf("unexpected artifact sync payload: %#v", transport.published[1])
	}

	inboundRoot := t.TempDir()
	inboundTeamRoot := filepath.Join(inboundRoot, "team", "project-team-sync")
	if err := os.MkdirAll(inboundTeamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(inboundTeamRoot) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(inboundTeamRoot, "team.json"), []byte(`{"team_id":"project-team-sync","title":"Team Sync"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	inboundTransport := &fakeTeamSyncTransport{}
	inboundRuntime, err := startTeamPubSubRuntime(inboundRoot, inboundTransport, "node-2")
	if err != nil {
		t.Fatalf("startTeamPubSubRuntime(inbound) error = %v", err)
	}
	if err := inboundRuntime.SyncOnce(context.Background(), nil); err != nil {
		t.Fatalf("SyncOnce(inbound prime) error = %v", err)
	}
	handler := inboundTransport.handlers["project-team-sync"]
	if handler == nil {
		t.Fatalf("expected inbound team subscription to be installed")
	}
	applied, err := handler(transport.published[0])
	if err != nil {
		t.Fatalf("handler(task) error = %v", err)
	}
	if !applied {
		t.Fatalf("expected inbound replicated task to apply")
	}
	applied, err = handler(transport.published[1])
	if err != nil {
		t.Fatalf("handler(artifact) error = %v", err)
	}
	if !applied {
		t.Fatalf("expected inbound replicated artifact to apply")
	}

	inboundStore, err := teamcore.OpenStore(inboundRoot)
	if err != nil {
		t.Fatalf("OpenStore(inbound) error = %v", err)
	}
	loadedTask, err := inboundStore.LoadTask("project-team-sync", "task-sync-1")
	if err != nil {
		t.Fatalf("LoadTask(inbound) error = %v", err)
	}
	if loadedTask.Status != "doing" {
		t.Fatalf("unexpected inbound replicated task: %#v", loadedTask)
	}
	loadedArtifact, err := inboundStore.LoadArtifact("project-team-sync", "artifact-sync-1")
	if err != nil {
		t.Fatalf("LoadArtifact(inbound) error = %v", err)
	}
	if loadedArtifact.TaskID != "task-sync-1" {
		t.Fatalf("unexpected inbound replicated artifact: %#v", loadedArtifact)
	}
	status := inboundRuntime.Status()
	if status.ReceivedTasks != 1 || status.AppliedTasks != 1 || status.ReceivedArtifacts != 1 || status.AppliedArtifacts != 1 {
		t.Fatalf("status = %+v, want task/artifact receive/apply counters", status)
	}
}

func TestTeamPubSubRuntimePublishesAndAppliesMemberPolicyChannel(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-team-sync")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(teamRoot) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-team-sync","title":"Team Sync"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := teamcore.OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	transport := &fakeTeamSyncTransport{}
	runtime, err := startTeamPubSubRuntime(root, transport, "node-1")
	if err != nil {
		t.Fatalf("startTeamPubSubRuntime error = %v", err)
	}
	if err := runtime.SyncOnce(context.Background(), nil); err != nil {
		t.Fatalf("SyncOnce(prime) error = %v", err)
	}

	memberVersion := runtime.startedAt.Add(time.Minute)
	if err := store.SaveMembers("project-team-sync", []teamcore.Member{
		{AgentID: "agent://pc75/owner", Role: "owner", Status: "active", JoinedAt: memberVersion.Add(-time.Hour), UpdatedAt: memberVersion},
		{AgentID: "agent://pc76/member", Role: "member", Status: "active", JoinedAt: memberVersion.Add(-30 * time.Minute), UpdatedAt: memberVersion},
	}); err != nil {
		t.Fatalf("SaveMembers error = %v", err)
	}
	if err := store.SavePolicy("project-team-sync", teamcore.Policy{
		MessageRoles:    []string{"owner", "member"},
		TaskRoles:       []string{"owner", "maintainer"},
		SystemNoteRoles: []string{"owner"},
		UpdatedAt:       memberVersion.Add(time.Minute),
	}); err != nil {
		t.Fatalf("SavePolicy error = %v", err)
	}
	if err := store.SaveChannel("project-team-sync", teamcore.Channel{
		ChannelID:   "research",
		Title:       "Research",
		Description: "sync channel",
		UpdatedAt:   memberVersion.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("SaveChannel error = %v", err)
	}

	if err := runtime.SyncOnce(context.Background(), nil); err != nil {
		t.Fatalf("SyncOnce(publish object snapshots) error = %v", err)
	}
	if len(transport.published) < 3 {
		t.Fatalf("published = %d, want at least 3", len(transport.published))
	}
	if transport.published[0].Type != teamcore.TeamSyncTypeMember || len(transport.published[0].Members) != 2 {
		t.Fatalf("unexpected member sync payload: %#v", transport.published[0])
	}
	if transport.published[1].Type != teamcore.TeamSyncTypePolicy || transport.published[1].Policy == nil {
		t.Fatalf("unexpected policy sync payload: %#v", transport.published[1])
	}
	channelPayloads := 0
	for _, payload := range transport.published {
		if payload.Type != teamcore.TeamSyncTypeChannel || payload.Channel == nil {
			continue
		}
		channelPayloads++
	}
	if channelPayloads == 0 {
		t.Fatalf("expected at least one channel sync payload, got %#v", transport.published)
	}

	inboundRoot := t.TempDir()
	inboundTeamRoot := filepath.Join(inboundRoot, "team", "project-team-sync")
	if err := os.MkdirAll(inboundTeamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(inboundTeamRoot) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(inboundTeamRoot, "team.json"), []byte(`{"team_id":"project-team-sync","title":"Team Sync"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	inboundTransport := &fakeTeamSyncTransport{}
	inboundRuntime, err := startTeamPubSubRuntime(inboundRoot, inboundTransport, "node-2")
	if err != nil {
		t.Fatalf("startTeamPubSubRuntime(inbound) error = %v", err)
	}
	if err := inboundRuntime.SyncOnce(context.Background(), nil); err != nil {
		t.Fatalf("SyncOnce(inbound prime) error = %v", err)
	}
	handler := inboundTransport.handlers["project-team-sync"]
	if handler == nil {
		t.Fatalf("expected inbound team subscription to be installed")
	}
	for _, payload := range transport.published {
		applied, err := handler(payload)
		if err != nil {
			t.Fatalf("handler(%s) error = %v", payload.Type, err)
		}
		if !applied {
			t.Fatalf("expected inbound %s payload to apply", payload.Type)
		}
	}

	inboundStore, err := teamcore.OpenStore(inboundRoot)
	if err != nil {
		t.Fatalf("OpenStore(inbound) error = %v", err)
	}
	gotMembers, err := inboundStore.LoadMembers("project-team-sync")
	if err != nil {
		t.Fatalf("LoadMembers(inbound) error = %v", err)
	}
	if len(gotMembers) != 2 {
		t.Fatalf("unexpected inbound members: %#v", gotMembers)
	}
	gotPolicy, err := inboundStore.LoadPolicy("project-team-sync")
	if err != nil {
		t.Fatalf("LoadPolicy(inbound) error = %v", err)
	}
	if len(gotPolicy.TaskRoles) != 2 {
		t.Fatalf("unexpected inbound policy: %#v", gotPolicy)
	}
	gotChannel, err := inboundStore.LoadChannel("project-team-sync", "research")
	if err != nil {
		t.Fatalf("LoadChannel(inbound) error = %v", err)
	}
	if gotChannel.Title != "Research" {
		t.Fatalf("unexpected inbound channel: %#v", gotChannel)
	}
	status := inboundRuntime.Status()
	if status.AppliedMembers != 1 || status.AppliedPolicies != 1 || status.AppliedConfigChannels < 1 {
		t.Fatalf("status = %+v, want member/policy and at least one channel snapshot", status)
	}
}

func TestTeamPubSubRuntimeFirstSyncPublishesObjectsCreatedAfterRuntimeStart(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-team-sync")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(teamRoot) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-team-sync","title":"Team Sync"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	transport := &fakeTeamSyncTransport{}
	runtime, err := startTeamPubSubRuntime(root, transport, "node-1")
	if err != nil {
		t.Fatalf("startTeamPubSubRuntime error = %v", err)
	}
	store, err := teamcore.OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	task := teamcore.Task{
		TeamID:    "project-team-sync",
		TaskID:    "task-start-window-1",
		ChannelID: "main",
		ContextID: "ctx-start-window-1",
		Title:     "created before first sync",
		CreatedBy: "agent://remote/task",
		Status:    "open",
		CreatedAt: runtime.startedAt.Add(2 * time.Second),
		UpdatedAt: runtime.startedAt.Add(2 * time.Second),
	}
	if err := store.AppendTask("project-team-sync", task); err != nil {
		t.Fatalf("AppendTask error = %v", err)
	}
	artifact := teamcore.Artifact{
		TeamID:     "project-team-sync",
		ArtifactID: "artifact-start-window-1",
		ChannelID:  "main",
		TaskID:     "task-start-window-1",
		Title:      "created before first sync",
		Kind:       "markdown",
		Content:    "artifact body",
		CreatedBy:  "agent://remote/task",
		CreatedAt:  runtime.startedAt.Add(3 * time.Second),
		UpdatedAt:  runtime.startedAt.Add(3 * time.Second),
	}
	if err := store.AppendArtifact("project-team-sync", artifact); err != nil {
		t.Fatalf("AppendArtifact error = %v", err)
	}
	if err := runtime.SyncOnce(context.Background(), nil); err != nil {
		t.Fatalf("SyncOnce error = %v", err)
	}
	if len(transport.published) != 2 {
		t.Fatalf("published = %d, want 2", len(transport.published))
	}
	if transport.published[0].Type != teamcore.TeamSyncTypeTask || transport.published[1].Type != teamcore.TeamSyncTypeArtifact {
		t.Fatalf("unexpected publish order or payloads: %#v", transport.published)
	}
	status := runtime.Status()
	if status.PublishedTasks != 1 || status.PublishedArtifacts != 1 {
		t.Fatalf("status = %+v, want published task/artifact counters", status)
	}
}

func TestTeamPubSubRuntimePersistsPublishedCursorAcrossRestart(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-team-sync")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(teamRoot) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-team-sync","title":"Team Sync"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := teamcore.OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}

	runtime1, err := startTeamPubSubRuntime(root, &fakeTeamSyncTransport{}, "node-1")
	if err != nil {
		t.Fatalf("startTeamPubSubRuntime(runtime1) error = %v", err)
	}
	runtime1.startedAt = time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)

	firstMsg := signedTeamMessage(t, "project-team-sync", "first persisted", runtime1.startedAt.Add(2*time.Second))
	if err := store.AppendMessage("project-team-sync", firstMsg); err != nil {
		t.Fatalf("AppendMessage(first) error = %v", err)
	}
	firstEvent := teamcore.ChangeEvent{
		TeamID:    "project-team-sync",
		Scope:     "message",
		Action:    "create",
		SubjectID: firstMsg.MessageID,
		Source:    "api",
		CreatedAt: firstMsg.CreatedAt.Add(time.Second),
	}
	if err := store.AppendHistory("project-team-sync", firstEvent); err != nil {
		t.Fatalf("AppendHistory(first) error = %v", err)
	}
	if err := runtime1.SyncOnce(context.Background(), nil); err != nil {
		t.Fatalf("SyncOnce(runtime1) error = %v", err)
	}
	if runtime1.Status().PublishedMessages != 1 || runtime1.Status().PublishedHistory != 1 {
		t.Fatalf("runtime1 status = %+v, want first publish to persist checkpoints", runtime1.Status())
	}

	secondMsgTime := time.Date(2026, 4, 4, 10, 0, 8, 0, time.UTC)
	secondMsg := signedTeamMessage(t, "project-team-sync", "second after restart", secondMsgTime)
	if err := store.AppendMessage("project-team-sync", secondMsg); err != nil {
		t.Fatalf("AppendMessage(second) error = %v", err)
	}
	secondEvent := teamcore.ChangeEvent{
		TeamID:    "project-team-sync",
		Scope:     "message",
		Action:    "create",
		SubjectID: secondMsg.MessageID,
		Source:    "api",
		CreatedAt: secondMsg.CreatedAt.Add(time.Second),
	}
	if err := store.AppendHistory("project-team-sync", secondEvent); err != nil {
		t.Fatalf("AppendHistory(second) error = %v", err)
	}

	transport2 := &fakeTeamSyncTransport{}
	runtime2, err := startTeamPubSubRuntime(root, transport2, "node-1")
	if err != nil {
		t.Fatalf("startTeamPubSubRuntime(runtime2) error = %v", err)
	}
	runtime2.startedAt = time.Date(2026, 4, 4, 10, 0, 20, 0, time.UTC)
	if err := runtime2.SyncOnce(context.Background(), nil); err != nil {
		t.Fatalf("SyncOnce(runtime2) error = %v", err)
	}
	if len(transport2.published) != 2 {
		t.Fatalf("published after restart = %d, want 2", len(transport2.published))
	}
	if transport2.published[0].Type != teamcore.TeamSyncTypeMessage || transport2.published[0].Message == nil || transport2.published[0].Message.MessageID != secondMsg.MessageID {
		t.Fatalf("unexpected first post-restart payload: %#v", transport2.published[0])
	}
	if transport2.published[1].Type != teamcore.TeamSyncTypeHistory || transport2.published[1].History == nil || transport2.published[1].History.SubjectID != secondMsg.MessageID {
		t.Fatalf("unexpected second post-restart payload: %#v", transport2.published[1])
	}
	status := runtime2.Status()
	if !status.StateLoaded || status.PersistedCursors == 0 {
		t.Fatalf("runtime2 status = %+v, want loaded persisted cursors", status)
	}
	if status.PublishedMessages != 1 || status.PublishedHistory != 1 {
		t.Fatalf("runtime2 status = %+v, want restart publish counters", status)
	}
}

func TestTeamPubSubRuntimeRetriesPendingUnackedObjects(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-team-sync")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(teamRoot) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-team-sync","title":"Team Sync"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := teamcore.OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	transport := &fakeTeamSyncTransport{}
	runtime, err := startTeamPubSubRuntime(root, transport, "node-1")
	if err != nil {
		t.Fatalf("startTeamPubSubRuntime error = %v", err)
	}
	runtime.startedAt = time.Date(2026, 4, 4, 13, 0, 0, 0, time.UTC)

	msg := signedTeamMessage(t, "project-team-sync", "retry me", runtime.startedAt.Add(2*time.Second))
	if err := store.AppendMessage("project-team-sync", msg); err != nil {
		t.Fatalf("AppendMessage error = %v", err)
	}
	event := teamcore.ChangeEvent{
		TeamID:    "project-team-sync",
		Scope:     "message",
		Action:    "create",
		SubjectID: msg.MessageID,
		Source:    "api",
		CreatedAt: msg.CreatedAt.Add(time.Second),
	}
	if err := store.AppendHistory("project-team-sync", event); err != nil {
		t.Fatalf("AppendHistory error = %v", err)
	}

	if err := runtime.SyncOnce(context.Background(), nil); err != nil {
		t.Fatalf("SyncOnce(first) error = %v", err)
	}
	if len(transport.published) != 2 {
		t.Fatalf("published first pass = %d, want 2", len(transport.published))
	}
	if runtime.Status().PendingAcks != 2 {
		t.Fatalf("status = %+v, want 2 pending acks after first publish", runtime.Status())
	}

	runtime.mu.Lock()
	for key, checkpoint := range runtime.state.Pending {
		checkpoint.UpdatedAt = time.Now().UTC().Add(-2 * teamSyncAckRetryAfter)
		runtime.state.Pending[key] = checkpoint
	}
	runtime.mu.Unlock()

	if err := runtime.SyncOnce(context.Background(), nil); err != nil {
		t.Fatalf("SyncOnce(retry) error = %v", err)
	}
	if len(transport.published) != 4 {
		t.Fatalf("published second pass = %d, want 4", len(transport.published))
	}
	status := runtime.Status()
	if status.RetriedPublishes != 2 || status.PendingAcks != 2 {
		t.Fatalf("status = %+v, want 2 retries and still-pending acks", status)
	}
}

func TestTeamPubSubRuntimeAppliesInboundAckForTargetNode(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-team-sync")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(teamRoot) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-team-sync","title":"Team Sync"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	transport := &fakeTeamSyncTransport{}
	runtime, err := startTeamPubSubRuntime(root, transport, "node-1")
	if err != nil {
		t.Fatalf("startTeamPubSubRuntime error = %v", err)
	}
	if err := runtime.SyncOnce(context.Background(), nil); err != nil {
		t.Fatalf("SyncOnce(prime) error = %v", err)
	}
	handler := transport.handlers["project-team-sync"]
	if handler == nil {
		t.Fatalf("expected team subscription to be installed")
	}

	runtime.mu.Lock()
	runtime.state.Pending["message:project-team-sync:main:agent://pc75/demo"] = teamSyncPendingState{
		VersionAt:  time.Date(2026, 4, 4, 11, 59, 0, 0, time.UTC),
		Key:        "message:project-team-sync:main:agent://pc75/demo",
		StateKey:   "message:project-team-sync:main:agent://pc75/demo",
		Status:     "pending",
		UpdatedAt:  time.Date(2026, 4, 4, 11, 59, 30, 0, time.UTC),
		RetryCount: 0,
	}
	runtime.status.PendingAcks = 1
	runtime.mu.Unlock()

	applied, err := handler(teamcore.TeamSyncMessage{
		Type:   teamcore.TeamSyncTypeAck,
		TeamID: "project-team-sync",
		Ack: &teamcore.TeamSyncAck{
			AckedKey:   "message:project-team-sync:main:agent://pc75/demo",
			AckedBy:    "node-2",
			TargetNode: "node-1",
			AppliedAt:  time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC),
		},
		SourceNode: "node-2",
		CreatedAt:  time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("handler(ack) error = %v", err)
	}
	if !applied {
		t.Fatalf("expected targeted ack to apply")
	}

	applied, err = handler(teamcore.TeamSyncMessage{
		Type:   teamcore.TeamSyncTypeAck,
		TeamID: "project-team-sync",
		Ack: &teamcore.TeamSyncAck{
			AckedKey:   "message:project-team-sync:main:agent://pc75/demo",
			AckedBy:    "node-2",
			TargetNode: "node-9",
			AppliedAt:  time.Date(2026, 4, 4, 12, 0, 1, 0, time.UTC),
		},
		SourceNode: "node-2",
		CreatedAt:  time.Date(2026, 4, 4, 12, 0, 1, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("handler(other target ack) error = %v", err)
	}
	if applied {
		t.Fatalf("expected foreign-target ack to be skipped")
	}

	status := runtime.Status()
	if status.ReceivedAcks != 2 || status.AppliedAcks != 1 || status.SkippedAcks != 1 {
		t.Fatalf("status = %+v, want ack receive/apply/skip counters", status)
	}
	if status.LastAckedKey != "message:project-team-sync:main:agent://pc75/demo" {
		t.Fatalf("status = %+v, want last acked key recorded", status)
	}
	if status.PendingAcks != 0 {
		t.Fatalf("status = %+v, want pending ack cleared", status)
	}
	if status.PersistedCursors == 0 {
		t.Fatalf("status = %+v, want persisted cursor write", status)
	}
	if status.PersistedPeerAcks != 1 || status.AckPeers != 1 {
		t.Fatalf("status = %+v, want persisted peer ack ledger", status)
	}
	state, err := loadTeamSyncState(runtime.statePath)
	if err != nil {
		t.Fatalf("loadTeamSyncState error = %v", err)
	}
	entry, ok := state.PeerAcks["node-2"]["message:project-team-sync:main:agent://pc75/demo"]
	if !ok {
		t.Fatalf("expected peer ack entry, got %#v", state.PeerAcks)
	}
	if entry.AckedBy != "node-2" || entry.AckedKey != "message:project-team-sync:main:agent://pc75/demo" {
		t.Fatalf("unexpected peer ack entry: %#v", entry)
	}
	pending, ok := state.Pending["message:project-team-sync:main:agent://pc75/demo"]
	if !ok || pending.Status != "acked" {
		t.Fatalf("expected acked pending entry, got %#v", state.Pending)
	}
}

func TestTeamPubSubRuntimeRecordsConflictWhenOlderReplicatedTaskIsSkipped(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-team-sync")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(teamRoot) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-team-sync","title":"Team Sync"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	store, err := teamcore.OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	localTask := teamcore.Task{
		TeamID:    "project-team-sync",
		TaskID:    "task-conflict-1",
		ChannelID: "main",
		ContextID: "ctx-conflict-1",
		Title:     "local newer task",
		CreatedBy: "agent://pc75/local",
		Status:    "doing",
		Priority:  "high",
		CreatedAt: time.Date(2026, 4, 4, 14, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 4, 14, 5, 0, 0, time.UTC),
	}
	if err := store.AppendTask("project-team-sync", localTask); err != nil {
		t.Fatalf("AppendTask(local) error = %v", err)
	}
	transport := &fakeTeamSyncTransport{}
	runtime, err := startTeamPubSubRuntime(root, transport, "node-1")
	if err != nil {
		t.Fatalf("startTeamPubSubRuntime error = %v", err)
	}
	if err := runtime.SyncOnce(context.Background(), nil); err != nil {
		t.Fatalf("SyncOnce(prime) error = %v", err)
	}
	handler := transport.handlers["project-team-sync"]
	if handler == nil {
		t.Fatalf("expected team subscription to be installed")
	}

	remoteOlder := localTask
	remoteOlder.Title = "remote older task"
	remoteOlder.Status = "review"
	remoteOlder.UpdatedAt = time.Date(2026, 4, 4, 14, 1, 0, 0, time.UTC)

	applied, err := handler(teamcore.TeamSyncMessage{
		Type:       teamcore.TeamSyncTypeTask,
		TeamID:     "project-team-sync",
		Task:       &remoteOlder,
		SourceNode: "node-2",
		CreatedAt:  remoteOlder.UpdatedAt,
	})
	if err != nil {
		t.Fatalf("handler(task conflict) error = %v", err)
	}
	if applied {
		t.Fatalf("expected older replicated task to be skipped")
	}

	status := runtime.Status()
	if status.SkippedTasks != 1 || status.Conflicts != 1 {
		t.Fatalf("status = %+v, want one skipped task conflict", status)
	}
	if status.LastConflictReason != "local_newer" {
		t.Fatalf("status = %+v, want conflict reason recorded", status)
	}
	state, err := loadTeamSyncState(runtime.statePath)
	if err != nil {
		t.Fatalf("loadTeamSyncState error = %v", err)
	}
	conflict, ok := state.Conflicts["task:task-conflict-1:"+remoteOlder.UpdatedAt.Format(time.RFC3339Nano)]
	if !ok {
		t.Fatalf("expected conflict entry, got %#v", state.Conflicts)
	}
	if conflict.SubjectID != "task-conflict-1" || conflict.SourceNode != "node-2" {
		t.Fatalf("unexpected conflict entry: %#v", conflict)
	}
	if conflict.Reason != "local_newer" {
		t.Fatalf("unexpected conflict reason: %#v", conflict)
	}
}

func TestTeamPubSubRuntimeCompactsPeerAcksAndSupersedesPending(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	teamRoot := filepath.Join(root, "team", "project-team-sync")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(teamRoot) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(teamRoot, "team.json"), []byte(`{"team_id":"project-team-sync","title":"Team Sync"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(team.json) error = %v", err)
	}
	runtime, err := startTeamPubSubRuntime(root, &fakeTeamSyncTransport{}, "node-1")
	if err != nil {
		t.Fatalf("startTeamPubSubRuntime error = %v", err)
	}

	runtime.mu.Lock()
	runtime.state.PeerAcks["node-2"] = map[string]teamSyncPeerAck{
		"old-entry": {
			AckedKey:  "old-entry",
			AckedBy:   "node-2",
			AppliedAt: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Now().UTC().Add(-teamSyncPeerAckTTL - time.Hour),
		},
	}
	for i := 0; i < teamSyncPeerAckPerPeer+2; i++ {
		key := "recent-" + strconv.Itoa(i)
		runtime.state.PeerAcks["node-2"][key] = teamSyncPeerAck{
			AckedKey:  key,
			AckedBy:   "node-2",
			AppliedAt: time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Second),
			UpdatedAt: time.Now().UTC().Add(-time.Duration(i) * time.Minute),
		}
	}
	runtime.mu.Unlock()

	if err := runtime.persistPeerAck(teamcore.TeamSyncMessage{
		Type:   teamcore.TeamSyncTypeAck,
		TeamID: "project-team-sync",
		Ack: &teamcore.TeamSyncAck{
			AckedKey:   "fresh-entry",
			AckedBy:    "node-2",
			TargetNode: "node-1",
			AppliedAt:  time.Now().UTC(),
		},
		SourceNode: "node-2",
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("persistPeerAck error = %v", err)
	}

	state, err := loadTeamSyncState(runtime.statePath)
	if err != nil {
		t.Fatalf("loadTeamSyncState(after ack) error = %v", err)
	}
	if len(state.PeerAcks["node-2"]) > teamSyncPeerAckPerPeer {
		t.Fatalf("peer ack ledger too large: %d", len(state.PeerAcks["node-2"]))
	}
	if _, ok := state.PeerAcks["node-2"]["old-entry"]; ok {
		t.Fatalf("expected old peer ack entry pruned, got %#v", state.PeerAcks["node-2"])
	}

	firstSync := teamcore.TeamSyncMessage{
		Type:       teamcore.TeamSyncTypeTask,
		TeamID:     "project-team-sync",
		SourceNode: "node-1",
		CreatedAt:  time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC),
		Task: &teamcore.Task{
			TeamID:    "project-team-sync",
			TaskID:    "task-sync-1",
			Title:     "first",
			Status:    "open",
			Priority:  "medium",
			CreatedBy: "agent://pc75/demo",
			CreatedAt: time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 4, 4, 12, 1, 0, 0, time.UTC),
		},
	}
	secondSync := firstSync
	secondSync.Task = &teamcore.Task{
		TeamID:    "project-team-sync",
		TaskID:    "task-sync-1",
		Title:     "second",
		Status:    "doing",
		Priority:  "high",
		CreatedBy: "agent://pc75/demo",
		CreatedAt: time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 4, 12, 2, 0, 0, time.UTC),
	}

	if err := runtime.persistPending(firstSync, firstSync.Task.UpdatedAt); err != nil {
		t.Fatalf("persistPending(first) error = %v", err)
	}
	if err := runtime.persistPending(secondSync, secondSync.Task.UpdatedAt); err != nil {
		t.Fatalf("persistPending(second) error = %v", err)
	}
	state, err = loadTeamSyncState(runtime.statePath)
	if err != nil {
		t.Fatalf("loadTeamSyncState(after pending) error = %v", err)
	}
	firstPending := state.Pending[firstSync.Key()]
	secondPending := state.Pending[secondSync.Key()]
	if firstPending.Status != "superseded" {
		t.Fatalf("expected first pending to be superseded, got %#v", firstPending)
	}
	if secondPending.Status != "pending" {
		t.Fatalf("expected second pending to remain pending, got %#v", secondPending)
	}
}

func TestWriteTeamSyncStatePreservesNewerResolvedConflict(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sync", "team_sync_state.json")
	base := newTeamSyncPersistedState()
	base.Conflicts["message:conflict-1"] = teamSyncConflict{
		Key:       "message:conflict-1",
		Type:      "message",
		TeamID:    "project-team-sync",
		SubjectID: "conflict-1",
		Reason:    "signature_rejected",
		UpdatedAt: time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC),
	}
	if err := writeTeamSyncState(path, base); err != nil {
		t.Fatalf("writeTeamSyncState(base) error = %v", err)
	}

	resolved := newTeamSyncPersistedState()
	resolved.Conflicts["message:conflict-1"] = teamSyncConflict{
		Key:        "message:conflict-1",
		Type:       "message",
		TeamID:     "project-team-sync",
		SubjectID:  "conflict-1",
		Reason:     "signature_rejected",
		Resolution: "dismiss",
		ResolvedBy: "agent://pc75/openclaw01",
		ResolvedAt: time.Date(2026, 4, 4, 12, 5, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 4, 4, 12, 5, 0, 0, time.UTC),
	}
	if err := writeTeamSyncState(path, resolved); err != nil {
		t.Fatalf("writeTeamSyncState(resolved) error = %v", err)
	}

	stale := newTeamSyncPersistedState()
	stale.Conflicts["message:conflict-1"] = teamSyncConflict{
		Key:       "message:conflict-1",
		Type:      "message",
		TeamID:    "project-team-sync",
		SubjectID: "conflict-1",
		Reason:    "signature_rejected",
		UpdatedAt: time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC),
	}
	if err := writeTeamSyncState(path, stale); err != nil {
		t.Fatalf("writeTeamSyncState(stale) error = %v", err)
	}

	state, err := loadTeamSyncState(path)
	if err != nil {
		t.Fatalf("loadTeamSyncState error = %v", err)
	}
	conflict := state.Conflicts["message:conflict-1"]
	if conflict.Resolution != "dismiss" || conflict.ResolvedBy != "agent://pc75/openclaw01" {
		t.Fatalf("expected resolved conflict preserved, got %#v", conflict)
	}
}

func signedTeamMessage(t *testing.T, teamID, content string, createdAt time.Time) teamcore.Message {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey error = %v", err)
	}
	msg := teamcore.Message{
		TeamID:          teamID,
		ChannelID:       "main",
		ContextID:       "ctx-" + content,
		AuthorAgentID:   "agent://remote/alpha",
		OriginPublicKey: hex.EncodeToString(publicKey),
		MessageType:     "note",
		Content:         content,
		CreatedAt:       createdAt,
	}
	payload, err := teamcore.MessageSignaturePayload(msg)
	if err != nil {
		t.Fatalf("MessageSignaturePayload error = %v", err)
	}
	msg.Signature = hex.EncodeToString(ed25519.Sign(privateKey, payload))
	msg.MessageID = strings.Join([]string{
		msg.TeamID,
		msg.ChannelID,
		msg.AuthorAgentID,
		msg.CreatedAt.UTC().Format(time.RFC3339Nano),
		msg.Content,
	}, ":")
	return msg
}
