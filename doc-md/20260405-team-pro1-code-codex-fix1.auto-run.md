# `20260405-team-pro1-code-codex-fix1.auto-run.md`

## Goal

- 补齐 [20260405-team-pro1.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/20260405-team-pro1.md) 和 [20260405-team-pro1-code.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/20260405-team-pro1-code.md) 中**已明确要求、但上一轮未真正落地**的 Team 新架构核心缺口，使 `Room Plugin + Channel Config + Room Theme` 不再停留在“能挂载”，而进入“按文档定义可长期扩展”的状态。

## Context

- 仓库 / 工作目录：
  - `/Users/haoniu/sh18/hao.news2/haonews`
- 输入文档：
  - [20260405-team-pro1.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/20260405-team-pro1.md)
  - [20260405-team-pro1-code.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/20260405-team-pro1-code.md)
  - 模板：[runbook-template.auto-run.md](/Users/haoniu/sh18/hao.news2/runbook-template.auto-run.md)
- 已完成基线：
  - Room Plugin Registry 已存在
  - `ChannelConfig` 数据层 / `ctx+compat` API 已存在
  - `plan-exchange` API 主链已存在
  - `minimal` Room Theme 已存在
  - `go build ./...` 与 `go test ./...` 已收绿
- 本轮不重复做的内容：
  - 不重做已完成的 `P1-P4` 基础挂载
  - 不改动 `internal/haonews/team/sync.go`
  - 不改动 P2P 协议和 JSON/JSONL 存储格式
- 当前明确还漏掉、且会影响“以后按这个模式一直走下去”的点：
  1. `RoomPlugin Manifest` 还缺源文档要求的 `MinTeamVersion` / `Routes`
  2. `ChannelConfig` 目前落在 `channel-configs/{channel}.json`，未对齐设计文档的 `{channels}/{channelID}/channel_config.json`
  3. Team API 只回了 `channel_configs`，没有按设计提供 `channels_config` 摘要语义
  4. `Room Theme` 仍是 `minimal` 硬编码分支，没有真正的 Theme Registry / Loader
  5. `minimal` 缺 `channel_item.html`
  6. `plan-exchange` 只完成了 API，没有完成专属 web 视图 / 三类表单 / Skill 已提炼标记
  7. `handler.go` 仍不是“纯路由分发”边界

## Execution Contract

- 读完整个文档后立即开始执行。
- 在本 runbook 完成或确认硬阻塞之前，不向用户提问。
- 普通实现分歧、路径分歧、文件落点分歧、命名分歧，一律按低风险、可回滚、与源文档更一致的方案直接裁决。
- 本轮允许对上一轮的实现做**兼容性收口**，但不允许把已发布能力回退。
- 任何涉及旧路径迁移的改动必须：
  - 保持旧数据可读
  - 保持旧 API 字段不立即消失
  - 以“新路径为主，旧路径兼容读/必要时写回迁移”为默认策略
- 如果出现阻塞：
  - 先尝试兼容 fallback
  - 再尝试局部 shim / adapter
  - 只有所有低风险 fallback 都失败后，才允许停止
- 停止前必须把 blocker、已完成内容、已尝试动作、精确恢复下一步写回本文档。

## Planning Rules

- 先补**架构约束缺口**，再补 UI。
- 先补**以后会复制使用的模式**，再补一次性页面细节。
- 优先做会影响后续 Room Plugin/Theme 扩展的基础件：
  - Manifest 规范
  - Theme Loader
  - ChannelConfig canonical path
  - Team API onboarding 语义
- `plan-exchange` web 页面必须在这些基础件收口后再补，避免第二次返工。

## Execution Plan

### Phase 1. Inspect And Reconcile Gaps

- [ ] 重新核对源文档与现实现状，确认以下差距仍真实存在：
  - `roomplugin.Manifest` 缺 `MinTeamVersion` / `Routes`
  - `ChannelConfig` canonical 路径未对齐
  - `channels_config` 摘要未返回
  - Theme 仍为硬编码分支
  - `minimal/channel_item.html` 缺失
  - `plan-exchange` web 专属视图未落地
  - `handler.go` 仍混有非路由职责
- [ ] 只读取最小必要文件：
  - [internal/plugins/haonewsteam/roomplugin/registry.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/roomplugin/registry.go)
  - [internal/haonews/team/channel_config.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/channel_config.go)
  - [internal/plugins/haonewsteam/plugin.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/plugin.go)
  - [internal/plugins/haonewsteam/handler.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/handler.go)
  - [internal/plugins/haonewsteam/handler_channel.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/handler_channel.go)
  - [internal/plugins/haonewsteam/rooms/planexchange/plugin.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/rooms/planexchange/plugin.go)
  - [internal/plugins/haonewsteam/rooms/planexchange/handler.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/rooms/planexchange/handler.go)
  - [internal/themes/room-themes/minimal/theme.go](/Users/haoniu/sh18/hao.news2/haonews/internal/themes/room-themes/minimal/theme.go)
- [ ] 在 `Status Writeback` 中记录确认后的缺口列表。
- 完成标准：
  - 本轮不是“猜测缺口”，而是明确知道要补哪些未落地要求

### Phase 2. Fix P1 Middle-Layer Omissions

- [ ] 扩展 `roomplugin.Manifest`，补齐：
  - `MinTeamVersion`
  - `Routes`（`web` / `api`）
- [ ] 保持 `roomplugin.json` 与 Manifest 结构一致，补齐 `plan-exchange` manifest 中对应字段。
- [ ] 让 `ChannelConfig` 存储进入 canonical 路径：
  - 新主路径：`{StoreRoot}/team/{teamID}/channels/{channelID}/channel_config.json`
  - 旧路径：`channel-configs/{channelID}.json` 作为兼容读取
  - 默认策略：
    - 读取优先新路径
    - 若新路径缺失则回退旧路径
    - 保存写入新路径
- [ ] Team API 对齐设计语义：
  - `GET /api/teams/{teamID}` 同时保留：
    - 现有 `channel_configs`
    - 新增 `channels_config` 摘要字段
  - 让 Agent onboarding 能不额外扫描页面即可拿到配置摘要
- [ ] 抽出 Room Theme Loader / Registry：
  - 不再在 `handler_channel.go` 里硬编码 `minimal`
  - 建立最小可扩展装载点
  - 按 `ChannelConfig.Theme` 查找 Theme
  - 渲染优先级对齐文档：
    - `Room Theme -> Team 默认 Theme -> 全局/当前 fallback`
- [ ] 收 `handler.go` 到更接近“路由分发”：
  - 只处理 Team 主分发
  - 把新增的 Room Plugin / ChannelConfig 业务逻辑继续留在专门 handler 文件
- 完成标准：
  - 以后新增 Room Plugin/Theme 时，不需要再改硬编码分支
  - `ChannelConfig` 路径与架构文档一致，同时不破坏旧数据读取

### Phase 3. Fix P2 plan-exchange Product Surface

- [ ] 为 `plan-exchange` 补齐 web 侧目录和模板：
  - `templates/channel.html`
  - `templates/plan_form.html`
  - `templates/skill_form.html`
  - `templates/snippet_form.html`
- [ ] `GET /teams/{teamID}/r/plan-exchange/` 不再只回 JSON：
  - web 路由返回结构化页面
  - API 路由继续返回 JSON
- [ ] 页面必须体现：
  - 按 `plan / skill / snippet` 分型的卡片视图
  - 按类型过滤
  - 三类专属发布表单，而不是单一自由文本框
- [ ] `Skill -> Artifact` 提炼链补齐页面语义：
  - Skill 卡片上有“提炼为 Skill 文档”动作
  - 已提炼的 Skill 有标记
  - 不改底层存储路径，只补 UI/查询层
- 完成标准：
  - `plan-exchange` 不再只是 API 插件，而是完成了源文档要求的首个“可用 Room Plugin”

### Phase 4. Fix P3 minimal Theme Omissions

- [ ] 新增：
  - `internal/themes/room-themes/minimal/web/templates/channel_item.html`
- [ ] `minimal` 主题按源文档设计原则渲染：
  - `[PLAN] / [SKILL] / [SNIPPET]` 前缀
  - 保留完整 `structured_data`
  - 适合 Agent 读取和开发调试
- [ ] 如果模板系统还不支持 item 级 override：
  - 先在 `channel.html` 中显式 include / 内联复用 `channel_item.html`
  - 不为此重写整个模板系统
- 完成标准：
  - `minimal` 不再只有一张大模板，而是具备最小消息项级结构

### Phase 5. Verify And Docs

- [ ] 补测试覆盖：
  - Manifest 新字段加载
  - ChannelConfig 新旧路径兼容读取
  - Team detail API 同时返回 `channel_configs` 和 `channels_config`
  - Theme loader 选择逻辑
  - `plan-exchange` web 页、过滤、表单提交、distill 标记
  - `minimal` 主题消息项渲染
- [ ] 跑最相关验证：
  - `go test ./internal/haonews/team -count=1`
  - `go test ./internal/plugins/haonewsteam/... -count=1`
  - `go build ./...`
  - `go test ./...`
- [ ] 更新：
  - [doc-md/team-room-plugin.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-room-plugin.md)
  - 明确写出：
    - canonical `channel_config` 路径
    - Manifest 新字段
    - Theme Loader / override 规则
    - `plan-exchange` 的 web + API 双界面
- 完成标准：
  - 本轮补齐项均有测试或可执行 smoke 证明
  - 文档足以支持后续继续按这个模式扩展

### Phase 6. Close Out

- [ ] 更新本文档 `Status Writeback`
- [ ] 明确写出：
  - 本轮补齐了哪些上一轮遗漏点
  - 还有哪些明确 `defer`
  - 是否还存在阻塞

## Verification

- 重点验证命令：
  - `go test ./internal/haonews/team -count=1`
  - `go test ./internal/plugins/haonewsteam/... -count=1`
  - `go build ./...`
  - `go test ./...`
- 重点 smoke：
  - `GET /api/teams/{teamID}`
  - `GET /api/teams/{teamID}/channels/{channelID}/config`
  - `PUT /api/teams/{teamID}/channels/{channelID}/config`
  - `GET /teams/{teamID}/r/plan-exchange/`
  - `GET /api/teams/{teamID}/r/plan-exchange/?channel_id=...&kind=skill`
  - `POST /api/teams/{teamID}/r/plan-exchange/messages`
  - `POST /api/teams/{teamID}/r/plan-exchange/distill`
- 完成标准：
  - [ ] Manifest / Theme / ChannelConfig 的核心缺口已补齐
  - [ ] `plan-exchange` web + API 双主链可用
  - [ ] `minimal` 主题达到源文档要求的最小完成态
  - [ ] `go build ./...` 通过
  - [ ] `go test ./...` 通过

## Fallback Rules

- `ChannelConfig` 路径迁移默认采用“双读单写”：
  - 读：新路径优先，旧路径兼容
  - 写：只写新路径
- Theme Loader 若暂时不做完整动态扫描：
  - 允许做“注册式 Loader”
  - 但不能继续把 Theme 判断硬编码在 `handler_channel.go`
- `plan-exchange` web 页面若无法一次做复杂前端：
  - 先做服务端模板 + 标准 HTML form
  - 不引入前端框架
- `channel_item.html` 若模板系统不支持单独 override：
  - 允许在 `channel.html` 内显式渲染 item partial
  - 但不能跳过该文件本身

## Blockers / Resume

- 硬阻塞定义：
  - 现有模板系统在不改核心框架的前提下无法支持最小 Theme Loader
  - 新旧 `ChannelConfig` 路径兼容会直接破坏现有数据读取，且没有安全 fallback
  - `go test ./...` 出现与本轮无关但无法绕开的新系统性失败
- 如果阻塞，必须写回：
  - `Blocked on`
  - `Tried`
  - `Why blocked`
  - `Exact next step`
- 恢复时：
  - 先通读本文
  - 从最后一个未完成且未失效的 Phase 继续

## Status Writeback

- `Completed`:
  - `done`:
    - Manifest 规范补齐：
      - [internal/plugins/haonewsteam/roomplugin/registry.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/roomplugin/registry.go)
      - `Manifest` 已补 `MinTeamVersion` / `Routes`
      - [internal/plugins/haonewsteam/rooms/planexchange/roomplugin.json](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/rooms/planexchange/roomplugin.json) 已对齐新字段
    - `ChannelConfig` canonical path 收口：
      - [internal/haonews/team/channel_config.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/channel_config.go)
      - 新主路径：`channels/{channelID}/channel_config.json`
      - 旧路径：`channel-configs/{channelID}.json` 兼容读取
      - 写入改为只写 canonical 路径
    - 修复 canonical path 与旧分片频道判定冲突：
      - [internal/haonews/team/paths.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/paths.go)
      - `isShardedChannel(...)` 改为检查目录中的 `*.jsonl` shard 文件，不再把 config 目录误判为分片频道
    - Team API onboarding 摘要语义：
      - [internal/plugins/haonewsteam/handler.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/handler.go)
      - [internal/plugins/haonewsteam/types.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/types.go)
      - `GET /api/teams/{teamID}` 现在同时返回：
        - `channel_configs`
        - `channels_config`
    - Room Theme Loader / Registry：
      - [internal/themes/room-themes/registry.go](/Users/haoniu/sh18/hao.news2/haonews/internal/themes/room-themes/registry.go)
      - [internal/themes/room-themes/minimal/theme.go](/Users/haoniu/sh18/hao.news2/haonews/internal/themes/room-themes/minimal/theme.go)
      - [internal/plugins/haonewsteam/plugin.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/plugin.go)
      - [internal/plugins/haonewsteam/handler_channel.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/handler_channel.go)
      - `minimal` 不再是硬编码分支，而是通过 Theme Registry 装载
    - `plan-exchange` web + API 双主链：
      - [internal/plugins/haonewsteam/rooms/planexchange/handler.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/rooms/planexchange/handler.go)
      - [internal/plugins/haonewsteam/rooms/planexchange/templates/channel.html](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/rooms/planexchange/templates/channel.html)
      - [internal/plugins/haonewsteam/rooms/planexchange/templates/plan_form.html](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/rooms/planexchange/templates/plan_form.html)
      - [internal/plugins/haonewsteam/rooms/planexchange/templates/skill_form.html](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/rooms/planexchange/templates/skill_form.html)
      - [internal/plugins/haonewsteam/rooms/planexchange/templates/snippet_form.html](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/rooms/planexchange/templates/snippet_form.html)
      - `GET /teams/{teamID}/r/plan-exchange/` 已返回结构化页面
      - 三类专属表单已接通
      - Skill 提炼后已有页面标记
    - `minimal` item 级模板补齐：
      - [internal/themes/room-themes/minimal/web/templates/channel_item.html](/Users/haoniu/sh18/hao.news2/haonews/internal/themes/room-themes/minimal/web/templates/channel_item.html)
      - [internal/themes/room-themes/minimal/web/templates/room_channel.html](/Users/haoniu/sh18/hao.news2/haonews/internal/themes/room-themes/minimal/web/templates/room_channel.html)
      - 现在按 `[PLAN] / [SKILL] / [SNIPPET]` 渲染消息项
    - 文档更新：
      - [doc-md/team-room-plugin.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-room-plugin.md)
    - 测试补齐：
      - [internal/haonews/team/channel_config_test.go](/Users/haoniu/sh18/hao.news2/haonews/internal/haonews/team/channel_config_test.go)
      - [internal/plugins/haonewsteam/roomplugin/registry_test.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/roomplugin/registry_test.go)
      - [internal/plugins/haonewsteam/plugin_test.go](/Users/haoniu/sh18/hao.news2/haonews/internal/plugins/haonewsteam/plugin_test.go)
  - `defer`:
    - 更完整的 Theme 动态文件扫描与热装载
    - `plan-exchange` 更复杂的富交互前端
    - 进一步把 `handler.go` 收成更薄的总路由壳
  - `Verification`:
    - `go test ./internal/haonews/team ./internal/plugins/haonewsteam/... -count=1`
    - `go build ./...`
    - `go test ./... -timeout 120s`
- `Blocked`:
  - None
- `Next Step`:
  - 这份补缺 runbook 已完成。
  - 若继续，最自然的是：
    - 单开发布批次做 `main + tag + release`
    - 或把 `.75 / .74` 升到这版并复核：
      - `/api/teams/{teamID}`
      - `/teams/{teamID}/r/plan-exchange/`
      - `/api/teams/{teamID}/channels/{channelID}/config`
