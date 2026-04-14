# 会议纪要整理与行动项生成台 项目总结

## 项目目标

做一套**独立本地程序**，把会议原文整理成：

- 可校对的纪要
- 可跟踪的行动项
- 可发布的归档
- 可导出的 Markdown / JSON

这条线的目标一直是：

- 与 Team 运行时解耦
- 单机可跑
- 文件持久化
- 任何大模型都能根据规格包继续实现或扩展

## 当前完成

当前程序已经具备：

- 单会议导入
- 批量导入会议稿
- 自动抽取摘要 / 议题 / 决议 / 行动项
- 人工校对与 revision 记录
- 行动项新增、状态流转、负责人视图
- 会议发布与归档
- Markdown / JSON 导出
- 搜索、过滤、排序、分页
- 提醒视图
- 负责人提醒聚合
- 多会议总览

## 运行入口

- CLI：
  - [/Users/haoniu/sh18/hao.news2/haonews/cmd/haonews/main.go](/Users/haoniu/sh18/hao.news2/haonews/cmd/haonews/main.go)
- 实现：
  - [/Users/haoniu/sh18/hao.news2/haonews/internal/meetingnotesdesk/server.go](/Users/haoniu/sh18/hao.news2/haonews/internal/meetingnotesdesk/server.go)
- 模板：
  - [/Users/haoniu/sh18/hao.news2/haonews/internal/meetingnotesdesk/templates/index.html](/Users/haoniu/sh18/hao.news2/haonews/internal/meetingnotesdesk/templates/index.html)
- 测试：
  - [/Users/haoniu/sh18/hao.news2/haonews/internal/meetingnotesdesk/server_test.go](/Users/haoniu/sh18/hao.news2/haonews/internal/meetingnotesdesk/server_test.go)

命令：

```bash
cd /Users/haoniu/sh18/hao.news2/haonews
go run ./cmd/haonews meetingnotes --listen 127.0.0.1:51923
```

## 核心页面和 API

页面：

- `/`
- `/meetings`
- `/tasks`
- `/owners`
- `/reminders`
- `/archive`

API：

- `/api/state`
- `/api/overview`
- `/api/meetings`
- `/api/meetings/batch`
- `/api/meetings/{meetingID}`
- `/api/tasks`
- `/api/owners`
- `/api/reminders`
- `/api/archive`

导出：

- `/exports/meeting/latest.md`
- `/exports/meeting/latest.json`

## 当前最重要的产品结果

### 1. 批量导入

支持把多场会议稿一次性导入，使用 `---` 分段。

导入结果会回显：

- `imported`
- `skipped`

### 2. 列表体验

会议列表现在支持：

- 搜索
- 排序
- 分页

行动项列表现在支持：

- 搜索
- 负责人过滤
- 状态过滤
- 会议过滤
- 截止时间优先排序
- 优先级优先排序

### 3. 提醒系统

提醒现在已经不只是简单罗列：

- `立即处理`
- `已逾期`
- `今日必须处理`
- `近期到期`
- `高优先级`

同时会给出负责人维度聚合，方便先看谁最堵。

### 4. 批量导入回显

批量导入不再是“只导入不解释”，现在会返回：

- `imported`
- `skipped`
- `errors`

## 与 Team 的关系

这套程序不是 Team 房间。

Team 在这条主线里的作用是：

- 多 agent 讨论
- 评审
- 冻结边界
- 产出规格 md

下游本地程序只消费规格包，不依赖 Team 运行时。

对应规格包：

- [/Users/haoniu/sh18/hao.news2/haonews/doc-md/meeting-notes-spec-package.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/meeting-notes-spec-package.md)

## 验证基线

已稳定使用的验证：

- `go test ./internal/meetingnotesdesk -count=1`
- `go build ./cmd/haonews`
- 本机实跑：
  - 导入会议
  - 批量导入
  - 列表分页
  - 行动项排序
  - 发布归档
  - 导出
  - 重启后状态保留

## 当前状态

这条线现在已经不是概念样品，而是可直接运行的本地工具。

后续如果继续，重点不再是补主干，而是：

- 转写稿批量导入体验再优化
- 更细的提醒规则
- 更完整的会议列表浏览体验
