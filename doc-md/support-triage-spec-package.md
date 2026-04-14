# 客服工单分诊台 规格共创样本 规格包导出

- team_id: `support-triage-spec1`
- profile: `spec-package`
- generated_at: `2026-04-14T12:36:26Z`
- document_count: `7`
- supporting_count: `4`

## 正文规格

### README Spec

- artifact_id: `readme-spec`
- section: `readme`
- channel_id: `artifacts`
- kind: `markdown`

规格包入口与边界

# 客服工单分诊台

这是一套独立于 Team 的本地客服工单分诊与升级系统规格包。

目标是让任何大模型都能根据这套 Markdown 文档，独立实现一个可部署的单机 Web 程序。

正文规格包括：Product、Workflows、Data Model、Screens、API And Runtime、Verification。

### Product Spec

- artifact_id: `product-spec`
- section: `product`
- channel_id: `artifacts`
- kind: `markdown`

产品目标、非目标和角色定义

# Product

## 目标
- 导入客服工单并形成统一收件池
- 根据关键词、来源和人工判断完成优先级分级
- 给负责人分派工单并跟踪 SLA
- 当工单超时或风险升高时进入升级链
- 产出每日待处理列表与已解决归档

## 非目标
- 多租户
- 外部聊天系统双向集成
- 复杂 RBAC
- 跨节点分布式部署

## 用户角色
- 值班坐席
- 复核员
- 升级负责人
- 管理员

### Workflow Spec

- artifact_id: `workflow-spec`
- section: `workflows`
- channel_id: `artifacts`
- kind: `markdown`

工单分诊、升级和关闭流程

# Workflows

1. 工单导入
2. 去重与初筛
3. 分级为 low / medium / high / critical
4. 人工复核并确认负责人
5. 进入 assigned 或 escalated
6. 解决后进入 resolved
7. 验收或回访后进入 closed

升级链：当优先级为 critical、超出 SLA、或客户连续追问时，工单进入 escalated，并记录升级原因、责任人和响应时间。

### Data Model Spec

- artifact_id: `data-model-spec`
- section: `data-model`
- channel_id: `artifacts`
- kind: `markdown`

Ticket 和升级链的数据模型

# Data Model

核心对象：
- Ticket
- ReviewNote
- Escalation
- SLAEvent
- Assignment
- Resolution
- ArchiveEntry

Ticket 最少字段：ticket_id、source、customer、title、content、priority、status、owner、created_at、due_at、updated_at、labels。

状态集合：open、triaged、assigned、escalated、resolved、closed、dropped。

### Screens And Interactions Spec

- artifact_id: `screens-spec`
- section: `screens-and-interactions`
- channel_id: `artifacts`
- kind: `markdown`

页面与交互

# Screens And Interactions

页面：
- 总览页：今日新增、待处理、即将超时、已升级
- 工单池：搜索、过滤、排序、批量分派
- 工单详情：原始内容、标签、优先级、负责人、SLA、升级记录
- 升级页：待升级、已升级、升级原因、责任人
- 归档页：已解决、已关闭、可导出

交互：
- 一键分级
- 一键指派
- 一键升级
- 一键标记解决
- 一键关闭或丢弃

### API And Runtime Spec

- artifact_id: `api-runtime-spec`
- section: `api-and-runtime`
- channel_id: `artifacts`
- kind: `markdown`

运行时与接口

# API And Runtime

## Runtime
- 单机运行
- 文件持久化
- 本地 Web UI
- JSON API
- Markdown/JSON 导出

## API
- GET /api/state
- GET /api/tickets
- POST /api/tickets
- POST /api/tickets/batch
- GET /api/tickets/{id}
- POST /api/tickets/{id}/review
- POST /api/tickets/{id}/assign
- POST /api/tickets/{id}/escalate
- POST /api/tickets/{id}/resolve
- POST /api/tickets/{id}/close
- GET /api/reminders
- GET /api/archive
- GET /exports/daily/latest.md

### Verification Spec

- artifact_id: `verification-spec`
- section: `verification`
- channel_id: `artifacts`
- kind: `markdown`

验证与验收

# Verification

最小验收链：
1. 导入一条工单
2. 自动或人工完成分级
3. 指派负责人
4. 触发一条升级
5. 解决并关闭工单
6. 归档可见
7. 重启后状态保留
8. 可导出当日待处理与已解决摘要

必须验证：
- 优先级排序正确
- SLA 到期提醒正确
- 升级动作会记录历史
- 关闭后不再出现在待处理列表

## 支撑产物

### 规格包目录与输出要求

- artifact_id: `support-skill-doc`
- section: `supporting`
- channel_id: `main`
- kind: `skill-doc`

规格包目录必须独立于 Team，可直接交给任何模型实现。

`{"kind":"skill","steps":["冻结 product","冻结 workflows","冻结 data model","冻结 api/runtime","冻结 verification"],"outputs":["README Spec","Product Spec","Workflow Spec","Data Model Spec","Screens And Interactions Spec","API And Runtime Spec","Verification Spec"]}`

### 运行时边界冻结评审结论

- artifact_id: `support-review-summary`
- section: `supporting`
- channel_id: `reviews`
- kind: `review-summary`

实现前必须冻结分级、升级和 SLA 统计边界。

`{"summary":"先冻结优先级矩阵、SLA 到期规则和升级动作，再进入实现。","decision":"先冻结边界再实现"}`

### 运行时边界冻结

- artifact_id: `support-decision-note`
- section: `supporting`
- channel_id: `decisions`
- kind: `decision-note`

决定先用单机文件持久化 + Web UI + JSON API。

`{"outcome":"单机文件持久化 + Web UI + JSON API","followups":["定义 tickets.json","定义 reminders.json","定义 daily export"]}`

### 规格包正文已发布

- artifact_id: `support-artifact-brief`
- section: `supporting`
- channel_id: `artifacts`
- kind: `artifact-brief`

正文规格已经齐套，可直接导出给下游实现。

`{"documents":["README","Product","Workflows","Data Model","Screens","API Runtime","Verification"],"status":"ready"}`
