Goal
- 把 `Team` 的下一阶段产品化收成可直接执行的一轮增强：补强总览页、任务系统和冲突治理，让普通团队成员更容易理解当前状态、推进工作和处理异常，而不回退现有 `sync / webhook / A2A / SSE / Team P2P` 主链。

Context
- 仓库 / 工作目录：
  - `/Users/haoniu/sh18/hao.news2/haonews`
- 参考模板：
  - `/Users/haoniu/sh18/hao.news2/runbook-template.auto-run.md`
- 上一轮已完成并发布：
  - `v0.5.77`
  - Team 首页活跃摘要
  - Team 详情页快捷操作与 policy 提示
  - Team sync/conflict 页面卡片化与健康分层
  - Team webhook 页面与 A2A 页面
  - 结构化 Team API 错误
- 当前稳定基线：
  - `.75 / .74` 是双节点运行基线
  - Team 核心对象链已真实可用：
    - `members / channels / messages / tasks / artifacts / history / archive`
  - Team 集成链已真实可用：
    - `sync / conflicts / webhook / A2A / SSE / Team P2P`
- 本轮边界：
  - 做产品层增强，不重做底层复制模型
  - 优先总览页、任务系统、冲突治理
  - 只在需要时补最小后端/视图模型，不新造重复 API

Execution Contract
- 读取完整文档后立即开始执行。
- 在本 runbook 完成或确认硬阻塞之前，不要向用户提问。
- 普通实现分歧、命名分歧、文件落点分歧、先后顺序分歧，直接按低风险方案自行裁决。
- 优先选择：
  - 风险更低
  - 更可回滚
  - 与当前代码和文档更一致
  - 改动更小但足够完成目标
  - 更容易验证
- 如果出现阻塞：
  - 先尝试本地 fallback
  - 再尝试不改变接口语义的替代路径
  - 仍失败时，把状态写回本文档再停止
- 收尾必须使用：
  - `Completed`
  - `Blocked`
  - `Next Step`

Planning Rules
- 先做用户最容易感知价值的页面和流程，再补所需 view-model / helper。
- 先增强现有 Team 页面，不另起一套“新 Team UI”。
- 先做不会改变接口语义的增强，再做有限的接口扩展。
- Task 增强优先围绕“负责人 / 截止时间 / 优先级 / 看板感”，不先做大而全的项目管理系统。
- 冲突治理优先做“看懂 + 处理 + 反馈”，不先做更强复制一致性。

Execution Plan

Phase 1. Inspect And Reclassify
- [ ] 读取最小必要上下文：
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/handler.go`
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/handler_task.go`
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/handler_sync.go`
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/types.go`
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/themes/haonews/web/templates/team.html`
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/themes/haonews/web/templates/team_detail.html`
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/themes/haonews/web/templates/team_sync.html`
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/themes/haonews/web/templates/team_tasks.html`
- [ ] 分类当前基线：
  - 总览页哪些能力已有，哪些只是摘要还不够“可行动”
  - Task 当前真实字段和 workflow 到哪一步
  - conflict 当前页面哪些信息仍偏工程视角
- [ ] 在 `Status Writeback` 里记录首轮分类：
  - `done / todo / defer / stale`

Phase 2. Team Dashboard Productization
- [ ] 目标：
  - 把 `/teams/{teamID}` 从“概览页”推进到“团队每日入口”。
- [ ] 做法：
  - 增加 dashboard block：
    - 今日重点
    - 我的任务
    - 最近变更
    - 最近消息
    - 最近 dead-letter / unresolved conflict
  - 补最小 view-model，避免模板里直接堆逻辑
  - 如果已有数据不足：
    - 优先做只读 helper
    - 不引入全量扫描
- [ ] 完成标准：
  - 团队成员打开 Team 详情页时，一眼知道：
    - 现在最重要的任务
    - 最近有什么变化
    - 当前有没有需要处理的异常

Phase 3. Task System Productization
- [ ] 目标：
  - 把 Task 从“条目记录”推进到“真正的协作骨架”。
- [ ] 关键增强：
  - `T1` 负责人字段显性化
    - 页面展示 assignee / assignees
    - 创建和更新入口最小可用
  - `T2` 截止时间字段
    - 页面展示 due date
    - 逾期 / 即将到期有轻量提示
  - `T3` 优先级可视化
    - `low / normal / high / urgent` 或等效枚举统一展示
  - `T4` 任务列表分组
    - 先按状态分组
    - 再在状态内按优先级或截止时间排序
  - `T5` 任务上下文入口
    - 明确“相关消息 / 相关产物 / 相关历史”的跳转
- [ ] 约束：
  - 不先做复杂子任务系统
  - 不先做全新看板页面，优先把现有任务页做出“看板感”
- [ ] 完成标准：
  - Task 页面已经具备日常协作推进的核心要素

Phase 4. Conflict Governance Productization
- [ ] 目标：
  - 把 `/teams/{teamID}/sync` 从“能看懂一些”推进到“能真的快速处理冲突”。
- [ ] 关键增强：
  - `C1` 冲突严重程度层级
    - `info / attention / risky / blocked` 或等效层级
  - `C2` 冲突前后差异摘要
    - 本地 vs 远端
    - 至少对 `task / artifact / policy / member / channel` 给出人类可读摘要
  - `C3` 处理动作后果说明
    - `accept_remote`
    - `keep_local`
    - `dismiss`
  - `C4` 最近处理历史
    - 最近一次 resolve 的对象、动作、处理人、时间
  - `C5` 过滤与分组
    - 按对象类型 / 未处理 / 已处理 / 高风险分组
- [ ] 完成标准：
  - 普通成员打开 sync 页面时，能更快判断“先处理哪个、为什么、处理后会怎样”

Phase 5. Minimal Backend And View-Model Support
- [ ] 目标：
  - 只补本轮产品化真正需要的最小后端支持。
- [ ] 允许做的后端增强：
  - 轻量计数 helper
  - 最近对象聚合 helper
  - conflict diff summary helper
  - task list sorting/grouping helper
  - 页面专用 view-model
- [ ] 不做：
  - 重做 Team 存储结构
  - 重做 Team sync 协议
  - 新建大而全的二级 API 体系

Phase 6. Verify
- [ ] 必跑验证：
  - `go test ./internal/haonews/team -count=1`
  - `go test ./internal/plugins/haonewsteam -count=1`
  - `go build ./cmd/haonews`
- [ ] 按改动补跑：
  - task 相关：
    - `go test ./internal/plugins/haonewsteam -run 'TestPluginBuild.*Task' -count=1`
  - sync/conflict 相关：
    - `go test ./internal/plugins/haonewsteam -run 'TestPluginBuildServesAndResolvesTeamSyncConflicts|TestPluginBuildServesTeamSyncHealthPageAndAPI' -count=1`
- [ ] 本地页面复核：
  - `/teams`
  - `/teams/{teamID}`
  - `/teams/{teamID}/tasks`
  - `/teams/{teamID}/sync`
- [ ] 如形成明显可见面变化，再做 `.75 / .74` 运行态复核：
  - `/teams/{teamID}`
  - `/teams/{teamID}/tasks`
  - `/teams/{teamID}/sync`

Phase 7. Close Out
- [ ] 更新本文档 `Status Writeback`
- [ ] 写明：
  - 完成了什么
  - defer 了什么
  - 是否形成可发布批次
- [ ] 若形成完整批次，可单开发布动作：
  - `main + tag + release`

Verification
- 首选验证命令：
  - `go test ./internal/haonews/team -count=1`
  - `go test ./internal/plugins/haonewsteam -count=1`
  - `go build ./cmd/haonews`
- 备用验证方式：
  - 本地打开：
    - `http://127.0.0.1:51818/teams`
    - `http://127.0.0.1:51818/teams/archive-demo`
    - `http://127.0.0.1:51818/teams/archive-demo/tasks`
    - `http://127.0.0.1:51818/teams/archive-demo/sync`
- 完成标准：
  - [ ] 总览页的下一阶段入口和摘要增强已落地
  - [ ] Task 页面具备更强协作骨架能力
  - [ ] sync/conflict 页面具备更清晰治理体验
  - [ ] 最相关测试和构建已通过
  - [ ] `Status Writeback` 已按真实执行情况写回

Fallback Rules
- 如果“总览页增强”需要太多新数据聚合：
  - 先做已有数据的更好组织
  - 不先扩成新统计系统
- 如果“任务系统增强”会明显改变存储语义：
  - 先只做展示和轻量字段
  - 不先做迁移
- 如果“冲突 diff”某类对象难以做精细对比：
  - 先做人类可读摘要
  - 不先上复杂结构化 merge UI
- 如果 `.75 / .74` 节点复核受环境影响：
  - 先完成本地验证
  - 节点复核写成后续发布批次的一部分

Blockers / Resume
- 硬阻塞定义：
  - 不同方案会导致明显不同的产品行为或数据语义，且没有安全默认值
  - 需要不可逆迁移，但仓库中没有安全回滚路径
  - 本地和节点 fallback 都无法完成最小验证
- 若阻塞，必须写回：
  - `Blocked on`
  - `Tried`
  - `Why blocked`
  - `Exact next step`
- 恢复执行时：
  - 先读完整个文档
  - 从最后一个未完成且仍有效的步骤继续

Status Writeback
- `Completed`:
  - `Phase 1` Inspect And Reclassify 已完成：
    - 总览页已有摘要，但缺“今日重点 / 最近变更 / 最近消息”这类每日入口
    - Task 已有 `assignees / priority / context / closed_at`，但缺 `due_at` 和更强列表分组
    - conflict 已有健康分层和动作，但缺更直接的严重程度、后果说明、已处理视图
  - `Phase 2` Team Dashboard Productization 已完成：
    - Team 详情页新增：
      - 今日重点
      - 关键任务
      - 最近变更
      - 最近消息
      - dashboard alerts
  - `Phase 3` Task System Productization 已完成：
    - `Task` 新增 `due_at`
    - create / update 表单支持 `due_at`
    - 任务列表按 lane 分组：
      - 推进中
      - 待确认
      - 待开始
      - 已完成
    - 任务页补：
      - 默认执行代理
      - 即将到期 / 已逾期
      - 任务详情页截止时间展示与编辑
  - `Phase 4` Conflict Governance Productization 已完成：
    - sync 页补：
      - 严重程度
      - 后果说明
      - 本地/远端版本标签
      - 未处理 / 已处理视图
      - 最近已处理冲突
  - `Phase 5` Minimal Backend And View-Model Support 已完成：
    - `Task.due_at`
    - `TaskIndexEntry.due_at`
    - `normalizeReplicatedTask` 保留 `due_at`
    - Dashboard / task lanes / conflict view 最小 view-model
  - `Phase 6` Verify 已完成：
    - `go test ./internal/plugins/haonewsteam -run 'TestPluginBuildServesTeamDetailAndAPI|TestPluginBuildHandlesTeamTaskFormWrites|TestPluginBuildServesAndResolvesTeamSyncConflicts|TestBuildTeamSyncConflictViewsExplainsSuggestions' -count=1`
    - `go test ./internal/haonews/team ./internal/plugins/haonewsteam -count=1`
    - `go build ./cmd/haonews`
    - 本地页面复核：
      - `/teams/archive-demo`
      - `/teams/archive-demo/tasks`
      - `/teams/archive-demo/sync`
- `Blocked`:
  - 无。
- `Next Step`:
  - 如果继续，最自然的是单开发布批次，把这轮 Team 下一阶段产品化改动做成 `main + tag + release`。
