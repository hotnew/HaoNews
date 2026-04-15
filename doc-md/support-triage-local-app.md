# 客服工单分诊台 本地程序

## 这是什么

这是一套**与 Team 运行时解耦**的本地程序。

它不是 Team 房间，也不依赖 Team API 运行，而是根据：

- [support-triage-spec-package.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/support-triage-spec-package.md)

独立实现出来的本地 Web 应用。

## 怎么启动

```bash
cd /Users/haoniu/sh18/hao.news2/haonews
go run ./cmd/haonews supporttriage --listen 127.0.0.1:51924
```

启动后打开：

- [http://127.0.0.1:51924/](http://127.0.0.1:51924/)

## 数据放哪里

默认状态文件：

- `/Users/haoniu/.hao-news/support-triage-system/state.json`

也可以自定义：

```bash
go run ./cmd/haonews supporttriage \
  --listen 127.0.0.1:51924 \
  --state /tmp/support-triage-state.json
```

## 当前能力

### 1. 工单导入

支持单条工单导入：

- 标题
- 客户
- 来源
- 优先级
- 截止时间
- 标签
- 内容

也支持批量导入：

- 页面：`批量导入工单`
- API：`POST /api/tickets/batch`

批量导入使用 `---` 分隔多条工单，每段可写：

```text
title: 客户无法登录
customer: Alice
source: 邮件
priority: high
due_at: 2026-04-15
content: 描述内容
---
title: 客户咨询开票
customer: Bob
source: 电话
priority: medium
due_at: 2026-04-16
content: 描述内容
```

### 2. 分诊与流转

当前支持：

- `review`
  - 复核并调整优先级、截止时间
- `assign`
  - 分派负责人
- `escalate`
  - 升级并记录升级原因
- `resolve`
  - 标记已解决
- `close`
  - 关闭并进入归档

状态集合：

- `open`
- `triaged`
- `assigned`
- `escalated`
- `resolved`
- `closed`
- `dropped`

### 3. 提醒与总览

当前已经内置：

- 总览统计
- 工单池筛选
- 负责人摘要
- 负责人任务视图
- 升级看板
- 提醒列表
- 最近历史

提醒口径：

- `已升级`
- `今天必须处理`
- `已逾期`
- `今日到期`
- `优先处理`
- `近期到期`
- `高优先级`

### 4. 归档与导出

关闭工单后会生成归档项，可在：

- `/archive`
- `/api/archive`

查看。

导出接口：

- Markdown：
  - `/exports/daily/latest.md`
- JSON：
  - `/exports/daily/latest.json`

## 当前页面与 API

页面：

- `/`
- `/tickets`
- `/owners`
- `/escalations`
- `/reminders`
- `/archive`

API：

- `/api/state`
- `/api/overview`
- `/api/owners`
- `/api/escalations`
- `/api/tickets`
- `/api/tickets/batch`
- `/api/tickets/{ticketID}`
- `/api/tickets/{ticketID}/review`
- `/api/tickets/{ticketID}/assign`
- `/api/tickets/{ticketID}/escalate`
- `/api/tickets/{ticketID}/resolve`
- `/api/tickets/{ticketID}/close`
- `/api/reminders`
- `/api/archive`

## 当前验证基线

已验证：

- `go test ./internal/supporttriagedesk -count=1`
- `go build ./cmd/haonews`
- 本机实机：
  - 创建工单
  - 复核
  - 指派
  - 升级
  - 负责人筛选
  - 升级看板查询
  - 解决
  - 关闭并归档
  - 批量导入
  - Markdown / JSON 导出
  - 重启后状态保留
