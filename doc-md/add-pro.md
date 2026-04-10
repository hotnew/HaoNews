# Hao.News 性能与并发优化方案

日期：2026-03-28

这份文档专门规划 Hao.News 当前 Web 浏览层、本地节点层、同步层的性能和并发优化路线。

目标不是“过早做大而全的优化”，而是先把最容易影响真实体验的路径收紧：

- 首页加载更快
- `/topics` 和 `/topics/<topic>` 更稳
- RSS / JSON 输出更轻
- `.75` 这种本地节点冷启动更平滑
- `ai.jie.news` 这种公网节点在多人访问时不容易抖


## 1. 当前问题定义

当前系统已经完成了较多功能：

- 首页 / topic 页 / source 页
- `New / Hot`
- AJAX 局部刷新
- topic RSS
- `pending-approval`
- reviewer 管理
- libp2p + HTTP fallback

但是性能层目前还主要停留在“能跑通”，不是“专门优化过”。

当前风险主要集中在：

1. 列表页每次请求仍然会实时拼装较多数据
2. facet / summary / topic/source 统计有重复计算
3. RSS / JSON 接口仍是请求时现算
4. 首页冷启动时会叠加 libp2p / DHT / 索引读取压力
5. 并发访问还没有经过正式压测
6. AJAX 优化的是交互体验，不等于后端吞吐已经优化


## 2. 性能目标

先定可执行目标，而不是抽象地说“更快”。

### 2.1 页面目标

本地节点 `.75`：

- 首页首字节时间：
  - 目标 `< 400ms`
- `/topics`
  - 目标 `< 350ms`
- `/topics/<topic>`
  - 目标 `< 350ms`
- `/topics/<topic>/rss`
  - 目标 `< 250ms`

公网节点 `ai.jie.news`：

- 首页首字节时间：
  - 目标 `< 600ms`
- `/topics`
  - 目标 `< 500ms`
- `/api/feed`
  - 目标 `< 350ms`

### 2.2 并发目标

第一阶段并发目标不要定太大，先收实用值：

- 单节点稳定承载：
  - `20-30` 个并发浏览请求
- RSS / API 轻接口：
  - `50+` 个并发请求不出现明显超时
- 冷启动后：
  - 不因为首页访问把 `serve` 打挂


## 3. 先测量，不盲改

优化前必须先把基线量出来。

### 3.1 需要测的路径

核心页面：

- `/`
- `/?tab=hot`
- `/topics`
- `/topics?tab=hot`
- `/topics/futures`
- `/topics/futures?tab=hot`
- `/topics/futures/rss`
- `/api/feed`
- `/api/topics`
- `/api/topics/futures`

审核链页面：

- `/pending-approval`
- `/moderation/reviewers`

### 3.2 需要采集的指标

每条路径都记录：

- TTFB
- 总响应时间
- HTML / JSON / RSS 大小
- 索引构建时间
- 过滤时间
- 排序时间
- 模板渲染时间

节点级也要记录：

- 进程 RSS 内存
- CPU 峰值
- goroutine 数量
- 冷启动时间

### 3.3 需要增加的内部诊断

建议增加一个轻量 profile 输出层，只在本地或 debug 模式显示：

- `index_load_ms`
- `filter_posts_ms`
- `sort_ms`
- `facet_ms`
- `render_ms`
- `rss_ms`
- `api_ms`

最直接做法：

- 在 handler 里做阶段计时
- 打到日志
- 或写进响应头，例如：
  - `X-HaoNews-Profile`


## 4. 热路径优化优先级

### 4.1 Priority A：列表页查询缓存

这是最值钱的一层。

首页、topic 页、source 页有很多重复请求：

- 同样的 query
- 同样的 tab
- 同样的 page
- 同样的 page_size
- 同样的 window

建议对以下结果做短时缓存：

- `FilterPosts(opts)` 结果
- `PaginatePosts(...)` 结果
- `BuildSummaryStats(...)`
- facet 结果

缓存 Key 建议包含：

- index revision
- path kind
- topic / source / reviewer
- query
- tab
- sort
- window
- page
- page_size

缓存 TTL：

- 本地节点：`5s - 15s`
- 公网节点：`10s - 30s`

关键原则：

- 不是永久缓存
- 只做短缓存，提升重复浏览命中率


### 4.2 Priority A：RSS 和 JSON 输出缓存

RSS 很适合缓存，因为：

- 内容是只读输出
- 同一 topic 常被重复拉
- 很适合订阅器轮询

建议先缓存：

- `/topics/<topic>/rss`
- `/api/feed`
- `/api/topics`
- `/api/topics/<topic>`

缓存粒度：

- 按请求参数精确命中
- 和 index revision 绑定

可以先只做内存缓存。


### 4.3 Priority A：facet / 统计复用

现在很多页面会重复做：

- topic stats
- source stats
- summary stats
- reviewer stats

建议把这些从“每次页面请求现算”改成：

- 基于 index revision 的共享结果

比如：

- `TopicStatsForPosts`
- `SourceStatsForPosts`
- `ReviewerStatsForPosts`

要避免在同一请求链内反复扫描同一批 posts。


## 5. 页面层优化

### 5.1 首页和 topic 页拆成更明确的两层

现在页面已经有 AJAX。

下一步应该明确拆成：

- 外壳层
  - sidebar
  - header
  - 固定说明
- 列表层
  - facets
  - posts
  - pagination

这样可以进一步做到：

- 只刷新列表层
- 不重复重新渲染整块 hero / stats / footer

### 5.2 避免无意义的大 HTML

已经收了一部分：

- 去掉列表摘要

下一步继续看：

- 首页底部说明区是否需要懒加载


## 6. 2026-03-28 压测结论

这轮已经做了真实本地 `.75` 压测，结论比原来的“感觉慢”清楚很多：

- **热缓存稳态已经很好**
- **问题集中在 TTL 过期后的第一次重建**
- **并发防穿透已经生效**
- **现在的瓶颈不是 stampede 重复重算，而是第一次重建本身太慢**

### 6.1 热缓存结果

`/api/feed`：

- `8` 并发：`p50 1.2ms / p95 2.1ms / p99 2.4ms`
- `16` 并发：`p50 1.9ms / p95 3.6ms / p99 4.3ms`
- `32` 并发：`p50 2.2ms / p95 3.7ms / p99 4.3ms`
- `64` 并发：`p50 2.5ms / p95 4.5ms / p99 6.3ms`

`/api/topics`：

- warm-cache：`p50 9.2ms / p95 12.6ms / p99 15.4ms`

`/api/topics/futures`：

- warm-cache：`p50 19.7ms / p95 38.6ms / p99 40.3ms`

`/topics/futures/rss`：

- warm-cache：`p50 21.0ms / p95 34.8ms / p99 38.8ms`

HTML：

- `/` warm-cache：`p50 6.7ms / p95 14.5ms / p99 20.3ms`
- `/topics` warm-cache：`p50 4.7ms / p95 9.5ms / p99 13.7ms`
- `/topics/futures` warm-cache：`p50 6.7ms / p95 13.6ms / p99 17.0ms`
- AJAX 片段请求：`p50 14.3ms / p95 38.9ms / p99 42.5ms`

### 6.2 TTL 过期后的长尾

这部分才是现在真正的性能问题。

`/api/feed`：

- `TTL-expiry + 8` 并发：`p95 456.4ms`
- `TTL-expiry + 16` 并发：`p95 994.9ms`
- `TTL-expiry + 32` 并发：`p95 1970.8ms`
- `TTL-expiry + 64` 并发：`p50 3646.6ms / p95 3651.9ms / p99 3818.2ms`

`/api/topics`：

- `TTL-expiry + 32` 并发：`p50 1629.8ms / p95 1651.9ms / p99 1797.8ms`

`/api/topics/futures`：

- `TTL-expiry + 32` 并发：`p50 3770.9ms / p95 3892.9ms / p99 4038.6ms`

`/topics/futures/rss`：

- `TTL-expiry + 32` 并发：`p50 1910.0ms / p95 1938.1ms / p99 1938.5ms`

HTML `probe` 过期后：

- `/`：`p95 ~1986ms`
- `/topics/futures`：`p95 ~1987ms`

### 6.3 冷启动 readiness

`.75` 本地节点重启到稳定可访问，大致落在：

- `31s ~ 39s`

并且：

- `/`
- `/topics`
- `/api/feed`

没有明显谁先起来，基本是整机 readiness 一起变 `200`。

### 6.4 当前判断

当前状态可以明确成：

1. 稳态性能已经够好
2. 当前长尾主要来自：
   - `index` 过期后的重建
   - `TTL` 过期后的第一次列表/API 重建
3. 防穿透已经起作用
4. 最值钱的下一步不是再加新的 HTTP cache header
5. 最值钱的是继续压：
   - `FilterPosts`
   - `BuildTopicDirectory`
   - `PaginatePosts`
   - `BuildSummaryStats`
   - `MarshalJSONBytes`
   - `TopicRSSBytes`


## 7. 下一步实现顺序

### P6：重建路径 profiling

对这些 handler 增加阶段计时：

- `/api/feed`
- `/api/topics`
- `/api/topics/<topic>`
- `/topics/<topic>/rss`
- `/`
- `/topics/<topic>`

阶段至少拆成：

- `index_ms`
- `filter_ms`
- `facet_ms`
- `paginate_ms`
- `summary_ms`
- `render_or_marshal_ms`

优先确认到底是：

- `Index()`
- 还是 `FilterPosts(opts)`
- 还是 topic/source facet 聚合
- 还是 JSON/RSS 输出

### P7：Filter 结果缓存

在 index revision 基础上，加一层短 TTL 的过滤结果缓存：

- `FilterPosts(opts)` 结果
- topic / source / reviewer 这些列表级结果

先只做最重的几条：

- `/api/feed`
- `/api/topics`
- `/api/topics/<topic>`

### P8：目录聚合缓存

给这些结果做共享聚合：

- `BuildTopicDirectory`
- `TopicStatsForPosts`
- `SourceStatsForPosts`
- `ReviewerStatsForPosts`
- `BuildSummaryStats`

避免同一条请求链里扫描同一批 posts 多次。

### P9：冷启动收尾

如果后面还要继续收冷启动，再单独做：

- readiness 标志
- 启动预热顺序
- sync 更晚启动
- index 预热和首屏 readiness 解耦
- subscription summary 是否要折叠
- network summary 是否要简化

目标是：

- 首页 HTML 控制在合理体积
- AJAX 返回更轻


### 5.3 分页策略再收紧

默认页大小目前还是偏宽松。

建议默认：

- Web 默认：`20`
- API 默认：`20`
- RSS 默认：`20` 或 `30`

并对极端 `page_size` 做上限：

- Web：`100`
- API：`100`
- RSS：`50`


## 6. 排序算法优化

### 6.1 `Hot` 提前计算

现在 `Hot` 逻辑已经有：

- `hot_score`
- `is_hot_candidate`

下一步要避免每次请求再重新算复杂逻辑。

建议：

- 在 index 构建阶段就把：
  - `VoteScore`
  - `CommentCount`
  - `HotScore`
  - `IsHotCandidate`
  计算好

请求阶段只做：

- window 过滤
- sort by precomputed fields

### 6.2 排序统一比较器

把：

- newest
- discussed
- vote score
- hot

统一成稳定排序器，避免页面间排序结果不一致。


## 7. 并发与锁策略

### 7.1 Index 读多写少结构化

当前系统天然适合：

- 后台构建新 index
- 前台读旧 index
- 一次性 swap

目标是做到：

- 读请求无锁或极轻锁
- 后台刷新索引不阻塞前台浏览

建议：

- 用 immutable snapshot 思路
- 后台构建完后原子替换当前 index 指针

### 7.2 缓存层要避免锁放大

缓存不能变成新的瓶颈。

建议：

- 读路径优先 `RWMutex`
- 小对象缓存直接 map + revision key
- 热门路径可以考虑 `sync.Map`，但不要乱上

### 7.3 避免 handler 内重复 I/O

任何页面 handler 都不要：

- 多次读磁盘
- 多次读 subscriptions / writer policy / decisions

这些配置类内容应该：

- 统一缓存
- 文件变更时再刷新


## 8. 冷启动优化

`.75` 当前最明显问题之一不是 steady-state，而是冷启动偏重。

### 8.1 启动路径拆分

启动时应先保证：

- HTTP 先起来
- 首页先可访问

再后台做：

- DHT bootstrap
- relay reservation refresh
- 重索引

### 8.2 非关键任务延后

非关键任务全部推迟到后台：

- 大范围网络检查
- 历史队列扫描
- reviewer 统计预热

### 8.3 首页首次访问兜底

首页在冷启动阶段应允许：

- 先返回最近缓存快照
- 再异步更新

而不是每次都卡在“所有东西都准备好”之后。


## 9. API / RSS / 浏览器缓存

### 9.1 HTTP 缓存头

给轻接口增加：

- `ETag`
- `Last-Modified`
- 短期 `Cache-Control`

优先路径：

- `/topics/<topic>/rss`
- `/api/feed`
- `/api/topics`
- `/api/topics/<topic>`

### 9.2 浏览器端 AJAX 去抖

当前 AJAX 已经有：

- 同路径局部刷新

下一步建议：

- 多次快速点筛选时，只保留最后一次请求
- 输入搜索框时做轻 debounce

### 9.3 自动刷新更温和

首页自动刷新已经恢复，但后面还要优化：

- 页面不可见时停掉
- 用户正在搜索输入时暂停
- 页面刚刚手动切换后延迟一轮


## 10. 压测计划

### 10.1 第一轮压测对象

本地节点 `.75`：

- `/`
- `/topics`
- `/topics/futures`
- `/topics/futures/rss`
- `/api/feed`

公网节点 `ai.jie.news`：

- `/`
- `/api/feed`
- `/topics/world`
- `/topics/world/rss`

### 10.2 压测场景

场景 A：静态并发浏览

- 10 并发
- 20 并发
- 30 并发

场景 B：混合访问

- 首页 40%
- topic 页 30%
- RSS 20%
- API 10%

场景 C：冷启动后访问

- 刚重启服务
- 立刻压首页和 topic

### 10.3 压测输出

每轮都记录：

- p50
- p95
- p99
- 错误率
- CPU
- 内存


## 11. 具体实施顺序

### Phase P1：测量与基线

- 增加 handler 分阶段计时
- 记录首页 / topic / RSS / API 基线
- 确认 `.75` 和 `ai.jie.news` 当前慢点

### Phase P2：列表和 facet 缓存

- 给首页 / topic / source / pending 做查询缓存
- 给 facet 和 summary 做 revision 级缓存

### Phase P3：RSS / API 缓存与 HTTP cache 头

- topic RSS
- `/api/feed`
- `/api/topics`
- `/api/topics/<topic>`

### Phase P4：冷启动和后台任务拆分

- HTTP 先起
- DHT / sync / network 检查后置
- 冷启动页面先可读

### Phase P5：正式压测和调优

- 本地压测
- 公网压测
- 根据 p95 / p99 再收最后一轮


## 12. 当前最值得先做的 3 件事

如果只先做最值钱的三件：

1. 首页 / topic 查询结果短时缓存
2. topic RSS / API 输出缓存
3. 冷启动路径拆分，保证 HTTP 先可访问

这三件会直接改善：

- 浏览速度
- 自动刷新体感
- 订阅器轮询压力
- 本地节点冷启动卡顿


## 13. 一句话结论

现在 Hao.News 的功能链已经比较完整，下一阶段不该再无节制加功能，而是应该把性能层做成：

- 有基线
- 有缓存
- 有冷启动策略
- 有并发指标

目标不是“追求极限 benchmark”，而是先把：

- 本地节点稳定浏览
- 公网节点稳定多人访问
- RSS/API 可持续轮询

这三件事做稳。


## 14. 最小可用本地压测脚本

仓库里已经放了一个不依赖第三方包的最小压测脚本：

- [scripts/bench_hao_news.py](/Users/haoniu/sh18/hao.news2/haonews/scripts/bench_hao_news.py)

它默认覆盖这 5 个入口：

- `/`
- `/topics`
- `/topics/futures`
- `/topics/futures/rss`
- `/api/feed`

### 14.1 直接运行

```bash
python3 scripts/bench_hao_news.py
```

默认目标地址是：

```text
http://127.0.0.1:51818
```

如果你的本地节点不是这个地址，可以显式指定：

```bash
python3 scripts/bench_hao_news.py --base-url http://127.0.0.1:51818
```

### 14.2 推荐压测命令

先跑一个低强度基线：

```bash
python3 scripts/bench_hao_news.py \
  --base-url http://127.0.0.1:51818 \
  --concurrency 4 \
  --requests-per-path 20 \
  --warmup-per-path 1
```

再跑一个中等强度版本：

```bash
python3 scripts/bench_hao_news.py \
  --base-url http://127.0.0.1:51818 \
  --concurrency 20 \
  --requests-per-path 50 \
  --warmup-per-path 1
```

如果要采集机器可读结果：

```bash
python3 scripts/bench_hao_news.py --json > bench.json
```

### 14.3 指标解释

脚本会按路径输出：

- `REQ`
  - 总请求数
- `OK`
  - 成功请求数
- `ERR%`
  - 错误率
- `P50(ms)`
  - 中位延迟
- `P95(ms)`
  - 95 分位延迟
- `P99(ms)`
  - 99 分位延迟
- `AVG(ms)`
  - 平均延迟

其中：

- `p50 / p95 / p99` 用的是**nearest-rank** 算法
- 错误率 = `失败请求数 / 总请求数`
- `HTTP 2xx/3xx` 计为成功
- `4xx/5xx` 和网络错误计为失败

### 14.4 建议如何看结果

第一轮先只看这几个值：

- 首页 `/` 的 `p95`
- `/topics` 的 `p95`
- `/topics/futures` 的 `p95`
- `/topics/futures/rss` 的 `p95`
- `/api/feed` 的 `p95`
- 各路径 `ERR%`

如果：

- `p95` 很高
- `ERR%` 不是 0
- `RSS / API` 明显比 HTML 慢

那就先查：

- 首页 / topic 查询缓存
- RSS / API 输出缓存
- 冷启动路径
- DHT / sync 是否干扰了前台响应


## 15. 2026-03-28 第二轮收敛结果

这一轮继续做了两件事：

- `FilterPosts` 和 `TopicDirectory` 短 TTL 结果缓存
- 首页、`/topics`、`/topics/<topic>` 的 HTML `full/fragment` 响应缓存

同时：

- HTML AJAX fragment 路径不再白算 `NodeStatus`
- HTML 响应缓存命中时，不再先走 `App.Index()` 重建链

### 15.1 结果

之前最明显的问题是：

- warm-cache 很快
- 但 TTL 过期后的第一波并发，首页和 topic HTML 还会出现秒级等待

这轮本机 `.75` 的实测结果是：

#### 首页 `/`

先请求一次，再等待 `6s` 让 HTML TTL 过期，然后执行：

```bash
ab -n 60 -c 32 http://127.0.0.1:51818/
```

结果：

- `p50 ≈ 13ms`
- `p95 ≈ 23ms`
- `p99 ≈ 201ms`

#### 单个 topic `/topics/futures`

先请求一次，再等待 `6s`，然后执行：

```bash
ab -n 60 -c 32 http://127.0.0.1:51818/topics/futures
```

结果：

- `p50 ≈ 6ms`
- `p95 ≈ 8ms`
- `p99 ≈ 201ms`

### 15.2 和前一轮对比

这轮的意义不是把 warm-cache 再压快，而是把：

- 首页 HTML
- topic HTML

从“TTL 一过就会接近秒级排队”

压到了：

- 主体请求已经回到十几毫秒内
- 只剩极少数单次重建请求还会落到约 `200ms`

也就是说：

- **HTML 这条最大的秒级长尾已经被打掉了**

### 15.3 当前剩余尾巴

现在还没完全收掉的，主要剩：

- `/api/feed` TTL 过期后的高并发重建
- `/api/topics/<topic>` TTL 过期后的高并发重建
- 冷启动 readiness 空窗

所以后面最值得继续的优化顺序变成：

1. API/topic feed 的更细粒度结果缓存
2. 目录/API 聚合结果缓存继续下沉
3. 冷启动 readiness 收尾


## 16. 2026-03-28 第三轮收敛结果

这轮继续做了两件关键事：

- 首页、`/topics`、`/topics/<topic>` 接入 HTML `full/fragment` 响应缓存
- 响应缓存改成真正的后台 `stale-while-revalidate`
  - TTL 过期后的**第一个请求也先吃 stale**
  - 后台再做重建

### 16.1 HTML 收敛结果

`.75` 本机实测：

#### 首页 `/`

```bash
curl -fsS http://127.0.0.1:51818/ >/dev/null
sleep 6
ab -n 60 -c 32 http://127.0.0.1:51818/
```

结果：

- `p50 ≈ 13ms`
- `p95 ≈ 23ms`
- `p99 ≈ 201ms`

#### Topic `/topics/futures`

```bash
curl -fsS http://127.0.0.1:51818/topics/futures >/dev/null
sleep 6
ab -n 60 -c 32 http://127.0.0.1:51818/topics/futures
```

结果：

- `p50 ≈ 6ms`
- `p95 ≈ 8ms`
- `p99 ≈ 201ms`

结论：

- HTML 页面对 TTL 过期的秒级长尾已经基本打掉

### 16.2 API 收敛结果

#### `/api/feed`

```bash
curl -fsS http://127.0.0.1:51818/api/feed >/dev/null
sleep 6
ab -n 60 -c 32 http://127.0.0.1:51818/api/feed
```

结果：

- `p50 ≈ 7ms`
- `p95 ≈ 11ms`
- `p99 ≈ 12ms`

#### `/api/topics/futures`

```bash
curl -fsS http://127.0.0.1:51818/api/topics/futures >/dev/null
sleep 6
ab -n 60 -c 32 http://127.0.0.1:51818/api/topics/futures
```

结果：

- `p50 ≈ 16ms`
- `p95 ≈ 18ms`
- `p99 ≈ 20ms`

结论：

- `/api/feed`
- `/api/topics/<topic>`

这两条 TTL 过期下的高并发长尾，现在也已经从秒级回到十几毫秒级。

### 16.3 当前判断

到这里：

- **稳态已经很快**
- **TTL 过期后的第一波并发也已经基本压平**

所以当前最大剩余项已经不再是浏览链，而是：

- 冷启动 readiness
- 更深一层的聚合缓存打磨

如果不继续追极限性能，当前浏览与 API 并发体验已经进入“可收尾”阶段。
