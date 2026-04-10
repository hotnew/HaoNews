# `20260403-haonews-task.auto-run.md`

## 1. 当前基线

- 当前代码基线提交：
  - `c5f8d4e`
- 当前线上验证节点：
  - `.75`
- 当前已完成的大块：
  - `Live / Topics / Team` 三模块继续完全分离
  - `Live / Topics / Team` 三条归档线已拆分
  - `Team` 独立工作区已经可用
  - `Redis` 热链已经接入并在线
  - 内容主链冷路径已经做过一轮压缩

## 2. 已完成事项摘要

### 2.1 Archive

- `Topics` 主归档入口：
  - `/archive/topics`
  - `/archive/topics/<day>`
  - `/archive/topics/messages/<infohash>`
  - `/archive/topics/raw/<infohash>`
- `Topics` 主 API：
  - `/api/archive/topics/list`
  - `/api/archive/topics/manifest`
- 旧 `api/history/*` 仍兼容

- `Live` 主归档入口：
  - `/archive/live`
  - `/archive/live/<room>`
  - `/archive/live/<room>/<archive>`
- `Live` 已有：
  - 手动归档
  - `05:30 CST` 日归档
  - 旧 `/live/history/*` 兼容层

- `Team` 主归档入口：
  - `/archive/team`
  - `/archive/team/<team>`
  - `/archive/team/<team>/<archive>`
- `Team` 已有：
  - 手动快照归档
  - 工作区归档入口

### 2.2 Team

- 独立 Team 存储
- 独立 Team 页面 / API
- 成员、频道、消息、任务、产物、历史、治理
- 成员审批和批量治理
- Team 历史 diff
- Team 工作区与 Team 归档分离

### 2.3 Performance / Redis

- Redis 热链已经接入：
  - `Live`
  - `sync status`
  - `announcement`
  - `queue refs`
  - `/network`
- 内容主链已做：
  - 持续后台预热索引
  - 持续后台预热过滤结果
  - 首页、`/topics`、`/api/feed` 预热
  - 热点 RSS 预热

## 3. 仍值得继续做的任务

当前已经不是“缺主功能”，剩下主要是收尾、稳态、运维和体验。

### A. Archive 收尾

#### A1. Topics 归档 API 最后统一

目标：
- 继续弱化旧 `api/history/*` 的存在感
- 保持兼容，但所有页面和文档只继续输出 `archive/topics/*`

动作：
- 再检查一轮：
  - 页面内回链
  - JSON 链接
  - 文案
  - feed / archive 导航

验收：
- 用户正常使用时，不再需要看到旧 `history` API 路径

#### A2. Live 日归档实跑复核

目标：
- 不只“代码上有”，还要形成运行规则

动作：
- 连续复核：
  - `daily-*` 批次命名
  - 不重复归档
  - 手动和自动并存
  - 房间页入口稳定
- 至少再复核：
  - `public-live-time`
  - `public-etf-pro-duo`
  - `public-etf-pro-kong`

验收：
- 每个目标房间都能看到：
  - `manual-*`
  - `daily-*`
  之一或并存

#### A3. Team 归档入口最后收顺

目标：
- Team 工作区和 Team 归档切换更自然

动作：
- 统一这些页里的归档入口位置：
  - `/teams`
  - `/teams/<team>`
  - `/teams/<team>/members`
  - `/teams/<team>/channels/<channel>`
  - `/teams/<team>/tasks`
  - `/teams/<team>/tasks/<task>`
  - `/teams/<team>/artifacts`
  - `/teams/<team>/artifacts/<artifact>`
  - `/teams/<team>/history`

验收：
- 不手改 URL，也能从任何 Team 工作区页自然进入 Team 归档

### B. Team 最后一轮收尾

#### B1. 页面统一性最后一轮

目标：
- 让 Team 全页系统一成一个工作台节奏

动作：
- 统一：
  - hero 按钮密度
  - 当前上下文 panel
  - 返回链接风格
  - 查看 JSON / 查看历史 / 查看归档 / 返回工作区 的位置
  - 空状态文案

验收：
- Team 全页系没有“孤立入口”或明显风格断层

#### B2. 治理可读性收尾

目标：
- 成员治理和历史回看更清楚

动作：
- 再压一轮：
  - 成员状态摘要
  - 批量治理结果提示
  - policy 变更摘要
  - history 回链

验收：
- 管理者不需要来回翻几页才能理解当前治理状态

#### B3. Task / Artifact 上下文收口

目标：
- 让任务、产物、频道、历史之间的跳转更顺

动作：
- 再检查并统一：
  - `task -> artifact`
  - `artifact -> task`
  - `task -> channel`
  - `history -> task/artifact/channel`

验收：
- 任务上下文和产物上下文形成稳定闭环

### C. Runtime / Deploy

#### C1. `.75` 持续验收

目标：
- 保持 `.75` 运行态长期稳定

动作：
- 复核：
  - `/api/network/bootstrap`
  - `/archive/topics`
  - `/archive/live`
  - `/archive/team`
  - Team 工作区
  - Redis 在线
- 观察是否有：
  - 归档页长尾
  - Live 归档重复
  - Team 手动归档异常

验收：
- `.75` 作为主节点持续可用

#### C2. `.76` 视需要升级

目标：
- 如果需要对端验证，再把 `.76` 切到同一版

动作：
- 更新 `.76` 二进制
- 重启服务
- 复核：
  - `/api/network/bootstrap`
  - 归档入口
  - Team 页面

验收：
- `.76` 与 `.75` 在新归档结构上行为一致

### D. Performance / Redis 后续

#### D1. 首页 / Topics 冷路径继续压

目标：
- 把当前剩余的冷首轮长尾继续往下压

动作：
- 重点看：
  - `/`
  - `/topics`
  - `/api/feed`
  - `/topics/<topic>/rss`
- 保持：
  - 热态两位数毫秒
  - 冷态不回到多秒级

验收：
- 用户体感不再出现明显“偶发慢一下”

#### D2. Redis 只做稳态，不再盲目扩面

目标：
- 不再为了“用了 Redis”而继续无边界加缓存点

动作：
- 仅在出现明确瓶颈时，再考虑继续扩：
  - topics 主读链
  - archive 热读链

验收：
- Redis 维持当前稳定收益，不引入额外复杂度

### E. 文档 / 发布 / 备份

#### E1. 文档持续同步

保持更新：
- `README.md`
- `add-updata-20260330.md`
- `add-归档-20260402.md`
- `hoanews-team-20260401.md`

补充原则：
- 只写当前真实实现
- 不再写“未来会...”但尚未落地的内容

#### E2. 发布节奏

执行规则：
1. 本地测试通过
2. `.75` 验收通过
3. 推：
   - `main`
   - `tag`
   - `release`
4. 如有必要，打本地 zip 备份

#### E3. 本地备份

保持：
- 每轮重要版本变更后：
  - `~/sh18/backup/*.zip`

## 4. 推荐执行顺序

### 第一组：最优先

1. `A2 Live 日归档实跑复核`
2. `A3 Team 归档入口最后收顺`
3. `B1 Team 页面统一性最后一轮`

### 第二组：中优先

4. `A1 Topics 归档 API 最后统一`
5. `B2 Team 治理可读性收尾`
6. `B3 Task / Artifact 上下文收口`

### 第三组：持续型

7. `C1 .75 持续验收`
8. `D1 首页 / Topics 冷路径继续压`
9. `E1/E2/E3 文档、发布、备份`

### 第四组：按需

10. `C2 .76 升级`
11. `D2 Redis 扩面`

## 5. 自动执行原则

这份计划按 `auto-run` 执行时，遵守下面原则：

- 小问题不单独停下来问
- 默认先做：
  - 低风险
  - 可验证
  - 能明显收尾
  的部分
- 不扩展新模块
- `Team / Live / Topics` 继续完全分离
- 无关本地脏改不带进提交

## 6. 当前判断

到 `2026-04-03` 这个时间点：

- `Archive` 主功能已经到位
- `Team` 主功能已经到位
- `Redis` 和内容主链性能优化已经到位到可用阶段

剩下真正值得继续做的，不是大改架构，而是：
- 把归档、页面、治理、运行态再收顺
- 把冷路径和运行态继续稳住
- 保持文档和发布整洁

一句话：

- **主功能已成型**
- **剩下是高质量收尾、稳态验证、性能压尾巴**

## 7. Execution Status

### Completed

- `A1` 已收口到 `archive/topics/*` 主语义。
  - 用户可见页面不再输出旧 `archive/messages/*` 文章归档链接。
  - 旧 `api/history/*` 仅保留兼容，不再作为主入口输出。
- `A2` 已完成 `Live` 日归档实跑复核。
  - `public-live-time` 已确认同时存在 `daily-*` 与 `manual-*`
  - `public-etf-pro-duo` 已确认存在 `daily-*`
  - `public-etf-pro-kong` 已确认存在 `manual-*`
- `A3` 已完成 `Team` 归档入口最后收顺。
  - `/teams/<team>`、`members`、`history`、`tasks`、`artifacts`、`channels/<channel>` 都能自然进入 `archive/team/<team>`
- `B1` 页面统一性最后一轮已完成到当前可用收尾状态。
- `B2` 治理可读性当前已完成到可用收尾状态。
- `B3` `Task / Artifact / Channel / History` 上下文已形成稳定闭环。
- `C1` `.75` 已完成一轮持续验收。
  - `/api/network/bootstrap` 为 `ready`
  - `/archive/topics`
  - `/archive/live`
  - `/archive/team`
  均已通过
- `D1` 已完成当前一轮性能回归确认。
  - `home` 热态 `p95 ~ 4.3ms`
  - `topics` 热态 `p95 ~ 0.5ms`
  - `feed` 热态 `p95 ~ 0.4ms`
  - `rss` 热态 `p95 ~ 0.8ms`

### Deferred / On-demand

- `C2` `.76` 升级：按需执行，当前不作为本轮完成前提
- `D2` Redis 扩面：按需执行，当前不作为本轮完成前提
