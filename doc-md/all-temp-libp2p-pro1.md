# all-temp-libp2p-pro1

## 任务输入

- 基础提案：`libp2p-pro1.md`
- 输出要求：
  - 先形成 repo 对应的方案文档 `doc-md/add-libp2p-pro1.md`
  - 再按该思路整改代码
  - 把整个过程整理到本文件

## 现状核对

先对仓库真实结构做了映射，确认 `libp2p-pro1.md` 中的 `internal/aip2p/*` 在当前项目里对应为：

- `haonews/internal/haonews/libp2p.go`
- `haonews/internal/haonews/pubsub.go`
- `haonews/internal/haonews/sync.go`
- `haonews/internal/haonews/network.go`
- `haonews/internal/haonews/status.go`
- `haonews/internal/haonews/store.go`

结论：

1. 当前同步链路确实只有 BT 路径，`syncRef` 不会优先尝试 libp2p 直传。
2. pubsub announcement 里没有发布者 `PeerID`，因此即使加了直传协议，也缺少“向谁拉”的线索。
3. 现有 queue 是纯文本 magnet/infohash 文件，直接改格式会牵连 `collect/enqueue/remove/rotate/sanitize` 全链路。
4. `startLibP2PRuntime` 只在 `RunSync` 调用一次，适合在这里挂新的 transfer handler。

## 方案收敛

没有照 `libp2p-pro1.md` 原样硬套，做了两点关键收敛：

### 1. peer 线索先放内存，不改 queue 格式

原因：

- queue 改格式会扩大风险面
- 当前第一版更适合把 `infohash -> peerID[]` 放在 `syncRuntime` 内存中
- daemon 生命周期内可以重复尝试 direct transfer
- 没有 peerID 时仍能自然回退 BT

### 2. 直传成功后本地重建 `.torrent`

原因：

- 现有很多本地一致性检查依赖 torrent 文件
- 只导入 bundle 目录不够，需要让 `hasCompleteLocalBundle`、后续 seeding 和 store 结构继续正常工作

## 已写的方案文档

已新增：

- `doc-md/add-libp2p-pro1.md`

核心内容：

- 新增 bundle transfer 协议
- pubsub announcement 自动带 `libp2p_peer_id`
- sync runtime 缓存 direct peers
- `syncRef` 优先 direct，失败回退 BT
- network config 新增 `libp2p_transfer_max_size`
- CLI 增加 `--direct-transfer`
- status 增加 direct / bittorrent 导入统计和最后 transport

## 实际整改

### 新增

- `haonews/internal/haonews/transfer.go`

实现了：

- `/haonews/bundle-transfer/1.0` stream protocol
- provider handler
- tar 打包/解包
- SHA-256 校验
- 安全解包
- 本地重建 `.torrent`

### 修改

- `haonews/internal/haonews/pubsub.go`
  - `SyncAnnouncement` 增加 `libp2p_peer_id`
  - `PublishAnnouncement` 自动填充本地 peer id

- `haonews/internal/haonews/libp2p.go`
  - runtime 挂载 transfer provider
  - status 增加直传能力与大小上限

- `haonews/internal/haonews/network.go`
  - 解析 `libp2p_transfer_max_size`
  - 默认配置文件写入该项

- `haonews/internal/haonews/status.go`
  - `SyncActivityStatus` 增加：
    - `direct_imported`
    - `bittorrent_imported`
    - `last_transport`
  - `SyncLibP2PStatus` 增加：
    - `direct_transfer_enabled`
    - `transfer_max_size`

- `haonews/internal/haonews/sync.go`
  - `SyncOptions` 增加 `DirectTransfer`
  - `SyncItemResult` 增加 `Transport`
  - `syncRuntime` 增加 direct peer 缓存
  - `handleAnnouncement` 记住 announcement 的 peer id
  - `syncRef` 先尝试 libp2p direct transfer，再回退 BT
  - direct 成功后仍保留现有本地 bundle / quota / torrent 语义

- `haonews/cmd/haonews/main.go`
  - `sync` 子命令新增 `--direct-transfer`，默认 `true`

## 测试补充

新增或扩展了这些测试：

- `TestLoadNetworkBootstrapConfig`
  - 覆盖 `libp2p_transfer_max_size`

- `TestHandleAnnouncementRemembersDirectPeer`
  - 覆盖 announcement 到 sync runtime 的 peer 线索缓存

- `TestSyncRefImportsViaLibP2PDirectTransfer`
  - 两个测试 host 建连
  - provider 发布 bundle
  - requester 通过 `syncRef` 直传拉取
  - 验证：
    - `result.Status == imported`
    - `result.Transport == libp2p`
    - 本地 bundle 完整
    - 本地 `.torrent` 存在

## 过程中出现的问题

### 1. 误判代码目录

最开始按文档中的 `internal/aip2p` 去搜，当前仓库并没有这个目录。后面改成对 `haonews/internal/haonews/*` 真实结构落地。

### 2. 工作区曾经磁盘满

前一轮修改中 `apply_patch` 和 `touch` 因磁盘满失败，后面空间清理后恢复。

### 3. 新测试第一次空指针

`TestHandleAnnouncementRemembersDirectPeer` 初版忘了给 `syncRuntime` 注入 `store`，导致 `hasCompleteLocalBundle` 空指针。已修复。

## 验证结果

执行通过：

```bash
go test ./internal/haonews
go test ./cmd/haonews
```

## 当前工作区状态说明

本次任务改动文件：

- `haonews/cmd/haonews/main.go`
- `haonews/internal/haonews/libp2p.go`
- `haonews/internal/haonews/network.go`
- `haonews/internal/haonews/pubsub.go`
- `haonews/internal/haonews/status.go`
- `haonews/internal/haonews/sync.go`
- `haonews/internal/haonews/sync_test.go`
- `haonews/internal/haonews/transfer.go`
- `doc-md/add-libp2p-pro1.md`

同时工作区里原本就还有与本任务无关的未提交改动：

- `haonews/internal/haonews/live/archive.go`
- `haonews/internal/haonews/live/protocol_test.go`

本次没有去回退或覆盖这些已有改动。

## 后续可继续推进

如果继续做第二阶段，建议按这个顺序：

1. history manifest / LAN history 路径也补上 peer 线索传播。
2. direct transfer 失败原因做更细的状态统计。
3. 对 queue 做可选的 peer hint 持久化，而不是只保留内存缓存。
4. 增加 direct transfer 的超时、成功率和平均耗时监控字段。
