package team

import (
	"context"
	"sort"
)

type TaskThread struct {
	Task     Task          `json:"task"`
	Dispatch *TaskDispatch `json:"dispatch,omitempty"`
	Messages []Message     `json:"messages,omitempty"`
}

func (s *Store) LoadTaskThreadCtx(ctx context.Context, teamID, taskID string, limit int) (TaskThread, error) {
	task, err := s.LoadTaskCtx(ctx, teamID, taskID)
	if err != nil {
		return TaskThread{}, err
	}
	thread := TaskThread{Task: task}
	if dispatch, err := s.LoadTaskDispatchCtx(ctx, teamID, taskID); err == nil {
		thread.Dispatch = &dispatch
	}
	merged := make([]Message, 0)
	seen := make(map[string]struct{})
	appendMessages := func(messages []Message) {
		for _, message := range messages {
			if _, ok := seen[message.MessageID]; ok {
				continue
			}
			seen[message.MessageID] = struct{}{}
			merged = append(merged, message)
		}
	}
	messages, err := s.LoadTaskMessagesCtx(ctx, teamID, taskID, limit)
	if err != nil {
		return TaskThread{}, err
	}
	appendMessages(messages)
	if task.ContextID != "" {
		contextMessages, err := s.LoadMessagesByContextCtx(ctx, teamID, task.ContextID, limit)
		if err != nil {
			return TaskThread{}, err
		}
		appendMessages(contextMessages)
	}
	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].CreatedAt.After(merged[j].CreatedAt)
	})
	thread.Messages = merged
	if limit > 0 && len(thread.Messages) > limit {
		thread.Messages = append([]Message(nil), thread.Messages[:limit]...)
	}
	return thread, nil
}
