# HaoNews 整改方案（Codex 复核版）

> 日期：2026-03-31
> 基线提交：`c4b81ff`
> 参考文档：`/Users/haoniu/sh18/docs/haonews-fix-20260331.md`
> 原则：只保留当前代码里仍然成立、且值得投入的整改项；已被现有实现覆盖的问题不再重复建设。

---

## 结论

原文档列出的 11 条问题里，按当前代码状态重新复核后可分为三类：

1. **应立即整改**
   - FIX-01 `credit_store` 全量磁盘读取无缓存
   - FIX-02 `buildIndex()` 双重 `ApplySubscriptionRules`
   - FIX-03 `currentIndexSignature()` 高频递归遍历文件树
   - FIX-04 `sync.writeStatus()` 高频落盘

2. **值得排期，但不必抢在最前**
   - FIX-05 `pubsub.Status()` 拿锁做大量拷贝
   - FIX-06 `filter_cache.clonePosts()` 大切片复制
   - FIX-09 `reconcileQueue` 最坏阻塞过长
   - FIX-11 `GetBalance()` 静默忽略错误

3. **当前不建议继续投入**
   - FIX-07 `index` 全量 IO：已被 index/filter/response cache 收敛一部分，先把前面的确定性热点打掉
   - FIX-08 `subscriptions.Normalize()` 重复调用：收益有限，排后
   - FIX-10 `lanpeer` 健康缓存：当前更多是产品监控问题，不是主性能瓶颈

---

## 当前进度

截至当前本地代码，这轮整改已经完成：

- 已完成 FIX-01
  - `CreditStore` 增加内存缓存、按日索引、按作者索引、脏标记、TTL
- 已完成 FIX-02
  - `buildIndex()` 删除第二次 `ApplySubscriptionRules(...)`
- 已完成 FIX-03
  - `currentIndexSignature()` 改成浅探测 + 周期性深探测
- 已完成 FIX-04
  - `sync.writeStatus()` 增加签名比对和节流
- 已完成 FIX-05
  - `pubsub.Status()` 改成锁内取快照、锁外复制
- 已完成 FIX-09
  - `syncRef()` 把总 timeout 拆成 `libp2p direct` / `HTTP fallback` 两段预算
- 已完成 FIX-11
  - 新增 `GetBalanceResult()` 返回错误，旧 `GetBalance()` 保持兼容 fallback

当前仍未实施的只剩：

- FIX-06
  - `filter_cache.clonePosts()` 大切片复制
  - 当前先保持原样，避免引入新的别名风险

也就是说，这份方案里真正高价值、且低风险能收的整改，当前已经基本完成。

---

## 当前复核结果

### A. 仍然成立的问题

#### A1. CreditStore 仍然是全量冷读模型

当前代码：
- [credit_store.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/credit_store.go)

现状：
- `GetAllBalances()` 直接走 `allProofs()`
- `GetBalance()` 仍然走 `GetAllBalances()`
- `GetProofsSince()` 仍然走 `allProofs()`
- `GetWitnessRoleStats()` 仍然走 `allProofs()`

这意味着：
- credit 相关查询仍然是“目录扫描 + 文件读取 + 反序列化 + 验证”的冷路径
- sync 循环和 API 一旦频繁碰 credit，IO/CPU 会被反复放大

判断：
- **P0**

整改方向：
- 给 `CreditStore` 增加内存缓存 + 脏标记 + TTL
- `SaveProof()`、archive 写入后只做脏标记
- `GetBalance()` 不再通过 `GetAllBalances()` 间接走全量冷读

---

#### A2. `buildIndex()` 仍然双重应用订阅规则

当前代码：
- [index_cache.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonews/index_cache.go)

现状：
- `ApplySubscriptionRules(...)` 在 `buildIndex()` 里被调用两次
- 两次之间只有：
  - `governanceIndex(...)`
  - `PrepareMarkdownArchive(...)`

风险：
- 额外 CPU
- 更难推理过滤链顺序
- 后续治理逻辑一旦再增加字段，容易出现“二次过滤”副作用

判断：
- **P0**

整改方向：
- 先确认 `PrepareMarkdownArchive()` 不修改订阅过滤相关字段
- 然后删除第二次 `ApplySubscriptionRules(...)`
- 增加回归测试，确保首页、topic、pending 视图结果不变

---

#### A3. `currentIndexSignature()` 仍然是高频文件树遍历

当前代码：
- [index_cache.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonews/index_cache.go)

现状：
- `indexCacheProbeInterval = 2 * time.Second`
- `currentIndexSignature()` 会对：
  - `store/data`
  - `store/torrents`
  - `subscriptions`
  - `writer policy`
  - `moderation`
  - `delegations`
  - `revocations`
  做递归签名

虽然你已经做了：
- `probeSignature / contentSignature` 分离
- 响应缓存和 filter 缓存

但这条本身仍然重：
- 目录树越大，稳态 CPU/IO 还是会被这条 probe 拖住

判断：
- **P0**

整改方向：
- 引入“文件系统版本号”思路，优先按已知变更源失效
- 把 `currentIndexSignature()` 从主动全量扫，逐步改成：
  - 写入后失效
  - 周期性轻量探测
  - 再退回慢路径

---

#### A4. `sync.writeStatus()` 频繁写盘

当前代码：
- [sync.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/sync.go)
- [status.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/status.go)

现状：
- `reconcileQueue()` 前后多次 `writeStatus(ctx)`
- sync 过程中状态变化密集时会持续写磁盘

风险：
- 本机磁盘放大
- 状态文件更新过于频繁
- 在高频同步场景里是纯管理性开销

判断：
- **P1**

整改方向：
- 增加节流：
  - 例如最短 1-2 秒才允许真正落盘一次
- 内存里继续即时更新
- 文件只保留最近稳定快照

---

### B. 值得排期，但可以放后

#### B1. `pubsub.Status()` 仍然做大对象复制

当前代码：
- [pubsub.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/pubsub.go)

问题性质：
- 主要是锁持有时间和切片复制成本
- 不是当前最致命瓶颈，但值得收

建议：
- 引入轻量快照对象
- 避免在锁内复制过多字段

---

#### B2. `clonePosts()` 复制大切片

当前代码：
- [filter_cache.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonews/filter_cache.go)

问题性质：
- 当前为了隔离缓存结果，复制 `[]Post`
- 正确性没问题，但在热门视图下会增加内存搬运

建议：
- 先 benchmark
- 如果确实显著，再改成：
  - 不可变 cache entry
  - 只复制分页窗口
  - 或复制更轻的引用结构

---

#### B3. `reconcileQueue` 最坏路径过长

当前代码：
- [sync.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/sync.go)

问题性质：
- timeout 是受控的
- 但最坏路径仍然会拉长一次 sync tick

建议：
- 分阶段 budget
- direct peer / http fallback / witness / announce 拆预算

---

#### B4. `GetBalance()` 静默忽略错误

当前代码：
- [credit_store.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/credit_store.go)

问题性质：
- 可观测性问题
- 不是性能问题

建议：
- 改成返回错误
- 或保留旧签名但至少记录告警

---

### C. 当前不建议继续投入

#### C1. `index` 全量 IO

当前代码确实仍然会：
- `WalkTorrentFiles`
- 读 bundle

但前面你已经做过：
- index cache
- filter cache
- response cache
- stale-while-revalidate

所以现在更值钱的是先打：
- signature/probe 路径
- 重复过滤

---

#### C2. `subscriptions.Normalize()` 重复调用

这是代码洁癖型优化，不是现在主热点。

---

#### C3. `lanpeer` 健康缓存

当前更多是：
- 状态页读写
- 健康探测策略

不是核心用户路径热点。

---

## 建议的整改顺序

### Phase 1：立即落地

1. `credit_store` 增加缓存与脏标记
2. 删除 `buildIndex()` 第二次 `ApplySubscriptionRules`
3. 给 `sync.writeStatus()` 加节流

### Phase 2：性能主链继续收敛

4. 重构 `currentIndexSignature()`，减少 2 秒一次全量递归
5. 给 `pubsub.Status()` 做轻量快照

### Phase 3：按压测结果决定

6. 评估 `clonePosts()` 是否值得重做
7. 拆 `reconcileQueue` timeout budget
8. `GetBalance()` 错误语义整改

---

## 我建议现在先做什么

如果只做一轮、并且希望收益最大，我建议立刻做这 3 条：

1. `credit_store` 缓存
2. `buildIndex()` 双重过滤删除
3. `sync.writeStatus()` 节流

原因：
- 改动范围有限
- 风险明确
- 性能和稳定性收益直接

---

## 不建议做的事情

- 不建议整包照抄原文档执行
- 不建议现在再大动 `LAN peer` 和 `subscriptions.Normalize()`
- 不建议把所有问题都当成同优先级

当前更适合：
- 小批次、可验证、逐项压测
