package team

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// ListTeamsCtx 返回当前 store 下所有 Team 的摘要视图。
// 副作用：无；会并发读取 team/member/channel 相关文件。
func (s *Store) ListTeamsCtx(ctx context.Context) ([]Summary, error) {
	if s == nil {
		return nil, nil
	}
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	teamIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if err := ctxErr(ctx); err != nil {
			return nil, err
		}
		if !entry.IsDir() {
			continue
		}
		teamID := NormalizeTeamID(entry.Name())
		if teamID == "" {
			continue
		}
		teamIDs = append(teamIDs, teamID)
	}
	out := make([]Summary, 0, len(teamIDs))
	type result struct {
		summary Summary
		ok      bool
		err     error
	}
	results := make(chan result, len(teamIDs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, teamID := range teamIDs {
		if err := ctxErr(ctx); err != nil {
			return nil, err
		}
		wg.Add(1)
		go func(teamID string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results <- result{err: ctx.Err()}
				return
			}
			defer func() { <-sem }()

			info, err := s.LoadTeamCtx(ctx, teamID)
			if err != nil {
				results <- result{err: err}
				return
			}
			members, err := s.LoadMembersCtx(ctx, teamID)
			if err != nil {
				results <- result{err: err}
				return
			}
			channels, err := s.ListChannelsCtx(ctx, teamID)
			channelCount := len(teamChannels(info))
			if err == nil && len(channels) > 0 {
				channelCount = len(channels)
			}
			results <- result{
				summary: Summary{
					Info:         info,
					MemberCount:  len(members),
					ChannelCount: channelCount,
				},
				ok: true,
			}
		}(teamID)
	}
	wg.Wait()
	close(results)
	for result := range results {
		if result.err != nil {
			if errors.Is(result.err, context.Canceled) || errors.Is(result.err, context.DeadlineExceeded) {
				return nil, result.err
			}
			continue
		}
		if !result.ok {
			continue
		}
		out = append(out, result.summary)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].TeamID < out[j].TeamID
	})
	return out, nil
}

// LoadTeamCtx 读取单个 Team 的基础信息。
// 前置条件：teamID 必须可规范化；副作用：无。
func (s *Store) LoadTeamCtx(ctx context.Context, teamID string) (Info, error) {
	if err := ctxErr(ctx); err != nil {
		return Info{}, err
	}
	return s.loadTeamNoCtx(teamID)
}

// SaveTeamCtx 保存 Team 基础信息。
// 前置条件：info.TeamID/title 等字段应已可用；副作用：会覆盖 team.json。
func (s *Store) SaveTeamCtx(ctx context.Context, info Info) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	return s.saveTeamNoCtx(info)
}

func (s *Store) LoadMembersCtx(ctx context.Context, teamID string) ([]Member, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	return s.loadMembersNoCtx(teamID)
}

func (s *Store) LoadPolicyCtx(ctx context.Context, teamID string) (Policy, error) {
	if err := ctxErr(ctx); err != nil {
		return Policy{}, err
	}
	return s.loadPolicyNoCtx(teamID)
}

func (s *Store) LoadMembersSnapshotCtx(ctx context.Context, teamID string) ([]Member, time.Time, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, time.Time{}, err
	}
	return s.loadMembersSnapshotNoCtx(teamID)
}

func (s *Store) LoadPolicySnapshotCtx(ctx context.Context, teamID string) (Policy, time.Time, error) {
	if err := ctxErr(ctx); err != nil {
		return Policy{}, time.Time{}, err
	}
	return s.loadPolicySnapshotNoCtx(teamID)
}

func (s *Store) LoadChannelSnapshotCtx(ctx context.Context, teamID, channelID string) (Channel, time.Time, error) {
	if err := ctxErr(ctx); err != nil {
		return Channel{}, time.Time{}, err
	}
	return s.loadChannelSnapshotNoCtx(teamID, channelID)
}

// LoadChannelConfigCtx 读取单个频道配置。
// 副作用：无；兼容 canonical 路径与旧路径读取。
func (s *Store) LoadChannelConfigCtx(ctx context.Context, teamID, channelID string) (ChannelConfig, error) {
	if err := ctxErr(ctx); err != nil {
		return ChannelConfig{}, err
	}
	return s.loadChannelConfigNoCtx(teamID, channelID)
}

// LoadMessagesCtx 按频道读取消息，limit=0 表示全量。
// 副作用：无；会读取 JSONL 或分片 JSONL。
func (s *Store) LoadMessagesCtx(ctx context.Context, teamID, channelID string, limit int) ([]Message, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	return s.loadMessagesNoCtx(teamID, channelID, limit)
}

func (s *Store) LoadAllMessagesCtx(ctx context.Context, teamID, channelID string) ([]Message, error) {
	return s.LoadMessagesCtx(ctx, teamID, channelID, 0)
}

// LoadChannelCtx 读取单个频道定义。
// 前置条件：teamID/channelID 必须可规范化；副作用：无。
func (s *Store) LoadChannelCtx(ctx context.Context, teamID, channelID string) (Channel, error) {
	if err := ctxErr(ctx); err != nil {
		return Channel{}, err
	}
	return s.loadChannelNoCtx(teamID, channelID)
}

// ListChannelsCtx 列出 Team 下所有频道摘要。
// 副作用：无；会合并频道配置与消息统计。
func (s *Store) ListChannelsCtx(ctx context.Context, teamID string) ([]ChannelSummary, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	return s.listChannelsNoCtx(teamID)
}

// ListChannelConfigsCtx 列出 Team 下所有频道配置。
// 副作用：无；用于 Room Plugin / Theme / Onboarding 聚合。
func (s *Store) ListChannelConfigsCtx(ctx context.Context, teamID string) ([]ChannelConfig, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	return s.listChannelConfigsNoCtx(teamID)
}

// LoadTasksCtx 读取 Team 任务列表，limit=0 表示全量。
// 副作用：无；返回值已按当前索引/存储内容归一化。
func (s *Store) LoadTasksCtx(ctx context.Context, teamID string, limit int) ([]Task, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	return s.loadTasksNoCtx(teamID, limit)
}

// LoadTaskCtx 读取单个任务。
// 前置条件：taskID 必须存在；副作用：无。
func (s *Store) LoadTaskCtx(ctx context.Context, teamID, taskID string) (Task, error) {
	if err := ctxErr(ctx); err != nil {
		return Task{}, err
	}
	return s.loadTaskNoCtx(teamID, taskID)
}

// ListMilestonesCtx 读取 Team 里程碑列表。
// 副作用：无；返回值已做时间与字段归一化。
func (s *Store) ListMilestonesCtx(ctx context.Context, teamID string) ([]Milestone, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	return s.loadMilestonesNoCtx(teamID)
}

// LoadMilestoneCtx 读取单个里程碑。
// 前置条件：milestoneID 必须可定位；副作用：无。
func (s *Store) LoadMilestoneCtx(ctx context.Context, teamID, milestoneID string) (Milestone, error) {
	if err := ctxErr(ctx); err != nil {
		return Milestone{}, err
	}
	return s.loadMilestoneNoCtx(teamID, milestoneID)
}

// ListMilestoneProgressCtx 读取里程碑进度聚合视图。
// 副作用：无；会聚合 milestone 与 task 进度。
func (s *Store) ListMilestoneProgressCtx(ctx context.Context, teamID string) ([]MilestoneProgress, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	return s.listMilestoneProgressNoCtx(teamID)
}

// LoadArtifactsCtx 读取 Team 产物列表，limit=0 表示全量。
// 副作用：无。
func (s *Store) LoadArtifactsCtx(ctx context.Context, teamID string, limit int) ([]Artifact, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	return s.loadArtifactsNoCtx(teamID, limit)
}

func (s *Store) LoadArtifactCtx(ctx context.Context, teamID, artifactID string) (Artifact, error) {
	if err := ctxErr(ctx); err != nil {
		return Artifact{}, err
	}
	return s.loadArtifactNoCtx(teamID, artifactID)
}

// LoadHistoryCtx 读取 Team 历史事件，limit=0 表示全量。
// 副作用：无；结果按时间倒序返回。
func (s *Store) LoadHistoryCtx(ctx context.Context, teamID string, limit int) ([]ChangeEvent, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	return s.loadHistoryNoCtx(teamID, limit)
}

func (s *Store) LoadWebhookConfigsCtx(ctx context.Context, teamID string) ([]PushNotificationConfig, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	return s.loadWebhookConfigsNoCtx(teamID)
}

func (s *Store) ListArchivesCtx(ctx context.Context, teamID string) ([]ArchiveSnapshot, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	return s.listArchivesNoCtx(teamID)
}

func (s *Store) LoadArchiveCtx(ctx context.Context, teamID, archiveID string) (ArchiveSnapshot, error) {
	if err := ctxErr(ctx); err != nil {
		return ArchiveSnapshot{}, err
	}
	return s.loadArchiveNoCtx(teamID, archiveID)
}

func (s *Store) LoadAgentCardCtx(ctx context.Context, teamID, agentID string) (AgentCard, error) {
	if err := ctxErr(ctx); err != nil {
		return AgentCard{}, err
	}
	return s.loadAgentCardNoCtx(teamID, agentID)
}

func (s *Store) ListAgentCardsCtx(ctx context.Context, teamID string) ([]AgentCard, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	return s.listAgentCardsNoCtx(teamID)
}

func (s *Store) LoadTasksByContextCtx(ctx context.Context, teamID, contextID string) ([]Task, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	return s.loadTasksByContextNoCtx(teamID, contextID)
}

func (s *Store) LoadTaskMessagesCtx(ctx context.Context, teamID, taskID string, limit int) ([]Message, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	teamID = NormalizeTeamID(teamID)
	taskID = strings.TrimSpace(taskID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	if taskID == "" {
		return nil, errors.New("empty task id")
	}
	task, err := s.LoadTaskCtx(ctx, teamID, taskID)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	channelSummaries, err := s.ListChannelsCtx(ctx, teamID)
	if err != nil {
		return nil, err
	}
	preferred := []string{}
	if task.TaskID != "" {
		preferred = append(preferred, task.ChannelID)
		if task.ContextID != "" {
			tasksByContext, err := s.LoadTasksByContextCtx(ctx, teamID, task.ContextID)
			if err != nil {
				return nil, err
			}
			for _, item := range tasksByContext {
				preferred = append(preferred, item.ChannelID)
			}
		}
	}
	preferred = append(preferred, "main")
	channels := orderedChannelIDs(channelSummaries, preferred...)
	return loadMessagesMatchingChannelsCtx(ctx, channels, limit, func(message Message) bool {
		return taskIDMatches(message.StructuredData, taskID)
	}, func(channelID string) ([]Message, error) {
		return s.LoadAllMessagesCtx(ctx, teamID, channelID)
	})
}

func (s *Store) LoadMessagesByContextCtx(ctx context.Context, teamID, contextID string, limit int) ([]Message, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	teamID = NormalizeTeamID(teamID)
	contextID = normalizeContextID(contextID)
	if teamID == "" {
		return nil, errors.New("empty team id")
	}
	if contextID == "" {
		return nil, errors.New("empty context id")
	}
	tasks, err := s.LoadTasksByContextCtx(ctx, teamID, contextID)
	if err != nil {
		return nil, err
	}
	channelSummaries, err := s.ListChannelsCtx(ctx, teamID)
	if err != nil {
		return nil, err
	}
	preferred := []string{"main"}
	for _, task := range tasks {
		preferred = append(preferred, task.ChannelID)
	}
	channels := orderedChannelIDs(channelSummaries, preferred...)
	return loadMessagesMatchingChannelsCtx(ctx, channels, limit, func(message Message) bool {
		return normalizeContextID(message.ContextID) == contextID || structuredDataContextID(message.StructuredData) == contextID
	}, func(channelID string) ([]Message, error) {
		return s.LoadAllMessagesCtx(ctx, teamID, channelID)
	})
}

// SaveMembersCtx 保存成员快照。
// 副作用：会覆盖 members.json，并影响权限与成员统计。
func (s *Store) SaveMembersCtx(ctx context.Context, teamID string, members []Member) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	return s.saveMembersNoCtx(teamID, members)
}

// SaveWebhookConfigsCtx 保存 Team webhook 配置。
// 副作用：会覆盖 webhook 配置集合并影响后续投递行为。
func (s *Store) SaveWebhookConfigsCtx(ctx context.Context, teamID string, configs []PushNotificationConfig) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	return s.saveWebhookConfigsNoCtx(teamID, configs)
}

// SavePolicyCtx 保存 Team policy。
// 副作用：会覆盖 policy.json，并影响权限检查与状态流转。
func (s *Store) SavePolicyCtx(ctx context.Context, teamID string, policy Policy) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	return s.savePolicyNoCtx(teamID, policy)
}

// SaveChannelConfigCtx 保存频道配置。
// 副作用：会覆盖 channel config，并影响 Room Plugin / Theme / Agent Onboarding。
func (s *Store) SaveChannelConfigCtx(ctx context.Context, teamID string, cfg ChannelConfig) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	return s.saveChannelConfigNoCtx(teamID, cfg)
}

// AppendMessageCtx 追加一条 Team 消息。
// 前置条件：teamID 可用、消息内容非空；副作用：会写消息文件、可能写通知、会发布 TeamEvent。
func (s *Store) AppendMessageCtx(ctx context.Context, teamID string, msg Message) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	return s.appendMessageNoCtx(teamID, msg)
}

// SaveChannelCtx 保存频道定义。
// 副作用：会覆盖频道配置并影响频道列表与消息入口。
func (s *Store) SaveChannelCtx(ctx context.Context, teamID string, channel Channel) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	return s.saveChannelNoCtx(teamID, channel)
}

// SaveMilestoneCtx 保存里程碑。
// 副作用：会覆盖 milestone 数据，并影响任务进度聚合。
func (s *Store) SaveMilestoneCtx(ctx context.Context, teamID string, milestone Milestone) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	return s.saveMilestoneNoCtx(teamID, milestone)
}

// DeleteMilestoneCtx 删除里程碑。
// 前置条件：调用方应先处理挂接任务；副作用：会移除 milestone 记录。
func (s *Store) DeleteMilestoneCtx(ctx context.Context, teamID, milestoneID string) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	return s.deleteMilestoneNoCtx(teamID, milestoneID)
}

func (s *Store) HideChannelCtx(ctx context.Context, teamID, channelID string) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	return s.hideChannelNoCtx(teamID, channelID)
}

// AppendTaskCtx 追加创建一条任务。
// 前置条件：title 必填，状态与依赖需通过校验；副作用：会写任务、通知和 TeamEvent。
func (s *Store) AppendTaskCtx(ctx context.Context, teamID string, task Task) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	return s.appendTaskNoCtx(teamID, task)
}

// SaveTaskCtx 保存现有任务。
// 前置条件：task.TaskID 必填；副作用：会更新任务、通知、hook 与 TeamEvent。
func (s *Store) SaveTaskCtx(ctx context.Context, teamID string, task Task) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	return s.saveTaskNoCtx(teamID, task)
}

func (s *Store) DeleteTaskCtx(ctx context.Context, teamID, taskID string) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	return s.deleteTaskNoCtx(teamID, taskID)
}

// AppendArtifactCtx 追加创建一条产物。
// 前置条件：artifact.Title 必填；副作用：会写产物并发布 TeamEvent。
func (s *Store) AppendArtifactCtx(ctx context.Context, teamID string, artifact Artifact) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	return s.appendArtifactNoCtx(teamID, artifact)
}

// SaveArtifactCtx 保存现有产物。
// 前置条件：artifact.ArtifactID 必填；副作用：会更新产物并影响检索结果。
func (s *Store) SaveArtifactCtx(ctx context.Context, teamID string, artifact Artifact) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	return s.saveArtifactNoCtx(teamID, artifact)
}

// DeleteArtifactCtx 删除产物。
// 前置条件：artifactID 必须存在；副作用：会移除产物并影响相关检索结果。
func (s *Store) DeleteArtifactCtx(ctx context.Context, teamID, artifactID string) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	return s.deleteArtifactNoCtx(teamID, artifactID)
}

// AppendHistoryCtx 追加一条 Team 历史事件。
// 前置条件：event.Scope 和 event.Action 不能为空；副作用：会写 history.jsonl 并发布 TeamEvent。
func (s *Store) AppendHistoryCtx(ctx context.Context, teamID string, event ChangeEvent) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	return s.appendHistoryNoCtx(teamID, event)
}

func (s *Store) CreateManualArchiveCtx(ctx context.Context, teamID string, now time.Time) (*ArchiveSnapshot, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	return s.createManualArchiveNoCtx(teamID, now)
}

func (s *Store) SaveAgentCardCtx(ctx context.Context, teamID string, card AgentCard) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	return s.saveAgentCardNoCtx(teamID, card)
}
