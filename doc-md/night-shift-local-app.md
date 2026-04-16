# 夜间快讯值班系统2 本地程序

## 这是什么

这不是 Team 页面里的另一个样本，而是一个**与 Team 运行时完全解耦**的本地程序。

它按 `doc-md/night-shift-system2/` 里的规格包实现，当前已经具备：

- 值班总览
- 来源池
- 风险复核
- 终审决策
- 事故处置
- 交接与简报
- 任务页
- 归档页

## 怎么启动

在仓库根目录运行：

```bash
cd /Users/haoniu/sh18/hao.news2/haonews
go run ./cmd/haonews nightshift --listen 127.0.0.1:51921
```

启动后打开：

- [http://127.0.0.1:51921/](http://127.0.0.1:51921/)

## 数据放哪里

默认状态文件：

- `/Users/haoniu/.hao-news/night-shift-system2/state.json`

可以改路径：

```bash
go run ./cmd/haonews nightshift \
  --listen 127.0.0.1:51921 \
  --state /tmp/night-shift-system2.json
```

## 当前能力

### 1. 首页与 8 个工作区页面

可以直接看：

- 待核来源数
- 待复核数
- 待终审数
- 未恢复事故数
- 待交接数
- 最近动态

并且页面入口已经独立成：

- `/`
- `/sources`
- `/reviews`
- `/decisions`
- `/briefs`
- `/incidents`
- `/handoffs`
- `/tasks`
- `/archive`

### 2. 来源池

可以直接：

- 新增来源
- 推进来源状态：
  - `new`
  - `triaging`
  - `needs_review`
  - `ready_for_decision`
  - `approved`
  - `deferred`
  - `rejected`
  - `handoff`

### 3. 风险复核

可以直接新增：

- `review`
- `risk`
- `decision-support`

并支持：

- `open`
- `resolved`

### 4. 终审决策

可以直接新增正式终审结论：

- `publish_now`
- `hold`
- `discard`
- `handoff`

并自动联动：

- 来源状态
- 任务状态
- `decision-note`
- 最近动态

### 5. 事故处置

可以直接走：

- `incident`
- `update`
- `recovery`

并自动联动任务：

- `incident -> blocked`
- `update -> doing`
- `recovery -> done`

恢复时会自动生成：

- `incident-summary`

### 6. 交接与简报

可以直接走：

- `handoff`
- `checkpoint`
- `accept`

并支持：

- 生成夜间简报
- 导出最新简报 Markdown
- 导出交接摘要 Markdown

`accept` 会自动生成：

- `handoff-summary`

### 7. 任务与归档

现在已经不是只看流程对象，还能直接看：

- `tasks`
- `archive`

归档里会出现：

- `decision-note`
- `brief`
- `incident-summary`
- `handoff-summary`

## 适合怎么用

这版最适合：

- 本地独立跑一轮夜间值班流程
- 用规格包驱动实现，而不是依赖 Team 运行时
- 作为后续继续产品化的稳定起点

## 与 Team 的关系

`Team` 只用于前期多 agent 协作、评审和产出规格 md。

这个程序本身：

- 不依赖 Team 页面
- 不依赖 Team API
- 不依赖 Team 数据文件
- 只复用 `night-shift-system2` 规格包里的文档内容作为设计输入

## 当前代码位置

- CLI 入口：
  - [/Users/haoniu/sh18/hao.news2/haonews/cmd/haonews/main.go](/Users/haoniu/sh18/hao.news2/haonews/cmd/haonews/main.go)
- 程序实现：
  - [/Users/haoniu/sh18/hao.news2/haonews/internal/nightshiftdesk/server.go](/Users/haoniu/sh18/hao.news2/haonews/internal/nightshiftdesk/server.go)
- 页面模板：
  - [/Users/haoniu/sh18/hao.news2/haonews/internal/nightshiftdesk/templates/index.html](/Users/haoniu/sh18/hao.news2/haonews/internal/nightshiftdesk/templates/index.html)
