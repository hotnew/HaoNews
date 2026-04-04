# `.75 / .74` Node Upgrade And Rollback

## Goal

- 把当前 `haonews` 的双节点升级、验收、回滚流程收成一份固定 runbook。
- 适用节点：
  - `.75` `192.168.102.75`
  - `.74` `192.168.102.74`

## Current Release Baseline

- 当前基线版本：
  - `v0.5.74`
- 当前 GitHub 发布节奏：
  1. 先推 `main`
  2. 再打 `tag`
  3. 再发 `release`
  4. 最后统一升级 `.75 / .74`

## Before Upgrade

- 确认当前仓库已经推到正式远端：
  - `haonews/main`
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
sshpass -p 'Grf123987!' scp \
  -o StrictHostKeyChecking=no \
  -o PreferredAuthentications=password \
  -o PubkeyAuthentication=no \
  -o IdentitiesOnly=yes \
  /Users/haoniu/go/bin/haonews \
  haoniu@192.168.102.74:/Users/haoniu/go/bin/haonews

sshpass -p 'Grf123987!' ssh \
  -o StrictHostKeyChecking=no \
  -o PreferredAuthentications=password \
  -o PubkeyAuthentication=no \
  -o IdentitiesOnly=yes \
  haoniu@192.168.102.74 '
  codesign --remove-signature /Users/haoniu/go/bin/haonews || true
  cp /Users/haoniu/go/bin/haonews /Users/haoniu/.hao-news/bin/hao-news-syncd
  codesign --force --sign - /Users/haoniu/go/bin/haonews
  codesign --force --sign - /Users/haoniu/.hao-news/bin/hao-news-syncd
  launchctl kickstart -k gui/501/com.haonews74.local
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
sshpass -p 'Grf123987!' ssh haoniu@192.168.102.74 'launchctl print gui/501/com.haonews74.local | rg "state = running"'
```

## Full Acceptance

- 升级完成后，不要只停在 bootstrap。
- 必须继续执行：
  - [runtime-75-74-validation.md](/Users/haoniu/sh18/hao.news2/haonews/docs/runtime-75-74-validation.md)
- 动态 webhook replay 建议使用：
  - `runtime-webhook-team`
- Live 验证除了 room API，还要额外检查：
  - `/api/live/status/public-live-time`

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
sshpass -p 'Grf123987!' scp \
  -o StrictHostKeyChecking=no \
  -o PreferredAuthentications=password \
  -o PubkeyAuthentication=no \
  -o IdentitiesOnly=yes \
  /Users/haoniu/go/bin/haonews \
  haoniu@192.168.102.74:/Users/haoniu/go/bin/haonews

sshpass -p 'Grf123987!' ssh \
  -o StrictHostKeyChecking=no \
  -o PreferredAuthentications=password \
  -o PubkeyAuthentication=no \
  -o IdentitiesOnly=yes \
  haoniu@192.168.102.74 '
  codesign --remove-signature /Users/haoniu/go/bin/haonews || true
  cp /Users/haoniu/go/bin/haonews /Users/haoniu/.hao-news/bin/hao-news-syncd
  codesign --force --sign - /Users/haoniu/go/bin/haonews
  codesign --force --sign - /Users/haoniu/.hao-news/bin/hao-news-syncd
  launchctl kickstart -k gui/501/com.haonews74.local
'
```

## Known Rules

- `serve` 负责托管 `syncd`，不要同时手工常驻独立 `hao-news-syncd sync`。
- `live_time_now.py` 使用独立 sender net：
  - `/Users/haoniu/.hao-news/hao_news_live_sender_net.inf`
- 节点问题优先走：
  1. GitHub `main + tag`
  2. 节点统一升级
  3. 运行态验证
- 不再把长期修复建立在本地临时补丁和临时后台进程上。
