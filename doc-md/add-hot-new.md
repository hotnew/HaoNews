# Hot / New 方案

时间：2026-03-28 CST

## 目标

在每个 `topic` 下增加两种内容分类：

- `New`
- `Hot`

默认显示 `New`。

适用范围：

- `topics` 页面
- topic 详情页
- 后续也可扩展到首页或 feed 页

---

## 最小定义

### New

`New` 的规则最简单：

- 所有新文章先进入 `New`
- 按发布时间倒序排序

也就是：

- 越新越靠前
- 不看投票
- 不看评论

### Hot

`Hot` 只看最近 `36` 小时内发布的文章。

排序依据：

- 投票
- 评论

第一版建议定义为：

```text
hot_score = upvotes - downvotes + comments * 0.5
```

也就是：

- 赞成票增加热度
- 反对票降低热度
- 评论也增加热度，但权重低于投票

---

## 建议约束

### 1. Hot 只看“最近 36 小时发布”的文章

先明确为：

- 文章发布时间在最近 `36` 小时内
- 才进入 `Hot` 候选集合

不采用：

- 老文章因为最近被评论或被投票，又重新冲上 `Hot`

原因：

- 实现更简单
- 结果更稳定
- 更符合“当前热点”语义

### 2. Hot 使用净投票，不用总投票

不要只看：

```text
upvotes + downvotes
```

建议看：

```text
upvotes - downvotes
```

原因：

- 纯总票数会让高争议内容异常靠前
- 净票更接近“认可度”

### 3. 评论权重低于投票

评论不能和投票 `1:1`。

建议第一版：

```text
hot_score = upvotes - downvotes + comments * 0.5
```

如果后面觉得评论权重还是太高，可以改成：

```text
hot_score = upvotes - downvotes + comments * 0.25
```

### 4. 需要最小门槛

不然只有 `1` 票或 `1` 条评论的内容也会进入 `Hot` 顶部。

建议第一版增加门槛：

- `hot_score >= 3`

或者：

- `upvotes + downvotes + comment_count >= 3`

推荐先用：

```text
hot_score >= 3
```

### 5. Hot / New 是 topic 内排序，不是全站混排

例如：

- `/topics/futures?tab=new`
- `/topics/futures?tab=hot`

每个 topic 自己计算：

- `futures` 的 `Hot`
- `news` 的 `Hot`
- `world` 的 `Hot`

而不是全站统一一个大 `Hot` 池。

### 6. 第一版不做 Top

先只做：

- `New`
- `Hot`

暂不做：

- `Top 24h`
- `Top 7d`
- `Top 30d`
- `Top all`

原因：

- 避免排序体系过早复杂化
- 先把 `Hot / New` 跑稳

---

## 数据层建议

为了让前端和 topic 页稳定，建议在索引层直接补这些字段：

- `upvotes`
- `downvotes`
- `comment_count`
- `hot_score`
- `is_hot_candidate`

其中：

- `upvotes`
  - 文章收到的赞成票数
- `downvotes`
  - 文章收到的反对票数
- `comment_count`
  - 回复数
- `hot_score`
  - 热度分
- `is_hot_candidate`
  - 是否满足最近 `36` 小时窗口

这样前端不需要实时计算复杂逻辑。

---

## 投票功能建议

先做最小投票模型：

- `upvote`
- `downvote`

不做：

- 表情反应并入热度
- 多维信誉投票
- 复杂真实性评分并入 Hot

第一版只需要：

- 对文章投赞成
- 对文章投反对
- 列表和详情展示：
  - `upvotes`
  - `downvotes`
  - `score`

建议存储口径：

```text
score = upvotes - downvotes
```

---

## 页面建议

### Topic 页

每个 topic 页顶部增加 tab：

- `New`
- `Hot`

默认：

- `New`

URL 形式建议：

- `/topics/futures`
  - 默认视为 `new`
- `/topics/futures?tab=hot`

### 列表卡片

当前卡片已经有：

- 投票区
- 评论区

后面只需要补充：

- `↑`
- `↓`
- 分数

不需要再额外塞复杂元信息。

---

## 第一版实现顺序

### Phase 1

先补索引字段：

- `upvotes`
- `downvotes`
- `comment_count`
- `hot_score`

### Phase 2

Topic 页支持：

- `tab=new`
- `tab=hot`

### Phase 3

补最小投票接口：

- `upvote`
- `downvote`

### Phase 4

前端按钮和展示：

- 文章卡片
- 单文章页

---

## 推荐最终口径

第一版建议最终确定为：

### New

```text
按发布时间倒序
```

### Hot

```text
候选范围：最近 36 小时内发布
排序分数：upvotes - downvotes + comments * 0.5
最小门槛：hot_score >= 3
```

---

## 一句话结论

先做：

- `New = 时间倒序`
- `Hot = 最近 36 小时内，按净投票 + 评论权重排序`

不要第一版就把热度算法做复杂。
