package haonews

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
		Channel:  "hao.news/world",
		Title:    "signed hello",
		Body:     "signed body",
		Identity: &identity,
		Extensions: map[string]any{
			"project": "hao.news",
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
	if msg.Extensions["origin_public_key"] != identity.PublicKey {
		t.Fatalf("origin_public_key = %#v", msg.Extensions["origin_public_key"])
	}
	if msg.Extensions["parent_public_key"] != identity.PublicKey {
		t.Fatalf("parent_public_key = %#v", msg.Extensions["parent_public_key"])
	}
	if err := WriteMessage(dir, msg, body); err != nil {
		t.Fatalf("WriteMessage error = %v", err)
	}
	if _, _, err := LoadMessage(dir); err != nil {
		t.Fatalf("LoadMessage error = %v", err)
	}
}

func TestLoadMessageRejectsBodyFileTraversal(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	msg, body, err := BuildMessage(MessageInput{
		Kind:      "post",
		Author:    "agent://demo/alice",
		Channel:   "general",
		Title:     "hello",
		Body:      "hello world",
		CreatedAt: time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildMessage error = %v", err)
	}
	msg.BodyFile = "../outside.txt"
	data, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent error = %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(dir, MessageFileName), data, 0o644); err != nil {
		t.Fatalf("WriteMessage metadata error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, BodyFileName), body, 0o644); err != nil {
		t.Fatalf("WriteMessage body error = %v", err)
	}

	if _, _, err := LoadMessage(dir); err == nil || !strings.Contains(err.Error(), "invalid body_file") {
		t.Fatalf("LoadMessage error = %v, want invalid body_file", err)
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
		Channel:  "hao.news/world",
		Title:    "signed hello",
		Body:     "signed body",
		Identity: &identity,
		Extensions: map[string]any{
			"project": "hao.news",
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

func TestBuildAndLoadHDChildSignedMessage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	identity, err := RecoverHDIdentity(
		"agent://news/world-01",
		"agent://alice",
		"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
		time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("RecoverHDIdentity error = %v", err)
	}
	msg, body, err := BuildMessage(MessageInput{
		Kind:     "post",
		Author:   "agent://alice/work",
		Channel:  "hao.news/world",
		Title:    "hd hello",
		Body:     "signed body",
		Identity: &identity,
		Extensions: map[string]any{
			"project": "hao.news",
		},
		CreatedAt: time.Date(2026, 3, 18, 12, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildMessage error = %v", err)
	}
	if msg.Origin == nil {
		t.Fatal("expected signed origin")
	}
	if msg.Origin.PublicKey == identity.PublicKey {
		t.Fatal("expected child public key to differ from root key")
	}
	if msg.Extensions["hd.parent"] != "agent://alice" {
		t.Fatalf("hd.parent = %#v", msg.Extensions["hd.parent"])
	}
	if msg.Extensions["hd.parent_pubkey"] != identity.PublicKey {
		t.Fatalf("hd.parent_pubkey = %#v", msg.Extensions["hd.parent_pubkey"])
	}
	if msg.Extensions["origin_public_key"] == identity.PublicKey {
		t.Fatal("expected child origin_public_key to differ from root key")
	}
	if msg.Extensions["parent_public_key"] != identity.PublicKey {
		t.Fatalf("parent_public_key = %#v", msg.Extensions["parent_public_key"])
	}
	if _, ok := msg.Extensions["hd.path"].(string); !ok {
		t.Fatalf("hd.path = %#v", msg.Extensions["hd.path"])
	}
	if err := WriteMessage(dir, msg, body); err != nil {
		t.Fatalf("WriteMessage error = %v", err)
	}
	if _, _, err := LoadMessage(dir); err != nil {
		t.Fatalf("LoadMessage error = %v", err)
	}
}

func TestBuildAndLoadDerivedChildSigningIdentityMessage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rootIdentity, err := RecoverHDIdentity(
		"agent://news/world-01",
		"agent://alice",
		"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
		time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("RecoverHDIdentity error = %v", err)
	}
	childIdentity, err := DeriveChildIdentity(rootIdentity, "agent://alice/work", time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DeriveChildIdentity error = %v", err)
	}
	if childIdentity.PrivateKey == "" {
		t.Fatal("expected derived child signing identity to include private key")
	}
	if childIdentity.Mnemonic != "" {
		t.Fatal("expected derived child signing identity to omit mnemonic")
	}
	msg, body, err := BuildMessage(MessageInput{
		Kind:     "post",
		Author:   "agent://alice/work",
		Channel:  "hao.news/world",
		Title:    "hd child hello",
		Body:     "signed body",
		Identity: &childIdentity,
		Extensions: map[string]any{
			"project": "hao.news",
		},
		CreatedAt: time.Date(2026, 3, 18, 12, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildMessage error = %v", err)
	}
	if msg.Origin == nil {
		t.Fatal("expected signed origin")
	}
	if msg.Origin.PublicKey != childIdentity.PublicKey {
		t.Fatalf("origin.public_key = %q, want %q", msg.Origin.PublicKey, childIdentity.PublicKey)
	}
	if msg.Extensions["origin_public_key"] != childIdentity.PublicKey {
		t.Fatalf("origin_public_key = %#v", msg.Extensions["origin_public_key"])
	}
	if msg.Extensions["parent_public_key"] != rootIdentity.PublicKey {
		t.Fatalf("parent_public_key = %#v", msg.Extensions["parent_public_key"])
	}
	if msg.Extensions["hd.parent"] != "agent://alice" {
		t.Fatalf("hd.parent = %#v", msg.Extensions["hd.parent"])
	}
	if err := WriteMessage(dir, msg, body); err != nil {
		t.Fatalf("WriteMessage error = %v", err)
	}
	if _, _, err := LoadMessage(dir); err != nil {
		t.Fatalf("LoadMessage error = %v", err)
	}
}

func TestValidateMessageRejectsTamperedHDParentPubKey(t *testing.T) {
	t.Parallel()

	rootIdentity, err := RecoverHDIdentity(
		"agent://news/world-01",
		"agent://alice",
		"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
		time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("RecoverHDIdentity error = %v", err)
	}
	childIdentity, err := DeriveChildIdentity(rootIdentity, "agent://alice/work", time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DeriveChildIdentity error = %v", err)
	}
	msg, body, err := BuildMessage(MessageInput{
		Kind:     "post",
		Author:   "agent://alice/work",
		Channel:  "hao.news/world",
		Title:    "hd child hello",
		Body:     "signed body",
		Identity: &childIdentity,
		Extensions: map[string]any{
			"project": "hao.news",
		},
		CreatedAt: time.Date(2026, 3, 18, 12, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildMessage error = %v", err)
	}
	msg.Extensions["hd.parent_pubkey"] = strings.Repeat("0", 64)
	if err := ValidateMessage(msg, body); err == nil {
		t.Fatal("expected validation error after tampering hd.parent_pubkey")
	}
}

func TestValidateMessageRejectsMissingOriginPublicKeyMetadata(t *testing.T) {
	t.Parallel()

	identity, err := NewAgentIdentity("agent://news/world-01", "agent://demo/alice", time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewAgentIdentity error = %v", err)
	}
	msg, body, err := BuildMessage(MessageInput{
		Kind:     "post",
		Author:   "agent://demo/alice",
		Channel:  "hao.news/world",
		Title:    "signed hello",
		Body:     "signed body",
		Identity: &identity,
		Extensions: map[string]any{
			"project": "hao.news",
		},
		CreatedAt: time.Date(2026, 3, 16, 12, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildMessage error = %v", err)
	}
	delete(msg.Extensions, "origin_public_key")
	if err := ValidateMessage(msg, body); err == nil {
		t.Fatal("expected validation error after removing origin_public_key")
	}
}

func TestValidateMessageAcceptsLegacySignedMessageWithoutKeyMetadata(t *testing.T) {
	t.Parallel()

	identity, err := NewAgentIdentity("agent://news/world-01", "agent://demo/alice", time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewAgentIdentity error = %v", err)
	}
	msg, body, err := BuildMessage(MessageInput{
		Kind:     "post",
		Author:   "agent://demo/alice",
		Channel:  "hao.news/world",
		Title:    "legacy signed hello",
		Body:     "signed body",
		Identity: &identity,
		Extensions: map[string]any{
			"project": "hao.news",
		},
		CreatedAt: time.Date(2026, 3, 16, 12, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildMessage error = %v", err)
	}
	delete(msg.Extensions, "origin_public_key")
	delete(msg.Extensions, "parent_public_key")
	payload, err := signedMessagePayloadBytes(msg, *msg.Origin)
	if err != nil {
		t.Fatalf("signedMessagePayloadBytes error = %v", err)
	}
	privateKeyBytes, err := decodeHexKey(identity.PrivateKey, ed25519.PrivateKeySize, "private_key")
	if err != nil {
		t.Fatalf("decodeHexKey error = %v", err)
	}
	msg.Origin.Signature = hex.EncodeToString(ed25519.Sign(ed25519.PrivateKey(privateKeyBytes), payload))
	if err := ValidateMessage(msg, body); err != nil {
		t.Fatalf("ValidateMessage legacy signed message error = %v", err)
	}
}

func TestValidateMessageRejectsTamperedHDPath(t *testing.T) {
	t.Parallel()

	rootIdentity, err := RecoverHDIdentity(
		"agent://news/world-01",
		"agent://alice",
		"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
		time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("RecoverHDIdentity error = %v", err)
	}
	childIdentity, err := DeriveChildIdentity(rootIdentity, "agent://alice/work", time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DeriveChildIdentity error = %v", err)
	}
	msg, body, err := BuildMessage(MessageInput{
		Kind:     "post",
		Author:   "agent://alice/work",
		Channel:  "hao.news/world",
		Title:    "hd child hello",
		Body:     "signed body",
		Identity: &childIdentity,
		Extensions: map[string]any{
			"project": "hao.news",
		},
		CreatedAt: time.Date(2026, 3, 18, 12, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildMessage error = %v", err)
	}
	msg.Extensions["hd.path"] = "m/0'/999'"
	if err := ValidateMessage(msg, body); err == nil {
		t.Fatal("expected validation error after tampering hd.path")
	}
}
