package haonews

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	ProtocolVersion = "haonews/0.1"
	MessageFileName = "haonews-message.json"
	BodyFileName    = "body.txt"
)

type Message struct {
	Protocol   string         `json:"protocol"`
	Kind       string         `json:"kind"`
	Author     string         `json:"author"`
	CreatedAt  string         `json:"created_at"`
	Channel    string         `json:"channel,omitempty"`
	Title      string         `json:"title,omitempty"`
	BodyFile   string         `json:"body_file"`
	BodySHA256 string         `json:"body_sha256"`
	ReplyTo    *MessageLink   `json:"reply_to,omitempty"`
	Tags       []string       `json:"tags,omitempty"`
	Origin     *MessageOrigin `json:"origin,omitempty"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

type MessageLink struct {
	InfoHash string `json:"infohash,omitempty"`
	Magnet   string `json:"magnet,omitempty"`
}

type MessageOrigin struct {
	Author    string `json:"author"`
	AgentID   string `json:"agent_id"`
	KeyType   string `json:"key_type"`
	PublicKey string `json:"public_key"`
	Signature string `json:"signature"`
}

type MessageInput struct {
	Kind       string
	Author     string
	Channel    string
	Title      string
	Body       string
	ReplyTo    *MessageLink
	Tags       []string
	Identity   *AgentIdentity
	Extensions map[string]any
	CreatedAt  time.Time
}

func (in MessageInput) Validate() error {
	if strings.TrimSpace(in.Author) == "" {
		return errors.New("author is required")
	}
	if strings.TrimSpace(in.Body) == "" {
		return errors.New("body is required")
	}
	if strings.TrimSpace(in.Kind) == "" {
		in.Kind = "post"
	}
	return nil
}

func BuildMessage(in MessageInput) (Message, []byte, error) {
	if err := in.Validate(); err != nil {
		return Message{}, nil, err
	}
	if in.CreatedAt.IsZero() {
		in.CreatedAt = time.Now().UTC()
	}
	bodyBytes := []byte(in.Body)
	sum := sha256.Sum256(bodyBytes)
	msg := Message{
		Protocol:   ProtocolVersion,
		Kind:       defaultKind(in.Kind),
		Author:     strings.TrimSpace(in.Author),
		CreatedAt:  in.CreatedAt.UTC().Format(time.RFC3339),
		Channel:    strings.TrimSpace(in.Channel),
		Title:      strings.TrimSpace(in.Title),
		BodyFile:   BodyFileName,
		BodySHA256: hex.EncodeToString(sum[:]),
		ReplyTo:    canonicalMessageLink(in.ReplyTo),
		Tags:       cleanTags(in.Tags),
		Extensions: cloneMap(in.Extensions),
	}
	if in.Identity != nil {
		origin, signedExtensions, err := BuildSignedOrigin(msg, *in.Identity)
		if err != nil {
			return Message{}, nil, err
		}
		msg.Extensions = signedExtensions
		msg.Origin = origin
	}
	return msg, bodyBytes, nil
}

func WriteMessage(dir string, msg Message, body []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	bodyPath := filepath.Join(dir, BodyFileName)
	if err := os.WriteFile(bodyPath, body, 0o644); err != nil {
		return err
	}
	messagePath := filepath.Join(dir, MessageFileName)
	data, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(messagePath, data, 0o644)
}

func LoadMessage(dir string) (Message, string, error) {
	data, err := os.ReadFile(filepath.Join(dir, MessageFileName))
	if err != nil {
		return Message{}, "", err
	}
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return Message{}, "", err
	}
	bodyPath, err := resolveBodyFilePath(dir, msg.BodyFile)
	if err != nil {
		return Message{}, "", err
	}
	bodyBytes, err := os.ReadFile(bodyPath)
	if err != nil {
		return Message{}, "", err
	}
	if err := ValidateMessage(msg, bodyBytes); err != nil {
		return Message{}, "", err
	}
	return msg, string(bodyBytes), nil
}

func resolveBodyFilePath(dir, bodyFile string) (string, error) {
	bodyFile = strings.TrimSpace(bodyFile)
	if err := validateBodyFilePath(bodyFile); err != nil {
		return "", fmt.Errorf("invalid body_file: %w", err)
	}
	baseDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve message dir: %w", err)
	}
	bodyPath, err := filepath.Abs(filepath.Join(baseDir, bodyFile))
	if err != nil {
		return "", fmt.Errorf("resolve body path: %w", err)
	}
	if bodyPath != filepath.Join(baseDir, bodyFile) {
		return "", fmt.Errorf("body_file escapes message dir: %s", bodyFile)
	}
	return bodyPath, nil
}

func validateBodyFilePath(bodyFile string) error {
	if bodyFile == "" {
		return errors.New("body_file is empty")
	}
	if filepath.IsAbs(bodyFile) {
		return errors.New("body_file must be relative")
	}
	if bodyFile != filepath.Base(bodyFile) {
		return errors.New("body_file must be a plain file name")
	}
	if strings.Contains(bodyFile, "..") {
		return errors.New("body_file must not contain '..'")
	}
	if strings.ContainsAny(bodyFile, `/\`) {
		return errors.New("body_file must not contain path separators")
	}
	return nil
}

func ValidateMessage(msg Message, body []byte) error {
	if msg.Protocol != ProtocolVersion {
		return errors.New("unsupported protocol version")
	}
	if strings.TrimSpace(msg.Author) == "" {
		return errors.New("author is required")
	}
	if msg.BodyFile == "" {
		return errors.New("body_file is required")
	}
	if _, err := time.Parse(time.RFC3339, msg.CreatedAt); err != nil {
		return errors.New("created_at must be RFC3339")
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != msg.BodySHA256 {
		return errors.New("body_sha256 mismatch")
	}
	if err := ValidateMessageOrigin(msg); err != nil {
		return err
	}
	return nil
}

func cleanTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func defaultKind(kind string) string {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return "post"
	}
	return kind
}

func cloneMap(value map[string]any) map[string]any {
	if len(value) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}
