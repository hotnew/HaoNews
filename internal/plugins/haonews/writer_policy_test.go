package newsplugin

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestApplyWriterPolicyOnlyKeepsReadWriteOrigins(t *testing.T) {
	t.Parallel()

	postAllowed := Bundle{
		InfoHash: "post-allowed",
		Message: Message{
			Kind:   "post",
			Title:  "Allowed",
			Author: "agent://writer/allowed",
			Origin: &MessageOrigin{
				AgentID:   "agent://writer/allowed",
				PublicKey: "aaaa",
			},
			Extensions: map[string]any{"project": "hao.news", "topics": []any{"all", "world"}},
		},
	}
	postReadOnly := Bundle{
		InfoHash: "post-readonly",
		Message: Message{
			Kind:   "post",
			Title:  "Read only",
			Author: "agent://writer/readonly",
			Origin: &MessageOrigin{
				AgentID:   "agent://writer/readonly",
				PublicKey: "bbbb",
			},
			Extensions: map[string]any{"project": "hao.news", "topics": []any{"all", "world"}},
		},
	}
	replyReadOnly := Bundle{
		InfoHash: "reply-readonly",
		Message: Message{
			Kind:   "reply",
			Author: "agent://writer/readonly",
			ReplyTo: &MessageLink{
				InfoHash: "post-allowed",
			},
			Origin: &MessageOrigin{
				AgentID:   "agent://writer/readonly",
				PublicKey: "bbbb",
			},
			Extensions: map[string]any{"project": "hao.news", "topics": []any{"all", "world"}},
		},
	}

	index := buildIndex([]Bundle{postAllowed, postReadOnly, replyReadOnly}, "hao.news")
	policy := WriterPolicy{
		SyncMode:          WriterSyncModeMixed,
		DefaultCapability: WriterCapabilityReadOnly,
		PublicKeyCapabilities: map[string]WriterCapability{
			"aaaa": WriterCapabilityReadWrite,
		},
	}

	filtered := ApplyWriterPolicy(index, "hao.news", policy)
	if len(filtered.Posts) != 1 {
		t.Fatalf("posts len = %d, want 1", len(filtered.Posts))
	}
	if filtered.Posts[0].InfoHash != "post-allowed" {
		t.Fatalf("post = %q, want post-allowed", filtered.Posts[0].InfoHash)
	}
	if got := len(filtered.RepliesByPost["post-allowed"]); got != 0 {
		t.Fatalf("reply count = %d, want 0", got)
	}
}

func TestWriterPolicyCapabilityPrefersExplicitMap(t *testing.T) {
	t.Parallel()

	policy := WriterPolicy{
		SyncMode:          WriterSyncModeMixed,
		AllowUnsigned:     false,
		DefaultCapability: WriterCapabilityReadOnly,
		PublicKeyCapabilities: map[string]WriterCapability{
			"allowed-key": WriterCapabilityReadWrite,
		},
	}
	allowed := &MessageOrigin{AgentID: "agent://writer/allowed", PublicKey: "allowed-key"}
	denied := &MessageOrigin{AgentID: "agent://writer/other", PublicKey: "other-key"}

	if !policy.allowsOrigin(allowed) {
		t.Fatal("expected explicit read_write writer to be accepted")
	}
	if policy.capabilityForOrigin(denied) != WriterCapabilityReadOnly {
		t.Fatalf("denied capability = %q, want read_only", policy.capabilityForOrigin(denied))
	}
	if policy.acceptsOrigin(denied) {
		t.Fatal("expected read_only writer to be rejected in mixed mode")
	}
}

func TestApplyWriterPolicyWhitelistAcceptsOnlyExplicitWriters(t *testing.T) {
	t.Parallel()

	postAllowed := Bundle{
		InfoHash: "post-allowed",
		Message: Message{
			Kind:   "post",
			Title:  "Allowed",
			Author: "agent://writer/allowed",
			Origin: &MessageOrigin{
				AgentID:   "agent://writer/allowed",
				PublicKey: "aaaa",
			},
			Extensions: map[string]any{"project": "hao.news", "topics": []any{"all", "world"}},
		},
	}
	postOther := Bundle{
		InfoHash: "post-other",
		Message: Message{
			Kind:   "post",
			Title:  "Other",
			Author: "agent://writer/other",
			Origin: &MessageOrigin{
				AgentID:   "agent://writer/other",
				PublicKey: "bbbb",
			},
			Extensions: map[string]any{"project": "hao.news", "topics": []any{"all", "world"}},
		},
	}

	index := buildIndex([]Bundle{postAllowed, postOther}, "hao.news")
	policy := WriterPolicy{
		SyncMode:          WriterSyncModeWhitelist,
		DefaultCapability: WriterCapabilityReadWrite,
		AllowedAgentIDs:   []string{"agent://writer/allowed"},
	}

	filtered := ApplyWriterPolicy(index, "hao.news", policy)
	if len(filtered.Posts) != 1 {
		t.Fatalf("posts len = %d, want 1", len(filtered.Posts))
	}
	if filtered.Posts[0].InfoHash != "post-allowed" {
		t.Fatalf("post = %q, want post-allowed", filtered.Posts[0].InfoHash)
	}
}

func TestApplyWriterPolicyParentAndChildrenTrustsChildAuthorWhenRootWhitelisted(t *testing.T) {
	t.Parallel()

	postChild := Bundle{
		InfoHash: "post-child",
		Message: Message{
			Kind:   "post",
			Title:  "Child",
			Author: "agent://alice/work",
			Origin: &MessageOrigin{
				AgentID:   "agent://news/root-01",
				PublicKey: "aaaa",
			},
			Extensions: map[string]any{
				"project":          "hao.news",
				"hd.parent":        "agent://alice",
				"hd.parent_pubkey": "root-key",
				"hd.path":          "m/0'/1'",
			},
		},
	}
	postOther := Bundle{
		InfoHash: "post-other",
		Message: Message{
			Kind:   "post",
			Title:  "Other",
			Author: "agent://bob/work",
			Origin: &MessageOrigin{
				AgentID:   "agent://news/root-02",
				PublicKey: "bbbb",
			},
			Extensions: map[string]any{"project": "hao.news"},
		},
	}

	index := buildIndex([]Bundle{postChild, postOther}, "hao.news")
	policy := WriterPolicy{
		SyncMode:          WriterSyncModeWhitelist,
		TrustMode:         WriterTrustModeParentAndChildren,
		DefaultCapability: WriterCapabilityReadWrite,
		AllowedAgentIDs:   []string{"agent://alice"},
	}

	filtered := ApplyWriterPolicy(index, "hao.news", policy)
	if len(filtered.Posts) != 1 {
		t.Fatalf("posts len = %d, want 1", len(filtered.Posts))
	}
	if filtered.Posts[0].InfoHash != "post-child" {
		t.Fatalf("post = %q, want post-child", filtered.Posts[0].InfoHash)
	}
}

func TestApplyWriterPolicyBlacklistOverridesParentAndChildrenTrust(t *testing.T) {
	t.Parallel()

	postChild := Bundle{
		InfoHash: "post-child",
		Message: Message{
			Kind:   "post",
			Title:  "Child",
			Author: "agent://alice/spam-bot",
			Origin: &MessageOrigin{
				AgentID:   "agent://news/root-01",
				PublicKey: "aaaa",
			},
			Extensions: map[string]any{"project": "hao.news"},
		},
	}

	index := buildIndex([]Bundle{postChild}, "hao.news")
	policy := WriterPolicy{
		SyncMode:          WriterSyncModeWhitelist,
		TrustMode:         WriterTrustModeParentAndChildren,
		DefaultCapability: WriterCapabilityReadWrite,
		AllowedAgentIDs:   []string{"agent://alice"},
		BlockedAgentIDs:   []string{"agent://alice/spam-bot"},
	}

	filtered := ApplyWriterPolicy(index, "hao.news", policy)
	if len(filtered.Posts) != 0 {
		t.Fatalf("posts len = %d, want 0", len(filtered.Posts))
	}
}

func TestApplyWriterPolicyAllModeKeepsSignedReadOnlyWritersUnlessBlocked(t *testing.T) {
	t.Parallel()

	postReadOnly := Bundle{
		InfoHash: "post-readonly",
		Message: Message{
			Kind:   "post",
			Title:  "Read only",
			Author: "agent://writer/readonly",
			Origin: &MessageOrigin{
				AgentID:   "agent://writer/readonly",
				PublicKey: "aaaa",
			},
			Extensions: map[string]any{"project": "hao.news", "topics": []any{"all", "world"}},
		},
	}
	postBlocked := Bundle{
		InfoHash: "post-blocked",
		Message: Message{
			Kind:   "post",
			Title:  "Blocked",
			Author: "agent://writer/blocked",
			Origin: &MessageOrigin{
				AgentID:   "agent://writer/blocked",
				PublicKey: "bbbb",
			},
			Extensions: map[string]any{"project": "hao.news", "topics": []any{"all", "world"}},
		},
	}

	index := buildIndex([]Bundle{postReadOnly, postBlocked}, "hao.news")
	policy := WriterPolicy{
		SyncMode:          WriterSyncModeAll,
		DefaultCapability: WriterCapabilityReadOnly,
		BlockedPublicKeys: []string{"bbbb"},
	}

	filtered := ApplyWriterPolicy(index, "hao.news", policy)
	if len(filtered.Posts) != 1 {
		t.Fatalf("posts len = %d, want 1", len(filtered.Posts))
	}
	if filtered.Posts[0].InfoHash != "post-readonly" {
		t.Fatalf("post = %q, want post-readonly", filtered.Posts[0].InfoHash)
	}
}

func TestApplyWriterPolicyAllModeRejectsUnsignedByDefault(t *testing.T) {
	t.Parallel()

	unsignedPost := Bundle{
		InfoHash: "post-unsigned",
		Message: Message{
			Kind:   "post",
			Title:  "Unsigned",
			Author: "agent://writer/unsigned",
			Extensions: map[string]any{
				"project": "hao.news",
				"topics":  []any{"all", "world"},
			},
		},
	}
	noKeyPost := Bundle{
		InfoHash: "post-no-key",
		Message: Message{
			Kind:   "post",
			Title:  "No key",
			Author: "agent://writer/no-key",
			Origin: &MessageOrigin{
				AgentID: "agent://writer/no-key",
			},
			Extensions: map[string]any{
				"project": "hao.news",
				"topics":  []any{"all", "world"},
			},
		},
	}

	index := buildIndex([]Bundle{unsignedPost, noKeyPost}, "hao.news")
	policy := WriterPolicy{
		SyncMode:          WriterSyncModeAll,
		AllowUnsigned:     false,
		DefaultCapability: WriterCapabilityReadWrite,
	}

	filtered := ApplyWriterPolicy(index, "hao.news", policy)
	if len(filtered.Posts) != 0 {
		t.Fatalf("posts len = %d, want 0", len(filtered.Posts))
	}
}

func TestWriterPolicyOriginWithoutPublicKeyCountsAsUnsigned(t *testing.T) {
	t.Parallel()

	policy := WriterPolicy{
		SyncMode:          WriterSyncModeAll,
		AllowUnsigned:     false,
		DefaultCapability: WriterCapabilityReadWrite,
		AgentCapabilities: map[string]WriterCapability{
			"agent://writer/legacy": WriterCapabilityReadWrite,
		},
	}
	legacy := &MessageOrigin{AgentID: "agent://writer/legacy"}

	if policy.capabilityForOrigin(legacy) != WriterCapabilityBlocked {
		t.Fatalf("capability = %q, want blocked", policy.capabilityForOrigin(legacy))
	}
	if policy.acceptsOrigin(legacy) {
		t.Fatal("expected origin without public key to be treated as unsigned and rejected")
	}
}

func TestLoadWriterPolicyMergesSharedRegistry(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey error = %v", err)
	}
	registry := SignedWriterRegistry{
		AuthorityID: "authority://news/main",
		KeyType:     latestAppKeyTypeEd25519,
		PublicKey:   hex.EncodeToString(publicKey),
		SignedAt:    "2026-03-15T00:00:00Z",
		PublicKeyCapabilities: map[string]WriterCapability{
			"shared-key": WriterCapabilityReadWrite,
		},
		RelayHostTrust: map[string]RelayTrust{
			"mirror.example": RelayTrustBlocked,
		},
	}
	registry.Normalize()
	payload, err := registry.payloadBytes()
	if err != nil {
		t.Fatalf("payloadBytes error = %v", err)
	}
	copyRegistry := registry
	copyRegistry.Signature = hex.EncodeToString(ed25519.Sign(privateKey, payload))
	registryPath := filepath.Join(root, "registry.json")
	data, err := json.MarshalIndent(copyRegistry, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent error = %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(registryPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile(registry) error = %v", err)
	}
	_ = payload
	policyPath := filepath.Join(root, "writer_policy.json")
	policyJSON := `{
  "sync_mode": "trusted_writers_only",
  "allow_unsigned": false,
  "default_capability": "read_only",
  "trusted_authorities": {
    "authority://news/main": "` + registry.PublicKey + `"
  },
  "shared_registries": [
    "` + registryPath + `"
  ]
}`
	if err := os.WriteFile(policyPath, []byte(policyJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(policy) error = %v", err)
	}
	policy, err := LoadWriterPolicy(policyPath)
	if err != nil {
		t.Fatalf("LoadWriterPolicy error = %v", err)
	}
	if !policy.acceptsOrigin(&MessageOrigin{AgentID: "agent://writer/shared", PublicKey: "shared-key"}) {
		t.Fatal("expected shared registry capability to be merged")
	}
	if policy.acceptsRelay("", "mirror.example") {
		t.Fatal("expected relay host from shared registry to be blocked")
	}
}

func TestApplyWriterPolicyDelegationUsesParentCapability(t *testing.T) {
	t.Parallel()

	post := Bundle{
		InfoHash: "post-delegated",
		Message: Message{
			Kind:   "post",
			Title:  "Delegated",
			Author: "agent://writer/child",
			Origin: &MessageOrigin{
				AgentID:   "agent://writer/child",
				PublicKey: "child-key",
			},
			Extensions: map[string]any{"project": "hao.news", "topics": []any{"all", "world"}},
		},
	}
	index := buildIndex([]Bundle{post}, "hao.news")
	policy := WriterPolicy{
		SyncMode:          WriterSyncModeTrustedWritersOnly,
		DefaultCapability: WriterCapabilityReadOnly,
		AgentCapabilities: map[string]WriterCapability{
			"agent://writer/parent": WriterCapabilityReadWrite,
		},
	}
	store := DelegationStore{
		Delegations: []WriterDelegation{
			{
				ParentAgentID:   "agent://writer/parent",
				ParentPublicKey: "parent-key",
				ChildAgentID:    "agent://writer/child",
				ChildPublicKey:  "child-key",
				Scopes:          []string{"post"},
				CreatedAt:       "2024-03-15T12:00:00Z",
			},
		},
	}

	filtered := ApplyWriterPolicyWithDelegations(index, "hao.news", policy, store)
	if len(filtered.Posts) != 1 {
		t.Fatalf("posts len = %d, want 1", len(filtered.Posts))
	}
	if filtered.Posts[0].Delegation == nil || filtered.Posts[0].Delegation.ParentAgentID != "agent://writer/parent" {
		t.Fatal("expected delegated post to carry parent metadata")
	}
}

func TestApplyWriterPolicyDelegationDoesNotOverrideExplicitChildReadOnly(t *testing.T) {
	t.Parallel()

	post := Bundle{
		InfoHash: "post-delegated",
		Message: Message{
			Kind:   "post",
			Title:  "Delegated",
			Author: "agent://writer/child",
			Origin: &MessageOrigin{
				AgentID:   "agent://writer/child",
				PublicKey: "child-key",
			},
			Extensions: map[string]any{"project": "hao.news", "topics": []any{"all", "world"}},
		},
	}
	index := buildIndex([]Bundle{post}, "hao.news")
	policy := WriterPolicy{
		SyncMode:          WriterSyncModeTrustedWritersOnly,
		DefaultCapability: WriterCapabilityReadOnly,
		AgentCapabilities: map[string]WriterCapability{
			"agent://writer/parent": WriterCapabilityReadWrite,
			"agent://writer/child":  WriterCapabilityReadOnly,
		},
	}
	store := DelegationStore{
		Delegations: []WriterDelegation{
			{
				ParentAgentID:   "agent://writer/parent",
				ParentPublicKey: "parent-key",
				ChildAgentID:    "agent://writer/child",
				ChildPublicKey:  "child-key",
				Scopes:          []string{"post"},
				CreatedAt:       "2024-03-15T12:00:00Z",
			},
		},
	}

	filtered := ApplyWriterPolicyWithDelegations(index, "hao.news", policy, store)
	if len(filtered.Posts) != 0 {
		t.Fatalf("posts len = %d, want 0", len(filtered.Posts))
	}
}

func TestDelegationStoreRevocationDisablesParentChildLink(t *testing.T) {
	t.Parallel()

	store := DelegationStore{
		Delegations: []WriterDelegation{
			{
				ParentAgentID:   "agent://writer/parent",
				ParentPublicKey: "parent-key",
				ChildAgentID:    "agent://writer/child",
				ChildPublicKey:  "child-key",
				Scopes:          []string{"reply"},
				CreatedAt:       "2024-03-15T12:00:00Z",
			},
		},
		Revocations: []WriterRevocation{
			{
				ParentAgentID:   "agent://writer/parent",
				ParentPublicKey: "parent-key",
				ChildAgentID:    "agent://writer/child",
				ChildPublicKey:  "child-key",
				CreatedAt:       "2024-03-15T12:10:00Z",
			},
		},
	}

	if _, ok := store.ActiveDelegationFor("agent://writer/child", "child-key", "reply", time.Date(2024, 3, 15, 13, 0, 0, 0, time.UTC)); ok {
		t.Fatal("expected revoked delegation to be inactive")
	}
}

func TestLoadWriterPolicyMergesWhitelistAndBlacklistINF(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	policyPath := filepath.Join(root, "writer_policy.json")
	if err := os.WriteFile(policyPath, []byte("{\n  \"sync_mode\": \"all\"\n}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(policy) error = %v", err)
	}
	whitelist := "# comment\nagent://news/publisher-01\npublic_key=abcd1234\n"
	if err := os.WriteFile(filepath.Join(root, writerWhitelistINFName), []byte(whitelist), 0o644); err != nil {
		t.Fatalf("WriteFile(whitelist) error = %v", err)
	}
	blacklist := "agent_id=agent://spam/bot-99\ndeadbeef9999\n"
	if err := os.WriteFile(filepath.Join(root, writerBlacklistINFName), []byte(blacklist), 0o644); err != nil {
		t.Fatalf("WriteFile(blacklist) error = %v", err)
	}

	policy, err := LoadWriterPolicy(policyPath)
	if err != nil {
		t.Fatalf("LoadWriterPolicy error = %v", err)
	}
	if !containsFold(policy.AllowedAgentIDs, "agent://news/publisher-01") {
		t.Fatal("expected whitelist agent to be merged")
	}
	if !containsFold(policy.AllowedPublicKeys, "abcd1234") {
		t.Fatal("expected whitelist public key to be merged")
	}
	if !containsFold(policy.BlockedAgentIDs, "agent://spam/bot-99") {
		t.Fatal("expected blacklist agent to be merged")
	}
	if !containsFold(policy.BlockedPublicKeys, "deadbeef9999") {
		t.Fatal("expected blacklist public key to be merged")
	}
}
