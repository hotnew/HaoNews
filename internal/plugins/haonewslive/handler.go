package haonewslive

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"hao.news/internal/haonews/live"
	newsplugin "hao.news/internal/plugins/haonews"
)

func handleLiveIndex(app *newsplugin.App, store *live.LocalStore, w http.ResponseWriter, r *http.Request) {
	rooms, err := store.ListRooms()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := liveIndexPageData{
		Project:    app.ProjectName(),
		Version:    app.VersionString(),
		PageNav:    app.PageNav("/live"),
		NodeStatus: app.NodeStatus(index),
		Now:        time.Now(),
		Rooms:      rooms,
		SummaryStats: []newsplugin.SummaryStat{
			{Label: "房间数", Value: formatCount(len(rooms))},
			{Label: "在线房间", Value: formatCount(countActiveRooms(rooms))},
			{Label: "已归档", Value: formatCount(countArchivedRooms(rooms))},
			{Label: "最近更新", Value: latestRoomValue(rooms)},
		},
	}
	if err := app.Templates().ExecuteTemplate(w, "live.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleLiveRoom(app *newsplugin.App, store *live.LocalStore, roomID string, w http.ResponseWriter, r *http.Request) {
	room, err := store.LoadRoom(roomID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such file") {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	events, err := store.ReadEvents(roomID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	archive, err := store.LoadArchiveResult(roomID)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	index, err := app.Index()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	taskSummaries := buildTaskSummaries(events)
	data := liveRoomPageData{
		Project:        app.ProjectName(),
		Version:        app.VersionString(),
		PageNav:        app.PageNav("/live"),
		NodeStatus:     app.NodeStatus(index),
		Now:            time.Now(),
		Room:           room,
		Events:         events,
		EventViews:     buildEventViews(events),
		TaskSummaries:  taskSummaries,
		TaskByStatus:   groupTasksByStatus(taskSummaries),
		TaskByAssignee: groupTasksByAssignee(taskSummaries),
		Roster:         live.BuildRoster(events, time.Now().UTC(), 30*time.Second),
		Archive:        archive,
	}
	if err := app.Templates().ExecuteTemplate(w, "live_room.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleAPILiveRooms(store *live.LocalStore, w http.ResponseWriter, r *http.Request) {
	rooms, err := store.ListRooms()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if rooms == nil {
		rooms = []live.RoomSummary{}
	}
	newsplugin.WriteJSON(w, http.StatusOK, rooms)
}

func handleAPILiveRoom(store *live.LocalStore, roomID string, w http.ResponseWriter, r *http.Request) {
	room, err := store.LoadRoom(roomID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	events, err := store.ReadEvents(roomID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	archive, err := store.LoadArchiveResult(roomID)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	taskSummaries := buildTaskSummaries(events)
	newsplugin.WriteJSON(w, http.StatusOK, map[string]any{
		"room":             room,
		"events":           events,
		"event_views":      buildEventViews(events),
		"task_summaries":   taskSummaries,
		"task_by_status":   groupTasksByStatus(taskSummaries),
		"task_by_assignee": groupTasksByAssignee(taskSummaries),
		"roster":           live.BuildRoster(events, time.Now().UTC(), 30*time.Second),
		"archive":          archive,
	})
}

func formatCount(value int) string {
	return strconv.Itoa(value)
}

func latestRoomValue(rooms []live.RoomSummary) string {
	if len(rooms) == 0 {
		return "暂无"
	}
	if !rooms[0].LastEventAt.IsZero() {
		return rooms[0].LastEventAt.Local().Format("2006-01-02 15:04 MST")
	}
	if !rooms[0].CreatedAt.IsZero() {
		return rooms[0].CreatedAt.Local().Format("2006-01-02 15:04 MST")
	}
	return "暂无"
}

func countArchivedRooms(rooms []live.RoomSummary) int {
	count := 0
	for _, room := range rooms {
		if room.Archive != nil {
			count++
		}
	}
	return count
}

func countActiveRooms(rooms []live.RoomSummary) int {
	count := 0
	for _, room := range rooms {
		if room.Active {
			count++
		}
	}
	return count
}

func buildEventViews(events []live.LiveMessage) []liveEventView {
	views := make([]liveEventView, 0, len(events))
	for _, event := range events {
		view := liveEventView{
			Type:      event.Type,
			Timestamp: event.Timestamp,
			Sender:    event.Sender,
			Heading:   eventHeading(event),
			Fields:    metadataFields(event.Payload.Metadata),
		}
		if task := buildTaskUpdateView(event.Payload.Metadata); task != nil {
			view.Task = task
			view.Note = "任务更新"
		} else if len(view.Fields) > 0 {
			view.Note = "附带结构化元数据"
		}
		views = append(views, view)
	}
	return views
}

func buildTaskSummaries(events []live.LiveMessage) []liveTaskSummaryView {
	index := make(map[string]*liveTaskSummaryView)
	order := make([]string, 0)
	for _, event := range events {
		task := buildTaskUpdateView(event.Payload.Metadata)
		if task == nil || strings.TrimSpace(task.TaskID) == "" {
			continue
		}
		item, ok := index[task.TaskID]
		if !ok {
			item = &liveTaskSummaryView{TaskID: task.TaskID}
			index[task.TaskID] = item
			order = append(order, task.TaskID)
		}
		item.UpdateCount++
		item.Status = firstNonEmptyString(task.Status, item.Status)
		item.Description = firstNonEmptyString(task.Description, item.Description)
		item.AssignedTo = firstNonEmptyString(task.AssignedTo, item.AssignedTo)
		item.Progress = firstNonEmptyString(task.Progress, item.Progress)
		item.LastSender = firstNonEmptyString(event.Sender, item.LastSender)
		item.LastUpdatedAt = firstNonEmptyString(event.Timestamp, item.LastUpdatedAt)
	}
	summaries := make([]liveTaskSummaryView, 0, len(order))
	for _, key := range order {
		summaries = append(summaries, *index[key])
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].LastUpdatedAt > summaries[j].LastUpdatedAt
	})
	return summaries
}

func groupTasksByStatus(tasks []liveTaskSummaryView) []liveTaskGroupView {
	return groupTasks(tasks, func(task liveTaskSummaryView) string {
		return firstNonEmptyString(task.Status, "未标记状态")
	})
}

func groupTasksByAssignee(tasks []liveTaskSummaryView) []liveTaskGroupView {
	return groupTasks(tasks, func(task liveTaskSummaryView) string {
		return firstNonEmptyString(task.AssignedTo, "未分配")
	})
}

func groupTasks(tasks []liveTaskSummaryView, fn func(liveTaskSummaryView) string) []liveTaskGroupView {
	counts := map[string]int{}
	for _, task := range tasks {
		key := strings.TrimSpace(fn(task))
		if key == "" {
			continue
		}
		counts[key]++
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	groups := make([]liveTaskGroupView, 0, len(keys))
	for _, key := range keys {
		groups = append(groups, liveTaskGroupView{Key: key, Count: counts[key]})
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Count == groups[j].Count {
			return groups[i].Key < groups[j].Key
		}
		return groups[i].Count > groups[j].Count
	})
	return groups
}

func eventHeading(event live.LiveMessage) string {
	content := strings.TrimSpace(event.Payload.Content)
	if content != "" {
		return content
	}
	switch event.Type {
	case live.TypeJoin:
		return "加入房间"
	case live.TypeLeave:
		return "离开房间"
	case live.TypeHeartbeat:
		return "在线心跳"
	case live.TypeTaskUpdate:
		return "任务状态更新"
	case live.TypeArchiveNotice:
		return "房间归档通知"
	default:
		return "控制事件"
	}
}

func buildTaskUpdateView(metadata map[string]any) *liveTaskUpdateView {
	if len(metadata) == 0 {
		return nil
	}
	taskID := metadataString(metadata, "task_id")
	status := metadataString(metadata, "status")
	description := metadataString(metadata, "description")
	assignedTo := metadataString(metadata, "assigned_to")
	progress := metadataProgress(metadata["progress"])
	if taskID == "" && status == "" && description == "" && assignedTo == "" && progress == "" {
		return nil
	}
	return &liveTaskUpdateView{
		TaskID:      taskID,
		Status:      status,
		Description: description,
		AssignedTo:  assignedTo,
		Progress:    progress,
	}
}

func metadataFields(metadata map[string]any) []liveFieldView {
	if len(metadata) == 0 {
		return nil
	}
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := make([]liveFieldView, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(fmt.Sprint(metadata[key]))
		if value == "" || value == "<nil>" {
			continue
		}
		fields = append(fields, liveFieldView{Key: key, Value: value})
	}
	return fields
}

func metadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func metadataProgress(value any) string {
	if value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return ""
	}
	if strings.HasSuffix(text, "%") {
		return text
	}
	return text + "%"
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
