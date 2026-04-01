package live

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"hao.news/internal/haonews"
)

type ArchiveOptions struct {
	StoreRoot    string
	NetPath      string
	IdentityFile string
	Author       string
	RoomID       string
	Channel      string
}

type ArchiveResult struct {
	RoomID     string                `json:"room_id"`
	Channel    string                `json:"channel"`
	Published  haonews.PublishResult `json:"published"`
	Events     int                   `json:"events"`
	ArchivedAt string                `json:"archived_at"`
	ViewerURL  string                `json:"viewer_url"`
}

func Archive(opts ArchiveOptions) (ArchiveResult, error) {
	var redisCfg haonews.RedisConfig
	if strings.TrimSpace(opts.NetPath) != "" {
		netCfg, err := haonews.LoadNetworkBootstrapConfig(opts.NetPath)
		if err != nil {
			return ArchiveResult{}, err
		}
		redisCfg = netCfg.Redis
	}
	localStore, err := OpenLocalStoreWithRedis(opts.StoreRoot, redisCfg)
	if err != nil {
		return ArchiveResult{}, err
	}
	defer localStore.Close()
	info, err := localStore.LoadRoom(opts.RoomID)
	if err != nil {
		return ArchiveResult{}, err
	}
	events, err := localStore.ReadEvents(opts.RoomID)
	if err != nil {
		return ArchiveResult{}, err
	}
	if len(events) == 0 {
		return ArchiveResult{}, fmt.Errorf("live room %s has no events to archive", opts.RoomID)
	}
	store, err := haonews.OpenStore(opts.StoreRoot)
	if err != nil {
		return ArchiveResult{}, err
	}
	identity, err := haonews.LoadAgentIdentity(strings.TrimSpace(opts.IdentityFile))
	if err != nil {
		return ArchiveResult{}, err
	}
	author := strings.TrimSpace(opts.Author)
	if author == "" {
		author = strings.TrimSpace(identity.Author)
	}
	if author == "" {
		return ArchiveResult{}, fmt.Errorf("author is required")
	}
	channel := strings.TrimSpace(opts.Channel)
	if channel == "" {
		channel = firstNonEmpty(info.Channel, "hao.news/live")
	}
	title := strings.TrimSpace(info.Title)
	if title == "" {
		title = "Live 房间归档 " + info.RoomID
	}
	body := buildArchiveBody(info, events)
	extensions := map[string]any{
		"project":           "hao.news",
		"live.room_id":      info.RoomID,
		"live.created_at":   info.CreatedAt,
		"live.creator":      info.Creator,
		"live.event_count":  len(events),
		"live.archive_type": "transcript",
	}
	result, err := haonews.PublishMessage(store, haonews.MessageInput{
		Kind:       "post",
		Author:     author,
		Channel:    channel,
		Title:      title,
		Body:       body,
		Tags:       []string{"live", "archive"},
		Identity:   &identity,
		Extensions: extensions,
		CreatedAt:  time.Now().UTC(),
	})
	if err != nil {
		return ArchiveResult{}, err
	}
	archiveResult := ArchiveResult{
		RoomID:     info.RoomID,
		Channel:    channel,
		Published:  result,
		Events:     len(events),
		ArchivedAt: time.Now().UTC().Format(time.RFC3339),
		ViewerURL:  "/posts/" + result.InfoHash,
	}
	if err := localStore.SaveArchiveResult(info.RoomID, archiveResult); err != nil {
		return ArchiveResult{}, err
	}
	return archiveResult, nil
}

func buildArchiveBody(info RoomInfo, events []LiveMessage) string {
	displayEvents := archiveDisplayEvents(events)
	var b strings.Builder
	startAt, endAt := archiveEventRange(displayEvents)
	participants := archiveParticipants(events)
	taskSummaries := archiveTaskSummaries(events)
	messageCount := archiveCountByType(displayEvents, TypeMessage)
	taskUpdateCount := archiveCountByType(displayEvents, TypeTaskUpdate)

	b.WriteString("# Live 房间归档\n\n")
	b.WriteString("## 房间摘要\n\n")
	fmt.Fprintf(&b, "- 房间 ID：`%s`\n", info.RoomID)
	fmt.Fprintf(&b, "- 标题：%s\n", firstNonEmpty(info.Title, "未命名房间"))
	fmt.Fprintf(&b, "- 创建者：`%s`\n", info.Creator)
	fmt.Fprintf(&b, "- 创建时间：%s\n", info.CreatedAt)
	fmt.Fprintf(&b, "- 事件数：%d\n", len(displayEvents))
	if startAt != "" {
		fmt.Fprintf(&b, "- 首条事件：%s\n", startAt)
	}
	if endAt != "" {
		fmt.Fprintf(&b, "- 末条事件：%s\n", endAt)
	}
	fmt.Fprintf(&b, "- 普通消息数：%d\n", messageCount)
	fmt.Fprintf(&b, "- 任务更新数：%d\n", taskUpdateCount)
	if len(participants) > 0 {
		fmt.Fprintf(&b, "- 参与者：%s\n", strings.Join(participants, "、"))
	}
	b.WriteString("\n")

	if len(taskSummaries) > 0 {
		b.WriteString("## 任务摘要\n\n")
		for _, task := range taskSummaries {
			fmt.Fprintf(&b, "- 任务 `%s`", task.TaskID)
			if task.Description != "" {
				fmt.Fprintf(&b, "：%s", task.Description)
			}
			b.WriteString("\n")
			if task.Status != "" {
				fmt.Fprintf(&b, "  - 当前状态：%s\n", task.Status)
			}
			if task.AssignedTo != "" {
				fmt.Fprintf(&b, "  - 负责人：%s\n", task.AssignedTo)
			}
			if task.Progress != "" {
				fmt.Fprintf(&b, "  - 当前进度：%s\n", task.Progress)
			}
			if task.UpdateCount > 0 {
				fmt.Fprintf(&b, "  - 更新次数：%d\n", task.UpdateCount)
			}
			if task.LastSender != "" {
				fmt.Fprintf(&b, "  - 最近更新者：%s\n", task.LastSender)
			}
			if task.LastUpdatedAt != "" {
				fmt.Fprintf(&b, "  - 最近更新时间：%s\n", task.LastUpdatedAt)
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("## 完整事件流\n\n")
	for _, event := range displayEvents {
		content := strings.TrimSpace(event.Payload.Content)
		if content == "" {
			content = "-"
		}
		fmt.Fprintf(&b, "- [%s] `%s` `%s`: %s\n", event.Timestamp, event.Type, event.Sender, content)
		if event.Type == TypeTaskUpdate {
			task := archiveTaskSummaryFromMetadata(event.Payload.Metadata)
			if task != nil {
				if task.TaskID != "" {
					fmt.Fprintf(&b, "  - task_id=%s\n", task.TaskID)
				}
				if task.Status != "" {
					fmt.Fprintf(&b, "  - status=%s\n", task.Status)
				}
				if task.AssignedTo != "" {
					fmt.Fprintf(&b, "  - assigned_to=%s\n", task.AssignedTo)
				}
				if task.Progress != "" {
					fmt.Fprintf(&b, "  - progress=%s\n", task.Progress)
				}
				if task.Description != "" {
					fmt.Fprintf(&b, "  - description=%s\n", task.Description)
				}
			}
		}
	}
	return strings.TrimSpace(b.String()) + "\n"
}

func archiveDisplayEvents(events []LiveMessage) []LiveMessage {
	filtered := make([]LiveMessage, 0, len(events))
	for _, event := range events {
		switch strings.TrimSpace(event.Type) {
		case TypeHeartbeat, TypeArchiveNotice:
			continue
		default:
			filtered = append(filtered, event)
		}
	}
	return filtered
}

type archiveTaskSummary struct {
	TaskID        string
	Status        string
	Description   string
	AssignedTo    string
	Progress      string
	UpdateCount   int
	LastSender    string
	LastUpdatedAt string
}

func archiveEventRange(events []LiveMessage) (string, string) {
	if len(events) == 0 {
		return "", ""
	}
	return strings.TrimSpace(events[0].Timestamp), strings.TrimSpace(events[len(events)-1].Timestamp)
}

func archiveParticipants(events []LiveMessage) []string {
	seen := make(map[string]struct{})
	participants := make([]string, 0)
	for _, event := range events {
		sender := strings.TrimSpace(event.Sender)
		if sender == "" {
			continue
		}
		if _, ok := seen[sender]; ok {
			continue
		}
		seen[sender] = struct{}{}
		participants = append(participants, sender)
	}
	sort.Strings(participants)
	return participants
}

func archiveCountByType(events []LiveMessage, messageType string) int {
	count := 0
	for _, event := range events {
		if strings.TrimSpace(event.Type) == strings.TrimSpace(messageType) {
			count++
		}
	}
	return count
}

func archiveTaskSummaries(events []LiveMessage) []archiveTaskSummary {
	index := make(map[string]*archiveTaskSummary)
	order := make([]string, 0)
	for _, event := range events {
		task := archiveTaskSummaryFromMetadata(event.Payload.Metadata)
		if task == nil || strings.TrimSpace(task.TaskID) == "" {
			continue
		}
		item, ok := index[task.TaskID]
		if !ok {
			item = &archiveTaskSummary{TaskID: task.TaskID}
			index[task.TaskID] = item
			order = append(order, task.TaskID)
		}
		item.UpdateCount++
		item.Status = archiveFirstNonEmpty(task.Status, item.Status)
		item.Description = archiveFirstNonEmpty(task.Description, item.Description)
		item.AssignedTo = archiveFirstNonEmpty(task.AssignedTo, item.AssignedTo)
		item.Progress = archiveFirstNonEmpty(task.Progress, item.Progress)
		item.LastSender = archiveFirstNonEmpty(strings.TrimSpace(event.Sender), item.LastSender)
		item.LastUpdatedAt = archiveFirstNonEmpty(strings.TrimSpace(event.Timestamp), item.LastUpdatedAt)
	}
	summaries := make([]archiveTaskSummary, 0, len(order))
	for _, key := range order {
		summaries = append(summaries, *index[key])
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].LastUpdatedAt > summaries[j].LastUpdatedAt
	})
	return summaries
}

func archiveTaskSummaryFromMetadata(metadata map[string]any) *archiveTaskSummary {
	if len(metadata) == 0 {
		return nil
	}
	taskID := archiveMetadataString(metadata, "task_id")
	status := archiveMetadataString(metadata, "status")
	description := archiveMetadataString(metadata, "description")
	assignedTo := archiveMetadataJoined(metadata, "assigned_to")
	progress := archiveMetadataProgress(metadata, "progress")
	if taskID == "" && status == "" && description == "" && assignedTo == "" && progress == "" {
		return nil
	}
	return &archiveTaskSummary{
		TaskID:      taskID,
		Status:      status,
		Description: description,
		AssignedTo:  assignedTo,
		Progress:    progress,
	}
}

func archiveMetadataString(metadata map[string]any, key string) string {
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func archiveMetadataJoined(metadata map[string]any, key string) string {
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case []string:
		return strings.Join(typed, ", ")
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				values = append(values, text)
			}
		}
		return strings.Join(values, ", ")
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func archiveMetadataProgress(metadata map[string]any, key string) string {
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case int:
		return strconv.Itoa(typed) + "%"
	case int64:
		return strconv.FormatInt(typed, 10) + "%"
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64) + "%"
	default:
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			return ""
		}
		if strings.HasSuffix(text, "%") {
			return text
		}
		return text + "%"
	}
}

func archiveFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
