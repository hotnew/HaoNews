package team

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Store) loadPolicySnapshotNoCtx(teamID string) (Policy, time.Time, error) {
	policy, err := s.loadPolicyNoCtx(teamID)
	if err != nil {
		return Policy{}, time.Time{}, err
	}
	return policy, policySnapshotVersion(policy), nil
}

func (s *Store) loadPolicyNoCtx(teamID string) (Policy, error) {
	if s == nil {
		return Policy{}, NewNilStoreError("Store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return Policy{}, NewEmptyIDError("team_id")
	}
	path := filepath.Join(s.root, teamID, "policy.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaultPolicy(), nil
	}
	if err != nil {
		return Policy{}, err
	}
	var policy Policy
	if err := json.Unmarshal(data, &policy); err != nil {
		return Policy{}, err
	}
	return normalizePolicy(policy), nil
}

func (s *Store) savePolicyNoCtx(teamID string, policy Policy) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	policy = normalizePolicy(policy)
	if policy.UpdatedAt.IsZero() {
		policy.UpdatedAt = time.Now().UTC()
	}
	err := s.withTeamLock(teamID, func() error {
		path := filepath.Join(s.root, teamID, "policy.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		body, err := json.MarshalIndent(policy, "", "  ")
		if err != nil {
			return err
		}
		body = append(body, '\n')
		return os.WriteFile(path, body, 0o644)
	})
	if err == nil {
		s.publish(TeamEvent{
			TeamID: teamID,
			Kind:   "policy",
			Action: "update",
		})
	}
	return err
}

func normalizePolicy(policy Policy) Policy {
	policy.MessageRoles = normalizePolicyRoles(policy.MessageRoles, []string{MemberRoleOwner, MemberRoleMaintainer, MemberRoleMember})
	policy.TaskRoles = normalizePolicyRoles(policy.TaskRoles, []string{MemberRoleOwner, MemberRoleMaintainer, MemberRoleMember})
	policy.SystemNoteRoles = normalizePolicyRoles(policy.SystemNoteRoles, []string{MemberRoleOwner, MemberRoleMaintainer})
	policy.Permissions = normalizePolicyPermissions(policy.Permissions)
	policy.TaskTransitions = normalizeTaskTransitions(policy.TaskTransitions)
	return policy
}

func defaultPolicy() Policy {
	return DefaultPolicy()
}

func normalizePolicyRoles(values []string, defaults []string) []string {
	if len(values) == 0 {
		values = defaults
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		role := normalizeMemberRole(value)
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		out = append(out, role)
	}
	if len(out) == 0 {
		return append([]string(nil), defaults...)
	}
	return out
}

func normalizePolicyPermissions(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string][]string, len(values))
	for action, roles := range values {
		action = normalizePolicyAction(action)
		if action == "" {
			continue
		}
		out[action] = normalizePolicyRoles(roles, nil)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizePolicyAction(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeWebhookConfigs(values []PushNotificationConfig) []PushNotificationConfig {
	if len(values) == 0 {
		return nil
	}
	out := make([]PushNotificationConfig, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, cfg := range values {
		cfg.URL = strings.TrimSpace(cfg.URL)
		if cfg.URL == "" {
			continue
		}
		cfg.Token = strings.TrimSpace(cfg.Token)
		cfg.Events = normalizeNonEmptyStrings(cfg.Events)
		if cfg.UpdatedAt.IsZero() {
			cfg.UpdatedAt = time.Now().UTC()
		}
		cfg.WebhookID = strings.TrimSpace(cfg.WebhookID)
		if cfg.WebhookID == "" {
			cfg.WebhookID = "webhook-" + cfg.UpdatedAt.UTC().Format("20060102T150405.000000000Z")
		}
		if _, ok := seen[cfg.WebhookID]; ok {
			continue
		}
		seen[cfg.WebhookID] = struct{}{}
		out = append(out, cfg)
	}
	return out
}

func (p Policy) Allows(action, role string) bool {
	action = normalizePolicyAction(action)
	role = normalizeMemberRole(role)
	if action == "" || role == "" {
		return false
	}
	if len(p.Permissions) > 0 {
		if roles, ok := p.Permissions[action]; ok {
			return containsRole(roles, role)
		}
	}
	return p.legacyAllows(action, role)
}

func (p Policy) legacyAllows(action, role string) bool {
	switch {
	case action == ActionMessageSend:
		return containsRole(p.MessageRoles, role)
	case strings.HasPrefix(action, "task."):
		return containsRole(p.TaskRoles, role)
	case strings.HasPrefix(action, "artifact."):
		return containsRole(p.TaskRoles, role)
	case strings.HasPrefix(action, "member."):
		return containsRole(p.SystemNoteRoles, role)
	case strings.HasPrefix(action, "channel."):
		return containsRole(p.SystemNoteRoles, role)
	case action == "policy.update":
		return containsRole(p.SystemNoteRoles, role)
	case action == "sync.conflict.resolve":
		return containsRole(p.SystemNoteRoles, role)
	case action == "archive.create":
		return containsRole(p.SystemNoteRoles, role)
	case action == "agent_card.register":
		return containsRole(p.SystemNoteRoles, role)
	default:
		return false
	}
}

func containsRole(values []string, role string) bool {
	role = normalizeMemberRole(role)
	for _, value := range values {
		if normalizeMemberRole(value) == role {
			return true
		}
	}
	return false
}
