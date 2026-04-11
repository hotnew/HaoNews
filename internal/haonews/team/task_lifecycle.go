package team

import "context"

type TaskTransitionEvent struct {
	TeamID    string
	Task      Task
	FromState string
	ToState   string
	ActorID   string
}

type TaskLifecycleHook interface {
	OnTransition(ctx context.Context, event TaskTransitionEvent)
}

type TaskLifecycleHookFunc func(ctx context.Context, event TaskTransitionEvent)

func (f TaskLifecycleHookFunc) OnTransition(ctx context.Context, event TaskTransitionEvent) {
	f(ctx, event)
}

type HookRegistry struct {
	hooks []TaskLifecycleHook
}

func (r *HookRegistry) Register(hooks ...TaskLifecycleHook) {
	if r == nil {
		return
	}
	r.hooks = append(r.hooks, hooks...)
}

func (r *HookRegistry) Fire(ctx context.Context, event TaskTransitionEvent) {
	if r == nil {
		return
	}
	for _, hook := range r.hooks {
		hook := hook
		go hook.OnTransition(ctx, event)
	}
}
