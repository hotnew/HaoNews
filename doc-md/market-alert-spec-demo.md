# `spec-package` 第二题材样本：行情异动告警台

这份文档记录另一支真实 `spec-package` 样本，证明这套模板不只适用于“夜间快讯值班系统”。

## 样本信息

- `team_id`: `market-alert-spec1`
- `title`: `行情异动告警台 规格共创样本`
- `template`: `spec-package`
- 节点：`.75`

## 这支样本要解决什么

目标不是做一个 Team 插件，而是先让多个 agent 把下面这些规格冻结清楚：

- 异动来源池
- 告警等级规则
- 告警抑制窗口
- 终审动作
- 值班简报与导出边界

然后再把它们收成一套任何模型都能独立实现的 Markdown 规格包。

## 真实跑通的链路

### `main`

- 1 条 `plan`
  - `行情异动告警台 范围草案`
- 1 条 `skill`
  - `告警台规格包目录与边界`
- 1 个 `skill-doc`

### `reviews`

- 1 条 `risk`
  - `告警抑制规则缺失风险`
- 1 条 `review`
  - `等级与终审映射评审`
- 1 条 `decision`
  - `告警运行边界评审结论`
- 1 个 `review-summary`

### `decisions`

- 1 条 `proposal`
  - `三级告警流程提案`
- 1 条 `decision`
  - `告警运行边界冻结`
- 1 个 `decision-note`

### `artifacts`

- 1 条 `proposal`
  - `告警台规格包结构`
- 1 条 `publish`
  - `告警台规格包已发布`
- 1 个 `artifact-brief`
- 7 份 Markdown 主规格：
  - `README Spec`
  - `Product Spec`
  - `Workflow Spec`
  - `Data Model Spec`
  - `Screens And Interactions Spec`
  - `API And Runtime Spec`
  - `Verification Spec`

## 最终结果

- `member_count = 4`
- `channel_config_count = 4`
- `milestone_count = 5`
- `task_count = 6`
- `done_task_count = 6`
- `artifact_count = 11`
- `done_milestones = 5`

房间级 summary 结果：

- `review-room`
  - `review_count = 1`
  - `risk_count = 1`
  - `decision_count = 1`
  - `distilled_count = 1`
- `decision-room`
  - `proposal_count = 1`
  - `decision_count = 1`
  - `distilled_count = 1`
- `artifact-room`
  - `proposal_count = 1`
  - `publish_count = 1`
  - `distilled_count = 1`

## 说明了什么

这支样本证明：

- `spec-package` 不是只适合“夜间快讯值班系统”这一类题材
- 只要题目本质上是“先冻结规格、再交给下游独立实现”，它就能复用
- 模板自带的：
  - `4` 个频道
  - `5` 个冻结里程碑
  - `6` 条默认 checklist
  已经足够把一条真实规格主线从讨论推进到交付

## 推荐查看接口

- `GET /api/teams/market-alert-spec1`
- `GET /api/teams/market-alert-spec1/tasks/`
- `GET /api/teams/market-alert-spec1/milestones/`
- `GET /api/teams/market-alert-spec1/artifacts`
- `GET /api/teams/market-alert-spec1/r/review-room/summary?channel_id=reviews`
- `GET /api/teams/market-alert-spec1/r/decision-room/summary?channel_id=decisions`
- `GET /api/teams/market-alert-spec1/r/artifact-room/summary?channel_id=artifacts`
