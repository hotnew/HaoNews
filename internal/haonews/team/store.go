package team

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store struct {
	root          string
	subMu         sync.RWMutex
	subscribers   map[string]map[chan TeamEvent]struct{}
	webhookClient *http.Client
}

type Info struct {
	TeamID               string    `json:"team_id"`
	Slug                 string    `json:"slug,omitempty"`
	Title                string    `json:"title"`
	Description          string    `json:"description,omitempty"`
	Visibility           string    `json:"visibility,omitempty"`
	OwnerAgentID         string    `json:"owner_agent_id,omitempty"`
	OwnerOriginPublicKey string    `json:"owner_origin_public_key,omitempty"`
	OwnerParentPublicKey string    `json:"owner_parent_public_key,omitempty"`
	Channels             []string  `json:"channels,omitempty"`
	CreatedAt            time.Time `json:"created_at,omitempty"`
	UpdatedAt            time.Time `json:"updated_at,omitempty"`
}

type Member struct {
	AgentID         string    `json:"agent_id"`
	OriginPublicKey string    `json:"origin_public_key,omitempty"`
	ParentPublicKey string    `json:"parent_public_key,omitempty"`
	Role            string    `json:"role,omitempty"`
	Status          string    `json:"status,omitempty"`
	JoinedAt        time.Time `json:"joined_at,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
}

type Policy struct {
	MessageRoles     []string                      `json:"message_roles,omitempty"`
	TaskRoles        []string                      `json:"task_roles,omitempty"`
	SystemNoteRoles  []string                      `json:"system_note_roles,omitempty"`
	Permissions      map[string][]string           `json:"permissions,omitempty"`
	RequireSignature bool                          `json:"require_signature,omitempty"`
	TaskTransitions  map[string]TaskTransitionRule `json:"task_transitions,omitempty"`
	UpdatedAt        time.Time                     `json:"updated_at,omitempty"`
}

type Summary struct {
	Info
	MemberCount  int `json:"member_count"`
	ChannelCount int `json:"channel_count"`
}

type Channel struct {
	ChannelID   string    `json:"channel_id"`
	Title       string    `json:"title,omitempty"`
	Description string    `json:"description,omitempty"`
	Hidden      bool      `json:"hidden,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

type ChannelSummary struct {
	Channel
	MessageCount  int       `json:"message_count"`
	LastMessageAt time.Time `json:"last_message_at,omitempty"`
}

type Message struct {
	MessageID       string         `json:"message_id"`
	TeamID          string         `json:"team_id"`
	ChannelID       string         `json:"channel_id"`
	ContextID       string         `json:"context_id,omitempty"`
	Signature       string         `json:"signature,omitempty"`
	Parts           []MessagePart  `json:"parts,omitempty"`
	References      []Reference    `json:"references,omitempty"`
	AuthorAgentID   string         `json:"author_agent_id"`
	OriginPublicKey string         `json:"origin_public_key,omitempty"`
	ParentPublicKey string         `json:"parent_public_key,omitempty"`
	MessageType     string         `json:"message_type"`
	Content         string         `json:"content"`
	StructuredData  map[string]any `json:"structured_data,omitempty"`
	CreatedAt       time.Time      `json:"created_at,omitempty"`
}

type Task struct {
	TaskID          string    `json:"task_id"`
	TeamID          string    `json:"team_id"`
	ChannelID       string    `json:"channel_id,omitempty"`
	ContextID       string    `json:"context_id,omitempty"`
	Title           string    `json:"title"`
	Description     string    `json:"description,omitempty"`
	CreatedBy       string    `json:"created_by,omitempty"`
	Assignees       []string  `json:"assignees,omitempty"`
	Status          string    `json:"status,omitempty"`
	Priority        string    `json:"priority,omitempty"`
	Labels          []string  `json:"labels,omitempty"`
	OriginPublicKey string    `json:"origin_public_key,omitempty"`
	ParentPublicKey string    `json:"parent_public_key,omitempty"`
	CreatedAt       time.Time `json:"created_at,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
	ClosedAt        time.Time `json:"closed_at,omitempty"`
}

type Artifact struct {
	ArtifactID      string    `json:"artifact_id"`
	TeamID          string    `json:"team_id"`
	ChannelID       string    `json:"channel_id,omitempty"`
	TaskID          string    `json:"task_id,omitempty"`
	Title           string    `json:"title"`
	Kind            string    `json:"kind,omitempty"`
	Summary         string    `json:"summary,omitempty"`
	Content         string    `json:"content,omitempty"`
	LinkURL         string    `json:"link_url,omitempty"`
	CreatedBy       string    `json:"created_by,omitempty"`
	OriginPublicKey string    `json:"origin_public_key,omitempty"`
	ParentPublicKey string    `json:"parent_public_key,omitempty"`
	Labels          []string  `json:"labels,omitempty"`
	CreatedAt       time.Time `json:"created_at,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
}

type ChangeEvent struct {
	EventID              string               `json:"event_id"`
	TeamID               string               `json:"team_id"`
	Scope                string               `json:"scope"`
	Action               string               `json:"action"`
	SubjectID            string               `json:"subject_id,omitempty"`
	Summary              string               `json:"summary,omitempty"`
	ActorAgentID         string               `json:"actor_agent_id,omitempty"`
	ActorOriginPublicKey string               `json:"actor_origin_public_key,omitempty"`
	ActorParentPublicKey string               `json:"actor_parent_public_key,omitempty"`
	Source               string               `json:"source,omitempty"`
	Diff                 map[string]FieldDiff `json:"diff,omitempty"`
	Metadata             map[string]any       `json:"metadata,omitempty"`
	CreatedAt            time.Time            `json:"created_at,omitempty"`
}

type FieldDiff struct {
	Before any `json:"before,omitempty"`
	After  any `json:"after,omitempty"`
}

type TaskThread struct {
	Task     Task      `json:"task"`
	Messages []Message `json:"messages,omitempty"`
}

type TeamEvent struct {
	EventID   string         `json:"event_id"`
	TeamID    string         `json:"team_id"`
	Kind      string         `json:"kind"`
	Action    string         `json:"action"`
	SubjectID string         `json:"subject_id,omitempty"`
	ChannelID string         `json:"channel_id,omitempty"`
	ContextID string         `json:"context_id,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at,omitempty"`
}

type PushNotificationConfig struct {
	WebhookID string    `json:"webhook_id,omitempty"`
	URL       string    `json:"url"`
	Token     string    `json:"token,omitempty"`
	Events    []string  `json:"events,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

func OpenStore(storeRoot string) (*Store, error) {
	root := filepath.Join(strings.TrimSpace(storeRoot), "team")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &Store{
		root:        root,
		subscribers: make(map[string]map[chan TeamEvent]struct{}),
		webhookClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}, nil
}

func (s *Store) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

func (s *Store) loadTeamNoCtx(teamID string) (Info, error) {
	if s == nil {
		return Info{}, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return Info{}, errors.New("empty team id")
	}
	path := filepath.Join(s.root, teamID, "team.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Info{}, err
	}
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return Info{}, err
	}
	if strings.TrimSpace(info.TeamID) == "" {
		info.TeamID = teamID
	}
	if strings.TrimSpace(info.Slug) == "" {
		info.Slug = teamID
	}
	info.TeamID = NormalizeTeamID(info.TeamID)
	info.Slug = NormalizeTeamID(info.Slug)
	if info.TeamID == "" {
		info.TeamID = teamID
	}
	if info.Slug == "" {
		info.Slug = info.TeamID
	}
	if strings.TrimSpace(info.Visibility) == "" {
		info.Visibility = "team"
	}
	info.Channels = teamChannels(info)
	return info, nil
}

func (s *Store) loadMembersNoCtx(teamID string) ([]Member, error) {
	if s == nil {
		return nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	path := filepath.Join(s.root, teamID, "members.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var members []Member
	if err := json.Unmarshal(data, &members); err != nil {
		return nil, err
	}
	for i := range members {
		members[i].Role = normalizeMemberRole(members[i].Role)
		members[i].Status = normalizeMemberStatus(members[i].Status)
		if members[i].UpdatedAt.IsZero() {
			members[i].UpdatedAt = members[i].JoinedAt
		}
	}
	sort.SliceStable(members, func(i, j int) bool {
		if members[i].Role != members[j].Role {
			return members[i].Role < members[j].Role
		}
		return members[i].AgentID < members[j].AgentID
	})
	return members, nil
}

func (s *Store) saveMembersNoCtx(teamID string, members []Member) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	err := s.withTeamLock(teamID, func() error {
		out := make([]Member, 0, len(members))
		seen := make(map[string]struct{}, len(members))
		for _, member := range members {
			member.AgentID = strings.TrimSpace(member.AgentID)
			if member.AgentID == "" {
				continue
			}
			if _, ok := seen[member.AgentID]; ok {
				continue
			}
			seen[member.AgentID] = struct{}{}
			member.Role = normalizeMemberRole(member.Role)
			member.Status = normalizeMemberStatus(member.Status)
			if member.JoinedAt.IsZero() {
				member.JoinedAt = time.Now().UTC()
			}
			if member.UpdatedAt.IsZero() {
				member.UpdatedAt = member.JoinedAt
			}
			out = append(out, member)
		}
		path := filepath.Join(s.root, teamID, "members.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		body, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return err
		}
		body = append(body, '\n')
		return os.WriteFile(path, body, 0o644)
	})
	if err == nil {
		s.publish(TeamEvent{
			TeamID:   teamID,
			Kind:     "member",
			Action:   "replace",
			Metadata: map[string]any{"member_count": len(members)},
		})
	}
	return err
}

func (s *Store) loadMembersSnapshotNoCtx(teamID string) ([]Member, time.Time, error) {
	members, err := s.loadMembersNoCtx(teamID)
	if err != nil {
		return nil, time.Time{}, err
	}
	return members, membersSnapshotVersion(members), nil
}

func (s *Store) loadPolicySnapshotNoCtx(teamID string) (Policy, time.Time, error) {
	policy, err := s.loadPolicyNoCtx(teamID)
	if err != nil {
		return Policy{}, time.Time{}, err
	}
	return policy, policySnapshotVersion(policy), nil
}

func (s *Store) loadChannelSnapshotNoCtx(teamID, channelID string) (Channel, time.Time, error) {
	channel, err := s.loadChannelNoCtx(teamID, channelID)
	if err != nil {
		return Channel{}, time.Time{}, err
	}
	return channel, channelSnapshotVersion(channel), nil
}

func (s *Store) loadWebhookConfigsNoCtx(teamID string) ([]PushNotificationConfig, error) {
	if s == nil {
		return nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	data, err := os.ReadFile(s.webhookConfigPath(teamID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var configs []PushNotificationConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, err
	}
	return normalizeWebhookConfigs(configs), nil
}

func (s *Store) saveWebhookConfigsNoCtx(teamID string, configs []PushNotificationConfig) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	configs = normalizeWebhookConfigs(configs)
	return s.withTeamLock(teamID, func() error {
		path := s.webhookConfigPath(teamID)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		body, err := json.MarshalIndent(configs, "", "  ")
		if err != nil {
			return err
		}
		body = append(body, '\n')
		return os.WriteFile(path, body, 0o644)
	})
}

func (s *Store) loadPolicyNoCtx(teamID string) (Policy, error) {
	if s == nil {
		return Policy{}, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return Policy{}, errors.New("empty team id")
	}
	path := filepath.Join(s.root, teamID, "policy.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaultPolicy(), nil
	}
	if err != nil {
		return Policy{}, err
	}
	var policy Policy
	if err := json.Unmarshal(data, &policy); err != nil {
		return Policy{}, err
	}
	return normalizePolicy(policy), nil
}

func (s *Store) savePolicyNoCtx(teamID string, policy Policy) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	policy = normalizePolicy(policy)
	if policy.UpdatedAt.IsZero() {
		policy.UpdatedAt = time.Now().UTC()
	}
	err := s.withTeamLock(teamID, func() error {
		path := filepath.Join(s.root, teamID, "policy.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		body, err := json.MarshalIndent(policy, "", "  ")
		if err != nil {
			return err
		}
		body = append(body, '\n')
		return os.WriteFile(path, body, 0o644)
	})
	if err == nil {
		s.publish(TeamEvent{
			TeamID: teamID,
			Kind:   "policy",
			Action: "update",
		})
	}
	return err
}

func (s *Store) appendMessageNoCtx(teamID string, msg Message) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	channelID := normalizeChannelID(msg.ChannelID)
	if channelID == "" {
		channelID = "main"
	}
	if strings.TrimSpace(msg.TeamID) == "" {
		msg.TeamID = teamID
	}
	msg.TeamID = NormalizeTeamID(msg.TeamID)
	if msg.TeamID != teamID {
		return fmt.Errorf("team message team_id %q does not match %q", msg.TeamID, teamID)
	}
	msg.ChannelID = channelID
	msg.ContextID = normalizeContextID(msg.ContextID)
	if msg.ContextID == "" && len(msg.StructuredData) > 0 {
		msg.ContextID = structuredDataContextID(msg.StructuredData)
	}
	msg.Signature = strings.TrimSpace(msg.Signature)
	msg.Parts = normalizeMessageParts(msg.Parts)
	msg.References = normalizeReferences(msg.References)
	msg.MessageType = strings.TrimSpace(msg.MessageType)
	if msg.MessageType == "" {
		msg.MessageType = "chat"
	}
	msg.Content = strings.TrimSpace(msg.Content)
	if msg.Content == "" {
		return errors.New("empty team message content")
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	if msg.ContextID != "" {
		if msg.StructuredData == nil {
			msg.StructuredData = make(map[string]any, 1)
		}
		msg.StructuredData["context_id"] = msg.ContextID
	}
	if strings.TrimSpace(msg.MessageID) == "" {
		msg.MessageID = buildMessageID(msg)
	}
	err := s.withTeamLock(teamID, func() error {
		policy, err := s.loadPolicyNoCtx(teamID)
		if err != nil {
			return err
		}
		if err := validateMessageSignaturePolicy(msg, policy); err != nil {
			return err
		}
		path := s.channelPath(teamID, channelID)
		if s.isShardedChannel(teamID, channelID) {
			path = s.channelShardPath(teamID, channelID, msg.CreatedAt)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer file.Close()
		body, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		if _, err := file.Write(append(body, '\n')); err != nil {
			return err
		}
		return nil
	})
	if err == nil {
		s.publish(TeamEvent{
			TeamID:    teamID,
			Kind:      "message",
			Action:    "create",
			SubjectID: msg.MessageID,
			ChannelID: msg.ChannelID,
			ContextID: msg.ContextID,
			Metadata: map[string]any{
				"author_agent_id": msg.AuthorAgentID,
				"message_type":    msg.MessageType,
			},
		})
	}
	return err
}

func (s *Store) loadMessagesNoCtx(teamID, channelID string, limit int) ([]Message, error) {
	if s == nil {
		return nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	channelID = normalizeChannelID(channelID)
	if channelID == "" {
		channelID = "main"
	}
	if s.isShardedChannel(teamID, channelID) {
		return s.loadMessagesFromShards(teamID, channelID, limit)
	}
	path := s.channelPath(teamID, channelID)
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var out []Message
	if limit > 0 {
		lines, err := readLastJSONLLines(path, limit)
		if err != nil {
			return nil, err
		}
		out = make([]Message, 0, len(lines))
		for _, line := range lines {
			var msg Message
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				logTeamEvent("corrupt_jsonl_line", "path", path, "error", err)
				continue
			}
			out = append(out, msg)
		}
	} else {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var msg Message
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				logTeamEvent("corrupt_jsonl_line", "path", path, "error", err)
				continue
			}
			out = append(out, msg)
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].MessageID > out[j].MessageID
	})
	return out, nil
}

func (s *Store) loadMessagesFromShards(teamID, channelID string, limit int) ([]Message, error) {
	dir := s.channelShardDir(teamID, channelID)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	sort.Slice(paths, func(i, j int) bool {
		return filepath.Base(paths[i]) > filepath.Base(paths[j])
	})
	out := make([]Message, 0)
	for _, path := range paths {
		var lines []string
		if limit > 0 {
			lines, err = readLastJSONLLines(path, limit-len(out))
		} else {
			lines, err = readAllJSONLLines(path)
		}
		if err != nil {
			return nil, err
		}
		for _, line := range lines {
			var msg Message
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				logTeamEvent("corrupt_jsonl_line", "path", path, "error", err)
				continue
			}
			out = append(out, msg)
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].MessageID > out[j].MessageID
	})
	if limit > 0 && len(out) > limit {
		out = append([]Message(nil), out[:limit]...)
	}
	return out, nil
}

func readAllJSONLLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	lines := make([]string, 0, 32)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func (s *Store) MigrateChannelToShards(teamID, channelID string) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	channelID = normalizeChannelID(channelID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	return s.withTeamLock(teamID, func() error {
		legacyPath := s.channelPath(teamID, channelID)
		if s.isShardedChannel(teamID, channelID) {
			if _, err := os.Stat(legacyPath); errors.Is(err, os.ErrNotExist) {
				return nil
			}
		}
		lines, err := readAllJSONLLines(legacyPath)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, line := range lines {
			var msg Message
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				continue
			}
			if msg.CreatedAt.IsZero() {
				msg.CreatedAt = time.Now().UTC()
			}
			shardPath := s.channelShardPath(teamID, channelID, msg.CreatedAt)
			if err := os.MkdirAll(filepath.Dir(shardPath), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(shardPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			if _, err := file.Write(append([]byte(line), '\n')); err != nil {
				_ = file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
		}
		backupPath := s.channelLegacyBackupPath(teamID, channelID)
		_ = os.Remove(backupPath)
		return os.Rename(legacyPath, backupPath)
	})
}

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

func (s *Store) loadChannelNoCtx(teamID, channelID string) (Channel, error) {
	if s == nil {
		return Channel{}, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	channelID = normalizeChannelID(channelID)
	if teamID == "" {
		return Channel{}, errors.New("empty team id")
	}
	channels, err := s.loadChannelConfigs(teamID)
	if err != nil {
		return Channel{}, err
	}
	for _, channel := range channels {
		if channel.ChannelID == channelID {
			return channel, nil
		}
	}
	if _, err := os.Stat(s.channelPath(teamID, channelID)); err == nil || s.isShardedChannel(teamID, channelID) {
		return defaultChannel(channelID), nil
	}
	return Channel{}, os.ErrNotExist
}

func (s *Store) saveChannelNoCtx(teamID string, channel Channel) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	channel = normalizeChannel(channel)
	if teamID == "" {
		return errors.New("empty team id")
	}
	if channel.ChannelID == "" {
		return errors.New("empty channel id")
	}
	err := s.withTeamLock(teamID, func() error {
		channels, err := s.loadChannelConfigs(teamID)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if channel.UpdatedAt.IsZero() {
			channel.UpdatedAt = now
		}
		updated := false
		for i := range channels {
			if channels[i].ChannelID != channel.ChannelID {
				continue
			}
			if channel.CreatedAt.IsZero() {
				channel.CreatedAt = channels[i].CreatedAt
			}
			if channel.CreatedAt.IsZero() {
				channel.CreatedAt = now
			}
			channels[i] = mergeChannel(channels[i], channel)
			channels[i].UpdatedAt = channel.UpdatedAt
			updated = true
			break
		}
		if !updated {
			if channel.CreatedAt.IsZero() {
				channel.CreatedAt = now
			}
			channels = append(channels, channel)
		}
		return s.saveChannels(teamID, channels)
	})
	if err == nil {
		s.publish(TeamEvent{
			TeamID:    teamID,
			Kind:      "channel",
			Action:    "upsert",
			SubjectID: channel.ChannelID,
			ChannelID: channel.ChannelID,
		})
	}
	return err
}

func (s *Store) hideChannelNoCtx(teamID, channelID string) error {
	channel, err := s.loadChannelNoCtx(teamID, channelID)
	if err != nil {
		return err
	}
	channel.Hidden = true
	channel.UpdatedAt = time.Now().UTC()
	return s.saveChannelNoCtx(teamID, channel)
}

func (s *Store) listChannelsNoCtx(teamID string) ([]ChannelSummary, error) {
	if s == nil {
		return nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	channels, err := s.loadChannelConfigs(teamID)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(channels))
	out := make([]ChannelSummary, 0, len(channels))
	for _, channel := range channels {
		summary, err := s.channelSummary(teamID, channel.ChannelID)
		if err != nil {
			return nil, err
		}
		seen[summary.ChannelID] = struct{}{}
		out = append(out, summary)
	}
	dir := filepath.Join(s.root, teamID, "channels")
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, entry := range entries {
		channelID := ""
		if entry.IsDir() {
			channelID = normalizeChannelID(entry.Name())
		} else if strings.HasSuffix(entry.Name(), ".jsonl") {
			channelID = normalizeChannelID(strings.TrimSuffix(entry.Name(), ".jsonl"))
		}
		if channelID == "" {
			continue
		}
		if _, ok := seen[channelID]; ok {
			continue
		}
		summary, err := s.channelSummary(teamID, channelID)
		if err != nil {
			return nil, err
		}
		seen[channelID] = struct{}{}
		out = append(out, summary)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].LastMessageAt.Equal(out[j].LastMessageAt) {
			return out[i].LastMessageAt.After(out[j].LastMessageAt)
		}
		return out[i].ChannelID < out[j].ChannelID
	})
	return out, nil
}

func (s *Store) appendTaskNoCtx(teamID string, task Task) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	if strings.TrimSpace(task.TeamID) == "" {
		task.TeamID = teamID
	}
	task.TeamID = NormalizeTeamID(task.TeamID)
	if task.TeamID != teamID {
		return fmt.Errorf("team task team_id %q does not match %q", task.TeamID, teamID)
	}
	task.Title = strings.TrimSpace(task.Title)
	if task.Title == "" {
		return errors.New("empty team task title")
	}
	task.Status = normalizeTaskStatus(task.Status)
	if task.Status == "" {
		task.Status = "open"
	}
	task.Priority = normalizeTaskPriority(task.Priority)
	task.ChannelID = normalizeChannelID(task.ChannelID)
	task.ContextID = normalizeContextID(task.ContextID)
	task.Description = strings.TrimSpace(task.Description)
	task.CreatedBy = strings.TrimSpace(task.CreatedBy)
	task.Assignees = normalizeNonEmptyStrings(task.Assignees)
	task.Labels = normalizeNonEmptyStrings(task.Labels)
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now().UTC()
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = task.CreatedAt
	}
	if task.ContextID == "" {
		task.ContextID = generateContextID(teamID)
	}
	if strings.TrimSpace(task.TaskID) == "" {
		task.TaskID = buildTaskID(task)
	}
	err := s.withTeamLock(teamID, func() error {
		policy, err := s.loadPolicyNoCtx(teamID)
		if err != nil {
			return err
		}
		if !IsValidTransitionWithPolicy("", task.Status, policy) {
			return fmt.Errorf("invalid task status transition %q -> %q", "", task.Status)
		}
		if IsTerminalState(task.Status) && task.ClosedAt.IsZero() {
			task.ClosedAt = task.UpdatedAt
		}
		return s.appendTaskCurrentLocked(teamID, task)
	})
	if err == nil {
		s.publish(TeamEvent{
			TeamID:    teamID,
			Kind:      "task",
			Action:    "create",
			SubjectID: task.TaskID,
			ChannelID: task.ChannelID,
			ContextID: task.ContextID,
		})
	}
	return err
}

func (s *Store) loadTasksNoCtx(teamID string, limit int) ([]Task, error) {
	if s == nil {
		return nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	return s.loadTasksCurrent(teamID, limit)
}

func (s *Store) loadLegacyTasks(teamID string) ([]Task, error) {
	if s == nil {
		return nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	path := filepath.Join(s.root, teamID, "tasks.jsonl")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	out := make([]Task, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var task Task
		if err := json.Unmarshal([]byte(line), &task); err != nil {
			continue
		}
		out = append(out, task)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].TaskID > out[j].TaskID
	})
	return out, nil
}

func (s *Store) loadTaskNoCtx(teamID, taskID string) (Task, error) {
	if s == nil {
		return Task{}, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	taskID = strings.TrimSpace(taskID)
	if teamID == "" {
		return Task{}, errors.New("empty team id")
	}
	if taskID == "" {
		return Task{}, errors.New("empty task id")
	}
	return s.loadTaskCurrent(teamID, taskID)
}

func (s *Store) saveTaskNoCtx(teamID string, task Task) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	taskID := strings.TrimSpace(task.TaskID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	if taskID == "" {
		return errors.New("empty task id")
	}
	err := s.withTeamLock(teamID, func() error {
		policy, err := s.loadPolicyNoCtx(teamID)
		if err != nil {
			return err
		}
		return s.saveTaskCurrentLocked(teamID, task, policy)
	})
	if err == nil {
		s.publish(TeamEvent{
			TeamID:    teamID,
			Kind:      "task",
			Action:    "update",
			SubjectID: task.TaskID,
			ChannelID: task.ChannelID,
			ContextID: task.ContextID,
			Metadata: map[string]any{
				"status":   task.Status,
				"priority": task.Priority,
			},
		})
	}
	return err
}

func (s *Store) deleteTaskNoCtx(teamID, taskID string) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	taskID = strings.TrimSpace(taskID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	if taskID == "" {
		return errors.New("empty task id")
	}
	err := s.withTeamLock(teamID, func() error {
		return s.deleteTaskCurrentLocked(teamID, taskID)
	})
	if err == nil {
		s.publish(TeamEvent{
			TeamID:    teamID,
			Kind:      "task",
			Action:    "delete",
			SubjectID: taskID,
		})
	}
	return err
}

func (s *Store) appendArtifactNoCtx(teamID string, artifact Artifact) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	if strings.TrimSpace(artifact.TeamID) == "" {
		artifact.TeamID = teamID
	}
	artifact.TeamID = NormalizeTeamID(artifact.TeamID)
	if artifact.TeamID != teamID {
		return fmt.Errorf("team artifact team_id %q does not match %q", artifact.TeamID, teamID)
	}
	artifact.Title = strings.TrimSpace(artifact.Title)
	if artifact.Title == "" {
		return errors.New("empty team artifact title")
	}
	artifact.Kind = normalizeArtifactKind(artifact.Kind)
	artifact.ChannelID = normalizeChannelID(artifact.ChannelID)
	artifact.TaskID = strings.TrimSpace(artifact.TaskID)
	artifact.Summary = strings.TrimSpace(artifact.Summary)
	artifact.Content = strings.TrimSpace(artifact.Content)
	artifact.LinkURL = strings.TrimSpace(artifact.LinkURL)
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = time.Now().UTC()
	}
	if artifact.UpdatedAt.IsZero() {
		artifact.UpdatedAt = artifact.CreatedAt
	}
	if strings.TrimSpace(artifact.ArtifactID) == "" {
		artifact.ArtifactID = buildArtifactID(artifact)
	}
	err := s.withTeamLock(teamID, func() error {
		return s.appendArtifactCurrentLocked(teamID, artifact)
	})
	if err == nil {
		s.publish(TeamEvent{
			TeamID:    teamID,
			Kind:      "artifact",
			Action:    "create",
			SubjectID: artifact.ArtifactID,
			ChannelID: artifact.ChannelID,
			Metadata: map[string]any{
				"task_id": artifact.TaskID,
				"kind":    artifact.Kind,
			},
		})
	}
	return err
}

func (s *Store) loadArtifactsNoCtx(teamID string, limit int) ([]Artifact, error) {
	if s == nil {
		return nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	return s.loadArtifactsCurrent(teamID, limit)
}

func (s *Store) loadLegacyArtifacts(teamID string) ([]Artifact, error) {
	if s == nil {
		return nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	path := filepath.Join(s.root, teamID, "artifacts.jsonl")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	out := make([]Artifact, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var artifact Artifact
		if err := json.Unmarshal([]byte(line), &artifact); err != nil {
			continue
		}
		out = append(out, artifact)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].ArtifactID > out[j].ArtifactID
	})
	return out, nil
}

func (s *Store) loadArtifactNoCtx(teamID, artifactID string) (Artifact, error) {
	if s == nil {
		return Artifact{}, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	artifactID = strings.TrimSpace(artifactID)
	if teamID == "" {
		return Artifact{}, errors.New("empty team id")
	}
	if artifactID == "" {
		return Artifact{}, errors.New("empty artifact id")
	}
	return s.loadArtifactCurrent(teamID, artifactID)
}

func (s *Store) appendHistoryNoCtx(teamID string, event ChangeEvent) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	event.TeamID = teamID
	event.Scope = strings.TrimSpace(event.Scope)
	event.Action = strings.TrimSpace(event.Action)
	event.ActorAgentID = strings.TrimSpace(event.ActorAgentID)
	event.ActorOriginPublicKey = strings.TrimSpace(event.ActorOriginPublicKey)
	event.ActorParentPublicKey = strings.TrimSpace(event.ActorParentPublicKey)
	event.Source = strings.TrimSpace(event.Source)
	event.Diff = normalizeFieldDiffs(event.Diff)
	if event.Scope == "" || event.Action == "" {
		return errors.New("empty team history scope or action")
	}
	if event.Source == "" {
		event.Source = "system"
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(event.EventID) == "" {
		event.EventID = buildChangeEventID(event)
	}
	err := s.withTeamLock(teamID, func() error {
		path := filepath.Join(s.root, teamID, "history.jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer file.Close()
		body, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := file.Write(append(body, '\n')); err != nil {
			return err
		}
		return nil
	})
	if err == nil {
		s.publish(TeamEvent{
			TeamID:    teamID,
			Kind:      "history",
			Action:    event.Action,
			SubjectID: event.SubjectID,
			Metadata: map[string]any{
				"scope": event.Scope,
			},
		})
	}
	return err
}

func normalizeFieldDiffs(diff map[string]FieldDiff) map[string]FieldDiff {
	if len(diff) == 0 {
		return nil
	}
	out := make(map[string]FieldDiff, len(diff))
	for key, item := range diff {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if reflect.DeepEqual(item.Before, item.After) {
			continue
		}
		out[key] = item
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *Store) loadHistoryNoCtx(teamID string, limit int) ([]ChangeEvent, error) {
	if s == nil {
		return nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	path := filepath.Join(s.root, teamID, "history.jsonl")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var out []ChangeEvent
	if limit > 0 {
		lines, err := readLastJSONLLines(path, limit)
		if err != nil {
			return nil, err
		}
		out = make([]ChangeEvent, 0, len(lines))
		for _, line := range lines {
			var event ChangeEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}
			out = append(out, event)
		}
	} else {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var event ChangeEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}
			out = append(out, event)
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].EventID > out[j].EventID
	})
	return out, nil
}

func (s *Store) saveArtifactNoCtx(teamID string, artifact Artifact) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	artifactID := strings.TrimSpace(artifact.ArtifactID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	if artifactID == "" {
		return errors.New("empty artifact id")
	}
	err := s.withTeamLock(teamID, func() error {
		return s.saveArtifactCurrentLocked(teamID, artifact)
	})
	if err == nil {
		s.publish(TeamEvent{
			TeamID:    teamID,
			Kind:      "artifact",
			Action:    "update",
			SubjectID: artifact.ArtifactID,
			ChannelID: artifact.ChannelID,
			Metadata: map[string]any{
				"task_id": artifact.TaskID,
				"kind":    artifact.Kind,
			},
		})
	}
	return err
}

func (s *Store) deleteArtifactNoCtx(teamID, artifactID string) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	artifactID = strings.TrimSpace(artifactID)
	if teamID == "" {
		return errors.New("empty team id")
	}
	if artifactID == "" {
		return errors.New("empty artifact id")
	}
	err := s.withTeamLock(teamID, func() error {
		return s.deleteArtifactCurrentLocked(teamID, artifactID)
	})
	if err == nil {
		s.publish(TeamEvent{
			TeamID:    teamID,
			Kind:      "artifact",
			Action:    "delete",
			SubjectID: artifactID,
		})
	}
	return err
}

func (s *Store) loadTasksByContextNoCtx(teamID, contextID string) ([]Task, error) {
	if s == nil {
		return nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	contextID = normalizeContextID(contextID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	if contextID == "" {
		return nil, errors.New("empty context id")
	}
	tasks, err := s.loadTasksNoCtx(teamID, 0)
	if err != nil {
		return nil, err
	}
	out := make([]Task, 0)
	for _, task := range tasks {
		if normalizeContextID(task.ContextID) == contextID {
			out = append(out, task)
		}
	}
	return out, nil
}

func (s *Store) Subscribe(teamID string) (<-chan TeamEvent, func(), error) {
	if s == nil {
		return nil, nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, nil, errors.New("empty team id")
	}
	ch := make(chan TeamEvent, 32)
	s.subMu.Lock()
	if s.subscribers == nil {
		s.subscribers = make(map[string]map[chan TeamEvent]struct{})
	}
	if s.subscribers[teamID] == nil {
		s.subscribers[teamID] = make(map[chan TeamEvent]struct{})
	}
	s.subscribers[teamID][ch] = struct{}{}
	s.subMu.Unlock()
	cancel := func() {
		s.subMu.Lock()
		defer s.subMu.Unlock()
		if subs := s.subscribers[teamID]; subs != nil {
			delete(subs, ch)
			if len(subs) == 0 {
				delete(s.subscribers, teamID)
			}
		}
	}
	return ch, cancel, nil
}

func (s *Store) publish(event TeamEvent) {
	if s == nil {
		return
	}
	event.TeamID = NormalizeTeamID(event.TeamID)
	event.Kind = strings.TrimSpace(event.Kind)
	event.Action = strings.TrimSpace(event.Action)
	event.SubjectID = strings.TrimSpace(event.SubjectID)
	event.ChannelID = normalizeChannelID(event.ChannelID)
	if event.ChannelID == "main" && strings.TrimSpace(event.Kind) != "message" && strings.TrimSpace(event.Kind) != "channel" {
		event.ChannelID = ""
	}
	event.ContextID = normalizeContextID(event.ContextID)
	if event.TeamID == "" || event.Kind == "" || event.Action == "" {
		return
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(event.EventID) == "" {
		event.EventID = fmt.Sprintf("%s:%s:%s:%s", event.TeamID, event.Kind, event.Action, event.CreatedAt.UTC().Format(time.RFC3339Nano))
	}
	s.subMu.RLock()
	subs := s.subscribers[event.TeamID]
	targets := make([]chan TeamEvent, 0, len(subs))
	for ch := range subs {
		targets = append(targets, ch)
	}
	s.subMu.RUnlock()
	for _, ch := range targets {
		select {
		case ch <- event:
		default:
		}
	}
	configs, err := s.loadWebhookConfigsNoCtx(event.TeamID)
	if err != nil || len(configs) == 0 {
		return
	}
	for _, cfg := range configs {
		if !matchesEventFilter(cfg.Events, event) {
			continue
		}
		go s.sendWebhook(cfg, event)
	}
}

func matchesEventFilter(filters []string, event TeamEvent) bool {
	if len(filters) == 0 {
		return true
	}
	kind := strings.ToLower(strings.TrimSpace(event.Kind))
	action := strings.ToLower(strings.TrimSpace(event.Action))
	full := kind + "." + action
	for _, filter := range filters {
		filter = strings.ToLower(strings.TrimSpace(filter))
		if filter == "" {
			continue
		}
		if filter == "*" || filter == kind || filter == full {
			return true
		}
	}
	return false
}

func (s *Store) sendWebhook(cfg PushNotificationConfig, event TeamEvent) {
	deliveryID := buildWebhookDeliveryID(event, cfg.WebhookID, time.Now().UTC())
	s.sendWebhookWithRecord(cfg, event, deliveryID, "")
}

func (s *Store) sendWebhookWithRecord(cfg PushNotificationConfig, event TeamEvent, deliveryID, replayedFrom string) {
	body, err := json.Marshal(event)
	if err != nil {
		logTeamEvent("webhook_marshal_failed", "team", event.TeamID, "webhook_id", cfg.WebhookID, "error", err)
		return
	}
	client := s.webhookClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	record := normalizeWebhookDeliveryRecord(WebhookDeliveryRecord{
		DeliveryID:   deliveryID,
		TeamID:       event.TeamID,
		WebhookID:    cfg.WebhookID,
		URL:          cfg.URL,
		Token:        cfg.Token,
		Events:       append([]string(nil), cfg.Events...),
		Event:        event,
		Status:       webhookDeliveryStatusPending,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
		ReplayedFrom: strings.TrimSpace(replayedFrom),
	})
	_ = s.withTeamLock(event.TeamID, func() error {
		return s.updateWebhookDeliveryLocked(event.TeamID, record)
	})

	for attempt := 1; attempt <= 3; attempt++ {
		now := time.Now().UTC()
		record.Attempt = attempt
		record.LastAttemptAt = now
		record.UpdatedAt = now
		req, err := http.NewRequest(http.MethodPost, cfg.URL, bytes.NewReader(body))
		if err != nil {
			logTeamEvent("webhook_request_failed", "team", event.TeamID, "webhook_id", cfg.WebhookID, "delivery_id", record.DeliveryID, "error", err)
			record.Status = webhookDeliveryStatusFailed
			record.Error = err.Error()
			_ = s.withTeamLock(event.TeamID, func() error {
				return s.updateWebhookDeliveryLocked(event.TeamID, record)
			})
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if token := strings.TrimSpace(cfg.Token); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			logTeamEvent("webhook_delivery_retrying", "team", event.TeamID, "webhook_id", cfg.WebhookID, "delivery_id", record.DeliveryID, "attempt", attempt, "error", err)
			record.Status = webhookDeliveryStatusRetrying
			record.Error = err.Error()
			record.StatusCode = 0
		} else {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				logTeamEvent("webhook_delivered", "team", event.TeamID, "webhook_id", cfg.WebhookID, "delivery_id", record.DeliveryID, "attempt", attempt, "status", resp.StatusCode)
				record.Status = webhookDeliveryStatusDelivered
				record.StatusCode = resp.StatusCode
				record.Error = ""
				record.DeliveredAt = time.Now().UTC()
				record.NextRetryAt = time.Time{}
				_ = s.withTeamLock(event.TeamID, func() error {
					return s.updateWebhookDeliveryLocked(event.TeamID, record)
				})
				return
			}
			logTeamEvent("webhook_http_status", "team", event.TeamID, "webhook_id", cfg.WebhookID, "delivery_id", record.DeliveryID, "attempt", attempt, "status", resp.StatusCode, "url", cfg.URL)
			record.StatusCode = resp.StatusCode
			record.Error = http.StatusText(resp.StatusCode)
			if !isWebhookRetriableStatus(resp.StatusCode) {
				record.Status = webhookDeliveryStatusFailed
				record.NextRetryAt = time.Time{}
				_ = s.withTeamLock(event.TeamID, func() error {
					return s.updateWebhookDeliveryLocked(event.TeamID, record)
				})
				return
			}
			record.Status = webhookDeliveryStatusRetrying
		}
		record.NextRetryAt = webhookNextRetryAt(time.Now().UTC(), attempt)
		_ = s.withTeamLock(event.TeamID, func() error {
			return s.updateWebhookDeliveryLocked(event.TeamID, record)
		})
		if attempt < 3 {
			time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
		}
	}
	record.Status = webhookDeliveryStatusDeadLetter
	record.NextRetryAt = time.Time{}
	record.UpdatedAt = time.Now().UTC()
	logTeamEvent("webhook_dead_letter", "team", event.TeamID, "webhook_id", cfg.WebhookID, "delivery_id", record.DeliveryID, "attempt", record.Attempt, "status", record.StatusCode, "error", record.Error)
	_ = s.withTeamLock(event.TeamID, func() error {
		return s.updateWebhookDeliveryLocked(event.TeamID, record)
	})
}

func normalizePolicy(policy Policy) Policy {
	policy.MessageRoles = normalizePolicyRoles(policy.MessageRoles, []string{"owner", "maintainer", "member"})
	policy.TaskRoles = normalizePolicyRoles(policy.TaskRoles, []string{"owner", "maintainer", "member"})
	policy.SystemNoteRoles = normalizePolicyRoles(policy.SystemNoteRoles, []string{"owner", "maintainer"})
	policy.Permissions = normalizePolicyPermissions(policy.Permissions)
	policy.TaskTransitions = normalizeTaskTransitions(policy.TaskTransitions)
	return policy
}

func defaultPolicy() Policy {
	return normalizePolicy(Policy{})
}

func normalizePolicyRoles(values []string, defaults []string) []string {
	if len(values) == 0 {
		values = defaults
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		role := normalizeMemberRole(value)
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		out = append(out, role)
	}
	if len(out) == 0 {
		return append([]string(nil), defaults...)
	}
	return out
}

func normalizePolicyPermissions(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string][]string, len(values))
	for action, roles := range values {
		action = normalizePolicyAction(action)
		if action == "" {
			continue
		}
		out[action] = normalizePolicyRoles(roles, nil)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizePolicyAction(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeWebhookConfigs(values []PushNotificationConfig) []PushNotificationConfig {
	if len(values) == 0 {
		return nil
	}
	out := make([]PushNotificationConfig, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, cfg := range values {
		cfg.URL = strings.TrimSpace(cfg.URL)
		if cfg.URL == "" {
			continue
		}
		cfg.Token = strings.TrimSpace(cfg.Token)
		cfg.Events = normalizeNonEmptyStrings(cfg.Events)
		if cfg.UpdatedAt.IsZero() {
			cfg.UpdatedAt = time.Now().UTC()
		}
		cfg.WebhookID = strings.TrimSpace(cfg.WebhookID)
		if cfg.WebhookID == "" {
			cfg.WebhookID = fmt.Sprintf("webhook-%s", cfg.UpdatedAt.UTC().Format("20060102T150405.000000000Z"))
		}
		if _, ok := seen[cfg.WebhookID]; ok {
			continue
		}
		seen[cfg.WebhookID] = struct{}{}
		out = append(out, cfg)
	}
	return out
}

func (p Policy) Allows(action, role string) bool {
	action = normalizePolicyAction(action)
	role = normalizeMemberRole(role)
	if action == "" || role == "" {
		return false
	}
	if len(p.Permissions) > 0 {
		if roles, ok := p.Permissions[action]; ok {
			return containsRole(roles, role)
		}
	}
	return p.legacyAllows(action, role)
}

func (p Policy) legacyAllows(action, role string) bool {
	switch {
	case action == "message.send":
		return containsRole(p.MessageRoles, role)
	case strings.HasPrefix(action, "task."):
		return containsRole(p.TaskRoles, role)
	case strings.HasPrefix(action, "artifact."):
		return containsRole(p.TaskRoles, role)
	case strings.HasPrefix(action, "member."):
		return containsRole(p.SystemNoteRoles, role)
	case strings.HasPrefix(action, "channel."):
		return containsRole(p.SystemNoteRoles, role)
	case action == "policy.update":
		return containsRole(p.SystemNoteRoles, role)
	case action == "sync.conflict.resolve":
		return containsRole(p.SystemNoteRoles, role)
	case action == "archive.create":
		return containsRole(p.SystemNoteRoles, role)
	case action == "agent_card.register":
		return containsRole(p.SystemNoteRoles, role)
	default:
		return false
	}
}

func containsRole(values []string, role string) bool {
	role = normalizeMemberRole(role)
	for _, value := range values {
		if normalizeMemberRole(value) == role {
			return true
		}
	}
	return false
}

func (s *Store) saveTaskIndexedLocked(teamID string, task Task, policy Policy) error {
	current, err := s.loadTaskFromIndex(teamID, task.TaskID)
	if err != nil {
		return err
	}
	task.TeamID = teamID
	task.TaskID = strings.TrimSpace(task.TaskID)
	task.Title = strings.TrimSpace(task.Title)
	if task.Title == "" {
		return errors.New("empty team task title")
	}
	task.Status = normalizeTaskStatus(task.Status)
	if task.Status == "" {
		task.Status = current.Status
		if task.Status == "" {
			task.Status = "open"
		}
	}
	if !IsValidTransitionWithPolicy(current.Status, task.Status, policy) {
		return fmt.Errorf("invalid task status transition %q -> %q", normalizeTaskStatus(current.Status), task.Status)
	}
	task.Priority = normalizeTaskPriority(task.Priority)
	task.ChannelID = normalizeChannelID(task.ChannelID)
	task.ContextID = normalizeContextID(task.ContextID)
	if task.ContextID == "" {
		task.ContextID = normalizeContextID(current.ContextID)
	}
	task.Description = strings.TrimSpace(task.Description)
	task.CreatedBy = strings.TrimSpace(task.CreatedBy)
	task.Assignees = normalizeNonEmptyStrings(task.Assignees)
	task.Labels = normalizeNonEmptyStrings(task.Labels)
	if task.CreatedAt.IsZero() {
		task.CreatedAt = current.CreatedAt
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = time.Now().UTC()
	}
	if IsTerminalState(task.Status) {
		if task.ClosedAt.IsZero() {
			task.ClosedAt = task.UpdatedAt
		}
	} else {
		task.ClosedAt = time.Time{}
	}
	offset, length, err := appendJSONLRecord(s.taskDataPath(teamID), task)
	if err != nil {
		return err
	}
	entries, err := s.loadTaskIndex(teamID)
	if err != nil {
		return err
	}
	entry := taskIndexEntryFromTask(task, offset, length)
	for i := range entries {
		if entries[i].TaskID == task.TaskID {
			entries[i] = entry
			return s.saveTaskIndex(teamID, entries)
		}
	}
	return os.ErrNotExist
}

func (s *Store) deleteTaskIndexedLocked(teamID, taskID string) error {
	entries, err := s.loadTaskIndex(teamID)
	if err != nil {
		return err
	}
	removed := false
	for i := range entries {
		if entries[i].TaskID == taskID && !entries[i].Deleted {
			entries[i].Deleted = true
			removed = true
		}
	}
	if !removed {
		return os.ErrNotExist
	}
	return s.saveTaskIndex(teamID, entries)
}

func (s *Store) saveArtifactIndexedLocked(teamID string, artifact Artifact) error {
	current, err := s.loadArtifactFromIndex(teamID, artifact.ArtifactID)
	if err != nil {
		return err
	}
	artifact.TeamID = teamID
	artifact.ArtifactID = strings.TrimSpace(artifact.ArtifactID)
	artifact.Title = strings.TrimSpace(artifact.Title)
	if artifact.Title == "" {
		return errors.New("empty team artifact title")
	}
	artifact.Kind = normalizeArtifactKind(artifact.Kind)
	artifact.ChannelID = normalizeChannelID(artifact.ChannelID)
	artifact.TaskID = strings.TrimSpace(artifact.TaskID)
	artifact.Summary = strings.TrimSpace(artifact.Summary)
	artifact.Content = strings.TrimSpace(artifact.Content)
	artifact.LinkURL = strings.TrimSpace(artifact.LinkURL)
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = current.CreatedAt
	}
	if artifact.UpdatedAt.IsZero() {
		artifact.UpdatedAt = time.Now().UTC()
	}
	offset, length, err := appendJSONLRecord(s.artifactDataPath(teamID), artifact)
	if err != nil {
		return err
	}
	entries, err := s.loadArtifactIndex(teamID)
	if err != nil {
		return err
	}
	entry := artifactIndexEntryFromArtifact(artifact, offset, length)
	for i := range entries {
		if entries[i].ArtifactID == artifact.ArtifactID {
			entries[i] = entry
			return s.saveArtifactIndex(teamID, entries)
		}
	}
	return os.ErrNotExist
}

func (s *Store) deleteArtifactIndexedLocked(teamID, artifactID string) error {
	entries, err := s.loadArtifactIndex(teamID)
	if err != nil {
		return err
	}
	removed := false
	for i := range entries {
		if entries[i].ArtifactID == artifactID && !entries[i].Deleted {
			entries[i].Deleted = true
			removed = true
		}
	}
	if !removed {
		return os.ErrNotExist
	}
	return s.saveArtifactIndex(teamID, entries)
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

func buildTaskID(task Task) string {
	return strings.Join([]string{
		strings.TrimSpace(task.TeamID),
		strings.TrimSpace(task.CreatedBy),
		task.CreatedAt.UTC().Format(time.RFC3339Nano),
		strings.TrimSpace(task.Title),
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

func buildArtifactID(artifact Artifact) string {
	return strings.Join([]string{
		strings.TrimSpace(artifact.TeamID),
		strings.TrimSpace(artifact.CreatedBy),
		artifact.CreatedAt.UTC().Format(time.RFC3339Nano),
		strings.TrimSpace(artifact.Title),
	}, ":")
}

func buildChangeEventID(event ChangeEvent) string {
	return strings.Join([]string{
		strings.TrimSpace(event.TeamID),
		strings.TrimSpace(event.Scope),
		strings.TrimSpace(event.Action),
		event.CreatedAt.UTC().Format(time.RFC3339Nano),
		strings.TrimSpace(event.SubjectID),
	}, ":")
}

func taskIDMatches(structuredData map[string]any, taskID string) bool {
	if len(structuredData) == 0 || taskID == "" {
		return false
	}
	for _, key := range []string{"task_id", "team_task_id"} {
		if value, ok := structuredData[key]; ok && strings.TrimSpace(fmt.Sprint(value)) == taskID {
			return true
		}
	}
	return false
}

func (s *Store) channelSummary(teamID, channelID string) (ChannelSummary, error) {
	count, lastAt, err := s.channelMessageStats(teamID, channelID)
	if err != nil {
		return ChannelSummary{}, err
	}
	channel, err := s.loadChannelNoCtx(teamID, channelID)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return ChannelSummary{}, err
	}
	if errors.Is(err, os.ErrNotExist) {
		channel = defaultChannel(channelID)
	}
	summary := ChannelSummary{
		Channel:       channel,
		MessageCount:  count,
		LastMessageAt: lastAt,
	}
	return summary, nil
}

func (s *Store) channelMessageStats(teamID, channelID string) (int, time.Time, error) {
	if s.isShardedChannel(teamID, channelID) {
		return s.shardedChannelMessageStats(teamID, channelID)
	}
	path := s.channelPath(teamID, channelID)
	count, err := countNonEmptyJSONLLines(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, time.Time{}, nil
	}
	if err != nil {
		return 0, time.Time{}, err
	}
	lastAt, err := latestMessageTimestampFromJSONL(path)
	if errors.Is(err, os.ErrNotExist) {
		return count, time.Time{}, nil
	}
	return count, lastAt, err
}

func (s *Store) shardedChannelMessageStats(teamID, channelID string) (int, time.Time, error) {
	dir := s.channelShardDir(teamID, channelID)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, time.Time{}, nil
	}
	if err != nil {
		return 0, time.Time{}, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	sort.Slice(paths, func(i, j int) bool {
		return filepath.Base(paths[i]) > filepath.Base(paths[j])
	})
	total := 0
	for _, path := range paths {
		count, err := countNonEmptyJSONLLines(path)
		if err != nil {
			return 0, time.Time{}, err
		}
		total += count
	}
	var lastAt time.Time
	for _, path := range paths {
		lastAt, err = latestMessageTimestampFromJSONL(path)
		if err == nil || !errors.Is(err, os.ErrNotExist) {
			return total, lastAt, err
		}
	}
	return total, time.Time{}, nil
}

func (s *Store) loadChannelConfigs(teamID string) ([]Channel, error) {
	info, err := s.loadTeamNoCtx(teamID)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]Channel, len(info.Channels))
	for _, channelID := range info.Channels {
		channel := defaultChannel(channelID)
		merged[channel.ChannelID] = channel
	}
	path := s.channelsConfigPath(info.TeamID)
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if len(data) > 0 {
		var channels []Channel
		if err := json.Unmarshal(data, &channels); err != nil {
			return nil, err
		}
		for _, channel := range channels {
			channel = normalizeChannel(channel)
			if channel.ChannelID == "" {
				continue
			}
			existing, ok := merged[channel.ChannelID]
			if ok {
				channel = mergeChannel(existing, channel)
			}
			merged[channel.ChannelID] = channel
		}
	}
	if len(merged) == 0 {
		channel := defaultChannel("main")
		merged[channel.ChannelID] = channel
	}
	out := make([]Channel, 0, len(merged))
	for _, channel := range merged {
		out = append(out, normalizeChannel(channel))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Hidden != out[j].Hidden {
			return !out[i].Hidden
		}
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].ChannelID < out[j].ChannelID
	})
	return out, nil
}

func (s *Store) saveChannels(teamID string, channels []Channel) error {
	path := s.channelsConfigPath(teamID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	normalized := make([]Channel, 0, len(channels))
	seen := make(map[string]struct{}, len(channels))
	for _, channel := range channels {
		channel = normalizeChannel(channel)
		if channel.ChannelID == "" {
			continue
		}
		if _, ok := seen[channel.ChannelID]; ok {
			continue
		}
		seen[channel.ChannelID] = struct{}{}
		if channel.CreatedAt.IsZero() {
			channel.CreatedAt = time.Now().UTC()
		}
		if channel.UpdatedAt.IsZero() {
			channel.UpdatedAt = channel.CreatedAt
		}
		normalized = append(normalized, channel)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].Hidden != normalized[j].Hidden {
			return !normalized[i].Hidden
		}
		return normalized[i].ChannelID < normalized[j].ChannelID
	})
	body, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}
