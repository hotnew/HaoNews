package team

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

func NormalizeTeamID(value string) string {
	value = strings.TrimSpace(value)
	if decoded, err := url.PathUnescape(value); err == nil {
		value = decoded
	}
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "\\", "-")
	value = strings.ReplaceAll(value, "/", "-")
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.ReplaceAll(value, " ", "-")
	value = strings.ReplaceAll(value, "..", "")
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	value = strings.Trim(value, "-.")
	if strings.Contains(value, "..") || filepath.IsAbs(value) {
		return ""
	}
	return value
}

func teamChannels(info Info) []string {
	if len(info.Channels) == 0 {
		return []string{"main"}
	}
	out := make([]string, 0, len(info.Channels))
	seen := make(map[string]struct{}, len(info.Channels))
	for _, channel := range info.Channels {
		channel = NormalizeTeamID(channel)
		if channel == "" {
			continue
		}
		if _, ok := seen[channel]; ok {
			continue
		}
		seen[channel] = struct{}{}
		out = append(out, channel)
	}
	if len(out) == 0 {
		out = append(out, "main")
	}
	return out
}

func defaultChannel(channelID string) Channel {
	channelID = normalizeChannelID(channelID)
	return Channel{
		ChannelID: channelID,
		Title:     channelID,
	}
}

func normalizeChannel(channel Channel) Channel {
	channel.ChannelID = normalizeChannelID(channel.ChannelID)
	channel.Title = strings.TrimSpace(channel.Title)
	channel.Description = strings.TrimSpace(channel.Description)
	if channel.Title == "" {
		channel.Title = channel.ChannelID
	}
	return channel
}

func mergeChannel(base, override Channel) Channel {
	base = normalizeChannel(base)
	override = normalizeChannel(override)
	if base.ChannelID == "" {
		base.ChannelID = override.ChannelID
	}
	if override.Title != "" {
		base.Title = override.Title
	}
	base.Description = override.Description
	base.Hidden = override.Hidden
	if base.CreatedAt.IsZero() {
		base.CreatedAt = override.CreatedAt
	}
	if !override.CreatedAt.IsZero() && (base.CreatedAt.IsZero() || override.CreatedAt.Before(base.CreatedAt)) {
		base.CreatedAt = override.CreatedAt
	}
	if !override.UpdatedAt.IsZero() {
		base.UpdatedAt = override.UpdatedAt
	}
	return normalizeChannel(base)
}

func normalizeChannelID(value string) string {
	value = NormalizeTeamID(value)
	if value == "" {
		return "main"
	}
	return value
}

func normalizeContextID(value string) string {
	return strings.TrimSpace(value)
}

func generateContextID(teamID string) string {
	return fmt.Sprintf("%s-%s", NormalizeTeamID(teamID), time.Now().UTC().Format("20060102T150405.000000000Z"))
}

func structuredDataContextID(values map[string]any) string {
	if len(values) == 0 {
		return ""
	}
	value, ok := values["context_id"]
	if !ok || value == nil {
		return ""
	}
	return normalizeContextID(fmt.Sprint(value))
}

func normalizeParentMessageID(value string) string {
	return strings.TrimSpace(value)
}

func normalizeMemberRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case MemberRoleOwner:
		return MemberRoleOwner
	case MemberRoleMaintainer:
		return MemberRoleMaintainer
	case MemberRoleObserver:
		return MemberRoleObserver
	case MemberRoleMember:
		return MemberRoleMember
	default:
		return MemberRoleMember
	}
}

func normalizeMemberStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case MemberStatusPending:
		return MemberStatusPending
	case MemberStatusMuted:
		return MemberStatusMuted
	case MemberStatusRemoved:
		return MemberStatusRemoved
	case MemberStatusActive:
		return MemberStatusActive
	default:
		return MemberStatusActive
	}
}

func normalizeArtifactKind(value string) string {
	raw := strings.TrimSpace(value)
	switch strings.ToLower(raw) {
	case ArtifactKindMarkdown:
		return ArtifactKindMarkdown
	case ArtifactKindJSON:
		return ArtifactKindJSON
	case ArtifactKindLink:
		return ArtifactKindLink
	case ArtifactKindPost:
		return ArtifactKindPost
	case ArtifactKindSkillDoc:
		return ArtifactKindSkillDoc
	case ArtifactKindPlanSummary:
		return ArtifactKindPlanSummary
	case ArtifactKindReviewSummary:
		return ArtifactKindReviewSummary
	case ArtifactKindIncident:
		return ArtifactKindIncident
	case ArtifactKindHandoff:
		return ArtifactKindHandoff
	case ArtifactKindArtifactBrief:
		return ArtifactKindArtifactBrief
	case ArtifactKindDecisionNote:
		return ArtifactKindDecisionNote
	default:
		if raw == "" {
			return ArtifactKindMarkdown
		}
		return strings.ToLower(raw)
	}
}

func normalizeTaskStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", TaskStateOpen:
		return strings.TrimSpace(strings.ToLower(value))
	case "todo":
		return TaskStateOpen
	case TaskStateDispatched:
		return TaskStateDispatched
	case TaskStateDoing, "in-progress", "in_progress", "progress":
		return TaskStateDoing
	case TaskStateBlocked, "hold":
		return TaskStateBlocked
	case TaskStateReview, "reviewing":
		return TaskStateReview
	case TaskStateDone, "closed", "complete", "completed":
		return TaskStateDone
	default:
		return strings.TrimSpace(strings.ToLower(value))
	}
}

func normalizeTaskRefID(value string) string {
	return strings.TrimSpace(value)
}

func normalizeTaskRefList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = normalizeTaskRefID(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeTaskPriority(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", TaskPriorityLow, TaskPriorityMedium, TaskPriorityHigh:
		return strings.TrimSpace(strings.ToLower(value))
	case "med", "normal":
		return TaskPriorityMedium
	case "urgent", "critical":
		return TaskPriorityHigh
	default:
		return strings.TrimSpace(strings.ToLower(value))
	}
}

func normalizeMilestoneID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return NormalizeTeamID(value)
}

func sanitizeArchiveID(value string) string {
	value = strings.TrimSpace(value)
	if decoded, err := url.PathUnescape(value); err == nil {
		value = decoded
	}
	value = strings.ReplaceAll(value, "/", "-")
	value = strings.ReplaceAll(value, "\\", "-")
	value = strings.ReplaceAll(value, " ", "-")
	value = strings.ReplaceAll(value, "..", "")
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	value = strings.Trim(value, "-.")
	if strings.Contains(value, "..") || filepath.IsAbs(value) {
		return ""
	}
	return value
}
