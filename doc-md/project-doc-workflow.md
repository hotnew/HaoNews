# 项目文档收口规则

这份文档定义项目完成后如何整理 Markdown 文档，避免文档长期散乱。

## 目标

- runbook 负责执行轨迹
- validation 负责验收证据
- summary 负责最终查看入口

也就是说，项目做完后，别人不应该先读十几份零散 `.md` 才知道结果。

## 文档角色

### 1. runbook / auto-run

作用：

- 记录执行计划
- 记录阶段状态
- 记录 blocker / resume 信息

特点：

- 面向执行
- 可能很长
- 可能包含阶段性细节

### 2. validation / baseline / node 文档

作用：

- 记录真实测试结果
- 记录节点复核
- 记录运行基线

特点：

- 面向证据
- 不负责项目全局解释

### 3. project summary

作用：

- 作为项目完成后的总入口
- 让其他人一份文档就看懂：
  - 项目是什么
  - 当前完成度
  - 怎么用
  - 为什么不是空壳
  - 哪里还能继续做

特点：

- 面向查看
- 面向后续接手
- 不重复 runbook 的流水账

## 标准流程

每条项目主线完成后，默认补齐：

1. runbook 最终状态写回
2. 必要的 validation / node / baseline 文档写回
3. 产出或更新一份 `*-project-summary.md`
4. 更新 `project-index.md`

建议在项目开工时就先明确：

- 这条主线最终对应哪一份 `*-project-summary.md`
- 它应该挂到 `project-index.md` 的哪个分组

## 命名建议

- 执行文档：
  - `xxx.auto-run.md`
- 验收文档：
  - `xxx-validation.md`
  - `xxx-baseline.md`
  - `xxx-node-*.md`
- 整合文档：
  - `xxx-project-summary.md`

## 当前项目入口示例

- 文档总览：
  - [project-index.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/project-index.md)
- Team 项目整合说明：
  - [team-project-summary.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-project-summary.md)

## 什么时候必须补 summary

满足以下任一条件时，默认需要补 summary：

- 完成了一条独立项目主线
- 完成了一次架构级改造
- 完成了一个以后会长期复用的系统能力
- 相关文档已经开始出现多份 runbook / validation / node 文档

## 不要这样做

- 不要只留下很多完成态 runbook，而没有总入口
- 不要把 summary 写成执行流水账
- 不要把 validation 文档当作项目说明文档来替代
- 不要新增了 summary 却忘了把它接进 `project-index.md`
- 不要等项目全部做完了才临时决定 summary 放哪
