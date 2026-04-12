# Topics 项目整合说明

## 1. Topics 是什么

`Topics` 是 `haonews` 里的公开内容发布、发现、订阅和归档主线。

它解决的是：

- 内容如何按 topic 被发现
- 内容如何进入本地订阅镜像
- 内容如何经过 review / pending / auto-approve
- 内容如何稳定输出到页面、API、RSS 和 archive

一句话：

**Topics = 公开内容发布与发现层 + 本地订阅镜像 + archive 主线。**

## 2. Topics 不是什么

`Topics` 不是：

- Team 的子层壳
- Live 的房间副本
- 只有首页列表的展示页
- 只有同步层、没有归档主语义的内容缓存

当前模块边界已经固定：

- `Topics`
- `Live`
- `Team`

三者平行，不互相吞并。

## 3. 当前已经完成了什么

### 3.1 公开内容主链

已经形成：

- 首页 / feed
- `/topics`
- `/topics/<topic>`
- `/topics/<topic>/rss`
- `/api/feed`
- `/api/topics`
- `/api/topics/<topic>`

### 3.2 订阅与治理链

已经支持：

- `subscriptions.json`
- topic whitelist / aliases
- reviewer 路由
- pending / approval
- auto-approve
- 本地订阅镜像

### 3.3 archive 主语义

`Topics` 当前主归档语义已经收口到：

- `/archive/topics`
- `/archive/topics/<day>`
- `/archive/topics/messages/<infohash>`
- `/archive/topics/raw/<infohash>`
- `/api/archive/topics/list`
- `/api/archive/topics/manifest`

也就是说，`archive/topics/*` 已经是正式主入口，不再长期保留多套“都像主入口”的语义。

### 3.4 性能与冷启动收口

已完成和固定下来的方向包括：

- 首页 / `/topics` / topic 页 / RSS / API 热路径压缩
- facet / summary / topic/source 统计复用
- query / result 短缓存
- coldstart 期间优先返回轻量页面壳和 `starting=true`

### 3.5 Hot / New 语义

当前 `Topics` 已经形成明确排序语义：

- `New`
  - 按时间倒序
- `Hot`
  - 基于时间窗口与热度规则

这让 topic 页不再只是“按时间排一遍”的单通道内容页。

## 4. 适合怎么用

### 4.1 公开内容发布与浏览

最适合：

- 公开 topic 内容
- 用 topic 聚合同类信息
- 通过页面 / RSS / API 对外消费

### 4.2 本地订阅镜像

如果你不是要“全网随便收”，而是要本地稳定消费一批内容：

- 用 `subscriptions.json`
- 用 whitelist / aliases / reviewer 路由
- 把内容收成自己的本地订阅镜像

### 4.3 归档入口

如果你想看长期稳定内容，不依赖瞬时热路径：

- 直接走 `archive/topics/*`

这条线更适合：

- 长期留存
- 外部引用
- 结构化回放

## 5. 为什么 Topics 不是空壳

因为它已经不是“只有列表页”的状态了，而是有完整主链：

- 发布
- 发现
- 订阅
- review / pending
- 首页与 topic 页
- RSS / API
- archive

也就是说，现在的 `Topics` 已经是：

**一个可公开发布、可本地治理、可稳定归档的内容系统。**

## 6. 真实抓手

当前最适合继续复用的 Topics 抓手包括：

- `/topics`
- `/topics/<topic>`
- `/topics/<topic>/rss`
- `/api/feed`
- `/api/topics`
- `/api/archive/topics/list`
- `/api/archive/topics/manifest`

如果要验证冷启动与性能，优先看：

- 首页
- `/topics`
- `/topics/<topic>`
- `/topics/<topic>/rss`

## 7. 最近解决和固定下来的关键点

### 7.1 `Topics / Live / Team` 模块完全分离

现结论：

- 三条线是平行模块
- 跨模块只能走桥接或引用
- 不能重新耦合成一个“大而混”的系统

### 7.2 `archive/topics/*` 成为正式主语义

现结论：

- `Topics` 归档主入口已经统一
- 页面和文档应优先输出 `archive/topics/*`

### 7.3 冷启动与热路径被单独治理

现结论：

- 不能只靠功能“能跑”
- 首页、topic 页、RSS、API 这些高频路径必须单独优化

### 7.4 `Hot / New` 不是随意前端切 tab

现结论：

- 它们是明确排序语义
- 应在索引和输出层有稳定口径

## 8. 当前边界和后续增强

当前已能作为稳定内容主线使用，但仍可继续增强：

- 更完整的 Topics 项目级样本文档
- 更细的 Hot / New / pending 治理说明
- 更稳定的 topic/source/reviewer summary 输出

这些属于增强项，不影响当前 Topics 主线已成立。

## 9. 关联文档

- 协议草案：
  - [protocol-v0.1.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/protocol-v0.1.md)
- Topics 归档与优化 runbook：
  - [20260403-haonews-task.auto-run.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/20260403-haonews-task.auto-run.md)
  - [20260404-team-codex.auto-run.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/20260404-team-codex.auto-run.md)
- Hot / New 方案：
  - [add-hot-new.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/add-hot-new.md)
- 性能与并发优化：
  - [add-pro.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/add-pro.md)
