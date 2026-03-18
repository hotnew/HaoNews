package newsplugin

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const latestAppKeyTypeEd25519 = "ed25519"

type RelayTrust string

const (
	RelayTrustNeutral RelayTrust = "neutral"
	RelayTrustTrusted RelayTrust = "trusted"
	RelayTrustBlocked RelayTrust = "blocked"
)

type SignedWriterRegistry struct {
	Version               string                      `json:"version,omitempty"`
	Scope                 string                      `json:"scope,omitempty"`
	AuthorityID           string                      `json:"authority_id"`
	KeyType               string                      `json:"key_type"`
	PublicKey             string                      `json:"public_key"`
	SignedAt              string                      `json:"signed_at"`
	AgentCapabilities     map[string]WriterCapability `json:"agent_capabilities,omitempty"`
	PublicKeyCapabilities map[string]WriterCapability `json:"public_key_capabilities,omitempty"`
	RelayPeerTrust        map[string]RelayTrust       `json:"relay_peer_trust,omitempty"`
	RelayHostTrust        map[string]RelayTrust       `json:"relay_host_trust,omitempty"`
	Signature             string                      `json:"signature"`
}

type unsignedWriterRegistry struct {
	Version               string                      `json:"version,omitempty"`
	Scope                 string                      `json:"scope,omitempty"`
	AuthorityID           string                      `json:"authority_id"`
	KeyType               string                      `json:"key_type"`
	PublicKey             string                      `json:"public_key"`
	SignedAt              string                      `json:"signed_at"`
	AgentCapabilities     map[string]WriterCapability `json:"agent_capabilities,omitempty"`
	PublicKeyCapabilities map[string]WriterCapability `json:"public_key_capabilities,omitempty"`
	RelayPeerTrust        map[string]RelayTrust       `json:"relay_peer_trust,omitempty"`
	RelayHostTrust        map[string]RelayTrust       `json:"relay_host_trust,omitempty"`
}

var registryHTTPClient = &http.Client{Timeout: 4 * time.Second}

func normalizeRelayTrust(value, fallback RelayTrust) RelayTrust {
	switch RelayTrust(strings.ToLower(strings.TrimSpace(string(value)))) {
	case RelayTrustNeutral:
		return RelayTrustNeutral
	case RelayTrustTrusted:
		return RelayTrustTrusted
	case RelayTrustBlocked:
		return RelayTrustBlocked
	default:
		return fallback
	}
}

func normalizeRelayTrustMap(items map[string]RelayTrust, foldKeys bool) map[string]RelayTrust {
	if len(items) == 0 {
		return nil
	}
	normalized := make(map[string]RelayTrust, len(items))
	for key, trust := range items {
		if foldKeys {
			key = normalizeFoldKey(key)
		} else {
			key = strings.TrimSpace(key)
		}
		if key == "" {
			continue
		}
		normalized[key] = normalizeRelayTrust(trust, RelayTrustNeutral)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func normalizePublicKeyMap(items map[string]string) map[string]string {
	if len(items) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(items))
	for key, value := range items {
		key = normalizeFoldKey(key)
		value = strings.ToLower(strings.TrimSpace(value))
		if key == "" || value == "" {
			continue
		}
		normalized[key] = value
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func uniqueTrim(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (r *SignedWriterRegistry) Normalize() {
	if r == nil {
		return
	}
	r.Version = strings.TrimSpace(r.Version)
	if r.Version == "" {
		r.Version = "aip2p-writer-registry/0.1"
	}
	r.Scope = strings.TrimSpace(r.Scope)
	if r.Scope == "" {
		r.Scope = "writer_registry"
	}
	r.AuthorityID = strings.TrimSpace(r.AuthorityID)
	r.KeyType = strings.TrimSpace(r.KeyType)
	if r.KeyType == "" {
		r.KeyType = latestAppKeyTypeEd25519
	}
	r.PublicKey = strings.ToLower(strings.TrimSpace(r.PublicKey))
	r.SignedAt = strings.TrimSpace(r.SignedAt)
	r.Signature = strings.ToLower(strings.TrimSpace(r.Signature))
	r.AgentCapabilities = normalizeCapabilityMap(r.AgentCapabilities, false)
	r.PublicKeyCapabilities = normalizeCapabilityMap(r.PublicKeyCapabilities, true)
	r.RelayPeerTrust = normalizeRelayTrustMap(r.RelayPeerTrust, false)
	r.RelayHostTrust = normalizeRelayTrustMap(r.RelayHostTrust, true)
}

func (r SignedWriterRegistry) Validate(trustedAuthorities map[string]string) error {
	r.Normalize()
	if r.AuthorityID == "" {
		return errors.New("authority_id is required")
	}
	if r.KeyType != latestAppKeyTypeEd25519 {
		return fmt.Errorf("unsupported authority key_type %q", r.KeyType)
	}
	if _, err := time.Parse(time.RFC3339, r.SignedAt); err != nil {
		return errors.New("signed_at must be RFC3339")
	}
	if len(trustedAuthorities) == 0 {
		return errors.New("trusted_authorities is required to verify shared registries")
	}
	expectedKey, ok := trustedAuthorities[normalizeFoldKey(r.AuthorityID)]
	if !ok {
		return fmt.Errorf("authority %q is not trusted", r.AuthorityID)
	}
	if expectedKey != r.PublicKey {
		return fmt.Errorf("authority %q public key mismatch", r.AuthorityID)
	}
	publicKey, err := decodeHexKey(r.PublicKey, ed25519.PublicKeySize, "public_key")
	if err != nil {
		return err
	}
	signature, err := decodeHexKey(r.Signature, ed25519.SignatureSize, "signature")
	if err != nil {
		return err
	}
	payload, err := r.payloadBytes()
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), payload, signature) {
		return errors.New("registry signature verification failed")
	}
	return nil
}

func (r SignedWriterRegistry) payloadBytes() ([]byte, error) {
	payload := unsignedWriterRegistry{
		Version:               strings.TrimSpace(r.Version),
		Scope:                 strings.TrimSpace(r.Scope),
		AuthorityID:           strings.TrimSpace(r.AuthorityID),
		KeyType:               strings.TrimSpace(r.KeyType),
		PublicKey:             strings.ToLower(strings.TrimSpace(r.PublicKey)),
		SignedAt:              strings.TrimSpace(r.SignedAt),
		AgentCapabilities:     normalizeCapabilityMap(r.AgentCapabilities, false),
		PublicKeyCapabilities: normalizeCapabilityMap(r.PublicKeyCapabilities, true),
		RelayPeerTrust:        normalizeRelayTrustMap(r.RelayPeerTrust, false),
		RelayHostTrust:        normalizeRelayTrustMap(r.RelayHostTrust, true),
	}
	return json.Marshal(payload)
}

func loadSignedWriterRegistrySource(source string) (SignedWriterRegistry, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return SignedWriterRegistry{}, errors.New("empty shared registry source")
	}
	var data []byte
	var err error
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		req, reqErr := http.NewRequest(http.MethodGet, source, nil)
		if reqErr != nil {
			return SignedWriterRegistry{}, reqErr
		}
		resp, respErr := registryHTTPClient.Do(req)
		if respErr != nil {
			return SignedWriterRegistry{}, respErr
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return SignedWriterRegistry{}, fmt.Errorf("fetch shared registry %s: status %d", source, resp.StatusCode)
		}
		data, err = io.ReadAll(resp.Body)
		if err != nil {
			return SignedWriterRegistry{}, err
		}
	} else {
		data, err = os.ReadFile(source)
		if err != nil {
			return SignedWriterRegistry{}, err
		}
	}
	var registry SignedWriterRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return SignedWriterRegistry{}, fmt.Errorf("parse shared registry %s: %w", source, err)
	}
	registry.Normalize()
	return registry, nil
}

func mergeRegistryCapabilities(dst, src map[string]WriterCapability) map[string]WriterCapability {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]WriterCapability, len(src))
	}
	for key, capability := range src {
		dst[key] = capability
	}
	return dst
}

func mergeRegistryRelayTrust(dst, src map[string]RelayTrust) map[string]RelayTrust {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]RelayTrust, len(src))
	}
	for key, trust := range src {
		dst[key] = trust
	}
	return dst
}

func decodeHexKey(raw string, size int, label string) ([]byte, error) {
	value, err := hex.DecodeString(strings.ToLower(strings.TrimSpace(raw)))
	if err != nil {
		return nil, fmt.Errorf("%s must be lowercase hex: %w", label, err)
	}
	if len(value) != size {
		return nil, fmt.Errorf("%s must be %d bytes", label, size)
	}
	return value, nil
}
