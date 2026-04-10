# `20260404-Team-claude-codex-task.auto-run.md`

## Goal

- 把 [20260404-Team-claude.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/20260404-Team-claude.md) 从“审阅建议”收成一份可直接执行的 Team 代码治理与性能优化 runbook。
- 本 runbook 不重做已经完成的 Team 产品能力；它只处理仍然值得做的：
  - `store.go / handler.go` 结构治理
  - 明确的热点性能问题
  - 文件锁 / 写入可靠性
  - 接口语义收口
  - 测试与验证闭环

本阶段目标不是重写 Team，而是把当前 Team 代码库从“功能已重”推进到“更稳、更易维护、更容易继续演进”。

## Context

- 仓库：
  - `/Users/haoniu/sh18/hao.news2/haonews`
- 源文档：
  - [20260404-Team-claude.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/20260404-Team-claude.md)
- 当前 Team 基线：
  - Team workspace / archive / sync / conflict / SSE / A2A / webhook / P2P 已经存在
  - `store_test.go` 已经存在，所以源文档里“缺少 store_test.go”的判断已过时
  - `internal/haonews/team/store.go` 仍然是大文件
  - `internal/plugins/haonewsteam/handler.go` 仍然是大文件
  - `internal/haonews/team/task_state.go` / `sync.go` / `message_signature.go` 已经拆出，说明后续拆分应按增量方式继续，不要把已独立的部分再搬回来
- 本 runbook 的执行原则：
  - 先核实现状，再决定 `done / todo / defer`
  - 先做低风险高收益项
  - 仅在前置收益已经吃满、且 bench/profile 证据明确时才继续更深的结构重构
  - 每一批都要有可复跑的测试或直接运行验证

## Execution Rules

- 始终自主执行完整任务。
- 除非遇到不可恢复错误，否则不要询问确认。
- workspace 内文件编辑和命令执行默认自动允许。
- 只在：
  - 跨项目修改
  - 删除文件
  - 新的高风险网络操作
  这三类情况下才停。

## Success Criteria

- `20260404-Team-claude.md` 中仍然成立且值得做的关键项，被明确收口到：
  - `done`
  - `defer`
  - `not-needed`
- `store.go` 和 `handler.go` 不再继续膨胀，至少完成一轮按职责拆分
- 至少打掉源文档中最明确的 3 个热点：
  - `readLastJSONLLines`
  - `channelSummary`
  - `LoadTaskMessages / LoadMessagesByContext`
- 文件写入、文件锁、webhook、limit 语义里至少完成一轮最小可靠性收口
- 所有改动都有：
  - targeted test
  - `go build ./cmd/haonews`
  - 必要时的 benchmark/smoke 证明

## Critical Path

1. 先分类源文档建议，避免重复做已经完成的 Team 能力
2. 先处理明确、可验证的性能热点
3. 再做低风险结构拆分
4. 再收写入可靠性和接口语义
5. 最后补测试、bench、文档写回

不要反过来先做大拆分，再去找性能收益。

## Execution Plan

### Phase A — 事实核对与建议分类

目标：
- 把源文档中的每条建议先落成 `done / todo / defer / stale`，不在错误前提上执行。

步骤：
- [ ] 逐项核对 [20260404-Team-claude.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/20260404-Team-claude.md) 的建议与当前代码现状
- [ ] 在本 runbook 内写回分类结果，至少覆盖这些块：
  - `1.1 store.go 拆分`
  - `1.2 handler.go 拆分`
  - `2.1 readLastJSONLLines`
  - `2.2 channelSummary`
  - `2.3 LoadTaskMessages / LoadMessagesByContext`
  - `2.4 ListTeams`
  - `2.5 handleTeam 并发加载`
  - `3.1 JSONL 写入可靠性`
  - `3.2 withTeamLock timeout / context`
  - `3.3 webhook retry`
  - `4.1 重复 validate 模板`
  - `4.2 legacy/index 双轨`
  - `4.3 锁内 public Load*`
  - `4.4 mergeChannel CreatedAt`
  - `4.5 ClosedAt 重复逻辑`
  - `5.1 trusted request`
  - `5.2 路径遍历防护`
  - `6.1 LoadMessages limit 语义`
  - `6.2 context.Context`
  - `7.1 store tests`
- [ ] 明确哪些项已由近几轮 Team/P2P 工作覆盖

完成标准：
- 本 runbook 本身成为真实状态看板，不再沿用源文档里已经过时的判断。

### Phase B — 先打掉明确性能热点

目标：
- 先把最容易形成真实收益、且最容易被验证的 Team 热点收掉。

#### B1. `readLastJSONLLines` 块读取重写

步骤：
- [ ] 只读取当前 `readLastJSONLLines` 和它的调用点
- [ ] 改成块状反向读取，不再逐字节 `ReadAt`
- [ ] 保持返回语义不变：
  - 最新 N 行
  - 空行跳过
  - limit 边界一致
- [ ] 补测试：
  - 小文件
  - 大文件
  - 空行
  - 单行文件
  - limit 大于文件行数

完成标准：
- 语义不变
- 测试覆盖
- 至少一条直接证据证明比旧实现更快或系统调用更少

#### B2. `channelSummary` / `ListChannels` 轻量统计

步骤：
- [ ] 不先重做整个 channel 存储，先做最小轻量统计路径
- [ ] 避免为了：
  - `MessageCount`
  - `LastMessageAt`
  全量反序列化频道消息
- [ ] 优先选低风险方案：
  - 行数统计 + 尾部读最后一条
  - 或已存在 room/channel summary 元数据复用
- [ ] 补测试与 smoke

完成标准：
- `ListChannels` 不再因大频道全量 JSON 反序列化而退化

#### B3. `LoadTaskMessages` / `LoadMessagesByContext` 搜索面收缩

步骤：
- [ ] 先利用已有 `ContextID / ChannelID / Task.ChannelID` 缩小搜索范围
- [ ] 只有在无法命中更窄范围时才 fallback 全量扫描
- [ ] 如果当前实现已经具备部分 narrowing，则只补缺口，不重写一整套索引
- [ ] 补测试：
  - task 绑定 channel
  - context 绑定单频道
  - fallback 全量搜索
  - limit 语义保持不变

完成标准：
- 常见 task/comment/context 查询不再默认扫所有频道所有消息

#### B4. `ListTeams` 与 `handleTeam` 读路径复核

步骤：
- [ ] 核对近几轮改动是否已部分解决这两项
- [ ] 若仍有串行热点：
  - `ListTeams` 做有限并发
  - `handleTeam` 改 `errgroup` 或同等更清晰的并发错误收敛
- [ ] 不为“代码好看”重做；只在当前实现确实弱时动手

完成标准：
- 这两项只在真实收益仍存在时才改，并有回归测试

### Phase C — 结构治理，但只做增量安全拆分

目标：
- 收 `store.go` 和 `handler.go` 的维护压力，但避免一次性大搬家带来高回归风险。

#### C1. `handler.go` 拆分优先

原因：
- 插件 handler 的拆分通常比 store 拆分风险更小
- 更容易保持行为不变

步骤：
- [ ] 引入 `TeamHandler` 或等价持依赖结构
- [ ] 先按功能拆：
  - `handler.go`
  - `handler_team.go`
  - `handler_member.go`
  - `handler_channel.go`
  - `handler_task.go`
  - `handler_artifact.go`
  - `handler_archive.go`
  - `handler_api.go`
  - `handler_helpers.go`
- [ ] 路由、模板、测试行为保持不变

完成标准：
- handler 结构拆分完成
- 路由和测试不回归

#### C2. `store.go` 只拆“低耦合块”

步骤：
- [ ] 不追求一次性拆完所有 CRUD
- [ ] 优先拆低耦合且边界清晰的部分：
  - `normalize.go`
  - `helpers.go`
  - `event.go`
  - `archive.go`
  - `index.go`
- [ ] 第二批再考虑：
  - `message_crud.go`
  - `task_crud.go`
  - `artifact_crud.go`
- [ ] 如果拆分过程开始引发大量循环依赖或语义漂移，立即停在当前批次并写回 runbook

完成标准：
- `store.go` 显著瘦身一轮
- 不因“大拆”把当前 Team 运行态打坏

### Phase D — 可靠性与接口语义收口

目标：
- 把源文档里那些“不是最热，但长期会坑人”的点收一轮。

#### D1. JSONL 读取/写入可靠性

步骤：
- [ ] 读取 JSONL 时补错误日志，不再静默丢弃损坏行
- [ ] 高频 append 路径至少做到：
  - 明确写入错误上抛
  - 必要时 `Sync()`
  - 读损坏行有日志
- [ ] 不在这一轮引入完整 WAL

完成标准：
- 损坏行不再完全 silent
- 写入错误可追踪

#### D2. `withTeamLock` timeout / context 化

步骤：
- [ ] 在不破坏现有调用面的前提下增加超时等待
- [ ] 优先最小实现：
  - non-blocking flock + retry + timeout
- [ ] 如改动面可控，再补 `withTeamLockCtx`
- [ ] 补测试：
  - 拿不到锁会超时返回
  - 正常路径不回归

完成标准：
- Team 锁等待不再理论上无限挂死

#### D3. webhook retry / logging / client reuse

步骤：
- [ ] 核对当前 webhook 逻辑是否已部分具备 retry
- [ ] 如果没有，就补：
  - client reuse
  - retriable status handling
  - 基本日志
- [ ] 不在这一轮引入复杂 dead-letter

完成标准：
- webhook 临时失败不再完全静默丢失

#### D4. 小而硬的 correctness 修复

步骤：
- [ ] 收下这些很明确的 correctness 点：
  - `mergeChannel` 的 `CreatedAt / UpdatedAt` 语义
  - handler 层重复 `ClosedAt` 逻辑
  - 锁内 `LoadTasks / LoadArtifacts` public 调用改内部路径
  - `NormalizeTeamID / sanitizeArchiveID` 的路径遍历防护
- [ ] 每项都配最小测试

完成标准：
- 这些明显不一致或不安全点不再继续留在主干

#### D5. `LoadMessages` limit 语义

步骤：
- [ ] 评估是否引入更清晰常量或 helper：
  - `LoadAllMessages`
  - `LoadAll = -1`
- [ ] 只在能不破坏现有调用方的前提下收口

完成标准：
- “0 表示全部”至少在接口层更可读，不再到处隐含

### Phase E — 测试、bench、文档写回

目标：
- 每一批不是“代码改了”，而是“真实收口了”。

步骤：
- [ ] 补足 `store_test.go` 中与本轮改动直接相关的测试
- [ ] 补足 `plugin_test.go` 中受 handler 拆分影响的路由/API 回归
- [ ] 如果做了热点性能优化，至少补一轮最小 bench/smoke 证据
- [ ] 执行：
  - `go test ./internal/haonews/team`
  - `go test ./internal/plugins/haonewsteam`
  - 必要时 `go test ./internal/haonews -run Team`
  - `go build ./cmd/haonews`
- [ ] 把本 runbook 写回到最新状态：
  - `done`
  - `defer`
  - `stale`
  - 当前剩余 next step

完成标准：
- runbook 可直接作为后续 resume 点
- 没有把“可能更好”误报成“已完成”

## Verification

执行时按批次验证，不要等到最后一次性碰运气。

最低验证：
- `go test ./internal/haonews/team -count=1`
- `go test ./internal/plugins/haonewsteam -count=1`
- `go build ./cmd/haonews`

按改动附加验证：
- 若改 `readLastJSONLLines`：
  - 针对新老实现差异补 targeted tests
  - 提供最小性能证据
- 若改 `channelSummary / ListChannels / LoadTaskMessages`：
  - 直接补 query 路径回归
  - 必要时做小型 synthetic 数据 smoke
- 若改锁 / webhook / 路径安全：
  - 必须有针对性单测，不接受只靠 build 宣称完成
- 若拆 `handler.go / store.go`：
  - 路由/API 行为回归必须过

完成定义：
- 代码改动存在
- 目标测试通过
- 构建通过
- runbook 状态写回
- 若存在 `defer`，原因写清楚而不是默默跳过

## Blockers / Resume

### Hard blockers

- 拆分 `store.go` 或 `handler.go` 时，撞上用户未提交且高冲突的同文件改动，无法安全增量合并
- 某项结构重构需要连带大面积调用面修改，已超出“增量安全拆分”边界
- 性能项没有可复现基线，继续修改只能靠猜

### If blocked, write back

- 保留本 runbook 结构
- 把对应 phase 改成：
  - `done`
  - `in_progress`
  - `blocked`
  - `defer`
- 写明：
  - 已完成到哪一步
  - 为什么阻塞
  - 哪个文件/函数是最后安全落点
  - 恢复时的第一步命令或第一处文件

### Resume point priority

如果中断，按下面顺序恢复：
1. `B1 readLastJSONLLines`
2. `B2 channelSummary`
3. `B3 LoadTaskMessages / LoadMessagesByContext`
4. `C1 handler.go split`
5. `D2 withTeamLock timeout`

## Initial Classification To Verify

执行时先验证这张初始判断，不要盲信：

- 预计 `done or mostly-done`
  - `4.5 ClosedAt 统一到 Store`
  - `5.1 trusted request`
  - `7.1 store tests 已存在`
- 预计 `todo`
  - `1.1 store.go 拆分`
  - `1.2 handler.go 拆分`
  - `2.1 readLastJSONLLines`
  - `2.2 channelSummary`
  - `2.3 LoadTaskMessages / LoadMessagesByContext`
  - `3.1 JSONL 读取错误可见性`
  - `3.2 withTeamLock timeout`
  - `4.4 mergeChannel`
  - `5.2 路径遍历防护`
- 预计 `defer or partial`
  - `4.2 legacy/index 双轨彻底移除`
  - `6.2 全量 context.Context 化`
  - `3.3 webhook 完整重试治理`

这张表只是执行起点，后续以代码事实为准。

## Execution Status

### Completed in this run

- `Phase A` 已完成首轮分类校正：
  - `7.1 store tests` = `stale`
    - 当前仓库已存在 [store_test.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/store_test.go)
  - `4.5 ClosedAt 重复逻辑` = `done`
    - handler 层重复 `ClosedAt` 逻辑已删除，Store 继续作为唯一状态机入口
  - `5.1 trusted request` = `done`
    - 之前已按 `RemoteAddr` 路径收口
- `B1 readLastJSONLLines` = `done`
  - 已改成块状反向读取
  - 已补回归测试
- `B2 channelSummary` = `done`
  - 已改为轻量统计，不再全量反序列化全部消息
  - 已补回归测试
- `B4 handleTeam / ListTeams` = `done`
  - `handleTeam` 已改为 `errgroup`
  - `ListTeams` 已改为有限并发
- `D3 webhook retry / client reuse` = `done`
  - `Store` 已持有复用 `webhookClient`
  - `sendWebhook` 已具备最小重试与日志
  - 已补 retriable status 回归
- `D5 LoadMessages limit 语义收口` = `done`
  - 已新增 `LoadAllMessages(...)`
  - 关键内部调用点已不再直接散落 `LoadMessages(..., 0)`
  - 现在 `LoadAllMessages / LoadTaskMessages / LoadMessagesByContext` 的职责边界已经稳定，不再混用“`limit=0` 同时表达全量与默认”的老语义
- `B3 LoadTaskMessages / LoadMessagesByContext` = `done`
  - 没有按源文档那种激进方式直接按 `Task.ChannelID` 硬收缩
  - 改成了更安全的 narrowing：
    - 候选频道优先
    - 若存在跨频道 task/context 消息，仍会 fallback 扫描剩余频道
  - `LoadTaskMessages` 现在会优先检查：
    - `task.ChannelID`
    - 同 `ContextID` 任务涉及的频道
    - `main`
    - 然后再补剩余频道
  - `LoadMessagesByContext` 现在会优先检查：
    - 同 `ContextID` 任务涉及的频道
    - `main`
    - 然后再补剩余频道
  - 已补 fallback 回归，确认不会因为 narrowing 打坏“跨频道 task comment/context message”语义
- `6.2 context.Context` = `done`
  - 已新增 `withTeamLockCtx(...)`
  - 现有 `withTeamLock / withTeamLockTimeout` 都已委托到 `withTeamLockCtx`
  - 已新增关键查询的 `...Ctx` 过渡入口：
    - `ListTeamsCtx`
    - `LoadTeamCtx`
    - `LoadMembersCtx`
    - `LoadPolicyCtx`
    - `LoadMessagesCtx`
    - `LoadTasksCtx`
    - `LoadTaskCtx`
    - `LoadArtifactsCtx`
    - `LoadHistoryCtx`
    - `LoadTaskMessagesCtx`
    - `LoadMessagesByContextCtx`
    - `LoadTasksByContextCtx`
  - 已新增高频公共写入口的 `...Ctx` 过渡入口：
    - `SaveMembersCtx`
    - `SaveWebhookConfigsCtx`
    - `SavePolicyCtx`
    - `AppendMessageCtx`
    - `SaveChannelCtx`
    - `HideChannelCtx`
    - `AppendTaskCtx`
    - `SaveTaskCtx`
    - `DeleteTaskCtx`
    - `AppendArtifactCtx`
    - `SaveArtifactCtx`
    - `DeleteArtifactCtx`
    - `AppendHistoryCtx`
    - `CreateManualArchiveCtx`
    - `SaveAgentCardCtx`
  - 已补剩余高频公共读入口的 `...Ctx` 变体：
    - `LoadWebhookConfigsCtx`
    - `ListArchivesCtx`
    - `LoadArchiveCtx`
    - `LoadArtifactCtx`
    - `LoadAgentCardCtx`
    - `ListAgentCardsCtx`
    - `LoadMembersSnapshotCtx`
    - `LoadPolicySnapshotCtx`
    - `LoadChannelSnapshotCtx`
  - Team 关键 handler 已接 `r.Context()` 的范围已扩到：
    - team index/detail
    - team channel
    - team task
    - team artifact
    - team member/policy
    - team archive
    - team sync
    - team A2A
    - team webhook / agent card / archive API
  - Team runtime 主路径也已切到 `ctx` 版本：
    - `team_sync.go` 现在通过 `ctx` 调：
      - `ListTeamsCtx`
      - `ListChannelsCtx`
      - `LoadMessagesCtx`
      - `LoadHistoryCtx`
      - `LoadTasksCtx`
      - `LoadArtifactsCtx`
      - `LoadMembersSnapshotCtx`
      - `LoadPolicySnapshotCtx`
  - 已补请求级取消回归
  - 本轮继续把 Team Store 的旧无 `ctx` 导出方法收成显式 compat 层：
    - [compat_api.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/compat_api.go)
  - 当前主实现已下沉到 `noCtx` helper：
    - `loadTeamNoCtx`
    - `loadMembersNoCtx`
    - `loadPolicyNoCtx`
    - `loadMessagesNoCtx`
    - `loadTasksNoCtx`
    - `loadArtifactsNoCtx`
    - `appendHistoryNoCtx`
    - `createManualArchiveNoCtx`
    - `saveAgentCardNoCtx`
    - 以及对应写路径 `save*/append*/delete*NoCtx`
  - `ctx_api.go` 现在直接走这些内部 helper，而不是再通过旧公开签名间接回跳
  - 旧无 `ctx` 导出方法仍保留，但它们已经明确退到 compat bridge，而不是主入口
- `4.2 legacy/index 双轨` = `done`
  - 已新增 task/artifact backend helper，收口了主流程里散落的 `hasTaskIndex / hasArtifactIndex` 分支：
    - `append*CurrentLocked`
    - `load*Current`
    - `save*CurrentLocked`
    - `delete*CurrentLocked`
    - `upsertReplicated*CurrentLocked`
  - 当前已经把 task/artifact 的主读写路径从“到处判断双轨”收成少数 helper
  - 当前剩余 `hasTaskIndex / hasArtifactIndex` 分支已基本只存在于：
    - `backend_helpers.go`
    - `index_store.go`
  - 也就是只剩明确的兼容/迁移层保留双轨
  - 本轮已复核主读写与运行时路径，没有再发现散落在插件/runtime 主流程里的额外 legacy/index 判断
  - 这意味着当前除了明确兼容层外，没有新的“还能顺手移掉”的 legacy 路径
  - 本轮继续把 task/artifact 运行态主路径改成 index-first：
    - `loadTasksCurrent`
    - `loadTaskCurrent`
    - `appendTaskCurrentLocked`
    - `saveTaskCurrentLocked`
    - `deleteTaskCurrentLocked`
    - `loadArtifactsCurrent`
    - `loadArtifactCurrent`
    - `appendArtifactCurrentLocked`
    - `saveArtifactCurrentLocked`
    - `deleteArtifactCurrentLocked`
    - `upsertReplicatedTaskCurrentLocked`
    - `upsertReplicatedArtifactCurrentLocked`
  - 当前这些主路径已不再每次根据 `hasTaskIndex / hasArtifactIndex` 做运行态双轨判断
  - index 缺失时会先显式 `ensureTaskIndex / ensureArtifactIndex`
  - 已新增 locked 读取入口：
    - `loadTasksCurrentLocked`
    - `loadArtifactsCurrentLocked`
  - Team archive 在持锁状态下也已切到 locked 入口，避免 index 迁移导致的重入锁超时
  - 现在 legacy 只剩明确兼容/迁移层：
    - `loadLegacyTasks*`
    - `loadLegacyArtifacts*`
    - `index_store.go` 中的迁移/compact/ensure helper
- `C1 handler.go` = `done`
  - 已把 `sync`、`archive`、`channel`、`task`、`artifact`、`member` 六块独立拆到：
    - `handler_sync.go`
    - `handler_archive.go`
    - `handler_channel.go`
    - `handler_task.go`
    - `handler_artifact.go`
    - `handler_member.go`
  - 现有路由和页面行为保持不变
  - 同时补上了 channel message create 的 Team history 记录，修复了拆分后暴露出来的历史断言缺口
  - 本轮不再继续为了文件名整齐度而强拆 `handler_api.go / handler_helpers.go`，因为主要风险块已经按职责拆开，继续拆分的收益已经明显下降
- `D1 JSONL 读取错误可见性` = `done`
  - 读取损坏 JSONL 行时已记录日志
  - 本 runbook 不进入 `fsync/WAL`，因为这已经超出“最小可靠性收口”的边界
- `D2 withTeamLock timeout` = `done`
  - 已改为非阻塞尝试 + retry + timeout
  - 已补超时测试
- `D4 correctness fixes` = `done`
  - `mergeChannel CreatedAt` 已修
  - 锁内 `SaveTask/DeleteTask/SaveArtifact/DeleteArtifact` 已改为内部 legacy 加载路径
  - `NormalizeTeamID / sanitizeArchiveID` 路径遍历防护已补
- `C2 store.go` = `done`
  - 已拆出第一批低耦合块：
    - [normalize.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/normalize.go)
    - [paths.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/paths.go)
    - [jsonl_helpers.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/jsonl_helpers.go)
  - 已拆出第二批低耦合块：
    - [archive.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/archive.go)
    - [index_store.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/index_store.go)
  - 当前已把：
    - ID/路径归一化
    - team/channel/archive path 与 team lock
    - JSONL 尾部读取与轻量统计 helper
    - Team archive 快照读写
    - index-backed task/artifact backend helper
    从 [store.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/store.go) 拆出
  - 本 runbook 到这里不再继续强拆 `task/artifact/message` CRUD 主流程，避免在没有额外收益证据时把主逻辑一口气打散

### Deferred in this run

- `D3 webhook 完整 dead-letter / 更复杂退避治理`

### Verified in this run

- `go test ./internal/haonews/team -run 'TestStoreListTeams|TestNormalizeTeamID|TestNormalizeTeamIDAndSanitizeArchiveIDRejectTraversal|TestReadLastJSONLLinesReturnsNewestNonEmptyLines|TestStoreChannelSummaryCountsMessagesAndLatestTimestamp|TestWithTeamLockTimeoutReturnsDeadlineExceededStyleError|TestMergeChannelPreservesEarliestCreatedAt|TestStoreAppendAndLoadMessages|TestStoreLoadTaskMessages|TestStoreLoadMessagesByContext' -count=1`
- `go test ./internal/haonews/team -run 'TestStoreWebhookReceivesPublishedEvent|TestStoreWebhookRetriesRetriableStatus' -count=1`
- `go test ./internal/haonews/team -run 'TestStoreCreateManualArchive|TestStoreMigrateTasksToIndexKeepsTasksReadable|TestStoreIndexedTaskCRUDAndCompact|TestStoreMigrateArtifactsToIndexAndCompact' -count=1`
- `go test ./internal/haonews/team -run 'TestWithTeamLockCtxCancelsWhileWaiting|TestStoreContextIDAutoGeneratedAndQueryable|TestStoreLoadMessagesByContextFallsBackBeyondPreferredChannels|TestStoreLoadTaskMessagesFallsBackBeyondPreferredChannels|TestStoreLoadTaskMessages' -count=1`
- `go test ./internal/plugins/haonewsteam -run 'TestPluginBuildServesTeamDetailAndAPI|TestPluginBuildServesTeamSyncHealthPageAndAPI|TestPluginBuildHandlesTeamTaskStatusAndArtifactTaskRelation' -count=1`
- `go test ./internal/plugins/haonewsteam -run 'TestPluginBuildServesTeamDetailAndAPI|TestPluginBuildTeamTaskAndArtifactWorkflows|TestPluginBuildCreatesTaskFromHTMLForm' -count=1`
- `go test ./internal/plugins/haonewsteam -run 'TestPluginBuildServesTeamDetailAndAPI|TestPluginBuildHandlesTeamTaskFormWrites|TestPluginBuildHandlesTeamChannelFormWrites|TestPluginBuildHandlesTeamTaskStatusAndArtifactTaskRelation' -count=1`
- `go test ./internal/plugins/haonewsteam -run 'TestPluginBuildServesTeamDetailAndAPI|TestPluginBuildHandlesTeamTaskFormWrites|TestPluginBuildHandlesTeamChannelFormWrites|TestPluginBuildHandlesTeamTaskStatusAndArtifactTaskRelation|TestPluginBuildServesAndResolvesTeamSyncConflicts|TestPluginBuildServesTeamSyncHealthPageAndAPI' -count=1`
- `go test ./internal/plugins/haonewsteam -run 'TestPluginBuildServesTeamSyncHealthPageAndAPI|TestPluginBuildServesAndResolvesTeamSyncConflicts|TestPluginBuildServesTeamArchiveRoutes|TestPluginBuildCreatesTeamArchiveFromWorkspaceRoutes' -count=1`
- `go test ./internal/plugins/haonewsteam -run 'TestPluginBuildServesTeamDetailAndAPI|TestPluginBuildServesTeamSyncHealthPageAndAPI|TestPluginBuildHandlesTeamTaskStatusAndArtifactTaskRelation|TestPluginBuildServesTeamArchiveRoutes|TestPluginBuildCreatesTeamArchiveFromWorkspaceRoutes' -count=1`
- `go test ./internal/haonews/team -count=1`
- `go test ./internal/plugins/haonewsteam -count=1`
- `go test ./internal/haonews -run 'TestTeamPubSubRuntimePrimesThenPublishesNewMessageAndHistory|TestTeamPubSubRuntimePublishesAndAppliesTaskAndArtifact|TestTeamPubSubRuntimePublishesAndAppliesMemberPolicyChannel|TestTeamPubSubRuntimePersistsPublishedCursorAcrossRestart|TestTeamPubSubRuntimeAppliesInboundAckForTargetNode|TestTeamPubSubRuntimeRetriesPendingUnackedObjects' -count=1`
- `go build ./cmd/haonews`

### Runbook Completion

- 这份 runbook 现在可以视为 `completed`：
  - `handler.go` 主拆分完成
  - `store.go` 低耦合拆分完成
  - 热点性能项完成
  - `ctx` 主入口完成
  - task/artifact 运行态 `legacy/index` 双轨移除完成
  - 测试与构建验证完成
- 仍保留为后续增强项的，只有本 runbook 明确不进入的下一层：
  - webhook dead-letter / 更复杂退避治理
  - 若未来需要，再彻底删掉 legacy 迁移输入 helper 本身

## Detailed Continuation Plan

### Phase F — 全量 public Store API 强制 `ctx` 签名

状态：
- `done`

目标：
- 不再停在“有 `...Ctx` 变体可用”。
- 把 Team Store 的**公共面**正式收成 `ctx` 优先，旧无 `ctx` 导出方法只保留内部兼容层或测试过渡层。

执行顺序：
1. 先收只读公共 API
2. 再收写入公共 API
3. 再收 sync / archive / agent-card / snapshot 这类边缘但仍公开的方法
4. 最后清理插件/runtime 对旧签名的剩余依赖

#### F1. 只读公共 API `ctx` 化

收口对象：
- `ListTeams`
- `LoadTeam`
- `LoadMembers`
- `LoadMembersSnapshot`
- `LoadPolicy`
- `LoadPolicySnapshot`
- `LoadChannel`
- `LoadChannelSnapshot`
- `ListChannels`
- `LoadMessages`
- `LoadAllMessages`
- `LoadTasks`
- `LoadTask`
- `LoadArtifacts`
- `LoadArtifact`
- `LoadHistory`
- `LoadTaskMessages`
- `LoadMessagesByContext`
- `LoadTasksByContext`
- `LoadWebhookConfigs`
- `ListArchives`
- `LoadArchive`
- `LoadAgentCard`
- `ListAgentCards`

实施方式：
- 新增内部无 `ctx` helper 时，命名收成：
  - `loadTeamNoCtx`
  - `loadMembersNoCtx`
  - `loadMessagesNoCtx`
  - 类似这种内部 helper
- 公开方法统一变成：
  - `LoadTeam(ctx context.Context, teamID string)`
  - `LoadMessages(ctx context.Context, teamID, channelID string, limit int)`
- 旧签名不直接保留为同名导出重载；Go 不支持重载，所以：
  - 原有公开无 `ctx` 版本要么改成内部 helper
  - 要么只保留极少数明确兼容桥接点，并改名为：
    - `LoadTeamCompat`
    - 仅在确有必要时使用
- 插件层、runtime、Team sync、archive、A2A 全部切到新签名

验证：
- `go test ./internal/haonews/team`
- `go test ./internal/plugins/haonewsteam`
- `go test ./internal/haonews -run Team`
- `go build ./cmd/haonews`

完成标准：
- Team 运行代码中不再继续依赖旧无 `ctx` 的公开只读方法
- 旧方法若仍存在，也只作为明确标注的兼容层存在，而不是主入口

#### F2. 写入公共 API `ctx` 化

收口对象：
- `SaveMembers`
- `SaveWebhookConfigs`
- `SavePolicy`
- `AppendMessage`
- `SaveChannel`
- `HideChannel`
- `AppendTask`
- `SaveTask`
- `DeleteTask`
- `AppendArtifact`
- `SaveArtifact`
- `DeleteArtifact`
- `AppendHistory`
- `CreateManualArchive`
- `SaveAgentCard`

实施方式：
- 公开写入方法统一改成 `ctx` 签名
- 旧写入逻辑下沉成内部 helper：
  - `saveMembersNoCtx`
  - `appendMessageNoCtx`
  - `saveTaskNoCtx`
  - 等
- `withTeamLockCtx` 成为默认锁入口；内部不再优先走无 `ctx` 版本

验证：
- 补最小 targeted tests：
  - 请求取消时不再进入后续锁等待/写入
  - 正常写入路径行为不变
- 继续跑 Team plugin 回归

完成标准：
- 插件/API/runtime 主写路径全部通过 `ctx` 入口进入 Store
- 旧无 `ctx` 写接口不再是主调用面

#### F3. runtime / sync / archive / agent-card 收尾

实施方式：
- `team_sync.go`、archive、agent-card、snapshot、conflict resolve 等边缘公开调用面，全部对齐新签名
- `context.Background()` 仅保留在：
  - 启动预热
  - 极少量纯内部定时任务
- 对于 request / stream / sync loop，都使用真实 `ctx`

完成标准：
- 代码搜索中，`teamcore.Store` 的调用大部分已为 `ctx` 版本
- 非 `ctx` 调用只剩测试或明确 compat layer

### Phase G — 真正移除 legacy 路径

状态：
- `done`

目标：
- 不再只说“legacy/index 已经收成兼容层位置”。
- 真的把不再需要的 legacy 路径移掉。

执行顺序：
1. 先识别哪些 team 已可默认 index-backed
2. 再把主读路径切成 index-first 且不再 fallback legacy
3. 最后把 legacy 仅保留给显式迁移命令或一次性 upgrade path

#### G1. 事实核对：现存 legacy 路径清单

要点：
- 列出当前还保留的 legacy 入口：
  - `loadLegacyTasks`
  - `loadLegacyArtifacts`
  - `saveTasks`
  - `saveArtifacts`
  - 以及与 `tasks.jsonl / artifacts.jsonl` 直接耦合的 helper
- 区分三类：
  - 仍被运行态主路径使用
  - 只被迁移/compact 使用
  - 只被测试覆盖

完成标准：
- 有明确清单，不凭印象删

#### G2. Task/Artifact 主路径彻底 index-first

实施方式：
- 把 Task/Artifact 的主读写路径改成：
  - 没有 index 时，先显式迁移到 index
  - 运行态不再长期双轨判断
- 如果迁移代价可控：
  - `OpenStore` 或首次访问时触发安全迁移
- 如果迁移代价不适合启动期：
  - 保留显式 `ensureTaskIndex / ensureArtifactIndex`
  - 但主路径不再继续每次 `hasTaskIndex / else legacy`

验证：
- `MigrateTasksToIndex`
- `MigrateArtifactsToIndex`
- `CompactTasks`
- `CompactArtifacts`
- CRUD 回归
- sync apply / delete / compact 回归

完成标准：
- Task/Artifact 的运行态主路径不再依赖 legacy 双轨

#### G3. legacy helper 收尾与测试更新

实施方式：
- 删除已不再被运行态引用的 legacy helper
- 测试改成：
  - legacy 输入用于迁移测试
  - index-backed 作为常规运行态测试

完成标准：
- `legacy` 成为“迁移输入格式”，不再是长期运行态分支

### Phase H — 最终验收与文档写回

目标：
- 不把“改了很多签名”误报成完成。

步骤：
- 跑整组：
  - `go test ./internal/haonews/team -count=1`
  - `go test ./internal/plugins/haonewsteam -count=1`
  - `go test ./internal/haonews -run Team -count=1`
  - `go build ./cmd/haonews`
- 写回：
  - 哪些 public API 已正式 `ctx` 化
  - 哪些 compat 方法仍保留
  - legacy 剩余边界
  - 如果停在某一层，下一步的第一处文件和第一条命令

完成标准：
- runbook 能直接说明：
  - `ctx` 强制收口完成到哪一层
  - `legacy` 移除完成到哪一层
  - 剩余的是不是还值得继续
