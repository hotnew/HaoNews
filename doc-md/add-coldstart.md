# Hao.News 冷启动 Readiness 优化计划

日期：2026-03-28

目标不是继续把稳态请求压到更低，而是收掉：

- `launchctl kickstart` 之后到首个 `200` 之间的空窗
- 首屏第一次命中时被同步预热链拖慢
- restart 后“服务像挂住一样”的假故障感


## 1. 当前判断

这轮代码和压测看下来，冷启动空窗主要不是稳定期吞吐问题，而是启动时序问题：

1. `content` 插件在 `Build()` 阶段同步执行：
   - `app.Index()`
   - `app.NodeStatus(index)`
2. 这两步发生在 HTTP 服务真正开始稳定接流量之前。
3. `NodeStatus()` 又会继续读：
   - sync runtime status
   - network bootstrap config
4. managed `sync` 虽然已经有初始延后，但 `serve` 这边自己的预热仍然是同步的。

结论：

- **当前冷启动主要是“同步预热阻塞 readiness”**
- 不是页面 handler 本身慢
- 也不是 steady-state 缓存失效问题


## 2. 优化目标

### 2.1 用户侧目标

- restart 后首页尽快返回 `200`
- restart 后 `/topics`、`/api/feed` 不再长时间 `000/timeout`
- 首次请求允许内容逐步变完整，但不能先整体不可用

### 2.2 工程目标

- 所有预热都改成后台进行
- HTTP 监听不再等待 index/status 完成
- NodeStatus 首次构建尽量退化成轻路径


## 3. 实施阶段

### Phase C1：移除阻塞式同步预热

做法：

- 把 `content` 插件里的：
  - `app.Index()`
  - `app.NodeStatus(index)`
  从 `Build()` 同步路径改成后台 goroutine

收益：

- `serve` 可以先起来
- 首个 `200` 不再被插件初始化卡住


### Phase C2：把 NodeStatus 首次构建再收轻

做法：

- 首次页面请求时，如果没有 sync runtime 文件，直接返回轻量 offline/starting 状态
- 不在首个请求里做多余的磁盘/网络侧细节构建

收益：

- 首页、归档、topic 等页第一次渲染更稳定


### Phase C3：显式 readiness 标志

做法：

- 增加内部 ready 状态：
  - `http listening`
  - `index warmed`
  - `sync observed`
- `/api/network/bootstrap` 或内部状态页可显示当前阶段

收益：

- restart 后更容易判断是“还在预热”还是“真故障”


### Phase C4：冷启动压测与回归

需要记录：

- kickstart 后到 `/` 首次 `200`
- kickstart 后到 `/topics` 首次 `200`
- kickstart 后到 `/api/feed` 首次 `200`

目标：

- 把 restart-to-first-200 明显缩短
- 后续每次改动都能回归验证


## 4. 本轮先做什么

本轮先落最值钱的一刀：

1. 完成 `Phase C1`
2. 在 `.75` 上重启实测
3. 记录 restart-to-first-200

如果这刀收益明显，再继续做：

4. `Phase C2`
5. `Phase C3`


## 5. 本轮实测结果

### 5.1 第一轮验证：仅移除 content 同步预热，不够

实测：

- `port_open ≈ 0.26s`
- `first_200 ≈ 29.20s`

这说明：

- 端口很早就被预留
- 但 HTTP 真正开始稳定响应仍然很晚
- 问题不在 steady-state handler，而在 `host.New()` 阶段的同步建站链


### 5.2 第二轮定位：`port_open` 不等于 HTTP 已开始服务

关键结论：

- `host.New()` 一开始就 reserve 了 listener
- 所以 `socket connect` 很快成功，不代表 `ListenAndServe()` 已开始处理请求
- 真正影响首个 `200` 的，是 plugin `Build()` 链上的同步重活


### 5.3 本轮落地的两刀

1. 首页、`/topics`、`/topics/<topic>`、`/api/feed`、`/api/topics`、`/api/topics/<topic>`
   - 在冷启动窗口内，如果索引还未 ready：
   - 直接返回可自动刷新的轻量 shell / starting JSON

2. `haonewslive` 的 `AnnouncementWatcher`
   - 从同步 `Build()` 路径改成后台启动
   - 避免 libp2p transport / bootstrap connect 阻塞 `host.New()`


### 5.4 第一轮 `.75` 冷启动结果

实测：

- `port_open ≈ 1.08s`
- `home_starting ≈ 1.08s`
- `api_starting ≈ 1.08s`
- `home_full ≈ 2.54s`
- `api_full ≈ 2.54s`

结论：

- restart 后不再有 30 秒假挂
- 首个页面和首个 API 都能在约 1 秒内返回“预热中”
- 完整首页和完整 feed 约 2.5 秒内恢复
- `/api/network/bootstrap` 现在会额外返回：
  - `readiness.stage = warming_index | ready`
  - `http_ready`
  - `index_ready`
  - `cold_starting`
  - `age_seconds`
  - 其中 bootstrap 默认直接报告自己的网络 ready 状态；只有测试强制冷启动时才会返回 `warming_index`


### 5.5 最新 `.75` 受控重启结果

在把 bootstrap readiness 语义从“内容索引是否 warm”改成“网络 bootstrap 自己是否 ready”之后，再做一次受控 `kickstart` 实测：

- `port_open ≈ 0.23s`
- `home_starting ≈ 0.23s`
- `home_full ≈ 1.28s`
- `api_starting ≈ 0.23s`
- `api_full ≈ 1.28s`
- `bootstrap_ready ≈ 0.23s`

结论：

- 现在 restart 后，端口、首页轻量壳、API 轻量壳、bootstrap readiness 基本同时可见
- 完整首页和完整 feed 已经缩到约 `1.3s`
- 冷启动主问题已经从“几十秒假挂”收敛到“约 1 秒内可见，约 1-2 秒内恢复完整内容”


## 6. 下一步

冷启动主问题已经基本收掉。下一步更像收尾，而不是抢救：

1. 给 `/topics/<topic>` 的完整恢复时间单独再量一轮
2. 如果公网节点也需要，复制同样策略到 `ai.jie.news`
3. 可选：增加显式 readiness 字段，区分
   - `listener_ready`
   - `index_ready`
   - `live_ready`
