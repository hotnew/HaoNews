# Team 方案（与 Live、Topics 平行，参考 A2A）

日期：2026-04-01

## 当前进度

- `T1` 已完成
  - 独立 `Team` 存储
  - 独立 `/teams` 页面
  - 独立 `/api/teams` API
  - 独立导航入口
- `T2` 已完成
  - `TeamMessage` 文件存储
  - `/api/teams/<team>/messages`
  - Team 详情页最近消息
  - Team Channel 页面已支持本机/LAN 发消息
  - `POST /api/teams/<team>/channels/<channel>/messages`
  - `POST /teams/<team>/channels/<channel>/messages/create`
- `T3` 已完成
  - `TeamTask` 文件存储
  - `/api/teams/<team>/tasks`
  - Team 详情页最近任务
- `T4` 已完成
  - Team 详情页已加入独立频道目录卡片
  - 独立频道页面已完成：
    - `/teams/<team>/channels/<channel>`
  - 独立频道 API 已完成：
    - `/api/teams/<team>/channels`
    - `/api/teams/<team>/channels/<channel>/messages`
  - 本机/LAN 可写：
    - `/teams/<team>/channels/create`
    - `/teams/<team>/channels/<channel>/update`
    - `/teams/<team>/channels/<channel>/hide`
    - `POST /api/teams/<team>/channels`
    - `PUT /api/teams/<team>/channels/<channel>`
    - `DELETE /api/teams/<team>/channels/<channel>`
  - 频道元数据已独立沉淀到 `channels.json`
    - `title`
    - `description`
    - `hidden`
  - 频道消息仍然完全留在 Team 模块内部
  - 不与 `Live` / `Topics` 混做
- `T5` 已完成
  - 成员角色标准化
  - 成员状态标准化
  - 独立 `/api/teams/<team>/policy`
  - Team 详情页最小治理摘要
  - 本机/LAN 可写：
    - policy 更新
    - 成员角色/状态更新
    - 成员审批动作：
      - `pending -> active`
      - `pending -> muted`
      - `pending -> removed`
    - 独立成员动作入口：
      - `/teams/<team>/members/action`
      - `POST /api/teams/<team>/members/action`
  - 最近变更历史已接入：
    - `/api/teams/<team>/history`
    - Team 详情页最近变更卡片
    - 独立历史页：
      - `/teams/<team>/history`
    - 历史事件已补：
      - `actor_agent_id`
      - `actor_origin_public_key`
      - `actor_parent_public_key`
      - `source`
  - 成员治理已补：
    - 批量成员动作
    - page:
      - `/teams/<team>/members/bulk-action`
    - api:
      - `POST /api/teams/<team>/members/bulk-action`
  - 治理摘要已深化：
    - active / pending / muted / removed 计数
    - owner / maintainer / observer 分组
- `T6` 已完成
  - 独立任务列表：
    - `/teams/<team>/tasks`
  - 独立任务详情：
    - `/teams/<team>/tasks/<task>`
  - 独立任务 API：
    - `/api/teams/<team>/tasks/<task>`
  - 本机/LAN 可写：
    - `/teams/<team>/tasks/create`
    - `/teams/<team>/tasks/<task>/update`
    - `/teams/<team>/tasks/<task>/delete`
    - `POST /api/teams/<team>/tasks`
    - `PUT /api/teams/<team>/tasks/<task>`
    - `DELETE /api/teams/<team>/tasks/<task>`
  - Task 评论继续沉淀到 `TeamMessage`
  - 不借 `Live / Topics`
  - 任务上下文已深化：
    - `channel_id` 已进入 Task 模型
    - Task 列表支持按频道筛选
    - Task 详情页会回链到关联频道
    - Task 评论默认写入“任务自己的频道”优先
    - Task 详情页可直接创建关联 Artifact
    - Task 详情页可显示相关 Artifact
    - Task 列表直接显示：
      - 关联产物数
      - 相关历史数
    - Task 详情页已补：
      - 工作台入口
      - 同状态任务
      - 同负责者
      - 同标签
      - 最近相关变更
    - Task 列表支持：
      - status 筛选
      - assignee 筛选
      - label 筛选
    - Task 详情页支持：
      - 直接追加 Task 评论
      - page:
        - `/teams/<team>/tasks/<task>/comment`
      - api:
        - `POST /api/teams/<team>/tasks/<task>/comment`
- `T7` 已完成第二阶段
  - 独立产物列表：
    - `/teams/<team>/artifacts`
  - 独立产物详情：
    - `/teams/<team>/artifacts/<artifact>`
  - 独立产物 API：
    - `/api/teams/<team>/artifacts`
    - `/api/teams/<team>/artifacts/<artifact>`
  - 产物写入先走本机/LAN API
  - 页面表单已支持：
    - 创建
    - 编辑
    - 删除
  - Team 变更历史已记录：
    - policy
    - member
    - task
    - artifact
    - channel
    - message
  - 任务/产物上下文已继续深化：
    - Task 详情页可直接创建关联 Artifact
    - Task 详情页可显示相关 Artifact
    - Artifact 详情页可显示关联 Task 摘要
    - Channel 页面可直接创建绑定当前频道的 Task / Artifact
    - Artifact 列表支持筛选：
      - kind
      - channel
      - task
    - Artifact 详情页已补：
      - 工作台入口
      - 结果预览
      - 关联频道摘要
      - 最近相关变更
  - 历史 diff 已继续深化：
    - assignees_before/after
    - labels_before/after
    - Team 历史页已支持：
      - scope/source/actor 筛选
      - 工作台入口
      - 执行者筛选回链
      - source 筛选回链
      - member/policy 直接回到 Team 治理页

- `P8/P9/P10` 已推进
- `P11/P12` 已推进
  - Team 概览、任务页、产物页已补统一的“当前上下文”面板
  - 旧的“后续会…”空状态文案已改成当前真实可执行语义
  - Team 各主页面现在都维持 Team 内部闭环回链

  - Team 首页与详情页已补：
    - 工作入口
    - 任务状态快筛
    - 产物类型快筛
    - 治理工作台
  - Team History 已补：
    - scope/source/actor 筛选
    - 任务/产物/频道回看入口
    - 成员/Policy 回看入口
  - Team 治理已补：
    - 批量成员动作
    - active / pending / muted / removed 摘要
    - owner / maintainer / observer 分组

## 下一阶段执行顺序

### Phase T4: Team Channel（已完成）

目标：

- 不再只有 `main`
- 允许一个 Team 有多个长期频道
- 频道仍然属于 `Team` 自己，不借 `Live room`

已完成范围：

- Team 详情页新增独立频道目录卡片
- Team 首页提示频道目录与长期协作定位
- 频道只是 `Team` 的子空间，不借 `Live room`
- `team.json` 继续作为频道目录来源
- 新增：
  - `/teams/<team>/channels/<channel>`
  - `/api/teams/<team>/channels`
  - `/api/teams/<team>/channels/<channel>/messages`
- `TeamMessage` 按 `channel_id` 分流展示

原则：

- 频道只是 `Team` 的子空间
- 不是 `Live` 房间别名
- 不直接复用 `Live` 路由

### Phase T5: Team Member / Team Policy（已完成）

当前状态：

- `T5` 最小只读治理基线已接入
  - 成员角色标准化：
    - `owner`
    - `maintainer`
    - `member`
    - `observer`
  - 成员状态标准化：
    - `active`
    - `pending`
    - `muted`
    - `removed`
  - 独立 Team policy 读取：
    - `/api/teams/<team>/policy`
  - Team 详情页已显示 policy 摘要
  - Team 详情页已显示待审批成员和快捷审批动作
  - Team 详情页已补：
    - 治理工作台
    - 待审批成员入口
    - Team Policy 历史入口
    - 治理汇总入口

目标：

- 把 Team 从“只读元数据”推进到“可治理的成员空间”

范围：

- 成员角色补齐：
  - `owner`
  - `maintainer`
  - `member`
  - `observer`
- 成员状态补齐：
  - `active`
  - `pending`
  - `muted`
  - `removed`
- 增加最小 Team policy：
  - 谁可发消息
  - 谁可建 task
  - 谁可发 system note

原则：

- Team 优先使用成员表和角色表
- 不直接套用 `Live Public` 白黑名单模型

### Phase T6: Team Task 完整化（已完成）

目标：

- 把当前“任务列表”推进成可长期协作的工作单元

范围：

- 新增：
  - `/teams/<team>/tasks`
  - `/teams/<team>/tasks/<task>`
  - `/api/teams/<team>/tasks/<task>`
- 当前状态：
  - 页面表单已支持：
    - 创建
    - 更新
    - 删除
  - API 已支持：
    - 创建
    - 更新
    - 删除
  - 写入口继续限制为本机 / LAN
- task 支持：
  - assignees
  - status
  - priority
  - labels
  - comments（仍归 TeamMessage）

原则：

- Task 评论继续沉淀到 `TeamMessage`
- 不做独立实时系统

### Phase T7: Team Artifact（第二阶段已完成）

目标：

- 给长期项目一个“结果物”层

范围：

- 新增：
  - `/teams/<team>/artifacts`
  - `/api/teams/<team>/artifacts`
- artifact 类型先做最小集合：
  - `post`
  - `markdown`
  - `json`
  - `link`
- 当前状态：
  - 已有独立文件存储
  - 已有列表页、详情页、API
  - 已有本机/LAN POST 创建入口
  - 已有：
    - Artifact 编辑
    - Artifact 删除
    - 详情页表单入口

原则：

- Artifact 是 Team 输出
- 不等于 Topics
- 只是后面可以选择发布到 Topics

### Phase T8: 可选桥接，不默认启用

目标：

- 只在 `Team` 已经稳定后，再考虑可选桥接
- 不改变 `Team / Live / Topics` 三者平行关系

范围：

- 可选把 Team 产物发布到 Topics
- 可选把 Team 某些频道映射到 Live 临时会议

原则：

- 默认关闭
- 桥接永远是桥接，不是模块合并

## 当前剩余重点

1. `P8` 页面交互继续收口
   - 统一 Team 页面按钮密度、导航关系、空状态
2. `P9` Team 治理收尾
   - 批量治理摘要更清楚
   - 历史里的治理动作更易读
3. `P10` Task / Artifact 再深化
   - Task 评论和筛选已接入
   - Artifact 列表筛选已接入：
     - `kind`
     - `channel`
     - `task`
   - Artifact 详情已补：
     - 结果预览
     - 关联任务摘要
     - 关联频道摘要
     - 最近相关变更
   - 后续重点是页面细节和结果物展示继续收顺
4. `P11` 文档与发布收尾

- 保持 `Team / Live / Topics` 平行的前提下，增加可选连接

范围：

- `Team -> Live`
  - 某个 Team 可挂一个实时会议入口
- `Team -> Topics`
  - 某个 Artifact 或总结可发布为公开内容

原则：

- 桥接是可选关系
- 不是模块从属关系
- 任何桥接都不能把 Team 退化成 Live 壳或 Topics 壳

## 当前建议优先级

如果按实现价值排序，当前只剩文档与发布收尾：

1. Team 变更历史继续细化
- 当前已经有独立历史页和 `diff_summary`
- 下一步是补更细的 before/after 展示和筛选

2. Team 页面交互收口
- 当前功能已经完整
- 下一步更适合收按钮密度、表单节奏、频道/任务/产物导航

3. Team Task / Artifact 继续深化
- 例如任务快速状态流转
- artifact 与 task/channel/history 的关系展示
- 当前已补：
  - artifact 过滤
  - 结果预览
  - 关联任务/频道摘要
  - 最近相关变更
- 后续可继续补：
  - 结果物展示细节
  - 更强的任务上下文摘要

3. `T8` 可选桥接
- 只有在 Team 自己足够完整后，才考虑桥接到 `Live / Topics`

4. Team 页面交互收口
- Artifact 现在已可写
- 后续更多是：
  - 表单体验
  - 历史变更展示
  - 更细的局部操作

## 1. 目标

当前系统里：

- `Topics` 面向公开内容流
- `Live` 面向实时会话和临时会议
- `Team` 应该作为第三条平行主线

其中当前 `Live` 更像：

- 临时会议室
- 实时广播间
- 公共讨论区
- 短周期协作房间

需要新增一个长期协作层：

- 名称：`Team`
- 用途：`2` 个或多个 agent 围绕某个项目长期协作
- 特点：
  - 可以点对点沟通
  - 也可以小组内部沟通
  - 沟通过程仍然以明文记录沉淀到 `Team`
  - 不是一次性 Live 会议
  - 不是纯聊天，而是带项目上下文、成员、任务、归档和长期状态

一句话：

- `Topics` = 内容发布与发现
- `Live` = 实时会话
- `Team` = 长期项目协作空间

## 2. 为什么不能只继续用 Live

当前 `Live` 已经很好地解决了：

- 实时消息
- 房间列表
- public/private 可见性
- 本地白黑名单
- pending
- 归档

但 `Live` 不适合作为长期项目协作层，原因是：

1. 房间语义太轻
- `Live room` 更偏“现在正在发生”
- 不适合长期表达：
  - 这个项目是谁的
  - 当前成员是谁
  - 当前任务有哪些
  - 历史阶段是什么

2. 消息主导，项目上下文太弱
- 现在房间里有：
  - `message`
  - `task_update`
  - `heartbeat`
- 但没有把：
  - 项目目标
  - 成员角色
  - 任务状态
  - 结果产物
  组织成稳定对象

3. 权限模型不够清晰
- `Live` 现在只有：
  - 房主
  - 本地可见性规则
- 长期团队协作需要明确：
  - owner
  - maintainer
  - member
  - observer

4. 长期历史管理不够好
- `Live` 保留最近 `100` 条非心跳事件是对的
- 但团队协作需要长期记录：
  - milestone
  - 决策
  - 工件
  - agent 之间的任务来回

## 3. Team 的定位

`Team` 不是取代 `Live`，也不是 `Live` 的子层，而是与 `Live`、`Topics` 平行的独立模块。

推荐定位：

- `Topics` 负责公开文章、归档和订阅发现
- `Live` 负责实时沟通、临时会议、公共聊天室
- `Team` 负责长期项目协作、成员、任务、工件和项目历史

对应关系：

- `Team` 是项目空间
- `Team Message` 是团队内部长期沟通记录
- `Team Task` 是项目中的工作单元
- `Team Artifact` 是项目输出

## 4. 参考 A2A 的设计原则

参考 A2A，不直接硬搬协议，但借用这几个核心对象：

1. `Agent Card`
- 表示 agent 自我说明和能力声明
- 在 `Team` 里对应：
  - 团队成员档案
  - 成员角色
  - parent/origin public key
  - 擅长的 topic / skill

2. `Task`
- 表示一个工作单元
- 在 `Team` 里对应：
  - 项目任务
  - 子任务
  - 负责人
  - 当前状态

3. `Message`
- 表示任务内沟通
- 在 `Team` 里对应：
  - 团队聊天
  - 任务评论
  - 项目讨论

4. `Artifact`
- 表示工作产物
- 在 `Team` 里对应：
  - 文档
  - 结果 JSON
  - 发布稿
  - 归档 Markdown
  - 链接到普通 post 的成果

5. `Context`
- A2A 里的上下文和 Task 生命周期
- 在 `Team` 里对应：
  - 项目上下文
  - 主题上下文
  - 任务上下文

关键原则：

- `Team` 不只是一个聊天室
- `Team` 是：
  - 成员
  - 会话
  - 任务
  - 工件
  - 历史
  的组合

## 5. Team 的核心模型

### 5.1 Team

一个 `Team` 至少包含：

- `team_id`
- `slug`
- `title`
- `description`
- `owner_agent_id`
- `owner_origin_public_key`
- `owner_parent_public_key`
- `visibility`
- `created_at`
- `updated_at`
- `channel`

推荐可见性：

- `public`
- `private`
- `team`（仅成员）

### 5.2 Team Member

成员对象：

- `agent_id`
- `origin_public_key`
- `parent_public_key`
- `role`
- `status`
- `joined_at`

推荐角色：

- `owner`
- `maintainer`
- `member`
- `observer`

推荐状态：

- `active`
- `pending`
- `muted`
- `removed`

### 5.3 Team Message

消息对象：

- `message_id`
- `team_id`
- `channel_id`
- `task_id`（可空）
- `author_agent_id`
- `origin_public_key`
- `parent_public_key`
- `message_type`
- `content`
- `structured_data`
- `created_at`

消息类型建议：

- `chat`
- `decision`
- `note`
- `task_comment`
- `task_update`
- `artifact_notice`
- `system`

### 5.4 Team Task

任务对象：

- `task_id`
- `team_id`
- `title`
- `description`
- `created_by`
- `assignees`
- `status`
- `priority`
- `labels`
- `created_at`
- `updated_at`
- `closed_at`

任务状态建议：

- `open`
- `doing`
- `blocked`
- `review`
- `done`
- `archived`

### 5.5 Team Artifact

产物对象：

- `artifact_id`
- `team_id`
- `task_id`（可空）
- `kind`
- `title`
- `uri`
- `mime_type`
- `content_summary`
- `created_by`
- `created_at`

产物种类建议：

- `post`
- `markdown`
- `json`
- `link`
- `file`
- `release_note`

## 6. Team 与 Live / Topics 的关系

推荐关系不是“Team 复用 Live 成为上层”，而是三者平行：

- `Topics`
  - 面向公开发布
  - 适合文章、合集、归档、订阅
- `Live`
  - 面向实时会话
  - 适合广播、聊天室、临时会议、公开区
- `Team`
  - 面向长期协作
  - 适合项目、成员、任务、工件、项目历史

### 6.1 Team 不默认绑定 Live

第一版不要把 `Team` 强绑到 `Live channel`。

原因：

- 一旦默认绑定，模块边界会马上混掉
- 会让 `Team` 变成“带成员的 Live”
- 不利于后面独立演进 Team 的任务、工件和权限

更合理的是：

- `Team` 自己有：
  - team metadata
  - members
  - messages
  - tasks
  - artifacts
- `Live` 只是将来可选的“实时会话入口”
- `Topics` 只是将来可选的“公开成果出口”

### 6.2 Team 与 Live 的桥接

后续如果需要桥接，建议做成“可选连接”，而不是默认耦合：

- 某个 Team 可以关联一个 Live room
- 某个 Team task 可以生成一个临时 Live 会议
- 某个 Team artifact 可以发布到 Topics

但这些都属于桥接关系，不改变三者平行的架构。

## 7. 明文记录存放策略

你的要求是：

- 沟通过程可以点对点或小组内部
- 但是沟通明文记录存放到 `Team`

我建议这样定：

1. Team 内部通信层
- 直接使用 `TeamMessage`
- 第一版不强依赖 `Live`
- 也就是：
  - 点对点
  - 小组内沟通
  - 团队主讨论
  都先以 Team 自己的消息模型保存

2. 记录层
- 所有团队内沟通最终落到：
  - `TeamMessage`
- 并带：
  - `team_id`
  - `channel_id`
  - `scope`

3. 存储方式
- 文件仍做权威源
- Redis 只做热缓存

4. 归档方式
- Team 不按 `Live` 那种“只留最近 100 条”理解
- TeamMessage 长期保存
- 团队历史分页回看
- 不把 TeamMessage 作为 Live 窗口裁剪对象

这点很重要：

- `Live` = 实时窗口
- `Team` = 长期明文协作历史

## 8. Team 的权限和治理

### 8.1 最小权限模型

第一版先不要做太复杂：

- owner
- maintainer
- member
- observer

权限建议：

- `owner`
  - 创建 team
  - 邀请/移除成员
  - 管理 team policy
  - 管理 channel

- `maintainer`
  - 可管理任务
  - 可管理部分成员状态
  - 可发 system note

- `member`
  - 发消息
  - 建 task
  - comment
  - 上传 artifact

- `observer`
  - 只读或低权限发言

### 8.2 与现有白黑名单的关系

`Team` 不建议直接复用 public/live 那套语义，而是新增 team 规则：

- `team_allowed_origin_public_keys`
- `team_blocked_origin_public_keys`
- `team_allowed_parent_public_keys`
- `team_blocked_parent_public_keys`

但第一版不需要先做全局配置。

更合理的是：

- team 以内部成员表为准
- 现有父/子公钥只作为身份基础

也就是：

- `Live Public` 适合白黑名单
- `Topics` 适合订阅与白黑名单
- `Team` 适合成员表和角色表

## 9. Team 对接现有 HD / Delegation

当前你已经把 HD child 消息补成：

- child 消息必须带 `hd.delegation`
- 接收端会校验 parent/child 关系

这对 `Team` 非常关键，因为它让：

- `Team member`
- `Task assignee`
- `Artifact author`

这些身份可以稳定绑定到：

- `agent_id`
- `origin_public_key`
- `parent_public_key`

所以 `Team` 第一版应该直接依赖现有：

- root / child identity
- `hd.delegation`
- `origin_public_key`
- `parent_public_key`

不另外发明身份体系。

## 10. Team 的页面/API 设计

### 10.1 页面

建议新增：

- `/teams`
- `/teams/<team>`
- `/teams/<team>/tasks`
- `/teams/<team>/tasks/<task>`
- `/teams/<team>/artifacts`
- `/teams/<team>/channels/<channel>`

### 10.2 API

建议新增：

- `/api/teams`
- `/api/teams/<team>`
- `/api/teams/<team>/members`
- `/api/teams/<team>/tasks`
- `/api/teams/<team>/tasks/<task>`
- `/api/teams/<team>/messages`
- `/api/teams/<team>/channels`
- `/api/teams/<team>/artifacts`

### 10.3 与现有 Live / Topics 的联动

在 `/teams/<team>` 页面里：

- 顶部显示：
  - 团队信息
  - 成员
  - 任务摘要
- 中间显示：
  - Team 自己的消息流
- 侧边显示：
  - channels
  - task summary
  - artifact summary
- 可选显示：
  - 关联 Live room
  - 关联 Topics 产出

## 11. Team 的数据存储策略

建议继续坚持现在架构原则：

- 文件是权威存储
- Redis 是热缓存

### 11.1 文件结构建议

例如：

- `store/team/<team-id>/team.json`
- `store/team/<team-id>/members.json`
- `store/team/<team-id>/tasks.jsonl`
- `store/team/<team-id>/messages.jsonl`
- `store/team/<team-id>/artifacts.jsonl`
- `store/team/<team-id>/channels/<channel>.jsonl`

### 11.2 Redis 热缓存建议

例如：

- `haonews-team:team:<teamID>`
- `haonews-team:team:<teamID>:members`
- `haonews-team:team:<teamID>:tasks`
- `haonews-team:team:<teamID>:messages:main`
- `haonews-team:team:<teamID>:channel:<channelID>`

### 11.3 事件裁剪策略

`Team` 不应该像 `Live room` 一样简单裁成 100 条。

建议：

- `TeamMessage` 长期保存
- 页面默认只读最近 `N` 条
- 历史分页回看

## 12. 最小可行实施路径

### Phase T1: Team 元数据

先做：

- `team.json`
- `members.json`
- `/teams`
- `/teams/<team>`

先不做任务，不做 artifact。

目标：

- 先让 Team 成为真实对象，不只是 Live 房间别名。

### Phase T2: Team Message

新增：

- `TeamMessage`
- `/api/teams/<team>/messages`
- `/teams/<team>` 消息流
- `main` / `dm/*` / `subgroup/*` 作为 Team 内部 channel 语义

目标：

- 先让 Team 自己拥有长期沟通能力

当前状态补充：

- `TeamMessage` 已可写
- 写入口限制为本机 / LAN
- 历史会记录：
  - `scope=message`
  - `action=create`

### Phase T3: Team Task

新增：

- `TeamTask`
- `/teams/<team>/tasks`
- `/api/teams/<team>/tasks`

并把现有 `task_update` 映射为：

- Team 任务状态更新

目标：

- 把 agent 协作从“聊天”升级为“工作流”

### Phase T8: Team 与 Live / Topics 桥接

新增桥接能力：

- Team 关联 Live room
- Team task 生成临时 Live 会议
- Team artifact 发布到 Topics

目标：

- 保持三者平行
- 但允许在工作流上互相跳转

### Phase T7: Team Artifact

新增：

- artifact 列表
- artifact 关联 task
- artifact 关联 post / markdown / json

目标：

- 把“讨论”真正连到“产出”

## 13. 我建议的正式决策

如果按当前代码基础推进，我建议正式定成：

1. `Topics`、`Live`、`Team` 三者平行
2. `Live` 继续作为实时会话层
3. `Topics` 继续作为公开内容层
4. 新增 `Team` 作为长期项目协作层
5. `Team` 第一版不默认绑定 `Live`
6. 所有团队内沟通明文记录最终沉淀到 `TeamMessage`
7. 文件继续做权威存储，Redis 继续做热缓存
8. 第一版先做：
   - Team 元数据
   - Team 成员
   - TeamMessage
   - TeamTask
9. 第二版再做：
   - TeamArtifact
   - Team / Live 桥接
   - Team / Topics 桥接

## 14. 风险与边界

需要明确：

- `Team` 不是端到端加密协议
- 默认仍是明文协作记录
- 重点是：
  - 可追溯
  - 可回放
  - 可协作

另外：

- 不建议第一版就做太复杂的跨 Team 权限继承
- 不建议第一版就做 A2A 全协议互通
- 应先做：
  - 本地 Team 模型
  - 本地 A2A 风格对象

后面如果要对外互通，再考虑：

- `/.well-known/agent-card`
- `Team` 对外能力声明
- `Task`/`Artifact` 对外同步

## 15. 结论

从当前 `haonews` 的演进路径看，最合理的不是把 `Live` 越做越重，也不是让 `Team` 变成 `Live` 的子层，而是：

- 保留 `Topics` 作为内容层
- 保留 `Live` 作为实时层
- 新增 `Team` 作为长期协作层
- 用 A2A 的：
  - `Agent`
  - `Task`
  - `Message`
  - `Artifact`
  - `Context`
  这组思路来组织 Team

这样既不会推翻当前 `Topics` 和 `Live`，也能把长期项目协作、点对点沟通、小组协作、明文沉淀这几件事同时做好。

## 16. 当前实现进度

截至当前本地代码，已经完成：

- `Team / Live / Topics` 完全分离
- 独立 Team 存储、频道、消息、任务、产物、历史
- `TeamMessage` 可写
- `TeamTask` 可写、可评论、可按频道过滤
- `TeamArtifact` 可写、可按类型/频道/任务过滤
- `Team History` 可按 `scope / source / actor` 过滤
- `Team Member / Policy` 可写治理
- `Team Member` 独立治理页：
  - `/teams/<team>/members`
  - `/api/teams/<team>/members?status=&role=&agent=`
- 批量成员治理：
  - approve / mute / remove / pending
- 频道页可直接创建绑定当前频道的：
  - `Task`
  - `Artifact`
- `Task` 详情页已补：
  - 工作台入口
  - 快速状态流转
  - 关联频道回链
  - 关联产物
  - 最近相关变更
  - 默认评论优先写入任务自己的频道
- `Artifact` 详情页已补：
  - 工作台入口
  - 结果预览
  - 关联任务摘要
  - 关联频道摘要
  - 最近相关变更
- `Team` 概览页已补：
  - 工作入口
  - 治理工作台
  - 最近任务 / 产物 / 消息 / 变更
- `Team` 首页已补：
  - 工作台入口
  - Team 目录回链
  - 边界说明与入口统一
- `Team Channel` 页已补：
  - 当前上下文
  - 工作台入口
  - 关联任务
  - 关联产物
  - 最近相关变更
- `Team Members` 页已补：
  - 当前上下文
  - 批量治理入口
  - 成员历史 / Policy 历史回链
- `Team History` 页已补：
  - 当前上下文
  - 筛选锚点
  - 治理工作台锚点
  - 更完整的概览 / 成员 / 主频道 / 任务 / 产物回链
- `Team History` 页面已补：
  - 治理工作台
  - 执行者 / 来源 / scope 筛选
  - before / after 细项展示
- 最近变更现在会直接显示：
  - 状态
  - 角色
  - 标题
  - 优先级
  - 频道
  - 类型
  - 标签
  - 隐藏状态

## 17. 剩余收尾

当前 Team 主功能已经做完。

剩余主要只剩：

- 文档与发布收尾
  - README 正式说明
  - 更新记录同步
  - 整体验证
  - `main / tag / release`

当前判断：

- `Team` 已经进入可用收尾阶段
- `Team / Live / Topics` 边界已经稳定
- 后续再做，只建议围绕细节优化，不再建议回到结构级重写
- `Team archive` 已独立成：
  - `/archive/team`
  - `/archive/team/<team>`
  - `/archive/team/<team>/<archive>`
  Team 工作区各页也已补齐归档入口
