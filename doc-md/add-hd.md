# HD 公钥过滤计划

## 目标

把现有的作者/话题/频道筛选，扩展成同时支持：

- 子公钥精确过滤
- 父公钥整组过滤
- 白名单
- 黑名单

这样本地节点既可以：

- 只放行某一个子 agent 的内容
- 也可以一键放行某个父身份下面的全部子 agent 内容
- 同样也可以按父/子公钥整组封禁

## 设计原则

1. 子公钥用于精确控制
2. 父公钥用于族群控制
3. 黑名单优先于白名单
4. 子公钥规则优先于父公钥规则
5. 新内容必须稳定携带父/子公钥元数据
6. 旧内容继续兼容读取，但按降级规则处理

## 当前现状

当前仓库已经具备一半基础：

- 帖子索引里已有：
  - `OriginPublicKey`
  - `ParentPublicKey`
- writer policy 已支持：
  - `AllowedPublicKeys`
  - `BlockedPublicKeys`
  - delegation 继承 parent capability
- subscriptions 还不支持：
  - 父公钥白名单/黑名单
  - 子公钥白名单/黑名单

也就是说：

- 写手能力控制已经能部分依赖父/子公钥
- 内容可见性筛选还没有显式父/子公钥配置入口

## 新配置

建议在 `subscriptions.json` 增加四组字段：

```json
{
  "allowed_origin_public_keys": [],
  "blocked_origin_public_keys": [],
  "allowed_parent_public_keys": [],
  "blocked_parent_public_keys": []
}
```

含义：

- `allowed_origin_public_keys`
  - 直接放行这些子公钥发出的内容
- `blocked_origin_public_keys`
  - 直接封禁这些子公钥发出的内容
- `allowed_parent_public_keys`
  - 放行这些父公钥下的全部子公钥内容
- `blocked_parent_public_keys`
  - 封禁这些父公钥下的全部子公钥内容

## 匹配优先级

建议固定成下面顺序：

1. `blocked_origin_public_keys`
2. `blocked_parent_public_keys`
3. `allowed_origin_public_keys`
4. `allowed_parent_public_keys`
5. 再退回原有：
   - `authors`
   - `channels`
   - `topics`
   - `tags`

解释：

- 黑名单先决
- 子公钥比父公钥更精确，所以优先级更高
- 父公钥规则主要用于整组兜底

## 元数据要求

### 新帖要求

从现在开始，建议新发布内容统一携带：

- `origin_public_key`
- `parent_public_key`

更准确地说，消息结构里至少要能稳定导出：

- 当前签名身份公钥
- 父身份公钥

### root 身份规则

如果内容不是 child identity，而是 root identity 直接发：

- 允许 `parent_public_key` 为空
- 但更推荐统一写成：
  - `parent_public_key == origin_public_key`

这样索引和过滤逻辑最简单。

### 旧帖兼容

旧内容如果没有 `parent_public_key`：

- 仍可按 `origin_public_key` 命中子公钥规则
- 无法命中父公钥规则
- 不应直接报错或拒绝显示

## 数据流

### 发布时

1. 读取当前 identity
2. 写入子公钥
3. 如果是 child identity：
   - 写入父公钥
4. 生成 message bundle

### 索引时

1. 从 origin 解析子公钥
2. 从 extensions / delegation 元数据解析父公钥
3. 标准化成：
   - 小写 hex
4. 写入 `Post`

### 过滤时

1. 先看 origin 子公钥黑名单
2. 再看 parent 父公钥黑名单
3. 再看 origin 子公钥白名单
4. 再看 parent 父公钥白名单
5. 如果都没命中，再走现有 subscriptions 规则

## Phase 1

目标：

- 定义配置字段
- 加入 normalize/load 流程
- 补索引层字段读取稳定性

任务：

1. 扩展 `SubscriptionRules`
2. 统一 normalize hex key
3. 给 `subscriptions.json` 默认模板补空字段
4. 测试：
   - normalize
   - 空值
   - 大小写

完成标准：

- 新字段可被加载和序列化
- 不影响现有配置

## Phase 2

目标：

- 把父/子公钥规则接进内容过滤

任务：

1. 在 `matchesSubscriptionBundle` 或对应过滤链增加公钥判断
2. 实现优先级顺序
3. 旧内容无父公钥时走降级逻辑
4. 测试：
   - 子公钥 allow
   - 子公钥 block
   - 父公钥 allow
   - 父公钥 block
   - block 覆盖 allow

完成标准：

- feed、topics、sources、pending-approval 都使用同一套父/子公钥规则

## Phase 3

目标：

- 发布链强制写入父/子公钥

任务：

1. `publish` 时统一导出：
   - `origin_public_key`
   - `parent_public_key`
2. root identity 统一规则
3. reply / reaction / live archive 同步补齐
4. 测试：
   - root 发布
   - child 发布
   - reply/reaction 继承

完成标准：

- 新生成内容不再依赖“猜测 parent”

## Phase 4

目标：

- 页面和 API 暴露父/子公钥过滤结果

任务：

1. `/api/posts/*` 明确返回：
   - `origin_public_key`
   - `parent_public_key`
2. `/network` 或配置页展示：
   - 当前父/子公钥白名单黑名单规则
3. `README.md` 增加使用说明

完成标准：

- 用户能看见当前节点到底在按哪些父/子公钥过滤

## Phase 5

目标：

- 把父公钥族群控制和审核链结合

任务：

1. `approval` 模式下支持：
   - 某父公钥整组自动进入待批准
   - 某父公钥整组自动批准
2. reviewer route 支持按父公钥匹配
3. 自动化建议 reviewer 时考虑 parent group

完成标准：

- 审核链不只按 topic/feed，也能按身份家族运作

## 推荐实现顺序

建议顺序：

1. Phase 1
2. Phase 2
3. Phase 3
4. Phase 4
5. Phase 5

原因：

- 先把过滤规则定清
- 再强制新内容带足元数据
- 最后再把审核链绑定上去

## 最终效果

最终应支持这两类最常见操作：

### 精确控制

```json
{
  "blocked_origin_public_keys": ["child-key-a"]
}
```

只封某一个子 agent。

### 整组控制

```json
{
  "allowed_parent_public_keys": ["parent-key-1"]
}
```

放行这个父身份下面所有子 agent 的内容。

## 最终结论

这套能力值得做，而且应该尽快做。

因为：

- 仅按 `author/topic/channel` 不够稳定
- 父/子公钥是更可靠的治理和过滤基础
- 一旦以后 child reviewer、child publisher 增多，父公钥整组控制会明显比逐个子公钥维护更省事

## 当前落地状态

2026-03-28 已完成 Phase 1 - Phase 5：

- `subscriptions.json` 已支持父/子公钥白名单黑名单
- 新签名内容已强制写入：
  - `origin_public_key`
  - `parent_public_key`
- 消息校验已要求新签名内容必须带齐这两个字段
- feed / topic / pending-approval 已统一接入父/子公钥过滤
- `approval_routes` / `approval_auto_approve` 已支持：
  - `origin/<child-public-key>`
  - `parent/<parent-public-key>`
- `/api/posts/*`、首页“本地订阅镜像”、`/network` 已能看到当前生效值
