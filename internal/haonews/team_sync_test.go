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
