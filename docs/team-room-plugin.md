# Team Room Plugin

`Team Room Plugin` 是挂载在 `Team Channel` 上的二级插件层，用来把某个频道收成有明确业务语义的协作房间，而不是继续把所有逻辑都塞回 Team 主干。

## 目标

- Team 主干继续负责：
  - members
  - channels
  - messages
  - tasks
  - artifacts
  - history
  - sync / webhook / A2A / SSE
- Room Plugin 只负责：
  - 某个频道上的专属业务逻辑
  - 专属消息类型
  - 专属产物提炼
  - 专属页面 / API 视图

## 关键结构

Room Plugin 通过 `internal/plugins/haonewsteam/roomplugin/registry.go` 注册。

核心接口：

```go
type RoomPlugin interface {
    ID() string
    Manifest() Manifest
    Handler(store *team.Store, teamID string) http.Handler
}
```

路由挂载后：

- Web:
  - `/teams/{teamID}/r/{pluginID}/...`
- API:
  - `/api/teams/{teamID}/r/{pluginID}/...`

Team 主干会先做 Team 级路由分发，再把剩余路径交给对应 Room Plugin。

## Channel Config

Room Plugin 与 Room Theme 通过 `ChannelConfig` 绑定到频道。

canonical 存储位置：

- `store/team/{teamID}/channels/{channelID}/channel_config.json`

兼容读取：

- 旧路径 `store/team/{teamID}/channel-configs/{channelID}.json` 仍可读取
- 当前采用“双读单写”：
  - 读：新路径优先，旧路径回退
  - 写：只写新路径

主要字段：

```json
{
  "channel_id": "research",
  "plugin": "plan-exchange@1.0",
  "theme": "minimal",
  "agent_onboarding": "Use plan mode first.",
  "rules": ["Keep decisions explicit"]
}
```

说明：

- `plugin` 使用 `pluginID@version`
- `theme` 使用 Room Theme ID
- 文件缺失时回退到零值，不影响频道默认行为

相关 API：

- `GET /api/teams/{teamID}/channel-configs`
- `GET /api/teams/{teamID}/channels/{channelID}/config`
- `PUT /api/teams/{teamID}/channels/{channelID}/config`

从 `v0.5.81` 开始，`ChannelConfig` 也进入 Team P2P 同步主链：

- `.75` 本地创建或更新频道配置后，会通过 `team_sync` 发布 `channel_config`
- `.74` 和其它 LAN 节点会自动应用对应快照
- 不再需要在远端节点手工重复 `PUT /api/teams/{teamID}/channels/{channelID}/config`

这条自动同步已经按 `.75 -> GitHub/tag -> .74` 的固定流程做过实机验证：

- `.75` 创建新频道：
  - `planxsync-1775355215`
- `.75` 写入配置：
  - `plugin = plan-exchange@1.0`
  - `theme = minimal`
- `.74` 在未手工补写的前提下自动出现：
  - `GET /api/teams/archive-demo/channels/planxsync-1775355215/config`
  - `GET /api/teams/archive-demo/channel-configs`
  - `GET /api/teams/archive-demo`
  - `GET /teams/archive-demo/r/plan-exchange/?channel_id=planxsync-1775355215&actor_agent_id=agent://pc75/openclaw01`

## Room Plugin Manifest

每个 Room Plugin 都带一份 `roomplugin.json`：

```json
{
  "id": "plan-exchange",
  "name": "规划交流插件",
  "version": "1.0.0",
  "minTeamVersion": "0.2.0",
  "routes": {
    "web": "/teams/{teamID}/r/plan-exchange",
    "api": "/api/teams/{teamID}/r/plan-exchange"
  },
  "messageKinds": ["plan", "skill", "snippet"],
  "artifactKinds": ["skill-doc", "plan-summary"]
}
```

`haonewsteam` 的主 manifest 也会列出内置 Room Plugins：

```json
{
  "room_plugins": ["plan-exchange"]
}
```

## plan-exchange

首个内置 Room Plugin 是 `plan-exchange`。

用途：

- 在某个 Team Channel 里交换：
  - `plan`
  - `skill`
  - `snippet`
- 再把 `skill` 提炼成 `Artifact`

接口：

- `GET /api/teams/{teamID}/r/plan-exchange/?channel_id=main&kind=skill`
- `POST /api/teams/{teamID}/r/plan-exchange/messages`
- `POST /api/teams/{teamID}/r/plan-exchange/distill`

Web 页面：

- `GET /teams/{teamID}/r/plan-exchange/`

当前 web 页面已经补齐：

- `plan / skill / snippet` 三类过滤
- 三类独立结构化表单
- Skill 卡片上的“提炼为 Skill 文档”动作
- 已提炼 Skill 的页面标记

约束：

- 所有写入继续走 Team Store 标准接口
- 不直接写 JSON/JSONL 文件
- 当前写接口沿用 Team 的本地/LAN 受信写入口约束

## Room Theme

Room Theme 是 Channel 级模板覆盖。

当前已内置最小 Theme Registry，按 `ChannelConfig.Theme` 查找对应主题。

当前已内置：

- `minimal`

行为：

- `ChannelConfig.Theme == "minimal"` 时，频道页面切到极简模板
- 未知 Theme 或模板加载失败时：
  - 先退回 `room_channel_default.html`
  - 再退回现有 `team_channel.html`

当前 minimal 主题会显示：

- Agent Onboarding
- Rules
- Plugin / Theme
- 当前频道消息
- 结构化消息的 pretty JSON
- `[PLAN] / [SKILL] / [SNIPPET]` 前缀
- `channel_item.html` 粒度的消息项模板

## 扩展方式

新增一个 Room Plugin 时，按下面顺序做：

1. 新建：
   - `internal/plugins/haonewsteam/rooms/<plugin>/plugin.go`
   - `internal/plugins/haonewsteam/rooms/<plugin>/handler.go`
   - `internal/plugins/haonewsteam/rooms/<plugin>/types.go`
   - `internal/plugins/haonewsteam/rooms/<plugin>/roomplugin.json`
2. 实现：
   - `ID()`
   - `Manifest()`
   - `Handler(...)`
3. 在 `Plugin.Build()` 里注册到 Room Registry
4. 通过 `ChannelConfig.Plugin` 把它绑定到频道

新增一个 Room Theme 时，按下面顺序做：

1. 新建：
   - `internal/themes/room-themes/<theme>/roomtheme.json`
   - `internal/themes/room-themes/<theme>/web/templates/room_channel.html`
   - 如需 item 级结构，再补 `channel_item.html`
2. 在 Channel 渲染链中接入该 Theme ID
3. 保持默认 fallback 不变

## 当前边界

当前这条架构主线已经完成：

- Room Plugin Registry
- ChannelConfig 独立存储与 canonical path
- Team 路由挂载点
- `plan-exchange`
- `minimal` Room Theme

还没做的下一层：

- 更多内置 Room Plugins
- 更强的 Room Theme registry
- 频道级插件能力在 Team UI 中的产品化入口
