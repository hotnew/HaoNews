# `20260404-team-codex.auto-run.md`

## Goal

- 基于当前已完成的基线：
  - `Live` 已修到：
    - 默认显示最近 `500` 条
    - 归档保存全部正文
    - `.75 -> .74` 的 `public-live-time` 已真实更新
  - `Topics` 已基本迁到 `archive/topics/*`
  - `Team` 已具备：
    - 工作区
    - archive
    - SSE / A2A / webhook
    - P2P 复制与第一层复制治理
- 把接下来**最值得继续做的前 10 条任务**，按收益和依赖顺序收成一份可直接执行的 runbook。
- 本文档的目标不是“列所有可能想法”，而是给后续执行一个明确的前 10 优先级清单。

## Context

- 仓库：
  - `/Users/haoniu/sh18/hao.news2/haonews`
- 当前节点基线：
  - `.75`：主开发/主运行节点
  - `.74`：当前 P2P / live 远端节点
  - `.76`：已停止参与当前主线
- 当前发布基线：
  - Team 复制治理：`v0.5.69`
  - Live sender/bootstrap 修复：`v0.5.71`
- 当前已经完成且不应重复作为主任务的内容：
  - Team P2P 对象复制：
    - `message/history`
    - `task/artifact`
    - `member/policy/channel`
  - Team 复制治理第一层：
    - cursor persistence
    - minimal ack
    - pending retry
    - per-peer ack ledger
    - conflict API / resolve
  - Live sender 与 watcher 端口拆分
  - `.74` 上 `public-live-time` 已可看到新消息

## Planning Rules

- 按价值和依赖排序，不按叙事顺序排序。
- 先做：
  - 会减少线上故障概率
  - 会减少后续排障成本
  - 会直接提升可用性/可解释性
  的任务。
- 不把高风险底层重构放进前 10，除非已有明确证据表明它是当前主瓶颈。
- 所有任务默认要有：
  - 最小实现边界
  - 可验证命令或直接运行态检查
  - 明确完成标准

## Top 10 Priority Tasks

### P1. 固化 `.75 / .74` 节点运行基线

目标：
- 把当前能跑通的 `.75 / .74` 运行方式固化成标准基线，避免后续再被手工进程和旧节点配置打散。

步骤：
- [ ] 核对并统一：
  - `hao_news_net.inf`
  - `hao_news_live_net.inf`
  - sender 专用 net 配置
  - LaunchAgent
- [ ] 清理 `.75 / .74` 上残留的旧 `.76` 心智和旧 peer/backoff 缓存策略。
- [ ] 形成节点基线文档：
  - `.75` 做什么
  - `.74` 做什么
  - 哪些进程应由 `serve` 托管
  - 哪些脚本独立运行

完成标准：
- `.75 / .74` 任一节点重启后，都能按同一套路恢复。

### P2. Live 同步回归矩阵

目标：
- 防止 `public-live-time` 这类“发了新消息但远端还显示旧窗口”的问题再次回归。

步骤：
- [ ] 建一个最小回归矩阵，至少覆盖：
  - `.75` 本地新消息是否入本地 room 文件
  - `.75` 本地 API 是否显示最新窗口
  - `.74` 远端 room 文件是否收到新消息
  - `.74` 远端 API 是否显示最新窗口
- [ ] 固定检查：
  - 默认显示 `500`
  - `show_all=1`
  - 手动归档 `message_count`

完成标准：
- Live 显示层、存储层、归档层三者的语义不再混淆。

### P3. Live 运行态状态页 / 指标增强

目标：
- 让 live 问题以后不靠猜，而是直接看状态。

步骤：
- [ ] 增加 live 运行态字段：
  - watcher peer
  - sender peer
  - sender listen port
  - 最近入站消息时间
  - 最近归档时间
  - 当前 room 可见消息数 / 总消息数
- [ ] 将这些状态接到：
  - `bootstrap`
  - 或单独 `live` 诊断 API

完成标准：
- 遇到 live 不更新时，先看状态就能判断是：
  - sender 没发
  - watcher 没收
  - 还是显示层没刷新

### P4. Live 归档进一步收口

目标：
- 把 `archive/live/*` 彻底变成长期稳定主语义。

步骤：
- [ ] 继续压缩旧 `/live/history/*` 的存在感，只保留兼容。
- [ ] 给 `archive/live` 页补更多统计：
  - 正文数
  - 心跳数
  - 归档时间范围
  - 来源（manual / daily）
- [ ] 做一轮 `manual + daily` 的回归验证。

完成标准：
- 用户看 Live 历史时，不会再混淆“当前房间窗口”和“归档批次”。

### P5. Topics 归档 API 最终收口

目标：
- 把 `archive/topics/*` 的页面和 API 完整收口，旧接口只保留兼容壳。

步骤：
- [ ] 统一主 API：
  - `/api/archive/topics/list`
  - `/api/archive/topics/manifest`
- [ ] 检查页面内所有 Topics archive 链接都输出新路径。
- [ ] 保留旧 `api/history/*` 兼容，但内部统一走新实现。

完成标准：
- Topics 归档不再同时有多套“看起来都像主入口”的路径。

### P6. Team Sync 健康页

目标：
- 把 Team sync 的运行态从 `bootstrap.team_sync` 里的调试字段，升级成可直接看的治理页。

步骤：
- [ ] 增加 Team sync 只读页：
  - 当前健康状态
  - pending
  - peer ack
  - 最近 conflict
- [ ] 与 Team detail / history 做自然入口连接。

完成标准：
- 不需要手打 API，就能看 Team P2P 当前是否健康。

### P7. Team 冲突治理 UI 收口

目标：
- 让现有 conflict API / resolve 不只对调试友好，也对日常使用友好。

步骤：
- [ ] Team detail 页增加：
  - 最近冲突摘要
  - 最近 unresolved 数量
- [ ] History 页增加：
  - `复制治理`
  - `最近冲突`
  - `最近 resolve`
- [ ] 对 `dismiss / accept_remote` 给出更明确的页面入口和结果反馈。

完成标准：
- 冲突治理不再只靠 API 和手工 curl。

### P8. Team 冲突合并策略第二层

目标：
- 在不引入 CRDT 的前提下，把当前“记录并 resolve”继续推进到更可控的合并规则。

步骤：
- [ ] 明确对象级策略：
  - `task`
  - `artifact`
  - `member`
  - `policy`
  - `channel`
- [ ] 为安全对象继续补：
  - `accept_remote` 回放边界
  - `local_newer / remote_newer / same_version_diverged` 的具体规则
- [ ] 对危险对象保持拒绝并给出可解释 reason。

完成标准：
- 冲突处理不再只是“能 dismiss”，而是对安全对象有稳定可预期的结果。

### P9. Team 复制治理的压缩/清理策略第二层

目标：
- 控制 `team_sync_state.json` 长期运行下的体积和噪音。

步骤：
- [ ] `peer ack ledger` 再压缩：
  - 每 peer 最近 N 条
  - 按时间窗口裁剪
- [ ] `pending outbox` 再治理：
  - 更明确 backoff
  - 更明确过期/清理条件
- [ ] conflict 过期/已处理条目清理策略

完成标准：
- 状态文件长期运行不会无限膨胀。

### P10. 节点升级 / 验收 / 回滚标准化

目标：
- 把我们这几轮“修复 -> GitHub -> 双节点升级 -> 运行验收”的经验固化成可复制流程。

步骤：
- [ ] 写一份节点升级标准流程：
  - GitHub main/tag
  - `.75/.74` 升级
  - codesign
  - launchctl restart
  - 验证命令
- [ ] 写一份回滚最小流程：
  - 切回某个 tag
  - 重启
  - 验证健康

完成标准：
- 以后再升级节点，不需要靠临场记忆。

## Critical Path

按依赖顺序，建议执行顺序固定为：

1. `P1` 节点运行基线
2. `P2` Live 同步回归矩阵
3. `P3` Live 运行态状态页 / 指标增强
4. `P4` Live 归档进一步收口
5. `P5` Topics 归档 API 最终收口
6. `P6` Team Sync 健康页
7. `P7` Team 冲突治理 UI 收口
8. `P8` Team 冲突合并策略第二层
9. `P9` Team 复制治理压缩/清理第二层
10. `P10` 节点升级 / 验收 / 回滚标准化

说明：
- `P1-P4` 先收 Live，是因为它直接影响线上可见性和后续排障效率。
- `P5` 再收 Topics，是因为它的归档语义已接近完成，收尾成本低。
- `P6-P9` 再进入 Team 第二层治理，因为当前已经能用，但还没完全“长期可运维”。
- `P10` 最后做，是为了把前面验证过的最佳实践沉淀下来。

## Verification

### 通用验证要求

- 每做完一项，至少执行一种最强验证：
  - `go test` 目标包
  - `go build ./cmd/haonews`
  - `.75 / .74` 运行态直接验证
- 不把“代码改完”当成“已经完成”。

### 按任务验证

- `P1`
  - 对比 `.75 / .74` 配置文件
  - 检查 `launchctl`、监听端口、bootstrap
- `P2`
  - 用真实 sender 发消息
  - 同时检查：
    - 本地 room 文件
    - 本地 API
    - 远端 room 文件
    - 远端 API
- `P3`
  - 验证新增状态字段真实变化，不是死值
- `P4`
  - 触发 `manual archive`
  - 验证 `message_count / event_count / latest timestamp`
- `P5`
  - 跑 archive topics API/page smoke
  - 确认旧路径仍兼容
- `P6-P9`
  - 跑 Team sync / Team store / Team plugin 相关 targeted tests
  - 在 `.75 / .74` 上做至少一轮真实 Team 复制治理验证
- `P10`
  - 按文档走一次最小升级演练

## Completion Standard

- 前 10 条任务不是“都动过”，而是：
  - 每条都有真实结果
  - 每条都有最小验证闭环
  - 关键运行态问题能直接解释
- 如果执行到一半发现某项收益显著下降或前提不成立：
  - 必须写回文档状态
  - 标清 `done / defer / blocked`

## Blockers / Resume

### Hard blockers

- 远端节点无 SSH / 无发布通道
- 节点运行版本与仓库版本不一致且无法升级
- 必要的运行态证据无法采集
- 某项高风险策略会破坏现有 `.75 / .74` 已跑通主线

### If blocked, write back

- 在本文档底部追加 `Execution Status`：
  - `done`
  - `in_progress`
  - `defer`
  - `blocked`
- 写清：
  - 当前卡在哪一条任务
  - 已完成到哪一步
  - 具体 blocker 是什么
  - 恢复时第一步该做什么

### Next step to resume

- 总是从**当前最高优先级且未完成的任务**恢复。
- 恢复时先做：
  - 重新确认当前节点基线
  - 再执行该任务的第一条验证命令

## Execution Status

- `done`
  - `P1` `.75 / .74` 节点运行基线
    - 新增基线文档：[doc-md/runtime-75-74-baseline.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/runtime-75-74-baseline.md)
    - `.75` 与 `.74` 都已收成 `launchctl -> serve -> managed sync` 的统一模式
    - `.76` 已退出当前主线
  - `P2` Live 同步回归矩阵
    - 新增验证脚本：[scripts/verify_live_replication.py](/Users/haoniu/sh18/hao.news2/haonews/scripts/verify_live_replication.py)
    - 已验证 `.75 / .74` 当前最新 `public-live-time` 一致
  - `P3` Live 运行态状态页 / 指标增强
    - 新增 `GET /api/live/status/{roomID}`
    - 已暴露 `watcher`、`sender_config`、`visible_event_count`、`total_event_count`、最新消息时间等字段
  - `P4` Live 归档进一步收口
    - 归档记录新增 `non_heartbeat_count / heartbeat_count`
    - `archive/live` 页面已显示正文数、心跳数和归档来源
    - 已完成 `manual archive` 真实验证
  - `P5` Topics 归档 API 最终收口
    - 主 API 已收口到 `/api/archive/topics/list` 与 `/api/archive/topics/manifest`
    - 主页面输出已统一到 `archive/topics/*`
    - 旧 `/api/history/*`、`/archive/messages/*`、`/archive/raw/*` 继续保留兼容
  - `P6` Team Sync 健康页
    - 新增：
      - `/teams/{teamID}/sync`
      - `/api/teams/{teamID}/sync`
    - 已接入 Team 详情页和历史页入口
  - `P7` Team 冲突治理 UI 收口
    - Team Sync 页已支持页面内 `dismiss / accept_remote`
    - Team 详情页和历史页已显示未处理冲突计数与治理入口
  - `P8` Team 冲突合并策略第二层
    - 已把 `allow_accept_remote / suggested_action` 暴露到 Team Sync 页和 JSON
    - 当前继续沿用安全对象：
      - `task`
      - `artifact`
      - `member`
      - `policy`
      - `channel`
      的 `accept_remote` 边界
  - `P9` Team 复制治理压缩/清理第二层
    - 已把 `peer_ack_prunes / expired_pending / superseded_pending / last_pruned_ack_*` 暴露到 Team Sync 页
    - 当前压缩/清理状态已进入日常可观测面
  - `P10` 节点升级 / 验收 / 回滚标准化
    - 新增标准流程文档：[doc-md/node-upgrade-75-74.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/node-upgrade-75-74.md)
