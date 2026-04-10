# HaoNews Team 模块优化建议

> 审阅日期：2026-04-04
> 审阅范围：`internal/haonews/team/` 包 + `internal/plugins/haonewsteam/` 插件 + `internal/haonews/team_sync.go` + `internal/haonews/pubsub.go`
> 代码总量：约 14,000 行 Go 代码 + 12 个 HTML 模板

---

## 一、架构层面优化

### 1.1 store.go 文件过大（3,462 行），职责过多

**现状**：`store.go` 单文件承载了所有数据模型定义、全部 CRUD 操作、文件锁、索引管理、归档、事件订阅、Webhook 推送、各种 normalize 函数等。

**问题**：可读性差，维护困难，不同关注点耦合在一起。

**优化方案**：按职责拆分为多个文件：

```
team/
  types.go           // 所有 struct 定义：Info, Member, Policy, Channel, Message, Task, Artifact, ChangeEvent, ArchiveSnapshot, TeamEvent, PushNotificationConfig, 各种 IndexEntry
  store.go           // Store 结构体、OpenStore、Root、withTeamLock（精简到 ~200 行）
  team_crud.go       // LoadTeam, ListTeams, SaveTeam
  member_crud.go     // LoadMembers, SaveMembers, LoadMembersSnapshot 及相关 normalize
  policy_crud.go     // LoadPolicy, SavePolicy, LoadPolicySnapshot, Policy.Allows 及相关
  channel_crud.go    // LoadChannel, SaveChannel, HideChannel, ListChannels, loadChannelConfigs, saveChannels
  message_crud.go    // AppendMessage, LoadMessages, LoadChannelMessages, LoadTaskMessages, LoadMessagesByContext, readLastJSONLLines, readAllJSONLLines, 分片相关
  task_crud.go       // AppendTask, LoadTasks, LoadTask, SaveTask, DeleteTask, LoadTasksByContext 及索引/遗留两套存储
  artifact_crud.go   // AppendArtifact, LoadArtifacts, LoadArtifact, SaveArtifact, DeleteArtifact 及索引/遗留两套存储
  history_crud.go    // AppendHistory, LoadHistory
  archive.go         // CreateManualArchive, ListArchives, LoadArchive, saveArchiveSnapshot
  index.go           // 索引相关：loadTaskIndex, saveTaskIndex, appendTaskIndexedLocked, rewriteTaskIndexLocked, CompactTasks, MigrateTasksToIndex 以及 Artifact 对应操作
  event.go           // Subscribe, publish, matchesEventFilter, sendWebhook, LoadWebhookConfigs, SaveWebhookConfigs
  normalize.go       // 所有 normalize 函数：NormalizeTeamID, normalizeChannelID, normalizeContextID, normalizeMemberRole, normalizeMemberStatus, normalizeTaskStatus, normalizeTaskPriority, normalizeArtifactKind, normalizeNonEmptyStrings, normalizeFieldDiffs 等
  helpers.go         // buildMessageID, buildTaskID, buildArtifactID, buildChangeEventID, sanitizeArchiveID, generateContextID, structuredDataContextID, taskIDMatches, reverseBytesToString 等
```

**具体操作步骤**：
1. 在 `team/` 目录下创建上述文件
2. 将对应函数和类型定义移动到各自文件（都保持 `package team`）
3. 确保 `go build ./...` 编译通过
4. 运行 `go test ./internal/haonews/team/...` 确保测试通过

---

### 1.2 handler.go 文件过大（1,300+ 行），handler 全部是顶层函数

**现状**：`handler.go` 中所有 handler 都是独立的顶层函数如 `handleTeamIndex(app, store, w, r)`，每次调用需要传入 `app` 和 `store`。

**问题**：参数重复传递、没有统一的中间件抽象。

**优化方案**：引入 handler 结构体统一持有依赖：

```go
// handler.go
type TeamHandler struct {
    app   *newsplugin.App
    store *teamcore.Store
}

func NewTeamHandler(app *newsplugin.App, store *teamcore.Store) *TeamHandler {
    return &TeamHandler{app: app, store: store}
}

func (h *TeamHandler) HandleTeamIndex(w http.ResponseWriter, r *http.Request) {
    // 原 handleTeamIndex 的逻辑，使用 h.app 和 h.store
}
```

同时按功能拆分文件：
```
haonewsteam/
  handler.go          // 路由注册 + TeamHandler 结构体
  handler_team.go     // handleTeamIndex, handleTeam
  handler_member.go   // handleTeamMembers, handleTeamMemberAction
  handler_channel.go  // handleTeamChannel, handleTeamChannelCreate, handleTeamChannelUpdate, handleTeamChannelHide
  handler_task.go     // handleTeamTasks, handleTeamTask, handleTeamTaskCreate, handleTeamTaskStatus, handleTeamTaskUpdate, handleTeamTaskDelete, handleTeamTaskComment
  handler_artifact.go // handleTeamArtifacts, handleTeamArtifact, handleTeamArtifactCreate, handleTeamArtifactUpdate, handleTeamArtifactDelete
  handler_archive.go  // handleTeamArchiveIndex, handleTeamArchive, handleTeamArchiveCreate
  handler_api.go      // 所有 JSON API handler
  handler_helpers.go  // filterMembers, filterTasks, formatTeamCount, teamRequestTrusted 等辅助函数
```

---

## 二、性能层面优化

### 2.1 `readLastJSONLLines` 逐字节反向读取（严重性能问题）

**现状**（store.go 第 841-879 行）：

```go
func readLastJSONLLines(path string, limit int) ([]string, error) {
    // ...
    for pos := info.Size() - 1; pos >= 0 && len(lines) < limit; pos-- {
        var b [1]byte
        if _, err := file.ReadAt(b[:], pos); err != nil {
            return nil, err
        }
        // 逐字节读取 + reverse
    }
}
```

**问题**：
- **每次读1字节触发一次系统调用**，对大文件（如数千条消息的频道 JSONL）性能极差
- `reverseBytesToString` 每次行结束都要分配新 byte 切片并反转
- 对于 UTF-8 多字节字符没有安全性考虑（虽然 JSON 是 ASCII 安全的，但可能出问题）

**优化方案**：使用块状反向读取（一次读 4KB-8KB 的块）：

```go
func readLastJSONLLines(path string, limit int) ([]string, error) {
    if limit <= 0 {
        return nil, nil
    }
    file, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer file.Close()
    info, err := file.Stat()
    if err != nil {
        return nil, err
    }
    size := info.Size()
    if size == 0 {
        return nil, nil
    }

    const blockSize = 8192
    lines := make([]string, 0, limit)
    remaining := make([]byte, 0, blockSize)

    for offset := size; offset > 0 && len(lines) < limit; {
        readSize := int64(blockSize)
        if readSize > offset {
            readSize = offset
        }
        offset -= readSize

        buf := make([]byte, readSize)
        if _, err := file.ReadAt(buf, offset); err != nil {
            return nil, err
        }

        // 将 remaining 追加到 buf 末尾处理
        buf = append(buf, remaining...)
        remaining = remaining[:0]

        // 从后往前扫描换行符
        for i := len(buf) - 1; i >= 0; i-- {
            if buf[i] == '\n' {
                line := strings.TrimSpace(string(buf[i+1:]))
                buf = buf[:i]
                if line != "" {
                    lines = append(lines, line)
                    if len(lines) >= limit {
                        break
                    }
                }
            }
        }
        remaining = append(remaining, buf...)
    }

    // 处理文件开头的最后一行
    if len(lines) < limit {
        if line := strings.TrimSpace(string(remaining)); line != "" {
            lines = append(lines, line)
        }
    }
    return lines, nil
}
```

**预期收益**：对 10MB 的 JSONL 文件，系统调用从数百万次降至约 1,200 次，速度提升 100x+。

---

### 2.2 `channelSummary` 加载全部消息只为统计数量

**现状**（store.go 第 3339-3361 行）：

```go
func (s *Store) channelSummary(teamID, channelID string) (ChannelSummary, error) {
    messages, err := s.LoadMessages(teamID, channelID, 0) // limit=0 表示加载全部！
    // ...
    summary := ChannelSummary{
        Channel:      channel,
        MessageCount: len(messages),  // 只用了 len()
    }
    for _, msg := range messages {
        if msg.CreatedAt.After(summary.LastMessageAt) {
            summary.LastMessageAt = msg.CreatedAt  // 只找最新时间
        }
    }
}
```

**问题**：为了获取消息数量和最新消息时间，加载了频道内所有消息到内存。如果一个频道有 10 万条消息，就要全部加载。

**优化方案**：新增轻量级的统计方法：

```go
// 方案A：只统计行数（不反序列化 JSON）
func (s *Store) channelMessageStats(teamID, channelID string) (count int, lastAt time.Time, err error) {
    if s.isShardedChannel(teamID, channelID) {
        return s.shardedChannelMessageStats(teamID, channelID)
    }
    path := s.channelPath(teamID, channelID)
    file, err := os.Open(path)
    if errors.Is(err, os.ErrNotExist) {
        return 0, time.Time{}, nil
    }
    if err != nil {
        return 0, time.Time{}, err
    }
    defer file.Close()

    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if line == "" {
            continue
        }
        count++
    }
    // 对最后几行反序列化获取 lastAt
    if count > 0 {
        lastLines, _ := readLastJSONLLines(path, 1)
        if len(lastLines) > 0 {
            var msg Message
            if json.Unmarshal([]byte(lastLines[0]), &msg) == nil {
                lastAt = msg.CreatedAt
            }
        }
    }
    return count, lastAt, scanner.Err()
}

// 方案B（更彻底）：在频道元数据中维护 message_count 和 last_message_at，每次 AppendMessage 时增量更新
```

**预期收益**：`ListChannels` 的时间从 O(全部消息数) 降到 O(行数扫描) 或 O(1)。

---

### 2.3 `LoadTaskMessages` 全量扫描所有频道所有消息

**现状**（store.go 第 1800-1845 行）：

```go
func (s *Store) LoadTaskMessages(teamID, taskID string, limit int) ([]Message, error) {
    channelSummaries, err := s.ListChannels(teamID)
    // ...
    for _, channelID := range channels {
        messages, err := s.LoadMessages(teamID, channelID, 0) // 加载每个频道的所有消息！
        for _, message := range messages {
            if taskIDMatches(message.StructuredData, taskID) {
                matched = append(matched, message)
            }
        }
    }
}
```

**问题**：为了找到与某个 Task 相关的消息，需要遍历所有频道的所有消息。如果有 5 个频道，每个频道 1 万条消息，就需要遍历 5 万条。

**优化方案**（选一即可）：

**方案A：消息索引增加 task_id 字段**
在 `AppendMessage` 时，如果 `StructuredData["task_id"]` 存在，维护一个按 task_id 索引的文件 `tasks_messages_index.json`，查询时直接命中。

**方案B：短期优化 - 基于 ContextID 预过滤**
Tasks 和 Messages 共享 ContextID，先通过 Task 的 ContextID 缩小搜索到单个频道：

```go
func (s *Store) LoadTaskMessages(teamID, taskID string, limit int) ([]Message, error) {
    task, err := s.LoadTask(teamID, taskID)
    if err != nil {
        return nil, err
    }
    // 如果 task 绑定了 channelID，优先只搜索该频道
    if task.ChannelID != "" {
        return s.loadTaskMessagesFromChannel(teamID, task.ChannelID, taskID, limit)
    }
    // fallback: 全量搜索（保留现有逻辑）
}
```

**同样的问题也存在于** `LoadMessagesByContext`（第 1847-1885 行），解决思路相同。

---

### 2.4 `ListTeams` 串行加载每个团队的信息

**现状**（store.go 第 256-299 行）：

```go
func (s *Store) ListTeams() ([]Summary, error) {
    entries, err := os.ReadDir(s.root)
    // ...
    for _, entry := range entries {
        info, err := s.LoadTeam(teamID)      // 串行读取 team.json
        members, err := s.LoadMembers(teamID) // 串行读取 members.json
        channels, err := s.ListChannels(teamID) // 串行读取 channels
        // ...
    }
}
```

**问题**：当团队数量增多时（如 50 个团队），串行 I/O 会很慢。

**优化方案**：并发加载：

```go
func (s *Store) ListTeams() ([]Summary, error) {
    entries, err := os.ReadDir(s.root)
    if err != nil {
        return nil, err
    }

    type result struct {
        summary Summary
        err     error
    }

    ch := make(chan result, len(entries))
    for _, entry := range entries {
        if !entry.IsDir() {
            continue
        }
        teamID := NormalizeTeamID(entry.Name())
        if teamID == "" {
            continue
        }
        go func(id string) {
            info, err := s.LoadTeam(id)
            if err != nil {
                ch <- result{err: err}
                return
            }
            members, _ := s.LoadMembers(id)
            channels, _ := s.ListChannels(id)
            channelCount := len(teamChannels(info))
            if len(channels) > 0 {
                channelCount = len(channels)
            }
            ch <- result{summary: Summary{
                Info:         info,
                MemberCount:  len(members),
                ChannelCount: channelCount,
            }}
        }(teamID)
    }
    // 收集结果...
}
```

---

### 2.5 `handleTeam` 详情页的并发加载优化良好，但错误处理可改进

**现状**（handler.go 第 55-206 行）：已经使用了 `sync.WaitGroup` + goroutine 并发加载 9 项数据。

**问题**：`errOnce` 只保留第一个错误，后续错误被丢弃；且即使有错误，其它 goroutine 仍然在运行不会提前取消。

**优化方案**：使用 `errgroup` 代替手动 WaitGroup：

```go
import "golang.org/x/sync/errgroup"

func handleTeam(app *newsplugin.App, store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
    info, err := store.LoadTeam(teamID)
    if err != nil {
        http.NotFound(w, r)
        return
    }

    g, ctx := errgroup.WithContext(r.Context())

    var members []teamcore.Member
    g.Go(func() error {
        items, err := store.LoadMembers(teamID)
        if err != nil {
            return err
        }
        members = items
        return nil
    })
    // ... 其它 goroutine 类似

    if err := g.Wait(); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
}
```

**好处**：带 context 取消支持，一个失败可提前取消其他（需要各方法支持 context）。

---

## 三、数据一致性与可靠性优化

### 3.1 文件写入不是原子操作（部分场景）

**现状**：`AppendMessage`、`AppendHistory` 直接使用 `os.O_APPEND` 写入 JSONL 文件：

```go
file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
// ...
file.Write(append(body, '\n'))
```

`saveTasks`、`saveArtifacts`、`saveChannels` 等使用了临时文件 + `os.Rename`（原子）。

**问题**：Append 操作如果在写入中途程序崩溃（如写了半行 JSON），会导致该 JSONL 文件出现不完整的行，后续 `json.Unmarshal` 会静默跳过（`continue`），导致数据丢失且无日志。

**优化方案**：

1. **写入前 fsync**：在 `file.Close()` 前调用 `file.Sync()` 确保数据落盘
2. **读取时记录错误**：将 `continue` 改为日志记录

```go
// 原代码（多处出现）
var msg Message
if err := json.Unmarshal([]byte(line), &msg); err != nil {
    continue  // 静默丢弃！
}

// 改为
var msg Message
if err := json.Unmarshal([]byte(line), &msg); err != nil {
    log.Printf("[team] corrupt JSONL line in %s (skipping): %v", path, err)
    continue
}
```

3. **大文件写入考虑 WAL**：对于频繁写入的 JSONL，考虑引入简单的 Write-Ahead-Log 机制，先写 WAL 再写数据文件。

---

### 3.2 `withTeamLock` 使用文件锁但无超时机制

**现状**（store.go 第 2542-2565 行）：

```go
func (s *Store) withTeamLock(teamID string, fn func() error) error {
    lockFile, err := os.OpenFile(s.teamLockPath(teamID), os.O_CREATE|os.O_RDWR, 0o644)
    // ...
    if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
        return err
    }
    defer func() {
        _ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
    }()
    return fn()
}
```

**问题**：
- `syscall.Flock` 是阻塞调用，无超时。如果持锁进程异常退出（但锁未释放），其他操作会永久阻塞
- 不支持 `context.Context`，无法在 HTTP 请求超时时取消
- 在 Windows 上不可用（`syscall.Flock` 是 Unix 专属）

**优化方案**：

```go
func (s *Store) withTeamLock(teamID string, fn func() error) error {
    // ... 创建 lockFile

    // 使用非阻塞尝试 + 重试 + 超时
    deadline := time.Now().Add(10 * time.Second)
    for {
        err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
        if err == nil {
            break
        }
        if !errors.Is(err, syscall.EWOULDBLOCK) {
            return fmt.Errorf("team lock failed: %w", err)
        }
        if time.Now().After(deadline) {
            return fmt.Errorf("team lock timeout for %q after 10s", teamID)
        }
        time.Sleep(50 * time.Millisecond)
    }
    defer func() {
        _ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
    }()
    return fn()
}
```

**进一步优化**：接受 `context.Context` 参数，监听 `ctx.Done()` 来提前退出。

---

### 3.3 Webhook 推送缺少重试和错误处理

**现状**（store.go 第 2101-2120 行）：

```go
func (s *Store) sendWebhook(cfg PushNotificationConfig, event TeamEvent) {
    body, err := json.Marshal(event)
    if err != nil {
        return // 静默丢弃
    }
    req, err := http.NewRequest(http.MethodPost, cfg.URL, bytes.NewReader(body))
    if err != nil {
        return // 静默丢弃
    }
    // ...
    client := &http.Client{Timeout: 5 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return // 静默丢弃
    }
    _ = resp.Body.Close()
}
```

**问题**：
- 所有错误静默丢弃，没有任何日志
- 没有重试机制，临时网络故障会导致事件丢失
- 没有检查 HTTP 响应状态码（如 500、429 等应重试）
- 全局共享的 `http.Client` 每次创建新实例（应复用）

**优化方案**：

```go
// 在 Store 中持有一个复用的 http.Client
type Store struct {
    root        string
    subMu       sync.RWMutex
    subscribers map[string]map[chan TeamEvent]struct{}
    webhookClient *http.Client  // 新增
}

func OpenStore(storeRoot string) (*Store, error) {
    // ...
    return &Store{
        root:          root,
        subscribers:   make(map[string]map[chan TeamEvent]struct{}),
        webhookClient: &http.Client{Timeout: 10 * time.Second},
    }, nil
}

func (s *Store) sendWebhook(cfg PushNotificationConfig, event TeamEvent) {
    body, err := json.Marshal(event)
    if err != nil {
        log.Printf("[team] webhook marshal error: %v", err)
        return
    }

    maxRetries := 3
    for attempt := 0; attempt < maxRetries; attempt++ {
        if attempt > 0 {
            time.Sleep(time.Duration(attempt) * 2 * time.Second) // 指数退避
        }

        req, err := http.NewRequest(http.MethodPost, cfg.URL, bytes.NewReader(body))
        if err != nil {
            log.Printf("[team] webhook request error: %v", err)
            return
        }
        req.Header.Set("Content-Type", "application/json")
        if token := strings.TrimSpace(cfg.Token); token != "" {
            req.Header.Set("Authorization", "Bearer "+token)
        }

        resp, err := s.webhookClient.Do(req)
        if err != nil {
            log.Printf("[team] webhook attempt %d failed: %v", attempt+1, err)
            continue
        }
        _ = resp.Body.Close()

        if resp.StatusCode >= 200 && resp.StatusCode < 300 {
            return // 成功
        }
        if resp.StatusCode == 429 || resp.StatusCode >= 500 {
            log.Printf("[team] webhook attempt %d got status %d, retrying", attempt+1, resp.StatusCode)
            continue
        }
        log.Printf("[team] webhook got non-retriable status %d", resp.StatusCode)
        return
    }
    log.Printf("[team] webhook exhausted retries for %s", cfg.URL)
}
```

---

## 四、代码质量与可维护性优化

### 4.1 大量重复的 nil 检查和 NormalizeTeamID 模板代码

**现状**：几乎每个 `Store` 方法都以相同的模板开头：

```go
func (s *Store) SomeMethod(teamID string, ...) (..., error) {
    if s == nil {
        return ..., errors.New("nil team store")
    }
    teamID = NormalizeTeamID(teamID)
    if teamID == "" {
        return ..., errors.New("empty team id")
    }
    // ...
}
```

这在 3,462 行文件中出现了约 **30 次**。

**优化方案**：提取公共验证方法：

```go
func (s *Store) validateTeamID(teamID string) (string, error) {
    if s == nil {
        return "", errors.New("nil team store")
    }
    teamID = NormalizeTeamID(teamID)
    if teamID == "" {
        return "", errors.New("empty team id")
    }
    return teamID, nil
}

// 使用
func (s *Store) LoadTeam(teamID string) (Info, error) {
    teamID, err := s.validateTeamID(teamID)
    if err != nil {
        return Info{}, err
    }
    // ...
}
```

---

### 4.2 遗留存储和索引存储的双轨并行

**现状**：Tasks 和 Artifacts 各有两套存储方式（Legacy JSONL vs Indexed），代码中到处都是：

```go
if s.hasTaskIndex(teamID) {
    return s.loadTasksFromIndex(teamID, limit)
}
return s.loadLegacyTasksWithLimit(teamID, limit)
```

这种分支在 `AppendTask`、`LoadTasks`、`LoadTask`、`SaveTask`、`DeleteTask` 以及对应的 Artifact 操作中反复出现（约 10 处）。

**问题**：增加了代码复杂度和测试负担，Legacy 和 Indexed 两套逻辑需要分别维护。

**优化方案**：

1. **短期**：定义 `TaskStorage` 接口，Legacy 和 Indexed 各实现一个：

```go
type TaskStorage interface {
    AppendTask(teamID string, task Task) error
    LoadTasks(teamID string, limit int) ([]Task, error)
    LoadTask(teamID, taskID string) (Task, error)
    SaveTask(teamID string, task Task, policy Policy) error
    DeleteTask(teamID, taskID string) error
}
```

2. **长期**：计划在某个版本统一迁移到 Indexed 存储，提供一次性迁移脚本，然后移除 Legacy 代码。

---

### 4.3 `SaveTask`（Legacy 模式）在锁内调用 `LoadTasks`

**现状**（store.go 第 1229-1308 行）：

```go
err := s.withTeamLock(teamID, func() error {
    // ...
    if s.hasTaskIndex(teamID) {
        return s.saveTaskIndexedLocked(teamID, task, policy)
    }
    tasks, err := s.LoadTasks(teamID, 0)  // LoadTasks 内部也会做 NormalizeTeamID、检查 hasTaskIndex 等
    // ...
})
```

**问题**：`LoadTasks` 是 public 方法，有自己的参数验证逻辑，在已经持有锁的上下文中被调用，虽然 `Flock` 是可重入的，但这是不好的设计。

**优化方案**：在锁内直接调用内部方法 `loadLegacyTasks`（已存在），而不是通过 public `LoadTasks`：

```go
// 将第 1237 行的
tasks, err := s.LoadTasks(teamID, 0)
// 改为
tasks, err := s.loadLegacyTasks(teamID)
```

同样的问题在 `DeleteTask`（第 1330 行）和 `SaveArtifact`（第 1703 行）也存在。

---

### 4.4 `mergeChannel` 中的 CreatedAt 逻辑有 bug

**现状**（store.go 第 2245-2268 行）：

```go
func mergeChannel(base, override Channel) Channel {
    // ...
    if base.CreatedAt.IsZero() {
        base.CreatedAt = override.CreatedAt
    }
    if override.CreatedAt.IsZero() {
        // keep existing created_at
    } else {
        base.CreatedAt = override.CreatedAt  // 这里又覆盖了！
    }
    // ...
}
```

**问题**：第一个 `if` 处理了 `base.CreatedAt` 为零的情况，但紧接着的 `if-else` 块在 `override.CreatedAt` 非零时又无条件覆盖了 `base.CreatedAt`。这意味着：
- 如果 base 有 CreatedAt 且 override 也有 CreatedAt，base 的值会被覆盖（语义不清 — CreatedAt 通常应该取最早的值）

**优化方案**：

```go
func mergeChannel(base, override Channel) Channel {
    base = normalizeChannel(base)
    override = normalizeChannel(override)
    if base.ChannelID == "" {
        base.ChannelID = override.ChannelID
    }
    if override.Title != "" {
        base.Title = override.Title
    }
    base.Description = override.Description
    base.Hidden = override.Hidden
    // CreatedAt：取最早的非零值
    if base.CreatedAt.IsZero() {
        base.CreatedAt = override.CreatedAt
    }
    // UpdatedAt：取最新的非零值
    if !override.UpdatedAt.IsZero() {
        base.UpdatedAt = override.UpdatedAt
    }
    return normalizeChannel(base)
}
```

---

### 4.5 `handleTeamTaskUpdate` 中 ClosedAt 处理与 Store 层重复且不一致

**现状**（handler.go 第 770-775 行）：

```go
if updated.Status == "done" && updated.ClosedAt.IsZero() {
    updated.ClosedAt = time.Now().UTC()
}
if updated.Status != "done" {
    updated.ClosedAt = time.Time{}
}
```

同时在 `store.go` 的 `SaveTask` 中也有类似逻辑（第 1278-1284 行）：

```go
if IsTerminalState(task.Status) {
    if task.ClosedAt.IsZero() {
        task.ClosedAt = task.UpdatedAt
    }
} else {
    task.ClosedAt = time.Time{}
}
```

**问题**：
- Handler 层只处理了 `"done"` 状态，而 Store 层使用了 `IsTerminalState`（包括 done、failed、cancelled、rejected）
- Handler 层的逻辑在 Store 层会被覆盖，属于冗余代码
- 两处逻辑略有不同，容易引起混淆

**优化方案**：移除 handler 层的 ClosedAt 处理逻辑，完全由 Store 层统一管理：

```go
// handler_task.go - 删除这两行
// if updated.Status == "done" && updated.ClosedAt.IsZero() { ... }
// if updated.Status != "done" { ... }
```

---

## 五、安全性优化

### 5.1 `teamRequestTrusted` 的 IP 校验可被绕过

**现状**：handler.go 中使用了 `teamRequestTrusted(r)` 做写操作的访问控制，检查请求是否来自本地或局域网。

**问题**：
- 如果在反向代理（如 nginx）后面，`r.RemoteAddr` 可能是代理的 IP，而非真实客户端 IP
- 可能没有检查 `X-Forwarded-For`、`X-Real-IP` 头（或者反过来，信任了不该信任的头）

**优化方案**：

```go
func teamRequestTrusted(r *http.Request) bool {
    // 1. 从 RemoteAddr 获取 IP（不信任 X-Forwarded-For 等 header）
    host, _, _ := net.SplitHostPort(r.RemoteAddr)
    ip, err := netip.ParseAddr(host)
    if err != nil {
        return false
    }

    // 2. 检查是否为回环地址
    if ip.IsLoopback() {
        return true
    }

    // 3. 检查是否为私有网段
    if ip.IsPrivate() {
        return true
    }

    return false
}
```

建议在生产环境增加更严格的认证机制（如 token-based auth），不仅仅依赖 IP 判断。

---

### 5.2 `sanitizeArchiveID` 和 `NormalizeTeamID` 缺少路径遍历防护

**现状**：

```go
func NormalizeTeamID(value string) string {
    value = strings.ToLower(strings.TrimSpace(value))
    value = strings.ReplaceAll(value, "/", "-")
    // ...
}
```

虽然 `/` 被替换为 `-`，但没有处理 `..`、`%2F` 等编码形式。

**优化方案**：添加路径安全检查：

```go
func NormalizeTeamID(value string) string {
    value = strings.ToLower(strings.TrimSpace(value))
    // URL 解码
    if decoded, err := url.PathUnescape(value); err == nil {
        value = decoded
    }
    value = strings.ReplaceAll(value, "/", "-")
    value = strings.ReplaceAll(value, "\\", "-")
    value = strings.ReplaceAll(value, "_", "-")
    value = strings.ReplaceAll(value, " ", "-")
    value = strings.ReplaceAll(value, "..", "")  // 防止路径遍历
    for strings.Contains(value, "--") {
        value = strings.ReplaceAll(value, "--", "-")
    }
    result := strings.Trim(value, "-.")
    // 最终安全检查
    if strings.Contains(result, "..") || filepath.IsAbs(result) {
        return ""
    }
    return result
}
```

---

## 六、接口设计优化

### 6.1 `LoadMessages` 的 limit=0 语义不明确

**现状**：`LoadMessages(teamID, channelID, 0)` 表示加载全部消息（无上限）。

**问题**：`0` 的语义不直观，容易误以为"不加载"。且在多处（channelSummary、CreateManualArchive、LoadTaskMessages 等）使用 limit=0 来全量加载，缺少防护。

**优化方案**：

```go
const (
    LoadAll     = -1  // 加载全部
    DefaultLoad = 50  // 默认加载数
)

func (s *Store) LoadMessages(teamID, channelID string, limit int) ([]Message, error) {
    if limit == 0 {
        limit = DefaultLoad
    }
    if limit == LoadAll {
        limit = 0  // 内部使用 0 表示无限
    }
    // ...
}
```

或者引入 `LoadAllMessages` 方法明确语义：

```go
func (s *Store) LoadAllMessages(teamID, channelID string) ([]Message, error) {
    return s.LoadMessages(teamID, channelID, 0)
}
```

---

### 6.2 缺少 `context.Context` 支持

**现状**：所有 Store 方法都没有 `context.Context` 参数。

**问题**：
- HTTP handler 取消时，底层 I/O 操作无法感知
- 无法传递请求级别的 deadline/timeout
- 无法做分布式追踪

**优化方案**（长期）：为所有 public 方法添加 context 参数：

```go
func (s *Store) LoadTeam(ctx context.Context, teamID string) (Info, error) { ... }
func (s *Store) LoadMembers(ctx context.Context, teamID string) ([]Member, error) { ... }
// ...
```

**短期过渡**：先为 `withTeamLock` 增加 context 支持：

```go
func (s *Store) withTeamLockCtx(ctx context.Context, teamID string, fn func() error) error {
    // 监听 ctx.Done() 取消锁等待
}
```

---

## 七、测试优化

### 7.1 `team_sync_test.go` 存在但 `store.go` 缺少单元测试

**现状**：`team_sync_test.go` 有约 1,065 行测试，但 `store.go` 没有对应的 `store_test.go`。

**问题**：Store 是最核心的数据层，缺少单元测试意味着：
- 索引存储/遗留存储的行为无法自动验证
- `readLastJSONLLines` 等关键函数没有边界测试
- 文件锁、并发访问场景未覆盖

**优化方案**：创建 `store_test.go`，至少覆盖：

```go
// store_test.go - 测试用例列表
func TestNormalizeTeamID(t *testing.T) { ... }
func TestOpenStore(t *testing.T) { ... }
func TestLoadTeam_NotFound(t *testing.T) { ... }
func TestLoadTeam_Valid(t *testing.T) { ... }
func TestListTeams_Empty(t *testing.T) { ... }
func TestListTeams_Multiple(t *testing.T) { ... }
func TestAppendMessage_Basic(t *testing.T) { ... }
func TestAppendMessage_EmptyContent(t *testing.T) { ... }
func TestAppendMessage_SignaturePolicy(t *testing.T) { ... }
func TestLoadMessages_Limit(t *testing.T) { ... }
func TestLoadMessages_Sharded(t *testing.T) { ... }
func TestReadLastJSONLLines_Small(t *testing.T) { ... }
func TestReadLastJSONLLines_Large(t *testing.T) { ... }
func TestReadLastJSONLLines_EmptyLines(t *testing.T) { ... }
func TestAppendTask_WithIndex(t *testing.T) { ... }
func TestAppendTask_Legacy(t *testing.T) { ... }
func TestSaveTask_StatusTransition(t *testing.T) { ... }
func TestSaveTask_InvalidTransition(t *testing.T) { ... }
func TestDeleteTask_NotFound(t *testing.T) { ... }
func TestCompactTasks(t *testing.T) { ... }
func TestMigrateTasksToIndex(t *testing.T) { ... }
func TestMergeChannel_CreatedAt(t *testing.T) { ... }
func TestPolicyAllows(t *testing.T) { ... }
func TestWebhookSend_Retry(t *testing.T) { ... }
func TestSubscribe_Publish(t *testing.T) { ... }
func TestConcurrentAccess(t *testing.T) { ... }
```

---

## 八、可观测性优化

### 8.1 完全缺少日志记录

**现状**：整个 `team/` 包没有引入任何日志库。错误要么返回、要么静默丢弃。

**问题**：
- JSON 反序列化失败时静默 `continue`（第 683、697、746、1159、1475 行等多处）
- Webhook 推送失败时无日志
- 文件操作异常时无法排查

**优化方案**：引入结构化日志（推荐 `log/slog`，Go 1.21+）：

```go
import "log/slog"

// 在 Store 中持有 logger
type Store struct {
    root        string
    logger      *slog.Logger
    // ...
}

func OpenStore(storeRoot string, opts ...StoreOption) (*Store, error) {
    s := &Store{
        root:   filepath.Join(strings.TrimSpace(storeRoot), "team"),
        logger: slog.Default().With("component", "team.store"),
    }
    // ...
}
```

**关键记录点**：
- JSON 反序列化失败（warn 级别）
- Webhook 推送失败（warn 级别）
- 文件锁超时（error 级别）
- 索引迁移/压缩操作（info 级别）
- 消息签名验证失败（warn 级别）

---

### 8.2 缺少 metrics 指标

**优化方案**：添加关键指标收集：

```go
// 可使用 prometheus/client_golang 或简单的 expvar
var (
    teamMessageCount   = expvar.NewMap("team_message_count")   // 按 teamID
    teamTaskCount      = expvar.NewMap("team_task_count")       // 按 teamID
    teamWebhookErrors  = expvar.NewInt("team_webhook_errors")
    teamLockWaitTime   = expvar.NewFloat("team_lock_wait_ms")
    teamSyncConflicts  = expvar.NewInt("team_sync_conflicts")
)
```

---

## 九、同步层优化

### 9.1 `TeamSyncMessage.Key()` 中重复的 `strings.ToLower(strings.TrimSpace(m.Type))` 调用

**现状**（sync.go 第 128-166 行）：`Key()` 方法在 switch 中对 `m.Type` 再次做 `ToLower+TrimSpace`，但 `Normalize()` 方法已经做过了。

**问题**：如果调用 Key() 前没有先 Normalize()，行为可能不一致。

**优化方案**：`Key()` 方法应假设已 normalized，或内部先调用 Normalize：

```go
func (m TeamSyncMessage) Key() string {
    // 确保使用 normalized 的值
    normalized := m.Normalize()
    switch normalized.Type {
    case TeamSyncTypeMessage:
        // ...
    }
}
```

---

### 9.2 同步冲突解决策略过于简单

**现状**：同步冲突检测基于 `UpdatedAt` 时间戳比较（local_newer vs remote_newer vs same_version_diverged）。

**问题**：时间戳可能在不同节点上不精确同步，导致 "last-write-wins" 语义在时钟偏移场景下不可靠。

**长期优化方案**：引入向量时钟或 Hybrid Logical Clock (HLC)：

```go
type HLC struct {
    PhysicalTime int64 `json:"pt"`  // wall clock
    LogicalTime  int32 `json:"lt"`  // logical counter
    NodeID       string `json:"nid"` // 节点标识
}
```

---

## 十、HTML 模板优化

### 10.1 模板数据结构过于扁平

**现状**（types.go）：

```go
type teamPageData struct {
    Project            string
    Version            string
    PageNav            []newsplugin.NavLink
    NodeStatus         newsplugin.NodeStatus
    Now                time.Time
    Team               teamcore.Info
    Policy             teamcore.Policy
    Members            []teamcore.Member
    ActiveMembers      []teamcore.Member
    PendingMembers     []teamcore.Member
    MutedMembers       []teamcore.Member
    RemovedMembers     []teamcore.Member
    Owners             []teamcore.Member
    Maintainers        []teamcore.Member
    Observers          []teamcore.Member
    Messages           []teamcore.Message
    Tasks              []teamcore.Task
    Channels           []teamcore.ChannelSummary
    Artifacts          []teamcore.Artifact
    History            []teamcore.ChangeEvent
    RecentConflicts    []corehaonews.TeamSyncConflictRecord
    TaskStatusCounts   map[string]int
    ArtifactKindCounts map[string]int
    SummaryStats       []newsplugin.SummaryStat
}
```

**问题**：30 个字段全部平铺，Members 按状态和角色预计算了 7 个切片。

**优化方案**：用嵌套结构组织：

```go
type teamPageData struct {
    Common     pageCommon   // Project, Version, PageNav, NodeStatus, Now
    Team       teamcore.Info
    Policy     teamcore.Policy
    Members    membersSummary
    Messages   []teamcore.Message
    Tasks      tasksSummary
    Channels   []teamcore.ChannelSummary
    Artifacts  artifactsSummary
    History    []teamcore.ChangeEvent
    Conflicts  []corehaonews.TeamSyncConflictRecord
    Stats      []newsplugin.SummaryStat
}

type pageCommon struct {
    Project    string
    Version    string
    PageNav    []newsplugin.NavLink
    NodeStatus newsplugin.NodeStatus
    Now        time.Time
}

type membersSummary struct {
    All        []teamcore.Member
    ByStatus   map[string][]teamcore.Member  // "active", "pending", "muted", "removed"
    ByRole     map[string][]teamcore.Member  // "owner", "maintainer", "observer", "member"
    StatusCounts map[string]int
    RoleCounts   map[string]int
}
```

---

## 十一、优化优先级建议

| 优先级 | 编号 | 优化项 | 预期收益 | 实施难度 |
|--------|------|--------|----------|----------|
| **P0** | 2.1 | readLastJSONLLines 性能优化 | 100x+ 速度提升 | 低 |
| **P0** | 2.2 | channelSummary 全量加载优化 | 大幅降低内存 | 低 |
| **P0** | 3.1 | JSONL 写入健壮性 + 错误日志 | 数据可靠性 | 低 |
| **P0** | 4.4 | mergeChannel CreatedAt bug 修复 | 正确性 | 低 |
| **P0** | 4.5 | ClosedAt 逻辑去重 | 代码一致性 | 低 |
| **P1** | 2.3 | LoadTaskMessages 全量扫描优化 | 性能提升 | 中 |
| **P1** | 3.2 | 文件锁超时机制 | 可靠性 | 中 |
| **P1** | 3.3 | Webhook 重试 + 日志 | 可靠性 | 低 |
| **P1** | 5.2 | 路径遍历防护 | 安全性 | 低 |
| **P1** | 8.1 | 引入日志记录 | 可观测性 | 中 |
| **P2** | 1.1 | store.go 拆分 | 可维护性 | 中 |
| **P2** | 1.2 | handler.go 拆分 + 结构体化 | 可维护性 | 中 |
| **P2** | 4.1 | 公共验证方法提取 | 代码简洁 | 低 |
| **P2** | 4.2 | Legacy/Indexed 双轨存储接口化 | 可维护性 | 高 |
| **P2** | 4.3 | 锁内调用内部方法 | 正确性 | 低 |
| **P2** | 2.4 | ListTeams 并发加载 | 性能 | 低 |
| **P2** | 2.5 | errgroup 替换 WaitGroup | 错误处理 | 低 |
| **P3** | 6.1 | limit=0 语义明确化 | API 清晰度 | 低 |
| **P3** | 6.2 | context.Context 支持 | 长期架构 | 高 |
| **P3** | 7.1 | store_test.go 单元测试 | 质量保障 | 高 |
| **P3** | 9.1 | Key() 方法规范化 | 一致性 | 低 |
| **P3** | 9.2 | HLC 替代时间戳冲突检测 | 分布式正确性 | 高 |
| **P3** | 10.1 | 模板数据结构重构 | 可维护性 | 中 |
| **P3** | 8.2 | Metrics 指标 | 可观测性 | 中 |

---

## 十二、快速修复清单（可立即执行）

以下修复可以在不改变整体架构的情况下快速完成：

1. **store.go 第 2245-2268 行**：修复 `mergeChannel` 中 `CreatedAt` 的重复覆盖 bug
2. **handler.go 第 770-775 行**：删除 handler 层的 `ClosedAt` 处理（Store 层已处理）
3. **store.go 所有 `json.Unmarshal` + `continue` 处**：添加 `log.Printf` 记录错误
4. **store.go 第 1237 行**：`SaveTask` Legacy 模式下将 `s.LoadTasks` 改为 `s.loadLegacyTasks`
5. **store.go 第 1330 行**：`DeleteTask` Legacy 模式下同上
6. **store.go 第 1703 行**：`SaveArtifact` Legacy 模式下将 `s.LoadArtifacts` 改为 `s.loadLegacyArtifacts`
7. **store.go 第 2101-2120 行**：`sendWebhook` 添加日志记录和 HTTP 状态码检查

---

## 附录：文件清单

| 文件路径 | 行数 | 主要职责 |
|---------|------|---------|
| `internal/haonews/team/store.go` | 3,462 | 核心数据存储层 |
| `internal/haonews/team/sync.go` | 928 | 同步消息类型与冲突检测 |
| `internal/haonews/team/task_state.go` | 115 | 任务状态机 |
| `internal/haonews/team/agent_card.go` | 248 | Agent 能力注册 |
| `internal/haonews/team/message_signature.go` | 163 | 消息签名验证 |
| `internal/haonews/team_sync.go` | 1,946 | PubSub 同步运行时 |
| `internal/haonews/team_sync_test.go` | 1,065 | 同步测试 |
| `internal/haonews/pubsub.go` | 1,037 | PubSub 基础设施 |
| `internal/haonews/sync.go` | 1,964 | 通用同步编排 |
| `internal/plugins/haonewsteam/handler.go` | 1,300+ | HTTP Handler |
| `internal/plugins/haonewsteam/plugin.go` | 124 | 插件入口 |
| `internal/plugins/haonewsteam/types.go` | 201 | 页面数据结构 |
| `internal/plugins/haonewsteam/a2a_bridge.go` | 277 | A2A 协议桥接 |
