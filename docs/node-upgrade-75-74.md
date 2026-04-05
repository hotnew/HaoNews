# `.75 / .74` Node Upgrade And Rollback

## Goal

- 把当前 `haonews` 的双节点升级、验收、回滚流程收成一份固定 runbook。
- 适用节点：
  - `.75` `192.168.102.75`
  - `.74` `192.168.102.74`

## Current Release Baseline

- 当前基线版本：
  - `v0.5.81`
- 当前 GitHub 发布节奏：
  1. 先在 `.75` 完成调试与验证
  2. 再推 `main`
  3. 再打 `tag`
  4. 再发 `release`
  5. 最后统一升级 `.74` 和其它节点

## Before Upgrade

- 确认当前仓库已经推到正式远端：
  - `haonews/main`
- 节点类问题默认流程固定为：
  1. `.75` 先调试好
  2. GitHub `main + tag + release`
  3. `.74` 从 GitHub/tag 升级
  4. 再做双节点验收
- `.74` 是发布验收节点，不再作为默认主调试场。
- 不再长期依赖本地临时补丁、临时 shell 后台进程或未发布二进制。
- `serve` 负责托管 `syncd`，不要同时手工常驻独立 `hao-news-syncd sync`。
- `.75` 的 live sender 必须继续使用：
  - `/Users/haoniu/.hao-news/hao_news_live_sender_net.inf`
- 节点升级完成后，统一按：
  - [runtime-75-74-baseline.md](/Users/haoniu/sh18/hao.news2/haonews/docs/runtime-75-74-baseline.md)
  - [runtime-75-74-validation.md](/Users/haoniu/sh18/hao.news2/haonews/docs/runtime-75-74-validation.md)
  做运行态验收。

## Upgrade Flow

### 1. GitHub 先行

- 先把修复推到 GitHub：
  - `main`
  - `tag`
- 当前正式远端是：
  - `haonews`
- 节点统一升级到同一 tag，不再长期依赖本地临时补丁。

### 2. `.75` 升级

```bash
cd /Users/haoniu/sh18/hao.news2/haonews
git checkout main
git pull haonews main || git pull origin main
git fetch --tags
git checkout <tag>

go build -o /Users/haoniu/go/bin/haonews ./cmd/haonews
cp /Users/haoniu/go/bin/haonews /Users/haoniu/.hao-news/bin/hao-news-syncd
codesign --force --sign - /Users/haoniu/go/bin/haonews
codesign --force --sign - /Users/haoniu/.hao-news/bin/hao-news-syncd
launchctl kickstart -k gui/501/com.haonews.local
```

### 3. `.74` 升级

```bash
sshpass -p 'Grf123987!' ssh \
  -o StrictHostKeyChecking=no \
  -o PreferredAuthentications=password \
  -o PubkeyAuthentication=no \
  -o IdentitiesOnly=yes \
  haoniu@192.168.102.74 '
  set -e
  git -C /Users/haoniu/sh18/HaoNews fetch --tags haonews >/dev/null 2>&1 || \
    git -C /Users/haoniu/sh18/HaoNews fetch --tags origin >/dev/null 2>&1
  git -C /Users/haoniu/sh18/HaoNews worktree prune
  rm -rf /tmp/HaoNews-upgrade
  git -C /Users/haoniu/sh18/HaoNews worktree add --detach /tmp/HaoNews-upgrade <tag> >/dev/null
  cd /tmp/HaoNews-upgrade
  /usr/local/go/bin/go build -o /Users/haoniu/go/bin/haonews ./cmd/haonews
  cp /Users/haoniu/go/bin/haonews /Users/haoniu/.hao-news/bin/hao-news-syncd
  codesign --force --sign - /Users/haoniu/go/bin/haonews
  codesign --force --sign - /Users/haoniu/.hao-news/bin/hao-news-syncd
  launchctl bootout gui/$(id -u)/com.haonews74.local >/dev/null 2>&1 || true
  launchctl bootout gui/$(id -u) /Users/haoniu/Library/LaunchAgents/com.haonews74.local.plist >/dev/null 2>&1 || true
  launchctl bootout gui/$(id -u)/com.haonews.local >/dev/null 2>&1 || true
  launchctl bootout gui/$(id -u) /Users/haoniu/Library/LaunchAgents/com.haonews.local.plist >/dev/null 2>&1 || true
  pkill -f "haonews serve" >/dev/null 2>&1 || true
  pkill -f "hao-news-syncd sync" >/dev/null 2>&1 || true
  launchctl bootstrap gui/$(id -u) /Users/haoniu/Library/LaunchAgents/com.haonews.local.plist
  cd /Users/haoniu/sh18/HaoNews
  git worktree remove /tmp/HaoNews-upgrade --force >/dev/null
'
```

## Minimal Validation

### `.75`

```bash
curl -s http://127.0.0.1:51818/api/network/bootstrap
curl -s http://127.0.0.1:51818/api/live/status/public-live-time
curl -s http://127.0.0.1:51818/api/teams
launchctl print gui/501/com.haonews.local | rg 'state = running'
```

### `.74`

```bash
curl -s http://192.168.102.74:51818/api/network/bootstrap
curl -s http://192.168.102.74:51818/api/live/status/public-live-time
curl -s http://192.168.102.74:51818/api/teams
sshpass -p 'Grf123987!' ssh haoniu@192.168.102.74 'launchctl print gui/501/com.haonews.local | rg "state = running"'
```

## Full Acceptance

- 升级完成后，不要只停在 bootstrap。
- 必须继续执行：
  - [runtime-75-74-validation.md](/Users/haoniu/sh18/hao.news2/haonews/docs/runtime-75-74-validation.md)
- 动态 webhook replay 建议使用：
  - `runtime-webhook-team`
- Live 验证除了 room API，还要额外检查：
  - `/api/live/status/public-live-time`
- 如果本次版本涉及 Team Room Plugin / ChannelConfig，还必须额外执行：
  - `GET /api/teams/{teamID}/channel-configs`
  - `GET /api/teams/{teamID}/channels/{channelID}/config`
  - `GET /teams/{teamID}/r/plan-exchange/?channel_id={channelID}&actor_agent_id=...`
- 从 `v0.5.81` 起，`channel_config` 自动同步是正式验收项，不再接受“频道已同步但配置靠远端手工 PUT”这种临时状态。

## Rollback

### `.75`

```bash
cd /Users/haoniu/sh18/hao.news2/haonews
git fetch --tags
git checkout <old-tag>
go build -o /Users/haoniu/go/bin/haonews ./cmd/haonews
cp /Users/haoniu/go/bin/haonews /Users/haoniu/.hao-news/bin/hao-news-syncd
codesign --force --sign - /Users/haoniu/go/bin/haonews
codesign --force --sign - /Users/haoniu/.hao-news/bin/hao-news-syncd
launchctl kickstart -k gui/501/com.haonews.local
```

### `.74`

```bash
sshpass -p 'Grf123987!' ssh \
  -o StrictHostKeyChecking=no \
  -o PreferredAuthentications=password \
  -o PubkeyAuthentication=no \
  -o IdentitiesOnly=yes \
  haoniu@192.168.102.74 '
  set -e
  git -C /Users/haoniu/sh18/HaoNews fetch --tags haonews >/dev/null 2>&1 || \
    git -C /Users/haoniu/sh18/HaoNews fetch --tags origin >/dev/null 2>&1
  git -C /Users/haoniu/sh18/HaoNews worktree prune
  rm -rf /tmp/HaoNews-upgrade
  git -C /Users/haoniu/sh18/HaoNews worktree add --detach /tmp/HaoNews-upgrade <old-tag> >/dev/null
  cd /tmp/HaoNews-upgrade
  /usr/local/go/bin/go build -o /Users/haoniu/go/bin/haonews ./cmd/haonews
  cp /Users/haoniu/go/bin/haonews /Users/haoniu/.hao-news/bin/hao-news-syncd
  codesign --force --sign - /Users/haoniu/go/bin/haonews
  codesign --force --sign - /Users/haoniu/.hao-news/bin/hao-news-syncd
  launchctl bootout gui/$(id -u)/com.haonews74.local >/dev/null 2>&1 || true
  launchctl bootout gui/$(id -u) /Users/haoniu/Library/LaunchAgents/com.haonews74.local.plist >/dev/null 2>&1 || true
  launchctl bootout gui/$(id -u)/com.haonews.local >/dev/null 2>&1 || true
  launchctl bootout gui/$(id -u) /Users/haoniu/Library/LaunchAgents/com.haonews.local.plist >/dev/null 2>&1 || true
  pkill -f "haonews serve" >/dev/null 2>&1 || true
  pkill -f "hao-news-syncd sync" >/dev/null 2>&1 || true
  launchctl bootstrap gui/$(id -u) /Users/haoniu/Library/LaunchAgents/com.haonews.local.plist
  cd /Users/haoniu/sh18/HaoNews
  git worktree remove /tmp/HaoNews-upgrade --force >/dev/null
'
```

## Known Rules

- `serve` 负责托管 `syncd`，不要同时手工常驻独立 `hao-news-syncd sync`。
- `live_time_now.py` 使用独立 sender net：
  - `/Users/haoniu/.hao-news/hao_news_live_sender_net.inf`
- 节点问题优先走：
  1. `.75` 先调试和验证
  2. GitHub `main + tag + release`
  3. `.74` 和其它节点统一升级
  4. 运行态验证
- 不再把长期修复建立在本地临时补丁和临时后台进程上。
