package team

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func normalizeInfo(info Info) Info {
	info.TeamID = NormalizeTeamID(info.TeamID)
	info.Slug = NormalizeTeamID(info.Slug)
	info.Title = strings.TrimSpace(info.Title)
	info.Description = strings.TrimSpace(info.Description)
	info.Visibility = strings.TrimSpace(strings.ToLower(info.Visibility))
	info.OwnerAgentID = strings.TrimSpace(info.OwnerAgentID)
	info.OwnerOriginPublicKey = strings.TrimSpace(info.OwnerOriginPublicKey)
	info.OwnerParentPublicKey = strings.TrimSpace(info.OwnerParentPublicKey)
	info.Channels = teamChannels(info)
	if info.Slug == "" {
		info.Slug = info.TeamID
	}
	if info.Visibility == "" {
		info.Visibility = "team"
	}
	return info
}

func (s *Store) saveTeamNoCtx(info Info) error {
	if s == nil {
		return NewNilStoreError("Store")
	}
	info = normalizeInfo(info)
	if info.TeamID == "" {
		return NewEmptyIDError("team_id")
	}
	if info.Title == "" {
		return &TeamError{Code: ErrCodeInvalidState, Context: "empty team title"}
	}
	if info.CreatedAt.IsZero() {
		existing, err := s.loadTeamNoCtx(info.TeamID)
		if err == nil && !existing.CreatedAt.IsZero() {
			info.CreatedAt = existing.CreatedAt
		} else {
			info.CreatedAt = time.Now().UTC()
		}
	}
	info.UpdatedAt = time.Now().UTC()
	return s.withTeamLock(info.TeamID, func() error {
		if err := os.MkdirAll(filepath.Dir(s.teamInfoPath(info.TeamID)), 0o755); err != nil {
			return err
		}
		body, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return err
		}
		body = append(body, '\n')
		return os.WriteFile(s.teamInfoPath(info.TeamID), body, 0o644)
	})
}
