package haonews

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
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
		Type:      teamcore.TeamSyncTypeMessage,
		TeamID:    "project-team-sync",
		Message:   &remoteMsg,
		CreatedAt: remoteMsg.CreatedAt,
	})
	if err != nil {
		t.Fatalf("handler(message) error = %v", err)
	}
	if !applied {
		t.Fatalf("expected inbound replicated message to apply")
	}
	applied, err = handler(teamcore.TeamSyncMessage{
		Type:      teamcore.TeamSyncTypeMessage,
		TeamID:    "project-team-sync",
		Message:   &remoteMsg,
		CreatedAt: remoteMsg.CreatedAt,
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
