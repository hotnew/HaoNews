# 20260405-team-pro1-code.md — 工程实施文档

## 前置条件

- 已阅读 `20260405-team-pro1.md`（架构设计文档）
- 仓库根目录：`haonews/`
- 语言：Go 1.26.0，模块名 `hao.news`
- 所有改动完成后必须通过 `go build ./...` 和 `go test ./...`
- 不改动 P2P 同步层（`internal/haonews/team/sync.go`、`internal/haonews/team_sync.go`）
- 不引入外部数据库，不引入新的序列化格式，保持 JSON / JSONL

## 关键 import 路径

```go
import (
    "hao.news/internal/apphost"                        // PluginManifest, Config, WebTheme, Site, HTTPPlugin
    teamcore "hao.news/internal/haonews/team"           // Store, Info, Channel, Message, Task, Artifact...
    newsplugin "hao.news/internal/plugins/haonews"      // App, NavItem, NodeStatus, SummaryStat
)
```

---

## 文件变更总表

| 序号 | 操作 | 路径 | Phase |
|------|------|------|-------|
| 1 | 新建 | `internal/plugins/haonewsteam/roomplugin/registry.go` | P1 |
| 2 | 修改 | `internal/haonews/team/store.go` | P1 |
| 3 | 新建 | `internal/haonews/team/channel_config.go` | P1 |
| 4 | 修改 | `internal/haonews/team/paths.go` | P1 |
| 5 | 修改 | `internal/haonews/team/ctx_api.go` | P1 |
| 6 | 修改 | `internal/haonews/team/compat_api.go` | P1 |
| 7 | 修改 | `internal/plugins/haonewsteam/plugin.go` | P1 |
| 8 | 修改 | `internal/plugins/haonewsteam/handler_channel.go` | P1 |
| 9 | 修改 | `internal/plugins/haonewsteam/types.go` | P1 |
| 10 | 新建 | `internal/plugins/haonewsteam/rooms/planexchange/plugin.go` | P2 |
| 11 | 新建 | `internal/plugins/haonewsteam/rooms/planexchange/handler.go` | P2 |
| 12 | 新建 | `internal/plugins/haonewsteam/rooms/planexchange/types.go` | P2 |
| 13 | 修改 | `internal/plugins/haonewsteam/haonews.plugin.json` | P2 |
| 14 | 新建 | `internal/plugins/haonewsteam/rooms/planexchange/roomplugin.json` | P2 |
| 15 | 新建 | `internal/themes/room-themes/minimal/roomtheme.json` | P3 |
| 16 | 新建 | `internal/themes/room-themes/minimal/web/templates/room_channel.html` | P3 |
| 17 | 新建 | `internal/plugins/haonews/web/templates/room_channel_default.html` | P3 |
| 18 | 新建 | `internal/haonews/team/channel_config_test.go` | P4 |
| 19 | 新建 | `internal/plugins/haonewsteam/roomplugin/registry_test.go` | P4 |

---

## Phase 1 — Room Plugin 框架搭建

### Step 1.1 — 新建 `roomplugin/registry.go`

**路径**：`internal/plugins/haonewsteam/roomplugin/registry.go`

**说明**：定义 Room Plugin 接口和注册表。Room Plugin 是挂载在 Team Channel 级别的二级插件，Team 主干通过 Registry 分发请求。

```go
package roomplugin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"

	teamcore "hao.news/internal/haonews/team"
)

// RoomPlugin 是所有 Room Plugin 必须实现的接口。
// 每个实现对应一种可挂载到 Team Channel 的业务逻辑。
type RoomPlugin interface {
	// ID 返回插件唯一标识，必须与 roomPlugin.json 中 id 字段一致。
	// 格式要求：小写字母 + 连字符，例如 "plan-exchange"。
	ID() string

	// Manifest 返回插件声明，用于 Channel Config 匹配和前端展示。
	Manifest() Manifest

	// Handler 返回此插件处理 HTTP 请求的 Handler。
	// Team 主干在路由分发时调用此方法。
	// 参数：
	//   store  — 可读写的 Team Store 实例（只通过 Ctx 方法操作）
	//   teamID — 当前 Team ID
	// 路由挂载后，请求路径已去除前缀：
	//   Web:  /teams/{teamID}/r/{pluginID}/...  → Handler 收到 /...
	//   API:  /api/teams/{teamID}/r/{pluginID}/... → Handler 收到 /...
	Handler(store *teamcore.Store, teamID string) http.Handler
}

// Manifest 是 Room Plugin 的静态声明信息。
type Manifest struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	Description   string   `json:"description,omitempty"`
	MessageKinds  []string `json:"messageKinds,omitempty"`
	ArtifactKinds []string `json:"artifactKinds,omitempty"`
}

// LoadManifestJSON 从 JSON 字节加载 Manifest。
func LoadManifestJSON(data []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("roomplugin: invalid manifest json: %w", err)
	}
	if m.ID == "" {
		return Manifest{}, fmt.Errorf("roomplugin: manifest missing id")
	}
	return m, nil
}

// LoadManifestFile 从文件路径加载 Manifest。
func LoadManifestFile(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	return LoadManifestJSON(data)
}

// Registry 管理所有已注册的 Room Plugin 实例。
// 线程安全，可在 Build 阶段并发注册。
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]RoomPlugin
}

// NewRegistry 创建一个空的 Room Plugin 注册表。
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]RoomPlugin),
	}
}

// Register 注册一个 Room Plugin。
// 如果 ID 重复则返回 error，不会覆盖。
func (r *Registry) Register(p RoomPlugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := p.ID()
	if id == "" {
		return fmt.Errorf("roomplugin: empty plugin id")
	}
	if _, exists := r.plugins[id]; exists {
		return fmt.Errorf("roomplugin: duplicate plugin id %q", id)
	}
	r.plugins[id] = p
	return nil
}

// MustRegister 注册一个 Room Plugin，失败则 panic。
func (r *Registry) MustRegister(p RoomPlugin) {
	if err := r.Register(p); err != nil {
		panic(err)
	}
}

// Get 按 ID 获取已注册的 Room Plugin。
func (r *Registry) Get(id string) (RoomPlugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[id]
	return p, ok
}

// All 返回所有已注册的 Room Plugin。
func (r *Registry) All() []RoomPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]RoomPlugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		out = append(out, p)
	}
	return out
}

// IDs 返回所有已注册的 Room Plugin ID。
func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.plugins))
	for id := range r.plugins {
		out = append(out, id)
	}
	return out
}

// Manifests 返回所有已注册 Room Plugin 的 Manifest。
func (r *Registry) Manifests() []Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Manifest, 0, len(r.plugins))
	for _, p := range r.plugins {
		out = append(out, p.Manifest())
	}
	return out
}
```

---

### Step 1.2 — 新建 `channel_config.go`（Channel 自描述配置）

**路径**：`internal/haonews/team/channel_config.go`

**说明**：为 Channel 增加 Room Plugin / Room Theme / Agent Onboarding 配置支持。数据存储为独立 JSON 文件，与现有 `channels.json` 并行，不修改现有 Channel 结构。

```go
package team

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ChannelConfig 是 Channel 级别的插件/主题/引导配置。
// 存储在：{StoreRoot}/team/{teamID}/channel-configs/{channelID}.json
// 此文件独立于 channels.json，缺失时所有字段取零值（即默认行为）。
type ChannelConfig struct {
	ChannelID       string            `json:"channel_id"`
	Plugin          string            `json:"plugin,omitempty"`           // Room Plugin ID@version，例如 "plan-exchange@1.0"
	Theme           string            `json:"theme,omitempty"`            // Room Theme ID，例如 "minimal"
	ThemeConfig     map[string]any    `json:"theme_config,omitempty"`     // 主题参数
	AgentOnboarding string            `json:"agent_onboarding,omitempty"` // 给 Agent 的自然语言引导
	Rules           []string          `json:"rules,omitempty"`            // 频道规则（自然语言列表）
	Metadata        map[string]string `json:"metadata,omitempty"`         // 自定义 kv 元数据
	CreatedAt       time.Time         `json:"created_at,omitempty"`
	UpdatedAt       time.Time         `json:"updated_at,omitempty"`
}

// PluginID 从 Plugin 字段提取不含版本的 ID 部分。
// 例如 "plan-exchange@1.0" → "plan-exchange"。
// 若无版本号，则直接返回原值。
func (c ChannelConfig) PluginID() string {
	for i, ch := range c.Plugin {
		if ch == '@' {
			return c.Plugin[:i]
		}
	}
	return c.Plugin
}

// channelConfigDir 返回某个 team 的 channel-configs 目录。
func (s *Store) channelConfigDir(teamID string) string {
	return filepath.Join(s.root, NormalizeTeamID(teamID), "channel-configs")
}

// channelConfigPath 返回某个 channel 的 config 文件路径。
func (s *Store) channelConfigPath(teamID, channelID string) string {
	return filepath.Join(s.channelConfigDir(teamID), normalizeChannelID(channelID)+".json")
}

// loadChannelConfigNoCtx 读取 Channel Config，文件不存在时返回零值而非 error。
func (s *Store) loadChannelConfigNoCtx(teamID, channelID string) (ChannelConfig, error) {
	if s == nil {
		return ChannelConfig{}, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	channelID = normalizeChannelID(channelID)
	if teamID == "" || channelID == "" {
		return ChannelConfig{}, fmt.Errorf("empty team_id or channel_id")
	}
	data, err := os.ReadFile(s.channelConfigPath(teamID, channelID))
	if errors.Is(err, os.ErrNotExist) {
		return ChannelConfig{ChannelID: channelID}, nil // 文件不存在 → 零值，不报错
	}
	if err != nil {
		return ChannelConfig{}, err
	}
	var cfg ChannelConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ChannelConfig{}, fmt.Errorf("invalid channel config json for %s/%s: %w", teamID, channelID, err)
	}
	cfg.ChannelID = channelID
	return cfg, nil
}

// saveChannelConfigNoCtx 保存 Channel Config。
func (s *Store) saveChannelConfigNoCtx(teamID string, cfg ChannelConfig) error {
	if s == nil {
		return errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	cfg.ChannelID = normalizeChannelID(cfg.ChannelID)
	if teamID == "" || cfg.ChannelID == "" {
		return fmt.Errorf("empty team_id or channel_id")
	}
	now := time.Now().UTC()
	if cfg.CreatedAt.IsZero() {
		// 尝试保留原 CreatedAt
		existing, _ := s.loadChannelConfigNoCtx(teamID, cfg.ChannelID)
		if !existing.CreatedAt.IsZero() {
			cfg.CreatedAt = existing.CreatedAt
		} else {
			cfg.CreatedAt = now
		}
	}
	cfg.UpdatedAt = now
	dir := s.channelConfigDir(teamID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.channelConfigPath(teamID, cfg.ChannelID), data, 0o644)
}

// listChannelConfigsNoCtx 列出某个 Team 下所有 Channel Config。
// 只列已有配置文件的 Channel（没有配置的 Channel 使用默认值）。
func (s *Store) listChannelConfigsNoCtx(teamID string) ([]ChannelConfig, error) {
	if s == nil {
		return nil, errors.New("nil team store")
	}
	teamID = NormalizeTeamID(teamID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	dir := s.channelConfigDir(teamID)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var configs []ChannelConfig
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) < 6 || name[len(name)-5:] != ".json" {
			continue
		}
		channelID := name[:len(name)-5]
		cfg, err := s.loadChannelConfigNoCtx(teamID, channelID)
		if err != nil {
			continue
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}
```

---

### Step 1.3 — 修改 `paths.go`

**路径**：`internal/haonews/team/paths.go`

**改动说明**：无需修改。`channelConfigDir` 和 `channelConfigPath` 已定义在 `channel_config.go` 中（与 Store 绑定），这是 Go 允许的——同 package 不同文件可直接访问。

---

### Step 1.4 — 修改 `ctx_api.go`

**路径**：`internal/haonews/team/ctx_api.go`

**改动方式**：在文件末尾追加以下方法（不修改已有代码）：

```go
// === Channel Config Ctx API ===

func (s *Store) LoadChannelConfigCtx(ctx context.Context, teamID, channelID string) (ChannelConfig, error) {
	if err := ctxErr(ctx); err != nil {
		return ChannelConfig{}, err
	}
	return s.loadChannelConfigNoCtx(teamID, channelID)
}

func (s *Store) SaveChannelConfigCtx(ctx context.Context, teamID string, cfg ChannelConfig) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	return s.saveChannelConfigNoCtx(teamID, cfg)
}

func (s *Store) ListChannelConfigsCtx(ctx context.Context, teamID string) ([]ChannelConfig, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	return s.listChannelConfigsNoCtx(teamID)
}
```

---

### Step 1.5 — 修改 `compat_api.go`

**路径**：`internal/haonews/team/compat_api.go`

**改动方式**：在文件末尾追加以下方法：

```go
func (s *Store) LoadChannelConfig(teamID, channelID string) (ChannelConfig, error) {
	return s.LoadChannelConfigCtx(context.Background(), teamID, channelID)
}

func (s *Store) SaveChannelConfig(teamID string, cfg ChannelConfig) error {
	return s.SaveChannelConfigCtx(context.Background(), teamID, cfg)
}

func (s *Store) ListChannelConfigs(teamID string) ([]ChannelConfig, error) {
	return s.ListChannelConfigsCtx(context.Background(), teamID)
}
```

---

### Step 1.6 — 修改 `plugin.go`（Room Plugin Registry 集成）

**路径**：`internal/plugins/haonewsteam/plugin.go`

**改动 1**：在 import 中增加 Room Plugin 包：

```go
import (
    // ... 保留现有 import ...
    "hao.news/internal/plugins/haonewsteam/roomplugin"
)
```

**改动 2**：修改 `Build()` 函数，在创建 handler 之前初始化 Registry 并注册内置插件：

找到现有代码（约第 52-59 行）：
```go
	if !strings.HasSuffix(filepathBase(os.Args[0]), ".test") {
		startTeamWorkspaceWarmup(ctx, app, store)
	}
	return &apphost.Site{
		Manifest: Plugin{}.Manifest(),
		Theme:    theme.Manifest(),
		Handler:  newHandler(app, store, staticFS),
	}, nil
```

替换为：
```go
	// --- Room Plugin Registry ---
	registry := roomplugin.NewRegistry()
	// 注册内置 Room Plugins（Phase 2 加入 plan-exchange）
	// registry.MustRegister(planexchange.New())

	if !strings.HasSuffix(filepathBase(os.Args[0]), ".test") {
		startTeamWorkspaceWarmup(ctx, app, store)
	}
	return &apphost.Site{
		Manifest: Plugin{}.Manifest(),
		Theme:    theme.Manifest(),
		Handler:  newHandler(app, store, staticFS, registry),
	}, nil
```

**改动 3**：修改 `newHandler` 函数签名和函数体。

找到现有签名（第 125 行）：
```go
func newHandler(app *newsplugin.App, store *teamcore.Store, staticFS fs.FS) http.Handler {
```

替换为：
```go
func newHandler(app *newsplugin.App, store *teamcore.Store, staticFS fs.FS, roomRegistry *roomplugin.Registry) http.Handler {
```

在 `newHandler` 函数体的 `/teams/` 路由分发中（现有代码第 314-332 行之间），找到：
```go
		if len(parts) == 3 && parts[1] == "channels" {
			channelID := normalizeTeamChannel(parts[2])
			if channelID == "" {
				http.NotFound(w, r)
				return
			}
			handleTeamChannel(app, store, teamID, channelID, w, r)
			return
		}
```

**在此段之前**插入 Room Plugin 路由分发：
```go
		// Room Plugin Web 路由：/teams/{teamID}/r/{pluginID}/...
		if len(parts) >= 3 && parts[1] == "r" {
			pluginID := parts[2]
			rp, ok := roomRegistry.Get(pluginID)
			if !ok {
				http.NotFound(w, r)
				return
			}
			// 去除前缀 /teams/{teamID}/r/{pluginID}，将剩余路径交给插件
			prefix := "/teams/" + teamID + "/r/" + pluginID
			http.StripPrefix(prefix, rp.Handler(store, teamID)).ServeHTTP(w, r)
			return
		}
```

同样，在 `/api/teams/` 路由分发中（现有代码第 361-484 行之间），找到：
```go
		if len(parts) == 2 && parts[1] == "channels" {
			handleAPITeamChannels(store, teamID, w, r)
			return
		}
```

**在此段之前**插入：
```go
		// Room Plugin API 路由：/api/teams/{teamID}/r/{pluginID}/...
		if len(parts) >= 3 && parts[1] == "r" {
			pluginID := parts[2]
			rp, ok := roomRegistry.Get(pluginID)
			if !ok {
				http.NotFound(w, r)
				return
			}
			prefix := "/api/teams/" + teamID + "/r/" + pluginID
			http.StripPrefix(prefix, rp.Handler(store, teamID)).ServeHTTP(w, r)
			return
		}
```

---

### Step 1.7 — 修改 `handler_channel.go`（Channel Config API）

**路径**：`internal/plugins/haonewsteam/handler_channel.go`

**改动 1**：在文件末尾追加以下 handler 函数：

```go
// === Channel Config Handlers ===

func handleAPITeamChannelConfig(store *teamcore.Store, teamID, channelID string, w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := store.LoadChannelConfigCtx(r.Context(), teamID, channelID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(cfg)

	case http.MethodPut:
		if !teamRequestTrusted(r) {
			http.Error(w, "channel config update is limited to local or LAN requests", http.StatusForbidden)
			return
		}
		if err := requireTeamAction(store, teamID, strings.TrimSpace(r.Header.Get("X-Actor-Agent-ID")), "channel.update"); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		var cfg teamcore.ChannelConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cfg.ChannelID = channelID
		if err := store.SaveChannelConfigCtx(r.Context(), teamID, cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		saved, _ := store.LoadChannelConfigCtx(r.Context(), teamID, channelID)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(saved)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAPITeamChannelConfigs 列出某个 Team 所有已配置的 Channel Configs。
func handleAPITeamChannelConfigs(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	configs, err := store.ListChannelConfigsCtx(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if configs == nil {
		configs = []teamcore.ChannelConfig{}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(configs)
}
```

**改动 2**：在现有 `handleTeamChannel()` 函数中传递 ChannelConfig 到模板。

找到现有代码（`handleTeamChannel` 函数中约第 61-86 行）：
```go
	data := teamChannelPageData{
		Project:        app.ProjectName(),
		Version:        app.VersionString(),
		PageNav:        app.PageNav("/teams"),
		NodeStatus:     app.NodeStatus(index),
		Now:            time.Now(),
		Team:           info,
		Channel:        currentSummary,
		ChannelID:      channelID,
		Channels:       channels,
		Messages:       messages,
		Tasks:          relatedTasksByChannel(tasks, channelID, 12),
		Artifacts:      relatedArtifactsByChannel(artifacts, channelID, 12),
		RelatedHistory: channelHistory(history, channelID, 12),
		SummaryStats: []newsplugin.SummaryStat{
```

在 `RelatedHistory` 行之后，`SummaryStats` 行之前，插入：
```go
		ChannelConfig:  loadChannelConfigSafe(store, r.Context(), teamID, channelID),
```

**改动 3**：在文件末尾追加辅助函数：

```go
func loadChannelConfigSafe(store *teamcore.Store, ctx context.Context, teamID, channelID string) teamcore.ChannelConfig {
	cfg, _ := store.LoadChannelConfigCtx(ctx, teamID, channelID)
	return cfg
}
```

需要在 import 中添加 `"context"` （如果还没有的话）。

---

### Step 1.8 — 修改 `types.go`（Page Data 增加 ChannelConfig）

**路径**：`internal/plugins/haonewsteam/types.go`

找到 `teamChannelPageData` 结构体定义。在该结构体中增加 `ChannelConfig` 字段。

当前的 `teamChannelPageData` 定义大约是：
```go
type teamChannelPageData struct {
	Project        string
	Version        string
	PageNav        []newsplugin.NavItem
	NodeStatus     newsplugin.NodeStatus
	Now            time.Time
	Team           teamcore.Info
	Channel        teamcore.ChannelSummary
	ChannelID      string
	Channels       []teamcore.ChannelSummary
	Messages       []teamcore.Message
	Tasks          []teamcore.Task
	Artifacts      []teamcore.Artifact
	RelatedHistory []teamcore.ChangeEvent
	SummaryStats   []newsplugin.SummaryStat
}
```

在 `RelatedHistory` 后增加一行：
```go
	ChannelConfig  teamcore.ChannelConfig
```

---

### Step 1.9 — 注册新路由到 `plugin.go`

**路径**：`internal/plugins/haonewsteam/plugin.go`

**改动**：在 `/api/teams/` 路由分发的 `newHandler` 函数中增加 Channel Config 路由。

找到现有代码：
```go
		if len(parts) == 3 && parts[1] == "channels" {
			channelID := normalizeTeamChannel(parts[2])
			if channelID == "" {
				http.NotFound(w, r)
				return
			}
			handleAPITeamChannel(store, teamID, channelID, w, r)
			return
		}
```

**在此段之后**插入：
```go
		// Channel Config API
		if len(parts) == 4 && parts[1] == "channels" && parts[3] == "config" {
			channelID := normalizeTeamChannel(parts[2])
			if channelID == "" {
				http.NotFound(w, r)
				return
			}
			handleAPITeamChannelConfig(store, teamID, channelID, w, r)
			return
		}
		if len(parts) == 3 && parts[1] == "channel-configs" {
			handleAPITeamChannelConfigs(store, teamID, w, r)
			return
		}
```

同样，在 `/api/teams/{teamID}` 的 `len(parts) == 1` 响应中（`handleAPITeam`），需要在 JSON 响应中追加 `channel_configs` 字段。

在 `handler.go` 中找到 `handleAPITeam` 函数，在返回 JSON 前获取 channel configs 列表，将其包含在响应中：

```go
// 在 handleAPITeam 函数内，加载 team info 后追加 channel configs
channelConfigs, _ := store.ListChannelConfigsCtx(r.Context(), teamID)
```

然后把 `channelConfigs` 加入 JSON 响应。若 `handleAPITeam` 直接序列化 `teamcore.Info`，则改为构造一个包含 Info + ChannelConfigs 的匿名结构体返回。

---

## Phase 2 — 第一个 Room Plugin：plan-exchange

### Step 2.1 — 新建 `rooms/planexchange/types.go`

**路径**：`internal/plugins/haonewsteam/rooms/planexchange/types.go`

```go
package planexchange

// PlanMessage 是"规划"类型的结构化消息。
// 用于描述一个完整的任务规划，包括目标、拆解步骤、放弃的备选方案、接口约定。
type PlanMessage struct {
	Kind       string              `json:"kind"`                  // 固定值 "plan"
	Title      string              `json:"title"`
	Goal       string              `json:"goal"`
	Steps      []string            `json:"steps,omitempty"`
	Abandoned  []AbandonedOption   `json:"abandoned,omitempty"`   // 放弃的备选方案
	Interfaces []string            `json:"interfaces,omitempty"`  // 接口/边界描述
	ReadyFor   []string            `json:"ready_for,omitempty"`   // 可以开始实现的 Agent ID 列表
}

// AbandonedOption 描述一个被放弃的备选方案。
type AbandonedOption struct {
	Option string `json:"option"`
	Reason string `json:"reason"`
}

// SkillMessage 是"技能"类型的结构化消息。
// 描述一种可复用的方法论/经验，语言无关。
type SkillMessage struct {
	Kind        string   `json:"kind"`                    // 固定值 "skill"
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Steps       []string `json:"steps,omitempty"`
	Traps       []string `json:"traps,omitempty"`         // 陷阱和注意事项
	ValidatedBy string   `json:"validated_by,omitempty"`  // 验证此 Skill 的 Agent ID
	Language    string   `json:"language,omitempty"`       // "language-agnostic" 或具体语言
}

// SnippetMessage 是"代码片段"类型的结构化消息。
// 描述关键逻辑的示意，可包含多种语言的示例。
type SnippetMessage struct {
	Kind       string            `json:"kind"`                  // 固定值 "snippet"
	Title      string            `json:"title"`
	Summary    string            `json:"summary,omitempty"`
	Pseudocode string            `json:"pseudocode,omitempty"`  // 伪代码
	Examples   map[string]string `json:"examples,omitempty"`    // 语言 → 代码示例
	Language   string            `json:"language,omitempty"`     // "multi" 或具体语言
	RelatedTo  string            `json:"related_to,omitempty"`  // 关联的 Plan/Task ID
}
```

---

### Step 2.2 — 新建 `rooms/planexchange/plugin.go`

**路径**：`internal/plugins/haonewsteam/rooms/planexchange/plugin.go`

```go
package planexchange

import (
	_ "embed"
	"net/http"

	teamcore "hao.news/internal/haonews/team"
	"hao.news/internal/plugins/haonewsteam/roomplugin"
)

//go:embed roomplugin.json
var manifestJSON []byte

type Plugin struct{}

func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ID() string {
	return "plan-exchange"
}

func (p *Plugin) Manifest() roomplugin.Manifest {
	m, err := roomplugin.LoadManifestJSON(manifestJSON)
	if err != nil {
		return roomplugin.Manifest{ID: "plan-exchange", Name: "Plan Exchange", Version: "1.0.0"}
	}
	return m
}

func (p *Plugin) Handler(store *teamcore.Store, teamID string) http.Handler {
	return newHandler(store, teamID)
}
```

---

### Step 2.3 — 新建 `rooms/planexchange/handler.go`

**路径**：`internal/plugins/haonewsteam/rooms/planexchange/handler.go`

```go
package planexchange

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	teamcore "hao.news/internal/haonews/team"
)

func newHandler(store *teamcore.Store, teamID string) http.Handler {
	mux := http.NewServeMux()

	// GET / — 列出该 Team 中所有 plan/skill/snippet 类型的消息
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "" {
			http.NotFound(w, r)
			return
		}
		handleListPlanExchangeMessages(store, teamID, w, r)
	})

	// POST /messages — 发布 plan/skill/snippet 消息到指定 Channel
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handlePostPlanExchangeMessage(store, teamID, w, r)
	})

	// POST /distill — 将 Skill 消息提炼为 Artifact
	mux.HandleFunc("/distill", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleDistillSkillToArtifact(store, teamID, w, r)
	})

	return mux
}

// handleListPlanExchangeMessages 列出指定 Channel 中的 plan/skill/snippet 消息。
// 参数：?channel_id=xxx&kind=plan|skill|snippet&limit=50
func handleListPlanExchangeMessages(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	channelID := strings.TrimSpace(r.URL.Query().Get("channel_id"))
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))        // plan, skill, snippet, 或空（全部）
	if channelID == "" {
		channelID = "main"
	}
	messages, err := store.LoadMessagesCtx(r.Context(), teamID, channelID, 200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// 过滤 plan-exchange 相关消息
	validKinds := map[string]bool{"plan": true, "skill": true, "snippet": true}
	var filtered []teamcore.Message
	for _, msg := range messages {
		msgType := strings.TrimSpace(msg.MessageType)
		if !validKinds[msgType] {
			continue
		}
		if kind != "" && msgType != kind {
			continue
		}
		filtered = append(filtered, msg)
	}
	if filtered == nil {
		filtered = []teamcore.Message{}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(filtered)
}

// postPlanExchangeRequest 是发布 plan/skill/snippet 消息的请求体。
type postPlanExchangeRequest struct {
	ChannelID      string         `json:"channel_id"`
	AuthorAgentID  string         `json:"author_agent_id"`
	Kind           string         `json:"kind"`            // "plan", "skill", "snippet"
	Content        string         `json:"content"`         // 纯文本摘要
	StructuredData map[string]any `json:"structured_data"` // PlanMessage / SkillMessage / SnippetMessage
}

func handlePostPlanExchangeMessage(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	var req postPlanExchangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	validKinds := map[string]bool{"plan": true, "skill": true, "snippet": true}
	if !validKinds[req.Kind] {
		http.Error(w, "kind must be plan, skill, or snippet", http.StatusBadRequest)
		return
	}
	if req.ChannelID == "" {
		req.ChannelID = "main"
	}
	if req.AuthorAgentID == "" {
		http.Error(w, "author_agent_id is required", http.StatusBadRequest)
		return
	}

	msg := teamcore.Message{
		TeamID:         teamID,
		ChannelID:      req.ChannelID,
		AuthorAgentID:  req.AuthorAgentID,
		MessageType:    req.Kind,
		Content:        req.Content,
		StructuredData: req.StructuredData,
		CreatedAt:      time.Now().UTC(),
	}
	if err := store.AppendMessageCtx(r.Context(), teamID, msg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "created", "kind": req.Kind})
}

// distillRequest 是 Skill → Artifact 提炼的请求体。
type distillRequest struct {
	ChannelID     string `json:"channel_id"`
	MessageID     string `json:"message_id"`      // 要提炼的 Skill 消息 ID
	ActorAgentID  string `json:"actor_agent_id"`
	Title         string `json:"title,omitempty"`  // 可选覆盖标题
}

func handleDistillSkillToArtifact(store *teamcore.Store, teamID string, w http.ResponseWriter, r *http.Request) {
	var req distillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ChannelID == "" || req.MessageID == "" || req.ActorAgentID == "" {
		http.Error(w, "channel_id, message_id, and actor_agent_id are required", http.StatusBadRequest)
		return
	}
	// 查找原始 Skill 消息
	messages, err := store.LoadAllMessagesCtx(r.Context(), teamID, req.ChannelID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var skillMsg *teamcore.Message
	for i := range messages {
		if messages[i].MessageID == req.MessageID && messages[i].MessageType == "skill" {
			skillMsg = &messages[i]
			break
		}
	}
	if skillMsg == nil {
		http.Error(w, "skill message not found", http.StatusNotFound)
		return
	}
	// 构建 Artifact
	title := req.Title
	if title == "" {
		title = skillMsg.Content
		if len(title) > 80 {
			title = title[:80]
		}
	}
	content, _ := json.MarshalIndent(skillMsg.StructuredData, "", "  ")
	artifact := teamcore.Artifact{
		TeamID:    teamID,
		ChannelID: req.ChannelID,
		Title:     title,
		Kind:      "skill-doc",
		Summary:   skillMsg.Content,
		Content:   string(content),
		CreatedBy: req.ActorAgentID,
		Labels:    []string{"distilled", "skill"},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := store.AppendArtifactCtx(r.Context(), teamID, artifact); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "distilled", "artifact_kind": "skill-doc"})
}
```

---

### Step 2.4 — 新建 `rooms/planexchange/roomplugin.json`

**路径**：`internal/plugins/haonewsteam/rooms/planexchange/roomplugin.json`

```json
{
  "id": "plan-exchange",
  "name": "规划交流插件",
  "version": "1.0.0",
  "description": "用于多 Agent 之间交换规划、Skill 描述和代码片段的专属插件。不要求完整代码，只交流意图和知识。",
  "messageKinds": ["plan", "skill", "snippet"],
  "artifactKinds": ["skill-doc", "plan-summary"]
}
```

---

### Step 2.5 — 在 `plugin.go` 中注册 plan-exchange

**路径**：`internal/plugins/haonewsteam/plugin.go`

取消 Phase 1 中注释掉的那行：

```go
// 将:
// registry.MustRegister(planexchange.New())
// 改为:
registry.MustRegister(planexchange.New())
```

同时在 import 中加入：
```go
"hao.news/internal/plugins/haonewsteam/rooms/planexchange"
```

---

### Step 2.6 — 修改 `haonews.plugin.json`

**路径**：`internal/plugins/haonewsteam/haonews.plugin.json`

```json
{
  "id": "hao-news-team",
  "name": "Hao.News Team",
  "version": "0.2.0",
  "plugin_kind": "team",
  "description": "Team plugin for long-running multi-agent collaboration with independent team pages, JSON APIs, and Room Plugin support.",
  "default_theme": "hao-news-theme",
  "room_plugins": ["plan-exchange"]
}
```

---

## Phase 3 — minimal Room Theme

### Step 3.1 — 新建 `roomtheme.json`

**路径**：`internal/themes/room-themes/minimal/roomtheme.json`

```json
{
  "id": "minimal",
  "name": "极简主题",
  "version": "1.0.0",
  "description": "极简频道视图，适合 Agent 机器解析和开发者调试。去除所有装饰性 UI。",
  "overrides": ["room_channel.html"]
}
```

### Step 3.2 — 新建极简频道模板

**路径**：`internal/themes/room-themes/minimal/web/templates/room_channel.html`

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<title>{{.Team.Title}} — {{.Channel.Title}} (minimal)</title>
<style>
body { font-family: monospace; max-width: 900px; margin: 0 auto; padding: 16px; background: #fafafa; color: #222; }
h1, h2 { font-size: 16px; margin: 8px 0; }
.msg { border-bottom: 1px solid #ddd; padding: 8px 0; }
.msg-meta { color: #888; font-size: 12px; }
.msg-type { font-weight: bold; text-transform: uppercase; }
.msg-type-plan { color: #2563eb; }
.msg-type-skill { color: #059669; }
.msg-type-snippet { color: #d97706; }
.structured { background: #f0f0f0; padding: 8px; margin: 4px 0; white-space: pre-wrap; font-size: 13px; overflow-x: auto; }
.onboarding { background: #eff6ff; border-left: 3px solid #2563eb; padding: 8px 12px; margin: 12px 0; }
.rules { background: #f0fdf4; border-left: 3px solid #059669; padding: 8px 12px; margin: 12px 0; }
</style>
</head>
<body>
<h1>[{{.Team.TeamID}}] {{.Team.Title}} / #{{.Channel.ChannelID}} {{.Channel.Title}}</h1>

{{if .ChannelConfig.AgentOnboarding}}
<div class="onboarding">
<strong>Agent Onboarding:</strong> {{.ChannelConfig.AgentOnboarding}}
</div>
{{end}}

{{if .ChannelConfig.Rules}}
<div class="rules">
<strong>Rules:</strong>
{{range .ChannelConfig.Rules}}<div>- {{.}}</div>{{end}}
</div>
{{end}}

{{if .ChannelConfig.Plugin}}
<div><strong>Plugin:</strong> {{.ChannelConfig.Plugin}} | <strong>Theme:</strong> {{.ChannelConfig.Theme}}</div>
{{end}}

<h2>Messages ({{len .Messages}})</h2>
{{range .Messages}}
<div class="msg">
<div class="msg-meta">
<span class="msg-type msg-type-{{.MessageType}}">[{{.MessageType}}]</span>
{{.AuthorAgentID}} — {{.CreatedAt.Format "2006-01-02 15:04:05"}}
{{if .MessageID}} ({{.MessageID}}){{end}}
</div>
<div>{{.Content}}</div>
{{if .StructuredData}}
<div class="structured">{{structuredJSON .StructuredData}}</div>
{{end}}
</div>
{{end}}

{{if not .Messages}}
<p>频道暂无消息。</p>
{{end}}
</body>
</html>
```

**注意**：此模板中引用了 `structuredJSON` 模板函数。需要在 Template FuncMap 中注册此函数：

```go
// 在模板加载时注册
funcMap["structuredJSON"] = func(data map[string]any) string {
    b, err := json.MarshalIndent(data, "", "  ")
    if err != nil {
        return fmt.Sprintf("%v", data)
    }
    return string(b)
}
```

如果现有模板系统已有类似功能（如 `json` 函数），可直接使用。否则在 `newsplugin.App` 的模板 FuncMap 注册。

### Step 3.3 — 默认频道 Room 模板（fallback）

**路径**：`internal/plugins/haonews/web/templates/room_channel_default.html`

这个是当 Channel 配置了 Room Theme 但模板未找到时的 fallback。可以直接复用现有的 `team_channel.html`，在其中增加 ChannelConfig 相关字段的展示。

在现有 `team_channel.html` 模板中，在频道标题区域之后追加：

```html
{{if .ChannelConfig.AgentOnboarding}}
<div class="team-card" style="margin-bottom: 12px;">
  <h4>Agent 引导</h4>
  <p>{{.ChannelConfig.AgentOnboarding}}</p>
</div>
{{end}}
{{if .ChannelConfig.Rules}}
<div class="team-card" style="margin-bottom: 12px;">
  <h4>频道规则</h4>
  {{range .ChannelConfig.Rules}}<div>• {{.}}</div>{{end}}
</div>
{{end}}
```

---

## Phase 4 — 测试

### Step 4.1 — `channel_config_test.go`

**路径**：`internal/haonews/team/channel_config_test.go`

```go
package team

import (
	"context"
	"os"
	"testing"
)

func TestChannelConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	// 先创建一个 team（需要 team.json 存在）
	info := Info{TeamID: "test-team", Title: "Test"}
	if err := store.SaveTeamCtx(context.Background(), info); err != nil {
		t.Fatal(err)
	}
	// 创建一个 channel
	ch := Channel{ChannelID: "dev", Title: "Dev Channel"}
	if err := store.SaveChannelCtx(context.Background(), "test-team", ch); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. 读取不存在的 config → 返回零值，不报错
	cfg, err := store.LoadChannelConfigCtx(ctx, "test-team", "dev")
	if err != nil {
		t.Fatalf("load non-existent config: %v", err)
	}
	if cfg.Plugin != "" {
		t.Fatalf("expected empty plugin, got %q", cfg.Plugin)
	}
	if cfg.ChannelID != "dev" {
		t.Fatalf("expected channel_id dev, got %q", cfg.ChannelID)
	}

	// 2. 保存 config
	cfg = ChannelConfig{
		ChannelID:       "dev",
		Plugin:          "plan-exchange@1.0",
		Theme:           "minimal",
		AgentOnboarding: "Welcome to dev channel",
		Rules:           []string{"Rule 1", "Rule 2"},
	}
	if err := store.SaveChannelConfigCtx(ctx, "test-team", cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	// 3. 重新加载
	loaded, err := store.LoadChannelConfigCtx(ctx, "test-team", "dev")
	if err != nil {
		t.Fatalf("load saved config: %v", err)
	}
	if loaded.Plugin != "plan-exchange@1.0" {
		t.Fatalf("expected plugin plan-exchange@1.0, got %q", loaded.Plugin)
	}
	if loaded.Theme != "minimal" {
		t.Fatalf("expected theme minimal, got %q", loaded.Theme)
	}
	if loaded.AgentOnboarding != "Welcome to dev channel" {
		t.Fatalf("expected onboarding text, got %q", loaded.AgentOnboarding)
	}
	if len(loaded.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(loaded.Rules))
	}
	if loaded.CreatedAt.IsZero() {
		t.Fatal("expected non-zero CreatedAt")
	}

	// 4. PluginID() 方法
	if loaded.PluginID() != "plan-exchange" {
		t.Fatalf("expected PluginID plan-exchange, got %q", loaded.PluginID())
	}

	// 5. ListChannelConfigs
	configs, err := store.ListChannelConfigsCtx(ctx, "test-team")
	if err != nil {
		t.Fatalf("list configs: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}

	// 6. 更新 config 保留 CreatedAt
	originalCreated := loaded.CreatedAt
	cfg.AgentOnboarding = "Updated onboarding"
	cfg.CreatedAt = loaded.CreatedAt // 显式传入
	if err := store.SaveChannelConfigCtx(ctx, "test-team", cfg); err != nil {
		t.Fatalf("update config: %v", err)
	}
	reloaded, _ := store.LoadChannelConfigCtx(ctx, "test-team", "dev")
	if reloaded.AgentOnboarding != "Updated onboarding" {
		t.Fatalf("expected updated onboarding, got %q", reloaded.AgentOnboarding)
	}
	if reloaded.CreatedAt != originalCreated {
		t.Fatal("CreatedAt should be preserved on update")
	}

	// 7. 确认文件位置正确
	expectedPath := store.channelConfigPath("test-team", "dev")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("config file should exist at %s: %v", expectedPath, err)
	}
}

func TestChannelConfigPluginIDNoVersion(t *testing.T) {
	cfg := ChannelConfig{Plugin: "my-plugin"}
	if cfg.PluginID() != "my-plugin" {
		t.Fatalf("expected my-plugin, got %q", cfg.PluginID())
	}
}

func TestChannelConfigPluginIDEmpty(t *testing.T) {
	cfg := ChannelConfig{}
	if cfg.PluginID() != "" {
		t.Fatalf("expected empty, got %q", cfg.PluginID())
	}
}
```

### Step 4.2 — `roomplugin/registry_test.go`

**路径**：`internal/plugins/haonewsteam/roomplugin/registry_test.go`

```go
package roomplugin

import (
	"net/http"
	"testing"

	teamcore "hao.news/internal/haonews/team"
)

type mockPlugin struct {
	id string
}

func (p mockPlugin) ID() string { return p.id }
func (p mockPlugin) Manifest() Manifest {
	return Manifest{ID: p.id, Name: "Mock " + p.id, Version: "0.1.0"}
}
func (p mockPlugin) Handler(store *teamcore.Store, teamID string) http.Handler {
	return http.NotFoundHandler()
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	mp := mockPlugin{id: "test-plugin"}
	if err := r.Register(mp); err != nil {
		t.Fatalf("register: %v", err)
	}
	got, ok := r.Get("test-plugin")
	if !ok {
		t.Fatal("expected plugin to be found")
	}
	if got.ID() != "test-plugin" {
		t.Fatalf("expected test-plugin, got %q", got.ID())
	}
}

func TestRegistryDuplicateReject(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(mockPlugin{id: "dup"})
	if err := r.Register(mockPlugin{id: "dup"}); err == nil {
		t.Fatal("expected error on duplicate register")
	}
}

func TestRegistryGetNotFound(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestRegistryAll(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(mockPlugin{id: "a"})
	r.MustRegister(mockPlugin{id: "b"})
	all := r.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(all))
	}
}

func TestRegistryIDs(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(mockPlugin{id: "x"})
	r.MustRegister(mockPlugin{id: "y"})
	ids := r.IDs()
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %d", len(ids))
	}
}

func TestRegistryEmptyID(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(mockPlugin{id: ""}); err == nil {
		t.Fatal("expected error on empty id")
	}
}

func TestLoadManifestJSON(t *testing.T) {
	data := []byte(`{"id":"test","name":"Test","version":"1.0.0","messageKinds":["plan"]}`)
	m, err := LoadManifestJSON(data)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if m.ID != "test" {
		t.Fatalf("expected id test, got %q", m.ID)
	}
	if len(m.MessageKinds) != 1 || m.MessageKinds[0] != "plan" {
		t.Fatalf("unexpected message kinds: %v", m.MessageKinds)
	}
}

func TestLoadManifestJSONMissingID(t *testing.T) {
	data := []byte(`{"name":"Test"}`)
	_, err := LoadManifestJSON(data)
	if err == nil {
		t.Fatal("expected error on missing id")
	}
}
```

---

## 新增 API 端点汇总

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/teams/{teamID}/channels/{channelID}/config` | 获取 Channel Config |
| PUT | `/api/teams/{teamID}/channels/{channelID}/config` | 更新 Channel Config（仅 local/LAN） |
| GET | `/api/teams/{teamID}/channel-configs` | 列出所有已配置 Channel Config |
| ANY | `/teams/{teamID}/r/{pluginID}/*` | Room Plugin Web 路由 |
| ANY | `/api/teams/{teamID}/r/{pluginID}/*` | Room Plugin API 路由 |
| GET | `/api/teams/{teamID}/r/plan-exchange/?channel_id=xxx&kind=plan` | 列出 plan-exchange 消息 |
| POST | `/api/teams/{teamID}/r/plan-exchange/messages` | 发布 plan/skill/snippet 消息 |
| POST | `/api/teams/{teamID}/r/plan-exchange/distill` | Skill → Artifact 提炼 |

---

## 数据存储路径汇总

```
{StoreRoot}/team/{teamID}/
├── team.json              # 现有（不动）
├── policy.json            # 现有（不动）
├── members.jsonl          # 现有（不动）
├── channels.json          # 现有（不动）
├── channels/              # 现有（不动）
│   ├── main.jsonl
│   └── dev.jsonl
├── tasks.data.jsonl       # 现有（不动）
├── artifacts.data.jsonl   # 现有（不动）
├── history.jsonl          # 现有（不动）
├── webhooks.json          # 现有（不动）
└── channel-configs/       # 新增
    ├── main.json          # main 频道的 Room Plugin/Theme 配置
    └── dev.json           # dev 频道的 Room Plugin/Theme 配置
```

---

## 执行规则

1. **按 Phase 顺序执行**：P1 → P2 → P3 → P4。每个 Phase 完成后运行 `go build ./...` 和 `go test ./...`
2. **不动 sync**：不修改 `sync.go`、`team_sync.go`、`team_sync_test.go`
3. **不动 store.go 现有代码**：ChannelConfig 相关代码全部在新文件 `channel_config.go` 中，`ctx_api.go` 和 `compat_api.go` 只在末尾追加方法
4. **向后兼容**：ChannelConfig 文件不存在时返回零值，所有 Channel 在没有配置时表现完全不变
5. **Room Plugin 挂载失败不影响主干**：`roomRegistry.Get()` 返回 false 时只返回 404，不 panic
6. **消息通过标准接口存储**：plan-exchange 插件不直接写 JSONL 文件，全部通过 `store.AppendMessageCtx` / `store.AppendArtifactCtx`
7. **P2P 同步无需改动**：plan/skill/snippet 消息只是 `message_type` 字段值不同的标准 Message，现有同步机制原样复制

## 成功标准

- `go build ./...` 通过
- `go test ./...` 通过（包括所有现有测试 + 新增测试）
- `GET /api/teams/{teamID}/channels/{channelID}/config` 返回正确的 ChannelConfig JSON
- `PUT /api/teams/{teamID}/channels/{channelID}/config` 可保存配置
- `POST /api/teams/{teamID}/r/plan-exchange/messages` 可发布三种消息类型
- `GET /api/teams/{teamID}/r/plan-exchange/?channel_id=dev&kind=skill` 可按类型过滤
- `POST /api/teams/{teamID}/r/plan-exchange/distill` 可将 Skill 提炼为 Artifact
- 所有现有页面和 API 功能不受影响

---

日期：2026-04-05
前置文档：20260405-team-pro1.md
状态：待实施
