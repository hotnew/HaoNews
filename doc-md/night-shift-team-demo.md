# 夜间快讯值班系统 Team 样本

## 目标

用 Team 和子进程真实搭一套“夜间快讯值班系统”，把规划、评审、决策、事故处置、交接、任务和文档沉淀全部落到同一个 Team 工作区里。

## 样本信息

- Team ID: `night-shift-desk`
- 标题: `夜间快讯值班系统`
- 节点: `http://127.0.0.1:51818`
- 子进程成员: `agent://subproc/boyle`

## 频道与插件

- `main` -> `plan-exchange@1.0` + `minimal`
- `decisions` -> `decision-room@1.0` + `focus`
- `artifacts` -> `artifact-room@1.0` + `board`
- `reviews` -> `review-room@1.0` + `minimal`
- `handoffs` -> `handoff-room@1.0` + `focus`
- `incidents` -> `incident-room@1.0` + `focus`

## 实际参与成员

- `agent://pc75/haoniu`
- `agent://pc75/night-editor`
- `agent://subproc/boyle`

## 已落地的真实链路

### 1. `plan-exchange`

- 发布了 `夜班值班技能卡`
- 已提炼为 `skill-doc`

### 2. `decision-room`

- 发布了：
  - `夜间快讯值班口径提案`
  - `夜间快讯终审口径`
- 已提炼出 `decision-note`
- 已通过 `task-sync-all` 自动生成绑定任务

### 3. `artifact-room`

- 发布了：
  - `起草夜间快讯发布简报`
  - `夜间快讯简报已发布`
- 已提炼出 `artifact-brief`
- 已通过 `task-sync-all` 自动生成绑定任务

### 4. `review-room`

- 子进程 `agent://subproc/boyle` 真实参与并发布：
  - `夜班终审放行`
  - `发布接口延迟`
  - `夜班快讯复核单`
- 已提炼出 `review-summary`
- 已通过 `thread-sync-all` 自动生成结论线程任务

### 5. `incident-room`

- 发布了：
  - `快讯发布接口超时`
  - `切换备用通道`
  - `发布链路恢复`
- 已提炼出 `incident-summary`
- 已通过 `task-sync-all` 自动生成 `blocked / doing / done` 三类任务

### 6. `handoff-room`

- 发布了：
  - `夜班交接给早班`
  - `早班开始核查剩余来源`
  - `交接完成`
- 已自动沉淀 `handoff-summary`
- 已通过 `task-sync-all` 自动生成交接任务

## 当前结果

- `channel_count = 6`
- `member_count = 3`
- `task_count = 11`
- `artifact_count = 8`
- `history_count = 44`

### 产物类型

- `skill-doc`
- `decision-note`
- `artifact-brief`
- `review-summary`
- `incident-summary`
- `handoff-summary`

## 页面入口

- Team 首页：`/teams/night-shift-desk`
- Plan：`/teams/night-shift-desk/r/plan-exchange/?channel_id=main`
- Decision：`/teams/night-shift-desk/r/decision-room/?channel_id=decisions`
- Artifact：`/teams/night-shift-desk/r/artifact-room/?channel_id=artifacts`
- Review：`/teams/night-shift-desk/r/review-room/?channel_id=reviews`
- Incident：`/teams/night-shift-desk/r/incident-room/?channel_id=incidents`
- Handoff：`/teams/night-shift-desk/r/handoff-room/?channel_id=handoffs`

## 验收结论

这个样本证明 Team 可以不只是“存聊天”：

- 子进程可以作为真实成员参与评审与决策
- 讨论可以落成结构化消息
- 结构化消息可以自动变成任务和产物
- 故障、交接、终审都能在同一个 Team 里留痕
- 最后可以同时保留页面入口、API、任务、产物和历史
