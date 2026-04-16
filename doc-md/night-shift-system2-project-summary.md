# 夜间快讯值班系统2 项目整合说明

## 1. 这是什么

`夜间快讯值班系统2` 是一个**独立本地运行**的值班系统。

它的上游输入来自：

- Team 中多 agent 协作形成的规格 md

但它自己的运行时：

- 不依赖 Team 页面
- 不依赖 Team API
- 不依赖 Team 数据文件

一句话：

**Team 负责共创规格，`night-shift-system2` 负责独立实现本地程序。**

## 2. 现在已经完成了什么

当前已经完成：

- 独立 CLI 入口：
  - `haonews nightshift`
- 独立状态文件：
  - `~/.hao-news/night-shift-system2/state.json`
- 本地 Web 页面：
  - `/`
  - `/sources`
  - `/reviews`
  - `/decisions`
  - `/briefs`
  - `/incidents`
  - `/handoffs`
  - `/tasks`
  - `/archive`
- 独立对象主链：
  - `sources`
  - `reviews`
  - `decisions`
  - `incidents`
  - `handoffs`
  - `briefs`
  - `tasks`
  - `archive`
  - `history`
- Markdown 导出：
  - 最新夜间简报
  - 最新交接摘要

## 3. 已落地的关键工作流

### 3.1 来源池

支持：

- 新增来源
- 状态流转：
  - `new`
  - `triaging`
  - `needs_review`
  - `ready_for_decision`
  - `approved`
  - `deferred`
  - `rejected`
  - `handoff`

### 3.2 风险复核

支持：

- `review`
- `risk`
- `decision-support`

状态：

- `open`
- `resolved`

### 3.3 终审决策

支持：

- `publish_now`
- `hold`
- `discard`
- `handoff`

自动联动：

- 更新来源状态
- 更新任务状态
- 生成 `decision-note`

### 3.4 事故处置

支持：

- `incident`
- `update`
- `recovery`

自动联动：

- `incident -> blocked`
- `update -> doing`
- `recovery -> done`
- `recovery` 自动生成 `incident-summary`

### 3.5 交接

支持：

- `handoff`
- `checkpoint`
- `accept`

自动联动：

- 交接任务状态变化
- `accept` 自动生成 `handoff-summary`

### 3.6 简报

支持：

- 生成 `brief`
- `draft / published`
- Markdown 导出

约束：

- 只收录 `approved` 来源

## 4. 验证结果

本轮已经完成的验证：

- `go test ./internal/nightshiftdesk -count=1`
- `go build ./cmd/haonews`
- 本地独立启动：
  - `go run ./cmd/haonews nightshift --listen 127.0.0.1:51922 --state ~/.hao-news/night-shift-system2/e2e-state.json`
- 页面访问通过：
  - `/`
  - `/sources`
  - `/reviews`
  - `/decisions`
  - `/briefs`
  - `/incidents`
  - `/handoffs`
  - `/tasks`
  - `/archive`
- 真实动作链通过：
  - 新增来源
  - 来源进入 `needs_review`
  - 新增并解决 `risk`
  - 新增 `publish_now` 决策
  - 新增事故并推进到 `recovery`
  - 新增交接并推进到 `accept`
  - 生成并导出 Markdown 简报
  - 重启后状态仍在

## 5. 与 Team 的正确关系

这条主线最关键的纠偏已经完成：

- `Team` 不再作为这个程序的运行时承载
- `Team` 只保留上游协作、讨论、评审、沉淀规格的定位
- 下游程序按 md 独立实现

这也是后面继续复用的标准模式：

1. 用 Team 做多 agent 共创
2. 产出规格包
3. 再独立做本地程序

## 6. 关联文档

- 规格包入口：
  - [night-shift-system2/README.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/README.md)
- 本地程序说明：
  - [night-shift-local-app.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-local-app.md)
- Team 上游定位：
  - [team-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-project-summary.md)
