# `spec-package` 标准操作法

这份文档定义 `spec-package` 的标准使用方法。

目标不是“在 Team 里直接做最终程序”，而是：

1. 用多个 agent 在 Team 里把目标、边界、风险、取舍说清
2. 把讨论收成一组稳定的 Markdown 规格文档
3. 再把这组规格文档交给下游本地 agent 或任意大模型独立实现

## 适用场景

适合下面这类任务：

- 做一个新的本地应用
- 做一个新的后端服务
- 做一个新的工具链或自动化程序
- 任何需要先冻结规格、再进入实现的项目

不适合：

- 直接在 Team 里承载最终运行时
- 把 Team 当最终产品数据库
- 把 Team 页面当目标产品页面

## 起手方式

直接用内置模板：

- `spec-package`

模板默认提供：

- 4 个频道
  - `main`
  - `reviews`
  - `decisions`
  - `artifacts`
- 5 个里程碑
  - `scope-frozen`
  - `workflow-frozen`
  - `data-model-ready`
  - `verification-ready`
  - `spec-package-ready`

## 角色分工

最小推荐角色：

- `owner`
  - 最终拍板
  - 负责冻结边界和验收规格包
- `proposer`
  - 提出目标、非目标、约束、候选方案
- `reviewer`
  - 专门找缺口、风险、歧义、冲突
- `editor`
  - 把讨论收成结构化 Markdown 产物

一个人也能兼多角色，但语义上最好保留这 4 个责任位。

## 频道职责

### `main`

插件：

- `plan-exchange`

只做：

- 目标
- 非目标
- 约束
- 候选方案
- 技能卡 / 片段
- 规格目录结构

不要在这里做：

- 长期 review 来回拉扯
- 最终取舍冻结
- 正式规格正文堆放

### `reviews`

插件：

- `review-room`

只做：

- `review`
- `risk`
- `decision`

主要职责：

- 提前打掉规格缺口
- 挑战模糊边界
- 发现可能导致返工的地方
- 产出线程级 `review-summary`

### `decisions`

插件：

- `decision-room`

只做：

- `proposal`
- `option`
- `decision`

主要职责：

- 冻结实现口径
- 冻结运行时边界
- 冻结必须接受的 tradeoff

这里的结论应该能直接回答：

- 为什么这样做
- 为什么不选另外一个方案
- 下游实现时哪些点不再重开讨论

### `artifacts`

插件：

- `artifact-room`

只做：

- 规格包正文
- 版本化 revision
- 最终 publish

至少应该沉淀这些文档：

- `README`
- `product`
- `workflows`
- `data-model`
- `screens-and-interactions`
- `api-and-runtime`
- `verification`

## 推荐推进顺序

### Phase 1. 冻结范围

在 `main` 里收：

- 目标
- 非目标
- 约束
- 候选方案

完成后，应能冻结：

- `scope-frozen`

### Phase 2. 打 review

在 `reviews` 里至少补：

- 1 条 `review`
- 1 条 `risk`

最好让 reviewer 明确回答：

- 现在最可能返工的点是什么
- 哪个边界还没说清
- 哪些动作/状态还没定义

### Phase 3. 冻结决策

在 `decisions` 里把最重要的 1 到 3 个边界冻结掉。

通常至少包括：

- 运行时边界
- 存储边界
- 导出/发布边界

完成后，应能冻结：

- `workflow-frozen`

### Phase 4. 写正文规格

在 `artifacts` 里沉淀 6 到 7 份 Markdown 主文档。

至少完成：

- `product`
- `workflows`
- `data-model`
- `verification`

完成后，应能冻结：

- `data-model-ready`
- `verification-ready`

### Phase 5. 规格包冻结

当下面几条都成立时，再冻结：

- 目标和非目标清楚
- 关键风险已被 review 过
- 关键取舍已进入 `decision-room`
- 正文规格已进入 `artifacts`
- 下游实现不再依赖 Team 页面/API/数据格式

最后冻结：

- `spec-package-ready`

## 交付标准

一个合格的 `spec-package` 应该至少有：

- 4 个频道都真实使用过
- 至少 1 条 `plan`
- 至少 1 条 `review`
- 至少 1 条 `risk`
- 至少 1 条 `decision`
- 至少 1 条线程级或消息级 `review-summary / decision-note`
- 至少 6 份 Markdown 规格文档

并且下游实现者在不看 Team 页面、不读 Team 源码的前提下，也能只凭这组 md 开始做程序。

## 产物清单建议

推荐最终产物目录：

- `README`
- `01-product.md`
- `02-workflows.md`
- `03-data-model.md`
- `04-screens-and-interactions.md`
- `05-api-and-runtime.md`
- `verification.md`

## 常见反模式

### 1. 在 Team 里直接做最终程序

这是错位。

Team 应该负责：

- 协作
- 评审
- 冻结边界
- 产出规格

不是负责承载目标程序运行时。

### 2. 只聊天，不产出 Markdown

这会导致下游实现无法接手。

`spec-package` 的真正交付物不是讨论记录本身，而是：

- 讨论后得到的规格包

### 3. 只有 `main`，没有 `reviews / decisions / artifacts`

这样很容易让：

- review 和方案讨论混在一起
- 决策没有冻结点
- 最后没人负责把东西收成文档

### 4. 没有冻结里程碑

没有冻结点，就会在实现期继续重开讨论。

`spec-package` 的 5 个里程碑就是为了避免这个问题。

## 当前真实样本

推荐参考：

- [spec-package-team-demo.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/spec-package-team-demo.md)

当前真实样本：

- `night-shift-spec3`
- 运行态复核模板：
  - `night-shift-spec4`

## 最终目标

`spec-package` 的价值不是“多一个 Team 模板”，而是让上游规格共创形成固定套路：

1. 先讨论
2. 再评审
3. 再冻结
4. 最后产出 Markdown 规格包
5. 再让下游独立实现

只有这样，Team 才真正回到了你最初设计的主思想上。
