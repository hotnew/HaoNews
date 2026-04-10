# `20260403-pro-claude-codex-task.auto-run.md`

## 1. 目标

把 [20260403-pro-claude.md](/Users/haoniu/sh18/hao.news2/20260403-pro-claude.md) 中提出的性能改良方案，重构成一份 **可持续执行、可分阶段落地、可验证、可发布** 的 `haonews` 自动执行 runbook。

这份 runbook 的目标不是照单全收，而是：

1. 先核实现状，避免重复做已经完成的优化
2. 按风险和收益重排顺序
3. 优先落低风险高收益项
4. 每完成一批就验证、部署、发布
5. 只在真正遇到硬阻塞时停

## 2. 当前基线

- 当前基线提交：
  - `72a0d90`
- 当前主验证节点：
  - `.75`
- 当前线上状态：
  - `bootstrap = ready`
  - Redis 在线
  - `Topics / Live / Team` 归档线已拆分
  - `Team / Live / Topics` 模块继续完全分离
- 当前已知热态性能：
  - `/` p95 约 `4.3ms`
  - `/topics` p95 约 `0.5ms`
  - `/api/feed` p95 约 `0.4ms`
  - `/topics/futures/rss` p95 约 `0.8ms`

## 3. 执行原则

- 不把原始方案当“全做清单”，而当“优化候选池”
- 每项先判定：
  - 是否已完成
  - 是否仍然值得做
  - 是否存在更小更稳的实现路径
- 低风险高收益优先
- 高风险项必须放到后段，并在进入前重新验证必要性
- 不把：
  - `Team`
  - `Live`
  - `Topics`
  重新耦合
- 不带入无关本地脏改

## 4. 现状核查清单

在动手之前，先把原文 19 项分成三类：

### 4.1 已完成或已被当前实现覆盖

这些项必须先核代码和测试，确认无需重复开发：

1. Redis 热链和状态摘要相关
2. Team 可写治理与 Team 存储收口
3. Live 归档与显示窗口语义修正
4. 内容主链预热和冷路径第一轮压缩
5. 先前已完成的：
   - `transfer` 绝对上限
   - `startSession` 清理
   - `directPeers` 上限
   - `BodyFile` 路径校验
   - 部分 Redis 热链接入

输出要求：
- 在 runbook 中写清“已完成 / 不再做”
- 不允许重复返工

### 4.2 仍值得做的低风险项

这些项优先进入自动执行主线：

1. Redis `KEYS -> SCAN`
2. Team 文件锁
3. Redis announcement 批量 pipeline
4. Subscriptions 哈希集合匹配
5. Sync supervisor 熔断保护
6. Live `ListRooms` 延迟 roster 计算 / 缓存
7. Team `LoadMessages` 分页 / 尾部读取
8. LAN peer 并行 HTTP
9. Witness 并行收集
10. Team / archive / runtime 的可观测性补充

### 4.3 高风险或需要二次判断的项

这些项不直接上，必须在前面阶段完成后重评是否仍有必要：

1. Transfer TAR 流式重构
2. PubSub goroutine 池化
3. Index 签名增量化的大改
4. Bundle 并行加载重构
5. Post 列表 clone 语义重构
6. Credit Store 分区加载
7. Daily Limit 索引缓存
8. Manifest 增量构建
9. JSON 序列化全局替换
10. mmap / sync.Pool 级别 I/O 激进优化

这些项只有在：
- 热态指标回退
- 冷态仍明显长尾
- profile 证据明确
时才进入执行。

## 5. 分阶段执行计划

## Phase A：核实现状并写回分级结果

目标：
- 先建立真实的“已完成 / 待做 / 暂缓”台账

步骤：
1. 核查原始 19 项在当前代码中的状态
2. 为每项写：
   - `done`
   - `todo`
   - `defer`
3. 将结果写入本文件附录区

验收：
- 19 项都有归类
- 后续执行只针对 `todo`

## Phase B：P0 / P1 低风险高收益主线

### B1. Redis `KEYS -> SCAN`

文件：
- `internal/haonews/redis_summary.go`

目标：
- 消除 Redis 阻塞式 `KEYS`

验收：
- 不再使用 `KEYS`
- 对应测试通过

### B2. Team 文件锁

文件：
- `internal/haonews/team/store.go`
- `internal/haonews/team/store_test.go`

目标：
- Team 写入路径不再裸写 JSON / JSONL

范围：
- `SaveTeam`
- `SavePolicy`
- `SaveMembers`
- `SaveChannel`
- `AppendMessage`
- `SaveTask`
- `SaveArtifact`
- `AppendHistory`
- `CreateArchive`

验收：
- 并发写测试通过
- `-race` 不出现明显写竞争

### B3. Redis announcement 批量 pipeline

文件：
- `internal/haonews/announcement_cache.go`
- 测试文件

目标：
- `N+1 GET` 改成 pipeline 批量取

验收：
- 路径仍兼容
- announcement 读缓存测试通过

### B4. Subscriptions 哈希集合

文件：
- `internal/haonews/subscriptions.go`
- 测试文件

目标：
- 高频匹配路径由线性搜索改成集合查找

验收：
- 行为不变
- 归一化后集合可复用

### B5. Sync supervisor 熔断保护

文件：
- `internal/plugins/haonews/sync_supervisor.go`
- 测试文件

目标：
- 避免 sync 失控重启

验收：
- 最大重启次数窗口生效
- 熔断状态可观测

### B6. Live `ListRooms` 延迟 roster 计算

文件：
- `internal/haonews/live/store.go`
- 测试文件

目标：
- 房间列表不再为每个房间全量扫事件

验收：
- room summary 可直接读缓存字段
- `ListRooms` 性能明显稳定

## Phase C：中风险收口项

### C1. Team `LoadMessages` 分页 / 尾部读取

目标：
- Team channel 最新消息读取不再全量读全文件

验收：
- 默认最新 `N` 条读取是尾部优化路径
- 全量读取保留给归档/快照

### C2. Witness 并行收集

目标：
- 降低最坏等待时间

验收：
- 并行 + 早停逻辑通过测试

### C3. LAN peer 并行 HTTP

目标：
- 降低 peer 探测最坏耗时

验收：
- 探测结果顺序语义不破坏
- timeout 行为清晰

### C4. Live notices 复用现有传输层

目标：
- 不再为一条 notice 重新拉完整 libp2p host

验收：
- session 存活时优先复用现有 host/topic
- detached 仅作兜底

## Phase D：需要证据后再做的高风险项

只有在 Phase B/C 做完后，且 profile/bench 仍显示显著瓶颈时，才进入：

### D1. Transfer 流式 TAR
### D2. PubSub goroutine 池化
### D3. Index 签名增量化深化
### D4. Bundle 并行加载重构
### D5. Post 列表避免 clone
### D6. Credit Store 分区加载
### D7. Daily Limit 索引化
### D8. Manifest 增量构建
### D9. JSON / I/O 全局激进优化

进入条件：
- 有 profile 证据
- 有明确回退策略
- 不会破坏当前已经稳定的冷/热路径

## Phase E：可观测性

### E1. pprof 与基础 tracing

目标：
- 给后续是否继续 D 段优化提供证据

范围：
- 首页 / topics / feed / RSS
- index build
- filter
- live append

### E2. 冷启动阶段指标

目标：
- 把当前冷启动链路关键时间点暴露出来

范围：
- HTTP ready
- libp2p ready
- index ready
- background warmup ready

## 6. 验证矩阵

每一阶段至少执行：

### 6.1 代码验证

- `go test ./... -count=1`
  - 若太慢，则按模块分批：
    - `./internal/haonews/...`
    - `./internal/plugins/...`
    - `./cmd/...`
- 必要时加：
  - `-race`

### 6.2 性能验证

固定观察：
- `/`
- `/topics`
- `/api/feed`
- `/topics/futures/rss`

目标：
- 热态两位数毫秒继续保持
- 冷态不回到多秒级

### 6.3 运行态验证

节点：
- `.75` 必做
- `.76` 视通道和权限决定

固定检查：
- `/api/network/bootstrap`
- Redis 状态
- 归档入口
- Team 工作区
- Live 房间

## 7. Git / 发布策略

### 批次发布原则

每完成一组完整可验证改动就单独发版：

- `Phase B` 可拆成 2-3 个小版本
- `Phase C` 可拆成 1-2 个版本
- `Phase D` 若进入，则单项或双项单独发版

### 每个发布批次必须满足

1. 相关测试通过
2. `.75` 至少一轮运行态验收
3. 无关本地脏改不入 commit
4. GitHub：
   - `main`
   - `tag`
   - `release`
5. 必要时打本地 zip 备份

## 8. 推荐自动执行顺序

### 第一批

1. `Phase A` 全部
2. `B1 Redis KEYS -> SCAN`
3. `B2 Team 文件锁`
4. `B3 Redis announcement pipeline`

### 第二批

5. `B4 Subscriptions 哈希集合`
6. `B5 Sync supervisor 熔断`
7. `B6 Live ListRooms 延迟 roster`

### 第三批

8. `C1 Team LoadMessages 尾部读取`
9. `C2 Witness 并行收集`
10. `C3 LAN peer 并行 HTTP`
11. `C4 Live notices 复用传输`

### 第四批

12. `E1/E2 可观测性`
13. 根据证据决定是否进入 `Phase D`

## 9. 默认不做的事情

在没有新证据前，下面这些不默认进入自动执行：

- 全局替换 JSON 库
- mmap
- 全项目 I/O 重写
- 大范围缓存语义改写
- 再次把 `Team / Live / Topics` 交叉耦合

## 10. 验收标准

完成这份 runbook 的标准不是“把原文 19 条机械写完”，而是：

1. 所有低风险高收益项落地
2. 中风险项在必要范围内落地
3. 高风险项只有在有性能证据时才进入
4. 热态性能不退化
5. 冷态性能继续改善或至少不变差
6. `.75` 实际稳定运行
7. 文档、发布、备份可追溯

## 11. 执行状态写回规则

执行过程中，必须把本文件持续写回成真实状态：

- 哪些项已完成
- 哪些项延期
- 哪些项因证据不足不做
- 下一个精确批次是什么

## 12. 当前建议

如果马上进入自动执行，建议从这里开始：

1. `Phase A`：先核 19 项现状
2. 直接做：
   - `B1`
   - `B2`
   - `B3`
3. 跑测试
4. 切 `.75`
5. 发一版

一句话：

- **这份 runbook 的核心是：先核现状，再分批做，不把高风险优化提前。**
