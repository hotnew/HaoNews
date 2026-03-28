package newsplugin

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	moderationActionApprove = "approve"
	moderationActionReject  = "reject"
	moderationActionRoute   = "route"
	visibilityStateRejected = "rejected"
)

type ModerationDecision struct {
	InfoHash            string   `json:"infohash"`
	Action              string   `json:"action"`
	TargetFeed          string   `json:"target_feed,omitempty"`
	TargetTopics        []string `json:"target_topics,omitempty"`
	AssignedReviewer    string   `json:"assigned_reviewer,omitempty"`
	AssignedReviewerKey string   `json:"assigned_reviewer_key,omitempty"`
	ActorAuthor         string   `json:"actor_author,omitempty"`
	ActorPublicKey      string   `json:"actor_public_key,omitempty"`
	ActorIdentity       string   `json:"actor_identity,omitempty"`
	Note                string   `json:"note,omitempty"`
	CreatedAt           string   `json:"created_at"`
}

type ModerationDecisionsFile struct {
	UpdatedAt time.Time            `json:"updated_at"`
	Decisions []ModerationDecision `json:"decisions"`
}

func RecentModerationActions(index Index, decisions map[string]ModerationDecision, limit int) []ModerationRecentAction {
	if len(decisions) == 0 || limit == 0 {
		return nil
	}
	items := make([]ModerationRecentAction, 0, len(decisions))
	for infoHash, decision := range decisions {
		if strings.TrimSpace(decision.Action) == "" {
			continue
		}
		title := infoHash
		if post, ok := index.PostByInfoHash[strings.ToLower(strings.TrimSpace(infoHash))]; ok {
			if strings.TrimSpace(post.Message.Title) != "" {
				title = strings.TrimSpace(post.Message.Title)
			}
		}
		items = append(items, ModerationRecentAction{
			InfoHash:         strings.ToLower(strings.TrimSpace(infoHash)),
			Title:            title,
			Action:           strings.TrimSpace(decision.Action),
			ActorIdentity:    strings.TrimSpace(decision.ActorIdentity),
			AssignedReviewer: strings.TrimSpace(decision.AssignedReviewer),
			CreatedAt:        strings.TrimSpace(decision.CreatedAt),
			Note:             strings.TrimSpace(decision.Note),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		left, errLeft := time.Parse(time.RFC3339, items[i].CreatedAt)
		right, errRight := time.Parse(time.RFC3339, items[j].CreatedAt)
		if errLeft == nil && errRight == nil && !left.Equal(right) {
			return left.After(right)
		}
		if items[i].CreatedAt != items[j].CreatedAt {
			return items[i].CreatedAt > items[j].CreatedAt
		}
		return items[i].InfoHash < items[j].InfoHash
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items
}

func ModerationDecisionsPath(writerPolicyPath string) string {
	root := strings.TrimSpace(filepath.Dir(strings.TrimSpace(writerPolicyPath)))
	if root == "" || root == "." {
		return ""
	}
	return filepath.Join(root, "moderation_decisions.json")
}

func LoadModerationDecisions(path string) (map[string]ModerationDecision, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return map[string]ModerationDecision{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]ModerationDecision{}, nil
		}
		return nil, err
	}
	var payload ModerationDecisionsFile
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	decisions := make(map[string]ModerationDecision, len(payload.Decisions))
	for _, decision := range payload.Decisions {
		infoHash := strings.ToLower(strings.TrimSpace(decision.InfoHash))
		if infoHash == "" {
			continue
		}
		decision.InfoHash = infoHash
		decision.Action = canonicalModerationAction(decision.Action)
		decision.AssignedReviewer = strings.TrimSpace(decision.AssignedReviewer)
		decision.AssignedReviewerKey = strings.ToLower(strings.TrimSpace(decision.AssignedReviewerKey))
		decision.TargetTopics = uniqueCanonicalTopics(decision.TargetTopics)
		if decision.Action == "" {
			continue
		}
		decisions[infoHash] = decision
	}
	return decisions, nil
}

func SaveModerationDecisions(path string, decisions map[string]ModerationDecision) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("moderation decisions path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	items := make([]ModerationDecision, 0, len(decisions))
	for _, decision := range decisions {
		if decision.Action == "" || strings.TrimSpace(decision.InfoHash) == "" {
			continue
		}
		items = append(items, decision)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].InfoHash < items[j].InfoHash
	})
	payload := ModerationDecisionsFile{
		UpdatedAt: time.Now().UTC(),
		Decisions: items,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func canonicalModerationAction(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case moderationActionApprove:
		return moderationActionApprove
	case moderationActionReject:
		return moderationActionReject
	case moderationActionRoute:
		return moderationActionRoute
	default:
		return ""
	}
}

func applyModerationDecisions(index Index, decisions map[string]ModerationDecision) Index {
	if len(decisions) == 0 {
		return index
	}
	visiblePosts := make([]Post, 0, len(index.Posts))
	for i := range index.Posts {
		post := index.Posts[i]
		if decision, ok := decisions[strings.ToLower(post.InfoHash)]; ok {
			post.ModerationAction = decision.Action
			post.ModerationActor = strings.TrimSpace(decision.ActorAuthor)
			post.ModerationActorKey = strings.TrimSpace(decision.ActorPublicKey)
			post.ModerationIdentity = strings.TrimSpace(decision.ActorIdentity)
			post.ModerationAt = strings.TrimSpace(decision.CreatedAt)
			post.AssignedReviewer = strings.TrimSpace(decision.AssignedReviewer)
			post.AssignedReviewerKey = strings.TrimSpace(decision.AssignedReviewerKey)
			switch decision.Action {
			case moderationActionApprove:
				post.PendingApproval = false
				post.VisibilityState = visibilityStateVisible
				post.ApprovedFeed = strings.TrimSpace(decision.TargetFeed)
				post.ApprovedTopics = append([]string(nil), decision.TargetTopics...)
				if decision.TargetFeed != "" {
					post.ChannelGroup = strings.TrimSpace(decision.TargetFeed)
				}
				if len(decision.TargetTopics) > 0 {
					post.Topics = append([]string(nil), decision.TargetTopics...)
				}
			case moderationActionReject:
				post.PendingApproval = false
				post.VisibilityState = visibilityStateRejected
			case moderationActionRoute:
				post.PendingApproval = true
				post.VisibilityState = visibilityStatePending
			}
		}
		index.Posts[i] = post
		index.PostByInfoHash[strings.ToLower(post.InfoHash)] = post
		if post.VisibilityState == visibilityStateVisible {
			visiblePosts = append(visiblePosts, post)
		}
	}
	index.ChannelStats = ChannelStatsForPosts(visiblePosts)
	index.TopicStats = TopicStatsForPosts(visiblePosts)
	index.SourceStats = SourceStatsForPosts(visiblePosts)
	return index
}

func mergeAutoApproveDecisions(index Index, decisions map[string]ModerationDecision, rules SubscriptionRules) map[string]ModerationDecision {
	if len(rules.ApprovalAutoApprove) == 0 {
		return decisions
	}
	if decisions == nil {
		decisions = map[string]ModerationDecision{}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, post := range index.Posts {
		if !post.PendingApproval {
			continue
		}
		infoHash := strings.ToLower(strings.TrimSpace(post.InfoHash))
		if infoHash == "" {
			continue
		}
		if _, ok := decisions[infoHash]; ok {
			continue
		}
		if !matchesAutoApproveSelector(post, rules.ApprovalAutoApprove) {
			continue
		}
		decisions[infoHash] = ModerationDecision{
			InfoHash:       infoHash,
			Action:         moderationActionApprove,
			TargetFeed:     strings.TrimSpace(post.ChannelGroup),
			TargetTopics:   append([]string(nil), post.Topics...),
			ActorIdentity:  "auto-approve",
			ActorAuthor:    "local://approval-rules",
			ActorPublicKey: "",
			CreatedAt:      now,
			Note:           "approval_auto_approve",
		}
	}
	return decisions
}

func matchesAutoApproveSelector(post Post, selectors []string) bool {
	if len(selectors) == 0 {
		return false
	}
	for _, selector := range selectors {
		selector = strings.ToLower(strings.TrimSpace(selector))
		switch {
		case strings.HasPrefix(selector, "feed/"):
			if "feed/"+strings.ToLower(strings.TrimSpace(post.ChannelGroup)) == selector {
				return true
			}
		case strings.HasPrefix(selector, "topic/"):
			target := strings.TrimPrefix(selector, "topic/")
			for _, topic := range post.Topics {
				if strings.EqualFold(strings.TrimSpace(topic), target) {
					return true
				}
			}
		}
	}
	return false
}
