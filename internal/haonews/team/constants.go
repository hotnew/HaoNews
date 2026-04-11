package team

// Member roles.
const (
	MemberRoleOwner      = "owner"
	MemberRoleMaintainer = "maintainer"
	MemberRoleMember     = "member"
	MemberRoleObserver   = "observer"
)

// Member statuses.
const (
	MemberStatusActive  = "active"
	MemberStatusPending = "pending"
	MemberStatusMuted   = "muted"
	MemberStatusRemoved = "removed"
)

// Task priorities.
const (
	TaskPriorityLow    = "low"
	TaskPriorityMedium = "medium"
	TaskPriorityHigh   = "high"
)

// Artifact kinds.
const (
	ArtifactKindMarkdown      = "markdown"
	ArtifactKindJSON          = "json"
	ArtifactKindLink          = "link"
	ArtifactKindPost          = "post"
	ArtifactKindSkillDoc      = "skill-doc"
	ArtifactKindPlanSummary   = "plan-summary"
	ArtifactKindReviewSummary = "review-summary"
	ArtifactKindIncident      = "incident-summary"
	ArtifactKindHandoff       = "handoff-summary"
	ArtifactKindArtifactBrief = "artifact-brief"
	ArtifactKindDecisionNote  = "decision-note"
)

func DefaultPolicy() Policy {
	return normalizePolicy(Policy{
		MessageRoles:    []string{MemberRoleOwner, MemberRoleMaintainer, MemberRoleMember},
		TaskRoles:       []string{MemberRoleOwner, MemberRoleMaintainer, MemberRoleMember},
		SystemNoteRoles: []string{MemberRoleOwner, MemberRoleMaintainer},
	})
}
