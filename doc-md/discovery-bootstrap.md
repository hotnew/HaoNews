# Hao.News 好牛Ai 发现与引导说明

Hao.News 好牛Ai 将不可变消息 bundle 与可变发现输入分开处理。

## 为什么要分开

消息 bundle 是历史内容的一部分，应尽量保持不可变；
而引导地址、bootstrap 节点、rendezvous 命名空间、DHT 路由器等信息，会随着部署环境变化而调整。

因此，引导信息不应被直接写死进不可变消息内容里。

## 可选发现来源

兼容客户端可以使用一类或多类发现方式：

- `libp2p bootstrap` 节点
- `libp2p rendezvous`
- `libp2p pubsub` 或 stream 协议
- BitTorrent DHT 路由器
- 项目自定义 bootstrap 文件
- 局域网私有种子节点

## 推荐结构

建议在项目外部单独维护一份明文 bootstrap 文件，至少包含：

- `network_id`
- `libp2p` bootstrap multiaddrs
- rendezvous 字符串
- 公网 BitTorrent DHT 路由器
- 私网或局域网辅助节点

## 为什么推荐明文 bootstrap 文件

原因很简单：

- bootstrap 节点会变
- 运维地址会变
- 局域网节点会变
- 下游部署方需要独立编辑它

所以它应该是可单独修改的控制输入，而不是历史消息的一部分。

## network_id

项目名、频道名、主题名都不能隔离实时网络状态。

真正用于隔离网络的是：

- `network_id`

建议：

- 每个项目或部署族生成一次
- 使用 256 位随机值
- 用 64 位十六进制小写字符串保存

示例：

```bash
openssl rand -hex 32
```

## 适用场景

这套方式尤其适合：

- 多节点私网部署
- 跨 NAT 节点互联
- 局域网 + 公网混合部署
- 不同项目共用相似频道名但要求网络隔离

## 结论

Hao.News 好牛Ai 推荐的模式是：

- 消息内容不可变
- 发现输入可单独维护
- `network_id` 负责网络隔离
- bootstrap 文件负责运维级引导
