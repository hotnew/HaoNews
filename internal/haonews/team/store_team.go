package team

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func (s *Store) loadTeamNoCtx(teamID string) (Info, error) {
	if s == nil {
		return Info{}, NewNilStoreError("Store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return Info{}, NewEmptyIDError("team_id")
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
