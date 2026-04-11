package team

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	MilestoneStateOpen = "open"
	MilestoneStateDone = "done"
)

type Milestone struct {
	MilestoneID string    `json:"milestone_id"`
	TeamID      string    `json:"team_id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status,omitempty"`
	DueAt       time.Time `json:"due_at,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

type MilestoneProgress struct {
	Milestone     Milestone `json:"milestone"`
	TaskCount     int       `json:"task_count"`
	Completed     int       `json:"completed_count"`
	Active        int       `json:"active_count"`
	Blocked       int       `json:"blocked_count"`
	CompletionPct int       `json:"completion_pct"`
}

func normalizeMilestoneStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", MilestoneStateOpen:
		return MilestoneStateOpen
	case MilestoneStateDone, "closed", "completed":
		return MilestoneStateDone
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizeMilestone(m Milestone) Milestone {
	m.MilestoneID = normalizeMilestoneID(m.MilestoneID)
	m.TeamID = NormalizeTeamID(m.TeamID)
	m.Title = strings.TrimSpace(m.Title)
	m.Description = strings.TrimSpace(m.Description)
	m.Status = normalizeMilestoneStatus(m.Status)
	if !m.DueAt.IsZero() {
		m.DueAt = m.DueAt.UTC()
	}
	if !m.CreatedAt.IsZero() {
		m.CreatedAt = m.CreatedAt.UTC()
	}
	if !m.UpdatedAt.IsZero() {
		m.UpdatedAt = m.UpdatedAt.UTC()
	}
	if !m.CompletedAt.IsZero() {
		m.CompletedAt = m.CompletedAt.UTC()
	}
	return m
}

func buildMilestoneID(m Milestone) string {
	return NormalizeTeamID(m.Title + "-" + time.Now().UTC().Format("20060102t150405.000000000"))
}

func (s *Store) loadMilestonesNoCtx(teamID string) ([]Milestone, error) {
	if s == nil {
		return nil, NewNilStoreError("Store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, NewEmptyIDError("team_id")
	}
	data, err := os.ReadFile(s.milestonePath(teamID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Milestone
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	for i := range out {
		out[i] = normalizeMilestone(out[i])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].MilestoneID < out[j].MilestoneID
	})
	return out, nil
}

func (s *Store) loadMilestoneNoCtx(teamID, milestoneID string) (Milestone, error) {
	items, err := s.loadMilestonesNoCtx(teamID)
	if err != nil {
		return Milestone{}, err
	}
	milestoneID = normalizeMilestoneID(milestoneID)
	if milestoneID == "" {
		return Milestone{}, NewEmptyIDError("milestone_id")
	}
	for _, item := range items {
		if item.MilestoneID == milestoneID {
			return item, nil
		}
	}
	return Milestone{}, os.ErrNotExist
}

func (s *Store) saveMilestoneNoCtx(teamID string, milestone Milestone) error {
	if s == nil {
		return NewNilStoreError("Store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return NewEmptyIDError("team_id")
	}
	milestone = normalizeMilestone(milestone)
	if milestone.TeamID == "" {
		milestone.TeamID = teamID
	}
	if milestone.TeamID != teamID {
		return &TeamError{Code: ErrCodeConflict, Context: "milestone team_id mismatch"}
	}
	if milestone.Title == "" {
		return &TeamError{Code: ErrCodeInvalidState, Context: "empty milestone title"}
	}
	if milestone.MilestoneID == "" {
		milestone.MilestoneID = buildMilestoneID(milestone)
	}
	if milestone.CreatedAt.IsZero() {
		if existing, err := s.loadMilestoneNoCtx(teamID, milestone.MilestoneID); err == nil && !existing.CreatedAt.IsZero() {
			milestone.CreatedAt = existing.CreatedAt
		} else {
			milestone.CreatedAt = time.Now().UTC()
		}
	}
	milestone.UpdatedAt = time.Now().UTC()
	if milestone.Status == MilestoneStateDone && milestone.CompletedAt.IsZero() {
		milestone.CompletedAt = milestone.UpdatedAt
	}
	if milestone.Status != MilestoneStateDone {
		milestone.CompletedAt = time.Time{}
	}
	return s.withTeamLock(teamID, func() error {
		items, err := s.loadMilestonesNoCtx(teamID)
		if err != nil {
			return err
		}
		updated := false
		for i := range items {
			if items[i].MilestoneID == milestone.MilestoneID {
				items[i] = milestone
				updated = true
				break
			}
		}
		if !updated {
			items = append(items, milestone)
		}
		body, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return err
		}
		body = append(body, '\n')
		return os.WriteFile(s.milestonePath(teamID), body, 0o644)
	})
}

func (s *Store) deleteMilestoneNoCtx(teamID, milestoneID string) error {
	if s == nil {
		return NewNilStoreError("Store")
	}
	teamID = NormalizeTeamID(teamID)
	milestoneID = normalizeMilestoneID(milestoneID)
	if teamID == "" {
		return NewEmptyIDError("team_id")
	}
	if milestoneID == "" {
		return NewEmptyIDError("milestone_id")
	}
	return s.withTeamLock(teamID, func() error {
		items, err := s.loadMilestonesNoCtx(teamID)
		if err != nil {
			return err
		}
		filtered := items[:0]
		removed := false
		for _, item := range items {
			if item.MilestoneID == milestoneID {
				removed = true
				continue
			}
			filtered = append(filtered, item)
		}
		if !removed {
			return os.ErrNotExist
		}
		body, err := json.MarshalIndent(filtered, "", "  ")
		if err != nil {
			return err
		}
		body = append(body, '\n')
		return os.WriteFile(s.milestonePath(teamID), body, 0o644)
	})
}

func (s *Store) computeMilestoneProgressNoCtx(teamID string, milestone Milestone) (MilestoneProgress, error) {
	tasks, err := s.loadTasksCurrent(teamID, 0)
	if err != nil {
		return MilestoneProgress{}, err
	}
	progress := MilestoneProgress{Milestone: milestone}
	for _, task := range tasks {
		if normalizeMilestoneID(task.MilestoneID) != milestone.MilestoneID {
			continue
		}
		progress.TaskCount++
		switch normalizeTaskStatus(task.Status) {
		case TaskStateDone:
			progress.Completed++
		case TaskStateBlocked:
			progress.Active++
			progress.Blocked++
		case TaskStateDoing, TaskStateReview, TaskStateDispatched, TaskStateOpen:
			progress.Active++
		}
	}
	if progress.TaskCount > 0 {
		progress.CompletionPct = progress.Completed * 100 / progress.TaskCount
	}
	return progress, nil
}

func (s *Store) listMilestoneProgressNoCtx(teamID string) ([]MilestoneProgress, error) {
	items, err := s.loadMilestonesNoCtx(teamID)
	if err != nil {
		return nil, err
	}
	out := make([]MilestoneProgress, 0, len(items))
	for _, item := range items {
		progress, err := s.computeMilestoneProgressNoCtx(teamID, item)
		if err != nil {
			return nil, err
		}
		out = append(out, progress)
	}
	return out, nil
}
