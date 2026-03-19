package newsplugin

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type WriterCapability string

const (
	WriterCapabilityReadWrite WriterCapability = "read_write"
	WriterCapabilityReadOnly  WriterCapability = "read_only"
	WriterCapabilityBlocked   WriterCapability = "blocked"
)

type WriterSyncMode string
type WriterTrustMode string

const (
	WriterSyncModeMixed              WriterSyncMode = "mixed"
	WriterSyncModeAll                WriterSyncMode = "all"
	WriterSyncModeTrustedWritersOnly WriterSyncMode = "trusted_writers_only"
	WriterSyncModeWhitelist          WriterSyncMode = "whitelist"
	WriterSyncModeBlacklist          WriterSyncMode = "blacklist"

	WriterTrustModeExact             WriterTrustMode = "exact"
	WriterTrustModeParentAndChildren WriterTrustMode = "parent_and_children"
)

type WriterPolicy struct {
	SyncMode              WriterSyncMode              `json:"sync_mode,omitempty"`
	TrustMode             WriterTrustMode             `json:"trust_mode,omitempty"`
	AllowUnsigned         bool                        `json:"allow_unsigned"`
	DefaultCapability     WriterCapability            `json:"default_capability,omitempty"`
	AgentCapabilities     map[string]WriterCapability `json:"agent_capabilities,omitempty"`
	PublicKeyCapabilities map[string]WriterCapability `json:"public_key_capabilities,omitempty"`
	AllowedAgentIDs       []string                    `json:"allowed_agent_ids"`
	AllowedPublicKeys     []string                    `json:"allowed_public_keys"`
	BlockedAgentIDs       []string                    `json:"blocked_agent_ids"`
	BlockedPublicKeys     []string                    `json:"blocked_public_keys"`
	TrustedAuthorities    map[string]string           `json:"trusted_authorities,omitempty"`
	SharedRegistries      []string                    `json:"shared_registries,omitempty"`
	RelayDefaultTrust     RelayTrust                  `json:"relay_default_trust,omitempty"`
	RelayPeerTrust        map[string]RelayTrust       `json:"relay_peer_trust,omitempty"`
	RelayHostTrust        map[string]RelayTrust       `json:"relay_host_trust,omitempty"`
}

const (
	writerWhitelistINFName = "WriterWhitelist.inf"
	writerBlacklistINFName = "WriterBlacklist.inf"
)

type WriterOriginDecision struct {
	Capability        WriterCapability
	Delegation        *WriterDelegation
	ExplicitReadWrite bool
}

func LoadWriterPolicy(path string) (WriterPolicy, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return defaultWriterPolicy(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			policy := defaultWriterPolicy()
			if err := policy.loadINFLists(path); err != nil {
				return WriterPolicy{}, err
			}
			return policy, nil
		}
		return WriterPolicy{}, err
	}
	var policy WriterPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		return WriterPolicy{}, err
	}
	policy.normalize()
	if err := policy.loadSharedRegistries(); err != nil {
		return WriterPolicy{}, err
	}
	if err := policy.loadINFLists(path); err != nil {
		return WriterPolicy{}, err
	}
	return policy, nil
}

func defaultWriterPolicy() WriterPolicy {
	policy := WriterPolicy{
		SyncMode:          WriterSyncModeAll,
		TrustMode:         WriterTrustModeExact,
		AllowUnsigned:     false,
		DefaultCapability: WriterCapabilityReadWrite,
		RelayDefaultTrust: RelayTrustNeutral,
	}
	policy.normalize()
	return policy
}

func (p *WriterPolicy) normalize() {
	if p == nil {
		return
	}
	p.SyncMode = normalizeWriterSyncMode(p.SyncMode, WriterSyncModeAll)
	p.TrustMode = normalizeWriterTrustMode(p.TrustMode, WriterTrustModeExact)
	p.DefaultCapability = normalizeWriterCapability(p.DefaultCapability, WriterCapabilityReadWrite)
	p.AllowedAgentIDs = uniqueFold(p.AllowedAgentIDs)
	p.AllowedPublicKeys = normalizeHexList(p.AllowedPublicKeys)
	p.BlockedAgentIDs = uniqueFold(p.BlockedAgentIDs)
	p.BlockedPublicKeys = normalizeHexList(p.BlockedPublicKeys)
	p.TrustedAuthorities = normalizePublicKeyMap(p.TrustedAuthorities)
	p.SharedRegistries = uniqueTrim(p.SharedRegistries)
	p.RelayDefaultTrust = normalizeRelayTrust(p.RelayDefaultTrust, RelayTrustNeutral)
	p.RelayPeerTrust = normalizeRelayTrustMap(p.RelayPeerTrust, false)
	p.RelayHostTrust = normalizeRelayTrustMap(p.RelayHostTrust, true)
	p.AgentCapabilities = normalizeCapabilityMap(p.AgentCapabilities, false)
	p.PublicKeyCapabilities = normalizeCapabilityMap(p.PublicKeyCapabilities, true)
}

func (p *WriterPolicy) Normalize() {
	p.normalize()
}

func (p WriterPolicy) Empty() bool {
	p.normalize()
	return p.AllowUnsigned &&
		p.SyncMode == WriterSyncModeAll &&
		p.TrustMode == WriterTrustModeExact &&
		p.DefaultCapability == WriterCapabilityReadWrite &&
		len(p.AgentCapabilities) == 0 &&
		len(p.PublicKeyCapabilities) == 0 &&
		len(p.AllowedAgentIDs) == 0 &&
		len(p.AllowedPublicKeys) == 0 &&
		len(p.BlockedAgentIDs) == 0 &&
		len(p.BlockedPublicKeys) == 0 &&
		len(p.TrustedAuthorities) == 0 &&
		len(p.SharedRegistries) == 0 &&
		p.RelayDefaultTrust == RelayTrustNeutral &&
		len(p.RelayPeerTrust) == 0 &&
		len(p.RelayHostTrust) == 0
}

func (p WriterPolicy) allowsOrigin(origin *MessageOrigin) bool {
	return p.allowsOriginWithDelegation(origin, "", DelegationStore{})
}

func (p WriterPolicy) acceptsOrigin(origin *MessageOrigin) bool {
	return p.acceptsOriginWithDelegation(origin, "", DelegationStore{})
}

func (p WriterPolicy) allowsOriginWithDelegation(origin *MessageOrigin, scope string, store DelegationStore) bool {
	return p.originDecision(origin, scope, store).Capability == WriterCapabilityReadWrite
}

func (p WriterPolicy) acceptsOriginWithDelegation(origin *MessageOrigin, scope string, store DelegationStore) bool {
	p.normalize()
	if isUnsignedOrigin(origin) {
		return p.AllowUnsigned
	}

	decision := p.originDecision(origin, scope, store)
	if decision.Capability == WriterCapabilityBlocked {
		return false
	}

	switch p.SyncMode {
	case WriterSyncModeAll:
		return true
	case WriterSyncModeBlacklist:
		return true
	case WriterSyncModeWhitelist:
		return decision.ExplicitReadWrite
	case WriterSyncModeTrustedWritersOnly:
		return decision.Capability == WriterCapabilityReadWrite
	case WriterSyncModeMixed:
		fallthrough
	default:
		return decision.Capability == WriterCapabilityReadWrite
	}
}

func (p WriterPolicy) capabilityForOrigin(origin *MessageOrigin) WriterCapability {
	return p.capabilityForOriginWithDelegation(origin, "", DelegationStore{})
}

func (p WriterPolicy) capabilityForOriginWithDelegation(origin *MessageOrigin, scope string, store DelegationStore) WriterCapability {
	return p.originDecision(origin, scope, store).Capability
}

func (p WriterPolicy) acceptsMessageWithDelegation(msg Message, scope string, store DelegationStore) bool {
	p.normalize()
	if isUnsignedOrigin(msg.Origin) {
		return p.AllowUnsigned
	}

	decision := p.messageDecision(msg, scope, store)
	if decision.Capability == WriterCapabilityBlocked {
		return false
	}

	switch p.SyncMode {
	case WriterSyncModeAll:
		return true
	case WriterSyncModeBlacklist:
		return true
	case WriterSyncModeWhitelist:
		return decision.ExplicitReadWrite
	case WriterSyncModeTrustedWritersOnly:
		return decision.Capability == WriterCapabilityReadWrite
	case WriterSyncModeMixed:
		fallthrough
	default:
		return decision.Capability == WriterCapabilityReadWrite
	}
}

func (p WriterPolicy) originDecision(origin *MessageOrigin, scope string, store DelegationStore) WriterOriginDecision {
	p.normalize()
	if isUnsignedOrigin(origin) {
		if !p.AllowUnsigned {
			return WriterOriginDecision{Capability: WriterCapabilityBlocked}
		}
		if p.hasLegacyWhitelist() {
			return WriterOriginDecision{Capability: WriterCapabilityReadOnly}
		}
		return WriterOriginDecision{Capability: p.DefaultCapability}
	}

	child := p.capabilityState(origin.AgentID, origin.PublicKey)
	if child.Capability == WriterCapabilityBlocked {
		return WriterOriginDecision{Capability: WriterCapabilityBlocked}
	}

	decision := WriterOriginDecision{
		Capability:        child.Capability,
		ExplicitReadWrite: child.ExplicitReadWrite,
	}
	scope = normalizeFoldKey(scope)
	if delegation, ok := store.ActiveDelegationFor(strings.TrimSpace(origin.AgentID), strings.ToLower(strings.TrimSpace(origin.PublicKey)), scope, time.Time{}); ok {
		parent := p.capabilityState(delegation.ParentAgentID, delegation.ParentPublicKey)
		if parent.Capability == WriterCapabilityBlocked {
			return WriterOriginDecision{
				Capability:        WriterCapabilityBlocked,
				Delegation:        delegation,
				ExplicitReadWrite: false,
			}
		}
		decision.Delegation = delegation
		if !child.ExplicitlyConfigured && capabilityRank(parent.Capability) > capabilityRank(decision.Capability) {
			decision.Capability = parent.Capability
		}
		if parent.ExplicitReadWrite {
			decision.ExplicitReadWrite = true
		}
	}
	return decision
}

func (p WriterPolicy) messageDecision(msg Message, scope string, store DelegationStore) WriterOriginDecision {
	decision := p.originDecision(msg.Origin, scope, store)
	author := strings.TrimSpace(msg.Author)
	if author == "" {
		return decision
	}
	if matchesWriterAuthorPolicy(author, p.BlockedAgentIDs, p.TrustMode) {
		return WriterOriginDecision{Capability: WriterCapabilityBlocked}
	}
	if p.hasLegacyWhitelist() && matchesWriterAuthorPolicy(author, p.AllowedAgentIDs, p.TrustMode) {
		if decision.Capability != WriterCapabilityBlocked {
			decision.Capability = WriterCapabilityReadWrite
			decision.ExplicitReadWrite = true
		}
	}
	return decision
}

func (p WriterPolicy) relayTrustFor(peerID, host string) RelayTrust {
	p.normalize()
	peerID = strings.TrimSpace(peerID)
	host = normalizeFoldKey(host)
	if peerID != "" {
		if trust, ok := p.RelayPeerTrust[peerID]; ok {
			return trust
		}
	}
	if host != "" {
		if trust, ok := p.RelayHostTrust[host]; ok {
			return trust
		}
	}
	return p.RelayDefaultTrust
}

func (p WriterPolicy) acceptsRelay(peerID, host string) bool {
	return p.relayTrustFor(peerID, host) != RelayTrustBlocked
}

func (p WriterPolicy) hasLegacyWhitelist() bool {
	return len(p.AllowedAgentIDs) > 0 || len(p.AllowedPublicKeys) > 0
}

func (p WriterPolicy) isExplicitlyAllowed(origin *MessageOrigin) bool {
	if origin == nil {
		return false
	}
	agentID := normalizeFoldKey(origin.AgentID)
	publicKey := strings.ToLower(strings.TrimSpace(origin.PublicKey))
	if publicKey != "" {
		if capability, ok := p.PublicKeyCapabilities[publicKey]; ok {
			return capability == WriterCapabilityReadWrite
		}
	}
	if agentID != "" {
		if capability, ok := p.AgentCapabilities[agentID]; ok {
			return capability == WriterCapabilityReadWrite
		}
	}
	if agentID != "" && containsFold(p.AllowedAgentIDs, agentID) {
		return true
	}
	if publicKey != "" && containsFold(p.AllowedPublicKeys, publicKey) {
		return true
	}
	return false
}

type writerCapabilityState struct {
	Capability           WriterCapability
	ExplicitlyConfigured bool
	ExplicitReadWrite    bool
}

func (p WriterPolicy) capabilityState(agentIDValue, publicKeyValue string) writerCapabilityState {
	agentID := normalizeFoldKey(agentIDValue)
	publicKey := strings.ToLower(strings.TrimSpace(publicKeyValue))

	if agentID != "" && containsFold(p.BlockedAgentIDs, agentID) {
		return writerCapabilityState{Capability: WriterCapabilityBlocked, ExplicitlyConfigured: true}
	}
	if publicKey != "" && containsFold(p.BlockedPublicKeys, publicKey) {
		return writerCapabilityState{Capability: WriterCapabilityBlocked, ExplicitlyConfigured: true}
	}
	if capability, ok := p.PublicKeyCapabilities[publicKey]; ok {
		return writerCapabilityState{
			Capability:           capability,
			ExplicitlyConfigured: true,
			ExplicitReadWrite:    capability == WriterCapabilityReadWrite,
		}
	}
	if capability, ok := p.AgentCapabilities[agentID]; ok {
		return writerCapabilityState{
			Capability:           capability,
			ExplicitlyConfigured: true,
			ExplicitReadWrite:    capability == WriterCapabilityReadWrite,
		}
	}
	if p.hasLegacyWhitelist() {
		if agentID != "" && containsFold(p.AllowedAgentIDs, agentID) {
			return writerCapabilityState{
				Capability:           WriterCapabilityReadWrite,
				ExplicitlyConfigured: true,
				ExplicitReadWrite:    true,
			}
		}
		if publicKey != "" && containsFold(p.AllowedPublicKeys, publicKey) {
			return writerCapabilityState{
				Capability:           WriterCapabilityReadWrite,
				ExplicitlyConfigured: true,
				ExplicitReadWrite:    true,
			}
		}
		return writerCapabilityState{Capability: WriterCapabilityReadOnly}
	}
	return writerCapabilityState{Capability: p.DefaultCapability}
}

func capabilityRank(capability WriterCapability) int {
	switch capability {
	case WriterCapabilityBlocked:
		return 0
	case WriterCapabilityReadOnly:
		return 1
	case WriterCapabilityReadWrite:
		return 2
	default:
		return 0
	}
}

func isUnsignedOrigin(origin *MessageOrigin) bool {
	if origin == nil {
		return true
	}
	return strings.TrimSpace(origin.PublicKey) == ""
}

func normalizeWriterSyncMode(value, fallback WriterSyncMode) WriterSyncMode {
	switch WriterSyncMode(strings.ToLower(strings.TrimSpace(string(value)))) {
	case WriterSyncModeMixed:
		return WriterSyncModeMixed
	case WriterSyncModeAll:
		return WriterSyncModeAll
	case WriterSyncModeTrustedWritersOnly:
		return WriterSyncModeTrustedWritersOnly
	case WriterSyncModeWhitelist:
		return WriterSyncModeWhitelist
	case WriterSyncModeBlacklist:
		return WriterSyncModeBlacklist
	default:
		return fallback
	}
}

func normalizeWriterCapability(value, fallback WriterCapability) WriterCapability {
	switch WriterCapability(strings.ToLower(strings.TrimSpace(string(value)))) {
	case WriterCapabilityReadWrite:
		return WriterCapabilityReadWrite
	case WriterCapabilityReadOnly:
		return WriterCapabilityReadOnly
	case WriterCapabilityBlocked:
		return WriterCapabilityBlocked
	default:
		return fallback
	}
}

func normalizeWriterTrustMode(value, fallback WriterTrustMode) WriterTrustMode {
	switch WriterTrustMode(strings.ToLower(strings.TrimSpace(string(value)))) {
	case WriterTrustModeExact:
		return WriterTrustModeExact
	case WriterTrustModeParentAndChildren:
		return WriterTrustModeParentAndChildren
	default:
		return fallback
	}
}

func normalizeCapabilityMap(items map[string]WriterCapability, hexKeys bool) map[string]WriterCapability {
	if len(items) == 0 {
		return nil
	}
	normalized := make(map[string]WriterCapability, len(items))
	for key, capability := range items {
		if hexKeys {
			key = strings.ToLower(strings.TrimSpace(key))
		} else {
			key = normalizeFoldKey(key)
		}
		if key == "" {
			continue
		}
		normalized[key] = normalizeWriterCapability(capability, WriterCapabilityReadWrite)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func normalizeFoldKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func matchesWriterAuthorPolicy(author string, entries []string, mode WriterTrustMode) bool {
	author = normalizeFoldKey(author)
	if author == "" {
		return false
	}
	for _, entry := range entries {
		entry = normalizeFoldKey(entry)
		if entry == "" {
			continue
		}
		if author == entry {
			return true
		}
		if mode == WriterTrustModeParentAndChildren && strings.HasPrefix(author, entry+"/") {
			return true
		}
	}
	return false
}

func normalizeHexList(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		normalized = append(normalized, item)
	}
	return uniqueFold(normalized)
}

func (p *WriterPolicy) loadSharedRegistries() error {
	if p == nil {
		return nil
	}
	p.normalize()
	if len(p.SharedRegistries) == 0 {
		return nil
	}
	local := *p
	merged := WriterPolicy{
		SyncMode:           local.SyncMode,
		AllowUnsigned:      local.AllowUnsigned,
		DefaultCapability:  local.DefaultCapability,
		TrustedAuthorities: local.TrustedAuthorities,
		SharedRegistries:   append([]string(nil), local.SharedRegistries...),
		RelayDefaultTrust:  local.RelayDefaultTrust,
	}
	for _, source := range local.SharedRegistries {
		registry, err := loadSignedWriterRegistrySource(source)
		if err != nil {
			return err
		}
		if err := registry.Validate(local.TrustedAuthorities); err != nil {
			return fmt.Errorf("verify shared registry %s: %w", source, err)
		}
		merged.AgentCapabilities = mergeRegistryCapabilities(merged.AgentCapabilities, registry.AgentCapabilities)
		merged.PublicKeyCapabilities = mergeRegistryCapabilities(merged.PublicKeyCapabilities, registry.PublicKeyCapabilities)
		merged.RelayPeerTrust = mergeRegistryRelayTrust(merged.RelayPeerTrust, registry.RelayPeerTrust)
		merged.RelayHostTrust = mergeRegistryRelayTrust(merged.RelayHostTrust, registry.RelayHostTrust)
	}
	merged.AgentCapabilities = mergeRegistryCapabilities(merged.AgentCapabilities, local.AgentCapabilities)
	merged.PublicKeyCapabilities = mergeRegistryCapabilities(merged.PublicKeyCapabilities, local.PublicKeyCapabilities)
	merged.RelayPeerTrust = mergeRegistryRelayTrust(merged.RelayPeerTrust, local.RelayPeerTrust)
	merged.RelayHostTrust = mergeRegistryRelayTrust(merged.RelayHostTrust, local.RelayHostTrust)
	merged.AllowedAgentIDs = append(append([]string(nil), merged.AllowedAgentIDs...), local.AllowedAgentIDs...)
	merged.AllowedPublicKeys = append(append([]string(nil), merged.AllowedPublicKeys...), local.AllowedPublicKeys...)
	merged.BlockedAgentIDs = append(append([]string(nil), merged.BlockedAgentIDs...), local.BlockedAgentIDs...)
	merged.BlockedPublicKeys = append(append([]string(nil), merged.BlockedPublicKeys...), local.BlockedPublicKeys...)
	merged.normalize()
	*p = merged
	return nil
}

func (p *WriterPolicy) loadINFLists(policyPath string) error {
	if p == nil {
		return nil
	}
	root := strings.TrimSpace(filepath.Dir(strings.TrimSpace(policyPath)))
	if root == "" || root == "." {
		return nil
	}
	allowedAgents, allowedKeys, err := loadWriterINFList(filepath.Join(root, writerWhitelistINFName))
	if err != nil {
		return err
	}
	blockedAgents, blockedKeys, err := loadWriterINFList(filepath.Join(root, writerBlacklistINFName))
	if err != nil {
		return err
	}
	p.AllowedAgentIDs = append(append([]string(nil), p.AllowedAgentIDs...), allowedAgents...)
	p.AllowedPublicKeys = append(append([]string(nil), p.AllowedPublicKeys...), allowedKeys...)
	p.BlockedAgentIDs = append(append([]string(nil), p.BlockedAgentIDs...), blockedAgents...)
	p.BlockedPublicKeys = append(append([]string(nil), p.BlockedPublicKeys...), blockedKeys...)
	p.normalize()
	return nil
}

func loadWriterINFList(path string) (agentIDs, publicKeys []string, err error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok {
			key = normalizeFoldKey(key)
			value = strings.TrimSpace(value)
			switch key {
			case "agent", "agent_id", "agentid":
				if value != "" {
					agentIDs = append(agentIDs, value)
				}
				continue
			case "public_key", "publickey", "pubkey", "key":
				if value != "" {
					publicKeys = append(publicKeys, value)
				}
				continue
			}
		}
		if strings.Contains(line, "agent://") || !looksLikeHex(line) {
			agentIDs = append(agentIDs, line)
			continue
		}
		publicKeys = append(publicKeys, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	return uniqueFold(agentIDs), normalizeHexList(publicKeys), nil
}

func looksLikeHex(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func ApplyWriterPolicy(index Index, project string, policy WriterPolicy) Index {
	return ApplyWriterPolicyWithDelegations(index, project, policy, DelegationStore{})
}

func ApplyWriterPolicyWithDelegations(index Index, project string, policy WriterPolicy, store DelegationStore) Index {
	policy.normalize()
	index = applyDelegationMetadata(index, project, store)
	if policy.Empty() {
		return index
	}
	filtered := make([]Bundle, 0, len(index.Bundles))
	allowed := make(map[string]struct{})
	for _, bundle := range index.Bundles {
		switch bundle.Message.Kind {
		case "post":
			if !policy.acceptsMessageWithDelegation(bundle.Message, bundle.Message.Kind, store) {
				continue
			}
			allowed[strings.ToLower(bundle.InfoHash)] = struct{}{}
			filtered = append(filtered, bundle)
		}
	}
	for _, bundle := range index.Bundles {
		switch bundle.Message.Kind {
		case "reply":
			if !policy.acceptsMessageWithDelegation(bundle.Message, bundle.Message.Kind, store) {
				continue
			}
			if bundle.Message.ReplyTo != nil {
				if _, ok := allowed[strings.ToLower(bundle.Message.ReplyTo.InfoHash)]; ok {
					filtered = append(filtered, bundle)
				}
			}
		case "reaction":
			if !policy.acceptsMessageWithDelegation(bundle.Message, bundle.Message.Kind, store) {
				continue
			}
			subject := strings.ToLower(nestedString(bundle.Message.Extensions, "subject", "infohash"))
			if _, ok := allowed[subject]; ok {
				filtered = append(filtered, bundle)
			}
		}
	}
	return buildIndex(filtered, project)
}

func applyDelegationMetadata(index Index, project string, store DelegationStore) Index {
	if len(store.Delegations) == 0 {
		return index
	}
	annotated := make([]Bundle, 0, len(index.Bundles))
	for _, bundle := range index.Bundles {
		if bundle.Message.Origin != nil {
			if delegation, ok := store.ActiveDelegationFor(bundle.Message.Origin.AgentID, bundle.Message.Origin.PublicKey, bundle.Message.Kind, time.Time{}); ok {
				bundle.Delegation = &DelegationInfo{
					Delegated:       true,
					ParentAgentID:   delegation.ParentAgentID,
					ParentKeyType:   delegation.ParentKeyType,
					ParentPublicKey: delegation.ParentPublicKey,
					Scopes:          append([]string(nil), delegation.Scopes...),
					CreatedAt:       delegation.CreatedAt,
					ExpiresAt:       delegation.ExpiresAt,
				}
			}
		}
		annotated = append(annotated, bundle)
	}
	return buildIndex(annotated, project)
}
