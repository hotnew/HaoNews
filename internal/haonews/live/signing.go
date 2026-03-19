package live

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"hao.news/internal/haonews"
)

type signableMessage struct {
	Protocol     string      `json:"protocol"`
	Type         string      `json:"type"`
	RoomID       string      `json:"room_id"`
	Sender       string      `json:"sender"`
	SenderPubKey string      `json:"sender_pubkey"`
	Seq          uint64      `json:"seq"`
	Timestamp    string      `json:"timestamp"`
	ParentSeq    uint64      `json:"parent_seq,omitempty"`
	Payload      LivePayload `json:"payload"`
}

func NewSignedMessage(identity haonews.AgentIdentity, author, roomID, messageType string, seq, parentSeq uint64, payload LivePayload) (LiveMessage, error) {
	signingIdentity, _, err := haonews.ResolveSigningIdentity(identity, author, nil)
	if err != nil {
		return LiveMessage{}, err
	}
	msg := LiveMessage{
		Protocol:     ProtocolVersion,
		Type:         strings.TrimSpace(messageType),
		RoomID:       strings.TrimSpace(roomID),
		Sender:       strings.TrimSpace(author),
		SenderPubKey: strings.ToLower(strings.TrimSpace(signingIdentity.PublicKey)),
		Seq:          seq,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		ParentSeq:    parentSeq,
		Payload:      payload,
	}
	if err := SignMessage(&msg, signingIdentity); err != nil {
		return LiveMessage{}, err
	}
	return msg, nil
}

func SignMessage(msg *LiveMessage, identity haonews.AgentIdentity) error {
	if msg == nil {
		return fmt.Errorf("message is required")
	}
	if strings.TrimSpace(identity.PrivateKey) == "" {
		return fmt.Errorf("identity does not contain a private key")
	}
	msg.Protocol = ProtocolVersion
	msg.SenderPubKey = strings.ToLower(strings.TrimSpace(identity.PublicKey))
	body, err := json.Marshal(signableMessage{
		Protocol:     msg.Protocol,
		Type:         strings.TrimSpace(msg.Type),
		RoomID:       strings.TrimSpace(msg.RoomID),
		Sender:       strings.TrimSpace(msg.Sender),
		SenderPubKey: strings.ToLower(strings.TrimSpace(msg.SenderPubKey)),
		Seq:          msg.Seq,
		Timestamp:    strings.TrimSpace(msg.Timestamp),
		ParentSeq:    msg.ParentSeq,
		Payload:      msg.Payload,
	})
	if err != nil {
		return fmt.Errorf("marshal live payload: %w", err)
	}
	privateKey, err := hex.DecodeString(strings.ToLower(strings.TrimSpace(identity.PrivateKey)))
	if err != nil {
		return fmt.Errorf("decode live private key: %w", err)
	}
	if len(privateKey) != ed25519.PrivateKeySize {
		return fmt.Errorf("live private key must be %d bytes", ed25519.PrivateKeySize)
	}
	msg.Signature = hex.EncodeToString(ed25519.Sign(ed25519.PrivateKey(privateKey), body))
	return nil
}

func VerifyMessage(msg LiveMessage) error {
	if err := ValidateMessage(msg); err != nil {
		return err
	}
	publicKey, err := hex.DecodeString(strings.ToLower(strings.TrimSpace(msg.SenderPubKey)))
	if err != nil {
		return fmt.Errorf("decode live public key: %w", err)
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("live public key must be %d bytes", ed25519.PublicKeySize)
	}
	signature, err := hex.DecodeString(strings.ToLower(strings.TrimSpace(msg.Signature)))
	if err != nil {
		return fmt.Errorf("decode live signature: %w", err)
	}
	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("live signature must be %d bytes", ed25519.SignatureSize)
	}
	body, err := json.Marshal(signableMessage{
		Protocol:     msg.Protocol,
		Type:         strings.TrimSpace(msg.Type),
		RoomID:       strings.TrimSpace(msg.RoomID),
		Sender:       strings.TrimSpace(msg.Sender),
		SenderPubKey: strings.ToLower(strings.TrimSpace(msg.SenderPubKey)),
		Seq:          msg.Seq,
		Timestamp:    strings.TrimSpace(msg.Timestamp),
		ParentSeq:    msg.ParentSeq,
		Payload:      msg.Payload,
	})
	if err != nil {
		return fmt.Errorf("marshal live payload for verify: %w", err)
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), body, signature) {
		return fmt.Errorf("live signature verification failed")
	}
	return nil
}

