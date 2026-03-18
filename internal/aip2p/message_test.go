package aip2p

import (
	"testing"
	"time"
)

func TestBuildAndLoadMessage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	msg, body, err := BuildMessage(MessageInput{
		Kind:    "post",
		Author:  "agent://demo/alice",
		Channel: "general",
		Title:   "hello",
		Body:    "hello world",
		Tags:    []string{"demo", "demo", "test"},
		Extensions: map[string]any{
			"project": "latest.org",
		},
		CreatedAt: time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildMessage error = %v", err)
	}
	if err := WriteMessage(dir, msg, body); err != nil {
		t.Fatalf("WriteMessage error = %v", err)
	}

	gotMsg, gotBody, err := LoadMessage(dir)
	if err != nil {
		t.Fatalf("LoadMessage error = %v", err)
	}

	if gotBody != "hello world" {
		t.Fatalf("body = %q, want %q", gotBody, "hello world")
	}
	if gotMsg.Protocol != ProtocolVersion {
		t.Fatalf("protocol = %q, want %q", gotMsg.Protocol, ProtocolVersion)
	}
	if len(gotMsg.Tags) != 2 {
		t.Fatalf("tags len = %d, want 2", len(gotMsg.Tags))
	}
	if gotMsg.Extensions["project"] != "latest.org" {
		t.Fatalf("extensions project = %v, want latest.org", gotMsg.Extensions["project"])
	}
}

func TestBuildAndLoadSignedMessage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	identity, err := NewAgentIdentity("agent://news/world-01", "agent://demo/alice", time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewAgentIdentity error = %v", err)
	}
	msg, body, err := BuildMessage(MessageInput{
		Kind:     "post",
		Author:   "agent://demo/alice",
		Channel:  "aip2p.public/world",
		Title:    "signed hello",
		Body:     "signed body",
		Identity: &identity,
		Extensions: map[string]any{
			"project": "aip2p.public",
			"topics":  []string{"all", "world"},
		},
		CreatedAt: time.Date(2026, 3, 16, 12, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildMessage error = %v", err)
	}
	if msg.Origin == nil {
		t.Fatal("expected signed origin")
	}
	if msg.Origin.AgentID != "agent://news/world-01" {
		t.Fatalf("origin.agent_id = %q", msg.Origin.AgentID)
	}
	if err := WriteMessage(dir, msg, body); err != nil {
		t.Fatalf("WriteMessage error = %v", err)
	}
	if _, _, err := LoadMessage(dir); err != nil {
		t.Fatalf("LoadMessage error = %v", err)
	}
}

func TestValidateMessageRejectsOriginAuthorMismatch(t *testing.T) {
	t.Parallel()

	identity, err := NewAgentIdentity("agent://news/world-01", "agent://demo/alice", time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewAgentIdentity error = %v", err)
	}
	msg, body, err := BuildMessage(MessageInput{
		Kind:     "post",
		Author:   "agent://demo/alice",
		Channel:  "aip2p.public/world",
		Title:    "signed hello",
		Body:     "signed body",
		Identity: &identity,
		Extensions: map[string]any{
			"project": "aip2p.public",
		},
		CreatedAt: time.Date(2026, 3, 16, 12, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildMessage error = %v", err)
	}
	msg.Origin.Author = "agent://demo/bob"
	if err := ValidateMessage(msg, body); err == nil {
		t.Fatal("expected origin validation error")
	}
}
