package haonewslive

import (
	"time"

	"hao.news/internal/haonews/live"
	newsplugin "hao.news/internal/plugins/haonews"
)

type liveIndexPageData struct {
	Project      string
	Version      string
	PageNav      []newsplugin.NavItem
	NodeStatus   newsplugin.NodeStatus
	Now          time.Time
	Rooms        []live.RoomSummary
	RoomLinks    map[string]liveRoomLinks
	PendingCount int
	SummaryStats []newsplugin.SummaryStat
}

type liveArchiveIndexPageData struct {
	Project      string
	Version      string
	PageNav      []newsplugin.NavItem
	NodeStatus   newsplugin.NodeStatus
	Now          time.Time
	Rooms        []liveArchiveRoomSummary
	SummaryStats []newsplugin.SummaryStat
}

type livePublicModerationPageData struct {
	Project                      string
	Version                      string
	PageNav                      []newsplugin.NavItem
	NodeStatus                   newsplugin.NodeStatus
	Now                          time.Time
	SaveOK                       bool
	SaveError                    string
	MutedOriginPublicKeys        []string
	MutedParentPublicKeys        []string
	PublicRateLimitMessages      int
	PublicRateLimitWindowSeconds int
	SummaryStats                 []newsplugin.SummaryStat
}

type livePendingIndexPageData struct {
	Project      string
	Version      string
	PageNav      []newsplugin.NavItem
	NodeStatus   newsplugin.NodeStatus
	Now          time.Time
	Rooms        []livePendingRoomSummary
	SummaryStats []newsplugin.SummaryStat
}

type liveRoomPageData struct {
	Project                      string
	Version                      string
	PageNav                      []newsplugin.NavItem
	NodeStatus                   newsplugin.NodeStatus
	Now                          time.Time
	Room                         live.RoomInfo
	RoomLinks                    liveRoomLinks
	RoomVisibility               string
	PublicHintTitle              string
	PublicHintBody               string
	PublicHintExample            string
	PublicGenerator              bool
	PublicMutedEvents            int
	PublicRateLimitedEvents      int
	PublicRateLimitMessages      int
	PublicRateLimitWindowSeconds int
	PublicDefaultRooms           []livePublicRoomEntry
	PendingBlockedEvents         int
	Events                       []live.LiveMessage
	EventViews                   []liveEventView
	TaskSummaries                []liveTaskSummaryView
	TaskByStatus                 []liveTaskGroupView
	TaskByAssignee               []liveTaskGroupView
	Roster                       []live.RosterEntry
	Archive                      *live.ArchiveRecord
	HistoryArchives              []live.RoomHistoryArchive
	ShowAll                      bool
	VisibleEventCount            int
	TotalEventCount              int
	ShowHeartbeats               bool
	AutoRefresh                  bool
	ArchiveCreated               bool
	ArchiveError                 string
}

type liveRoomHistoryPageData struct {
	Project         string
	Version         string
	PageNav         []newsplugin.NavItem
	NodeStatus      newsplugin.NodeStatus
	Now             time.Time
	Room            live.RoomInfo
	RoomLinks       liveRoomLinks
	Archive         *live.RoomHistoryArchive
	Archives        []live.RoomHistoryArchive
	EventViews      []liveEventView
	ArchiveNotFound bool
}

type livePublicRoomEntry struct {
	Name        string
	Slug        string
	Description string
	RoomURL     string
	APIURL      string
}

type livePendingRoomPageData struct {
	Project           string
	Version           string
	PageNav           []newsplugin.NavItem
	NodeStatus        newsplugin.NodeStatus
	Now               time.Time
	Room              live.RoomInfo
	RoomLinks         liveRoomLinks
	RoomVisibility    string
	BlockedEvents     []live.LiveMessage
	EventViews        []liveEventView
	BlockedEventCount int
	ShowHeartbeats    bool
}

type livePendingRoomSummary struct {
	RoomID            string              `json:"room_id"`
	Title             string              `json:"title"`
	Creator           string              `json:"creator"`
	CreatedAt         time.Time           `json:"created_at"`
	LastEventAt       time.Time           `json:"last_event_at,omitempty"`
	Channel           string              `json:"channel,omitempty"`
	Archive           *live.ArchiveRecord `json:"archive,omitempty"`
	RoomVisibility    string              `json:"room_visibility,omitempty"`
	BlockedEventCount int                 `json:"blocked_event_count"`
	BlockedReason     string              `json:"blocked_reason,omitempty"`
	PendingURL        string              `json:"pending_url,omitempty"`
	APIURL            string              `json:"api_url,omitempty"`
}

type liveRoomLinks struct {
	RoomURL       string
	APIURL        string
	PendingURL    string
	HistoryURL    string
	APIHistoryURL string
	ArchiveNowURL string
	APIArchiveURL string
}

type liveNetConfigSummary struct {
	Path         string   `json:"path"`
	Exists       bool     `json:"exists"`
	NetworkMode  string   `json:"network_mode,omitempty"`
	Listen       []string `json:"listen,omitempty"`
	ListenPort   int      `json:"listen_port,omitempty"`
	LANPeers     []string `json:"lan_peers,omitempty"`
	PublicPeers  []string `json:"public_peers,omitempty"`
	RelayPeers   []string `json:"relay_peers,omitempty"`
	RedisEnabled bool     `json:"redis_enabled"`
}

type liveIdentitySummary struct {
	IdentityFile string `json:"identity_file,omitempty"`
	AgentID      string `json:"agent_id,omitempty"`
	Author       string `json:"author,omitempty"`
	PublicKey    string `json:"public_key,omitempty"`
	KeyType      string `json:"key_type,omitempty"`
	Known        bool   `json:"known,omitempty"`
}

type liveArchiveStats struct {
	ArchiveCount        int    `json:"archive_count"`
	LatestArchiveID     string `json:"latest_archive_id,omitempty"`
	LatestArchiveKind   string `json:"latest_archive_kind,omitempty"`
	LatestArchiveLabel  string `json:"latest_archive_label,omitempty"`
	LatestArchiveAt     string `json:"latest_archive_at,omitempty"`
	LatestArchiveStart  string `json:"latest_archive_start_at,omitempty"`
	LatestArchiveEnd    string `json:"latest_archive_end_at,omitempty"`
	LatestArchiveEvents int    `json:"latest_archive_events,omitempty"`
	LatestArchiveMsgs   int    `json:"latest_archive_message_count,omitempty"`
	LatestArchiveHBs    int    `json:"latest_archive_heartbeat_count,omitempty"`
}

type liveRoomStatusView struct {
	Room                 live.RoomInfo             `json:"room"`
	RoomLinks            liveRoomLinks             `json:"room_links"`
	Watcher              *live.BootstrapStatus     `json:"watcher,omitempty"`
	WatcherPeerID        string                    `json:"watcher_peer_id,omitempty"`
	WatcherListenPort    int                       `json:"watcher_listen_port,omitempty"`
	SenderConfig         liveNetConfigSummary      `json:"sender_config"`
	SenderIdentity       liveIdentitySummary       `json:"sender_identity"`
	SenderPeerID         string                    `json:"sender_peer_id,omitempty"`
	SenderListenPort     int                       `json:"sender_listen_port,omitempty"`
	RoomFilePath         string                    `json:"room_file_path,omitempty"`
	RoomFileModTime      string                    `json:"room_file_mod_time,omitempty"`
	EventsFileModTime    string                    `json:"events_file_mod_time,omitempty"`
	ArchiveFileModTime   string                    `json:"archive_file_mod_time,omitempty"`
	HistoryDirModTime    string                    `json:"history_dir_mod_time,omitempty"`
	VisibleEventCount    int                       `json:"visible_event_count"`
	TotalEventCount      int                       `json:"total_event_count"`
	LatestEventAt        string                    `json:"latest_event_at,omitempty"`
	LatestVisibleAt      string                    `json:"latest_visible_at,omitempty"`
	LatestNonHeartbeatAt string                    `json:"latest_non_heartbeat_at,omitempty"`
	LatestLocalWriteAt   string                    `json:"latest_local_write_at,omitempty"`
	LatestCacheRefreshAt string                    `json:"latest_cache_refresh_at,omitempty"`
	LatestArchiveAt      string                    `json:"latest_archive_at,omitempty"`
	Archive              *live.ArchiveRecord       `json:"archive,omitempty"`
	ArchiveStats         liveArchiveStats          `json:"archive_stats"`
	HistoryArchives      []live.RoomHistoryArchive `json:"history_archives,omitempty"`
}

type liveRoomStatusPageData struct {
	Project         string
	Version         string
	PageNav         []newsplugin.NavItem
	NodeStatus      newsplugin.NodeStatus
	Now             time.Time
	Status          liveRoomStatusView
	SummaryStats    []newsplugin.SummaryStat
	RoomLinks       liveRoomLinks
	Archive         *live.ArchiveRecord
	Room            live.RoomInfo
	HistoryArchives []live.RoomHistoryArchive
	ShowAll         bool
	ShowHeartbeats  bool
	AutoRefresh     bool
}

type liveArchiveRoomSummary struct {
	Room          live.RoomInfo
	RoomLinks     liveRoomLinks
	ArchiveCount  int
	LastArchived  string
	LatestArchive *live.RoomHistoryArchive
	ArchiveStats  liveArchiveStats
}

type liveEventView struct {
	Type         string              `json:"type"`
	Timestamp    string              `json:"timestamp"`
	Sender       string              `json:"sender"`
	Visibility   string              `json:"live_visibility,omitempty"`
	Heading      string              `json:"heading"`
	HeadingLines []string            `json:"heading_lines,omitempty"`
	Note         string              `json:"note,omitempty"`
	Fields       []liveFieldView     `json:"fields,omitempty"`
	Task         *liveTaskUpdateView `json:"task,omitempty"`
}

type liveFieldView struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type liveTaskUpdateView struct {
	TaskID      string `json:"task_id,omitempty"`
	Status      string `json:"status,omitempty"`
	Description string `json:"description,omitempty"`
	AssignedTo  string `json:"assigned_to,omitempty"`
	Progress    string `json:"progress,omitempty"`
}

type liveTaskSummaryView struct {
	TaskID        string `json:"task_id"`
	Status        string `json:"status,omitempty"`
	Description   string `json:"description,omitempty"`
	AssignedTo    string `json:"assigned_to,omitempty"`
	Progress      string `json:"progress,omitempty"`
	UpdateCount   int    `json:"update_count"`
	LastUpdatedAt string `json:"last_updated_at,omitempty"`
	LastSender    string `json:"last_sender,omitempty"`
}

type liveTaskGroupView struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}
