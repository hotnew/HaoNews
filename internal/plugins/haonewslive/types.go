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
	SummaryStats []newsplugin.SummaryStat
}

type liveRoomPageData struct {
	Project        string
	Version        string
	PageNav        []newsplugin.NavItem
	NodeStatus     newsplugin.NodeStatus
	Now            time.Time
	Room           live.RoomInfo
	Events         []live.LiveMessage
	EventViews     []liveEventView
	TaskSummaries  []liveTaskSummaryView
	TaskByStatus   []liveTaskGroupView
	TaskByAssignee []liveTaskGroupView
	Roster         []live.RosterEntry
	Archive        *live.ArchiveRecord
}

type liveEventView struct {
	Type      string              `json:"type"`
	Timestamp string              `json:"timestamp"`
	Sender    string              `json:"sender"`
	Heading   string              `json:"heading"`
	Note      string              `json:"note,omitempty"`
	Fields    []liveFieldView     `json:"fields,omitempty"`
	Task      *liveTaskUpdateView `json:"task,omitempty"`
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
