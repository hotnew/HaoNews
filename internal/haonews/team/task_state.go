package team

import "strings"

const (
	TaskStateOpen      = "open"
	TaskStateDoing     = "doing"
	TaskStateBlocked   = "blocked"
	TaskStateReview    = "review"
	TaskStateDone      = "done"
	TaskStateFailed    = "failed"
	TaskStateCancelled = "cancelled"
	TaskStateRejected  = "rejected"
)

type TaskTransitionRule struct {
	Allowed []string `json:"allowed,omitempty"`
}

func IsTerminalState(value string) bool {
	switch normalizeTaskStatus(value) {
	case TaskStateDone, TaskStateFailed, TaskStateCancelled, TaskStateRejected:
		return true
	default:
		return false
	}
}

func IsValidTransition(from, to string) bool {
	from = normalizeTaskStatus(from)
	to = normalizeTaskStatus(to)
	if to == "" {
		to = TaskStateOpen
	}
	if from == "" {
		return inNormalizedStatusSet(to, TaskStateOpen, TaskStateDoing, TaskStateBlocked, TaskStateReview, TaskStateDone, TaskStateFailed, TaskStateCancelled, TaskStateRejected)
	}
	if from == to {
		return true
	}
	switch from {
	case TaskStateOpen:
		return inNormalizedStatusSet(to, TaskStateDoing, TaskStateBlocked, TaskStateReview, TaskStateDone, TaskStateFailed, TaskStateCancelled, TaskStateRejected)
	case TaskStateDoing:
		return inNormalizedStatusSet(to, TaskStateBlocked, TaskStateReview, TaskStateDone, TaskStateFailed, TaskStateCancelled)
	case TaskStateBlocked:
		return inNormalizedStatusSet(to, TaskStateDoing, TaskStateCancelled, TaskStateFailed)
	case TaskStateReview:
		return inNormalizedStatusSet(to, TaskStateDoing, TaskStateDone, TaskStateRejected, TaskStateCancelled)
	default:
		return false
	}
}

func IsValidTransitionWithPolicy(from, to string, policy Policy) bool {
	from = normalizeTaskStatus(from)
	to = normalizeTaskStatus(to)
	if to == "" {
		to = TaskStateOpen
	}
	if len(policy.TaskTransitions) == 0 {
		return IsValidTransition(from, to)
	}
	rule, ok := policy.TaskTransitions[from]
	if !ok {
		return IsValidTransition(from, to)
	}
	if from == to {
		return true
	}
	for _, candidate := range rule.Allowed {
		if normalizeTaskStatus(candidate) == to {
			return true
		}
	}
	return false
}

func normalizeTaskTransitions(in map[string]TaskTransitionRule) map[string]TaskTransitionRule {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]TaskTransitionRule, len(in))
	for from, rule := range in {
		from = normalizeTaskStatus(from)
		if strings.TrimSpace(from) == "" {
			from = ""
		}
		allowed := make([]string, 0, len(rule.Allowed))
		seen := make(map[string]struct{}, len(rule.Allowed))
		for _, next := range rule.Allowed {
			next = normalizeTaskStatus(next)
			if next == "" {
				next = TaskStateOpen
			}
			if _, ok := seen[next]; ok {
				continue
			}
			seen[next] = struct{}{}
			allowed = append(allowed, next)
		}
		out[from] = TaskTransitionRule{Allowed: allowed}
	}
	return out
}

func inNormalizedStatusSet(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == normalizeTaskStatus(candidate) {
			return true
		}
	}
	return false
}
