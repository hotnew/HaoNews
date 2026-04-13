package team

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type TemplateRoleBinding struct {
	Alias  string `json:"alias"`
	Role   string `json:"role"`
	Status string `json:"status,omitempty"`
}

type TemplateSeedTask struct {
	TaskID          string   `json:"task_id"`
	Title           string   `json:"title"`
	Description     string   `json:"description,omitempty"`
	ChannelID       string   `json:"channel_id,omitempty"`
	MilestoneID     string   `json:"milestone_id,omitempty"`
	DependsOn       []string `json:"depends_on,omitempty"`
	AssigneeAliases []string `json:"assignee_aliases,omitempty"`
	Status          string   `json:"status,omitempty"`
	Priority        string   `json:"priority,omitempty"`
	Labels          []string `json:"labels,omitempty"`
}

type TeamTemplate struct {
	TemplateID     string                `json:"template_id"`
	Title          string                `json:"title"`
	Description    string                `json:"description,omitempty"`
	Channels       []Channel             `json:"channels,omitempty"`
	Policy         Policy                `json:"policy"`
	ChannelConfigs []ChannelConfig       `json:"channel_configs,omitempty"`
	RoleBindings   []TemplateRoleBinding `json:"role_bindings,omitempty"`
	SeedMilestones []Milestone           `json:"seed_milestones,omitempty"`
	SeedTasks      []TemplateSeedTask    `json:"seed_tasks,omitempty"`
}

func BuiltinTeamTemplates() []TeamTemplate {
	return []TeamTemplate{
		{
			TemplateID:  "spec-package",
			Title:       "Spec Package",
			Description: "多 agent 规格共创模板，专门用于把讨论、评审、决策和 Markdown 产物收成可直接交付的规格包。",
			Channels: []Channel{
				{ChannelID: "main", Title: "Scope"},
				{ChannelID: "reviews", Title: "Reviews"},
				{ChannelID: "decisions", Title: "Decisions"},
				{ChannelID: "artifacts", Title: "Artifacts"},
			},
			Policy: DefaultPolicy(),
			ChannelConfigs: []ChannelConfig{
				{ChannelID: "main", Plugin: "plan-exchange@1.0", Theme: "minimal", AgentOnboarding: "提出目标、非目标、方案、约束和可拼装 md 片段。"},
				{ChannelID: "reviews", Plugin: "review-room@1.0", Theme: "focus", AgentOnboarding: "针对规格缺口、风险、歧义和边界做 review / risk / decision。"},
				{ChannelID: "decisions", Plugin: "decision-room@1.0", Theme: "board", AgentOnboarding: "冻结规格边界、取舍和最终结论，避免实现时重新讨论。"},
				{ChannelID: "artifacts", Plugin: "artifact-room@1.0", Theme: "board", AgentOnboarding: "沉淀 product / workflows / data-model / api / verification 等规格文档。"},
			},
			RoleBindings: []TemplateRoleBinding{
				{Alias: "owner", Role: MemberRoleOwner, Status: MemberStatusActive},
				{Alias: "proposer", Role: MemberRoleMaintainer, Status: MemberStatusActive},
				{Alias: "reviewer", Role: MemberRoleMaintainer, Status: MemberStatusActive},
				{Alias: "editor", Role: MemberRoleMaintainer, Status: MemberStatusActive},
			},
			SeedMilestones: []Milestone{
				{MilestoneID: "scope-frozen", Title: "目标与边界冻结", Status: MilestoneStateOpen},
				{MilestoneID: "workflow-frozen", Title: "流程冻结", Status: MilestoneStateOpen},
				{MilestoneID: "data-model-ready", Title: "数据模型完成", Status: MilestoneStateOpen},
				{MilestoneID: "verification-ready", Title: "验证标准完成", Status: MilestoneStateOpen},
				{MilestoneID: "spec-package-ready", Title: "规格包冻结", Status: MilestoneStateOpen},
			},
			SeedTasks: []TemplateSeedTask{
				{
					TaskID:          "scope-goals-and-nongoals",
					Title:           "冻结目标、非目标和范围",
					Description:     "在 main 频道收敛目标、非目标、核心约束和成功标准，形成第一版 scope 说明。",
					ChannelID:       "main",
					MilestoneID:     "scope-frozen",
					AssigneeAliases: []string{"proposer"},
					Status:          TaskStateOpen,
					Priority:        TaskPriorityHigh,
					Labels:          []string{"spec-package", "scope"},
				},
				{
					TaskID:          "review-scope-gaps-and-risks",
					Title:           "评审范围缺口和主要风险",
					Description:     "在 reviews 频道补齐 scope 缺口、歧义和高风险项，产出可进入 decision 的 review/risk 结论。",
					ChannelID:       "reviews",
					MilestoneID:     "scope-frozen",
					DependsOn:       []string{"scope-goals-and-nongoals"},
					AssigneeAliases: []string{"reviewer"},
					Status:          TaskStateOpen,
					Priority:        TaskPriorityHigh,
					Labels:          []string{"spec-package", "review"},
				},
				{
					TaskID:          "freeze-workflow-decisions",
					Title:           "冻结流程和关键取舍",
					Description:     "在 decisions 频道明确流程、边界和关键取舍，避免实现时重新讨论运行时行为。",
					ChannelID:       "decisions",
					MilestoneID:     "workflow-frozen",
					DependsOn:       []string{"review-scope-gaps-and-risks"},
					AssigneeAliases: []string{"owner", "editor"},
					Status:          TaskStateOpen,
					Priority:        TaskPriorityHigh,
					Labels:          []string{"spec-package", "workflow"},
				},
				{
					TaskID:          "write-data-model-spec",
					Title:           "完成数据模型规格",
					Description:     "在 artifacts 频道沉淀实体、状态、约束和字段说明，保证任何实现方都能按同一模型开发。",
					ChannelID:       "artifacts",
					MilestoneID:     "data-model-ready",
					DependsOn:       []string{"freeze-workflow-decisions"},
					AssigneeAliases: []string{"editor"},
					Status:          TaskStateOpen,
					Priority:        TaskPriorityMedium,
					Labels:          []string{"spec-package", "data-model"},
				},
				{
					TaskID:          "write-verification-spec",
					Title:           "完成验证与验收规格",
					Description:     "补齐最小验证集、验收动作和完成标准，让下游实现方能独立验证程序是否达标。",
					ChannelID:       "artifacts",
					MilestoneID:     "verification-ready",
					DependsOn:       []string{"write-data-model-spec"},
					AssigneeAliases: []string{"reviewer", "editor"},
					Status:          TaskStateOpen,
					Priority:        TaskPriorityMedium,
					Labels:          []string{"spec-package", "verification"},
				},
				{
					TaskID:          "freeze-spec-package",
					Title:           "冻结并交付规格包",
					Description:     "整理 README、product、workflow、data-model、api/runtime、verification 等 Markdown 规格，形成最终可交付包。",
					ChannelID:       "artifacts",
					MilestoneID:     "spec-package-ready",
					DependsOn:       []string{"write-verification-spec"},
					AssigneeAliases: []string{"owner", "editor"},
					Status:          TaskStateOpen,
					Priority:        TaskPriorityHigh,
					Labels:          []string{"spec-package", "delivery"},
				},
			},
		},
		{
			TemplateID:  "incident-response",
			Title:       "Incident Response",
			Description: "故障响应模板，包含分诊、时间线和恢复频道。",
			Channels: []Channel{
				{ChannelID: "main", Title: "Command"},
				{ChannelID: "timeline", Title: "Timeline"},
				{ChannelID: "recovery", Title: "Recovery"},
			},
			Policy: DefaultPolicy(),
			ChannelConfigs: []ChannelConfig{
				{ChannelID: "main", Plugin: "incident-room@1.0", Theme: "focus", AgentOnboarding: "聚焦事故分诊、阻塞项和恢复状态。"},
				{ChannelID: "timeline", Plugin: "handoff-room@1.0", Theme: "board", AgentOnboarding: "维护故障时间线、交接和 checkpoint。"},
			},
			RoleBindings: []TemplateRoleBinding{
				{Alias: "owner", Role: MemberRoleOwner, Status: MemberStatusActive},
				{Alias: "incident-commander", Role: MemberRoleMaintainer, Status: MemberStatusActive},
				{Alias: "observer", Role: MemberRoleObserver, Status: MemberStatusActive},
			},
			SeedMilestones: []Milestone{
				{MilestoneID: "stabilize", Title: "恢复稳定", Status: MilestoneStateOpen},
			},
		},
		{
			TemplateID:  "code-review",
			Title:       "Code Review",
			Description: "评审模板，包含 review 决策和产物沉淀频道。",
			Channels: []Channel{
				{ChannelID: "main", Title: "Review"},
				{ChannelID: "decisions", Title: "Decisions"},
			},
			Policy: DefaultPolicy(),
			ChannelConfigs: []ChannelConfig{
				{ChannelID: "main", Plugin: "review-room@1.0", Theme: "focus", AgentOnboarding: "沉淀 review、risk、decision，并尽快归并到任务。"},
				{ChannelID: "decisions", Plugin: "decision-room@1.0", Theme: "board", AgentOnboarding: "记录 proposal、option、decision 结论。"},
			},
			RoleBindings: []TemplateRoleBinding{
				{Alias: "owner", Role: MemberRoleOwner, Status: MemberStatusActive},
				{Alias: "reviewer", Role: MemberRoleMaintainer, Status: MemberStatusActive},
			},
			SeedMilestones: []Milestone{
				{MilestoneID: "review-ready", Title: "完成评审结论", Status: MilestoneStateOpen},
			},
		},
		{
			TemplateID:  "planning",
			Title:       "Planning",
			Description: "规划模板，包含方案交换、决策和产物沉淀频道。",
			Channels: []Channel{
				{ChannelID: "main", Title: "Planning"},
				{ChannelID: "decisions", Title: "Decisions"},
				{ChannelID: "artifacts", Title: "Artifacts"},
			},
			Policy: DefaultPolicy(),
			ChannelConfigs: []ChannelConfig{
				{ChannelID: "main", Plugin: "plan-exchange@1.0", Theme: "minimal", AgentOnboarding: "交换计划、方案和 skill/snippet。"},
				{ChannelID: "decisions", Plugin: "decision-room@1.0", Theme: "focus", AgentOnboarding: "把规划结论快速收成 decision。"},
				{ChannelID: "artifacts", Plugin: "artifact-room@1.0", Theme: "board", AgentOnboarding: "沉淀 proposal、revision、publish 产物。"},
			},
			RoleBindings: []TemplateRoleBinding{
				{Alias: "owner", Role: MemberRoleOwner, Status: MemberStatusActive},
				{Alias: "planner", Role: MemberRoleMaintainer, Status: MemberStatusActive},
			},
			SeedMilestones: []Milestone{
				{MilestoneID: "plan-approved", Title: "规划通过", Status: MilestoneStateOpen},
			},
		},
	}
}

func LookupBuiltinTeamTemplate(templateID string) (TeamTemplate, bool) {
	templateID = strings.TrimSpace(templateID)
	for _, item := range BuiltinTeamTemplates() {
		if item.TemplateID == templateID {
			return item, true
		}
	}
	return TeamTemplate{}, false
}

func (s *Store) CreateTeamFromTemplateCtx(ctx context.Context, info Info, templateID string, agentBindings map[string]string) (Info, TeamTemplate, error) {
	template, ok := LookupBuiltinTeamTemplate(templateID)
	if !ok {
		return Info{}, TeamTemplate{}, NewNotFoundError("template_id")
	}
	info = normalizeInfo(info)
	if info.TeamID == "" {
		return Info{}, TeamTemplate{}, NewEmptyIDError("team_id")
	}
	if info.Title == "" {
		info.Title = template.Title
	}
	if len(info.Channels) == 0 {
		info.Channels = make([]string, 0, len(template.Channels))
		for _, channel := range template.Channels {
			info.Channels = append(info.Channels, channel.ChannelID)
		}
	}
	if info.CreatedAt.IsZero() {
		info.CreatedAt = time.Now().UTC()
	}
	info.UpdatedAt = info.CreatedAt
	if err := s.SaveTeamCtx(ctx, info); err != nil {
		return Info{}, TeamTemplate{}, err
	}
	for _, channel := range template.Channels {
		if err := s.SaveChannelCtx(ctx, info.TeamID, channel); err != nil {
			return Info{}, TeamTemplate{}, err
		}
	}
	for _, cfg := range template.ChannelConfigs {
		if err := s.SaveChannelConfigCtx(ctx, info.TeamID, cfg); err != nil {
			return Info{}, TeamTemplate{}, err
		}
	}
	if err := s.SavePolicyCtx(ctx, info.TeamID, template.Policy); err != nil {
		return Info{}, TeamTemplate{}, err
	}
	members := make([]Member, 0, len(template.RoleBindings))
	resolvedAgents := map[string]string{}
	for _, binding := range template.RoleBindings {
		agentID := strings.TrimSpace(agentBindings[binding.Alias])
		if binding.Alias == "owner" && agentID == "" {
			agentID = info.OwnerAgentID
		}
		if agentID == "" {
			continue
		}
		resolvedAgents[binding.Alias] = agentID
		members = append(members, Member{
			AgentID:   agentID,
			Role:      binding.Role,
			Status:    binding.Status,
			JoinedAt:  time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		})
	}
	if len(members) > 0 {
		if err := s.SaveMembersCtx(ctx, info.TeamID, members); err != nil {
			return Info{}, TeamTemplate{}, err
		}
	}
	for _, milestone := range template.SeedMilestones {
		milestone.TeamID = info.TeamID
		if milestone.MilestoneID == "" {
			milestone.MilestoneID = NormalizeTeamID(fmt.Sprintf("%s-%s", template.TemplateID, milestone.Title))
		}
		if err := s.SaveMilestoneCtx(ctx, info.TeamID, milestone); err != nil {
			return Info{}, TeamTemplate{}, err
		}
	}
	for _, seed := range template.SeedTasks {
		taskID := strings.TrimSpace(seed.TaskID)
		if taskID == "" {
			taskID = NormalizeTeamID(fmt.Sprintf("%s-%s", template.TemplateID, seed.Title))
		}
		assignees := make([]string, 0, len(seed.AssigneeAliases))
		for _, alias := range seed.AssigneeAliases {
			agentID := strings.TrimSpace(resolvedAgents[alias])
			if agentID == "" {
				continue
			}
			assignees = append(assignees, agentID)
		}
		if err := s.AppendTaskCtx(ctx, info.TeamID, Task{
			TaskID:      taskID,
			TeamID:      info.TeamID,
			ChannelID:   seed.ChannelID,
			MilestoneID: seed.MilestoneID,
			DependsOn:   append([]string(nil), seed.DependsOn...),
			Title:       seed.Title,
			Description: seed.Description,
			CreatedBy:   strings.TrimSpace(info.OwnerAgentID),
			Assignees:   assignees,
			Status:      seed.Status,
			Priority:    seed.Priority,
			Labels:      append([]string(nil), seed.Labels...),
			CreatedAt:   info.CreatedAt,
			UpdatedAt:   info.CreatedAt,
		}); err != nil {
			return Info{}, TeamTemplate{}, err
		}
	}
	saved, err := s.LoadTeamCtx(ctx, info.TeamID)
	if err != nil {
		saved = info
	}
	return saved, template, nil
}
