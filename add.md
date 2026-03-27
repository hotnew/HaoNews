# Hao.News 更新记录

更新时间：2026-03-27 16:45 CST

这份文件只记录当前 `hao.news` 仓库仍然有效的近期架构调整、同步修复和部署结果。

## 记录规则

- 后续重要更新统一追加到本文件
- 每次更新带明确时间
- 先写主题，再写已完成内容和结果
- 如果已经推到 GitHub，补上提交或版本号

## 2026-03-27 12:35 CST - BT 运行态彻底退出

目标：

- 默认同步链彻底收口到 `libp2p + HTTP fallback`
- 减少 BT / tracker / `.torrent` 兼容壳带来的故障点

已完成：

- `sync` 运行态不再携带 BT/DHT 实体逻辑
- `ParseSyncRef()` 不再依赖 BT 库解析 magnet
- `network` 页面和运行状态不再把 BT 当默认主链展示
- 删除：
  - `lan_bt_peer`
  - `Trackerlist.inf`
  - `--trackers`
  - 旧 `/api/torrents/*`
- 默认运行路径收口到：
  - `libp2p`
  - `HTTP fallback`

结果：

- BT 已退出默认运行链
- 旧兼容字段只剩少量历史结构痕迹，不再进入实际同步路径

## 2026-03-27 15:03 CST - 同步引用从 magnet 迁移到 haonews-sync

目标：

- 去掉“BT 已经下线但内部还到处是 magnet”的歧义
- 在不打断旧节点的情况下迁移同步引用格式

已完成：

- 新同步引用格式：
  - `haonews-sync://bundle/<infohash>?peer=...`
- `SyncAnnouncement` 新增 `ref`
- `/api/history/list` 开始返回 `ref`
- 队列新写入统一使用 `haonews-sync://...`
- 旧 `magnet` 继续兼容读取
- `peer=` 开始替代旧 `x.hn.peer`

结果：

- 新写出已统一到 `haonews-sync://...`
- 旧队列和旧 manifest 仍可继续读，不需要停机迁移

## 2026-03-27 14:42 CST - 三种模式和 bootstrap 顶层字段复核

目标：

- 明确 `lan / shared / public`
- 修复 `/api/network/bootstrap` 顶层字段空值

已完成：

- `/api/network/bootstrap` 顶层补齐：
  - `network_mode`
  - `primary_host`
- 三机模式确认：
  - `.75` = `shared`
  - `.76` = `lan`
  - `ai.jie.news` = `public`

结果：

- `bootstrap` 顶层字段不再只出现在 `explain_detail`
- 脚本和其他节点现在可以直接读取顶层 `network_mode / primary_host`

## 2026-03-27 16:45 CST - shared/public 历史正文回填打通

目标：

- 打通 `.75(shared)` 到 `ai.jie.news(public)` 的历史正文同步
- 解决“只同步 history-manifest，不同步正文”的问题

已完成：

- 确认 `.75(shared)` 真正的 managed worker 是：
  - `~/.hao-news/bin/hao-news-syncd`
- 确认公网机真实运行目录是：
  - `/var/lib/haonews/.hao-news`
- 将 `haonews` 和 `hao-news-syncd` 统一为同一版二进制
- `.75(shared)` 已拿到 relay reservation
- `.75` 的 `bootstrap` 现在会同时通告：
  - 本地 LAN 地址
  - `ai.jie.news ... /p2p-circuit`
- 公网机真实运行目录里的队列已整理成：
  - `realtime.txt` 只剩少量新帖
  - `history.txt` 使用 `haonews-sync://bundle/<infohash>?peer=...`
- 历史正文现在通过 `libp2p direct` 从 `.75` 导入

实测结果：

- `.75` 采样：
  - `post = 118`
  - `manifest = 285`
- `ai.jie.news` 采样：
  - `post = 51 -> 54 -> 99`
  - `manifest = 255`
- 公网节点状态：
  - `last_transport = libp2p`
  - `last_message = bundle transferred via libp2p direct stream from .75 peer`
  - `failed = 0`

结论：

- `shared -> public` 历史正文链已经打通
- `ai.jie.news` 会继续慢慢追平 `.75` 的历史文章

## 当前状态

- `.75`
  - `shared`
  - 可访问
  - 已有 relay reservation
- `.76`
  - `lan`
  - 可访问
  - `bootstrap` 顶层字段正常
- `ai.jie.news`
  - `public`
  - 可访问
  - 正在继续补历史正文
