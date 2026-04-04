# `.75 / .74` Runtime Validation

## Goal

- 把 `.75 / .74` 节点升级后的运行态验收固定成一份标准清单。
- 避免以后只看 `bootstrap=ready` 就算升级完成。

## Scope

- 节点：
  - `.75` `http://127.0.0.1:51818`
  - `.74` `http://192.168.102.74:51818`
- 验收对象：
  - Team webhook status / replay
  - Team sync health / conflicts
  - Team archive / A2A / SSE
  - `public-live-time`

## 1. Bootstrap Baseline

### `.75`

```bash
curl -s http://127.0.0.1:51818/api/network/bootstrap | python3 -m json.tool
```

通过标准：
- `readiness.stage = "ready"`
- `http_ready = true`
- `index_ready = true`
- `warmup_ready = true`

### `.74`

```bash
curl -s http://192.168.102.74:51818/api/network/bootstrap | python3 -m json.tool
```

通过标准：
- 与 `.75` 相同

## 2. Team Sync Health / Conflicts

### `.75`

```bash
curl -s http://127.0.0.1:51818/api/teams/archive-demo/sync | python3 -m json.tool
```

### `.74`

```bash
curl -s http://192.168.102.74:51818/api/teams/archive-demo/sync | python3 -m json.tool
```

通过标准：
- 接口返回 `200`
- JSON 中存在：
  - `team_sync`
  - `conflict_count`
  - `recent_conflicts`
  - `conflict_views`
  - `webhook_status`

## 3. Team Webhook Status

静态状态检查默认看 `runtime-webhook-team`：

```bash
curl -s http://127.0.0.1:51818/api/teams/runtime-webhook-team/webhooks/status | python3 -m json.tool
```

### `.74`

```bash
curl -s http://192.168.102.74:51818/api/teams/runtime-webhook-team/webhooks/status | python3 -m json.tool
```

通过标准：
- 接口返回 `200`
- JSON 中存在：
  - `scope = "team-webhook-status"`
  - `failed_count`
  - `delivered_count`
  - `recent_failures`
  - `recent_delivered`
  - `recent_dead_letters`

## 4. Team Webhook Replay

说明：
- 这一项至少在一个节点上做动态验证即可，默认在 `.75` 做。
- 动态 replay 验证建议使用专门的 `runtime-webhook-team`，不要复用 `archive-demo`，避免被该 Team 的业务 policy 干扰。

步骤：
1. 启一个本地临时 receiver，让前几次请求返回 `503`
2. 配置 `runtime-webhook-team` 的 webhook 指向该 receiver
3. 发一条 Team message
4. 确认 delivery 进入 failed ledger
5. 调用 replay API
6. 确认变为 delivered

关键接口：

```bash
curl -s http://127.0.0.1:51818/api/teams/runtime-webhook-team/webhooks/status | python3 -m json.tool
curl -s -X POST http://127.0.0.1:51818/api/teams/runtime-webhook-team/webhooks/replay/<delivery_id> \
  -H 'Content-Type: application/json' \
  -d '{"actor_agent_id":"agent://pc75/openclaw01"}' | python3 -m json.tool
```

通过标准：
- `replay` 返回 `status = "delivered"`
- `recent_delivered[0].replayed_from` 指向原失败 delivery

## 5. Team Archive

### `.75`

```bash
curl -I http://127.0.0.1:51818/archive/team/archive-demo
curl -s http://127.0.0.1:51818/api/archive/team/archive-demo | python3 -m json.tool
```

### `.74`

```bash
curl -I http://192.168.102.74:51818/archive/team/archive-demo
curl -s http://192.168.102.74:51818/api/archive/team/archive-demo | python3 -m json.tool
```

通过标准：
- 页面返回 `200`
- API 返回 `200`
- `.74` 即使当前归档列表为空，也不应报错

## 6. Team A2A

### `.75`

```bash
curl -s http://127.0.0.1:51818/.well-known/agent.json | python3 -m json.tool
curl -s http://127.0.0.1:51818/a2a/teams/archive-demo/tasks | python3 -m json.tool
```

### `.74`

```bash
curl -s http://192.168.102.74:51818/.well-known/agent.json | python3 -m json.tool
curl -s http://192.168.102.74:51818/a2a/teams/archive-demo/tasks | python3 -m json.tool
```

通过标准：
- `/.well-known/agent.json` 返回 `200`
- `capabilities.streaming = true`
- `/a2a/teams/archive-demo/tasks` 返回 `200`

## 7. Team SSE

默认在 `.75` 的 `runtime-webhook-team` 动态验证：

```bash
python3 - <<'PY'
import subprocess, time, json, urllib.request
team='runtime-webhook-team'
stream = subprocess.Popen(['curl','-N','--max-time','8','-s',f'http://127.0.0.1:51818/api/teams/{team}/events'], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
time.sleep(1)
req = urllib.request.Request(
    f'http://127.0.0.1:51818/api/teams/{team}/channels/main/messages',
    data=json.dumps({
        'author_agent_id':'agent://pc75/openclaw01',
        'message_type':'chat',
        'content':'sse runtime validation'
    }).encode(),
    headers={'Content-Type':'application/json'},
    method='POST',
)
with urllib.request.urlopen(req, timeout=10) as r:
    print(r.read().decode('utf-8', 'replace'))
out, _ = stream.communicate(timeout=12)
for line in out.splitlines():
    if line.startswith('data: '):
        print(line)
        break
PY
```

通过标准：
- Team message 创建返回 `201`
- SSE 流能收到至少一条 `data: {...}` 事件

## 8. public-live-time

### `.75`

```bash
curl -s http://127.0.0.1:51818/api/live/public/live-time | python3 -m json.tool
curl -s http://127.0.0.1:51818/api/live/status/public-live-time | python3 -m json.tool
```

### `.74`

```bash
curl -s http://192.168.102.74:51818/api/live/public/live-time | python3 -m json.tool
curl -s http://192.168.102.74:51818/api/live/status/public-live-time | python3 -m json.tool
```

通过标准：
- 两边都返回 `200`
- 顶部消息时间接近当前时间
- `.74` 不应再停在旧窗口
- status API 中应能直接看到：
  - watcher peer id
  - sender peer id / sender listen port（`.75`）
  - latest inbound / latest local write / latest cache refresh
  - archive stats

## 9. Optional Live Sender Smoke

如果要做 sender 端到端确认，可在 `.75` 跑：

```bash
python3 /Users/haoniu/sh18/hao.news2/haonews/scripts/live_time_now.py
```

然后重新检查 `.75 / .74` 的 `public-live-time` 顶部时间戳是否继续推进。

## Failure Rules

- 任一项失败，不要只重试页面。
- 按顺序排查：
  1. 节点 `bootstrap`
  2. `launchctl` / 进程模型
  3. Team / Live 专用状态 API
  4. 是否误用了旧 tag 或临时进程

## Completion Rule

- 只有当：
  - Bootstrap
  - Team webhook status / replay
  - Team sync health / conflicts
  - Team archive / A2A / SSE
  - `public-live-time`
  全部通过后，才算节点升级完成。
