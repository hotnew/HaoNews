package team

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

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
		for _, notification := range buildMentionNotifications(teamID, msg) {
			_ = s.appendNotificationNoCtx(teamID, notification)
		}
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
		return nil, NewNilStoreError("Store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, NewEmptyIDError("team_id")
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
