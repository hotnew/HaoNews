package team

import (
	"errors"
	"fmt"
	"os"
)

func normalizeTaskPhase5Fields(task *Task) {
	if task == nil {
		return
	}
	task.ParentTaskID = normalizeTaskRefID(task.ParentTaskID)
	task.DependsOn = normalizeTaskRefList(task.DependsOn)
	task.MilestoneID = normalizeMilestoneID(task.MilestoneID)
}

func (s *Store) validateTaskRelationsLocked(teamID string, task Task) error {
	normalizeTaskPhase5Fields(&task)
	if task.ParentTaskID != "" {
		if task.ParentTaskID == task.TaskID {
			return &TeamError{Code: ErrCodeInvalidState, Context: "task cannot parent itself"}
		}
		if _, err := s.loadTaskCurrentLocked(teamID, task.ParentTaskID); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return NewNotFoundError("parent_task_id")
			}
			return err
		}
	}
	for _, depID := range task.DependsOn {
		if depID == task.TaskID {
			return &TeamError{Code: ErrCodeInvalidState, Context: "task cannot depend on itself"}
		}
		if depID == task.ParentTaskID && depID != "" {
			continue
		}
		if _, err := s.loadTaskCurrentLocked(teamID, depID); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return &TeamError{Code: ErrCodeNotFound, Context: fmt.Sprintf("depends_on:%s", depID)}
			}
			return err
		}
	}
	if task.MilestoneID != "" {
		if _, err := s.loadMilestoneNoCtx(teamID, task.MilestoneID); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return NewNotFoundError("milestone_id")
			}
			return err
		}
	}
	return nil
}

func (s *Store) validateTaskDependencyProgressLocked(teamID string, current Task, next Task) error {
	if normalizeTaskStatus(next.Status) != TaskStateDoing {
		return nil
	}
	for _, depID := range normalizeTaskRefList(next.DependsOn) {
		dep, err := s.loadTaskCurrentLocked(teamID, depID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return &TeamError{Code: ErrCodeNotFound, Context: fmt.Sprintf("depends_on:%s", depID)}
			}
			return err
		}
		if !IsTerminalState(dep.Status) {
			return &TeamError{
				Code:    ErrCodeInvalidState,
				Context: fmt.Sprintf("dependency_not_done:%s", depID),
				Err:     NewInvalidTransitionError(current.Status, next.Status),
			}
		}
	}
	return nil
}
