package newsplugin

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"time"
)

const defaultMaxAgeDays = 99999999
const defaultMaxBundleMB = 10
const defaultMaxItemsPerDay int64 = 999999999999

const (
	whitelistModeStrict        = "strict"
	whitelistModeApproval      = "approval"
	defaultPendingApprovalFeed = "pending-approval"
	visibilityStateVisible     = "visible"
	visibilityStatePending     = "pending_approval"
)

const (
	discoveryFeedGlobal  = "global"
	discoveryFeedNews    = "news"
	discoveryFeedLive    = "live"
	discoveryFeedArchive = "archive"
	discoveryFeedNewbies = "new-agents"
)

func LoadSubscriptionRules(path string) (SubscriptionRules, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return SubscriptionRules{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SubscriptionRules{}, nil
		}
		return SubscriptionRules{}, err
	}
	var rules SubscriptionRules
	if err := json.Unmarshal(data, &rules); err != nil {
		return SubscriptionRules{}, err
	}
	rules.normalize()
	return rules, nil
}

func (r *SubscriptionRules) normalize() {
	if r == nil {
		return
	}
	r.WhitelistMode = normalizeWhitelistMode(r.WhitelistMode)
	r.ApprovalFeed = normalizeApprovalFeed(r.ApprovalFeed)
	r.TopicAliases = normalizedTopicAliases(r.TopicAliases)
	whitelist := topicWhitelistSet(r.TopicWhitelist, r.TopicAliases)
	r.ApprovalRoutes = normalizedApprovalRoutes(r.ApprovalRoutes, r.TopicAliases, whitelist)
	r.ApprovalAutoApprove = normalizedApprovalSelectors(r.ApprovalAutoApprove, r.TopicAliases, whitelist)
	r.TopicWhitelist = whitelistToSlice(whitelist)
	r.Channels = uniqueFold(r.Channels)
	r.Topics = uniqueCanonicalTopicsWithAliases(r.Topics, r.TopicAliases, whitelist)
	r.Tags = uniqueFold(r.Tags)
	r.Authors = uniqueFold(r.Authors)
	r.DiscoveryFeeds = uniqueCanonicalDiscoveryFeeds(r.DiscoveryFeeds)
	r.DiscoveryTopics = uniqueCanonicalTopicsWithAliases(r.DiscoveryTopics, r.TopicAliases, whitelist)
	r.HistoryChannels = uniqueFold(r.HistoryChannels)
	r.HistoryTopics = uniqueCanonicalTopicsWithAliases(r.HistoryTopics, r.TopicAliases, whitelist)
	r.HistoryAuthors = uniqueFold(r.HistoryAuthors)
	if r.MaxAgeDays <= 0 {
		r.MaxAgeDays = defaultMaxAgeDays
	}
	if r.MaxBundleMB <= 0 {
		r.MaxBundleMB = defaultMaxBundleMB
	}
	if r.MaxItemsPerDay <= 0 {
		r.MaxItemsPerDay = defaultMaxItemsPerDay
	}
}

func (r SubscriptionRules) Empty() bool {
	r.normalize()
	return len(r.Channels) == 0 && len(r.Topics) == 0 && len(r.Tags) == 0 && len(r.Authors) == 0 &&
		len(r.HistoryChannels) == 0 && len(r.HistoryTopics) == 0 && len(r.HistoryAuthors) == 0 &&
		r.MaxAgeDays >= defaultMaxAgeDays && r.MaxBundleMB >= defaultMaxBundleMB && r.MaxItemsPerDay >= defaultMaxItemsPerDay
}

func ApplySubscriptionRules(index Index, project string, rules SubscriptionRules) Index {
	rules.normalize()
	if rules.Empty() {
		return index
	}
	allowed := make(map[string]struct{})
	pending := make(map[string]struct{})
	dailyCounts := make(map[string]int64)
	for _, bundle := range index.Bundles {
		if bundle.Message.Kind != "post" {
			continue
		}
		if matchesSubscriptionBundle(bundle, rules) {
			if !reserveDailyQuota(dailyCounts, bundle.Message.CreatedAt, rules.MaxItemsPerDay) {
				continue
			}
			allowed[strings.ToLower(bundle.InfoHash)] = struct{}{}
			continue
		}
		if rules.approvalMode() {
			pending[strings.ToLower(bundle.InfoHash)] = struct{}{}
		}
	}
	filtered := make([]Bundle, 0, len(index.Bundles))
	for _, bundle := range index.Bundles {
		switch bundle.Message.Kind {
		case "post":
			infoHash := strings.ToLower(bundle.InfoHash)
			if _, ok := allowed[infoHash]; ok {
				filtered = append(filtered, bundle)
			} else if _, ok := pending[infoHash]; ok {
				filtered = append(filtered, bundle)
			}
		case "reply":
			if bundle.Message.ReplyTo != nil {
				parent := strings.ToLower(bundle.Message.ReplyTo.InfoHash)
				if _, ok := allowed[parent]; ok {
					filtered = append(filtered, bundle)
				} else if _, ok := pending[parent]; ok {
					filtered = append(filtered, bundle)
				}
			}
		case "reaction":
			subject := strings.ToLower(nestedString(bundle.Message.Extensions, "subject", "infohash"))
			if _, ok := allowed[subject]; ok {
				filtered = append(filtered, bundle)
			} else if _, ok := pending[subject]; ok {
				filtered = append(filtered, bundle)
			}
		}
	}
	built := buildIndex(filtered, project)
	return applyVisibilityState(built, allowed, pending, rules)
}

func applyVisibilityState(index Index, allowed, pending map[string]struct{}, rules SubscriptionRules) Index {
	if len(allowed) == 0 && len(pending) == 0 {
		return index
	}
	visiblePosts := make([]Post, 0, len(index.Posts))
	for i := range index.Posts {
		post := index.Posts[i]
		infoHash := strings.ToLower(post.InfoHash)
		if _, ok := pending[infoHash]; ok {
			post.VisibilityState = visibilityStatePending
			post.PendingApproval = true
			post.ApprovalFeed = rules.ApprovalFeed
		} else {
			post.VisibilityState = visibilityStateVisible
		}
		index.Posts[i] = post
		index.PostByInfoHash[infoHash] = post
		if !post.PendingApproval {
			visiblePosts = append(visiblePosts, post)
		}
	}
	index.ChannelStats = ChannelStatsForPosts(visiblePosts)
	index.TopicStats = TopicStatsForPosts(visiblePosts)
	index.SourceStats = SourceStatsForPosts(visiblePosts)
	return index
}

func matchesSubscriptionBundle(bundle Bundle, rules SubscriptionRules) bool {
	rules.normalize()
	whitelist := topicWhitelistSet(rules.TopicWhitelist, rules.TopicAliases)
	if !withinMaxAge(bundle.Message.CreatedAt, rules.MaxAgeDays) {
		return false
	}
	if !withinMaxBundleSize(bundle.SizeBytes, rules.MaxBundleMB) {
		return false
	}
	if rules.Empty() {
		return true
	}
	if containsFold(rules.Topics, reservedTopicAll) {
		return true
	}
	if containsFold(rules.Channels, bundle.Message.Channel) {
		return true
	}
	if containsFold(rules.Authors, bundle.Message.Author) {
		return true
	}
	for _, topic := range uniqueCanonicalTopicsWithAliases(stringSlice(bundle.Message.Extensions["topics"]), rules.TopicAliases, whitelist) {
		if containsFold(rules.Topics, topic) {
			return true
		}
	}
	for _, tag := range bundle.Message.Tags {
		if containsFold(rules.Tags, tag) {
			return true
		}
	}
	return false
}

func withinMaxAge(createdAt string, maxAgeDays int) bool {
	if maxAgeDays <= 0 {
		maxAgeDays = defaultMaxAgeDays
	}
	createdAt = strings.TrimSpace(createdAt)
	if createdAt == "" {
		return true
	}
	parsed, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return true
	}
	maxAge := time.Duration(maxAgeDays) * 24 * time.Hour
	return time.Since(parsed.UTC()) <= maxAge
}

func withinMaxBundleSize(sizeBytes int64, maxBundleMB int) bool {
	if maxBundleMB <= 0 {
		maxBundleMB = defaultMaxBundleMB
	}
	if sizeBytes <= 0 {
		return true
	}
	return sizeBytes <= int64(maxBundleMB)*1024*1024
}

func normalizeWhitelistMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", whitelistModeStrict:
		return whitelistModeStrict
	case whitelistModeApproval:
		return whitelistModeApproval
	default:
		return whitelistModeStrict
	}
}

func normalizeApprovalFeed(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return defaultPendingApprovalFeed
	}
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.ReplaceAll(value, " ", "-")
	if value == "pending" || value == "approval" {
		return defaultPendingApprovalFeed
	}
	return value
}

func (r SubscriptionRules) approvalMode() bool {
	return normalizeWhitelistMode(r.WhitelistMode) == whitelistModeApproval
}

func reserveDailyQuota(counts map[string]int64, createdAt string, maxItemsPerDay int64) bool {
	if maxItemsPerDay <= 0 {
		maxItemsPerDay = defaultMaxItemsPerDay
	}
	dayKey := utcDayKey(createdAt)
	if dayKey == "" {
		return true
	}
	if counts[dayKey] >= maxItemsPerDay {
		return false
	}
	counts[dayKey]++
	return true
}

func utcDayKey(createdAt string) string {
	createdAt = strings.TrimSpace(createdAt)
	if createdAt == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return ""
	}
	return parsed.UTC().Format("2006-01-02")
}

func uniqueFold(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func uniqueCanonicalDiscoveryFeeds(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = canonicalDiscoveryFeed(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func uniqueCanonicalTopics(items []string) []string {
	return uniqueCanonicalTopicsWithAliases(items, nil, nil)
}

func uniqueCanonicalTopicsWithAliases(items []string, aliases map[string]string, whitelist map[string]struct{}) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = canonicalTopicWithAliases(item, aliases)
		if item == "" {
			continue
		}
		if !topicAllowedByWhitelist(item, whitelist) {
			continue
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func canonicalDiscoveryFeed(feed string) string {
	feed = strings.TrimSpace(strings.ToLower(feed))
	feed = strings.TrimPrefix(feed, "hao.news/")
	switch feed {
	case "", "all":
		return discoveryFeedGlobal
	case "new agents", "new-agent", "newagents", "newbie", "newbies", "intro", "introductions", "新手", "报道区", "报到区":
		return discoveryFeedNewbies
	case discoveryFeedGlobal, discoveryFeedNews, discoveryFeedLive, discoveryFeedArchive, discoveryFeedNewbies:
		return feed
	default:
		return feed
	}
}

func canonicalTopic(topic string) string {
	return canonicalTopicWithAliases(topic, nil)
}

func canonicalTopicWithAliases(topic string, aliases map[string]string) string {
	original := strings.TrimSpace(topic)
	if original == "" {
		return ""
	}
	switch strings.ToLower(original) {
	case reservedTopicAll:
		return reservedTopicAll
	case "world", "世界", "国际":
		return "world"
	case "news", "新闻":
		return "news"
	case "futures", "期货":
		return "futures"
	default:
		if aliases != nil {
			if canonical, ok := aliases[strings.ToLower(original)]; ok {
				return canonical
			}
		}
		return original
	}
}

func normalizedTopicAliases(raw map[string]string) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for alias, canonical := range raw {
		alias = strings.ToLower(strings.TrimSpace(alias))
		canonical = canonicalTopic(canonical)
		if alias == "" || canonical == "" {
			continue
		}
		out[alias] = canonical
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func topicWhitelistSet(items []string, aliases map[string]string) map[string]struct{} {
	if len(items) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = canonicalTopicWithAliases(item, aliases)
		if item == "" || item == reservedTopicAll {
			continue
		}
		set[strings.ToLower(item)] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

func whitelistToSlice(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for item := range set {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func normalizedApprovalRoutes(raw map[string]string, aliases map[string]string, whitelist map[string]struct{}) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for selector, reviewer := range raw {
		reviewer = strings.TrimSpace(reviewer)
		if reviewer == "" {
			continue
		}
		selector = strings.TrimSpace(selector)
		if selector == "" {
			continue
		}
		key := strings.ToLower(selector)
		switch {
		case strings.HasPrefix(key, "feed/"):
			feed := canonicalDiscoveryFeed(strings.TrimPrefix(key, "feed/"))
			if feed == "" {
				continue
			}
			out["feed/"+feed] = reviewer
		default:
			topic := key
			if strings.HasPrefix(key, "topic/") {
				topic = strings.TrimPrefix(key, "topic/")
			}
			topic = canonicalTopicWithAliases(topic, aliases)
			if topic == "" || !topicAllowedByWhitelist(topic, whitelist) {
				continue
			}
			out["topic/"+topic] = reviewer
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizedApprovalSelectors(items []string, aliases map[string]string, whitelist map[string]struct{}) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		selector := canonicalApprovalSelector(item, aliases, whitelist)
		if selector == "" {
			continue
		}
		if _, ok := seen[selector]; ok {
			continue
		}
		seen[selector] = struct{}{}
		out = append(out, selector)
	}
	return out
}

func canonicalApprovalSelector(selector string, aliases map[string]string, whitelist map[string]struct{}) string {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return ""
	}
	key := strings.ToLower(selector)
	switch {
	case strings.HasPrefix(key, "feed/"):
		feed := canonicalDiscoveryFeed(strings.TrimPrefix(key, "feed/"))
		if feed == "" {
			return ""
		}
		return "feed/" + feed
	default:
		topic := key
		if strings.HasPrefix(key, "topic/") {
			topic = strings.TrimPrefix(key, "topic/")
		}
		topic = canonicalTopicWithAliases(topic, aliases)
		if topic == "" || !topicAllowedByWhitelist(topic, whitelist) {
			return ""
		}
		return "topic/" + topic
	}
}

func topicAliasPairs(aliases map[string]string) []string {
	if len(aliases) == 0 {
		return nil
	}
	out := make([]string, 0, len(aliases))
	for alias, canonical := range aliases {
		alias = strings.TrimSpace(alias)
		canonical = strings.TrimSpace(canonical)
		if alias == "" || canonical == "" {
			continue
		}
		out = append(out, alias+" -> "+canonical)
	}
	sort.Strings(out)
	return out
}

func topicAllowedByWhitelist(topic string, whitelist map[string]struct{}) bool {
	if len(whitelist) == 0 || strings.EqualFold(topic, reservedTopicAll) {
		return true
	}
	_, ok := whitelist[strings.ToLower(strings.TrimSpace(topic))]
	return ok
}
