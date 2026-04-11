# HaoNews Team 模块工程优化代码

> 目标：让代码结构对 LLM（大模型）更友好，使其可以直接理解意图、安全地扩展功能、并自主完成新功能的实现。
> 
> 原则：
> 1. **意图显式化**：接口、类型、函数名传达"为什么"而不只是"是什么"
> 2. **边界清晰**：每个包/文件职责单一，LLM 不需要读完整个 store.go 才能干活
> 3. **可测试性**：提供 fixtures 和 builder，LLM 生成测试时有明确的起点
> 4. **错误语义化**：用类型化错误替代字符串错误，LLM 可以精确处理各类失败
> 5. **扩展点显式声明**：Hook / Middleware 接口让 LLM 知道"可以在哪里插代码"

---

## 1. 类型化错误（替换字符串错误）

**现状问题**：`errors.New("empty team id")` 这类错误导致调用方只能字符串匹配，LLM 也无法系统性地处理。

**优化：新增 `internal/haonews/team/errors.go`**

```go
package team

import "fmt"

// TeamError 是所有 team 包错误的基础类型。
// LLM 扩展新错误时，直接在此文件中追加 New* 函数即可。
type TeamError struct {
    Code    ErrorCode
    Context string // 附加上下文，如 teamID、agentID
    Err     error  // 原始错误（可选）
}

func (e *TeamError) Error() string {
    if e.Err != nil {
        return fmt.Sprintf("[%s] %s: %v", e.Code, e.Context, e.Err)
    }
    return fmt.Sprintf("[%s] %s", e.Code, e.Context)
}

func (e *TeamError) Unwrap() error { return e.Err }

// Is 支持 errors.Is(err, ErrNotFound) 形式的判断
func (e *TeamError) Is(target error) bool {
    t, ok := target.(*TeamError)
    if !ok {
        return false
    }
    return e.Code == t.Code
}

type ErrorCode string

const (
    ErrCodeNotFound      ErrorCode = "NOT_FOUND"
    ErrCodeEmptyID       ErrorCode = "EMPTY_ID"
    ErrCodeForbidden     ErrorCode = "FORBIDDEN"
    ErrCodeInvalidState  ErrorCode = "INVALID_STATE"
    ErrCodeConflict      ErrorCode = "CONFLICT"
    ErrCodeNilStore      ErrorCode = "NIL_STORE"
)

// 哨兵错误，供 errors.Is 比较
var (
    ErrNotFound     = &TeamError{Code: ErrCodeNotFound}
    ErrEmptyID      = &TeamError{Code: ErrCodeEmptyID}
    ErrForbidden    = &TeamError{Code: ErrCodeForbidden}
    ErrInvalidState = &TeamError{Code: ErrCodeInvalidState}
    ErrConflict     = &TeamError{Code: ErrCodeConflict}
)

func NewNotFoundError(subject string) error {
    return &TeamError{Code: ErrCodeNotFound, Context: subject}
}

func NewForbiddenError(action, agentID string) error {
    return &TeamError{Code: ErrCodeForbidden, Context: fmt.Sprintf("action=%s agent=%s", action, agentID)}
}

func NewInvalidTransitionError(from, to string) error {
    return &TeamError{Code: ErrCodeInvalidState, Context: fmt.Sprintf("from=%s to=%s", from, to)}
}
```

**调用方改写示例**（原 store.go 中的 nil 检查）：

```go
// 改写前
if s == nil {
    return nil, errors.New("nil team store")
}

// 改写后
if s == nil {
    return nil, &TeamError{Code: ErrCodeNilStore, Context: "Store"}
}
```

---

## 2. 权限检查接口化（PolicyEnforcer）

**现状问题**：`requireTeamAction(store, teamID, actorID, action)` 是一个散落在各 handler 里的函数调用，LLM 扩展新操作时需要知道"在哪里加权限检查"，且无法在测试中轻松 mock。

**优化：新增 `internal/haonews/team/enforcer.go`**

```go
package team

import "context"

// PolicyEnforcer 是权限检查的核心接口。
// 所有需要鉴权的操作都通过这个接口，而非直接读取 Policy 结构体。
// LLM 实现新的权限策略时，只需实现此接口。
type PolicyEnforcer interface {
    // Allow 检查指定 agent 是否可以执行 action。
    // action 格式：domain.verb，如 "task.create"、"member.update"、"policy.update"
    Allow(ctx context.Context, teamID, agentID, action string) error
}

// Action 常量集中定义，LLM 可以快速查找所有可用动作。
const (
    ActionMessageSend       = "message.send"
    ActionTaskCreate        = "task.create"
    ActionTaskUpdate        = "task.update"
    ActionTaskTransition    = "task.transition"
    ActionArtifactCreate    = "artifact.create"
    ActionArtifactUpdate    = "artifact.update"
    ActionMemberInvite      = "member.invite"
    ActionMemberUpdate      = "member.update"
    ActionMemberRemove      = "member.remove"
    ActionChannelCreate     = "channel.create"
    ActionChannelUpdate     = "channel.update"
    ActionPolicyUpdate      = "policy.update"
    ActionSyncConflictResolve = "sync.conflict.resolve"
    ActionArchiveCreate     = "archive.create"
    ActionAgentCardRegister = "agent_card.register"
)

// StorePolicyEnforcer 是基于 Store 的默认实现。
type StorePolicyEnforcer struct {
    store *Store
}

func NewPolicyEnforcer(store *Store) PolicyEnforcer {
    return &StorePolicyEnforcer{store: store}
}

func (e *StorePolicyEnforcer) Allow(ctx context.Context, teamID, agentID, action string) error {
    agentID = strings.TrimSpace(agentID)
    if agentID == "" {
        return NewForbiddenError(action, "(empty)")
    }
    policy, err := e.store.LoadPolicyCtx(ctx, teamID)
    if err != nil {
        return err
    }
    members, err := e.store.LoadMembersCtx(ctx, teamID)
    if err != nil {
        return err
    }
    role := findMemberRole(members, agentID)
    if !policyAllowsAction(policy, role, action) {
        return NewForbiddenError(action, agentID)
    }
    return nil
}

// findMemberRole 返回指定 agent 的角色，未找到返回空字符串。
func findMemberRole(members []Member, agentID string) string {
    for _, m := range members {
        if m.AgentID == agentID && m.Status == MemberStatusActive {
            return m.Role
        }
    }
    return ""
}

// policyAllowsAction 判断给定角色是否允许执行 action。
func policyAllowsAction(policy Policy, role, action string) bool {
    if role == MemberRoleOwner {
        return true // owner 拥有所有权限
    }
    // 检查 Permissions map（精确匹配）
    if roles, ok := policy.Permissions[action]; ok {
        for _, r := range roles {
            if r == role {
                return true
            }
        }
        return false
    }
    // 回退到粗粒度的角色检查
    switch action {
    case ActionMessageSend:
        return containsRole(policy.MessageRoles, role)
    case ActionTaskCreate, ActionTaskUpdate, ActionTaskTransition:
        return containsRole(policy.TaskRoles, role)
    case ActionPolicyUpdate, ActionMemberUpdate, ActionMemberRemove:
        return role == MemberRoleMaintainer
    default:
        return containsRole(policy.SystemNoteRoles, role)
    }
}
```

---

## 3. Task 状态机钩子（TaskLifecycleHook）

**现状问题**：任务状态转换的副作用（发通知、触发 Agent 派发、更新统计）都混在 handler 函数里，新增副作用需要修改多处代码。

**优化：新增 `internal/haonews/team/task_lifecycle.go`**

```go
package team

import "context"

// TaskTransitionEvent 是任务状态发生变化时传递给 Hook 的数据。
// LLM 扩展新的 Hook 时，所有需要的信息都在这个结构体里。
type TaskTransitionEvent struct {
    TeamID    string
    Task      Task   // 更新后的 Task
    FromState string // 变更前的状态
    ToState   string // 变更后的状态
    ActorID   string // 触发变更的 agent
}

// TaskLifecycleHook 是任务生命周期的扩展点接口。
// 实现此接口来响应任务状态变化，无需修改核心状态机代码。
//
// 已有实现（可参考）：
//   - NotificationHook：@mention 相关成员
//   - DispatchHook：自动派发任务给匹配的 Agent
//   - StatsHook：更新成员贡献统计
type TaskLifecycleHook interface {
    // OnTransition 在任务状态发生合法转换后调用（异步，不影响主流程）。
    OnTransition(ctx context.Context, event TaskTransitionEvent)
}

// TaskLifecycleHookFunc 是函数式实现，方便快速注册 Hook。
type TaskLifecycleHookFunc func(ctx context.Context, event TaskTransitionEvent)

func (f TaskLifecycleHookFunc) OnTransition(ctx context.Context, event TaskTransitionEvent) {
    f(ctx, event)
}

// HookRegistry 管理所有注册的 Hook。
// LLM 注册新 Hook 时，只需调用 Register 方法，无需修改任何现有代码。
type HookRegistry struct {
    hooks []TaskLifecycleHook
}

func (r *HookRegistry) Register(hooks ...TaskLifecycleHook) {
    r.hooks = append(r.hooks, hooks...)
}

// Fire 触发所有已注册 Hook（并发执行，互不阻塞）。
func (r *HookRegistry) Fire(ctx context.Context, event TaskTransitionEvent) {
    for _, h := range r.hooks {
        h := h
        go h.OnTransition(ctx, event)
    }
}

// ---- 示例 Hook 实现 ----

// LogTransitionHook 打印状态变更日志，供调试使用。
type LogTransitionHook struct{}

func (h *LogTransitionHook) OnTransition(_ context.Context, event TaskTransitionEvent) {
    // 记录：task {TaskID} {FromState} → {ToState} by {ActorID}
    _ = event // 替换为实际日志调用
}
```

**在 Store 里集成**：

```go
// Store 增加字段
type Store struct {
    // ... 已有字段
    TaskHooks *HookRegistry
}

// UpdateTaskStatusCtx 改写时触发 Hook
func (s *Store) UpdateTaskStatusCtx(ctx context.Context, teamID, taskID, toStatus, actorID string) (Task, error) {
    // ... 原有逻辑
    before := task.Status
    task.Status = toStatus
    if err := s.saveTaskNoCtx(teamID, task); err != nil {
        return Task{}, err
    }
    // 触发钩子（非阻塞）
    if s.TaskHooks != nil {
        s.TaskHooks.Fire(ctx, TaskTransitionEvent{
            TeamID: teamID, Task: task,
            FromState: before, ToState: toStatus, ActorID: actorID,
        })
    }
    return task, nil
}
```

---

## 4. Store 拆分：读写接口分离

**现状问题**：`store.go` 已超过千行，包含数据模型、文件读写、锁逻辑、webhook 发送。LLM 处理这个文件时上下文消耗极大且容易出错。

**优化：将 Store 按职责拆分为多个文件，并暴露明确的接口**

**新增 `internal/haonews/team/interfaces.go`**

```go
package team

import "context"

// TeamReader 是只读视图，供不需要写权限的组件使用（如 API handler、搜索）。
// LLM 实现只读功能时，依赖此接口即可，无需引入完整 Store。
type TeamReader interface {
    LoadTeamCtx(ctx context.Context, teamID string) (Info, error)
    LoadMembersCtx(ctx context.Context, teamID string) ([]Member, error)
    LoadPolicyCtx(ctx context.Context, teamID string) (Policy, error)
    LoadChannelCtx(ctx context.Context, teamID, channelID string) (Channel, error)
    ListChannelsCtx(ctx context.Context, teamID string) ([]Channel, error)
    ListTasksCtx(ctx context.Context, teamID string, filter TaskFilter) ([]Task, error)
    LoadTaskCtx(ctx context.Context, teamID, taskID string) (Task, error)
    ListArtifactsCtx(ctx context.Context, teamID string, filter ArtifactFilter) ([]Artifact, error)
    ListMessagesCtx(ctx context.Context, teamID, channelID string, limit int) ([]Message, error)
    ListAgentCardsCtx(ctx context.Context, teamID string) ([]AgentCard, error)
}

// TeamWriter 是写操作接口。
// LLM 实现写功能时，明确知道哪些方法会改变状态。
type TeamWriter interface {
    SaveTeamCtx(ctx context.Context, info Info) error
    SaveMembersCtx(ctx context.Context, teamID string, members []Member) error
    SavePolicyCtx(ctx context.Context, teamID string, policy Policy) error
    AppendMessageCtx(ctx context.Context, msg Message) error
    SaveTaskCtx(ctx context.Context, teamID string, task Task) error
    SaveArtifactCtx(ctx context.Context, teamID string, artifact Artifact) error
    AppendHistoryCtx(ctx context.Context, event ChangeEvent) error
}

// TeamStore 是完整读写接口，Store 实现此接口。
type TeamStore interface {
    TeamReader
    TeamWriter
}
```

**文件拆分建议**（现有 store.go 按此拆分，每文件 ≤ 200 行）：

| 文件 | 职责 |
|------|------|
| `store.go` | Store 结构体、OpenStore、锁工具函数 |
| `store_team.go` | Team Info 的读写 |
| `store_member.go` | Member 的读写、filterMembers |
| `store_policy.go` | Policy 的读写 |
| `store_channel.go` | Channel + ChannelConfig 的读写 |
| `store_message.go` | Message 的 JSONL 读写、索引 |
| `store_task.go` | Task 的 JSONL 读写、状态更新 |
| `store_artifact.go` | Artifact 的 JSONL 读写 |
| `store_history.go` | ChangeEvent 的 JSONL 追加、查询 |
| `store_webhook.go` | Webhook 配置 + 投递记录 |
| `store_agent.go` | AgentCard 的读写（已有 agent_card.go） |

---

## 5. 测试 Fixtures 与 Builder

**现状问题**：没有测试辅助代码，LLM 生成测试时需要手写所有测试数据，容易出现 ID 不合法、时间戳为零等问题。

**优化：新增 `internal/haonews/team/testfixtures/fixtures.go`**

```go
// Package testfixtures 提供 team 包的测试辅助数据构建器。
// LLM 写测试时，优先使用这里的 Builder，而不是直接 struct literal。
package testfixtures

import (
    "time"
    team "hao.news/internal/haonews/team"
)

// TeamBuilder 构建测试用 Team Info。
type TeamBuilder struct {
    info team.Info
}

func NewTeam(id string) *TeamBuilder {
    return &TeamBuilder{info: team.Info{
        TeamID:    id,
        Slug:      id,
        Title:     "Test Team " + id,
        Visibility: "team",
        CreatedAt: time.Now().UTC(),
        UpdatedAt: time.Now().UTC(),
    }}
}

func (b *TeamBuilder) WithOwner(agentID string) *TeamBuilder {
    b.info.OwnerAgentID = agentID
    return b
}

func (b *TeamBuilder) Build() team.Info { return b.info }

// MemberBuilder 构建测试用 Member。
type MemberBuilder struct {
    member team.Member
}

func NewMember(agentID string) *MemberBuilder {
    return &MemberBuilder{member: team.Member{
        AgentID:  agentID,
        Role:     team.MemberRoleMember,
        Status:   team.MemberStatusActive,
        JoinedAt: time.Now().UTC(),
    }}
}

func (b *MemberBuilder) WithRole(role string) *MemberBuilder {
    b.member.Role = role
    return b
}

func (b *MemberBuilder) WithStatus(status string) *MemberBuilder {
    b.member.Status = status
    return b
}

func (b *MemberBuilder) Build() team.Member { return b.member }

// TaskBuilder 构建测试用 Task。
type TaskBuilder struct {
    task team.Task
}

func NewTask(id, teamID, title string) *TaskBuilder {
    return &TaskBuilder{task: team.Task{
        TaskID:    id,
        TeamID:    teamID,
        Title:     title,
        Status:    team.TaskStateOpen,
        Priority:  "medium",
        CreatedAt: time.Now().UTC(),
        UpdatedAt: time.Now().UTC(),
    }}
}

func (b *TaskBuilder) WithStatus(status string) *TaskBuilder {
    b.task.Status = status
    return b
}

func (b *TaskBuilder) WithAssignees(agents ...string) *TaskBuilder {
    b.task.Assignees = agents
    return b
}

func (b *TaskBuilder) Build() team.Task { return b.task }

// FullTeamScenario 返回一个"标准场景"：1 个 owner + 2 个 member + 3 个任务，
// 覆盖最常见的测试用例起点。
type FullTeamScenario struct {
    Team    team.Info
    Members []team.Member
    Tasks   []team.Task
    Policy  team.Policy
}

func StandardTeamScenario(teamID string) FullTeamScenario {
    return FullTeamScenario{
        Team: NewTeam(teamID).WithOwner("agent-owner").Build(),
        Members: []team.Member{
            NewMember("agent-owner").WithRole(team.MemberRoleOwner).Build(),
            NewMember("agent-alice").WithRole(team.MemberRoleMember).Build(),
            NewMember("agent-bob").WithRole(team.MemberRoleMember).Build(),
        },
        Tasks: []team.Task{
            NewTask("task-1", teamID, "设计 API 方案").WithAssignees("agent-alice").Build(),
            NewTask("task-2", teamID, "实现核心逻辑").WithStatus(team.TaskStateDoing).WithAssignees("agent-bob").Build(),
            NewTask("task-3", teamID, "Code Review").WithStatus(team.TaskStateReview).Build(),
        },
        Policy: team.DefaultPolicy(),
    }
}
```

---

## 6. 成员角色/状态常量集中化

**现状问题**：`"owner"`、`"active"` 等字符串散落在代码中，LLM 容易拼错，也无法利用 IDE 补全。

**优化：在 `normalize.go` 或新文件 `constants.go` 中暴露常量**

```go
package team

// 成员角色常量
const (
    MemberRoleOwner      = "owner"
    MemberRoleMaintainer = "maintainer"
    MemberRoleMember     = "member"
    MemberRoleObserver   = "observer"
)

// 成员状态常量
const (
    MemberStatusActive  = "active"
    MemberStatusPending = "pending"
    MemberStatusMuted   = "muted"
    MemberStatusRemoved = "removed"
)

// 任务优先级常量
const (
    TaskPriorityLow    = "low"
    TaskPriorityMedium = "medium"
    TaskPriorityHigh   = "high"
)

// Artifact 类型常量
const (
    ArtifactKindMarkdown    = "markdown"
    ArtifactKindJSON        = "json"
    ArtifactKindLink        = "link"
    ArtifactKindPost        = "post"
    ArtifactKindSkillDoc    = "skill-doc"
    ArtifactKindPlanSummary = "plan-summary"
    ArtifactKindReview      = "review-summary"
)

// DefaultPolicy 返回开箱即用的默认策略，测试和模板初始化时使用。
func DefaultPolicy() Policy {
    return Policy{
        MessageRoles:    []string{MemberRoleOwner, MemberRoleMaintainer, MemberRoleMember},
        TaskRoles:       []string{MemberRoleOwner, MemberRoleMaintainer, MemberRoleMember},
        SystemNoteRoles: []string{MemberRoleOwner, MemberRoleMaintainer},
    }
}
```

---

## 7. 频道上下文快照接口（ChannelContextProvider）

**目标**：让 LLM Agent 调用一个接口就能拿到"我需要做决策的全部信息"，不需要调用 5 个 API。

**新增 `internal/haonews/team/context_provider.go`**

```go
package team

import "context"

// ChannelContext 是频道的完整状态快照，专为 LLM Agent 设计。
// 字段命名和注释尽量使用自然语言，便于 LLM 直接理解并生成提示词。
type ChannelContext struct {
    // 团队基本信息
    Team Info `json:"team"`
    // 当前频道信息
    Channel Channel `json:"channel"`
    // 频道的 Agent 引导 prompt（由频道管理员配置）
    AgentOnboarding string `json:"agent_onboarding,omitempty"`
    // 当前活跃中的任务（status: open / doing / review）
    ActiveTasks []Task `json:"active_tasks,omitempty"`
    // 最近的消息记录（最多 20 条，按时间倒序）
    RecentMessages []Message `json:"recent_messages,omitempty"`
    // 当前活跃成员列表
    ActiveMembers []Member `json:"active_members,omitempty"`
    // 当前团队策略（角色权限概述）
    Policy Policy `json:"policy"`
    // 上下文生成时间
    SnapshotAt string `json:"snapshot_at"`
}

// ChannelContextProvider 提供频道上下文快照的能力接口。
// 默认实现由 StoreChannelContextProvider 提供。
// LLM 若需要扩展（如注入外部数据），可实现此接口的装饰器版本。
type ChannelContextProvider interface {
    GetChannelContext(ctx context.Context, teamID, channelID string) (ChannelContext, error)
}

// StoreChannelContextProvider 是基于 Store 的默认实现。
type StoreChannelContextProvider struct {
    store *Store
}

func NewChannelContextProvider(store *Store) ChannelContextProvider {
    return &StoreChannelContextProvider{store: store}
}

func (p *StoreChannelContextProvider) GetChannelContext(ctx context.Context, teamID, channelID string) (ChannelContext, error) {
    team, err := p.store.LoadTeamCtx(ctx, teamID)
    if err != nil {
        return ChannelContext{}, err
    }
    channel, err := p.store.LoadChannelCtx(ctx, teamID, channelID)
    if err != nil {
        return ChannelContext{}, err
    }
    channelCfg, _ := p.store.LoadChannelConfigCtx(ctx, teamID, channelID)
    members, _ := p.store.LoadMembersCtx(ctx, teamID)
    activeMembers := filterMembersByStatus(members, MemberStatusActive)
    policy, _ := p.store.LoadPolicyCtx(ctx, teamID)
    tasks, _ := p.store.ListTasksCtx(ctx, teamID, TaskFilter{
        Statuses: []string{TaskStateOpen, TaskStateDoing, TaskStateReview},
    })
    messages, _ := p.store.ListMessagesCtx(ctx, teamID, channelID, 20)

    return ChannelContext{
        Team:            team,
        Channel:         channel,
        AgentOnboarding: channelCfg.AgentOnboarding,
        ActiveTasks:     tasks,
        RecentMessages:  reverseMessages(messages),
        ActiveMembers:   activeMembers,
        Policy:          policy,
        SnapshotAt:      nowUTCString(),
    }, nil
}
```

---

## 8. 结构化 Filter 类型（替代散乱的 query 参数）

**现状问题**：`ListTasksCtx` 等函数参数列表不固定，LLM 调用时容易遗漏参数或传错顺序。

**优化：统一使用 Filter 结构体**

```go
package team

import "time"

// TaskFilter 是任务列表查询的所有可选过滤条件。
// LLM 构造查询时，只需填写关心的字段，零值字段自动忽略。
type TaskFilter struct {
    // 按状态过滤，空表示不限制
    Statuses []string `json:"statuses,omitempty"`
    // 按指派 agent 过滤
    AssigneeAgentID string `json:"assignee_agent_id,omitempty"`
    // 按创建者过滤
    CreatedByAgentID string `json:"created_by_agent_id,omitempty"`
    // 按优先级过滤
    Priorities []string `json:"priorities,omitempty"`
    // 按标签过滤（任意匹配）
    Labels []string `json:"labels,omitempty"`
    // 按频道过滤
    ChannelID string `json:"channel_id,omitempty"`
    // 截止时间范围
    DueAfter  time.Time `json:"due_after,omitempty"`
    DueBefore time.Time `json:"due_before,omitempty"`
    // 分页
    Limit  int `json:"limit,omitempty"`
    Offset int `json:"offset,omitempty"`
}

// ArtifactFilter 是 Artifact 列表查询的过滤条件。
type ArtifactFilter struct {
    Kinds     []string `json:"kinds,omitempty"`
    TaskID    string   `json:"task_id,omitempty"`
    ChannelID string   `json:"channel_id,omitempty"`
    Labels    []string `json:"labels,omitempty"`
    CreatedBy string   `json:"created_by,omitempty"`
    Limit     int      `json:"limit,omitempty"`
    Offset    int      `json:"offset,omitempty"`
}

// MessageFilter 是消息列表查询的过滤条件。
type MessageFilter struct {
    ChannelID   string    `json:"channel_id,omitempty"`
    ContextID   string    `json:"context_id,omitempty"`
    AuthorID    string    `json:"author_id,omitempty"`
    MessageType string    `json:"message_type,omitempty"`
    After       time.Time `json:"after,omitempty"`
    Limit       int       `json:"limit,omitempty"`
    Offset      int       `json:"offset,omitempty"`
}
```

---

## 9. 同步层的类型安全升级

**现状问题**：`TeamSyncMessage` 是一个大的 Union 类型，所有字段都是可选的，LLM 处理时无法静态知道"这个 type 应该有哪些非空字段"。

**优化：增加类型断言辅助方法 + 验证函数**

```go
package team

import "fmt"

// Validate 验证 TeamSyncMessage 的结构完整性，
// 确保每种 type 的必填字段不为空。
// LLM 构造 sync 消息时，调用此方法可以在发送前发现问题。
func (m TeamSyncMessage) Validate() error {
    m = m.Normalize()
    if m.TeamID == "" {
        return fmt.Errorf("sync message missing team_id")
    }
    switch m.Type {
    case TeamSyncTypeMessage:
        if m.Message == nil {
            return fmt.Errorf("sync type=%s requires Message field", m.Type)
        }
        if m.Message.MessageID == "" || m.Message.ChannelID == "" {
            return fmt.Errorf("sync message missing message_id or channel_id")
        }
    case TeamSyncTypeTask:
        if m.Task == nil || m.Task.TaskID == "" {
            return fmt.Errorf("sync type=%s requires Task with task_id", m.Type)
        }
    case TeamSyncTypeArtifact:
        if m.Artifact == nil || m.Artifact.ArtifactID == "" {
            return fmt.Errorf("sync type=%s requires Artifact with artifact_id", m.Type)
        }
    case TeamSyncTypeMember:
        if len(m.Members) == 0 {
            return fmt.Errorf("sync type=%s requires non-empty Members", m.Type)
        }
    case TeamSyncTypePolicy:
        if m.Policy == nil {
            return fmt.Errorf("sync type=%s requires Policy field", m.Type)
        }
    case TeamSyncTypeChannel:
        if m.Channel == nil || m.Channel.ChannelID == "" {
            return fmt.Errorf("sync type=%s requires Channel with channel_id", m.Type)
        }
    case TeamSyncTypeHistory:
        if m.History == nil || m.History.EventID == "" {
            return fmt.Errorf("sync type=%s requires History with event_id", m.Type)
        }
    case TeamSyncTypeAck:
        if m.Ack == nil || m.Ack.AckedKey == "" {
            return fmt.Errorf("sync type=%s requires Ack with acked_key", m.Type)
        }
    default:
        return fmt.Errorf("unknown sync type: %q", m.Type)
    }
    return nil
}

// NewMessageSync / NewTaskSync 等工厂函数，
// 让 LLM 构造 sync 消息时有明确的起点，避免漏填字段。
func NewMessageSyncMsg(teamID, sourceNode string, msg Message) TeamSyncMessage {
    return TeamSyncMessage{
        Type:       TeamSyncTypeMessage,
        TeamID:     teamID,
        Message:    &msg,
        SourceNode: sourceNode,
    }.Normalize()
}

func NewTaskSyncMsg(teamID, sourceNode string, task Task) TeamSyncMessage {
    return TeamSyncMessage{
        Type:       TeamSyncTypeTask,
        TeamID:     teamID,
        Task:       &task,
        SourceNode: sourceNode,
    }.Normalize()
}

func NewMemberSyncMsg(teamID, sourceNode string, members []Member) TeamSyncMessage {
    return TeamSyncMessage{
        Type:       TeamSyncTypeMember,
        TeamID:     teamID,
        Members:    members,
        SourceNode: sourceNode,
    }.Normalize()
}
```

---

## 10. LLM 友好的代码注释规范

对于 LLM 作为代码维护者，以下注释规范可以显著提升自动修改的准确性：

```go
// --- 规范示例 ---

// SaveTaskCtx 持久化一个 Task。
// 如果 task.TaskID 已存在，则覆盖更新；否则新建。
// 调用方应确保：
//   - task.TeamID 已设置且合法
//   - 状态转换已通过 IsValidTransitionWithPolicy 验证
//   - 已调用 PolicyEnforcer.Allow(ActionTaskUpdate) 完成权限检查
//
// 副作用：触发 TeamEvent{Kind:"task", Action:"save"}，Webhook 和 SSE 订阅者会收到通知。
//
// LLM 扩展提示：若需要在保存前做额外校验（如检查 DependsOn 的任务是否存在），
// 注册 TaskLifecycleHook.OnBeforeSave（待实现）而非修改此函数。
func (s *Store) SaveTaskCtx(ctx context.Context, teamID string, task Task) error {
    // ...
}
```

规范要点：
- 每个 `Save*` / `Load*` / `List*` 函数都说明**调用前置条件**
- 说明**副作用**（事件发布、文件写入、锁）
- 提供**LLM 扩展提示**：告诉 LLM 应该在哪里插入新逻辑，而不是修改现有函数体
- 接口文件（interfaces.go）中每个方法都有完整的 godoc，包含使用示例

---

## 汇总：改造优先级

| 优先级 | 改造项 | 预计影响行数 | LLM 收益 |
|--------|--------|-------------|----------|
| P0 | 常量集中化（第 6 节） | ~50 行新增 | 消除魔法字符串，极大减少 LLM 拼写错误 |
| P0 | 类型化错误（第 1 节） | ~80 行新增 | LLM 可精确处理各类错误分支 |
| P0 | Store 文件拆分（第 4 节） | 重构，不增加代码量 | 每次 LLM 上下文窗口只需加载 1 个文件 |
| P1 | PolicyEnforcer 接口（第 2 节） | ~120 行新增 | 权限检查可 mock，LLM 生成测试更容易 |
| P1 | Filter 结构体（第 8 节） | ~60 行新增 | LLM 构造查询时有明确类型约束 |
| P1 | TaskLifecycleHook（第 3 节） | ~100 行新增 | LLM 扩展副作用时有明确插件点 |
| P2 | 测试 Fixtures（第 5 节） | ~150 行新增 | LLM 生成测试时有完整起点 |
| P2 | ChannelContextProvider（第 7 节） | ~100 行新增 | LLM Agent 一次调用拿到完整上下文 |
| P2 | Sync 消息验证（第 9 节） | ~80 行新增 | LLM 构造 sync 消息时有静态验证 |
| P3 | 注释规范（第 10 节） | 零代码，约定 | 长期收益，减少 LLM 误改核心逻辑 |
