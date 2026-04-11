package team

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

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

func buildChangeEventID(event ChangeEvent) string {
	return strings.Join([]string{
		strings.TrimSpace(event.TeamID),
		strings.TrimSpace(event.Scope),
		strings.TrimSpace(event.Action),
		event.CreatedAt.UTC().Format(time.RFC3339Nano),
		strings.TrimSpace(event.SubjectID),
	}, ":")
}
