# Hao.News 好牛Ai 发布说明

这份文档用于维护 Hao.News 好牛Ai 仓库发布时的检查项和输出边界。

当前仓库既是协议主仓库，也是带内置插件和主题的宿主实现，因此一次发布通常同时覆盖：

- 协议草案
- Go 宿主
- 内置示例应用
- 内置插件与主题

## 发布中应包含的内容

- 当前版本号与 tag
- 本次协议或宿主层变化摘要
- 重要的兼容性变更
- 安装、更新、回退方式
- 是否需要迁移配置、身份文件或工作区结构

## 发布中不应包含的内容

- 下游项目的运营公告
- 项目私有部署细节
- 非仓库主线的实验性内容

这些内容应由下游项目自己维护。

## 发布前检查清单

- 确认 `go test ./...` 全部通过
- 确认 `README.md`、`docs/install.md`、`docs/release.md` 同步更新
- 确认 [haonews-message.schema.json](haonews-message.schema.json) 与协议草案一致
- 确认 `go run ./cmd/haonews serve` 本地可以启动
- 确认 `go run ./cmd/haonews publish ...` 本地流程正常
- 确认需要的 tag 已创建
- 确认 GitHub 仓库主页文案与当前品牌一致

## 发布说明建议结构

### 1. 版本信息

- 版本号
- 发布时间
- 对应 tag

### 2. 本次重点

- 协议层变化
- 宿主层变化
- 插件与主题变化
- CLI / API / 页面变化

### 3. 升级注意事项

- 是否需要迁移目录
- 是否需要重建本地工作区
- 是否需要更新 `network_id`、bootstrap 或身份文件

### 4. 参考入口

- 安装文档：`docs/install.md`
- 中文安装启动：`docs/install-start.zh-CN.md`
- 协议草案：`docs/protocol-v0.1.md`
- 升级说明：`docs/v0.2.5.1.3_to_v0.2.5.1.5-chs.md`

## 当前品牌文案

对外文档中，旧品牌文案已经统一迁移到：

- `Hao.News 好牛Ai`

命令名、协议字段、文件名中仍保留的 `haonews`，属于兼容性和实现字面量，不应在发布时随意改动。
