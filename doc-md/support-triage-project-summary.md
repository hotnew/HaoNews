# 客服工单分诊台 项目总结

## 项目目标

做一套**独立本地程序**，把客服工单从导入到分诊、升级、解决、归档收成一个单机可运行的 Web 工具。

目标一直是：

- 与 Team 运行时解耦
- 单机可跑
- 文件持久化
- 任何大模型都能根据规格包继续实现或扩展

## 当前完成

当前程序已经具备：

- 单条工单导入
- 批量导入工单
- 优先级分诊
- 负责人分派
- 负责人任务视图
- 升级流转
- 升级看板
- 解决与关闭
- 归档
- Markdown / JSON 导出
- 工单筛选与排序
- 负责人摘要
- 提醒视图
- 总览页

## 运行入口

- CLI：
  - [/Users/haoniu/sh18/hao.news2/haonews/cmd/haonews/main.go](/Users/haoniu/sh18/hao.news2/haonews/cmd/haonews/main.go)
- 实现：
  - [/Users/haoniu/sh18/hao.news2/haonews/internal/supporttriagedesk/server.go](/Users/haoniu/sh18/hao.news2/haonews/internal/supporttriagedesk/server.go)
- 模板：
  - [/Users/haoniu/sh18/hao.news2/haonews/internal/supporttriagedesk/templates/index.html](/Users/haoniu/sh18/hao.news2/haonews/internal/supporttriagedesk/templates/index.html)
- 测试：
  - [/Users/haoniu/sh18/hao.news2/haonews/internal/supporttriagedesk/server_test.go](/Users/haoniu/sh18/hao.news2/haonews/internal/supporttriagedesk/server_test.go)

命令：

```bash
cd /Users/haoniu/sh18/hao.news2/haonews
go run ./cmd/haonews supporttriage --listen 127.0.0.1:51924
```

## 核心页面和 API

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

导出：

- `/exports/daily/latest.md`
- `/exports/daily/latest.json`

## 与 Team 的关系

这套程序不是 Team 房间。

Team 在这条主线里的作用是：

- 多 agent 讨论
- 评审
- 冻结边界
- 产出规格 md

下游本地程序只消费规格包，不依赖 Team 运行时。

对应规格包：

- [/Users/haoniu/sh18/hao.news2/haonews/doc-md/support-triage-spec-package.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/support-triage-spec-package.md)

## 验证基线

已完成：

- `go test ./internal/supporttriagedesk -count=1`
- `go build ./cmd/haonews`
- 本机实跑：
  - 创建工单
  - 复核
  - 分派
  - 升级
  - 负责人视图
  - 升级看板
  - 解决
  - 关闭并归档
  - 批量导入
  - 导出
  - 重启后状态保留

## 当前状态

这条线已经不是规格样品，而是可直接运行的本地工具。

后续如果继续，重点不再是补主干，而是：

- 更强的工单搜索和分页
- 更细的 SLA 策略
- 批量动作和更强的归档检索
