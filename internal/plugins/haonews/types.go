package newsplugin

import "time"

type Message struct {
	Protocol   string         `json:"protocol"`
	Kind       string         `json:"kind"`
	Author     string         `json:"author"`
	CreatedAt  string         `json:"created_at"`
	Channel    string         `json:"channel,omitempty"`
	Title      string         `json:"title,omitempty"`
	BodyFile   string         `json:"body_file"`
	BodySHA256 string         `json:"body_sha256"`
	ReplyTo    *MessageLink   `json:"reply_to,omitempty"`
	Tags       []string       `json:"tags,omitempty"`
	Origin     *MessageOrigin `json:"origin,omitempty"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

type MessageLink struct {
	InfoHash string `json:"infohash,omitempty"`
	Magnet   string `json:"magnet,omitempty"`
}

type MessageOrigin struct {
	Author    string `json:"author"`
	AgentID   string `json:"agent_id"`
	KeyType   string `json:"key_type"`
	PublicKey string `json:"public_key"`
	Signature string `json:"signature"`
}

type DelegationInfo struct {
	Delegated       bool
	ParentAgentID   string
	ParentKeyType   string
	ParentPublicKey string
	Scopes          []string
	CreatedAt       string
	ExpiresAt       string
}

type Bundle struct {
	InfoHash          string
	Magnet            string
	SizeBytes         int64
	Dir               string
	ArchiveMD         string
	Message           Message
	Body              string
	CreatedAt         time.Time
	SharedByLocalNode bool
	Delegation        *DelegationInfo
}

type Post struct {
	Bundle
	SourceName           string
	SourceSiteName       string
	SourceURL            string
	OriginPublicKey      string
	ParentPublicKey      string
	HasSourcePage        bool
	EventTime            *time.Time
	Topics               []string
	ChannelGroup         string
	PostType             string
	Summary              string
	ReplyCount           int
	CommentCount         int
	ReactionCount        int
	Upvotes              int
	Downvotes            int
	VoteScore            int
	HotScore             float64
	IsHotCandidate       bool
	VisibilityState      string
	PendingApproval      bool
	ApprovalFeed         string
	ApprovedFeed         string
	ApprovedTopics       []string
	ModerationAction     string
	ModerationActor      string
	ModerationActorKey   string
	ModerationIdentity   string
	ModerationAt         string
	AssignedReviewer     string
	AssignedReviewerKey  string
	SuggestedReviewer    string
	SuggestedReason      string
	TruthScoreAverage    *float64
	SourceScoreAverage   *float64
	LatestReactionAuthor string
}

type Reply struct {
	Bundle
	ParentInfoHash string
}

type Reaction struct {
	Bundle
	SubjectInfoHash string
	ReactionType    string
	VoteValue       int
	ScoreValue      *float64
	Explanation     string
}

type Index struct {
	Bundles         []Bundle
	Posts           []Post
	PostByInfoHash  map[string]Post
	RepliesByPost   map[string][]Reply
	ReactionsByPost map[string][]Reaction
	ChannelStats    []FacetStat
	TopicStats      []FacetStat
	SourceStats     []FacetStat
}

type FacetStat struct {
	Name  string
	Count int
}

type FeedOptions struct {
	Channel         string
	Topic           string
	Source          string
	Reviewer        string
	PendingApproval bool
	Tab             string
	Sort            string
	Query           string
	Window          string
	Page            int
	PageSize        int
	Now             time.Time
}

type FeedFacet struct {
	Name   string
	Count  int
	URL    string
	Active bool
}

type SortOption struct {
	Name   string
	Value  string
	URL    string
	Active bool
}

type TabOption struct {
	Name   string
	Value  string
	URL    string
	Active bool
}

type TimeWindowOption struct {
	Name   string
	Value  string
	URL    string
	Active bool
}

type ActiveFilter struct {
	Label string
	URL   string
}

type SummaryStat struct {
	Label string
	Value string
}

type PageSizeOption struct {
	Name   string
	Value  int
	URL    string
	Active bool
}

type PaginationLink struct {
	Label    string
	URL      string
	Active   bool
	Disabled bool
}

type PaginationState struct {
	Page       int
	PageSize   int
	TotalItems int
	TotalPages int
	FromItem   int
	ToItem     int
	PrevURL    string
	NextURL    string
	Links      []PaginationLink
}

type SubscriptionRules struct {
	Channels                         []string          `json:"channels,omitempty"`
	Topics                           []string          `json:"topics,omitempty"`
	Tags                             []string          `json:"tags,omitempty"`
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
	MaxAgeDays                       int               `json:"max_age_days,omitempty"`
	MaxBundleMB                      int               `json:"max_bundle_mb,omitempty"`
	MaxItemsPerDay                   int64             `json:"max_items_per_day,omitempty"`
	HistoryDays                      int               `json:"history_days,omitempty"`
	HistoryMaxItems                  int               `json:"history_max_items,omitempty"`
	HistoryChannels                  []string          `json:"history_channels,omitempty"`
	HistoryTopics                    []string          `json:"history_topics,omitempty"`
	HistoryAuthors                   []string          `json:"history_authors,omitempty"`

	channelSet           map[string]struct{} `json:"-"`
	topicSet             map[string]struct{} `json:"-"`
	tagSet               map[string]struct{} `json:"-"`
	authorSet            map[string]struct{} `json:"-"`
	allowedOriginKeySet  map[string]struct{} `json:"-"`
	blockedOriginKeySet  map[string]struct{} `json:"-"`
	allowedParentKeySet  map[string]struct{} `json:"-"`
	blockedParentKeySet  map[string]struct{} `json:"-"`
	historyChannelSet    map[string]struct{} `json:"-"`
	historyTopicSet      map[string]struct{} `json:"-"`
	historyAuthorSet     map[string]struct{} `json:"-"`
}

type ArchiveDay struct {
	Date          string
	StoryCount    int
	ReplyCount    int
	ReactionCount int
	URL           string
	Active        bool
}

type ArchiveEntry struct {
	InfoHash   string
	Kind       string
	Title      string
	Author     string
	CreatedAt  time.Time
	ArchiveMD  string
	Day        string
	ThreadURL  string
	ViewerURL  string
	RawURL     string
	Channel    string
	SourceName string
}

type HistoryManifestEntry struct {
	Protocol          string   `json:"protocol"`
	InfoHash          string   `json:"infohash"`
	Ref               string   `json:"ref,omitempty"`
	Magnet            string   `json:"magnet"`
	LibP2PPeerID      string   `json:"libp2p_peer_id,omitempty"`
	SourceHost        string   `json:"source_host,omitempty"`
	SizeBytes         int64    `json:"size_bytes,omitempty"`
	Kind              string   `json:"kind,omitempty"`
	Channel           string   `json:"channel,omitempty"`
	Title             string   `json:"title,omitempty"`
	Author            string   `json:"author,omitempty"`
	CreatedAt         string   `json:"created_at,omitempty"`
	Project           string   `json:"project,omitempty"`
	NetworkID         string   `json:"network_id,omitempty"`
	Topics            []string `json:"topics,omitempty"`
	Tags              []string `json:"tags,omitempty"`
	OriginAuthor      string   `json:"origin_author,omitempty"`
	OriginAgentID     string   `json:"origin_agent_id,omitempty"`
	OriginKeyType     string   `json:"origin_key_type,omitempty"`
	OriginPublicKey   string   `json:"origin_public_key,omitempty"`
	OriginSigned      bool     `json:"origin_signed,omitempty"`
	Delegated         bool     `json:"delegated,omitempty"`
	ParentAgentID     string   `json:"parent_agent_id,omitempty"`
	ParentKeyType     string   `json:"parent_key_type,omitempty"`
	ParentPublicKey   string   `json:"parent_public_key,omitempty"`
	SharedByLocalNode bool     `json:"shared_by_local_node,omitempty"`
}

type HistoryManifestAPIResponse struct {
	Project          string                 `json:"project"`
	Version          string                 `json:"version"`
	NetworkID        string                 `json:"network_id,omitempty"`
	ManifestInfoHash string                 `json:"manifest_infohash,omitempty"`
	GeneratedAt      string                 `json:"generated_at,omitempty"`
	Page             int                    `json:"page,omitempty"`
	PageSize         int                    `json:"page_size,omitempty"`
	TotalEntries     int                    `json:"total_entries,omitempty"`
	TotalPages       int                    `json:"total_pages,omitempty"`
	Cursor           string                 `json:"cursor,omitempty"`
	NextCursor       string                 `json:"next_cursor,omitempty"`
	HasMore          bool                   `json:"has_more,omitempty"`
	EntryCount       int                    `json:"entry_count"`
	Entries          []HistoryManifestEntry `json:"entries"`
}
