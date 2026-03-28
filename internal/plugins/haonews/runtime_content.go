package newsplugin

import (
	"net/url"
	"strconv"
	"strings"
	"time"
)

func BuildFeedFacets(stats []FacetStat, opts FeedOptions, basePath, key string, omit ...string) []FeedFacet {
	items := make([]FeedFacet, 0, len(stats)+1)
	items = append(items, FeedFacet{
		Name:   "All",
		Count:  0,
		URL:    pageURL(basePath, opts, key, "", omit...),
		Active: activeFeedValue(opts, key) == "",
	})
	limit := len(stats)
	if limit > 8 {
		limit = 8
	}
	for _, stat := range stats[:limit] {
		items = append(items, FeedFacet{
			Name:   stat.Name,
			Count:  stat.Count,
			URL:    pageURL(basePath, opts, key, stat.Name, omit...),
			Active: strings.EqualFold(activeFeedValue(opts, key), stat.Name),
		})
	}
	return items
}

func BuildFacetLinks(stats []FacetStat, opts FeedOptions, basePath, key string, omit ...string) []FeedFacet {
	limit := len(stats)
	if limit > 8 {
		limit = 8
	}
	items := make([]FeedFacet, 0, limit)
	for _, stat := range stats[:limit] {
		items = append(items, FeedFacet{
			Name:   stat.Name,
			Count:  stat.Count,
			URL:    pageURL(basePath, opts, key, stat.Name, omit...),
			Active: strings.EqualFold(activeFeedValue(opts, key), stat.Name),
		})
	}
	return items
}

func BuildSortOptions(opts FeedOptions, basePath string, omit ...string) []SortOption {
	order := []struct {
		Name  string
		Value string
	}{
		{Name: "Newest", Value: "new"},
		{Name: "Discussed", Value: "discussed"},
		{Name: "Vote Score", Value: "score"},
		{Name: "Truth", Value: "truth"},
		{Name: "Source Quality", Value: "source"},
	}
	items := make([]SortOption, 0, len(order))
	active := opts.Sort
	if active == "" {
		active = "new"
	}
	for _, item := range order {
		items = append(items, SortOption{
			Name:   item.Name,
			Value:  item.Value,
			URL:    pageURL(basePath, opts, "sort", item.Value, omit...),
			Active: item.Value == active,
		})
	}
	return items
}

func BuildTabOptions(opts FeedOptions, basePath string, omit ...string) []TabOption {
	order := []struct {
		Name  string
		Value string
	}{
		{Name: "New", Value: "new"},
		{Name: "Hot", Value: "hot"},
	}
	active := canonicalTab(opts.Tab)
	items := make([]TabOption, 0, len(order))
	for _, item := range order {
		items = append(items, TabOption{
			Name:   item.Name,
			Value:  item.Value,
			URL:    pageURL(basePath, opts, "tab", item.Value, omit...),
			Active: item.Value == active,
		})
	}
	return items
}

func BuildWindowOptions(opts FeedOptions, basePath string, omit ...string) []TimeWindowOption {
	order := []struct {
		Name  string
		Value string
	}{
		{Name: "All time", Value: ""},
		{Name: "24h", Value: "24h"},
		{Name: "7d", Value: "7d"},
		{Name: "30d", Value: "30d"},
	}
	active := canonicalWindow(opts.Window)
	items := make([]TimeWindowOption, 0, len(order))
	for _, item := range order {
		items = append(items, TimeWindowOption{
			Name:   item.Name,
			Value:  item.Value,
			URL:    pageURL(basePath, opts, "window", item.Value, omit...),
			Active: canonicalWindow(item.Value) == active,
		})
		if item.Value == "" && active == "" {
			items[len(items)-1].Active = true
		}
	}
	return items
}

func BuildPageSizeOptions(opts FeedOptions, basePath string, omit ...string) []PageSizeOption {
	sizes := []int{20, 50, 100}
	items := make([]PageSizeOption, 0, len(sizes))
	active := opts.PageSize
	if active == 0 {
		active = 20
	}
	for _, size := range sizes {
		items = append(items, PageSizeOption{
			Name:   strconv.Itoa(size),
			Value:  size,
			URL:    pageURL(basePath, opts, "page_size", strconv.Itoa(size), omit...),
			Active: size == active,
		})
	}
	return items
}

func BuildActiveFilters(opts FeedOptions, basePath string, omit ...string) []ActiveFilter {
	filters := make([]ActiveFilter, 0, 6)
	if opts.Query != "" {
		filters = append(filters, ActiveFilter{
			Label: "Search: " + opts.Query,
			URL:   pageURL(basePath, opts, "q", "", omit...),
		})
	}
	if opts.Window != "" {
		filters = append(filters, ActiveFilter{
			Label: "Window: " + strings.ToUpper(opts.Window),
			URL:   pageURL(basePath, opts, "window", "", omit...),
		})
	}
	if canonicalTab(opts.Tab) == "hot" {
		filters = append(filters, ActiveFilter{
			Label: "Tab: Hot",
			URL:   pageURL(basePath, opts, "tab", "new", omit...),
		})
	}
	if opts.Channel != "" {
		filters = append(filters, ActiveFilter{
			Label: "Channel: " + opts.Channel,
			URL:   pageURL(basePath, opts, "channel", "", omit...),
		})
	}
	if opts.Topic != "" && !contains(omit, "topic") {
		topic := canonicalTopic(opts.Topic)
		if topic == "" {
			topic = opts.Topic
		}
		filters = append(filters, ActiveFilter{
			Label: "Topic: " + topic,
			URL:   pageURL(basePath, opts, "topic", "", omit...),
		})
	}
	if opts.Source != "" && !contains(omit, "source") {
		filters = append(filters, ActiveFilter{
			Label: "Source: " + opts.Source,
			URL:   pageURL(basePath, opts, "source", "", omit...),
		})
	}
	if opts.Reviewer != "" && !contains(omit, "reviewer") {
		filters = append(filters, ActiveFilter{
			Label: "Reviewer: " + opts.Reviewer,
			URL:   pageURL(basePath, opts, "reviewer", "", omit...),
		})
	}
	if opts.PageSize > 0 && opts.PageSize != 20 {
		filters = append(filters, ActiveFilter{
			Label: "Per page: " + strconv.Itoa(opts.PageSize),
			URL:   pageURL(basePath, opts, "page_size", "20", omit...),
		})
	}
	return filters
}

func BuildSummaryStats(posts []Post) []SummaryStat {
	return []SummaryStat{
		{Label: "Visible stories", Value: strconv.Itoa(len(posts))},
		{Label: "Replies", Value: strconv.Itoa(CountReplies(posts))},
		{Label: "Reactions", Value: strconv.Itoa(CountReactions(posts))},
	}
}

func PaginatePosts(posts []Post, opts FeedOptions, basePath string) ([]Post, PaginationState) {
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	totalItems := len(posts)
	totalPages := 1
	if totalItems > 0 {
		totalPages = (totalItems + pageSize - 1) / pageSize
	}
	page := opts.Page
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}
	start := 0
	end := totalItems
	fromItem := 0
	toItem := 0
	if totalItems > 0 {
		start = (page - 1) * pageSize
		if start > totalItems {
			start = totalItems
		}
		end = start + pageSize
		if end > totalItems {
			end = totalItems
		}
		fromItem = start + 1
		toItem = end
	}
	currentOpts := opts
	currentOpts.Page = page
	currentOpts.PageSize = pageSize
	state := PaginationState{
		Page:       page,
		PageSize:   pageSize,
		TotalItems: totalItems,
		TotalPages: totalPages,
		FromItem:   fromItem,
		ToItem:     toItem,
	}
	if page > 1 {
		state.PrevURL = pageURL(basePath, currentOpts, "page", strconv.Itoa(page-1))
	}
	if page < totalPages {
		state.NextURL = pageURL(basePath, currentOpts, "page", strconv.Itoa(page+1))
	}
	startPage := page - 2
	if startPage < 1 {
		startPage = 1
	}
	endPage := startPage + 4
	if endPage > totalPages {
		endPage = totalPages
	}
	if endPage-startPage < 4 {
		startPage = endPage - 4
		if startPage < 1 {
			startPage = 1
		}
	}
	for p := startPage; p <= endPage; p++ {
		state.Links = append(state.Links, PaginationLink{
			Label:  strconv.Itoa(p),
			URL:    pageURL(basePath, currentOpts, "page", strconv.Itoa(p)),
			Active: p == page,
		})
	}
	return posts[start:end], state
}

func BuildDirectorySummaryStats(stats []FacetStat, posts []Post) []SummaryStat {
	return []SummaryStat{
		{Label: "Tracked groups", Value: strconv.Itoa(len(stats))},
		{Label: "Stories", Value: strconv.Itoa(len(posts))},
		{Label: "Replies", Value: strconv.Itoa(CountReplies(posts))},
	}
}

func BuildSourceDirectory(index Index) []DirectoryItem {
	items := make([]DirectoryItem, 0, len(index.SourceStats))
	if len(index.SourceStats) == 0 {
		return items
	}
	type sourceAggregate struct {
		storyCount    int
		replyCount    int
		reactionCount int
		externalURL   string
		posts         []Post
	}
	aggregates := make(map[string]*sourceAggregate, len(index.SourceStats))
	for _, post := range index.Posts {
		if !post.HasSourcePage || post.SourceName == "" {
			continue
		}
		aggregate, ok := aggregates[post.SourceName]
		if !ok {
			aggregate = &sourceAggregate{}
			aggregates[post.SourceName] = aggregate
		}
		aggregate.storyCount++
		aggregate.replyCount += post.ReplyCount
		aggregate.reactionCount += post.ReactionCount
		if aggregate.externalURL == "" && post.SourceURL != "" {
			aggregate.externalURL = post.SourceURL
		}
		aggregate.posts = append(aggregate.posts, post)
	}
	for _, stat := range index.SourceStats {
		aggregate := aggregates[stat.Name]
		if aggregate == nil {
			aggregate = &sourceAggregate{}
		}
		items = append(items, DirectoryItem{
			Name:          stat.Name,
			URL:           SourcePath(stat.Name),
			ExternalURL:   aggregate.externalURL,
			StoryCount:    aggregate.storyCount,
			ReplyCount:    aggregate.replyCount,
			ReactionCount: aggregate.reactionCount,
			AvgTruth:      formatAverageTruth(aggregate.posts),
		})
	}
	return items
}

func BuildTopicDirectory(index Index, opts FeedOptions) []DirectoryItem {
	basePosts := index.FilterPosts(FeedOptions{
		Tab:    opts.Tab,
		Window: opts.Window,
		Now:    opts.Now,
	})
	return buildTopicDirectoryFromPosts(index, opts, basePosts)
}

func buildTopicDirectoryFromPosts(index Index, opts FeedOptions, basePosts []Post) []DirectoryItem {
	counts := make(map[string]*DirectoryItem, len(index.TopicStats))
	for _, post := range basePosts {
		for _, topic := range post.Topics {
			item, ok := counts[topic]
			if !ok {
				item = &DirectoryItem{
					Name: topic,
					URL:  pageURL(TopicPath(topic), opts, "topic", "", "topic"),
				}
				counts[topic] = item
			}
			item.StoryCount++
			item.ReplyCount += post.ReplyCount
			item.ReactionCount += post.ReactionCount
		}
	}
	items := make([]DirectoryItem, 0, len(index.TopicStats))
	for _, stat := range index.TopicStats {
		item, ok := counts[stat.Name]
		if !ok {
			continue
		}
		items = append(items, *item)
	}
	return items
}

func ChannelStatsForPosts(posts []Post) []FacetStat {
	counts := make(map[string]int)
	for _, post := range posts {
		if post.ChannelGroup == "" {
			continue
		}
		counts[post.ChannelGroup]++
	}
	return facetStats(counts)
}

func TopicStatsForPosts(posts []Post) []FacetStat {
	counts := make(map[string]int)
	for _, post := range posts {
		for _, topic := range post.Topics {
			counts[topic]++
		}
	}
	return facetStats(counts)
}

func SourceStatsForPosts(posts []Post) []FacetStat {
	counts := make(map[string]int)
	for _, post := range posts {
		if !post.HasSourcePage || post.SourceName == "" {
			continue
		}
		counts[post.SourceName]++
	}
	return facetStats(counts)
}

func ReviewerStatsForPosts(posts []Post) []FacetStat {
	counts := make(map[string]int)
	for _, post := range posts {
		name := strings.TrimSpace(post.AssignedReviewer)
		if name == "" {
			name = strings.TrimSpace(post.SuggestedReviewer)
		}
		if name == "" {
			continue
		}
		counts[name]++
	}
	return facetStats(counts)
}

func SourceURLFromPosts(posts []Post) string {
	for _, post := range posts {
		if post.SourceURL != "" {
			return post.SourceURL
		}
	}
	return ""
}

func HasSource(index Index, name string) bool {
	for _, stat := range index.SourceStats {
		if strings.EqualFold(stat.Name, name) {
			return true
		}
	}
	return false
}

func HasTopic(index Index, name string) bool {
	name = canonicalTopic(name)
	for _, stat := range index.TopicStats {
		if strings.EqualFold(stat.Name, name) {
			return true
		}
	}
	return false
}

func PathValue(prefix, path string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	value := strings.TrimPrefix(path, prefix)
	if value == "" || strings.Contains(value, "/") {
		return ""
	}
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(decoded)
}

func SourcePath(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return "/sources/" + url.PathEscape(name)
}

func TopicPath(name string) string {
	name = canonicalTopic(name)
	if name == "" {
		return ""
	}
	return "/topics/" + url.PathEscape(name)
}

func APIOptions(opts FeedOptions) map[string]string {
	result := map[string]string{
		"channel":  opts.Channel,
		"topic":    canonicalTopic(opts.Topic),
		"source":   opts.Source,
		"reviewer": strings.TrimSpace(opts.Reviewer),
		"tab":      canonicalTab(opts.Tab),
		"sort":     opts.Sort,
		"q":        opts.Query,
		"window":   canonicalWindow(opts.Window),
	}
	if opts.PendingApproval {
		result["approval"] = "pending"
	}
	if opts.Page > 1 {
		result["page"] = strconv.Itoa(opts.Page)
	}
	if opts.PageSize > 0 {
		result["page_size"] = strconv.Itoa(opts.PageSize)
	}
	return result
}

func APIPosts(posts []Post) []map[string]any {
	out := make([]map[string]any, 0, len(posts))
	for _, post := range posts {
		out = append(out, APIPost(post, false))
	}
	return out
}

func APIPost(post Post, withBody bool) map[string]any {
	origin := apiOrigin(post.Message.Origin)
	payload := map[string]any{
		"infohash":              post.InfoHash,
		"magnet":                post.Magnet,
		"archive_md":            post.ArchiveMD,
		"title":                 post.Message.Title,
		"author":                post.Message.Author,
		"origin":                origin,
		"origin_signed":         origin != nil,
		"delegation":            apiDelegation(post.Delegation),
		"shared_by_local_node":  post.SharedByLocalNode,
		"created_at":            post.CreatedAt.Format(time.RFC3339),
		"channel":               post.Message.Channel,
		"channel_group":         post.ChannelGroup,
		"source_name":           post.SourceName,
		"source_site_name":      post.SourceSiteName,
		"source_url":            post.SourceURL,
		"origin_public_key":     post.OriginPublicKey,
		"parent_public_key":     post.ParentPublicKey,
		"topics":                post.Topics,
		"post_type":             post.PostType,
		"summary":               post.Summary,
		"reply_count":           post.ReplyCount,
		"comment_count":         post.CommentCount,
		"reaction_count":        post.ReactionCount,
		"upvotes":               post.Upvotes,
		"downvotes":             post.Downvotes,
		"vote_score":            post.VoteScore,
		"hot_score":             post.HotScore,
		"is_hot_candidate":      post.IsHotCandidate,
		"visibility_state":      post.VisibilityState,
		"pending_approval":      post.PendingApproval,
		"approval_feed":         post.ApprovalFeed,
		"approved_feed":         post.ApprovedFeed,
		"approved_topics":       append([]string(nil), post.ApprovedTopics...),
		"moderation_action":     post.ModerationAction,
		"moderation_actor":      post.ModerationActor,
		"moderation_actor_key":  post.ModerationActorKey,
		"moderation_identity":   post.ModerationIdentity,
		"moderation_at":         post.ModerationAt,
		"assigned_reviewer":     post.AssignedReviewer,
		"assigned_reviewer_key": post.AssignedReviewerKey,
		"suggested_reviewer":    post.SuggestedReviewer,
		"suggested_reason":      post.SuggestedReason,
		"truth_score":           scoreValue(post.TruthScoreAverage),
		"source_quality":        scoreValue(post.SourceScoreAverage),
		"thread_path":           "/posts/" + post.InfoHash,
		"source_path":           sourcePathForPost(post),
		"latest_reaction":       post.LatestReactionAuthor,
		"event_time":            timeValue(post.EventTime),
		"topic_paths":           topicPaths(post.Topics),
		"message_tags":          post.Message.Tags,
		"message_protocol":      post.Message.Protocol,
	}
	if withBody {
		payload["body"] = post.Body
	}
	return payload
}

func APIReplies(replies []Reply) []map[string]any {
	out := make([]map[string]any, 0, len(replies))
	for _, reply := range replies {
		origin := apiOrigin(reply.Message.Origin)
		out = append(out, map[string]any{
			"infohash":             reply.InfoHash,
			"magnet":               reply.Magnet,
			"archive_md":           reply.ArchiveMD,
			"author":               reply.Message.Author,
			"origin":               origin,
			"origin_signed":        origin != nil,
			"delegation":           apiDelegation(reply.Delegation),
			"shared_by_local_node": reply.SharedByLocalNode,
			"created_at":           reply.CreatedAt.Format(time.RFC3339),
			"parent_hash":          reply.ParentInfoHash,
			"body":                 reply.Body,
		})
	}
	return out
}

func APIReactions(reactions []Reaction) []map[string]any {
	out := make([]map[string]any, 0, len(reactions))
	for _, reaction := range reactions {
		origin := apiOrigin(reaction.Message.Origin)
		out = append(out, map[string]any{
			"infohash":             reaction.InfoHash,
			"magnet":               reaction.Magnet,
			"archive_md":           reaction.ArchiveMD,
			"author":               reaction.Message.Author,
			"origin":               origin,
			"origin_signed":        origin != nil,
			"delegation":           apiDelegation(reaction.Delegation),
			"shared_by_local_node": reaction.SharedByLocalNode,
			"created_at":           reaction.CreatedAt.Format(time.RFC3339),
			"subject_hash":         reaction.SubjectInfoHash,
			"reaction_type":        reaction.ReactionType,
			"vote_value":           reaction.VoteValue,
			"score_value":          scoreValue(reaction.ScoreValue),
			"explanation":          reaction.Explanation,
		})
	}
	return out
}

func pageURL(basePath string, opts FeedOptions, key, value string, omit ...string) string {
	next := withOption(opts, key, value)
	encoded := encodeOptions(next, omit...)
	if encoded == "" {
		return basePath
	}
	return basePath + "?" + encoded
}

func withOption(opts FeedOptions, key, value string) FeedOptions {
	next := opts
	switch key {
	case "channel":
		next.Channel = value
	case "topic":
		next.Topic = canonicalTopic(value)
	case "source":
		next.Source = value
	case "reviewer":
		next.Reviewer = strings.TrimSpace(value)
	case "tab":
		next.Tab = canonicalTab(value)
	case "sort":
		next.Sort = value
	case "q":
		next.Query = value
	case "window":
		next.Window = canonicalWindow(value)
	case "page":
		next.Page = parsePositiveInt(value, 1)
	case "page_size":
		next.PageSize = parseFeedPageSize(value)
	}
	if key != "page" {
		next.Page = 1
	}
	return next
}

func encodeOptions(opts FeedOptions, omit ...string) string {
	query := url.Values{}
	ignored := make(map[string]struct{}, len(omit))
	for _, key := range omit {
		ignored[key] = struct{}{}
	}
	set := func(key, value string) {
		if value == "" {
			return
		}
		if _, skip := ignored[key]; skip {
			return
		}
		query.Set(key, value)
	}
	set("channel", opts.Channel)
	set("topic", canonicalTopic(opts.Topic))
	set("source", opts.Source)
	set("reviewer", strings.TrimSpace(opts.Reviewer))
	if canonicalTab(opts.Tab) == "hot" {
		set("tab", "hot")
	}
	if opts.Sort != "" && opts.Sort != "new" {
		set("sort", opts.Sort)
	}
	set("q", opts.Query)
	if window := canonicalWindow(opts.Window); window != "" {
		set("window", window)
	}
	if opts.Page > 1 {
		query.Set("page", strconv.Itoa(opts.Page))
	}
	if opts.PageSize > 0 && opts.PageSize != 20 {
		query.Set("page_size", strconv.Itoa(opts.PageSize))
	}
	return query.Encode()
}

func activeFeedValue(opts FeedOptions, key string) string {
	switch key {
	case "channel":
		return opts.Channel
	case "topic":
		return canonicalTopic(opts.Topic)
	case "source":
		return opts.Source
	case "reviewer":
		return strings.TrimSpace(opts.Reviewer)
	case "tab":
		return canonicalTab(opts.Tab)
	case "window":
		return canonicalWindow(opts.Window)
	default:
		return ""
	}
}

func compactIdentity(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if isPublicKeyish(value) {
		if len(value) <= 10 {
			return value
		}
		return value[:10] + "..."
	}
	if len(value) <= 24 {
		return value
	}
	return value[:24] + "..."
}

func isPublicKeyish(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 32 {
		return false
	}
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			continue
		}
		return false
	}
	return true
}

func parsePositiveInt(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func parseFeedPageSize(raw string) int {
	value := parsePositiveInt(raw, 20)
	if value < 1 {
		return 20
	}
	if value > 200 {
		return 200
	}
	return value
}

func formatAverageTruth(posts []Post) string {
	var sum float64
	var count int
	for _, post := range posts {
		if post.TruthScoreAverage == nil {
			continue
		}
		sum += *post.TruthScoreAverage
		count++
	}
	if count == 0 {
		return "-"
	}
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(sum/float64(count), 'f', 2, 64), "0"), ".")
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func sourcePathForPost(post Post) string {
	if !post.HasSourcePage || strings.TrimSpace(post.SourceName) == "" {
		return ""
	}
	return SourcePath(post.SourceName)
}

func apiOrigin(origin *MessageOrigin) map[string]any {
	if origin == nil {
		return nil
	}
	return map[string]any{
		"author":     origin.Author,
		"agent_id":   origin.AgentID,
		"key_type":   origin.KeyType,
		"public_key": origin.PublicKey,
		"signature":  origin.Signature,
	}
}

func apiDelegation(info *DelegationInfo) map[string]any {
	if info == nil || !info.Delegated {
		return nil
	}
	return map[string]any{
		"delegated":         true,
		"parent_agent_id":   info.ParentAgentID,
		"parent_key_type":   info.ParentKeyType,
		"parent_public_key": info.ParentPublicKey,
		"scopes":            append([]string(nil), info.Scopes...),
		"created_at":        info.CreatedAt,
		"expires_at":        info.ExpiresAt,
	}
}

func topicPaths(topics []string) map[string]string {
	out := make(map[string]string, len(topics))
	for _, topic := range topics {
		out[topic] = TopicPath(topic)
	}
	return out
}

func scoreValue(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func timeValue(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.Format(time.RFC3339)
}
