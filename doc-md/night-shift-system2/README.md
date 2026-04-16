# 夜间快讯值班系统2

## 目标

这是一套**与 Team 无关**的独立本地程序规格包。

它的来源是前期用 Team + 多 agent 讨论出来的流程和规则，但最终实现时：

- 不依赖 Team 页面
- 不依赖 Team API
- 不依赖 Team 数据结构
- 不依赖 Room Plugin

任何大模型只要拿到这个目录下的 md，就应该能独立实现一个本地可运行的 `夜间快讯值班系统2`。

## 阅读顺序

1. [01-product.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/01-product.md)
2. [02-workflows.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/02-workflows.md)
3. [03-data-model.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/03-data-model.md)
4. [04-screens-and-interactions.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/04-screens-and-interactions.md)
5. [05-api-and-runtime.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/05-api-and-runtime.md)

补充参考：

- [product-spec.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/product-spec.md)
- [flows-and-states.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/flows-and-states.md)
- [data-model.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/data-model.md)
- [api-contract.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/api-contract.md)
- [verification.md](/Users/haoniu/sh18/hao.news2/haonews/doc-md/night-shift-system2/verification.md)

## 交付要求

实现方需要做出的不是文档系统，而是一个独立本地程序：

- 本地可启动
- 浏览器可访问
- 有真实状态流转
- 有本地持久化
- 能完成一次完整夜班值守流程

## 实现约束

- 默认实现为 **本地 Web 程序**
- 默认单机运行
- 默认本地文件持久化
- 默认无登录系统
- 默认无外部网络依赖
- 默认无 LLM 强依赖

如果实现方要偏离这些默认值，必须保证：

- 最终用户行为不变
- 数据结果不变
- 核心流程不变
