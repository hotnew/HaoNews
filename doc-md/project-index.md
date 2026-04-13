# 项目文档总览

这份文档作为 `doc-md/` 的总入口，用来避免项目文档长期散乱。

建议查看顺序：

1. 先看 `*-project-summary.md`
2. 再看对应 `validation / baseline / node` 文档
3. 最后再回看 runbook / auto-run 执行轨迹

## 0. 推荐阅读顺序

### 第一次看项目

建议按这个顺序：

1. [README.md](/Users/haoniu/sh18/hao.news2/haonews/README.md)
2. [topics-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/topics-project-summary.md)
3. [live-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/live-project-summary.md)
4. [team-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-project-summary.md)
5. [runtime-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/runtime-project-summary.md)

### 只想快速理解当前系统

优先看：

1. [README.md](/Users/haoniu/sh18/hao.news2/haonews/README.md)
2. `Topics / Live / Team / Runtime` 四份 `*-project-summary.md`

### 只想追一条主线

- Topics：
  - [topics-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/topics-project-summary.md)
- Live：
  - [live-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/live-project-summary.md)
- Team：
  - [team-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-project-summary.md)
- Runtime：
  - [runtime-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/runtime-project-summary.md)

## 0.1 按角色看文档

### 新接手开发

建议顺序：

1. [README.md](/Users/haoniu/sh18/hao.news2/haonews/README.md)
2. [project-doc-workflow.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/project-doc-workflow.md)
3. 相关主线的 `*-project-summary.md`
4. 再看对应 `*.auto-run.md`

### 运维 / 节点验证

优先看：

1. [runtime-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/runtime-project-summary.md)
2. [node-upgrade-75-74.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/node-upgrade-75-74.md)
3. [runtime-75-74-validation.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/runtime-75-74-validation.md)

### 只看 Team

优先看：

1. [team-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-project-summary.md)
2. [team-room-plugin.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-room-plugin.md)
3. [team-node-192.168.102.8-feiji-app-validation.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-node-192.168.102.8-feiji-app-validation.md)
4. [night-shift-team-demo.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-team-demo.md)
5. [night-shift-system-manual.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system-manual.md)

## 0.2 按任务类型找文档

### 开发实现

优先看：

1. 对应主线的 `*-project-summary.md`
2. 对应 `*.auto-run.md`
3. 相关架构说明

当前入口：

- Topics 开发：
  - [topics-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/topics-project-summary.md)
  - [20260403-haonews-task.auto-run.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/20260403-haonews-task.auto-run.md)
- Team 开发：
  - [team-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-project-summary.md)
  - [team-dev-architecture.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-dev-architecture.md)
  - [night-shift-system2/README.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/README.md)
- Live 开发：
  - [live-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/live-project-summary.md)
  - [readme-live.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/readme-live.md)

### 架构阅读

优先看：

1. [protocol-v0.1.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/protocol-v0.1.md)
2. [team-dev-architecture.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-dev-architecture.md)
3. 各主线 `*-project-summary.md`

### 发版

优先看：

1. [release.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/release.md)
2. [project-doc-workflow.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/project-doc-workflow.md)
3. 对应项目 summary 和 validation

### 节点升级 / 运维

优先看：

1. [runtime-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/runtime-project-summary.md)
2. [node-upgrade-75-74.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/node-upgrade-75-74.md)
3. [runtime-75-74-validation.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/runtime-75-74-validation.md)

### 节点验收 / 真机复核

优先看：

1. [team-node-192.168.102.8-feiji-app-validation.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-node-192.168.102.8-feiji-app-validation.md)
2. [runtime-75-74-validation.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/runtime-75-74-validation.md)
3. [night-shift-team-demo.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-team-demo.md)

### 项目总览

优先看：

1. [topics-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/topics-project-summary.md)
2. [live-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/live-project-summary.md)
3. [team-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-project-summary.md)
4. [runtime-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/runtime-project-summary.md)

## 1. 项目整合入口

### Team

- 项目整合说明：
  - [team-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-project-summary.md)
- 架构与插件说明：
  - [team-room-plugin.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-room-plugin.md)
  - [team-dev-architecture.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-dev-architecture.md)
- 推荐起手模板：
  - `spec-package`
  - 用于多 agent 讨论、评审、冻结边界并产出规格 md
- `spec-package` 真实样本：
  - [spec-package-team-demo.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/spec-package-team-demo.md)
- 真实节点验收：
  - [team-node-192.168.102.8-feiji-app-validation.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-node-192.168.102.8-feiji-app-validation.md)
- 真实协作样本：
  - [night-shift-team-demo.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-team-demo.md)
  - [night-shift-system-manual.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system-manual.md)
- Team 产出的独立程序规格包：
  - [night-shift-system2/README.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/README.md)
  - [night-shift-system2/01-product.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/01-product.md)
  - [night-shift-system2/02-workflows.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/02-workflows.md)
  - [night-shift-system2/03-data-model.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/03-data-model.md)
  - [night-shift-system2/04-screens-and-interactions.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/04-screens-and-interactions.md)
  - [night-shift-system2/05-api-and-runtime.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/05-api-and-runtime.md)

### Live

- 项目整合说明：
  - [live-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/live-project-summary.md)
- 使用与方案：
  - [readme-live.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/readme-live.md)
  - [add-live-public.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/add-live-public.md)
- 测试计划：
  - [live-test.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/live-test.md)

### Topics

- 项目整合说明：
  - [topics-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/topics-project-summary.md)
- 协议与方案：
  - [protocol-v0.1.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/protocol-v0.1.md)
  - [add-hot-new.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/add-hot-new.md)
- 优化与收口：
  - [add-pro.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/add-pro.md)
  - [20260403-haonews-task.auto-run.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/20260403-haonews-task.auto-run.md)

## 2. 运行与节点基线

- 运行节点整合说明：
  - [runtime-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/runtime-project-summary.md)
- `.75 / .74` 升级说明：
  - [node-upgrade-75-74.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/node-upgrade-75-74.md)
- `.75 / .74` 运行基线：
  - [runtime-75-74-baseline.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/runtime-75-74-baseline.md)
- `.75 / .74` 运行复核：
  - [runtime-75-74-validation.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/runtime-75-74-validation.md)

## 3. 发布与流程规则

- 发布说明：
  - [release.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/release.md)
- 项目文档收口规则：
  - [project-doc-workflow.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/project-doc-workflow.md)
- 项目整合说明模板：
  - [project-summary-template.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/project-summary-template.md)

## 4. 执行文档说明

执行文档仍然保留在 `doc-md/`，主要用于：

- 执行轨迹
- blocker / resume
- 阶段验证

但默认不再把它们当成“项目说明入口”。

如果要快速理解一个项目，优先看：

- `*-project-summary.md`

如果要追溯执行细节，再看：

- `*.auto-run.md`
- `*-validation.md`
- `*-baseline.md`
- `*-node-*.md`
