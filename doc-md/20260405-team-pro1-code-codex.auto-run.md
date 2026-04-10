# `20260405-team-pro1-code-codex.auto-run.md`

## Goal

- 严格按 [20260405-team-pro1.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/20260405-team-pro1.md) 与 [20260405-team-pro1-code.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/20260405-team-pro1-code.md) 的架构与实施步骤，完成 Team 的本质性重构：引入 `Room Plugin + Channel Config + Room Theme` 框架，并落地第一个 `plan-exchange` 插件与 `minimal` 主题，同时保持 Team 现有主链和 P2P 同步语义不回退。

## Context

- 仓库 / 工作目录：
  - `/Users/haoniu/sh18/hao.news2/haonews`
- 输入文档：
  - [20260405-team-pro1.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/20260405-team-pro1.md)
  - [20260405-team-pro1-code.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/20260405-team-pro1-code.md)
  - 参考模板：[runbook-template.auto-run.md](/Users/haoniu/sh18/hao.news2/runbook-template.auto-run.md)
- 本轮必须遵守的硬约束：
  - 不改动 `internal/haonews/team/sync.go`
  - 不改动 `internal/haonews/team_sync.go`
  - 不重做 P2P 协议、存储格式、同步模型
  - 不引入外部数据库或新的序列化格式，保持 `JSON / JSONL`
  - Channel Config 缺失时必须回退到零值默认行为
  - Room Plugin 找不到时只返回 `404`，不影响 Team 主功能
  - 所有 Room Plugin 数据写入必须走 Team Store 标准接口
- 已知基线：
  - Team 当前已有：`members / channels / messages / tasks / artifacts / history / archive`
  - Team 当前已有：`sync / conflicts / webhook / A2A / SSE / Team P2P`
  - Team 插件已有产品化页面，但还没有 `Room Plugin / Room Theme / ChannelConfig` 这条中层解耦架构
- 强制验证要求：
  - `go build ./...`
  - `go test ./...`
  - 若 Phase 级别改动完成，也应尽量先跑相关子集测试，再跑全量

## Execution Contract

- 读完整个文档后立即开始执行。
- 本 runbook 的目标是严格落地源文档定义的架构，不自行偷换成“更小但不同”的方案。
- 不把普通实现分歧、路径分歧、命名分歧升级成用户确认项。
- 若存在多条实现路径，优先选择：
  - 风险更低
  - 更可回滚
  - 与两份源文档更一致
  - 改动更小但不改变目标架构
  - 更容易验证
- 不允许把本轮重构缩水成纯 UI/纯页面增强。
- 若出现阻塞：
  - 先尝试本文件中的 fallback
  - 再尝试不改变架构目标的本地 fallback
  - 仍失败时，把真实状态写回本文档再停止
- 执行过程中持续更新 checklist，不要只在最后统一回填。

## Planning Rules

- 严格按源文档的主顺序执行：`P1 Team 中层解耦 → P2 第一个 Room Plugin → P3 第一个 Room Theme → P4 测试与文档`
- 允许为编译、测试或现有代码适配做必要的小调整，但不改变以下核心设计：
  - `RoomPlugin` 为 Channel 级二级插件
  - `ChannelConfig` 独立存储
  - `plan-exchange` 通过标准 Team Message / Artifact 接口工作
  - Room Theme 为 Channel 级模板覆盖
- 先完成框架，再落地第一个内置插件，再做主题，再收测试与文档。
- 每一阶段只读取和修改该阶段真正必要的文件，不做无关扫描。

## Execution Plan

### Phase 1. Inspect And Reconcile Source Docs

- [ ] 读取并对齐两份源文档的共同结论：
  - `Room Plugin` 是 Channel 级二级插件
  - `ChannelConfig` 独立于现有 `channels.json`
  - `Room Theme` 是 Channel 级模板覆盖
  - `plan-exchange` 是首个内置插件
- [ ] 读取最小必要代码上下文：
  - [internal/haonews/team/store.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/store.go)
  - [internal/haonews/team/paths.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/paths.go)
  - [internal/haonews/team/ctx_api.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/ctx_api.go)
  - [internal/haonews/team/compat_api.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/compat_api.go)
  - [internal/plugins/haonewsteam/plugin.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/plugin.go)
  - [internal/plugins/haonewsteam/handler.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/handler.go)
  - [internal/plugins/haonewsteam/handler_channel.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/handler_channel.go)
  - [internal/plugins/haonewsteam/types.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/types.go)
  - [internal/plugins/haonewsteam/haonews.plugin.json](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/haonews.plugin.json)
- [ ] 在 `Status Writeback` 中记录初始分类：
  - `done`
  - `todo`
  - `defer`
  - `stale`
- 完成标准：
  - 当前代码与源文档之间的差距已明确
  - 后续 P1-P4 不需要再重新理解目标

### Phase 2. Team Middle Layer Decoupling (`P1`)

- [ ] 新建 Room Plugin Registry：
  - [internal/plugins/haonewsteam/roomplugin/registry.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/roomplugin/registry.go)
  - 严格实现源文档定义的：
    - `RoomPlugin`
    - `Manifest`
    - `Registry`
    - `LoadManifestJSON`
    - `LoadManifestFile`
- [ ] 新建 Channel Config 数据层：
  - [internal/haonews/team/channel_config.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/channel_config.go)
  - 必须提供：
    - `ChannelConfig`
    - `PluginID()`
    - `channelConfigDir`
    - `channelConfigPath`
    - `loadChannelConfigNoCtx`
    - `saveChannelConfigNoCtx`
    - `listChannelConfigsNoCtx`
- [ ] 按源文档要求扩展 `ctx_api.go`：
  - `LoadChannelConfigCtx`
  - `SaveChannelConfigCtx`
  - `ListChannelConfigsCtx`
- [ ] 按源文档要求扩展 `compat_api.go`：
  - `LoadChannelConfig`
  - `SaveChannelConfig`
  - `ListChannelConfigs`
- [ ] 修改 `plugin.go`：
  - `Build()` 中初始化 `roomplugin.Registry`
  - 先接入 Registry，再在 P2 注册 `plan-exchange`
  - `newHandler` 增加 `roomRegistry` 参数
  - 增加：
    - `/teams/{teamID}/r/{pluginID}/*`
    - `/api/teams/{teamID}/r/{pluginID}/*`
- [ ] 修改 `handler_channel.go`：
  - 增加：
    - `handleAPITeamChannelConfig`
    - `handleAPITeamChannelConfigs`
    - `loadChannelConfigSafe`
  - Channel 页面渲染数据中加入 `ChannelConfig`
- [ ] 修改 `types.go`：
  - `teamChannelPageData` 增加 `ChannelConfig`
- [ ] 修改 API 汇总入口：
  - `GET /api/teams/{teamID}` 响应中加入 `channel_configs`
  - `GET /api/teams/{teamID}/channels/{channelID}/config`
  - `PUT /api/teams/{teamID}/channels/{channelID}/config`
  - `GET /api/teams/{teamID}/channel-configs`
- 完成标准：
  - Team 中层已具备 Room Plugin/Theme/ChannelConfig 的最小框架
  - 没有配置的 Channel 行为保持完全不变

### Phase 3. First Room Plugin `plan-exchange` (`P2`)

- [ ] 新建目录：
  - [internal/plugins/haonewsteam/rooms/planexchange/plugin.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/rooms/planexchange/plugin.go)
  - [internal/plugins/haonewsteam/rooms/planexchange/handler.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/rooms/planexchange/handler.go)
  - [internal/plugins/haonewsteam/rooms/planexchange/types.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/rooms/planexchange/types.go)
  - [internal/plugins/haonewsteam/rooms/planexchange/roomplugin.json](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/rooms/planexchange/roomplugin.json)
- [ ] 在 `types.go` 中定义三类结构：
  - `PlanMessage`
  - `SkillMessage`
  - `SnippetMessage`
  - 以及 `AbandonedOption`
- [ ] 在 `handler.go` 中实现：
  - `GET /` 列出 `plan|skill|snippet`
  - `POST /messages` 通过 `AppendMessageCtx` 写入结构化消息
  - `POST /distill` 通过 `AppendArtifactCtx` 把 `skill` 提炼为 `skill-doc`
- [ ] 在 `plugin.go` 中实现：
  - `ID()`
  - `Manifest()`
  - `Handler(...)`
- [ ] 修改：
  - [internal/plugins/haonewsteam/plugin.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/plugin.go)
  - 在 Registry 注册 `planexchange.New()`
- [ ] 修改：
  - [internal/plugins/haonewsteam/haonews.plugin.json](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/haonews.plugin.json)
  - 版本升为 `0.2.0`
  - 增加 `room_plugins`
- 完成标准：
  - `plan-exchange` 插件已能挂载
  - 三类消息能通过标准 Team Message 路径存储
  - Skill 提炼能通过标准 Artifact 路径落地

### Phase 4. First Room Theme `minimal` (`P3`)

- [ ] 新建 Room Theme 目录：
  - [internal/themes/room-themes/minimal/roomtheme.json](/Users/haoniu/sh18/hao.news2/haonews/internal/themes/room-themes/minimal/roomtheme.json)
  - [internal/themes/room-themes/minimal/web/templates/room_channel.html](/Users/haoniu/sh18/hao.news2/haonews/internal/themes/room-themes/minimal/web/templates/room_channel.html)
- [ ] 新建默认 fallback 模板：
  - [internal/plugins/haonews/web/templates/room_channel_default.html](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonews/web/templates/room_channel_default.html)
- [ ] 在 Team Channel 渲染链上接入 Room Theme 优先级：
  - `Room Theme → 全局 Theme / 默认 Team Channel 模板`
- [ ] 如果模板系统需要，注册 `structuredJSON` 或等效模板函数。
- [ ] Channel 页面需展示：
  - `AgentOnboarding`
  - `Rules`
  - `Plugin`
  - `Theme`
- 完成标准：
  - minimal 主题能渲染结构化频道页面
  - 没有 Room Theme 的频道继续使用现有 Team Channel 页面

### Phase 5. Tests And Docs (`P4`)

- [ ] 新建测试：
  - [internal/haonews/team/channel_config_test.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/channel_config_test.go)
  - [internal/plugins/haonewsteam/roomplugin/registry_test.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/roomplugin/registry_test.go)
- [ ] 增补插件层测试，覆盖至少：
  - `GET /api/teams/{teamID}/channels/{channelID}/config`
  - `PUT /api/teams/{teamID}/channels/{channelID}/config`
  - Room Plugin 路由挂载
  - `POST /api/teams/{teamID}/r/plan-exchange/messages`
  - `GET /api/teams/{teamID}/r/plan-exchange/?channel_id=...&kind=...`
  - `POST /api/teams/{teamID}/r/plan-exchange/distill`
- [ ] 新建文档：
  - [doc-md/team-room-plugin.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-room-plugin.md)
  - 内容必须包括：
    - Room Plugin 接口
    - 注册方式
    - `roomplugin.json`
    - Channel Config 使用方式
    - Theme 覆盖方式
- 完成标准：
  - 新 API / 路由 / 插件主链都有测试覆盖
  - 开发文档能支撑后续长期按这个模式继续扩展

### Phase 6. Final Verification And Writeback

- [ ] 先跑阶段性验证：
  - `go test ./internal/haonews/team -count=1`
  - `go test ./internal/plugins/haonewsteam -count=1`
  - 如新增 roomplugin / room themes 包，补对应 package tests
- [ ] 最终强制验证：
  - `go build ./...`
  - `go test ./...`
- [ ] 若本地可起服务，补最小 smoke：
  - Channel Config API
  - Room Plugin 路由
  - plan-exchange 消息创建 / 过滤 / distill
- [ ] 更新本文档 `Status Writeback`
- [ ] 只在所有本轮任务完成后，才允许进入单独发布批次
- 完成标准：
  - 两份源文档要求的 P1-P4 已执行完
  - 所有强制验证已通过
  - 剩余项若存在，必须明确标 `defer` 并给出原因

## Verification

- 分阶段首选验证：
  - `go test ./internal/haonews/team -run 'TestChannelConfig.*' -count=1`
  - `go test ./internal/plugins/haonewsteam/roomplugin -count=1`
  - `go test ./internal/plugins/haonewsteam -run 'Test.*ChannelConfig|Test.*RoomPlugin|Test.*PlanExchange' -count=1`
- 最终强制验证：
  - `go build ./...`
  - `go test ./...`
- 若起本地服务，补充：
  - `GET /api/teams/{teamID}/channels/{channelID}/config`
  - `PUT /api/teams/{teamID}/channels/{channelID}/config`
  - `GET /api/teams/{teamID}/r/plan-exchange/?channel_id=...&kind=skill`
  - `POST /api/teams/{teamID}/r/plan-exchange/messages`
  - `POST /api/teams/{teamID}/r/plan-exchange/distill`
- 完成标准：
  - [ ] `go build ./...` 通过
  - [ ] `go test ./...` 通过
  - [ ] Channel Config API 工作正常
  - [ ] Room Plugin 路由可挂载
  - [ ] `plan-exchange` 可发布 / 查询 / 提炼
  - [ ] minimal 主题可渲染
  - [ ] 现有 Team 主链不回退

## Fallback Rules

- 若 Team 模板系统暂时无法直接支持 Room Theme 多级覆盖：
  - 先实现 `Room Theme → room_channel_default.html`
  - 再保持原 Team Channel 模板作为最终 fallback
- 若 `structuredJSON` 模板函数接入点不明显：
  - 先用 handler 侧预格式化字符串或现有 JSON helper
  - 不为此重写整个模板系统
- 若 `handleAPITeam` 当前返回结构过于固定：
  - 允许改为兼容型匿名结构体，附加 `channel_configs`
  - 不拆出新的大 API 体系
- 若 `plan-exchange` Web 页面一时做不全：
  - 先保证 API 完整
  - 但不能跳过消息创建 / 列表 / distill 主链
- 不允许的 fallback：
  - 跳过 `P1` 直接硬塞业务逻辑进现有 Team handler
  - 把 `ChannelConfig` 塞回现有 `channels.json`
  - 直接改动 Team P2P / sync 核心层
  - 以“先文档、后实现”替代本轮要求的真实代码落地

## Blockers / Resume

- 硬阻塞定义：
  - 现有模板 / 插件装载机制无法在不改底层核心的前提下支持 Room Theme / Room Plugin
  - 源文档要求与现有核心约束直接冲突，且没有兼容实现路径
  - `go build ./...` 或 `go test ./...` 因无法规避的环境/依赖问题持续失败
- 如果阻塞，必须写回：
  - `Blocked on`: 精确卡点
  - `Tried`: 已尝试的 fallback
  - `Why blocked`: 为什么仍无法继续
  - `Exact next step`: 恢复时第一步
- 恢复执行时：
  - 先通读本文件
  - 从最后一个未完成且未失效的 Phase 恢复

## Status Writeback

- `Completed`:
  - `done`:
    - Team 主链、Team 产品化页面、Team P2P、Webhook/A2A/SSE 基线
    - `P1` Room Plugin Registry：
      - [internal/plugins/haonewsteam/roomplugin/registry.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/roomplugin/registry.go)
      - [internal/haonews/team/channel_config.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/channel_config.go)
      - `ctx/compat` Channel Config API
      - Team `/teams/{teamID}/r/{pluginID}` 与 `/api/teams/{teamID}/r/{pluginID}` 挂载点
      - Team detail API `channel_configs`
    - `P2` 第一个 Room Plugin：
      - [internal/plugins/haonewsteam/rooms/planexchange/plugin.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/rooms/planexchange/plugin.go)
      - [internal/plugins/haonewsteam/rooms/planexchange/handler.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/rooms/planexchange/handler.go)
      - [internal/plugins/haonewsteam/rooms/planexchange/types.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/rooms/planexchange/types.go)
      - `POST /messages`
      - `GET /?channel_id=...&kind=...`
      - `POST /distill`
    - `P3` 第一个 Room Theme：
      - [internal/themes/room-themes/minimal/theme.go](/Users/haoniu/sh18/hao.news2/haonews/internal/themes/room-themes/minimal/theme.go)
      - [internal/themes/room-themes/minimal/web/templates/room_channel.html](/Users/haoniu/sh18/hao.news2/haonews/internal/themes/room-themes/minimal/web/templates/room_channel.html)
      - [internal/themes/haonews/web/templates/room_channel_default.html](/Users/haoniu/sh18/hao.news2/haonews/internal/themes/haonews/web/templates/room_channel_default.html)
    - `P4` 测试与文档：
      - [internal/haonews/team/channel_config_test.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/channel_config_test.go)
      - [internal/plugins/haonewsteam/roomplugin/registry_test.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/roomplugin/registry_test.go)
      - [doc-md/team-room-plugin.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-room-plugin.md)
    - 额外收口：
      - [internal/plugins/haonews/index_cache.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonews/index_cache.go)
      - 调整为 `governanceIndex → ApplySubscriptionRules → moderation decisions`
      - 修复 pending-approval / auto-approve 在治理重建索引后被冲掉的问题
    - 测试稳定性修正：
      - [internal/haonews/team/store_test.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/store_test.go)
      - [internal/plugins/haonews/index_cache_test.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonews/index_cache_test.go)
      - 前者等待 webhook delivery ledger 落地，后者移除会互踩全局变量的 `t.Parallel()`
  - `defer`:
    - 新的复制协议
    - 底层存储重构
    - 非必要的全局 UI 重做
  - `Verification`:
    - `go test ./internal/haonews/team ./internal/plugins/haonewsteam ./internal/plugins/haonewsteam/roomplugin ./internal/themes/room-themes/minimal ./internal/plugins/haonewscontent -count=1`
    - `go test ./internal/plugins/haonews -count=1`
    - `go test ./internal/haonews -count=1`
    - `go test ./... -timeout 120s`
    - `go build ./...`
- `Blocked`:
  - None
- `Next Step`:
  - 若继续本架构主线，可进入单独发布批次。
  - 这轮额外收掉了原先的测试 blocker：
    - [internal/haonews/team_sync.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team_sync.go)
      - `shouldRetryPending(...)` 只重试真正 `pending` 的项
      - pending 过期判断改看 `UpdatedAt`，不再错误依赖 `VersionAt`
      - `compactTeamSyncState(...)` 同步改为按 `UpdatedAt` 处理 pending 生命周期
    - [internal/haonews/listenaddr_test.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/listenaddr_test.go)
      - 改成显式申请共享 TCP/UDP 端口，消除 `TestResolveLibP2PListenAddrsIncrementsSharedPort` 的随机端口冲突
