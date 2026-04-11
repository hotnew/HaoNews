package testfixtures

import (
	"testing"

	team "hao.news/internal/haonews/team"
)

func TestStandardTeamScenario(t *testing.T) {
	t.Parallel()

	scenario := StandardTeamScenario("fixture-team")
	if scenario.Team.TeamID != "fixture-team" || scenario.Team.OwnerAgentID != "agent-owner" {
		t.Fatalf("unexpected team = %#v", scenario.Team)
	}
	if len(scenario.Members) != 3 {
		t.Fatalf("members = %d, want 3", len(scenario.Members))
	}
	if scenario.Members[0].Role != team.MemberRoleOwner {
		t.Fatalf("owner role = %q, want %q", scenario.Members[0].Role, team.MemberRoleOwner)
	}
	if len(scenario.Tasks) != 3 {
		t.Fatalf("tasks = %d, want 3", len(scenario.Tasks))
	}
	if scenario.Tasks[1].Status != team.TaskStateDoing || scenario.Tasks[2].Status != team.TaskStateDone {
		t.Fatalf("unexpected task statuses = %#v", scenario.Tasks)
	}
}
