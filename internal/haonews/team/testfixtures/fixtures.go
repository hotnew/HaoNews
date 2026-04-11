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
	now := time.Now().UTC()
	return &TeamBuilder{info: team.Info{
		TeamID:     id,
		Slug:       id,
		Title:      "Test Team " + id,
		Visibility: "team",
		CreatedAt:  now,
		UpdatedAt:  now,
	}}
}

func (b *TeamBuilder) WithOwner(agentID string) *TeamBuilder {
	b.info.OwnerAgentID = agentID
	return b
}

func (b *TeamBuilder) Build() team.Info {
	return b.info
}

// MemberBuilder 构建测试用 Member。
type MemberBuilder struct {
	member team.Member
}

func NewMember(agentID string) *MemberBuilder {
	now := time.Now().UTC()
	return &MemberBuilder{member: team.Member{
		AgentID:   agentID,
		Role:      team.MemberRoleMember,
		Status:    team.MemberStatusActive,
		JoinedAt:  now,
		UpdatedAt: now,
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

func (b *MemberBuilder) Build() team.Member {
	return b.member
}

// TaskBuilder 构建测试用 Task。
type TaskBuilder struct {
	task team.Task
}

func NewTask(id, teamID, title string) *TaskBuilder {
	now := time.Now().UTC()
	return &TaskBuilder{task: team.Task{
		TaskID:    id,
		TeamID:    teamID,
		Title:     title,
		Status:    team.TaskStateOpen,
		Priority:  team.TaskPriorityMedium,
		CreatedAt: now,
		UpdatedAt: now,
	}}
}

func (b *TaskBuilder) WithStatus(status string) *TaskBuilder {
	b.task.Status = status
	return b
}

func (b *TaskBuilder) WithAssignees(agents ...string) *TaskBuilder {
	b.task.Assignees = append([]string(nil), agents...)
	return b
}

func (b *TaskBuilder) Build() team.Task {
	return b.task
}

type StandardScenario struct {
	Team    team.Info
	Members []team.Member
	Tasks   []team.Task
}

func StandardTeamScenario(teamID string) StandardScenario {
	return StandardScenario{
		Team: NewTeam(teamID).WithOwner("agent-owner").Build(),
		Members: []team.Member{
			NewMember("agent-owner").WithRole(team.MemberRoleOwner).Build(),
			NewMember("agent-member-1").Build(),
			NewMember("agent-member-2").Build(),
		},
		Tasks: []team.Task{
			NewTask("task-open", teamID, "Open task").Build(),
			NewTask("task-doing", teamID, "Doing task").WithStatus(team.TaskStateDoing).Build(),
			NewTask("task-done", teamID, "Done task").WithStatus(team.TaskStateDone).Build(),
		},
	}
}
