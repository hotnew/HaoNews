package aip2p

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type IdentityRegistry struct {
	Entries map[string]IdentityRegistryEntry `json:"entries"`
}

type IdentityRegistryEntry struct {
	MasterPubKey string `json:"master_pubkey"`
	TrustLevel   string `json:"trust_level"`
	AddedAt      string `json:"added_at"`
	Notes        string `json:"notes,omitempty"`
}

func LoadIdentityRegistry(path string) (*IdentityRegistry, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("registry path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &IdentityRegistry{Entries: map[string]IdentityRegistryEntry{}}, nil
		}
		return nil, err
	}
	var registry IdentityRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, err
	}
	registry.normalize()
	return &registry, nil
}

func (r *IdentityRegistry) Add(author, masterPubKey, trustLevel, notes string, now time.Time) error {
	if r == nil {
		return errors.New("identity registry is nil")
	}
	author = strings.TrimSpace(author)
	masterPubKey = strings.ToLower(strings.TrimSpace(masterPubKey))
	if author == "" {
		return errors.New("author is required")
	}
	if masterPubKey == "" {
		return errors.New("master_pubkey is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	r.normalize()
	r.Entries[author] = IdentityRegistryEntry{
		MasterPubKey: masterPubKey,
		TrustLevel:   normalizeTrustLevel(trustLevel),
		AddedAt:      now.UTC().Format(time.RFC3339),
		Notes:        strings.TrimSpace(notes),
	}
	return nil
}

func (r *IdentityRegistry) Get(author string) (IdentityRegistryEntry, bool) {
	if r == nil {
		return IdentityRegistryEntry{}, false
	}
	r.normalize()
	entry, ok := r.Entries[strings.TrimSpace(author)]
	return entry, ok
}

func (r *IdentityRegistry) Remove(author string) bool {
	if r == nil {
		return false
	}
	r.normalize()
	author = strings.TrimSpace(author)
	if _, ok := r.Entries[author]; !ok {
		return false
	}
	delete(r.Entries, author)
	return true
}

func (r *IdentityRegistry) Save(path string) error {
	if r == nil {
		return errors.New("identity registry is nil")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("registry path is required")
	}
	r.normalize()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (r *IdentityRegistry) normalize() {
	if r.Entries == nil {
		r.Entries = map[string]IdentityRegistryEntry{}
		return
	}
	normalized := make(map[string]IdentityRegistryEntry, len(r.Entries))
	for author, entry := range r.Entries {
		author = strings.TrimSpace(author)
		if author == "" {
			continue
		}
		entry.MasterPubKey = strings.ToLower(strings.TrimSpace(entry.MasterPubKey))
		entry.TrustLevel = normalizeTrustLevel(entry.TrustLevel)
		entry.Notes = strings.TrimSpace(entry.Notes)
		normalized[author] = entry
	}
	r.Entries = normalized
}

func normalizeTrustLevel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "trusted":
		return "trusted"
	case "unknown":
		return "unknown"
	case "known":
		fallthrough
	default:
		return "known"
	}
}

func DefaultIdentityRegistryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return "", errors.New("user home directory is empty")
	}
	return filepath.Join(home, ".hao-news", "identity_registry.json"), nil
}

func LoadMasterIdentity(author string) (*AgentIdentity, error) {
	author = strings.TrimSpace(author)
	if author == "" {
		return nil, errors.New("author is required")
	}
	rootAuthor, err := RootAuthor(author)
	if err != nil {
		return nil, err
	}
	registryPath, err := DefaultIdentityRegistryPath()
	if err != nil {
		return nil, err
	}
	registry, err := LoadIdentityRegistry(registryPath)
	if err != nil {
		return nil, err
	}
	entry, ok := registry.Get(rootAuthor)
	if !ok {
		return nil, fmt.Errorf("identity %s not found in registry", rootAuthor)
	}
	return &AgentIdentity{
		Author:       rootAuthor,
		KeyType:      KeyTypeEd25519,
		PublicKey:    entry.MasterPubKey,
		MasterPubKey: entry.MasterPubKey,
		HDEnabled:    true,
	}, nil
}
