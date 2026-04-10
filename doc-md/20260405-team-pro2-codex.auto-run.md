# `20260405-team-pro2-codex.auto-run.md`

## Goal

- 把 Team 的 `Room Plugin + Channel Config + Room Theme` 从“已可挂载”推进到“频道级可配置、可发现、可进入、可扩展”的下一阶段产品化主线，并落地第二个内置 Room Plugin 的最小闭环。

## Context

- 仓库 / 工作目录：
  - `/Users/haoniu/sh18/hao.news2/haonews`
- 已知事实：
  - `Room Plugin Registry`、`ChannelConfig` canonical path、`plan-exchange`、`minimal` Theme 已完成并已发布到 `v0.5.80+`
  - `channel_config` 已进入 TeamSync 自动复制主链，并已按 `.75 -> GitHub/tag -> .74` 验证通过
  - 当前文档 [doc-md/team-room-plugin.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-room-plugin.md) 明确指出下一层是：
    - 更多内置 Room Plugins
    - 更强的 Room Theme registry
    - 频道级插件能力在 Team UI 中的产品化入口
  - 当前不要再把 `.74` 当主调试场，节点问题默认按：
    - `.75` 先调试
    - GitHub 发布
    - `.74` 从 tag 升级
- 当前明确不要做的事：
  - 不重写 Team Store 主干
  - 不改坏现有 Team sync / webhook / A2A / SSE 主链
  - 不为了“更多插件”引入高风险通用 DSL 或复杂插件沙箱
- 输入材料：
  - [20260405-team-pro1.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/20260405-team-pro1.md)
  - [20260405-team-pro1-code.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/20260405-team-pro1-code.md)
  - [doc-md/team-room-plugin.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/team-room-plugin.md)
  - 现有代码：
    - `internal/plugins/haonewsteam/rooms/planexchange`
    - `internal/plugins/haonewsteam/roomplugin`
    - `internal/themes/room-themes`

## Execution Contract

- 读完整个文档后立即开始执行。
- 在本 runbook 完成或确认硬阻塞之前，不要向用户提问。
- 普通实现分歧、文件落点分歧、命名分歧，直接选低风险、可回滚、与现状一致的路径继续。
- 当前主线优先级：
  1. 频道级插件入口产品化
  2. 第二个内置 Room Plugin 最小闭环
  3. Theme Registry 第二层
  4. 测试 / 文档 / 运行态验证
- 遇到小问题先本地 fallback，不要因为普通不确定性停下来。
- 若出现硬阻塞，必须先把状态写回本文档，再停止。

## Planning Rules

- 按“先让用户看见和用到，再补第二个插件，再补 registry 第二层”的顺序执行。
- 先做 Team UI 和 ChannelConfig 的产品化入口，减少后续插件落地的摩擦。
- 第二个内置 Room Plugin 必须：
  - 复用现有 Team Store 和 Team Message/Artifact 语义
  - 不引入新的底层存储格式
  - 能通过 web + API 双路径验证
- Theme Registry 第二层只做最小实用增强：
  - 支持 metadata / label / preview 等静态信息
  - 不做复杂运行时动态加载

## Execution Plan

### Phase 1. Inspect and Freeze the Baseline

- [x] 只读取最小必要上下文：
  - `doc-md/team-room-plugin.md`
  - `internal/plugins/haonewsteam/roomplugin/*`
  - `internal/plugins/haonewsteam/rooms/planexchange/*`
  - `internal/themes/room-themes/*`
  - Team detail / channel / config 相关模板与 handler
- [x] 明确当前差距：
  - Team UI 里哪些地方还不能直接发现或进入 Room Plugin
  - Theme registry 还缺哪些最小元数据
  - 第二个 Room Plugin 该复用哪条现有语义主线

完成标准：
- 能列清：
  - 哪些页面/API要改
  - 第二个插件的最小作用域
  - 哪些文件会被改

### Phase 2. Productize Channel Plugin Entry

- [x] 在 Team 详情页 / 频道页补齐频道级能力入口：
  - 当前频道使用的 `plugin / theme`
  - 进入 Room Plugin 的直接链接
  - 进入 Channel Config 的直接入口
  - 未配置插件时的明确空状态提示
- [x] 让 Team API / 频道 API 返回更适合 UI 的插件入口信息：
  - `room_plugin_id`
  - `room_plugin_route`
  - `room_theme_id`
  - `channel_config_state`
- [x] 把 `channel-configs` 从“纯设置接口”收成“可发现的产品入口”

完成标准：
- Team 页面上能一眼看出：
  - 哪些频道有 Room Plugin
  - 用的是什么 Theme
  - 点击即可进入对应房间页

### Phase 3. Add the Second Built-in Room Plugin

- [x] 选一个低风险、复用现有语义的第二个内置 Room Plugin：
  - 默认采用“review-room / 决策评审室”这类轻量插件
  - 只基于 Team Message / Artifact / ContextID，不引入新底层格式
- [x] 新建插件目录：
  - `internal/plugins/haonewsteam/rooms/<plugin>/`
- [x] 补齐：
  - `plugin.go`
  - `handler.go`
  - `types.go`
  - `roomplugin.json`
  - 最小模板
- [x] 至少提供：
  - `GET web`
  - `GET api`
  - `POST message/create`
  - 可选 `distill` 或 `artifact/create`
- [x] 注册到 Room Registry，并通过 `ChannelConfig.Plugin` 可挂载

完成标准：
- 第二个插件能和 `plan-exchange` 一样通过 `ChannelConfig` 挂到某个频道
- web + API 都能访问
- 不破坏现有 `plan-exchange`

### Phase 4. Theme Registry Level 2

- [x] 给 Theme Registry 增加最小 metadata：
  - `id`
  - `name`
  - `version`
  - `description`
  - `overrides`
  - `preview_class` 或等价轻量展示字段
- [x] Team UI / Channel API 能看到可选 Theme 的摘要
- [x] `minimal` 先升级到新 registry 结构，不引入行为回归
- [x] 已配置 Theme 的频道也能回到 `?view=channel` 工作台继续编辑 Room Plugin / Theme 配置

完成标准：
- Theme 不再只是“能加载”，还具备最小产品层可见信息

### Phase 5. Verify and Write Back

- [x] 运行最相关测试：
  - Room Plugin registry
  - Channel config
  - Team plugin routes
  - 新插件 web + API
  - Theme registry
- [x] 运行 `go build ./cmd/haonews`
- [x] 本地运行态至少验证：
  - `/api/teams/{teamID}`
  - `/api/teams/{teamID}/channel-configs`
  - `/teams/{teamID}/channels/{channelID}`
  - `/teams/{teamID}/r/<plugin>/`
- [x] 把真实结果写回本文档

## Verification

- 首选验证命令：
  - `go test ./internal/plugins/haonewsteam ./internal/haonews/team -count=1`
  - `go test ./internal/plugins/haonewsteam/rooms/... -count=1`
  - `go build ./cmd/haonews`
- 备用验证方式：
  - `curl -s http://127.0.0.1:51818/api/teams/<teamID> | python3 -m json.tool`
  - `curl -s http://127.0.0.1:51818/api/teams/<teamID>/channel-configs | python3 -m json.tool`
  - `curl -I http://127.0.0.1:51818/teams/<teamID>/r/<plugin>/?channel_id=<channelID>`
- 完成标准：
  - [ ] 频道级插件入口已产品化
  - [ ] 第二个内置 Room Plugin 已落地
  - [ ] Theme Registry 第二层已落地
  - [ ] 关键测试和构建已通过
  - [ ] 最终汇报包含 `Completed / Blocked / Next Step`

## Fallback Rules

- 第二个插件默认优先选：
  - 复用现有 Team message/artifact 模式的轻量插件
  - 不选需要新存储模型的新插件
- 如果 Theme registry 第二层过大：
  - 先做 metadata 可见性
  - 延后更复杂的动态发现和 UI picker
- 如果某个产品化入口需要较大模板重写：
  - 先补可用入口和状态摘要
  - 后续再做视觉打磨

## Blockers / Resume

- 硬阻塞定义：
  - 第二个插件的最小语义无法在现有 Team Store 上安全复用
  - 现有 Team 路由 / Room Registry 结构出现必须重写级冲突
  - 本地无法完成最小运行态验证且合理 fallback 均失败
- 如果阻塞，必须写回：
  - `Blocked on`
  - `Tried`
  - `Why blocked`
  - `Exact next step`
- 恢复执行时：
  - 先读完整个文档
  - 从最后一个未完成 checklist 继续

## Status Writeback

- `Completed`:
  - 新 runbook 已建立，并已完成 `Phase 1` 到 `Phase 4` 的当前主线：
    - 频道级插件入口产品化
    - Team detail / channel / channel API 的 room entry 收口
    - 第二个内置插件 `review-room`
    - Theme Registry 第二层 metadata / 可见性
    - 频道页内 Room Plugin / Theme 配置器
    - 已应用 Theme 的频道通过 `?view=channel` 回到配置工作台
    - `review-room` 页面统计、历史入口、Review Summary 入口
    - `review-room` 结构化卡片视图：摘要 / 决策或影响 / 行动项
    - 第二个内置 Room Theme：`focus`
    - `review-room` 默认页的 `decision / risk / review` lane 摘要
    - `review-room` Summary API
    - `review-room` lane 状态：`待沉淀 / 待跟进 / 已提炼`
    - `review-room` 工作台状态分组：`待沉淀决策 / 待跟进风险 / 最近已提炼`
    - `review-room` 已提炼卡片可直接打开对应 `Review Summary` Artifact
    - `review-room` 决策沉淀 / 最近产物聚合视图
    - `review-room` Summary API 的 `decision_digests / artifact_digests`
    - `review-room` 结论级产物聚合：`artifact_count / latest_artifact_title / latest_artifact_link`
    - `review-room` 的 `decision_threads`：把 risk/review 聚到对应 decision 下
    - `review-room` 的 `decision_threads` 已接回 Team 主链：`task_search_link / artifact_search_link / history_search_link`
    - `review-room` 的消息和结论线程已支持真实 `task_id / artifact_id` 绑定
    - `review-room` 的结论线程已支持直接动作：
      - `标记任务进行中`
      - `标记任务完成`
      - `沉淀结论线程`
    - `review-room` 的结论线程已支持确定性的 `自动同步线程`
      - 自动建议任务状态
      - 自动同步到 Team Task
      - 在线程缺少结论级沉淀时自动补 `Review Summary`
      - 在线程尚无绑定任务时自动创建并挂接 Team Task
    - `review-room` 已支持 `批量同步全部结论线程`
      - 批量执行自动同步
      - 返回同步线程数 / 新建任务数 / 新建 Artifact 数
    - `review-room` 已支持跨线程工作台摘要
      - `thread_workbench`
      - 已绑定任务 / 待自动建任务
      - 已沉淀产物 / 待补沉淀产物
      - 建议 `blocked / doing / done` 分布
      - 页面可从摘要直接跳到对应结论线程
    - `review-room` 已支持批处理结果持久化/回显
      - 批量同步结果写回 Team history 主链
      - 页面与 Summary API 暴露 `recent_batch_runs`
      - 可见最近批量同步时间 / actor / 已同步线程 / 新建任务数 / 新建产物数 / 建议状态分布
      - 可直接打开本轮新建的 Task / Artifact
      - 可直接打开本轮批处理对应的 Team history
    - `review-room` 已支持更细的线程工作流状态
      - `待风险跟进 / 待评审 / 待沉淀 / 已沉淀待挂接 / 已完成`
      - 已同步进入 `decision_threads / thread_workbench / recent_batch_runs`
      - 页面已形成对应工作台分组
    - `review-room` 已支持跨频道 / 跨上下文收敛
      - `cross_channel_digests`
      - `context_digests`
      - 页面可直接查看“跨频道收敛 / 上下文收敛”
    - 第三个 Room Theme：`board`
      - Theme 选择器里可见
      - 频道页可切换并真实渲染
    - 第三个内置 Room Plugin：`incident-room`
      - `incident / update / recovery`
      - `Summary API`
      - `incident-summary` 提炼链
      - web + API 双入口
      - 已进入 Room Plugin Registry 和频道配置入口
      - 单条消息已支持 `同步到任务`
        - `incident -> blocked`
        - `update -> doing`
        - `recovery -> done`
        - 缺少 `task_id` 时自动创建并挂接 Team Task
      - 当前频道已支持 `批量同步全部消息到任务`
        - `task-sync-all`
        - Summary API 与页面统计已补：
          - `bound_task_count / unbound_task_count`
          - `suggested_blocked_count / suggested_doing_count / suggested_done_count`
        - 会对尚未沉淀的 `recovery` 消息自动补 `incident-summary`
      - `incident-room` 的批量同步结果已写回并回显：
        - Team history 中记录 `message_scope = incident-room`
        - `batch_action = task-sync-all`
        - Summary API 暴露 `recent_batch_runs`
        - 页面可见最近批处理结果、批处理历史入口、新建任务入口、新增产物数
        - 本轮新建的 `Incident Summary` 也可直接打开
    - 第四个内置 Room Plugin：`handoff-room`
      - `handoff / checkpoint / accept`
      - `Summary API`
      - `handoff-summary` 提炼链
      - web + API 双入口
      - 已进入 Room Plugin Registry 和频道配置入口
      - 单条消息已支持 `同步到任务`
        - `handoff -> doing`
        - `checkpoint -> doing`
        - `accept -> done`
        - 缺少 `task_id` 时自动创建并挂接 Team Task
      - 当前频道已支持 `批量同步全部消息到任务`
        - `task-sync-all`
        - Summary API 与页面统计已补：
          - `bound_task_count / unbound_task_count`
          - `suggested_doing_count / suggested_done_count`
        - 会对尚未沉淀的 `accept` 消息自动补 `handoff-summary`
      - `handoff-room` 的批量同步结果已写回并回显：
        - Team history 中记录 `message_scope = handoff-room`
        - `batch_action = task-sync-all`
        - Summary API 暴露 `recent_batch_runs`
        - 页面可见最近批处理结果、批处理历史入口、新建任务入口、新增产物入口
    - 第五个内置 Room Plugin：`artifact-room`
      - `proposal / revision / publish`
      - `artifact-brief` 提炼链
      - web + API 双入口
      - `Summary API`
      - 页面已具备：
        - 三类结构化表单
        - 产物推进统计
        - `Artifact Brief` 入口
        - Team 历史入口
      - 单条消息已支持 `同步到任务`
        - `proposal -> doing`
        - `revision -> doing`
        - `publish -> done`
        - 缺少 `task_id` 时自动创建并挂接 Team Task
      - 当前频道已支持 `批量同步全部消息到任务`
        - `task-sync-all`
        - Summary API 与页面统计已补：
          - `bound_task_count / unbound_task_count`
          - `suggested_doing_count / suggested_done_count`
        - 会对尚未沉淀的 `publish` 消息自动补 `artifact-brief`
      - `artifact-room` 的批量同步结果已写回并回显：
        - Team history 中记录 `message_scope = artifact-room`
        - `batch_action = task-sync-all`
        - Summary API 暴露 `recent_batch_runs`
        - 页面可见最近批处理结果、批处理历史入口、新建任务入口、新增产物入口
    - 第六个内置 Room Plugin：`decision-room`
      - `proposal / option / decision`
      - `decision-note` 提炼链
      - web + API 双入口
      - `Summary API`
      - 单条消息已支持 `同步到任务`
        - `proposal -> doing`
        - `option -> doing`
        - `decision -> done`
        - 缺少 `task_id` 时自动创建并挂接 Team Task
      - 当前频道已支持 `批量同步全部消息到任务`
        - `task-sync-all`
        - Summary API 与页面统计已补：
          - `bound_task_count / unbound_task_count`
          - `suggested_doing_count / suggested_done_count`
        - 会对尚未沉淀的 `decision` 消息自动补 `decision-note`
      - `decision-room` 的批量同步结果已写回并回显：
        - Team history 中记录 `message_scope = decision-room`
        - `batch_action = task-sync-all`
        - Summary API 暴露 `recent_batch_runs`
        - 页面可见最近批处理结果、批处理历史入口、新建任务入口、新增产物入口
    - `incident-room / handoff-room / artifact-room / decision-room`
      的任务绑定现在都支持通过 Team history 自动回填：
      - 即使原始消息没有显式 `task_id`
      - 执行过 `task-sync / task-sync-all` 后
      - 页面与 Summary API 的 `bound_task_count / unbound_task_count`
        以及 `打开绑定任务` 都会稳定回显
- `Blocked`:
  - None
- `Next Step`:
  - 继续时优先做：
    - 继续把 `decision-room / artifact-room / handoff-room / incident-room` 的自动联动做得更强
    - 或继续补新的内置 Room Plugin
    - 若要发布，再单开 `main + tag + release` 批次
