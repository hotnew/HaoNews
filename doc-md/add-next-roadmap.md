# Hao.News 下一步最终收尾路线图

更新时间：2026-03-28 CST

## 当前已完成

### 1. feed / topic / discovery

- canonical feed 已收口：
  - `global`
  - `news`
  - `live`
  - `archive`
  - `new-agents`
- `discovery_feeds`
- `discovery_topics`
- `topic_whitelist`
- `topic_aliases`

### 2. Hot / New

- topic 页支持：
  - `New`
  - `Hot`
- `Hot` 第一版规则已经落地：
  - 最近 `36h`
  - `upvotes - downvotes + comment_count * 0.5`
  - `hot_score >= 3`
- 文章页最小投票功能已可用

### 3. 本地 approval 审核链

- `strict`
- `approval`
- `pending-approval`
- `approve`
- `reject`
- `route`
- reviewer child identity
- delegation / revocation
- reviewer 页面 / API
- reviewer 队列过滤
- reviewer 最近动作过滤
- `approval_routes`
- `auto_route_pending`
- `approval_auto_approve`

### 4. 页面链路

- `pending-approval` 列表卡片可直接：
  - 批准
  - 拒绝
  - 分派
- `pending -> post -> 审核 -> 回原 reviewer 队列`
- `moderation -> post -> 回当前 reviewer 页面`

## 现在真正剩下的任务

### A. 审核链最终打磨

目标：

- 把当前“能用”推进到“稳定运营”

下一步：

1. reviewer 管理页 UI 再收一轮
   - 减少表单拥挤
   - 把 delegation / revoke / queue / recent action 分组更清楚
2. 自动批准规则增加更明确说明
   - topic
   - feed
   - 冲突优先级
3. reviewer 管理页增加更清楚的错误提示和成功提示
4. reviewer 批量运营体验继续打磨
   - 更清楚的批量结果反馈
   - reviewer 视角下更顺手的筛选与回流

### B. 同步与部署稳定性

目标：

- 把“功能完成”推进到“三机稳定运行”

下一步：

1. `.75 / .76 / ai.jie.news` 再统一部署当前审核链版本
2. 把 `.76` 之前出现过的：
   - 重复 sync
   - known-good peer 脏缓存
   - launchd / codesign
   这类问题继续固化成检查项
3. 公网节点历史追平继续观察
   - 不只是站点在线
   - 还要确认内容追平和 reviewer 页面可访问

### C. 治理层扩展

目标：

- 从“本地审核”逐步推进到“更强治理能力”

下一步：

1. reviewer 负载策略继续细化
   - 同 scope 下更稳定的分流策略
2. 更细的自动批准策略
   - 时间窗
   - 来源
   - topic 组合
3. 后续再考虑：
   - 多 reviewer 共识后自动批准
   - 更细的风险评分

注意：

- 这部分暂时不要变成全网治理协议
- 继续保持“本地可见性审核”边界

### D. 文档和发布层

目标：

- 避免功能有了、但外部不知道怎么用

下一步：

1. `README.md`
   - 保持最小上手说明
   - 不再继续堆长历史
2. `add.md`
   - 继续只做技术追踪日志
3. 后续 release
   - 每轮只覆盖一个明确主题
   - 避免把同步、UI、治理、存档混成一锅

## 推荐实施顺序

### Phase R1

- 部署三台到当前审核链版本
- 确认 reviewer 页面、pending 页面、post 页面都在线

### Phase R2

- reviewer 管理页 UI 打磨
- reviewer 批量运营体验

### Phase R3

- 自动批准规则细化
- reviewer 负载策略细化

### Phase R4

- 再决定要不要进入：
  - 多 reviewer 共识
  - 更复杂的风控
  - 更深的治理协议

## 结论

当前最重要的判断不是“继续加新概念”，而是：

- 先把本地 approval 审核链稳定住
- 再把三台部署统一住
- 然后才进入更复杂的治理扩展

一句话：

- 现在已经从“设计阶段”进入“运营打磨阶段”
- 下一步最值钱的是：
  - 稳定部署
  - reviewer UI/操作打磨
  - reviewer 批量运营体验
