# 会议纪要整理与行动项生成台 本地程序

## 这是什么

这是一套**与 Team 运行时解耦**的本地程序。

它不是 Team 里的一个房间或插件，而是根据：

- [/Users/haoniu/sh18/hao.news2/haonews/doc-md/meeting-notes-spec-package.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/meeting-notes-spec-package.md)

独立实现出来的本地 Web 应用。

## 怎么启动

在仓库根目录运行：

```bash
cd /Users/haoniu/sh18/hao.news2/haonews
go run ./cmd/haonews meetingnotes --listen 127.0.0.1:51923
```

启动后打开：

- [http://127.0.0.1:51923/](http://127.0.0.1:51923/)

## 数据放哪里

默认状态文件：

- `/Users/haoniu/.hao-news/meeting-notes-system/state.json`

本次实测使用的是：

- `/Users/haoniu/.hao-news/meeting-notes-system/e2e-state.json`

也可以自定义：

```bash
go run ./cmd/haonews meetingnotes \
  --listen 127.0.0.1:51923 \
  --state /tmp/meeting-notes-state.json
```

## 当前能力

### 1. 会议导入

可以直接导入会议原文：

- 标题
- 参会人
- 原始会议文本

导入后会自动抽取：

- 摘要
- 议题
- 决议
- 行动项

也支持批量导入多场会议：

- 页面：`批量导入多场会议`
- API：`POST /api/meetings/batch`

批量导入使用 `---` 分隔多场会议，每段可写：

```text
标题: 早会
参与人: 张三, 李四
议题: 发布检查
行动: 核对发布单 | 张三 | 2026-04-20 | high
---
标题: 午会
参与人: 王五
议题: 缺陷复盘
行动: 补回归用例 | 王五 | 2026-04-21 | medium
```

### 2. 人工校对

可以继续人工编辑：

- 摘要
- 议题
- 决议
- 原始文本

每次更新都会留下 revision 记录。

### 3. 行动项管理

现在可以直接：

- 新增行动项
- 更新行动项状态
- 按负责人筛选任务
- 按关键词、状态、会议过滤任务
- 查看负责人汇总视图

当前状态支持：

- `open`
- `confirmed`
- `done`
- `dropped`

### 4. 发布与归档

会议纪要可以从 `draft` 发布为 `published`。

发布后会自动生成归档项：

- `meeting-summary`

并可在：

- `/archive`
- `/api/archive`

查看。

### 5. 导出

支持导出最新会议：

- Markdown：
  - `/exports/meeting/latest.md`
- JSON：
  - `/exports/meeting/latest.json`

### 6. 独立页面与 API

页面：

- `/`
- `/meetings`
- `/tasks`
- `/owners`
- `/reminders`
- `/archive`

首页现在已经是多会议汇总台，会直接展示：

- 会议总数 / 草稿 / 已发布
- 行动项状态分布
- 最近会议
- 负责人摘要
- 当前提醒

API：

- `/api/state`
- `/api/overview`
- `/api/meetings`
- `/api/meetings/{meetingID}`
- `/api/tasks`
- `/api/owners`
- `/api/reminders`
- `/api/archive`

其中筛选已经可用：

- `/api/meetings?q=关键词`
- `/api/tasks?q=关键词&owner=张三&status=confirmed`

排序和分页也已经可用：

- `/api/meetings?sort=title_asc&page=1&page_size=10`
- `/api/tasks?sort=due_asc`

会议排序支持：

- `updated_desc`
- `created_desc`
- `title_asc`
- `status`

任务排序支持：

- `status`
- `due_asc`
- `priority`
- `updated_desc`

`/api/tasks` 还会返回：

- `owners`
- `board`

用于展示负责人任务汇总。

`/owners` 和 `/api/owners` 用于：

- 按负责人查看行动项
- 按负责人再叠加状态、会议、关键词过滤

`/tasks` 页面现在也有按状态分列的看板：

- `Open`
- `Confirmed`
- `Done`
- `Dropped`

`/reminders` 和 `/api/reminders` 用于：

- 查看已逾期任务
- 查看今日到期和近期到期任务
- 查看高优先级提醒
- 查看“立即处理”任务
- 查看负责人提醒聚合

`/api/overview` 现在还会返回：

- `reminder_owners`

动作入口：

- `/actions/meeting/import`
- `/actions/meeting/import-batch`
- `/actions/meeting/regenerate`
- `/actions/meeting/update`
- `/actions/action-item`
- `/actions/action-item-status`
- `/actions/meeting/publish`

## 已完成的真实验证

本地已实测跑通一条完整链：

1. 导入会议 `项目晨会 04-13`
2. 自动生成 3 条行动项
3. 人工校对摘要、议题、决议
4. 将行动项 `action-1` 更新为 `confirmed`
5. 发布纪要并生成归档
6. 导出 Markdown 和 JSON
7. 重启进程后重新读取同一状态文件
8. 批量导入两场会议稿并跳过 1 段无效内容
9. `/api/meetings?sort=title_asc&page=1&page_size=1`
10. `/api/tasks?sort=due_asc`
11. `/api/reminders`

重启后仍可看到：

- `meeting_count = 1`
- `task_count = 3`
- `archive_count = 1`

## 与 Team 的关系

`Team` 只用于前期多 agent 协作、评审和产出规格 md。

这个程序本身：

- 不依赖 Team 页面
- 不依赖 Team API
- 不依赖 Team 数据结构
- 只把规格包当设计输入

## 当前代码位置

- CLI 入口：
  - [/Users/haoniu/sh18/hao.news2/haonews/cmd/haonews/main.go](/Users/haoniu/sh18/hao.news2/haonews/cmd/haonews/main.go)
- 程序实现：
  - [/Users/haoniu/sh18/hao.news2/haonews/internal/meetingnotesdesk/server.go](/Users/haoniu/sh18/hao.news2/haonews/internal/meetingnotesdesk/server.go)
- 页面模板：
  - [/Users/haoniu/sh18/hao.news2/haonews/internal/meetingnotesdesk/templates/index.html](/Users/haoniu/sh18/hao.news2/haonews/internal/meetingnotesdesk/templates/index.html)
- 测试：
  - [/Users/haoniu/sh18/hao.news2/haonews/internal/meetingnotesdesk/server_test.go](/Users/haoniu/sh18/hao.news2/haonews/internal/meetingnotesdesk/server_test.go)
