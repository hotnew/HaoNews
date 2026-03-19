package live

import (
	"fmt"
	"strings"
	"time"

	"hao.news/internal/haonews"
)

type ArchiveOptions struct {
	StoreRoot    string
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
	localStore, err := OpenLocalStore(opts.StoreRoot)
	if err != nil {
		return ArchiveResult{}, err
	}
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
	var b strings.Builder
	b.WriteString("# Live 房间归档\n\n")
	fmt.Fprintf(&b, "- 房间 ID：`%s`\n", info.RoomID)
	fmt.Fprintf(&b, "- 标题：%s\n", firstNonEmpty(info.Title, "未命名房间"))
	fmt.Fprintf(&b, "- 创建者：`%s`\n", info.Creator)
	fmt.Fprintf(&b, "- 创建时间：%s\n", info.CreatedAt)
	fmt.Fprintf(&b, "- 事件数：%d\n\n", len(events))
	b.WriteString("## 事件流\n\n")
	for _, event := range events {
		content := strings.TrimSpace(event.Payload.Content)
		if content == "" {
			content = "-"
		}
		fmt.Fprintf(&b, "- [%s] `%s` `%s`: %s\n", event.Timestamp, event.Type, event.Sender, content)
	}
	return strings.TrimSpace(b.String()) + "\n"
}
