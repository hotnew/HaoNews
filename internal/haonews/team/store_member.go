package team

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (s *Store) loadMembersNoCtx(teamID string) ([]Member, error) {
	if s == nil {
		return nil, NewNilStoreError("Store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, NewEmptyIDError("team_id")
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
