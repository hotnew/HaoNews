# .75 / .74 Runtime Baseline

## Roles
- `.75`:
  - primary development node
  - main `haonews serve`
  - managed `hao-news-syncd`
  - `live_time_now.py` sender with dedicated sender net on `51585`
- `.74`:
  - secondary LAN node
  - main `haonews serve`
  - managed `hao-news-syncd`
  - live watcher on `51584`

## Local Config Files
### .75
- `~/.hao-news/hao_news_net.inf`
  - main network, `50584`, `lan_peer=192.168.102.74`, Redis enabled
- `~/.hao-news/hao_news_live_net.inf`
  - live watcher network, `51584`, `lan_peer=192.168.102.74`, Redis enabled
- `~/.hao-news/hao_news_live_sender_net.inf`
  - dedicated live sender network, `51585`, `lan_peer=192.168.102.74`, Redis enabled

### .74
- `~/.hao-news/hao_news_net.inf`
  - main network, `50584`, `lan_peer=192.168.102.75`
- `~/.hao-news/hao_news_live_net.inf`
  - live watcher network, `51584`, `lan_peer=192.168.102.75`
- `~/Library/LaunchAgents/com.haonews.local.plist`
  - 标准 `serve` 托管入口，已替代旧的 `com.haonews74.local`

## Process Model
### .75
- LaunchAgent: `com.haonews.local`
- `haonews serve` managed by `launchctl`
- `hao-news-syncd sync` spawned by `serve`
- `scripts/live_time_now.py` runs independently and launches `haonews live host` with `hao_news_live_sender_net.inf`

### .74
- LaunchAgent: `com.haonews.local`
- `haonews serve` managed by `launchctl`
- `hao-news-syncd sync` spawned by `serve`
- no separate sender process by default

## Verification Commands
### .75
- `curl -s http://127.0.0.1:51818/api/network/bootstrap | python3 -m json.tool`
- `curl -s http://127.0.0.1:51818/api/live/bootstrap | python3 -m json.tool`
- `ps -axo pid,ppid,command | rg 'haonews serve|hao-news-syncd sync|live_time_now.py|haonews live host'`

### .74
- `curl -s http://192.168.102.74:51818/api/network/bootstrap | python3 -m json.tool`
- `curl -s http://192.168.102.74:51818/api/live/bootstrap | python3 -m json.tool`
- `ssh haoniu@192.168.102.74 'launchctl print gui/501/com.haonews.local | sed -n "1,20p"'`
- `ssh haoniu@192.168.102.74 'ps -axo pid,ppid,command | grep -E "haonews serve|hao-news-syncd sync|haonews live host"'`

## Live Sync Smoke
1. Ensure `.75` sender is running:
   - `python3 scripts/live_time_now.py`
2. Check local latest event:
   - `curl -s http://127.0.0.1:51818/api/live/public/live-time`
3. Check remote latest event:
   - `curl -s http://192.168.102.74:51818/api/live/public/live-time`
4. Confirm latest timestamp/content matches current minute.

## Team Verification Pointers

- Team sync health / conflicts:
  - `curl -s http://127.0.0.1:51818/api/teams/archive-demo/sync | python3 -m json.tool`
  - `curl -s http://192.168.102.74:51818/api/teams/archive-demo/sync | python3 -m json.tool`
- Team webhook status:
  - `archive-demo` 主要看 sync API 内联的 `webhook_status`
  - 动态 webhook replay 验证继续使用 `runtime-webhook-team`
  - `curl -s http://127.0.0.1:51818/api/teams/runtime-webhook-team/webhooks/status | python3 -m json.tool`
  - `curl -s http://192.168.102.74:51818/api/teams/runtime-webhook-team/webhooks/status | python3 -m json.tool`
- Team webhook replay runtime verify:
  - use `runtime-webhook-team`
  - `curl -s http://127.0.0.1:51818/api/teams/runtime-webhook-team/webhooks/status | python3 -m json.tool`
  - `curl -s -X POST http://127.0.0.1:51818/api/teams/runtime-webhook-team/webhooks/replay/<delivery_id> -H 'Content-Type: application/json' -d '{"actor_agent_id":"agent://pc75/openclaw01"}' | python3 -m json.tool`
- Team archive:
  - `curl -I http://127.0.0.1:51818/archive/team/archive-demo`
  - `curl -I http://192.168.102.74:51818/archive/team/archive-demo`
- A2A:
  - `curl -s http://127.0.0.1:51818/.well-known/agent.json | python3 -m json.tool`
  - `curl -s http://192.168.102.74:51818/.well-known/agent.json | python3 -m json.tool`
- Team SSE:
  - `curl -N http://127.0.0.1:51818/api/teams/runtime-webhook-team/events`
  - 运行态建议使用 `runtime-webhook-team`，避免 `archive-demo` 的签名策略干扰无签名验证

完整升级验收清单见：
- [runtime-75-74-validation.md](/Users/haoniu/sh18/hao.news2/haonews/docs/runtime-75-74-validation.md)

## Recovery Rules
- Do not run `live host` sender on `.75` with `hao_news_live_net.inf`; use `hao_news_live_sender_net.inf` only.
- Do not run standalone `hao-news-syncd` when `serve` is already managing sync.
- Prefer `launchctl kickstart -k` for service restarts.
- `.74` 已不再使用 `com.haonews74.local`，如发现该 agent 存在，先 `bootout` 再继续验收。
- After any live transport fix, redeploy both `.75` and `.74` to the same Git tag before validating.
