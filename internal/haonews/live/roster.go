package live

import (
	"sort"
	"strings"
	"time"
)

type RosterEntry struct {
	Sender       string    `json:"sender"`
	SenderPubKey string    `json:"sender_pubkey"`
	JoinedAt     time.Time `json:"joined_at,omitempty"`
	LastSeen     time.Time `json:"last_seen,omitempty"`
	Online       bool      `json:"online"`
}

func BuildRoster(events []LiveMessage, now time.Time, timeout time.Duration) []RosterEntry {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	entries := map[string]*RosterEntry{}
	for _, event := range events {
		key := strings.TrimSpace(event.Sender) + "\x00" + strings.TrimSpace(event.SenderPubKey)
		if key == "\x00" {
			continue
		}
		ts, err := time.Parse(time.RFC3339, event.Timestamp)
		if err != nil {
			continue
		}
		entry := entries[key]
		if entry == nil {
			entry = &RosterEntry{
				Sender:       strings.TrimSpace(event.Sender),
				SenderPubKey: strings.TrimSpace(event.SenderPubKey),
			}
			entries[key] = entry
		}
		switch strings.TrimSpace(event.Type) {
		case TypeJoin:
			if entry.JoinedAt.IsZero() {
				entry.JoinedAt = ts
			}
			entry.LastSeen = ts
			entry.Online = true
		case TypeHeartbeat, TypeMessage, TypeTaskUpdate:
			if entry.JoinedAt.IsZero() {
				entry.JoinedAt = ts
			}
			entry.LastSeen = ts
			entry.Online = true
		case TypeLeave:
			if entry.JoinedAt.IsZero() {
				entry.JoinedAt = ts
			}
			entry.LastSeen = ts
			entry.Online = false
		}
	}
	out := make([]RosterEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.LastSeen.IsZero() && now.Sub(entry.LastSeen) > timeout {
			entry.Online = false
		}
		out = append(out, *entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Online != out[j].Online {
			return out[i].Online
		}
		if out[i].LastSeen.Equal(out[j].LastSeen) {
			return out[i].Sender < out[j].Sender
		}
		return out[i].LastSeen.After(out[j].LastSeen)
	})
	return out
}
