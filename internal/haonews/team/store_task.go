package team

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

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
	normalizeTaskPhase5Fields(&task)
	if !task.DueAt.IsZero() {
		task.DueAt = task.DueAt.UTC()
	}
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
		if err := s.validateTaskRelationsLocked(teamID, task); err != nil {
			return err
		}
		if !IsValidTransitionWithPolicy("", task.Status, policy) {
			return NewInvalidTransitionError("", task.Status)
		}
		if err := s.validateTaskDependencyProgressLocked(teamID, Task{}, task); err != nil {
			return err
		}
		if IsTerminalState(task.Status) && task.ClosedAt.IsZero() {
			task.ClosedAt = task.UpdatedAt
		}
		return s.appendTaskCurrentLocked(teamID, task)
	})
	if err == nil {
		for _, notification := range buildTaskNotifications(teamID, nil, task, true) {
			_ = s.appendNotificationNoCtx(teamID, notification)
		}
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
		return nil, NewNilStoreError("Store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, NewEmptyIDError("team_id")
	}
	return s.loadTasksCurrent(teamID, limit)
}

func (s *Store) loadTaskNoCtx(teamID, taskID string) (Task, error) {
	if s == nil {
		return Task{}, NewNilStoreError("Store")
	}
	teamID = NormalizeTeamID(teamID)
	taskID = strings.TrimSpace(taskID)
	if teamID == "" {
		return Task{}, NewEmptyIDError("team_id")
	}
	if taskID == "" {
		return Task{}, NewEmptyIDError("task_id")
	}
	return s.loadTaskCurrent(teamID, taskID)
}

func (s *Store) saveTaskNoCtx(teamID string, task Task) error {
	if s == nil {
		return NewNilStoreError("Store")
	}
	teamID = NormalizeTeamID(teamID)
	taskID := strings.TrimSpace(task.TaskID)
	if teamID == "" {
		return NewEmptyIDError("team_id")
	}
	if taskID == "" {
		return NewEmptyIDError("task_id")
	}
	fromState := ""
	current, err := s.loadTaskCurrent(teamID, taskID)
	if err == nil {
		fromState = current.Status
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	err = s.withTeamLock(teamID, func() error {
		policy, err := s.loadPolicyNoCtx(teamID)
		if err != nil {
			return err
		}
		return s.saveTaskCurrentLocked(teamID, task, policy)
	})
	if err == nil {
		var previous *Task
		if fromState != "" || current.TaskID != "" {
			copyCurrent := current
			previous = &copyCurrent
		}
		for _, notification := range buildTaskNotifications(teamID, previous, task, false) {
			_ = s.appendNotificationNoCtx(teamID, notification)
		}
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
		if s.TaskHooks != nil {
			s.TaskHooks.Fire(context.Background(), TaskTransitionEvent{
				TeamID:    teamID,
				Task:      task,
				FromState: normalizeTaskStatus(fromState),
				ToState:   normalizeTaskStatus(task.Status),
				ActorID:   strings.TrimSpace(task.CreatedBy),
			})
		}
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
		return NewInvalidTransitionError(normalizeTaskStatus(current.Status), task.Status)
	}
	task.Priority = normalizeTaskPriority(task.Priority)
	task.ChannelID = normalizeChannelID(task.ChannelID)
	task.ContextID = normalizeContextID(task.ContextID)
	normalizeTaskPhase5Fields(&task)
	if !task.DueAt.IsZero() {
		task.DueAt = task.DueAt.UTC()
	}
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
	if err := s.validateTaskRelationsLocked(teamID, task); err != nil {
		return err
	}
	if err := s.validateTaskDependencyProgressLocked(teamID, current, task); err != nil {
		return err
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

func buildTaskID(task Task) string {
	return strings.Join([]string{
		strings.TrimSpace(task.TeamID),
		strings.TrimSpace(task.CreatedBy),
		task.CreatedAt.UTC().Format(time.RFC3339Nano),
		strings.TrimSpace(task.Title),
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
