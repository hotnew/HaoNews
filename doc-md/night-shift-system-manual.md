# 夜间快讯值班系统操作手册

## 1. 系统定位

这套系统不是单独的新服务，而是基于 `haonews Team` 的一个完整值班工作区。

它把夜班快讯的几类核心动作放到同一个 Team 里：

- 来源核验
- 终审结论
- 简报沉淀
- 风险复核
- 故障处置
- 早晚班交接

当前对应的真实 Team 样本：

- Team ID: `night-shift-desk`
- 页面入口：`/teams/night-shift-desk`

## 2. 频道分工

### `main`

用途：

- 记录值班技能卡
- 记录值班规则
- 记录短期执行步骤

插件：

- `plan-exchange@1.0`

适合放：

- 值班 SOP
- 快讯筛选标准
- “先做什么、后做什么”的技能卡

### `decisions`

用途：

- 记录夜班正式口径
- 记录哪些内容先发、哪些暂缓

插件：

- `decision-room@1.0`

适合放：

- 终审结论
- 发布口径
- 值班主编的正式决定

### `artifacts`

用途：

- 沉淀夜间简报和发布说明

插件：

- `artifact-room@1.0`

适合放：

- 夜间简报
- 复盘简报
- 早班接手材料

### `reviews`

用途：

- 记录 review / risk / decision
- 让子进程或复核员参与

插件：

- `review-room@1.0`

适合放：

- 复核意见
- 风险说明
- 临时决策线程

### `incidents`

用途：

- 记录夜间突发故障
- 跟踪恢复过程

插件：

- `incident-room@1.0`

适合放：

- 发布接口异常
- 推送延迟
- 备用通道切换

### `handoffs`

用途：

- 记录夜班到早班的交接

插件：

- `handoff-room@1.0`

适合放：

- 待确认来源
- 早班继续跟进事项
- 交接完成标记

## 3. 一次标准值班流程

### 第一步：先在 `main` 确认值班技能和处理步骤

例如：

- 先核来源
- 再定口径
- 最后做交接

这一步的产物可以沉淀为：

- `skill-doc`

### 第二步：在 `reviews` 让复核员或子进程参与

至少形成三类信息：

- `review`
- `risk`
- `decision`

这一步的作用是把“是不是该发、风险在哪、有没有遗漏”说清楚。

### 第三步：在 `decisions` 形成正式发布口径

例如：

- 已核实内容先发
- 争议来源暂缓
- 早班继续补核实

这一步会沉淀为：

- `decision-note`

并可自动挂到任务主链。

### 第四步：在 `artifacts` 形成夜间简报

例如：

- 已发快讯
- 待确认来源
- 交接事项

这一步会沉淀为：

- `artifact-brief`

### 第五步：如果出故障，就走 `incidents`

标准三段：

- `incident`
- `update`
- `recovery`

这一步既会留下事故历史，也会自动生成对应任务和 `incident-summary`。

### 第六步：值班结束时走 `handoffs`

标准三段：

- `handoff`
- `checkpoint`
- `accept`

这样夜班退出前，早班要接什么、做到哪一步、是否真正接住，都能留在主链里。

## 4. 这个系统已经真实跑通的结果

当前 `night-shift-desk` 已经真实落下：

- `member_count = 3`
- `channel_count = 6`
- `channel_config_count = 6`
- `task_count = 11`
- `artifact_count = 8`

当前真实 artifact 类型包括：

- `skill-doc`
- `decision-note`
- `artifact-brief`
- `review-summary`
- `incident-summary`
- `handoff-summary`

## 5. 子进程怎么参与

子进程最适合参与的是：

- `review-room`
- `decision-room`

推荐角色：

- 复核员
- 风险挑战者
- 夜班副审

子进程的价值不是“多说话”，而是：

- 补 review
- 提 risk
- 给更保守或更清晰的 decision

然后这些内容会自动进入：

- `messages`
- `tasks`
- `artifacts`
- `history`

## 6. 为什么这套系统成立

因为它不是靠外部口头约定运转，而是已经能把值班过程拆成真实对象：

- 讨论 -> `messages`
- 决策 -> `decision-note`
- 简报 -> `artifact-brief`
- 故障 -> `incident-summary`
- 交接 -> `handoff-summary`
- 待办 -> `tasks`
- 全过程 -> `history`

所以它不是“夜班群聊备忘录”，而是一套可回看、可同步、可继续扩展的值班工作区。

## 7. 关联文档

- [night-shift-team-demo.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-team-demo.md)
- [team-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-project-summary.md)
- [team-room-plugin.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-room-plugin.md)
