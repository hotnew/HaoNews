# HaoNews Team 模块优化方案

> 分析日期：2026-04-10
> 分析范围：`internal/haonews/team/` + `internal/plugins/haonewsteam/`

---

## 一、现状评估

HaoNews 的 Team 模块已经建立了一套相对完整的协作基础设施：四级角色体系（owner / maintainer / member / observer）、细粒度权限策略（Policy.Permissions）、完整审计链（ChangeEvent）、多节点 P2P 同步（TeamSyncMessage）、6 个专项 Room 插件、以及 AgentCard 能力注册机制。

但对照"以 AI Agent 为核心成员的协作平台"这个目标，现有设计在以下几个方向还有明显的提升空间。

---

## 二、优化方向

### 方向 1：Agent 编排与自动派发（最高优先级）

**现状**  
`AgentCard` + `MatchAgentsForTask` 已能通过 label 匹配到候选 Agent，但匹配结果仅用于展示，系统不会自动将任务派发给 Agent，也没有 Agent 执行反馈的闭环。

**问题**  
- Agent 匹配仅靠 tag 字符串，语义对齐弱，容易漏配或误配
- 没有任务队列 / 竞价机制，多个 Agent 可能抢同一任务
- 没有执行超时、重试、降级的编排策略
- Agent 执行结果与 Task 状态机未打通

**优化建议**

1. **引入 `TaskDispatch` 结构体**，记录派发决策：
   ```
   TaskDispatch {
     TaskID, AssignedAgentID, MatchReason string
     DispatchedAt, AckedAt, CompletedAt time.Time
     Status: queued | acked | running | done | timeout | failed
     RetryCount int
     TimeoutSeconds int
   }
   ```

2. **增加 Agent 健康/负载上报**（在 AgentCard 里扩展 `LiveStatus`），包括当前队列长度、最近响应时间，让系统做负载均衡分配。

3. **任务状态机扩展**：在 `open → doing` 的路径上插入 `dispatched` 中间态，确保 Agent 真正 Ack 后才推进。

4. **派发策略可配置**（Policy 级别）：`dispatch_mode: manual | auto_single | auto_compete`

---

### 方向 2：Thread（消息线索）与任务的深度绑定

**现状**  
`Message.ContextID` 和 `Task.ContextID` 字段已存在，意图是把消息归属到某个对话上下文，但目前后端没有对 Context 建独立索引，UI 也没有呈现线索视图。

**问题**  
- 一个 Task 可能涉及多轮讨论，但消息散落在频道流里找不到
- AI Agent 回复时缺少上下文注入点（不知道当前 Task 的历史讨论）
- 无法从 Task 跳转到完整的讨论线索

**优化建议**

1. 为 `ContextID` 建立独立的 `Thread` 索引文件：`/team/{id}/threads/{contextID}.jsonl`

2. API 增加 `GET /api/teams/{teamID}/tasks/{taskID}/thread`，聚合该 Task 下所有相关消息

3. 在 ChannelConfig 的 `AgentOnboarding` 字段里注入 Thread 摘要，让 LLM Agent 接管任务时拿到完整上下文

4. `Message` 增加可选的 `ParentMessageID`，支持二级回复（不强制层级树，但支持 1 层 reply）

---

### 方向 3：成员贡献分析与活跃度感知

**现状**  
审计日志（ChangeEvent）记录了所有操作，但只有原始流水，没有聚合统计。系统无法回答"这个 Agent 这周做了什么"。

**问题**  
- 团队 Owner 无法快速了解成员活跃情况
- 无法识别"实际没有参与"的僵尸成员
- Agent 的能力利用率无法衡量

**优化建议**

1. 引入轻量 `MemberStats` 结构（可按天懒计算，存到 `/team/{id}/stats/`）：
   ```
   MemberStats {
     AgentID string
     Period  string  // "2026-04-10"
     MessageCount, TaskCreated, TaskClosed, ArtifactCount int
     LastActiveAt time.Time
   }
   ```

2. Team 成员页增加"贡献热力图"视图（按周/月汇总）

3. Policy 可配置 `inactive_days_threshold`，超过阈值自动将成员状态置为 `muted`，Owner 审核后再恢复

---

### 方向 4：Team 模板与快速启动

**现状**  
创建团队只能从空白状态开始，需要手动添加频道、配置 Policy、邀请成员。

**问题**  
- 对于常见场景（如 incident 响应团队、code review 团队、项目管理团队），每次从零配置成本高
- 缺少 AI Agent 配置的预设（哪些 Agent 默认加入、默认分配什么角色）

**优化建议**

1. 引入 `TeamTemplate` 结构：
   ```
   TeamTemplate {
     TemplateID, Name, Description string
     DefaultPolicy  Policy
     DefaultChannels []ChannelTemplate  // 含 Plugin 配置
     DefaultAgents  []AgentRole         // AgentID → 默认角色
   }
   ```

2. 内置 3 个模板：
   - `incident-response`：incident room + 值班 Agent + escalation policy
   - `code-review`：review room + CI Agent + 自动分配 reviewer
   - `planning`：plan-exchange room + 决策 room + 项目追踪 Agent

3. API 支持 `POST /api/teams?from_template={templateID}`

---

### 方向 5：任务依赖与里程碑

**现状**  
Task 是扁平结构，没有父子关系或依赖链。

**问题**  
- 复杂项目无法拆解为子任务
- 无法表达"Task B 依赖 Task A 完成"的前置条件
- 没有里程碑节点，无法做进度追踪

**优化建议**

1. Task 增加字段：
   ```
   ParentTaskID  string    // 父任务 ID（可选）
   DependsOn     []string  // 前置任务 ID 列表
   MilestoneID   string    // 所属里程碑
   ```

2. 状态机扩展：当 `DependsOn` 中有任务未完成，不允许将本 Task 从 `open` 推进到 `doing`（可配置，通过 Policy.TaskTransitions 的前置检查钩子）

3. 新增 `Milestone` 实体，聚合一批 Task，计算整体进度百分比

---

### 方向 6：频道级 AI 上下文注入（对 LLM Agent 友好）

**现状**  
`ChannelConfig.AgentOnboarding` 字段存在但使用场景不清晰，也没有标准化的 context 组装逻辑。

**问题**  
- LLM Agent 进入频道时拿不到结构化的"当前团队状态"
- Agent 需要自行从 API 拼 context，容易遗漏关键信息

**优化建议**

1. 定义标准的 `ChannelContext` 结构，由系统自动组装：
   ```
   ChannelContext {
     Team         Info
     Channel      Channel
     ActiveTasks  []Task         // status in [open, doing, review]
     RecentMessages []Message    // 最近 20 条
     Members      []Member       // active 成员
     Policy       Policy
     AgentOnboarding string      // 频道自定义 prompt
   }
   ```

2. API 增加 `GET /api/teams/{teamID}/channels/{channelID}/context`，以 Markdown 或 JSON 格式返回，专门供 LLM Agent 调用

3. Webhook 事件里增加 `context_snapshot` 字段，Push 给订阅的 Agent

---

### 方向 7：冲突解决自动化

**现状**  
P2P 同步的冲突（`TeamSyncConflict`）需要人工通过 `/teams/{id}/sync` 页面手动解决。

**问题**  
- 多 Agent 高频写入时，冲突可能积压
- 简单冲突（如时间戳更新同一字段）完全可以自动合并

**优化建议**

1. 对每类 Scope 定义合并策略：
   - `message`：总是追加，无需解决
   - `task.status`：取更晚时间戳的版本（last-write-wins）
   - `member`：以本地为准，除非有 owner/maintainer 签名的远端变更
   - `policy`：需要人工确认

2. `TeamSyncConflict` 增加 `AutoResolvable bool` 字段，系统在同步时优先自动解决可自动处理的冲突，只留人工队列给真正有歧义的情况

---

### 方向 8：通知中心与 Agent 唤醒机制

**现状**  
Webhook 支持推送事件到外部 URL，但没有内置的通知中心，也没有"@mention → 唤醒特定 Agent"的机制。

**问题**  
- 人类成员无法在 UI 里看到面向自己的通知摘要
- Agent 需要轮询 API 或依赖 Webhook，缺少精准唤醒

**优化建议**

1. 引入 `Notification` 实体：
   ```
   Notification {
     ID, TeamID, ChannelID string
     TargetAgentID   string
     Kind            string  // mention | task_assigned | task_blocked | review_needed
     ReferenceType   string  // message | task | artifact
     ReferenceID     string
     Read            bool
     CreatedAt       time.Time
   }
   ```

2. Message 解析时识别 `@agentID` 语法，自动生成 mention 通知

3. SSE（Server-Sent Events）端点 `GET /api/teams/{teamID}/notifications/stream`，供 Agent 长连接监听，替代轮询

---

## 三、优先级排序

| 优先级 | 方向 | 理由 |
|--------|------|------|
| P0 | Agent 编排与自动派发 | 核心差异化能力，直接影响 AI Agent 工作流 |
| P0 | 频道级 AI 上下文注入 | 提升每个 Agent 的"上下文质量" |
| P1 | Thread 与任务深度绑定 | 解决信息碎片化，Agent 决策质量依赖此 |
| P1 | 通知中心与唤醒机制 | 减少 Agent 轮询，提升响应速度 |
| P2 | 任务依赖与里程碑 | 支持复杂项目，增加平台粘性 |
| P2 | 成员贡献分析 | 可观测性，运营价值 |
| P3 | Team 模板 | 降低使用门槛，加速团队创建 |
| P3 | 冲突解决自动化 | 减少运维负担，在高并发场景下更重要 |

---

## 四、与现有架构的兼容性

所有建议均基于现有数据模型扩展（新增字段而非重构），存储层沿用 JSONL + 文件锁模式，不引入新的外部依赖。AgentCard 和 Policy 的扩展向后兼容——缺省值即当前行为。P2P 同步层只需在 `TeamSyncMessage` 中增加新 type 常量（如 `TeamSyncTypeDispatch`、`TeamSyncTypeNotification`）即可。
