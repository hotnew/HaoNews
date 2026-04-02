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
	RoomURL    string
	APIURL     string
	PendingURL string
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
