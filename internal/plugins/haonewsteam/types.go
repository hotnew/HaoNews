package haonewsteam

import (
	"time"

	corehaonews "hao.news/internal/haonews"
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
	Project             string
	Version             string
	PageNav             []newsplugin.NavItem
	NodeStatus          newsplugin.NodeStatus
	Now                 time.Time
	Team                teamcore.Info
	Policy              teamcore.Policy
	Members             []teamcore.Member
	ActiveMembers       []teamcore.Member
	PendingMembers      []teamcore.Member
	MutedMembers        []teamcore.Member
	RemovedMembers      []teamcore.Member
	Owners              []teamcore.Member
	Maintainers         []teamcore.Member
	Observers           []teamcore.Member
	Messages            []teamcore.Message
	Tasks               []teamcore.Task
	Channels            []teamcore.ChannelSummary
	Artifacts           []teamcore.Artifact
	History             []teamcore.ChangeEvent
	RecentConflicts     []corehaonews.TeamSyncConflictRecord
	UnresolvedConflicts int
	ResolvedConflicts   int
	TaskStatusCounts    map[string]int
	ArtifactKindCounts  map[string]int
	SummaryStats        []newsplugin.SummaryStat
}

type teamMembersPageData struct {
	Project        string
	Version        string
	PageNav        []newsplugin.NavItem
	NodeStatus     newsplugin.NodeStatus
	Now            time.Time
	Team           teamcore.Info
	Policy         teamcore.Policy
	Members        []teamcore.Member
	PendingMembers []teamcore.Member
	FilterStatus   string
	FilterRole     string
	FilterAgent    string
	AppliedFilters []string
	Statuses       []string
	Roles          []string
	StatusCounts   map[string]int
	RoleCounts     map[string]int
	SummaryStats   []newsplugin.SummaryStat
}

type teamHistoryPageData struct {
	Project             string
	Version             string
	PageNav             []newsplugin.NavItem
	NodeStatus          newsplugin.NodeStatus
	Now                 time.Time
	Team                teamcore.Info
	History             []teamcore.ChangeEvent
	FilterScope         string
	FilterSource        string
	FilterActor         string
	AppliedFilters      []string
	Scopes              []string
	Sources             []string
	RecentConflicts     []corehaonews.TeamSyncConflictRecord
	UnresolvedConflicts int
	ResolvedConflicts   int
	ScopeCounts         map[string]int
	SourceCounts        map[string]int
	SummaryStats        []newsplugin.SummaryStat
}

type teamSyncConflictView struct {
	Record            corehaonews.TeamSyncConflictRecord `json:"record"`
	AllowAcceptRemote bool                               `json:"allow_accept_remote"`
	AllowKeepLocal    bool                               `json:"allow_keep_local"`
	SuggestedAction   string                             `json:"suggested_action,omitempty"`
	ReasonLabel       string                             `json:"reason_label,omitempty"`
	ActionHint        string                             `json:"action_hint,omitempty"`
	SubjectLabel      string                             `json:"subject_label,omitempty"`
	ConflictClass     string                             `json:"conflict_class,omitempty"`
	Actions           []teamSyncConflictActionView       `json:"actions,omitempty"`
}

type teamSyncConflictActionView struct {
	Value   string `json:"value"`
	Label   string `json:"label"`
	Primary bool   `json:"primary,omitempty"`
}

type teamSyncMetricValue struct {
	Label string
	Value string
}

type teamSyncStatusGroup struct {
	Title    string
	Subtitle string
	Metrics  []teamSyncMetricValue
	Details  []string
}

type teamSyncPageData struct {
	Project         string
	Version         string
	PageNav         []newsplugin.NavItem
	NodeStatus      newsplugin.NodeStatus
	Now             time.Time
	Team            teamcore.Info
	SyncNotice      string
	SyncStatus      corehaonews.SyncTeamSyncStatus
	WebhookStatus   teamcore.WebhookDeliveryStatus
	RecentConflicts []corehaonews.TeamSyncConflictRecord
	ConflictViews   []teamSyncConflictView
	StatusGroups    []teamSyncStatusGroup
	SummaryStats    []newsplugin.SummaryStat
}

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

type teamTasksPageData struct {
	Project        string
	Version        string
	PageNav        []newsplugin.NavItem
	NodeStatus     newsplugin.NodeStatus
	Now            time.Time
	Team           teamcore.Info
	Tasks          []teamcore.Task
	ArtifactCounts map[string]int
	HistoryCounts  map[string]int
	FilterStatus   string
	FilterAssignee string
	FilterLabel    string
	FilterChannel  string
	AppliedFilters []string
	Statuses       []string
	Assignees      []string
	Labels         []string
	Channels       []teamcore.ChannelSummary
	SummaryStats   []newsplugin.SummaryStat
}

type teamTaskPageData struct {
	Project            string
	Version            string
	PageNav            []newsplugin.NavItem
	NodeStatus         newsplugin.NodeStatus
	Now                time.Time
	Team               teamcore.Info
	Task               teamcore.Task
	Tasks              []teamcore.Task
	Channels           []teamcore.ChannelSummary
	Messages           []teamcore.Message
	Artifacts          []teamcore.Artifact
	RelatedChannel     *teamcore.ChannelSummary
	RelatedHistory     []teamcore.ChangeEvent
	DefaultCommentType string
	DefaultChannelID   string
	SummaryStats       []newsplugin.SummaryStat
}

type teamArtifactsPageData struct {
	Project        string
	Version        string
	PageNav        []newsplugin.NavItem
	NodeStatus     newsplugin.NodeStatus
	Now            time.Time
	Team           teamcore.Info
	Artifacts      []teamcore.Artifact
	FilterKind     string
	FilterChannel  string
	FilterTask     string
	AppliedFilters []string
	Kinds          []string
	Channels       []teamcore.ChannelSummary
	Tasks          []teamcore.Task
	SummaryStats   []newsplugin.SummaryStat
}

type teamArtifactPageData struct {
	Project        string
	Version        string
	PageNav        []newsplugin.NavItem
	NodeStatus     newsplugin.NodeStatus
	Now            time.Time
	Team           teamcore.Info
	Artifact       teamcore.Artifact
	Artifacts      []teamcore.Artifact
	RelatedTask    *teamcore.Task
	RelatedChannel *teamcore.ChannelSummary
	RelatedHistory []teamcore.ChangeEvent
	SummaryStats   []newsplugin.SummaryStat
}

type teamArchiveIndexPageData struct {
	Project      string
	Version      string
	PageNav      []newsplugin.NavItem
	NodeStatus   newsplugin.NodeStatus
	Now          time.Time
	Teams        []teamcore.Summary
	SummaryStats []newsplugin.SummaryStat
}

type teamArchivePageData struct {
	Project      string
	Version      string
	PageNav      []newsplugin.NavItem
	NodeStatus   newsplugin.NodeStatus
	Now          time.Time
	Team         teamcore.Info
	Archives     []teamcore.ArchiveSnapshot
	Archive      *teamcore.ArchiveSnapshot
	SummaryStats []newsplugin.SummaryStat
}
