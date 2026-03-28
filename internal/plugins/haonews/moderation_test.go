package newsplugin

import "testing"

func TestApplyModerationDecisionsApprovePromotesPendingPost(t *testing.T) {
	t.Parallel()

	index := Index{
		Posts: []Post{
			{
				Bundle:          Bundle{InfoHash: "post-tech"},
				ChannelGroup:    "hao.news/tech",
				Topics:          []string{"technology"},
				VisibilityState: visibilityStatePending,
				PendingApproval: true,
				ApprovalFeed:    defaultPendingApprovalFeed,
			},
		},
		PostByInfoHash: map[string]Post{
			"post-tech": {
				Bundle:          Bundle{InfoHash: "post-tech"},
				ChannelGroup:    "hao.news/tech",
				Topics:          []string{"technology"},
				VisibilityState: visibilityStatePending,
				PendingApproval: true,
				ApprovalFeed:    defaultPendingApprovalFeed,
			},
		},
	}

	index = applyModerationDecisions(index, map[string]ModerationDecision{
		"post-tech": {
			InfoHash:     "post-tech",
			Action:       moderationActionApprove,
			TargetFeed:   "hao.news/news",
			TargetTopics: []string{"world", "usa"},
		},
	})

	post := index.PostByInfoHash["post-tech"]
	if post.PendingApproval {
		t.Fatalf("post still pending after approve")
	}
	if post.VisibilityState != visibilityStateVisible {
		t.Fatalf("visibility state = %q, want %q", post.VisibilityState, visibilityStateVisible)
	}
	if post.ChannelGroup != "hao.news/news" {
		t.Fatalf("channel group = %q, want hao.news/news", post.ChannelGroup)
	}
	if len(post.Topics) != 2 || post.Topics[0] != "world" || post.Topics[1] != "usa" {
		t.Fatalf("topics = %#v, want [world usa]", post.Topics)
	}
	if len(index.Posts) != 1 || index.Posts[0].VisibilityState != visibilityStateVisible {
		t.Fatalf("index posts not updated: %#v", index.Posts)
	}
}

func TestApplyModerationDecisionsRejectHidesPendingPost(t *testing.T) {
	t.Parallel()

	index := Index{
		Posts: []Post{
			{
				Bundle:          Bundle{InfoHash: "post-tech"},
				ChannelGroup:    "hao.news/tech",
				Topics:          []string{"technology"},
				VisibilityState: visibilityStatePending,
				PendingApproval: true,
				ApprovalFeed:    defaultPendingApprovalFeed,
			},
		},
		PostByInfoHash: map[string]Post{
			"post-tech": {
				Bundle:          Bundle{InfoHash: "post-tech"},
				ChannelGroup:    "hao.news/tech",
				Topics:          []string{"technology"},
				VisibilityState: visibilityStatePending,
				PendingApproval: true,
				ApprovalFeed:    defaultPendingApprovalFeed,
			},
		},
	}

	index = applyModerationDecisions(index, map[string]ModerationDecision{
		"post-tech": {
			InfoHash: "post-tech",
			Action:   moderationActionReject,
		},
	})

	post := index.PostByInfoHash["post-tech"]
	if post.PendingApproval {
		t.Fatalf("post still pending after reject")
	}
	if post.VisibilityState != visibilityStateRejected {
		t.Fatalf("visibility state = %q, want %q", post.VisibilityState, visibilityStateRejected)
	}
	if len(index.ChannelStats) != 0 || len(index.TopicStats) != 0 || len(index.SourceStats) != 0 {
		t.Fatalf("expected hidden rejected post to stay out of visible stats")
	}
}

func TestApplyModerationDecisionsRouteKeepsPostPending(t *testing.T) {
	t.Parallel()

	index := Index{
		Posts: []Post{
			{
				Bundle:          Bundle{InfoHash: "post-tech"},
				ChannelGroup:    "tech",
				Topics:          []string{"technology"},
				VisibilityState: visibilityStatePending,
				PendingApproval: true,
				ApprovalFeed:    defaultPendingApprovalFeed,
			},
		},
		PostByInfoHash: map[string]Post{
			"post-tech": {
				Bundle:          Bundle{InfoHash: "post-tech"},
				ChannelGroup:    "tech",
				Topics:          []string{"technology"},
				VisibilityState: visibilityStatePending,
				PendingApproval: true,
				ApprovalFeed:    defaultPendingApprovalFeed,
			},
		},
	}

	index = applyModerationDecisions(index, map[string]ModerationDecision{
		"post-tech": {
			InfoHash:            "post-tech",
			Action:              moderationActionRoute,
			AssignedReviewer:    "reviewer-usa",
			AssignedReviewerKey: "abc123",
		},
	})

	post := index.PostByInfoHash["post-tech"]
	if !post.PendingApproval {
		t.Fatalf("routed post should remain pending")
	}
	if post.VisibilityState != visibilityStatePending {
		t.Fatalf("visibility state = %q, want %q", post.VisibilityState, visibilityStatePending)
	}
	if post.AssignedReviewer != "reviewer-usa" {
		t.Fatalf("assigned reviewer = %q, want reviewer-usa", post.AssignedReviewer)
	}
	if len(index.ChannelStats) != 0 || len(index.TopicStats) != 0 || len(index.SourceStats) != 0 {
		t.Fatalf("expected routed pending post to stay out of visible stats")
	}
}
