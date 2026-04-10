# Team Room Plugin

`Team Room Plugin` 是挂载在 `Team Channel` 上的二级插件层，用来把某个频道收成有明确业务语义的协作房间，而不是继续把所有逻辑都塞回 Team 主干。

## 目标

- Team 主干继续负责：
  - members
  - channels
  - messages
  - tasks
  - artifacts
  - history
  - sync / webhook / A2A / SSE
- Room Plugin 只负责：
  - 某个频道上的专属业务逻辑
  - 专属消息类型
  - 专属产物提炼
  - 专属页面 / API 视图

## 关键结构

Room Plugin 通过 `internal/plugins/haonewsteam/roomplugin/registry.go` 注册。

核心接口：

```go
type RoomPlugin interface {
    ID() string
    Manifest() Manifest
    Handler(store *team.Store, teamID string) http.Handler
}
```

路由挂载后：

- Web:
  - `/teams/{teamID}/r/{pluginID}/...`
- API:
  - `/api/teams/{teamID}/r/{pluginID}/...`

Team 主干会先做 Team 级路由分发，再把剩余路径交给对应 Room Plugin。

## Channel Config

Room Plugin 与 Room Theme 通过 `ChannelConfig` 绑定到频道。

canonical 存储位置：

- `store/team/{teamID}/channels/{channelID}/channel_config.json`

兼容读取：

- 旧路径 `store/team/{teamID}/channel-configs/{channelID}.json` 仍可读取
- 当前采用“双读单写”：
  - 读：新路径优先，旧路径回退
  - 写：只写新路径

主要字段：

```json
{
  "channel_id": "research",
  "plugin": "plan-exchange@1.0",
  "theme": "minimal",
  "agent_onboarding": "Use plan mode first.",
  "rules": ["Keep decisions explicit"]
}
```

说明：

- `plugin` 使用 `pluginID@version`
- `theme` 使用 Room Theme ID
- 文件缺失时回退到零值，不影响频道默认行为

相关 API：

- `GET /api/teams/{teamID}/channel-configs`
- `GET /api/teams/{teamID}/channels/{channelID}/config`
- `PUT /api/teams/{teamID}/channels/{channelID}/config`
- `POST /teams/{teamID}/channels/{channelID}/config/update`

从 `v0.5.83+` 开始，频道页本身已经提供 Room 配置工作台：

- 可直接选择 `Room Plugin`
- 可直接选择 `Theme`
- 可直接编辑 `Agent Onboarding`
- 可直接维护逐行 `Rules`
- 对已经套用 Theme 的频道，可通过 `?view=channel` 回到频道工作台，不需要手写 JSON

从 `v0.5.81` 开始，`ChannelConfig` 也进入 Team P2P 同步主链：

- `.75` 本地创建或更新频道配置后，会通过 `team_sync` 发布 `channel_config`
- `.74` 和其它 LAN 节点会自动应用对应快照
- 不再需要在远端节点手工重复 `PUT /api/teams/{teamID}/channels/{channelID}/config`

这条自动同步已经按 `.75 -> GitHub/tag -> .74` 的固定流程做过实机验证：

- `.75` 创建新频道：
  - `planxsync-1775355215`
- `.75` 写入配置：
  - `plugin = plan-exchange@1.0`
  - `theme = minimal`
- `.74` 在未手工补写的前提下自动出现：
  - `GET /api/teams/archive-demo/channels/planxsync-1775355215/config`
  - `GET /api/teams/archive-demo/channel-configs`
  - `GET /api/teams/archive-demo`
  - `GET /teams/archive-demo/r/plan-exchange/?channel_id=planxsync-1775355215&actor_agent_id=agent://pc75/openclaw01`

## Room Plugin Manifest

每个 Room Plugin 都带一份 `roomplugin.json`：

```json
{
  "id": "plan-exchange",
  "name": "规划交流插件",
  "version": "1.0.0",
  "minTeamVersion": "0.2.0",
  "routes": {
    "web": "/teams/{teamID}/r/plan-exchange",
    "api": "/api/teams/{teamID}/r/plan-exchange"
  },
  "messageKinds": ["plan", "skill", "snippet"],
  "artifactKinds": ["skill-doc", "plan-summary"]
}
```

`haonewsteam` 的主 manifest 也会列出内置 Room Plugins：

```json
{
  "room_plugins": ["artifact-room", "decision-room", "handoff-room", "incident-room", "plan-exchange", "review-room"]
}
```

## plan-exchange

首个内置 Room Plugin 是 `plan-exchange`。

用途：

- 在某个 Team Channel 里交换：
  - `plan`
  - `skill`
  - `snippet`
- 再把 `skill` 提炼成 `Artifact`

接口：

- `GET /api/teams/{teamID}/r/plan-exchange/?channel_id=main&kind=skill`
- `POST /api/teams/{teamID}/r/plan-exchange/messages`
- `POST /api/teams/{teamID}/r/plan-exchange/distill`

Web 页面：

- `GET /teams/{teamID}/r/plan-exchange/`

当前 web 页面已经补齐：

- `plan / skill / snippet` 三类过滤
- 三类独立结构化表单
- Skill 卡片上的“提炼为 Skill 文档”动作
- 已提炼 Skill 的页面标记

## review-room

第二个内置 Room Plugin 是 `review-room`。

用途：

- 在某个 Team Channel 里整理：
  - `review`
  - `risk`
  - `decision`
- 再把消息提炼成：
  - `review-summary`

接口：

- `GET /api/teams/{teamID}/r/review-room/?channel_id=main`
- `POST /api/teams/{teamID}/r/review-room/messages`
- `POST /api/teams/{teamID}/r/review-room/distill`

Web 页面：

- `GET /teams/{teamID}/r/review-room/`

当前页面已收口：

- `review / risk / decision` 三类过滤
- 当前频道的评审类型统计
- `Summary API`
  - `GET /api/teams/{teamID}/r/review-room/summary?channel_id=...`
- 直达 `Review Summary` 和 Team 历史入口
- 卡片级“提炼为 Review Summary”动作
- 默认页的 `decision / risk / review` 三栏 lane 摘要
- lane 状态：
  - `待沉淀`
  - `待跟进`
  - `已提炼`
- 结构化字段直出：
  - 摘要
  - 决策 / 建议 / 影响
  - 后续动作 / 检查项 / 缓解动作
- 工作台状态分组：
  - `待沉淀决策`
  - `待跟进风险`
  - `最近已提炼`
- 已提炼卡片可直接打开对应 `Review Summary` Artifact
- 进一步的沉淀摘要：
  - `决策沉淀`
  - `最近产物`
  - Summary API 中的：
    - `decision_digests`
    - `artifact_digests`
- `decision_digests` 现在会继续暴露：
  - `artifact_count`
  - `latest_artifact_title`
  - `latest_artifact_link`
  用来把“结论本身”和“最新沉淀结果”串起来
- `decision_threads` 现在会把：
  - `decision`
  - `risk`
  - `review`
  聚到同一个结论下，并暴露：
  - `risk_count`
  - `review_count`
  - `open_risk_count`
  - `pending_review_count`
  - `latest_artifact_link`
- 每条结论线程现在也补了 Team 主链直达入口：
  - `task_search_link`
  - `artifact_search_link`
  - `history_search_link`
  让 `review-room` 能直接回到 Team 的任务、产物、历史视图继续推进
- 从当前版本开始，`review-room` 消息还可以显式携带：
  - `task_id`
  - `artifact_id`
  这样结论线程不只会给搜索入口，还能直接给出：
  - 绑定任务页
  - 绑定产物页
- 从当前版本开始，`review-room` 的结论线程还可以直接推动 Team 主链动作：
  - `标记任务进行中`
  - `标记任务完成`
  - `沉淀结论线程`
  这些动作继续复用 Team 现有的 `task.update` / `artifact.create` 主链，不引入新存储格式。
- 从当前版本开始，`review-room` 还增加了一个确定性的 `自动同步线程` 动作：
  - 若存在未跟进风险，建议并自动推到 `blocked`
  - 若评审仍在处理中，建议并自动推到 `doing`
  - 若结论已沉淀且没有待处理评审，建议并自动推到 `done`
  - 若当前线程还没有结论级 `Review Summary`，会自动补一份线程级沉淀 Artifact
  - 若当前线程还没有绑定 Task，会自动创建一个 Team Task，并通过线程级 Artifact 把绑定稳定下来
- 从当前版本开始，`review-room` 还支持批量收敛入口：
  - `批量同步全部结论线程`
  - 会对当前频道下所有结论线程逐个执行同一套自动同步规则
  - 返回本次同步线程数、自动创建任务数、自动创建 Artifact 数
- 从当前版本开始，`review-room` 的批量同步结果会写回 Team 历史主链：
  - `scope = room`
  - `action = sync`
  - `message_scope = review-room`
  - `batch_action = thread-sync-all`
  这样页面和 Summary API 都能直接回显最近几次批处理结果，而不需要额外存储格式。
- 从当前版本开始，`review-room` 还补上了更细的线程工作流状态：
  - `待风险跟进`
  - `待评审`
  - `待沉淀`
  - `已沉淀待挂接`
  - `已完成`
  这些状态会同时进入：
  - `decision_threads[].workflow_state / workflow_label`
  - `thread_workbench`
  - `recent_batch_runs`
- 批量同步结果现在还会继续暴露：
  - workflow 计数
  - `history_link`
  - 本轮新建 `Task / Artifact` 的直达链接
  这样 `review-room` 不只知道“本轮跑了多少线程”，还知道“哪些线程还缺风险跟进、评审、沉淀或挂接”。
- 从当前版本开始，`review-room` 还补上了更强的全局收敛摘要：
  - `cross_channel_digests`
    - 同一个结论跨多少频道出现
    - 涉及哪些频道
    - 待跟进风险 / 待处理评审分布
  - `context_digests`
    - 同一个 `ContextID` 下有多少结论线程 / 任务
    - 直接回到任务搜索、历史搜索和相关频道
- 从当前版本开始，`review-room` 页面也会直接展示：
  - `跨频道收敛`
  - `上下文收敛`
  这样不会只停在“当前频道内看消息”，而是能开始看结论在 Team 里的扩散和收束情况。

约束：

- 所有写入继续走 Team Store 标准接口
- 不直接写 JSON/JSONL 文件
- 当前写接口沿用 Team 的本地/LAN 受信写入口约束

## incident-room

第三个内置 Room Plugin 是 `incident-room`。

用途：

- 在某个 Team Channel 里收口：
  - `incident`
  - `update`
  - `recovery`
- 再把故障处理流提炼成：
  - `incident-summary`

接口：

- `GET /api/teams/{teamID}/r/incident-room/?channel_id=main`
- `GET /api/teams/{teamID}/r/incident-room/summary?channel_id=main`
- `POST /api/teams/{teamID}/r/incident-room/messages`
- `POST /api/teams/{teamID}/r/incident-room/distill`

Web 页面：

- `GET /teams/{teamID}/r/incident-room/`

当前页面已具备最小闭环：

- `incident / update / recovery` 三类过滤
- 当前频道的故障处理统计
- `Summary API`
- 直达 `Incident Summary` 和 Team 历史入口
- 三类结构化表单：
  - Incident
  - Update
  - Recovery
- 卡片级“提炼为 Incident Summary”动作
- 结构化字段直出：
  - `severity`
  - `incident_ref`
  - `summary`
  - `resolution / findings / next_steps`
- 单条消息已支持直接同步到 Team Task：
  - `POST /api/teams/{teamID}/r/incident-room/task-sync`
  - 页面动作：`同步到任务`
  - 自动策略：
    - `incident -> blocked`
    - `update -> doing`
    - `recovery -> done`
  - 若当前消息还没有绑定 `task_id`，会自动创建并挂接 Team Task
- 当前频道已支持批量同步全部消息到任务：
  - `POST /api/teams/{teamID}/r/incident-room/task-sync-all`
  - 页面动作：`批量同步全部消息到任务`
  - Summary API 和页面统计已补：
    - `bound_task_count`
    - `unbound_task_count`
    - `suggested_blocked_count`
    - `suggested_doing_count`
    - `suggested_done_count`
  - 对尚未沉淀的 `recovery` 消息，会在批量同步时自动补一份 `incident-summary`
- `incident-room` 的批量同步结果现在也会写回并回显：
  - `scope = room`
  - `action = sync`
  - `message_scope = incident-room`
  - `batch_action = task-sync-all`
  - Summary API 新增：
    - `recent_batch_runs`
  - 页面新增：
    - `最近批处理结果`
    - 本轮同步条数
    - 本轮新建任务数
    - 本轮新增产物数
    - 建议状态分布
    - 直达本轮批处理历史
    - 直达本轮新建任务
    - 直达本轮新建 `Incident Summary`

当前语义边界：

- 不引入新的底层存储格式
- 继续复用 Team 的：
  - `Message`
  - `Artifact`
  - `History`
- 目前先把 `incident-room` 定位成：
  - 故障协作房间
  - 恢复结论沉淀入口
  - 发布前 / 升级后异常处理看板

## handoff-room

第四个内置 Room Plugin 是 `handoff-room`。

用途：

- 在某个 Team Channel 里收口：
  - `handoff`
  - `checkpoint`
  - `accept`
- 再把交接结论提炼成：
  - `handoff-summary`

接口：

- `GET /api/teams/{teamID}/r/handoff-room/?channel_id=main`
- `GET /api/teams/{teamID}/r/handoff-room/summary?channel_id=main`
- `POST /api/teams/{teamID}/r/handoff-room/messages`
- `POST /api/teams/{teamID}/r/handoff-room/distill`
- `POST /api/teams/{teamID}/r/handoff-room/task-sync`
- `POST /api/teams/{teamID}/r/handoff-room/task-sync-all`

Web 页面：

- `GET /teams/{teamID}/r/handoff-room/`

当前页面已具备最小闭环：

- `handoff / checkpoint / accept` 三类过滤
- 当前频道的交接处理统计
- `Summary API`
- 直达 `Handoff Summary` 和 Team 历史入口
- 三类结构化表单：
  - Handoff
  - Checkpoint
  - Accept
- 卡片级“提炼为 Handoff Summary”动作
- 结构化字段直出：
  - `owner`
  - `receiver`
  - `summary`
  - `context / findings / resolution / followups`
- 单条消息已支持直接同步到 Team Task：
  - `handoff -> doing`
  - `checkpoint -> doing`
  - `accept -> done`
  - 若当前消息还没有绑定 `task_id`，会自动创建并挂接 Team Task
- 当前频道已支持批量同步全部消息到任务：
  - `task-sync-all`
  - Summary API 和页面统计已补：
    - `bound_task_count`
    - `unbound_task_count`
    - `suggested_doing_count`
    - `suggested_done_count`
  - 对尚未沉淀的 `accept` 消息，会在批量同步时自动补一份 `handoff-summary`
- `handoff-room` 的批量同步结果也会写回并回显：
  - `scope = room`
  - `action = sync`
  - `message_scope = handoff-room`
  - `batch_action = task-sync-all`
  - Summary API 暴露：
    - `recent_batch_runs`
  - 页面可直接打开：
    - 本轮新建任务
    - 本轮新建 `Handoff Summary`
    - 本轮批处理历史

## artifact-room

第五个内置 Room Plugin 是 `artifact-room`。

用途：

- 在某个 Team Channel 里收口：
  - `proposal`
  - `revision`
  - `publish`
- 再把产物推进过程提炼成：
  - `artifact-brief`

接口：

- `GET /api/teams/{teamID}/r/artifact-room/?channel_id=main`
- `GET /api/teams/{teamID}/r/artifact-room/summary?channel_id=main`
- `POST /api/teams/{teamID}/r/artifact-room/messages`
- `POST /api/teams/{teamID}/r/artifact-room/distill`
- `POST /api/teams/{teamID}/r/artifact-room/task-sync`
- `POST /api/teams/{teamID}/r/artifact-room/task-sync-all`

Web 页面：

- `GET /teams/{teamID}/r/artifact-room/`

当前页面已具备最小闭环：

- `proposal / revision / publish` 三类过滤
- 当前频道的产物推进统计
- `Summary API`
- 直达 `Artifact Brief` 和 Team 历史入口
- 三类结构化表单：
  - Proposal
  - Revision
  - Publish
- 卡片级“提炼为 Artifact Brief”动作
- 卡片级“同步到任务”动作
- 整频道“批量同步全部消息到任务”
- 结构化字段直出：
  - `artifact_kind`
  - `owner`
  - `summary`
  - `outline / changes / result / followups`
- 继续复用 Team 的：
  - `Message`
  - `Artifact`
  - `History`
  - 可选 `task_id` 绑定

当前自动联动规则：

- `proposal -> doing`
- `revision -> doing`
- `publish -> done`
- 缺少 `task_id` 时，自动创建并挂接 Team Task
- `task-sync-all` 会对尚未沉淀的 `publish` 自动补 `artifact-brief`

当前批处理结果也会写回并回显：

- Team history 中记录：
  - `message_scope = artifact-room`
  - `batch_action = task-sync-all`
- Summary API 暴露：
  - `recent_batch_runs`
- 页面可直接打开：
  - 本轮新建任务
  - 本轮新建 Artifact Brief
  - 本轮批处理历史

从当前版本开始，`artifact-room` 的任务绑定也会通过 Team history 回填：

- 即使原始消息没有显式写入 `task_id`
- 只要执行过：
  - `task-sync`
  - `task-sync-all`
- 页面和 Summary API 里的：
  - `bound_task_count / unbound_task_count`
  - `打开绑定任务`
  也会按最近的同步结果稳定回显

## decision-room

第六个内置 Room Plugin 是 `decision-room`。

用途：

- 在某个 Team Channel 里收口：
  - `proposal`
  - `option`
  - `decision`
- 再把最终结论提炼成：
  - `decision-note`

接口：

- `GET /api/teams/{teamID}/r/decision-room/?channel_id=main`
- `GET /api/teams/{teamID}/r/decision-room/summary?channel_id=main`
- `POST /api/teams/{teamID}/r/decision-room/messages`
- `POST /api/teams/{teamID}/r/decision-room/distill`
- `POST /api/teams/{teamID}/r/decision-room/task-sync`
- `POST /api/teams/{teamID}/r/decision-room/task-sync-all`

Web 页面：

- `GET /teams/{teamID}/r/decision-room/`

当前页面已具备最小闭环：

- `proposal / option / decision` 三类过滤
- 当前频道的决策推进统计
- `Summary API`
- 直达 `Decision Note` 和 Team 历史入口
- 三类结构化表单：
  - Proposal
  - Option
  - Decision
- 卡片级：
  - `同步到任务`
  - `提炼为 Decision Note`
- 整频道：
  - `批量同步全部消息到任务`

当前自动联动规则：

- `proposal -> doing`
- `option -> doing`
- `decision -> done`
- 缺少 `task_id` 时，自动创建并挂接 Team Task
- `task-sync-all` 会对尚未沉淀的 `decision` 自动补 `decision-note`

当前批处理结果也会写回并回显：

- Team history 中记录：
  - `message_scope = decision-room`
  - `batch_action = task-sync-all`
- Summary API 暴露：
  - `recent_batch_runs`
- 页面可直接打开：
  - 本轮新建任务
  - 本轮新建 Decision Note
  - 本轮批处理历史

从当前版本开始，`decision-room` 的任务绑定也会通过 Team history 回填：

- 即使原始消息没有显式写入 `task_id`
- 只要执行过：
  - `task-sync`
  - `task-sync-all`
- 页面和 Summary API 里的：
  - `bound_task_count / unbound_task_count`
  - `打开绑定任务`
  也会按最近的同步结果稳定回显

同样的回填规则也已经补到：

- `incident-room`
- `handoff-room`

这样这几类房间插件在“自动建任务后刷新页面看不到绑定”的问题上，已经进入统一行为。

## Room Theme

Room Theme 是 Channel 级模板覆盖。

当前已内置最小 Theme Registry，按 `ChannelConfig.Theme` 查找对应主题。

当前已内置：

- `minimal`
- `focus`
- `board`

当前 Team API / 频道 API 已会返回可选 Theme 摘要，频道页也会显示 Theme 可选项和当前 Theme 信息。

从 `v0.5.83+` 开始，Theme 选择器已经具备最小实际选择价值：

- `minimal`
  - 保持最小信息密度
- `focus`
  - 把 Onboarding / Rules / 最近消息收成更像工作台的布局
- `board`
  - 把最近消息、配置提示和快速入口收成更像看板的频道视图

行为：

- `ChannelConfig.Theme == "minimal"` 时，频道页面切到极简模板
- 未知 Theme 或模板加载失败时：
  - 先退回 `room_channel_default.html`
  - 再退回现有 `team_channel.html`

当前 minimal 主题会显示：

- Agent Onboarding
- Rules
- Plugin / Theme
- 当前频道消息
- 结构化消息的 pretty JSON
- `[PLAN] / [SKILL] / [SNIPPET]` 前缀
- `channel_item.html` 粒度的消息项模板

## 扩展方式

新增一个 Room Plugin 时，按下面顺序做：

1. 新建：
   - `internal/plugins/haonewsteam/rooms/<plugin>/plugin.go`
   - `internal/plugins/haonewsteam/rooms/<plugin>/handler.go`
   - `internal/plugins/haonewsteam/rooms/<plugin>/types.go`
   - `internal/plugins/haonewsteam/rooms/<plugin>/roomplugin.json`
2. 实现：
   - `ID()`
   - `Manifest()`
   - `Handler(...)`
3. 在 `Plugin.Build()` 里注册到 Room Registry
4. 通过 `ChannelConfig.Plugin` 把它绑定到频道

新增一个 Room Theme 时，按下面顺序做：

1. 新建：
   - `internal/themes/room-themes/<theme>/roomtheme.json`
   - `internal/themes/room-themes/<theme>/web/templates/room_channel.html`
   - 如需 item 级结构，再补 `channel_item.html`
2. 在 Channel 渲染链中接入该 Theme ID
3. 保持默认 fallback 不变

## 当前边界

当前这条架构主线已经完成：

- Room Plugin Registry
- ChannelConfig 独立存储与 canonical path
- Team 路由挂载点
- `plan-exchange`
- `minimal` Room Theme
- `focus` Room Theme
- `board` Room Theme
- `review-room`
- 频道工作台内 Room Plugin / Theme 配置器
- `review-room` Summary API
- `review-room` 状态分组工作台和 Artifact 直达
- `review-room` 的决策沉淀 / 最近产物聚合视图
- `review-room` 的结论级产物聚合（按结论直达最新 Review Summary）
- `review-room` 的 risk/review -> decision 关联聚合
- `review-room` 的结论线程已接回 Team 主链（任务 / 产物 / 历史）
- `review-room` 的 decision thread 已支持真实 task / artifact 绑定
- `review-room` 的 decision thread 已支持：
  - 任务状态流转
  - 线程级 Artifact 生成动作
- `review-room` 的 decision thread 已支持确定性的自动联动：
  - 自动建议任务状态
  - 自动同步线程
  - 自动补线程级 `Review Summary`
  - 自动创建并挂接 Team Task（在线程尚无绑定任务时）
- `review-room` 已支持线程级批量收敛：
  - 一次性同步当前频道全部结论线程
  - 汇总返回批量同步结果
- `review-room` 已支持跨线程工作台摘要：
  - 线程总数
  - 已绑定任务 / 待自动建任务
  - 已沉淀产物 / 待补沉淀产物
  - 建议 `blocked / doing / done` 分布
  - 页面可直接从摘要跳到对应结论线程
- `review-room` 已支持最近批处理结果回显：
  - `recent_batch_runs`
  - 最近批量同步时间 / actor / 已同步线程
  - 新建任务数 / 新建产物数
  - 建议 `blocked / doing / done` 分布
  - 以及本轮新建出来的 Task / Artifact 直达链接
- `review-room` 已支持跨频道 / 跨上下文收敛：
  - `cross_channel_digests`
  - `context_digests`
  - 页面可直接看到“跨频道收敛 / 上下文收敛”面板

还没做的下一层：

- 更多内置 Room Plugins
- `review-room` 更强的任务/产物自动联动策略
  - 例如更细的状态策略、批处理结果和更多 Team 主链对象的联动
