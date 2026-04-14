# 客服工单分诊台 `spec-package` 样本

这是一支新的真实 `spec-package` 样本：

- `team_id = support-triage-spec1`
- `title = 客服工单分诊台 规格共创样本`

它验证的是另一类独立本地程序题材也可以复用同一套 Team 上游规格共创流程，而不是只适用于夜间快讯或行情告警。

## 样本结果

- `member_count = 4`
- `channel_config_count = 4`
- `milestone_count = 5`
- `done_milestones = 5`
- `task_count = 6`
- `done_task_count = 6`
- `artifact_count = 11`
- `document_count = 7`
- `supporting_count = 4`

## 多 agent 分工

- `owner`
  - `agent://pc75/haoniu`
- `proposer`
  - `agent://spec/proposer`
- `reviewer`
  - `agent://spec/reviewer`
- `editor`
  - `agent://spec/editor`

## 产出链

### `main / plan-exchange`

- 方案草案：
  - `客服工单分诊台总体方案`
- 技能卡：
  - `规格包目录与输出要求`

### `reviews / review-room`

- `review`
  - `规格缺口评审`
- `risk`
  - `升级与 SLA 风险`
- `decision`
  - `运行时边界冻结评审结论`

### `decisions / decision-room`

- `proposal`
  - `运行时边界候选方案`
- `decision`
  - `运行时边界冻结`

### `artifacts / artifact-room`

- `proposal`
  - `规格包正文结构`
- `publish`
  - `规格包正文已发布`

## 正文规格

- [support-triage-spec-package.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/support-triage-spec-package.md)

正文固定包括：

- `README Spec`
- `Product Spec`
- `Workflow Spec`
- `Data Model Spec`
- `Screens And Interactions Spec`
- `API And Runtime Spec`
- `Verification Spec`

## 导出方式

- JSON：
  - `GET /api/teams/support-triage-spec1/artifacts/export?profile=spec-package`
- Markdown：
  - `GET /api/teams/support-triage-spec1/artifacts/export?profile=spec-package&format=markdown`

这说明 Team 当前已经能作为上游多 agent 协作空间，把讨论、评审、冻结和正文沉淀收成一份可直接交给下游实现的 Markdown 规格包。
