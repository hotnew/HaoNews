# Team 项目整合说明

## 1. Team 是什么

`Team` 是 `haonews` 里的协作工作区系统。

它把下面几类对象放进同一个长期可追溯空间：

- `members`
- `channels`
- `messages`
- `tasks`
- `artifacts`
- `history`
- `sync / webhook / SSE / A2A / archive`

它不是单纯聊天页，也不是单纯任务板，而是一个以 Markdown/结构化记录为核心的协作与存档系统。

一句话：

**Team = 协作过程记录 + 结果沉淀 + 任务推进 + 多节点同步的团队工作空间。**

## 2. Team 不是什么

`Team` 不是：

- 只有 UI 的空壳页面
- 只有 message 的聊天工具
- 只有 task 的 issue 面板
- 只有 artifact 的文档库
- 只有 sync 的底层复制模块

它也不是“功能都写在主干里、房间语义全靠约定”的旧模式。

现在的 Team 已经是：

- 主干负责通用协作对象
- Room Plugin 负责频道级业务语义
- Room Theme 负责频道级展示形态

## 3. 现在已经完成了什么

截至 `v0.5.87`，Team 这条主线已经完成了这些核心能力。

### 3.1 主干能力

- Team / member / channel / policy 的正式存储与 API
- `messages / tasks / artifacts / history` 四条主链
- Team search
- Team webhook status / replay
- Team SSE
- Team A2A
- Team archive
- TeamSync 多节点复制

### 3.2 产品化页面

- Team 总览页
- Team 详情页
- Task 列表/详情页
- Sync / conflict 页面
- Webhook 页面
- A2A 页面
- Search 页面
- 频道级 Room 配置工作台

### 3.3 Room Plugin 架构

已完成：

- Room Plugin Registry
- Room Theme Registry
- `ChannelConfig` canonical 存储
- Team 主路由挂载 Room Plugin
- Team 页面和 API 暴露 room metadata

### 3.4 已落地的内置 Room Plugin

当前已内置：

- `plan-exchange`
- `review-room`
- `incident-room`
- `handoff-room`
- `artifact-room`
- `decision-room`

这些插件都不是“挂名插件”，都已经有：

- web + API 入口
- 结构化消息
- summary/workbench
- distill 到 Team Artifact
- 与 Team Task / History 的联动

### 3.5 已落地的 Room Theme

当前已内置：

- `minimal`
- `focus`
- `board`

### 3.6 中层与可维护性改造

已完成：

- typed errors
- filters
- `PolicyEnforcer`
- `TaskLifecycleHook`
- `ChannelContextProvider`
- `TaskDispatch`
- `TaskThread`
- notification / notification SSE
- member stats
- milestone / team template
- TeamSync 自动收敛基础
- store 分文件拆分
- handler 优先依赖 `TeamReader / TeamWriter`

## 4. Team 现在适合怎么用

### 4.1 项目协作

典型方式：

- `main` 用 `plan-exchange`
- `decisions` 用 `decision-room`
- `artifacts` 用 `artifact-room`
- `reviews` 用 `review-room`
- `handoffs` 用 `handoff-room`
- `incidents` 用 `incident-room`

结果是：

- 规划、决策、评审、故障、交接、产物各自有独立语义
- 最后又都能回到 Team 的任务、产物、历史主链

### 4.1.1 规格共创

如果目标不是“直接在 Team 里承载最终程序”，而是让多个 agent 先把规格 md 讨论清楚，再交给下游实现，那么现在推荐直接从内置模板：

- `spec-package`

起步。

这套模板默认会拉起 4 个频道：

- `main`
  - `plan-exchange`
  - 用来放目标、非目标、约束、候选方案和 md 片段
- `reviews`
  - `review-room`
  - 用来收 review / risk / decision，专门打掉规格缺口
- `decisions`
  - `decision-room`
  - 用来冻结边界、取舍和最终实现口径
- `artifacts`
  - `artifact-room`
  - 用来沉淀 `product / workflows / data-model / api / verification`

同时会预置 5 个规格冻结里程碑：

- `scope-frozen`
- `workflow-frozen`
- `data-model-ready`
- `verification-ready`
- `spec-package-ready`

并直接预置 6 条规格共创待办：

- `冻结目标、非目标和范围`
- `评审范围缺口和主要风险`
- `冻结流程和关键取舍`
- `完成数据模型规格`
- `完成验证与验收规格`
- `冻结并交付规格包`

而且这套默认待办已经在真实样本里跑通过一整轮：

- `night-shift-spec8`
  - `task_count = 6`
  - `done_task_count = 6`
  - `milestone_count = 5`
  - `done milestone = 5`
- 同时也已经在另一类题材上跑通：
  - `market-alert-spec1`
  - 说明这套模板不只适用于“夜间快讯值班系统”，也适用于“行情异动告警台”这类独立本地程序规格共创

也就是说，`Team` 在这条用法里最重要的职责不是“承载目标程序”，而是：

- 组织多 agent 协作
- 产出稳定规格包
- 把讨论收成可下游实现的 md

### 4.2 文档沉淀

Team 现在很适合做“讨论 -> 结论 -> Markdown 产物”的长期沉淀。

比如：

- `plan-summary`
- `review-summary`
- `incident-summary`
- `handoff-summary`
- `artifact-brief`
- `decision-note`

### 4.3 多节点团队记忆

Team 不是只适合本机使用。

现在已经验证过：

- `.75`
- `192.168.102.8`

之间可以同步：

- members
- channel config
- tasks
- artifacts
- history

所以它也适合做跨节点团队记忆，而不是把上下文锁死在一台机器上。

## 5. 为什么 Team 不是空壳

因为这套东西已经有真实运行闭环，而不是概念模型。

已经做过真实验证的链路包括：

- 创建真实 Team
- 创建真实 channel config
- 发真实 room 消息
- 生成真实 task
- 生成真实 artifact
- 触发真实 history
- webhook status / replay
- SSE
- A2A
- archive
- `.75 <-> 192.168.102.8` 的 TeamSync

也就是说，现在的 Team 已经是：

**可工作的协作底座**

而不是“有页面、没主链”的演示件。

## 6. 真实样本

当前最适合反复复用的真实样本是：

- `feiji-app`
- `night-shift-desk`

节点：

- `192.168.102.8`
- `.75`

这个样本已经真实覆盖：

- `plan-exchange`
- `decision-room`
- `artifact-room`
- `handoff-room`
- `review-room`
- `incident-room`

以及：

- task 自动挂接
- artifact 提炼
- history 回显
- channel config 同步
- member 同步
- task/history/artifact 同步

详细验收文档见：

- [team-node-192.168.102.8-feiji-app-validation.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-node-192.168.102.8-feiji-app-validation.md)
- [night-shift-team-demo.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-team-demo.md)
- [night-shift-system-manual.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system-manual.md)

### 6.0 Team 作为上游共创工具

`Team` 不只可以直接做协作空间，也可以作为“上游多 agent 讨论与产出规格”的工具。

当前已经补了一条明确样本：

- 先在 Team 里用多 agent 把夜间值班系统的流程、角色、风险、交接、产物说清
- 再把这些讨论结果收成一组**与 Team 解耦**的独立程序规格文档
- 同时也已经有一支更贴近正确用法的 `spec-package` 样本：
  - [spec-package-team-demo.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/spec-package-team-demo.md)

对应规格包：

- [night-shift-system2/README.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/README.md)
- [night-shift-system2/01-product.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/01-product.md)
- [night-shift-system2/02-workflows.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/02-workflows.md)
- [night-shift-system2/03-data-model.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/03-data-model.md)
- [night-shift-system2/04-screens-and-interactions.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/04-screens-and-interactions.md)
- [night-shift-system2/05-api-and-runtime.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/05-api-and-runtime.md)

这条样本的重点是：

- Team 负责前期协作和共创
- 独立程序规格负责后续实现
- 最终程序不依赖 Team API、Team 页面或 Team 数据格式

这条主线从现在开始的推荐落法也已经固定：

1. 用 `spec-package` 模板建 Team
2. 用多个 agent 在 `main / reviews / decisions / artifacts` 里迭代
3. 冻结规格包
4. 再让下游本地 agent 独立实现程序

当前最贴近这条推荐路径的真实样本是：

- `night-shift-spec3`
  - [spec-package-team-demo.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/spec-package-team-demo.md)

这条主线现在还有一份可直接复用的固定 SOP：

- [spec-package-sop.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/spec-package-sop.md)

对应的独立程序整合入口：

- [night-shift-system2-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2-project-summary.md)

### 6.1 `night-shift-desk`

这是一个专门用来说明 “Team + 子进程 + Room Plugin” 怎么协同工作的本地样本。

它真实覆盖了：

- `plan-exchange`
- `decision-room`
- `artifact-room`
- `review-room`
- `incident-room`
- `handoff-room`

而且不是静态文档，而是真实跑通了：

- 子进程成员参与 review / decision
- 房间消息沉淀为 artifact
- 房间批处理自动生成 task
- Team history 回显整条夜班流程

适合拿来解释：

- Team 能用来做什么
- 为什么它不是聊天壳
- Room Plugin 怎么接回 Team 主链

## 7. 这条线最近解决过的关键问题

### 7.1 `decision-room task-sync-all` 重复建任务

现象：

- 单条 `task-sync` 已有绑定任务后
- 再跑 `task-sync-all`
- 同一条 `decision` 还会再建任务

状态：

- 已修复

### 7.2 `agent://...` 这类 ID 导致详情页/API 404

现象：

- `task_id / artifact_id` 内含 `agent://...`
- 旧路由按 `URL.Path` 拆段
- `%2F` 提前还原成 `/`

状态：

- 已修复为按 `EscapedPath` 分段解析

### 7.3 Room Plugin artifact kind 被降级成 `markdown`

现象：

- `decision-note / review-summary / incident-summary / artifact-brief`
- 在同步后被写成统一的 `markdown`

状态：

- 已修复

## 8. 当前最值得继续增强的方向

虽然主线已经可用，但后面继续增强时，最值的方向还是这几类：

### 8.1 现有 Room Plugin 更强自动联动

重点是：

- 更细状态策略
- 更强 task / artifact / history 自动挂接
- 更强批处理结果回显

### 8.2 更多内置 Room Plugin

现在已经有 6 个。

如果继续扩展，应该优先做：

- 仍然复用现有 Team 主链
- 语义明确
- 不引入新一套底层存储

### 8.3 更强的项目总结面

现在 Room 和 Team 主链都已经有数据，下一层更值的是：

- 把“讨论 -> 决策 -> 任务 -> 产物 -> 历史”进一步收成管理视图

## 9. 关联文档

- [team-room-plugin.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-room-plugin.md)
- [team-node-192.168.102.8-feiji-app-validation.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-node-192.168.102.8-feiji-app-validation.md)
- [team-dev-architecture.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-dev-architecture.md)

## 10. 当前结论

当前 `Team` 最准确的定位是：

**一个围绕 Markdown/结构化记录工作的协作空间系统。**

它已经具备：

- 长期记录
- 任务推进
- 结果沉淀
- 多节点同步
- 房间级语义扩展

所以它现在不是“要不要做”的概念阶段了，而是已经进入：

**继续产品化和继续扩展房间能力** 的阶段。
