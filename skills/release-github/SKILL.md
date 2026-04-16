---
name: release-github
description: 当需要为 Hao.News 好牛Ai 及配套应用仓库发布一个小版本时使用。覆盖版本号更新、测试、全新克隆验证、安全推送、tag 创建和 GitHub Release 创建流程。
---

# GitHub 发布流程

当任务是为 GitHub 仓库发布一个新版本时，使用这份 skill。

## 适用仓库

当前主要面向：

- `HaoNews/HaoNews`
- 配套应用仓库

如果本地工作区是一个大根目录，推送时不要直接从根目录把子目录强推到 GitHub。

## 目标结果

每次发布至少完成这些步骤：

1. 更新源码和文档中的版本号
2. 更新发布说明草稿
3. 本地跑测试
4. 在需要时做一次全新克隆验证
5. 临时克隆 GitHub 仓库到新目录
6. 只复制相关子树到临时仓库
7. 使用 GitHub `noreply` 邮箱提交
8. 推送 `main`
9. 创建并推送 tag
10. 在 GitHub 上创建 Release

## 版本规则

- 优先做小版本递增
- 避免一次发布同时混入大范围无关改动

至少同步这些位置：

- `README.md`
- `doc-md/release.md`
- 相关升级说明
- 相关发布说明草稿

## 推送前测试

先执行：

```bash
go test ./...
```

如果改动涉及运行行为，还应补做：

- 全新克隆后的安装验证
- `go run ./cmd/haonews serve`
- 必要的 CLI / API 冒烟检查

## 安全推送流程

不要直接从当前复杂工作区向远端发版。

推荐流程：

1. 创建临时目录
2. 全新克隆目标 GitHub 仓库
3. 把本地目标子树复制到临时 clone
4. 排除 `.git`
5. 不要把临时构建产物、二进制和缓存带进去

推荐复制方式：

```bash
rsync -a --delete --exclude '.git' /local/path/haonews/ /tmp/push-haonews/
```

## 提交身份

GitHub 可能拒绝暴露私有邮箱的提交。

提交前设置：

```bash
git config user.name HaoNews
git config user.email <github-id>+HaoNews@users.noreply.github.com
```

如果遇到 `GH007`，重新设作者：

```bash
git commit --amend --no-edit --reset-author
```

## Tag 与 Release

每次发布应做：

1. 推送 `main`
2. 创建或更新 tag
3. 推送 tag
4. 依据发布说明创建 GitHub Release

## 额外提醒

- 发布文案统一使用 `Hao.News 好牛Ai`
- 命令名 `haonews`、协议字段、文件名保持兼容字面量
- 如果远端已有不同历史，只有在明确确认后才使用 `--force-with-lease`
