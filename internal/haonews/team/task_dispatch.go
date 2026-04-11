package team

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	TaskDispatchStatusQueued  = "queued"
	TaskDispatchStatusAcked   = "acked"
	TaskDispatchStatusRunning = "running"
	TaskDispatchStatusDone    = "done"
	TaskDispatchStatusTimeout = "timeout"
	TaskDispatchStatusFailed  = "failed"
)

type TaskDispatch struct {
	TaskID           string    `json:"task_id"`
	AssignedAgentID  string    `json:"assigned_agent_id,omitempty"`
	MatchReason      string    `json:"match_reason,omitempty"`
	DispatchedAt     time.Time `json:"dispatched_at,omitempty"`
	AckedAt          time.Time `json:"acked_at,omitempty"`
	CompletedAt      time.Time `json:"completed_at,omitempty"`
	Status           string    `json:"status,omitempty"`
	RetryCount       int       `json:"retry_count,omitempty"`
	TimeoutSeconds   int       `json:"timeout_seconds,omitempty"`
	LastResponseAt   time.Time `json:"last_response_at,omitempty"`
	CurrentQueueSize int       `json:"current_queue_size,omitempty"`
}

func normalizeTaskDispatch(dispatch TaskDispatch) TaskDispatch {
	dispatch.TaskID = strings.TrimSpace(dispatch.TaskID)
	dispatch.AssignedAgentID = strings.TrimSpace(dispatch.AssignedAgentID)
	dispatch.MatchReason = strings.TrimSpace(dispatch.MatchReason)
	dispatch.Status = normalizeTaskDispatchStatus(dispatch.Status)
	if dispatch.Status == "" {
		dispatch.Status = TaskDispatchStatusQueued
	}
	if dispatch.DispatchedAt.IsZero() {
		dispatch.DispatchedAt = time.Now().UTC()
	}
	if dispatch.AckedAt.IsZero() && dispatch.Status == TaskDispatchStatusAcked {
		dispatch.AckedAt = dispatch.DispatchedAt
	}
	if dispatch.CompletedAt.IsZero() && dispatch.Status == TaskDispatchStatusDone {
		dispatch.CompletedAt = time.Now().UTC()
	}
	if !dispatch.LastResponseAt.IsZero() {
		dispatch.LastResponseAt = dispatch.LastResponseAt.UTC()
	}
	return dispatch
}

func normalizeTaskDispatchStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", TaskDispatchStatusQueued:
		return strings.ToLower(strings.TrimSpace(value))
	case TaskDispatchStatusAcked:
		return TaskDispatchStatusAcked
	case TaskDispatchStatusRunning:
		return TaskDispatchStatusRunning
	case TaskDispatchStatusDone:
		return TaskDispatchStatusDone
	case TaskDispatchStatusTimeout:
		return TaskDispatchStatusTimeout
	case TaskDispatchStatusFailed:
		return TaskDispatchStatusFailed
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func (s *Store) taskDispatchDir(teamID string) string {
	return filepath.Join(s.root, NormalizeTeamID(teamID), "task-dispatches")
}

func (s *Store) taskDispatchPath(teamID, taskID string) string {
	return filepath.Join(s.taskDispatchDir(teamID), strings.TrimSpace(taskID)+".json")
}

func (s *Store) loadTaskDispatchNoCtx(teamID, taskID string) (TaskDispatch, error) {
	if s == nil {
		return TaskDispatch{}, NewNilStoreError("Store")
	}
	teamID = NormalizeTeamID(teamID)
	taskID = strings.TrimSpace(taskID)
	if teamID == "" {
		return TaskDispatch{}, NewEmptyIDError("team_id")
	}
	if taskID == "" {
		return TaskDispatch{}, NewEmptyIDError("task_id")
	}
	data, err := os.ReadFile(s.taskDispatchPath(teamID, taskID))
	if err != nil {
		return TaskDispatch{}, err
	}
	var dispatch TaskDispatch
	if err := json.Unmarshal(data, &dispatch); err != nil {
		return TaskDispatch{}, err
	}
	dispatch = normalizeTaskDispatch(dispatch)
	dispatch.TaskID = taskID
	return dispatch, nil
}

func (s *Store) saveTaskDispatchNoCtx(teamID string, dispatch TaskDispatch) error {
	if s == nil {
		return NewNilStoreError("Store")
	}
	teamID = NormalizeTeamID(teamID)
	dispatch = normalizeTaskDispatch(dispatch)
	if teamID == "" {
		return NewEmptyIDError("team_id")
	}
	if dispatch.TaskID == "" {
		return NewEmptyIDError("task_id")
	}
	if _, err := s.loadTaskCurrent(teamID, dispatch.TaskID); err != nil {
		return err
	}
	return s.withTeamLock(teamID, func() error {
		if err := os.MkdirAll(s.taskDispatchDir(teamID), 0o755); err != nil {
			return err
		}
		body, err := json.MarshalIndent(dispatch, "", "  ")
		if err != nil {
			return err
		}
		body = append(body, '\n')
		return os.WriteFile(s.taskDispatchPath(teamID, dispatch.TaskID), body, 0o644)
	})
}

func (s *Store) LoadTaskDispatchCtx(ctx context.Context, teamID, taskID string) (TaskDispatch, error) {
	if err := ctxErr(ctx); err != nil {
		return TaskDispatch{}, err
	}
	return s.loadTaskDispatchNoCtx(teamID, taskID)
}

func (s *Store) SaveTaskDispatchCtx(ctx context.Context, teamID string, dispatch TaskDispatch) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	return s.saveTaskDispatchNoCtx(teamID, dispatch)
}

func (s *Store) ListTaskDispatchesCtx(ctx context.Context, teamID string) ([]TaskDispatch, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, NewNilStoreError("Store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, NewEmptyIDError("team_id")
	}
	entries, err := os.ReadDir(s.taskDispatchDir(teamID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]TaskDispatch, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		taskID := strings.TrimSuffix(entry.Name(), ".json")
		dispatch, err := s.loadTaskDispatchNoCtx(teamID, taskID)
		if err != nil {
			continue
		}
		out = append(out, dispatch)
	}
	return out, nil
}
