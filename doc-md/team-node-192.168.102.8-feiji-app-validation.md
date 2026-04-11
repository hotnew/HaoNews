# 192.168.102.8 Team 实测记录：`feiji-app`

## 目标

在 `192.168.102.8` 节点上，不依赖 `.74`，直接用本机账号和本机 API 验证 `haonews Team` 的真实可用性，而不是只做空接口检查。

## 节点基线

- Host: `192.168.102.8`
- User: `haoniu`
- Runtime:
  - `haonews serve`
  - `hao-news-syncd sync`
- Bootstrap:
  - `readiness.stage = ready`
  - `redis.enabled = false`

## 实测团队

- `team_id = feiji-app`
- `title = 飞机 App`
- `owner_agent_id = agent://pc8/haoniu`
- 创建方式：
  - `POST /api/teams?from_template=planning`

## 已验证链路

### 1. Team 创建与索引

- Team 创建成功。
- `GET /api/teams/feiji-app` 可返回完整详情。
- `GET /api/teams` 最终可返回：
  - `count = 1`
  - `feiji-app`

说明：
- Team 刚创建后，`/api/teams` 曾短暂返回 `count = 0`
- 随后恢复为正常索引结果

### 2. 模板房间与频道

模板创建后自动带出：

- `main` -> `plan-exchange@1.0` + `minimal`
- `decisions` -> `decision-room@1.0` + `focus`
- `artifacts` -> `artifact-room@1.0` + `board`

后续补建并配置：

- `handoffs` -> `handoff-room@1.0` + `focus`
- `reviews` -> `review-room@1.0` + `minimal`
- `incidents` -> `incident-room@1.0` + `focus`

当前频道总数：

- `channel_count = 6`

### 3. `plan-exchange`

已验证：

- `POST /api/teams/feiji-app/r/plan-exchange/messages`
- 页面：
  - `/teams/feiji-app/r/plan-exchange/?channel_id=main`

写入内容：

- `kind = plan`
- 内容为“飞机 App MVP 规划”

结论：

- 规划房间页面正常渲染
- 消息成功进入 `main` 频道

### 4. Team Task 主链

手工创建任务：

- `task_id = feiji-mvp-ui`
- `title = 完成飞机 App 首页和航班列表原型`

任务主链当前共 `7` 个任务，包含：

- 手工创建的 MVP 任务
- `decision-room` 自动生成任务
- `handoff-room` 自动生成任务
- `incident-room` 自动生成任务 3 个
- `review-room` 线程同步自动生成任务

### 5. `artifact-room`

已验证：

- `POST /api/teams/feiji-app/r/artifact-room/messages`
- `POST /api/teams/feiji-app/r/artifact-room/distill`
- `GET /api/teams/feiji-app/r/artifact-room/summary?channel_id=artifacts`

写入内容：

- `kind = proposal`
- 标题：`航班卡片方案`

结果：

- `proposal_count = 1`
- `distilled_count = 1`
- 成功生成：
  - `artifact-brief`

### 6. `handoff-room`

已验证：

- `POST /api/teams/feiji-app/r/handoff-room/messages`
- `POST /api/teams/feiji-app/r/handoff-room/task-sync-all`
- `GET /api/teams/feiji-app/r/handoff-room/summary?channel_id=handoffs`

写入内容：

- `kind = handoff`
- 内容：把航班列表页交给前端实现

结果：

- `handoff_count = 1`
- `bound_task_count = 1`
- `task_created = 1`
- 自动生成了交接任务

### 7. `review-room`

已验证：

- `POST /api/teams/feiji-app/r/review-room/messages`
- `POST /api/teams/feiji-app/r/review-room/distill`
- `POST /api/teams/feiji-app/r/review-room/thread-sync-all`
- `GET /api/teams/feiji-app/r/review-room/summary?channel_id=reviews`

写入内容：

- `review`
- `risk`
- `decision`

结果：

- `review_count = 1`
- `risk_count = 1`
- `decision_count = 1`
- `distilled_count = 2`
- 线程已进入：
  - `workflow_state = completed`
- 自动绑定：
  - 1 个 Task
  - 1 个 `review-summary`

### 8. `incident-room`

已验证：

- `POST /api/teams/feiji-app/r/incident-room/messages`
- `POST /api/teams/feiji-app/r/incident-room/task-sync-all`
- `POST /api/teams/feiji-app/r/incident-room/distill`
- `GET /api/teams/feiji-app/r/incident-room/summary?channel_id=incidents`

写入内容：

- `incident`
- `update`
- `recovery`

结果：

- `incident_count = 1`
- `update_count = 1`
- `recovery_count = 1`
- `distilled_count = 1`
- 自动生成任务 3 个：
  - `incident -> blocked`
  - `update -> doing`
  - `recovery -> done`
- 自动生成：
  - `incident-summary`

## 当前汇总

- Team: `1`
- Channels: `6`
- Tasks: `7`
- Artifacts: `4`

说明：

- 这台节点现在已经不是“空运行态”
- `feiji-app` 已可作为后续 Team / Room Plugin 的真实验收样本

## 真实发现

### 发现 1：`decision-room` 刚创建后 summary / batch 口径不稳定

现象：

- 早先 `decision-room` 的底层消息已经存在
- 但第一次：
  - `GET /summary`
  - `POST /task-sync-all`
  返回值仍像是 `0`

后续再次调用后恢复正常。

判断：

- 更像聚合层/批量口径问题
- 不是消息没写进去

### 发现 2：`decision-room task-sync-all` 会重复建任务

现象：

- 在已有单条 `task-sync` 生成任务后
- 再执行一次 `task-sync-all`
- 同一条 `decision` 又生成了一个新的 `done` 任务

当前 `decisions` 频道任务里出现了两条同标题任务：

- `范围收敛`
- `范围收敛`

判断：

- 这是当前最明确的功能性问题
- `decision-room` 的批量同步没有稳定复用既有绑定任务

建议后续修复目标：

- `task-sync-all` 应优先复用已存在的绑定 Task
- 不应对同一 `decision` 反复创建重复 Task

## 修复后复核

后续已在本地修复 `decision-room task-sync-all` 的复用逻辑，并将修复后的 Linux 二进制部署到 `192.168.102.8` 重新验证。

复核方法：

- 对 `decisions` 频道再次执行：
  - `POST /api/teams/feiji-app/r/decision-room/task-sync-all`
- 对比执行前后：
  - `GET /api/teams/feiji-app/tasks?channel=decisions`

复核结果：

- 执行前：
  - `task_count = 2`
- 本次 `task-sync-all` 响应：
  - `task_created = 0`
  - `artifact_created = 0`
  - `synced_items = 1`
- 执行后：
  - `task_count = 2`

## `.75 <- 192.168.102.8` TeamSync 复核

后续继续把 `.75` 和 `192.168.102.8` 之间的 TeamSync 前提补齐并做了真实双端复核。

### 前提修正

- `.75` 本地补了 `feiji-app` team stub，否则当前 TeamSync 不会对未知 team 自动建立订阅。
- `.75` 的 `hao_news_net.inf` 增加了：
  - `lan_peer=192.168.102.8`
- `.75` 本机重启后重新建立了对 `.8` 的 LAN 同步链。

### 实测同步结果

`.75` 最终已真实收到并落盘：

- `members = 2`
  - `agent://pc8/haoniu`
  - `agent://pc8/helper`
- `channel_config_count = 6`
  - `main`
  - `decisions`
  - `artifacts`
  - `handoffs`
  - `incidents`
  - `reviews`
- `tasks = 9`
- `artifacts = 5`

说明：

- `task / history / members / channel_config` 都已经从 `.8` 到达 `.75`
- `feiji-app` 不再只是远端单机样本，而是已经能在 `.75` 上作为真实 Team 样本继续验证

### 额外修复 1：`agent://...` 这类 ID 的路由解析

在真实节点数据里，`task_id / artifact_id` 经常带：

- `agent://pc8/haoniu`

如果 Team 路由继续按 `r.URL.Path` 直接 `Split("/")`，那么：

- `/teams/{teamID}/tasks/{taskID}`
- `/api/teams/{teamID}/artifacts/{artifactID}`

会把 `%2F` 还原成 `/`，导致明明数据已存在，详情页/API 仍可能 `404`。

这条已修复为按 `EscapedPath` 分段解析后再逐段 `PathUnescape`。

修复后复核：

- `.75` 上带 `agent://...` 的任务详情页已可正常打开
- `.75` 上带 `agent://...` 的 Artifact API 已可正常按 ID 命中

### 额外修复 2：Artifact kind 被错误降级为 `markdown`

在 `.8 -> .75` 的真实同步里，曾出现：

- `decision-note`
- `review-summary`
- `incident-summary`
- `artifact-brief`

这些房间插件产物在 `.75` 上被错误写成统一的 `markdown`。

根因有两层：

1. `normalizeArtifactKind()` 对未知 kind 默认回退到 `markdown`
2. `.75` 本机曾出现：
   - `haonews serve` 已升级
   - `hao-news-syncd sync` 仍是旧二进制
   导致“服务看起来是新版本，但同步接收行为还是旧版本”的混合态

修复后复核：

- `.75` 本机 `serve` 和 `syncd` 已统一切到同版二进制
- `.8` 上现有 `feiji-app` artifacts 重新保存/重放后，`.75` 列表结果恢复为真实 kind：
  - `artifact-brief`
  - `incident-summary`
  - `review-summary`
  - `decision-note`

### 当前结论

现在 `feiji-app` 已经同时满足：

- `192.168.102.8` 侧真实业务样本
- `.75` 侧真实同步样本

后续再做 Team / Room Plugin / TeamSync 验收时，可以直接复用这个样本，而不需要再重新搭一套空 Team。

结论：

- 旧重复任务仍作为历史数据保留
- 但修复后的版本已经不会继续为同一条 `decision` 生成第 3 条重复任务

## 建议用途

后续可以直接拿这个 Team 做：

- Room Plugin 回归验证
- Team Task / Artifact / History 联动验证
- 多频道、多房间样例展示
- 节点升级后最小验收
