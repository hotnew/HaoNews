package newsplugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anacrolix/torrent/metainfo"
	"hao.news/internal/haonews"
)

const (
	messageFileName = "haonews-message.json"
	bodyFileName    = "body.txt"
)

const (
	hotWindow    = 36 * time.Hour
	hotThreshold = 3.0
)

func LoadIndex(storeRoot, project string) (Index, error) {
	refs, err := loadTorrentRefs(filepath.Join(storeRoot, "torrents"))
	if err != nil {
		return Index{}, err
	}
	bundles, err := loadBundles(filepath.Join(storeRoot, "data"), refs, project)
	if err != nil {
		return Index{}, err
	}
	return buildIndex(bundles, project), nil
}

type torrentRef struct {
	InfoHash  string
	Magnet    string
	Name      string
	SizeBytes int64
}

func (idx Index) Clone() Index {
	cloned := Index{
		Bundles:         append([]Bundle(nil), idx.Bundles...),
		Posts:           append([]Post(nil), idx.Posts...),
		PostByInfoHash:  make(map[string]Post, len(idx.PostByInfoHash)),
		RepliesByPost:   make(map[string][]Reply, len(idx.RepliesByPost)),
		ReactionsByPost: make(map[string][]Reaction, len(idx.ReactionsByPost)),
		ChannelStats:    append([]FacetStat(nil), idx.ChannelStats...),
		TopicStats:      append([]FacetStat(nil), idx.TopicStats...),
		SourceStats:     append([]FacetStat(nil), idx.SourceStats...),
	}
	for key, post := range idx.PostByInfoHash {
		cloned.PostByInfoHash[key] = post
	}
	for key, replies := range idx.RepliesByPost {
		cloned.RepliesByPost[key] = append([]Reply(nil), replies...)
	}
	for key, reactions := range idx.ReactionsByPost {
		cloned.ReactionsByPost[key] = append([]Reaction(nil), reactions...)
	}
	return cloned
}

func loadTorrentRefs(dir string) (map[string]torrentRef, error) {
	refs := map[string]torrentRef{}
	store := &haonews.Store{TorrentDir: dir}
	if err := store.WalkTorrentFiles(func(_ string, path string) error {
		mi, err := metainfo.LoadFromFile(path)
		if err != nil {
			return fmt.Errorf("load torrent %s: %w", path, err)
		}
		info, err := mi.UnmarshalInfo()
		if err != nil {
			return fmt.Errorf("decode torrent %s: %w", path, err)
		}
		hash := strings.ToLower(mi.HashInfoBytes().HexString())
		refs[info.Name] = torrentRef{
			InfoHash:  hash,
			Magnet:    canonicalMagnet(hash, info.Name),
			Name:      info.Name,
			SizeBytes: info.TotalLength(),
		}
		return nil
	}); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]torrentRef{}, nil
		}
		return nil, err
	}
	return refs, nil
}

func canonicalMagnet(infoHash, displayName string) string {
	infoHash = strings.ToLower(strings.TrimSpace(infoHash))
	if infoHash == "" {
		return ""
	}
	values := url.Values{}
	values.Set("xt", "urn:btih:"+infoHash)
	displayName = strings.TrimSpace(displayName)
	if displayName != "" {
		values.Set("dn", displayName)
	}
	return "magnet:?" + values.Encode()
}

func loadBundles(dir string, refs map[string]torrentRef, project string) ([]Bundle, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var bundles []Bundle
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		bundleDir := filepath.Join(dir, entry.Name())
		bundle, ok, err := loadBundle(bundleDir, refs[entry.Name()], project)
		if err != nil {
			return nil, err
		}
		if ok {
			bundles = append(bundles, bundle)
		}
	}
	sort.Slice(bundles, func(i, j int) bool {
		return bundles[i].CreatedAt.After(bundles[j].CreatedAt)
	})
	return bundles, nil
}

func loadBundle(dir string, ref torrentRef, project string) (Bundle, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, messageFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Bundle{}, false, nil
		}
		return Bundle{}, false, err
	}
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return Bundle{}, false, fmt.Errorf("decode %s: %w", dir, err)
	}
	if !isProjectMessage(msg, project) {
		return Bundle{}, false, nil
	}
	body, err := os.ReadFile(filepath.Join(dir, bodyFileName))
	if err != nil {
		return Bundle{}, false, err
	}
	createdAt, err := time.Parse(time.RFC3339, msg.CreatedAt)
	if err != nil {
		return Bundle{}, false, err
	}
	return Bundle{
		InfoHash:          ref.InfoHash,
		Magnet:            ref.Magnet,
		SizeBytes:         ref.SizeBytes,
		Dir:               dir,
		Message:           msg,
		Body:              string(body),
		CreatedAt:         createdAt,
		SharedByLocalNode: true,
	}, true, nil
}

func isProjectMessage(msg Message, project string) bool {
	value, ok := stringFromMap(msg.Extensions, "project")
	if !ok {
		return false
	}
	if strings.EqualFold(value, project) {
		return true
	}
	if strings.EqualFold(project, "hao.news") && strings.EqualFold(value, "latest") {
		return true
	}
	return false
}

func buildIndex(bundles []Bundle, project string) Index {
	posts := make(map[string]Post)
	repliesByPost := map[string][]Reply{}
	reactionsByPost := map[string][]Reaction{}
	channelCounts := map[string]int{}
	topicCounts := map[string]int{}
	sourceCounts := map[string]int{}

	for _, bundle := range bundles {
		switch bundle.Message.Kind {
		case "post":
			post := Post{
				Bundle:          bundle,
				SourceName:      sourceGroupName(bundle.Message),
				SourceSiteName:  nestedString(bundle.Message.Extensions, "source", "name"),
				SourceURL:       nestedString(bundle.Message.Extensions, "source", "url"),
				OriginPublicKey: originPublicKey(bundle.Message),
				ParentPublicKey: parentPublicKey(bundle.Message),
				HasSourcePage:   hasSourcePage(bundle.Message),
				Topics:          stringSlice(bundle.Message.Extensions["topics"]),
				ChannelGroup:    channelGroup(bundle.Message.Channel),
				PostType:        stringValue(bundle.Message.Extensions["post_type"]),
				Summary:         summarize(bundle.Body, 220),
			}
			if eventTime, ok := timeFromMap(bundle.Message.Extensions, "event_time"); ok {
				post.EventTime = &eventTime
			}
			posts[bundle.InfoHash] = post
			if post.ChannelGroup != "" {
				channelCounts[post.ChannelGroup]++
			}
			for _, topic := range post.Topics {
				topicCounts[topic]++
			}
			if post.HasSourcePage && post.SourceName != "" {
				sourceCounts[post.SourceName]++
			}
		case "reply":
			parent := bundle.Message.ReplyTo
			if parent == nil || strings.TrimSpace(parent.InfoHash) == "" {
				continue
			}
			infoHash := strings.ToLower(parent.InfoHash)
			repliesByPost[infoHash] = append(repliesByPost[infoHash], Reply{
				Bundle:         bundle,
				ParentInfoHash: infoHash,
			})
		case "reaction":
			subject := nestedString(bundle.Message.Extensions, "subject", "infohash")
			if subject == "" {
				continue
			}
			subject = strings.ToLower(subject)
			reactionsByPost[subject] = append(reactionsByPost[subject], parseReaction(bundle))
		}
	}

	index := Index{
		Bundles:         append([]Bundle(nil), bundles...),
		Posts:           make([]Post, 0, len(posts)),
		PostByInfoHash:  make(map[string]Post, len(posts)),
		RepliesByPost:   repliesByPost,
		ReactionsByPost: reactionsByPost,
		ChannelStats:    facetStats(channelCounts),
		TopicStats:      facetStats(topicCounts),
		SourceStats:     facetStats(sourceCounts),
	}
	for infoHash, post := range posts {
		replies := repliesByPost[infoHash]
		reactions := reactionsByPost[infoHash]
		post.ReplyCount = len(replies)
		post.CommentCount = len(replies)
		post.ReactionCount = len(reactions)
		post.Upvotes, post.Downvotes = voteBreakdown(reactions)
		post.VoteScore = post.Upvotes - post.Downvotes
		post.HotScore = hotScore(post)
		post.TruthScoreAverage = averageScore(reactions, "truth_score")
		post.SourceScoreAverage = averageScore(reactions, "source_quality")
		if author := latestReactionAuthor(reactions); author != "" {
			post.LatestReactionAuthor = author
		}
		index.Posts = append(index.Posts, post)
		index.PostByInfoHash[infoHash] = post
		sort.Slice(replies, func(i, j int) bool {
			return replies[i].CreatedAt.Before(replies[j].CreatedAt)
		})
		index.RepliesByPost[infoHash] = replies
	}
	sort.Slice(index.Posts, func(i, j int) bool {
		left := index.Posts[i].CreatedAt
		right := index.Posts[j].CreatedAt
		if left.Equal(right) {
			return index.Posts[i].InfoHash < index.Posts[j].InfoHash
		}
		return left.After(right)
	})
	return index
}

func parseReaction(bundle Bundle) Reaction {
	reactionType, _ := stringFromMap(bundle.Message.Extensions, "reaction_type")
	vote, _ := intFromMap(bundle.Message.Extensions, "value")
	score, scoreOK := floatFromMap(bundle.Message.Extensions, "value")
	var scoreValue *float64
	if scoreOK {
		scoreValue = &score
	}
	return Reaction{
		Bundle:          bundle,
		SubjectInfoHash: strings.ToLower(nestedString(bundle.Message.Extensions, "subject", "infohash")),
		ReactionType:    reactionType,
		VoteValue:       vote,
		ScoreValue:      scoreValue,
		Explanation:     stringValue(bundle.Message.Extensions["explanation"]),
	}
}

func voteBreakdown(reactions []Reaction) (int, int) {
	upvotes := 0
	downvotes := 0
	for _, reaction := range reactions {
		if reaction.ReactionType == "vote" {
			switch {
			case reaction.VoteValue > 0:
				upvotes++
			case reaction.VoteValue < 0:
				downvotes++
			}
		}
	}
	return upvotes, downvotes
}

func hotScore(post Post) float64 {
	return float64(post.Upvotes-post.Downvotes) + float64(post.CommentCount)*0.5
}

func averageScore(reactions []Reaction, reactionType string) *float64 {
	var sum float64
	var count int
	for _, reaction := range reactions {
		if reaction.ReactionType != reactionType || reaction.ScoreValue == nil {
			continue
		}
		sum += *reaction.ScoreValue
		count++
	}
	if count == 0 {
		return nil
	}
	value := sum / float64(count)
	return &value
}

func latestReactionAuthor(reactions []Reaction) string {
	if len(reactions) == 0 {
		return ""
	}
	sort.Slice(reactions, func(i, j int) bool {
		return reactions[i].CreatedAt.After(reactions[j].CreatedAt)
	})
	return reactions[0].Message.Author
}

func (idx Index) FilterPosts(opts FeedOptions) []Post {
	filtered := make([]Post, 0, len(idx.Posts))
	now := opts.referenceTime()
	topic := canonicalTopic(opts.Topic)
	tab := canonicalTab(opts.Tab)
	reviewer := strings.TrimSpace(opts.Reviewer)
	for _, post := range idx.Posts {
		if opts.PendingApproval {
			if !post.PendingApproval || post.VisibilityState != visibilityStatePending {
				continue
			}
		} else {
			if post.PendingApproval {
				continue
			}
			if post.VisibilityState != "" && post.VisibilityState != visibilityStateVisible {
				continue
			}
		}
		if opts.Channel != "" && !strings.EqualFold(post.ChannelGroup, opts.Channel) {
			continue
		}
		if topic != "" && !containsFold(post.Topics, topic) {
			continue
		}
		if opts.Source != "" && !strings.EqualFold(post.SourceName, opts.Source) {
			continue
		}
		if reviewer != "" &&
			!strings.EqualFold(strings.TrimSpace(post.AssignedReviewer), reviewer) &&
			!strings.EqualFold(strings.TrimSpace(post.SuggestedReviewer), reviewer) {
			continue
		}
		if opts.Query != "" && !matchesQuery(post, opts.Query) {
			continue
		}
		if !matchesWindow(post, opts.Window, now) {
			continue
		}
		post.IsHotCandidate = isHotCandidate(post, now)
		post.HotScore = hotScore(post)
		if tab == "hot" && (!post.IsHotCandidate || post.HotScore < hotThreshold) {
			continue
		}
		filtered = append(filtered, post)
	}
	sortPosts(filtered, effectiveSort(opts))
	return filtered
}

func effectiveSort(opts FeedOptions) string {
	switch canonicalTab(opts.Tab) {
	case "hot":
		return "hot"
	case "new":
		return "new"
	default:
		return opts.Sort
	}
}

func (idx Index) RelatedPosts(infoHash string, limit int) []Post {
	base, ok := idx.PostByInfoHash[strings.ToLower(infoHash)]
	if !ok || limit <= 0 {
		return nil
	}
	type scoredPost struct {
		post  Post
		score int
	}
	var ranked []scoredPost
	for _, post := range idx.Posts {
		if post.InfoHash == base.InfoHash {
			continue
		}
		score := 0
		if base.SourceName != "" && strings.EqualFold(base.SourceName, post.SourceName) {
			score += 3
		}
		for _, topic := range post.Topics {
			if containsFold(base.Topics, topic) {
				score += 2
			}
		}
		if base.ChannelGroup != "" && strings.EqualFold(base.ChannelGroup, post.ChannelGroup) {
			score++
		}
		if score == 0 {
			continue
		}
		ranked = append(ranked, scoredPost{post: post, score: score})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		if !ranked[i].post.CreatedAt.Equal(ranked[j].post.CreatedAt) {
			return ranked[i].post.CreatedAt.After(ranked[j].post.CreatedAt)
		}
		return ranked[i].post.InfoHash < ranked[j].post.InfoHash
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make([]Post, 0, len(ranked))
	for _, item := range ranked {
		out = append(out, item.post)
	}
	return out
}

func CountReplies(posts []Post) int {
	total := 0
	for _, post := range posts {
		total += post.ReplyCount
	}
	return total
}

func CountReactions(posts []Post) int {
	total := 0
	for _, post := range posts {
		total += post.ReactionCount
	}
	return total
}

func sortPosts(posts []Post, mode string) {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "new"
	}
	sort.Slice(posts, func(i, j int) bool {
		left := posts[i]
		right := posts[j]
		switch mode {
		case "hot":
			if left.HotScore != right.HotScore {
				return left.HotScore > right.HotScore
			}
		case "discussed":
			if left.ReplyCount != right.ReplyCount {
				return left.ReplyCount > right.ReplyCount
			}
		case "score":
			if left.VoteScore != right.VoteScore {
				return left.VoteScore > right.VoteScore
			}
		case "truth":
			ls := scoreOrNeg(left.TruthScoreAverage)
			rs := scoreOrNeg(right.TruthScoreAverage)
			if ls != rs {
				return ls > rs
			}
		case "source":
			ls := scoreOrNeg(left.SourceScoreAverage)
			rs := scoreOrNeg(right.SourceScoreAverage)
			if ls != rs {
				return ls > rs
			}
		}
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.After(right.CreatedAt)
		}
		return left.InfoHash < right.InfoHash
	})
}

func scoreOrNeg(value *float64) float64 {
	if value == nil {
		return -1
	}
	return *value
}

func (opts FeedOptions) referenceTime() time.Time {
	if opts.Now.IsZero() {
		return time.Now()
	}
	return opts.Now
}

func canonicalTab(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "new":
		return "new"
	case "hot":
		return "hot"
	default:
		return "new"
	}
}

func isHotCandidate(post Post, now time.Time) bool {
	if post.CreatedAt.IsZero() {
		return false
	}
	return !post.CreatedAt.Before(now.Add(-hotWindow))
}

func matchesWindow(post Post, window string, now time.Time) bool {
	window = canonicalWindow(window)
	if window == "" {
		return true
	}
	var horizon time.Duration
	switch window {
	case "24h":
		horizon = 24 * time.Hour
	case "7d":
		horizon = 7 * 24 * time.Hour
	case "30d":
		horizon = 30 * 24 * time.Hour
	default:
		return true
	}
	return postReferenceTime(post).After(now.Add(-horizon))
}

func canonicalWindow(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "all":
		return ""
	case "24h", "7d", "30d":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func postReferenceTime(post Post) time.Time {
	if post.EventTime != nil {
		return *post.EventTime
	}
	return post.CreatedAt
}

func facetStats(counts map[string]int) []FacetStat {
	items := make([]FacetStat, 0, len(counts))
	for name, count := range counts {
		items = append(items, FacetStat{Name: name, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].Name < items[j].Name
	})
	return items
}

func channelGroup(channel string) string {
	channel = strings.TrimSpace(channel)
	if channel == "" {
		return ""
	}
	parts := strings.Split(channel, "/")
	if len(parts) >= 2 {
		return strings.TrimSpace(parts[1])
	}
	return channel
}

func matchesQuery(post Post, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	haystack := []string{
		post.Message.Title,
		post.Body,
		post.Summary,
		post.SourceName,
		post.SourceSiteName,
		post.OriginPublicKey,
		post.Message.Author,
		post.ChannelGroup,
		post.PostType,
		post.Message.Channel,
		strings.Join(post.Topics, " "),
	}
	for _, item := range haystack {
		if strings.Contains(strings.ToLower(item), query) {
			return true
		}
	}
	return false
}

func summarize(body string, max int) string {
	body = summarizePlainText(body)
	body = strings.Join(strings.Fields(strings.TrimSpace(body)), " ")
	if body == "" {
		return ""
	}
	if len(body) <= max {
		return body
	}
	return body[:max-3] + "..."
}

var htmlSummaryStripper = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>|<style[^>]*>.*?</style>|<[^>]+>`)

func summarizePlainText(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if !looksLikeHTMLDocument(body) {
		return body
	}
	body = htmlSummaryStripper.ReplaceAllString(body, " ")
	body = html.UnescapeString(body)
	body = strings.Join(strings.Fields(strings.TrimSpace(body)), " ")
	return body
}

func sourceGroupName(msg Message) string {
	if value := originPublicKey(msg); value != "" {
		return value
	}
	if value := strings.TrimSpace(nestedString(msg.Extensions, "source", "name")); value != "" {
		return value
	}
	if msg.Origin != nil {
		if value := strings.TrimSpace(msg.Origin.AgentID); value != "" {
			return value
		}
		if value := strings.TrimSpace(msg.Origin.Author); value != "" {
			return value
		}
	}
	return strings.TrimSpace(msg.Author)
}

func originPublicKey(msg Message) string {
	if value := nestedString(msg.Extensions, "origin_public_key"); value != "" {
		return strings.TrimSpace(value)
	}
	if msg.Origin == nil {
		return ""
	}
	return strings.TrimSpace(msg.Origin.PublicKey)
}

func parentPublicKey(msg Message) string {
	if value := nestedString(msg.Extensions, "parent_public_key"); value != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(nestedString(msg.Extensions, "hd.parent_pubkey"))
}

func hasSourcePage(msg Message) bool {
	return originPublicKey(msg) != ""
}

func containsFold(items []string, target string) bool {
	for _, item := range items {
		if strings.EqualFold(item, target) {
			return true
		}
	}
	return false
}

func nestedString(root map[string]any, keys ...string) string {
	if len(keys) == 0 {
		return ""
	}
	var current any = root
	for _, key := range keys {
		next, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = next[key]
	}
	return stringValue(current)
}

func stringFromMap(root map[string]any, key string) (string, bool) {
	value := stringValue(root[key])
	return value, value != ""
}

func timeFromMap(root map[string]any, key string) (time.Time, bool) {
	value := stringValue(root[key])
	if value == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func intFromMap(root map[string]any, key string) (int, bool) {
	value, ok := root[key]
	if !ok {
		return 0, false
	}
	switch v := value.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	default:
		return 0, false
	}
}

func floatFromMap(root map[string]any, key string) (float64, bool) {
	value, ok := root[key]
	if !ok {
		return 0, false
	}
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	default:
		return 0, false
	}
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func stringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		if typed, ok := value.([]string); ok {
			return uniqueCanonicalTopics(typed)
		}
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text := stringValue(item)
		if text != "" {
			out = append(out, text)
		}
	}
	return uniqueCanonicalTopics(out)
}

func reactionLabel(reactionType string, value *float64, vote int) string {
	switch reactionType {
	case "vote":
		return strconv.Itoa(vote)
	case "truth_score", "source_quality":
		if value == nil {
			return "-"
		}
		return strconv.FormatFloat(*value, 'f', 2, 64)
	default:
		return reactionType
	}
}
