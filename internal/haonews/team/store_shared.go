package team

import (
	"strings"
	"time"
)

func reverseBytesToString(input []byte) string {
	if len(input) == 0 {
		return ""
	}
	out := make([]byte, len(input))
	for i := range input {
		out[len(input)-1-i] = input[i]
	}
	return string(out)
}

func buildMessageID(msg Message) string {
	return strings.Join([]string{
		strings.TrimSpace(msg.TeamID),
		normalizeChannelID(msg.ChannelID),
		strings.TrimSpace(msg.AuthorAgentID),
		msg.CreatedAt.UTC().Format(time.RFC3339Nano),
		strings.TrimSpace(msg.Content),
	}, ":")
}

func normalizeNonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
