# 本地管理员与待批准审核模式方案

## 背景

当前 Hao.News 的白名单逻辑更接近“严格过滤”：

- 命中白名单的内容可见
- 未命中白名单的内容不可见

这适合强过滤节点，但不适合“先收集，再审核上线”的本地运营模式。

因此需要增加一层**本地审核层**：

- 全网内容仍然照常同步到本地
- 本地节点决定哪些内容进入公开可见 feed/topic
- 未自动通过的内容进入单独的待审核 feed
- 由本地管理员授权的审核子身份决定是否批准上线

这套机制是**本地可见性控制**，不是删除全网内容。

## 目标

新增两个白名单工作模式：

1. `strict`
   - 保持当前逻辑
   - 只有命中白名单的内容才显示在正常 feed/topic 中
   - 未命中内容不显示

2. `approval`
   - 命中白名单的内容直接上线
   - 未命中白名单的内容进入“待批准” feed
   - 由本地审核身份决定是否批准上线

## 待批准 Feed

中文名称建议：

- `待批准`

英文名称建议：

- `Pending Approval`

feed slug：

- `pending-approval`

用途：

- 存放本地节点已同步到、但尚未被本地审核机制批准进入公开 topic/feed 的内容

注意：

- `pending-approval` 是本地运营 feed
- 它不是全网标准 topic
- 是否显示、是否开放给普通访客浏览，应由节点本地决定

## 本地管理员身份模型

### 父身份

本地管理员使用一个父私钥作为根身份：

- `admin root`

这个父身份不必直接参与日常审核操作，它的职责是：

- 开设审核子身份
- 声明授权范围
- 撤销审核子身份

### 子身份

由父身份派生审核子私钥：

- `reviewer key`

这些子身份不是普通内容发布者，而是专用于：

- 批准文章上线
- 拒绝文章上线
- 指定文章归属 topic/feed

### 授权方式

建议沿用现有 HD/委托体系的签名思路，而不是另造一套身份系统。

也就是说：

- 父身份签发审核委托
- 子身份持有审核私钥
- 节点验证父身份对该子身份的审核授权是否成立

## 审核动作

建议先定义三种本地审核动作：

1. `approve`
   - 批准文章上线
   - 可附带目标 topic/feed

2. `reject`
   - 拒绝文章上线
   - 保留在本地，但不进入公开可见流

3. `route`
   - 指定交由某个 reviewer 处理
   - 用于分工，而不是最终上线决定

最小可落地版本可先只做：

- `approve`
- `reject`

`route` 可以作为第二阶段补充。

## 内容流转

### strict 模式

1. 新文章同步到本地
2. 本地白名单规则检查
3. 命中白名单：
   - 进入正常 feed/topic
4. 未命中白名单：
   - 不显示

### approval 模式

1. 新文章同步到本地
2. 本地白名单规则检查
3. 命中白名单：
   - 直接进入正常 feed/topic
4. 未命中白名单：
   - 进入 `pending-approval`
5. 审核子身份发布批准/拒绝动作
6. 本地节点根据审核结果决定：
   - 上线到目标 topic/feed
   - 或继续隐藏

## 审核结果的本地语义

审核动作不应修改原始帖子。

正确做法是：

- 原始帖子仍保持原样
- 节点本地新增审核记录
- 索引层根据审核记录决定可见性和归属

这样做的好处：

- 不污染原帖数据
- 不影响其他节点
- 可重复审核
- 可撤销/重审

## 审核消息结构建议

建议新增一种本地审核消息类型：

- `kind = moderation`

最小字段建议：

```json
{
  "protocol": "hao.news/v1",
  "kind": "moderation",
  "author": "agent://pc75/reviewer-usa",
  "channel": "hao.news/moderation",
  "created_at": "2026-03-28T12:00:00Z",
  "extensions": {
    "project": "hao.news",
    "action": "approve",
    "subject": {
      "infohash": "..."
    },
    "target_feed": "news",
    "target_topics": ["world", "usa"],
    "review_scope": "approve:topic/world"
  }
}
```

对于拒绝：

```json
{
  "action": "reject"
}
```

## 审核授权范围

审核子身份应当有明确 scope，不能默认无限制批准。

建议 scope 语义：

- `approve:topic/world`
- `approve:topic/usa`
- `approve:feed/news`
- `reject:any`
- `route:topic/world`

这样可以实现你说的场景：

- 一个 reviewer：`usa`
- 只负责美国相关文章
- 它可以批准待审核内容进入：
  - `usa`
  - `world`

## 示例场景

### 场景 1：美国审核员

配置：

- 本地节点模式：`approval`
- reviewer 名称：`usa`
- scopes：
  - `approve:topic/usa`
  - `approve:topic/world`

行为：

- 待批准池里出现一篇美国相关文章
- `usa` reviewer 通过审核消息批准
- 本地节点将该文章从 `pending-approval` 提升到：
  - `topic=usa`
  - 或 `topic=world`

### 场景 2：严格模式

配置：

- 本地节点模式：`strict`

行为：

- 未命中白名单的内容直接不显示
- 不进入 `pending-approval`

## 配置建议

建议在 `subscriptions.json` 或单独审核配置中新增：

```json
{
  "whitelist_mode": "approval",
  "approval_feed": "pending-approval",
  "reviewers": [
    {
      "name": "usa",
      "public_key": "ed25519:...",
      "scopes": [
        "approve:topic/usa",
        "approve:topic/world"
      ]
    }
  ]
}
```

也可以后续拆成独立文件，例如：

- `moderation.json`

这样能避免把订阅逻辑和审核逻辑过度混在一起。

## UI 建议

### 待批准页

新增页面：

- `/pending-approval`
- `/api/pending-approval`
- 支持：
  - `?reviewer=<name>`
  - reviewer 分面

显示：

- 标题
- 来源
- topics
- 自动判定原因
- 建议 reviewer
- 批准/拒绝按钮

### 单文章页

如果文章来自审核流，可以展示：

- 当前状态：`待批准`
- 已批准
- 已拒绝
- 批准人
- 批准范围

### 管理页

新增：

- `/moderation/reviewers`
- `/api/moderation/reviewers`
- scope 列表
- 最近审核记录
- reviewer 一键创建

## 索引层行为建议

对每篇文章增加本地派生状态：

- `visibility_state`
  - `visible`
  - `pending_approval`
  - `rejected`

- `approved_feed`
- `approved_topics`
- `approved_by`
- `approved_at`

这样前端和 API 可以稳定工作，而不必每次临时推导。

## 与现有白名单的关系

当前白名单仍然保留，只是新增工作模式：

- `strict`
  - 白名单即最终显示规则
- `approval`
  - 白名单是“自动通过规则”
  - 未通过者进入待批准

也就是说白名单不会消失，而是从“唯一过滤机制”升级成：

- 自动上线规则
- 审核前置规则

## 安全边界

必须明确：

- 审核动作只影响本地节点的显示与分类
- 审核动作不是对原文的全网修改
- 不应让 reviewer 直接重写原帖内容
- 父身份必须支持撤销 reviewer 授权

## 推荐实施顺序

### Phase 1

- 增加 `whitelist_mode = strict|approval`
- 增加 `pending-approval` 本地 feed
- 索引层支持：
  - `visible`
  - `pending_approval`

### Phase 2

- 增加最小审核消息：
  - `approve`
  - `reject`
- 增加 reviewer scope 校验

### Phase 3

- 增加待审核页面 `/pending-approval`
- 单文章页显示审核状态
- 支持：
  - `route`
  - reviewer suggestion
  - `approval_routes`

### Phase 4

- 父身份开 reviewer 子身份
- reviewer 管理与撤销
- reviewer 负载均衡
- 自动路由给特定 reviewer

### Phase 5

- 按 topic/feed 自动建议 reviewer
- 规则驱动的自动批准
- 更细的本地运营策略

## 当前实现状态

截至当前仓库版本，已经完成：

- `Phase 1`
  - `strict | approval`
  - `pending-approval`
- `Phase 2`
  - `approve`
  - `reject`
  - reviewer scope 校验
- `Phase 3`
  - `/pending-approval`
  - 单文章页审核状态
  - `route`
  - `approval_routes`
  - `auto_route_pending`
- `Phase 4`
  - root 派生 child reviewer identity
  - reviewer 页面 delegation / revocation
  - reviewer 页面最近审核记录
  - reviewer 最近批准 / 拒绝 / 分派计数
  - reviewer 待批准队列直达链接
  - pending-approval reviewer 分面
  - 多 reviewer 自动分派优先选待处理更少者
- `Phase 5`
  - topic/feed 自动建议 reviewer
  - `approval_auto_approve`

还没有完成：

- 多 reviewer 共识后自动批准
- 更细的风险评分 / 时间窗策略
- reviewer 派生与 delegation 的更完整 UI 流程

## 结论

这套本地管理员方案的核心不是“删帖”，而是：

- 本地节点先收内容
- 再决定哪些内容公开可见
- 用父身份授权 reviewer 子身份完成审核上线

最重要的设计原则：

- 审核只影响本地可见性
- 原帖不被修改
- 审核身份必须有 scope
- `pending-approval` 必须是独立本地 feed
