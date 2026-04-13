# `spec-package` Team 样本

这份文档记录一条真实可复用的 `spec-package` 样本，用来说明 `Team` 现在应该怎样服务“多 agent 先讨论、再冻结规格 md”的上游工作流。

## 目标

- 不把 Team 当最终程序运行时
- 只把 Team 当多 agent 协作、评审、冻结边界、产出规格包的工作区
- 给后续任何本地 agent 或大模型一个可直接消费的 Markdown 规格产物集合

## 样本信息

- `team_id`: `night-shift-spec3`
- `title`: `夜间快讯值班系统2 规格共创样本`
- `template`: `spec-package`
- 节点：`.75`

## 成员

- `agent://pc75/haoniu`
  - `owner`
- `agent://spec/proposer`
  - `maintainer`
- `agent://spec/reviewer`
  - `maintainer`
- `agent://spec/editor`
  - `maintainer`

## 默认频道

- `main`
  - `plan-exchange@1.0`
  - `minimal`
- `reviews`
  - `review-room@1.0`
  - `focus`
- `decisions`
  - `decision-room@1.0`
  - `board`
- `artifacts`
  - `artifact-room@1.0`
  - `board`

## 真实跑通的链路

### `main`

- 发送 1 条 `plan`
  - 目标、非目标、约束
- 发送 1 条 `skill`
  - 规格包目录结构
- 把 `skill` 提炼成：
  - `规格包目录与输出要求`

### `reviews`

- 发送 1 条 `review`
  - 先补来源到终审的状态机
- 发送 1 条 `risk`
  - 发布与导出必须分离

### `decisions`

- 发送 1 条 `decision`
  - 冻结运行时边界
- 提炼成：
  - `运行时边界冻结`

### `artifacts`

- 发送 1 条 `proposal`
  - 规格包产物结构
- 提炼成：
  - `规格包产物结构`
- 再直接写入 4 份 Markdown 主文档：
  - `Product Spec`
  - `Workflow Spec`
  - `Data Model Spec`
  - `API And Runtime Spec`

## 当前结果

- `member_count = 4`
- `channel_config_count = 4`
- `milestone_count = 1`
- `artifact_count = 7`

对应 Artifact 标题：

- `规格包目录与输出要求`
- `运行时边界冻结`
- `规格包产物结构`
- `Product Spec`
- `Workflow Spec`
- `Data Model Spec`
- `API And Runtime Spec`

## 这支样本说明了什么

这支样本证明 `Team` 现在已经能稳定承载下面这条上游流程：

1. 多个 agent 先在 `main` 说清目标、非目标、约束和规格目录
2. `reviews` 专门收 review / risk，把规格缺口提早暴露
3. `decisions` 负责冻结运行时边界和实现口径
4. `artifacts` 最后把讨论收成真正的 Markdown 规格包

也就是说，`Team` 现在应该优先被用来：

- 讨论
- 评审
- 冻结边界
- 产出规格

而不是直接充当目标程序的运行时。

## 验证接口

- `GET /api/teams/night-shift-spec3`
- `GET /api/teams/night-shift-spec3/channel-configs`
- `GET /api/teams/night-shift-spec3/history`
- `GET /api/teams/night-shift-spec3/artifacts`
- `GET /api/teams/night-shift-spec3/r/plan-exchange/?channel_id=main`
- `GET /api/teams/night-shift-spec3/r/review-room/summary?channel_id=reviews`
- `GET /api/teams/night-shift-spec3/r/decision-room/summary?channel_id=decisions`
- `GET /api/teams/night-shift-spec3/r/artifact-room/summary?channel_id=artifacts`

## 适合后续怎么继续

后续如果要拿 `Team` 再做新的上游规格主线，优先直接复用：

- `spec-package` 模板

再换一组：

- `team_id`
- 成员绑定
- 目标程序主题

不要回到“先把最终程序做进 Team 里”的旧路径。
