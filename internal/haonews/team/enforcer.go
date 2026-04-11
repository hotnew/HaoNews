package team

import (
	"context"
	"strings"
)

type PolicyEnforcer interface {
	Allow(ctx context.Context, teamID, agentID, action string) error
}

const (
	ActionMessageSend          = "message.send"
	ActionTaskCreate           = "task.create"
	ActionTaskUpdate           = "task.update"
	ActionTaskDelete           = "task.delete"
	ActionTaskTransition       = "task.transition"
	ActionArtifactCreate       = "artifact.create"
	ActionArtifactUpdate       = "artifact.update"
	ActionArtifactDelete       = "artifact.delete"
	ActionMemberInvite         = "member.invite"
	ActionMemberUpdate         = "member.update"
	ActionMemberTransition     = "member.transition"
	ActionMemberBulkTransition = "member.bulk-transition"
	ActionChannelCreate        = "channel.create"
	ActionChannelUpdate        = "channel.update"
	ActionChannelHide          = "channel.hide"
	ActionPolicyUpdate         = "policy.update"
	ActionSyncConflictResolve  = "sync.conflict.resolve"
	ActionArchiveCreate        = "archive.create"
	ActionAgentCardRegister    = "agent_card.register"
)

type StorePolicyEnforcer struct {
	store *Store
}

func NewPolicyEnforcer(store *Store) PolicyEnforcer {
	return &StorePolicyEnforcer{store: store}
}

func RequireAction(ctx context.Context, store *Store, teamID, agentID, action string) error {
	return NewPolicyEnforcer(store).Allow(ctx, teamID, agentID, action)
}

func (e *StorePolicyEnforcer) Allow(ctx context.Context, teamID, agentID, action string) error {
	if e == nil || e.store == nil {
		return NewNilStoreError("PolicyEnforcer")
	}
	teamID = NormalizeTeamID(teamID)
	action = normalizePolicyAction(action)
	if teamID == "" {
		return NewEmptyIDError("team_id")
	}
	if action == "" {
		return NewEmptyIDError("action")
	}
	info, err := e.store.LoadTeamCtx(ctx, teamID)
	if err != nil {
		return err
	}
	role, resolvedAgentID, err := e.resolveRole(ctx, teamID, agentID, info)
	if err != nil {
		return err
	}
	policy, err := e.store.LoadPolicyCtx(ctx, teamID)
	if err != nil {
		return err
	}
	if !policyAllowsAction(policy, role, action) {
		return NewForbiddenError(action, resolvedAgentID)
	}
	return nil
}

func (e *StorePolicyEnforcer) resolveRole(ctx context.Context, teamID, agentID string, info Info) (string, string, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		agentID = strings.TrimSpace(info.OwnerAgentID)
		if agentID == "" {
			return MemberRoleOwner, "", nil
		}
	}
	members, err := e.store.LoadMembersCtx(ctx, teamID)
	if err != nil {
		return "", agentID, err
	}
	role := findMemberRole(members, agentID)
	if role != "" {
		return role, agentID, nil
	}
	if agentID == strings.TrimSpace(info.OwnerAgentID) {
		return MemberRoleOwner, agentID, nil
	}
	return "", agentID, NewForbiddenError("resolve_role", agentID)
}

func findMemberRole(members []Member, agentID string) string {
	agentID = strings.TrimSpace(agentID)
	for _, member := range members {
		if strings.TrimSpace(member.AgentID) != agentID {
			continue
		}
		if normalizeMemberStatus(member.Status) != MemberStatusActive {
			continue
		}
		return normalizeMemberRole(member.Role)
	}
	return ""
}

func policyAllowsAction(policy Policy, role, action string) bool {
	role = normalizeMemberRole(role)
	if role == MemberRoleOwner {
		return true
	}
	return policy.Allows(action, role)
}
