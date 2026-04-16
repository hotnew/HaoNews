# 02 标准流程与状态机

## 2.1 标准值班主链

夜间快讯值班系统2 的标准流程固定为：

1. 来源进入来源池
2. 夜班编辑做初筛
3. 复核员补 review / risk
4. 值班主编形成终审决策
5. 若有异常，进入 incident 链
6. 形成夜间简报
7. 形成交接摘要
8. 早班接手并确认

## 2.2 来源状态

每条来源至少有以下状态：

- `new`
  - 新进入，尚未处理
- `checking`
  - 正在核验
- `verified`
  - 已核实，可进入终审
- `deferred`
  - 暂缓，等待更多信息
- `dropped`
  - 放弃，不再跟进

### 状态转换规则

- `new -> checking`
- `checking -> verified`
- `checking -> deferred`
- `checking -> dropped`
- `deferred -> checking`
- `verified` 不允许直接回到 `new`

## 2.3 终审决策状态

每条决策至少有以下状态：

- `draft`
- `approved`
- `hold`
- `rejected`

含义：

- `approved`
  - 已决定可发
- `hold`
  - 暂缓，等待更多信息
- `rejected`
  - 明确不发

## 2.4 风险状态

风险项至少有：

- `open`
- `monitoring`
- `resolved`

## 2.5 事故状态

事故项至少有：

- `incident`
- `mitigating`
- `recovered`
- `postmortem`

## 2.6 交接状态

交接项至少有：

- `draft`
- `ready`
- `accepted`

## 2.7 简报状态

简报至少有：

- `draft`
- `published`

## 2.8 关键边界分支

### 分支 1：来源已进池，但还没核实

要求：

- 不允许直接进入 `approved`
- 必须至少有一条核验说明或风险说明

### 分支 2：有高风险但主编决定先发

要求：

- 必须保留风险说明
- 必须留一条“为何仍先发”的决策说明

### 分支 3：事故发生后恢复

要求：

- 不能只写 `recovered`
- 必须有至少一条 `mitigating` 过程

### 分支 4：交接完成

要求：

- 必须先有 `ready`
- 然后才允许 `accepted`

## 2.9 最小自动化建议

第一版程序可内建这些低风险自动化：

- `verified` 来源可自动出现在“待终审”面板
- `approved` 决策可自动进入简报候选
- `open` 风险可自动出现在首页提醒
- `incident` / `mitigating` 自动出现在事故面板顶部
- `ready` handoff 自动出现在交接面板顶部
