Goal
- 把 `Team` 从“工程上能用的工作区”继续推进到“普通团队成员一眼能懂、能直接用”的协作产品层，同时保持当前已经跑通的能力主链不回退：
  - `members / channels / messages / tasks / artifacts / history / archive`
  - `sync / conflicts / webhook / A2A / SSE / Team P2P`
- 本轮目标不是重做底层复制和存储，而是在现有稳定基线之上完成：
  - 团队入口与概览产品化
  - sync/conflict 页面产品化
  - webhook / A2A 的人类可用入口
  - policy / 签名限制提示与引导
  - 兼容层清理收尾
  - SSE 实时可见性

Context
- 源文档：
  - `/Users/haoniu/sh18/hao.news2/haonews/doc-md/20260404-haonews-team2000.md`
- 参考模板：
  - `/Users/haoniu/sh18/hao.news2/runbook-template.auto-run.md`
- 当前已知基线：
  - Team 核心能力已经真实存在，不再是空壳
  - Team sync health / conflicts / webhook status / replay / A2A / SSE 已有后端基础
  - Team Store `ctx` 主入口、handler/store 拆分、task/artifact index-first、legacy 清理第一阶段已完成
  - `.75 / .74` 已作为当前双节点运行基线
  - 现有正式运维文档已经落库：
    - `/Users/haoniu/sh18/hao.news2/haonews/doc-md/node-upgrade-75-74.md`
    - `/Users/haoniu/sh18/hao.news2/haonews/doc-md/runtime-75-74-baseline.md`
    - `/Users/haoniu/sh18/hao.news2/haonews/doc-md/runtime-75-74-validation.md`
- 本轮执行约束：
  - 优先做低风险、可回滚、与现有实现一致的产品化增强
  - 不为了“好看”而重写稳定主链
  - 不把普通实现分歧升级成用户确认项
  - 如果某项需要真实节点验证，优先先做本地/单节点闭环，再做 `.75 / .74` 运行态复核

Execution Contract
- 读取整份 runbook 后直接开始执行，不停在分析或普通进度汇报。
- 小分歧、小缺口、小命名问题、小文件落点问题，一律自行做低风险判断并继续。
- 若遇硬阻塞：
  - 先尝试 fallback
  - 再把真实状态写回本文件
  - 最后才停止
- 收尾必须使用：
  - `Completed`
  - `Blocked`
  - `Next Step`

Planning Rules
- 先做“用户一眼能感知价值”的页面/交互，再补支撑它们的轻量后端与 view-model。
- 先做不会改变接口语义的增强，再做可能触及接口返回结构的优化。
- 先用已有后端能力暴露产品化入口，不新造重复 API。
- 验证必须分层：
  - 最相关单测 / 模板回归
  - `go build ./cmd/haonews`
  - 必要时做 `.75 / .74` 运行态复核

Execution Plan

Phase 1 Inspect And Classify
- 目标：
  - 把源文档中的候选项先分成 `done / todo / defer / stale`，避免重复做已存在能力。
- 必读文件：
  - `/Users/haoniu/sh18/hao.news2/haonews/doc-md/20260404-haonews-team2000.md`
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/handler.go`
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/handler_sync.go`
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/handler_webhook.go`
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/a2a_bridge.go`
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/themes/haonews/web/templates/team.html`
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/themes/haonews/web/templates/team_detail.html`
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/themes/haonews/web/templates/team_sync.html`
- 输出：
  - 在本文件 `Status Writeback` 中先记录首轮分类
- 完成标准：
  - 明确哪些项已被现有代码覆盖，哪些仍是本轮关键路径

Phase 2 Team Entry Productization
- 目标：
  - 让 `/teams` 和 `/teams/{teamID}` 一进来就能看懂“这个 Team 现在在干什么”和“下一步最常见动作是什么”。
- 关键任务：
  - `P1` Team index 活跃摘要面板
    - 引入 `teamActivityDigest` 或等效 view-model
    - 在 `handleTeamIndex` 里构建：
      - `UnreadMessages` 或最近消息计数
      - `OpenTasks`
      - `UnresolvedConflicts`
      - `WebhookDeadLetters`
      - `LastActivityAt`
      - `TopAction`
      - `TopActionURL`
    - 页面改造：
      - `/teams` 首页卡片化摘要
  - `P2` Team detail 快捷操作和冗余导航收口
    - 去掉重复导航入口
    - 增加高频动作：
      - 发消息
      - 建任务
      - 看同步状态
      - 看归档
    - 若 `RequireSignature=true`，快捷发消息入口改成政策提示，不给出误导性表单
  - `P3` 轻量计数 helper
    - `CountRecentMessagesCtx`
    - `CountTasksByStatusCtx`
    - 尽量基于 shard/index/尾部读取，不引入全量 `Load*Ctx(..., 0)`
- 验证：
  - Team index / detail 页面访问回归
  - 新增 targeted tests 覆盖 digest / quick action / policy gating
- 完成标准：
  - `/teams` 和 `/teams/{teamID}` 已能体现“活跃摘要 + 常用动作”

Phase 3 Sync And Conflict Productization
- 目标：
  - 把 sync/conflict 从工程面板提升为普通成员也能理解的页面。
- 关键任务：
  - `P4` 冲突卡片化
    - 丰富 `buildTeamSyncConflictViews`
    - 明确：
      - `ConflictClass`
      - 中文 `ReasonLabel`
      - `ActionHint`
      - `Actions`
    - `team_sync.html` 改为卡片 UI，不只显示原始 metrics
  - `P5` Sync 健康状态分层
    - 先显示：
      - `healthy`
      - `attention`
      - `stale`
      - `disabled`
    - 再把原始 `StatusGroups` 放到详情折叠区
  - `P6` 冲突解决反馈闭环
    - `dismiss / accept_remote / keep_local` 后页面有明确反馈
- 验证：
  - sync 页面单测 / 模板渲染回归
  - 真实制造至少一类 conflict，确认可展示和可 resolve
- 完成标准：
  - 普通用户打开 `/teams/{teamID}/sync` 时能看懂状态和冲突动作

Phase 4 Webhook And A2A Productization
- 目标：
  - 把已有的 webhook / A2A 能力从“只有接口”提升为“有人类可读入口”。
- 关键任务：
  - `P7` Webhook 页面
    - 路由：
      - `/teams/{teamID}/webhooks`
    - 展示：
      - 当前 webhook 配置
      - delivery 状态摘要
      - 最近 delivery
      - 失败原因
      - replay 按钮
  - `P8` Replay 表单支持
    - 保持 JSON API 兼容
    - 从 HTML 页面点击 replay 后能返回页面态反馈
  - `P9` A2A 页面
    - 路由：
      - `/teams/{teamID}/a2a`
    - 展示：
      - agent cards
      - A2A 端点一览
      - 最近任务/动作
- 验证：
  - webhook 页面/状态/replay 回归
  - A2A 页面能正确渲染已有 agent/task 数据
- 完成标准：
  - webhook / A2A 不再只有 JSON 接口可用

Phase 5 Policy Guidance And Error UX
- 目标：
  - 把 “Policy 存在但用户不理解” 变成 “用户一眼知道这个 Team 有什么规则、出错时该怎么办”。
- 关键任务：
  - `P10` Team detail Policy 摘要面板
    - 重点提示：
      - `RequireSignature`
      - `MessageRoles`
      - `TaskRoles`
      - 自定义 permissions / transitions
  - `P11` 结构化 Team API 错误
    - 引入稳定错误响应结构，例如：
      - `message_signature_required`
      - `permission_denied`
      - `invalid_task_transition`
      - `team_not_found`
    - 保持现有 API 兼容优先；若已经有调用方依赖纯文本错误，先走“增强 JSON path / 保守 HTML path”
  - `P12` 快捷表单前置提示
    - RequireSignature Team 不显示误导性网页消息表单
- 验证：
  - RequireSignature Team 和普通 Team 各跑一遍
  - API 错误结构 targeted tests
- 完成标准：
  - 策略限制不再表现为“看起来像坏了”

Phase 6 Compat And Migration Cleanup Finish
- 目标：
  - 把兼容层继续收口，但不做高风险、无收益的彻底删除。
- 关键任务：
  - `P13` compat API 标记废弃
    - 在 compat 层加 `Deprecated` 注释和迁移方向
  - `P14` migration helper 隔离
    - 迁移输入 helper 移到独立 `migration.go` 或等效文件
    - 标清删除条件
- 验证：
  - Team 包全量测试
  - 插件包回归
- 完成标准：
  - compat / migration 的职责边界明确，不再继续污染主路径

Phase 7 SSE Visibility And Global Nav Alignment
- 目标：
  - 让用户能直观看到 Team 是实时的，并把 Team 导航进一步融进全局使用路径。
- 关键任务：
  - `P15` Team detail SSE 指示器 + toast
    - 实时连接状态
    - 新事件 toast
  - `P16` PageNav / 全局导航整合
    - Team 入口与：
      - detail
      - sync
      - webhooks
      - a2a
      - archive
      之间的导航关系收口
- 验证：
  - 页面打开后 EventSource 正常连通
  - 另一终端发消息后页面收到 toast
- 完成标准：
  - 用户能明显感知 Team 是实时协作工具，而不是静态页面

Phase 8 Final Verification And Release
- 目标：
  - 形成可发布批次，并在需要时完成 `.75 / .74` 的运行态复核。
- 验证：
  - 最相关单测：
    - `go test ./internal/haonews/team -count=1`
    - `go test ./internal/plugins/haonewsteam -count=1`
    - 如修改到 Team sync / webhook / A2A / SSE，再加 targeted tests
  - 构建：
    - `go build ./cmd/haonews`
  - 如页面/运行态变化明显，再做节点复核：
    - Team detail / sync / webhooks / a2a / archive
    - `public-live-time` 只在本轮改动触及 Team/Live 交叉区域时再复核
- 完成标准：
  - runbook 中列出的 `todo` 项全部完成，或已清楚标成 `defer` 并有原因
  - 若形成可发布批次，补 `main + tag + release`

Verification
- 最小必跑：
  - `go test ./internal/haonews/team -count=1`
  - `go test ./internal/plugins/haonewsteam -count=1`
  - `go build ./cmd/haonews`
- 按改动补跑：
  - webhook 相关：
    - `go test ./internal/haonews/team -run 'TestStoreWebhook|Test.*Delivery|Test.*Replay' -count=1`
  - sync/conflict 相关：
    - `go test ./internal/plugins/haonewsteam -run 'TestPluginBuildServesAndResolvesTeamSyncConflicts|TestPluginBuildServesTeamSyncHealthPageAndAPI' -count=1`
  - A2A 相关：
    - `go test ./internal/plugins/haonewsteam -run 'TestPluginBuildServesA2ABridge' -count=1`
  - SSE 相关：
    - `go test ./internal/plugins/haonewsteam -run 'TestPluginBuildStreamsTeamEvents' -count=1`
- 若做节点复核：
  - `.75` / `.74` 统一看：
    - `/api/teams`
    - `/teams/{teamID}`
    - `/teams/{teamID}/sync`
    - `/teams/{teamID}/webhooks`
    - `/teams/{teamID}/a2a`
    - `/archive/team/{teamID}`

Fallback Rules
- 如果某个 UI 产品化项需要大范围改模板和 CSS，优先先做：
  - view-model
  - 局部模板增强
  - 最小 CSS 补丁
  不做全站风格重构。
- 如果结构化错误响应会明显破坏现有 API 调用方：
  - 保留现有 HTTP status 和核心错误语义
  - 只对 JSON path 增补结构化字段
  - HTML path 保持兼容
- 如果 `.75 / .74` 节点复核被节点环境拖住：
  - 先完成本地和单节点验证
  - 在 `Status Writeback` 里明确缺的只是运行态复核

Blockers / Resume
- 真实硬阻塞定义：
  - 同一目标存在多种方案且会导致明显不同的产品行为/接口语义，且没有安全默认值
  - 现有节点环境无法完成最小运行态验证，且本地 fallback 也失败
  - 变更会触及不可逆迁移，且仓库内没有安全回滚路径
- 若阻塞：
  - 先把本文件 `Status Writeback` 更新到最新
  - 写清：
    - 已完成项
    - 失败项
    - 已尝试 fallback
    - 精确恢复下一步

Status Writeback
- Initial classification:
  - `done`:
    - Team 核心能力主链
    - sync/conflict 基础页
    - webhook status / replay API
    - A2A bridge JSON 能力
    - SSE 基础链路
    - Store ctx 主入口 / compat 第一阶段收口 / legacy 第一阶段收口
  - `todo`:
    - Team index 活跃摘要面板
    - Team detail 快捷动作与 policy 前置提示
    - conflict 卡片化与健康分层
    - webhook 页面
    - A2A 页面
    - 结构化 Team API 错误
    - compat 废弃标记与 migration 隔离
    - SSE 可见性和导航整合
  - `defer`:
    - 重做 Team 底层复制模型
    - CRDT / 更强一致性新体系
    - 大范围视觉重构
  - `stale`:
    - 把 Team 定义为“很多能力还是空壳”的判断已过时
- Final execution result:
  - `done`:
    - Team index 活跃摘要面板
    - Team detail 快捷动作与 policy 前置提示
    - conflict 卡片化与健康分层
    - webhook 页面
    - A2A 页面
    - 结构化 Team API 错误
    - compat 废弃标记与 migration 隔离
    - SSE 可见性和导航整合
    - `CountRecentMessagesCtx / CountTasksByStatusCtx / LoadRecentDeliveriesCtx`
    - `teamActivityDigest` 与 Team 首页产品化摘要
    - `team_sync.html` view-model 化
    - `TeamSyncMessage.Key()` 热路径标准化与 benchmark
  - `defer`:
    - `.75 / .74` 节点发布复核
      - 本轮改动是 Team 产品化页面、view-model、错误结构和 compat/migration 收口
      - 已完成本地运行态验证，未触及 Team/Live 跨节点协议语义
      - 因此节点发布不作为本 runbook 完成前置条件
    - `main + tag + release`
      - 形成可发布批次，但发布动作单独进行，避免和工作区其他无关变更混在同一轮执行判断里
  - Verification summary:
    - `go test ./internal/haonews/team ./internal/plugins/haonewsteam -count=1`
    - `go build ./cmd/haonews`
    - 本地运行态复核通过：
      - `/teams`
      - `/teams/archive-demo`
      - `/teams/archive-demo/sync`
      - `/teams/archive-demo/webhooks`
      - `/teams/archive-demo/a2a`
      - `/api/teams/archive-demo/sync`
      - `/api/teams/archive-demo/webhooks/status`
      - `/.well-known/agent.json`
      - `/a2a/teams/archive-demo/tasks`
  - Resume point:
    - 若继续推进，下一步不再补这份 runbook，而是单开发布/部署批次或新的 Team 下一阶段增强 runbook
