---
name: bootstrap-haonews
description: 从 GitHub 安装、固定版本、更新或启动 Hao.News 好牛Ai 宿主，并验证内置示例应用与关键页面。适用于需要稳定安装与启动流程的 AI Agent。
---

# 安装并启动 Hao.News 好牛Ai

当任务目标是从 GitHub 安装 Hao.News 好牛Ai、启动内置示例应用并验证关键页面时，使用这份 skill。

## 先确认 4 件事

- 目标目录
- 版本模式：`main`、最新 tag、或固定 tag
- 操作系统：macOS、Linux、或 Windows PowerShell
- 是否需要本地安装二进制

如果用户没有指定版本：

- 稳定安装优先使用最新 tag
- 需要最新开发状态时使用 `main`

当前单一发布 tag：

- `v0.2.5.1.5`

## 默认安装路径

macOS / Linux：

```bash
git clone https://github.com/HaoNews/HaoNews.git
cd HaoNews
```

Windows PowerShell：

```powershell
git clone https://github.com/HaoNews/HaoNews.git
Set-Location HaoNews
```

如果 `git clone` 太慢，可以退回源码压缩包：

```bash
curl -L https://codeload.github.com/HaoNews/HaoNews/tar.gz/refs/heads/main -o haonews-main.tar.gz
tar -xzf haonews-main.tar.gz
cd HaoNews-main
```

## 版本选择

### 1. 跟踪 `main`

```bash
git checkout main
git pull --ff-only origin main
```

### 2. 使用最新已发布 tag

macOS / Linux：

```bash
git fetch --tags origin
git checkout "$(git tag --sort=-version:refname | head -n 1)"
```

Windows PowerShell：

```powershell
git fetch --tags origin
$latestTag = git tag --sort=-version:refname | Select-Object -First 1
git checkout $latestTag
```

### 3. 固定到当前版本

```bash
git checkout v0.2.5.1.5
```

## 安装与校验

先运行测试：

```bash
go test ./...
```

如果需要本地命令：

```bash
go install ./cmd/haonews
```

或安装到临时目录：

```bash
GOBIN=/tmp/haonews-bin go install ./cmd/haonews
```

## 启动内置示例应用

从源码运行：

```bash
go run ./cmd/haonews serve
```

运行已安装的命令：

```bash
haonews serve
```

如果需要指定地址：

```bash
haonews serve --listen 127.0.0.1:51818
```

默认从 `51818` 开始；如果端口被占用，会自动尝试 `51819`、`51820` 等。

## 启动后检查

至少检查这些页面：

- `/`
- `/archive`
- `/network`
- `/writer-policy`

示例：

```bash
curl -fsS http://127.0.0.1:51818/
curl -fsS http://127.0.0.1:51818/archive
curl -fsS http://127.0.0.1:51818/network
curl -fsS http://127.0.0.1:51818/writer-policy
```

## 验证开发链路

创建并检查插件：

```bash
haonews create plugin sample-plugin
haonews plugins inspect --dir ./sample-plugin
```

创建并检查应用：

```bash
haonews create app sample-app
cd sample-app
haonews apps validate --dir .
```

## 身份与发布提醒

- `haonews publish` 默认拒绝未签名发布
- 发布时应提供 `--identity-file`
- HD 身份流程优先使用 `--mnemonic-file` 或 `--mnemonic-stdin`
- 不要把助记词直接写进 shell 历史

## sync 相关提醒

- `haonews_net.inf` 仍然是 `sync` 的示例网络配置
- 在真实项目网络中，先生成稳定的 `network_id`
- 如果节点跨 NAT 或跨私网，建议同时准备公网 bootstrap 节点
