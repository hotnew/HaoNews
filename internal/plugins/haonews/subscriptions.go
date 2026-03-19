package newsplugin

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"
)

const defaultMaxAgeDays = 99999999
const defaultMaxBundleMB = 10
const defaultMaxItemsPerDay int64 = 999999999999

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
	r.Channels = uniqueFold(r.Channels)
	r.Topics = uniqueFold(r.Topics)
	r.Tags = uniqueFold(r.Tags)
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
	return len(r.Channels) == 0 && len(r.Topics) == 0 && len(r.Tags) == 0 && r.MaxAgeDays >= defaultMaxAgeDays && r.MaxBundleMB >= defaultMaxBundleMB && r.MaxItemsPerDay >= defaultMaxItemsPerDay
}

func ApplySubscriptionRules(index Index, project string, rules SubscriptionRules) Index {
	rules.normalize()
	if rules.Empty() {
		return index
	}
	allowed := make(map[string]struct{})
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
		}
	}
	filtered := make([]Bundle, 0, len(index.Bundles))
	for _, bundle := range index.Bundles {
		switch bundle.Message.Kind {
		case "post":
			if _, ok := allowed[strings.ToLower(bundle.InfoHash)]; ok {
				filtered = append(filtered, bundle)
			}
		case "reply":
			if bundle.Message.ReplyTo != nil {
				if _, ok := allowed[strings.ToLower(bundle.Message.ReplyTo.InfoHash)]; ok {
					filtered = append(filtered, bundle)
				}
			}
		case "reaction":
			subject := strings.ToLower(nestedString(bundle.Message.Extensions, "subject", "infohash"))
			if _, ok := allowed[subject]; ok {
				filtered = append(filtered, bundle)
			}
		}
	}
	return buildIndex(filtered, project)
}

func matchesSubscriptionBundle(bundle Bundle, rules SubscriptionRules) bool {
	rules.normalize()
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
	for _, topic := range stringSlice(bundle.Message.Extensions["topics"]) {
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
