# `20260410-team-codex.auto-run.md`

## Goal

- 在现有 HaoNews Team 基线之上，完成一次面向长期演进的中层重构与能力补齐：先把 Team 包改造成对 LLM 和后续插件开发更友好的结构，再落地 Agent 编排、Thread/Context、通知/唤醒、任务依赖等高优先级能力，且保持现有 Team / Room Plugin / TeamSync 主路径兼容可运行。

## Context

- 仓库 / 工作目录：
  - `/Users/haoniu/sh18/hao.news2/haonews`
- 输入材料：
  - `/Users/haoniu/sh18/hao.news2/haonews/20260410/20260410-team-claude.md`
  - `/Users/haoniu/sh18/hao.news2/haonews/20260410/20260410-team-claude-code.md`
  - `/Users/haoniu/sh18/hao.news2/runbook-template.auto-run.md`
- 当前已知基线：
  - `internal/haonews/team/` 已经拆出 `channel_config.go / sync.go / ctx_api.go / compat_api.go / webhook_delivery.go / product_metrics.go`
  - `internal/plugins/haonewsteam/` 已有 Team 页面、Search、Sync、Webhook，以及 6 个内置 Room Plugin
  - Room Plugin Registry 已存在：`internal/plugins/haonewsteam/roomplugin/registry.go`
  - TeamSync 已支持 `channel_config`，`.75 -> GitHub -> .74` 的 `channel_config` 自动同步已验证过
  - 现状中仍存在：
    - `store.go` 承载大量核心类型与多类存取逻辑
    - 权限检查和副作用扩展点不够显式
    - 错误仍大量使用字符串错误
    - `ContextID` / Thread / Agent Dispatch / Notification 等能力尚未形成正式主链
- 本次不是“小修页面”，而是“先搭长期架构，再挂产品能力”的大修改。
- 兼容性约束：
  - 不允许破坏当前 Team 页面、现有 Room Plugin、TeamSync、Webhook、A2A、SSE 的已上线语义
  - 优先新增字段、接口、文件和兼容层；只有在验证充分且替换收益明确时才收旧路径

## Execution Contract

- 读完整个文档后立即开始执行。
- 在本 runbook 完成或确认硬阻塞之前，不要向用户提问。
- 遇到小问题、小缺口、小歧义时，直接做最合理的低风险判断并继续。
- 如果存在多条可行路径，优先选择：
  - 风险更低的路径
  - 更可回滚的路径
  - 更符合当前代码和文档现状的路径
- 不要因为普通不确定性停下来等待确认。
- 只有在出现真实硬阻塞时才允许停止。
- 如果出现硬阻塞：
  - 先尝试本文件中已有 fallback
  - 再尝试本地可验证的合理 fallback
  - 如果仍无法继续，把阻塞信息、已尝试动作、当前结论、精确下一步写回本文档，然后停止
- 执行过程中持续更新 checklist 状态，不要只在最后统一回填。

## Planning Rules

- 按“基础中层 -> 扩展点 -> 高优先级产品能力 -> 次级产品能力 -> 文档/验证”排序，不按原始文档叙事顺序排序。
- 先做能显著降低后续改动难度的中层项：
  - 常量集中化
  - 类型化错误
  - Filter 结构体
  - PolicyEnforcer
  - TaskLifecycleHook
  - ChannelContextProvider
  - Sync Validate / Builder
- 再做依赖这些中层能力的功能项：
  - Agent 编排 / 自动派发
  - Thread / Context 深绑定
  - 通知中心 / SSE 唤醒
  - 频道级 AI Context 注入
- 高风险项必须后置：
  - Store 文件大拆分
  - 任务依赖 / 里程碑
  - Team 模板
  - 冲突自动合并
- 任何一步如果可以通过“新文件 + 接口接入 + 兼容老入口”完成，不要先做破坏式替换。

## Execution Plan

### Phase 1. Ground Truth / Baseline

- [x] 只读取最小必要代码，确认以下基线并写回：
  - `internal/haonews/team/store.go`
  - `internal/haonews/team/sync.go`
  - `internal/haonews/team/ctx_api.go`
  - `internal/haonews/team/channel_config.go`
  - `internal/plugins/haonewsteam/plugin.go`
  - `internal/plugins/haonewsteam/handler*.go`
- [x] 明确哪些建议已部分存在，哪些完全未落地：
  - 常量
  - 类型化错误
  - Enforcer
  - Hook Registry
  - Filter
  - Context Provider
  - Sync Validate / Builders
  - Dispatch / Notification / Thread / Milestone / Template
- [x] 建立本轮最小修改清单：
  - 首轮必须补的中层文件
  - 首轮必须接入的调用点
  - 首轮必须带上的测试

### Phase 2. P0 中层基建：让 Team 对 LLM/后续开发友好

- [x] 新增 `internal/haonews/team/constants.go`
  - 集中成员角色、成员状态、任务优先级、Artifact kind、默认 policy
  - 替换散落字符串的核心调用点
- [x] 新增 `internal/haonews/team/errors.go`
  - 定义 `TeamError / ErrorCode / sentinel errors / New*Error`
  - 先替换最核心入口里的高频字符串错误：
    - `nil store`
    - `empty team id`
    - `not found`
    - `invalid transition`
    - `forbidden`
- [x] 新增 `internal/haonews/team/filters.go`
  - 定义 `TaskFilter / ArtifactFilter / MessageFilter`
  - 接入现有 `List*Ctx` 主入口，不要求一轮替换所有旧签名，但必须让新 Filter 形式可用
- [x] 新增 `internal/haonews/team/enforcer.go`
  - 定义 `PolicyEnforcer`
  - 定义集中 `Action*` 常量
  - 提供 `StorePolicyEnforcer`
  - 接入至少一条真实主路径：
    - task update / transition
    - member update
    - policy update
- [x] 新增 `internal/haonews/team/task_lifecycle.go`
  - 定义 `TaskTransitionEvent / TaskLifecycleHook / HookRegistry`
  - 在 Task 状态更新主链中接入 Hook 触发点，默认不改变行为
- [x] 新增 `internal/haonews/team/context_provider.go`
  - 定义 `ChannelContext / ChannelContextProvider`
  - 默认实现基于 Store 和现有 `ChannelConfig`
- [x] 为 `TeamSyncMessage` 增加：
  - `Validate()`
  - `NewMessageSyncMsg / NewTaskSyncMsg / NewMemberSyncMsg` 等 builder
  - 在发送前或测试中至少接入一条真实校验链

### Phase 3. P0 产品能力：Agent 编排 + Channel Context + Thread 主链

- [x] 新增 `TaskDispatch` 数据模型与最小持久化主链
  - 字段至少覆盖：
    - `task_id`
    - `assigned_agent_id`
    - `match_reason`
    - `status`
    - `dispatched_at`
    - `acked_at`
    - `completed_at`
    - `retry_count`
    - `timeout_seconds`
  - 不要求首轮做复杂调度器，但必须让“派发决策”成为正式实体
- [x] 在任务状态机里加入兼容式 `dispatched` 中间态
  - 保持现有 `open / doing / blocked / review / done` 可兼容读取
  - 新状态必须经过 transition 校验
- [x] 在 AgentCard 或相关状态结构上补最小健康/负载字段
  - 至少支持：
    - queue length
    - last response / heartbeat
- [x] 增加 Channel Context API
  - `GET /api/teams/{teamID}/channels/{channelID}/context`
  - 返回 JSON
  - 首轮可不做 Markdown 变体
- [x] 为 Thread 建正式聚合主链
  - 先优先做“逻辑索引 + API 聚合”，不强求首轮直接落单独 `threads/{context}.jsonl`
  - 至少补：
    - `GET /api/teams/{teamID}/tasks/{taskID}/thread`
    - `Message.ParentMessageID` 可选字段
    - Task/Message 通过 `ContextID` 聚合
- [x] 在 ChannelConfig.AgentOnboarding 的使用链里接入 Thread 摘要 / ChannelContext
  - 目标是 LLM Agent 能一次拿到当前频道和关联任务的上下文摘要

### Phase 4. P1 产品能力：通知 / 唤醒 / 贡献统计

- [x] 新增 `Notification` 实体与最小持久化
  - 至少支持：
    - `mention`
    - `task_assigned`
    - `task_blocked`
    - `review_needed`
- [x] Message 解析 `@agentID`
  - 自动写通知
- [x] 新增通知 API + SSE：
  - `GET /api/teams/{teamID}/notifications`
  - `GET /api/teams/{teamID}/notifications/stream`
- [x] 新增 `MemberStats`
  - 首轮支持按天聚合即可
  - 至少输出：
    - message count
    - task created / closed
    - artifact count
    - last active at
- [x] 成员页或 Team API 暴露基础贡献统计摘要

### Phase 5. P2 能力：任务依赖 / 里程碑 / 模板

- [x] 扩展 `Task`：
  - `parent_task_id`
  - `depends_on`
  - `milestone_id`
- [x] 新增 `Milestone` 实体与最小 CRUD / 聚合进度
- [x] 在状态推进处增加依赖检查
  - 若前置任务未完成，不允许推进到 `doing`
  - 通过 policy 或显式检查保证可扩展
- [x] 新增 `TeamTemplate`
  - 支持 `incident-response / code-review / planning`
- [x] API 支持：
  - `POST /api/teams?from_template={templateID}`
- [x] 模板默认带频道、policy、插件配置和默认 agent/role 映射

### Phase 6. P2/P3 能力：冲突自动化 + Store 结构化拆分

- [x] 为 `TeamSyncConflict` 增加自动可解字段与自动策略基础：
  - `auto_resolvable`
  - 至少接入：
    - `message` 总是追加
    - `task.status` 按时间戳 LWW
    - `policy` 保持人工确认
- [x] 将 `store.go` 逐步拆成职责文件
  - `store.go` 只保留 `Store`、OpenStore、锁工具
  - 至少拆出：
    - `store_team.go`
    - `store_member.go`
    - `store_policy.go`
    - `store_channel.go`
    - `store_message.go`
    - `store_task.go`
    - `store_artifact.go`
    - `store_history.go`
    - `store_webhook.go`
- [x] 新增 `interfaces.go`
  - `TeamReader / TeamWriter / TeamStore`
  - 让 handler / 搜索 / context provider 优先依赖接口而非完整 Store

### Phase 7. Tests / Fixtures / Docs / Comment Contract

- [x] 新增 `internal/haonews/team/testfixtures/fixtures.go`
  - 至少提供：
    - `NewTeam`
    - `NewMember`
    - `NewTask`
    - `StandardTeamScenario`
- [x] 为本轮新增能力补最小闭环测试：
  - typed errors
  - enforcer
  - task hook
  - channel context provider
  - thread API
  - dispatch 主链
  - notification API / stream
  - template / milestone / dependency 检查
  - sync validate / builders
- [x] 补 Team 开发文档：
  - 新增文件的职责说明
  - 扩展点说明
  - 后续新增 Room Plugin / Agent 编排能力时应该挂在哪
- [x] 对关键 `Save* / Load* / List*` 入口补 LLM 友好的前置条件 / 副作用注释

### Phase 8. Finish / Writeback

- [x] 更新本文档 checklist 状态。
- [x] 写明最终结果：
  - 哪些 Phase 完成
  - 哪些能力 defer
  - 哪些点只是兼容层，尚未彻底替换旧路径

## Verification

- 首选验证命令：
  - `cd /Users/haoniu/sh18/hao.news2/haonews && go test ./internal/haonews/team ./internal/plugins/haonewsteam -count=1`
  - `cd /Users/haoniu/sh18/hao.news2/haonews && go test ./internal/haonews -run 'TestTeam|TestWriteTeamSyncState|TestTeamPubSubRuntime' -count=1`
  - `cd /Users/haoniu/sh18/hao.news2/haonews && go build ./cmd/haonews`
- 若新增 API 已落地，追加最小 smoke：
  - `GET /api/teams/{teamID}/channels/{channelID}/context`
  - `GET /api/teams/{teamID}/tasks/{taskID}/thread`
  - `GET /api/teams/{teamID}/notifications`
  - `GET /api/teams/{teamID}/notifications/stream`
- 若模板、派发、里程碑已落地，追加直接验证：
  - 创建 Team from template
  - 创建/推进带依赖任务
  - 触发 task dispatch / notification / hook
- 完成标准：
  - [x] P0 中层基建已形成正式文件和调用点，不只是文档占位
  - [x] 至少一条 Agent Dispatch 主链可运行
  - [x] 至少一条 Thread / Context API 主链可运行
  - [x] 至少一条 Notification / SSE 主链可运行
  - [x] 高优先级新增能力有真实测试或可执行 smoke 证明
  - [x] 最终汇报包含 `Completed / Blocked / Next Step`

## Fallback Rules

- 默认优先：
  - 现有 Team Store / TeamSync / RoomPlugin / ChannelConfig 主链
  - 兼容式新增字段和接口
  - 最小行为变更
- 如果某项建议过大，优先按以下顺序降阶：
  - 先做正式数据结构和 API
  - 再做最小持久化
  - 最后再做自动化策略
- Thread 若首轮独立 `threads/{context}.jsonl` 成本过高：
  - 先基于现有 message/task/history 索引聚合
  - 但 API 契约必须先落地
- Store 大拆分若一次性改动过大：
  - 先拆新增文件和接口
  - 再逐段迁移调用点
  - 不允许一口气做无验证的大搬家
- 不要采用：
  - 一次性重写 Team 数据模型
  - 引入新的外部数据库 / 队列依赖
  - 为了“更优雅”破坏现有 Team 页面和 TeamSync 兼容语义

## Blockers / Resume

- 硬阻塞定义：
  - 缺失必须的权限 / 密钥 / 外部访问
  - 文档要求与现有线上兼容性存在无法安全自动裁决的直接冲突
  - 某项能力若继续实现将必然破坏现有 TeamSync / RoomPlugin 主链，且无安全兼容方案
  - 本地上下文不足以继续，且任何合理 fallback 都无法验证
- 如果阻塞，必须写回：
  - `Blocked on`: 卡在哪
  - `Tried`: 已尝试过什么
  - `Why blocked`: 为什么这些尝试还不够
  - `Exact next step`: 下次恢复时第一步做什么
- 恢复执行时：
  - 先读完整个文档
  - 从最后一个未完成且未失效的 Phase 继续
  - 不回头重做已验证通过的步骤

## Status Writeback

- `Completed`:
  - `Phase 1` 已完成：最小必要基线已核对，确认当前 Team 已具备 RoomPlugin / ChannelConfig / TeamSync / Team 页面基础，但缺 typed errors、enforcer、hooks、filters、context provider、sync validate 等显式中层。
  - `Phase 2` 已完成：
    - 新增 `constants.go`
    - 新增 `errors.go`
    - 新增 `filters.go`
    - 新增 `enforcer.go`
    - 新增 `task_lifecycle.go`
    - 新增 `context_provider.go`
    - 为 `TeamSyncMessage` 增加 `Validate()` 和 builder
    - 将主 Team handler 和各 room handler 的权限检查统一接到 `PolicyEnforcer`
    - 在 Task 保存链接入 `TaskHooks`
  - `Phase 3` 已完成：
    - 已落地 `GET /api/teams/{teamID}/channels/{channelID}/context`
    - `ChannelContextProvider` 已接入真实 API 和插件测试
    - 已新增 `TaskDispatch` 正式数据模型与最小持久化：
      - `internal/haonews/team/task_dispatch.go`
      - `GET/POST /api/teams/{teamID}/tasks/{taskID}/dispatch`
    - 已将 `TaskStateDispatched` 接入状态机：
      - `open -> dispatched`
      - `dispatched -> doing|blocked|cancelled|failed`
    - 已新增 Thread 聚合主链：
      - `internal/haonews/team/task_thread.go`
      - `GET /api/teams/{teamID}/tasks/{taskID}/thread`
    - `Message.ParentMessageID` 已进入正式结构、normalize、sync normalize 与测试链
    - 已把 `ChannelConfig.AgentOnboarding` 接到正式 agent prompt 使用链：
      - `ChannelContextProvider` 现在会输出 `threads` 与 `agent_prompt`
      - `GET /api/teams/{teamID}/channels/{channelID}/context` 返回可直接消费的频道 prompt
      - `GET /api/teams/{teamID}/channels/{channelID}/config` 返回 `agent_prompt / context_api_path / thread_api_prefix`
      - Team 频道工作台与 `minimal/focus/board` 主题已显示 `Agent Prompt Preview`
    - `AgentCard` 已补最小健康/负载字段：
      - `queue_length`
      - `last_heartbeat_at`
      - `last_response_at`
      - Team agent card API 和 store 测试已覆盖
    - store / plugin 两层已补真实测试，验证 dispatch 与 thread API 可运行
  - `Phase 4` 已完成：
    - 已新增 `internal/haonews/team/notification.go`
    - Notification 已支持：
      - `mention`
      - `task_assigned`
      - `task_blocked`
      - `review_needed`
    - Message 已解析 `@agent://...` 并自动写通知
    - Task create / blocked / review 已自动写通知
    - 已新增：
      - `GET /api/teams/{teamID}/notifications`
      - `GET /api/teams/{teamID}/notifications/stream`
    - 通知流会先补发最近通知，再继续实时流
    - 已新增 `internal/haonews/team/member_stats.go`
    - `MemberStats` 已输出：
      - `message_count`
      - `task_created`
      - `task_closed`
      - `artifact_count`
      - `last_active_at`
    - 成员页和 `GET /api/teams/{teamID}/members` 已暴露基础贡献统计摘要
  - `Phase 5` 已完成：
    - `Task` 已扩展：
      - `parent_task_id`
      - `depends_on`
      - `milestone_id`
    - 已新增 `internal/haonews/team/milestone.go`
    - 已落地里程碑 CRUD / 聚合进度：
      - `GET/POST /api/teams/{teamID}/milestones`
      - `GET/PUT/DELETE /api/teams/{teamID}/milestones/{milestoneID}`
    - 任务状态推进已接入依赖检查：
      - 前置任务未完成时，不允许进入 `doing`
    - 已新增 `internal/haonews/team/team_template.go`
    - 已支持：
      - `incident-response`
      - `code-review`
      - `planning`
    - 已落地：
      - `POST /api/teams?from_template={templateID}`
    - 模板已可带：
      - 频道
      - policy
      - channel config
      - agent/role 绑定
      - seed milestones
  - `Phase 6` 已完成：
    - `TeamSyncConflict` 已新增 `auto_resolvable`
    - 已接入自动策略基础：
      - `message` 继续按追加语义
      - `task.status` 冲突只在非状态字段一致时标为 `auto_resolvable`
      - `ResolveTeamSyncConflict(..., action=auto)` 已可按时间戳自动收敛 task conflict
      - `policy` 仍保持人工确认
    - Sync API / 页面 / 冲突动作已回显 `auto_resolvable` 与 `自动收敛`
    - 已新增 `internal/haonews/team/interfaces.go`
      - `TeamReader / TeamWriter / TeamStore`
    - `ChannelContextProvider` 已优先依赖 `TeamReader`
    - Team Search 主链已优先依赖 `TeamReader`
      - `handler_search.go` 已改成接口读取，不再要求完整 `*Store`
    - `TeamReader / TeamWriter` 已扩展到覆盖：
      - filter-style `ListTasksCtx / ListMessagesCtx`
      - load-style task / message / history / artifact / thread 读取
      - 核心 append/write 入口
    - `store.go` 已开始结构化拆分，首批已挪出：
      - `store_team.go`
      - `store_member.go`
      - `store_policy.go`
      - `store_channel.go`
      - `store_webhook.go`
      - `store_artifact.go`
      - `store_history.go`
      - `store_message.go`
      - `store_task.go`
    - 本轮继续拆出：
      - `types.go`
      - channel / policy / task / artifact / history 的索引与辅助实现已迁出 `store.go`
    - 本轮继续完成接口化读取边界：
      - `handleTeamMembers / handleAPITeamMembers` 的只读链已依赖 `TeamReader`
      - `handleAPITeamNotifications / handleAPITeamNotificationsStream` 已依赖 `TeamReader`
      - `TeamReader` 已补：
        - `ComputeMemberStatsCtx`
        - `ListNotificationsCtx`
        - `Subscribe`
      - `APITeamMembers` 的 `POST` 写入分支显式回落到可写 `*Store`，保持兼容
    - 当前 `store.go` 已进一步收敛到：
      - `Store` 本体
      - `OpenStore / Root`
    - 运行时事件 / webhook / 通用 helper 已继续迁出：
      - `store_runtime.go`
      - `store_shared.go`
    - `store.go` 现已不再承载消息 ID/helper、订阅发布、webhook 运行时实现
  - `Phase 7` 已完成：
    - 已新增 `internal/haonews/team/testfixtures/fixtures.go`
      - `NewTeam`
      - `NewMember`
      - `NewTask`
      - `StandardTeamScenario`
    - 已新增 `internal/haonews/team/testfixtures/fixtures_test.go`
    - 已补强本轮测试：
      - typed errors
      - enforcer
      - task hook
      - channel context provider
      - thread API
      - dispatch 主链
      - notification API / stream
      - task dependency / milestone / template
      - `task.status` auto-resolvable sync conflict
      - sync page `自动收敛` 动作
      - search API / team detail / team notifications
    - 已新增开发文档：
      - `doc-md/team-dev-architecture.md`
    - 已补关键 ctx API 注释：
      - `Load* / Save* / List* / Append*` 主入口现已标注前置条件 / 副作用
    - 已按 runbook 验证口径跑通：
      - `go test ./internal/haonews/team/... ./internal/plugins/haonewsteam -count=1`
      - `go test ./internal/haonews -run 'TestTeam|TestWriteTeamSyncState|TestTeamPubSubRuntime' -count=1`
      - `go build ./cmd/haonews`
- `Blocked`:
  - None
- `Next Step`:
  - 本 runbook 已完成。
  - 后续若继续，不再属于本文件必做项，最自然的是：
    - 继续把更多 mutation-heavy handler 从 `*Store` 收到更细接口
    - 或进入新的 Team 产品/架构 runbook
