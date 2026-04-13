# `20260413-team-fix1-auto-run.md`

## Goal

- 把 `Team` 主线纠偏为“多 agent 协同产出规格 md 的上游工具”，并把 `夜间快讯值班系统2` 推进成一个**与 Team 运行时完全解耦**、可本地启动、可持久化、可验证的独立程序。

## Context

- 仓库 / 工作目录：
  - `/Users/haoniu/sh18/hao.news2/haonews`
- 项目整合文档：
  - `/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-project-summary.md`
  - `/Users/haoniu/sh18/hao.news2/haonews/doc-md/project-index.md`
- 已知事实：
  - 当前已经有一份 Team 样本主线：
    - `/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-team-demo.md`
    - `/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system-manual.md`
  - 当前已经有一套独立程序规格包：
    - `/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/README.md`
    - `/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/01-product.md`
    - `/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/02-workflows.md`
    - `/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/03-data-model.md`
    - `/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/04-screens-and-interactions.md`
    - `/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/05-api-and-runtime.md`
  - 当前已有一个本地原型程序，但它仍带明显 Team 语义：
    - `/Users/haoniu/sh18/hao.news2/haonews/internal/nightshiftdesk/server.go`
    - `/Users/haoniu/sh18/hao.news2/haonews/internal/nightshiftdesk/templates/index.html`
    - `/Users/haoniu/sh18/hao.news2/haonews/cmd/haonews/main.go`
  - 当前原型存在的偏差：
    - 仍使用 `TeamID`
    - 仍使用 `channels/plugin/theme`
    - 仍把 Team 样本当成本地程序语义来源
    - 仍缺少真正产品语义里的 `sources / briefs / exports`
- 当前明确不能做的事：
  - 不能让 `夜间快讯值班系统2` 依赖 Team API、Team 数据文件或 Team 页面
  - 不能把“再建一个 Team”当作实现完成
- 输入材料：
  - `/Users/haoniu/sh18/hao.news2/haonews/20260413-team-fix1.md`
  - `/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-local-app.md`
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/nightshiftdesk/server_test.go`

## Execution Contract

- 读完整个文档后立即开始执行。
- 在本 runbook 完成或确认硬阻塞之前，不要向用户提问。
- 普通实现分歧统一按以下优先级自行裁决：
  - 风险更低
  - 更可回滚
  - 与现有代码/文档更一致
  - 改动更小但能完成目标
  - 更容易验证
- 不允许把普通命名、页面布局、字段顺序、目录落点问题升级成用户确认。
- 这条 runbook 的重点不是继续做 Team，而是把**独立程序**从“过渡原型”收成真正的 `night-shift-system2`。
- 执行过程中持续更新 checklist 状态，不要只在最后统一回填。

## Planning Rules

- 先切语义边界，再补功能；不要先堆功能再纠偏语义。
- 先处理会导致后续实现继续走偏的结构问题，再处理页面和导出。
- 只读取和修改当前关键路径需要的文件：
  - `internal/nightshiftdesk/*`
  - `cmd/haonews/main.go`
  - `doc-md/night-shift-system2/*`
  - `doc-md/team-project-summary.md`
  - `doc-md/project-index.md`
- 若发现无关问题，不扩大本次范围。

## Execution Plan

### Phase 1. Inspect And Freeze Boundaries

- [x] 读取最小必要上下文：
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/nightshiftdesk/server.go`
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/nightshiftdesk/templates/index.html`
  - `/Users/haoniu/sh18/hao.news2/haonews/cmd/haonews/main.go`
  - `night-shift-system2` 全部规格文档
- [x] 明确这条主线的 summary 文档路径：
  - 更新 `/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-project-summary.md`
  - 更新 `/Users/haoniu/sh18/hao.news2/haonews/doc-md/project-index.md`
  - 若独立程序实现已经形成完整主线，再补新的 `doc-md/night-shift-system2-project-summary.md`
- [x] 明确当前基线：
  - 原型程序当前哪些字段/页面仍是 Team 语义
  - 哪些规格项还没进入程序

### Phase 2. Refactor Runtime Model Away From Team

- [x] 将 `internal/nightshiftdesk/server.go` 的顶层状态从 Team 语义改成产品语义：
  - `TeamID` -> `SystemID`
  - 移除 `Channels`
  - 新增 `Sources`
  - 新增 `Briefs`
  - 保留 `Reviews / Decisions / Incidents / Handoffs / History`
- [x] 新增或重构最小产品对象：
  - `Source`
  - `Brief`
  - `Operator` 或直接保留成员但去 Team 化命名
- [x] 移除运行时中对 `plugin / theme / channel_id` 的核心依赖：
  - 若某些字段仅用于显示，改成产品语义区块而不是 Team 房间
- [x] 保持状态文件仍可本地 JSON 持久化
- [x] 若旧状态文件不兼容，提供低风险兼容加载或自动迁移策略：
  - 优先兼容读取旧字段
  - 写出时按新结构保存

### Phase 3. Implement Product Flows Required By Spec

- [x] 实现来源池主链：
  - 新增来源
  - 来源状态流转：
    - `new`
    - `checking`
    - `verified`
    - `deferred`
    - `dropped`
- [x] 实现风险复核主链：
  - 新增 `review`
  - 新增 `risk`
  - `open / resolved`
- [x] 实现终审主链：
  - 基于来源新增 `decision`
  - 决策状态：
    - `draft`
    - `approved`
    - `hold`
    - `rejected`
- [x] 实现事故主链：
  - 新增 `incident / mitigating / recovered / postmortem`
- [x] 实现交接主链：
  - 新增 `handoff`
  - 推进：
    - `draft`
    - `ready`
    - `accepted`
- [x] 实现简报主链：
  - 生成 `brief`
  - 至少支持 `draft / published`

### Phase 4. Rebuild Screens Around Product Semantics

- [x] 重构首页模板，不再出现：
  - Team 样本
  - Team 频道
  - Team 插件
  - Team theme
- [x] 首页改成真正的值班总览：
  - 待核来源数
  - 待终审数
  - 高风险数
  - 未恢复事故数
  - 待交接项数
  - 最近动态
- [x] 模板内必须出现 6 个产品区块：
  - 总览
  - 来源池
  - 终审决策
  - 风险复核
  - 事故处置
  - 交接与简报
- [x] 为每个主链补最小表单动作：
  - 来源新增 / 状态更新
  - 复核新增
  - 决策新增
  - 事故新增 / 状态推进
  - 交接新增 / 状态推进
  - 简报生成 / 导出

### Phase 5. Add Export And Runtime Entry

- [x] 新增 Markdown 导出能力：
  - 夜间简报导出
  - 交接摘要导出
- [x] 导出结果至少支持：
  - HTTP 下载
  - 或写入本地文件
- [x] 调整 CLI 入口：
  - 继续使用现有 `haonews` CLI 子命令承载，但命令语义应明确是独立程序
  - 启动输出中不再把该程序描述成 Team 样本
- [x] 更新本地程序说明文档：
  - `/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-local-app.md`
  - 明确它现在对应的是 `夜间快讯值班系统2`

### Phase 6. Verify And Close Out

- [x] 运行最强可行验证：
  - `go test ./internal/nightshiftdesk -count=1`
  - `go build ./cmd/haonews`
  - 启动本地程序并访问首页
  - 真实执行一次：
    - 新增来源
    - 推进到 `verified`
    - 新增决策
    - 新增风险
    - 新增事故并推进到 `recovered`
    - 新增交接并推进到 `accepted`
    - 导出一份 Markdown 简报
- [x] 若验证失败，先修再重跑
- [x] 更新本文档完成状态
- [x] 更新或补 summary：
  - `team-project-summary.md` 只保留 Team 上游定位
  - 若程序已形成完整独立主线，补 `night-shift-system2-project-summary.md`
- [x] 更新 `project-index.md`

## Verification

- 首选验证命令：
  - `cd /Users/haoniu/sh18/hao.news2/haonews && go test ./internal/nightshiftdesk -count=1`
  - `cd /Users/haoniu/sh18/hao.news2/haonews && go build ./cmd/haonews`
- 运行态验证：
  - `go run ./cmd/haonews nightshift ...`
  - `curl http://127.0.0.1:<port>/api/state`
  - 对动作端点做 `POST` 验证
- 备用验证方式：
  - 若 HTTP 动作不方便全测，至少验证：
    - 状态文件写入成功
    - 导出的 Markdown 文件真实生成

### 完成标准

- [x] 运行时状态模型已经去 Team 化
- [x] 页面信息结构已经按规格文档重组
- [x] `sources / reviews / decisions / incidents / handoffs / briefs` 全部进入程序
- [x] 支持 Markdown 导出
- [x] 程序重启后数据仍在
- [x] 文档入口已经明确：
  - Team 是上游协作工具
  - `night-shift-system2` 是下游独立程序
- [x] 最终汇报包含 `Completed / Blocked / Next Step`

## Fallback Rules

- 若一次性完全重构模板风险过高，优先先完成：
  - 状态模型去 Team 化
  - 动作端点去 Team 化
  - 导出能力
  - 然后再重排页面
- 若旧状态文件结构影响开发，优先加兼容读取，不要直接丢弃旧数据
- 若某个页面区块来不及完全独立拆页，允许先做单页多区块工作台，只要语义已经完全产品化
- 若需要保留少量旧字段做兼容，必须：
  - 在代码里标明兼容目的
  - 不再让其出现在对外页面主文案中

## Blockers / Resume

- 硬阻塞定义：
  - `night-shift-system2` 规格包内部出现互相冲突、且无法自动裁决的产品语义
  - 本地程序核心运行链反复失败，且在改变实现后仍无法通过最小 build/run 验证
  - 当前工作区出现用户未说明但会直接冲突覆盖的同文件修改
- 如果阻塞，必须写回：
  - `Blocked on`
  - `Tried`
  - `Why blocked`
  - `Exact next step`
- 恢复执行时：
  - 先读完整个文档
  - 从第一个未完成 checkbox 继续

## Status Writeback

- `Completed`:
  - `internal/nightshiftdesk/server.go` 已从 Team 语义重构成独立产品语义，状态文件默认落到 `~/.hao-news/night-shift-system2/state.json`
  - 程序主链已补齐：`sources / reviews / decisions / incidents / handoffs / briefs / tasks / archive / history`
  - 页面与路由已补齐：`/ /sources /reviews /decisions /briefs /incidents /handoffs /tasks /archive`
  - API 已补齐：`/api/state /api/sources /api/reviews /api/decisions /api/incidents /api/handoffs /api/briefs /api/tasks /api/archive`
  - 已支持 Markdown 导出：`/exports/brief/latest`、`/exports/handoff/latest`
  - 已完成真实运行链验证：新增来源、复核、终审、事故恢复、交接 accept、简报生成、导出、重启后读回
  - 已新增独立项目整合文档：`doc-md/night-shift-system2-project-summary.md`
- `Blocked`:
  - None
  - None
- `Next Step`:
  - 从 `Phase 1` 开始执行，先读取 `internal/nightshiftdesk/*` 和 `night-shift-system2` 规格包，然后直接进入运行时去 Team 化重构

## Project Summary Rule

- 这份 runbook 对应的是一条独立产品主线整改：
  - 上游：Team 协作定位纠偏
  - 下游：`night-shift-system2` 独立程序实现
- 完成时默认：
  - 更新 `/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-project-summary.md`
  - 更新 `/Users/haoniu/sh18/hao.news2/haonews/doc-md/project-index.md`
  - 若程序实现闭环完成，新增：
    - `/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2-project-summary.md`
