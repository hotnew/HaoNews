# Hao.News libp2p 直传优化落地方案 (add-libp2p-pro1)

## 目标

基于 [libp2p-pro1.md](/Users/haoniu/sh18/hao.news2/libp2p-pro1.md) 的方向，为 Hao.News 当前 `internal/haonews` 同步链路补上“小 bundle 优先走 libp2p 直传，失败再回退 BT”的能力，同时尽量复用现有 pubsub、sync、store、status 结构，避免把队列格式和历史回流路径一次性改得过重。

## 当前代码对应关系

- libp2p host/runtime: `haonews/internal/haonews/libp2p.go`
- announcement/pubsub: `haonews/internal/haonews/pubsub.go`
- sync 主循环与 `syncRef`: `haonews/internal/haonews/sync.go`
- store / torrent 路径: `haonews/internal/haonews/store.go`
- 状态输出: `haonews/internal/haonews/status.go`
- 网络配置: `haonews/internal/haonews/network.go`

## 对原提案的收敛

原文方向正确，但示例代码直接把多处接口一起扩散了。按当前仓库结构，先做一版更稳的落地：

1. 新增 bundle transfer 协议与 handler。
2. pubsub announcement 自动附带发布者 `libp2p_peer_id`。
3. sync 收到 announcement 后把 `infohash -> peerID` 缓存在运行时内存里。
4. `syncRef` 在 BT 前优先尝试这些已知 peer 的 libp2p 直传。
5. 直传成功后本地重建 `.torrent` 文件，保持现有 store/seed/check 逻辑一致。
6. status 补充本轮最后一次导入所走 transport，以及累计 direct / bittorrent 导入计数。
7. network config 增加 `libp2p_transfer_max_size`。
8. CLI 增加 `--direct-transfer`，默认开启。

## 为什么不先改 queue 格式

当前 `sync` 队列是纯文本 magnet/infohash 文件，相关函数很多：

- `collectSyncRefs`
- `enqueueSyncRef`
- `removeSyncRef`
- `rotateSyncRef`
- `sanitizeSyncQueueFile`

如果为了保存 peerID 改队列格式，会把兼容、清洗和去重逻辑一起拖大。第一版更适合把 peer 线索留在 `syncRuntime` 的内存缓存里：

- pubsub announcement 到达时写入 `directPeers[infohash]`
- 队列仍只负责持久化 bundle ref
- daemon 生命周期内可多次重试直传
- 即使缓存丢失，也仍会自动回退 BT，不影响正确性

## 计划改动

### 1. `internal/haonews/transfer.go`

新增：

- `BundleTransferProtocol`
- `bundleTransferProvider`
- `FetchBundleViaLibP2P`
- tar 打包/解包与 SHA-256 校验
- 基于 torrent info 定位 contentDir
- 直传成功后重建 `.torrent`

约束：

- 只允许传输完整 bundle 目录
- 默认大小上限 20MB，可配置
- 解包必须防路径穿越
- 解包后必须通过 `LoadMessage`

### 2. `internal/haonews/pubsub.go`

新增字段：

- `SyncAnnouncement.LibP2PPeerID`

行为：

- `PublishAnnouncement` 自动从本地 host 填充 peer id
- `normalizeAnnouncement` 负责清洗该字段

### 3. `internal/haonews/libp2p.go`

调整：

- `startLibP2PRuntime(ctx, cfg, store)`
- 在 runtime 启动时注册 bundle transfer handler
- runtime 保存 transfer provider 和 max size

### 4. `internal/haonews/network.go`

新增配置：

- `libp2p_transfer_max_size=<bytes>`

处理：

- 默认写入 20MB
- 配置缺省时回退默认值
- 非法值时忽略并回退默认值

### 5. `internal/haonews/sync.go`

新增：

- `SyncOptions.DirectTransfer`
- `syncRuntime.directTransfer`
- `syncRuntime.directPeers map[string][]peer.ID`
- `rememberDirectPeer / directPeerIDs / clearDirectPeers`

行为：

- `handleAnnouncement` 记住 `announcement.LibP2PPeerID`
- `processQueue` 调 `syncRef` 时把候选 peerIDs 带进去
- `syncRef` 先试 libp2p 直传，再回退 BT
- `SyncItemResult` 增加 `Transport`

### 6. `internal/haonews/status.go`

新增状态字段：

- `SyncActivityStatus.DirectImported`
- `SyncActivityStatus.BitTorrentImported`
- `SyncActivityStatus.LastTransport`

### 7. `cmd/haonews/main.go`

新增：

- `--direct-transfer`，默认 `true`

## 风险控制

- 直传只作为优先路径，不替换 BT 回退
- 没有 peerID、peer 不在线、stream 超时、bundle 过大，都会自动回退 BT
- 队列格式不变，降低兼容风险
- 状态面只补字段，不改现有结构含义

## 验证重点

1. direct transfer 成功后 bundle 可被 `LoadMessage` 正常读取。
2. 直传成功后本地有对应 `.torrent` 文件。
3. announcement 自动带 `libp2p_peer_id`。
4. `syncRef` 在 direct 失败后仍能维持原 BT 路径。
5. network config 能正确解析 `libp2p_transfer_max_size`。
