# 夜间快讯值班系统2 API 契约

## 1. 目标

这个文件不是要求必须做 REST 才算完成，而是为了把**动作语义**钉死。

即使实现方不用完全相同的接口路径，也必须保证等价动作存在，且输入输出语义一致。

## 2. 读取接口

### `GET /api/state`

返回整个系统当前快照，至少包括：

- `sources`
- `reviews`
- `decisions`
- `briefs`
- `incidents`
- `handoffs`
- `tasks`
- `archive`
- `events`

### `GET /api/sources`

支持按状态筛选：

- `status`

### `GET /api/tasks`

支持按状态筛选：

- `status`

## 3. 写入动作

### `POST /api/sources`

作用：

- 新增来源

最小字段：

- `title`
- `summary`
- `source_name`
- `source_url`
- `credibility`

默认行为：

- 新来源创建后状态为 `new`

### `POST /api/sources/{id}/status`

作用：

- 更新来源状态

允许值：

- `triaging`
- `needs_review`
- `ready_for_decision`
- `approved`
- `deferred`
- `rejected`
- `handoff`

### `POST /api/reviews`

作用：

- 新增 review / risk / decision-support

最小字段：

- `source_id`
- `kind`
- `title`
- `body`
- `actor`

默认行为：

- 新 review 创建后状态为 `open`

### `POST /api/decisions`

作用：

- 形成正式终审结论

最小字段：

- `source_id`
- `outcome`
- `title`
- `body`

自动行为：

- 更新对应 `Source.status`
- 生成或更新相关 `Task`
- 生成 `decision-note`
- 写入事件流

### `POST /api/briefs`

作用：

- 新建或更新夜间简报

最小字段：

- `title`
- `status`
- `items`
- `summary`

自动约束：

- `items` 中每个来源必须已 `approved`

### `POST /api/incidents`

作用：

- 写入事故链事件

最小字段：

- `stage`
- `severity`
- `title`
- `body`

自动行为：

- 生成或更新事故相关任务
- 写入事件流
- `recovery` 阶段允许自动生成 `incident-summary`

### `POST /api/handoffs`

作用：

- 写入交接链事件

最小字段：

- `stage`
- `title`
- `body`

自动行为：

- 生成或更新交接任务
- 写入事件流
- `accept` 阶段允许自动生成 `handoff-summary`

### `POST /api/tasks/{id}/status`

作用：

- 更新任务状态

允许值：

- `todo`
- `doing`
- `blocked`
- `done`

## 4. 输出契约

所有写入动作完成后，必须至少满足：

- 状态已持久化
- 最近动态可见
- 相关对象之间能互相追溯

## 5. 失败语义

以下情况必须返回明确错误，不允许静默吞掉：

- 引用不存在的 `source_id`
- 给未 `approved` 的来源创建简报
- 传入不合法状态值
- 更新不存在的任务

## 6. 默认安全约束

如果规格没有明确要求自动创建对象，默认不要偷偷创建。

仅下面动作允许自动生成对象：

- `Decision` 自动生成 `decision-note`
- `Incident.recovery` 自动生成 `incident-summary`
- `Handoff.accept` 自动生成 `handoff-summary`
- 风险/事故/交接动作自动创建或更新相关任务
