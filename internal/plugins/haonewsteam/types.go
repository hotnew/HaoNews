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
	Digests      []teamActivityDigest
	SummaryStats []newsplugin.SummaryStat
}

type teamActivityDigest struct {
	TeamID              string
	Title               string
	Description         string
	Visibility          string
	MemberCount         int
	ChannelCount        int
	RecentMessages      int
	OpenTasks           int
	UnresolvedConflicts int
	WebhookDeadLetters  int
	LastActivityAt      time.Time
	TopAction           string
	TopActionURL        string
	HealthLabel         string
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
	WebhookStatus       teamcore.WebhookDeliveryStatus
	TaskStatusCounts    map[string]int
	ArtifactKindCounts  map[string]int
	DefaultActorAgentID string
	CanQuickPost        bool
	PolicyNotice        string
	FocusTasks          []teamTaskFocusItem
	RecentMessageItems  []teamMessagePreview
	RecentChangeItems   []teamChangePreview
	DashboardAlerts     []string
	RoomEntries         []teamRoomEntry
	AvailableRoomThemes []teamRoomThemeSummary
	SummaryStats        []newsplugin.SummaryStat
}

type teamTaskFocusItem struct {
	TaskID        string
	Title         string
	Status        string
	Priority      string
	DueAt         time.Time
	DueLabel      string
	ChannelID     string
	Assignees     []string
	ArtifactCount int
	HistoryCount  int
}

type teamMessagePreview struct {
	MessageID     string
	ChannelID     string
	AuthorAgentID string
	Content       string
	CreatedAt     time.Time
}

type teamChangePreview struct {
	EventID    string
	Scope      string
	Action     string
	Summary    string
	SubjectID  string
	ActorAgent string
	CreatedAt  time.Time
}

type teamChannelConfigSummary struct {
	ChannelID       string    `json:"channel_id"`
	Plugin          string    `json:"plugin,omitempty"`
	PluginID        string    `json:"plugin_id,omitempty"`
	Theme           string    `json:"theme,omitempty"`
	AgentOnboarding string    `json:"agent_onboarding,omitempty"`
	Rules           []string  `json:"rules,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
}

type teamRoomEntry struct {
	ChannelID           string    `json:"channel_id"`
	Plugin              string    `json:"plugin,omitempty"`
	PluginID            string    `json:"plugin_id,omitempty"`
	Theme               string    `json:"theme,omitempty"`
	Configured          bool      `json:"configured"`
	ChannelPath         string    `json:"channel_path,omitempty"`
	RoomWebPath         string    `json:"room_web_path,omitempty"`
	RoomAPIPath         string    `json:"room_api_path,omitempty"`
	ConfigAPIPath       string    `json:"config_api_path,omitempty"`
	AgentOnboarding     string    `json:"agent_onboarding,omitempty"`
	RuleCount           int       `json:"rule_count"`
	UpdatedAt           time.Time `json:"updated_at,omitempty"`
	ChannelTitle        string    `json:"channel_title,omitempty"`
	ChannelDescription  string    `json:"channel_description,omitempty"`
	ChannelHidden       bool      `json:"channel_hidden,omitempty"`
	ChannelMessageCount int       `json:"channel_message_count,omitempty"`
}

type teamRoomThemeSummary struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description,omitempty"`
	Overrides    []string `json:"overrides,omitempty"`
	PreviewClass string   `json:"preview_class,omitempty"`
}

type teamRoomPluginSummary struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	ConfigValue   string   `json:"config_value"`
	Description   string   `json:"description,omitempty"`
	MinTeamVer    string   `json:"min_team_version,omitempty"`
	MessageKinds  []string `json:"message_kinds,omitempty"`
	ArtifactKinds []string `json:"artifact_kinds,omitempty"`
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
	Record             corehaonews.TeamSyncConflictRecord `json:"record"`
	AllowAcceptRemote  bool                               `json:"allow_accept_remote"`
	AllowKeepLocal     bool                               `json:"allow_keep_local"`
	SuggestedAction    string                             `json:"suggested_action,omitempty"`
	ReasonLabel        string                             `json:"reason_label,omitempty"`
	ActionHint         string                             `json:"action_hint,omitempty"`
	SubjectLabel       string                             `json:"subject_label,omitempty"`
	ConflictClass      string                             `json:"conflict_class,omitempty"`
	SeverityLabel      string                             `json:"severity_label,omitempty"`
	ConsequenceHint    string                             `json:"consequence_hint,omitempty"`
	LocalVersionLabel  string                             `json:"local_version_label,omitempty"`
	RemoteVersionLabel string                             `json:"remote_version_label,omitempty"`
	Actions            []teamSyncConflictActionView       `json:"actions,omitempty"`
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
	Project               string
	Version               string
	PageNav               []newsplugin.NavItem
	NodeStatus            newsplugin.NodeStatus
	Now                   time.Time
	Team                  teamcore.Info
	SyncNotice            string
	SyncStatus            corehaonews.SyncTeamSyncStatus
	WebhookStatus         teamcore.WebhookDeliveryStatus
	RecentConflicts       []corehaonews.TeamSyncConflictRecord
	ConflictViews         []teamSyncConflictView
	OpenConflictViews     []teamSyncConflictView
	ResolvedConflictViews []teamSyncConflictView
	StatusGroups          []teamSyncStatusGroup
	HealthLevel           string
	HealthTitle           string
	HealthHint            string
	ResolvedTitle         string
	ResolvedHint          string
	SummaryStats          []newsplugin.SummaryStat
}

type teamWebhookPageData struct {
	Project          string
	Version          string
	PageNav          []newsplugin.NavItem
	NodeStatus       newsplugin.NodeStatus
	Now              time.Time
	Team             teamcore.Info
	Webhooks         []teamcore.PushNotificationConfig
	WebhookStatus    teamcore.WebhookDeliveryStatus
	RecentDeliveries []teamcore.WebhookDeliveryRecord
	ReplayNotice     string
}

type teamA2AEndpointInfo struct {
	Method      string
	Path        string
	Description string
}

type teamA2APageData struct {
	Project      string
	Version      string
	PageNav      []newsplugin.NavItem
	NodeStatus   newsplugin.NodeStatus
	Now          time.Time
	Team         teamcore.Info
	Agents       []teamcore.AgentCard
	Tasks        []teamcore.Task
	Endpoints    []teamA2AEndpointInfo
	SummaryStats []newsplugin.SummaryStat
}

type teamSearchPageData struct {
	Project       string
	Version       string
	PageNav       []newsplugin.NavItem
	NodeStatus    newsplugin.NodeStatus
	Now           time.Time
	Team          teamcore.Info
	Query         string
	Scope         string
	ScopeOptions  []teamSearchScopeOption
	Sections      []teamSearchSectionView
	SummaryStats  []newsplugin.SummaryStat
	SearchTips    []string
	ResultSummary string
}

type teamSearchScopeOption struct {
	Value  string `json:"value"`
	Label  string `json:"label"`
	Active bool   `json:"active"`
	Count  int    `json:"count,omitempty"`
}

type teamSearchSectionView struct {
	Key     string                 `json:"key"`
	Title   string                 `json:"title"`
	Hint    string                 `json:"hint,omitempty"`
	Count   int                    `json:"count"`
	Results []teamSearchResultView `json:"results,omitempty"`
}

type teamSearchResultView struct {
	Kind      string    `json:"kind"`
	ID        string    `json:"id,omitempty"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary,omitempty"`
	URL       string    `json:"url"`
	Meta      string    `json:"meta,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

type teamChannelPageData struct {
	Project              string
	Version              string
	PageNav              []newsplugin.NavItem
	NodeStatus           newsplugin.NodeStatus
	Now                  time.Time
	Team                 teamcore.Info
	Channel              teamcore.ChannelSummary
	ChannelID            string
	ViewMode             string
	Channels             []teamcore.ChannelSummary
	Messages             []teamcore.Message
	Tasks                []teamcore.Task
	Artifacts            []teamcore.Artifact
	ChannelConfig        teamcore.ChannelConfig
	RoomEntry            teamRoomEntry
	CurrentRoomPlugin    teamRoomPluginSummary
	CurrentRoomTheme     teamRoomThemeSummary
	AvailableRoomPlugins []teamRoomPluginSummary
	AvailableRoomThemes  []teamRoomThemeSummary
	ConfigNotice         string
	RelatedHistory       []teamcore.ChangeEvent
	SummaryStats         []newsplugin.SummaryStat
}

type teamTasksPageData struct {
	Project             string
	Version             string
	PageNav             []newsplugin.NavItem
	NodeStatus          newsplugin.NodeStatus
	Now                 time.Time
	Team                teamcore.Info
	Tasks               []teamcore.Task
	ArtifactCounts      map[string]int
	HistoryCounts       map[string]int
	FilterStatus        string
	FilterAssignee      string
	FilterLabel         string
	FilterChannel       string
	AppliedFilters      []string
	Statuses            []string
	Assignees           []string
	Labels              []string
	Channels            []teamcore.ChannelSummary
	DefaultActorAgentID string
	OverdueCount        int
	DueSoonCount        int
	MyOpenTaskCount     int
	TaskLanes           []teamTaskLane
	SummaryStats        []newsplugin.SummaryStat
}

type teamTaskLane struct {
	Key   string
	Title string
	Hint  string
	Count int
	Tasks []teamcore.Task
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
