# 05 本地 API、运行方式、验收

## 5.1 运行方式

推荐第一版：

- 本地 HTTP 服务
- 默认监听 `127.0.0.1`
- 单进程
- 单状态文件

## 5.2 最小 API

如果实现为本地 Web 程序，最少建议提供这些接口：

### 读取

- `GET /api/state`
- `GET /api/sources`
- `GET /api/decisions`
- `GET /api/reviews`
- `GET /api/incidents`
- `GET /api/handoffs`
- `GET /api/briefs`

### 写入

- `POST /actions/source`
- `POST /actions/source-status`
- `POST /actions/review`
- `POST /actions/decision`
- `POST /actions/incident`
- `POST /actions/handoff`
- `POST /actions/brief-generate`

说明：

- 第一版不要求 REST 纯度
- 允许页面表单直接 POST 到动作端点

## 5.3 最小导出能力

至少支持导出：

- 当前夜间简报 Markdown
- 当前交接摘要 Markdown

导出结果应可直接保存为文件。

## 5.4 验收命令

任意实现至少应能通过：

1. 启动程序
2. 打开首页
3. 新增来源
4. 把来源推进到 `verified`
5. 基于来源新增决策
6. 新增一条风险
7. 新增一条事故并推进到 `recovered`
8. 新增一条交接并推进到 `accepted`
9. 导出一份 Markdown 简报
10. 重启程序后数据仍在

## 5.5 验收结果定义

只有当以下全部成立时，才算“做出了夜间快讯值班系统2”：

- 有独立程序入口
- 有独立本地运行页面
- 不依赖 Team API
- 不依赖 Team 数据文件
- 数据可持久化
- 值班主链可完整走通
- 可导出 Markdown

## 5.6 禁止误实现

以下情况都不算完成：

- 只是 Team 样本页面
- 只是把 Team 页面 iframe 包一层
- 只是读取 Team API 再展示
- 只有静态原型、不能改数据
- 能展示但不能持久化
