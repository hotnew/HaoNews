# 03 数据模型与本地存储

## 3.1 存储原则

默认采用：

- 单机
- 文件持久化
- JSON

推荐状态文件：

- `state.json`

可选导出文件：

- `brief-YYYYMMDD.md`
- `handoff-YYYYMMDD.md`

## 3.2 顶层数据结构

顶层建议包含：

- `team_name`
- `operators`
- `sources`
- `reviews`
- `decisions`
- `incidents`
- `handoffs`
- `briefs`
- `history`
- `updated_at`

## 3.3 Source

字段最少应有：

- `source_id`
- `headline`
- `summary`
- `origin`
- `url`
- `status`
- `priority`
- `tags`
- `notes`
- `created_at`
- `updated_at`

### status 枚举

- `new`
- `checking`
- `verified`
- `deferred`
- `dropped`

## 3.4 Review

字段最少应有：

- `review_id`
- `source_id`
- `kind`
- `title`
- `body`
- `actor`
- `status`
- `created_at`

### kind 枚举

- `review`
- `risk`

### status 枚举

- `open`
- `resolved`

## 3.5 Decision

字段最少应有：

- `decision_id`
- `source_ids`
- `title`
- `body`
- `status`
- `owner`
- `impact`
- `created_at`
- `updated_at`

### status 枚举

- `draft`
- `approved`
- `hold`
- `rejected`

## 3.6 Incident

字段最少应有：

- `incident_id`
- `title`
- `body`
- `severity`
- `status`
- `owner`
- `created_at`
- `updated_at`

### severity 枚举

- `low`
- `medium`
- `high`

### status 枚举

- `incident`
- `mitigating`
- `recovered`
- `postmortem`

## 3.7 Handoff

字段最少应有：

- `handoff_id`
- `title`
- `body`
- `status`
- `owner`
- `created_at`
- `updated_at`

### status 枚举

- `draft`
- `ready`
- `accepted`

## 3.8 Brief

字段最少应有：

- `brief_id`
- `title`
- `markdown`
- `status`
- `generated_from`
- `created_at`
- `updated_at`

### status 枚举

- `draft`
- `published`

## 3.9 History

字段最少应有：

- `event_id`
- `scope`
- `action`
- `title`
- `detail`
- `created_at`

## 3.10 ID 规则

建议：

- 全部使用稳定字符串 ID
- 前缀按对象区分

例如：

- `src-...`
- `rev-...`
- `dec-...`
- `inc-...`
- `han-...`
- `brf-...`

## 3.11 实现约束

如果采用文件持久化，必须保证：

1. 每次写入都原子替换
2. 状态文件可直接读回
3. 重启后不丢最近一次成功提交的数据
