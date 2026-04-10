# `20260404-team-legacy-remove-pro1.auto-run.md`

## Goal

- 在 Team 已完成 `ctx` 主入口与 task/artifact index-first 之后，继续清理剩余 legacy 迁移输入/兼容层。
- 这份 runbook 的目标不是粗暴删代码，而是：
  - 识别仍被运行态需要的 compat
  - 删除真正不再需要的 legacy helper
  - 保持迁移路径和测试语义清晰

## Context

- 仓库：
  - `/Users/haoniu/sh18/hao.news2/haonews`
- 当前状态：
  - Team Store 主路径已 `ctx` 优先
  - task/artifact 已 index-first
  - legacy 主要残留在：
    - `loadLegacyTasks*`
    - `loadLegacyArtifacts*`
    - `ensureTaskIndex / ensureArtifactIndex`
    - compat bridge
- 相关文件：
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/store.go`
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/backend_helpers.go`
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/index_store.go`
  - `/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/compat_api.go`

## Execution Rules

- 始终自主执行完整任务。
- 除非遇到不可恢复错误，否则不要询问确认。
- workspace 内文件编辑和命令执行默认自动允许。
- 只在：
  - 跨项目修改
  - 删除文件
  - 新的高风险网络操作
  这三类情况下才停。

## Success Criteria

- 明确列出现存 legacy 清单
- 删除真正不再需要的 legacy helper
- compat bridge 范围收窄到明确保留项
- task/artifact 迁移层保留与否有清晰决定
- 所有改动都有 targeted tests，并通过：
  - `go test ./internal/haonews/team`
  - `go test ./internal/plugins/haonewsteam`
  - `go test ./internal/haonews -run Team`
  - `go build ./cmd/haonews`

## Critical Path

1. 先做清单
2. 再做只读 legacy 删除
3. 再做写入/迁移 helper 删除
4. 最后收 compat bridge

不要跳过清单直接删。

## Execution Plan

### Phase A — 现存 legacy 清单

步骤：
- [x] 列出当前仍存在的 legacy / compat 层：
  - `loadLegacyTasks*`
  - `loadLegacyArtifacts*`
  - `readJSONRecordAt / appendJSONLRecord` 是否仍为主路径需要
  - `compat_api.go` 哪些导出方法仍被仓库内引用
- [x] 区分三类：
  - `runtime required`
  - `migration only`
  - `test only`

完成标准：
- 有真实清单，不凭印象删除

结果：
- `runtime required`
  - `appendJSONLRecord / readJSONRecordAt`
  - `ensureTaskIndex / ensureArtifactIndex`
  - `hasTaskIndex / hasArtifactIndex`
- `migration only`
  - `loadLegacyTasks`
  - `loadLegacyArtifacts`
  - `tasks.jsonl / artifacts.jsonl` 作为旧输入源
- `test only`
  - `plugin_test.go` 对 `tasks.jsonl` 的显式 seed
- `compat_api.go`
  - 仍保留为显式兼容层
  - 但 runtime 主路径已不再依赖它

### Phase B — 删除不再需要的 runtime legacy

步骤：
- [x] 删除 task/artifact 运行态已不再使用的 legacy helper
- [x] 保留真正仍需的迁移入口
- [x] 让 backend helper / index store 的职责更清晰：
  - runtime
  - migration
  - compact

验证：
- CRUD / compact / migrate 回归

完成标准：
- 运行态不再依赖 legacy 输入 helper

结果：
- 删除了不再需要的 helper：
  - `loadLegacyTasksWithLimit`
  - `loadLegacyArtifactsWithLimit`
  - `readLastJSONLByScan`
  - `saveTasks`
  - `saveArtifacts`
- `loadHistoryNoCtx(limit>0)` 已切到统一的 `readLastJSONLLines`
- 运行态 task/artifact 主路径继续保持 `index-first`
- legacy 输入 helper 仅剩：
  - `loadLegacyTasks`
  - `loadLegacyArtifacts`
  - 用于 `ensure*Index` 从旧 `tasks.jsonl / artifacts.jsonl` 做一次性迁移

### Phase C — compat bridge 收窄

步骤：
- [x] 检查 `compat_api.go` 的仓库内引用
- [x] 如果调用已都迁到 `Ctx`，继续收窄 compat：
  - 留测试兼容
  - 或仅保留明确外部 API 兼容
- [x] 不做无必要的破坏式公共面删除，除非仓库内外边界都已明确

验证：
- 搜索确认主路径已不依赖 compat

完成标准：
- compat bridge 不再成为默认调用面

结果：
- Team runtime 主路径已不再引用无 `ctx` compat 导出方法
- 同包内部调用已切到：
  - `*Ctx`
  - `*NoCtx`
- `compat_api.go` 现在只保留：
  - 测试兼容
  - 潜在外部调用兼容
- 本轮不做破坏式公共面删除，避免仓库外用户无预告断裂

### Phase D — 测试与文档写回

步骤：
- [x] 更新 runbook 的清单与结果
- [x] 写明：
  - 删除了哪些 helper
  - 保留了哪些兼容层
  - 为什么保留

## Verified

- `go test ./internal/haonews/team -count=1`
- `go test ./internal/plugins/haonewsteam -count=1`
- `go test ./internal/haonews -run Team -count=1`
- `go build ./cmd/haonews`

## Runbook Completion

- `status: completed`
- `runtime legacy removed: yes`
- `compat narrowed: yes`
- `migration-only helpers retained: yes`
- `notes`
  - Team runtime 主路径已不再依赖 compat bridge
  - 剩余 legacy 只保留旧 `tasks.jsonl / artifacts.jsonl` 的迁移输入
  - 如果下一阶段继续，可单独新开 runbook 评估是否彻底删除这些迁移输入 helper

## Blockers

如遇硬阻塞，先写回本文件：
- `status: blocked`
- `blocked_on: ...`
- `last_safe_file: ...`
- `resume_from: ...`

再停止执行。

## Verification

- `go test ./internal/haonews/team -count=1`
- `go test ./internal/plugins/haonewsteam -count=1`
- `go test ./internal/haonews -run Team -count=1`
- `go build ./cmd/haonews`

## Completion Standard

本 runbook 视为完成，需同时满足：
- legacy 清单已写清
- runtime 不再依赖多余 legacy helper
- compat bridge 已收窄
- 测试与构建通过
