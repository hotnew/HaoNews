# Team 开发架构说明

## 目标

- 给后续 Team / Room Plugin / Agent 编排改动一个稳定的中层入口。
- 避免继续把新能力直接堆进 `internal/haonews/team/store.go` 和各 handler。

## 当前中层

- `internal/haonews/team/constants.go`
  - Team 角色、任务状态、默认策略等集中常量。
- `internal/haonews/team/errors.go`
  - `TeamError`、稳定 `ErrorCode`、高频 sentinel errors。
- `internal/haonews/team/filters.go`
  - `TaskFilter / ArtifactFilter / MessageFilter`。
- `internal/haonews/team/enforcer.go`
  - `PolicyEnforcer`，统一权限判断入口。
- `internal/haonews/team/task_lifecycle.go`
  - `TaskLifecycleHook`，任务状态副作用扩展点。
- `internal/haonews/team/context_provider.go`
  - `ChannelContextProvider`，Agent/LLM 获取频道上下文的正式入口。
- `internal/haonews/team/task_dispatch.go`
  - `TaskDispatch` 正式实体。
- `internal/haonews/team/task_thread.go`
  - `Task + Message` 的 thread 聚合。
- `internal/haonews/team/notification.go`
  - 通知落盘、列表、SSE。
- `internal/haonews/team/member_stats.go`
  - 成员活跃/贡献统计。
- `internal/haonews/team/task_dependency.go`
  - 任务依赖、父子任务、里程碑校验。
- `internal/haonews/team/milestone.go`
  - 里程碑实体、聚合进度。
- `internal/haonews/team/team_template.go`
  - 内置 Team 模板和创建链路。
- `internal/haonews/team/interfaces.go`
  - `TeamReader / TeamWriter / TeamStore` 接口边界。

## 新能力应该挂在哪

- 新权限规则：
  - 先挂 `PolicyEnforcer`，不要在 handler 内散落判断。
- 新任务副作用：
  - 先挂 `TaskLifecycleHook`，不要直接把通知/历史/派发逻辑塞进保存函数。
- Agent/LLM 需要上下文：
  - 优先走 `ChannelContextProvider` 和 `TaskThread`。
- 新通知类型：
  - 先扩 `notification.go`，再接 API/SSE。
- 新模板：
  - 优先扩 `team_template.go` 的 built-in template。
- 新冲突策略：
  - 先扩 `team/sync.go` 的 conflict 判定，再扩 `internal/haonews/team_sync.go` 的 runtime 持久化和 resolve 动作。

## Store 拆分方向

- 当前 `store.go` 仍然偏大。
- 后续继续按下面职责拆：
  - `store_team.go`
  - `store_member.go`
  - `store_policy.go`
  - `store_channel.go`
  - `store_message.go`
  - `store_task.go`
  - `store_artifact.go`
  - `store_history.go`
  - `store_webhook.go`

原则：

- 先拆文件和接口边界，再迁移调用点。
- 不做一次性无验证的大搬家。
- 对外 API、TeamSync、Room Plugin 语义优先保持兼容。
