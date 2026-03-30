package haonews

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
const defaultHistoryDays = 7
const defaultHistoryMaxItems = 500

const (
	discoveryFeedGlobal   = "global"
	discoveryFeedNews     = "news"
	discoveryFeedLive     = "live"
	discoveryFeedArchive  = "archive"
	discoveryFeedNewbies  = "new-agents"
	whitelistModeStrict   = "strict"
	whitelistModeApproval = "approval"
	defaultApprovalFeed   = "pending-approval"
)

type SyncSubscriptions struct {
	Channels                         []string          `json:"channels"`
	Topics                           []string          `json:"topics"`
	Tags                             []string          `json:"tags"`
	Authors                          []string          `json:"authors,omitempty"`
	AllowedOriginKeys                []string          `json:"allowed_origin_public_keys,omitempty"`
	BlockedOriginKeys                []string          `json:"blocked_origin_public_keys,omitempty"`
	AllowedParentKeys                []string          `json:"allowed_parent_public_keys,omitempty"`
	BlockedParentKeys                []string          `json:"blocked_parent_public_keys,omitempty"`
	LiveAllowedOriginKeys            []string          `json:"live_allowed_origin_public_keys,omitempty"`
	LiveBlockedOriginKeys            []string          `json:"live_blocked_origin_public_keys,omitempty"`
	LiveAllowedParentKeys            []string          `json:"live_allowed_parent_public_keys,omitempty"`
	LiveBlockedParentKeys            []string          `json:"live_blocked_parent_public_keys,omitempty"`
	LivePublicMutedOriginKeys        []string          `json:"live_public_muted_origin_public_keys,omitempty"`
	LivePublicMutedParentKeys        []string          `json:"live_public_muted_parent_public_keys,omitempty"`
	LivePublicRateLimitMessages      int               `json:"live_public_rate_limit_messages,omitempty"`
	LivePublicRateLimitWindowSeconds int               `json:"live_public_rate_limit_window_seconds,omitempty"`
	WhitelistMode                    string            `json:"whitelist_mode,omitempty"`
	ApprovalFeed                     string            `json:"approval_feed,omitempty"`
	AutoRoutePending                 bool              `json:"auto_route_pending,omitempty"`
	ApprovalRoutes                   map[string]string `json:"approval_routes,omitempty"`
	ApprovalAutoApprove              []string          `json:"approval_auto_approve,omitempty"`
	DiscoveryFeeds                   []string          `json:"discovery_feeds,omitempty"`
	DiscoveryTopics                  []string          `json:"discovery_topics,omitempty"`
	TopicWhitelist                   []string          `json:"topic_whitelist,omitempty"`
	TopicAliases                     map[string]string `json:"topic_aliases,omitempty"`
	MaxAgeDays                       int               `json:"max_age_days"`
	MaxBundleMB                      int               `json:"max_bundle_mb"`
	MaxItemsPerDay                   int64             `json:"max_items_per_day"`
	HistoryDays                      int               `json:"history_days,omitempty"`
	HistoryMaxItems                  int               `json:"history_max_items,omitempty"`
	HistoryChannels                  []string          `json:"history_channels,omitempty"`
	HistoryTopics                    []string          `json:"history_topics,omitempty"`
	HistoryAuthors                   []string          `json:"history_authors,omitempty"`
}

func LoadSyncSubscriptions(path string) (SyncSubscriptions, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return SyncSubscriptions{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SyncSubscriptions{}, nil
		}
		return SyncSubscriptions{}, err
	}
	var rules SyncSubscriptions
	if err := json.Unmarshal(data, &rules); err != nil {
		return SyncSubscriptions{}, err
	}
	rules.Normalize()
	return rules, nil
}

func (r *SyncSubscriptions) Normalize() {
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
	r.AllowedOriginKeys = uniqueNormalizedPublicKeys(r.AllowedOriginKeys)
	r.BlockedOriginKeys = uniqueNormalizedPublicKeys(r.BlockedOriginKeys)
	r.AllowedParentKeys = uniqueNormalizedPublicKeys(r.AllowedParentKeys)
	r.BlockedParentKeys = uniqueNormalizedPublicKeys(r.BlockedParentKeys)
	r.LiveAllowedOriginKeys = uniqueNormalizedPublicKeys(r.LiveAllowedOriginKeys)
	r.LiveBlockedOriginKeys = uniqueNormalizedPublicKeys(r.LiveBlockedOriginKeys)
	r.LiveAllowedParentKeys = uniqueNormalizedPublicKeys(r.LiveAllowedParentKeys)
	r.LiveBlockedParentKeys = uniqueNormalizedPublicKeys(r.LiveBlockedParentKeys)
	r.LivePublicMutedOriginKeys = uniqueNormalizedPublicKeys(r.LivePublicMutedOriginKeys)
	r.LivePublicMutedParentKeys = uniqueNormalizedPublicKeys(r.LivePublicMutedParentKeys)
	if r.LivePublicRateLimitMessages < 0 {
		r.LivePublicRateLimitMessages = 0
	}
	if r.LivePublicRateLimitWindowSeconds < 0 {
		r.LivePublicRateLimitWindowSeconds = 0
	}
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
		return defaultApprovalFeed
	}
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.ReplaceAll(value, " ", "-")
	if value == "pending" || value == "approval" {
		return defaultApprovalFeed
	}
	return value
}

func (r SyncSubscriptions) discoveryFeeds() []string {
	r.Normalize()
	if len(r.DiscoveryFeeds) != 0 {
		return append([]string(nil), r.DiscoveryFeeds...)
	}
	return []string{discoveryFeedGlobal, discoveryFeedNews}
}

func (r SyncSubscriptions) discoveryTopics() []string {
	r.Normalize()
	return append([]string(nil), r.DiscoveryTopics...)
}

func (r SyncSubscriptions) Empty() bool {
	r.Normalize()
	return len(r.Channels) == 0 && len(r.Topics) == 0 && len(r.Tags) == 0 && len(r.Authors) == 0 &&
		len(r.AllowedOriginKeys) == 0 && len(r.BlockedOriginKeys) == 0 &&
		len(r.AllowedParentKeys) == 0 && len(r.BlockedParentKeys) == 0 &&
		len(r.LiveAllowedOriginKeys) == 0 && len(r.LiveBlockedOriginKeys) == 0 &&
		len(r.LiveAllowedParentKeys) == 0 && len(r.LiveBlockedParentKeys) == 0 &&
		len(r.LivePublicMutedOriginKeys) == 0 && len(r.LivePublicMutedParentKeys) == 0 &&
		r.LivePublicRateLimitMessages == 0 && r.LivePublicRateLimitWindowSeconds == 0 &&
		len(r.HistoryChannels) == 0 && len(r.HistoryTopics) == 0 && len(r.HistoryAuthors) == 0 &&
		r.MaxAgeDays >= defaultMaxAgeDays && r.MaxBundleMB >= defaultMaxBundleMB && r.MaxItemsPerDay >= defaultMaxItemsPerDay
}

func (r SyncSubscriptions) historyDays() int {
	if r.HistoryDays <= 0 {
		return defaultHistoryDays
	}
	return r.HistoryDays
}

func (r SyncSubscriptions) historyMaxItems() int {
	if r.HistoryMaxItems <= 0 {
		return defaultHistoryMaxItems
	}
	return r.HistoryMaxItems
}

func (r SyncSubscriptions) hasHistorySelectors() bool {
	r.Normalize()
	return len(r.HistoryChannels) > 0 || len(r.HistoryTopics) > 0 || len(r.HistoryAuthors) > 0
}

func matchesHistoryAnnouncement(announcement SyncAnnouncement, rules SyncSubscriptions) bool {
	announcement = normalizeAnnouncement(announcement)
	rules.Normalize()
	if blocked, allowed := matchPublicKeyFilters(announcement.OriginPublicKey, announcement.ParentPublicKey, rules); blocked {
		return false
	} else if allowed {
		return true
	}
	whitelist := topicWhitelistSet(rules.TopicWhitelist, rules.TopicAliases)
	announcement.Topics = uniqueCanonicalTopicsWithAliases(announcement.Topics, rules.TopicAliases, whitelist)
	if !withinMaxAge(announcement.CreatedAt, rules.MaxAgeDays) {
		return false
	}
	if !withinMaxBundleSize(announcement.SizeBytes, rules.MaxBundleMB) {
		return false
	}
	if rules.hasHistorySelectors() {
		if containsFold(rules.HistoryTopics, reservedTopicAll) {
			return true
		}
		if containsFold(rules.HistoryChannels, announcement.Channel) {
			return true
		}
		if containsFold(rules.HistoryAuthors, announcement.Author) {
			return true
		}
		for _, topic := range announcement.Topics {
			if containsFold(rules.HistoryTopics, topic) {
				return true
			}
		}
		return false
	}
	return matchesAnnouncement(announcement, rules)
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

func normalizePublicKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != 64 {
		return ""
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return ""
		}
	}
	return value
}

func uniqueNormalizedPublicKeys(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = normalizePublicKey(item)
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

func matchPublicKeyFilters(originKey, parentKey string, rules SyncSubscriptions) (blocked bool, allowed bool) {
	originKey = normalizePublicKey(originKey)
	parentKey = normalizePublicKey(parentKey)
	if containsFold(rules.BlockedOriginKeys, originKey) {
		return true, false
	}
	if containsFold(rules.BlockedParentKeys, parentKey) {
		return true, false
	}
	if containsFold(rules.AllowedOriginKeys, originKey) {
		return false, true
	}
	if containsFold(rules.AllowedParentKeys, parentKey) {
		return false, true
	}
	return false, false
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
		case strings.HasPrefix(key, "origin/"), strings.HasPrefix(key, "parent/"):
			prefix := "origin/"
			if strings.HasPrefix(key, "parent/") {
				prefix = "parent/"
			}
			publicKey := normalizePublicKey(strings.TrimPrefix(key, prefix))
			if publicKey == "" {
				continue
			}
			out[prefix+publicKey] = reviewer
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
	case strings.HasPrefix(key, "origin/"), strings.HasPrefix(key, "parent/"):
		prefix := "origin/"
		if strings.HasPrefix(key, "parent/") {
			prefix = "parent/"
		}
		publicKey := normalizePublicKey(strings.TrimPrefix(key, prefix))
		if publicKey == "" {
			return ""
		}
		return prefix + publicKey
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

func containsFold(items []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), target) {
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
