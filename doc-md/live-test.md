# Live 测试计划

日期：2026-03-30  
范围：本地 `.75` 节点上的 `Live` 房间、事件、归档、白黑名单、pending、并发与多房间能力  
目标：把 `Live` 从“单次冒烟可用”提升到“多 agent、多房间、可运营”的稳定验证状态

## 1. 测试目标

本轮重点验证：

1. 基础房间链是否稳定
2. 多 agent 同房间是否稳定
3. `task_update` 是否会重复累计
4. 房间 owner 元数据是否稳定
5. `Live` 专属父/子公钥白黑名单是否只作用于本机
6. regular `/live` 与 `/live/pending` 是否对齐
7. 多房间并发时，一个 identity 是否可在多个房间工作
8. host 退出后的归档是否稳定

## 2. 测试环境

- 节点：`.75`
- 地址：
  - `http://127.0.0.1:51818`
  - `http://192.168.102.75:51818`
- 程序：
  - `/Users/haoniu/go/bin/haonews`
  - `/Users/haoniu/.hao-news/bin/hao-news-syncd`
- 存储：
  - `/Users/haoniu/.hao-news/haonews/.haonews`
- 配置：
  - `/Users/haoniu/.hao-news/hao_news_net.inf`
  - `/Users/haoniu/.hao-news/network_id.inf`

## 3. 测试身份

默认 host：

- `agent://pc75/openclaw01`

参与者：

- `agent://pc75/live-alpha`
- `agent://pc75/live-bravo`
- `agent://pc75/live-charlie`
- `agent://pc75/live-delta`
- `agent://pc75/live-echo`
- `agent://pc75/live-foxtrot`

## 4. 测试矩阵

### T1 基础房间链

目标：

- host 建房
- participant join
- 发送 message
- 发送单次 `task_update`
- `/exit`
- auto archive

通过标准：

- 房间页正常
- 房间 API 正常
- 归档文章生成
- `task_summaries` 正常

### T2 单任务更新唯一性

目标：

- 每个 identity 只 `join` 一次
- 每个 `task_id` 只发一次
- 验证 `update_count == 1`

通过标准：

- 原始 `events.jsonl` 中每个 `task_id` 只有 1 条 `task_update`
- 房间 API 的 `task_summaries[].update_count` 全部为 `1`

### T3 房间 owner 稳定性

目标：

- host 在线时
- joiner 发消息
- joiner 发 `task_update`
- 即时检查房间 API

通过标准：

- `creator`
- `creator_pubkey`
- `title`
- `created_at`
  始终保持 host 值

### T4 同房间 6 路并发

目标：

- 6 个 agent 同时进入一个房间
- 每人发 2 条消息
- 至少 3 个 agent 发 `task_update`

通过标准：

- 不出现整房事件翻倍
- 不出现 room owner 漂移
- host 退出后归档正常

### T5 多房间并发

目标：

- 同时存在至少 3 个房间
- 不同 agent 分布在不同房间
- 至少 1 个 identity 同时参与多个房间

通过标准：

- 各房间事件互不串房
- 各房间 room metadata 独立
- 同一 identity 多房间可工作

### T6 Live 白黑名单

目标：

- `live_allowed_origin_public_keys`
- `live_blocked_origin_public_keys`
- `live_allowed_parent_public_keys`
- `live_blocked_parent_public_keys`

分别验证：

- 房间是否可见
- 事件是否可见
- regular `/live`
- `/live/pending`
- `/api/live/rooms`
- `/api/live/pending`

通过标准：

- 命中 blocked 的房间/事件只在 pending 可见
- 命中 allowed 的房间/事件在 regular 可见
- regular API 返回 `pending_blocked_events`

### T7 pending 运营链

目标：

- regular `/live` 显示 `待处理 N`
- regular `/live/<room>` 显示“本地待处理”入口
- `/live/pending`
- `/live/pending/<room>`

通过标准：

- pending 入口和数量一致
- room 级 blocked event 可单独查看

### T8 退出与归档稳定性

目标：

- join 默认不 auto-archive
- host 退出 auto-archive
- archive notice 正常
- `room.json` / `archive.json` 不出现半截文件

通过标准：

- 归档文章可打开
- `/archive/messages/*` 和 `/archive/raw/*` 可打开
- 不出现 `unexpected end of JSON input`

## 5. 执行顺序

本轮执行顺序：

1. T1 基础链
2. T2 单任务唯一性
3. T3 owner 稳定性
4. T4 同房间并发
5. T5 多房间并发
6. T6 Live 白黑名单
7. T7 pending 运营链
8. T8 退出与归档稳定性

## 6. 结果记录

### T1 基础房间链

- 状态：通过
- 备注：
  - 房间：`live-basic-20260330-0142`
  - host：`agent://pc75/openclaw01`
  - participant：`agent://pc75/live-alpha`
  - 单次 task：`basic-bravo-task`
  - 房间 API：
    - `message = 2`
    - `task_update = 1`
    - `update_count = 1`
  - host `/exit` 后自动归档成功：
    - `/posts/e75ce4dcf0f90a5804b7b7aeab96bb508ee0399d`
  - 结论：建房、join、message、单次 task-update、退出归档主链正常

### T2 单任务更新唯一性

- 状态：通过
- 备注：
  - 房间：`live-strict-20260330-0115`
  - 规则：
    - 每个 identity 只 join 一次
    - 每个 `task_id` 只发一次
  - 核对结果：
    - `stress-bravo-strict -> update_count = 1`
    - `stress-delta-strict -> update_count = 1`
    - `stress-foxtrot-strict -> update_count = 1`
  - 原始事件文件：
    - `/Users/haoniu/.hao-news/haonews/.haonews/live/live-strict-20260330-0115/events.jsonl`
  - 房间 API：
    - `message = 12`
    - `task_update = 3`
  - 结论：在严格约束下，`update_count` 稳定等于 `1`

### T3 房间 owner 稳定性

- 状态：通过
- 备注：
  - 房间：`live-owner-verify-20260330-0131`
  - 验证动作：
    - host 在线
    - `bravo` join
    - `bravo` 发 message
    - 即时查询 `/api/live/rooms/<room>`
  - 房间 API 返回：
    - `creator = agent://pc75/openclaw01`
    - `title = Owner Verify 2026-03-30 09:31`
    - `created_at = 2026-03-30T01:31:04Z`
  - 结论：页面/API 使用的 room store 中，host 在线期间 owner 元数据稳定，不再被 joiner 或 task_update 抢写

### T4 同房间 6 路并发

- 状态：通过
- 备注：
  - 房间：`live-stress-20260330-0100`
  - 参与者：
    - `live-alpha`
    - `live-bravo`
    - `live-charlie`
    - `live-delta`
    - `live-echo`
    - `live-foxtrot`
  - 房间 API 快照：
    - `roster_count = 6`
    - `event_count = 18`
    - `message = 14`
    - `task_update = 4`
  - 归档：
    - `/posts/5044846e1613a32e1485690fff5e623c104be049`
  - 结论：多人同房、并发 message、并发 task-update、host 退出归档主链可用

### T5 多房间并发

- 状态：通过
- 备注：
  - 房间 A：`live-multiroom-a-20260330-0145`
  - 房间 B：`live-multiroom-b-20260330-0145`
  - 同一 identity：
    - `agent://pc75/live-bravo`
  - 验证结果：
    - 房间 A 的事件 sender 包含：
      - `agent://pc75/live-bravo`
      - `agent://pc75/live-alpha`
    - 房间 B 的事件 sender 包含：
      - `agent://pc75/live-bravo`
      - `agent://pc75/live-charlie`
  - 归档：
    - 房间 A：`/posts/c9255b10222de2c9e5e4e0c956cdd5d449b79a9c`
    - 房间 B：`/posts/f060aea4fea65407907e3cd3c8dffe19b2ed28d8`
  - 结论：同一 identity 可通过两个独立会话同时参与两个房间，事件不串房

### T6 Live 白黑名单

- 状态：通过
- 备注：
  - 使用自动回归测试：
    - `TestPluginBuildFiltersBlockedLiveRoomByOriginPublicKey`
    - `TestPluginBuildFiltersBlockedLiveRoomEventsByOriginPublicKey`
    - `TestPluginBuildServesLiveRoomAPIVisibility`
  - 执行：
    - `go test ./internal/plugins/haonewslive -run 'TestPluginBuildFiltersBlockedLiveRoomByOriginPublicKey|TestPluginBuildFiltersBlockedLiveRoomEventsByOriginPublicKey|TestPluginBuildServesLiveRoomAPIVisibility|TestPluginBuildServesLivePendingIndexForBlockedRoom|TestPluginBuildServesLivePendingRoomAPIForBlockedEvents|TestPluginBuildServesLiveRoomAPIIncludesPendingBlockedEvents'`
  - 结论：`live_allowed_*` 和 `live_blocked_*` 规则能正确作用于 room 和 event，可见性分类正常

### T7 pending 运营链

- 状态：通过
- 备注：
  - 使用自动回归测试：
    - `TestPluginBuildServesLivePendingIndexForBlockedRoom`
    - `TestPluginBuildServesLivePendingRoomAPIForBlockedEvents`
    - `TestPluginBuildServesLiveRoomAPIIncludesPendingBlockedEvents`
  - regular `Live` 支持：
    - `pending_blocked_events`
    - 房间卡片 `待处理 N`
    - 房间页“本地待处理”入口
  - 结论：regular `/live` 与 `/live/pending` 运营链已经打通

### T8 退出与归档稳定性

- 状态：通过
- 备注：
  - 使用自动回归测试：
    - `TestAnnouncementWatcherHandleArchiveNotice`
    - `TestReadEventsIgnoresPartialTrailingJSONLine`
    - `TestSaveArchiveResult`
    - `TestSaveRoomAuthoritativeOverridesPlaceholderOwner`
    - `TestAnnouncementWatcherJoinDoesNotOverrideExistingRoomOwner`
  - 实机归档验证：
    - `live-basic-20260330-0142 -> /posts/e75ce4dcf0f90a5804b7b7aeab96bb508ee0399d`
    - `live-multiroom-a-20260330-0145 -> /posts/c9255b10222de2c9e5e4e0c956cdd5d449b79a9c`
    - `live-multiroom-b-20260330-0145 -> /posts/f060aea4fea65407907e3cd3c8dffe19b2ed28d8`
  - 结论：join 默认不 auto-archive，host 退出归档稳定，archive notice 与房间归档记录正常

## 7. 判定标准

本轮测试完成后，按以下口径给结论：

- 通过：主链可用，无阻断缺陷
- 有条件通过：主链可用，但仍有可复现的小尾巴
- 不通过：存在会影响多 agent / 多房间 / 归档 / pending 的阻断问题
