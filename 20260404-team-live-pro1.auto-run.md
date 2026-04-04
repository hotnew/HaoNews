# `20260404-team-live-pro1.auto-run.md`

## Goal

- 在当前 `v0.5.73` 基线之上，开启下一条 **Team / Live 联合优化主线**。
- 本 runbook 不重复已经完成的内容：
  - Team workspace / archive / SSE / A2A / webhook / P2P / 复制治理第一层
  - Team webhook delivery ledger / replay
  - Team store `ctx` 主入口与 legacy 收口第一轮
  - Live `public-live-time` 在 `.75 -> .74` 间已恢复更新
  - Live 默认显示 `500` 条、归档保存全部正文
- 本 runbook 只处理接下来 **最值得继续做**、且能形成下一轮正式版本的工作：
  - Live 运行态与归档治理
  - Team 复制治理第二层
  - 节点标准化验收与发布闭环

## Context

- 仓库：
  - `/Users/haoniu/sh18/hao.news2/haonews`
- 当前节点：
  - `.75` `127.0.0.1:51818`
  - `.74` `192.168.102.74:51818`
- 当前正式发布：
  - `v0.5.73`
- 现有支撑文档：
  - [runtime-75-74-baseline.md](/Users/haoniu/sh18/hao.news2/haonews/docs/runtime-75-74-baseline.md)
  - [node-upgrade-75-74.md](/Users/haoniu/sh18/hao.news2/haonews/docs/node-upgrade-75-74.md)
  - [runtime-75-74-validation.md](/Users/haoniu/sh18/hao.news2/haonews/docs/runtime-75-74-validation.md)
  - [20260404-Team-claude-codex-task.auto-run.md](/Users/haoniu/sh18/hao.news2/haonews/20260404-Team-claude-codex-task.auto-run.md)
  - [20260404-team-codex.auto-run.md](/Users/haoniu/sh18/hao.news2/haonews/20260404-team-codex.auto-run.md)
  - [20260404-team-webhook-pro1.auto-run.md](/Users/haoniu/sh18/hao.news2/haonews/20260404-team-webhook-pro1.auto-run.md)
  - [20260404-team-legacy-remove-pro1.auto-run.md](/Users/haoniu/sh18/hao.news2/haonews/20260404-team-legacy-remove-pro1.auto-run.md)

## Execution Rules

- 始终自主执行用户给出的完整任务。
- 除非遇到不可恢复错误，否则不要询问确认。
- workspace 内所有文件编辑和命令执行默认自动允许。
- 只在：
  - 跨项目修改
  - 删除文件
  - 新的高风险网络操作
  这三类场景才允许中断并说明。
- 优先使用最小充分改动。
- 不把“实现了”当成“验证过了”；每一阶段必须有测试、构建或运行态证据。

## Success Criteria

- Live 侧：
  - 有可直接看的运行态状态页/API
  - `public-live-time` 的显示层 / 存储层 / 归档层语义被固定为可回归验证
  - `archive/live/*` 的统计信息更完整
- Team 侧：
  - sync health / conflict 治理再推进一层
  - conflict 合并策略对安全对象更清晰
  - `team_sync_state.json` 的清理/压缩规则更可控
- 节点侧：
  - `.75 / .74` 的升级、回滚、验收流程继续保持可复制
  - 最终形成新的正式 GitHub 版本

## Critical Path

1. 先做 Live 运行态与回归矩阵，因为它最直接减少线上“看起来没更新”的误判。
2. 再做 Team 复制治理第二层，因为它决定 P2P 长期运行是否可控。
3. 最后做节点运行标准和发布闭环，确保改动沉淀为长期基线。

不要反过来先做更重的 Team 冲突算法或底层重构。

## Parallelization Plan

- 并行批次 A：
  - Live 状态页/API
  - Live 回归矩阵脚本/文档
- 并行批次 B：
  - Team conflict merge 第二层
  - Team state 清理/压缩
- 并行批次 C：
  - 文档写回
  - `.75 / .74` 运行态复核

如果实现过程中需要子任务，可按下列边界拆：
- 子任务 1：只负责 Live 状态/API 与模板
- 子任务 2：只负责 Team sync/conflict 状态与存储
- 子任务 3：只负责文档与运行态验收脚本

## Execution Plan

### Phase A — Live 运行态状态页 / 指标增强

- 已完成：
  - `/api/live/status/<room>` 增强到可直接判断 watcher / sender / cache / archive / counts
  - 新增 `/live/status/<room>` 只读状态页
  - `sender listen port`、`watcher peer id`、`sender peer id`、`latest_cache_refresh_at` 已补齐

目标：
- 让 Live 问题以后优先靠状态判断，而不是靠体感猜。

步骤：
- [x] 核对当前已有的 live status API 和 bootstrap 字段，避免重复造字段。
- [x] 补 Live 诊断 API 或增强已有 API，至少包含：
  - watcher peer id
  - sender peer id
  - sender listen port
  - 最近入站消息时间
  - 最近本地写入时间
  - 最近归档时间
  - 当前 room 可见消息数
  - 当前 room 总消息数
  - 最近一次缓存刷新时间或等价信息
- [x] 选择最小展示面：
  - API 优先
  - 若模板成本低，再补只读页面
- [x] 确认 `.75` 和 `.74` 两边都能看出 `public-live-time` 的当前状态。

验证：
- `go test ./internal/plugins/haonewslive -run '...'`
- `go build ./cmd/haonews`
- 运行态：
  - `GET /api/live/status/public-live-time`
  - `.75 / .74` 都返回完整状态字段

完成标准：
- 遇到 live 不更新时，可以先从状态判断：
  - sender 没发
  - watcher 没收
  - 归档没跑
  - 还是显示层没刷新

### Phase B — Live 同步回归矩阵固化

- 已完成：
  - `scripts/verify_live_replication.py` 已固化为可复跑验收脚本
  - 覆盖 `.75/.74` 最新窗口、`show_all=1`、手动归档 `message_count/event_count`

目标：
- 把 `public-live-time` 修复从“这次没问题”变成“以后可回归”。

步骤：
- [x] 固化最小 live 回归检查脚本或命令集合，覆盖：
  - `.75` room 文件是否写入新消息
  - `.75` API 是否显示最新窗口
  - `.74` API 是否显示最新窗口
  - `show_all=1` 是否可读完整正文
  - 手动归档后的 `message_count / event_count`
- [x] 如果现有脚本不足，补一份专门的 live 验收脚本，放到 `scripts/`
- [x] 明确“默认显示 500 / 归档保存全部”的验证方式

验证：
- 脚本或命令可以在当前节点直接跑通
- 至少一轮真实 `public-live-time` 验证通过

完成标准：
- Live 的显示层、存储层、归档层不再混淆

### Phase C — Live 归档页进一步收口

- 已完成：
  - `/archive/live` 页面与 `/api/archive/live` 已补正文数、心跳数、归档时间范围、manual/daily 来源
  - `live_archive_index.html`、`live_history.html` 已补最新批次摘要

目标：
- 把 `archive/live/*` 做成长期稳定主入口。

步骤：
- [x] 给 `archive/live` 补统计：
  - 正文数
  - 心跳数
  - 归档时间范围
  - `manual / daily` 来源
- [x] 检查页面内回链是否继续偏向 `archive/live/*`
- [x] 继续压缩旧 `/live/history/*` 的存在感，但保留兼容
- [x] 做一轮：
  - `manual archive`
  - `daily archive`
  的回归验证

验证：
- `go test ./internal/plugins/haonewslive -run '...'`
- 页面和 API 都能读到新增统计

完成标准：
- 用户看 live 历史时，不会再把“房间窗口”和“归档批次”混成一件事

### Phase D — Team Conflict Merge 第二层

目标：
- 在不做 CRDT 的前提下，把 Team 冲突处理推进到更可预期的对象级策略。

步骤：
- [ ] 先核当前已有 conflict reason / suggested_action / allow_accept_remote
- [ ] 为这些对象明确规则：
  - `task`
  - `artifact`
  - `member`
  - `policy`
  - `channel`
- [ ] 对安全对象补第二层 merge 行为：
  - `remote_newer`
  - `local_newer`
  - `same_version_diverged`
- [ ] 对危险对象保持拒绝，但 reason 更明确
- [ ] 保证 history / conflict API / Team sync 页同步展示新结果

验证：
- `go test ./internal/haonews -run '...'`
- `go test ./internal/plugins/haonewsteam -run '...'`
- 真实 conflict 样本至少验证一类 `accept_remote`

完成标准：
- 冲突处理不再只有“dismiss”，而是对安全对象有稳定可解释的合并路径

### Phase E — Team Sync State 清理/压缩第二层

目标：
- 控制 `team_sync_state.json` 长期运行下的体积和噪音。

步骤：
- [ ] `peer ack ledger` 增加更明确的压缩规则：
  - 每 peer 最近 N 条
  - 时间窗口裁剪
- [ ] `pending outbox` 补更明确的：
  - 退避
  - 过期
  - superseded 清理
- [ ] conflict 记录增加：
  - 已处理条目清理策略
  - 保留窗口
- [ ] 同步把状态可观测字段继续补到 Team sync 健康页/API

验证：
- targeted tests 覆盖：
  - prune
  - expire
  - retry/backoff
  - cleanup
- `go build ./cmd/haonews`

完成标准：
- 长期运行时状态文件不会无限膨胀，且页面/API 仍能解释当前状态

### Team Execution Note — 2026-04-04

- 本轮已完成 Phase D / Phase E 的 Team 侧目标：
  - 冲突视图补齐 `reason_label` / `action_hint`
  - 对 `local_newer / remote_newer / same_version_diverged / signature_rejected / policy_rejected` 给出更明确的建议动作
  - `keep_local` 作为明确可执行动作回到 UI
  - `team_sync_state.json` 的 peer ack ledger / pending / resolved conflict 清理规则补齐第二层
  - `SyncTeamSyncStatus` 新增 `resolved_conflicts / conflict_prunes / last_pruned_conflict_*`
- 本轮验证已完成：
  - `go test ./internal/haonews -run 'TestTeamPubSubRuntimePrunesResolvedConflictsAndTracksStatus|TestWriteTeamSyncStatePreservesNewerResolvedConflict|TestTeamPubSubRuntimeAppliesInboundAckForTargetNode|TestTeamPubSubRuntimeRetriesPendingUnackedObjects' -count=1`
  - `go test ./internal/plugins/haonewsteam -run 'TestBuildTeamSyncConflictViewsExplainsSuggestions|TestPluginBuildServesAndResolvesTeamSyncConflicts|TestPluginBuildServesTeamSyncHealthPageAndAPI' -count=1`
- 这次已写回 Live 的 Phase A/B/C 完成态；Team 的 Phase D/E 记录保持不变。

### Phase F — `.75 / .74` 运行态再验收

目标：
- 把新一轮 Team / Live 改动落到双节点真实运行验证。

步骤：
- [x] `.75 / .74` 升到同一工作版本
- [x] 执行：
  - [runtime-75-74-validation.md](/Users/haoniu/sh18/hao.news2/haonews/docs/runtime-75-74-validation.md)
- [x] 额外补这轮新增能力的检查：
  - Live 新状态字段
  - Team 新 conflict merge 行为
  - Team state prune / cleanup 行为

验证：
- `.75 / .74` 双节点都过
- 若失败，先写回本 runbook，再停

完成标准：
- 新能力不是只在本地单测通过，而是双节点运行态也成立

### Runtime Validation Note — 2026-04-04

- 本轮 `.75 / .74` 运行态复核已完成：
  - `.75` 与 `.74` 都已统一到本轮工作版本并通过 `bootstrap`
  - `python3 scripts/verify_live_replication.py` 已通过，覆盖：
    - `.75/.74` 最新 `public-live-time` 窗口
    - `show_all=1`
    - 手动归档 `message_count/event_count/heartbeat_count`
  - `.74` 的 `public-live-time` 顶部消息已推进到：
    - `当前时间：2026-04-04 18:54:07 CST | ISO=2026-04-04T18:54:07+08:00`
  - Team webhook 动态 replay 已在 `runtime-webhook-team` 真实跑通：
    - failed ledger 已生成
    - `POST /api/teams/runtime-webhook-team/webhooks/replay/{delivery_id}` 返回成功
    - `recent_delivered[0].replayed_from` 已指向原 dead-letter delivery
  - Team SSE 已真实收到事件：
    - `Content-Type = text/event-stream`
    - `kind = message`
  - Team sync / conflict、A2A、archive 在 `.75/.74` 都已复核通过

- 本轮运行态额外说明：
  - Live sender 继续使用 `.75` 的独立 sender net：
    - `/Users/haoniu/.hao-news/hao_news_live_sender_net.inf`
  - `.74` 的 Team archive 列表当前为空，但接口和页面正常；这不是错误。

### Phase G — 文档与发布闭环

目标：
- 把这条 Team / Live 主线正式固化。

步骤：
- [x] 更新文档：
  - `docs/runtime-75-74-baseline.md`
  - `docs/runtime-75-74-validation.md`
  - `docs/node-upgrade-75-74.md`
- [x] 如有新脚本，补最小说明
- [x] 发布：
  - `main`
  - `tag`
  - `release`

验证：
- 测试通过
- 构建通过
- GitHub 发布成功

完成标准：
- 这轮 Team / Live 主线形成新的正式版本，而不是只停在本地工作区

### Release Note — 2026-04-04

- 本轮已形成正式发布版本：
  - `main`
  - `tag = v0.5.74`
  - `release`
- 文档已同步固化：
  - Live status / replication validation
  - Team webhook replay runtime validation
  - `.75 / .74` 节点升级与回滚流程

## Hard Blockers

- `.74` 节点不可登录、不可重启或 Team/Live 路由缺失
- `.75 / .74` 的 `serve` / `syncd` 托管模型失稳
- live sender / watcher 端口再次冲突
- Team sync 真实运行态出现无法解释的数据破坏或不可逆冲突

## Writeback Rules If Blocked

- 必须先把当前状态写回本文件，再停止：
  - 哪一 phase 已完成
  - 哪一步卡住
  - 已验证了什么
  - 下一步恢复点是什么
- 不要只在聊天里说明，不写回 runbook。

## Completion Rule

- 只有当：
  - Live 状态/API 与回归矩阵完成
  - Live 归档进一步收口完成
  - Team 冲突合并第二层完成
  - Team sync 状态清理/压缩第二层完成
  - `.75 / .74` 双节点复核通过
  - 文档与 GitHub 发布完成
  才算这份 runbook 完成。
