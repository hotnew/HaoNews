# `20260403-pro2-claude-codex2-task.auto-run.md`

## Goal

- 把 [20260403-pro2-claude.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/20260403-pro2-claude.md) 收敛成一份可直接执行的 `haonews` 改良 runbook。
- 目标不是机械完成原文所有条目，而是：
  - 先核对原文判断和当前代码现实是否一致
  - 再按关键路径执行仍然值得做的项
  - 每一批都带验证闭环
  - 只有在新 profile / bench 证据支持时才进入高风险重构

## Context

- 仓库：
  - `/Users/haoniu/sh18/hao.news2/haonews`
- 源文档：
  - [20260403-pro2-claude.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/20260403-pro2-claude.md)
- 后续执行模式目标：
  - `$auto-run-plan2`
- 当前规划必须假设：
  - 源文档有一部分判断可能已经过时
  - 不能直接相信“仍需 / 未实现”标签，必须先核当前代码
  - 当前稳定面不能被随意打破：
    - `Team / Live / Topics` 保持分离
    - 归档三线保持分离
    - `.75` 是主要运行态验收节点

### 已知高价值方向

- 请求路径串行 I/O：
  - `Team` 页面 handler
  - `Live archive` 列表 handler
- 并发与稳定性：
  - witness 限流
  - LAN peer cache 竞态
  - pubsub goroutine / connect fanout 控制
- 高频读路径：
  - subscription normalize 重复成本
  - Team handler 过大 limit
  - Team message 尾部读取
- 仅在证据明确时才进入的大改：
  - `Live AppendEvent` 锁内拆分
  - index 签名增量化
  - bundle 并行加载
  - transfer 流式 TAR

### 规划约束

- 先做“最小充分改动”，不要一上来翻大底层。
- 不把一次实现当成验证；每批都要单独验证。
- 不把不相关脏改带入提交。
- 如果某条在当前代码里已经完成，只写回 `done`，不重复改。
- 如果某条缺少新证据支持，不进入实现，写回 `defer`。

## Execution Plan

### Phase A — 源文档与当前代码对齐

目标：
- 先建立正确任务模型，避免后续执行跑偏。

#### A1. 建立执行矩阵

- [ ] 逐条提取源文档中的候选项，至少覆盖：
  - `N1 Team/Live handler N+1`
  - `N2 Witness semaphore`
  - `N3 LAN peer health cache race`
  - `N4 Subscription normalize/cache`
  - `N5 Team handler limit`
  - `N6 Live AppendEvent 锁拆分`
  - `#2 Transfer 流式 TAR`
  - `#3 PubSub goroutine 控制`
  - `#5 Index 签名增量化`
  - `#6 Bundle 并行加载`
  - `#11 Team LoadMessages 尾部读取`
  - `S1 X-Forwarded-For`
- [ ] 为每条记录：
  - 关联文件
  - 当前状态：
    - `done`
    - `todo`
    - `defer`
  - 证据来源：
    - 文件路径
    - 测试名
    - bench / profile 结论

#### A2. 强制核对已知易过时项

- [ ] 强制核对 `#11 Team LoadMessages 尾部读取` 是否已在当前代码里实现。
- [ ] 强制核对 `N2 Witness semaphore` 是否仍缺失，还是仅缺更细的并发上限。
- [ ] 强制核对 `N3 LAN peer health cache race` 是否仍真实存在。
- [ ] 强制核对 `S1 X-Forwarded-For` 在当前 live moderation 路径里是否仍有风险。

完成标准：
- 源文档中的候选项都有状态归类。
- 后续执行不再重复碰已完成项。

### Phase B — 第一批直接做的低风险高收益项

进入条件：
- `Phase A` 完成，且这些项状态为 `todo`。

#### B1. Team/Live handler N+1 并行化

目标：
- 降低 Team 页面和 Live archive 列表页的串行 I/O 等待。

文件：
- [internal/plugins/haonewsteam/handler.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/handler.go)
- [internal/plugins/haonewslive/handler.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewslive/handler.go)

步骤：
- [ ] 识别 `handleTeam()` 中可并行的独立读取：
  - `LoadTeam`
  - `LoadMembers`
  - `LoadPolicy`
  - `LoadMessages`
  - `LoadTasks`
  - `LoadArtifacts`
  - `LoadHistory`
  - `ListChannels`
  - `app.Index` 或等效只读索引依赖
- [ ] 用并发组收敛这些读取；保持错误传播明确，不吞错。
- [ ] `handleLiveArchiveIndex()` 对多房间归档索引读取改成有限并行。
- [ ] 并发数加上上限，不做无限 goroutine fanout。

完成标准：
- `Team` 详情页和 `Live archive index` 不再是明显串行 I/O 链。
- 页面行为和错误语义不变。

#### B2. Team handler limit 收口

目标：
- 去掉明显偏大的列表预取和不受控 limit。

文件：
- [internal/plugins/haonewsteam/handler.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/handler.go)

步骤：
- [ ] 核对 Team 页面当前 preview limit 和 API limit 解析逻辑。
- [ ] 引入统一的 limit clamp helper。
- [ ] 页面 preview 默认值收成较小且足够的值。
- [ ] API limit 只允许落在明确范围内。

完成标准：
- Team 页面不再默认预取过多消息 / 任务 / 产物。
- API 不能靠极大 limit 放大读放大。

#### B3. Subscription normalize / cache

目标：
- 避免高频请求重复做同一份 normalize 工作。

文件：
- [internal/plugins/haonews/subscriptions.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonews/subscriptions.go)
- [internal/haonews/subscriptions.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/subscriptions.go)

步骤：
- [ ] 确认当前是否仍在请求路径重复 normalize。
- [ ] 如果仍存在，按文件 modtime 或等效签名缓存 normalized 规则。
- [ ] 保持匹配语义不变，缓存失效条件明确。

完成标准：
- 同一份订阅规则不会在热路径重复 normalize。
- 订阅匹配结果保持兼容。

#### B4. Witness semaphore

目标：
- 在已有并行 witness 的基础上补齐有限并发控制。

文件：
- [internal/haonews/credit_witness.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/credit_witness.go)

步骤：
- [ ] 如果当前已是并行请求，补上 semaphore / worker 限流。
- [ ] `needed` 达成后继续早停。
- [ ] 统一 timeout 和错误收集行为。

完成标准：
- witness 不再因无界并行扩大尾部风险。
- witness 最坏等待时间不回退。

#### B5. LAN peer health cache race

目标：
- 修复 LAN peer 健康缓存的并发读写风险。

文件：
- [internal/haonews/lanpeer.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/lanpeer.go)

步骤：
- [ ] 核对 `lanPeerHealthCache` 当前锁策略。
- [ ] 若确有并发读写窗口，补 `RWMutex` 或等效同步。
- [ ] 不改变现有健康评分语义。

完成标准：
- `go test -race` 不再对这条缓存发警报。

#### B6. S1 `X-Forwarded-For` 信任边界

目标：
- 不再无条件信任 `X-Forwarded-For` 作为 moderation 安全判断依据。

文件：
- [internal/plugins/haonewslive/handler.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewslive/handler.go)

步骤：
- [ ] 核对 `livePublicClientIP()` 当前逻辑。
- [ ] 仅当 `RemoteAddr` 命中可信代理配置时，才解析 `X-Forwarded-For`。
- [ ] 默认配置保持保守：无可信代理时只信任 `RemoteAddr`。
- [ ] 给 moderation 相关路径补最小安全测试。

完成标准：
- 伪造 `X-Forwarded-For` 不再能绕过本地 / 内网限制判断。

### Phase C — 第二批可做但必须先验证收益的项

进入条件：
- `Phase B` 全通过，且新的 bench/profile 仍显示相关路径值得继续做。

#### C1. PubSub goroutine / connect fanout 控制

文件：
- [internal/haonews/pubsub.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/pubsub.go)

步骤：
- [ ] 核对当前订阅 goroutine 数量控制是否缺失。
- [ ] 为订阅数和 connect fanout 增加明确上限。
- [ ] 连接尝试改成 semaphore 控制。

完成标准：
- goroutine 不再随 topic / peers 无限膨胀。

#### C2. Team `LoadMessages` 尾部读取 / 分页

文件：
- [internal/haonews/team/store.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/store.go)

步骤：
- [ ] 仅在 `Phase A` 证实这条仍未完成时才进入。
- [ ] 分离“最新 N 条读取”和“全量读取”。
- [ ] 最新消息路径改成尾部读取；归档路径保留全量。

完成标准：
- Team 工作区“最新消息”路径不再全量扫整个 JSONL。

#### C3. Live archive 列表页深度优化

文件：
- [internal/plugins/haonewslive/handler.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewslive/handler.go)

步骤：
- [ ] 如果 `B1` 后 live archive 列表仍慢，再引入缓存摘要或有限并行聚合。
- [ ] 只优化 index / summary 路径，不改 archive 数据语义。

完成标准：
- `archive/live` 列表页 p95 下降，且不引入额外一致性问题。

### Phase D — 高风险项，只在新证据明确时进入

进入条件：
- `Phase B/C` 做完后仍有明确 bench / profile / race / pprof 证据指向这些点。
- 如果没有新证据，全部保持 `defer`。

候选项：
- [ ] `N6 Live AppendEvent` 锁内拆分
- [ ] `#5 Index` 签名增量化
- [ ] `#6 Bundle` 并行加载
- [ ] `#2 Transfer` 流式 TAR
- [ ] `#7 Post` 列表避免全量克隆

执行规则：
- 每次最多进入一项。
- 每做一项前先写下进入理由：
  - 当前瓶颈证据
  - 预期收益
  - 回滚点
- 未拿到证据前，不得提前实现。

完成标准：
- 高风险项必须有“做前证据”和“做后对比”。
- 没有对比结论，不算完成。

## Verification

### A. 代码与测试

- 运行最相关测试，不做无差别全仓扫：
  - `go test ./internal/plugins/haonewsteam`
  - `go test ./internal/plugins/haonewslive`
  - `go test ./internal/plugins/haonews`
  - `go test ./internal/haonews`
  - `go test ./internal/haonews/team`
  - `go test ./internal/haonews/live`
- 并发/竞态修复项必须补：
  - `go test ... -race`
- 构建至少要过：
  - `go build ./cmd/haonews`

### B. Bench / 压测

- 对关键页做基线前后对比：
  - `Team` 详情页
  - `archive/live`
  - `/`
  - `/topics`
  - `/api/feed`
- 如果已有 bench 脚本，优先复用；没有就使用最小 HTTP bench 命令。
- 只有在“做后指标更好或至少不回退”时才算通过。

### C. 运行态

- `.75` 作为主要验证节点。
- 至少检查：
  - `/api/network/bootstrap`
  - Team 页面关键路径
  - Live archive 列表页
  - moderation 相关安全路径
- 冷态与热态都至少各测一轮。

### 完成标准

- 至少一批低风险项落地并通过最相关测试/构建。
- runbook 中每一项都有明确状态：
  - `done`
  - `todo`
  - `defer`
  - `blocked`
- 若进入高风险项，必须附带前后对比证据；否则继续 `defer`。

## Blockers / Resume

### Hard blockers

- `go test -race` 指向真实并发问题，且修复需要跨模块大改。
- Bench / profile 证据和源文档判断明显冲突，无法确定是否值得继续。
- `.75` 运行态验证失败且原因不在当前改动范围内。
- 当前 worktree 存在会污染提交的无关脏改，且无法安全隔离。

### If blocked, write back

- 把当前状态写回本文件底部，新增或更新 `Execution Status`：
  - 已完成项
  - 当前阻塞项
  - 阻塞证据
  - 放弃理由或 defer 理由
- 写清具体卡在哪个文件、哪条命令、哪类证据。
- 不要写“继续排查”这种空话；必须留下可恢复下一步。

### Next step to resume

- 如果卡在 `Phase A`：
  - 下一步是完成剩余候选项的状态归类。
- 如果卡在 `Phase B`：
  - 下一步是从第一个未完成的 `B*` 条目继续。
- 如果卡在 `Phase C/D`：
  - 下一步是先补新的 profile / bench 证据，再决定继续还是写 `defer`。

## Execution Status

- 当前状态：
  - `Phase A` 已完成
  - `Phase B` 第一批已部分完成
  - `Phase C/D` 尚未进入

### 已核对状态

- `done`
  - `#11 Team LoadMessages 尾部读取`
    - 证据：
      - [internal/haonews/team/store.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/store.go)
      - `LoadMessages(limit>0)` 已使用尾部读取 helper
- `todo`
  - 无
- `defer`
  - `N6 Live AppendEvent 锁拆分`
  - `#5 Index 签名增量化`
  - `#6 Bundle 并行加载`
  - `#2 Transfer 流式 TAR`
  - `Phase D` 高风险重构项

### 本轮已完成

- `N1 / B1 Team/Live handler N+1`
  - [internal/plugins/haonewsteam/handler.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/handler.go)
  - [internal/plugins/haonewslive/handler.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewslive/handler.go)
  - `handleTeam()` 改为并行收敛主要只读加载。
  - `Live archive` 页面和 API 改为复用有限并行摘要加载。
- `B2 Team handler limit 收口`
  - [internal/plugins/haonewsteam/handler.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/handler.go)
  - Team messages / channel messages / tasks / artifacts API 增加 `limit` clamp。
- `N2 / B4 Witness semaphore`
  - [internal/haonews/credit_witness.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/credit_witness.go)
- `N3 / B5 LAN peer health cache race`
  - [internal/haonews/lanpeer.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/lanpeer.go)
  - 为健康缓存读写补 `RWMutex`，保存时做快照拷贝。
- `S1 / B6 X-Forwarded-For`
  - [internal/plugins/haonewslive/handler.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewslive/handler.go)
  - moderation 信任判断改为仅基于 `RemoteAddr`，不再直接信任 `X-Forwarded-For`。
- `B3 Subscription normalize / cache`
  - [internal/plugins/haonews/server.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonews/server.go)
  - [internal/plugins/haonews/index_cache.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonews/index_cache.go)
  - `subscriptionRules()` 增加基于 `modTime/size` 的轻量缓存，`invalidateIndexCache()` 会同步失效。
- `C1 PubSub goroutine / connect fanout 控制`
  - [internal/haonews/pubsub.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/pubsub.go)
  - 为 pubsub 订阅 goroutine 增加订阅上限保护。
  - `findPeersOnce()` 改成有限并行连接，连接 fanout 受 semaphore 控制。
- `C3 Live archive 列表页深度优化`
  - [internal/haonews/live/store.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/live/store.go)
  - `ListHistoryArchives()` 增加 Redis 短 TTL 缓存。
  - 新归档写入时主动失效 `history list` 缓存，`archive/live` 列表页不再每次都全目录读历史批次。
- `C2 Team 冷路径尾部读取优化`
  - [internal/haonews/team/store.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/store.go)
  - [internal/haonews/team/store_test.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/store_test.go)
  - `LoadTasks(limit>0)`、`LoadArtifacts(limit>0)`、`LoadHistory(limit>0)` 改成扫描文件尾部最新 N 条。
  - `saveTasks()`、`saveArtifacts()` 落盘前按 `UpdatedAt` 升序稳定排序，保证“文件尾部 = 最新记录”。
- `E2 Team / Live 启动预热`
  - [internal/plugins/haonewsteam/plugin.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/plugin.go)
  - [internal/plugins/haonewslive/plugin.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewslive/plugin.go)
  - 非测试进程启动后持续后台预热：
    - Team 工作区所需的 team/members/policy/messages/tasks/artifacts/history/channels/archives
    - Live archive 房间摘要和相关只读索引
  - 目标是把 `archive/live` 与 Team 详情页的首轮冷构建从秒级压到可接受范围。
- `E3 Index / NodeStatus stale-while-revalidate`
  - [internal/plugins/haonews/server.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonews/server.go)
  - [internal/plugins/haonews/ops_status.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonews/ops_status.go)
  - [internal/plugins/haonews/index_cache_test.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonews/index_cache_test.go)
  - [internal/plugins/haonews/ops_status_cache_test.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonews/ops_status_cache_test.go)
  - `NodeStatus` 过期后先回旧值、后台刷新。
  - `Index` 在已有缓存但超过 probe 复检窗口时，先回旧索引、后台做签名复检/重建。
  - 目标是消除 `archive/live` / Team 主详情页“隔一会儿第一下又卡 3 秒”的周期性长尾。

### 本轮验证

- `go test ./internal/haonews -run 'TestCollectWitnessesFromCandidatesStopsAfterLimit|TestCollectWitnessesFromCandidatesCapsConcurrency|TestSortLANPeerCandidates|TestLANPeerHealthCacheRoundTrip|TestResolveLANBootstrapPeersFetchesCandidatesInParallel'`
- `go test -race ./internal/haonews -run 'TestCollectWitnessesFromCandidatesStopsAfterLimit|TestCollectWitnessesFromCandidatesCapsConcurrency|TestSortLANPeerCandidates|TestLANPeerHealthCacheRoundTrip|TestResolveLANBootstrapPeersFetchesCandidatesInParallel'`
- `go test ./internal/plugins/haonewslive -run 'TestPluginBuildUpdatesLivePublicModerationRules|TestPluginBuildRejectsSpoofedForwardedForOnLivePublicModeration|TestPluginBuildCreatesAndServesLiveHistoryArchive'`
- `go test ./internal/plugins/haonewsteam -run 'TestPluginBuildServesTeamDetailAndAPI|TestPluginBuildTeamTaskAndArtifactWorkflows|TestPluginBuildCreatesTaskFromHTMLForm'`
- `go test ./internal/plugins/haonews -run 'TestAppSubscriptionRulesCachesUntilFileChanges|TestLoadSubscriptionRulesNormalizesDiscoverySelectors|TestApplySubscriptionRulesFiltersByTopicAndCarriesReplies'`
- `go test ./internal/haonews -run 'TestPubSubRuntimeReserveSubscriptionRespectsLimit|TestPubSubRuntimeConnectDiscoveredPeersCapsConcurrency|TestPubSubRuntimeConnectDiscoveredPeersRecordsErrors|TestPubSubRuntimeConnectDiscoveredPeersSkipsExistingAndSelf|TestSubscribedAnnouncementTopics|TestMatchesAnnouncement'`
- `go test -race ./internal/haonews -run 'TestPubSubRuntimeReserveSubscriptionRespectsLimit|TestPubSubRuntimeConnectDiscoveredPeersCapsConcurrency|TestPubSubRuntimeConnectDiscoveredPeersRecordsErrors|TestPubSubRuntimeConnectDiscoveredPeersSkipsExistingAndSelf|TestSubscribedAnnouncementTopics|TestMatchesAnnouncement'`
- `go test ./internal/haonews/live -run 'TestListHistoryArchivesUsesRedisCache|TestCreateManualAndDailyHistoryArchives|TestHistoryArchivesKeepAllVisibleEventsBeyondDisplayWindow'`
- `go test ./internal/plugins/haonewslive -run 'TestPluginBuildCreatesAndServesLiveHistoryArchive'`
- `go test ./internal/haonews/team -run 'TestStoreAppendAndLoadMessages|TestStoreLoadMessagesLimitReadsLatestMessages|TestStoreAppendAndLoadTasks|TestStoreLoadTasksLimitReadsLatestTasks|TestStoreSaveMembersAndLoadArtifacts|TestStoreLoadArtifactsAndHistoryLimitReadLatestEntries|TestStoreAppendTaskConcurrentPreservesAllTasks|TestStoreSaveAndDeleteTask'`
- `go test ./internal/plugins/haonewsteam -run 'TestPluginBuildServesTeamDetailAndAPI|TestPluginBuildTeamTaskAndArtifactWorkflows|TestPluginBuildCreatesTaskFromHTMLForm|TestPluginBuildServesTeamArchiveRoutes|TestPluginBuildCreatesTeamArchiveFromWorkspaceRoutes'`
- `go test ./internal/plugins/haonewslive -run 'TestPluginBuildCreatesAndServesLiveHistoryArchive|TestPluginBuildLimitsVisibleLiveEventsButCanShowAll|TestPluginBuildServesLiveIndex'`
- `go test ./internal/plugins/haonews -run 'TestAppIndexCachesUntilStoreSignatureChanges|TestCurrentIndexSignatureUsesQuickProbeCacheBetweenDeepChecks|TestAppIndexReturnsStaleWhileRefreshingAfterProbeExpiry|TestNodeStatusReturnsStaleWhileRefreshing'`
- `go build ./cmd/haonews`

### 最新运行态证据

- `.75` 已切到包含启动预热的本地二进制，并确认：
  - `http_ready=true`
  - `index_ready=true`
  - `warmup_ready=true`
- 复测结论：
  - 仅靠 `E2` 启动预热并不能消除周期性长尾。
  - 根因是 `app.Index()` 的 `2s` probe 复检窗口过期后，会由首个请求同步承担签名检查链。
- `E3` 落地后的关键实测：
  - 超过 `indexCacheProbeInterval` 后再次访问：
    - `/archive/live`
      - `~54.5ms / 5.1ms / 8.4ms`
    - `/teams/archive-demo`
      - `~25.5ms / 2.9ms / 4.5ms`
  - 说明这两个页面已经不再在 probe 复检点掉回 3 秒级。
- 热态基线未退化：
  - `/` p95 `~8.3ms`
  - `/topics` p95 `~1.8ms`
  - `/api/feed` p95 `~2.9ms`
  - `/topics/futures/rss` p95 `~3.1ms`

### 恢复下一步

- 当前低风险与中风险主线已收完；`archive/live` / `Team` 周期性首击长尾也已经通过 `Index stale-while-revalidate` 压下去。
- 现阶段没有新的证据支持进入 `Phase D` 高风险项。
- 如果继续执行，优先顺序是：
  - 直接走 `main + tag + release`
  - 然后在 `.75` 持续观察
  - 只有当新的 bench / profile 明确指出仍有稳定瓶颈时，再重开 `Phase D`
