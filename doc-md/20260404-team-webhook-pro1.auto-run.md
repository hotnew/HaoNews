# `20260404-team-webhook-pro1.auto-run.md`

## Goal

- 把 Team webhook 从“最小可用 + 最小 retry”推进到“可观测、可恢复、可治理”的下一阶段。
- 这份 runbook 不重做已经完成的 webhook 能力；它只处理仍然值得继续做的：
  - dead-letter
  - 更细 backoff / retry
  - 失败状态可见性
  - 最小管理入口

目标不是把 webhook 做成复杂消息队列，而是在当前 Team 架构里补齐最容易真实出问题的投递治理。

## Context

- 仓库：
  - `/Users/haoniu/sh18/hao.news2/haonews`
- 当前 webhook 基线：
  - 已支持配置、过滤、异步 POST
  - 已有最小 retry 和复用 `http.Client`
  - 当前缺少：
    - dead-letter 持久化
    - delivery attempt 状态
    - 失败后人工回放入口
    - 可读的 webhook 健康/失败视图
- 相关代码主要在：
  - `/Users/haoniu/sh18/hao.news2/hao.news2/haonews/internal/haonews/team/store.go`
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/handler_sync.go`
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/handler.go`

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

- webhook 失败不再只存在瞬时日志里
- 至少有一个持久化 dead-letter / failed-delivery 记录层
- 至少有一个只读 API 或页面能看到 webhook 失败状态
- 至少支持一条最小 replay / retry 动作
- 所有改动都有 targeted tests，并通过：
  - `go test ./internal/haonews/team`
  - `go test ./internal/plugins/haonewsteam`
  - `go build ./cmd/haonews`

## Critical Path

1. 先补持久化失败账本
2. 再补 retry/backoff 语义
3. 再补可观测性/API
4. 最后补最小 replay

不要反过来先做 UI，再去找状态落点。

## Execution Plan

### Phase A — 现状核对与边界确认

目标：
- 明确当前 webhook 逻辑已经有什么，避免重复造轮子。

步骤：
- [ ] 核对当前 `sendWebhook` / `publish` / config store / tests
- [ ] 写回本 runbook：
  - 已完成
  - 待完成
  - 明确不做

完成标准：
- runbook 本身成为 webhook 当前真实状态看板

### Phase B — dead-letter / failed delivery 持久化

目标：
- 失败状态从“日志瞬时”变成“可追踪实体”。

步骤：
- [ ] 新增最小持久化结构：
  - `WebhookDeliveryRecord`
  - `WebhookFailureRecord`
- [ ] 记录至少这些字段：
  - `team_id`
  - `webhook_id` 或 endpoint 标识
  - `event_id`
  - `kind/action`
  - `attempt`
  - `status_code`
  - `error`
  - `next_retry_at`
  - `created_at`
  - `updated_at`
- [ ] 落地路径优先简单：
  - `webhook_failures.jsonl`
  - 或 `sync/webhook/*.json`
- [ ] 不先做复杂数据库/WAL

验证：
- 单测覆盖：
  - 网络错误
  - 5xx
  - 非重试类 4xx

完成标准：
- webhook 失败后，状态能持久化，不再只留在日志

### Phase C — retry / backoff 第二层

目标：
- 让重试不只是“立刻多打几次”，而是有最小治理语义。

步骤：
- [ ] 定义重试类别：
  - `network error`
  - `429`
  - `5xx`
  - `non-retriable 4xx`
- [ ] 引入最小 backoff：
  - 固定递增或指数退避都可
  - 但要可测试、可观测
- [ ] 增加上限：
  - `max_attempts`
- [ ] 超过上限后进入 dead-letter，而不是无限试

验证：
- 单测：
  - retriable 状态会进入下一次计划
  - non-retriable 直接终止
  - 超过上限进入 dead-letter

完成标准：
- webhook delivery 生命周期明确：
  - `pending`
  - `retrying`
  - `failed`
  - `dead_letter`

### Phase D — API / 页面可观测性

目标：
- 失败和待重试状态可以被直接查看。

步骤：
- [ ] 新增只读 API：
  - `/api/teams/{teamID}/webhooks/status`
  - 或同等入口
- [ ] 至少返回：
  - 最近失败
  - 最近 dead-letter
  - 最近成功统计
  - retrying 数量
- [ ] 若改动面小，再补最小页面：
  - `/teams/{teamID}/webhooks`

验证：
- plugin tests 覆盖：
  - JSON
  - 页面 smoke

完成标准：
- 不是只有日志才能看到 webhook 出问题

### Phase E — 最小 replay / retry 动作

目标：
- 至少允许人工把一条失败投递重新发送一次。

步骤：
- [ ] 新增动作：
  - `POST /api/teams/{teamID}/webhooks/replay/{deliveryID}`
  - 或 `.../retry/{deliveryID}`
- [ ] replay 要求：
  - 不直接重放未知 payload
  - 必须能找到原始事件/原始失败记录
- [ ] replay 成功后更新状态

验证：
- 单测：
  - replay 成功
  - replay 对不存在记录返回 not found

完成标准：
- 至少存在一条最小人工恢复路径

### Phase F — 写回与验收

步骤：
- [ ] 写回本 runbook 的真实执行状态
- [ ] 写明：
  - 哪些治理已完成
  - 哪些仍 defer
  - 恢复下一步从哪开始

## Blockers

如遇硬阻塞，先写回本文件：
- `status: blocked`
- `blocked_on: ...`
- `last_safe_file: ...`
- `resume_from: ...`

再停止执行。

## Verification

- `go test ./internal/haonews/team -count=1`
- `go test ./internal/plugins/haonewsteam -count=1`
- `go build ./cmd/haonews`

## Completion Standard

本 runbook 视为完成，需同时满足：
- dead-letter 已存在
- retry/backoff 第二层已存在
- 至少一个状态 API 或页面已存在
- 至少一个 replay/retry 动作已存在
- 测试与构建通过

## Execution Status

### Completed in this run

- `Phase A` = `done`
  - 已核实当前 webhook 基线：
    - 已有配置存储
    - 已有异步 POST
    - 已有最小 retry
    - 已有复用 `http.Client`
  - 当前真正缺的确实是：
    - 持久化失败账本
    - 状态 API
    - replay 动作
- `Phase B` = `done`
  - 已新增持久化失败账本：
    - [webhook_delivery.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/webhook_delivery.go)
  - 新增：
    - `WebhookDeliveryRecord`
    - `WebhookDeliveryStatus`
  - 失败/成功记录已持久化到：
    - `webhook-deliveries.json`
- `Phase C` = `done`
  - `sendWebhook` 已升级为有状态发送：
    - `pending`
    - `retrying`
    - `delivered`
    - `failed`
    - `dead_letter`
  - 已按类别处理：
    - network error
    - `429`
    - `5xx`
    - non-retriable `4xx`
  - 已引入第二层 backoff：
    - `attempt * 200ms`
  - 已设置上限：
    - `3 attempts`
- `Phase D` = `done`
  - 已新增只读状态 API：
    - `/api/teams/{teamID}/webhooks/status`
  - 路由接入：
    - [plugin.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/plugin.go)
  - handler：
    - [handler_webhook.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/handler_webhook.go)
- `Phase E` = `done`
  - 已新增最小 replay 动作：
    - `POST /api/teams/{teamID}/webhooks/replay/{deliveryID}`
  - replay 基于原始失败记录，不重放未知 payload
- 相关文件：
  - [webhook_delivery.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/webhook_delivery.go)
  - [store.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/store.go)
  - [paths.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/paths.go)
  - [handler_webhook.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/handler_webhook.go)
  - [plugin.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/plugin.go)
  - [store_test.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/store_test.go)
  - [plugin_test.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/plugin_test.go)

### Deferred in this run

- 页面级 webhook 管理入口
- dead-letter 的更复杂分页/清理策略
- 更细的指数退避 / jitter / delivery scheduler

### Verified in this run

- `go test ./internal/haonews/team -run 'TestStoreWebhookReceivesPublishedEvent|TestStoreWebhookRetriesRetriableStatus|TestStoreWebhookPersistsDeadLetterAndReplay|TestStoreWebhookPersistsNonRetriableFailure' -count=1`
- `go test ./internal/plugins/haonewsteam -run 'TestPluginBuildConfiguresAndFiresTeamWebhook|TestPluginBuildServesWebhookStatusAndReplay' -count=1`
- `go test ./internal/haonews/team -count=1`
- `go test ./internal/plugins/haonewsteam -count=1`
- `go build ./cmd/haonews`

### Runbook Completion

- 这份 runbook 已完成。
- 当前 webhook 已经具备：
  - 持久化失败账本
  - 第二层 retry/backoff
  - 状态 API
  - replay 动作
- 如果继续，下一阶段才是更重的治理增强，而不是这份 runbook 的未完成项。
