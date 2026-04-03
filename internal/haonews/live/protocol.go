package live

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	ProtocolVersion = "haonews-live/0.1"
	TopicPrefix     = "haonews/live/"
	RosterSuffix    = "/roster"
	GlobalNamespace = "haonews/live"
	RoomsTopic      = "haonews/live/rooms"
	EventsTopic     = "haonews/live/events"
	// Live room pages/APIs default to a bounded visible non-heartbeat window.
	LiveRoomDisplayNonHeartbeatEvents = 500
	// Local room stores retain only a small heartbeat window for presence.
	LiveRoomRetainHeartbeatEvents = 20
)

const (
	TypeMessage       = "message"
	TypeJoin          = "join"
	TypeLeave         = "leave"
	TypeHeartbeat     = "heartbeat"
	TypeTaskUpdate    = "task_update"
	TypeArchiveNotice = "archive_notice"
	TypeRoomAnnounce  = "room_announce"
)

type LivePayload struct {
	Content     string         `json:"content,omitempty"`
	ContentType string         `json:"content_type,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type LiveMessage struct {
	Protocol     string      `json:"protocol"`
	Type         string      `json:"type"`
	RoomID       string      `json:"room_id"`
	Sender       string      `json:"sender"`
	SenderPubKey string      `json:"sender_pubkey"`
	Seq          uint64      `json:"seq"`
	Timestamp    string      `json:"timestamp"`
	ParentSeq    uint64      `json:"parent_seq,omitempty"`
	Payload      LivePayload `json:"payload"`
	Signature    string      `json:"signature"`
}

type RoomInfo struct {
	RoomID          string   `json:"room_id"`
	Title           string   `json:"title"`
	Creator         string   `json:"creator"`
	CreatorPubKey   string   `json:"creator_pubkey,omitempty"`
	ParentPublicKey string   `json:"parent_public_key,omitempty"`
	CreatedAt       string   `json:"created_at"`
	NetworkID       string   `json:"network_id,omitempty"`
	Channel         string   `json:"channel,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Description     string   `json:"description,omitempty"`
}

func RoomTopic(roomID string) string {
	return TopicPrefix + strings.TrimSpace(roomID)
}

func RosterTopic(roomID string) string {
	return RoomTopic(roomID) + RosterSuffix
}

func RoomNamespace(roomID string) string {
	return RoomTopic(roomID)
}

func RoomAnnounceTopic() string {
	return RoomsTopic
}

func LiveEventsTopic() string {
	return EventsTopic
}

func GenerateRoomID(creator string) (string, error) {
	short := strings.TrimSpace(creator)
	short = strings.TrimPrefix(short, "agent://")
	if idx := strings.LastIndex(short, "/"); idx >= 0 {
		short = short[idx+1:]
	}
	if short == "" {
		short = "anon"
	}
	if len(short) > 16 {
		short = short[:16]
	}
	buf := make([]byte, 2)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate room id entropy: %w", err)
	}
	return fmt.Sprintf("%s-%s-%s", short, time.Now().UTC().Format("20060102"), hex.EncodeToString(buf)), nil
}

func ValidateMessage(msg LiveMessage) error {
	if strings.TrimSpace(msg.Protocol) != ProtocolVersion {
		return fmt.Errorf("unsupported live protocol %q", msg.Protocol)
	}
	if strings.TrimSpace(msg.Type) == "" {
		return fmt.Errorf("type is required")
	}
	switch msg.Type {
	case TypeMessage, TypeJoin, TypeLeave, TypeHeartbeat, TypeTaskUpdate, TypeArchiveNotice, TypeRoomAnnounce:
	default:
		return fmt.Errorf("unknown live message type %q", msg.Type)
	}
	if strings.TrimSpace(msg.RoomID) == "" {
		return fmt.Errorf("room_id is required")
	}
	if strings.TrimSpace(msg.Sender) == "" {
		return fmt.Errorf("sender is required")
	}
	if strings.TrimSpace(msg.SenderPubKey) == "" {
		return fmt.Errorf("sender_pubkey is required")
	}
	if strings.TrimSpace(msg.Timestamp) == "" {
		return fmt.Errorf("timestamp is required")
	}
	if _, err := time.Parse(time.RFC3339, msg.Timestamp); err != nil {
		return fmt.Errorf("timestamp must be RFC3339: %w", err)
	}
	return nil
}
