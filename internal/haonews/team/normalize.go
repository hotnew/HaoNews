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

func normalizeMemberRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "owner":
		return "owner"
	case "maintainer":
		return "maintainer"
	case "observer":
		return "observer"
	case "member":
		return "member"
	default:
		return "member"
	}
}

func normalizeMemberStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pending":
		return "pending"
	case "muted":
		return "muted"
	case "removed":
		return "removed"
	case "active":
		return "active"
	default:
		return "active"
	}
}

func normalizeArtifactKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "markdown":
		return "markdown"
	case "json":
		return "json"
	case "link":
		return "link"
	case "post":
		return "post"
	case "skill-doc":
		return "skill-doc"
	case "plan-summary":
		return "plan-summary"
	case "review-summary":
		return "review-summary"
	default:
		return "markdown"
	}
}

func normalizeTaskStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "open":
		return strings.TrimSpace(strings.ToLower(value))
	case "todo":
		return "open"
	case "doing", "in-progress", "in_progress", "progress":
		return "doing"
	case "blocked", "hold":
		return "blocked"
	case "review", "reviewing":
		return "review"
	case "done", "closed", "complete", "completed":
		return "done"
	default:
		return strings.TrimSpace(strings.ToLower(value))
	}
}

func normalizeTaskPriority(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "low", "medium", "high":
		return strings.TrimSpace(strings.ToLower(value))
	case "med", "normal":
		return "medium"
	case "urgent", "critical":
		return "high"
	default:
		return strings.TrimSpace(strings.ToLower(value))
	}
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
