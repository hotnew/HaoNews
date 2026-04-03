package team

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type MessagePart struct {
	Kind     string         `json:"kind"`
	Text     string         `json:"text,omitempty"`
	MIMEType string         `json:"mime_type,omitempty"`
	URL      string         `json:"url,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

type Reference struct {
	RefType  string `json:"ref_type,omitempty"`
	TargetID string `json:"target_id,omitempty"`
	URL      string `json:"url,omitempty"`
	Label    string `json:"label,omitempty"`
}

func verifyMessageSignature(msg Message) bool {
	publicKeyHex := strings.TrimSpace(msg.OriginPublicKey)
	signatureHex := strings.TrimSpace(msg.Signature)
	if publicKeyHex == "" || signatureHex == "" {
		return false
	}
	publicKey, err := hex.DecodeString(publicKeyHex)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return false
	}
	signature, err := hex.DecodeString(signatureHex)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return false
	}
	payload, err := messageSignaturePayload(msg)
	if err != nil {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(publicKey), payload, signature)
}

func messageSignaturePayload(msg Message) ([]byte, error) {
	payload := struct {
		TeamID          string         `json:"team_id"`
		ChannelID       string         `json:"channel_id"`
		ContextID       string         `json:"context_id,omitempty"`
		AuthorAgentID   string         `json:"author_agent_id"`
		OriginPublicKey string         `json:"origin_public_key,omitempty"`
		ParentPublicKey string         `json:"parent_public_key,omitempty"`
		MessageType     string         `json:"message_type"`
		Content         string         `json:"content"`
		StructuredData  map[string]any `json:"structured_data,omitempty"`
		Parts           []MessagePart  `json:"parts,omitempty"`
		References      []Reference    `json:"references,omitempty"`
	}{
		TeamID:          NormalizeTeamID(msg.TeamID),
		ChannelID:       normalizeChannelID(msg.ChannelID),
		ContextID:       normalizeContextID(msg.ContextID),
		AuthorAgentID:   strings.TrimSpace(msg.AuthorAgentID),
		OriginPublicKey: strings.TrimSpace(msg.OriginPublicKey),
		ParentPublicKey: strings.TrimSpace(msg.ParentPublicKey),
		MessageType:     strings.TrimSpace(msg.MessageType),
		Content:         strings.TrimSpace(msg.Content),
		StructuredData:  normalizedStructuredDataForSignature(msg.StructuredData, msg.ContextID),
		Parts:           normalizeMessageParts(msg.Parts),
		References:      normalizeReferences(msg.References),
	}
	return json.Marshal(payload)
}

func MessageSignaturePayload(msg Message) ([]byte, error) {
	return messageSignaturePayload(msg)
}

func normalizedStructuredDataForSignature(values map[string]any, contextID string) map[string]any {
	if len(values) == 0 {
		return nil
	}
	contextID = normalizeContextID(contextID)
	out := make(map[string]any, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" || value == nil {
			continue
		}
		if key == "context_id" && contextID != "" && normalizeContextID(strings.TrimSpace(toString(value))) == contextID {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func toString(value any) string {
	return strings.TrimSpace(fmt.Sprint(value))
}

func normalizeMessageParts(parts []MessagePart) []MessagePart {
	if len(parts) == 0 {
		return nil
	}
	out := make([]MessagePart, 0, len(parts))
	for _, part := range parts {
		part.Kind = strings.TrimSpace(part.Kind)
		part.Text = strings.TrimSpace(part.Text)
		part.MIMEType = strings.TrimSpace(part.MIMEType)
		part.URL = strings.TrimSpace(part.URL)
		if part.Kind == "" && part.Text == "" && part.URL == "" && len(part.Data) == 0 {
			continue
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeReferences(references []Reference) []Reference {
	if len(references) == 0 {
		return nil
	}
	out := make([]Reference, 0, len(references))
	for _, ref := range references {
		ref.RefType = strings.TrimSpace(ref.RefType)
		ref.TargetID = strings.TrimSpace(ref.TargetID)
		ref.URL = strings.TrimSpace(ref.URL)
		ref.Label = strings.TrimSpace(ref.Label)
		if ref.RefType == "" && ref.TargetID == "" && ref.URL == "" && ref.Label == "" {
			continue
		}
		out = append(out, ref)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func validateMessageSignaturePolicy(msg Message, policy Policy) error {
	requireSignature := policy.RequireSignature
	hasSignature := strings.TrimSpace(msg.Signature) != ""
	if !requireSignature && !hasSignature {
		return nil
	}
	if !verifyMessageSignature(msg) {
		if requireSignature {
			return errors.New("team message signature verification failed")
		}
		return errors.New("invalid optional team message signature")
	}
	return nil
}
