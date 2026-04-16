# 夜间快讯值班系统2 流程与状态

## 1. 主流程

标准夜班流程固定为：

1. 新增来源
2. 初筛
3. 复核
4. 终审决策
5. 进入简报或丢弃
6. 若出事故则进入事故流
7. 班次结束进入交接

## 2. 来源状态

`Source.status` 只能取以下值：

- `new`
- `triaging`
- `needs_review`
- `ready_for_decision`
- `approved`
- `deferred`
- `rejected`
- `handoff`

### 含义

- `new`
  - 新录入，尚未处理
- `triaging`
  - 正在做来源核验和基础判断
- `needs_review`
  - 需要复核员介入
- `ready_for_decision`
  - 已有足够材料，可进入终审
- `approved`
  - 已决定进入发布/简报
- `deferred`
  - 暂缓，留给早班或后续继续跟进
- `rejected`
  - 明确丢弃
- `handoff`
  - 纳入交接事项

## 3. 复核状态

`Review.status` 只能取：

- `open`
- `resolved`

`Review.kind` 只能取：

- `review`
- `risk`
- `decision-support`

## 4. 决策状态

`Decision.outcome` 只能取：

- `publish_now`
- `hold`
- `discard`
- `handoff`

这 4 个值必须直接决定下游行为：

- `publish_now`
  - 进入简报
  - 对应任务可流转到 `done` 或 `doing`
- `hold`
  - 继续观察
  - 来源保持 `deferred`
- `discard`
  - 结束处理
  - 来源转 `rejected`
- `handoff`
  - 转交接
  - 来源转 `handoff`

## 5. 事故状态

`Incident.stage` 只能取：

- `incident`
- `update`
- `recovery`

`Incident.severity` 只能取：

- `low`
- `medium`
- `high`

## 6. 交接状态

`Handoff.stage` 只能取：

- `handoff`
- `checkpoint`
- `accept`

## 7. 任务状态

`Task.status` 只能取：

- `todo`
- `doing`
- `blocked`
- `done`

## 8. 自动联动规则

这些联动必须明确写死，不能交给实现方自由发挥：

### 8.1 来源 -> 复核

当来源被标记为风险不确定时：

- `Source.status = needs_review`
- 自动创建一条 `Task`

### 8.2 决策 -> 来源

当创建一条 `Decision` 时，必须同步更新对应 `Source.status`：

- `publish_now -> approved`
- `hold -> deferred`
- `discard -> rejected`
- `handoff -> handoff`

### 8.3 决策 -> 简报

当 `Decision.outcome = publish_now`：

- 允许加入简报
- 可以自动创建或更新简报任务

### 8.4 事故 -> 任务

- `incident -> blocked`
- `update -> doing`
- `recovery -> done`

### 8.5 交接 -> 任务

- `handoff` 会生成待办
- `checkpoint` 会更新进度
- `accept` 会将对应交接任务标记完成

## 9. 去重与唯一性规则

这些规则必须写死，否则不同实现会明显分叉：

### 9.1 Source

- 一条来源只能有一个当前状态
- 一条来源可以有多条 `Review`
- 一条来源可以有多条 `Decision` 历史
- 但**只能有一条“当前生效”的最新 Decision**

### 9.2 Task

- 同一条来源触发的“复核任务”只能保留 1 条活跃任务
- 同一条事故链在同一阶段不能重复生成等价任务
- 同一条交接链在 `accept` 后不得再自动生成新的开放任务

### 9.3 Brief

- 一条来源最多被加入同一份简报一次
- `approved` 之外的来源不得进入简报

### 9.4 Archive

- 同一条最新生效 `Decision` 至少对应 1 条 `decision-note`
- 一次完整事故链至少允许生成 1 条 `incident-summary`
- 一次完整交接链至少允许生成 1 条 `handoff-summary`

## 10. 一个完整样本流程

必须能用以下样本跑通：

1. 新增来源：`深夜政策快讯`
2. 初筛后标记 `needs_review`
3. 复核员补一条 `risk`
4. 主编做一条 `Decision.outcome = publish_now`
5. 简报区新增一条夜间简报记录
6. 中途新增事故：`发布接口超时`
7. 事故完成 `recovery`
8. 夜班结束生成 `handoff`
9. 早班 `accept`
