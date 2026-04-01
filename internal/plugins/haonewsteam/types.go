package haonewsteam

import (
	"time"

	teamcore "hao.news/internal/haonews/team"
	newsplugin "hao.news/internal/plugins/haonews"
)

type teamIndexPageData struct {
	Project      string
	Version      string
	PageNav      []newsplugin.NavItem
	NodeStatus   newsplugin.NodeStatus
	Now          time.Time
	Teams        []teamcore.Summary
	SummaryStats []newsplugin.SummaryStat
}

type teamPageData struct {
	Project        string
	Version        string
	PageNav        []newsplugin.NavItem
	NodeStatus     newsplugin.NodeStatus
	Now            time.Time
	Team           teamcore.Info
	Policy         teamcore.Policy
	Members        []teamcore.Member
	PendingMembers []teamcore.Member
	Messages       []teamcore.Message
	Tasks          []teamcore.Task
	Channels       []teamcore.ChannelSummary
	Artifacts      []teamcore.Artifact
	History        []teamcore.ChangeEvent
	SummaryStats   []newsplugin.SummaryStat
}

type teamHistoryPageData struct {
	Project      string
	Version      string
	PageNav      []newsplugin.NavItem
	NodeStatus   newsplugin.NodeStatus
	Now          time.Time
	Team         teamcore.Info
	History      []teamcore.ChangeEvent
	FilterScope  string
	FilterSource string
	FilterActor  string
	Scopes       []string
	Sources      []string
	SummaryStats []newsplugin.SummaryStat
}

type teamChannelPageData struct {
	Project      string
	Version      string
	PageNav      []newsplugin.NavItem
	NodeStatus   newsplugin.NodeStatus
	Now          time.Time
	Team         teamcore.Info
	Channel      teamcore.ChannelSummary
	ChannelID    string
	Channels     []teamcore.ChannelSummary
	Messages     []teamcore.Message
	SummaryStats []newsplugin.SummaryStat
}

type teamTasksPageData struct {
	Project      string
	Version      string
	PageNav      []newsplugin.NavItem
	NodeStatus   newsplugin.NodeStatus
	Now          time.Time
	Team         teamcore.Info
	Tasks        []teamcore.Task
	SummaryStats []newsplugin.SummaryStat
}

type teamTaskPageData struct {
	Project      string
	Version      string
	PageNav      []newsplugin.NavItem
	NodeStatus   newsplugin.NodeStatus
	Now          time.Time
	Team         teamcore.Info
	Task         teamcore.Task
	Tasks        []teamcore.Task
	Messages     []teamcore.Message
	SummaryStats []newsplugin.SummaryStat
}

type teamArtifactsPageData struct {
	Project      string
	Version      string
	PageNav      []newsplugin.NavItem
	NodeStatus   newsplugin.NodeStatus
	Now          time.Time
	Team         teamcore.Info
	Artifacts    []teamcore.Artifact
	SummaryStats []newsplugin.SummaryStat
}

type teamArtifactPageData struct {
	Project      string
	Version      string
	PageNav      []newsplugin.NavItem
	NodeStatus   newsplugin.NodeStatus
	Now          time.Time
	Team         teamcore.Info
	Artifact     teamcore.Artifact
	Artifacts    []teamcore.Artifact
	SummaryStats []newsplugin.SummaryStat
}
