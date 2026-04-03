package team

import (
	"errors"
	"strings"
	"time"
)

const (
	TeamSyncTypeMessage = "message"
	TeamSyncTypeHistory = "history"
)

type TeamSyncMessage struct {
	Type       string       `json:"type"`
	TeamID     string       `json:"team_id"`
	Message    *Message     `json:"message,omitempty"`
	History    *ChangeEvent `json:"history,omitempty"`
	SourceNode string       `json:"source_node,omitempty"`
	CreatedAt  time.Time    `json:"created_at,omitempty"`
}

func (m TeamSyncMessage) Normalize() TeamSyncMessage {
	m.Type = strings.ToLower(strings.TrimSpace(m.Type))
	m.TeamID = NormalizeTeamID(m.TeamID)
	m.SourceNode = strings.TrimSpace(m.SourceNode)
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	if m.Message != nil {
		msg := *m.Message
		msg.TeamID = m.TeamID
		msg.ChannelID = normalizeChannelID(msg.ChannelID)
		msg.ContextID = normalizeContextID(msg.ContextID)
		if strings.TrimSpace(msg.MessageID) == "" {
			msg.MessageID = buildMessageID(msg)
		}
		m.Message = &msg
	}
	if m.History != nil {
		event := *m.History
		event.TeamID = m.TeamID
		event.Scope = strings.TrimSpace(event.Scope)
		event.Action = strings.TrimSpace(event.Action)
		event.Source = strings.TrimSpace(event.Source)
		event.Diff = normalizeFieldDiffs(event.Diff)
		if event.CreatedAt.IsZero() {
			event.CreatedAt = m.CreatedAt
		}
		if strings.TrimSpace(event.EventID) == "" {
			event.EventID = buildChangeEventID(event)
		}
		m.History = &event
	}
	return m
}

func (m TeamSyncMessage) Key() string {
	switch strings.ToLower(strings.TrimSpace(m.Type)) {
	case TeamSyncTypeMessage:
		if m.Message != nil {
			return TeamSyncTypeMessage + ":" + strings.TrimSpace(m.Message.MessageID)
		}
	case TeamSyncTypeHistory:
		if m.History != nil {
			return TeamSyncTypeHistory + ":" + strings.TrimSpace(m.History.EventID)
		}
	}
	return ""
}

func (s *Store) ApplyReplicatedSync(sync TeamSyncMessage) (bool, error) {
	if s == nil {
		return false, errors.New("nil team store")
	}
	sync = sync.Normalize()
	if sync.TeamID == "" {
		return false, errors.New("empty team id")
	}
	switch sync.Type {
	case TeamSyncTypeMessage:
		if sync.Message == nil {
			return false, errors.New("missing team sync message payload")
		}
		return s.ApplyReplicatedMessage(sync.TeamID, *sync.Message)
	case TeamSyncTypeHistory:
		if sync.History == nil {
			return false, errors.New("missing team sync history payload")
		}
		return s.ApplyReplicatedHistory(sync.TeamID, *sync.History)
	default:
		return false, errors.New("unsupported team sync type")
	}
}

func (s *Store) ApplyReplicatedMessage(teamID string, msg Message) (bool, error) {
	if s == nil {
		return false, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return false, errors.New("empty team id")
	}
	msg.TeamID = teamID
	msg.ChannelID = normalizeChannelID(msg.ChannelID)
	msg.ContextID = normalizeContextID(msg.ContextID)
	if msg.ContextID == "" && len(msg.StructuredData) > 0 {
		msg.ContextID = structuredDataContextID(msg.StructuredData)
	}
	if strings.TrimSpace(msg.MessageID) == "" {
		msg.MessageID = buildMessageID(msg)
	}
	if !verifyMessageSignature(msg) {
		return false, errors.New("replicated team message signature verification failed")
	}
	exists, err := s.HasMessageID(teamID, msg.ChannelID, msg.MessageID)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	if err := s.AppendMessage(teamID, msg); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ApplyReplicatedHistory(teamID string, event ChangeEvent) (bool, error) {
	if s == nil {
		return false, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return false, errors.New("empty team id")
	}
	event.TeamID = teamID
	event.Scope = strings.TrimSpace(event.Scope)
	if event.Scope != "message" {
		return false, errors.New("replicated team history only supports message scope")
	}
	if strings.TrimSpace(event.EventID) == "" {
		event.EventID = buildChangeEventID(event)
	}
	exists, err := s.HasHistoryEventID(teamID, event.EventID)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	if err := s.AppendHistory(teamID, event); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) HasMessageID(teamID, channelID, messageID string) (bool, error) {
	if s == nil {
		return false, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	channelID = normalizeChannelID(channelID)
	messageID = strings.TrimSpace(messageID)
	if teamID == "" || channelID == "" || messageID == "" {
		return false, nil
	}
	items, err := s.LoadMessages(teamID, channelID, 0)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if strings.TrimSpace(item.MessageID) == messageID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) HasHistoryEventID(teamID, eventID string) (bool, error) {
	if s == nil {
		return false, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	eventID = strings.TrimSpace(eventID)
	if teamID == "" || eventID == "" {
		return false, nil
	}
	items, err := s.LoadHistory(teamID, 0)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if strings.TrimSpace(item.EventID) == eventID {
			return true, nil
		}
	}
	return false, nil
}
