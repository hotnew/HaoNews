package aip2p

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIdentityRegistryAddSaveLoadAndRemove(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "identity_registry.json")
	registry, err := LoadIdentityRegistry(path)
	if err != nil {
		t.Fatalf("LoadIdentityRegistry() error = %v", err)
	}
	if err := registry.Add("agent://alice", "AABBCC", "trusted", "root identity", time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := registry.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := LoadIdentityRegistry(path)
	if err != nil {
		t.Fatalf("LoadIdentityRegistry(saved) error = %v", err)
	}
	entry, ok := loaded.Get("agent://alice")
	if !ok {
		t.Fatal("expected entry for agent://alice")
	}
	if entry.MasterPubKey != "aabbcc" {
		t.Fatalf("master_pubkey = %q", entry.MasterPubKey)
	}
	if entry.TrustLevel != "trusted" {
		t.Fatalf("trust_level = %q", entry.TrustLevel)
	}
	if !loaded.Remove("agent://alice") {
		t.Fatal("expected remove to report true")
	}
	if loaded.Remove("agent://alice") {
		t.Fatal("expected second remove to report false")
	}
}

func TestLoadMasterIdentityUsesDefaultRegistry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	registryPath := filepath.Join(home, ".hao-news", "identity_registry.json")
	registry := &IdentityRegistry{}
	if err := registry.Add("agent://alice", "aabbcc", "trusted", "", time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if err := registry.Save(registryPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	identity, err := LoadMasterIdentity("agent://alice/work")
	if err != nil {
		t.Fatalf("LoadMasterIdentity() error = %v", err)
	}
	if identity.Author != "agent://alice" {
		t.Fatalf("author = %q", identity.Author)
	}
	if identity.MasterPubKey != "aabbcc" {
		t.Fatalf("master_pubkey = %q", identity.MasterPubKey)
	}

	if _, err := os.Stat(registryPath); err != nil {
		t.Fatalf("expected registry file to exist: %v", err)
	}
}
