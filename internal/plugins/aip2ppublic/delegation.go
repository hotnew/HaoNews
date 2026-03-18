package newsplugin

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const latestAppDelegationVersion = "aip2p-delegation/0.1"

type DelegationKind string

const (
	DelegationKindWriterDelegation DelegationKind = "writer_delegation"
	DelegationKindWriterRevocation DelegationKind = "writer_revocation"
)

type WriterDelegation struct {
	Type            DelegationKind `json:"type"`
	Version         string         `json:"version"`
	ParentAgentID   string         `json:"parent_agent_id"`
	ParentKeyType   string         `json:"parent_key_type"`
	ParentPublicKey string         `json:"parent_public_key"`
	ChildAgentID    string         `json:"child_agent_id"`
	ChildKeyType    string         `json:"child_key_type"`
	ChildPublicKey  string         `json:"child_public_key"`
	Scopes          []string       `json:"scopes,omitempty"`
	CreatedAt       string         `json:"created_at"`
	ExpiresAt       string         `json:"expires_at,omitempty"`
	Signature       string         `json:"signature"`
}

type WriterRevocation struct {
	Type            DelegationKind `json:"type"`
	Version         string         `json:"version"`
	ParentAgentID   string         `json:"parent_agent_id"`
	ParentKeyType   string         `json:"parent_key_type"`
	ParentPublicKey string         `json:"parent_public_key"`
	ChildAgentID    string         `json:"child_agent_id"`
	ChildKeyType    string         `json:"child_key_type"`
	ChildPublicKey  string         `json:"child_public_key"`
	Reason          string         `json:"reason,omitempty"`
	CreatedAt       string         `json:"created_at"`
	Signature       string         `json:"signature"`
}

type unsignedWriterDelegation struct {
	Type            DelegationKind `json:"type"`
	Version         string         `json:"version"`
	ParentAgentID   string         `json:"parent_agent_id"`
	ParentKeyType   string         `json:"parent_key_type"`
	ParentPublicKey string         `json:"parent_public_key"`
	ChildAgentID    string         `json:"child_agent_id"`
	ChildKeyType    string         `json:"child_key_type"`
	ChildPublicKey  string         `json:"child_public_key"`
	Scopes          []string       `json:"scopes,omitempty"`
	CreatedAt       string         `json:"created_at"`
	ExpiresAt       string         `json:"expires_at,omitempty"`
}

type unsignedWriterRevocation struct {
	Type            DelegationKind `json:"type"`
	Version         string         `json:"version"`
	ParentAgentID   string         `json:"parent_agent_id"`
	ParentKeyType   string         `json:"parent_key_type"`
	ParentPublicKey string         `json:"parent_public_key"`
	ChildAgentID    string         `json:"child_agent_id"`
	ChildKeyType    string         `json:"child_key_type"`
	ChildPublicKey  string         `json:"child_public_key"`
	Reason          string         `json:"reason,omitempty"`
	CreatedAt       string         `json:"created_at"`
}

type DelegationStore struct {
	Delegations []WriterDelegation
	Revocations []WriterRevocation
}

func (d *WriterDelegation) Normalize() {
	if d == nil {
		return
	}
	d.Type = DelegationKind(strings.TrimSpace(string(d.Type)))
	if d.Type == "" {
		d.Type = DelegationKindWriterDelegation
	}
	d.Version = strings.TrimSpace(d.Version)
	if d.Version == "" {
		d.Version = latestAppDelegationVersion
	}
	d.ParentAgentID = strings.TrimSpace(d.ParentAgentID)
	d.ParentKeyType = strings.TrimSpace(d.ParentKeyType)
	if d.ParentKeyType == "" {
		d.ParentKeyType = latestAppKeyTypeEd25519
	}
	d.ParentPublicKey = strings.ToLower(strings.TrimSpace(d.ParentPublicKey))
	d.ChildAgentID = strings.TrimSpace(d.ChildAgentID)
	d.ChildKeyType = strings.TrimSpace(d.ChildKeyType)
	if d.ChildKeyType == "" {
		d.ChildKeyType = latestAppKeyTypeEd25519
	}
	d.ChildPublicKey = strings.ToLower(strings.TrimSpace(d.ChildPublicKey))
	d.Scopes = uniqueFold(d.Scopes)
	d.CreatedAt = strings.TrimSpace(d.CreatedAt)
	d.ExpiresAt = strings.TrimSpace(d.ExpiresAt)
	d.Signature = strings.ToLower(strings.TrimSpace(d.Signature))
}

func (r *WriterRevocation) Normalize() {
	if r == nil {
		return
	}
	r.Type = DelegationKind(strings.TrimSpace(string(r.Type)))
	if r.Type == "" {
		r.Type = DelegationKindWriterRevocation
	}
	r.Version = strings.TrimSpace(r.Version)
	if r.Version == "" {
		r.Version = latestAppDelegationVersion
	}
	r.ParentAgentID = strings.TrimSpace(r.ParentAgentID)
	r.ParentKeyType = strings.TrimSpace(r.ParentKeyType)
	if r.ParentKeyType == "" {
		r.ParentKeyType = latestAppKeyTypeEd25519
	}
	r.ParentPublicKey = strings.ToLower(strings.TrimSpace(r.ParentPublicKey))
	r.ChildAgentID = strings.TrimSpace(r.ChildAgentID)
	r.ChildKeyType = strings.TrimSpace(r.ChildKeyType)
	if r.ChildKeyType == "" {
		r.ChildKeyType = latestAppKeyTypeEd25519
	}
	r.ChildPublicKey = strings.ToLower(strings.TrimSpace(r.ChildPublicKey))
	r.Reason = strings.TrimSpace(r.Reason)
	r.CreatedAt = strings.TrimSpace(r.CreatedAt)
	r.Signature = strings.ToLower(strings.TrimSpace(r.Signature))
}

func ValidateWriterDelegation(delegation WriterDelegation) error {
	delegation.Normalize()
	if delegation.Type != DelegationKindWriterDelegation {
		return fmt.Errorf("unsupported delegation type %q", delegation.Type)
	}
	if delegation.Version != latestAppDelegationVersion {
		return fmt.Errorf("unsupported delegation version %q", delegation.Version)
	}
	if delegation.ParentAgentID == "" || delegation.ChildAgentID == "" {
		return errors.New("parent_agent_id and child_agent_id are required")
	}
	if delegation.ParentKeyType != latestAppKeyTypeEd25519 || delegation.ChildKeyType != latestAppKeyTypeEd25519 {
		return errors.New("only ed25519 delegations are supported")
	}
	if _, err := time.Parse(time.RFC3339, delegation.CreatedAt); err != nil {
		return errors.New("created_at must be RFC3339")
	}
	if delegation.ExpiresAt != "" {
		if _, err := time.Parse(time.RFC3339, delegation.ExpiresAt); err != nil {
			return errors.New("expires_at must be RFC3339")
		}
	}
	parentPublicKey, err := decodeHexKey(delegation.ParentPublicKey, ed25519.PublicKeySize, "parent_public_key")
	if err != nil {
		return err
	}
	if _, err := decodeHexKey(delegation.ChildPublicKey, ed25519.PublicKeySize, "child_public_key"); err != nil {
		return err
	}
	signature, err := decodeHexKey(delegation.Signature, ed25519.SignatureSize, "signature")
	if err != nil {
		return err
	}
	payload, err := delegation.payloadBytes()
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(parentPublicKey), payload, signature) {
		return errors.New("delegation signature verification failed")
	}
	return nil
}

func ValidateWriterRevocation(revocation WriterRevocation) error {
	revocation.Normalize()
	if revocation.Type != DelegationKindWriterRevocation {
		return fmt.Errorf("unsupported revocation type %q", revocation.Type)
	}
	if revocation.Version != latestAppDelegationVersion {
		return fmt.Errorf("unsupported revocation version %q", revocation.Version)
	}
	if revocation.ParentAgentID == "" || revocation.ChildAgentID == "" {
		return errors.New("parent_agent_id and child_agent_id are required")
	}
	if revocation.ParentKeyType != latestAppKeyTypeEd25519 || revocation.ChildKeyType != latestAppKeyTypeEd25519 {
		return errors.New("only ed25519 revocations are supported")
	}
	if _, err := time.Parse(time.RFC3339, revocation.CreatedAt); err != nil {
		return errors.New("created_at must be RFC3339")
	}
	parentPublicKey, err := decodeHexKey(revocation.ParentPublicKey, ed25519.PublicKeySize, "parent_public_key")
	if err != nil {
		return err
	}
	if _, err := decodeHexKey(revocation.ChildPublicKey, ed25519.PublicKeySize, "child_public_key"); err != nil {
		return err
	}
	signature, err := decodeHexKey(revocation.Signature, ed25519.SignatureSize, "signature")
	if err != nil {
		return err
	}
	payload, err := revocation.payloadBytes()
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(parentPublicKey), payload, signature) {
		return errors.New("revocation signature verification failed")
	}
	return nil
}

func (d WriterDelegation) payloadBytes() ([]byte, error) {
	d.Normalize()
	return json.Marshal(unsignedWriterDelegation{
		Type:            d.Type,
		Version:         d.Version,
		ParentAgentID:   d.ParentAgentID,
		ParentKeyType:   d.ParentKeyType,
		ParentPublicKey: d.ParentPublicKey,
		ChildAgentID:    d.ChildAgentID,
		ChildKeyType:    d.ChildKeyType,
		ChildPublicKey:  d.ChildPublicKey,
		Scopes:          d.Scopes,
		CreatedAt:       d.CreatedAt,
		ExpiresAt:       d.ExpiresAt,
	})
}

func (r WriterRevocation) payloadBytes() ([]byte, error) {
	r.Normalize()
	return json.Marshal(unsignedWriterRevocation{
		Type:            r.Type,
		Version:         r.Version,
		ParentAgentID:   r.ParentAgentID,
		ParentKeyType:   r.ParentKeyType,
		ParentPublicKey: r.ParentPublicKey,
		ChildAgentID:    r.ChildAgentID,
		ChildKeyType:    r.ChildKeyType,
		ChildPublicKey:  r.ChildPublicKey,
		Reason:          r.Reason,
		CreatedAt:       r.CreatedAt,
	})
}

func LoadWriterDelegation(path string) (WriterDelegation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WriterDelegation{}, err
	}
	var delegation WriterDelegation
	if err := json.Unmarshal(data, &delegation); err != nil {
		return WriterDelegation{}, err
	}
	if err := ValidateWriterDelegation(delegation); err != nil {
		return WriterDelegation{}, err
	}
	return delegation, nil
}

func LoadWriterRevocation(path string) (WriterRevocation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WriterRevocation{}, err
	}
	var revocation WriterRevocation
	if err := json.Unmarshal(data, &revocation); err != nil {
		return WriterRevocation{}, err
	}
	if err := ValidateWriterRevocation(revocation); err != nil {
		return WriterRevocation{}, err
	}
	return revocation, nil
}

func LoadDelegationStore(delegationDir, revocationDir string) (DelegationStore, error) {
	delegations, err := loadDelegationsFromDir(delegationDir)
	if err != nil {
		return DelegationStore{}, err
	}
	revocations, err := loadRevocationsFromDir(revocationDir)
	if err != nil {
		return DelegationStore{}, err
	}
	return DelegationStore{
		Delegations: delegations,
		Revocations: revocations,
	}, nil
}

func (s DelegationStore) ActiveDelegationFor(childAgentID, childPublicKey, scope string, now time.Time) (*WriterDelegation, bool) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	childAgentID = strings.TrimSpace(childAgentID)
	childPublicKey = strings.ToLower(strings.TrimSpace(childPublicKey))
	scope = normalizeFoldKey(scope)
	var candidates []WriterDelegation
	for _, delegation := range s.Delegations {
		delegation.Normalize()
		if strings.TrimSpace(delegation.ChildAgentID) != childAgentID {
			continue
		}
		if strings.ToLower(strings.TrimSpace(delegation.ChildPublicKey)) != childPublicKey {
			continue
		}
		if !delegationScopeMatches(delegation, scope) {
			continue
		}
		if delegationExpired(delegation, now) {
			continue
		}
		if s.isRevoked(delegation, now) {
			continue
		}
		candidates = append(candidates, delegation)
	}
	if len(candidates) == 0 {
		return nil, false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].CreatedAt > candidates[j].CreatedAt
	})
	selected := candidates[0]
	return &selected, true
}

func (s DelegationStore) isRevoked(delegation WriterDelegation, now time.Time) bool {
	delegationTime, err := time.Parse(time.RFC3339, delegation.CreatedAt)
	if err != nil {
		return true
	}
	for _, revocation := range s.Revocations {
		revocation.Normalize()
		if revocation.ParentAgentID != delegation.ParentAgentID {
			continue
		}
		if revocation.ParentPublicKey != delegation.ParentPublicKey {
			continue
		}
		if revocation.ChildAgentID != delegation.ChildAgentID {
			continue
		}
		if revocation.ChildPublicKey != delegation.ChildPublicKey {
			continue
		}
		revocationTime, err := time.Parse(time.RFC3339, revocation.CreatedAt)
		if err != nil {
			continue
		}
		if revocationTime.Before(delegationTime) {
			continue
		}
		if !revocationTime.After(now) {
			return true
		}
	}
	return false
}

func delegationExpired(delegation WriterDelegation, now time.Time) bool {
	if strings.TrimSpace(delegation.ExpiresAt) == "" {
		return false
	}
	expiresAt, err := time.Parse(time.RFC3339, delegation.ExpiresAt)
	if err != nil {
		return true
	}
	return !expiresAt.After(now)
}

func delegationScopeMatches(delegation WriterDelegation, scope string) bool {
	if len(delegation.Scopes) == 0 {
		return true
	}
	if scope == "" {
		return true
	}
	for _, item := range delegation.Scopes {
		if normalizeFoldKey(item) == scope {
			return true
		}
	}
	return false
}

func loadDelegationsFromDir(root string) ([]WriterDelegation, error) {
	paths, err := jsonFilesInDir(root)
	if err != nil {
		return nil, err
	}
	out := make([]WriterDelegation, 0, len(paths))
	for _, path := range paths {
		item, err := LoadWriterDelegation(path)
		if err != nil {
			return nil, fmt.Errorf("load delegation %s: %w", path, err)
		}
		out = append(out, item)
	}
	return out, nil
}

func loadRevocationsFromDir(root string) ([]WriterRevocation, error) {
	paths, err := jsonFilesInDir(root)
	if err != nil {
		return nil, err
	}
	out := make([]WriterRevocation, 0, len(paths))
	for _, path := range paths {
		item, err := LoadWriterRevocation(path)
		if err != nil {
			return nil, fmt.Errorf("load revocation %s: %w", path, err)
		}
		out = append(out, item)
	}
	return out, nil
}

func jsonFilesInDir(root string) ([]string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		out = append(out, filepath.Join(root, entry.Name()))
	}
	sort.Strings(out)
	return out, nil
}
