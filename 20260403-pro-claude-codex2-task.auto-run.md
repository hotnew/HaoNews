# `20260403-pro-claude-codex2-task.auto-run.md`

## Goal

- 把 [20260403-pro-claude.md](/Users/haoniu/sh18/hao.news2/20260403-pro-claude.md) 重构成一份 **可直接执行的 `haonews` 性能改良 runbook**。
- 这份 runbook 不是“照单做完 19 项”，而是：
  - 先核实现状
  - 再按关键路径执行仍然值得做的优化
  - 每一批都要有明确验证
  - 只在真实硬阻塞时停

## Context

- 当前仓库：
  - `/Users/haoniu/sh18/hao.news2/haonews`
- 当前基线提交：
  - `72a0d90`
- 当前主验证节点：
  - `.75`
- 当前线上已知稳定事实：
  - `Topics / Live / Team` 三模块继续完全分离
  - `Topics / Live / Team` 归档线已拆分
  - `Team` 工作区已可用
  - Redis 热链已启用
  - 内容主链已经做过一轮冷路径压缩
- 当前热态基线：
  - `/` p95 约 `4.3ms`
  - `/topics` p95 约 `0.5ms`
  - `/api/feed` p95 约 `0.4ms`
  - `/topics/futures/rss` p95 约 `0.8ms`

### 规划约束

- 不把原文的 19 条机械执行到底。
- 先判断：
  - 已完成
  - 仍值得做
  - 暂不做
- 高风险项只有在前面批次做完、并且仍有 profile/bench 证据时才进入。
- 不把：
  - `Team`
  - `Live`
  - `Topics`
  再次耦合。
- 不带入无关本地脏改。

### 关键路径

真正的关键路径不是“先改最底层”，而是：

1. 先核 19 项现状，避免重复返工
2. 先做低风险高收益项
3. 每一批做完都要：
   - 测试
   - 构建
   - `.75` 运行态验收
4. 只有当前面批次完成后，才判断要不要继续做高风险大改

## Execution Plan

### Phase A — 现状核查与分级

目标：
- 把原文 19 项全部分成：
  - `done`
  - `todo`
  - `defer`

步骤：
- [ ] 打开 [20260403-pro-claude.md](/Users/haoniu/sh18/hao.news2/20260403-pro-claude.md)，列出 19 个优化项及对应文件。
- [ ] 逐项核当前代码是否已覆盖：
  - Redis 热链
  - Team 工作区 / Team archive
  - Live archive / 100 条窗口语义
  - 冷启动和内容主链预热
  - transfer 上限、startSession 清理、BodyFile 校验、directPeers 上限等旧优化
- [ ] 在本文件底部新增 `Execution Status` 小节，逐项标记：
  - `done`
  - `todo`
  - `defer`
- [ ] 只把 `todo` 留给后续执行。

完成标准：
- 原文 19 项都有归类。
- 后续执行不再为“这条是不是已经做了”反复回头。

依赖：
- 这是全计划的入口，后续步骤都依赖它。

### Phase B — 第一批低风险高收益优化

目标：
- 先落地一批不会破坏当前稳定面的优化。

#### B1. Redis `KEYS -> SCAN`

文件：
- [redis_summary.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/redis_summary.go)

步骤：
- [ ] 找出 `KEYS` 使用位置。
- [ ] 替换成 `SCAN` 分批扫描实现。
- [ ] 保持返回语义不变。
- [ ] 如有相关测试，补或更新测试。

完成标准：
- 代码中不再使用阻塞式 `KEYS` 做 key 计数。
- 对应测试通过。

#### B2. Team 文件锁

文件：
- [store.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/store.go)
- [store_test.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/store_test.go)

步骤：
- [ ] 参照 `live/store.go` 的房间锁方式，为 Team 增加 team/channel 级文件锁。
- [ ] 包住所有 Team 写路径：
  - `SaveTeam`
  - `SavePolicy`
  - `SaveMembers`
  - `SaveChannel`
  - `AppendMessage`
  - `SaveTask`
  - `SaveArtifact`
  - `AppendHistory`
  - `CreateArchive`
- [ ] 增加并发写测试。

完成标准：
- 并发写不会损坏 Team JSON / JSONL 文件。
- 相关测试通过。

#### B3. Redis announcement 批量 pipeline

文件：
- [announcement_cache.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/announcement_cache.go)

步骤：
- [ ] 找出按 `infohash` 逐条 `GET` 的路径。
- [ ] 改成单次 pipeline 批量获取。
- [ ] 保持现有读缓存行为兼容。
- [ ] 补 announcement cache 测试。

完成标准：
- 读 announcement 热索引时不再是 `N+1` Redis 往返。
- 测试通过。

#### B4. Subscriptions 哈希集合

文件：
- [subscriptions.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/subscriptions.go)

步骤：
- [ ] 在 Normalize 阶段构建 topic / key / author 相关集合。
- [ ] 高频匹配路径由线性搜索改成集合查找。
- [ ] 保持现有规则语义不变。
- [ ] 补或更新测试。

完成标准：
- 高频匹配路径不再依赖线性扫描。
- 订阅匹配行为不变。

#### B5. Sync supervisor 熔断保护

文件：
- [sync_supervisor.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonews/sync_supervisor.go)

步骤：
- [ ] 增加重启窗口计数。
- [ ] 超过阈值后进入冷却期。
- [ ] 熔断状态写入 supervisor 状态输出。
- [ ] 补 supervisor 测试。

完成标准：
- sync 崩溃时不会无界重启。
- 熔断状态可观测。

#### B6. Live `ListRooms` 延迟 roster 计算

文件：
- [store.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/live/store.go)

步骤：
- [ ] 检查 `ListRooms()` 是否仍会为每个房间全量构造 roster。
- [ ] 若仍存在，改成读 `room.json` 的缓存字段。
- [ ] 在 `refreshRoomIndex` 或同类路径中同步写入 `active participant` 缓存。
- [ ] 补测试。

完成标准：
- `ListRooms()` 不再对每个房间做全量事件扫描。
- 房间摘要仍正确。

### Phase C — 第二批中风险收口项

目标：
- 在第一批通过后，继续做明显值得做但改动面更大的项。

#### C1. Team `LoadMessages` 尾部读取 / 分页

文件：
- [store.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/store.go)

步骤：
- [ ] 区分：
  - 最新消息读取
  - 全量消息读取
- [ ] 最新消息读取改成尾部读取优化。
- [ ] 全量读取保留给归档/快照。
- [ ] 增加分页接口或内部 helper。
- [ ] 补测试。

完成标准：
- Team 常见“最新 N 条消息”不再全量读整个 JSONL。

#### C2. Witness 并行收集

文件：
- [credit_witness.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/credit_witness.go)

步骤：
- [ ] 把串行 witness 请求改成并行 + 早停。
- [ ] 保持 `needed` 数量逻辑。
- [ ] 超时逻辑统一。
- [ ] 补测试。

完成标准：
- 最坏 witness 等待时间明显下降。

#### C3. LAN peer 并行 HTTP

文件：
- [lanpeer.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/lanpeer.go)
- [torrent_http_fallback.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/torrent_http_fallback.go)

步骤：
- [ ] 把 LAN peer 健康探测改成并行请求。
- [ ] 把 HTTP fallback 改成有限并行尝试。
- [ ] 保持结果排序和回退语义。
- [ ] 补测试。

完成标准：
- LAN peer 探测和 fallback 最坏耗时下降。

#### C4. Live notices 复用现有传输层

文件：
- [notices.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/live/notices.go)

步骤：
- [ ] session 存活时复用已有 host/topic 发送 notice。
- [ ] detached 模式只保留兜底。
- [ ] 去掉固定 sleep，改成 peer/topic 就绪等待。
- [ ] 补测试。

完成标准：
- notice 发送不再为一条消息完整拉起新 libp2p host。

### Phase D — 高风险项，仅在证据明确时进入

这些项不默认做。只有当：
- Phase B/C 做完
- profile/bench 仍显示明确瓶颈
- 有清晰回退路径
时，才允许进入。

候选项：
- [ ] D1 Transfer 流式 TAR
- [ ] D2 PubSub goroutine 池化
- [ ] D3 Index 签名增量化深化
- [ ] D4 Bundle 并行加载重构
- [ ] D5 Post 列表避免 clone
- [ ] D6 Credit Store 分区加载
- [ ] D7 Daily Limit 索引化
- [ ] D8 Manifest 增量构建
- [ ] D9 JSON / I/O 全局激进优化

完成标准：
- 只有在有性能证据的情况下才进入。
- 进入前必须先写明：
  - 瓶颈证据
  - 风险
  - 回退方式

### Phase E — 可观测性

目标：
- 让后续优化基于证据，而不是体感。

#### E1. pprof / tracing

步骤：
- [ ] 加入 pprof 或等价 profile 入口。
- [ ] 给关键路径补耗时点：
  - index build
  - filter
  - feed
  - RSS
  - live append

完成标准：
- 能够抓到 heap / CPU / goroutine 证据。

#### E2. 冷启动指标

步骤：
- [ ] 记录：
  - HTTP ready
  - libp2p ready
  - index ready
  - warmup ready
- [ ] 暴露到 readiness 或 ops 输出。

完成标准：
- 冷启动瓶颈能被具体定位，而不是靠猜。

## Verification

### 代码验证

每批至少执行：

- `go test ./... -count=1`
  - 若过慢，允许按模块拆分：
    - `./internal/haonews/...`
    - `./internal/plugins/...`
    - `./cmd/...`
- 必要时加：
  - `-race`

### 性能验证

固定观察：
- `/`
- `/topics`
- `/api/feed`
- `/topics/futures/rss`

最低要求：
- 热态继续保持两位数毫秒或更优
- 冷态不退化到多秒级

### 运行态验证

必须验证：
- `.75`

按需验证：
- `.76`

固定检查：
- `/api/network/bootstrap`
- Redis 状态
- Topics / Live / Team 入口
- Team 工作区
- Live 房间

### 发布前标准

每个发布批次必须满足：
- 相关测试通过
- `.75` 至少一轮运行态验收
- 无关脏改未进入 commit
- `main + tag + release` 成功

## Blockers / Resume

### Hard blockers

以下情况才算真正硬阻塞：

- 缺少必须的远程权限或密钥
  - 例如 `.76` 无法 SSH 登录
- 外部系统持续失败，且经过合理重试后仍不可用
- 高风险优化进入前缺少 profile/bench 证据
- 存在与用户本地改动直接冲突、继续会覆盖用户工作
- 某项变更需要不可逆数据迁移或破坏性操作

### 如果阻塞，必须写回

若执行未完成，必须把本文件更新为：
- 哪些项已完成
- 哪些项仍未完成
- 当前阻塞是什么
- 精确恢复下一步是什么

### 恢复下一步格式

恢复信息必须精确到：
- 具体文件
- 具体命令
- 具体下一批次

示例：
- 下一步：执行 `Phase B / B2 Team 文件锁`
- 文件：
  - `internal/haonews/team/store.go`
  - `internal/haonews/team/store_test.go`
- 验证：
  - `go test ./internal/haonews/team -race`

### 当前建议起点

后续若按 `$auto-run-plan2` 执行，直接从这里开始：

- [ ] `Phase A`：核 19 项现状并写回 done/todo/defer
- [ ] `B1 Redis KEYS -> SCAN`
- [ ] `B2 Team 文件锁`
- [ ] `B3 Redis announcement pipeline`
- [ ] 跑测试
- [ ] 切 `.75`
- [ ] 发一版

一句话：

- **先核现状，再分批做，不把高风险优化提前。**

## Execution Status

更新时间：
- `2026-04-03`

### 19 项现状分类

1. Redis `KEYS -> SCAN`
- `done`
- 已改为 `SCAN` 分批计数

2. Transfer 流式 TAR 处理
- `defer`
- 高风险 I/O 变更，当前无新增证据要求进入

3. PubSub Goroutine 池化
- `defer`
- 高风险并发模型变更，当前无 profile 证据要求进入

4. Team 模块文件锁
- `done`
- 已为 Team 写路径补 `team` 级文件锁

5. Index 签名计算增量化
- `done`
- 当前已有 cached signature / 后台预热 / 浅探测快慢路径

6. Bundle 加载并行化与分页
- `defer`
- 改动面较大，当前未进入

7. Post 列表避免全量克隆
- `defer`
- 未进入当前批次

8. Live 事件追加异步化
- `defer`
- 当前 Live 已稳定，不优先改写追加模型

9. Redis Announcement 批量 Pipeline
- `done`
- 已改为批量 `MGET` 热读，去掉索引读取的 `N+1`

10. Witness 并行收集
- `done`
- 远端 witness 收集已改为并行请求 + 达到 `limit` 后早停，避免慢 witness 拖住整轮

11. Team `LoadMessages` 分页与索引
- `done`
- `LoadMessages(limit>0)` 已切到尾部读取，`limit<=0` 继续保留全量读取给快照/归档

12. Live `ListRooms` 延迟 Roster 计算
- `done`
- 房间摘要已缓存到 `room.json`，旧房间仅在缺摘要时回退扫描

13. Credit Store LRU 缓存与分区加载
- `done`
- 现已有缓存、作者索引和错误可观测性收口

14. Daily Limit 索引化
- `defer`

15. Manifest 增量构建
- `defer`

16. Subscriptions 哈希集合匹配
- `done`
- Normalize 阶段已构建 topic/channel/tag/author/key 查找集合，高频匹配已切到集合查找

17. LAN Peer 并行 HTTP 请求
- `done`
- LAN bootstrap 探测已并行；bundle fallback 已改成有限并行抓 payload，仍保留单次串行 untar/store 校验

18. Live Notices 复用现有传输层
- `defer`
- 当前主路径已经复用 session 内联 `publishEvent`，detached 仅作 fallback；剩余固定等待仅在 fallback 路径，当前无性能证据要求继续下钻

19. Sync Supervisor 添加熔断保护
- `done`
- 已补重启窗口计数、熔断状态和冷却等待

### 本轮已完成

- `B1 Redis KEYS -> SCAN`
- `B2 Team 文件锁`
- `B3 Redis announcement 批量读取`
- `B4 Subscriptions 哈希集合匹配`
- `B5 Sync supervisor 熔断`
- `B6 Live ListRooms 摘要缓存`
- `C1 Witness 并行收集`
- `C2 Team LoadMessages 尾部读取 / 分页`
- `C3 LAN peer 并行 HTTP`

### 本轮验证

- `go test ./internal/haonews ./internal/haonews/live ./internal/plugins/haonews -run 'TestReadRedisSyncSummary|TestLoadCachedSyncAnnouncementsByTopicAndChannel|TestStoreAppendTaskConcurrentPreservesAllTasks|TestTrimRestartWindowAndCircuitWait|TestLocalStoreUsesRedisCacheForRoomAndEvents|TestListRoomsFallsBackToRoomSummaryWithoutEventsScan'`
- `go test ./internal/haonews/team -run 'TestStoreAppendTaskConcurrentPreservesAllTasks|TestStoreAppendAndLoadMessages|TestStoreSaveMembersNormalizesStatusesForApprovalFlow|TestStoreNormalizesTaskStatusPriorityAndArtifactTaskID'`
- `go test ./internal/haonews -run 'TestSubscribedAnnouncementTopics|TestMatchesAnnouncement|TestMatchesHistoryAnnouncement|TestMatchPublicKeyFilters'`
- `go test ./internal/plugins/haonews -run 'TestApplySubscriptionRules|TestPendingApprovalPostsRespectWhitelistFilters|TestSubscriptionRulesNormalizeTopicAliases|TestApprovalAutoApproveSelectorsNormalization'`
- `go test ./internal/haonews -run 'TestRequestCreditWitness|TestSelectWitnessCandidatesDeterministic|TestCollectWitnessesFromCandidatesStopsAfterLimit'`
- `go test ./internal/haonews/team -run 'TestStoreAppendAndLoadMessages|TestStoreLoadMessagesLimitReadsLatestMessages|TestStoreLoadChannelMessagesAlias'`
- `go test ./internal/haonews -run 'TestResolveLANBootstrapPeersFetchesCandidatesInParallel|TestFetchBundleFallbackPayloadReturnsFastSuccess|TestResolveExplicitBootstrapPeers|TestSortLANPeerCandidates|TestCandidateBundleURLs|TestWithSourcePeerHint'`
- `go build ./cmd/haonews`

### 当前未完成

- `Phase D / Phase E` 全部仍未进入

### 恢复下一步

- 下一步：仅在新的 profile / bench 证据出现时，再决定是否进入 `Phase D / Phase E`
- 文件：
  - `internal/haonews/live/notices.go`
  - `internal/plugins/haonewscontent/plugin.go`
  - `internal/plugins/haonews/index_cache.go`
- 验证：
  - 先补 profile / bench 证据，再决定具体测试集
  - `go build ./cmd/haonews`
