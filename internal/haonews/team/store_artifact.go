package team

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

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

func buildArtifactID(artifact Artifact) string {
	return strings.Join([]string{
		strings.TrimSpace(artifact.TeamID),
		strings.TrimSpace(artifact.CreatedBy),
		artifact.CreatedAt.UTC().Format(time.RFC3339Nano),
		strings.TrimSpace(artifact.Title),
	}, ":")
}
