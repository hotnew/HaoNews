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
	HasSourcePage        bool
	EventTime            *time.Time
	Topics               []string
	ChannelGroup         string
	PostType             string
	Summary              string
	ReplyCount           int
	ReactionCount        int
	VoteScore            int
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
	Channel  string
	Topic    string
	Source   string
	Sort     string
	Query    string
	Window   string
	Page     int
	PageSize int
	Now      time.Time
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
	Channels       []string `json:"channels,omitempty"`
	Topics         []string `json:"topics,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	MaxAgeDays     int      `json:"max_age_days,omitempty"`
	MaxBundleMB    int      `json:"max_bundle_mb,omitempty"`
	MaxItemsPerDay int64    `json:"max_items_per_day,omitempty"`
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
	Magnet            string   `json:"magnet"`
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
	EntryCount       int                    `json:"entry_count"`
	Entries          []HistoryManifestEntry `json:"entries"`
}
