# 运行节点项目整合说明

## 1. 这条主线是什么

这条主线负责把 `haonews` 的运行节点升级、重启、回滚、双节点验收和真实运行基线收成稳定流程。

它解决的不是功能开发本身，而是：

- 节点怎么升级
- 节点怎么验收
- 节点怎么回滚
- 哪些运行态结果才算“真的好了”

一句话：

**Runtime = 节点升级流程 + 运行态验收基线 + 可回滚的运维主线。**

## 2. 它不是什么

它不是：

- 单纯一份 upgrade 命令清单
- 单纯一个 bootstrap=ready 的检查页
- 单纯临时 shell 操作记录

它也不是“只要服务能起来就算完成”的弱验收流程。

## 3. 当前已经完成了什么

### 3.1 节点升级流程

已经固定：

1. `.75` 先调试和验证
2. GitHub `main + tag + release`
3. `.74` 和其它节点从 GitHub/tag 升级
4. 再做运行态验收

对应文档：

- [node-upgrade-75-74.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/node-upgrade-75-74.md)

### 3.2 双节点运行基线

已经写清：

- `.75` 的角色
- `.74` 的角色
- `serve / syncd / live sender` 的进程模型
- 本地配置文件位置
- 基础恢复规则

对应文档：

- [runtime-75-74-baseline.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/runtime-75-74-baseline.md)

### 3.3 标准运行态验收

已经固定的正式验收对象包括：

- Team sync health / conflicts
- Team channel config replication
- Team webhook status / replay
- Team archive
- Team A2A
- Team SSE
- `public-live-time`

对应文档：

- [runtime-75-74-validation.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/runtime-75-74-validation.md)

### 3.4 TeamSync 真实样本链

除了 `.75 / .74` 双节点基线，现在还补了一个真实 Team 样本链：

- `.75`
- `192.168.102.8`
- `feiji-app`

这条链已经真实验证了：

- tasks
- artifacts
- history
- members
- channel config
- Room Plugin 产物 kind 保留

对应文档：

- [team-node-192.168.102.8-feiji-app-validation.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-node-192.168.102.8-feiji-app-validation.md)

## 4. 适合怎么用

### 4.1 发版前节点验收

每次版本准备发到 GitHub 前：

- 先在 `.75` 验证
- 只在 `.75` 调试
- 等 `.75` 稳定后再发版

### 4.2 发版后节点升级

每次正式版本发布后：

- 让其它节点从 tag 升级
- 不再长期依赖远端临时补丁

### 4.3 真实同步排障

如果 TeamSync 出问题，不再只看 bootstrap。

应该直接核：

- team list
- task list
- artifact list
- history
- channel config

## 5. 为什么这条线不是空壳

因为它已经不只是“写了几份文档”，而是有真实节点结果：

- `.75` 和 `.74` 的运行态基线
- `192.168.102.8` 的 Linux 节点上线与验证
- `feiji-app` 的真实 Team 样本
- TeamSync 的真实对象到达验证
- Room Plugin 的真实 artifact kind 校验

也就是说，这条线已经是：

**可执行、可复核、可回滚的节点运维基线**

而不是“随手记下来的命令备忘”。

## 6. 真实样本

当前最值得反复复用的真实运行样本有两组：

### 6.1 `.75 / .74`

适合验证：

- 标准双节点升级
- Team / Live / webhook / SSE / A2A 运行态

### 6.2 `.75 / 192.168.102.8 / feiji-app`

适合验证：

- TeamSync 真实对象同步
- members / channel config / tasks / artifacts / history
- Room Plugin 页面和 artifact kind

## 7. 最近解决的关键问题

### 7.1 `channel_config` 自动同步正式纳入验收

现结论：

- 不再接受“频道已同步但配置靠远端手工 PUT”这种临时结果
- `channel_config` 自动同步是正式验收项

### 7.2 `agent://...` 这类 ID 的 Team 路由解析

现结论：

- Team 路由必须按 `EscapedPath` 分段解析
- 否则 `task_id / artifact_id` 带 `agent://...` 时详情页/API 会被拆坏

### 7.3 Room Plugin artifact kind 在 TeamSync 中不能被降级

现结论：

- `decision-note`
- `review-summary`
- `incident-summary`
- `artifact-brief`

这些 kind 现在会按真实类型保留，不再统一变成 `markdown`

### 7.4 本地 `serve` 与 `syncd` 必须一起升级

现结论：

- 只升级 `haonews serve` 不够
- `~/.hao-news/bin/hao-news-syncd` 也必须同步升级
- 否则会出现“运行态看起来很怪，但其实是二进制版本不一致”的假象

## 8. 当前边界和后续增强

当前已经有稳定基线，但还可以继续增强：

- 把更多真实节点纳入固定验收链
- 把 TeamSync 状态页和真实对象结果进一步对齐
- 把 Linux / macOS 多平台升级流程再抽成更统一脚本

这些属于增强项，不影响当前基线已可用。

## 9. 关联文档

- 节点升级与回滚：
  - [node-upgrade-75-74.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/node-upgrade-75-74.md)
- 双节点运行基线：
  - [runtime-75-74-baseline.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/runtime-75-74-baseline.md)
- 双节点运行验收：
  - [runtime-75-74-validation.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/runtime-75-74-validation.md)
- Team 节点真实样本：
  - [team-node-192.168.102.8-feiji-app-validation.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-node-192.168.102.8-feiji-app-validation.md)
